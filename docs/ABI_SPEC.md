# ABI Specification
## Agent Behavior Interface for Tolang and TOS Network

**Status:** Draft  
**Version:** 0.1  
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
  "abi_version": "0.1",
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

The following fields should be required in all ABI objects:

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
  "verifiable": true,
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
- `selector`: call selector
- `inputs`: typed input schema
- `outputs`: typed output schema
- `state_mutability`: mutability classification

### 7.2 Authorization fields

- `requires_capability`: array of required capability symbols
- `accepts_delegation`: boolean
- `delegation_scope`: optional structured scope descriptor
- `caller_kind`: optional enum such as `user`, `agent`, `contract`, `any`

### 7.3 Payment fields

- `payment.mode`: `none`, `direct`, `escrow`, `stake`, `fee`
- `payment.asset`: canonical asset symbol or ID
- `payment.required`: boolean
- `payment.min_amount`: optional numeric lower bound
- `payment.settlement_rule`: optional text or structured rule reference

### 7.4 Verification fields

- `verifiable`: boolean
- `proof_kind`: optional enum such as `attestation`, `signature`, `merkle`, `zk`, `custom`
- `proof_schema`: optional schema URI or structured object
- `challenge_window`: optional block/time window

### 7.5 Effect fields

- `effects.reads`: declared state read set
- `effects.writes`: declared state write set
- `effects.emits`: declared event set
- `effects.calls`: declared external call references
- `effects.value_transfer`: boolean or structured transfer descriptor

### 7.6 Bound fields

- `bounds.external_calls_max`
- `bounds.storage_writes_max`
- `bounds.storage_reads_max`
- `bounds.loop_class`: `bounded`, `conservative`, `unbounded`
- `bounds.max_output_size`: optional

### 7.7 Gas fields

- `gas.upper`
- `gas.model`
- `gas.conservative`: boolean

### 7.8 Composability fields

- `composability.non_composable`
- `composability.reason`
- `composability.reentrant_sensitive`: optional boolean

### 7.9 Failure semantics

- `failure_modes`: symbolic failure list
- `revert_schema`: optional structured revert descriptor

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

```json
{
  "task_descriptor": {
    "task_type": "generic",
    "state_machine": "tol-task-v1",
    "escrow_enabled": true,
    "proof_required": true
  }
}
```

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

## 11. Trust Model

ABI metadata should be treated as trustworthy only when:

1. It is compiler-emitted or compiler-verified.
2. It is bound to the deployed bytecode hash.
3. The consumer has verified artifact integrity.

Human-authored or off-chain rewritten metadata must not be treated as authoritative unless explicitly marked as unverified.

## 12. Recommended Implementation Strategy

Tolang should evolve ABI in three stages.

### Stage 1 — Canonical emitted fields
Normalize current emitted metadata:

- `@effects`
- `@bounds`
- `@gas`
- `@requires`
- `@delegated`
- `@verifiable`
- `@pay`
- `manifest`

### Stage 2 — Unified ABI JSON schema
Emit a single normalized ABI object from the compiler or packer.

### Stage 3 — Ecosystem tooling
Build:

- ABI validators
- ABI diff tools
- service discovery indexers
- agent preflight analyzers
- marketplace compatibility checkers

## 13. Non-Goals

ABI does not attempt to:

- replace source code review,
- model arbitrary off-chain legal contracts,
- guarantee economic profitability,
- or encode non-deterministic behavior as if it were trustworthy.

ABI exists to make executable policy and coordination semantics explicit.

## 14. Summary

ABI is the next layer of interface standardization for Tolang.

The traditional ABI made contracts callable.
The Agent Behavior Interface makes contracts understandable to agents.

That difference is what turns a contract language into a language for the Agent Economy.
