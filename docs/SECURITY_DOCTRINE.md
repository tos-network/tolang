# Security Doctrine
## Tolang Security Philosophy for the Agent Economy

**Status:** Draft  
**Version:** 0.1  
**Intended location:** `docs/SECURITY_DOCTRINE.md`

## 1. Why Tolang Needs an Explicit Security Doctrine

Most smart contract ecosystems document security through scattered audit notes, runtime restrictions, and informal best practices. That is not sufficient for an agent-native language.

In the Agent Economy, software systems will invoke contracts continuously, compose services automatically, consume delegated permissions, move value without human review on every step, and react to machine-readable policy. In such a world, security is not merely a property of the VM. It is a property of the language, the compiler, the artifact, the interface, and the execution model together.

Tolang therefore adopts an explicit security doctrine.

This doctrine is not a list of implementation bugs. It is the set of principles that define what Tolang should optimize for, what kinds of power it should expose, and what kinds of implicit risk it should reject.

## 2. Principle One — Determinism Over Opaque Intelligence

Tolang is designed for a world where agents may use AI, external data, and off-chain computation. But consensus and settlement cannot rely on opaque, non-deterministic behavior.

Therefore:

- economically relevant execution must be deterministic,
- non-deterministic workflows must be mediated through explicit settlement patterns,
- and any AI- or oracle-assisted action must become economically legible through attestations, proofs, state transitions, or challengeable outputs.

Tolang does not reject intelligence.  
It rejects intelligence that cannot be safely settled.

## 3. Principle Two — Explicit Authority Over Hidden Power

In insecure systems, authority is often buried inside application logic, helper contracts, or undocumented assumptions. Tolang must move in the opposite direction.

Authority should be visible in:

- syntax,
- type-level concepts,
- compiler-emitted metadata,
- and interface declarations.

Capabilities, delegation acceptance, payment requirements, and verification expectations should be declared, not implied.

A caller — human or agent — should be able to know in advance what kind of authority is required and what kind of authority may be exercised.

## 4. Principle Three — Bounded Execution Over Expressive Chaos

The Agent Economy cannot be built on execution models that are impossible to meter, impossible to predict, or impossible to inspect.

Tolang therefore prefers:

- analyzable loops,
- bounded or conservatively modeled resource consumption,
- explicit package constraints,
- predictable storage behavior,
- and stable gas semantics.

Expressiveness matters, but unconstrained expressiveness is not a virtue when autonomous systems are expected to trust and compose with one another.

The right question is not “can the language express anything?”  
The right question is “can the language express economically meaningful behavior safely enough to automate?”

## 5. Principle Four — Artifact Trust Over Source-Only Trust

Source code matters, but automation cannot rely on source code alone.

Agents, wallets, verifiers, and marketplaces need a machine-trustable unit that is closer to execution reality. In Tolang, that unit should be the compiled artifact and its bytecode-bound metadata.

This means:

- emitted metadata should be compiler-generated or compiler-verified,
- important semantics should be bound to bytecode hashes where possible,
- and automation should prefer verified artifacts over unauthenticated prose descriptions.

Source can explain intent.  
Artifacts define what will actually settle.

## 6. Principle Five — Machine-Readable Safety Over Human-Only Review

Traditional contract ecosystems assume that a human expert reads source code and manually reasons about safety.

That assumption breaks down when:

- contracts are numerous,
- interactions are continuous,
- callers are autonomous,
- and composition happens at machine speed.

Tolang therefore treats machine-readable safety as a first-class design objective.

Effects, bounds, gas ceilings, capability requirements, composability flags, verification semantics, and payment conditions should be explicit enough for software systems to inspect before execution.

This does not eliminate human audit.  
It makes human audit scalable by making semantics inspectable.

## 7. Principle Six — Safe Composition Over Unrestricted Composability

Unrestricted composability is often celebrated in blockchain design, but in practice it can turn local complexity into systemic fragility.

Tolang should not encourage composition blindly.  
It should encourage composition that is:

- authority-aware,
- effect-aware,
- cost-aware,
- and failure-aware.

A contract that hides external calls, mutates more state than expected, or embeds unclear privilege assumptions is not safely composable, even if it is technically callable.

In Tolang, safe composition should always take priority over unrestricted composition.

## 8. Principle Seven — Capability-Scoped Delegation Over Raw Key Power

Delegation will be central to agent-native systems. But delegation without scope becomes indistinguishable from unsafe key reuse.

Tolang should prefer delegation models that are:

- explicit,
- minimal,
- time-bounded where appropriate,
- non-replayable where required,
- and visible in metadata or interface policy.

The purpose of delegation in Tolang is not to copy raw authority.  
It is to create controlled authority surfaces suitable for automation.

## 9. Principle Eight — Security as a Language Property

A runtime can block certain dangerous operations, but a language has a deeper responsibility: it decides what is easy to express, what is hard to express, and what must be declared.

Tolang should therefore internalize security not only through runtime checks, but through:

- syntax design,
- semantic analysis,
- lowering rules,
- artifact schemas,
- package constraints,
- and interface standards.

The strongest security property is one that becomes difficult to violate by construction.

## 10. Practical Consequences

This doctrine implies several concrete engineering priorities.

### 10.1 Prefer declarative metadata
Effects, bounds, gas budgets, and policy annotations should remain core compiler outputs.

### 10.2 Treat bytecode-bound metadata as a primary trust surface
Verification and tooling should anchor to artifacts, not only to source text.

### 10.3 Restrict hidden nondeterminism
Language and runtime evolution should continue to reject sources of nondeterministic settlement.

### 10.4 Make authority explicit
Capability, delegation, payment, and verifiability semantics should remain visible at the interface layer.

### 10.5 Build for automated inspection
The ecosystem should assume that agents, not only humans, are first-class consumers of security-relevant metadata.

## 11. What This Doctrine Rejects

Tolang does not optimize for:

- arbitrary dynamic power with weak metering,
- hidden authority paths,
- undocumented execution side effects,
- opaque AI inside consensus,
- or “trust me” interface semantics.

These may be tolerable in experimental scripting environments. They are not acceptable foundations for agent-native settlement.

## 12. Summary

Tolang is not trying to be the loosest language, the most magical language, or the easiest language to misuse.

It aims to be the language that autonomous systems can trust enough to automate against.

That requires a security philosophy with a clear center:

- deterministic settlement,
- explicit authority,
- bounded execution,
- artifact-level trust,
- machine-readable safety,
- and safe composition.

These are not just implementation choices.  
They are the reason Tolang can become a serious language for the Agent Economy.
