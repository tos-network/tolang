# ABI Specification
## Agent Behavior Interface for Tolang and TOS Network

**Status:** Stable
**Version:** 1.0
**Intended location:** `docs/ABI_SPEC.md`

## 1. Purpose

ABI (Agent Behavior Interface) is the machine-readable interface standard for Tolang contracts and packages.

Traditional ABI tells a caller:

- what function exists,
- how to encode inputs,
- how to decode outputs.

ABI extends this model so that autonomous agents can also know:

- what authority is required,
- what side effects may occur,
- what gas or resource envelope applies,
- whether the function is safe to compose,
- whether delegation is accepted,
- whether payment or escrow is required,
- whether the output is verifiable,
- and what policy conditions govern execution.

ABI is intended to make Tolang contracts discoverable, analyzable, and automatable in the Agent Economy.

## 2. Design Goals

ABI is designed to satisfy six goals:

1. **Machine readability**
   The interface must be directly consumable by agents, SDKs, verifiers, wallets, marketplaces, and policy engines.

2. **Bytecode-bound trust**
   ABI metadata should be emitted by the compiler and bound to the deployed bytecode hash where applicable.

3. **Safety before execution**
   A caller should be able to understand capability requirements, side effects, and resource bounds before making a call.

4. **Extensibility**
   ABI should support task, oracle, proof, attestation, escrow, and future agent-market patterns without breaking existing consumers.

5. **Deterministic semantics**
   Fields that affect settlement, authorization, and execution safety must have deterministic meaning.

6. **Versioned compatibility**
   ABI must include explicit versioning to allow forward evolution.

## 3. Relationship to the Traditional ABI

Tolang's ABI (Agent Behavior Interface) is a strict superset of the traditional ABI (Application Binary Interface) used in Solidity and EVM-compatible systems.

- The **traditional ABI** (Application Binary Interface) describes call encoding and decoding: function selectors, input types, and output types.
- The **Agent Behavior Interface** extends this to describe the economic, authority, and execution semantics that surround each call.

An Agent Behavior Interface record may embed a traditional ABI-compatible object, but a traditional ABI object alone is not a complete Agent Behavior Interface description.

## 4. Top-Level ABI Object

A contract or package should expose a top-level ABI object like this:

```json
{
  "abi_version": "1.0",
  "language": "tolang",
  "language_version": "0.3",
  "bytecode_hash": "0x...",
  "source_hash": "0x...",
  "contract_name": "ExampleAgentService",
  "kind": "contract",
  "manifest": {
    "name": "ExampleAgentService",
    "version": "1.0.0",
    "description": "Task settlement endpoint for agent jobs"
  },
  "capabilities": [
    "AGENT_EXECUTE",
    "TASK_SETTLE"
  ],
  "purposes": [
    "task.submit",
    "task.fulfill"
  ],
  "functions": []
}
```

## 5. Core Fields

### 5.1 Required fields

The following fields are required in all ABI objects:

- `abi_version`
- `language`
- `language_version`
- `bytecode_hash` (where deployed/runtime artifact exists)
- `contract_name`
- `functions`

### 5.2 Recommended fields

The following fields are strongly recommended:

- `source_hash`
- `manifest`
- `capabilities`
- `purposes`
- `security_profile`
- `notes`

## 6. Function-Level ABI Schema

Each callable function should have an ABI entry.

Example:

```json
{
  "name": "acceptTask",
  "selector": "0x12345678",
  "inputs": [
    {"name": "taskId", "type": "uint256"},
    {"name": "proof", "type": "bytes"}
  ],
  "outputs": [
    {"name": "ok", "type": "bool"}
  ],
  "state_mutability": "nonpayable",
  "requires_capability": ["TASK_ACCEPT"],
  "accepts_delegation": true,
  "delegation_scope": {
    "action": "TASK_ACCEPT",
    "contract": "agent",
    "expiry_ms": 86400000,
    "nonce": 1
  },
  "verifiable": true,
  "proof_schema": "none",
  "payment": {
    "mode": "escrow",
    "asset": "TOS",
    "required": false
  },
  "effects": {
    "reads": ["tasks", "agents"],
    "writes": ["tasks", "escrow"],
    "emits": ["TaskAccepted"],
    "calls": []
  },
  "bounds": {
    "external_calls_max": 0,
    "storage_writes_max": 2
  },
  "gas": {
    "upper": 180000,
    "model": "tol-gas-v1"
  },
  "composability": {
    "non_composable": false
  },
  "failure_modes": [
    "MissingCapability",
    "InvalidTaskState",
    "ProofRejected"
  ]
}
```

## 7. Standard Function Fields

### 7.1 Identity fields

- `name`: canonical function name
- `selector`: 4-byte call selector encoded as `0x`-prefixed hex string
- `inputs`: array of `{"name": string, "type": string}` input descriptors
- `outputs`: array of `{"name": string, "type": string}` output descriptors
- `state_mutability`: one of `"pure"`, `"view"`, `"nonpayable"`, `"payable"`

### 7.2 Authorization fields

- `requires_capability`: array of required capability symbols
- `accepts_delegation`: boolean indicating whether the function accepts delegated calls
- `delegation_scope`: structured scope descriptor for delegated authority. Schema:

  ```json
  {
    "action": "string",
    "contract": "agent",
    "expiry_ms": "u64",
    "nonce": "u64"
  }
  ```

  Field definitions:
  - `action`: the capability or action being delegated (e.g. `"TASK_ACCEPT"`)
  - `contract`: the agent address or identifier of the contract granting delegation. Type is `agent` (the TOL canonical identity type).
  - `expiry_ms`: delegation expiry as a millisecond Unix timestamp. A value of `0` means no expiry.
  - `nonce`: monotonically increasing nonce to prevent replay of revoked delegations.

  This field is required when `accepts_delegation` is `true`. It is omitted when `accepts_delegation` is `false` or absent.

- `caller_kind`: one of `"user"`, `"agent"`, `"contract"`, `"any"`. Defaults to `"any"` if omitted.

### 7.3 Payment fields

- `payment.mode`: one of `"none"`, `"direct"`, `"escrow"`, `"stake"`, `"fee"`
- `payment.asset`: canonical asset symbol or ID (e.g. `"TOS"`)
- `payment.required`: boolean
- `payment.min_amount`: optional numeric lower bound in base units (tomi)
- `payment.settlement_rule`: optional structured rule reference or text description

### 7.4 Verification fields

- `verifiable`: boolean indicating whether the function output can be independently verified
- `proof_kind`: one of `"attestation"`, `"signature"`, `"merkle"`, `"zk"`, `"custom"`. Required when `verifiable` is `true`.
- `proof_schema`: proof protocol identifier. One of:

  - `"none"` -- no proof required
  - `"sigma-range-v1"` -- Schnorr sigma protocol with range proof (used for UNO encrypted balance proofs)
  - `"transcript-binding-v1"` -- Merlin transcript-bound proof with chain context (used for UNO Shield/Transfer/Unshield)

  This field is required when `verifiable` is `true`. When `verifiable` is `false` or absent, this field should be omitted or set to `"none"`.

- `challenge_window`: optional dispute window specified as a number of blocks or milliseconds (e.g. `{"blocks": 100}` or `{"ms": 360000}`)

### 7.5 Effect fields

- `effects.reads`: array of declared state read keys (e.g. `["tasks", "agents"]`)
- `effects.writes`: array of declared state write keys (e.g. `["tasks", "escrow"]`)
- `effects.emits`: array of declared event names (e.g. `["TaskAccepted"]`)
- `effects.calls`: array of external call references, each with schema:

  ```json
  {
    "cap": "string",
    "iface": "string",
    "selector": "0x...",
    "max_gas": "u64",
    "max_calls": "u32",
    "max_depth": "u32",
    "wildcard": "bool"
  }
  ```

- `effects.value_transfer`: boolean or structured transfer descriptor

### 7.6 Bound fields

- `bounds.external_calls_max`: maximum number of external calls
- `bounds.storage_writes_max`: maximum number of storage write operations
- `bounds.storage_reads_max`: maximum number of storage read operations
- `bounds.loop_class`: one of `"bounded"`, `"conservative"`, `"unbounded"`
- `bounds.max_output_size`: optional maximum output size in bytes

### 7.7 Gas fields

- `gas.upper`: worst-case gas cost estimate (numeric)
- `gas.model`: gas model version string (e.g. `"tolang/0.2.0"`)
- `gas.conservative`: boolean indicating whether the estimate is conservative

### 7.8 Composability fields

- `composability.non_composable`: boolean
- `composability.reason`: human-readable reason string (required when `non_composable` is `true`)
- `composability.reentrant_sensitive`: optional boolean

### 7.9 Failure semantics

- `failure_modes`: array of symbolic failure identifiers (e.g. `["MissingCapability", "InvalidTaskState"]`)
- `revert_schema`: optional structured revert descriptor referencing custom error types

### 7.10 Error Model

TOL distinguishes three error types at the codegen level. Each produces a structured error value (Lua table) with a `selector` field, enabling the runtime and agents to differentiate error origins.

#### Error types

| Source | Selector | ABI Signature | Meaning |
|--------|----------|---------------|---------|
| `require(cond, msg)` | `0x08c379a0` | `Error(string)` | Precondition failure (caller error, recoverable). Used for input validation, permission checks, balance checks. |
| `assert(cond)` | `0x4e487b71` | `Panic(uint256)` | Invariant violation (bug indicator). Should never trigger in correct code. Panic code 1 = assertion failure. |
| `revert "msg"` | `0x08c379a0` | `Error(string)` | Explicit revert with human-readable message. |
| `revert CustomError(args)` | `"custom"` | Custom error ABI | Explicit revert with typed custom error. The `data` field contains the ABI-encoded error payload. |

#### Runtime representation

All three error types produce a Lua table passed to `error()`:

```lua
-- require(false, "insufficient balance")
error({selector = "0x08c379a0", msg = "insufficient balance"})

-- assert(false)
error({selector = "0x4e487b71", code = 1})

-- revert "something went wrong"
error({selector = "0x08c379a0", msg = "something went wrong"})

-- revert InsufficientBalance(amount, required)
error({selector = "custom", data = <abi-encoded-payload>})
```

#### try-catch interaction

The `try-catch` statement uses Lua `pcall` to catch errors. Currently `pcall` catches all errors indiscriminately, including Lua runtime panics (stack overflow, OOM). Ideally, catch should only intercept typed contract errors (tables with a `selector` field) and let Lua runtime panics propagate. This limitation is documented and deferred to a future change.

#### Agent implications

An agent inspecting the ABI can determine from `failure_modes` and error selectors whether a function failure is:
- **Recoverable** (`0x08c379a0`): the caller provided invalid input; retry with corrected parameters.
- **A bug** (`0x4e487b71`): the contract has an invariant violation; do not retry.
- **Typed** (`custom`): the contract reverted with a specific error type; decode the payload for details.

## 8. Contract-Level Fields

The top-level ABI object may also describe contract-wide semantics.

### 8.1 Manifest

The `manifest` field should capture human-meaningful metadata that is also machine-readable.

Suggested fields:

- `name`
- `version`
- `description`
- `homepage`
- `license`
- `service_kind`
- `agent_role`

### 8.2 Capability declarations

`capabilities` should list capability symbols declared or consumed by the contract.

### 8.3 Purpose declarations

`purposes` should list the purpose bits or purpose labels assigned by the compiler/runtime model.

### 8.4 Account profile

For account-like contracts or agent accounts, the ABI object may include:

```json
{
  "account_profile": {
    "is_account": true,
    "validation_mode": "aa",
    "delegation_supported": true
  }
}
```

## 9. Agent-Specific Extensions

ABI should support agent-native endpoint descriptors.

### 9.1 Agent descriptor

```json
{
  "agent_descriptor": {
    "registry_backed": true,
    "reputation_source": "AgentRegistry",
    "stake_source": "AgentRegistry"
  }
}
```

### 9.2 Task endpoint descriptor

The `task_descriptor` describes task-processing semantics for an endpoint. The `state_machine` field is a generic, contract-defined state machine descriptor. Contracts define their own states and transitions; there is no hardcoded task state machine in the language.

```json
{
  "task_descriptor": {
    "task_type": "generic",
    "state_machine": {
      "version": "1.0",
      "states": ["open", "assigned", "submitted", "verified", "settled", "disputed"],
      "transitions": [
        {"from": "open", "to": "assigned"},
        {"from": "assigned", "to": "submitted"},
        {"from": "submitted", "to": "verified"},
        {"from": "submitted", "to": "disputed"},
        {"from": "verified", "to": "settled"},
        {"from": "disputed", "to": "settled"}
      ]
    },
    "escrow_enabled": true,
    "proof_required": true
  }
}
```

State machine schema:

```json
{
  "version": "1.0",
  "states": ["string"],
  "transitions": [
    {"from": "string", "to": "string"}
  ]
}
```

Field definitions:

- `version`: state machine schema version (currently `"1.0"`)
- `states`: array of unique state name strings
- `transitions`: array of valid transitions, each with a `from` state and a `to` state. Both must reference entries in `states`.

Contracts are free to define any set of states and transitions. The ABI consumer uses this descriptor for preflight analysis and UI rendering, not for enforcement (enforcement is in the contract logic).

### 9.3 Oracle endpoint descriptor

```json
{
  "oracle_descriptor": {
    "write_once": true,
    "fulfillment_mode": "authorized-writer",
    "proof_required": false
  }
}
```

## 10. Versioning

Every ABI object must declare `abi_version`.

Compatibility rules:

- Minor additions must be backward compatible.
- A consumer must ignore unknown fields unless a field is marked critical.
- Breaking changes require a new major version.

Optional field:

```json
{
  "critical_fields": ["proof_kind", "payment.mode"]
}
```

## 11. ABI Compatibility Guarantee

Starting with version 1.0, the following compatibility rules are guaranteed:

1. **Field additions are backward-compatible.** New optional fields may be added in minor version increments (e.g. 1.0 to 1.1). Existing consumers must ignore unknown fields gracefully.

2. **Field removals require a major version bump.** Removing a field that existed in a prior stable version requires incrementing the major version (e.g. 1.x to 2.0). Consumers of the prior version must be given a deprecation notice at least one minor version in advance.

3. **Type changes require a major version bump.** Changing the type of an existing field (e.g. from `string` to `object`, from `bool` to `enum`) is a breaking change and requires a major version increment.

4. **Semantic changes require a major version bump.** Changing the meaning of an existing field value (e.g. redefining what `"escrow"` means in `payment.mode`) is a breaking change.

5. **Enum extensions are backward-compatible.** Adding new values to an existing enum field (e.g. adding a new `proof_schema` variant) is a minor change. Consumers must handle unknown enum values gracefully, either by treating them as opaque strings or by rejecting the ABI with a clear version-mismatch diagnostic.

6. **The `abi_version` field is immutable.** Its type (`string`), position (top-level), and semantics (major.minor version identifier) will never change.

## 12. Error Model

Tolang defines three error mechanisms with distinct selectors and semantics. ABI consumers (agents, wallets, indexers) use the selector to classify failures without parsing the error payload.

### 12.1 `require` -- Precondition Failure

**Selector:** `0x08c379a0` (`Error(string)`)

`require(condition, message)` checks caller-supplied preconditions: input validation, permission checks, balance sufficiency. A `require` failure indicates the caller made an invalid request. The error is recoverable -- the caller can retry with corrected inputs.

ABI encoding: the 4-byte selector `0x08c379a0` followed by the ABI-encoded `string` message.

```
0x08c379a0
+ abi.encode(string message)
```

### 12.2 `assert` -- Invariant Violation

**Selector:** `0x4e487b71` (`Panic(uint256)`)

`assert(condition)` checks internal invariants that must always hold in correct code. An `assert` failure indicates a bug in the contract, not a caller error. The error is not recoverable through retry.

ABI encoding: the 4-byte selector `0x4e487b71` followed by the ABI-encoded `uint256` panic code.

```
0x4e487b71
+ abi.encode(uint256 panicCode)
```

Standard panic codes:

| Code | Meaning |
|------|---------|
| 0x01 | Generic assertion failure |
| 0x11 | Arithmetic overflow/underflow |
| 0x12 | Division by zero |
| 0x21 | Enum conversion out of range |
| 0x31 | Pop on empty array |
| 0x32 | Array index out of bounds |
| 0x41 | Resource allocation failure |
| 0x51 | Internal function call error |

### 12.3 `revert` -- Custom Error

**Selector:** first 4 bytes of `keccak256(ErrorName(param_types...))`

`revert CustomError(args...)` emits a contract-defined typed error. Custom errors allow structured, gas-efficient error reporting. The selector is computed identically to function selectors.

ABI encoding: the 4-byte custom selector followed by the ABI-encoded error parameters.

```
keccak256("InsufficientBalance(uint256,uint256)")[:4]
+ abi.encode(uint256 available, uint256 required)
```

Custom errors must be declared in the contract and are included in the ABI under the `errors` array at the top level:

```json
{
  "errors": [
    {
      "name": "InsufficientBalance",
      "selector": "0xcf479181",
      "inputs": [
        {"name": "available", "type": "uint256"},
        {"name": "required", "type": "uint256"}
      ]
    }
  ]
}
```

### 12.4 Error Classification for Agents

An agent receiving a revert can classify it by the first 4 bytes:

1. `0x08c379a0` -- precondition failure. Read the string message. Potentially retryable with different inputs.
2. `0x4e487b71` -- invariant violation. Read the panic code. Do not retry; report as contract bug.
3. Any other 4 bytes -- custom error. Look up the selector in the contract ABI `errors` array to decode parameters.
4. Empty revert data -- bare `revert()` with no message. Treat as an unspecified failure.

## 13. Trust Model

ABI metadata should be treated as trustworthy only when:

1. It is compiler-emitted or compiler-verified.
2. It is bound to the deployed bytecode hash.
3. The consumer has verified artifact integrity.

Human-authored or off-chain rewritten metadata must not be treated as authoritative unless explicitly marked as unverified.

## 14. Recommended Implementation Strategy

Tolang should evolve ABI in three stages.

### Stage 1 -- Canonical emitted fields
Normalize current emitted metadata:

- `@effects`
- `@bounds`
- `@gas`
- `@requires`
- `@delegated`
- `@verifiable`
- `@pay`
- `manifest`

### Stage 2 -- Unified ABI JSON schema
Emit a single normalized ABI object from the compiler or packer.

### Stage 3 -- Ecosystem tooling
Build:

- ABI validators
- ABI diff tools
- service discovery indexers
- agent preflight analyzers
- marketplace compatibility checkers

## 15. Non-Goals

ABI does not attempt to:

- replace source code review,
- model arbitrary off-chain legal contracts,
- guarantee economic profitability,
- or encode non-deterministic behavior as if it were trustworthy.

ABI exists to make executable policy and coordination semantics explicit.

## 16. Summary

ABI is the next layer of interface standardization for Tolang.

The traditional ABI made contracts callable.
The Agent Behavior Interface makes contracts understandable to agents.

That difference is what turns a contract language into a language for the Agent Economy.
