# Plan V2: LVM Confidential Computing Primitives for Privacy Smart Contracts

## Context

GTOS privacy (UNO) currently works only at the protocol level — Shield/Transfer/Unshield
are native tx types. Smart contracts cannot manipulate encrypted values, meaning no
confidential tokens, no private voting, no private OTC via contracts.

We add a unified `tos.ciphertext.*` namespace to the LVM (Lua VM) with 22 functions that
give TOL contracts **Fhenix-equivalent encrypted arithmetic** — add, sub, mul, div,
compare, select, min, max — all on Twisted ElGamal ciphertexts (64 bytes = commitment 32B
+ handle 32B, Ristretto255).

**Key design choice**: operations that ElGamal cannot compute natively (mul, div, compare,
select, min, max) use a **proof bundle** mechanism — the transaction caller pre-computes
results off-chain and attaches ZK proofs; the LVM verifies automatically. Contract
developers see a clean API identical to Fhenix and never handle proofs manually.

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
add(a, b)         →  Enc(a+b)      // EC point addition
sub(a, b)         →  Enc(a-b)      // EC point subtraction
add_scalar(ct, n) →  Enc(a+n)      // scalar-mult + point-add
sub_scalar(ct, n) →  Enc(a-n)      // scalar-mult + point-sub
mul_scalar(ct, n) →  Enc(a×n)      // scalar-mult (repeated addition)
div_scalar(ct, n) →  Enc(a/n)      // scalar-inverse + scalar-mult
```

**Tier 2 — Proof-bundle verified** (caller pre-computes, LVM verifies ZK proof):

```
mul(a, b)         →  Enc(a×b)      // Sigma protocol: Com(c) = Com(a×b)
div(a, b)         →  Enc(a÷b)      // proves a = b×q + r, r ∈ [0,b)
rem(a, b)         →  Enc(a%b)      // remainder from div proof
lt(a, b)          →  bool          // sub(b,a) + range proof
gt(a, b)          →  bool          // sub(a,b) + range proof
eq(a, b)          →  bool          // sub(a,b) + zero-commitment check
min(a, b)         →  Enc(min)      // compare + selection proof
max(a, b)         →  Enc(max)      // compare + selection proof
select(c, a, b)   →  Enc(c?a:b)    // conditional selection proof
verify_transfer   →  bool          // CT validity (same amount, correct keys)
verify_eq         →  bool          // commitment equality
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

Contract developers never see this — they just write `tos.ciphertext.mul(a, b)`.
The user's wallet/SDK generates the proof bundle when constructing the transaction.

### Type Representation

| Type | In Lua | In Storage | In ABI | Size |
|------|--------|------------|--------|------|
| `ciphertext` | `"0x" + 128 hex` | 2 × 32B slots | 2 × 32B words | 64B |

### API: 22 Functions

All functions live in the `tos.ciphertext` namespace.

#### Tier 1 — Native Homomorphic (9 functions)

| # | Function | Signature | Gas | Implementation |
|---|----------|-----------|-----|----------------|
| 1 | `add` | `(ct, ct) → ct` | 8000 | EC point add (commitment + handle) |
| 2 | `sub` | `(ct, ct) → ct` | 8000 | EC point sub (commitment + handle) |
| 3 | `add_scalar` | `(ct, u64) → ct` | 6000 | scalar-mult(G,n) + point-add |
| 4 | `sub_scalar` | `(ct, u64) → ct` | 6000 | scalar-mult(G,n) + point-sub |
| 5 | `mul_scalar` | `(ct, u64) → ct` | 10000 | 2× variable-base scalar-mult |
| 6 | `div_scalar` | `(ct, u64) → ct` | 12000 | scalar-invert + 2× scalar-mult |
| 7 | `zero` | `() → ct` | 100 | precomputed identity ciphertext |
| 8 | `encrypt` | `(bytes32 pubkey, u64 amount) → ct` | 15000 | full ElGamal encryption |
| 9 | `from_parts` | `(bytes32 commit, bytes32 handle) → ct` | 100 | concatenation |

#### Tier 2 — Proof-Bundle Verified (9 functions)

| # | Function | Signature | Gas | Proof type |
|---|----------|-----------|-----|------------|
| 10 | `mul` | `(ct, ct) → ct` | 200000 | Sigma: Com(c) = Com(a×b) |
| 11 | `div` | `(ct, ct) → ct` | 200000 | Sigma: a = b×q + r, r∈[0,b) |
| 12 | `rem` | `(ct, ct) → ct` | 200000 | remainder from div proof |
| 13 | `lt` | `(ct, ct) → bool` | 160000 | sub + 64-bit range proof |
| 14 | `gt` | `(ct, ct) → bool` | 160000 | sub + 64-bit range proof |
| 15 | `eq` | `(ct, ct) → bool` | 150000 | sub + zero-commitment check |
| 16 | `min` | `(ct, ct) → ct` | 170000 | compare + selection proof |
| 17 | `max` | `(ct, ct) → ct` | 170000 | compare + selection proof |
| 18 | `select` | `(bool, ct, ct) → ct` | 160000 | conditional selection proof |

#### Accessors & Verification (4 functions)

| # | Function | Signature | Gas | Purpose |
|---|----------|-----------|-----|---------|
| 19 | `commitment` | `(ct) → bytes32` | 100 | extract commitment point (first 32B) |
| 20 | `handle` | `(ct) → bytes32` | 100 | extract handle point (last 32B) |
| 21 | `verify_transfer` | `(ct, bytes32 sPub, bytes32 rPub, bytes proof) → bool` | 100000 | CT validity: same amount encrypted under both keys |
| 22 | `verify_eq` | `(ct, bytes32 commit, bytes32 pubkey, bytes proof) → bool` | 100000 | commitment equality: ct and commit bind same value |

### Comparison with Fhenix

| Operation | Fhenix | GTOS | Parity |
|-----------|--------|------|--------|
| `add(ct, ct)` | `FHE.add(ea, eb)` | `tos.ciphertext.add(a, b)` | ✅ |
| `sub(ct, ct)` | `FHE.sub(ea, eb)` | `tos.ciphertext.sub(a, b)` | ✅ |
| `mul(ct, ct)` | `FHE.mul(ea, eb)` | `tos.ciphertext.mul(a, b)` | ✅ |
| `div(ct, ct)` | `FHE.div(ea, eb)` | `tos.ciphertext.div(a, b)` | ✅ |
| `rem(ct, ct)` | `FHE.rem(ea, eb)` | `tos.ciphertext.rem(a, b)` | ✅ |
| `lt / gt` | `FHE.lt / gt` | `tos.ciphertext.lt / gt` | ✅ |
| `eq` | `FHE.eq` | `tos.ciphertext.eq` | ✅ |
| `min / max` | `FHE.min / max` | `tos.ciphertext.min / max` | ✅ |
| `select` | `FHE.select` | `tos.ciphertext.select` | ✅ |
| `ct + scalar` | ❌ | `tos.ciphertext.add_scalar` | **GTOS extra** |
| `ct × scalar` | ❌ | `tos.ciphertext.mul_scalar` | **GTOS extra** |
| transfer proof | ❌ | `tos.ciphertext.verify_transfer` | **GTOS extra** |
| commitment eq | ❌ | `tos.ciphertext.verify_eq` | **GTOS extra** |

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
    Op         string // "mul", "div", "rem", "lt", "gt", "eq", "min", "max", "select"
    InputHash  common.Hash // keccak256(op || input_a || input_b)
    ResultData []byte      // encrypted result (64B ciphertext or 1B bool)
    Proof      []byte      // ZK proof bytes
}

// Next pops the next entry, verifying it matches the expected operation and inputs.
func (pb *ProofBundle) Next(op string, a, b []byte) (*ProofEntry, error)
```

**Sub-table registration:**

```go
ctTable := L.NewTable()

// Tier 1 — native homomorphic
L.SetField(ctTable, "add", L.NewFunction(...))          // (ct, ct) → ct
L.SetField(ctTable, "sub", L.NewFunction(...))          // (ct, ct) → ct
L.SetField(ctTable, "add_scalar", L.NewFunction(...))   // (ct, u64) → ct
L.SetField(ctTable, "sub_scalar", L.NewFunction(...))   // (ct, u64) → ct
L.SetField(ctTable, "mul_scalar", L.NewFunction(...))   // (ct, u64) → ct
L.SetField(ctTable, "div_scalar", L.NewFunction(...))   // (ct, u64) → ct
L.SetField(ctTable, "zero", L.NewFunction(...))         // () → ct
L.SetField(ctTable, "encrypt", L.NewFunction(...))      // (pubkey, amount) → ct
L.SetField(ctTable, "from_parts", L.NewFunction(...))   // (commit, handle) → ct

// Tier 2 — proof-bundle verified
L.SetField(ctTable, "mul", L.NewFunction(...))           // (ct, ct) → ct
L.SetField(ctTable, "div", L.NewFunction(...))           // (ct, ct) → ct
L.SetField(ctTable, "rem", L.NewFunction(...))           // (ct, ct) → ct
L.SetField(ctTable, "lt", L.NewFunction(...))            // (ct, ct) → bool
L.SetField(ctTable, "gt", L.NewFunction(...))            // (ct, ct) → bool
L.SetField(ctTable, "eq", L.NewFunction(...))            // (ct, ct) → bool
L.SetField(ctTable, "min", L.NewFunction(...))           // (ct, ct) → ct
L.SetField(ctTable, "max", L.NewFunction(...))           // (ct, ct) → ct
L.SetField(ctTable, "select", L.NewFunction(...))        // (bool, ct, ct) → ct

// Accessors & verification
L.SetField(ctTable, "commitment", L.NewFunction(...))    // (ct) → bytes32
L.SetField(ctTable, "handle", L.NewFunction(...))        // (ct) → bytes32
L.SetField(ctTable, "verify_transfer", L.NewFunction(...)) // (ct, sPub, rPub, proof) → bool
L.SetField(ctTable, "verify_eq", L.NewFunction(...))     // (ct, commit, pubkey, proof) → bool

L.SetField(tosTable, "ciphertext", ctTable)
```

### 2. `~/gtos/core/vm/lvm.go` — Execute() Hook

```go
// Extract proof bundle from calldata (appended after ABI-encoded args)
proofBundle := extractProofBundle(ctx.Data)

// Register tos.ciphertext table
registerCryptoTable(L, tosTable, l.StateDB, contractAddr, &gas, ctx.Readonly, proofBundle)
```

### 3. `~/gtos/core/vm/lvm_crypto_test.go` — Tests

**Tier 1 tests:**
- `add(a, b)` then `sub(result, b)` recovers `a`
- `add_scalar` / `sub_scalar` round-trip
- `mul_scalar` then `div_scalar` round-trip; `div_scalar(ct, 0)` reverts
- `zero()` is additive identity: `add(ct, zero) == ct`
- `encrypt` produces valid ciphertext
- `from_parts(commitment(ct), handle(ct)) == ct`

**Tier 2 tests (with proof bundles):**
- `mul`: Enc(3) × Enc(7) = Enc(21); wrong result fails
- `div`: Enc(17) ÷ Enc(5) = Enc(3); `rem` = Enc(2)
- `lt`: Enc(3) < Enc(7) = true; Enc(7) < Enc(3) = false
- `gt`: inverse of `lt`
- `eq`: Enc(5) == Enc(5) = true; Enc(5) == Enc(6) = false
- `min(Enc(3), Enc(7))` = Enc(3)
- `max(Enc(3), Enc(7))` = Enc(7)
- `select(true, Enc(3), Enc(7))` = Enc(3)
- Missing proof bundle entry → revert
- Tampered proof → revert
- Gas consumption within expected ranges

**Verification tests:**
- `verify_transfer` with valid/invalid CT validity proof
- `verify_eq` with valid/invalid commitment equality proof

### 4. `~/gtos/core/types/transaction.go` — Proof Bundle Encoding

Add proof bundle extraction from transaction calldata:

```go
// ExtractProofBundle parses the proof bundle appended after standard ABI calldata.
// Format: [standard calldata] [0xPBND magic] [entry count u16] [entries...]
// Each entry: [op u8] [input_hash 32B] [result_len u16] [result ...] [proof_len u16] [proof ...]
func ExtractProofBundle(data []byte) (*ProofBundle, []byte)
```

### 5. `~/tolang/tol/sema/sema.go` — Type System

- `isValidAtomicTOLType`: add `"ciphertext"` case
- `isValueTOLType`: add `"ciphertext"` (can be function param/return)
- `isDefaultInitializableTOLType`: add `"ciphertext"` (default = `tos.ciphertext.zero()`)
- Do NOT add to `isValidMappingKeyType` (ciphertext should not be a mapping key)
- Support `==` and `!=` comparison via `tos.ciphertext.eq`
- Reject `<`, `>`, `+`, `-`, `*`, `/` operators on ciphertext (must use `tos.ciphertext.*`)

### 6. `~/tolang/tol_ir_direct_lowering.go` — Code Generation

Add `storageKindCiphertext` constant alongside existing scalar/mapping/array.

`classifyStorageSlotKind`: when the leaf type resolves to `"ciphertext"`, return
`storageKindCiphertext`.

`defaultValueExprForType`: add case for `"ciphertext"` → `"0x" + 128 zeros`.

Storage emit paths — for `storageKindCiphertext`:
- **Scalar store**: emit `tos.ciphertext.store(slotKey, value)` (2-slot write)
- **Scalar load**: emit `tos.ciphertext.load(slotKey)` (2-slot read)
- **Mapping store**: emit `tos.ciphertext.map_store(mapName, key1, ..., value)`
- **Mapping load**: emit `tos.ciphertext.map_load(mapName, key1, ...)`

Note: `store/load/map_store/map_load` are internal LVM functions used by the compiler's
code generation. They are NOT part of the 22-function public API — contract developers
use normal TOL assignment syntax: `balances[account] = ct;`

### 7. `~/tolang/cryptolib.go` — ABI Encoding

Add `"ciphertext"` case to ABI encode/decode:
- Encode: 2 consecutive 32-byte words (commitment, handle)
- Decode: consume 2 × 32-byte slots, concatenate as `"0x" + 128 hex`

### 8. `~/tolang/tol/sema/sema_test.go` — Compiler Tests

- `ciphertext` accepted as state variable, param, return type
- `ciphertext` in `mapping(agent => ciphertext)` works
- `ciphertext` rejected as mapping key
- `==` / `!=` compile to `tos.ciphertext.eq`; `<`, `>`, `+`, `-` rejected

## Sample Contract (Confidential TRC20)

```tol
pragma tolang 0.4.0;

contract ConfidentialToken {
    agent minter;
    mapping(agent => ciphertext) balances;
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

    // Mint: minter creates ciphertext off-chain, submits with range proof.
    function mint(agent to, ciphertext mintCt) external {
        require(msg.sender == minter, "OnlyMinter");
        require(publicKeys[to] != bytes32(0), "NotRegistered");

        // Verify minted amount ≥ 0 via proof bundle (lt checks internally)
        // In practice the range proof is in the proof bundle for the mint operation.
        balances[to] = tos.ciphertext.add(balances[to], mintCt);
        emit Mint(to);
    }

    // Transfer: pure ciphertext arithmetic. SDK generates proof bundle.
    function transfer(agent to, ciphertext amount) external {
        require(publicKeys[msg.sender] != bytes32(0), "SenderNotRegistered");
        require(publicKeys[to] != bytes32(0), "ReceiverNotRegistered");

        // Subtract from sender (ct - ct, native homomorphic)
        ciphertext newSenderBal = tos.ciphertext.sub(balances[msg.sender], amount);

        // Verify sender has sufficient balance: newSenderBal >= 0
        // The gt(newSenderBal, zero) check is verified via proof bundle
        require(tos.ciphertext.gt(newSenderBal, tos.ciphertext.zero()), "InsufficientBalance");

        // Verify the transfer ciphertext is valid for both keys
        require(
            tos.ciphertext.verify_transfer(
                amount,
                publicKeys[msg.sender],
                publicKeys[to],
                ""  // proof from calldata proof bundle
            ),
            "BadTransferProof"
        );

        balances[msg.sender] = newSenderBal;
        balances[to] = tos.ciphertext.add(balances[to], amount);

        emit ConfidentialTransfer(msg.sender, to);
    }

    function balanceOf(agent account) public view returns (ciphertext) {
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

The contract logic is **structurally identical**. The only difference is under the hood:
Fhenix sends to a coprocessor; GTOS verifies a proof bundle.

## Implementation Sequence

| Phase | Work | Repo |
|-------|------|------|
| 1 | `ProofBundle` type + `ExtractProofBundle` in transaction layer | gtos |
| 2 | `lvm_crypto.go`: Tier 1 functions (9 native homomorphic) | gtos |
| 3 | `lvm_crypto.go`: Tier 2 functions (9 proof-bundle verified) | gtos |
| 4 | `lvm_crypto.go`: accessors + verification (4 functions) | gtos |
| 5 | `lvm_crypto_test.go`: full test coverage for all 22 functions | gtos |
| 6 | Sema: `ciphertext` type + lowering: `storageKindCiphertext` + ABI encode/decode | tolang |
| 7 | Compiler tests + sample ConfidentialToken contract + integration test | both |

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
