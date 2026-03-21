# Claude Code Remaining Tasks

This document packages the remaining Tolang / GTOS work into five separate
Claude Code prompts.

These should **not** be treated as one giant task. They are intentionally
split by scope and risk:

1. doc sync cleanup
2. control-plane completion
3. execution-plane completion
4. market-plane completion
5. protocol-closure design pack

Recommended order:

1. Prompt 1
2. Prompt 2
3. Prompt 3
4. Prompt 4
5. Prompt 5

---

## Prompt 1: Doc Sync Cleanup

```text
Task: Clean up stale status text across Tolang docs so they match the current repo state.

Repos:
- /home/tomi/tolang
- /home/tomi/gtos (read-only unless a doc there must be referenced)

Context:
The 5 major follow-on gaps are now resolved:
- cross-contract atomicity
- privacy family completion
- recurring/subscription payments
- @requires(caller: Cap)
- selective disclosure

But some Tolang docs still contain stale text that says parts of those are unfinished.

Read first:
- /home/tomi/tolang/docs/AGENT_NATIVE_STDLIB_2046.md
- /home/tomi/tolang/docs/STDLIB_CAPABILITY_ANALYSIS.md
- /home/tomi/tolang/docs/TOLANG_SHORTCOMINGS.md
- /home/tomi/tolang/docs/CLAUDE_CODE_TASK_NEXT.md
- /home/tomi/gtos/docs/SELECTIVE-DISCLOSURE.md
- /home/tomi/gtos/docs/Atomic-Execution-v1.md
- /home/tomi/gtos/docs/Native-Scheduled-Tasks.md

What to do:
- Find stale statements that still mark resolved work as missing or partial.
- Update wording so the docs are internally consistent.
- Do not inflate remaining work into “resolved” if code/docs do not support it.
- Keep the remaining backlog focused on what is truly still open.

Expected outputs:
- Cleaned docs
- Brief summary of contradictions fixed

Acceptance:
- No code changes outside docs unless strictly required
- Final diff should show that resolved work is marked resolved consistently
```

---

## Prompt 2: Control Plane Completion

```text
Task: Complete the remaining control-plane stdlib gaps in Tolang.

Repo:
- /home/tomi/tolang

Current resolved work:
- stdlib package map exists
- runtime tests exist
- @requires(caller: Cap) is resolved
- major privacy/atomicity gaps are resolved

Remaining control-plane targets:
- per-role / per-delegate spend caps in PolicyAccount
- require_terminal(...) style convenience enforcement
- named terminal/trust taxonomy instead of raw u256-only usage
- stronger reusable step-up enforcement

Read first:
- /home/tomi/tolang/docs/AGENT_NATIVE_STDLIB_2046.md
- /home/tomi/tolang/docs/STDLIB_CAPABILITY_ANALYSIS.md
- /home/tomi/tolang/docs/STDLIB_THREAT_MODEL_MATRIX.md
- /home/tomi/tolang/stdlib/account/PolicyAccount.tol
- /home/tomi/tolang/stdlib/session/SessionBook.tol
- /home/tomi/tolang/stdlib_runtime_test.go
- /home/tomi/tolang/stdlib_composed_runtime_test.go

Instructions:
- Spawn 3 agents in parallel.
- Agent 1 owns PolicyAccount changes.
- Agent 2 owns SessionBook changes.
- Agent 3 owns tests and doc sync for this workstream.
- Do not add new package families.
- Preserve existing public behavior unless the new behavior is clearly additive.
- Add runtime coverage, not only compile coverage.

Deliverables:
- Code changes in stdlib/account and stdlib/session
- New or extended runtime/composed tests
- Doc status updates in AGENT_NATIVE_STDLIB_2046.md and STDLIB_CAPABILITY_ANALYSIS.md

Acceptance:
- cd /home/tomi/tolang && go test ./...
- if release artifacts change:
  - cd /home/tomi/tolang && go run ./cmd/stdlib-export
  - cd /home/tomi/tolang && go test -run 'TestStdlibReleaseArtifactsAreCurrent' -v .
```

---

## Prompt 3: Execution Plane Completion

```text
Task: Complete the remaining execution-plane commercial settlement gaps in Tolang.

Repo:
- /home/tomi/tolang

Open targets:
- milestone staged release
- slash distribution
- auto-receipt binding
- explicit invoice / subscription agreement subtypes
- remove stale docs that still say recurring payments are unresolved

Read first:
- /home/tomi/tolang/docs/AGENT_NATIVE_STDLIB_2046.md
- /home/tomi/tolang/docs/STDLIB_CAPABILITY_ANALYSIS.md
- /home/tomi/tolang/stdlib/settlement/TaskSettlement.tol
- /home/tomi/tolang/stdlib/settlement/RecurringPayment.tol
- /home/tomi/tolang/stdlib/agreement/CommercialAgreement.tol
- /home/tomi/tolang/stdlib/receipt/ReceiptBook.tol
- /home/tomi/tolang/examples/stdlib_composed/
- /home/tomi/tolang/stdlib_runtime_test.go
- /home/tomi/tolang/stdlib_composed_runtime_test.go

Instructions:
- Spawn 4 agents.
- Agent 1 owns TaskSettlement.
- Agent 2 owns RecurringPayment + CommercialAgreement.
- Agent 3 owns ReceiptBook + receipt auto-binding integration.
- Agent 4 owns tests and composed examples.
- Keep file ownership disjoint.
- Prefer additive design over breaking ABI unless clearly justified.
- Runtime coverage is mandatory.

Deliverables:
- Updated settlement/agreement/receipt contracts
- New composed examples if needed
- Runtime and composed-flow tests
- Doc status updates

Acceptance:
- cd /home/tomi/tolang && go test ./...
- if artifacts change:
  - cd /home/tomi/tolang && go run ./cmd/stdlib-export
  - cd /home/tomi/tolang && go test -run 'TestStdlibReleaseArtifactsAreCurrent' -v .
```

---

## Prompt 4: Market Plane Completion

```text
Task: Complete the remaining market-plane stdlib gaps in Tolang.

Repo:
- /home/tomi/tolang

Open targets:
- higher-level composed privacy ergonomics on top of already-resolved GTOS selective disclosure
- typed discovery / capability schema normalization on top of the current `ServiceDirectory`

Read first:
- /home/tomi/tolang/docs/AGENT_NATIVE_STDLIB_2046.md
- /home/tomi/tolang/docs/STDLIB_CAPABILITY_ANALYSIS.md
- /home/tomi/tolang/docs/PRIVACY_STDLIB_FAMILY.md
- /home/tomi/tolang/docs/PRIVACY_COMPOSITION_HELPERS.md
- /home/tomi/tolang/docs/DISCOVERY_TYPED_SCHEMA.md
- /home/tomi/tolang/stdlib/trust/TrustRegistry.tol
- /home/tomi/tolang/stdlib/discovery/ServiceDirectory.tol
- /home/tomi/tolang/stdlib/privacy/
- /home/tomi/tolang/examples/stdlib_composed/
- /home/tomi/tolang/stdlib_runtime_test.go
- /home/tomi/tolang/stdlib_composed_runtime_test.go
- /home/tomi/gtos/docs/SELECTIVE-DISCLOSURE.md

Instructions:
- Spawn 4 agents.
- Agent 1 owns TrustRegistry.
- Agent 2 owns ServiceDirectory.
- Agent 3 owns privacy composed helper contracts/examples only.
- Agent 4 owns tests and doc sync.
- Do not re-implement GTOS selective disclosure cryptography in Tolang.
- Build stdlib convenience flows on top of what GTOS already provides.
- Follow `PRIVACY_COMPOSITION_HELPERS.md` for helper contracts/examples.
- Follow `DISCOVERY_TYPED_SCHEMA.md` for typed discovery fields, enums, and export shape.

Deliverables:
- trust/discovery/privacy code updates
- runtime/composed tests
- any needed example contracts
- doc updates

Acceptance:
- cd /home/tomi/tolang && go test ./...
- if artifacts change:
  - cd /home/tomi/tolang && go run ./cmd/stdlib-export
  - cd /home/tomi/tolang && go test -run 'TestStdlibReleaseArtifactsAreCurrent' -v .
```

---

## Prompt 5: Protocol Closure Design Pack

```text
Task: Produce the next protocol-closure design pack for Tolang/GTOS. This is design-first work, not breadth-first coding.

Repos:
- /home/tomi/tolang
- /home/tomi/gtos

This task is about the remaining structural gaps after the 5 major workstreams were resolved.

Design targets:
1. protocol backing for @delegated / @verifiable / @pay / capability semantics
2. stable package identity and publishing model independent of filesystem layout
3. unified ABI / discovery / capability schema for agent runtimes
4. migration path from host-shaped economic primitives to more VM-native/runtime-native interfaces

Read first:
- /home/tomi/tolang/docs/TOLANG_SHORTCOMINGS.md
- /home/tomi/tolang/docs/AGENT_NATIVE_STDLIB_2046.md
- /home/tomi/tolang/docs/STDLIB_CAPABILITY_ANALYSIS.md
- /home/tomi/tolang/docs/DISCOVERY_TYPED_SCHEMA.md
- /home/tomi/tolang/docs/CALLER_CAPABILITY_SYNTAX.md
- /home/tomi/gtos/docs/Atomic-Execution-v1.md
- /home/tomi/gtos/docs/SELECTIVE-DISCLOSURE.md

Instructions:
- Spawn 4 read/write agents with disjoint doc ownership.
- Agent 1: annotation protocol backing
- Agent 2: package identity and publishing
- Agent 3: ABI/discovery/capability unification
- Agent 4: VM-native economic primitive roadmap
- Main agent integrates into a coherent design pack.
- Do not do speculative coding unless a small code spike is necessary to validate a design claim.
- Treat `DISCOVERY_TYPED_SCHEMA.md` as the immediate concrete design home for the
  discovery/capability normalization subtask, and align it with
  `AGENT_ABI_SCHEMA.md`.

Expected outputs:
- 3 to 4 new design docs or major doc expansions
- explicit problem statement, invariants, threat model, migration strategy, acceptance criteria
- recommended implementation order across tolang and gtos

Acceptance:
- doc set is internally consistent
- each design has a clear scope boundary and next implementation step
- no unrelated code churn
```

---

## Controller Prompt

Use this when delegating the remaining work as a coordinated Claude Code run.

```text
Do not treat this as one giant task. Execute Prompts 1-5 as separate workstreams. For Prompts 2-4, carry work through implementation + tests. For Prompt 5, stay design-first.
```
