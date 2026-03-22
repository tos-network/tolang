# Unified ABI/Discovery/Capability Schema for Agent Runtimes

**Status**: DESIGN
**Date**: 2026-03-21

---

## Problem Statement

Agent runtimes currently consume four separate artifact types to understand a
contract: `.toc` (ABI + bytecode), `.discovery.json` (service metadata),
`.agentpkg.json` (capability + discovery + artifact refs), and `FunctionMeta`
(extracted from `.toc`). These overlap substantially but use different shapes,
leading to redundant data, inconsistent fields, and no single canonical schema
an agent can consume.

---

## Current State

| Artifact | Schema | Contains | Consumer |
|----------|--------|----------|----------|
| `.toc` | `ContractMetadata` (metadata.go) | ABI functions, events, errors, gas model, capabilities, policy profile | Compiler, deployment tools |
| `.discovery.json` | ad hoc JSON | `contract_type`, `service_kinds`, `interface_methods` with `risk_level`/`failure_modes`/`summary`, `human_summary`, `tags` | Discovery clients |
| `.agentpkg.json` | ad hoc JSON | `artifact_ref`, `capabilities` (verifiable/delegated methods), `discovery` (embedded), `human_summary` | Agent runtimes, OpenFox |
| `FunctionMeta` | `metadata.go` struct | `requires_capability`, `effects`, `gas_upper`, `verifiable`, `delegated`, `failure_modes`, `risk_level` | Intent routing, approval UX |

Problems:
- `interface_methods` in `.discovery.json` duplicates `FunctionMeta` fields
  but uses different field names (`payable` vs `mutability: "payable"`)
- `capabilities` in `.agentpkg.json` lists `verifiable_methods` separately
  from `interface_methods` in the embedded `discovery` block
- `artifact_ref` appears in both `.agentpkg.json` and `.discovery.json`
- No single document answers: "What can I call, what are the risks, what
  capabilities do I need, and what proofs can I get?"

---

## Proposed Mechanism

### Single canonical schema: `AgentContractProfile`

Replace the three separate JSON artifacts with one canonical schema that an
agent runtime can consume as a single document:

```
AgentContractProfile {
  schema_version: string           // "0.2.0"
  identity: {
    package_name: string
    package_version: string
    content_hash: string           // keccak256 of .tor
    bytecode_hash: string
    abi_hash: string
  }
  contract: {
    name: string
    type: string                   // "account", "settlement", "token", "custom"
    is_account: bool
    policy_profile: PolicyProfile?
    tags: []string
  }
  capabilities: []string           // declared capability names
  functions: [{
    name: string
    selector: string
    mutability: string             // "pure", "view", "payable", "nonpayable"
    risk_level: string             // "low", "medium", "high"
    requires_capability: []string
    delegated: bool
    verifiable: bool
    effects: EffectsMeta?
    gas_upper: uint64
    failure_modes: []FailureMode
    params: []ParamMeta
    returns: []ParamMeta
  }]
  events: []EventMeta
  errors: []ErrorMeta
  service_kinds: []string          // "query", "token_transfer", "escrow", etc.
  human_summary: string
  gas_model: GasModelMeta
}
```

### Derivation

The `AgentContractProfile` is derived deterministically from the `.toc`
artifact at export time. It replaces `.discovery.json` and `.agentpkg.json`
as separate files. The `.toc` remains the canonical compiler output;
the profile is the canonical agent-facing view.

### Bundle profiles

For multi-contract families (e.g., `privacy`), a bundle profile lists
the per-contract profiles plus family-level metadata:

```
AgentBundleProfile {
  schema_version: string
  family: string
  package_name: string
  contracts: []AgentContractProfile
  human_summary: string
}
```

This replaces `.bundle.agentpkg.json` and `.bundle.discovery.json`.

Status note:

- per-contract `.profile.json` is implemented
- family-level `.bundle.profile.json` is now emitted by the release exporter
- GTOS now consumes the unified profile family by returning `profile`,
  `bundle_profile`, `parsed_card`, and `suggested_card` views, and can publish
  a recommended discovery card directly from deployed metadata
- GTOS also exposes a read-only suggested-card path so clients can fetch the
  canonical structured card for deployed `.toc` / `.tor` code without
  publishing into discovery first
- `tosclient` now exposes typed discovery/suggested-card wrappers plus a typed
  `GetContractMetadata(...)` client, so external agent runtimes can consume the
  unified surface without crafting raw RPC payloads by hand; the same client
  layer now also exposes typed reads for protocol registry and package-
  governance facts such as capability, delegation, package, publisher,
  verifier, verification, settlement policy, and agent identity
- `gtosclient` now adds a higher-level `GetAgentRuntimeSurface(...)` helper on
  top of those typed RPCs, so clients can consume one normalized object instead
  of branching separately on deployed `.toc` vs `.tor` metadata
- `gtosclient` also now adds `GetDiscoveredAgentSurface(...)`, which joins a
  published discovery card with the deployed runtime metadata surface when the
  structured card advertises a canonical `agent_address`
- `gtosclient` also now adds `SearchDiscoveredAgentSurfaces(...)` and
  `DirectorySearchDiscoveredAgentSurfaces(...)`, which batch-join capability
  search results with published card data and deployed runtime metadata
- `gtosclient` also now adds `SearchTrustedAgentSurfaces(...)` and
  `DirectorySearchTrustedAgentSurfaces(...)`, which filter and rank those
  joined provider surfaces using discovery trust summaries plus package trust
  hints from the deployed runtime metadata
- `gtosclient` also now adds `SearchPreferredAgentSurfaces(...)`,
  `DirectorySearchPreferredAgentSurfaces(...)`, and
  `SelectPreferredAgentSurface(...)`, which apply higher-level connection,
  package, typed-routing, disclosure, and trust thresholds on top of the
  trusted provider surface
- `gtosclient` also now adds `ResolvePreferredAgentSurface(...)` and
  `ResolveDirectoryPreferredAgentSurface(...)`, which collapse search/join/
  trust/ranking/preference-selection into a single client-side resolve step
- `gtosclient` also now adds
  `SearchPreferredAgentSurfaceDiagnostics(...)` and
  `DirectorySearchPreferredAgentSurfaceDiagnostics(...)`, which produce a
  structured explanation of trust failures, preference failures, and final
  `preferred` status for each candidate surface
- `tosdk` now mirrors that higher-level discovery consumption layer for
  TypeScript applications via `searchPreferredAgentProvider(...)`,
  `directorySearchPreferredAgentProvider(...)`, and the matching
  `...WithDiagnostics(...)` helpers; it also now exposes
  `summarizeAgentProviderDiagnostics(...)`,
  `requirePreferredAgentProvider(...)`, and the matching `...OrThrow(...)`
  helpers for explainable failure handling in app code
- OpenFox now consumes the same typed metadata hints directly in its
  agent-discovery selection policy and exposes
  `resolveCapabilityProvider(...)`, `diagnoseCapabilityProviders(...)`, and
  `resolveCapabilityProviderWithDiagnostics(...)`; higher-level request paths
  now also surface provider-selection failure reasons so runtime-level
  provider selection can be explained to users and agents, and that same
  explainability now reaches signer/paymaster CLI discovery plus the
  `discover_capability_providers` tool surface; gateway session selection,
  solver bounty discovery, and opportunity scouting now also persist the
  latest provider-selection explanation into local state for orchestration
  and planner consumption, while direct provider invocation paths now also
  fall back across ranked providers and feed those outcomes back into the
  local ranking model used for future selection
- legacy `.discovery.json` / `.agentpkg.json` bundle artifacts are still emitted
  for compatibility during the transition

### Protocol alignment layer

The unified profile now also carries additive `protocol_alignment`,
`threat_model`, and `runtime_boundary` sections.

`protocol_alignment` is not a new execution primitive. It is a machine-readable
hint layer for the next GTOS-owned wave:

- `settlement_bus`
- `registry_governance`
- `package_governance`

The field exists to help agent runtimes and release tools understand which
contracts are aligned with the next protocol surfaces without changing any
existing ABI or discovery fields.

Example:

```json
{
  "protocol_alignment": {
    "schema_version": "0.1.0",
    "settlement_bus": true,
    "registry_governance": false,
    "package_governance": true,
    "release_artifacts": ["profile_json", "discovery_json", "agent_package_json"]
  }
}
```

For family-level bundle artifacts, the merged `protocol_alignment.release_artifacts`
also includes:

- `bundle_profile_json`
- `bundle_discovery_json`
- `bundle_agent_package_json`

The new `threat_model` section lifts the family-level release posture from
`docs/STDLIB_THREAT_MODEL_MATRIX.md` into machine-readable metadata. It is
exported in:

- per-contract `.profile.json`
- per-contract `.discovery.json`
- per-contract `.agentpkg.json`
- family `.bundle.profile.json`
- family `.bundle.discovery.json`
- family `.bundle.agentpkg.json`

Example:

```json
{
  "threat_model": {
    "schema_version": "0.1.0",
    "family": "settlement",
    "scope": "contract",
    "trust_boundary": "task poster, worker, dispute resolver",
    "critical_invariants": [
      "escrowed reward must map to exactly one terminal payout path",
      "reject, dispute, and resolve states must be exclusive"
    ],
    "failure_posture": "fail closed on wrong status or wrong actor",
    "runtime_dependency": "strong dependency on host escrow and release correctness plus rollback"
  }
}
```

### Runtime boundary layer

`runtime_boundary` is the machine-readable answer to:

- which surfaces are GTOS-native protocol semantics
- which openlib package/contract entrypoints should be preferred by developers
- which long-term protocol domains should eventually collapse into clearer
  system-contract shapes

Example:

```json
{
  "runtime_boundary": {
    "schema_version": "0.1.0",
    "native_surfaces": ["settlement_bus", "package_registry"],
    "preferred_openlib": [
      "tolang.openlib.settlement.task",
      "tolang.openlib.settlement.task.TaskSettlement"
    ],
    "future_system_surfaces": ["system.settlement", "system.receipt", "system.package_registry"],
    "notes": ["prefer_openlib_entrypoints", "native_surfaces_are_protocol_semantics"]
  }
}
```

This boundary layer is emitted alongside the same profile/discovery/agent
package surfaces as `threat_model`.

### Migration path

1. Add `AgentContractProfile` to `metadata/metadata.go`
2. Add `BuildAgentProfile(cm ContractMetadata) AgentContractProfile` to metadata package
3. Update `cmd/openlib-export` to emit `.profile.json` alongside existing artifacts
4. Deprecate `.discovery.json` and `.agentpkg.json` (keep emitting for one version)
5. Remove deprecated artifacts after consumers migrate

---

## GTOS Dependencies

- **Deployed metadata RPC**: The GTOS `tos_getContractMetadata` RPC should
  return `AgentContractProfile` directly, rather than requiring the caller
  to reconstruct it from `.toc` fields
- **No consensus changes**: profile generation is a build-time/RPC concern

---

## Acceptance Criteria

- [ ] `AgentContractProfile` struct defined in `metadata/metadata.go`
- [x] `BuildAgentProfile` produces identical information to current `.discovery.json` + `.agentpkg.json`
- [x] Single `.profile.json` file per contract is emitted alongside the legacy files
- [x] Bundle `.profile.json` is emitted alongside `.bundle.discovery.json` + `.bundle.agentpkg.json`
- [x] Machine-readable `threat_model` is emitted alongside profile/discovery/agent package artifacts
- [ ] No information loss compared to current artifacts
- [ ] `risk_level`, `failure_modes`, `requires_capability`, `verifiable`, `delegated` all present in one document
- [x] GTOS `tos_getContractMetadata` returns the unified profile

---

## Related Documents

- `metadata/metadata.go` -- current `ContractMetadata`, `FunctionMeta`, `ArtifactRef`
- `docs/TOLANG_SHORTCOMINGS.md` -- shortcoming #6 (ABI/discovery not unified)
- `docs/AGENT_NATIVE_STDLIB_2046.md` -- discovery and manifest design
- `openlib/releases/index.json` -- current release index
- `docs/FEATURE_MATURITY_MATRIX.md` -- unified ABI schema marked as partial
