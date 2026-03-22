# Tolang Shortcomings Exposed By Building The Agent-Native Stdlib

## Executive Summary

Building the stdlib clarified an important point:

Tolang is already stronger on language shape, compiler structure, and artifact
metadata than it is on runtime-native economic semantics.

The main gap is not "missing Solidity-like helpers".
The main gap is that several agent-native ideas are expressible in source and
artifacts, but are not yet fully closed across:

- compiler lowering
- package/import resolution
- LVM host functions
- protocol-backed runtime semantics

In short:

Tolang can already express a large part of the agent-native model, but the
runtime and protocol layers are not yet strong enough to make every one of
those semantics native, uniform, and reliable.

## Problems The Stdlib Work Exposed And Already Forced Us To Fix

### 1. Init/runtime compilation was not package-consistent

When stdlib packages started importing each other in realistic ways, we found
that runtime compilation and init compilation did not behave consistently.

`CompileInitBytecode(...)` needed to resolve imports the same way runtime
artifact compilation did.

Evidence:

- `tol_api.go`

Impact:

- package-based contracts could compile as runtime artifacts but fail on init
  artifact generation
- this directly breaks constructor-time package use

What this revealed:

- the init/runtime split existed, but package-awareness had not been carried
  through uniformly

### 2. Imported contract/interface values were not treated as first-class enough

The stdlib uses imported contract types pervasively:

- `PolicyAccount(account_addr)`
- `AuthorityBook(authority_addr)`
- `ReceiptBook(receipt_addr)`

That exposed a lowering gap: imported contract/interface casts needed to behave
as identity casts, but were not fully handled that way before.

Evidence:

- `tol_ir_direct_lowering.go`

Impact:

- package-composed contracts could be written in the intended style, but the
  compiler did not fully understand the type/value boundary for imported
  contract handles

What this revealed:

- Tolang was still weaker than it should be at "contract-as-typed-address"
  semantics, which is central for packageized stdlib design

### 3. Basic stdlib-heavy surface syntax was still incomplete in lowering

Real stdlib code quickly touched constructs such as:

- `bytes32(0)`
- `addr.call(data)`
- `addr.staticcall(data)`
- `addr.delegatecall(data)`

These are not exotic features for an agent-native standard library.
They are routine building blocks.

Evidence:

- `tol_ir_direct_lowering.go`

Impact:

- syntax existed at the language surface, but not all of it was lowered cleanly
  enough for real package implementations

What this revealed:

- Tolang had already grown a serious surface area, but parts of the lowering
  layer were still behind the language it appeared to offer

## Structural Shortcomings That Still Exist

### 1. Package calls are compiler-real but runtime-dependent — RESOLVED

**Resolved (2026-03-22):**

`tos.package_call` in GTOS LVM now performs registry-backed package identity
validation.  After the lease check, it computes `keccak256(deployedCode)` and
queries `pkgregistry.ReadPackageByHash`.  If a registry record exists:

- Revoked package → returns `(false, "PACKAGE_REVOKED")`
- Revoked publisher → returns `(false, "PUBLISHER_REVOKED")`
- Active/deprecated → call proceeds normally

Unregistered packages still work (backward-compatible permissive fallback with
TODO to tighten).  4 focused tests in `lvm_pkgreg_test.go`.

### 2. External call semantics are still too thin — LARGELY RESOLVED

The stdlib work pushed heavily on:

- `target.call(data)`
- sponsor relays
- account-mediated execution
- coordinator-style multi-contract orchestration

That exposed a deeper issue: the current host call model is still too thin for
agent-native economic flows.

Evidence:

- in the runtime harness, `tos.call` is basically modeled as `(ok, ret)`
- `tos.package_call` is likewise a host hook

Previously missing or not clearly hardened:

- nested call atomicity
- rollback semantics on failure
- revert-data propagation as a first-class contract surface
- gas/accounting propagation across nested host calls

**Resolved (2026-03-21):**

Two layers of rollback are now in place:

1. **GTOS LVM (on-chain):** StateDB snapshot/revert was already correct for all
   call paths (call, staticcall, delegatecall, package_call, create).  Three
   hardening fixes were added: `LVM.Call` now reverts on insufficient balance
   (previously leaked `CreateAccount`), `tos.staticcall` reverts on success as
   defense-in-depth, and `deployRawContract` snapshots before all state writes.
   Four focused regression tests in `lvm_rollback_test.go` cover nested storage
   rollback, value rollback, sponsor relay rollback, and structured revert
   propagation.

2. **Tolang test harness (off-chain):** `snapshotLuaStorage` /
   `revertLuaStorage` helpers now deep-copy `__tol_storage` and
   `__tol_transient_storage` before every top-level or cross-contract call, and
   revert on error.  Applied in `invokeStdlib`, `invokeStdlibErr`,
   `invokeCallContractCalldata`, and `invokePackageContractCalldata`.  Six
   regression tests in `stdlib_composed_runtime_test.go` prove per-contract
   atomicity for PolicyAccount, SponsorPolicyRelay, TaskSettlement, ReceiptBook,
   ConfidentialEscrow, and a composed cross-contract scenario.

**Cross-contract atomicity — RESOLVED (2026-03-21):**

`tos.multicall` is now implemented in GTOS LVM (`core/vm/lvm.go`).
It takes a single outer `stateDB.Snapshot()`, executes N child calls
sequentially, and reverts ALL on any failure.  This provides all-or-nothing
cross-contract atomicity for coordinator flows like "finalize receipt +
release escrow".

7 regression tests in `lvm_rollback_test.go` and 1 composed test in
`stdlib_composed_runtime_test.go` prove the semantics.  Full design in
`/home/tomi/gtos/docs/Atomic-Execution-v1.md`.

### 3. Agent-native annotations are ahead of protocol backing — RESOLVED

The maturity matrix is already explicit about this.

Evidence:

- `docs/FEATURE_MATURITY_MATRIX.md`

Features such as:

All agent-native annotations now have protocol-level backing:

- `capability` + `@requires(...)` — **RESOLVED (2026-03-21)**: compiler
  pipeline + `tos.hascapability` registry-backed lookup
- `@delegated` — **RESOLVED (2026-03-22)**: compiler emits
  `buildDelegatedPreamble` calling `tos.hasdelegation`; GTOS `registry/`
  DelegationRecord with grant/revoke/expiry; backward-compatible (no hook =
  unrestricted)
- `@verifiable` — **RESOLVED**: off-chain verification by design; GTOS
  `verifyregistry/` provides `tos.isverified` for protocol-backed attestation
- `@pay` — **RESOLVED (2026-03-22)**: compiler emits `buildPayPreamble`
  calling `__tol_host_transfer`; GTOS `tos.host_transfer` implemented;
  `paypolicy/` provides `tos.canpay` for policy-backed validation

### 4. Package resolution still depends too much on filesystem layout — RESOLVED

**Resolved (2026-03-22):**

`OSFileResolver` now accepts `PackageSearchPaths []string` — explicit
directories where package-style imports resolve before the walk-up heuristic.
`CompileOptions` and `ArtifactOptions` both expose `PackageSearchPaths`.

Example: `PackageSearchPaths: ["/repo/stdlib"]` makes `import
"tolang.stdlib.account"` resolve by checking `/repo/stdlib/account/` first,
without relying on the importing file's position in the directory tree.

Test: `TestPackageImportResolvesFromSearchPaths` verifies compilation with
a real file path + search path (no synthetic compile name hack).

### 5. Namespace hygiene — RESOLVED (lexer unreserve + directory rename)

Previously `stdlib/session` had to become `stdlib/session_book` because `session`
was a parser reserved word. Fixed: `session` was removed from the lexer's reserved
word list and the package directory was renamed back from `stdlib/session_book` to
`stdlib/session` (package `tolang.stdlib.session`).

Why this matters:

- standard library naming should reflect the conceptual model, not parser
  keyword collisions
- when reserved words leak into package naming, it signals that the language
  namespace model is still too brittle

### 6. The ABI/discovery layer is still not unified enough for agents — RESOLVED

The stdlib effort made clear that agents do not consume source code first.
They consume:

- ABI
- metadata
- discovery manifests
- capability summaries
- receipt/proof anchors

**Resolved (2026-03-22):**

`AgentContractProfile` (schema version 0.2.0) unifies ContractMetadata,
DiscoveryManifest, and AgentPackageInfo into a single `.profile.json` per
contract. `BuildAgentProfile(cm)` assembles identity, contract info,
capabilities, functions, events, errors, service kinds, human summary, gas
model, and typed discovery into one document.

The stdlib exporter now emits `.profile.json` alongside existing artifacts.
All release index entries include the profile path. 3 tests in
`metadata/agent_profile_test.go`.

### 7. Core economic primitives are still too host-shaped — IN PROGRESS

To execute the stdlib meaningfully in tests, we had to supply a large host
surface manually:

- `agentload`
- `escrow`
- `release`
- UNO ciphertext helpers
- `uno.balance(...)`
- `uno.transfer(...)`
- package-call hooks
- call routing hooks

Evidence:

- `stdlib_runtime_test.go`
- `stdlib_composed_runtime_test.go`

Why this matters:

- this is a sign that many of the truly agent-economic primitives still live as
  host conventions, not as deeply standardized VM/runtime capabilities

Diagnosis:

- the language already points toward an agent economy, but the VM/runtime still
  exposes too much of that economy through ad hoc host plumbing

## Overall Status (2026-03-22)

Of the 7 structural shortcomings identified by the stdlib effort:

| # | Shortcoming | Status |
|---|------------|--------|
| 1 | Package calls host-dependent | **RESOLVED** — registry-backed identity validation |
| 2 | External call semantics too thin | **RESOLVED** — rollback + `tos.multicall` |
| 3 | Annotations ahead of protocol backing | **RESOLVED** — all 4 annotations registry-backed |
| 4 | Package resolution filesystem-dependent | **RESOLVED** — `PackageSearchPaths` |
| 5 | Namespace hygiene | **RESOLVED** — `session` unreserved + renamed |
| 6 | ABI/discovery not unified for agents | **RESOLVED** — unified `AgentContractProfile` (.profile.json) |
| 7 | Economic primitives too host-shaped | **IN PROGRESS** — design doc ready |

**6 of 7 resolved.  1 remaining** (#7) is incremental refinement work,
not a structural blocker.

## Priority Order For Fixing These Gaps

If these shortcomings are addressed in the wrong order, Tolang risks becoming a
language that looks advanced in source form but remains fragile in economic
execution.

The priority order should be:

1. ~~Runtime/LVM transaction semantics for nested calls, package calls, and
   atomic multi-contract execution.~~ — **RESOLVED (2026-03-21)**: per-contract
   atomicity via StateDB snapshot/revert; cross-contract atomicity via
   `tos.multicall`; 10 regression tests across both repos.
2. ~~Native, dependable runtime support for package calls and contract capability
   routing.~~ — **RESOLVED (2026-03-22)**: `tos.package_call` now validates
   deployed package identity via `pkgregistry.ReadPackageByHash`; revoked
   packages/publishers are blocked; 4 tests in `lvm_pkgreg_test.go`.
3. ~~Protocol-backed registries and enforcement for delegation, verification,
   settlement, and agent identity semantics.~~ — **RESOLVED (2026-03-22)**:
   GTOS `registry/` (Capability + Delegation), `verifyregistry/` (Verification),
   `paypolicy/` (Settlement/Pay) all implemented with real state-backed CRUD.
   LVM host functions `tos.hascapability`, `tos.hasdelegation`, `tos.isverified`,
   `tos.canpay` are registry-backed. `tos.host_transfer` wired for @pay.
   Compiler emits preambles for `@requires`, `@delegated`, `@pay`.
4. ~~Stable package identity and import resolution independent of local source
   layout.~~ — **RESOLVED (2026-03-22)**: `OSFileResolver.PackageSearchPaths`
   enables filesystem-independent resolution; `CompileOptions` and
   `ArtifactOptions` expose the field; GTOS `pkgregistry/` provides protocol-
   grade package identity model.
5. ~~Unified ABI/discovery/capability schema designed explicitly for agents.~~ —
   **RESOLVED (2026-03-22)**: `AgentContractProfile` (v0.2.0) unifies
   ContractMetadata + DiscoveryManifest + AgentPackageInfo into one
   `.profile.json`; exporter emits it; 3 tests.
6. ~~Namespace cleanup so standard library naming is conceptually clean and not
   parser-accidental.~~ — **RESOLVED**: `session` removed from lexer reserved
   words; `stdlib/session_book` directory renamed to `stdlib/session`.

## Design homes for the five strategic follow-on gaps

This document is the right home for the **structural** part of the five major
follow-on gaps, but not for every detail of their implementation.

The split should be:

| Gap | Why it touches this document | Detailed design home |
| --- | --- | --- |
| Cross-contract atomicity | **RESOLVED** — `tos.multicall` implemented | `/home/tomi/gtos/docs/Atomic-Execution-v1.md` |
| Privacy family completion | **RESOLVED** — all 6 contracts; ZK/token layers pending | `docs/PRIVACY_STDLIB_FAMILY.md` |
| Recurring / subscription settlement | **RESOLVED** — `RecurringPayment` contract; protocol scheduler pending | `/home/tomi/gtos/docs/Native-Scheduled-Tasks.md` |
| `@requires(caller: Cap)` | **RESOLVED** — compiler pipeline implemented + tested | `docs/CALLER_CAPABILITY_SYNTAX.md` |
| Selective disclosure (`ZK + token`) | **RESOLVED** — all 3 layers implemented in GTOS (DisclosureProof, DecryptionToken, AuditorKey) | `/home/tomi/gtos/docs/SELECTIVE-DISCLOSURE.md` plus `docs/PRIVACY_STDLIB_FAMILY.md` |

Practical rule:

- if the question is "what capability is still missing from stdlib?",
  track it in `docs/STDLIB_CAPABILITY_ANALYSIS.md`
- if the question is "what structural weakness in Tolang / LVM causes this?",
  track it here
- if the question is "what exact mechanism should we build?",
  use the dedicated design document

## Bottom Line

The stdlib effort was valuable not only because it produced packages.

It also acted as a stress test for Tolang itself.

That stress test showed:

- Tolang already has a serious compiler and metadata foundation
- packageized agent-native contracts are now practical to write
- but the runtime, host, and protocol layers still need substantial work before
  Tolang can honestly claim that agent-native economic semantics are native
  end-to-end

That is the real lesson from building the stdlib.
