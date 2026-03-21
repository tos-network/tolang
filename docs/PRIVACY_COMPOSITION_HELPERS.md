# Privacy Composition Helpers

**Status**: DESIGN READY FOR IMPLEMENTATION  
**Date**: 2026-03-21

---

## Purpose

GTOS already resolves the protocol-layer privacy stack:

- `DisclosureProof`
- `DecryptionToken`
- `AuditorKey`

TOL stdlib already resolves the contract-layer privacy family:

- `ConfidentialVault`
- `ConfidentialEscrow`
- `ConfidentialPayment`
- `ConfidentialTreasury`
- `ConfidentialAllowance`
- `AuditorDisclosureBook`

What is still missing is the **composition layer**: reusable business helpers
that package those primitives into concrete agent workflows.

This document defines that missing layer.

---

## Problem Statement

Today, a developer can assemble confidential flows manually, but the last mile is
still too bespoke:

- escrow and disclosure are not bound together by default
- receipts are not automatically shaped for arbitrator / auditor workflows
- treasury spends and regulated checkouts still require app-specific glue
- selective disclosure exists, but there is no canonical stdlib helper that says
  "when dispute X happens, disclose Y to role Z under policy P"

The gap is not protocol correctness.
The gap is **agent-facing business ergonomics**.

---

## Scope

This document defines the stdlib-side helper layer for:

1. arbitrated confidential settlement
2. regulator / auditor ready private checkout
3. treasury spends with structured disclosure policy
4. receipt shaping for privacy-aware dispute and audit flows

It does **not** define new cryptographic primitives.

---

## Non-goals

This work must not:

- re-implement GTOS selective disclosure cryptography in Tolang
- add new consensus rules
- duplicate the existing privacy family contracts
- replace `AuditorDisclosureBook` with a new disclosure storage primitive
- turn stdlib helper contracts into proof verifiers

GTOS remains the home of:

- proof systems
- token generation / verification
- auditor-key consensus behavior

TOL stdlib remains the home of:

- policy composition
- receipt/disclosure linkage
- helper workflows
- business-state coordination

---

## Design Principles

1. **Compose, do not fork**
   Every helper must sit on top of existing privacy-family contracts and GTOS
   protocol features.

2. **Disclosure is policy-shaped**
   Helpers should standardize *who may learn what, when, and under what
   reference*, not how cryptography works internally.

3. **Receipts are first-class**
   Every helper flow should produce stable machine-auditable receipt linkage.

4. **Dispute and audit are different**
   Arbitrator disclosure, regulator disclosure, and auditor disclosure should
   not be collapsed into one generic "reveal" path.

5. **Confidential flows must degrade explicitly**
   A helper must say whether failure leads to:
   - refund
   - dispute
   - hold
   - force-disclose

---

## V1 Deliverables

The first implementation wave should produce **three helpers** and **one shared
receipt/disclosure schema pattern**.

### 1. `PrivateDisputeEscrow`

Purpose:
- combine confidential escrow with dispute evidence and selective disclosure

Composes:
- `ConfidentialEscrow`
- `EvidenceBook`
- `AuditorDisclosureBook`
- `ReceiptBook`

Core use case:
- payer funds confidential escrow
- payee expects release on successful delivery
- if dispute happens, evidence is attached and disclosure policy is activated
- receipt records whether the flow resolved by release, refund, or dispute

Required API shape:

```text
openPrivateEscrow(...)
approvePrivateRelease(...)
openDispute(...)
authorizeArbitratorDisclosure(...)
resolveDisputeRelease(...)
resolveDisputeRefund(...)
receiptOf(...)
disclosurePolicyOf(...)
```

Must standardize:
- disclosure scope ref
- arbitrator role binding
- receipt linkage
- evidence linkage
- refund vs release terminal states

### 2. `RegulatedPrivateCheckout`

Purpose:
- package a merchant/private-payment flow that is confidential by default but
  regulator/auditor-ready

Composes:
- `PolicyAccount`
- `SponsorPolicyRelay`
- `ConfidentialEscrow` or `ConfidentialPayment`
- `ReceiptBook`
- `AuditorDisclosureBook`

Core use case:
- weak terminal checkout
- sponsor-supported execution
- encrypted settlement
- regulator/auditor path pre-bound by policy

Required API shape:

```text
prepareCheckout(...)
commitCheckout(...)
settleCheckout(...)
refundCheckout(...)
authorizeAuditView(...)
receiptOf(...)
checkoutStatusOf(...)
```

Must standardize:
- sponsor attribution in receipt
- confidential settlement ref
- audit scope ref
- merchant proof / delivery ref

### 3. `TreasuryDisclosureFlow`

Purpose:
- package confidential treasury spend with structured disclosure lifecycle

Composes:
- `ConfidentialTreasury`
- `AuditorDisclosureBook`
- `ReceiptBook`

Core use case:
- treasury authorizes confidential spend
- spend executes under multi-signer policy
- auditor/regulator view is attached in a canonical way

Required API shape:

```text
proposeTreasurySpend(...)
approveTreasurySpend(...)
executeTreasurySpend(...)
attachDisclosurePolicy(...)
finalizeTreasuryReceipt(...)
```

Must standardize:
- spend id to receipt ref binding
- disclosure scope semantics
- signer approval to disclosure policy linkage

### 4. Shared receipt/disclosure schema pattern

The helper layer should define a reusable shape, even if it is implemented via
existing contracts and refs rather than one new on-chain struct.

Every privacy helper flow should standardize these references:

- `receipt_ref`
- `settlement_ref`
- `evidence_ref`
- `disclosure_scope_ref`
- `auditor_scope_ref`
- `arbitrator_scope_ref`
- `policy_ref`

At minimum, every helper must say:

- which refs are mandatory
- who is allowed to attach them
- at what lifecycle stage they become immutable

---

## Lifecycle Model

### Private dispute escrow

States:

1. `OPEN`
2. `FUNDED`
3. `RELEASE_PENDING`
4. `DISPUTED`
5. `RELEASED`
6. `REFUNDED`
7. `EXPIRED`

Disclosure transitions:

- no disclosure by default in `OPEN` / `FUNDED`
- optional auditor disclosure in normal settlement
- arbitrator disclosure allowed only after `DISPUTED`
- final receipt records whether disclosure occurred and under what scope ref

### Regulated private checkout

States:

1. `PREPARED`
2. `COMMITTED`
3. `SETTLED`
4. `REFUNDED`
5. `AUDIT_READY`

### Treasury disclosure flow

States:

1. `PROPOSED`
2. `APPROVED`
3. `EXECUTED`
4. `DISCLOSURE_BOUND`
5. `RECEIPTED`

---

## Canonical Policy Questions

Every helper must answer these questions explicitly:

1. Who may request disclosure?
2. Who may authorize disclosure?
3. Which lifecycle states allow disclosure?
4. Is disclosure pre-authorized, post-dispute, or post-settlement only?
5. What receipt fields must reference the disclosure decision?

If a helper cannot answer these cleanly, it is not ready.

---

## Required Runtime Coverage

Implementation is not complete without runtime tests for:

1. success path without disclosure
2. dispute path with arbitrator disclosure authorization
3. refund path preserving confidentiality semantics
4. sponsor + receipt + private settlement path
5. treasury spend with disclosure-bound receipt
6. rollback behavior when downstream confidential settlement fails

At least one composed runtime example must cover:

- privacy + receipt + evidence
- privacy + sponsor + receipt
- privacy + treasury + disclosure

---

## Acceptance Criteria

This design is considered implemented only when:

- at least 2 privacy composition helpers exist in code
- helper APIs compile and import as stdlib packages or examples
- runtime/composed-flow tests exist
- receipts include stable privacy/disclosure refs
- no new cryptographic primitives are introduced
- GTOS selective disclosure is consumed, not reimplemented
- docs clearly say which helper is for arbitrator, auditor, and regulator flows

---

## Recommended Implementation Order

1. `PrivateDisputeEscrow`
   Highest leverage because it closes privacy + dispute + receipt together
2. `RegulatedPrivateCheckout`
   Highest commercial value for weak-terminal / merchant flows
3. `TreasuryDisclosureFlow`
   Important, but narrower than checkout + dispute
4. Shared receipt/disclosure schema cleanup
   Normalize refs after helper semantics are stable

---

## Relationship To Existing Documents

- `docs/PRIVACY_STDLIB_FAMILY.md`
  This document defines the helper layer on top of the already-complete privacy
  family
- `docs/AGENT_NATIVE_STDLIB_2046.md`
  This document closes the "higher-level privacy composition helpers" backlog
- `docs/STDLIB_CAPABILITY_ANALYSIS.md`
  This document turns the privacy ergonomics backlog into implementable work
- `/home/tomi/gtos/docs/SELECTIVE-DISCLOSURE.md`
  GTOS protocol source of truth for disclosure primitives

