# LVM vs openlib Boundary Cleanup

**Status**: IMPLEMENTED V1  
**Date**: 2026-03-22

---

## Why this document exists

The first GTOS/openlib closure wave intentionally landed several new
protocol-native surfaces in LVM:

- settlement bus + receipt hooks
- protocol registries
- package/publisher governance
- runtime inspection
- UNO rails

That was the correct move for correctness, rollback, and inspection.
It does **not** mean all of those surfaces should become direct
developer-facing APIs.

This document defines the boundary:

- what remains GTOS-native
- what should be treated as the preferred openlib developer surface
- what should eventually collapse into a clearer system-contract shape

---

## Boundary rule

1. **GTOS / LVM owns protocol semantics**
   - atomic value movement
   - receipt state
   - protocol registry truth
   - package/publisher trust
   - runtime inspection
   - UNO-native balance movement

2. **openlib owns developer-facing economic composition**
   - task settlement flows
   - receipt-oriented commercial flows
   - sponsor routing
   - confidential escrow/payment/treasury composition
   - session/account/discovery/trust workflows

3. **Future system-contract surfaces should expose stable named protocol
   domains**, instead of leaving all long-term developer ergonomics in raw
   `tos.*` form.

---

## V1 classification

### Native GTOS surfaces

These remain protocol-native and machine-checkable:

- `settlement_bus`
- `protocol_registry`
- `package_registry`
- `runtime_inspection`
- `uno_rail`

### Preferred openlib entry layer

For contract/package authors, the preferred entrypoints remain openlib
contracts and packages, for example:

- `tolang.openlib.settlement.TaskSettlement`
- `tolang.openlib.receipt.ReceiptBook`
- `tolang.openlib.sponsor.SponsorPolicyRelay`
- `tolang.openlib.account.PolicyAccount`
- `tolang.openlib.discovery.ServiceDirectory`
- `tolang.openlib.privacy.*`

### Future system-contract targets

These are the protocol domains that should eventually become clearer
system-facing surfaces:

- `system.settlement`
- `system.receipt`
- `system.registry`
- `system.package_registry`
- `system.uno`

---

## Machine-readable export

This boundary is now exported in a new additive metadata section:

- `runtime_boundary`

It is emitted in:

- per-contract `.profile.json`
- per-contract `.discovery.json`
- per-contract `.agentpkg.json`
- family `.bundle.profile.json`
- family `.bundle.discovery.json`
- family `.bundle.agentpkg.json`

The schema currently carries:

- `native_surfaces`
- `preferred_openlib`
- `future_system_surfaces`
- `notes`

This is intentionally additive: old consumers can ignore it.

---

## Practical interpretation

### If you are changing GTOS/LVM

You are allowed to add or harden protocol-native semantics when the change is
about:

- rollback / accounting
- registry-backed truth
- inspection / trust facts
- settlement / receipt integrity
- UNO-native correctness

### If you are changing openlib

You should prefer expressing developer workflows in openlib rather than adding
new direct runtime entrypoints unless the behavior truly must be protocol
native.

### If you are changing SDK/runtime clients

You should consume:

- `profile`
- `bundle_profile`
- `routing_profile`
- `threat_model`
- `protocol_alignment`
- `runtime_boundary`

so provider selection and developer UX can distinguish:

- protocol guarantees
- preferred developer-facing entrypoints
- future migration targets

---

## Completion bar for P-1

P-1 is considered complete when:

- boundary policy is documented
- boundary policy is exported in machine-readable metadata
- release artifacts include that boundary profile
- GTOS metadata RPC continues to surface the enriched profiles without extra
  reconstruction logic

That bar is now met.

---

## P-2 adoption result

P-2 is now the proof that this boundary is implementable in code, not only in
metadata.

The developer-facing settlement entry layer has been normalized onto the
Tolang `settlement.*` helper surface, while GTOS still owns the underlying
protocol semantics:

- `settlement.openReceipt(...)`
- `settlement.transferPublic(...)`
- `settlement.refundPublic(...)`
- `settlement.releaseEscrowPublic(...)`
- `settlement.transferUno(...)`
- `settlement.refundUno(...)`

That helper layer now backs openlib settlement adoption in:

- `openlib/settlement/TaskSettlement.tol`
- `openlib/settlement/RecurringPayment.tol`
- `openlib/privacy/ConfidentialEscrow.tol`
- `openlib/privacy/ConfidentialTreasury.tol`
- `openlib/sponsor/SponsorPolicyRelay.tol`

So the architecture line is now concrete:

- GTOS / LVM owns settlement-bus truth, receipt state, rollback, and effect
  inspection.
- openlib owns the developer-facing economic flow shape that contract authors
  actually compose.
