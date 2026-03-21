# Claude Code GTOS Protocol Implementation Prompt

This prompt is intended for Claude Code to execute the **next GTOS protocol
implementation wave** in parallel, based on the three GTOS design documents:

- `/home/tomi/gtos/docs/GTOS_PROTOCOL_REGISTRIES.md`
- `/home/tomi/gtos/docs/LVM_NATIVE_ECONOMIC_PRIMITIVES.md`
- `/home/tomi/gtos/docs/PACKAGE_PUBLISHING_REGISTRY.md`

It assumes the current stdlib closure wave is already complete:

- cross-contract atomicity
- privacy-family completion
- recurring payment support
- `@requires(caller: Cap)`
- GTOS selective disclosure
- stdlib settlement / receipt / typed discovery hardening

The goal is no longer “finish stdlib packages”.
The goal is to make the underlying GTOS runtime and protocol layers catch up.

---

## Paste This Into Claude Code

```text
Task: Implement the next GTOS protocol wave in parallel, based on the existing design docs for registries, VM-native economic primitives, and package publishing identity.

Repos:
- /home/tomi/gtos
- /home/tomi/tolang

Mission:
Carry the GTOS-side protocol closure work from design into code, tests, RPC/metadata integration, and documentation sync where justified.

This is not a greenfield task.
The stdlib/productization wave is already done.
Do not reopen resolved Tolang stdlib tasks unless blocked by a concrete GTOS integration issue.

Primary design homes:
- /home/tomi/gtos/docs/GTOS_PROTOCOL_REGISTRIES.md
- /home/tomi/gtos/docs/LVM_NATIVE_ECONOMIC_PRIMITIVES.md
- /home/tomi/gtos/docs/PACKAGE_PUBLISHING_REGISTRY.md

Read first:
- /home/tomi/tolang/docs/AGENT_NATIVE_STDLIB_2046.md
- /home/tomi/tolang/docs/TOLANG_SHORTCOMINGS.md
- /home/tomi/tolang/docs/AGENT_ABI_SCHEMA.md
- /home/tomi/tolang/docs/DISCOVERY_TYPED_SCHEMA.md
- /home/tomi/gtos/docs/Atomic-Execution-v1.md
- /home/tomi/gtos/docs/Agent-Discovery-v1.md
- /home/tomi/gtos/docs/Agent-Gateway-v1.md
- /home/tomi/gtos/docs/SELECTIVE-DISCLOSURE.md
- /home/tomi/gtos/core/vm/lvm.go
- /home/tomi/gtos/internal/tosapi/

Important constraints:
- Do not treat the three design docs as independent forever; integrate them where it reduces duplication.
- Do not reopen settled stdlib business-logic work unless the GTOS-side protocol change truly requires it.
- Preserve backward compatibility where practical.
- Avoid broad speculative refactors.
- If a design item is too large for one pass, complete a defensible v1 slice and document the boundary clearly.
- The current worktree may already be dirty. Do not revert unrelated changes.

Execution strategy:
Spawn 5 agents in parallel with disjoint ownership. The main agent integrates, resolves conflicts, runs tests, updates docs, and produces the final report.

Agent 1: Protocol registries core
Ownership:
- /home/tomi/gtos new or updated registry packages
- /home/tomi/gtos sysaction integration for registry writes
- registry-specific tests
Task:
- implement the first protocol registry slice from GTOS_PROTOCOL_REGISTRIES.md
- minimum required v1:
  - Capability Registry
  - Delegation Registry
- define state layout, record shapes, lifecycle states, and write paths
- include revocation/deprecation support where applicable
- add focused unit/integration coverage

Agent 2: LVM runtime primitives
Ownership:
- /home/tomi/gtos/core/vm/lvm.go
- nearby VM/runtime files only if required
- VM-focused tests under /home/tomi/gtos/core/
Task:
- implement the first runtime-backed integration of the new registry layer into LVM
- minimum required v1:
  - registry-backed capability lookup
  - fail-closed runtime behavior on unresolved capability names
  - native query surface improvements for agent/capability inspection
- if package_call hardening or agentload cleanup is required for coherence, make the smallest defensible change
- add focused rollback/error-path/runtime tests

Agent 3: Package identity and publishing
Ownership:
- GTOS-side package identity / publishing registry implementation if protocol-grade slice is feasible
- otherwise toolchain-compatible protocol skeleton plus RPC/types
- tests and docs only in this area
Task:
- implement a narrow but real v1 from PACKAGE_PUBLISHING_REGISTRY.md
- minimum target:
  - protocol-facing package identity record type
  - publisher record type
  - RPC/read-path support
- if full on-chain publishing is too large, land a registry-backed read/query skeleton that establishes the canonical model without faking “done”
- do not break current Tolang local package export/import flow

Agent 4: RPC / metadata / discovery integration
Ownership:
- /home/tomi/gtos/internal/tosapi/
- GTOS metadata-facing read APIs
- minimal /home/tomi/tolang metadata/export changes only if strictly required for compatibility
Task:
- make GTOS expose the new protocol-backed information in agent-facing inspection surfaces
- target:
  - deployed contract metadata RPC can join registry/package facts where available
  - responses remain backward-compatible where practical
- align with AGENT_ABI_SCHEMA.md and DISCOVERY_TYPED_SCHEMA.md rather than inventing parallel JSON shapes
- add RPC-focused tests

Agent 5: Validation / doc sync / cross-repo integration
Ownership:
- /home/tomi/gtos/docs
- /home/tomi/tolang/docs only where cross-repo status must be synchronized
- non-overlapping integration tests
Task:
- keep GTOS/Tolang docs aligned with what is actually implemented
- update status only for work that is truly landed in code and covered by tests
- write down any deliberate v1 cuts or postponed items
- if a tiny Tolang-side compatibility patch is required to consume the new GTOS behavior, make it and cover it with tests

Recommended implementation order inside the run:
1. Capability Registry data model + writes + reads
2. LVM capability lookup integration
3. GTOS metadata/RPC exposure
4. Delegation Registry v1
5. Package identity registry v1 or query skeleton

Definition of done for this wave:
- GTOS has a real protocol-backed registry slice, not just a stubbed design
- LVM uses registry-backed capability resolution for at least one real runtime path
- unresolved capability names fail closed in protocol/runtime, not just in Tolang lowering
- GTOS metadata/RPC exposes the new protocol-backed facts to agent consumers
- package identity/publisher model has a concrete implementation foothold, even if full publishing is phased
- tests cover state, runtime behavior, and RPC/read surfaces

Acceptance:
- cd /home/tomi/gtos && go test ./...
- cd /home/tomi/tolang && go test ./...   (only if Tolang-side integration changed)
- if Tolang release/export output changed:
  - cd /home/tomi/tolang && go run ./cmd/stdlib-export
  - cd /home/tomi/tolang && go test -run 'TestStdlibReleaseArtifactsAreCurrent' -v .

Required final report:
- what part of GTOS_PROTOCOL_REGISTRIES.md was implemented
- what part of LVM_NATIVE_ECONOMIC_PRIMITIVES.md was implemented
- what part of PACKAGE_PUBLISHING_REGISTRY.md was implemented
- exact files changed
- exact tests added
- exact commands run
- what remains intentionally open after this wave

Execution posture:
Do not stop at analysis. Spawn the 5 agents now and carry the work through implementation, tests, integration, and final verification.
```

---

## Notes

- This prompt is intentionally GTOS-first. It is meant for the part of the
  roadmap that can no longer be closed by stdlib work alone.
- The narrowest defensible v1 is:
  1. Capability Registry
  2. LVM capability lookup integration
  3. GTOS metadata/RPC exposure
  4. Delegation Registry skeleton
  5. Package identity query skeleton
- If scope must be cut, cut package publishing before cutting registry-backed
  capability enforcement.
