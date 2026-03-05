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

This is why TOL demands strict, machine-verifiable guarantees — not out of language purism, but because the failure modes at agent scale are categorically different from those at human scale.

---

## Three Pillars

### 1. Verifiable — Know what will happen before calling

TOL functions carry compiler-verified `@effects` annotations directly in the `.toc` ABI artifact:

```tol
/**
 * @notice Transfers `amount` tokens from caller to `to`.
 * @effects reads:  storage.balances[caller], storage.allowances[caller,to]
 * @effects writes: storage.balances[caller], storage.balances[to]
 * @effects emits:  Transfer
 * @effects calls:  []
 * @gas     upper:  50000
 */
fn transfer(to: address, amount: u256) -> (ok: bool) external {
    ...
}
```

The compiler checks that `effects_actual ⊆ effects_declared`. If the implementation reads a storage slot that is not listed, compilation fails. The resulting `.toc` artifact binds these declarations to the `BytecodeHash`, so any caller — human or agent — can trust the metadata without re-reading the source.

An agent reading this ABI knows, before sending a single transaction:
- Exactly which storage keys may change.
- Which event will be emitted.
- No external calls — no re-entrancy risk.
- Maximum gas to budget.

Without verifiable effects, an agent must either *simulate blindly* (expensive and slow) or *call blindly* (risk at scale). TOL eliminates the dilemma.

### 2. Metered — Know the cost before committing

Every bounded TOL function can carry a static gas upper bound:

```tol
/// @bounds positions_len <= 64
/// @gas     upper: 8200 + positions_len * 420 + OracleCap.max_gas
fn settle(n: u256) external { ... }
```

The compiler verifies this bound against a conservative cost model (SSTORE, SLOAD, LOG, external call budgets). If the declared bound is too low, compilation fails.

For agents operating in task markets, oracle networks, or prediction markets, this matters enormously:

- Estimate cost → decide whether to execute.
- Compute fee, quote, or margin *before* the transaction.
- Give external partners a verifiable SLA with a concrete price.

Without a verified upper bound, agents must choose between being conservatively idle (low utilization) or being drained by gas-trap contracts (uncontrolled cost).

### 3. Composable — Combine modules with contracts, not conventions

External calls in TOL are expressed as capability references:

```tol
/// @effects calls: cap:OracleCap iface:IOracle selector:0x12345678 max_gas:3000 max_calls:1 max_depth:1
```

This is not documentation — it is a machine-checkable authorization boundary. A module is only allowed to call `oracle.getPrice`, not any other method. It may call it at most once, with a bounded gas budget, at a call depth of at most 1.

Without capability boundaries, composing contracts means trusting that every module behaves correctly by convention. Agents need *machine-checkable* authorization, because they cannot manually audit every call chain before orchestrating it.

A function that makes arbitrary external calls is marked `non_composable: true` in the ABI JSON — a fast-path signal for tooling that this function requires additional human review before automation.

---

## The Language

TOL (The Open Language) is a statically-typed, Solidity-inspired language. Developers familiar with Solidity will find the same type system, storage model, ABI, events, modifiers, and inheritance. The surface syntax is deliberately explicit:

- Storage writes require `set`.
- Local variables require `let`.
- No implicit casts, no hidden state transitions, no assembly escape hatches.

```tol
source tolang 0.2.0

contract TRC20 {
    storage {
        slot total_supply: u256;
        slot balances:     mapping(address => u256);
        slot allowances:   mapping(address => mapping(address => u256));
    }

    event Transfer(from: address indexed, to: address indexed, value: u256)
    event Approval(owner: address indexed, spender: address indexed, value: u256)

    constructor(initialSupply: u256) {
        let owner: address = msg.sender;
        set total_supply = initialSupply;
        set balances[owner] = initialSupply;
        emit Transfer("0x0000000000000000000000000000000000000000", owner, initialSupply);
        return;
    }

    /**
     * @effects reads:  storage.balances[caller]
     * @effects writes: []
     * @effects emits:  []
     * @effects calls:  []
     * @gas     upper:  2500
     */
    fn balanceOf(owner: address) -> (balance: u256) public view {
        return balances[owner];
    }

    /**
     * @effects reads:  storage.balances[caller]
     * @effects writes: storage.balances[caller], storage.balances[to]
     * @effects emits:  Transfer
     * @effects calls:  []
     * @gas     upper:  50000
     */
    fn transfer(to: address, amount: u256) -> (ok: bool) external {
        require(balances[msg.sender] >= amount, "INSUFFICIENT_BALANCE");
        set balances[msg.sender] -= amount;
        set balances[to] += amount;
        emit Transfer(msg.sender, to, amount);
        return true;
    }
}
```

See [`examples/trc20_tol/`](examples/trc20_tol/) for a complete TRC20 implementation.

---

## The VM

TOL compiles to a hardened, deterministic Lua 5.4 VM. The VM is stripped of all non-deterministic operations — no I/O, no floating-point, no OS access — and extended with:

- **256-bit integer arithmetic** — `LUint256` is a native `struct{lo, ml, mh, hi uint64}`. All hot-path operations (add, sub, mul, bitwise, shift, compare) use `math/bits` with zero allocations. Division uses Knuth Algorithm D natively. No `math/big` in production paths.
- **Gas metering** — every VM instruction increments a counter; execution halts deterministically when the limit is reached.
- **Signed integer intrinsics** — `i8..i256` two's-complement arithmetic built on the same native primitives.

```
TOL source → sema → lower → codegen → Lua bytecode → VM
Lua source  →                          Lua bytecode → VM
```

Both paths produce the same bytecode format and run on the same VM.

### Standard libraries

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
| `.toc` | Artifact | Compiled bytecode + ABI JSON (with `@effects`) + source/bytecode hashes |
| `.abi` | Interface | Human- and machine-readable interface definition for cross-contract typing |
| `.tor` | Package | Deterministic ZIP archive: manifest + `.toc` + `.abi` + sources |

All artifacts are content-addressed. The `.toc` `BytecodeHash` field covers exactly the bytecode that was verified against the effect declarations, making the ABI JSON as trustworthy as the bytecode itself.

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
source tolang 0.2.0

test TRC20Suite {
    setup {
        deploy TRC20(1000000) -> token;
    }

    fn test_transfer() {
        with msg.sender = "0x0000000000000000000000000000000000000001" {
            assert_eq(token.transfer("0x0000000000000000000000000000000000000002", 100), true);
        }
        assert_eq(token.balanceOf("0x0000000000000000000000000000000000000002"), 100);
    }

    fn test_insufficient_balance() {
        assert_revert(fn() { token.transfer("0x0000000000000000000000000000000000000002", 9999999); });
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

The Lua VM implementation builds on the work of [yuin/gopher-lua](https://github.com/yuin/gopher-lua) (MIT).
