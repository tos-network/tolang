# TODO / 任务一览

Last updated: 2026-03-22

## Next Stage Backlog

| ID | Item | Status | Notes |
|---|---|---|---|
| N-1 | GTOS settlement bus + receipt hooks | Done | GTOS now exposes `tos.settle(...)`, `tos.settle_refund(...)`, `tos.settle_escrow(...)`, `tos.receipt_open/success/failure/info`, and `tos.settlement_info(...)`, backed by stateful `RuntimeReceipt` + `SettlementEffect` records and VM/runtime tests |
| N-2 | Registry governance + revocation workflows | Done | GTOS registry v1.1 now enforces owner/principal authorization plus governor override, stores lifecycle metadata (`created_at` / `updated_at`), and exposes governance facts through protocol RPC for capability, delegation, verification, and pay-policy surfaces. Design home: `/home/tomi/gtos/docs/GTOS_PROTOCOL_REGISTRIES.md` |
| N-3 | Package namespace + publisher governance | Done | GTOS package/publisher registry v1.3 now enforces namespace ownership with controller-or-governor authorization, records lifecycle metadata (`created_at` / `updated_at`) for publishers and packages, tightens publish/deprecate/revoke flows, and exposes governance/trust facts through package and publisher RPCs. Design home: `/home/tomi/gtos/docs/PACKAGE_PUBLISHING_REGISTRY.md` |
| N-4 | Release/discovery/threat-model tightening | Ongoing | Keep `.profile.json`, family `bundle.profile.json`, `typed_discovery`, `protocol_alignment`, machine-readable `threat_model`, threat-model docs, and release manifests aligned as GTOS-native protocol surfaces land |
| N-5 | Agent-facing protocol alignment export | Done | `AgentContractProfile`, `DiscoveryManifest`, and `AgentPackageInfo` now carry additive `protocol_alignment` hints for settlement bus, registry governance, and package governance; exporter emits them into release artifacts |

## Current openlib backlog closures

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
| S-11 | GTOS escrow / release rollback closure | Done | LVM now has direct rollback coverage for escrow reserve and nested release failure, on top of existing reserve/release/slash balance movement tests |
| S-12 | GTOS UNO runtime contract normalization | Done | GTOS now exposes explicit `tos.uno_balance(...)` / `tos.uno_transfer(...)` aliases, lowers Tolang `uno.balance/transfer` onto them, fails closed on malformed addresses, and has top-level + nested rollback coverage |

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

## Shortcomings Closure Follow-Ups

| ID | Item | Status | Notes |
|---|---|---|---|
| A-1 | `@delegated` compiler/runtime alignment | Done | `buildDelegatedPreamble(...)` now passes `(principal, delegate, scope_ref)` into `tos.hasdelegation(...)`, with `tx.origin` as principal fallback and a `bytes32` scope ref encoded as canonical hex |
| A-2 | `@pay` protocol-backed enforcement | Done | `buildPayPreamble(...)` now checks `tos.canpay(...)` before transfer and routes payment through `tos.host_transfer(...)` when available rather than the legacy ad hoc path |
| A-3 | `@verifiable` runtime stub body | Done | synthesized `verify_*` entrypoints now re-execute the original view/pure function in the lowering stage and compare actual outputs with `expected_*` arguments instead of always reverting |
| A-4 | `tos.package_call` strict published-package closure | Done | GTOS LVM now rejects unpublished packages with `PACKAGE_UNPUBLISHED`, validates package addr/data strictly, and tests the real registry-backed path end to end |

## Shortcomings Closure Audit Follow-Ups

| ID | Item | Status | Notes |
|---|---|---|---|
| B-1 | `@delegated` overload-safe scope refs | Done | Delegation scope refs now hash the canonical function signature (for example `transfer(agent,u256)`), so overloads no longer collide on bare source name |
| B-2 | `@verifiable` proof-bound v1 semantics | Done | `verify_*` now binds the `proof` argument to a deterministic witness digest over the canonical target signature, original inputs, and expected outputs before re-executing the function |
| B-3 | strict address parsing parity for registry-backed hooks | Done | `tos.hascapability(...)` and `tos.isverified(...)` now reject malformed addresses with the same fail-closed posture already used by `hasdelegation`, `canpay`, and `package_call` |
| B-4 | GTOS chain-level `@pay` integration coverage | Done | Added live VM end-to-end coverage that compiles a real `@pay` contract, seeds pay-policy registry state, and proves both deny and allow paths on-chain |
