# TODO / 任务一览

Last updated: 2026-03-22

## Current stdlib backlog closures

| ID | Item | Status | Notes |
|---|---|---|---|
| S-1 | Slash distribution | Done | `TaskSettlement.setSlashPolicy(...)` now precommits dispute split policy before submission/dispute, and runtime coverage verifies poster/worker payout split |
| S-2 | Auto-receipt binding | Done | `TaskSettlement` now opens/finalizes `ReceiptBook` receipts on approval, dispute resolution, cancellation, reclaim, and final milestone release |
| S-3 | Named terminal/trust taxonomy | Done | `SessionBook` now enforces the named terminal/trust ranges instead of accepting arbitrary raw values |
| S-4 | Stronger reusable step-up enforcement | Done | `SessionBook.requireTerminal(...)` and `enforceStepUp(...)` now sit on top of validated taxonomy and reusable runtime guards |
| S-5 | Privacy composition helpers | Done | `PrivateDisputeEscrow` now has real stateful runtime coverage for confidential open/settle/dispute/refund flows, not only compile coverage |
| S-6 | Typed discovery schema normalization | Done | `ServiceDirectory` now exposes typed discovery fields, and `metadata.BuildDiscoveryManifest(...)` exports a normalized `typed_discovery` profile for agent-facing artifacts |
| S-7 | Broader privacy helper family | Done | v1 helper family now includes `PrivateDisputeEscrow`, `RegulatedPrivateCheckout`, and `TreasuryDisclosureFlow` with runtime/e2e coverage |
| S-8 | GTOS typed routing consumption | Done | `PrivateServiceOrder` routes on typed discovery fields and GTOS metadata RPC now returns `routing_profile` |
| S-9 | GTOS package publishing trust integration | Done | `pkgregistry` now maintains latest-by-channel indexes, RPC exposes latest active package resolution, and deployed metadata joins published package/publisher trust |
| S-10 | LVM native inspection expansion | Done | GTOS LVM now exposes `tos.agentinfo(...)`, `tos.packageinfo(...)`, `tos.packagelatest(...)`, and `tos.publisherinfo(...)` for runtime protocol-backed inspection |

## Review Follow-Ups

| ID | Item | Status | Notes |
|---|---|---|---|
| R-1 | `TaskSettlement.rejectTask(...)` receipt interaction | Done | rejection now opens the canonical receipt when configured, so rejected submissions no longer leave receipt binding entirely absent |
| R-2 | `TaskSettlement.resolveDispute(...)` worker-loss receipt amount | Done | `SettlementReceipt` now emits the worker-side payout on loss paths instead of the poster-side release amount |
| R-3 | `ServiceDirectory` duplicate capability API | Done | removed the redundant `setCapabilityKind(...)` / `capabilityKindOf(...)` alias pair and kept `capabilityType` as the single source of truth |
| R-4 | `PrivateDisputeEscrow` two-step refund inconsistency | Done | refund path now uses `ConfidentialEscrow.refundEscrowTo(...)`, so receipt finalization only happens after the escrow package successfully transfers to the original payer |
| R-5 | Receipt-finalization rollback regression | Done | added runtime coverage proving failed receipt finalization rolls settlement state and host-side release effects back |
| R-6 | UNO refund-transfer failure regression | Done | added composed runtime coverage proving failed confidential refund transfer leaves escrow/receipt/dispute state consistent |

## Audit Follow-Ups

| ID | Item | Status | Notes |
|---|---|---|---|
| T-4 | Make default `.tol -> bytecode/.toc/.tor` outputs reproducible across host paths by stripping source-map/debug metadata unless explicitly requested | Done | Default compile/package outputs no longer embed host-dependent `SourceName` paths |
| T-5 | Remove host-side interrupt channel from the deterministic VM execution path | Done | Execution termination is gas-driven only; host cancellation API removed |
| T-6 | Bound hash-table tombstone growth while preserving stale-key iteration semantics for `next/pairs` | Done | Tombstones are retained only for active stale traversal and compacted afterwards |
