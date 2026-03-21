# Agent-Native Stdlib Threat Model Matrix

This matrix is the first release baseline for the TOL stdlib.

It does not attempt to replace full audits.
Its purpose is narrower and practical:

- identify the primary trust boundary of each stdlib family
- make the key invariants explicit
- state the expected failure posture
- highlight where correct behavior still depends on runtime or protocol semantics

## Reading The Matrix

- `Trust boundary`: who or what the package must trust to behave correctly
- `Critical invariants`: conditions that must remain true for funds, authority, or receipts to stay safe
- `Failure posture`: whether the package should fail closed, fail open, or rely on explicit recovery
- `Runtime dependency`: which parts still depend on LVM/host/protocol behavior beyond pure contract logic

## Matrix

| Family | Trust boundary | Critical invariants | Failure posture | Runtime dependency |
| --- | --- | --- | --- | --- |
| `account` | account owner, guardian, delegate executor | daily limit never underflows; allowlist gates stay authoritative; delegate budget and expiry stay bounded | fail closed on overspend, suspension, or unauthorized execution | strong dependency on nested-call atomicity and external-call rollback |
| `authority` | grant writer / revoker | scope must be unique enough; nonce consumption must be monotonic; revocation must dominate reuse | fail closed on stale or replayed authority | moderate dependency on durable replay semantics across hosts |
| `execution_binding` | approver and receipt-binding consumer | single-use binding must never be consumable twice; expiry must dominate late execution; value must stay within ceiling | fail closed on replay, expiry, or over-limit use | moderate dependency on timestamp correctness and receipt-link consistency |
| `session_book` | session issuer and terminal-trust policy | session expiry must dominate reuse; step-up threshold must be enforced before spend; revoked session must become inert immediately | fail closed on inactive or degraded session | moderate dependency on trustworthy terminal/session evidence outside chain |
| `agreement` | counterparties and evidence finalizer | agreement state machine must remain monotonic; accepted terms must not mutate after acceptance; fulfill/cancel must be exclusive terminal outcomes | fail closed on invalid transition | low dependency beyond standard storage consistency |
| `settlement` | task poster, worker, dispute resolver | escrowed reward must map to exactly one terminal payout path; reject/dispute/resolve states must be exclusive; reclaim must only occur after expiry | fail closed on wrong status or wrong actor | strong dependency on host escrow/release correctness and rollback |
| `sponsor` | sponsor treasury owner and authorized relayer | relayer spend must not exceed budget; sponsor spend must not exceed deposits; policy hash must bind the route; failed downstream call must not silently consume budget semantics | fail closed on budget, expiry, or policy mismatch | very strong dependency on external-call atomicity, revert propagation, and accounting semantics |
| `evidence` | evidence writer / finalizer | evidence id must remain unique; finalization must be monotonic; challenge/settlement refs must preserve audit linkage | fail closed on duplicate or inconsistent evidence state | moderate dependency on off-chain proof systems and evidence availability |
| `receipt` | receipt writer | each receipt id must be unique; success/failure finalization must be terminal; binding/proof refs must remain stable once opened | fail closed on duplicate open or repeated finalize | low dependency in pure storage terms; higher dependency when used as settlement/audit anchor |
| `trust` | registry writer / scoring authority | eligibility and reputation updates must not fabricate stake or status; slash/suspend semantics must remain explicit and monotonic | fail closed on untrusted registry mutation | high dependency on external reputation, stake, or slashing systems |
| `privacy` | UNO bridge, resolver, disclosure policy | confidential balance movement must preserve ciphertext integrity; release/refund paths must be exclusive; disclosure permissions must not outlive revocation intent | fail closed on zero/invalid confidential value or unauthorized disclosure path | very strong dependency on UNO native rails, ciphertext helpers, and proof validity |
| `recovery` | guardian set, owner, recovery controller | freeze and rotate flows must not allow bypass of active guardian threshold; recovery must not silently resurrect revoked authority | fail closed during contested recovery; explicit recovery path required | moderate dependency on external guardian coordination and terminal-loss workflow |
| `discovery` | directory writer and market publisher | active/inactive flag must dominate routing; provider/manifest linkage must stay stable; capability metadata must not drift from actual contract surface | fail closed on inactive or mismatched service record | moderate dependency on off-chain discovery consumers respecting metadata |

## Cross-Cutting Risks

Several risks cut across nearly every family:

### 1. Nested call rollback — resolved for per-contract atomicity

Packages such as `account`, `settlement`, `sponsor`, and composed checkout
coordinators all rely on the assumption that downstream failure does not leave
upstream budget, receipt, or settlement state in a half-committed condition.

**Resolved (2026-03-21):**

Per-contract atomicity is now guaranteed at both layers:

- **GTOS LVM (on-chain):** StateDB snapshot/revert covers all call paths.
  Three hardening fixes added (`LVM.Call` balance revert, `tos.staticcall`
  defense-in-depth, `deployRawContract` snapshot scope).  Four regression tests
  in `lvm_rollback_test.go`.
- **Tolang test harness (off-chain):** `snapshotLuaStorage` /
  `revertLuaStorage` now snapshot `__tol_storage` before every call entry point
  and revert on error.  Six regression tests in
  `stdlib_composed_runtime_test.go` cover PolicyAccount, SponsorPolicyRelay,
  TaskSettlement, ReceiptBook, ConfidentialEscrow, and composed flows.

**Cross-contract atomicity — RESOLVED (2026-03-21):**
`tos.atomic_multicall` now provides all-or-nothing semantics for multi-contract
flows.  Coordinators can wrap sequential calls in a single atomic batch —
if any child fails, all mutations (including earlier successful calls) are
reverted.  See `/home/tomi/gtos/docs/Atomic-Execution-v1.md` for full design.

### 2. Receipt correctness is becoming a system invariant, not a helper feature

In the 2046 model, receipts are the machine-readable audit boundary.

If receipt linkage is wrong, agents can no longer reliably answer:

- who authorized execution
- who sponsored it
- which proof justified it
- whether settlement actually completed

Receipt integrity should be treated as a first-class protocol concern.

### 3. Package identity and package resolution still need stronger publishing semantics

The stdlib now has real release artifacts in `stdlib/releases/`, but package
identity is still more source-tree-aware than a mature public package ecosystem
should be.

Release artifacts help, but do not by themselves solve:

- global naming policy
- compatibility/version negotiation
- signed publisher identity policy
- package discovery semantics

### 4. Confidential value flows are only as strong as the UNO bridge beneath them

`ConfidentialVault` and `ConfidentialEscrow` are useful stdlib surfaces, but
their safety still depends on:

- native ciphertext arithmetic correctness
- correct `payable(uno)` handling
- trustworthy `uno.transfer(...)` semantics
- proof validation at the chain/runtime layer

The stdlib can make these flows usable.
It cannot, by itself, make the underlying confidential rails sound.

## Release Guidance

Before calling a stdlib family "commercially ready", the minimum bar should be:

1. deterministic `.toc/.tor` release artifact exists
2. compile/import tests exist
3. runtime tests exist for the family's state machine
4. at least one composed-flow test exists if the family is intended for orchestration
5. the family's trust boundary and failure posture are documented here

That bar is now substantially met for the current first-wave stdlib set, but
runtime rollback semantics and protocol backing remain the main blockers for a
true "production-complete" claim.
