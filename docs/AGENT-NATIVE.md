# Agent-Native TOL: Gap Analysis

## Context

[THESIS.md](../gtos/THESIS.md) frames TOS Network as the economic settlement layer for an
AI-driven agent economy. The six pillars it identifies — agent-to-agent hiring, machine-to-machine
micropayments, collateralized trust, identity and reputation, AI-mediated UX, and governance
coordination — all ultimately require programmable contracts. TOL is that programming language.

`docs/AGENT_PROTOCOL_DRAFT.tol` shows we can already express the agent economy protocols
(`IAgentRegistry`, `IReputationHub`, `ITaskEscrow`, `IDisputeResolver`, `IPredictionMarket`,
`IRewardVault`) in valid TOL. But this is **library-level** agent support: agent concepts are
represented as plain `address` values and raw `u256` identifiers, with semantics enforced only at
runtime through explicit require-guards hand-written by the developer.

The gap between "a language you can use to write agent contracts" and a genuinely **Agent-Native**
language is the same gap that existed between a general scripting language and Solidity before DeFi:
Solidity elevated `msg.sender`, `payable`, `transfer()`, and `msg.value` to first-class primitives.
An Agent-Native TOL must elevate **identity, task, capability, payment, and trust** to the same
level.

---

## What Is Missing

### 1. `agent` — Native Data Type

**Current state:** agent identity is a bare `address`. All registry lookups, stake checks, and
reputation reads are hand-written calls to external interfaces.

```tol
// today
address worker = msg.sender;
require(IAgentRegistry(registry).isRegistered(worker), "NotRegistered");
u256 stake = IAgentRegistry(registry).stakeOf(worker);
```

**Agent-Native:** `agent` is a first-class type — a superset of `address` that the compiler
integrates with the registry automatically.

```tol
agent worker = agent(msg.sender);   // compiler inserts registry verification
worker.stake        // → u256
worker.reputation   // → i256
worker.is_active    // → bool
worker.capabilities // → u256 bitmap
```

The compiler generates registry query code at every property access. Developers never manually call
`IAgentRegistry`. This is the single highest-leverage addition because every other agent primitive
builds on it.

---

### 2. `manifest {}` Block — Agent Self-Description

**Current state:** there is no machine-readable way for a deployed contract to declare what it is,
what it can do, or what it costs. Capability discovery is entirely off-chain and informal.

**Agent-Native:** a `manifest` block inside a contract compiles into the `.toc` ABI file, making
the contract automatically discoverable by off-chain agent marketplaces and indexers.

```tol
contract PriceOracle {
  manifest {
    version:        "1.0.0";
    capabilities:   [DataFetcher, PriceOracle];
    sla_uptime:     9900;           // bps, 99.00 %
    price_per_call: 1_000_000;      // micro-TOS
    spec:           "ipfs://Qm..."; // JSON-LD description
  }
  // ...
}
```

This directly addresses the **identity and reputation** pillar from THESIS.md. Without it, agent
discovery relies on off-chain registries and social trust — precisely the centralized dependency
that TOS Network is designed to eliminate.

---

### 3. `capability` Type + `@requires` Annotation — Static Permission Checks

**Current state:** access control is enforced via hand-written `require` statements with no
compiler assistance. Nothing prevents a developer from forgetting a check.

**Agent-Native:** capabilities are declared as named types; the `@requires` annotation on a
function causes the compiler to insert the check automatically.

```tol
capability DataFetcher;
capability Arbitrator;

@requires(caller: Arbitrator)
function rule(u256 dispute_id, address winner, u16 slash_bps, string reason) public { ... }
```

Compiler output: `require(registry.hasCapability(msg.sender, Arbitrator), "CapabilityDenied")`.

The annotation is also recorded in the `.toc` ABI so that off-chain orchestrators can reason about
which agents are eligible to call which functions before sending a transaction.

---

### 4. `@pay(amount)` Annotation — Micropayment Primitive

**Current state:** per-call payments require manual `require(msg.value >= ...)` guards and explicit
transfer logic.

**Agent-Native:** the `@pay` annotation makes per-call pricing a one-liner, directly enforced by
the compiler.

```tol
@pay(1_000_000)   // 1 micro-TOS per call, compiler-enforced
function getPrice(bytes32 pair) public view returns (u256 price) { ... }
```

Compiler generates: `require(msg.value >= 1_000_000, "InsufficientPayment")` plus any configured
fee-routing logic. This is the language-level primitive for the **machine-to-machine micropayments**
pillar. Without it, every pay-per-call service requires bespoke boilerplate.

---

### 5. `task<T>` Type — Lifecycle-Aware Task Primitive

**Current state:** `TaskEscrow` tracks task state in raw `mapping(u256 => u8)` with numeric status
codes. The state machine is invisible to the compiler — it cannot detect illegal state transitions.

**Agent-Native:** `task<T>` is a parameterized type with an explicit lifecycle. The compiler
enforces that transitions are valid.

```tol
task<bytes32> t = TaskEscrow.post(spec_hash, reward, deadline);
t.id       // → u256
t.status   // → TaskStatus  (Open | Accepted | Submitted | Approved | Disputed | Cancelled)
t.worker   // → agent
t.fulfill(result_hash);   // compiler error if status != Accepted
t.dispute(evidence_hash); // compiler error if status != Submitted
```

Static state-machine checking catches a class of escrow bugs (double-approval, out-of-order
submission) at compile time rather than at runtime under real value.

---

### 6. `escrow` / `slash` / `release` — Built-in Trust Primitives

**Current state:** collateral management is verbose, pattern-repeated code that appears identically
in `TaskEscrow`, `DisputeResolver`, and any future staking contract.

**Agent-Native:** collateral operations are language keywords with defined semantics.

```tol
escrow(worker, reward);       // lock funds assigned to worker
slash(worker, slash_amount);  // penalize; compiler routes proceeds per policy
release(worker);              // unlock and transfer
```

This addresses the **collateralized trust** pillar directly. The keywords desugar to the existing
transfer/balance mechanics but make the intent explicit, auditable at a glance, and impossible to
accidentally miswire.

---

### 7. `oracle<T>` Type — Prediction Market Primitive

**Current state:** oracle resolution in `PredictionMarket` is manual state mutation with no
language-level guarantee of write-once semantics.

**Agent-Native:** `oracle<T>` is a write-once, read-many type.

```tol
oracle<u8> winning_outcome;

@authorized(resolver: admin)
function resolve(u8 outcome) public {
  winning_outcome.fulfill(outcome);   // compile error if called twice
}

if winning_outcome.is_set {
  payout(winning_outcome.value);
}
```

The compiler emits a storage guard ensuring `fulfill` can only be called once. This eliminates an
entire category of oracle manipulation bugs.

---

### 8. `@verifiable` Annotation — ZK Readiness

**Current state:** TOL has no concept of zero-knowledge proofs. THESIS.md lists
**zero-knowledge payments** as a required enabling technology.

**Agent-Native:** `@verifiable` marks functions whose outputs can be proven off-chain and verified
on-chain without re-execution. In the current compiler it is recorded as metadata in the `.toc` ABI
(a no-op at execution time) but reserves the semantic space for a future ZK backend.

```tol
@verifiable
function computeScore(address agent) public view returns (i256 score) { ... }
```

Establishing the annotation now means existing contracts automatically become ZK-compatible when
the proof backend is added, without source changes.

---

## Prioritized Roadmap

| Priority | Feature | THESIS.md Pillar |
|----------|---------|-----------------|
| P0 | `agent` native type (address superset + registry integration) | Identity & reputation |
| P0 | `manifest {}` block + `.toc` ABI extension | Identity & discoverability |
| P1 | `capability` type + `@requires` annotation | Identity & trust |
| P1 | `@pay(amount)` annotation | Machine-to-machine micropayments |
| P2 | `task<T>` type with compiler-enforced state machine | Agent-to-agent hiring |
| P2 | `escrow` / `slash` / `release` keywords | Collateralized trust |
| P3 | `oracle<T>` write-once type | Prediction markets |
| P3 | `@verifiable` annotation (ZK placeholder) | Zero-knowledge payments |

---

## Design Principle

Each feature above follows the same pattern: take something that today requires **hand-written,
error-prone, auditor-unfriendly boilerplate** and elevate it to a **compiler-checked, ABI-visible,
semantically precise** language construct. The goal is that an agent contract written in
Agent-Native TOL should be self-describing, statically safe, and machine-readable — not just by
human auditors, but by the autonomous agents that will call it.
