# TODO / 任务一览

Last updated: 2026-03-20

## Audit Follow-Ups

| ID | Item | Status | Notes |
|---|---|---|---|
| T-4 | Make default `.tol -> bytecode/.toc/.tor` outputs reproducible across host paths by stripping source-map/debug metadata unless explicitly requested | Done | Default compile/package outputs no longer embed host-dependent `SourceName` paths |
| T-5 | Remove host-side interrupt channel from the deterministic VM execution path | Done | Execution termination is gas-driven only; host cancellation API removed |
| T-6 | Bound hash-table tombstone growth while preserving stale-key iteration semantics for `next/pairs` | Done | Tombstones are retained only for active stale traversal and compacted afterwards |
