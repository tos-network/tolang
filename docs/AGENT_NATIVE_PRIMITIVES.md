# Agent-Native TOL: Oracle/Vote/Task VM Primitives + .toc ABI Extensions

## Context

This document records the design and implementation of the tolang-side agent-native primitives.
The gtos infrastructure (agent/capability/delegation/reputation packages, AA two-phase,
`lvm.go` `tos.agentload`/`tos.hascapability`/escrow primitives) is assumed complete.
This spec covers the compiler and language additions.

Reference: `docs/AGENT-NATIVE.md` (§III VM Storage Primitives, §VI Compiler & ABI Extensions)

---

## Key Design Decisions

- **Purpose bits are contract-local**: compiler assigns ordinals 0–255 in declaration order (no
  global registry). Emitted as `local __tol_pur_X = N` compile-time Lua locals.
- **Capability bits are global registry**: resolved at runtime via `tos.capabilitybit(name)`,
  cached as `local __tol_cap_X = tos and type(tos.capabilitybit)=="function" and tos.capabilitybit("X") or 0`.
- **Lua prelude fallback pattern**: all new host functions use `tos and type(tos.X)=="function" and tos.X or function(...) ... end`
  — enables offline/test mode without gtos.
- **`oracle<T>`**: two slots (`val_slot`, `set_slot`), single-write enforced by `__tol_oracle_fulfill`
  (atomic SSTORE guard; delegates to `tos.oracle_fulfill` when available).
- **`vote<T>`**: six slots (tally/eligible/voted-mapping-base/threshold/deadline/result).
  Eligible count snapshotted at `__tol_vote_new` time from `tos.totaleligible(cap_bit)`.
- **`task<T>`**: state machine with 7 states (None=0/Open=1/Accepted=2/Submitted=3/Approved=4/
  Rejected=5/Disputed=6/Cancelled=7). Transitions enforced by `__tol_task_transition`
  (delegates to `tos.task_transition` atomic CAS when available).
- **task ID**: `keccak256(poster_bytes ++ block.number_bytes)` — unique per-poster-per-block.
- **`agent` type**: TOL's native identity type. Reading a slot of type `agent` as an expression
  compiles to an identity value; casting `agent(expr)` inlines a guard via `tos.agentload`.
- **Manifest block**: compile-time key/value metadata emitted verbatim into `.toc` ABI JSON
  under a `"manifest"` top-level key.

---

## New TOL Language Syntax

### 1. Capability Declarations

```tol
capability Registrar;
capability Auditor;
```

Declares a named capability. The compiler emits a Lua local that resolves the bit at runtime:

```lua
local __tol_cap_Registrar = tos and type(tos.capabilitybit)=="function" and tos.capabilitybit("Registrar") or 0
```

Capability names must be unique within a contract (TOL2300). Max: unlimited (runtime uint256 bitmap).

### 2. Purpose Declarations

```tol
purpose WorkEscrow;
purpose SlashReserve;
```

Declares a named escrow purpose. The compiler assigns ordinals in declaration order:

```lua
local __tol_pur_WorkEscrow  = 0
local __tol_pur_SlashReserve = 1
```

Max 256 purposes per contract (ordinals 0–255, TOL2312). Names must be unique (TOL2301).

### 3. Manifest Block

```tol
manifest {
    name: "TaskBoard",
    version: "1.0.0",
    description: "Agent-native task board"
}
```

Emitted verbatim into the `.toc` ABI JSON as a top-level `"manifest"` object. Keys `name`,
`version`, `description` map to named fields; all other keys go into `"extra"`.
Only one manifest block per contract (TOL2307).

### 4. Agent-Native Storage Types

#### `oracle<T>` — Write-once value slot

```tol
oracle<uint256> priceOracle;
```

Expands to two compile-time slot hash constants:

```lua
local __tol_s_priceOracle_val = "0x..."  -- keccak256("tol.oracle.<C>.priceOracle.value")
local __tol_s_priceOracle_set = "0x..."  -- keccak256("tol.oracle.<C>.priceOracle.set")
```

Helper functions injected once per contract:

```lua
local __tol_oracle_fulfill = tos and type(tos.oracle_fulfill)=="function" and tos.oracle_fulfill or
  function(val_slot, set_slot, value)
    if __tol_sload(set_slot) ~= 0 then error("OracleAlreadySet") end
    __tol_sstore(set_slot, 1)
    __tol_sstore(val_slot, value)
  end
local __tol_oracle_is_set = function(set_slot) return __tol_sload(set_slot) ~= 0 end
local __tol_oracle_value  = function(val_slot) return __tol_sload(val_slot) end
```

Type restriction: `T` must be a value type (no mapping/array) — TOL2303.

#### `vote<T>` — On-chain ballot

```tol
vote<uint256> governance;
```

Expands to six compile-time slot hash constants:

```lua
local __tol_s_governance_tally     = "0x..."  -- running tally
local __tol_s_governance_eligible  = "0x..."  -- snapshot of eligible voter count
local __tol_s_governance_voted     = "0x..."  -- mapping base (voted[addr])
local __tol_s_governance_threshold = "0x..."  -- quorum threshold
local __tol_s_governance_deadline  = "0x..."  -- deadline block/timestamp
local __tol_s_governance_result    = "0x..."  -- final result
```

Helper functions injected once per contract:

```lua
local __tol_vote_new     = function(elig_slot, thresh_slot, ddl_slot, cap_bit, threshold, deadline) ... end
local __tol_vote_cast    = tos and type(tos.vote_cast)=="function" and tos.vote_cast or function(...) ... end
local __tol_vote_tally   = function(tally_slot) return __tol_sload(tally_slot) end
local __tol_vote_decided = function(tally_slot, thresh_slot) ... end
```

Type restriction: `T` must be numeric (uint/int/bool) — TOL2304.

#### `task<T>` — Task state machine

```tol
task<JobSpec> jobs;
```

Expands to a single mapping-base slot hash constant:

```lua
local __tol_s_jobs_base = "0x..."  -- keccak256("tol.task.<C>.jobs")
```

Task state is stored at `__tol_mkey(tid, __tol_s_jobs_base)`.

Helper functions injected once per contract:

```lua
local __tol_task_post = function(task_base, poster)
  local tid = keccak256(tostring(poster) .. tostring(block and block.number or 0))
  local state_slot = __tol_mkey(tid, task_base)
  __tol_sstore(state_slot, 1)  -- Open
  return tid
end
local __tol_task_transition = tos and type(tos.task_transition)=="function" and tos.task_transition or
  function(task_base, tid, from_state, to_state, guard_addr)
    local slot = __tol_mkey(tid, task_base)
    local cur = __tol_sload(slot)
    if cur ~= from_state then error("TaskInvalidTransition") end
    if guard_addr ~= nil and guard_addr ~= 0 and guard_addr ~= msg.sender then error("TaskUnauthorized") end
    __tol_sstore(slot, to_state)
  end
local __tol_task_state = function(task_base, tid) return __tol_sload(__tol_mkey(tid, task_base)) end
```

Task state enum:

| Value | Name      |
|-------|-----------|
| 0     | None      |
| 1     | Open      |
| 2     | Accepted  |
| 3     | Submitted |
| 4     | Approved  |
| 5     | Rejected  |
| 6     | Disputed  |
| 7     | Cancelled |

Type restriction: `T` should be a struct type — TOL2305.

#### `agent` — native identity type

```tol
agent worker;
```

Stored as a native agent identity slot. Casting `agent(expr)`
inlines a runtime registration check.

---

## New Doc Annotations

### `@requires(caller: CapName)`

```tol
/// @requires(caller: Registrar)
function postJob(uint256 reward) external returns (uint256 tid) { ... }
```

At function entry, emits:

```lua
if not (tos and type(tos.hascapability)=="function" and tos.hascapability(msg.sender, __tol_cap_Registrar)) then
  error("CapabilityDenied:Registrar")
end
```

Sema validates that `Registrar` is declared with `capability Registrar;` in the same contract (TOL2302).
Emitted in `.toc` ABI JSON as `"requires_capability": "Registrar"`.

Multiple `@requires` lines accumulate: all named capabilities must be held.

### `@pay(amount=expr, recipient=expr)`

```tol
/// @pay(amount=100, recipient=msg.sender)
function claimReward(uint256 tid) external payable returns (uint256) { ... }
```

Documents the payment semantics. Emitted in `.toc` ABI JSON as `"pay_amount_tomi": "100"`.
Sema validates: amount must be a uint256 expression (TOL2308), recipient must be an agent expression (TOL2309).

### `@delegated`

```tol
/// @delegated
function acceptJob(uint256 tid) external payable { ... }
```

Marks a function as accepting delegated calls. Injects `__tol_delegation_verify` prelude.
Emitted in `.toc` ABI JSON as `"delegated": true`.

### `@verifiable`

```tol
/// @verifiable
function getJobState(uint256 tid) external view returns (uint256) { ... }
```

Marks a function whose result can be verified off-chain. Must be `pure` or `view` (TOL2314).
Emitted in `.toc` ABI JSON as `"verifiable": true`.

---

## Slot Hash Derivation

All agent-native slot hashes use the same `keccak256`-based scheme as regular storage slots:

| Slot kind     | Hash input                                          |
|---------------|-----------------------------------------------------|
| oracle value  | `"tol.oracle.<C>.<name>.value"`                     |
| oracle set    | `"tol.oracle.<C>.<name>.set"`                       |
| vote tally    | `"tol.vote.<C>.<name>.tally"`                       |
| vote eligible | `"tol.vote.<C>.<name>.eligible"`                    |
| vote voted    | `"tol.vote.<C>.<name>.voted"` (mapping base)        |
| vote threshold| `"tol.vote.<C>.<name>.threshold"`                   |
| vote deadline | `"tol.vote.<C>.<name>.deadline"`                    |
| vote result   | `"tol.vote.<C>.<name>.result"`                      |
| task base     | `"tol.task.<C>.<name>"` (mapping base, key=task_id) |

`<C>` = contract name, `<name>` = slot name.

---

## .toc ABI Extensions

### Per-Function Fields

```json
{
  "name": "postJob",
  "visibility": "external",
  "selector": "0x2602744a",
  "params": ["u256", "u256"],
  "returns": ["u256"],
  "requires_capability": "Registrar",
  "pay_amount_tomi": "",
  "total_cost_tomi": "",
  "verifiable": false,
  "delegated": false
}
```

| Field                | Type     | Source                             |
|----------------------|----------|------------------------------------|
| `requires_capability`| `string` | `@requires(caller: X)` annotation  |
| `pay_amount_tomi`    | `string` | `@pay(amount=expr)` annotation      |
| `total_cost_tomi`    | `string` | `pay_amount_tomi + gas_bound×10gtomi`|
| `verifiable`         | `bool`   | `@verifiable` annotation           |
| `delegated`          | `bool`   | `@delegated` annotation            |

### Top-Level Manifest Section

```json
{
  "manifest": {
    "name": "TaskBoard",
    "version": "1.0.0",
    "description": "Agent-native task board with escrow",
    "extra": {
      "custom_key": "custom_value"
    }
  }
}
```

Keys `name`, `version`, `description` map to typed fields. All other manifest keys
go into `"extra": { ... }`.

---

## Diagnostics

| Code    | Meaning                                                          |
|---------|------------------------------------------------------------------|
| TOL2300 | Capability already declared in this contract                     |
| TOL2301 | Purpose already declared in this contract                        |
| TOL2302 | `@requires` references undeclared capability                     |
| TOL2303 | `oracle<T>`: type parameter must be a value type                 |
| TOL2304 | `vote<T>`: type parameter must be numeric (uint/int/bool)        |
| TOL2305 | `task<T>`: T should be a struct type                             |
| TOL2306 | `agent` cast on non-agent expression                           |
| TOL2307 | Manifest block already declared                                  |
| TOL2308 | `@pay`: amount must be a uint256 expression                      |
| TOL2309 | `@pay`: recipient must be an agent expression                  |
| TOL2310 | `delegation.verify()` outside `@delegated` function             |
| TOL2311 | `escrow`/`release`/`slash` outside payable context              |
| TOL2312 | Purpose ordinal overflow (max 256 purposes per contract)         |
| TOL2313 | Capability name must be a string literal                         |
| TOL2314 | `@verifiable` requires `pure` or `view` function                 |
| TOL2315 | Task state transition invalid (not in allowed set)               |

---

## Files Modified

| File                          | Change                                                           |
|-------------------------------|------------------------------------------------------------------|
| `tol/diag/diag.go`            | TOL2300–TOL2315 constants                                        |
| `tol/ast/ast.go`              | `CapabilityDecl`, `PurposeDecl`, `ManifestDecl`; `ContractDecl` fields; `DocMeta` fields |
| `tol/parser/parser.go`        | `parseCapabilityDecl`, `parsePurposeDecl`, `parseManifestDecl`, `parseAgentTypeSlot`; `@requires`/`@pay`/`@delegated`/`@verifiable` in `parseDocMeta` |
| `tol/sema/sema.go`            | `isValidTOLType` accepts agent-native types; calls `checkAgentNativeDecls` |
| `tol/sema/agent.go`           | `checkAgentNativeDecls` — dup/overflow/unknown-cap/verifiable-purity checks |
| `tol/lower/lower.go`          | `Program.Capabilities/Purposes/Manifest`; `Function.Doc`; `FromTypedContract` populates them |
| `tol_ir_direct_lowering.go`   | `buildAgentNativePrelude`, `buildRequiresCapPreamble`; injected into bootstrap chunk |
| `tol_artifact.go`             | `tocABIManifest`; extended `tocABIFunction`; manifest population in `buildArtifactMetadataForContract` |

---

## Example

```tol
pragma tolang 0.2.0;

struct JobSpec {
    uint256 reward;
    uint256 deadline;
}

contract TaskBoard {
    capability Registrar;
    purpose WorkEscrow;

    manifest {
        name: "TaskBoard",
        version: "1.0.0"
    }

    task<JobSpec> jobs;
    oracle<uint256> priceOracle;

    /// @requires(caller: Registrar)
    function postJob(uint256 reward, uint256 deadline) external returns (uint256 tid) {
        return __tol_task_post(__tol_s_jobs_base, msg.sender);
    }

    /// @delegated
    function acceptJob(uint256 tid) external payable {
        __tol_task_transition(__tol_s_jobs_base, tid, 1, 2, agent(0));
    }

    /// @verifiable
    function getJobState(uint256 tid) external view returns (uint256 state) {
        return __tol_task_state(__tol_s_jobs_base, tid);
    }
}
```

Generated ABI excerpt:

```json
{
  "functions": [
    { "name": "postJob",      "requires_capability": "Registrar" },
    { "name": "acceptJob",    "delegated": true },
    { "name": "getJobState",  "verifiable": true }
  ],
  "manifest": { "name": "TaskBoard", "version": "1.0.0" }
}
```
