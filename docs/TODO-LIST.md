# TODO / 任务一览

Last updated: 2026-03-21

## Current stdlib backlog closures

| ID | Item | Status | Notes |
|---|---|---|---|
| S-1 | Slash distribution | Done | `TaskSettlement.setSlashPolicy(...)` now precommits dispute split policy before submission/dispute, and runtime coverage verifies poster/worker payout split |
| S-2 | Auto-receipt binding | Done | `TaskSettlement` now opens/finalizes `ReceiptBook` receipts on approval, dispute resolution, cancellation, reclaim, and final milestone release |
| S-3 | Named terminal/trust taxonomy | Done | `SessionBook` now enforces the named terminal/trust ranges instead of accepting arbitrary raw values |
| S-4 | Stronger reusable step-up enforcement | Done | `SessionBook.requireTerminal(...)` and `enforceStepUp(...)` now sit on top of validated taxonomy and reusable runtime guards |
| S-5 | Privacy composition helpers | Done | `PrivateDisputeEscrow` now has real stateful runtime coverage for confidential open/settle/dispute/refund flows, not only compile coverage |
| S-6 | Typed discovery schema normalization | Done | `ServiceDirectory` now exposes typed discovery fields, and `metadata.BuildDiscoveryManifest(...)` exports a normalized `typed_discovery` profile for agent-facing artifacts |

## Audit Follow-Ups

| ID | Item | Status | Notes |
|---|---|---|---|
| T-4 | Make default `.tol -> bytecode/.toc/.tor` outputs reproducible across host paths by stripping source-map/debug metadata unless explicitly requested | Done | Default compile/package outputs no longer embed host-dependent `SourceName` paths |
| T-5 | Remove host-side interrupt channel from the deterministic VM execution path | Done | Execution termination is gas-driven only; host cancellation API removed |
| T-6 | Bound hash-table tombstone growth while preserving stale-key iteration semantics for `next/pairs` | Done | Tombstones are retained only for active stale traversal and compacted afterwards |
