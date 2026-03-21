# Protocol-Level Runtime Registries for Agent-Native Annotations

**Status**: DESIGN
**Date**: 2026-03-21

---

## Problem Statement

Four agent-native annotations compile to correct bytecode and appear in ABI
metadata, but lack protocol-level runtime registries to enforce their semantics
end-to-end:

| Annotation | Compiler status | Runtime status |
|------------|----------------|----------------|
| `@delegated` | Emitted in ABI `delegated: true` | No delegation registry; no protocol enforcement of delegation chain validity |
| `@verifiable` | Emitted in ABI `verifiable: true`; `proof_type: "state_proof"` in `.agentpkg.json` | No on-chain proof verification hook; agents trust the annotation but cannot verify |
| `@pay` | Parsed and lowered; settlement preamble emitted | No protocol settlement bus; payment routing depends on ad hoc host calls |
| `@requires(caller: Cap)` | Full pipeline (parser/sema/lower/codegen/ABI); runtime preamble calls `tos.hascapability` | `tos.hascapability` exists but the capability registry is host-provided, not protocol-native |

The compiler can describe these semantics. The protocol cannot yet enforce them.

---

## Proposed Mechanism

### 1. `@requires` / capability registry

**Current**: `tos.hascapability(caller, capName)` is a host function. The
registry backing it is implementation-defined.

**Needed**: A protocol-native capability registry in GTOS StateDB, keyed by
`(account, capName) -> bool`. Capabilities are granted/revoked via system
actions (like `SetAuditorKey`). The LVM reads the registry directly rather
than delegating to a host hook.

**GTOS changes**: New `policywallet/capability.go` with `ReadCapability` /
`WriteCapability`. New system action `ActionPolicyGrantCapability` /
`ActionPolicyRevokeCapability`. LVM replaces host-function lookup with
StateDB read.

### 2. `@delegated` / delegation registry

**Current**: `delegated: true` in ABI tells agents a function supports
delegation, but there is no on-chain record of who delegated what to whom.

**Needed**: A protocol-native delegation registry: `(principal, delegate,
scope, expiry, budget) -> DelegationRecord`. The LVM checks delegation
validity before executing `@delegated` functions when `msg.sender != principal`.

**GTOS changes**: New `delegation/registry.go` with `GrantDelegation` /
`RevokeDelegation` / `CheckDelegation`. LVM preamble for `@delegated`
functions queries the registry. Delegation records expire automatically.

### 3. `@verifiable` / proof verification hook

**Current**: `verifiable: true` in ABI and `proof_type: "state_proof"` in
agent metadata. No runtime verification.

**Needed**: A protocol-level state proof verification path. When an agent
calls a `@verifiable` function via `tos.staticcall`, the LVM can optionally
return a state proof alongside the result. This requires GTOS to expose
Merkle proof generation for the storage slots read during execution.

**GTOS changes**: New `tos.verified_staticcall(addr, data)` host function
that returns `(ok, result, proof)`. Proof format: list of `(slot, value,
merkle_path)` tuples. No consensus change; proof generation is read-only.

### 4. `@pay` / settlement bus

**Current**: `@pay` emits a settlement preamble that calls host functions
(`tos.transfer`, `uno.transfer`). Routing is ad hoc.

**Needed**: A protocol-level settlement bus that routes `@pay`-annotated
calls through a canonical escrow/transfer path with receipt generation.
This is the most complex registry because it touches value transfer.

**GTOS changes**: New `tos.settle(recipient, amount, receipt_ref)` host
function that atomically transfers value and emits a settlement event.
Settlement bus validates that the caller's contract has an active `@pay`
annotation for the function being executed.

---

## Implementation Order

1. **Capability registry** -- lowest complexity, highest immediate value;
   unblocks `@requires` from host-dependent to protocol-native
2. **Delegation registry** -- required before `@delegated` is safe for
   production multi-agent flows
3. **Proof verification hook** -- enables verifiable compute without
   trust assumptions; read-only, no consensus change
4. **Settlement bus** -- highest complexity; depends on stable escrow
   and receipt primitives

---

## Acceptance Criteria

- [ ] `tos.hascapability` reads from StateDB, not from a host-injected table
- [ ] Capability grant/revoke via system actions with tests in `lvm_rollback_test.go`
- [ ] `@delegated` functions reject unauthorized delegates at protocol level
- [ ] Delegation records support expiry, budget, and scope filtering
- [ ] `tos.verified_staticcall` returns Merkle proofs for `@verifiable` functions
- [ ] `tos.settle` provides atomic value transfer with receipt binding
- [ ] All existing `@requires` tests continue to pass unchanged
- [ ] ABI metadata fields (`delegated`, `verifiable`, `requires_capability`) unchanged

---

## Related Documents

- `docs/CALLER_CAPABILITY_SYNTAX.md` -- implemented `@requires` pipeline
- `docs/TOLANG_SHORTCOMINGS.md` -- shortcoming #3 (annotations ahead of protocol backing)
- `docs/AGENT_NATIVE_STDLIB_2046.md` -- authority/delegation/settlement design
- `/home/tomi/gtos/docs/Atomic-Execution-v1.md` -- `tos.multicall` precedent for protocol primitives
- `/home/tomi/gtos/docs/SELECTIVE-DISCLOSURE.md` -- `SetAuditorKey` precedent for system actions
