# Agent-Native TOL: Gap Analysis

## Context

[THESIS.md](../gtos/THESIS.md) frames TOS Network as the economic settlement layer for an
AI-driven agent economy. The six pillars it identifies — agent-to-agent hiring, machine-to-machine
micropayments, collateralized trust, identity and reputation, AI-mediated UX, and governance
coordination — all ultimately require programmable contracts. TOL is that programming language.

`docs/AGENT_PROTOCOL_DRAFT.tol` shows we can already express the agent economy protocols
(`IAgentRegistry`, `IReputationHub`, `ITaskEscrow`, `IDisputeResolver`, `IPredictionMarket`,
`IRewardVault`) in valid TOL. But this is **library-level** agent support: agent concepts are
represented as plain `agent` values and raw `u256` identifiers, with semantics enforced only at
runtime through explicit require-guards hand-written by the developer.

The gap between "a language you can use to write agent contracts" and a genuinely **Agent-Native**
language is the same gap that existed between a general scripting language and Solidity before DeFi:
Solidity elevated `msg.sender`, `payable`, `transfer()`, and `msg.value` to first-class primitives.
An Agent-Native TOL must elevate **identity, task, capability, payment, and trust** to the same
level.

---

## Protocol Infrastructure Requirements

The language features described in this document are not purely compiler work. They require
corresponding infrastructure from the gtos blockchain protocol. This section enumerates every
protocol-level requirement, grouped by layer, so that gtos core development can be planned
independently of the TOL compiler roadmap.

---

### I. Account State Extensions

The gtos account state must be extended beyond the standard `(balance, nonce, code_hash,
storage_root)` tuple. Two fields are added as **protocol-native account state**, meaning they
are maintained by the consensus layer — not by any contract — and are readable via a dedicated
low-cost opcode rather than a cross-contract call.

| Field | Type | Written by | Read cost | Rationale |
|---|---|---|---|---|
| `stake` | `u256` | `AGENT_REGISTRY` system contract (deposit/withdraw ops) | Protocol opcode (AGENTLOAD) | Slashing must be atomic with balance changes; cannot be split across a cross-contract call |
| `suspended` | `bool` | `AGENT_REGISTRY` system contract (suspend/unsuspend ops) | Protocol opcode (AGENTLOAD) | Suspension check is on the hot path of every `agent(addr)` cast |

**New opcode: `AGENTLOAD <addr> <field_id>`**
Returns the value of a protocol-native agent field for the given agent in O(1), equivalent
to an SLOAD on the caller's own storage. This replaces the cross-contract call path for
`worker.stake` and `worker.suspended`, making the two most frequently checked properties free
of external-call overhead and reentrancy risk.

`worker.is_active` is a **derived property** — no storage needed:
```
is_active(addr) = isRegistered(addr) AND NOT suspended(addr) AND stake(addr) >= MIN_AGENT_STAKE
```

The remaining `agent` properties (`reputation`, `capabilities`) are NOT protocol-native; they
live in system contracts (see §II) because their update logic must be governable.

---

### II. System Contracts (Deployed at Genesis)

Four contracts are deployed at genesis at fixed, consensus-level addresses. They are privileged
in that certain protocol operations (staking, slashing, capability grant) can only be triggered
through them. Their source code is part of the gtos protocol specification.

#### `tol.lang.AGENT_REGISTRY` — Identity & Stake

Manages agent registration and the mapping between `agent` identity. Writes to
protocol-native account state (`stake`, `suspended`) via privileged opcodes unavailable to
user contracts.

```
Interface:
  register(metadata_uri: string) payable               — msg.value becomes stake (AGENTLOAD write)
  updateProfile(metadata_uri: string)
  increaseStake() payable
  decreaseStake(amount: u256)                           — reverts if stake - amount < MIN_AGENT_STAKE;
                                                          stake enters UNSTAKE_FREEZE_BLOCKS cooldown
                                                          before it is withdrawable (see design note below)
  withdraw()                                            — transfers unfrozen stake to caller; reverts if
                                                          cooldown not elapsed
  suspend(addr: agent, reason: string)                — requires Registrar capability
  unsuspend(addr: agent)                              — requires Registrar capability

  isRegistered(addr: agent) → bool
  isSuspended(addr: agent) → bool
  stakeOf(addr: agent) → u256                        — reads protocol-native account state
  frozenStakeOf(addr: agent) → u256                  — stake in cooldown (still slashable)
  unstakeUnlocksAt(addr: agent) → u64                — block number when frozen stake becomes withdrawable
  metadataOf(addr: agent) → string
```

**Exit cooldown (design note):** When an agent calls `decreaseStake` or deregisters, the
withdrawn amount enters a **freeze period** of `UNSTAKE_FREEZE_BLOCKS` blocks (consensus
constant; recommended value: ~7 days at the target block time). During the freeze period the
stake is still visible via `stakeOf` (it remains slashable) but cannot be transferred out.
`withdraw()` transfers the unfrozen amount and reverts if `block.number < unstakeUnlocksAt`.
This prevents the attack pattern of: register → acquire capability → act maliciously →
immediately unstake to escape slashing. An agent's `is_active` returns false as soon as
`decreaseStake` drops the remaining unfrozen stake below `MIN_AGENT_STAKE`.

**Registrar bootstrap:** The genesis block sets a protocol-owned multisig agent as the
initial holder of the `Registrar` capability. This agent can then grant `Registrar` to
governance-approved contracts or committees, and eventually to a fully on-chain governance
contract. The genesis Registrar agent is a consensus constant (`tol.lang.GENESIS_REGISTRAR`).

The `agent(addr)` cast in TOL compiles to: `require(AGENT_REGISTRY.isRegistered(addr))`.
The `agent(addr).stake` property compiles to: `AGENTLOAD addr STAKE_FIELD` (no external call).
The `agent(addr).is_active` property compiles to an inline check against both fields.

#### `tol.lang.CAPABILITY_REGISTRY` — Capability Bitmap

Manages the global namespace of `capability` names and per-agent capability bitmaps (u256,
one bit per declared capability, up to 256 capabilities chain-wide).

```
Interface:
  registerCapability(name: string) → bit_index: u8     — requires Registrar; name → bit mapping
  grantCapability(addr: agent, bit: u8)               — requires Registrar
  revokeCapability(addr: agent, bit: u8)              — requires Registrar
  hasCapability(addr: agent, bit: u8) → bool
  capabilitiesOf(addr: agent) → u256                  — full bitmap
  totalEligible(bit: u8) → u256                         — count of agents with this capability
                                                          (used by vote<T> for quorum snapshot)
  capabilityBit(name: string) → u8                      — name → bit index lookup (compile-time use)
```

**Who maintains the capability bitmap:** Only agents holding the `Registrar` capability can
call `grantCapability` and `revokeCapability`. An agent **cannot grant or revoke its own
capabilities** — self-authorization would defeat the purpose of the permission system. The
`Registrar` role itself is bootstrapped at genesis (see `AGENT_REGISTRY` design note above).
In practice, `Registrar` will be held by: the genesis multisig initially, then transferred to
a governance contract that votes on capability grants through a `vote<bool>` proposal.

`@requires(caller: Arbitrator)` compiles to:
`require(CAPABILITY_REGISTRY.hasCapability(msg.sender, ARBITRATOR_BIT))`.

The `capability Arbitrator;` declaration in TOL registers the name at compile time and resolves
`ARBITRATOR_BIT` as a compile-time constant via `capabilityBit("Arbitrator")`.

#### `tol.lang.DELEGATION_REGISTRY` — Revocation & Nonce Tracking

Manages delegation nonce consumption and explicit revocations. Keeps a minimal footprint:
only spent/revoked nonces are stored (never-used nonces cost zero storage).

```
Interface:
  markUsed(principal: agent, nonce: u256)             — called by delegation.verify(); atomic
  revoke(principal: agent, nonce: u256)               — called by principal to revoke
  isUsed(principal: agent, nonce: u256) → bool
  nextNonce(principal: agent) → u256                  — convenience; nonces need not be sequential
```

`delegation.verify(sig, principal, scope, expiry)` compiles to a sequence that:
1. Recovers signer from `sig` over the EIP-712 typed payload
2. Checks `!DELEGATION_REGISTRY.isUsed(principal, nonce)`
3. Calls `DELEGATION_REGISTRY.markUsed(principal, nonce)`
4. Checks `block.timestamp_ms < expiry`
All four steps are atomic within the transaction.

#### `tol.lang.REPUTATION_HUB` — Reputation Scores

Manages per-agent reputation scores and the set of authorized scorers. Decoupled from
`AGENT_REGISTRY` so that scoring algorithms can be upgraded by governance without touching
identity or staking logic.

```
Interface:
  authorizeScorer(scorer: agent, enabled: bool)       — requires Registrar
  recordScore(who: agent, delta: i256,
              reason: string, ref_id: bytes32)          — requires Scorer capability
  totalScoreOf(who: agent) → i256
  ratingCountOf(who: agent) → u256
```

**Who are the Scorers:** The `Scorer` capability is granted by `Registrar` to specific
addresses. In the initial protocol deployment, Scorers are expected to be:

1. **Protocol system contracts** — `TaskEscrow` and `DisputeResolver` are granted `Scorer`
   at genesis. When a task is `approve`d, `TaskEscrow` calls `recordScore(worker, +1, "task_approved", task_id)`.
   When a dispute is ruled, `DisputeResolver` calls `recordScore(winner, +1, ...)` and
   `recordScore(loser, -1, ...)` automatically. No human intervention required.
2. **Governance-approved committee** — for edge cases, appeals, and off-chain-evidenced events
   that no contract can automatically adjudicate. Committee membership is controlled by `Registrar`.
3. **Future: oracle-backed Scorer** — a Scorer contract that accepts ZK-verified proofs of
   off-chain quality metrics (e.g., verified delivery confirmations) and records scores based
   on proof validation. This requires the `@verifiable` ZK backend (P3 roadmap item).

An agent **cannot score itself** (only `Scorer`-capable addresses can call `recordScore`).
Scorer addresses are published on-chain via events and indexable by any off-chain monitor.

`agent(addr).reputation` compiles to: `REPUTATION_HUB.totalScoreOf(addr)` (one STATICCALL).
`agent(addr).rating_count` compiles to: `REPUTATION_HUB.ratingCountOf(addr)`.

---

### III. VM Storage Primitives

The following storage abstractions are implemented in the TOL VM (the Lua bytecode executor),
not in user contracts. Each is backed by a dedicated storage namespace allocated by the compiler.

#### Escrow Ledger

The VM maintains a flat mapping:
```
escrow_balance[contract_addr][agent_addr][purpose_bit] → u256
```
`escrow(agent, amount, purpose: X)` increments this entry and deducts from the contract's
balance. `release(agent, amount, purpose: X)` decrements and transfers out. `slash(agent,
amount, recipient: R, purpose: X)` decrements and transfers to `R`. All three are VM
instructions, not Lua library calls — they cannot be spoofed by user bytecode.

The `purpose` label is resolved to a `u8` bit at compile time (same mechanism as `capability`).
`escrow_balance_of(agent, purpose: X)` is a read-only VM query available to TOL code.

#### `oracle<T>` Write-Once Slots

Each `oracle<T>` field in a contract compiles to two storage slots:
- `oracle_value_slot`: the stored value (written once)
- `oracle_set_slot`: a `bool` flag (false → not set, true → fulfilled)

The VM overrides `SSTORE` for `oracle_value_slot`: if `oracle_set_slot` is already `true`,
the write reverts with `OracleAlreadyFulfilled`. This check happens at the VM instruction level,
not in compiler-generated Lua code — it cannot be bypassed by raw bytecode.

#### `vote<T>` Tally Storage

Each `vote<T>` field compiles to a storage namespace containing:
- `eligible_snapshot: u256` — captured at `vote<T>.new(...)` from `CAPABILITY_REGISTRY.totalEligible(bit)`
- `quorum_bps: u16`, `deadline_ms: u64`, `tie_value: T` — creation parameters
- `tally: mapping(u8 → u256)` — vote count per option value
- `voted: mapping(agent → bool)` — per-voter participation flag

The VM enforces the per-voter single-write: `cast(voter, choice)` checks and sets the `voted`
flag atomically. The `eligible_snapshot` is immutable after creation.

#### `task<T>` State Machine Storage

Each `task<T>` field compiles to a storage namespace per task ID:
- `poster: agent`, `worker: agent`, `reward: u256`, `deadline_ms: u64`, `spec_hash: bytes32`
- `status: u8` (0=None, 1=Open, 2=Accepted, 3=Submitted, 4=Approved, 5=Rejected, 6=Disputed, 7=Cancelled)
- `result: T` (written by `submit`)

State transition guards are VM instructions that check the current `status` before allowing
the transition, reverting with a typed error (e.g., `TaskInvalidTransition`) if violated.

---

### IV. Mempool & Sequencer Extensions (Account Abstraction)

The gtos mempool and block-builder must implement a two-phase transaction lifecycle for
transactions originating from `account contract` addresses.

**Transaction phases:**

On gtos, validators operate under DPoS and are compensated via block rewards plus the
standard 10 gtomi/gas fee on every executed transaction. There is no separate bundler
deposit or validation fee ledger (unlike ERC-4337). Gas for `validate()` is charged
from the account's main `balance`, just like any other call.

```
Phase 1 — Validation (before inclusion in block):
  1. Check account.balance >= VALIDATION_GAS_CAP * GAS_PRICE_GTOMI + estimated_execution_gas_cost.
     Reject if insufficient funds (protects the validator from unpaid work).
  2. Call account.validate(tx_hash, sig) with hard gas cap = VALIDATION_GAS_CAP (50,000).
     - If validate() returns false → reject transaction (no gas charge to account).
     - If validate() exceeds gas cap → treat as false (reject; no charge).
     - If validate() reverts → treat as false (reject; no charge).
  3. Deduct actual validation gas cost from account.balance.
  4. If validate() returned true → proceed to Phase 2.

Phase 2 — Execution (normal transaction execution):
  1. Call account.beforeTransfer(to, amount) before any value transfer from this account.
  2. Execute the transaction payload.
  3. Charge execution gas to account's main balance at 10 gtomi/gas.
```

**Consensus constants required:**
```
tol.lang.VALIDATION_GAS_CAP    = 50_000    (gas units)
tol.lang.GAS_PRICE_GTOMI        = 10        (gtomi per gas unit; fixed)
tol.lang.MIN_AGENT_STAKE       = TBD       (minimum u256 stake for agent registration)
```

**System contract addresses (fixed at genesis):**
```
tol.lang.AGENT_REGISTRY        = 0x0000000000000000000000000000000000000101
tol.lang.CAPABILITY_REGISTRY   = 0x0000000000000000000000000000000000000102
tol.lang.DELEGATION_REGISTRY   = 0x0000000000000000000000000000000000000103
tol.lang.REPUTATION_HUB        = 0x0000000000000000000000000000000000000104
```

---

### V. Block Context Extension

The existing `block` context object must expose millisecond-precision timestamps, required
by `task<T>.is_expired`, `vote<T>.is_decided`, and `oracle<T>` challenge windows:

```
block.timestamp_ms   → u64   (milliseconds since Unix epoch; consensus-provided)
block.number         → u64   (block height)
block.hash           → bytes32
```

`block.timestamp_ms` must be included in the block header and validated by consensus.
Millisecond granularity is required because task and delegation deadlines are specified in
milliseconds and the language guarantees `t.is_expired = (block.timestamp_ms > t.deadline_ms)`.

---

### VI. Compiler & ABI Toolchain Extensions

The TOL compiler (`tol/codegen/`) and the `.toc` ABI format must be extended to carry
agent-native metadata that off-chain AI orchestrators consume.

**`.toc` ABI additions per function:**
```json
{
  "name": "getPrice",
  "requires_capability": "Arbitrator",
  "pay_amount_tomi": 1000000,
  "gas_bound": 450000,
  "total_cost_tomi": 4501000000,
  "effects": ["writes:balances[recipient]", "emits:Transfer(agent,agent,u256)"],
  "verifiable": false
}
```

**`.toc` manifest section (from `manifest {}` block):**
```json
{
  "manifest": {
    "version": "1.0.0",
    "capabilities": ["DataFetcher", "PriceOracle"],
    "sla_uptime_bps": 9900,
    "price_per_call": { "getPrice": 1000000, "getBatch": 5000000 },
    "spec_hash": "0xabc123...",
    "spec_uri": "ipfs://Qm...",
    "sla_escrow_wei": 10000000
  }
}
```

**`@effects` extension:** The existing `@effects` system must be updated to recognise
`agent`-typed parameters. `emits: Transfer(agent, agent, u256)` must become
`emits: Transfer(agent, agent, u256)` when the event parameters are declared as `agent` type,
so orchestrators can reason about registered identity rather than raw addresses.

---

### Summary: Who Implements What

| Requirement | Owner |
|---|---|
| `stake` + `suspended` in account state | gtos consensus / state trie |
| `AGENTLOAD` opcode | gtos VM |
| `AGENT_REGISTRY` system contract | gtos protocol (TOL source, genesis deploy) |
| `CAPABILITY_REGISTRY` system contract | gtos protocol (TOL source, genesis deploy) |
| `DELEGATION_REGISTRY` system contract | gtos protocol (TOL source, genesis deploy) |
| `REPUTATION_HUB` system contract | gtos protocol (TOL source, genesis deploy) |
| Escrow ledger VM instructions | TOL VM (`vm.go` / `tol_ir_direct_lowering.go`) |
| `oracle<T>` write-once SSTORE guard | TOL VM |
| `vote<T>` tally + snapshot storage | TOL VM |
| `task<T>` state machine storage | TOL VM |
| Two-phase AA transaction lifecycle | gtos mempool + sequencer |
| `block.timestamp_ms` in block header | gtos consensus |
| `.toc` ABI extensions | TOL compiler (`tol/codegen/`) |
| `@effects` agent-type awareness | TOL sema + codegen |

---

## What Is Missing

### 1. `agent` — Native Data Type — ✅ Done

**Current state:** agent identity is a bare identity value. All registry lookups, stake checks, and
reputation reads are hand-written calls to external interfaces.

```tol
// today
agent worker = msg.sender;
require(IAgentRegistry(registry).isRegistered(worker), "NotRegistered");
u256 stake = IAgentRegistry(registry).stakeOf(worker);
```

**Agent-Native:** `agent` is a first-class type — a superset of raw identity that the compiler
integrates with the system registry automatically.

```tol
agent worker = agent(msg.sender);   // runtime: reverts if msg.sender is not a registered agent
worker.stake        // → u256
worker.reputation   // → i256
worker.is_active    // → bool
worker.capabilities // → u256 (each bit = one declared capability; supports up to 256 global caps)
```

**Design constraints:**

- **Registry binding:** The system registry agent is a consensus-level constant
  (`tol.lang.AGENT_REGISTRY`), not a constructor argument. This removes the coupling ambiguity —
  there is exactly one authoritative registry per chain.
- **Property access cost:** Each distinct property read (`worker.stake`, `worker.reputation`, …)
  compiles to one external call into `AGENT_REGISTRY`. The registry implementation stores each
  field in its own storage slot; once a slot is touched it enters the EVM warm-storage set for
  the remainder of that transaction, so repeated reads of the same property within one call are
  cheap. However, the first read of each *distinct* property across a cross-contract boundary is
  still one warm call (not a cold SLOAD on the caller's side). Developers should cache
  frequently-read properties in local variables rather than relying on implicit caching.
- **Null agent:** `agent(0)` is the zero agent. `agent(addr)` on an unregistered agent
  reverts with `AgentNotFound`. Use the raw identity without registry lookup.
- **Chicken-and-egg:** Inside `AgentRegistry` itself, properties like `worker.stake` resolve to
  local storage directly (the compiler detects self-registry calls and skips the
  external call). All other contracts use the external call path.

**`msg.agent` context variable:**
Within any function, `msg.agent` gives the agent-typed sender directly. It equals
`agent(msg.sender)` when `msg.sender` is a registered agent, and is the zero agent otherwise.
Using `msg.agent` instead of `agent(msg.sender)` avoids an explicit revert on unregistered
callers and lets the function decide how to handle them.

```tol
function acceptTask(u256 task_id) public {
  require(msg.agent != agent(0), "CallerNotAgent");
  tasks[task_id].accept(msg.agent);
}
```

This is the single highest-leverage addition because every other agent primitive builds on it.

---

### 2. `manifest {}` Block — Agent Self-Description — ✅ Done

**Current state:** there is no machine-readable way for a deployed contract to declare what it is,
what it can do, or what it costs. Capability discovery is entirely off-chain and informal.

**Agent-Native:** a `manifest` block inside a contract compiles into the `.toc` ABI file, making
the contract automatically discoverable by off-chain agent marketplaces and indexers.

```tol
contract PriceOracle {
  manifest {
    version:        "1.0.0";
    capabilities:   [DataFetcher, PriceOracle];
    sla_uptime:     9900;           // advisory bps, 99.00% — not on-chain enforced
    price_per_call: 1_000_000;      // micro-TOS; compiler verifies this matches @pay on callable fns
    spec_hash:      0xabc123...;    // keccak256 of the JSON-LD capability description
    spec_uri:       "ipfs://Qm..."; // hint only; spec_hash is the immutable commitment
  }
  // ...
}
```

**Design constraints:**

- **`price_per_call` is a compile-time constant and is valid here** because gtos fixes the gas
  price at 10 gtomi at the consensus layer. This makes all execution costs fully deterministic at
  compile time. The compiler cross-checks that `manifest.price_per_call` equals the literal in
  every `@pay(amount)` annotation on functions exposed by this contract; a mismatch is a compile
  error (`TOL2208: manifest.price_per_call does not match @pay annotation`). There is no risk of
  the manifest diverging from the on-chain enforced price.
- `spec_hash` (a `bytes32` keccak256 digest) is the authoritative commitment; `spec_uri` is a
  retrieval hint only. URI availability is not guaranteed; the hash never changes.
- `sla_uptime` is advisory metadata for off-chain indexers; it carries no on-chain enforcement.
  A contract making SLA guarantees must implement a separate on-chain uptime oracle.
- Capabilities listed in `manifest.capabilities` must be a subset of the globally declared
  `capability` names in scope (Section 3). The compiler rejects undeclared capability names.
- **Multiple `@pay` functions:** `price_per_call` in the manifest is a map from function
  selector to amount, not a single scalar. If all paying functions share the same price, a single
  value is shorthand. If they differ, the form is:
  ```tol
  price_per_call: { getPrice: 1_000_000, getBatch: 5_000_000 };
  ```
  The compiler verifies each entry matches the corresponding `@pay` annotation.

This directly addresses the **identity and reputation** pillar from THESIS.md.

---

### 3. `capability` Type + `@requires` Annotation — Runtime Permission Checks — ✅ Done

**Current state:** access control is enforced via hand-written `require` statements with no
compiler assistance. Nothing prevents a developer from forgetting a check.

**Agent-Native:** capabilities are declared as named types at file scope; the `@requires`
annotation on a function causes the compiler to insert a runtime registry check automatically.

```tol
capability DataFetcher;
capability Arbitrator;

@requires(caller: Arbitrator)
function rule(u256 dispute_id, agent winner, u16 slash_bps, string reason) public { ... }
```

Compiler generates at function entry:
`require(AGENT_REGISTRY.hasCapability(msg.sender, Arbitrator), "CapabilityDenied")`.

**Design constraints:**

- The check is always **runtime** (capabilities can be granted and revoked between transactions).
  The compiler never elides the check based on static analysis.
- `@requires` checks `msg.sender` only — the immediate caller. For delegation chains
  (agent A calls agent B which calls this function), only B's capabilities are checked. Callers
  that need to attest on behalf of another agent must use an explicit delegation mechanism
  (future work: `@delegates` annotation).
- Capability names are global within a compilation unit. Two files declaring `capability Foo`
  in the same project refer to the same bit in the registry bitmap. Conflicting names across
  packages are a linker error.
- The annotation is recorded in the `.toc` ABI so that off-chain orchestrators can determine
  which agents are eligible to call which functions before submitting a transaction.

---

### 4. `@pay(amount, recipient:)` Annotation — Micropayment Primitive — ✅ Done

**Current state:** per-call payments require manual `require(msg.value >= ...)` guards and explicit
transfer logic in every function that charges a fee.

**Agent-Native:** the `@pay` annotation enforces a payment requirement at the language level.
The amount is a **compile-time constant** expressed in micro-TOS (1 TOS = 10¹² micro-TOS).

```tol
@pay(1_000_000, recipient: fee_account)   // 1 micro-TOS per call; compiler-enforced
function getPrice(bytes32 pair) public returns (u256 price) { ... }
```

Compiler generates at function entry:
`require(msg.value >= 1_000_000, "InsufficientPayment")`, transfers exactly `1_000_000` tomi to
`fee_account`, and refunds any `msg.value` surplus to `msg.sender`.

**Why compile-time constants are correct here:** gtos fixes the gas price at **10 gtomi** at the
consensus layer. This eliminates gas price volatility as a reason for runtime fee adjustment.
The unit cost of each gas unit is fixed; the total gas *consumed* still varies by execution path,
but the **worst-case cost** of any call is statically boundable (see `@total_cost` in Section 13).
The `@pay` constant can be advertised in `manifest.price_per_call` and the compiler guarantees
they stay in sync (see Section 2). Contracts that genuinely need governance-adjustable fees must
redeploy with a new literal; dynamic pricing is out of scope for `@pay`.

**Refund semantics (safe pattern):** any `msg.value` surplus above the `@pay` amount is returned
to `msg.sender` using the **checks-effects-interactions** pattern: the surplus transfer happens
as the *last* operation after all state changes, and uses a fixed gas stipend (2300 gas) to
prevent reentrancy. If the refund transfer fails (e.g., caller is a contract that rejects
TOS), the entire call reverts — the compiler does not silently swallow failed refunds.

**Design constraints:**

- **`@pay` is incompatible with `view`.** A `view` function cannot be `payable`: receiving TOS
  modifies the contract balance, which is state mutation. The compiler rejects `@pay` + `view`
  with error `TOL2206: @pay cannot be applied to view functions`. For read APIs that need access
  control, use subscription pre-payment or signed access tokens; off-chain payment channels are
  the recommended pattern for high-frequency M2M reads.
- The `recipient:` field is required. The compiler does not silently route funds to `agent(this)`.
  Making the destination explicit prevents accidental fund trapping.
- `@pay` is appropriate for per-call on-chain payments. For very high-frequency M2M interactions,
  on-chain per-call payment remains gas-intensive; use pre-paid subscription quotas (`@quota`,
  future work) or state channels. The fixed gas price makes the break-even frequency calculable
  at design time.

---

### 5. `task<T>` Type — Lifecycle-Aware Task Primitive — ✅ Done

> **Removed.** The `task<T>` compiler intrinsic has been replaced by the openlib pattern
> in `openlib/Task.tol`. Use explicit state constants and `require()` guards instead.

**Current state:** `TaskEscrow` tracks task state in raw `mapping(u256 => u8)` with numeric status
codes. The state machine is invisible to the type system.

**Agent-Native:** `task<T>` is a parameterized storage type with an explicit lifecycle enforced at
**runtime** by the VM. `T` is the result type (typically `bytes32` as a content hash; complex
results live off-chain).

```tol
// Posting — function must be payable; reward = msg.value automatically
task<bytes32> t = task<bytes32>.post(msg.agent, spec_hash, deadline_ms);

t.id        // → u256  (stable storage key)
t.status    // → TaskStatus (Open | Accepted | Submitted | Approved | Disputed | Cancelled)
t.poster    // → agent
t.worker    // → agent  (zero agent if status == Open)
t.reward    // → u256
t.is_expired // → bool  (block.timestamp_ms > t.deadline_ms)

t.accept(msg.agent);           // runtime revert if status != Open
t.submit(result_hash);         // runtime revert if status != Accepted or caller != t.worker
t.approve();                   // runtime revert if status != Submitted or caller != t.poster
t.reject(reason);              // runtime revert if status != Submitted or caller != t.poster
t.dispute();                   // runtime revert if status not in {Submitted, Rejected}
t.cancel();                    // runtime revert if status != Open or caller != t.poster
```

**Design constraints:**

- State-machine violations are **runtime reverts**, not compile-time errors. Tasks span multiple
  transactions; the compiler cannot know the task's status at call time.
- `task<T>` is a **language-level storage abstraction**, not a library contract. The compiler
  allocates a dedicated storage namespace for each `task<T>` mapping in a contract. There is no
  external `TaskEscrow` dependency; the semantics are built into the VM.
- **Posting caller must be a registered agent.** `task<T>.post(msg.agent, ...)` reverts with
  `AgentRequired` if `msg.agent` is the zero agent (i.e., `msg.sender` is not registered).
  Anonymous task posting is not permitted — every task must have an on-chain accountable poster
  for dispute resolution and slashing to function correctly.
- `t.worker` on an Open task returns the zero agent (`agent(0)`), not a revert. Callers
  should check `t.status` before reading worker-specific fields.
- The generic parameter `T` must be a value type or `bytes32`. Dynamic types (string, bytes)
  are not supported as task result types; use a `bytes32` content hash pointing to off-chain data.

---

### 6. `escrow` / `slash` / `release` — Built-in Trust Primitives — ✅ Done

**Current state:** collateral management is verbose, pattern-repeated code appearing identically
in `TaskEscrow`, `DisputeResolver`, and any staking contract.

**Agent-Native:** collateral operations are built-in statements with explicit, auditable semantics.

```tol
escrow(worker, reward, purpose: TaskReward);   // lock `reward` tomi associated with `worker`
                                               // under the label TaskReward
slash(loser, amount, recipient: treasury);     // penalize `loser` by `amount`; send to `treasury`
release(worker, amount, purpose: TaskReward);  // unlock `amount` from worker's TaskReward balance
```

**Design constraints:**

- **`escrow` requires a `purpose` label.** A contract may escrow funds for multiple reasons
  (task reward, dispute bond, stake top-up). Without a label, balances from different purposes
  would be mixed, making partial release and accounting impossible. Purpose labels are
  **declared types** (like `capability`), not string constants — this catches typos at compile
  time:
  ```tol
  purpose TaskReward;
  purpose DisputeBond;
  escrow(worker, reward, purpose: TaskReward);   // compile error if TaskReward is undeclared
  ```
- **`slash` requires an explicit `recipient`.** The destination of slashed funds is an economic
  design decision (burn, treasury, victim compensation). The compiler does not silently route
  slashed funds — "per policy" is not a valid substitute for an explicit agent.
- **`slash` draws exclusively from the named escrow balance.** `slash(loser, amount, recipient: treasury)`
  deducts `amount` only from `loser`'s escrowed balance under the active purpose label. It never
  touches the agent's free balance or stake in the registry. If the escrowed balance is
  insufficient, `slash` reverts with `InsufficientEscrow` — the caller must decide whether to
  proceed with a partial slash by querying `escrow_balance_of(loser, purpose: X)` first.
- **`release` requires an `amount`.** Full-balance release is `release(worker, worker_escrow_balance(purpose: TaskReward))`.
  There is no zero-argument `release` that implicitly releases everything; ambiguity in collateral
  release has caused multiple DeFi exploits.
- All three are statements (not expressions) that modify contract storage. They are illegal inside
  `view` or `pure` functions.

---

### 7. `oracle<T>` Type — Write-Once Data Feed — ✅ Done

> **Removed.** The `oracle<T>` compiler intrinsic has been replaced by the openlib pattern
> in `openlib/Oracle.tol`. Use plain `value` + `is_set` storage with a `require(!is_set)` guard.

**Current state:** oracle resolution is manual state mutation with no language-level write-once
guarantee. A second `resolve()` call can silently overwrite the outcome.

**Agent-Native:** `oracle<T>` is a write-once storage type. The VM enforces single-write at the
bytecode level — a second `fulfill()` reverts regardless of how the contract is called.

```tol
oracle<u8> winning_outcome;

@requires(caller: OracleResolver)          // unified with the capability system (not @authorized)
function resolve(u8 outcome) public {
  require(block.timestamp_ms >= close_time_ms, "MarketNotClosed");
  winning_outcome.fulfill(outcome);        // runtime revert if already fulfilled
  emit Resolved(outcome, agent(msg.sender));
}

function claim() public {
  require(winning_outcome.is_set, "NotResolved");
  u256 payout = computePayout(msg.agent, winning_outcome.value);
  release(msg.agent, payout, purpose: WinningClaim);
}
```

**Design constraints:**

- **Access control uses `@requires`, not `@authorized`.** There is one unified authorization
  mechanism in the language. `@authorized` does not exist; `@requires(caller: OracleResolver)`
  is the correct form.
- **No built-in challenge period.** `oracle<T>` guarantees write-once integrity but not data
  correctness. Contracts that need a dispute window must implement it explicitly
  (e.g., a `challenge_deadline_ms` field checked before `fulfill` becomes externally readable).
  This is an explicit design choice: oracles that need dispute resolution should compose with
  `task<T>` or `IDisputeResolver`.
- The write-once guarantee is enforced at the **storage slot level** in the VM, not only in
  compiler-generated code. A contract calling the bytecode directly (bypassing TOL) still cannot
  overwrite a fulfilled oracle.
- `T` must be a value type (`bool`, `u8`–`u256`, `agent`, `bytes32`).

---

### 8. `@effects` / `@gas` / `@bounds` — Agent-Legible Contracts — ✅ Done

**Relevance:** THESIS.md Pillar 5 states that AI agents acting as user interfaces must verify
"the safety of each action" before executing it. This requires contracts to be machine-readable
about what they do, not just what they return.

TOL already implements the `@effects`, `@gas`, and `@bounds` annotation system (see
`docs/TOL_EFFECTS.md`). This is directly Agent-Native infrastructure:

```tol
/// @effects(writes: balances[recipient])
/// @effects(emits: Transfer)
/// @gas(upper: 50000)
/// @bounds(amount > 0)
function transfer(agent recipient, u256 amount) public returns (bool ok) { ... }
```

An AI orchestrator can read the `.toc` ABI, inspect the declared effects, gas ceiling, and
invariant bounds, and decide whether to approve a transaction **before submitting it** — without
simulating the full execution. This is the machine-readable safety contract that Pillar 5 requires.

**What still needs to happen:** The `@effects` system must be extended to understand `agent`-typed
parameters (current implementation uses `agent`). Effects like `emits: Transfer(agent, agent,
u256)` should reflect the agent types in the ABI doc output so orchestrators can reason about
identity, not just raw addresses.

---

### 9. `delegation` — Principal Tracking for AI-Mediated UX — ✅ Done

**Relevance:** THESIS.md Pillar 5 — individuals delegate tasks to AI assistants that manage
portfolios and execute transactions on their behalf. The language must express *who authorized
an action*, not just *who sent the transaction*.

**Current gap:** `msg.sender` (or `msg.agent`) is the immediate caller. When a human delegates
to an AI agent that calls a contract, the contract sees the AI agent as sender. There is no
on-chain record that the human principal authorized this specific action.

**Agent-Native:** a `delegation` type and `@delegated` annotation express principal-agent
relationships at the language level.

```tol
// Human creates a delegation off-chain (EIP-712 signed) or on-chain
delegation d = delegation.verify(sig, principal, scope, expiry);

d.principal    // → agent  (the human who delegated)
d.delegate     // → agent  (the AI agent acting on their behalf)
d.scope        // → bytes32 (hash of permitted action set)
d.is_valid     // → bool   (not expired, not revoked)
```

```tol
// Contract function that accepts delegated calls
@delegated
function rebalancePortfolio(delegation d, ...) public {
  require(d.is_valid, "DelegationExpired");
  require(d.delegate == msg.agent, "WrongDelegate");
  // act on behalf of d.principal
  _rebalance(d.principal, ...);
}
```

**Design constraints:**

- **Replay protection:** every delegation carries a `nonce` (incrementing counter per principal)
  and a `chain_id`. `delegation.verify()` checks that the nonce has not been consumed and marks
  it spent atomically. Sub-delegations inherit a nonce namespace scoped to the parent delegation
  id, preventing replay of child delegations after parent revocation.
- **Domain separation:** the signed payload is structured as
  `keccak256(chain_id ++ contract_address ++ function_selector ++ scope ++ expiry ++ nonce)`,
  following EIP-712 typed-data conventions. Cross-contract replay of the same signature is
  structurally impossible.
- `delegation` is verified against a registry of revocations (`tol.lang.DELEGATION_REGISTRY`),
  analogous to the agent registry. A principal can revoke a delegation at any time; subsequent
  calls with the old signature revert. Revocation is keyed by `(principal, nonce)` so only the
  specific delegation is revoked, not all delegations from that principal.
- `d.scope` limits what the delegate can do. The compiler can check that actions taken inside a
  `@delegated` function are within the declared scope (future: static scope analysis).
- Delegation chains (human → AI agent → sub-agent) are supported by composing delegations:
  `d2 = d1.subdelegate(sub_agent, narrower_scope)`. Sub-delegation scope must be a strict subset
  of the parent scope; the VM rejects sub-delegations that claim wider permissions.
- This is distinct from `capability`: capabilities describe what an agent *is allowed to do*
  globally; delegations describe what a *specific principal* has authorized for a *specific
  action set* at a *specific time*.

---

### 10. `vote<T>` Type — Governance Coordination Primitive — ✅ Done

> **Removed.** The `vote<T>` compiler intrinsic has been replaced by the openlib pattern
> in `openlib/Vote.tol`. Use mappings and threshold logic with `require()` guards.

**Relevance:** THESIS.md Pillar 6 — AI agents analyze proposals and recommend voting decisions
in decentralized governance. The language needs a native governance primitive that is as safe
and auditable as `oracle<T>` but multi-party.

**Current gap:** On-chain governance is built from raw mappings and manual tally logic. There is
no type-system guarantee that a voter can only vote once, or that tallying happens correctly at
the deadline.

**Agent-Native:** `vote<T>` is a multi-party, write-once-per-voter type with automatic tally
semantics. `T` is the option type (typically `bool` for binary proposals, `u8` for multi-choice).

```tol
vote<bool> proposal;    // declared as a storage field

// Casting a vote — each eligible agent may call this exactly once
@requires(caller: GovernanceMember)
function castVote(bool choice) public {
  proposal.cast(msg.agent, choice);  // runtime revert if msg.agent has already voted
}

// Reading results after quorum or deadline
function execute() public {
  require(proposal.is_decided, "NotDecided");   // quorum reached or deadline passed
  if proposal.result {
    _applyProposal();
  }
}

proposal.vote_count   // → u256  total votes cast
proposal.yes_count    // → u256  (when T == bool)
proposal.no_count     // → u256  (when T == bool)
proposal.is_decided   // → bool  (quorum met OR deadline passed)
proposal.result       // → T     (winning option; runtime revert if not decided)
```

**Design constraints:**

- `vote<T>` is instantiated with quorum and deadline parameters:
  ```tol
  vote<bool> proposal = vote<bool>.new(
    quorum:      5100,       // 51% of eligible_snapshot — snapshot taken AT CREATION TIME
    deadline_ms: close_ts,
    tie:         false       // value returned by `result` when yes == no at deadline
  );
  ```
- **Quorum denominator is snapshot at creation time**, not at evaluation time. When
  `vote<bool>.new(...)` is called, the VM records `eligible_snapshot` = the count of agents
  holding the required capability at that block. This prevents capability-churn attacks where
  adversaries mint or burn capability grants near the deadline to manipulate whether quorum is
  met. `is_decided` becomes `true` when
  `yes_count + no_count >= eligible_snapshot * quorum / 10000` OR `deadline_ms` has passed.
- **Tie-breaking**: when `yes_count == no_count` at deadline, `result` returns the value of the
  `tie:` parameter specified at construction (default `false` = proposal rejected on tie). This
  is a storage constant set at creation, not a runtime decision, preserving determinism.
  For non-`bool` votes, tie-breaking among equal-count options uses the lowest ordinal value.
- Each agent may call `cast()` exactly once per `vote<T>` instance. The VM enforces this at the
  storage slot level (same guarantee as `oracle<T>` write-once).
- `@requires(caller: GovernanceMember)` controls who is eligible to vote. `vote<T>` does not
  bake in eligibility rules — that is the `capability` system's job.
- `T` must be a value type. For ranked-choice or scored voting, use `vote<u8>` with application-
  level semantics on the `u8` values.
- AI agents can read `proposal.vote_count`, `proposal.yes_count`, and the proposal spec hash
  from the `.toc` ABI without any off-chain indexing, enabling fully on-chain governance
  recommendation engines.

---

### 11. `account` Contract Type — Account Abstraction for Autonomous Agents — ✅ Done

**Relevance:** THESIS.md explicitly lists "Account abstraction allows programmable accounts
suitable for autonomous agents" as a required enabling technology. An autonomous AI agent cannot
rely on a human holding a private key to sign every transaction it initiates. The agent itself
must control its on-chain account.

**Current gap:** TOL contracts are deployed accounts, but they cannot autonomously *initiate*
transactions — that requires a signing key (EOA) or a protocol-level account abstraction
facility. Without this, every agent still needs a human-controlled EOA as a "puppeteer" to
submit its transactions, which breaks autonomy.

**Agent-Native:** an `account` contract type declares that this contract is a programmable wallet,
integrating with gtos protocol-level account abstraction. The VM treats `account` contracts as
first-class transaction originators with user-defined validation logic.

```tol
account contract AgentWallet {
  agent owner;              // the registered agent identity bound to this wallet
  u256  daily_limit;        // max spend per day without multi-sig
  mapping(agent => bool) trusted_delegates;

  // The VM calls validate() before executing any transaction from this account
  // instead of the usual ECDSA signature check
  function validate(bytes32 tx_hash, bytes sig) public view returns (bool ok) {
    // Custom logic: capability check, delegation verification, spending limit
    if trusted_delegates[recover(tx_hash, sig)] { return true; }
    return owner == agent(recover(tx_hash, sig));
  }

  // Spending guard called by VM before any value transfer
  function beforeTransfer(agent to, u256 amount) public {
    require(amount <= daily_limit, "DailyLimitExceeded");
  }
}
```

**Design constraints:**

- `account` is a modifier on `contract`, not a separate keyword. An `account contract` compiles
  to a contract that implements the gtos account abstraction interface recognised by the
  sequencer/mempool.
- The `validate()` function replaces ECDSA signature verification for transactions sent from
  this account. It must be a `view` function (no state changes during validation) and must
  return `bool`. Returning `false` causes the transaction to be rejected by the mempool without
  consuming gas from the account.
- **Hard validation gas cap:** the VM enforces a protocol-level gas limit of **50,000 gas** for
  `validate()` execution. Any `validate()` that exceeds this limit is treated as returning
  `false` (rejected). This cap prevents mempool DoS: an attacker cannot submit transactions
  whose validation consumes unbounded compute at the sequencer's expense. The cap is a
  consensus constant (`tol.lang.VALIDATION_GAS_CAP = 50000`), not configurable per contract.
- **Validation fee sponsorship:** gas consumed by `validate()` itself is charged to the account's
  pre-deposited validation fee balance (a separate ledger from the account's main balance),
  not from `gasLimit` of the inner call. The account must maintain a non-zero validation fee
  balance to remain submittable; transactions from accounts with depleted validation balances
  are rejected by the mempool before validation runs.
- `account` contracts interact naturally with `delegation`: the wallet's `validate()` can call
  `delegation.verify()` to approve transactions signed by a delegated AI agent on behalf of a
  human principal, consuming from the 50,000 gas cap.
- `agent` type and `account` type are orthogonal: `agent` describes *who* (identity + reputation
  in the registry); `account` describes *how* the entity controls its on-chain funds. A human
  could have an `account contract` wallet but not be a registered `agent`. An AI agent has both:
  a registry entry (`agent` type) and a programmable wallet (`account contract`).
- Gas for `validate()` execution is pre-paid via the account abstraction fee mechanism; it does
  not consume from `gasLimit` of the inner call.

---

### 12. `@verifiable` Annotation — ZK Readiness — ✅ Done (stub)

**Current state:** TOL has no concept of zero-knowledge proofs. THESIS.md lists
**zero-knowledge payments** as a required enabling technology.

**Important distinction:** `@verifiable` addresses **ZK computation integrity** (proving that a
computation was performed correctly without re-executing it) — NOT **ZK payment privacy**
(hiding sender, receiver, and amount using commitments and nullifiers). These are separate
problems. `@verifiable` alone does not provide private transfers; a separate ZK payment rails
layer (e.g., a Groth16/PLONK-based shielded pool contract) is required for privacy. That layer
is out of scope for the language spec and belongs in the protocol layer.

**Agent-Native:** `@verifiable` marks `view` functions whose outputs can be proven off-chain and
verified on-chain without re-execution. The annotation is valid **only on `view` functions**
(read-only, deterministic, no side effects — exactly the ZK circuit model requires).

```tol
@verifiable
function totalScoreOf(agent who) public view returns (i256 score) { ... }
```

For each `@verifiable` function `f`, the compiler also emits a companion verification entry point:

```tol
// auto-generated by compiler; not written by developer
function verify_totalScoreOf(bytes proof, agent who, i256 expected_score) public view returns (bool);
```

**Design constraints:**

- **`@verifiable` is only valid on `view` functions.** Non-view functions have side effects
  that cannot be captured in a ZK proof. The compiler rejects `@verifiable` on non-view functions
  with `TOL2207: @verifiable requires view`.
- **ZK compatibility constraints** — the compiler warns (future: errors) if a `@verifiable`
  function violates any of:
  - Makes a cross-contract call (external state is not part of the circuit witness)
  - Reads `block.timestamp_ms`, `block.number`, or other block-context globals (non-deterministic
    across proof generation and verification)
  - Uses types outside the prime field (`string`, dynamic `bytes` — use `bytes32` instead)
- In the current compiler, the `verify_*` entry point is a stub that reverts
  (`ZKBackendNotImplemented`). The annotation reserves the ABI slot so existing contracts become
  ZK-compatible when the proof backend ships, without source changes.
- The parameter type uses `agent` (not `address`) consistently with the rest of the type system.

---

### 13. Fixed Gas Price — Unique Language Features Unlocked — ✅ Done (`@quota`, `@total_cost` compiler support)

gtos fixes the gas price at **10 gtomi** at the consensus layer. This is not just an economic
detail — it enables a class of language features that are impossible or unsound on variable-price
chains. These features have no equivalent in Solidity/EVM today.

#### `@total_cost(max: N)` — Compile-Time Cost Certification

```tol
/// @total_cost(max: 500_000)   // worst-case: 500k gas × 10 gtomi + @pay = 0.005 TOS + fee
/// @gas(upper: 450_000)
/// @pay(50_000, recipient: fee_account)
function getPrice(bytes32 pair) public returns (u256 price) { ... }
```

The compiler computes the declared worst-case cost in tomi: `gas_bound × 10 gtomi + pay_amount`.
This value is written into the `.toc` ABI as a machine-readable field. AI agent orchestrators
can read it before submitting a transaction and know the **maximum they will ever pay** —
without simulation, without gas estimation, without oracle lookups.

The `@total_cost` annotation is checked against `@gas` + `@pay`: if the declared max is below
the sum of the gas bound and pay amount at 10 gtomi, the compiler emits
`TOL2209: @total_cost inconsistent with @gas and @pay`.

#### Native Prepaid Quota Contracts (`@quota`)

Because unit cost is fixed, "N calls for M micro-TOS" contracts have exact, non-approximated
depletion math:

```tol
@quota(calls: 1000, price: 1_000_000_000)   // 1000 calls for 0.001 TOS total
function getPrice(bytes32 pair) public returns (u256 price) { ... }
```

The compiler generates a quota ledger in contract storage and a per-call decrement check.
Quota balances are exact (no rounding, no gas estimation buffer) because cost is deterministic.
This is the high-frequency M2M micropayment primitive that `@pay` cannot serve (Pillar 2).

#### Cost-Certifiable ABI

Every function in a `.toc` file compiled for gtos includes:

```json
{
  "name": "getPrice",
  "gas_bound": 450000,
  "pay_amount": 50000,
  "total_cost_tomi": 4550000000000,
  "total_cost_gtomi": "4550"
}
```

An AI agent can select the cheapest service provider from multiple competing contracts purely
by reading their `.toc` files — no simulation, no on-chain interaction required. This enables
a fully **machine-negotiable service market**.

#### Deterministic SLA Escrows

```tol
/// @gas(upper: 100_000)
/// @total_cost(max: 1_050_000)
function processOrder(bytes32 order_id) public returns (bool ok) { ... }

// If actual gas exceeds the declared bound (detected post-execution by VM),
// the VM automatically compensates the caller from a pre-funded SLA escrow.
sla_escrow: 10_000_000;   // manifest field: compensation pool in micro-TOS
```

When a function declares `@gas(upper: N)` and the VM observes actual gas exceeding N (which should
never happen if the bound is correct — the VM can verify this), it triggers an automatic
compensation from the contract's SLA escrow pool. This is only safe because gas price is fixed;
on a variable-price chain, the compensation formula would require a price oracle.

---

## Prioritized Roadmap

| Priority | Feature | THESIS.md Pillar / Tech | Status |
|----------|---------|------------------------|--------|
| **Done** | `@effects` / `@gas` / `@bounds` annotation system | Pillar 5: AI safety verification | ✅ Done |
| P0 | `agent` native type + `msg.agent` (registry-backed; property caching; zero-agent semantics) | Pillar 4: Identity & reputation | ✅ Done — `agent(expr)` cast, property access, and `msg.agent` context var all implemented |
| P0 | `capability` + `purpose` declared types; `@requires` (runtime, `msg.sender`-scoped) | Pillar 4: Trust | ✅ Done |
| P1 | `account` contract type (AA wallet; `validate()` with 50k gas cap; validation fee ledger) | Tech: Account abstraction | ✅ Done — `account contract` keyword, AA marker sstore in constructor, TOL2316 validate() check; gtos two-phase AA already in `state_transition.go` |
| P1 | `delegation` type + `@delegated` + nonce/replay/domain-sep + revocation + sub-delegation | Pillar 5: AI-mediated UX | ✅ Done — `delegation` type, `delegation.verify/consume/revoke()`, `d.subdelegate()`, `d.principal/delegate/scope/is_valid` properties, `@delegated` annotation, ABI field, gtos LVM hooks all implemented |
| P1 | `manifest {}` block (`spec_hash`+`spec_uri`; per-function `price_per_call` map) | Pillar 4: Discoverability | ✅ Done (string/numeric/array values, `;` separator) |
| P2 | `@pay(amount, recipient:)` — compile-time constant; non-view only; safe refund semantics | Pillar 2: M2M micropayments | ✅ Done — bare `@pay(amount)` and `@pay(amount, recipient: expr)` both implemented |
| P2 | `@quota(calls:, price:)` — prepaid N-call bundles; exact depletion math (fixed gas) | Pillar 2: High-freq M2M | ✅ Done — compiler preamble (per-call decrement guard), ABI field `quota_calls`/`quota_price` emitted |
| P2 | `@total_cost(max:)` + cost-certifiable `.toc` ABI — machine-negotiable service market | Pillar 2 + Pillar 5 | ✅ Done — TOL2209 validates `gas_upper * 10gwei + pay_amount ≤ max`; ABI field `declared_total_cost_max` emitted |
| P2 | `escrow` / `slash(recipient:)` / `release(amount, purpose:)` — escrow-only slash | Pillar 3: Collateralized trust | ✅ Done (2-arg and 3-arg forms; purpose defaults to 0 when omitted) |
| P2 | `task<T>` storage type (VM-native state machine; AgentRequired on post; runtime reverts) | Pillar 1: Agent hiring | ✅ Done (mapping-of-task OOP: `.new/.accept/.submit/.approve/.reject/.dispute/.cancel`; local handle `task<T> t = tasks[id]`) |
| P3 | `oracle<T>` write-once (VM-level guard; `@requires` for access control) | Pillar 1: Prediction markets | ✅ Done (`.fulfill()`, `.is_set`, `.value`; write-once enforced in prelude) |
| P3 | `vote<T>` (snapshot quorum; tie param; non-bool ordinal tie-break; capability-gated) | Pillar 6: Governance | ✅ Done — `.cast()`, `.new()`, `.is_decided`, `.result`, `.vote_count`, `.yes_count`, `.no_count` all wired |
| P3 | `@verifiable` on `view` only (ZK integrity, NOT ZK privacy); stub `verify_*`; constraints | Tech: ZK integrity | ✅ Done — annotation, ABI field, and auto-generated `verify_*` stub entry points all implemented |

---

## Design Principles

**1. Every primitive has exactly one authoritative form.**
There is one authorization mechanism (`@requires` + `capability`), one payment mechanism
(`@pay`), one collateral mechanism (`escrow`/`slash`/`release`). Aliases and overlapping
mechanisms (e.g., `@authorized` alongside `@requires`) are not added.

**2. Implicit behavior is rejected.**
`slash` without a `recipient`, `release` without an `amount`, `@pay` without a `recipient` —
all are compile errors. Making economic intent explicit is more important than brevity.

**3. Runtime enforcement over compile-time illusion.**
Task state transitions, oracle write-once, and capability checks are runtime reverts backed by
VM storage semantics. The compiler does not claim to enforce things it cannot know statically
(cross-transaction state). Compile-time checks are reserved for things the compiler genuinely
knows: type mismatches, incompatible annotation combinations (`@pay` + `view`,
`@verifiable` + non-view), undeclared capability names.

**4. The `agent` type is the only identity primitive.**
`agent` is used exclusively within agent protocol contracts. Functions that accept raw identity values where `agent` is semantically intended are
a type error.

**5. Authorization is layered, not flat.**
`capability` answers "is this agent type allowed?" (global, role-based).
`delegation` answers "has this principal authorized this agent for this action?" (scoped, time-bound).
`account contract` answers "does this wallet approve this transaction?" (per-wallet, programmable).
The three layers compose: a `@delegated` function that also has `@requires(caller: Arbitrator)`
requires the caller to satisfy both the capability check AND present a valid delegation. Neither
layer replaces the other.

**6. Fixed gas price is a language feature, not just an economic detail.**
`@total_cost`, `@quota`, and cost-certifiable ABI are only correct because gas price is a
consensus constant. Features that would require a gas price oracle on other chains are first-class
language constructs here. The compiler must refuse to emit `@total_cost` values for contracts
deployed to chains without a fixed gas price guarantee.
