# Protocol-Level Runtime Registries for Agent-Native Annotations

**Status**: V1 IMPLEMENTED, NEXT-WAVE DESIGN OPEN
**Date**: 2026-03-22

---

## Current Status

The original gap was that four agent-native annotations compiled correctly and
appeared in ABI metadata, but lacked protocol-level runtime registries to
enforce their semantics end-to-end.

That v1 closure is now implemented. The remaining work is no longer "add any
runtime backing at all", but "deepen the protocol hooks for proofs, settlement,
and richer governance."

| Annotation | Compiler status | Runtime status |
|------------|----------------|----------------|
| `@delegated` | Emitted in ABI `delegated: true`; compiler preamble derives `(principal, delegate, scope_ref)` from the canonical function signature | GTOS delegation registry is live; `tos.hasdelegation(principal, delegate, scope_ref)` is state-backed and enforced in runtime |
| `@verifiable` | Emitted in ABI `verifiable: true`; `verify_*` entrypoints are synthesized | v1 runtime body binds `proof` to a deterministic witness digest, re-executes the original pure/view function, and compares `expected_*` outputs; GTOS `verifyregistry/` exposes `tos.isverified(...)`; proof-return hooks remain future work |
| `@pay` | Parsed and lowered; preamble checks `msg.value` and `tos.canpay(...)` | GTOS `paypolicy/` backs `tos.canpay(...)`; transfer path uses `tos.host_transfer(...)`; chain-level E2E now covers deny/allow execution; protocol settlement bus remains future work |
| `@requires(caller: Cap)` | Full pipeline (parser/sema/lower/codegen/ABI); runtime preamble calls `tos.hascapability` | GTOS capability registry is live; `tos.hascapability(...)` is protocol-native with strict address parsing |

The compiler and protocol now agree on the baseline semantics. The next wave is
about richer proof artifacts and a native settlement bus.

---

## What Is Implemented In V1

### 1. `@requires` / capability registry

**Current**: `tos.hascapability(caller, capName)` is state-backed in GTOS.

**Implemented**: GTOS capability registry state and LVM lookup back the Tolang
`@requires(caller: Cap)` preamble directly.

**Remaining**: governance, namespacing, and richer registry metadata.

### 2. `@delegated` / delegation registry

**Current**: `@delegated` lowers to a runtime preamble that derives principal
from `tx.origin`, delegate from `msg.sender`, and scope from a canonical
`bytes32` hash of the full function signature.

**Implemented**: GTOS delegation registry state backs
`tos.hasdelegation(principal, delegate, scope_ref)` with grant/revoke/expiry
checks.

**Remaining**: richer delegation budgets/sub-scopes and governance workflows.

### 3. `@verifiable` / proof verification hook

**Current**: `verify_*` entrypoints are synthesized and no longer hard-revert
in the direct lowering path.

**Implemented**: the lowering stage emits a verification body that first binds
`proof` to a deterministic witness digest over the canonical target signature,
original inputs, and `expected_*` outputs, then re-executes the original
pure/view function and compares the actual result to the `expected_*`
arguments carried by the stub ABI. GTOS `verifyregistry/` provides
`tos.isverified(...)` for protocol-backed attestation status.

**Remaining**: `tos.verified_staticcall(...)` and proof payload generation for
state-read proofs.

### 4. `@pay` / settlement bus

**Current**: `@pay` emits a preamble that checks `msg.value` and
`tos.canpay(...)` before transferring via `tos.host_transfer(...)` when the
dedicated host primitive is available.

**Implemented**: GTOS `paypolicy/` provides protocol-backed policy validation,
Tolang lowering uses that live runtime surface instead of relying only on ad
hoc host transfer calls, and GTOS chain-level tests now prove both deny and
allow execution paths for a real `@pay` contract under the live VM.

**Remaining**: a native settlement bus with receipt/proof hooks.

---

## Next-Wave Design Targets

1. **Proof verification hook** -- `tos.verified_staticcall(...)` and proof
   payload generation for `@verifiable`
2. **Settlement bus** -- canonical settlement routing with receipt binding for
   `@pay`
3. **Registry governance** -- revocation, namespacing, and policy evolution

---

## Acceptance Criteria

- [x] `tos.hascapability` reads from StateDB, not from a host-injected table
- [x] Capability grant/revoke is backed by protocol registry state
- [x] `@delegated` functions reject unauthorized delegates at protocol level
- [x] Delegation records support expiry and scope filtering in runtime lookup
- [x] `@pay` consults `tos.canpay(...)` and routes through `tos.host_transfer(...)`
- [x] Existing `@requires` ABI metadata remains unchanged
- [ ] `tos.verified_staticcall` returns proof payloads for `@verifiable` functions
- [ ] native settlement bus provides atomic value transfer with receipt binding

---

## Related Documents

- `docs/CALLER_CAPABILITY_SYNTAX.md` -- implemented `@requires` pipeline
- `docs/TOLANG_SHORTCOMINGS.md` -- shortcoming #3 (annotations ahead of protocol backing)
- `docs/AGENT_NATIVE_STDLIB_2046.md` -- authority/delegation/settlement design
- `/home/tomi/gtos/docs/GTOS_PROTOCOL_REGISTRIES.md` -- registry v1 implementation status
- `/home/tomi/gtos/docs/GTOS_SETTLEMENT_BUS_AND_RECEIPT_HOOKS.md` -- next-wave settlement bus design
- `/home/tomi/gtos/docs/Atomic-Execution-v1.md` -- `tos.multicall` precedent for protocol primitives
- `/home/tomi/gtos/docs/SELECTIVE-DISCLOSURE.md` -- `SetAuditorKey` precedent for system actions
