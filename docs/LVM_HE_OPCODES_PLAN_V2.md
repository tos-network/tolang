# Plan V2: LVM Confidential Computing Primitives for Privacy Smart Contracts

## Context

GTOS privacy (UNO) currently works only at the protocol level — Shield/Transfer/Unshield
are native tx types. Smart contracts cannot manipulate encrypted values, meaning no
confidential tokens, no private voting, no private OTC via contracts.

We add the `Uno` type to TOL — a first-class encrypted integer backed by Twisted ElGamal
ciphertexts (64 bytes = commitment 32B + handle 32B, Ristretto255). The `Uno` type
exposes 22 methods giving TOL contracts **Fhenix-equivalent encrypted arithmetic** — add,
sub, mul, div, compare, select, min, max — with clean method-call syntax.

**Key design choices**:
- `Uno` is a built-in TOL type with method syntax: `a.add(b)`, `a.lt(b)`, `Uno.zero()`
- The TOL compiler desugars method calls to `tos.ciphertext.*` Lua calls (LVM unchanged)
- Operations that ElGamal cannot compute natively (mul, div, compare, select, min, max)
  use a **proof bundle** — the caller pre-computes results off-chain and attaches ZK
  proofs in the transaction; the LVM verifies automatically
- Contract developers never handle proofs — they write `a.mul(b)` and it just works

## Design

### Cryptographic Foundation

| Property | GTOS | Fhenix |
|----------|------|--------|
| Scheme | Twisted ElGamal (additive homomorphic) | TFHE (fully homomorphic) |
| Curve | Ristretto255 | Lattice-based |
| Ciphertext size | **64 bytes** | ~KB |
| ct + ct | **native** (EC point add) | native (but slow) |
| ct × ct | **proof bundle** (ZK verify) | native (but very slow) |
| Execution | **on-chain in LVM** | off-chain coprocessor |
| Trust model | **math (ZK unforgeable)** | coprocessor honest |
| ct + ct latency | **~0.1ms** | ~10-50ms |

### Two Tiers of Operations

**Tier 1 — Native homomorphic** (LVM computes directly, no proof needed):

```
a.add(b)            →  Enc(a+b)      // EC point addition
a.sub(b)            →  Enc(a-b)      // EC point subtraction
a.add_scalar(n)     →  Enc(a+n)      // scalar-mult + point-add
a.sub_scalar(n)     →  Enc(a-n)      // scalar-mult + point-sub
a.mul_scalar(n)     →  Enc(a×n)      // scalar-mult (repeated addition)
a.div_scalar(n)     →  Enc(a/n)      // scalar-inverse + scalar-mult
```

**Tier 2 — Proof-bundle verified** (caller pre-computes, LVM verifies ZK proof):

```
a.mul(b)            →  Enc(a×b)      // Sigma protocol: Com(c) = Com(a×b)
a.div(b)            →  Enc(a÷b)      // proves a = b×q + r, r ∈ [0,b)
a.rem(b)            →  Enc(a%b)      // remainder from div proof
a.lt(b)             →  bool          // sub(b,a) + range proof
a.gt(b)             →  bool          // sub(a,b) + range proof
a.eq(b)             →  bool          // sub(a,b) + zero-commitment check
a.min(b)            →  Enc(min)      // compare + selection proof
a.max(b)            →  Enc(max)      // compare + selection proof
Uno.select(c, a, b) →  Enc(c?a:b)   // conditional selection proof
a.verify_transfer(sPub, rPub) → bool // CT validity (proof from bundle)
a.verify_eq(commit, pubkey)   → bool // commitment equality (proof from bundle)
```

### Proof Bundle Mechanism

Transactions carry a proof bundle appended to standard calldata:

```
┌──────────────────────────────┐
│  Standard ABI calldata       │
├──────────────────────────────┤
│  Proof bundle                │
│  ┌────────────────────────┐  │
│  │ entry[0]:              │  │
│  │   op = "mul"           │  │
│  │   inputs = hash(a, b)  │  │
│  │   result = Enc(c)      │  │
│  │   proof  = π           │  │
│  ├────────────────────────┤  │
│  │ entry[1]:              │  │
│  │   op = "lt"            │  │
│  │   inputs = hash(a, b)  │  │
│  │   result = true        │  │
│  │   proof  = π           │  │
│  └────────────────────────┘  │
└──────────────────────────────┘
```

When the LVM encounters a Tier 2 operation:
1. Pop the next entry from the proof bundle
2. Verify `entry.inputs == hash(op, a, b)` (matches the operation)
3. Verify the ZK proof against the inputs and claimed result
4. Charge gas
5. Return the result
6. If bundle is exhausted or proof is invalid → revert

Contract developers never see this — they just write `a.mul(b)`.
The user's wallet/SDK generates the proof bundle when constructing the transaction.

### TOL Syntax: `Uno` Type

`Uno` is a built-in TOL type. The compiler desugars method calls to `tos.ciphertext.*`
Lua function calls. No LVM changes needed — only the TOL compiler front-end.

```tol
// TOL source (what developers write)       // Lua output (what LVM executes)
Uno x = Uno.zero();                         // x = tos.ciphertext.zero()
Uno y = a.add(b);                           // y = tos.ciphertext.add(a, b)
bool ok = a.lt(b);                          // ok = tos.ciphertext.lt(a, b)
Uno m = a.min(b);                           // m = tos.ciphertext.min(a, b)
Uno s = Uno.select(cond, a, b);             // s = tos.ciphertext.select(cond, a, b)
bytes32 c = a.commitment();                 // c = tos.ciphertext.commitment(a)
bool v = a.verify_transfer(sPub, rPub);     // v = tos.ciphertext.verify_transfer(a, sPub, rPub)
```

### Type Representation

| Type | In TOL | In Lua | In Storage | In ABI | Size |
|------|--------|--------|------------|--------|------|
| `Uno` | first-class type | `"0x" + 128 hex` | 2 × 32B slots | 2 × 32B words | 64B |

### API: 22 Methods

#### Tier 1 — Native Homomorphic (9 methods)

| # | TOL Method | Signature | Gas | Implementation |
|---|------------|-----------|-----|----------------|
| 1 | `a.add(b)` | `(Uno, Uno) → Uno` | 8000 | EC point add (commitment + handle) |
| 2 | `a.sub(b)` | `(Uno, Uno) → Uno` | 8000 | EC point sub (commitment + handle) |
| 3 | `a.add_scalar(n)` | `(Uno, u64) → Uno` | 6000 | scalar-mult(G,n) + point-add |
| 4 | `a.sub_scalar(n)` | `(Uno, u64) → Uno` | 6000 | scalar-mult(G,n) + point-sub |
| 5 | `a.mul_scalar(n)` | `(Uno, u64) → Uno` | 10000 | 2× variable-base scalar-mult |
| 6 | `a.div_scalar(n)` | `(Uno, u64) → Uno` | 12000 | scalar-invert + 2× scalar-mult |
| 7 | `Uno.zero()` | `() → Uno` | 100 | precomputed identity ciphertext |
| 8 | `Uno.encrypt(pk, amt)` | `(bytes32, u64) → Uno` | 15000 | full ElGamal encryption |
| 9 | `Uno.from_parts(c, h)` | `(bytes32, bytes32) → Uno` | 100 | concatenation |

#### Tier 2 — Proof-Bundle Verified (9 methods)

| # | TOL Method | Signature | Gas | Proof type |
|---|------------|-----------|-----|------------|
| 10 | `a.mul(b)` | `(Uno, Uno) → Uno` | 200000 | Sigma: Com(c) = Com(a×b) |
| 11 | `a.div(b)` | `(Uno, Uno) → Uno` | 200000 | Sigma: a = b×q + r, r∈[0,b) |
| 12 | `a.rem(b)` | `(Uno, Uno) → Uno` | 200000 | remainder from div proof |
| 13 | `a.lt(b)` | `(Uno, Uno) → bool` | 160000 | sub + 64-bit range proof |
| 14 | `a.gt(b)` | `(Uno, Uno) → bool` | 160000 | sub + 64-bit range proof |
| 15 | `a.eq(b)` | `(Uno, Uno) → bool` | 150000 | sub + zero-commitment check |
| 16 | `a.min(b)` | `(Uno, Uno) → Uno` | 170000 | compare + selection proof |
| 17 | `a.max(b)` | `(Uno, Uno) → Uno` | 170000 | compare + selection proof |
| 18 | `Uno.select(c, a, b)` | `(bool, Uno, Uno) → Uno` | 160000 | conditional selection proof |

#### Accessors (2 methods)

| # | TOL Method | Signature | Gas | Purpose |
|---|------------|-----------|-----|---------|
| 19 | `a.commitment()` | `(Uno) → bytes32` | 100 | extract commitment point (first 32B) |
| 20 | `a.handle()` | `(Uno) → bytes32` | 100 | extract handle point (last 32B) |

#### Proof-Bundle Verification (2 methods)

| # | TOL Method | Signature | Gas | Purpose |
|---|------------|-----------|-----|---------|
| 21 | `a.verify_transfer(sPub, rPub)` | `(Uno, bytes32, bytes32) → bool` | 100000 | CT validity: same amount under both keys (proof from bundle) |
| 22 | `a.verify_eq(commit, pubkey)` | `(Uno, bytes32, bytes32) → bool` | 100000 | commitment equality: ct and commit bind same value (proof from bundle) |

### Comparison with Fhenix

| Operation | Fhenix (Solidity) | GTOS (TOL) | Parity |
|-----------|-------------------|------------|--------|
| add | `FHE.add(ea, eb)` | `a.add(b)` | ✅ |
| sub | `FHE.sub(ea, eb)` | `a.sub(b)` | ✅ |
| mul | `FHE.mul(ea, eb)` | `a.mul(b)` | ✅ |
| div | `FHE.div(ea, eb)` | `a.div(b)` | ✅ |
| rem | `FHE.rem(ea, eb)` | `a.rem(b)` | ✅ |
| lt / gt | `FHE.lt(ea, eb)` | `a.lt(b)` | ✅ |
| eq | `FHE.eq(ea, eb)` | `a.eq(b)` | ✅ |
| min / max | `FHE.min(ea, eb)` | `a.min(b)` | ✅ |
| select | `FHE.select(c, ea, eb)` | `Uno.select(c, a, b)` | ✅ |
| ct + scalar | ❌ | `a.add_scalar(n)` | **GTOS extra** |
| ct × scalar | ❌ | `a.mul_scalar(n)` | **GTOS extra** |
| transfer proof | ❌ | `a.verify_transfer(...)` | **GTOS extra** |
| commitment eq | ❌ | `a.verify_eq(...)` | **GTOS extra** |

## Changes

### 1. `~/gtos/core/vm/lvm_crypto.go` — New File

Single registration function called from `Execute()`:

```go
func registerCryptoTable(L *lua.LState, tosTable *lua.LTable, stateDB StateDB,
    contractAddr common.Address, gas *uint64, readonly bool,
    proofBundle *ProofBundle)
```

**Gas constants:**

```go
const (
    // Tier 1 — native homomorphic
    gasCtAdd       uint64 = 8000
    gasCtSub       uint64 = 8000
    gasCtAddScalar uint64 = 6000
    gasCtSubScalar uint64 = 6000
    gasCtMulScalar uint64 = 10000
    gasCtDivScalar uint64 = 12000
    gasCtZero      uint64 = 100
    gasCtEncrypt   uint64 = 15000
    gasCtFromParts uint64 = 100
    gasCtAccessor  uint64 = 100

    // Tier 2 — proof-bundle verified
    gasCtMul       uint64 = 200000
    gasCtDiv       uint64 = 200000
    gasCtRem       uint64 = 200000
    gasCtLt        uint64 = 160000
    gasCtGt        uint64 = 160000
    gasCtEq        uint64 = 150000
    gasCtMin       uint64 = 170000
    gasCtMax       uint64 = 170000
    gasCtSelect    uint64 = 160000

    // Verification
    gasCtVerifyTransfer uint64 = 100000
    gasCtVerifyEq       uint64 = 100000
)
```

**Internal helpers:**

```go
func parseCiphertextHex(s string) ([]byte, error)  // "0x"+128hex → 64 bytes
func ciphertextToHex(ct []byte) string              // 64 bytes → "0x"+128hex
func ctCommitSlot(key string) common.Hash           // keccak256("gtos.lua.ct." + key)
func ctHandleSlot(key string) common.Hash           // keccak256("gtos.lua.ct." + key + ".handle")
func mapCtCommitSlot(mapName string, keys []string) common.Hash
func mapCtHandleSlot(mapName string, keys []string) common.Hash
```

**ProofBundle type:**

```go
// ProofBundle holds pre-computed results and ZK proofs for Tier 2 operations.
// Extracted from transaction calldata at the start of LVM execution.
// Entries are consumed sequentially as Tier 2 operations execute.
type ProofBundle struct {
    entries []ProofEntry
    cursor  int
}

type ProofEntry struct {
    Op         string      // "mul", "div", "rem", "lt", "gt", "eq", "min", "max", "select"
    InputHash  common.Hash // keccak256(op || input_a || input_b)
    ResultData []byte      // encrypted result (64B ciphertext or 1B bool)
    Proof      []byte      // ZK proof bytes
}

// Next pops the next entry, verifying it matches the expected operation and inputs.
func (pb *ProofBundle) Next(op string, a, b []byte) (*ProofEntry, error)
```

**Sub-table registration** (LVM layer — unchanged from V2, compiler handles Uno→tos.ciphertext mapping):

```go
ctTable := L.NewTable()

// Tier 1 — native homomorphic
L.SetField(ctTable, "add", L.NewFunction(...))
L.SetField(ctTable, "sub", L.NewFunction(...))
L.SetField(ctTable, "add_scalar", L.NewFunction(...))
L.SetField(ctTable, "sub_scalar", L.NewFunction(...))
L.SetField(ctTable, "mul_scalar", L.NewFunction(...))
L.SetField(ctTable, "div_scalar", L.NewFunction(...))
L.SetField(ctTable, "zero", L.NewFunction(...))
L.SetField(ctTable, "encrypt", L.NewFunction(...))
L.SetField(ctTable, "from_parts", L.NewFunction(...))

// Tier 2 — proof-bundle verified
L.SetField(ctTable, "mul", L.NewFunction(...))
L.SetField(ctTable, "div", L.NewFunction(...))
L.SetField(ctTable, "rem", L.NewFunction(...))
L.SetField(ctTable, "lt", L.NewFunction(...))
L.SetField(ctTable, "gt", L.NewFunction(...))
L.SetField(ctTable, "eq", L.NewFunction(...))
L.SetField(ctTable, "min", L.NewFunction(...))
L.SetField(ctTable, "max", L.NewFunction(...))
L.SetField(ctTable, "select", L.NewFunction(...))

// Accessors & verification
L.SetField(ctTable, "commitment", L.NewFunction(...))
L.SetField(ctTable, "handle", L.NewFunction(...))
L.SetField(ctTable, "verify_transfer", L.NewFunction(...))  // (ct, sPub, rPub) → bool; proof from bundle
L.SetField(ctTable, "verify_eq", L.NewFunction(...))        // (ct, commit, pubkey) → bool; proof from bundle

L.SetField(tosTable, "ciphertext", ctTable)
```

### 2. `~/gtos/core/vm/lvm.go` — Execute() Hook

```go
proofBundle := extractProofBundle(ctx.Data)
registerCryptoTable(L, tosTable, l.StateDB, contractAddr, &gas, ctx.Readonly, proofBundle)
```

### 3. `~/gtos/core/vm/lvm_crypto_test.go` — Tests

**Tier 1 tests:**
- `add` then `sub` round-trip
- `add_scalar` / `sub_scalar` round-trip
- `mul_scalar` then `div_scalar` round-trip; `div_scalar(ct, 0)` reverts
- `zero` is additive identity
- `encrypt` produces valid ciphertext
- `from_parts(commitment(ct), handle(ct)) == ct`

**Tier 2 tests (with proof bundles):**
- `mul`: Enc(3) × Enc(7) = Enc(21); wrong result fails
- `div`: Enc(17) ÷ Enc(5) = Enc(3); `rem` = Enc(2)
- `lt`: Enc(3) < Enc(7) = true; Enc(7) < Enc(3) = false
- `gt`: inverse of `lt`
- `eq`: Enc(5) == Enc(5) = true; Enc(5) == Enc(6) = false
- `min(Enc(3), Enc(7))` = Enc(3); `max` = Enc(7)
- `select(true, Enc(3), Enc(7))` = Enc(3)
- Missing proof bundle → revert; tampered proof → revert

**Verification tests:**
- `verify_transfer` with valid/invalid CT validity proof
- `verify_eq` with valid/invalid commitment equality proof

### 4. `~/gtos/core/types/transaction.go` — Proof Bundle Encoding

```go
// ExtractProofBundle parses the proof bundle appended after standard ABI calldata.
// Format: [standard calldata] [0xPBND magic] [entry count u16] [entries...]
// Each entry: [op u8] [input_hash 32B] [result_len u16] [result ...] [proof_len u16] [proof ...]
func ExtractProofBundle(data []byte) (*ProofBundle, []byte)
```

### 5. `~/tolang/tol/sema/sema.go` — Type System

- `isValidAtomicTOLType`: add `"Uno"` case (canonical name; `"ciphertext"` accepted as alias)
- `isValueTOLType`: add `"Uno"` (can be function param/return)
- `isDefaultInitializableTOLType`: add `"Uno"` (default = `Uno.zero()`)
- Do NOT add to `isValidMappingKeyType` (Uno should not be a mapping key)
- Method resolution: when sema sees `expr.method(args)` on an `Uno`-typed expression,
  validate method name against the 22 known methods and check argument types
- `==` and `!=` on Uno compile to `a.eq(b)` / `!a.eq(b)`
- Reject `<`, `>`, `+`, `-`, `*`, `/` operators (must use explicit methods)

### 6. `~/tolang/tol_ir_direct_lowering.go` — Code Generation

**Uno method desugaring** (compiler front-end only, no LVM changes):

```
a.add(b)              →  tos.ciphertext.add(a, b)
a.lt(b)               →  tos.ciphertext.lt(a, b)
a.commitment()        →  tos.ciphertext.commitment(a)
Uno.zero()            →  tos.ciphertext.zero()
Uno.encrypt(pk, amt)  →  tos.ciphertext.encrypt(pk, amt)
Uno.select(c, a, b)   →  tos.ciphertext.select(c, a, b)
```

**Storage for `Uno` type:**

Add `storageKindCiphertext` constant alongside existing scalar/mapping/array.

`classifyStorageSlotKind`: when the leaf type resolves to `"Uno"`, return
`storageKindCiphertext`.

`defaultValueExprForType`: add case for `"Uno"` → `tos.ciphertext.zero()`.

Storage emit paths — for `storageKindCiphertext`:
- **Scalar store**: emit `tos.ciphertext.store(slotKey, value)` (2-slot write)
- **Scalar load**: emit `tos.ciphertext.load(slotKey)` (2-slot read)
- **Mapping store**: emit `tos.ciphertext.map_store(mapName, key1, ..., value)`
- **Mapping load**: emit `tos.ciphertext.map_load(mapName, key1, ...)`

Note: `store/load/map_store/map_load` are internal LVM functions used by the compiler's
code generation. They are NOT part of the 22-method public API — contract developers
use normal TOL assignment syntax: `balances[account] = x;`

### 7. `~/tolang/cryptolib.go` — ABI Encoding

Add `"Uno"` case to ABI encode/decode:
- Encode: 2 consecutive 32-byte words (commitment, handle)
- Decode: consume 2 × 32-byte slots, concatenate as `"0x" + 128 hex`

### 8. `~/tolang/tol/sema/sema_test.go` — Compiler Tests

- `Uno` accepted as state variable, param, return type
- `Uno` in `mapping(agent => Uno)` works
- `Uno` rejected as mapping key
- `a.add(b)` type-checks; `a.add("string")` rejected
- `a.lt(b)` returns `bool`; `a.add(b)` returns `Uno`
- `==` / `!=` compile to `a.eq(b)`; `<`, `>`, `+`, `-` rejected
- `Uno.zero()` is valid; `Uno.encrypt(pk, amt)` is valid
- Unknown method `a.foo()` rejected

## Sample Contract (Confidential TRC20)

```tol
pragma tolang 0.4.0;

contract ConfidentialToken {
    agent minter;
    mapping(agent => Uno) balances;
    mapping(agent => bytes32) publicKeys;

    event PublicKeyRegistered(agent indexed account, bytes32 pubkey)
    event Mint(agent indexed to)
    event ConfidentialTransfer(agent indexed from, agent indexed to)

    constructor(agent _minter) {
        minter = _minter;
    }

    function registerPublicKey(bytes32 pubkey) external {
        publicKeys[msg.sender] = pubkey;
        emit PublicKeyRegistered(msg.sender, pubkey);
    }

    function mint(agent to, Uno mintAmount) external {
        require(msg.sender == minter, "OnlyMinter");
        require(publicKeys[to] != bytes32(0), "NotRegistered");
        balances[to] = balances[to].add(mintAmount);
        emit Mint(to);
    }

    function transfer(agent to, Uno amount) external {
        require(publicKeys[msg.sender] != bytes32(0), "SenderNotRegistered");
        require(publicKeys[to] != bytes32(0), "ReceiverNotRegistered");

        // Verify amount is encrypted under both sender's and receiver's keys
        require(
            amount.verify_transfer(publicKeys[msg.sender], publicKeys[to]),
            "BadTransferProof"
        );

        Uno newBal = balances[msg.sender].sub(amount);
        require(newBal.gt(Uno.zero()), "InsufficientBalance");

        balances[msg.sender] = newBal;
        balances[to] = balances[to].add(amount);

        emit ConfidentialTransfer(msg.sender, to);
    }

    function balanceOf(agent account) public view returns (Uno) {
        return balances[account];
    }
}
```

Compare with Fhenix equivalent:

```solidity
// Fhenix (Solidity + FHE library)
function transfer(address to, euint64 amount) public {
    euint64 newBal = FHE.sub(balances[msg.sender], amount);
    require(FHE.gt(newBal, FHE.asEuint64(0)));
    balances[msg.sender] = newBal;
    balances[to] = FHE.add(balances[to], amount);
}
```

GTOS is **more concise** — `a.sub(b)` vs `FHE.sub(a, b)`.

## Implementation Sequence

| Phase | Work | Repo |
|-------|------|------|
| 1 | `ProofBundle` type + `ExtractProofBundle` in transaction layer | gtos |
| 2 | `lvm_crypto.go`: Tier 1 functions (9 native homomorphic) | gtos |
| 3 | `lvm_crypto.go`: Tier 2 functions (9 proof-bundle verified) | gtos |
| 4 | `lvm_crypto.go`: accessors + verification (4 functions) | gtos |
| 5 | `lvm_crypto_test.go`: full test coverage for all 22 functions | gtos |
| 6 | Sema: `Uno` type + method resolution + lowering + ABI encode/decode | tolang |
| 7 | Compiler tests + sample contracts + integration test | both |

## Verification

```bash
# gtos: build
go build ./core/vm/... ./crypto/priv/... ./core/priv/... ./core/types/...

# gtos: crypto primitive tests
go test -v ./core/vm/ -run "TestCiphertext" -timeout 120s

# gtos: full suite
go test -p 48 ./... -timeout 600s

# tolang: compiler tests
cd ~/tolang && go test ./tol/sema/... ./...

# integration: compile + deploy sample contract
cd ~/tolang && go run ./cmd/tolc examples/confidential_token/ConfidentialToken.tol
```
