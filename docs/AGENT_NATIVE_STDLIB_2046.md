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

## The question TOL stdlib must answer

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

## What belongs in the language and what belongs in stdlib

TOL should not try to solve everything in stdlib.

Some things belong in the language and compiler:

- deterministic execution
- explicit effects
- gas bounds
- capability syntax
- delegation semantics
- reentrancy visibility
- machine-readable metadata emission

Those are language-level guarantees.

Stdlib begins where reusable economic structure begins.

The stdlib should standardize:

- reusable authority envelopes
- reusable policy patterns
- reusable agreement shapes
- reusable receipt schemas
- reusable sponsor and recovery flows
- reusable evidence and privacy interfaces
- reusable confidential-value interfaces over raw UNO bridge primitives

In short:

- language = execution guarantees
- stdlib = economic semantics

## UNO bridge must be surfaced as stdlib, not left as raw primitives

TOL already has a real confidential-value bridge:

- `uno.balance(addr)` for reading native encrypted balances
- `uno.transfer(to, ct)` for moving encrypted value from contract to native
  balance
- `payable(uno)` plus `msg.uno_value` for receiving encrypted deposits into
  contract execution

These are powerful low-level rails, but they are still rails.

Most application authors should not be forced to design commercial confidential
flows directly from these raw bridge points every time.

Therefore, stdlib should treat UNO as a first-class substrate for reusable
package families, not merely as a compiler feature.

The goal is:

- language exposes `uno`
- stdlib turns `uno` into canonical confidential business APIs

In practice, that means the stdlib should wrap the raw bridge into reusable
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

## Deriving the stdlib package map

If the semantic kernel above is correct, then the TOL stdlib should be organized
around package families that correspond to recurring economic roles.

## Current implementation status

As of the current repo state, the package map below is no longer purely
aspirational.

- ✅ Implemented package seeds in `stdlib/`:
  `account`, `authority`, `execution_binding`, `session`, `agreement`,
  `settlement`, `sponsor`, `evidence`, `receipt`, `trust`, `privacy`,
  `recovery`, `discovery`
- ✅ Compile/import coverage for every package seed plus composed example
  coverage in `stdlib_packages_test.go`, `stdlib_composition_test.go`, and
  `stdlib_examples_test.go`
- ✅ Metadata, human-readable summary, discovery-manifest, and agent-package
  end-to-end coverage for composed examples in `e2e/stdlib_examples_e2e_test.go`
- ✅ Runtime execution coverage for core stdlib packages in
  `stdlib_runtime_test.go`, including authority, execution binding, session,
  receipt, sponsor, evidence, recovery, trust, discovery, agreement,
  account, settlement, and privacy/UNO flows
- ✅ Composed runtime package-call coverage for `PolicySponsoredCheckout` and
  `PrivateServiceOrder` in `stdlib_composed_runtime_test.go`
- ✅ Stateful composed runtime flows with real downstream stdlib contract state
  in `stdlib_composed_runtime_test.go`
- ✅ Stateful composed runtime write flows now mutate downstream stdlib state
  through the coordinators themselves, including receipt finalization and
  agreement/settlement completion in `stdlib_composed_runtime_test.go`
- ✅ Confidential escrow + receipt composed example coverage now exists in
  `examples/stdlib_composed/PrivateEscrowCheckout.tol`,
  `stdlib_examples_test.go`, `e2e/stdlib_examples_e2e_test.go`, and
  `stdlib_composed_runtime_test.go`
- ✅ Sponsored confidential checkout coverage now exists in
  `examples/stdlib_composed/SponsoredPrivateEscrowCheckout.tol`, combining
  execution binding, sponsor relay, receipt finalization, confidential escrow,
  and real downstream call execution in `stdlib_composed_runtime_test.go`
- ✅ Deterministic stdlib release artifacts now exist under `stdlib/releases/`,
  produced by `cmd/stdlib-export` and locked by `stdlib_release_test.go`
- ✅ Family-level stdlib bundle packages now exist for multi-contract families,
  so package identity is not limited to per-contract `.tor` artifacts
- ✅ Family-local bundle catalogs now exist beside those bundle packages, so a
  consumer can resolve a multi-contract stdlib family without first loading the
  global release index
- ✅ Family-level bundle discovery and agent-package metadata now exist beside
  those bundle packages, so discovery clients can consume a multi-contract
  family directly rather than reconstructing one from contract-level records
- ✅ A first-pass stdlib threat model baseline now exists in
  `docs/STDLIB_THREAT_MODEL_MATRIX.md`
- ✅ Low-level external-call runtime coverage now executes real target contracts
  behind `target.call(data)` for sponsor/account paths, not just host stubs, in
  `stdlib_runtime_test.go` and `stdlib_composed_runtime_test.go`
- ✅ Confidential escrow seed and UNO runtime coverage are now implemented in
  `stdlib/privacy/ConfidentialEscrow.tol` and `stdlib_runtime_test.go`
- ✅ Direct `msg.uno_value.*` UNO method-call runtime coverage now has a focused
  regression test in `tol_ir_direct_lowering_uno_test.go`, closing an env-member
  type-inference/lowering gap exposed by stdlib privacy flows

### `stdlib/account`

Status: ✅ implemented seed in `stdlib/account/PolicyAccount.tol`

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

### `stdlib/authority`

Status: ✅ implemented seed in `stdlib/authority/AuthorityBook.tol`

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

### `stdlib/execution_binding`

Status: ✅ implemented seed in `stdlib/execution_binding/ExecutionBindingBook.tol`

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
boundary layer, not to on-chain stdlib.

The on-chain role is smaller:

- bind an approved execution to allowed authority
- bind execution to the relevant policy snapshot
- prevent replay and stale approval reuse
- preserve machine-readable linkage into receipts and proofs

So this package is not an on-chain intent engine.
It is the contract-side execution-binding layer that lets intent-native runtime
systems safely anchor approval and settlement to chain execution.

### `stdlib/session`

Status: ✅ implemented seed in `stdlib/session_book/SessionBook.tol`

Note: the concrete import namespace uses `session_book` instead of `session`
because `session` currently collides with a parser keyword.

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

### `stdlib/agreement`

Status: ✅ implemented seed in `stdlib/agreement/CommercialAgreement.tol`

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

### `stdlib/settlement`

Status: ✅ implemented seed in `stdlib/settlement/TaskSettlement.tol`

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

Current seeds:

- `stdlib/Task.tol`
- `examples/agent_economy/TaskEscrow.tol`
- `examples/agent_economy/MerchantPayment.tol`
- `examples/agent_economy/RecurringPayment.tol`

### `stdlib/sponsor`

Status: ✅ implemented seed in `stdlib/sponsor/SponsorPolicyRelay.tol`

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

### `stdlib/evidence`

Status: ✅ implemented seed in `stdlib/evidence/EvidenceBook.tol`

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

- `stdlib/Oracle.tol`
- `examples/agent_economy/OracleResolver.tol`

### `stdlib/receipt`

Status: ✅ implemented seed in `stdlib/receipt/ReceiptBook.tol`

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

### `stdlib/trust`

Status: ✅ implemented seed in `stdlib/trust/TrustRegistry.tol`

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

### `stdlib/privacy`

Status: ✅ implemented seeds in `stdlib/privacy/ConfidentialVault.tol` and
`stdlib/privacy/ConfidentialEscrow.tol`

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

It should be explicit that `stdlib/privacy` is not only about disclosure after
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

Current seeds:

- `stdlib/privacy/ConfidentialVault.tol`
- `stdlib/privacy/ConfidentialEscrow.tol`

### `stdlib/recovery`

Status: ✅ implemented seed in `stdlib/recovery/RecoveryController.tol`

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

### `stdlib/discovery`

Status: ✅ implemented seed in `stdlib/discovery/ServiceDirectory.tol`

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

A serious TOL stdlib package should not be only a reusable `.tol` snippet.

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

- `stdlib/Task.tol`
- `stdlib/Oracle.tol`
- `stdlib/Vote.tol`
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

The stdlib and release pipeline are no longer the main unfinished layer.

The current highest-value remaining task is:

`Finish the deepest remaining VM/protocol gap for Tolang/GTOS: nested-call rollback and atomic execution semantics across LVM, with cross-stack tests and docs.`

This is not a greenfield task.
The stdlib and release pipeline are already substantially complete.

### Current context

- The guiding design is this document:
  `docs/AGENT_NATIVE_STDLIB_2046.md`
- The threat model is:
  `docs/STDLIB_THREAT_MODEL_MATRIX.md`
- The exposed framework/runtime gaps are tracked in:
  `docs/TOLANG_SHORTCOMINGS.md`
- Stdlib families, release artifacts, discovery metadata, agent package
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
- Existing stdlib, runtime, and release behavior is not broken
- New tests exist and pass

### Important constraints

- Do not add new stdlib families
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
- `/home/tomi/tolang/stdlib_runtime_test.go`
- `/home/tomi/tolang/stdlib_composed_runtime_test.go`
- `/home/tomi/gtos/core/lvm_tol_e2e_test.go`
- `/home/tomi/gtos/core/lvm_tol_stdlib_e2e_test.go`

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

#### Agent 4: Tolang stdlib and composed-flow regressions

Ownership:

- `/home/tomi/tolang/stdlib_runtime_test.go`
- `/home/tomi/tolang/stdlib_composed_runtime_test.go`
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
- If any stdlib release artifact or metadata output changes:
  - `/home/tomi/tolang`: `go run ./cmd/stdlib-export`
  - `/home/tomi/tolang`: `go test -run 'TestStdlibReleaseArtifactsAreCurrent' -v .`
- Final report must state:
  - exact rollback semantics now guaranteed
  - files changed
  - tests added
  - remaining unresolved protocol gaps after this task

### Delegation packet form

If this work is delegated to a parallel coding system such as Claude Code, the
kickoff prompt should be:

`Start the 4 agents now. Prioritize nested-call rollback and atomicity. Do not expand scope into new stdlib families.`

A more aggressive execution posture is:

`Do not stop at analysis. Carry the fix through implementation, regression tests, full verification, and final integration in both repos.`

## Remaining capability backlog from the implementation audit

The stdlib seeds, release artifacts, runtime coverage, and discovery surfaces
are now substantially complete, but the capability audit in
`docs/STDLIB_CAPABILITY_ANALYSIS.md` makes clear that several commercially
important tasks are still not closed.

These are not hypothetical future ideas.
They are the current remaining implementation backlog.

### Missing contracts

These contracts are described by the 2046 design but do not yet exist in
`stdlib/privacy/`:

- `ConfidentialPayment`
  batch and individual encrypted payment flows
- `ConfidentialTreasury`
  multi-owner confidential treasury with selective disclosure
- `ConfidentialAllowance`
  encrypted allowance and approval patterns
- `AuditorDisclosureBook`
  structured auditor disclosure with snapshot-oriented disclosure records

### Missing control-plane capability features

These gaps exist even though the seed contracts compile and have runtime tests:

- `PolicyAccount` still lacks per-role or per-employee spend caps
  `setSpendCaps(...)` is currently owner-scoped rather than delegate- or
  employee-scoped
- `SessionBook` and related flows still lack single-call convenience APIs such
  as `require_terminal(...)`
- terminal and trust modeling is still represented as raw `u256` constants
  rather than a named `6 terminal types x 5 trust tiers` semantic matrix
- step-up logic is still too query-oriented
  the stdlib has `requiresStepUp(...)`, but not a stronger canonical
  enforcement layer that downstream packages can reuse directly

### Missing execution-plane capability features

These are the main remaining commercial-flow gaps:

- recurring and subscription payments
  no canonical `schedule(...)`, periodic debit, or subscription settlement
  mechanism exists yet
- milestone staged release
  `TaskSettlement` and `ConfidentialEscrow` still model single-release payout,
  not multi-milestone settlement
- slash distribution
  dispute resolution exists, but there is no configurable slash or split
  distribution model
- auto-receipt binding
  `ReceiptBook` exists, but settlement does not yet auto-bind receipt creation
  and finalization
- invoice and subscription agreement sub-types
  `CommercialAgreement` models offer/accept/fulfill/cancel/expire, but not a
  distinct invoice or subscription state model

### Missing market-plane capability features

These are the main remaining scale-out and privacy gaps:

- reputation writes and scorer callbacks
  `TrustRegistry` can read eligibility and reputation snapshots, but it still
  lacks canonical `updateReputation(...)` or scorer callback flows
- per-agreement or per-service stake lock semantics
  bond deposit exists, but there is no stronger agreement-bound stake lock
  model
- structured discovery fields
  `ServiceDirectory` still stores service metadata as opaque `bytes32`
  references rather than structured SLA duration, fee amount, capability enum,
  and quote fields
- selective disclosure ZK proof gate
  auditor authorization exists, but there is not yet a proof-gated selective
  disclosure layer
- selective disclosure decryption token layer
  there is still no canonical per-counterparty decryption token issuance or
  disclosure-token flow
- the composed "arbitrator selective disclosure" path is still not truly
  closed
  that scenario requires the missing `AuditorDisclosureBook` plus deeper
  privacy-layer work

### Missing compiler and language features

The capability audit also identifies language-level gaps that should not remain
hand-rolled forever:

- `@requires(caller: Cap)`
  compiler-enforced capability-based access control syntax is still not
  implemented, so access control remains manually written in seed contracts

### Priority order after nested-call rollback

Once the current VM and protocol frontier on nested-call rollback and atomicity
is closed, the next implementation order should be:

1. close the privacy-family contract gap:
   `ConfidentialPayment`, `ConfidentialTreasury`,
   `ConfidentialAllowance`, `AuditorDisclosureBook`
2. close recurring and staged settlement:
   subscription payments, milestone release, slash distribution,
   invoice/subscription agreement forms
3. close receipt automation:
   canonical auto-receipt binding between settlement and `ReceiptBook`
4. close policy and session ergonomics:
   per-role spend caps, named terminal/trust taxonomy, stronger step-up and
   `require_terminal(...)` style APIs
5. close market-scale semantics:
   reputation writes, scorer callbacks, stake locks, and structured discovery
   fields
6. close privacy-proof and compiler semantics:
   ZK selective disclosure gate, decryption token layer, and
   `@requires(caller: Cap)`

### Acceptance bar for capability-complete closure

These backlog items should not be considered closed merely because a seed
contract exists.

For each remaining item, the closure bar should be:

- implementation exists in stdlib or compiler
- compile/import coverage exists
- runtime or composed-flow tests exist where applicable
- release metadata and discovery semantics are updated when applicable
- the threat-model implications are reflected in
  `docs/STDLIB_THREAT_MODEL_MATRIX.md` when materially changed

## A hard rule

TOL stdlib should standardize economic flows, not only safety helpers.

It should answer questions like:

- How does a weak terminal request a bounded merchant payment?
- How does an agent receive scoped authority to operate an account?
- How does a sponsor bind itself to a policy-approved route?
- How does an off-chain approval become a replay-safe execution binding?
- How does a task completion proof become a payout receipt?
- How does a recovery flow revoke old terminal authority?
- How does an auditor receive a redacted but sufficient proof surface?
- How does a discovery client know a contract is safe to compose with?

If the stdlib can answer those questions coherently, TOL becomes a real
agent-native platform.

## Bottom line

The first-principles result is simple:

TOL stdlib is not primarily a security-helper library.

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

That is what TOL stdlib should standardize.
