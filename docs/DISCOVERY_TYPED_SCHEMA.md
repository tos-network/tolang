# Discovery Typed Schema

**Status**: V1 IMPLEMENTED (2026-03-22)  
**Date**: 2026-03-22

---

## Purpose

This document specifies the next step after the current `ServiceDirectory`
reference-based model: a **typed discovery and capability schema** that agent
runtimes can consume directly.

The goal is not to remove rich off-chain metadata.
The goal is to stop using `bytes32` references as the only structured language
for service discovery.

---

## Problem Statement

Today, `ServiceDirectory` stores:

- `manifest_ref`
- `capability_ref`
- `version_ref`
- `quote_ref`
- `fee`
- `sla_duration_ms`

This is enough to register a service, but not enough to let an agent answer
basic routing questions without dereferencing external documents:

- what class of service is this?
- is it query-only, escrow-capable, sponsor-capable, or settlement-capable?
- what is the pricing model?
- what privacy mode does it support?
- what trust floor does it expect?
- what receipt behavior should a caller expect?

The current model is still too reference-heavy for autonomous agent routing.

---

## Scope

This document defines:

1. the typed schema that should normalize discovery and capability information
2. the minimal on-chain fields `ServiceDirectory` should expose
3. the agent-facing exported schema that should appear in release metadata
4. the migration path from the current reference-only model

---

## Non-goals

This document does not:

- replace the ABI
- replace the package identity model
- define GTOS registry consensus rules
- ban off-chain manifests
- require every capability detail to be on-chain

The design target is:

- **typed minimal core on-chain**
- **rich extensible detail off-chain**
- **stable normalized export for agents**

---

## Design Principles

1. **Typed core, referenced detail**
   Store stable, route-critical fields in typed form.
   Keep long descriptions and complex manifests as refs.

2. **Routing first**
   The schema should answer the questions an autonomous runtime asks before
   spending money or invoking a contract.

3. **Capability classes, not only hashes**
   A runtime should not have to dereference a document to know whether a service
   is an escrow, oracle, query endpoint, sponsor relay, or privacy-aware flow.

4. **Compatibility over purity**
   The new schema must coexist with current release artifacts during migration.

5. **Discovery and ABI must compose**
   A caller should be able to join typed discovery with
   `AgentContractProfile`-style metadata cleanly.

---

## Canonical Questions The Schema Must Answer

The typed schema must let an agent answer:

1. What kind of service is this?
2. What capability class does it advertise?
3. What is the pricing model?
4. What SLA or freshness promise exists?
5. What privacy mode does it support?
6. What receipt/audit behavior should the caller expect?
7. What trust or stake floor must counterparties satisfy?

If a schema cannot answer those without chasing refs, it is incomplete.

---

## Proposed Typed On-Chain Schema

### `ServiceDirectory` should evolve from:

```text
registerService(manifest_ref, capability_ref, version_ref, quote_ref)
```

to a model that still keeps refs, but adds typed routing fields.

### Minimum typed fields

Each service entry should expose:

```text
service_kind: u16
capability_kind: u16
pricing_kind: u16
privacy_mode: u16
receipt_mode: u16
fee_amount: u256
sla_duration_ms: u256
trust_floor_ref: bytes32
manifest_ref: bytes32
capability_ref: bytes32
version_ref: bytes32
quote_ref: bytes32
```

The refs remain, but the route-critical fields stop being opaque.

---

## Required Enums

### `service_kind`

Initial enum set:

1. `QUERY`
2. `PAYMENT`
3. `ESCROW`
4. `SETTLEMENT`
5. `SPONSOR_RELAY`
6. `ORACLE`
7. `TREASURY`
8. `DISCOVERY`
9. `PRIVACY_HELPER`

### `capability_kind`

Initial enum set:

1. `READ_ONLY`
2. `VALUE_TRANSFER`
3. `ESCROW_LIFECYCLE`
4. `SETTLEMENT_LIFECYCLE`
5. `SPONSOR_EXECUTION`
6. `DISCLOSURE_MANAGEMENT`
7. `AUDIT_RECEIPT`

### `pricing_kind`

Initial enum set:

1. `FREE`
2. `FIXED_FEE`
3. `QUOTE_REQUIRED`
4. `SUBSCRIPTION`
5. `METERED`

### `privacy_mode`

Initial enum set:

1. `PUBLIC_ONLY`
2. `CONFIDENTIAL_OPTIONAL`
3. `CONFIDENTIAL_REQUIRED`
4. `DISCLOSURE_READY`

### `receipt_mode`

Initial enum set:

1. `NO_RECEIPT`
2. `MANUAL_RECEIPT`
3. `AUTO_RECEIPT_EXPECTED`
4. `AUDIT_RECEIPT_REQUIRED`

---

## Off-Chain Normalized Export

The on-chain typed fields should be mirrored into an exported normalized schema.

This document does not replace [AGENT_ABI_SCHEMA.md](/home/tomi/tolang/docs/AGENT_ABI_SCHEMA.md).
It complements it by defining the discovery subdocument more concretely.

### Proposed `TypedDiscoveryProfile`

```text
TypedDiscoveryProfile {
  schema_version: string
  contract_name: string
  service_kind: string
  capability_kind: string
  pricing: {
    kind: string
    base_fee: string
    quote_ref: string
  }
  privacy: {
    mode: string
    disclosure_ready: bool
  }
  receipts: {
    mode: string
    receipt_contract_ref: string?
  }
  sla: {
    duration_ms: string
  }
  refs: {
    manifest_ref: string
    capability_ref: string
    version_ref: string
    trust_floor_ref: string?
  }
}
```

This should either:

1. become a subdocument inside `AgentContractProfile`, or
2. be emitted as a stable `.typed-discovery.json` during migration

The first option is cleaner long term.

---

## What Should Stay As Refs

The following should remain referenced, not forced on-chain:

- human-readable manifest text
- long SLA/legal text
- full capability matrix
- regional/compliance attachments
- pricing formulas that exceed simple fixed-fee description

The typed schema should answer routing questions.
The refs should answer rich semantic questions.

---

## Required `ServiceDirectory` API Additions

The next implementation wave should add typed setters/getters rather than only
raw refs.

Minimum target:

```text
setServiceKind(service_id, service_kind)
setCapabilityKind(service_id, capability_kind)
setPricingKind(service_id, pricing_kind)
setPrivacyMode(service_id, privacy_mode)
setReceiptMode(service_id, receipt_mode)

serviceKindOf(service_id)
capabilityKindOf(service_id)
pricingKindOf(service_id)
privacyModeOf(service_id)
receiptModeOf(service_id)
```

These are additive.
They do not remove the existing ref-based fields.

---

## Migration Strategy

### Phase 1: Add typed fields without breaking refs

- keep current `registerService(...)`
- add typed setters/getters
- keep `manifest_ref` / `capability_ref` / `quote_ref`

### Phase 2: Export normalized typed discovery

- extend `cmd/stdlib-export`
- emit typed discovery profile next to current artifacts
- make GTOS deployed metadata RPC return the typed discovery view if available

### Phase 3: Make typed registration ergonomic

Add a convenience registration path such as:

```text
registerTypedService(
  manifest_ref,
  capability_ref,
  version_ref,
  quote_ref,
  service_kind,
  capability_kind,
  pricing_kind,
  privacy_mode,
  receipt_mode,
  fee_amount,
  sla_duration_ms
)
```

This phase is optional if additive setters are sufficient.

---

## Test Requirements

Implementation is not complete without:

1. runtime tests for typed field setters/getters
2. composed examples showing routing-relevant metadata
3. exporter tests verifying typed discovery output
4. compatibility tests proving legacy ref-based paths still work
5. metadata tests proving typed discovery aligns with agent-facing ABI/profile exports

---

## Acceptance Criteria

This design is considered implemented when:

- `ServiceDirectory` exposes typed discovery fields
- refs remain backward compatible
- release export includes a normalized typed discovery representation
- GTOS/Tolang metadata consumers can read typed routing fields without ref chasing
- existing services can migrate incrementally
- at least one composed example uses typed discovery for routing decisions

V1 status:

- `ServiceDirectory` exposes typed routing fields on-chain
- release/export emits normalized `typed_discovery`
- `PrivateServiceOrder` now uses typed discovery for route acceptance
- GTOS deployed metadata RPC now exposes a normalized `routing_profile`

---

## Recommended Implementation Order

1. Add typed fields to `ServiceDirectory`
2. Add runtime tests
3. Extend exporter metadata output
4. Align with `AgentContractProfile`
5. Add composed example using typed discovery in route selection

---

## Relationship To Existing Documents

- [AGENT_ABI_SCHEMA.md](/home/tomi/tolang/docs/AGENT_ABI_SCHEMA.md)
  This document defines the broader unified agent-facing ABI/profile direction
- [AGENT_NATIVE_STDLIB_2046.md](/home/tomi/tolang/docs/AGENT_NATIVE_STDLIB_2046.md)
  This document closes the "discovery schema normalization" backlog item
- [STDLIB_CAPABILITY_ANALYSIS.md](/home/tomi/tolang/docs/STDLIB_CAPABILITY_ANALYSIS.md)
  This document turns the discovery ergonomics gap into concrete implementation
  work
- [PACKAGE_IDENTITY_MODEL.md](/home/tomi/tolang/docs/PACKAGE_IDENTITY_MODEL.md)
  Package identity remains a separate concern from typed discovery
