<p align="center">
  <img src="tolang.png" width="120" alt="TOL logo">
</p>

# TOL — The Open Language

**A smart contract language built for Agents: verifiable, metered, and composable.**

---

## Why Agent-Native?

Traditional smart contracts are designed to be *human-readable*. A developer reads the source, audits the logic, and makes a judgment call. When something goes wrong, a human can stop, review, and recover.

Agents are different:

- **High-frequency** — thousands of calls per day, per strategy, per account.
- **Automated** — no gut feeling that says "this seems wrong, let me pause."
- **Contagious** — one flawed strategy can replicate instantly across thousands of agents.
- **Compositional** — multiple contracts, modules, and external services are chained together automatically.

The root problem is not that agents make mistakes. It is that **mistakes get automated and amplified.** A human auditor catching a bug in one contract saves one contract. An agent framework that can *verify* contracts before executing them saves every contract in the ecosystem.

TOL elevates **identity, task, capability, payment, and trust** to first-class language primitives — the same leap Solidity made when it introduced `msg.sender`, `payable`, `transfer()`, and `msg.value`.

---

## Three Pillars

### 1. Verifiable — Know what will happen before calling

TOL functions carry compiler-verified `@effects` annotations directly in the `.toc` ABI artifact:

```tol
/**
 * @effects reads:  storage.balances[caller], storage.allowances[caller,to]
 * @effects writes: storage.balances[caller], storage.balances[to]
 * @effects emits:  Transfer
 * @effects calls:  []
 * @gas     upper:  50000
 */
function transfer(agent to, u256 amount) external returns (bool ok) {
    require(balances[msg.sender] >= amount, "INSUFFICIENT_BALANCE");
    balances[msg.sender] -= amount;
    balances[to] += amount;
    emit Transfer(msg.sender, to, amount);
    return true;
}
```

The compiler checks that `effects_actual ⊆ effects_declared`. If the implementation reads a storage slot that is not listed, compilation fails. The resulting `.toc` artifact binds these declarations to the `BytecodeHash`, so any caller — human or agent — can trust the metadata without re-reading the source.

An agent reading this ABI knows, before sending a single transaction:
- Exactly which storage keys may change.
- Which event will be emitted.
- No external calls — no re-entrancy risk.
- Maximum gas to budget.

### 2. Metered — Know the cost before committing

Every bounded TOL function can carry a static gas upper bound:

```tol
/// @bounds positions_len <= 64
/// @gas     upper: 8200 + positions_len * 420 + OracleCap.max_gas
function settle(u256 n) external { ... }
```

The compiler verifies this bound against a conservative cost model (SSTORE, SLOAD, LOG, external call budgets). If the declared bound is too low, compilation fails.

For agents operating in task markets, oracle networks, or prediction markets, this matters enormously:
- Estimate cost → decide whether to execute.
- Compute fee, quote, or margin *before* the transaction.
- Give external partners a verifiable SLA with a concrete price.

### 3. Composable — Combine modules with contracts, not conventions

External calls in TOL are expressed as capability references:

```tol
/// @effects calls: cap:OracleCap iface:IOracle selector:0x12345678 max_gas:3000 max_calls:1 max_depth:1
```

This is not documentation — it is a machine-checkable authorization boundary. A module is only allowed to call `oracle.getPrice`, not any other method. It may call it at most once, with a bounded gas budget, at a call depth of at most 1.

A function that makes arbitrary external calls is marked `non_composable: true` in the ABI JSON — a fast-path signal for tooling that this function requires additional human review before automation.

---

## Agent-Native Primitives

TOL 0.3 introduces five first-class primitives that eliminate boilerplate guards and manual state machines in agent contracts.

### `capability` — Permission Declarations

```tol
capability Registrar;    // may register / suspend agents
capability Arbitrator;   // may rule on disputes
capability OracleResolver;
```

Use `@requires(caller: X)` to gate a function. The compiler inserts the capability check automatically:

```tol
@requires(caller: Arbitrator)
function rule(u256 dispute_id, agent winner, u16 slash_bps, string reason) public { ... }
```

Emitted in the `.toc` ABI as `"requires_capability": "Arbitrator"` — so any agent can check authorization without reading source.

### `oracle<T>` — Write-Once Value Slot

```tol
oracle<u8> winning_outcome;   // write-once; second fulfill() is a hard revert
```

```tol
@requires(caller: OracleResolver)
function resolve(u8 outcome) public {
    winning_outcome.fulfill(outcome);   // compiler prevents double-resolution
    emit Resolved(outcome, agent(msg.sender));
}

function claim() public {
    require(winning_outcome.is_set, "NotResolved");
    u256 payout = shares[msg.sender][winning_outcome.value] * totalPool / totalShares;
    release(agent(msg.sender), payout);
}
```

### `task<T>` — Compiler-Enforced State Machine

Replaces `mapping(u256 => u8)` + raw numeric codes. The compiler tracks valid transitions:

```
Open → Accepted → Submitted → Approved
                ↘ Rejected  → Disputed
Open → Cancelled
```

```tol
mapping(u256 => task<bytes32>) tasks;

function postTask(bytes32 spec_hash, u64 deadline_ms) public payable returns (u256 task_id) {
    task_id = next_task_id++;
    tasks[task_id] = task<bytes32>.new(agent(msg.sender), msg.value, deadline_ms);
    escrow(agent(msg.sender), msg.value);
}

function approveTask(u256 task_id) public {
    task<bytes32> t = tasks[task_id];
    t.approve();                          // compile error if status != Submitted
    release(t.worker, t.reward);
}
```

### `agent` — Identity with Native Properties

`agent` is TOL's native identity type — registry semantics are built in, no manual `require(registry.isActive(addr))` needed:

```tol
function updateProfile(string metadata_uri) public {
    require(agent(msg.sender).is_active, "NotActive");   // reads from protocol registry
    emit AgentProfileUpdated(agent(msg.sender), metadata_uri);
}

function totalScoreOf(agent who) public view returns (i256 score) {
    return who.reputation;       // agent property — no storage lookup boilerplate
}
```

Available properties: `.stake`, `.is_active`, `.reputation`, `.rating_count`, `.suspended`.

### `escrow` / `release` / `slash` — Intent-Explicit Payment

```tol
escrow(agent(msg.sender), msg.value);          // lock funds against agent identity
release(t.worker, t.reward);                   // unlock reward to worker
slash(loser, loser.stake * slash_bps / 10000); // penalize + route proceeds
```

### `manifest {}` — Machine-Readable Contract Metadata

```tol
manifest {
    version:      "1.0.0";
    capabilities: [Arbitrator];
    spec:         "ipfs://Qm_task_escrow_spec";
    sla_uptime:   9900;
}
```

Emitted verbatim into the `.toc` ABI JSON as a top-level `"manifest"` key — inspectable by tooling without executing the contract.

### `@pay` / `@verifiable` / `@delegated`

```tol
@pay(10_000_000)                  // enforces msg.value >= fee at language level
function createBinaryMarket(...) public returns (agent market) { ... }

@verifiable                       // result provable off-chain; ABI field "verifiable": true
function totalScoreOf(agent who) public view returns (i256 score) { ... }

@delegated                        // accepts delegated calls; injects delegation verify
function acceptJob(u256 tid) external payable { ... }
```

---

## Language

TOL is a statically-typed, Solidity-inspired language. Developers familiar with Solidity will recognize the same type system, storage model, ABI encoding, events, modifiers, and inheritance. The surface syntax is deliberately explicit — no implicit casts, no hidden state transitions.

```tol
pragma tolang 0.3.0;

contract TRC20 {
    u256                             total_supply;
    mapping(agent => u256)           balances;
    mapping(agent => mapping(agent => u256)) allowances;

    constant NOBODY: agent = "0x0000000000000000000000000000000000000000000000000000000000000000";

    event Transfer(agent indexed from, agent indexed to, u256 value)
    event Approval(agent indexed owner, agent indexed spender, u256 value)

    constructor(u256 initialSupply) {
        total_supply = initialSupply;
        balances[msg.sender] = initialSupply;
        emit Transfer(NOBODY, msg.sender, initialSupply);
    }

    /**
     * @effects reads:  storage.balances[caller]
     * @effects writes: storage.balances[caller], storage.balances[to]
     * @effects emits:  Transfer
     * @gas     upper:  50000
     */
    function transfer(agent to, u256 amount) external returns (bool ok) {
        require(balances[msg.sender] >= amount, "INSUFFICIENT_BALANCE");
        balances[msg.sender] -= amount;
        balances[to] += amount;
        emit Transfer(msg.sender, to, amount);
        return true;
    }

    function balanceOf(agent owner) external view returns (u256 balance) {
        return balances[owner];
    }
}
```

---

## Agent-Native Example

The following excerpt shows a prediction market where double-resolution is structurally impossible and payout is automatic — no manual guard required:

```tol
pragma tolang 0.3.0;

capability OracleResolver;

contract PredictionMarket is IPredictionMarket {
    manifest {
        version:      "1.0.0";
        capabilities: [OracleResolver];
        spec:         "ipfs://Qm_prediction_market_spec";
    }

    oracle<u8> winning_outcome;            // write-once; second .fulfill() reverts
    mapping(u8 => u256)                    pool;
    mapping(agent => mapping(u8 => u256)) shares;

    @requires(caller: OracleResolver)
    function resolve(u8 outcome) public {
        require(block.timestamp_ms >= close_time_ms, "MarketNotClosed");
        winning_outcome.fulfill(outcome);  // compiler enforces single-write
        emit Resolved(outcome, agent(msg.sender));
    }

    function claim() public {
        require(winning_outcome.is_set, "NotResolved");
        u256 payout = shares[msg.sender][winning_outcome.value]
                      * (pool[0] + pool[1])
                      / pool[winning_outcome.value];
        release(agent(msg.sender), payout);
        emit Claimed(agent(msg.sender), payout);
    }
}
```

See [`docs/AGENT_PROTOCOL_DRAFT2.tol`](docs/AGENT_PROTOCOL_DRAFT2.tol) for a complete agent economy protocol (registry, reputation, task escrow, dispute resolution, prediction market, reward vault).

---

## The VM

TOL compiles to a hardened, deterministic Lua 5.4 VM. All non-deterministic operations are removed — no I/O, no floating-point, no OS access — and the VM is extended with:

- **256-bit integer arithmetic** — `LUint256` is a native `struct{lo, ml, mh, hi uint64}`. All hot-path operations (add, sub, mul, bitwise, shift, compare) use `math/bits` with zero allocations. No `math/big` in production paths.
- **Gas metering** — every VM instruction increments a counter; execution halts deterministically when the limit is reached.
- **Signed integer intrinsics** — `i8..i256` two's-complement arithmetic built on the same native primitives.

```
TOL source → sema → lower → codegen → Lua bytecode → VM
Lua source  →                          Lua bytecode → VM
```

Both paths produce the same bytecode format and run on the same VM.

### Standard Libraries

| Library | Status |
|---------|--------|
| base (restricted) | available |
| table | available |
| string | available |
| math (integer subset) | available |
| io, os, debug, coroutine | **removed** |

Removed from base: `print`, `dofile`, `loadfile`, `load`, `loadstring`, `require`, `collectgarbage`.

---

## Artifact Formats

The TOL toolchain produces three artifact types:

| Extension | Name | Contents |
|-----------|------|----------|
| `.toc` | Artifact | Compiled bytecode + ABI JSON (with `@effects`, `manifest`, capability fields) + source/bytecode hashes |
| `.abi` | Interface | Human- and machine-readable interface definition for cross-contract typing |
| `.tor` | Package | Deterministic ZIP archive: manifest + `.toc` + `.abi` + sources |

All artifacts are content-addressed. The `.toc` `BytecodeHash` field covers exactly the bytecode that was verified against the effect and capability declarations, making the ABI JSON as trustworthy as the bytecode itself.

Example `.toc` ABI excerpt for an agent-native function:

```json
{
  "name": "rule",
  "visibility": "public",
  "selector": "0x...",
  "params": ["u256", "agent", "u16", "string"],
  "returns": [],
  "requires_capability": "Arbitrator",
  "verifiable": false,
  "delegated": false
}
```

---

## CLI

```bash
go build -o bin/tol ./cmd/tolang
```

```
tol compile Contract.tol             # compile to .toc artifact
tol compile --emit abi Contract.tol  # compile to .abi interface
tol compile --emit tor ./pkg/        # compile to .tor package
tol inspect artifact.toc             # show metadata (text)
tol inspect --json artifact.toc      # show metadata (JSON)
tol verify artifact.toc              # verify artifact integrity
tol verify --source src.tol a.toc    # verify + source hash check
tol pack -o out.tor ./contracts/     # package directory into .tor
tol test ./contracts/                # run *_test.tol test files
tol --help
```

---

## Test Framework

Write tests alongside contracts in `*_test.tol` files:

```tol
pragma tolang 0.3.0;

test TRC20Suite {
    setup {
        deploy TRC20(1000000) -> token;
    }

    function test_transfer() {
        with msg.sender = 0x0000000000000000000000000000000000000001 {
            assert_eq(token.transfer(0x0000000000000000000000000000000000000002, 100), true);
        }
        assert_eq(token.balanceOf(0x0000000000000000000000000000000000000002), 100);
    }

    function test_insufficient_balance() {
        assert_revert(function() { token.transfer(0x0000000000000000000000000000000000000002, 9999999); });
    }
}
```

```bash
tol test ./examples/trc20_tol/
# ok   ./examples/trc20_tol/trc20_test.tol   (10 tests, 0 failures)
```

Coverage gates:

```bash
tol test --cover --covermin 80 ./contracts/
```

---

## Embedding

```go
import lua "github.com/tos-network/tolang"

L := lua.NewState()
defer L.Close()

L.SetGasLimit(1_000_000)
L.RegisterModule("chain", map[string]lua.LGFunction{
    "get": getStorage,
    "set": setStorage,
})

if err := L.DoString(src); err != nil {
    // handle error
}
fmt.Println("gas used:", L.GasUsed())
```

Compile TOL source directly:

```go
// Compile to bytecode artifact (.toc)
artifact, err := lua.CompileArtifact(src, "TRC20")

// Or compile to raw bytecode
bc, err := lua.CompileBytecode(src, "TRC20")
err = L.DoBytecode(bc)
```

---

## License

MIT. See [LICENSE](LICENSE).
