# Stable Package Identity and Publishing Model

**Status**: DESIGN
**Date**: 2026-03-21

---

## Problem Statement

Package identity is currently derived from filesystem layout and source paths.
Import resolution depends on directory structure relative to a compile root.
This makes package identity fragile, non-portable, and unsuitable for
content-addressed publishing or cross-environment reproducibility.

---

## Current State

| Component | How it works today | Limitation |
|-----------|-------------------|------------|
| Package name | Derived from source directory path (e.g., `tolang.stdlib.account`) | Tied to filesystem layout |
| Import resolution | Compiler walks directories relative to `--pkg-root` | Breaks when source tree moves |
| `.tor` archives | ZIP containing bytecode + ABI + init; identified by `package_hash` (keccak256 of archive) | Content-addressed but not version-resolved |
| `manifest.json` | Optional version string in contract manifest block | Not enforced by toolchain |
| `stdlib/releases/index.json` | Maps family/contract to release paths and hashes | Flat file; no dependency resolution |
| `.agentpkg.json` | Per-contract and per-bundle metadata with `package_name` and `package_version` | Name is dot-path from source, not a stable identifier |

The `package_hash` in `ArtifactRef` is already content-addressed. The gap is
that there is no stable identity model that decouples package naming from
source layout and supports version resolution at import time.

---

## Proposed Mechanism

### Package identity

A package is identified by a triple: `(name, version, content_hash)`.

- **name**: reverse-domain style, declared in source (`package tolang.stdlib.account;`)
  rather than inferred from directory path
- **version**: semver, declared in manifest block, enforced by compiler
- **content_hash**: keccak256 of the `.tor` archive (already computed as `package_hash`)

The compiler validates that `package_name` in source matches the directory-derived
name during development, but published packages are resolved by name+version, not
by path.

### Import resolution

Current: `import "tolang/stdlib/account" as Account;` resolves to a filesystem path.

Proposed: resolution order becomes:

1. **Local source** -- `--pkg-root` relative paths (development mode)
2. **Package cache** -- `~/.tolang/packages/<name>/<version>/` (installed packages)
3. **Registry lookup** -- on-chain or HTTP registry returns `.tor` by `(name, version)`

The compiler flag `--pkg-registry <url>` enables remote resolution. Without it,
behavior is identical to today (local-only).

### Publishing pipeline

```
tol publish --name tolang.stdlib.account --version 1.0.0
```

1. Compile source to `.tor` archive
2. Compute `content_hash` = keccak256(`.tor`)
3. Register `(name, version, content_hash)` in registry
4. Upload `.tor` + `.agentpkg.json` + `.discovery.json` to content store

The registry is append-only: a `(name, version)` pair cannot be overwritten.

### Dependency declaration

A new `requires` block in the manifest:

```
manifest {
    version = "1.0.0";
    requires = {
        "tolang.stdlib.authority" = "^1.0.0",
        "tolang.stdlib.receipt" = "^1.0.0"
    };
}
```

The compiler resolves dependencies before compilation. Version constraints
use semver ranges. The resolved versions are locked in a `tol.lock` file.

### Relation to `stdlib/releases/`

The current `stdlib/releases/` pipeline produced by `cmd/stdlib-export` becomes
the seed content for the package cache. `index.json` evolves into a registry
index format. Existing hashes and paths remain valid.

---

## GTOS Dependencies

- **On-chain registry (optional)**: a `PackageRegistry` system contract that
  maps `(name, version)` to `content_hash`. This enables on-chain import
  verification but is not required for the first iteration.
- **No consensus changes** for the initial model. Package resolution happens
  at compile time, not at execution time.

---

## Acceptance Criteria

- [ ] `package` declaration in source is validated against directory-derived name
- [ ] `--pkg-registry` flag enables remote package resolution
- [ ] Published packages are identified by `(name, version, content_hash)` triple
- [ ] `tol.lock` file records resolved dependency versions
- [ ] `stdlib/releases/index.json` format is forward-compatible with registry index
- [ ] Existing compile paths (`--pkg-root`) continue to work unchanged
- [ ] `.tor` content hashes are stable across publish/install cycles

---

## Related Documents

- `docs/TOLANG_SHORTCOMINGS.md` -- shortcoming #4 (filesystem-dependent resolution)
- `docs/AGENT_NATIVE_STDLIB_2046.md` -- package families and release pipeline
- `stdlib/releases/index.json` -- current release index format
- `metadata/metadata.go` -- `ArtifactRef.PackageHash` and `ArtifactRef.Version`
