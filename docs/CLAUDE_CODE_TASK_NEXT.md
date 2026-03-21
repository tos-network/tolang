# Claude Code Next Task

## Task

`Finish the deepest remaining VM/protocol gap for Tolang/GTOS: nested-call rollback and atomic execution semantics across LVM, with cross-stack tests and docs.`

## Master Prompt

```text
You are working in two local repos:

- /home/tomi/tolang
- /home/tomi/gtos

This is not a greenfield task. The stdlib and release pipeline are already substantially complete.

Current context:
- The guiding design is /home/tomi/tolang/docs/AGENT_NATIVE_STDLIB_2046.md
- The threat model is /home/tomi/tolang/docs/STDLIB_THREAT_MODEL_MATRIX.md
- The exposed framework/runtime gaps are tracked in /home/tomi/tolang/docs/TOLANG_SHORTCOMINGS.md
- Stdlib families, release artifacts, discovery metadata, agent package metadata, GTOS package target validation, explicit gas caps, typed custom reverts, and deployed TOL metadata RPC are already implemented
- The highest-value unresolved gap is nested call rollback / atomicity across account/sponsor/settlement/package-call flows

Your mission:
Implement and harden nested-call rollback semantics and atomic execution behavior in GTOS/LVM, then prove it with cross-stack tests in both repos.

What “done” means:
- Failed child calls do not leave half-committed upstream state
- Value transfer, storage mutation, receipt state, sponsor/account budget state, and settlement state obey clear rollback semantics
- Revert data still propagates correctly
- Raw Lua compatibility is preserved
- TOL’s 32-byte agent normalization boundary is preserved
- Existing stdlib/runtime/release behavior is not broken
- New tests exist and pass

Important constraints:
- Do not add new stdlib families
- Do not broaden scope into unrelated protocol systems
- Do not revert unrelated changes
- Keep changes minimal and defensible
- Prefer primary fixes in GTOS/LVM and only add TOL changes when needed for tests or clear contract-boundary correctness
- Preserve existing behavior for explicit gas caps, typed custom errors, package target validation, and deployed metadata RPC

Start by reading:
- /home/tomi/tolang/docs/AGENT_NATIVE_STDLIB_2046.md
- /home/tomi/tolang/docs/STDLIB_THREAT_MODEL_MATRIX.md
- /home/tomi/tolang/docs/TOLANG_SHORTCOMINGS.md
- /home/tomi/gtos/core/vm/lvm.go
- /home/tomi/tolang/stdlib_runtime_test.go
- /home/tomi/tolang/stdlib_composed_runtime_test.go
- /home/tomi/gtos/core/lvm_tol_e2e_test.go
- /home/tomi/gtos/core/lvm_tol_stdlib_e2e_test.go

Spawn 4 agents in parallel with disjoint ownership:

Agent 1: Rollback semantics analysis
Ownership:
- Read-only analysis across /home/tomi/gtos/core/vm/lvm.go and relevant tests
Task:
- Map current snapshot/revert behavior for call/staticcall/delegatecall/package_call/create paths
- Identify exactly where atomicity is incomplete or ambiguous
- Produce a short actionable implementation note for the main agent
- Do not edit files unless explicitly necessary

Agent 2: GTOS VM implementation
Ownership:
- /home/tomi/gtos/core/vm/lvm.go
- small helper additions in the same VM area if needed
Task:
- Implement the rollback/atomicity fix
- Ensure nested child failure restores the correct state boundary
- Preserve revert-data propagation
- Preserve raw Lua behavior
- Do not touch tosapi or unrelated subsystems unless required

Agent 3: GTOS regression/e2e tests
Ownership:
- /home/tomi/gtos/core/*test.go
- /home/tomi/gtos/internal/tosapi/*test.go only if required
Task:
- Add focused regression tests covering:
  - nested call storage rollback
  - nested call value rollback
  - package_call rollback
  - sponsor/account path rollback
  - structured custom revert propagation through failed nested execution
- Prefer minimal, direct tests that fail before the fix and pass after

Agent 4: Tolang stdlib/composed-flow regressions
Ownership:
- /home/tomi/tolang/stdlib_runtime_test.go
- /home/tomi/tolang/stdlib_composed_runtime_test.go
- /home/tomi/tolang/e2e/* only if needed
- /home/tomi/tolang/docs/TOLANG_SHORTCOMINGS.md and /home/tomi/tolang/docs/STDLIB_THREAT_MODEL_MATRIX.md if semantics change materially
Task:
- Add cross-stack regression tests proving that failed composed flows do not leave half-committed state in:
  - PolicyAccount
  - SponsorPolicyRelay
  - TaskSettlement
  - ReceiptBook
  - ConfidentialEscrow where relevant
- Update docs only if the implemented rollback semantics materially clarify a previously open shortcoming

Coordination rules:
- The main agent integrates results
- Agents must not overwrite each other’s files
- Trust the analysis agent’s codepath mapping unless there is hard evidence it is wrong
- Do not duplicate work
- Keep the critical path moving

Acceptance criteria:
- /home/tomi/gtos: go test ./core/...
- /home/tomi/gtos: go test ./internal/tosapi
- /home/tomi/gtos: go test -cover ./core/... ./internal/tosapi
- /home/tomi/tolang: go test ./...
- If any stdlib release artifact or metadata output changes:
  - /home/tomi/tolang: go run ./cmd/stdlib-export
  - /home/tomi/tolang: go test -run 'TestStdlibReleaseArtifactsAreCurrent' -v .
- Final report must state:
  - exact rollback semantics now guaranteed
  - files changed
  - tests added
  - remaining unresolved protocol gaps after this task
```

## One-Line Kickoff

```text
Start the 4 agents now. Prioritize nested-call rollback and atomicity. Do not expand scope into new stdlib families.
```

## More Aggressive Variant

```text
Do not stop at analysis. Carry the fix through implementation, regression tests, full verification, and final integration in both repos.
```
