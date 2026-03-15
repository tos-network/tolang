# Tolang → Agent Intent Language: Gap Analysis

## Current State

Tolang already has strong agent-native foundations:

- **Real compiler pipeline**: lexer → parser → sema → lower → IR → codegen → artifacts
- **Core contract model**: contracts, interfaces, inheritance, storage, functions, events, modifiers
- **Agent-native primitives**: `agent` type, `oracle<T>`, `task<T>`, `capability`, `escrow`/`release`/`slash`
- **Rich metadata**: `@effects`, `@bounds`, `@gas`, `@requires`, `@verifiable`, `@delegated`, `@pay`
- **Artifact formats**: `.toc` (compiled), `.abi` (interface), `.tor` (oracle response)
- **Test framework** with assertions

The aspirational target is `AGENT_PROTOCOL_DRAFT2.tol` — a full agent protocol contract that
exercises every agent-native feature. Currently it does not compile. The gaps below are what
stands between tolang today and a complete agent intent language.

---

## G1 — Syntax Quick-Wins

**Status**: Not implemented
**Effort**: Small
**Depends on**: Nothing

| Sub-gap | Description |
|---------|-------------|
| G1a | `deploy` keyword — alias for `new` when instantiating contracts |
| G1b | `manifest {}` extensions — accept `;` separators, numeric values, array values `[A, B]` |
| G1c | `escrow`/`release`/`slash` optional purpose — allow 2-arg form, purpose defaults to 0 |
| G1d | `@pay(amount)` bare form — single-expression shorthand without named keys |

**Why it matters**: These are small syntax gaps that cause DRAFT2 to fail parsing. Each is
independent and can be fixed without touching other features.

**Files**: `tol/lexer/token.go`, `tol/parser/parser.go`, `tol/ast/ast.go`,
`tol/sema/agent.go`, `tol_ir_direct_lowering.go`, `tol_artifact.go`

---

## G2 — Top-level `capability` Declarations

**Status**: Not implemented
**Effort**: Medium
**Depends on**: Nothing

Currently `capability` declarations can only appear inside a `contract` body. Agent protocols
need file-level capabilities shared across multiple contracts in the same module.

**Example** (not yet supported):
```tol
pragma tolang 0.2;

capability Registrar;   // ← top-level, shared across all contracts below
capability Resolver;

contract AgentRegistry {
    /// @requires(caller: Registrar)
    function register(agent a) external { ... }
}
```

**Why it matters**: Agent intent protocols define capabilities at the protocol level, not per
contract. Without this, every contract must redundantly declare the same capabilities.

**Files**: `tol/ast/ast.go` (`Module.Capabilities`), `tol/parser/parser.go` (`parseModule`),
`tol/sema/sema.go` (merge module caps into contract check), `tol/lower/lower.go` (propagate)

---

## G3 — `oracle<T>` OOP Member Interface

**Status**: Not implemented
**Effort**: Medium
**Depends on**: Existing oracle prelude

Oracle storage slots currently work as opaque values. Agent protocols need OOP-style access:

| Member | Type | Description |
|--------|------|-------------|
| `.fulfill(v)` | method | Set the oracle value (authorized caller only) |
| `.is_set` | property | Whether the oracle has been fulfilled |
| `.value` | property | Read the current oracle value |

**Example** (not yet supported):
```tol
oracle<uint256> price;

/// @requires(caller: Resolver)
function resolve(uint256 v) external {
    price.fulfill(v);          // ← method call on oracle slot
}

function getPrice() external view returns (uint256) {
    require(price.is_set);     // ← property read on oracle slot
    return price.value;        // ← property read on oracle slot
}
```

**Why it matters**: Oracles are the bridge between off-chain agent decisions and on-chain state.
Without OOP access, oracle interaction requires low-level storage manipulation, breaking the
agent-native abstraction.

**Files**: `tol/sema/sema.go` (whitelist oracle members), `tol_ir_direct_lowering.go`
(new `lowerOracleSlotExpr` function)

---

## G4 — `task<T>` OOP Interface

**Status**: Not implemented
**Effort**: Medium-Large
**Depends on**: G4a → G4b → G4c (internal ordering)

Tasks are the core economic primitive — they represent work units with lifecycle state
transitions, exactly matching MetaWorld's intent model. Three sub-phases:

### G4a: `mapping(K => task<T>)` as valid storage type

Accept task mappings in storage declarations. The sema layer already partially supports this
but the lowering needs sub-slot hash generation for task fields (poster, worker, reward,
deadline, data).

### G4b: `tasks[tid].method()` — mapping-element method calls

| Method | State Transition | Description |
|--------|-----------------|-------------|
| `.accept(worker)` | Open → Accepted | Worker claims the task |
| `.submit(data)` | Accepted → Submitted | Worker submits deliverable |
| `.approve()` | Submitted → Approved | Poster approves the work |
| `.reject()` | Submitted → Rejected | Poster rejects the work |
| `.dispute()` | → Disputed | Either party disputes |
| `.cancel()` | → Cancelled | Cancel the task |

Also includes property reads: `.worker`, `.poster`, `.reward`, `.is_expired`

### G4c: Task local variable handles

```tol
task<bytes32> t = tasks[task_id];   // ← bind to local
t.accept(msg.sender);              // ← call method on local handle
```

**Why it matters**: Tasks are the on-chain representation of agent intents. The full lifecycle
(post → accept → submit → approve → pay) maps directly to MetaWorld's economic loop
(intent → match → execute → review → settle). Without task OOP, agent contracts cannot
express work coordination natively.

**Files**: `tol/sema/sema.go`, `tol_ir_direct_lowering.go` (new `lowerTaskMappingCallExpr`,
extended prelude with sub-slot hashes and `__tol_task_new` helper)

---

## G5 — `agent` Property Access

**Status**: Not implemented
**Effort**: Medium
**Depends on**: Nothing

Agent-typed values need readable properties for on-chain agent state inspection:

| Property | Type | Description |
|----------|------|-------------|
| `.stake` | `uint256` | Agent's staked amount |
| `.is_active` | `bool` | Whether agent is registered, not suspended, and meets minimum stake |
| `.reputation` | `uint256` | Agent's accumulated reputation score |
| `.rating_count` | `uint256` | Number of ratings received |
| `.suspended` | `bool` | Whether agent is suspended |

**Example** (not yet supported):
```tol
function isActive() external view returns (bool) {
    return agent(msg.sender).is_active;
}

function getStake() external view returns (uint256) {
    return agent(msg.sender).stake;
}
```

**Why it matters**: Agent reputation and stake are the economic trust signals that drive
matching decisions. Without property access, contracts cannot reason about agent
trustworthiness — the core requirement for autonomous economic agents.

**Files**: `tol/sema/sema.go` (whitelist agent properties), `tol_ir_direct_lowering.go`
(new `lowerAgentPropertyExpr`, prelude `__tol_agent_prop` helper)

---

## Implementation Order

```
G1 (quick wins)  ──→  independent, do first
G2 (top-level caps)  ──→  independent
G3 (oracle OOP)  ──→  independent
G4a (task mapping type)  ──→  G4b (task methods)  ──→  G4c (task locals)
G5 (agent props)  ──→  independent
```

G1 should be done first (unblocks DRAFT2 parsing). G2, G3, G5 are independent and can be
parallelized. G4 is the largest and has internal ordering constraints.

## Verification Target

All gaps are closed when `AGENT_PROTOCOL_DRAFT2.tol` compiles with zero errors:

```bash
go run ./cmd/tolang compile --emit toc -o /tmp/draft2.toc docs/AGENT_PROTOCOL_DRAFT2.tol
```

---

## Detailed Implementation Plan

See `DRAFT2_GAP_CLOSURE_PLAN.md` for file-by-file implementation instructions, code snippets,
and per-gap verification scripts.
