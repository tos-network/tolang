# TOL 2046 Agent-Native Standard Library From First Principles

## Premise

The 2046 world is not "humans using wallet UIs to call contracts."

It is a world where persistent agents act across:

- weak terminals
- sponsored execution paths
- policy-bound accounts
- delegated sessions
- machine-readable trust boundaries
- auditable receipts
- selective disclosure and privacy constraints

In that world, the purpose of a TOL standard library is not to hand human
developers a bag of defensive utilities.

Its purpose is to give autonomous economic actors a canonical language for
expressing:

- who is acting
- under what authority
- toward what intent
- under which policy
- with what proof
- through which settlement path
- producing which receipt
- with which recovery and dispute escape hatches

That is the starting point.

## The question TOL openlib must answer

The right question is not:

> What helper contracts do developers often reuse?

The right question is:

> What must an autonomous agent be able to express, safely and
> machine-readably, in order to participate in economic life?

If TOL answers that question well, it becomes commercially usable.
If it does not, it remains a language with nice syntax and incomplete economic
semantics.

## The minimal semantic kernel

From first principles, an autonomous agent participating in economic activity
must be able to express the following semantic categories.

### 1. Identity and role

An agent must be able to express:

- who it is
- whom it represents
- whether it is the principal, operator, sponsor, merchant, provider, guardian,
  terminal, or auditor

This is more than `msg.sender`.
It is the role graph of the action.

### 2. Authority source and scope

An agent must be able to express:

- where authority comes from
- what functions or claims are allowed
- for how long
- with what budget
- under what revocation conditions

The core object here is not ownership.
It is scoped, bounded, inspectable authority.

### 3. Intent

An agent is not merely making a function call.
It is trying to satisfy an intent.

It must be able to express:

- desired outcome
- acceptable routes
- unacceptable routes
- expiry
- fallback logic
- supersession and replay boundaries

### 4. Policy

An agent must be able to express the constraints under which it may act:

- spending limits
- counterparty classes
- terminal trust requirements
- escalation thresholds
- delay windows
- privacy constraints
- compliance constraints

Policy is not metadata garnish.
It is the executable boundary of legitimate action.

### 5. Session and terminal context

The 2046 wallet is not one app on one device.

An agent must be able to express:

- which terminal class originated the request
- what trust tier that terminal has
- whether the session is weak, degraded, or high assurance
- whether step-up approval is required
- whether terminal loss or revocation has occurred

### 6. Counterparty admissibility

An agent must be able to express who it is willing to transact with.

That includes:

- capability requirements
- reputation floors
- allowlists or restricted classes
- service compatibility
- discovery metadata
- regulated or private-route requirements

### 7. Commitment form

Economic life is built from commitments, not raw calls.

An agent must be able to express structured commitments such as:

- quote
- offer
- acceptance
- task assignment
- delivery
- invoice
- subscription
- refund
- dispute

### 8. Resource exposure

An agent must be able to express how resources are committed:

- direct spend
- confidential spend
- escrow
- allowance reservation
- stake bond
- milestone release
- slashing exposure
- refund hold

The important concept is not transfer.
It is controlled exposure of value.

### 9. Evidence and proof dependence

An agent rarely settles on narrative alone.
It settles on evidence.

It must be able to express:

- oracle dependency
- attestation dependency
- proof references
- challenge windows
- verification route
- redacted versus full evidence paths

### 10. Routing and sponsorship

An agent must be able to express how execution is reached:

- who sponsors gas
- what sponsor policy applies
- what relayer is allowed
- what quote was accepted
- what fallback route is allowed
- how attribution is preserved

### 11. Receipt and audit surface

An agent must be able to express not only what it will do, but how the result
will be recorded.

That means:

- execution receipt
- approval linkage
- policy decision reference
- sponsor attribution
- signer and actor attribution
- proof reference linkage
- callback and timeout records

### 12. Recovery, revocation, and dispute

An agent economy that cannot recover is not safe for scale.

An agent must be able to express:

- revocation
- freeze
- guardian intervention
- recovery timelock
- terminal loss handling
- dispute opening
- challenge resolution
- post-recovery restricted mode

### 13. Privacy and disclosure boundary

An agent must be able to express what is disclosed to:

- counterparties
- terminals
- sponsors
- auditors
- regulators
- dispute resolvers

This is not the same thing as cryptography itself.
It is the policy of who may learn what, under what proof surface.

It also includes the question of whether value itself is handled in:

- plaintext rails
- confidential rails
- mixed public/private settlement paths

## What belongs in the language and what belongs in openlib

TOL should not try to solve everything in openlib.

Some things belong in the language and compiler:

- deterministic execution
- explicit effects
- gas bounds
- capability syntax
- delegation semantics
- reentrancy visibility
- machine-readable metadata emission

Those are language-level guarantees.

Openlib begins where reusable economic structure begins.

The openlib should standardize:

- reusable authority envelopes
- reusable policy patterns
- reusable agreement shapes
- reusable receipt schemas
- reusable sponsor and recovery flows
- reusable evidence and privacy interfaces
- reusable confidential-value interfaces over raw UNO bridge primitives

In short:

- language = execution guarantees
- openlib = economic semantics

## UNO bridge must be surfaced as openlib, not left as raw primitives

TOL already has a real confidential-value bridge:

- `uno.balance(addr)` for reading native encrypted balances
- `uno.transfer(to, ct)` for moving encrypted value from contract to native
  balance
- `payable(uno)` plus `msg.uno_value` for receiving encrypted deposits into
  contract execution

These are powerful low-level rails, but they are still rails.

Most application authors should not be forced to design commercial confidential
flows directly from these raw bridge points every time.

Therefore, openlib should treat UNO as a first-class substrate for reusable
package families, not merely as a compiler feature.

The goal is:

- language exposes `uno`
- openlib turns `uno` into canonical confidential business APIs

In practice, that means the openlib should wrap the raw bridge into reusable
contract surfaces such as:

- confidential deposit and withdrawal flows
- confidential escrow flows
- confidential recurring payment flows
- confidential payroll flows
- confidential merchant settlement flows
- confidential treasury and balance-book patterns
- selective disclosure and auditor-view attachments over encrypted state

This is the difference between "having encrypted arithmetic" and "having a
usable confidential commerce standard library."

## Deriving the openlib package map

If the semantic kernel above is correct, then the TOL openlib should be organized
around package families that correspond to recurring economic roles.

## Current implementation status

As of the current repo state, the package map below is no longer purely
aspirational.

- ✅ Implemented package seeds in `openlib/`:
  `account`, `authority`, `execution_binding`, `session`, `agreement`,
  `settlement`, `sponsor`, `evidence`, `receipt`, `trust`, `privacy`,
  `recovery`, `discovery`
- ✅ Compile/import coverage for every package seed plus composed example
  coverage in `openlib_packages_test.go`, `openlib_composition_test.go`, and
  `openlib_examples_test.go`
- ✅ Metadata, human-readable summary, discovery-manifest, and agent-package
  end-to-end coverage for composed examples in `e2e/openlib_examples_e2e_test.go`
- ✅ Runtime execution coverage for core openlib packages in
  `openlib_runtime_test.go`, including authority, execution binding, session,
  receipt, sponsor, evidence, recovery, trust, discovery, agreement,
  account, settlement, and privacy/UNO flows
- ✅ Composed runtime package-call coverage for `PolicySponsoredCheckout` and
  `PrivateServiceOrder` in `openlib_composed_runtime_test.go`
- ✅ Stateful composed runtime flows with real downstream openlib contract state
  in `openlib_composed_runtime_test.go`
- ✅ Stateful composed runtime write flows now mutate downstream openlib state
  through the coordinators themselves, including receipt finalization and
  agreement/settlement completion in `openlib_composed_runtime_test.go`
- ✅ Confidential escrow + receipt composed example coverage now exists in
  `examples/openlib_composed/PrivateEscrowCheckout.tol`,
  `openlib_examples_test.go`, `e2e/openlib_examples_e2e_test.go`, and
  `openlib_composed_runtime_test.go`
- ✅ Sponsored confidential checkout coverage now exists in
  `examples/openlib_composed/SponsoredPrivateEscrowCheckout.tol`, combining
  execution binding, sponsor relay, receipt finalization, confidential escrow,
  and real downstream call execution in `openlib_composed_runtime_test.go`
- ✅ Deterministic openlib release artifacts now exist under `openlib/releases/`,
  produced by `cmd/openlib-export` and locked by `openlib_release_test.go`
- ✅ Family-level openlib bundle packages now exist for multi-contract families,
  so package identity is not limited to per-contract `.tor` artifacts
- ✅ Family-local bundle catalogs now exist beside those bundle packages, so a
  consumer can resolve a multi-contract openlib family without first loading the
  global release index
- ✅ Family-level bundle discovery and agent-package metadata now exist beside
  those bundle packages, so discovery clients can consume a multi-contract
  family directly rather than reconstructing one from contract-level records
- ✅ A first-pass openlib threat model baseline now exists in
  `docs/STDLIB_THREAT_MODEL_MATRIX.md`
- ✅ Low-level external-call runtime coverage now executes real target contracts
  behind `target.call(data)` for sponsor/account paths, not just host stubs, in
  `openlib_runtime_test.go` and `openlib_composed_runtime_test.go`
- ✅ Confidential escrow seed and UNO runtime coverage are now implemented in
  `openlib/privacy/ConfidentialEscrow.tol` and `openlib_runtime_test.go`
- ✅ Direct `msg.uno_value.*` UNO method-call runtime coverage now has a focused
  regression test in `tol_ir_direct_lowering_uno_test.go`, closing an env-member
  type-inference/lowering gap exposed by openlib privacy flows

### `openlib/account`

Status: ✅ implemented seed in `openlib/account/PolicyAccount.tol`

Derived from:

- identity and role
- policy
- resource exposure
- recovery

What it standardizes:

- policy-bound accounts
- spend budgets
- allowlists
- terminal-aware spend control
- delegated account operation
- guardian and recovery flows
- emergency freeze

Current seeds:

- `examples/policy_wallet/PolicyWallet.tol`
- `examples/policy_wallet/SpendGuard.tol`
- `examples/policy_wallet/TerminalAuthority.tol`
- `examples/policy_wallet/GuardianRecovery.tol`
- `examples/policy_wallet/DelegatedAgent.tol`

### `openlib/authority`

Status: ✅ implemented seed in `openlib/authority/AuthorityBook.tol`

Derived from:

- authority source and scope
- counterparty admissibility
- recovery and revocation

What it standardizes:

- capability grants
- delegation grants
- scope envelopes
- expiry windows
- replay guards
- revocation records
- approval binding records

This package is the canonical surface for bounded authority, not "ownership."

### `openlib/execution_binding`

Status: ✅ implemented seed in `openlib/execution_binding/ExecutionBindingBook.tol`

Derived from:

- intent
- policy
- routing and sponsorship
- receipt linkage

What it standardizes:

- approval binding
- nonce and replay protection
- expiry windows
- policy snapshot binding
- sponsor policy binding
- proof-reference binding
- optional `intent_ref` / `plan_ref` anchors for cross-layer audit linkage

This package must be kept narrow.

`IntentEnvelope`, `PlanRecord`, route selection, fallback planning, approval
narrative, and intent lifecycle management belong to OpenFox and the shared
boundary layer, not to on-chain openlib.

The on-chain role is smaller:

- bind an approved execution to allowed authority
- bind execution to the relevant policy snapshot
- prevent replay and stale approval reuse
- preserve machine-readable linkage into receipts and proofs

So this package is not an on-chain intent engine.
It is the contract-side execution-binding layer that lets intent-native runtime
systems safely anchor approval and settlement to chain execution.

### `openlib/session`

Status: ✅ implemented seed in `openlib/session/SessionBook.tol`

Derived from:

- session and terminal context
- authority source and scope
- recovery and revocation

What it standardizes:

- terminal session grants
- trust-tier classes
- step-up rules
- session continuity
- session revocation
- lost-terminal recovery hooks
- degraded-mode authority limits

### `openlib/agreement`

Status: ✅ implemented seed in `openlib/agreement/CommercialAgreement.tol`

Derived from:

- commitment form
- counterparty admissibility
- evidence dependence

What it standardizes:

- quote
- offer
- acceptance
- invoice
- task order
- subscription agreement
- merchant acceptance agreement

This package is where TOL stops looking like a raw contract language and starts
looking like a language of machine commerce.

### `openlib/settlement`

Status: ✅ implemented — `TaskSettlement.tol` + `RecurringPayment.tol` (2026-03-21)

Derived from:

- resource exposure
- commitment form
- routing and sponsorship
- receipt and audit surface

What it standardizes:

- escrow
- confidential escrow
- staged release
- milestone payout
- refund windows
- split settlement
- reserve accounting
- slash distribution

For 2046 TOL, settlement should not assume plaintext by default.
Its canonical forms should support both:

- public value rails
- UNO confidential value rails

Current contracts:

- `openlib/settlement/TaskSettlement.tol` — task escrow with dispute/proof/receipt
- `openlib/settlement/RecurringPayment.tol` — subscription/periodic payment scheduler

Current seeds:

- `openlib/Task.tol`
- `examples/agent_economy/TaskEscrow.tol`
- `examples/agent_economy/MerchantPayment.tol`
- `examples/agent_economy/RecurringPayment.tol`

### `openlib/sponsor`

Status: ✅ implemented seed in `openlib/sponsor/SponsorPolicyRelay.tol`

Derived from:

- routing and sponsorship
- policy
- receipt and audit surface

What it standardizes:

- sponsor authorization
- sponsor budget and quota
- sponsor policy binding
- relayer permissions
- fallback sponsor routing
- sender versus sponsor attribution

Current seeds:

- `examples/agent_economy/SponsorRelay.tol`

### `openlib/evidence`

Status: ✅ implemented seed in `openlib/evidence/EvidenceBook.tol`

Derived from:

- evidence and proof dependence
- commitment form
- privacy and disclosure boundary

What it standardizes:

- oracle resolution
- attested values
- challenge windows
- proof reference attachment
- verification hooks
- verifiable read conventions

Current seeds:

- `openlib/Oracle.tol`
- `examples/agent_economy/OracleResolver.tol`

### `openlib/receipt`

Status: ✅ implemented seed in `openlib/receipt/ReceiptBook.tol`

Derived from:

- receipt and audit surface
- intent
- policy
- sponsorship

What it standardizes:

- execution receipts
- policy decision receipts
- approval linkage
- execution-binding linkage
- sponsor attribution
- proof references
- timeout and callback records
- settlement trace envelopes

This package is central because 2046 users and agents consume receipts, not raw
transaction internals.

### `openlib/trust`

Status: ✅ implemented seed in `openlib/trust/TrustRegistry.tol`

Derived from:

- counterparty admissibility
- resource exposure
- evidence dependence

What it standardizes:

- stake bonds
- scorer hooks
- reputation updates
- slash policy
- counterparty trust floors
- trust snapshot interfaces

This package converts reputation and stake from ambient chain data into
commercially reusable contract semantics.

### `openlib/privacy`

Status: ✅ all 6 contracts implemented (2026-03-21)

Derived from:

- privacy and disclosure boundary
- evidence dependence
- session and terminal context

What it standardizes:

- confidential value handling on UNO rails
- deposit/withdraw bridge patterns over `payable(uno)`, `msg.uno_value`,
  `uno.balance()`, and `uno.transfer()`
- selective disclosure flows
- bounded proof gates
- auditor views
- redacted receipts
- privacy terminal classes
- disclosure windows

This package matters because public terminals and weak terminals cannot be
treated as neutral observers.

It should be explicit that `openlib/privacy` is not only about disclosure after
the fact.

It is also about making confidential value usable during execution.

That means this package should provide friendly APIs over the raw UNO bridge,
for example:

- `ConfidentialVault`
- `ConfidentialEscrow`
- `ConfidentialPayment`
- `ConfidentialAllowance`
- `ConfidentialTreasury`
- `AuditorDisclosureBook`

These should let authors write business logic in terms of:

- confidential deposit
- confidential withdraw
- confidential reserve
- confidential payout
- disclosure authorization

rather than directly wiring low-level UNO bridge calls each time.

Current contracts:

- `openlib/privacy/ConfidentialVault.tol` — deposit/withdraw/auditor auth
- `openlib/privacy/ConfidentialEscrow.tol` — escrow/release/refund on UNO rails
- `openlib/privacy/ConfidentialPayment.tol` — batch/individual encrypted payments
- `openlib/privacy/ConfidentialTreasury.tol` — multi-signer treasury with auditor disclosure
- `openlib/privacy/ConfidentialAllowance.tol` — encrypted approve/transferFrom with expiry
- `openlib/privacy/AuditorDisclosureBook.tol` — snapshot-based auditor disclosure management

### `openlib/recovery`

Status: ✅ implemented seed in `openlib/recovery/RecoveryController.tol`

Derived from:

- recovery, revocation, and dispute
- session and terminal context
- authority source and scope

What it standardizes:

- guardian recovery
- authority rotation
- emergency freeze
- terminal-loss flows
- post-recovery cooldowns
- dispute opening and challenge paths

Some implementations will live near `account` and `authority`, but the semantic
surface is important enough to justify its own package family.

### `openlib/discovery`

Status: ✅ implemented seed in `openlib/discovery/ServiceDirectory.tol`

Derived from:

- counterparty admissibility
- identity and role
- routing and sponsorship

What it standardizes:

- service manifests
- capability advertisements
- version compatibility markers
- quote envelopes
- provider identity surfaces
- discovery-facing metadata conventions

This package is how agent-to-agent commerce becomes discoverable rather than
hardcoded.

## The package unit should be a protocol package, not a helper file

A serious TOL openlib package should not be only a reusable `.tol` snippet.

It should normally ship:

1. Canonical interface contracts
2. Reference implementations
3. Manifest templates
4. Standard event schemas
5. Standard receipt schemas
6. Recommended `@effects`, `@bounds`, `@gas`, `@delegated`, and `@verifiable`
   profiles
7. Threat model notes
8. Discovery-facing metadata conventions
9. End-to-end examples

That is because the reusable unit in an agent economy is not just code.
It is a reusable economic protocol surface.

## Implementation order

Not every package family should land at once.

The right order is driven by what an agent must express earliest to behave
safely in production.

### Control-plane wave

These packages define who may act, under what policy, from which surface:

1. `account` ✅
2. `authority` ✅
3. `execution_binding` ✅
4. `session` ✅
5. `recovery` ✅

Without this wave, TOL cannot safely support multi-terminal, delegated,
sponsored consumer finance.

### Execution-plane wave

These packages define how commercial actions actually commit, settle, and get
recorded:

1. `agreement` ✅
2. `settlement` ✅
3. `sponsor` ✅
4. `evidence` ✅
5. `receipt` ✅

Without this wave, there is no canonical machine-commerce layer.

### Market-plane wave

These packages define how agents choose counterparties, protect private state,
and reason about trust:

1. `trust` ✅
2. `privacy` ✅
3. `discovery` ✅

Without this wave, there is no scalable marketplace for autonomous services.

## Implications for the current repo

The current repo already contains useful seeds:

- `openlib/Task.tol`
- `openlib/Oracle.tol`
- `openlib/Vote.tol`
- `examples/policy_wallet/*`
- `examples/agent_economy/*`
- `examples/confidential_vault/*`
- `examples/confidential_token/*`
- `examples/private_*/*`

But these seeds are still mostly mechanism-level.

The next step is to evolve from:

- isolated patterns

to:

- canonical package families with standard authority, receipt, manifest, and
  discovery semantics

Concretely, that means:

- from `Task.tol` to `agreement` plus `settlement`
- from `Oracle.tol` to `evidence`
- from `Vote.tol` to governance, guardian, and policy-specific vote packages
- from raw UNO bridge primitives to `privacy` and confidential-settlement
  packages
- from vague on-chain "intent handling" to explicit execution-binding
  primitives
- from examples to importable, versioned, documented protocol packages

## Current execution frontier

The openlib and release pipeline are no longer the main unfinished layer.

The current highest-value remaining task is:

`Finish the deepest remaining VM/protocol gap for Tolang/GTOS: nested-call rollback and atomic execution semantics across LVM, with cross-stack tests and docs.`

This is not a greenfield task.
The openlib and release pipeline are already substantially complete.

### Current context

- The guiding design is this document:
  `docs/AGENT_NATIVE_STDLIB_2046.md`
- The threat model is:
  `docs/STDLIB_THREAT_MODEL_MATRIX.md`
- The exposed framework/runtime gaps are tracked in:
  `docs/TOLANG_SHORTCOMINGS.md`
- Openlib families, release artifacts, discovery metadata, agent package
  metadata, GTOS package target validation, explicit gas caps, typed custom
  reverts, and deployed TOL metadata RPC are already implemented
- The highest-value unresolved gap is nested call rollback and atomicity across
  account, sponsor, settlement, and package-call flows

### Mission

Implement and harden nested-call rollback semantics and atomic execution
behavior in GTOS/LVM, then prove it with cross-stack tests in both repos.

### Definition of done

- Failed child calls do not leave half-committed upstream state
- Value transfer, storage mutation, receipt state, sponsor/account budget
  state, and settlement state obey clear rollback semantics
- Revert data still propagates correctly
- Raw Lua compatibility is preserved
- TOL's 32-byte agent normalization boundary is preserved
- Existing openlib, runtime, and release behavior is not broken
- New tests exist and pass

### Important constraints

- Do not add new openlib families
- Do not broaden scope into unrelated protocol systems
- Do not revert unrelated changes
- Keep changes minimal and defensible
- Prefer primary fixes in GTOS/LVM and only add TOL changes when needed for
  tests or clear contract-boundary correctness
- Preserve existing behavior for explicit gas caps, typed custom errors,
  package target validation, and deployed metadata RPC

### Required reading before implementation

- `/home/tomi/tolang/docs/AGENT_NATIVE_STDLIB_2046.md`
- `/home/tomi/tolang/docs/STDLIB_THREAT_MODEL_MATRIX.md`
- `/home/tomi/tolang/docs/TOLANG_SHORTCOMINGS.md`
- `/home/tomi/gtos/core/vm/lvm.go`
- `/home/tomi/tolang/openlib_runtime_test.go`
- `/home/tomi/tolang/openlib_composed_runtime_test.go`
- `/home/tomi/gtos/core/lvm_tol_e2e_test.go`
- `/home/tomi/gtos/core/lvm_tol_openlib_e2e_test.go`

### Parallel execution plan

The work should be split across four agents with disjoint ownership.

#### Agent 1: Rollback semantics analysis

Ownership:

- read-only analysis across `/home/tomi/gtos/core/vm/lvm.go` and relevant
  tests

Task:

- map current snapshot/revert behavior for `call`, `staticcall`,
  `delegatecall`, `package_call`, and `create` paths
- identify exactly where atomicity is incomplete or ambiguous
- produce a short actionable implementation note for the main agent
- do not edit files unless explicitly necessary

#### Agent 2: GTOS VM implementation

Ownership:

- `/home/tomi/gtos/core/vm/lvm.go`
- small helper additions in the same VM area if needed

Task:

- implement the rollback/atomicity fix
- ensure nested child failure restores the correct state boundary
- preserve revert-data propagation
- preserve raw Lua behavior
- do not touch `tosapi` or unrelated subsystems unless required

#### Agent 3: GTOS regression and end-to-end tests

Ownership:

- `/home/tomi/gtos/core/*test.go`
- `/home/tomi/gtos/internal/tosapi/*test.go` only if required

Task:

- add focused regression tests covering:
  - nested call storage rollback
  - nested call value rollback
  - `package_call` rollback
  - sponsor/account path rollback
  - structured custom revert propagation through failed nested execution
- prefer minimal, direct tests that fail before the fix and pass after

#### Agent 4: Tolang openlib and composed-flow regressions

Ownership:

- `/home/tomi/tolang/openlib_runtime_test.go`
- `/home/tomi/tolang/openlib_composed_runtime_test.go`
- `/home/tomi/tolang/e2e/*` only if needed
- `/home/tomi/tolang/docs/TOLANG_SHORTCOMINGS.md` and
  `/home/tomi/tolang/docs/STDLIB_THREAT_MODEL_MATRIX.md` if semantics change
  materially

Task:

- add cross-stack regression tests proving that failed composed flows do not
  leave half-committed state in:
  - `PolicyAccount`
  - `SponsorPolicyRelay`
  - `TaskSettlement`
  - `ReceiptBook`
  - `ConfidentialEscrow` where relevant
- update docs only if the implemented rollback semantics materially clarify a
  previously open shortcoming

### Coordination rules

- The main agent integrates results
- Agents must not overwrite each other's files
- Trust the analysis agent's codepath mapping unless there is hard evidence it
  is wrong
- Do not duplicate work
- Keep the critical path moving

### Acceptance criteria

- `/home/tomi/gtos`: `go test ./core/...`
- `/home/tomi/gtos`: `go test ./internal/tosapi`
- `/home/tomi/gtos`: `go test -cover ./core/... ./internal/tosapi`
- `/home/tomi/tolang`: `go test ./...`
- If any openlib release artifact or metadata output changes:
  - `/home/tomi/tolang`: `go run ./cmd/openlib-export`
  - `/home/tomi/tolang`: `go test -run 'TestOpenlibReleaseArtifactsAreCurrent' -v .`
- Final report must state:
  - exact rollback semantics now guaranteed
  - files changed
  - tests added
  - remaining unresolved protocol gaps after this task

### Delegation packet form

If this work is delegated to a parallel coding system such as Claude Code, the
kickoff prompt should be:

`Start the 4 agents now. Prioritize nested-call rollback and atomicity. Do not expand scope into new openlib families.`

A more aggressive execution posture is:

`Do not stop at analysis. Carry the fix through implementation, regression tests, full verification, and final integration in both repos.`

## Remaining capability backlog from the implementation audit

The openlib seeds, release artifacts, runtime coverage, and discovery surfaces
are now substantially complete.  The five major follow-on gaps (cross-contract
atomicity, privacy family completion, recurring payments, `@requires` syntax,
and selective disclosure) are all resolved. The next six follow-on delivery
items are also now closed in code:

- slash distribution
- canonical auto-receipt binding
- named terminal / trust taxonomy
- stronger reusable step-up enforcement
- v1 privacy composition helper coverage
- typed discovery / capability schema normalization

The capability audit in `docs/STDLIB_CAPABILITY_ANALYSIS.md` now treats those
as resolved implementation work, not open backlog.

### Missing contracts — RESOLVED (2026-03-21)

All previously missing privacy-family contracts are now implemented in
`openlib/privacy/`:

- ~~`ConfidentialPayment`~~ — **IMPLEMENTED**: batch and individual encrypted payment flows
- ~~`ConfidentialTreasury`~~ — **IMPLEMENTED**: multi-owner confidential treasury with selective disclosure
- ~~`ConfidentialAllowance`~~ — **IMPLEMENTED**: encrypted allowance and approval patterns
- ~~`AuditorDisclosureBook`~~ — **IMPLEMENTED**: structured auditor disclosure with snapshot-oriented disclosure records

### Missing control-plane capability features

The previous control-plane gaps are now resolved in the current openlib wave:

- ~~`PolicyAccount` still lacks per-role or per-employee spend caps~~ —
  **RESOLVED (2026-03-21)**: `setDelegateCaps(...)` and execute-path
  enforcement now provide delegate-scoped daily/single caps
- ~~`SessionBook` and related flows still lack single-call convenience APIs such
  as `require_terminal(...)`~~ — **RESOLVED (2026-03-21)**:
  `requireTerminal(...)` exists as a convenience enforcement wrapper
- ~~terminal and trust modeling is still represented as raw `u256` constants
  rather than a named `6 terminal types x 5 trust tiers` semantic matrix~~ —
  **RESOLVED (2026-03-21)**: `SessionBook` now ships named terminal/trust
  constants and rejects out-of-taxonomy values at runtime
- ~~step-up logic is still too query-oriented~~ —
  **RESOLVED (2026-03-21)**: `SessionBook.enforceStepUp(...)` and
  `requireTerminal(...)` provide canonical reusable enforcement guards

### Missing execution-plane capability features

These are the main remaining commercial-flow gaps:

- ~~recurring and subscription payments~~ — **RESOLVED (2026-03-21)**:
  `RecurringPayment` contract provides subscribe/execute/pause/resume/cancel
  lifecycle; protocol-level native scheduler pending (see
  `/home/tomi/gtos/docs/Native-Scheduled-Tasks.md`)
- ~~milestone staged release~~ — **RESOLVED (2026-03-21)**:
  `TaskSettlement.openMilestoneTask` / `completeMilestone` / `milestoneStatusOf`
  with floor-division remainder handling and partial-reclaim support
- ~~slash distribution~~ — **RESOLVED (2026-03-21)**:
  `TaskSettlement.setSlashPolicy(...)` now precommits a configurable dispute
  split before submission/dispute and is enforced at resolution time
- ~~auto-receipt binding~~ — **RESOLVED (2026-03-21)**:
  `TaskSettlement` now binds `ReceiptBook.openReceipt(...)` /
  `finalizeSuccess(...)` / `finalizeFailure(...)` into approval, dispute,
  cancellation, reclaim, and final milestone settlement paths
- ~~invoice sub-type~~ — **RESOLVED (2026-03-21)**:
  `CommercialAgreement.createInvoice` / `agreementTypeOf` (TYPE_OFFER=1,
  TYPE_INVOICE=2)
- subscription lifecycle is handled by `RecurringPayment`
  it is not modeled as a separate `CommercialAgreement` sub-type

### Missing market-plane capability features

These are the main remaining scale-out and privacy gaps:

- ~~reputation writes and scorer callbacks~~ — **RESOLVED (2026-03-21)**:
  `TrustRegistry.updateReputation` / `setScorerCallback`; effective reputation
  composes native baseline + local delta
- ~~per-agreement or per-service stake lock semantics~~ — **RESOLVED (2026-03-21)**:
  `TrustRegistry.lockStake` / `unlockStake` / `lockedStakeOf`
- ~~structured discovery fields~~ — **RESOLVED (2026-03-21)**:
  `ServiceDirectory` now exposes typed service/capability/pricing/privacy/
  receipt/trust-floor fields and exporter-generated `typed_discovery` metadata
- ~~selective disclosure is now RESOLVED at the GTOS protocol layer~~ —
  **RESOLVED (2026-03-21)**: GTOS provides DisclosureProof, DecryptionToken,
  and AuditorKey; openlib now also has stateful `PrivateDisputeEscrow` helper
  coverage for privacy-aware dispute/refund coordination

### Missing compiler and language features — RESOLVED (2026-03-21)

- ~~`@requires(caller: Cap)`~~ — **RESOLVED**: compiler pipeline implemented
  and tested (parser/sema/lower/codegen/ABI); 3 tests; design doc at
  `docs/CALLER_CAPABILITY_SYNTAX.md`

### Priority order for the next evolution wave

The following items have been resolved:

- ~~close the privacy-family contract gap~~ — **RESOLVED**: all 6 implemented
- ~~close recurring settlement~~ — **RESOLVED**: `RecurringPayment` implemented
- ~~close compiler semantics: `@requires(caller: Cap)`~~ — **RESOLVED**: pipeline implemented + tested

The first openlib closure wave is now complete.

Next evolution order:

1. keep tightening release/discovery/threat-model documentation and exporter
   surfaces as the GTOS-native protocol layers above land

### Acceptance bar for capability-complete closure

These backlog items should not be considered closed merely because a seed
contract exists.

For each remaining item, the closure bar should be:

- implementation exists in openlib or compiler
- compile/import coverage exists
- runtime or composed-flow tests exist where applicable
- release metadata and discovery semantics are updated when applicable
- the threat-model implications are reflected in
  `docs/STDLIB_THREAT_MODEL_MATRIX.md` when materially changed

## Document ownership of the five major follow-on gaps

These five workstreams are different in scale and should not be collapsed into
one implementation bucket.

They should be documented as follows:

| Gap | Nature | Primary summary document | Capability / shortcoming document | Detailed design home |
| --- | --- | --- | --- | --- |
| Cross-contract atomicity | VM / protocol — **RESOLVED**: `tos.multicall` implemented | `docs/AGENT_NATIVE_STDLIB_2046.md` | `docs/TOLANG_SHORTCOMINGS.md` | `/home/tomi/gtos/docs/Atomic-Execution-v1.md` |
| Privacy family completion | openlib family — **RESOLVED**: all 6 contracts implemented | `docs/AGENT_NATIVE_STDLIB_2046.md` | `docs/STDLIB_CAPABILITY_ANALYSIS.md` | `docs/PRIVACY_STDLIB_FAMILY.md` |
| Recurring / subscription settlement | openlib — **RESOLVED**: `RecurringPayment` contract; protocol scheduler pending | `docs/AGENT_NATIVE_STDLIB_2046.md` | `docs/STDLIB_CAPABILITY_ANALYSIS.md` | `/home/tomi/gtos/docs/Native-Scheduled-Tasks.md` |
| `@requires(caller: Cap)` | language / compiler — **RESOLVED**: pipeline implemented + tested | `docs/AGENT_NATIVE_STDLIB_2046.md` | `docs/TOLANG_SHORTCOMINGS.md` | `docs/CALLER_CAPABILITY_SYNTAX.md` |
| Selective disclosure (`ZK + token`) | privacy / protocol — **RESOLVED**: all 3 layers in GTOS | `docs/AGENT_NATIVE_STDLIB_2046.md` | `docs/STDLIB_CAPABILITY_ANALYSIS.md` | `/home/tomi/gtos/docs/SELECTIVE-DISCLOSURE.md` plus `docs/PRIVACY_STDLIB_FAMILY.md` |

### Status note

All five items above are now resolved (2026-03-21).  Their design homes remain
stable for future evolution:

- atomicity — **RESOLVED** (`tos.multicall`); design home: GTOS / LVM
- privacy-family completion — **RESOLVED** (all 6 contracts); design home: openlib family
- recurring settlement — **RESOLVED** (`RecurringPayment`); design home: scheduler + settlement
- caller capability syntax — **RESOLVED** (compiler pipeline + tests); design home: compiler
- selective disclosure — **RESOLVED** (all 3 GTOS layers); design home: GTOS privacy/protocol

### Design homes for the next evolution wave

The current openlib closure wave is complete. These design homes now describe
the next GTOS-owned protocol wave, not unresolved correctness gaps in the
present openlib codebase.

| Next evolution item | Primary implementation surface | Detailed design home |
| --- | --- | --- |
| GTOS-native settlement bus and receipt hooks | GTOS VM/runtime + system receipt surface | `/home/tomi/gtos/docs/GTOS_SETTLEMENT_BUS_AND_RECEIPT_HOOKS.md` |
| Registry governance and revocation workflows | GTOS protocol registries + sysactions + RPC inspection | `/home/tomi/gtos/docs/GTOS_PROTOCOL_REGISTRIES.md` |
| Release/discovery/threat-model tightening (current closure wave complete; future protocol consumers may extend it) | Tolang docs + exporter + release/profile manifests | `docs/STDLIB_THREAT_MODEL_MATRIX.md` plus `docs/AGENT_NATIVE_STDLIB_2046.md` |

The Tolang release/export layer now also emits an additive
`protocol_alignment` marker in `.profile.json`, `.discovery.json`, and
`.agentpkg.json` so the next GTOS wave can consume machine-readable hints for:

- settlement bus alignment
- registry governance alignment
- package governance alignment

This is preparatory metadata only. It does not depend on the GTOS runtime
surfacing those features yet.

The release/export surface has now tightened one step further:

- per-contract `.profile.json` remains the canonical agent-facing contract view
- family bundles now also emit `.bundle.profile.json`
- per-contract and family bundle artifacts now also emit machine-readable
  `threat_model` sections derived from the openlib threat matrix
- GTOS deployed metadata RPC now returns the unified per-contract `profile`
  and package-level `bundle_profile`, alongside `discovery`, `agent_package`,
  and `routing_profile`
- GTOS agent discovery card consumption now exposes a structured `parsed_card`
  view for standard card fields such as capability entries, `routing_profile`,
  `threat_model`, and release/profile refs, while keeping raw `cardJson`
  compatibility
- GTOS now also builds a recommended `suggested_card` directly from unified
  contract/bundle metadata so discovery clients can bootstrap a standard card
  shape without hand-assembling routing/threat/profile hints
- GTOS discovery APIs can now publish that recommended `suggested_card`
  directly from deployed contract/package metadata, so discovery providers do
  not need to hand-assemble standard card JSON before advertising an agent
- GTOS also exposes a read-only suggested-card path for deployed `.toc` and
  `.tor` code, so clients can fetch the canonical structured card shape
  without publishing it into discovery first
- the first client-consumption slice is now in place too: `tosclient`
  exposes typed wrappers for discovery info/search/card methods and for
  suggested-card fetch/publication; it now also exposes a typed
  `GetContractMetadata(...)` wrapper so agent runtimes no longer need to
  hand-assemble raw `tos_*` JSON RPC payloads to consume the unified profile,
  routing, package trust, and discovery surface; the same client surface now
  also exposes typed reads for capability/delegation/package/publisher/
  verifier/verification/pay-policy/agent-identity governance data
- GTOS now also exposes a higher-level `gtosclient.GetAgentRuntimeSurface(...)`
  helper that normalizes deployed `.toc` / `.tor` metadata into a single
  client-side object carrying the effective profile or bundle profile,
  routing hints, suggested discovery card, and package trust/publisher facts
- GTOS now also exposes `gtosclient.GetDiscoveredAgentSurface(...)`, which
  starts from a published discovery card and, when the structured
  `parsed_card.agent_address` is canonical, joins it directly with the
  deployed runtime metadata surface so agent runtimes can go from discovery
  to callable on-chain semantics in one client helper
- GTOS now also exposes `gtosclient.SearchDiscoveredAgentSurfaces(...)` and
  `DirectorySearchDiscoveredAgentSurfaces(...)`, which take discovery search
  results and return joined card + runtime surfaces for each match, so
  capability search can flow directly into callable contract/package metadata
- GTOS now also exposes `gtosclient.SearchTrustedAgentSurfaces(...)` and
  `DirectorySearchTrustedAgentSurfaces(...)`, which apply a first-pass trust
  gate over joined provider surfaces (registered, not suspended, on-chain
  capability present, and package trust when applicable) and sort the
  survivors by local rank score
- GTOS now also exposes `gtosclient.SearchPreferredAgentSurfaces(...)`,
  `DirectorySearchPreferredAgentSurfaces(...)`, and
  `SelectPreferredAgentSurface(...)`, which layer connection-mode, package
  prefix, typed-routing, disclosure-readiness, and minimum-trust
  preferences on top of the trusted/ranked provider surface
- GTOS now also exposes `gtosclient.ResolvePreferredAgentSurface(...)` and
  `ResolveDirectoryPreferredAgentSurface(...)`, which collapse discovery
  search, runtime join, trust filtering, ranking, and preference selection
  into one end-to-end provider resolution helper for agent runtimes
- GTOS now also exposes
  `gtosclient.SearchPreferredAgentSurfaceDiagnostics(...)` and
  `DirectorySearchPreferredAgentSurfaceDiagnostics(...)`, which return
  per-provider trust failures, preference failures, and `preferred` state so
  agent runtimes can explain selection outcomes instead of treating provider
  filtering as a black box
- `tosdk` now mirrors that higher-level discovery consumption layer for
  TypeScript clients via `searchPreferredAgentProvider(...)`,
  `directorySearchPreferredAgentProvider(...)`, and the matching
  `...WithDiagnostics(...)` helpers; it now also exposes
  `summarizeAgentProviderDiagnostics(...)`, `requirePreferredAgentProvider(...)`,
  and the matching `...OrThrow(...)` helpers, so app code can either keep
  structured diagnostics or fail with a stable, explainable error string
- OpenFox now consumes the same typed-discovery wave end to end: published
  cards can embed `agent_address`, package/profile refs, `routing_profile`,
  and `threat_model` hints, and OpenFox now exposes
  `resolveCapabilityProvider(...)`, `diagnoseCapabilityProviders(...)`, and
  `resolveCapabilityProviderWithDiagnostics(...)`; its higher-level request
  paths also now surface provider-selection failure reasons instead of only
  throwing opaque `No provider found` errors, so runtime-level selection is
  explainable rather than opaque; the same explainability now extends to
  signer/paymaster CLI discovery and the `discover_capability_providers`
  tool, which surface package/routing hints and provider-selection failures;
  the same diagnostics now also feed OpenFox runtime/orchestration paths,
  including gateway session selection, solver bounty discovery, and
  opportunity scouting, which persist the last provider-selection explanation
  into local state so planner/execution layers can explain why no provider
  was selected; OpenFox execution paths now also automatically fall back
  across ranked discovery providers and feed per-provider success/failure
  outcomes back into the existing local ranking model, so future execution
  selection stops preferring the same failing provider when a viable
  alternative exists
- legacy `.discovery.json` / `.agentpkg.json` bundle artifacts remain for
  compatibility while consumers migrate

### GTOS protocol design homes for the next stage

Some of the next-stage work is no longer primarily a Tolang openlib problem.
It requires GTOS-side protocol, runtime, or publishing changes.

Those GTOS-owned design homes are:

| GTOS protocol workstream | Why GTOS must change | Detailed design home |
| --- | --- | --- |
| Protocol registries for capability / delegation / verification / settlement-policy / agent identity | Tolang can express these semantics, but GTOS must provide canonical registry-backed truth, revocation, and query surfaces | `/home/tomi/gtos/docs/GTOS_PROTOCOL_REGISTRIES.md` |
| LVM-native economic primitives | `package_call`, capability routing, `agentload`, `escrow/release`, and UNO rails need stable VM/runtime-native semantics rather than host-shaped conventions | `/home/tomi/gtos/docs/LVM_NATIVE_ECONOMIC_PRIMITIVES.md` |
| Settlement bus and receipt hooks | first-wave openlib settlement now works, but GTOS still needs a protocol-native bus for atomic value movement, receipt finalization, sponsor/escrow/refund joins, and public/UNO rail normalization | `/home/tomi/gtos/docs/GTOS_SETTLEMENT_BUS_AND_RECEIPT_HOOKS.md` |
| Package identity and publishing registry | local package resolution is not enough for agent-network trust; publisher identity, version/channel, and revocation need a protocol-grade model if GTOS adopts network publishing | `/home/tomi/gtos/docs/PACKAGE_PUBLISHING_REGISTRY.md` |

Practical split:

- Tolang should continue to own expression, openlib composition, metadata, and
  exporter shape.
- GTOS should own protocol registries, VM-native economic primitives, and any
  network-grade package publishing identity model.

Current GTOS implementation status (2026-03-22):

- protocol registries now have a concrete v1:
  capability, delegation, verification, pay-policy, package, publisher, and
  agent-identity query surfaces are implemented in GTOS
- registry governance and revocation workflows are now implemented on top of
  registry v1: capability records have explicit owners; delegation
  grant/revoke is scoped to the principal with governor override; verifier and
  pay-policy records now support owner/controller-aware revocation with
  governor override; capability, delegation, verifier, verification-claim,
  and pay-policy RPC responses now expose lifecycle metadata
  (`created_at`, `updated_at`) and controller/owner fields where applicable
- LVM now consumes protocol-backed capability / delegation / verification /
  pay-policy state instead of leaving those surfaces as permissive stubs
- package publishing is no longer design-only: GTOS now has package/publisher
  state, sysactions, RPC lookup by name/version/hash, latest-by-channel
  indexes, and deployed metadata joins that expose published package identity
  plus publisher trust
- package namespace + publisher governance is now resolved on top of that v1:
  package/publisher registry v1.3 records lifecycle timestamps
  (`created_at`, `updated_at`), enforces controller-or-governor authorization
  for publisher/package lifecycle changes, models publisher suspension/resume,
  and exposes those governance facts through package/publisher RPC payloads
- LVM now also exposes runtime-backed inspection primitives over GTOS protocol
  state: `tos.agentinfo(...)`, `tos.packageinfo(...)`,
  `tos.packagelatest(...)`, and `tos.publisherinfo(...)`
- escrow / release semantics now have explicit VM-level rollback coverage:
  reserve/release/slash balance movement is tested, and both top-level revert
  and nested-call failure restore the escrow ledger correctly
- UNO runtime-contract normalization is now resolved:
  GTOS exposes `tos.uno_value`, `tos.uno_balance(...)`, and
  `tos.uno_transfer(...)` as explicit VM-native UNO rails, preserves
  `tos.ciphertext.balance/transfer` compatibility, fails closed on malformed
  addresses, and has rollback coverage for both top-level revert and nested
  call failure
- settlement bus and receipt hooks now have a concrete v1:
  GTOS exposes `tos.settle(...)`, `tos.settle_refund(...)`,
  `tos.settle_escrow(...)`, `tos.receipt_open/success/failure/info`, and
  `tos.settlement_info(...)`, backed by stateful `RuntimeReceipt` and
  `SettlementEffect` records in `settlement/`, `PublicSettlementAPI` query
  methods, and VM/runtime tests for public transfer, split-phase receipt
  finalization, escrow release, UNO settlement, and rollback on missing/open
  receipt preconditions

What remains for the next GTOS-owned wave is no longer "make openlib work."
The current GTOS/tolang/tosdk/openfox closure wave is now complete through:

- protocol registries + governance
- package publishing trust + namespace governance
- settlement bus + receipt hooks v1
- unified release/profile/discovery/threat-model export
- GTOS metadata/discovery/client consumption
- higher-level TypeScript and OpenFox provider selection, diagnostics, and
  execution fallback

The next post-closure wave is now:

1. `LVM vs openlib boundary cleanup` — **RESOLVED (2026-03-22)**
   the stack now exports a machine-readable `runtime_boundary` profile and a
   dedicated design home at `docs/LVM_VS_OPENLIB_BOUNDARY.md`, explicitly
   distinguishing GTOS-native protocol semantics, preferred openlib
   entrypoints, and future system-contract targets.
2. `Settlement-bus adoption cleanup`
   standardize how openlib and higher-level runtimes consume
   `tos.settle(...)`, `tos.receipt_*`, and runtime settlement records so
   public rail, UNO rail, escrow, sponsor, and receipt flows converge on one
   stable developer-facing model.
3. `OpenFox / SDK orchestration policy layer`
   lift discovery/runtime/trust/fallback logic into reusable execution policy
   bundles so planner/executor paths stop embedding one-off selection logic.
4. `Governance v2 hardening`
   extend registry/package governance beyond v1 lifecycle metadata and basic
   governor override into clearer dispute, namespace, revocation-propagation,
   and operator workflow semantics.

Execution prompt for this GTOS-owned wave:

- Claude Code parallel implementation prompt:
  `docs/CLAUDE_CODE_GTOS_PROTOCOL_IMPLEMENTATION.md`

That prompt is designed to execute directly against the GTOS design
homes above:

- `/home/tomi/gtos/docs/GTOS_PROTOCOL_REGISTRIES.md`
- `/home/tomi/gtos/docs/LVM_NATIVE_ECONOMIC_PRIMITIVES.md`
- `/home/tomi/gtos/docs/GTOS_SETTLEMENT_BUS_AND_RECEIPT_HOOKS.md`
- `/home/tomi/gtos/docs/PACKAGE_PUBLISHING_REGISTRY.md`

## A hard rule

TOL openlib should standardize economic flows, not only safety helpers.

It should answer questions like:

- How does a weak terminal request a bounded merchant payment?
- How does an agent receive scoped authority to operate an account?
- How does a sponsor bind itself to a policy-approved route?
- How does an off-chain approval become a replay-safe execution binding?
- How does a task completion proof become a payout receipt?
- How does a recovery flow revoke old terminal authority?
- How does an auditor receive a redacted but sufficient proof surface?
- How does a discovery client know a contract is safe to compose with?

If the openlib can answer those questions coherently, TOL becomes a real
agent-native platform.

## Bottom line

The first-principles result is simple:

TOL openlib is not primarily a security-helper library.

It is the canonical package layer for expressing:

- authority
- policy
- session context
- commitment form
- resource exposure
- evidence dependence
- sponsorship
- receipts
- recovery
- trust
- privacy
- discovery

That is what an autonomous agent must naturally express in order to participate
in economic life.

That is what TOL openlib should standardize.
