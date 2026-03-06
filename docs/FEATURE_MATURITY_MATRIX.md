# Feature Maturity Matrix
## Tolang Implementation Status

**Status:** Draft  
**Version:** 0.1  
**Intended location:** `docs/FEATURE_MATURITY_MATRIX.md`

## Purpose

This document separates:

- features implemented and usable today,
- features implemented in the compiler but still depending on gtos/runtime support,
- and features that are proposed or partially specified.

This distinction is essential for technical credibility. Tolang already has a strong architecture and strong agent-native direction; this matrix makes the implementation boundary explicit.

## Status Labels

- **Implemented Today** — available end-to-end in the current compiler/artifact pipeline and usable with current documented execution flow
- **Compiler-Complete / Runtime-Dependent** — implemented in parser/sema/lowering/artifact metadata, but full semantics depend on gtos runtime, registries, or host support
- **In Progress** — significant implementation exists but semantics or integration are incomplete
- **Proposed** — design exists, but the feature is not yet a stable implementation target

## Matrix

| Feature | Parser | Sema | Lowering / IR | Artifact / ABI | Runtime / gtos | Status |
|---|---:|---:|---:|---:|---:|---|
| `.tol` source compilation pipeline | ✅ | ✅ | ✅ | ✅ | ✅ | Implemented Today |
| `.toc` compiled artifact | ✅ | ✅ | ✅ | ✅ | ✅ | Implemented Today |
| `.tor` deterministic packaging | ✅ | ✅ | ✅ | ✅ | ✅ | Implemented Today |
| init / runtime split | ✅ | ✅ | ✅ | ✅ | ✅ | Implemented Today |
| Solidity-inspired contracts / interfaces / structs / mappings | ✅ | ✅ | ✅ | ✅ | ✅ | Implemented Today |
| `tol.lang` auto-import platform layer | ✅ | ✅ | ✅ | ✅ | ✅ | Implemented Today |
| `@effects` metadata | ✅ | ✅ | ✅ | ✅ | — | Implemented Today / evolving |
| `@bounds` metadata | ✅ | ✅ | ✅ | ✅ | — | Implemented Today / evolving |
| `@gas` metadata | ✅ | ✅ | ✅ | ✅ | — | Implemented Today / evolving |
| `manifest {}` | ✅ | ✅ | ✅ | ✅ | — | Implemented Today |
| `capability` declarations | ✅ | ✅ | ✅ | ✅ | partial | Compiler-Complete / Runtime-Dependent |
| `@requires(...)` | ✅ | ✅ | ✅ | ✅ | partial | Compiler-Complete / Runtime-Dependent |
| `agent` type | ✅ | ✅ | ✅ | partial | depends on registry | Compiler-Complete / Runtime-Dependent |
| `oracle<T>` | ✅ | ✅ | ✅ | partial | partial host support | In Progress |
| `task<T>` | ✅ | ✅ | ✅ | partial | partial host support | In Progress |
| `@delegated` | ✅ | ✅ | ✅ | ✅ | depends on delegation infra | Compiler-Complete / Runtime-Dependent |
| `@verifiable` | ✅ | ✅ | ✅ | ✅ | depends on verifier/proof flow | Compiler-Complete / Runtime-Dependent |
| `@pay` | ✅ | ✅ | ✅ | ✅ | depends on settlement rules | Compiler-Complete / Runtime-Dependent |
| account-style / AA-aligned contract markers | ✅ | ✅ | ✅ | ✅ | depends on validation path | Compiler-Complete / Runtime-Dependent |
| ABI unified schema | partial | partial | partial | partial | — | Proposed |
| proof-carrying task completion | — | — | partial | partial | — | Proposed |
| task marketplace standard interfaces | — | — | — | — | — | Proposed |
| dispute / slashing standard model | — | — | — | — | — | Proposed |

## Notes

### 1. “Implemented” does not always mean “fully protocol-backed”
Some features are already real at the language and compiler layer, but their strongest semantics still depend on protocol-level registries, host functions, or system contracts in gtos.

### 2. Metadata is a major existing strength
Tolang already has unusually strong artifact and metadata architecture. Even where full runtime semantics are still maturing, the compiler-side shape of agent-native semantics is significantly ahead of many contract languages.

### 3. This matrix should evolve with releases
Each stable language release should update this document and pin feature status to a language version.
