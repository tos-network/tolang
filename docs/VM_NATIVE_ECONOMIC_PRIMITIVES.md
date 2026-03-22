# Roadmap: Host-Shaped to VM-Native Economic Primitives

**Status**: DESIGN
**Date**: 2026-03-21

---

## Problem Statement

Core economic operations (escrow, transfer, capability checks, agent identity)
are currently exposed as ad hoc host functions injected into the Lua VM by GTOS.
This makes them implementation-defined rather than protocol-guaranteed, and
forces every test harness to re-implement the same host surface manually.

---

## Current State

| Primitive | Current form | Provided by | Protocol-guaranteed? |
|-----------|-------------|-------------|---------------------|
| `tos.call` | Host function | LVM | Yes (snapshot/revert) |
| `tos.staticcall` | Host function | LVM | Yes |
| `tos.delegatecall` | Host function | LVM | Yes |
| `tos.multicall` | Host function | LVM | Yes (atomic batch) |
| `tos.sload` / `tos.sstore` | Host function | LVM | Yes (StateDB) |
| `tos.transfer` | Host function | LVM | Yes (balance transfer) |
| `tos.hascapability` | Host function | LVM / host-injected | No -- registry is host-defined |
| `tos.package_call` | Host function | LVM | Partially -- depends on package resolution |
| `tos.escrow` | Host function | Test harness only | No -- not in LVM |
| `tos.release` | Host function | Test harness only | No -- not in LVM |
| `uno.balance` | Host function | LVM | Yes (UNO subsystem) |
| `uno.transfer` | Host function | LVM | Yes (UNO subsystem) |
| `tos.agentload` | Host function | Test harness only | No -- not in LVM |

---

## Proposed Lifecycle

Economic primitives should graduate through three stages:

### Stage 1: Host shim (current)

Host function injected by the runtime or test harness. No protocol guarantee.
Behavior may differ between environments. This is where `tos.escrow`,
`tos.release`, and `tos.agentload` sit today.

### Stage 2: Stable ABI host function

Host function with a protocol-defined ABI: specified argument types, return
types, gas costs, error codes, and rollback behavior. Implemented in LVM with
StateDB backing. Tests verify cross-environment consistency.

Candidates for promotion to Stage 2:
- `tos.hascapability` -- backed by StateDB capability registry
- `tos.escrow` / `tos.release` -- backed by a protocol escrow ledger in StateDB
- `tos.agentload` -- backed by protocol agent identity registry

### Stage 3: VM opcode

Bytecode-level instruction with fixed gas cost and deterministic semantics.
Only warranted when the operation is so frequent that host-function call
overhead is measurable, or when the operation must be atomic within a single
instruction boundary.

Candidates for promotion to Stage 3:
- `sload` / `sstore` -- already effectively opcodes (host functions with
  fixed StateDB semantics); formal opcode status is documentation, not
  a behavior change
- `hascapability` -- if capability checks become as common as storage reads

Operations that should remain as Stage 2 host functions indefinitely:
- `tos.call` / `tos.staticcall` / `tos.delegatecall` -- complex control flow
- `tos.multicall` -- batch semantics too complex for a single opcode
- `uno.transfer` -- UNO subsystem interaction

---

## Implementation Order

1. **`tos.hascapability` to Stage 2**: Move capability registry to StateDB.
   Remove host-injection dependency. (See `PROTOCOL_ANNOTATION_BACKING.md`.)

2. **`tos.escrow` / `tos.release` to Stage 2**: Add protocol escrow ledger
   to GTOS StateDB. Define escrow lifecycle (open, release, refund, expire)
   as StateDB state transitions. Remove test-harness-only implementations.

3. **`tos.agentload` to Stage 2**: Add protocol agent identity to GTOS.
   Agent metadata (type, capabilities, delegation chain) stored in StateDB
   and queryable at execution time.

4. **Evaluate Stage 3 candidates**: After Stage 2 is stable and gas-profiled,
   determine whether any primitives warrant opcode promotion based on
   measured call frequency and overhead.

---

## GTOS Dependencies

- Escrow ledger requires new StateDB storage slots per escrow record
- Agent identity requires new system contract or StateDB namespace
- Capability registry is a prerequisite (tracked in `PROTOCOL_ANNOTATION_BACKING.md`)
- No consensus changes for Stage 2; Stage 3 would require VM instruction set changes

---

## Acceptance Criteria

- [ ] `tos.hascapability` backed by StateDB, not host-injected table
- [ ] `tos.escrow` / `tos.release` implemented in LVM with StateDB escrow ledger
- [ ] Escrow lifecycle (open/release/refund/expire) has rollback semantics via snapshot/revert
- [ ] `tos.agentload` implemented in LVM with StateDB agent identity
- [ ] Test harness implementations removed or reduced to thin wrappers over LVM primitives
- [ ] Gas costs defined for all Stage 2 primitives
- [ ] Existing openlib contracts work unchanged after primitive promotion

---

## Related Documents

- `docs/TOLANG_SHORTCOMINGS.md` -- shortcoming #7 (economic primitives too host-shaped)
- `docs/PROTOCOL_ANNOTATION_BACKING.md` -- capability registry design
- `docs/AGENT_NATIVE_STDLIB_2046.md` -- economic semantic kernel
- `/home/tomi/gtos/docs/Atomic-Execution-v1.md` -- `tos.multicall` as Stage 2 precedent
