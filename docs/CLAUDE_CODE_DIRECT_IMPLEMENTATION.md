# Claude Code Direct Implementation Prompt

This prompt is intended for Claude Code to execute the **remaining Tolang stdlib
backlog directly in code**, with multiple agents running in parallel.

It assumes the major foundational work is already complete:

- cross-contract atomicity
- privacy-family completion
- recurring payment support
- `@requires(caller: Cap)`
- GTOS selective disclosure

The remaining work is now a smaller, concrete implementation wave:

1. slash distribution
2. auto-receipt binding
3. named terminal/trust taxonomy
4. stronger reusable step-up enforcement
5. higher-level privacy composition helpers
6. typed discovery / capability schema normalization

---

## Paste This Into Claude Code

```text
Task: Complete the remaining Tolang stdlib backlog end-to-end, with parallel agents, code changes, tests, docs, and exporter updates where needed.

Repos:
- /home/tomi/tolang
- /home/tomi/gtos (read-only unless a very small metadata/export alignment change is required)

Mission:
Finish the remaining stdlib/productization backlog in Tolang. Do not stop at design or analysis. Carry each selected item through implementation, tests, documentation sync, and artifact/export updates where applicable.

Current remaining backlog:
1. slash distribution
2. auto-receipt binding
3. named terminal/trust taxonomy
4. stronger reusable step-up enforcement
5. higher-level privacy composition helpers
6. typed discovery / capability schema normalization

Important context:
- The previous major workstreams are already resolved and should not be reopened.
- The repo already has package seeds, runtime/composed tests, release artifacts, metadata/export, and GTOS integration.
- The goal is not to add random breadth. The goal is to close the highest-value remaining product gaps.

Read first:
- /home/tomi/tolang/docs/AGENT_NATIVE_STDLIB_2046.md
- /home/tomi/tolang/docs/STDLIB_CAPABILITY_ANALYSIS.md
- /home/tomi/tolang/docs/STDLIB_THREAT_MODEL_MATRIX.md
- /home/tomi/tolang/docs/PRIVACY_COMPOSITION_HELPERS.md
- /home/tomi/tolang/docs/DISCOVERY_TYPED_SCHEMA.md
- /home/tomi/tolang/docs/AGENT_ABI_SCHEMA.md
- /home/tomi/tolang/docs/CLAUDE_CODE_REMAINING_TASKS.md
- /home/tomi/tolang/stdlib/settlement/TaskSettlement.tol
- /home/tomi/tolang/stdlib/receipt/ReceiptBook.tol
- /home/tomi/tolang/stdlib/session/SessionBook.tol
- /home/tomi/tolang/stdlib/discovery/ServiceDirectory.tol
- /home/tomi/tolang/stdlib/privacy/
- /home/tomi/tolang/stdlib_runtime_test.go
- /home/tomi/tolang/stdlib_composed_runtime_test.go
- /home/tomi/tolang/e2e/

Execution strategy:
Spawn 5 agents in parallel with disjoint ownership. The main agent integrates, resolves collisions, updates docs, runs tests, and produces the final report.

Agent 1: Settlement automation
Ownership:
- /home/tomi/tolang/stdlib/settlement/TaskSettlement.tol
- /home/tomi/tolang/stdlib/receipt/ReceiptBook.tol only for receipt-binding integration points if needed
- focused tests in /home/tomi/tolang/stdlib_runtime_test.go and /home/tomi/tolang/stdlib_composed_runtime_test.go
Task:
- implement slash distribution in `TaskSettlement`
- define a minimal, defensible split/slash model rather than an overdesigned generic system
- implement canonical auto-receipt binding between settlement transitions and `ReceiptBook`
- preserve current task lifecycle behavior where possible
- add runtime coverage for:
  - dispute resolved with split/slash
  - receipt auto-open/finalize behavior
  - rollback behavior if receipt finalization fails

Agent 2: Control-plane ergonomics
Ownership:
- /home/tomi/tolang/stdlib/session/SessionBook.tol
- related runtime/composed tests only
Task:
- implement named terminal/trust taxonomy on top of the current raw `u256` model
- add reusable step-up enforcement, not only `requiresStepUp(...)` queries
- preserve backward compatibility where practical by keeping current raw interfaces usable
- add runtime coverage for:
  - named terminal classification
  - step-up-required denial
  - enforced step-up success path

Agent 3: Privacy composition helpers
Ownership:
- new helper contracts/examples under /home/tomi/tolang/stdlib/privacy/ or /home/tomi/tolang/examples/stdlib_composed/
- privacy-focused runtime/composed tests
Task:
- implement the first helper wave from `PRIVACY_COMPOSITION_HELPERS.md`
- minimum target:
  - `PrivateDisputeEscrow`
  - `RegulatedPrivateCheckout`
- if scope is too large, complete one helper fully and one thinner example flow, but do not leave both half-finished
- use existing privacy-family contracts and GTOS selective disclosure assumptions; do not reimplement cryptography
- add composed runtime coverage for:
  - happy-path confidential settlement
  - dispute path with disclosure policy binding
  - receipt linkage

Agent 4: Typed discovery schema
Ownership:
- /home/tomi/tolang/stdlib/discovery/ServiceDirectory.tol
- /home/tomi/tolang/metadata/
- /home/tomi/tolang/cmd/stdlib-export/
- release/export tests
Task:
- implement the first wave from `DISCOVERY_TYPED_SCHEMA.md`
- add typed discovery fields to `ServiceDirectory`
- add getters/setters and preserve legacy reference fields
- extend exported metadata so typed discovery information is visible to agent-facing outputs
- align with `AGENT_ABI_SCHEMA.md` rather than inventing another incompatible shape
- add tests for:
  - runtime setter/getter behavior
  - metadata/export output
  - backward compatibility with current reference-only fields

Agent 5: Verification and doc sync
Ownership:
- /home/tomi/tolang/docs/AGENT_NATIVE_STDLIB_2046.md
- /home/tomi/tolang/docs/STDLIB_CAPABILITY_ANALYSIS.md
- /home/tomi/tolang/docs/STDLIB_THREAT_MODEL_MATRIX.md if semantics materially change
- non-overlapping tests where needed
Task:
- keep docs aligned with the actual implemented result
- update backlog status only for work that is truly complete in code and covered by tests
- review threat-model implications of slash distribution, receipt automation, step-up enforcement, privacy helpers, and typed discovery
- do not mark anything resolved unless the implementation and tests justify it

Coordination rules:
- Each agent owns a disjoint write scope.
- The main agent integrates and may adjust code after the agents return.
- Do not duplicate work across agents.
- Do not reopen already-resolved foundational work unless blocked by a concrete bug.
- Do not introduce new stdlib package families unless absolutely necessary.
- Prefer additive evolution over breaking ABI unless a break is clearly justified and documented.

Implementation constraints:
- Keep changes pragmatic and bounded.
- Favor real runtime/composed coverage over compile-only coverage.
- If release metadata changes, regenerate artifacts instead of hand-editing generated files.
- Preserve importability and package/export consistency.

Required final deliverables:
- code changes for the backlog items completed
- new runtime/composed tests
- updated exporter/metadata output where applicable
- doc synchronization in the main design/status docs
- a concise final report listing:
  - what was completed
  - what was intentionally left open
  - files changed
  - tests added
  - exact commands run

Acceptance:
- cd /home/tomi/tolang && go test ./...
- if metadata/export/artifacts changed:
  - cd /home/tomi/tolang && go run ./cmd/stdlib-export
  - cd /home/tomi/tolang && go test -run 'TestStdlibReleaseArtifactsAreCurrent' -v .
- final docs must remain internally consistent

Execution posture:
Do not stop at analysis. Spawn the 5 agents now and carry the work through implementation, tests, integration, and final verification.
```

---

## Notes

- This prompt is intentionally implementation-first.
- It is suitable only because the remaining backlog is now narrow and already
  has design homes.
- If scope needs to be reduced, the best cut line is:
  1. settlement automation
  2. control-plane ergonomics
  3. typed discovery
  4. privacy composition helpers

