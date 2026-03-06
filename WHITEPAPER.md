# Tolang Whitepaper
## The Agent-Native Smart Contract Language for TOS Network

**Version:** Draft v0.1  
**Project:** TOS Network / Tolang  
**Document Type:** Whitepaper  
**Status:** Public Draft  

---

## Abstract

Tolang is a statically typed, Solidity-inspired smart contract language designed to become the primary contract language of the Agent Economy on TOS Network. Unlike earlier generations of contract languages that treated the function call as the dominant abstraction, Tolang progressively elevates identity, capability, task, delegation, payment, verifiability, and trust into language-visible and machine-readable semantics.

Tolang already operates as a real source-to-settlement system. `.tol` source files are compiled into `.toc` artifacts containing bytecode, ABI JSON, source hash, bytecode hash, storage and runtime metadata, and machine-readable policy fields. These artifacts are assembled into deterministic `.tor` deployment packages for execution on TOS Network. The purpose of Tolang is not merely to make contracts deployable on a new chain, but to make autonomous economic coordination analyzable, verifiable, budgetable, and secure enough for software agents to use at scale.

This whitepaper defines the vision, architecture, semantics, interface model, security doctrine, and roadmap of Tolang. Its central claim is straightforward: Tolang should not be understood as “TOS Solidity.” It should be understood as the first serious attempt to build a smart contract language whose long-term center of gravity is **agency**, not merely application logic.

---

## Keywords

Tolang, TOS Network, agent economy, smart contract language, agent-native, capability, task, oracle, delegation, reputation, ACI, ABI, deterministic execution, machine-readable policy, secure composition

---

## Table of Contents

1. [Vision](#1-vision)  
2. [Present Architecture](#2-present-architecture)  
3. [Agent-Native Semantics](#3-agent-native-semantics)  
4. [ACI — Agent Coordination Interface](#4-aci--agent-coordination-interface)  
5. [Security Doctrine](#5-security-doctrine)  
6. [Roadmap](#6-roadmap)  
7. [Feature Maturity Model](#7-feature-maturity-model)  
8. [Positioning](#8-positioning)  
9. [Conclusion](#9-conclusion)  
10. [Glossary](#10-glossary)

---

# 1. Vision

## 1.1 The shift from applications to agents

The first generation of blockchain languages was built for a world in which the main actor was the human user. A developer authored a contract, a wallet signed a transaction, and a front end organized the experience. The contract exposed callable functions, and the application assembled those functions into products.

That model remains important, but it is no longer sufficient.

The next decade will increasingly be defined by **autonomous economic agents**: software entities that can discover services, evaluate trust, request work, accept obligations, consume data, post collateral, invoke models, negotiate terms, verify outputs, and settle value continuously. In such a world, the traditional function-centric smart contract model becomes too narrow. What matters is not only what a function does when called, but also:

- who is authorized to call it,
- what capabilities are required,
- what state may change,
- what external systems may be touched,
- what costs may be incurred,
- whether delegation is allowed,
- whether outputs are verifiable,
- and how trust is built or lost over time.

The language of this world cannot merely describe computation. It must describe **economic agency**.

## 1.2 Why Tolang exists

Tolang exists to serve that transition.

It is designed as the smart contract language of **TOS Network**, not simply to host more applications, but to become the policy, coordination, and settlement language of the Agent Economy. Its long-term role is to let autonomous agents express capability, accept tasks, manage delegation, escrow value, prove outcomes, emit machine-readable trust signals, and settle under deterministic rules.

In this model, the blockchain is not merely a programmable ledger. It becomes the **economic coordination layer for machine actors**. Tolang is the language through which those actors participate.

## 1.3 What Tolang is not

Tolang is not intended to be:

- a cosmetic syntax wrapper over an older contract model,
- a purely experimental “AI x crypto” scripting system,
- a language that sacrifices determinism in the name of intelligence,
- or a compatibility veneer whose only value is porting legacy EVM patterns.

Tolang may be Solidity-inspired on the surface, but its strategic objective is different. Solidity optimized for human-authored decentralized applications. Tolang must optimize for **machine-readable authority, machine-readable effects, and safe machine-driven coordination**.

## 1.4 Core thesis

The core thesis of this whitepaper is:

> The dominant abstraction of the next blockchain decade will not be the application. It will be the autonomous economic actor. Therefore, the dominant smart contract language of that era must make agency, capability, task, and verifiability first-class concepts.

That is the role Tolang is being built to serve.

---

# 2. Present Architecture

## 2.1 Overview

Tolang already stands on a concrete source-to-runtime architecture. It is not merely a language proposal. It is a compiler, an artifact format, a package system, and an execution path integrated with TOS Network.

At a high level, the current architecture is:

```text
.tol source
  -> lexer
  -> parser
  -> semantic analysis
  -> lowering
  -> IR / code generation
  -> .toc artifacts
  -> .tor package
  -> deployment on TOS Network
  -> execution in the TOS Lua VM path
```

This architecture is strategically important because it means Tolang already controls the full lifecycle of a contract: source semantics, compile-time policy extraction, artifact metadata, deterministic packaging, and runtime execution.

## 2.2 Source model

Tolang source files use the `.tol` extension. The language supports a contract-oriented programming model with familiar concepts for developers coming from Solidity-like ecosystems, including contracts, interfaces, inheritance, mappings, structs, typed functions, visibility, mutability, imports, and package organization.

This choice is intentional. Adoption friction matters. A language does not become foundational merely by being radically different; it becomes foundational when it can offer a better semantic model without forcing developers to relearn everything at once.

Tolang therefore begins with a familiar shell, while shifting the deeper semantics toward agent-native coordination.

## 2.3 Compiler pipeline

The current compiler path can be described in the following stages.

### 2.3.1 Lexing and parsing

The lexer tokenizes `.tol` source into a language-specific token stream. The parser constructs an AST representing contract declarations, interfaces, types, functions, annotations, and top-level metadata.

### 2.3.2 Semantic analysis

The semantic analysis stage validates names, symbols, imports, types, method signatures, inheritance behavior, interface compatibility, and contract-level correctness. This stage is also where language-level policy constructs begin to matter: capabilities, manifests, task and oracle declarations, and function annotations become semantically meaningful rather than raw syntax.

### 2.3.3 Lowering

The lowering stage transforms the high-level AST into a normalized program representation suitable for code generation and artifact production. This layer is especially important for Tolang because it is where language-level semantic intent can be preserved in a machine-usable form.

### 2.3.4 IR and code generation

The backend compiles normalized program structures into register-based bytecode suitable for the TOS Lua execution environment. This bytecode is not intended merely as an opaque runtime blob. It is paired with metadata that allows tooling and agents to reason about the code before execution.

## 2.4 Artifacts: `.toc`

A `.toc` file is more than a bytecode container. It is the compiled unit of trust.

A Tolang artifact should be understood as containing, or being designed to contain, the following classes of information:

- executable bytecode,
- ABI JSON,
- source hash,
- bytecode hash,
- storage layout information,
- stack and runtime metadata,
- flags related to boundedness or execution behavior,
- and machine-readable policy fields emitted from source-level declarations and annotations.

This artifact-first approach is one of Tolang’s most important architectural ideas. It allows the economic meaning of a contract to be derived not only from source code but from compiled, hash-bound metadata that agents and tools can consume directly.

## 2.5 Packages: `.tor`

Multiple compiled outputs are assembled into deterministic `.tor` deployment packages. Deterministic packaging matters for three reasons:

1. it makes artifact identity stable,
2. it allows independent verification of build outputs,
3. and it reduces ambiguity between source, compiled code, and deployed state.

The package model is therefore part of Tolang’s security and trust design, not just a build-system convenience.

## 2.6 Init/runtime split

Tolang distinguishes deployment-time initialization from runtime logic. Constructor or init paths are compiled separately from persistent callable runtime logic. The runtime package represents the long-lived contract logic stored and invoked on-chain.

This split improves clarity and safety:

- one-time setup is separated from perpetual callable behavior,
- deployment semantics become easier to reason about,
- and runtime artifacts remain leaner and more stable.

## 2.7 Execution on TOS Network

Tolang is designed to execute on TOS Network through the chain’s embedded Lua-based contract execution path. The design goal is not arbitrary script freedom, but deterministic, metered, and hardened execution.

This is critical. Tolang’s target is not “maximum language flexibility.” Its target is **trustworthy programmability under economic constraints**.

## 2.8 Present architectural identity

Today, Tolang can be accurately described as:

- a Solidity-inspired contract language,
- with a real compiler pipeline,
- with deterministic artifacts and package outputs,
- with growing machine-readable policy metadata,
- and with explicit movement toward agent-native semantics.

That already makes it more than a conventional chain language. But to become the first true language of the Agent Economy, Tolang must advance from contract-oriented machinery to agent-oriented formalism.

---

# 3. Agent-Native Semantics

## 3.1 Why semantics matter more than keywords

Adding a few agent-related keywords is not enough to create an agent-native language. A language becomes agent-native only when its semantics, artifacts, interfaces, and safety model all begin to reflect the needs of autonomous economic actors.

Tolang’s next historical step is not the invention of more syntax. It is the consistent elevation of agency into the language’s semantic center.

## 3.2 First-class concepts

The emerging semantic core of Tolang revolves around the following concepts.

### 3.2.1 Agent

An **agent** is more than an address. It is an economic actor with identity, status, and policy-relevant properties. A future-complete Tolang model treats an agent as something that may:

- hold stake,
- possess or be granted capabilities,
- accept delegated authority,
- perform tasks,
- accumulate reputation,
- and participate in settlement logic.

This is a major departure from traditional contract languages, where economic actors are usually modeled as raw addresses with application-specific wrappers.

### 3.2.2 Capability

A **capability** is an explicit representation of authority. Instead of scattering permission checks across handwritten business logic, capability-aware programming makes authority visible and machine-readable.

In Tolang, capability should become the preferred way to express questions such as:

- who may invoke this operation,
- what class of action is being authorized,
- whether an action may be delegated,
- and what downstream systems may be touched.

This moves power from implicit logic into inspectable policy.

### 3.2.3 Task

A **task** is a first-class economic unit.

Traditional contract systems revolve around functions. But agents do not think naturally in isolated functions. They think in goals, obligations, deadlines, acceptable outputs, payment terms, and completion rules.

A task-centric model allows Tolang programs to express:

- requested work,
- preconditions,
- lifecycle transitions,
- verifiable completion conditions,
- payment or escrow release,
- and failure handling.

This is one of the clearest conceptual lines separating Tolang from earlier contract languages.

### 3.2.4 Oracle

An **oracle** in Tolang should not be treated merely as an external service hook. It should be represented through explicit language and interface semantics, so that contracts and agents can reason about when facts are pending, finalized, trusted, or challengeable.

The key principle is that external fact dependency must become visible and bounded.

### 3.2.5 Manifest

A **manifest** is a contract-level declaration of machine-readable identity and policy metadata. It is one of the strongest ways to make a contract discoverable and understandable to other agents without forcing full source interpretation.

Manifests should evolve into a standard place for:

- service identity,
- protocol role,
- compatibility declarations,
- expected interaction modes,
- and future ecosystem discovery metadata.

### 3.2.6 Delegation

Delegation is central to agent economies. Users delegate to assistant agents; assistant agents may invoke worker agents; service agents may need narrowly scoped rights for downstream settlement.

A language that does not model delegation will remain trapped in a human-wallet worldview.

Tolang should therefore treat delegation as more than a boolean flag. It should evolve toward semantics that capture:

- delegated authority scope,
- time bounds,
- single-use or replay-safe behavior,
- revocation,
- and interaction with capabilities.

### 3.2.7 Verifiability

Agents cannot scale trust on narrative alone. Results must be verifiable.

Verifiability in Tolang should cover multiple layers:

- machine-readable interface promises,
- attestations and proof-carrying outputs,
- compiler-checked effects and bounds,
- and explicit result semantics for task completion.

In the Agent Economy, a “successful call” is not enough. The system must know what kind of success was promised and how that success can be trusted.

## 3.3 Semantic direction of Tolang

Taken together, these concepts define Tolang’s long-term semantic direction:

- from address to agent,
- from function to task,
- from implicit authorization to capability,
- from hidden logic to manifest and policy metadata,
- from source-only trust to artifact-bound trust,
- and from execution-only semantics to verifiable settlement semantics.

That is the essence of an agent-native language.

---

# 4. ACI — Agent Coordination Interface

## 4.1 Why ABI is no longer enough

Traditional smart contract ABIs were designed for human-facing applications. They tell a wallet or front end how to encode a call and decode a result. That is useful, but insufficient for autonomous coordination.

An agent deciding whether to interact with a contract needs more than parameter types. It needs to know:

- what authority is required,
- what state may be touched,
- what external calls may occur,
- what gas ceiling applies,
- whether the function is safely composable,
- whether value transfer or escrow is involved,
- and whether the result should be interpreted as a task status, a proof-bearing result, or a simple return value.

For this reason, Tolang should define a formal standard called **ACI: Agent Coordination Interface**.

## 4.2 Definition

ACI is the machine-readable interface standard through which agents discover, evaluate, negotiate with, and safely compose Tolang services on TOS Network.

ACI extends the notion of ABI from **call encoding** to **economic coordination semantics**.

## 4.3 Design goals

ACI should satisfy six design goals.

### 4.3.1 Discoverability

An agent should be able to query a contract or package and determine what kind of service it provides.

### 4.3.2 Authorization clarity

An agent should know what capabilities, delegation scopes, or authority assumptions apply before calling.

### 4.3.3 Effect transparency

An agent should be able to inspect declared reads, writes, events, and external call surfaces before execution.

### 4.3.4 Resource predictability

An agent should be able to estimate whether a call fits within a gas or execution budget.

### 4.3.5 Verifiability

An agent should know whether outputs are ordinary values, proof-bearing values, or part of a task lifecycle.

### 4.3.6 Safe composition

An agent should be able to tell whether a function is generally composable, non-composable, or subject to additional operational constraints.

## 4.4 Proposed ACI fields

A first formal version of ACI should standardize the following categories of fields.

### 4.4.1 Core callable schema

- selector / name
- input types
- output types
- mutability
- payable behavior

### 4.4.2 Policy schema

- required capability
- delegated-call acceptance
- verifiable-result flag
- payment semantics
- task endpoint flag
- oracle fulfillment or query role

### 4.4.3 Effects schema

- declared storage reads
- declared storage writes
- emitted events
- external call references
- capability-sensitive side effects

### 4.4.4 Bounds schema

- call count bounds
- storage access bounds
- external reference bounds
- path-sensitive or conservative upper bounds where applicable

### 4.4.5 Gas schema

- gas upper bound
- gas model version
- boundedness status
- warnings for dynamic or conservative estimates

### 4.4.6 Identity and discovery schema

- manifest data
- service category
- protocol compatibility tags
- package / artifact hash linkage
- version fields

### 4.4.7 Verification schema

- expected result form
- acceptable proof or attestation hooks
- challengeability metadata
- trust surface disclosure

## 4.5 Why ACI is essential

Without ACI, every serious agent ecosystem ends up rebuilding its own side-channel metadata, handwritten integration logic, or natural-language documentation layer. That makes composition brittle and ecosystem growth slow.

With ACI, Tolang can define a common machine protocol for:

- agent marketplaces,
- service registries,
- autonomous wallets,
- preflight verifiers,
- policy engines,
- audit tools,
- and reputation systems.

In practical terms, ACI is how Tolang stops being “a language with useful metadata” and becomes a platform language for machine coordination.

## 4.6 Immediate next step

A formal `ACI_SPEC.md` should be written and versioned. It should define:

- schema,
- required vs optional fields,
- serialization rules,
- compatibility policy,
- and extension rules.

That document will likely become one of the most strategically important pieces of the entire Tolang project.

---

# 5. Security Doctrine

## 5.1 Why Tolang needs a doctrine, not just safeguards

All serious smart contract languages eventually get judged not only by their expressiveness, but by what categories of failure they make easy.

This is even more true in an agent-native world. Agents can operate at machine speed, compose services automatically, and amplify mistakes far beyond the pace of human review. Therefore, Tolang cannot rely on scattered implementation notes or runtime guardrails alone. It needs an explicit security doctrine.

That doctrine should explain the philosophical rules of the language: what kinds of power are allowed, what must remain visible, what must remain bounded, and what kinds of risk are unacceptable for machine-driven finance and coordination.

## 5.2 Principle 1 — Determinism over opaque intelligence

Tolang must preserve deterministic settlement.

AI, inference systems, off-chain data providers, and human-generated attestations may all participate in agent workflows. But once economic consequences reach consensus, the relevant semantics must be deterministic, analyzable, and explicitly modeled.

Tolang should therefore favor:

- verifiable off-chain computation,
- explicit oracle/result state,
- proof-bearing completion conditions,
- and bounded settlement rules.

It should reject the temptation to hide non-deterministic intelligence inside consensus execution.

## 5.3 Principle 2 — Explicit authority over hidden power

Authority must be visible.

Capabilities, delegation, payment assumptions, and verification requirements should not be buried in arbitrary source logic whenever they can be made explicit in language or artifact metadata.

This is one of Tolang’s strongest potential differentiators. Machine actors need inspectable power structures.

## 5.4 Principle 3 — Bounded execution over expressive chaos

For the Agent Economy, predictability matters more than unrestricted expressiveness.

Tolang should prefer:

- bounded or conservatively modeled loops,
- analyzable storage effects,
- package limits,
- deterministic runtime behavior,
- and gas estimates that err on the side of safety.

An agent cannot safely automate what it cannot budget.

## 5.5 Principle 4 — Artifact trust over source-only trust

The primary unit of machine trust in Tolang should be the compiled artifact, not merely the source file.

That means:

- bytecode hashes matter,
- ABI and policy metadata should be bound to artifact identity,
- and verification should target the source-to-artifact-to-deployment chain.

This is a crucial philosophical shift. Human developers may read source. Autonomous systems must be able to trust compiled facts.

## 5.6 Principle 5 — Machine-readable safety over human-only review

A language for agents must make safety machine-readable.

Effects, bounds, gas ceilings, capability requirements, delegation acceptance, and result semantics should be exposed to automated analysis and preflight checks. If safety depends entirely on a human reading source code line by line, the language is not truly fit for machine coordination.

## 5.7 Principle 6 — Safe composition over unrestricted composability

The blockchain world often celebrates composability as an unconditional good. In practice, unconstrained composition is one of the main sources of hidden risk.

Tolang should promote a different doctrine:

- composition is valuable,
- but only when authority is visible,
- side effects are declared,
- costs are bounded,
- and failure surfaces are explicit.

This means Tolang should favor **safe composition**, not blind composition.

## 5.8 Consequence for ecosystem design

If Tolang adopts this doctrine clearly, it gains a distinctive identity:

- not the most permissive contract language,
- not the most experimental AI language,
- but the most serious language for **secure autonomous economic coordination**.

That is exactly the position it should seek.

---

# 6. Roadmap

## 6.1 Roadmap philosophy

The Tolang roadmap must distinguish between three categories:

- what is already implemented,
- what exists in compiler or design form but depends on runtime / protocol integration,
- and what is still strategic direction.

This distinction is important for credibility. Tolang should present itself as an ambitious language, but also as a precise one.

## 6.2 Phase I — Core stabilization

### Objectives

- stabilize compiler invariants,
- harden `.toc` and `.tor` specifications,
- freeze artifact semantics enough for tooling,
- document deterministic build and verification flow,
- and improve diagnostics and developer ergonomics.

### Deliverables

- stable artifact schema,
- package format documentation,
- versioned compiler output behavior,
- verification tooling improvements,
- and stronger test coverage for compilation and packaging.

### Strategic outcome

Tolang becomes indisputably real as a deployable contract language, not merely a concept.

## 6.3 Phase II — Formal agent-native language surface

### Objectives

- formalize `agent`, `task`, `oracle`, `capability`, and `manifest` semantics,
- formalize annotations such as `@requires`, `@delegated`, `@pay`, and `@verifiable`,
- define source-level rules and edge cases,
- and publish language-level examples and guidance.

### Deliverables

- versioned language specification,
- semantics reference,
- example contracts,
- migration guidance for Solidity-style developers,
- and a clear statement of which features are compiler-complete versus runtime-dependent.

### Strategic outcome

Tolang stops looking like “a language that may become agent-native someday” and is recognized as a language already organized around agent primitives.

## 6.4 Phase III — ACI standardization

### Objectives

- turn existing metadata concepts into a formal interface standard,
- define required and optional fields,
- formalize effect, bound, gas, policy, and manifest schema,
- and make ACI versioned and ecosystem-targetable.

### Deliverables

- `ACI_SPEC.md`,
- schema examples,
- serialization rules,
- compatibility policy,
- and ACI-aware inspection tooling.

### Strategic outcome

Tolang gains a machine-native coordination standard that other tools, agents, and protocols can build against.

## 6.5 Phase IV — Runtime and protocol integration

### Objectives

- tighten integration with TOS Network runtime primitives,
- standardize agent registry interfaces,
- standardize capability and delegation registry behavior,
- provide canonical task and oracle handling paths,
- and align contract semantics with protocol-level accountability features.

### Deliverables

- standard protocol packages,
- registry interface references,
- canonical task lifecycle behavior,
- oracle settlement patterns,
- and runtime-backed capability enforcement where applicable.

### Strategic outcome

Tolang becomes the native programming language of the TOS agent infrastructure, not just a frontend for bytecode emission.

## 6.6 Phase V — Agent economy tooling

### Objectives

- build agent-facing SDKs,
- build ACI-aware discovery and marketplace tools,
- build preflight risk analyzers,
- build artifact policy validators,
- and build audit tools that reason about machine-readable policy rather than source alone.

### Deliverables

- ecosystem SDKs,
- contract and service registries,
- policy inspection tools,
- ACI-aware explorers,
- and integration libraries for autonomous agents.

### Strategic outcome

Tolang becomes usable not only by developers but directly by machine systems as a coordination substrate.

## 6.7 Phase VI — Full agent economy layer

### Objectives

- make Tolang the default service language for machine actors on TOS Network,
- support open agent marketplaces,
- support trust and reputation portability,
- enable delegated and verifiable task execution at scale,
- and establish Tolang as the canonical language of machine commerce.

### Strategic outcome

At this stage, Tolang is no longer best described as just a smart contract language. It becomes the policy and settlement language of an open agent economy.

---

# 7. Feature Maturity Model

## 7.1 Why maturity labeling matters

A project at Tolang’s stage needs disciplined communication. One of the easiest ways to lose trust is to blur the line between implemented features, compiler-level groundwork, runtime-dependent semantics, and strategic plans.

Therefore, Tolang documentation should explicitly classify features into maturity tiers.

## 7.2 Proposed maturity tiers

### Tier A — Implemented Today

Features that are available in the current compiler and execution flow, stable enough to be used and documented as present reality.

Examples may include:

- source-to-artifact compilation pipeline,
- deterministic packaging,
- ABI generation,
- init/runtime split,
- inspection and verification tooling,
- and current machine-readable metadata already emitted by compiler outputs.

### Tier B — Compiler-complete / runtime-dependent

Features whose syntax, semantic checks, or artifact emission are already present, but whose full economic meaning depends on runtime support, host functions, registry contracts, or protocol-level semantics.

Examples may include:

- full agent registry-backed behavior,
- capability enforcement that depends on protocol integration,
- task lifecycle atomicity with runtime support,
- oracle settlement hooks,
- and advanced delegation semantics.

### Tier C — Proposed / planned

Features whose direction is clear but whose stable semantics, implementation, or ecosystem support are still in development.

Examples may include:

- formal ACI v1,
- proof-carrying task completion standards,
- richer dispute and slashing semantics,
- standardized agent marketplaces,
- and deep AI-verification integration patterns.

## 7.3 Recommended repository practice

The Tolang repository should include a machine-readable or table-based feature matrix with columns such as:

| Feature | Parser | Sema | Lowering | ABI/Artifact | Runtime | Status |
|---|---:|---:|---:|---:|---:|---|
| capability | ✅ | ✅ | ✅ | ✅ | partial | Tier B |
| task | ✅ | ✅ | ✅ | partial | partial | Tier B |
| oracle | ✅ | ✅ | ✅ | partial | partial | Tier B |
| manifest | ✅ | ✅ | ✅ | ✅ | n/a | Tier A |
| ACI | partial | partial | partial | partial | n/a | Tier C |

This single practice would materially improve clarity for developers, partners, and auditors.

---

# 8. Positioning

## 8.1 Against the old category system

Tolang should not position itself as a better syntax for existing decentralized applications. That would understate its real ambition.

The more accurate positioning is:

> Tolang is the contract language of the Agent Economy on TOS Network.

This framing matters because it changes what people evaluate.

If Tolang is judged as “another smart contract language,” the conversation becomes about syntax familiarity and VM comparisons.

If Tolang is judged as “the first serious language for machine-coordinated economic actors,” the conversation becomes about:

- authority representation,
- task semantics,
- machine-readable guarantees,
- safe composition,
- and economic trust design.

That is the field in which Tolang can become historically important.

## 8.2 Relative differentiation

Tolang’s likely long-term differentiation is not just that it is:

- typed,
- deterministic,
- and deployable on a new chain.

Its deeper differentiation is that it aims to combine:

- Solidity-inspired familiarity,
- artifact-bound machine-readable policy,
- task and capability semantics,
- deterministic execution,
- and a path toward a formal coordination interface for autonomous agents.

That combination is rare.

## 8.3 The strongest possible claim

The strongest defensible claim Tolang can make is not:

> “We support agents.”

Many projects can say that.

The stronger claim is:

> “We are building the first contract language whose core abstractions, metadata model, and security philosophy are designed for autonomous economic coordination rather than only human-driven application logic.”

That is the right level of ambition.

---

# 9. Conclusion

Tolang begins from a real implementation base: a compiler pipeline, deterministic artifacts, deployable package formats, and an execution path on TOS Network. But its significance will not be measured merely by whether it can deploy contracts.

Its significance will be measured by whether it can define a new category of blockchain language.

The world does not only need more smart contracts. It needs a language in which autonomous actors can safely hold authority, accept obligations, coordinate tasks, verify outcomes, budget execution, and build trust.

Tolang is being built for that purpose.

If Solidity was a language of programmable applications, Tolang seeks to become a language of **programmable agency**.

If earlier contract systems were primarily designed for users, Tolang is being shaped for users **and** the software actors that will increasingly act on their behalf.

If earlier contract ecosystems relied heavily on human interpretation, Tolang aims toward machine-readable effects, machine-readable policy, and machine-readable trust surfaces.

That is why Tolang matters.

It is not simply a language for TOS Network.
It is the language through which TOS Network can become an economy of agents.

---

# 10. Glossary

**ACI (Agent Coordination Interface)**  
A proposed machine-readable interface standard extending ABI into an economic coordination schema for autonomous agents.

**Agent**  
An autonomous economic actor represented in contracts and protocol logic as more than a raw address.

**Artifact**  
A compiled output unit, such as `.toc`, containing executable code and machine-readable metadata.

**Capability**  
An explicit representation of authority or permission that can be checked, delegated, and exposed through machine-readable policy.

**Delegation**  
The act of granting scoped authority from one actor to another, ideally with explicit limits, revocation rules, and anti-replay semantics.

**Manifest**  
Top-level machine-readable metadata describing service identity, protocol role, or contract policy.

**Oracle**  
A mechanism through which off-chain facts or externally determined results are introduced into contract logic under explicit and bounded semantics.

**Task**  
A first-class economic unit representing requested work, preconditions, lifecycle, acceptable results, and settlement behavior.

**Tolang**  
The smart contract language of TOS Network, designed to evolve into the primary language of the Agent Economy.

**TOS Network**  
The settlement and execution environment in which Tolang contracts are deployed and run.

---

## Suggested companion documents

To complete the Tolang documentation stack, the following companion documents are recommended:

- `docs/ACI_SPEC.md`
- `docs/SECURITY_DOCTRINE.md`
- `docs/FEATURE_MATURITY_MATRIX.md`
- `docs/LANGUAGE_SPEC.md`
- `docs/ARTIFACT_AND_PACKAGE_SPEC.md`
- `docs/AGENT_NATIVE_SEMANTICS.md`

These documents would turn the whitepaper into a full language and ecosystem specification suite.
