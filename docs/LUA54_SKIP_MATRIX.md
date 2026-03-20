# Lua 5.4 `TOL: SKIP` Matrix

## Purpose

This document explains why some files under `_lua-5.4.8-tests/` carry a `TOL: SKIP` header in the
TOL repository.

Important context:

- Every script listed below is currently a **fully commented-out no-op wrapper** in this repo.
- Therefore, a `PASS` result for one of these files does **not** mean the underlying upstream Lua
  feature is supported by the deterministic TOL VM.
- Grouping is by the **dominant disablement reason**. Some scripts touch multiple subsystems.
- `cstack.lua` is placed under **perf** because it is primarily a host-stack / stress test, even
  though it also depends on the C API.
- `all.lua` is placed under **io-os** because it is the original shell-driven runner and is mainly
  disabled by removed host facilities (`io`, `os`, `load`, `dofile`), even though it also touches
  coroutines.

## Coroutine

| Script | Header Quote | Why It Is Disabled In TOL | Re-enable Prerequisites |
|---|---|---|---|
| `coroutine.lua` | "coroutine library removed from TOL" | TOL intentionally does not expose the `coroutine` library. The deterministic VM is designed around gas-bounded execution without resumable user threads. | A deterministic coroutine model, audited gas semantics for yield/resume/close, and a product decision to expose coroutine APIs in consensus-visible code. |

## Debug

| Script | Header Quote | Why It Is Disabled In TOL | Re-enable Prerequisites |
|---|---|---|---|
| `db.lua` | "debug library removed from TOL" | The `debug` library exposes runtime internals, stack inspection, hooks, and mutation surfaces that are intentionally absent from the deterministic contract VM. | A deliberately scoped debug surface, or a separate non-consensus test target that is not part of contract execution guarantees. |
| `code.lua` | "requires debug library for internal bytecode checks" | This script depends on low-level inspection helpers from the debug layer rather than contract-visible language semantics. TOL does not expose those internals to contracts. | A dedicated internal bytecode test harness, or an explicitly supported debug/introspection API. |
| `api.lua` | "C API tests not applicable to TOL Go VM" | The upstream file validates Lua's C API contract. TOL is a Go VM embedded for deterministic smart-contract execution, so that API boundary is not the product surface being preserved. | A Go-native low-level API conformance suite, or a compatibility layer that intentionally mirrors the Lua C API. |

## IO/OS

| Script | Header Quote | Why It Is Disabled In TOL | Re-enable Prerequisites |
|---|---|---|---|
| `files.lua` | "io library removed from TOL" | TOL forbids filesystem-backed I/O in consensus-visible execution. File handles, streams, shell pipes, and locale-sensitive host behavior are outside the deterministic VM contract. | A separate host-environment test target, not the chain VM; or a deliberate design change to expose file APIs, which would conflict with current determinism goals. |
| `main.lua` | "requires io/os/load/dofile — all removed from TOL" | This script targets the stand-alone interpreter, shell integration, dynamic loading, and OS process behavior. None of those are part of TOL's contract runtime. | A stand-alone interpreter mode for TOL, or a separate compatibility runner outside the deterministic VM. |
| `all.lua` | "original runner — uses io/os/load/dofile/coroutine" | Upstream `all.lua` is the orchestration script for the full Lua test suite. It assumes removed host capabilities and therefore is preserved only as a disabled historical wrapper. | A host-side compatibility runner that is intentionally broader than the on-chain VM, or a TOL-specific orchestrator that only calls supported semantic tests. |

## GC

| Script | Header Quote | Why It Is Disabled In TOL | Re-enable Prerequisites |
|---|---|---|---|
| `gc.lua` | "collectgarbage/GC not available in TOL" | TOL does not expose `collectgarbage`. Consensus termination is gas-driven, and host GC behavior is intentionally not part of the contract-visible surface. | A supported, deterministic GC control API, or a separate runtime-level test suite that validates host implementation details rather than contract semantics. |
| `gengc.lua` | "collectgarbage/GC not available in TOL" | This file specifically targets generational GC mode and collector barrier behavior, which are implementation details outside TOL's public VM contract. | The same as `gc.lua`, plus an explicit commitment to generational GC semantics as part of the public runtime model. |
| `tracegc.lua` | "collectgarbage/GC not available in TOL" | This helper depends on GC callbacks and collector progress visibility. Those hooks are deliberately absent from TOL. | An exposed GC tracing surface for non-consensus runtime diagnostics, not for contract execution. |

## Perf

| Script | Header Quote | Why It Is Disabled In TOL | Re-enable Prerequisites |
|---|---|---|---|
| `big.lua` | "stress/performance test — run manually" | This is a stress script, not a default semantic regression test. It is useful for manual capacity experiments, but too heavy and environment-sensitive for normal deterministic regression coverage. | A dedicated stress/perf lane with explicit resource budgets and expected runtime ceilings. |
| `heavy.lua` | "stress/performance test — run manually" | Same reason as `big.lua`: it is valuable for pressure testing, but it is not a stable signal for default VM correctness in CI-style regression runs. | A dedicated heavy-test profile with time and memory limits tuned for the target environment. |
| `verybig.lua` | "stress/performance test — run manually" | Same reason as `big.lua` and `heavy.lua`: it is intended for manual large-scale stress behavior, not regular semantic compatibility checks. | A dedicated heavy-test profile with environment-specific thresholds. |
| `cstack.lua` | "C-stack tests require C API not available in TOL" | This file probes host C-stack limits, recursion depth behavior, and runtime failure modes that do not map cleanly onto the Go-based deterministic VM. It is closer to a host stress test than to a contract-language semantic test. | A Go-specific stack-pressure suite, or an intentionally supported low-level host compatibility target. |

## Notes For Maintainers

- If a file keeps the `TOL: SKIP` header but is still listed in `script_test.go`, that file is
  currently acting as a **documented no-op placeholder**, not as active semantic coverage.
- When a formerly skipped upstream script becomes meaningfully supported again, prefer one of these
  paths:
  - restore active executable contents for that script, or
  - port the relevant assertions into a TOL-specific Go or Lua test that matches the deterministic
    VM surface.
- Do not treat these files as proof of support unless the file is active again and the reason for
  the skip has been removed.
