# What the TOL Standard Library Enables

**Date**: 2026-03-20
**Basis**: `docs/AGENT_NATIVE_STDLIB_2046.md` (first-principles design)
**Implementation audit**: 2026-03-21

---

## One-Sentence Summary

The TOL stdlib lets an autonomous agent write a policy-constrained,
evidence-backed, privacy-preserving, multi-terminal commercial transaction
in under 50 lines of code.

---

## Implementation Status Overview

All 13 stdlib seed contracts exist and compile. Runtime test coverage exists
for every package (`stdlib_runtime_test.go`, `stdlib_composed_runtime_test.go`).
Core lifecycles (grant/revoke, escrow/release, recovery, receipts) are
functionally closed.

**Overall implementation: ~97%.**

Current follow-on work is no longer the six implementation gaps that were
previously open. Those are now resolved:

1. settlement automation now includes configurable slash distribution and
   canonical auto-receipt binding
2. control-plane ergonomics now include named terminal/trust taxonomy
   enforcement and reusable step-up guards
3. typed discovery and privacy-helper work now have concrete v1 implementations

What remains is broader expansion, not missing baseline capability closure:

1. expand privacy composition from the current `PrivateDisputeEscrow` helper to
   the wider family described in `docs/PRIVACY_COMPOSITION_HELPERS.md`
2. continue typed discovery adoption beyond stdlib release/export into broader
   GTOS/OpenFox consumption paths

---

## Capabilities by Wave

### Wave 1: Control Plane — Making Agents Safe to Use

**Wave status: ~98% implemented.** All 5 seed contracts exist with runtime
tests. The named terminal/trust taxonomy and reusable step-up enforcement are
now closed in the current wave.

| Scenario | Without stdlib | With stdlib | Status |
|----------|--------------|-------------|--------|
| Employee agent daily spend cap of 1000 TOS | Hand-rolled storage + time-window arithmetic | `PolicyAccount.setSpendCaps(daily_limit, single_limit)` + `setDelegateCaps(delegate, daily_limit, single_limit)` | **~90%** — owner caps plus enforced per-delegate daily/single caps |
| NFC card limited to 100 TOS at POS terminals | Hand-rolled terminal type check + amount guard | `SessionBook.grantSession(session_id, terminal, scope, trust_tier, budget, ...)` + `requireTerminal(session_id, amount)` | **~95%** — session grant, trust tier, budget, convenience guard, named taxonomy constants, and runtime validation exist |
| Guardian recovers a compromised account | Hand-rolled timelock + challenge period + ownership transfer | `RecoveryController.startRecovery(new_owner)` → `approveRecovery()` → `executeRecovery()` | **~90%** — full lifecycle with timelock, cancel, freeze; multi-step rather than single-call |
| AI agent delegated with revocable authority | Hand-rolled delegation map + expiry + revocation | `AuthorityBook.grant(operator, scope, budget, expiry_ms, policy_hash, nonce_floor)` / `PolicyAccount.authorizeDelegate(delegate, allowance, expiry_ms)` | **~85%** — capped, time-bounded, revocable delegation in both contracts |
| Off-chain approval bound to on-chain execution | Hand-rolled nonce + policy hash + replay guard | `ExecutionBindingBook.approve(binding_id, principal, executor, expiry_ms, nonce, policy_hash, ...)` | **~85%** — full binding with nonce, policy hash, expiry, proof ref |

**Gaps:**
- control-plane baseline gaps in this wave are now resolved; remaining work is
  mainly higher-level ergonomics and additional helper breadth

---

### Wave 2: Execution Plane — Making Agents Commercially Productive

**Wave status: ~96% implemented.** Core task escrow, agreement, sponsor relay,
evidence, and receipt contracts exist. `RecurringPayment` now provides
subscription/periodic payment scheduling, and settlement automation now binds
slash policy plus canonical receipts.

| Scenario | With stdlib | Status |
|----------|-------------|--------|
| Bounty task: post → claim → submit → approve → pay | `TaskSettlement.openTask(task_ref, deadline_ms, receipt_ref)` → `acceptTask` → `submitTask` → `approveTask` | **~90%** — full lifecycle with dispute, reject, reclaim |
| Oracle: weather data written once, immutable | `EvidenceBook.openEvidence(evidence_id, claim_ref, attester, ...)` → `fulfill(evidence_id, value, proof_ref)` | **~80%** — write-once guard via status check; semantics shifted from Oracle to Evidence |
| Merchant POS payment, sponsor pays gas | `SponsorPolicyRelay.relay(target, data, user, cost, policy_hash, binding_ref, receipt_ref)` | **~70%** — relay with policy + budget exists; no `sponsored_payment` convenience wrapper |
| Monthly subscription auto-debit | `RecurringPayment.subscribe(subscription_id, provider, amount, interval_ms, max_cycles, agreement_ref)` + `executePayment(subscription_id, receipt_ref)` | **~85%** — coordinator-triggered periodic payments with pause/resume/cancel |
| Every settlement auto-generates audit receipt | `TaskSettlement.setReceiptBook(book)` + built-in `ReceiptBook.openReceipt(...)` / `finalizeSuccess(...)` / `finalizeFailure(...)` | **~95%** — settlement now auto-binds receipts on approval, dispute resolution, cancellation, reclaim, and final milestone completion |
| Milestone task payout with staged release | `TaskSettlement.openMilestoneTask(task_ref, milestone_count, deadline_ms, receipt_ref)` → `acceptTask` → `completeMilestone` | **~90%** — staged release exists with milestone status, partial reclaim, and final remainder handling |
| Quote → offer → acceptance → invoice lifecycle | `CommercialAgreement.createOffer(counterparty, amount, expiry_ms, quote_ref, terms_ref)` / `createInvoice(payer, amount, due_ms, terms_ref)` → `accept` → `fulfill` | **~90%** — offer/accept/fulfill/cancel/expire lifecycle plus explicit invoice subtype |

**Gaps:**
- ~~**Recurring/subscription payments**~~ — **RESOLVED**: `RecurringPayment`
  contract provides subscribe/execute/pause/resume/cancel lifecycle
- ~~**Milestone staged release**~~ — **RESOLVED**: `TaskSettlement.openMilestoneTask`
  / `completeMilestone` / `milestoneStatusOf` with remainder handling
- ~~**Slash distribution**~~ — **RESOLVED**: `TaskSettlement.setSlashPolicy(...)`
  precommits configurable poster/worker split before submission/dispute
- ~~**Auto-receipt binding**~~ — **RESOLVED**: settlement now opens/finalizes
  `ReceiptBook` entries canonically
- ~~**Invoice sub-type**~~ — **RESOLVED**: `CommercialAgreement.createInvoice`
  / `agreementTypeOf`

---

### Wave 3: Market Plane — Making Agent Economies Scale

**Wave status: ~92% implemented.** All privacy-family contracts now exist
(ConfidentialVault, ConfidentialEscrow, ConfidentialPayment,
ConfidentialTreasury, ConfidentialAllowance, AuditorDisclosureBook).
Typed discovery now has a normalized on-chain/exported v1, and privacy
composition now has a stateful `PrivateDisputeEscrow` helper seed.

| Scenario | With stdlib | Status |
|----------|-------------|--------|
| Encrypted payroll: employees see amounts, not each other | `ConfidentialPayment.batchPay(batch_id, receipt_ref)` + `addPayee(batch_id, payee, amount)` + `releaseBatch(batch_id, settlement_ref)` | **~85%** — batch payment with per-payee amounts implemented |
| Auditor verifies financials without seeing individual txs | `AuditorDisclosureBook.publishSnapshot(snapshot_id, data_ref, proof_ref, period_start, period_end)` + `authorizeAuditor(auditor, scope_ref, expiry_ms)` | **~85%** — snapshot-based disclosure with auditor authorization and finalization |
| Agent filters counterparties by reputation | `TrustRegistry.isEligible(subject)` / `snapshotReputationOf(subject)` / `updateReputation(subject, delta, reason_ref)` | **~85%** — eligibility composes native reputation baseline with local deltas; scorer callback exists |
| New agent advertises translation capability | `ServiceDirectory.registerService(...)` + typed schema setters/getters + exported `typed_discovery` metadata | **~90%** — registration plus structured fee/SLA and typed service/capability/pricing/privacy/receipt/trust-floor fields exist |
| Confidential treasury with selective disclosure | `ConfidentialTreasury.deposit()` + `authorizeSpend(spend_id, recipient, amount, purpose_ref)` + `executeSpend(spend_id, settlement_ref)` + `authorizeAuditor(auditor, disclosure_ref)` | **~85%** — multi-signer treasury with auditor disclosure |
| Stake-backed service guarantee | `TrustRegistry.depositBond()` / `setTrustFloor(min_stake, min_reputation)` / `lockStake(subject, agreement_ref, amount)` / `unlockStake(subject, agreement_ref)` | **~80%** — bond floor plus per-agreement stake lock exists |

**Gaps:**
- ~~**Missing contracts**~~ — **RESOLVED**: all 6 privacy-family contracts
  implemented (ConfidentialVault, ConfidentialEscrow, ConfidentialPayment,
  ConfidentialTreasury, ConfidentialAllowance, AuditorDisclosureBook)
- ~~**Reputation writes**~~ — **RESOLVED**: `TrustRegistry.updateReputation` /
  `setScorerCallback`; effective reputation = native baseline + local delta
- ~~**Discovery structure**~~ — **RESOLVED**: `ServiceDirectory` now exposes
  typed discovery fields and release/export metadata includes a normalized
  `typed_discovery` profile
- ~~**Selective disclosure composition ergonomics**~~ — **RESOLVED for v1**:
  `PrivateDisputeEscrow` now has stateful runtime coverage over confidential
  open/settle/dispute/refund with receipt and auditor-disclosure linkage

---

## Comparison With Existing Ecosystems

| Capability | Solidity + OpenZeppelin | TOL + stdlib | Implementation status |
|------------|----------------------|--------------|----------------------|
| Safe value transfer | `SafeERC20.safeTransfer()` library | `TaskSettlement.openTask` / `ConfidentialEscrow.openEscrow` — escrow-native | **~90%** |
| Reentrancy protection | `ReentrancyGuard` modifier | Compiler-enforced `@effects` + `set` keyword — no library needed | **~90%** — compiler-level |
| Access control | `Ownable` / `AccessControl` | `@requires(caller: Cap)` compiler-enforced + `tos.hascapability()` runtime check | **~90%** — compiler pipeline implemented + tested (parser/sema/lower/codegen/ABI); stdlib contracts can migrate from hand-rolled checks |
| Proxy delegation | `approve()` + `transferFrom()` | `AuthorityBook.grant` / `.revoke` / `.consume` — capped, time-bounded, revocable | **~85%** |
| Multi-terminal support | Not supported | `SessionBook.grantSession` with trust tier, budget, step-up threshold | **~90%** — named 6x5 taxonomy plus runtime validation and reusable terminal guards |
| Encrypted transfers | Not supported | `uno.transfer()` + `ConfidentialEscrow.openEscrow` / `.releaseEscrow` | **~90%** |
| Selective disclosure | Not supported | GTOS: DisclosureProof (ZK/DLEQ) + DecryptionToken + AuditorKey (consensus); stdlib: `AuditorDisclosureBook` + `ConfidentialVault.authorizeAuditor` | **RESOLVED at protocol layer** — all 3 layers implemented in GTOS; stdlib provides contract-level management and can add better composed helpers later |
| Task marketplace | ~200 lines hand-rolled state machine | `CommercialAgreement.createOffer` + `TaskSettlement.openTask` | **~90%** |
| Gas sponsorship | ERC-4337 complex stack | `SponsorPolicyRelay.relay` — native sponsor binding + policy + budget | **~85%** |
| Machine-readable audit | Optional event logs | `ReceiptBook.openReceipt` / `.finalizeSuccess` — structured evidence chain | **~92%** — `TaskSettlement` now auto-binds receipts into settlement lifecycle |
| Verifiable effects | Source audit required | `@effects` declarations verified by compiler, published in ABI | **~90%** — compiler-level |
| Gas bounds | Guessed or empirical | `@gas(upper: N)` verified by compiler, bound-checked | **~90%** — compiler-level |
| Terminal-scoped policy | Not supported | `SessionBook` + `PolicyAccount` — per-session trust tier and budget | **~70%** — requires manual composition of two contracts |
| Guardian recovery | ~150 lines hand-rolled | `RecoveryController.startRecovery` → `approveRecovery` → `executeRecovery` — timelocked, cancellable | **~90%** |
| Discovery metadata | No standard | `ServiceDirectory.registerService(manifest_ref, capability_ref, version_ref, quote_ref)` + `setServiceFee` + `setServiceSLA` | **~80%** — fee and SLA are structured fields; capability metadata remains reference-based |
| Confidential DeFi | Not supported | `ConfidentialEscrow` + `ConfidentialVault` + `ConfidentialPayment` + `ConfidentialTreasury` + `ConfidentialAllowance` + `AuditorDisclosureBook` — full privacy family on UNO rails | **~85%** — all 6 contracts implemented; remaining gap is higher-level composed privacy ergonomics |

---

## The Question TOL Answers That Solidity Does Not Ask

Solidity asks: *How do I write a safe DeFi contract?*

TOL asks:

> An AI agent enters through an NFC card at a POS terminal. Under policy
> constraints (daily cap, terminal limit, merchant allowlist), it pays with
> encrypted UNO balance for a service. The payment is escrowed until delivery
> is confirmed. A dispute triggers arbitration with selective disclosure to
> the arbitrator. The settlement produces a machine-auditable receipt with
> sponsor attribution and proof references. The agent's guardian can freeze
> the account at any time.
>
> **How many lines of contract code does this take?**

With the TOL stdlib: **under 50 lines.**

Without it: hundreds of lines of hand-rolled state machines, policy checks,
encryption handling, receipt formatting, and terminal discrimination logic —
repeated differently in every contract, with different bugs each time.

**Implementation note:** The end-to-end scenario above is partially
demonstrated in `stdlib_composed_runtime_test.go` (`PolicySponsoredCheckout`,
`PrivateServiceOrder`, `SponsoredPrivateEscrowCheckout`). The selective
disclosure to arbitrator step is no longer blocked by missing protocol or
contract primitives — GTOS now provides DisclosureProof, DecryptionToken, and
AuditorKey, and stdlib includes `AuditorDisclosureBook`. What remains is a
cleaner composed example and convenience surface.

---

## What Each Package Eliminates

| Package | What developers no longer hand-write | Implementation | Gaps |
|---------|-------------------------------------|----------------|------|
| `account` | Spend cap arithmetic, allowlist storage, suspension flags | **~90%** — `PolicyAccount` with `setSpendCaps`, `setAllowlisted`, `suspend`, `authorizeDelegate`, `setDelegateCaps`, `delegateDailyRemaining` | Per-delegate caps now enforced in execute path |
| `authority` | Delegation maps, expiry checks, revocation propagation | **~90%** — `AuthorityBook` with `grant`, `revoke`, `consume`, scoped by operator+scope key | — |
| `execution_binding` | Nonce management, policy hash binding, replay guards | **~90%** — `ExecutionBindingBook` with `approve`, `cancel`, `consume`, nonce+expiry+policy_hash | — |
| `session` | Terminal type discrimination, trust tier checks, step-up logic | **~80%** — `SessionBook` with `grantSession`, `consume`, `requiresStepUp`, `requireTerminal` | `requireTerminal` provides convenience enforcement; trust tiers are raw u256 |
| `recovery` | Timelock arithmetic, challenge periods, ownership transfer | **~90%** — `RecoveryController` with `startRecovery`, `approveRecovery`, `executeRecovery`, `freeze` | — |
| `agreement` | Quote/offer/acceptance state machines | **~90%** — `CommercialAgreement` with `createOffer`, `createInvoice`, `accept`, `cancel`, `fulfill`, `expire`, `agreementTypeOf` | Invoice subtype implemented; subscription state remains externalized to `RecurringPayment` |
| `settlement` | Escrow state machines, milestone tracking, slash distribution | **~90%** — `TaskSettlement` with full task lifecycle + dispute + milestone staged release; `RecurringPayment` with subscribe/execute/cancel/pause/resume | No configurable slash distribution |
| `sponsor` | Sponsor authorization, budget tracking, attribution records | **~85%** — `SponsorPolicyRelay` with `authorizeRelayer`, `relay`, budget tracking | — |
| `evidence` | Oracle write-once guards, proof reference attachment | **~85%** — `EvidenceBook` with `openEvidence`, `fulfill`, `challenge`, `finalize` | — |
| `receipt` | Receipt formatting, approval linkage, settlement traces | **~80%** — `ReceiptBook` with `openReceipt`, `finalizeSuccess`, `finalizeFailure` | Not auto-emitted; requires explicit caller integration |
| `trust` | Reputation queries, stake checks, scorer integration | **~85%** — `TrustRegistry` with `depositBond`, `setTrustFloor`, `isEligible`, `snapshotReputationOf`, `updateReputation`, `setScorerCallback`, `lockStake`, `unlockStake`, `lockedStakeOf` | Reputation writes affect eligibility; per-agreement stake locks implemented |
| `privacy` | UNO bridge wiring, disclosure flow setup, auditor view construction | **~85%** — `ConfidentialVault` (deposit/withdraw/auditor auth) + `ConfidentialEscrow` (escrow/release/refund) + `ConfidentialPayment` (batch/individual payments) + `ConfidentialTreasury` (multi-signer treasury) + `ConfidentialAllowance` (encrypted approve/transferFrom) + `AuditorDisclosureBook` (snapshot-based disclosure) | GTOS selective disclosure stack is resolved; remaining work is higher-level composed helper flows |
| `discovery` | Manifest construction, capability advertisement, version markers | **~80%** — `ServiceDirectory` with `registerService`, `updateManifest`, `updateQuote`, `deactivate`, `setServiceFee`, `setServiceSLA`, `feeOf`, `slaOf` | Fee and SLA are structured u256 fields; capability metadata is still reference-based |

---

## Consolidated Gap List

### Missing contracts — RESOLVED (2026-03-21)

All previously missing contracts have been implemented:

| Contract | Package | Status |
|----------|---------|--------|
| `ConfidentialPayment` | `privacy` | **IMPLEMENTED** — batch/individual encrypted payment flows |
| `ConfidentialTreasury` | `privacy` | **IMPLEMENTED** — multi-owner confidential treasury with selective disclosure |
| `ConfidentialAllowance` | `privacy` | **IMPLEMENTED** — encrypted allowance/approval patterns |
| `AuditorDisclosureBook` | `privacy` | **IMPLEMENTED** — structured auditor disclosure with snapshots |
| `RecurringPayment` | `settlement` | **IMPLEMENTED** — subscription/recurring payment scheduler |

### Capability status in existing contracts

| Feature | Affected contract | Description |
|---------|------------------|-------------|
| ~~Milestone staged release~~ | `TaskSettlement` | **RESOLVED** — `openMilestoneTask` / `completeMilestone` / `milestoneStatusOf` |
| ~~Slash distribution~~ | `TaskSettlement` | **RESOLVED** — `setSlashPolicy(...)` precommits the worker/poster split and enforces it at dispute resolution |
| ~~Auto-receipt binding~~ | `TaskSettlement` + `ReceiptBook` | **RESOLVED** — settlement now opens/finalizes canonical receipts on approval, reject/dispute resolution, cancellation, reclaim, and final milestone completion |
| ~~Invoice type~~ | `CommercialAgreement` | **RESOLVED** — `createInvoice` / `agreementTypeOf` |
| ~~Per-role spend caps~~ | `PolicyAccount` | **RESOLVED** — `setDelegateCaps` / `delegateDailyRemaining`; enforced in execute path |
| ~~Reputation writes~~ | `TrustRegistry` | **RESOLVED** — `updateReputation` / `setScorerCallback`; composes native + local |
| ~~Structured discovery fields~~ | `ServiceDirectory` | **RESOLVED** — `setServiceFee` / `setServiceSLA` / `feeOf` / `slaOf` |

### Missing compiler/language features

| Feature | Description |
|---------|-------------|
| ~~`@requires(caller: Cap)`~~ | **RESOLVED** — compiler pipeline implemented (parser/sema/lower/codegen/ABI); 3 tests; design doc completed |
| ~~Selective disclosure (ZK layer)~~ | **RESOLVED** — DisclosureProof (DLEQ Sigma) implemented in GTOS `crypto/priv/disclosure.go` |
| ~~Selective disclosure (decryption token layer)~~ | **RESOLVED** — DecryptionToken implemented in GTOS `core/priv/decryption_token.go` |
| ~~Selective disclosure (regulatory / auditor key layer)~~ | **RESOLVED** — AuditorKey consensus path implemented and documented in `/home/tomi/gtos/docs/SELECTIVE-DISCLOSURE.md` |

### Document placement for the five strategic workstreams

These five follow-on workstreams should not all be tracked as contract-level
feature tickets.

They split cleanly between stdlib capability backlog, structural shortcomings,
and deeper design homes:

| Workstream | This document tracks | Detailed design home |
|------------|----------------------|----------------------|
| Cross-contract atomicity | **RESOLVED** — `tos.multicall` implemented | `/home/tomi/gtos/docs/Atomic-Execution-v1.md` |
| Privacy family completion | **RESOLVED** — all 6 contracts implemented; GTOS selective disclosure stack also resolved | `docs/PRIVACY_STDLIB_FAMILY.md` |
| Recurring / subscription settlement | **RESOLVED** — `RecurringPayment` contract; protocol scheduler pending | `/home/tomi/gtos/docs/Native-Scheduled-Tasks.md` |
| `@requires(caller: Cap)` | **RESOLVED** — compiler pipeline implemented + 3 tests | `docs/CALLER_CAPABILITY_SYNTAX.md` |
| Selective disclosure (`ZK + token`) | **RESOLVED** — all 3 layers in GTOS (DisclosureProof, DecryptionToken, AuditorKey) | `/home/tomi/gtos/docs/SELECTIVE-DISCLOSURE.md` plus `docs/PRIVACY_STDLIB_FAMILY.md` |

Status guidance:

- This document is the right place to state what stdlib capability is still not
  commercially closed.
- It is not the right place to hold full VM, compiler, or cryptographic design.

---

## The Commercial Value Proposition

TOL without stdlib: a language with nice syntax and incomplete economic
semantics.

TOL with stdlib: **the first smart contract platform where "agent-native"
is not marketing — it is the actual development experience.**

A developer importing `stdlib/settlement` and `stdlib/privacy` can write a
confidential escrow contract in 20 lines. The same contract works across
all terminal types, respects policy wallet constraints, produces audit
receipts, and supports selective disclosure to regulators — all without the
developer thinking about any of it.

That is what makes TOL commercially viable at scale.

**Current reality (2026-03-21):** All 18 stdlib seed contracts compile and
have package/artifact coverage.  The privacy family is now complete with 6
contracts (ConfidentialVault, ConfidentialEscrow, ConfidentialPayment,
ConfidentialTreasury, ConfidentialAllowance, AuditorDisclosureBook).  The
settlement family has both TaskSettlement and RecurringPayment. The current
stdlib closure wave also includes slash distribution, auto-receipt binding,
named terminal/trust taxonomy, reusable step-up enforcement, v1 privacy
composition helpers, and typed discovery normalization. The remaining work is
longer-horizon evolution: broader privacy helper coverage, broader
GTOS/OpenFox typed discovery consumption, and continued
release/discovery/threat-model tightening.
