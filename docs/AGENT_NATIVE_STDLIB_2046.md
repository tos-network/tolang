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

### `stdlib/account`

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

### `stdlib/recovery`

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

1. `account`
2. `authority`
3. `execution_binding`
4. `session`
5. `recovery`

Without this wave, TOL cannot safely support multi-terminal, delegated,
sponsored consumer finance.

### Execution-plane wave

These packages define how commercial actions actually commit, settle, and get
recorded:

1. `agreement`
2. `settlement`
3. `sponsor`
4. `evidence`
5. `receipt`

Without this wave, there is no canonical machine-commerce layer.

### Market-plane wave

These packages define how agents choose counterparties, protect private state,
and reason about trust:

1. `trust`
2. `privacy`
3. `discovery`

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
