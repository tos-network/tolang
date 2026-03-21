# Privacy Stdlib Family

## Purpose

This document is the design home for the TOL privacy-family stdlib surface.

It is not limited to one contract.
It defines the package family shape for privacy-preserving value movement,
treasury control, allowance semantics, and selective disclosure over UNO rails.

## Scope

This document should own the stdlib-side design for:

- `ConfidentialVault`
- `ConfidentialEscrow`
- `ConfidentialPayment`
- `ConfidentialTreasury`
- `ConfidentialAllowance`
- `AuditorDisclosureBook`

It should also define how these packages connect to:

- UNO value rails
- receipts
- settlement
- trust and audit flows
- selective disclosure policies

## Non-goals

This document is not the place to fully specify:

- VM-level atomic execution
- low-level ZK proof systems
- GTOS cryptographic primitive internals

Those belong in GTOS protocol and crypto design documents.

## Current status

**Contract layer: COMPLETE (2026-03-21).** All 6 privacy-family contracts are
implemented, compile, and have package/artifact coverage:

| Contract | Purpose | UNO | Status |
|----------|---------|-----|--------|
| `ConfidentialVault` | Deposit/withdraw/auditor auth | Yes | Seed + runtime tests |
| `ConfidentialEscrow` | Escrow/release/refund on UNO rails | Yes | Seed + runtime tests + rollback tests |
| `ConfidentialPayment` | Batch/individual encrypted payments | Yes | Seed + compile test |
| `ConfidentialTreasury` | Multi-signer treasury with auditor disclosure | Yes | Seed + compile test |
| `ConfidentialAllowance` | Encrypted approve/transferFrom with expiry | Yes | Seed + compile test |
| `AuditorDisclosureBook` | Snapshot-based auditor disclosure management | No | Seed + compile test |

**Protocol layer: COMPLETE (verified 2026-03-21).** All three selective
disclosure layers are implemented in GTOS (`docs/SELECTIVE-DISCLOSURE.md`):

- Layer 1: **DisclosureProof** — DLEQ Sigma protocol for ZK amount/range proofs
- Layer 2: **DecryptionToken** — per-ciphertext audit tokens with DLEQ honesty proof
- Layer 3: **AuditorKey** — consensus-enforced regulatory disclosure via PolicyWallet

Remaining frontier areas (contract-level, not protocol-blocking):

- milestone-based confidential release
- privacy-aware receipt automation
- tighter integration with trust, sponsor, and settlement flows

## Core questions this document must answer

1. What is the canonical privacy-family package map?
2. Which flows should be first-class:
   payroll, treasury, escrow, subscription, allowance, audit, dispute?
3. What should be plain stdlib policy and what must be GTOS protocol?
4. How do confidential flows bind into receipts and settlement traces?
5. How does selective disclosure integrate with the family without turning each
   contract into a proof system?

## Proposed sections

1. Family goals and invariants
2. Contract-by-contract responsibilities
3. Shared data model and receipt hooks
4. Settlement integration
5. Audit and disclosure integration
6. GTOS dependencies
7. Threat model deltas
8. Release plan

## Initial work packages

- define the privacy-family architecture as one coherent package family
- identify which privacy behaviors should become shared helpers or interfaces
- define the boundary with GTOS selective disclosure
- define the receipt / settlement / audit linkage shape
- define milestone and recurring-payment implications for confidential flows

## Acceptance for this design doc

This document is ready for implementation work when it can answer:

- which privacy-family behaviors belong in stdlib
- which belong in GTOS / protocol
- which contracts are stable
- what test matrix is required before claiming closure

## Related documents

- `docs/AGENT_NATIVE_STDLIB_2046.md`
- `docs/STDLIB_CAPABILITY_ANALYSIS.md`
- `docs/STDLIB_THREAT_MODEL_MATRIX.md`
- `/home/tomi/gtos/docs/SELECTIVE-DISCLOSURE.md`

