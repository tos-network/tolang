# Plan: LVM Homomorphic Encryption Primitives for Privacy Smart Contracts

## Context

GTOS privacy (UNO) currently works only at the protocol level — Shield/Transfer/Unshield are native tx types. Smart contracts cannot manipulate encrypted values. This means no confidential tokens, no private AMM, no private voting via contracts.

We add HE ciphertext operations to the LVM (Lua VM), exposing the existing `crypto/priv` and `core/priv` Go functions as namespaced `tos.*` primitives. TOL contracts get new opaque types (`ciphertext`, `point`, `scalar`) and full EC algebra, matching XELIS feature parity.

**API style**: namespaced sub-tables following the existing `tos.abi.*`, `tos.bytes.*`, `tos.block.*` pattern:

```
tos.ciphertext.*   — Twisted ElGamal ciphertext operations
tos.ristretto.*    — Ristretto255 EC point operations
tos.scalar.*       — Scalar field arithmetic
tos.proof.*        — Zero-knowledge proof construction, verification, and accessors
tos.transcript.*   — Merlin transcript (Fiat-Shamir) construction
tos.crypto.*       — General crypto (blake3, sha3, ristretto signature verify)
```

## Design

### Encrypted Arithmetic Beyond Additive Homomorphism

Twisted ElGamal is **additively homomorphic**: ct+ct, ct-ct, ct×scalar work natively.
Operations like ct×ct, ct÷ct, comparison, and conditional select are **not** natively
supported. However, they can be achieved via **"off-chain compute + on-chain ZK verify"**:

**Comparison** (built from existing primitives — no new proof type needed):
```
a < b   ⟺  verify_range(commit(Enc(b) - Enc(a)), proof, 64, t)  // b-a ∈ [0, 2^64)
a == b  ⟺  commitment(sub(encA, encB)) == identity_point
min(a,b) = compare + verify_commitment_eq to confirm selection
```
The contract computes `sub(encB, encA)` homomorphically, then the user provides a range
proof. If `a > b`, the subtraction wraps and the range proof fails. No new primitives needed.

**Multiplication** (requires `verify_multiplication`):
```
User (off-chain):  knows a,b → computes c = a×b → encrypts Enc(c)
                   generates ZK proof π: "c = a×b where a ∈ Enc(A), b ∈ Enc(B)"
Contract (on-chain): verify_multiplication(encA, encB, encC, π, transcript)
```
The proof is a Sigma protocol over Pedersen commitments: prover shows
`Com(c) = Com(a×b)` without revealing a, b, or c.

**Division with remainder** (requires `verify_division`):
```
User (off-chain):  knows a,b → computes q = a÷b, r = a%b → encrypts Enc(q), Enc(r)
                   generates ZK proof π: "a = b×q + r, r ∈ [0, b)"
Contract (on-chain): verify_division(encA, encB, encQ, encR, π, transcript)
```
This subsumes modulo (`a % b` = the `encR` output) and integer division.

**Comparison with Fhenix (TFHE):**
```
Fhenix:  contract computes on ciphertexts directly → slow (needs coprocessor)
GTOS:    user computes off-chain + contract verifies ZK proof → fast, trustless
```

### Type Representations

| Type | In Lua | In Storage | In ABI | Size |
|------|--------|------------|--------|------|
| `ciphertext` | `"0x" + 128 hex` | 2 × 32B slots | 2 × 32B words | 64B |
| `point` | `"0x" + 64 hex` | 1 × 32B slot | 1 × 32B word | 32B (compressed) |
| `scalar` | `"0x" + 64 hex` | 1 × 32B slot | 1 × 32B word | 32B |

### Gas Costs

#### `tos.ciphertext.*`

| Function | Gas | Rationale |
|----------|-----|-----------|
| `add(a, b)` | 8000 | 2 EC point additions |
| `sub(a, b)` | 8000 | 2 EC point subtractions |
| `add_scalar(ct, n)` | 6000 | 1 scalar-mult + 1 point-add |
| `sub_scalar(ct, n)` | 6000 | 1 scalar-mult + 1 point-sub |
| `mul_scalar(ct, n)` | 10000 | 2 full scalar-mult operations |
| `div_scalar(ct, n)` | 12000 | 1 scalar inversion + 2 scalar-mult |
| `encrypt(pubkey, amount)` | 15000 | Full ElGamal encryption (random nonce) |
| `zero()` | 100 | Return precomputed identity |
| `from_parts(c, h)` | 100 | Concatenation only |
| `commitment(ct)` | 100 | Hex slice (first 32B) |
| `handle(ct)` | 100 | Hex slice (last 32B) |
| `store(key, ct)` | 10000 | 2 × sstore |
| `load(key)` | 200 | 2 × sload |
| `map_store(map, k..., ct)` | 10000 | 2 × sstore |
| `map_load(map, k...)` | 200 | 2 × sload |

#### `tos.ristretto.*`

| Function | Gas | Rationale |
|----------|-----|-----------|
| `add(P, Q)` | 1500 | EC point addition |
| `sub(P, Q)` | 1500 | EC point subtraction |
| `add_scalar(P, s)` | 6000 | scalar-mult(G) + point-add |
| `sub_scalar(P, s)` | 6000 | scalar-mult(G) + point-sub |
| `mul_scalar(P, s)` | 5000 | Variable-base scalar-mult |
| `div_scalar(P, s)` | 7000 | Scalar inversion + scalar-mult |
| `from_bytes(b)` | 500 | Ristretto decompression + validation |
| `to_bytes(P)` | 200 | Ristretto compression |
| `identity()` | 100 | Return precomputed identity |
| `is_identity(P)` | 100 | Point comparison |
| `G` (constant) | 0 | Base generator |
| `H` (constant) | 0 | Pedersen commitment generator |

#### `tos.scalar.*`

| Function | Gas | Rationale |
|----------|-----|-----------|
| `from_u64(v)` | 100 | Cast to scalar |
| `from_bytes(b)` | 100 | Parse 32B canonical |
| `to_bytes(s)` | 100 | Serialize 32B |
| `add(a, b)` | 100 | Modular addition |
| `sub(a, b)` | 100 | Modular subtraction |
| `mul(a, b)` | 200 | Modular multiplication |
| `div(a, b)` | 300 | Modular division (invert + mul) |
| `invert(s)` | 250 | Modular inverse |
| `is_zero(s)` | 100 | Zero check |
| `mul_base(s)` | 5000 | Fixed-base scalar-mult (s × G) |
| `ZERO` (constant) | 0 | Additive identity |
| `ONE` (constant) | 0 | Multiplicative identity |

#### `tos.proof.*`

| Function | Gas | Rationale |
|----------|-----|-----------|
| **Verification** | | |
| `verify_range(commit, proof, bits, transcript)` | 150000 | Bulletproof verification; bits=1..64 |
| `verify_range_batch(commits[], proof, bits, transcript)` | 150000 + 5000×N | Batch bulletproof |
| `verify_ct_validity(commit, dHandle, sHandle, dPub, sPub, proof, transcript)` | 100000 | CT validity sigma protocol |
| `verify_commitment_eq(pubkey, ct, commit, proof, transcript)` | 100000 | Commitment equality sigma |
| `verify_arbitrary_range(pubkey, ct, proof, transcript)` | 120000 | Arbitrary-max range proof |
| `verify_ownership(pubkey, ct, proof, transcript)` | 100000 | Knowledge-of-decryption-key |
| `verify_balance(pubkey, ct, proof, transcript)` | 80000 | Balance sufficiency |
| `verify_shield(commit, handle, pubkey, amount, proof)` | 80000 | TOS-specific shield proof |
| **Arithmetic relation proofs (TOS-specific)** | | |
| `verify_multiplication(encA, encB, encC, proof, transcript)` | 200000 | Prove Enc(c) encrypts a×b where a∈Enc(A), b∈Enc(B) |
| `verify_division(encA, encB, encQ, encR, proof, transcript)` | 200000 | Prove a = b×q + r where q=quotient, r=remainder, r∈[0,b) |
| **Constructors** | | |
| `new_arbitrary_range(max_value, delta_commit, eq_proof, range_proof)` | 500 | Compose ArbitraryRangeProof |
| `new_ownership(amount, commit, eq_proof, range_proof)` | 500 | Compose OwnershipProof |
| `new_balance(amount, eq_proof)` | 250 | Compose BalanceProof |
| **Accessors** | | |
| `arbitrary_range_max_value(proof)` | 100 | → u64 |
| `arbitrary_range_delta_commitment(proof)` | 100 | → point |
| `arbitrary_range_commitment_eq_proof(proof)` | 100 | → commitment_eq_proof bytes |
| `arbitrary_range_range_proof(proof)` | 100 | → range_proof bytes |
| `ownership_amount(proof)` | 100 | → u64 |
| `ownership_commitment(proof)` | 100 | → point |
| `ownership_commitment_eq_proof(proof)` | 100 | → commitment_eq_proof bytes |
| `ownership_range_proof(proof)` | 100 | → range_proof bytes |
| `balance_amount(proof)` | 100 | → u64 |
| `balance_commitment_eq_proof(proof)` | 100 | → commitment_eq_proof bytes |

#### `tos.crypto.*`

| Function | Gas | Rationale |
|----------|-----|-----------|
| `blake3(input)` | 3000 + len(input) | BLAKE3 hash → 32B hex |
| `sha3(input)` | 7500 + 4×len(input) | SHA3-256 (NOT keccak) → 32B hex |
| `sig_verify(sig, data, point)` | 500 | Ristretto EdDSA signature verify → bool |

#### `tos.transcript.*`

| Function | Gas | Rationale |
|----------|-----|-----------|
| `new(label)` | 200 | Initialize Merlin transcript |
| `append_message(t, label, msg)` | 100 + len(msg) | Append bytes |
| `append_point(t, label, P)` | 200 | Append compressed point (32B) |
| `validate_and_append_point(t, label, P)` | 500 | Validate canonical + append |
| `append_scalar(t, label, s)` | 200 | Append scalar (32B) |
| `challenge_scalar(t, label)` | 300 | Extract 64B entropy → reduce to scalar |
| `challenge_bytes(t, label, len)` | 5 × len | Extract arbitrary bytes |

## XELIS Coverage Matrix

### `tos.ciphertext.*` — 15 functions

| # | XELIS Function | TOS Function | Notes |
|---|----------------|--------------|-------|
| 1 | `ciphertext_add_plaintext(ct, u64)` | `tos.ciphertext.add_scalar(ct, n)` | |
| 2 | `ciphertext_sub_plaintext(ct, u64)` | `tos.ciphertext.sub_scalar(ct, n)` | |
| 3 | `ciphertext_mul_plaintext(ct, u64)` | `tos.ciphertext.mul_scalar(ct, n)` | |
| 4 | `ciphertext_div_plaintext(ct, u64)` | `tos.ciphertext.div_scalar(ct, n)` | |
| 5 | `ciphertext_new(addr, amount)` | `tos.ciphertext.encrypt(pubkey, amount)` | Takes pubkey directly |
| 6 | `ciphertext_zero()` | `tos.ciphertext.zero()` | |
| 7 | `ciphertext_commitment(ct)` | `tos.ciphertext.commitment(ct)` | Returns point hex |
| 8 | `ciphertext_handle(ct)` | `tos.ciphertext.handle(ct)` | Returns point hex |
| — | (no XELIS equivalent) | `tos.ciphertext.add(a, b)` | **TOS extra**: ct + ct |
| — | (no XELIS equivalent) | `tos.ciphertext.sub(a, b)` | **TOS extra**: ct - ct |
| — | (no XELIS equivalent) | `tos.ciphertext.from_parts(c, h)` | **TOS extra**: construct from parts |
| — | (no XELIS equivalent) | `tos.ciphertext.store(key, ct)` | **TOS extra**: 2-slot storage |
| — | (no XELIS equivalent) | `tos.ciphertext.load(key)` | **TOS extra**: 2-slot load |
| — | (no XELIS equivalent) | `tos.ciphertext.map_store(...)` | **TOS extra**: mapping storage |
| — | (no XELIS equivalent) | `tos.ciphertext.map_load(...)` | **TOS extra**: mapping load |

### `tos.ristretto.*` — 12 functions + 2 constants

| # | XELIS Function | TOS Function |
|---|----------------|--------------|
| 1 | `ristretto_add(P, Q)` | `tos.ristretto.add(P, Q)` |
| 2 | `ristretto_sub(P, Q)` | `tos.ristretto.sub(P, Q)` |
| 3 | `ristretto_add_scalar(P, s)` | `tos.ristretto.add_scalar(P, s)` |
| 4 | `ristretto_sub_scalar(P, s)` | `tos.ristretto.sub_scalar(P, s)` |
| 5 | `ristretto_mul_scalar(P, s)` | `tos.ristretto.mul_scalar(P, s)` |
| 6 | `ristretto_div_scalar(P, s)` | `tos.ristretto.div_scalar(P, s)` |
| 7 | `ristretto_from_bytes(b)` | `tos.ristretto.from_bytes(b)` |
| 8 | `ristretto_to_bytes(P)` | `tos.ristretto.to_bytes(P)` |
| 9 | `ristretto_identity()` | `tos.ristretto.identity()` |
| 10 | `ristretto_is_identity(P)` | `tos.ristretto.is_identity(P)` |
| 11 | `Ristretto.G` | `tos.ristretto.G` |
| 12 | `Ristretto.H` | `tos.ristretto.H` |

### `tos.scalar.*` — 11 functions + 2 constants

| # | XELIS Function | TOS Function |
|---|----------------|--------------|
| 1 | `scalar_from_u64(v)` | `tos.scalar.from_u64(v)` |
| 2 | `scalar_from_bytes(b)` | `tos.scalar.from_bytes(b)` |
| 3 | `scalar_to_bytes(s)` | `tos.scalar.to_bytes(s)` |
| 4 | `scalar_add(a, b)` | `tos.scalar.add(a, b)` |
| 5 | `scalar_sub(a, b)` | `tos.scalar.sub(a, b)` |
| 6 | `scalar_mul(a, b)` | `tos.scalar.mul(a, b)` |
| 7 | `scalar_div(a, b)` | `tos.scalar.div(a, b)` |
| 8 | `scalar_invert(s)` | `tos.scalar.invert(s)` |
| 9 | `scalar_is_zero(s)` | `tos.scalar.is_zero(s)` |
| 10 | `scalar_mul_base(s)` | `tos.scalar.mul_base(s)` |
| 11 | `Scalar.ZERO` | `tos.scalar.ZERO` |
| 12 | `Scalar.ONE` | `tos.scalar.ONE` |

### `tos.proof.*` — 22 functions

| # | XELIS Function | TOS Function | Notes |
|---|----------------|--------------|-------|
| | **Verification** | | |
| 1 | `RangeProof.verify_single(commit, transcript, bits)` | `tos.proof.verify_range(commit, proof, bits, transcript)` | bits param added |
| 2 | `RangeProof.verify_multiple(commits[], transcript, bits)` | `tos.proof.verify_range_batch(commits, proof, bits, transcript)` | bits param added |
| 3 | `CiphertextValidityProof.verify(commit, dPub, sPub, dHandle, sHandle, transcript)` | `tos.proof.verify_ct_validity(commit, dHandle, sHandle, dPub, sPub, proof, transcript)` | |
| 4 | `CommitmentEqProof.verify(sPub, ct, commit, transcript)` | `tos.proof.verify_commitment_eq(pubkey, ct, commit, proof, transcript)` | |
| 5 | `ArbitraryRangeProof.verify(sPub, sCt, transcript)` | `tos.proof.verify_arbitrary_range(pubkey, ct, proof, transcript)` | |
| 6 | `OwnershipProof.verify(sPub, sCt, transcript)` | `tos.proof.verify_ownership(pubkey, ct, proof, transcript)` | |
| 7 | `BalanceProof.verify(sPub, sCt, transcript)` | `tos.proof.verify_balance(pubkey, ct, proof, transcript)` | |
| — | (no XELIS equivalent) | `tos.proof.verify_shield(...)` | **TOS extra**: native shield proof |
| — | (no XELIS equivalent) | `tos.proof.verify_multiplication(encA, encB, encC, proof, t)` | **TOS extra**: ct×ct via ZK |
| — | (no XELIS equivalent) | `tos.proof.verify_division(encA, encB, encQ, encR, proof, t)` | **TOS extra**: ct÷ct via ZK |
| | **Constructors** | | |
| 8 | `ArbitraryRangeProof.new(max_value, delta_commit, eq_proof, range_proof)` | `tos.proof.new_arbitrary_range(max_value, delta_commit, eq_proof, range_proof)` | |
| 9 | `OwnershipProof.new(amount, commit, eq_proof, range_proof)` | `tos.proof.new_ownership(amount, commit, eq_proof, range_proof)` | |
| 10 | `BalanceProof.new(amount, eq_proof)` | `tos.proof.new_balance(amount, eq_proof)` | |
| | **Accessors** | | |
| 11 | `ArbitraryRangeProof.max_value(self)` | `tos.proof.arbitrary_range_max_value(proof)` | |
| 12 | `ArbitraryRangeProof.delta_commitment(self)` | `tos.proof.arbitrary_range_delta_commitment(proof)` | |
| 13 | `ArbitraryRangeProof.commitment_eq_proof(self)` | `tos.proof.arbitrary_range_commitment_eq_proof(proof)` | |
| 14 | `ArbitraryRangeProof.range_proof(self)` | `tos.proof.arbitrary_range_range_proof(proof)` | |
| 15 | `OwnershipProof.amount(self)` | `tos.proof.ownership_amount(proof)` | |
| 16 | `OwnershipProof.commitment(self)` | `tos.proof.ownership_commitment(proof)` | |
| 17 | `OwnershipProof.commitment_eq_proof(self)` | `tos.proof.ownership_commitment_eq_proof(proof)` | |
| 18 | `OwnershipProof.range_proof(self)` | `tos.proof.ownership_range_proof(proof)` | |
| 19 | `BalanceProof.amount(self)` | `tos.proof.balance_amount(proof)` | |
| 20 | `BalanceProof.commitment_eq_proof(self)` | `tos.proof.balance_commitment_eq_proof(proof)` | |

### `tos.transcript.*` — 7 functions

| # | XELIS Function | TOS Function |
|---|----------------|--------------|
| 1 | `transcript_new(label)` | `tos.transcript.new(label)` |
| 2 | `transcript_append_message(t, label, msg)` | `tos.transcript.append_message(t, label, msg)` |
| 3 | `transcript_append_point(t, label, P)` | `tos.transcript.append_point(t, label, P)` |
| 4 | `transcript_validate_and_append_point(t, label, P)` | `tos.transcript.validate_and_append_point(t, label, P)` |
| 5 | `transcript_append_scalar(t, label, s)` | `tos.transcript.append_scalar(t, label, s)` |
| 6 | `transcript_challenge_scalar(t, label)` | `tos.transcript.challenge_scalar(t, label)` |
| 7 | `transcript_challenge_bytes(t, label, len)` | `tos.transcript.challenge_bytes(t, label, len)` |

### `tos.crypto.*` — 3 functions

| # | XELIS Function | TOS Function | Notes |
|---|----------------|--------------|-------|
| 1 | `blake3(input) → Hash` | `tos.crypto.blake3(input)` | → 32B hex |
| 2 | `sha3(input) → Hash` | `tos.crypto.sha3(input)` | SHA3-256 (not keccak) |
| 3 | `Signature.verify(data, point) → bool` | `tos.crypto.sig_verify(sig, data, point)` | Ristretto EdDSA |

Note: LVM already has `tos.keccak256`, `tos.sha256`, `tos.ripemd160`, `tos.ecrecover` for
secp256k1/EVM-style crypto. The `tos.crypto.*` namespace adds Ristretto255-native crypto
that XELIS exposes.

### Coverage Summary

| Namespace | XELIS Functions | TOS Covers | TOS Extra | Coverage |
|-----------|-----------------|------------|-----------|----------|
| ciphertext | 8 | 8 | 7 (add/sub ct+ct, from_parts, storage) | **100%** |
| ristretto | 10 + 2 const | 10 + 2 const | 0 | **100%** |
| scalar | 10 + 2 const | 10 + 2 const | 0 | **100%** |
| proof | 20 (7 verify + 3 ctor + 10 accessor) | 20 | 3 (verify_shield, verify_multiplication, verify_division) | **100%** |
| transcript | 7 | 7 | 0 | **100%** |
| crypto | 3 (blake3, sha3, sig_verify) | 3 | 0 | **100%** |
| **Total** | **58** | **58** | **10** | **100%** |

## Changes

### 1. `~/gtos/core/vm/lvm_crypto.go` — New File: All Crypto Primitives

Separate file to keep `lvm.go` manageable. Exports a single registration function called from `Execute()`.

```go
// registerCryptoTables registers tos.ciphertext, tos.ristretto, tos.scalar,
// tos.proof, tos.transcript, and tos.crypto sub-tables on tosTable.
func registerCryptoTables(L *lua.LState, tosTable *lua.LTable, stateDB StateDB,
    contractAddr common.Address, gas *uint64, readonly bool)
```

**Gas constants** (top of file):

```go
// --- ciphertext ---
gasCtAdd          uint64 = 8000
gasCtSub          uint64 = 8000
gasCtAddScalar    uint64 = 6000
gasCtSubScalar    uint64 = 6000
gasCtMulScalar    uint64 = 10000
gasCtDivScalar    uint64 = 12000
gasCtEncrypt      uint64 = 15000
gasCtZero         uint64 = 100
gasCtParts        uint64 = 100
gasCtStore        uint64 = 10000
gasCtLoad         uint64 = 200

// --- ristretto ---
gasRistAdd        uint64 = 1500
gasRistSub        uint64 = 1500
gasRistAddScalar  uint64 = 6000
gasRistSubScalar  uint64 = 6000
gasRistMulScalar  uint64 = 5000
gasRistDivScalar  uint64 = 7000
gasRistFromBytes  uint64 = 500
gasRistToBytes    uint64 = 200
gasRistIdentity   uint64 = 100
gasRistIsIdentity uint64 = 100

// --- scalar ---
gasScalarFromU64    uint64 = 100
gasScalarFromBytes  uint64 = 100
gasScalarToBytes    uint64 = 100
gasScalarAdd        uint64 = 100
gasScalarSub        uint64 = 100
gasScalarMul        uint64 = 200
gasScalarDiv        uint64 = 300
gasScalarInvert     uint64 = 250
gasScalarIsZero     uint64 = 100
gasScalarMulBase    uint64 = 5000

// --- proof ---
gasProofRange         uint64 = 150000
gasProofRangeBatch    uint64 = 150000  // base; +5000 per commitment
gasProofRangeBatchPer uint64 = 5000
gasProofCtValidity    uint64 = 100000
gasProofCommitmentEq  uint64 = 100000
gasProofArbRange      uint64 = 120000
gasProofOwnership     uint64 = 100000
gasProofBalance       uint64 = 80000
gasProofShield        uint64 = 80000
gasProofMul           uint64 = 200000  // verify_multiplication
gasProofDiv           uint64 = 200000  // verify_division
gasProofNewComposite  uint64 = 500     // new_arbitrary_range, new_ownership
gasProofNewBalance    uint64 = 250     // new_balance (simpler)
gasProofAccessor      uint64 = 100

// --- transcript ---
gasTranscriptNew             uint64 = 200
gasTranscriptAppendMsg       uint64 = 100   // base; +1 per byte
gasTranscriptAppendPoint     uint64 = 200
gasTranscriptValidatePoint   uint64 = 500
gasTranscriptAppendScalar    uint64 = 200
gasTranscriptChallengeScalar uint64 = 300
gasTranscriptChallengeBytes  uint64 = 5     // per byte of output

// --- crypto ---
gasCryptoBlake3Base   uint64 = 3000    // base; +1 per input byte
gasCryptoSha3Base     uint64 = 7500    // base; +4 per input byte
gasCryptoSigVerify    uint64 = 500
```

**Internal helpers**:

```go
func parseCiphertextHex(s string) ([]byte, error)     // "0x"+128hex → 64 bytes
func ciphertextToHex(ct []byte) string                 // 64 bytes → "0x"+128hex
func parsePointHex(s string) ([]byte, error)           // "0x"+64hex → 32 bytes
func pointToHex(p []byte) string                       // 32 bytes → "0x"+64hex
func parseScalarHex(s string) ([]byte, error)          // "0x"+64hex → 32 bytes
func scalarToHex(s []byte) string                      // 32 bytes → "0x"+64hex
func ctCommitSlot(key string) common.Hash
func ctHandleSlot(key string) common.Hash
func mapCtCommitSlot(mapName string, keys []string) common.Hash
func mapCtHandleSlot(mapName string, keys []string) common.Hash
```

**Sub-table registration** (inside `registerCryptoTables`):

```go
// tos.ciphertext
ctTable := L.NewTable()
L.SetField(ctTable, "add", L.NewFunction(...))          // ct + ct
L.SetField(ctTable, "sub", L.NewFunction(...))          // ct - ct
L.SetField(ctTable, "add_scalar", L.NewFunction(...))   // ct + u64
L.SetField(ctTable, "sub_scalar", L.NewFunction(...))   // ct - u64
L.SetField(ctTable, "mul_scalar", L.NewFunction(...))   // ct × u64
L.SetField(ctTable, "div_scalar", L.NewFunction(...))   // ct ÷ u64
L.SetField(ctTable, "encrypt", L.NewFunction(...))      // pubkey, amount → ct
L.SetField(ctTable, "zero", L.NewFunction(...))         // identity ct
L.SetField(ctTable, "from_parts", L.NewFunction(...))   // point, point → ct
L.SetField(ctTable, "commitment", L.NewFunction(...))   // ct → point
L.SetField(ctTable, "handle", L.NewFunction(...))       // ct → point
L.SetField(ctTable, "store", L.NewFunction(...))        // key, ct → void
L.SetField(ctTable, "load", L.NewFunction(...))         // key → ct|nil
L.SetField(ctTable, "map_store", L.NewFunction(...))    // map, k..., ct → void
L.SetField(ctTable, "map_load", L.NewFunction(...))     // map, k... → ct|nil
L.SetField(tosTable, "ciphertext", ctTable)

// tos.ristretto
ristTable := L.NewTable()
L.SetField(ristTable, "add", L.NewFunction(...))
L.SetField(ristTable, "sub", L.NewFunction(...))
L.SetField(ristTable, "add_scalar", L.NewFunction(...))
L.SetField(ristTable, "sub_scalar", L.NewFunction(...))
L.SetField(ristTable, "mul_scalar", L.NewFunction(...))
L.SetField(ristTable, "div_scalar", L.NewFunction(...))
L.SetField(ristTable, "from_bytes", L.NewFunction(...))
L.SetField(ristTable, "to_bytes", L.NewFunction(...))
L.SetField(ristTable, "identity", L.NewFunction(...))
L.SetField(ristTable, "is_identity", L.NewFunction(...))
L.SetField(ristTable, "G", lua.LString(basePointGHex))
L.SetField(ristTable, "H", lua.LString(basePointHHex))
L.SetField(tosTable, "ristretto", ristTable)

// tos.scalar
scalarTable := L.NewTable()
L.SetField(scalarTable, "from_u64", L.NewFunction(...))
L.SetField(scalarTable, "from_bytes", L.NewFunction(...))
L.SetField(scalarTable, "to_bytes", L.NewFunction(...))
L.SetField(scalarTable, "add", L.NewFunction(...))
L.SetField(scalarTable, "sub", L.NewFunction(...))
L.SetField(scalarTable, "mul", L.NewFunction(...))
L.SetField(scalarTable, "div", L.NewFunction(...))
L.SetField(scalarTable, "invert", L.NewFunction(...))
L.SetField(scalarTable, "is_zero", L.NewFunction(...))
L.SetField(scalarTable, "mul_base", L.NewFunction(...))
L.SetField(scalarTable, "ZERO", lua.LString(scalarZeroHex))
L.SetField(scalarTable, "ONE", lua.LString(scalarOneHex))
L.SetField(tosTable, "scalar", scalarTable)

// tos.proof — verification
proofTable := L.NewTable()
L.SetField(proofTable, "verify_range", L.NewFunction(...))             // commit, proof, bits, transcript
L.SetField(proofTable, "verify_range_batch", L.NewFunction(...))       // commits[], proof, bits, transcript
L.SetField(proofTable, "verify_ct_validity", L.NewFunction(...))
L.SetField(proofTable, "verify_commitment_eq", L.NewFunction(...))
L.SetField(proofTable, "verify_arbitrary_range", L.NewFunction(...))
L.SetField(proofTable, "verify_ownership", L.NewFunction(...))
L.SetField(proofTable, "verify_balance", L.NewFunction(...))
L.SetField(proofTable, "verify_shield", L.NewFunction(...))            // TOS-specific
// tos.proof — arithmetic relation proofs (TOS-specific, beyond XELIS)
L.SetField(proofTable, "verify_multiplication", L.NewFunction(...))    // encA, encB, encC, proof, transcript
L.SetField(proofTable, "verify_division", L.NewFunction(...))          // encA, encB, encQ, encR, proof, transcript
// tos.proof — constructors
L.SetField(proofTable, "new_arbitrary_range", L.NewFunction(...))      // max_value, delta_commit, eq_proof, range_proof → blob
L.SetField(proofTable, "new_ownership", L.NewFunction(...))            // amount, commit, eq_proof, range_proof → blob
L.SetField(proofTable, "new_balance", L.NewFunction(...))              // amount, eq_proof → blob
// tos.proof — accessors (arbitrary range)
L.SetField(proofTable, "arbitrary_range_max_value", L.NewFunction(...))
L.SetField(proofTable, "arbitrary_range_delta_commitment", L.NewFunction(...))
L.SetField(proofTable, "arbitrary_range_commitment_eq_proof", L.NewFunction(...))
L.SetField(proofTable, "arbitrary_range_range_proof", L.NewFunction(...))
// tos.proof — accessors (ownership)
L.SetField(proofTable, "ownership_amount", L.NewFunction(...))
L.SetField(proofTable, "ownership_commitment", L.NewFunction(...))
L.SetField(proofTable, "ownership_commitment_eq_proof", L.NewFunction(...))
L.SetField(proofTable, "ownership_range_proof", L.NewFunction(...))
// tos.proof — accessors (balance)
L.SetField(proofTable, "balance_amount", L.NewFunction(...))
L.SetField(proofTable, "balance_commitment_eq_proof", L.NewFunction(...))
L.SetField(tosTable, "proof", proofTable)

// tos.transcript
transcriptTable := L.NewTable()
L.SetField(transcriptTable, "new", L.NewFunction(...))
L.SetField(transcriptTable, "append_message", L.NewFunction(...))
L.SetField(transcriptTable, "append_point", L.NewFunction(...))
L.SetField(transcriptTable, "validate_and_append_point", L.NewFunction(...))
L.SetField(transcriptTable, "append_scalar", L.NewFunction(...))
L.SetField(transcriptTable, "challenge_scalar", L.NewFunction(...))
L.SetField(transcriptTable, "challenge_bytes", L.NewFunction(...))
L.SetField(tosTable, "transcript", transcriptTable)

// tos.crypto — general Ristretto-native crypto
cryptoTable := L.NewTable()
L.SetField(cryptoTable, "blake3", L.NewFunction(...))        // input → 32B hash hex
L.SetField(cryptoTable, "sha3", L.NewFunction(...))          // input → 32B hash hex (SHA3-256, not keccak)
L.SetField(cryptoTable, "sig_verify", L.NewFunction(...))    // sig, data, point → bool (Ristretto EdDSA)
L.SetField(tosTable, "crypto", cryptoTable)
```

### 2. `~/gtos/core/vm/lvm.go` — Execute() Hook

Add single call in `Execute()` after existing `tosTable` setup:

```go
registerCryptoTables(L, tosTable, l.StateDB, contractAddr, &gas, ctx.Readonly)
```

### 3. `~/gtos/core/vm/lvm_crypto_test.go` — New Test File

Test cases following existing `lvm_*_test.go` patterns:

**Ciphertext tests**:
- `add`/`sub` round-trips match Go-level `AddCompressedCiphertexts`/`SubCompressedCiphertexts`
- `add_scalar`/`sub_scalar`/`mul_scalar`/`div_scalar` correctness
- `mul_scalar` then `div_scalar` round-trip recovers original
- `div_scalar` by zero reverts
- `zero()` returns identity ciphertext
- `encrypt` produces valid ciphertext (decrypt round-trip)
- `from_parts`/`commitment`/`handle` decomposition
- `store`/`load` fidelity; `map_store`/`map_load` fidelity
- `store` reverts in readonly mode

**Ristretto tests**:
- `add(P, Q)` then `sub(result, Q)` recovers P
- `mul_scalar(P, s)` matches Go-level result
- `div_scalar` by zero reverts
- `from_bytes(to_bytes(P))` round-trip
- `identity()` is additive identity (`add(P, identity) == P`)
- `is_identity` returns true/false correctly
- `G` and `H` constants are valid non-identity points

**Scalar tests**:
- `from_u64` round-trip via `to_bytes`/`from_bytes`
- `add`/`sub`/`mul`/`div` arithmetic correctness
- `div` by zero reverts
- `invert(s)`: `mul(s, invert(s)) == ONE`
- `is_zero(ZERO)` true; `is_zero(ONE)` false
- `mul_base(ONE) == G`

**Proof tests**:
- Generate each proof type via `core/priv/prover.go`, verify in Lua contract
- Tampered proofs return false (not revert)
- `verify_range` with bits parameter (1..64)
- Batch range proof with multiple commitments
- Constructors: `new_arbitrary_range`, `new_ownership`, `new_balance` round-trip
- All accessor functions return correct values (decompose → re-compose → verify)
- `verify_multiplication`: Enc(3)×Enc(7)=Enc(21) valid; Enc(22) invalid
- `verify_division`: Enc(17)÷Enc(5)=Enc(3) remainder Enc(2) valid; wrong quotient invalid
- Comparison pattern: `sub(encB, encA)` + `verify_range` correctly distinguishes a<b vs a>b

**Transcript tests**:
- `new` + `append_message` + `challenge_scalar` deterministic output
- `append_point`/`append_scalar` match Go-level Merlin output
- `validate_and_append_point` rejects non-canonical encoding
- `challenge_bytes` length constraint (1..256)

**Crypto tests**:
- `blake3` matches Go-level BLAKE3 output
- `sha3` matches Go-level SHA3-256 output (verify it's SHA3, not keccak)
- `sig_verify` with valid/invalid Ristretto EdDSA signatures

**Gas tests**:
- Each namespace spot-checks gas consumption within expected range

### 4. `~/tolang/tol/sema/sema.go` — Type System

- `isValidAtomicTOLType`: add `"ciphertext"` case
- `isValueTOLType`: add `"ciphertext"` (can be function param/return)
- `isDefaultInitializableTOLType`: add `"ciphertext"` (default = ct_zero)
- Do NOT add to `isValidMappingKeyType` (ciphertext should not be a key)
- Support `==` and `!=` comparison; reject `<`, `>`, arithmetic operators

Note: `point` and `scalar` are NOT TOL-level types. They are only used inside
`tos.ciphertext.*` / `tos.ristretto.*` / `tos.scalar.*` calls as `bytes32` hex
strings. This avoids type system complexity — contracts pass them as `bytes32`.

### 5. `~/tolang/tol_ir_direct_lowering.go` — Code Generation

Add `storageKindCiphertext` constant alongside existing scalar/mapping/array.

`classifyStorageSlotKind`: when the leaf type resolves to `"ciphertext"`, return `storageKindCiphertext`.

`defaultValueExprForType`: add case for `"ciphertext"` → `"0x" + 128 zeros`.

Storage emit paths — for `storageKindCiphertext`:
- **Scalar store**: emit `tos.ciphertext.store(slotKey, value)`
- **Scalar load**: emit `tos.ciphertext.load(slotKey)`
- **Mapping store** (when mapping value type is ciphertext): emit `tos.ciphertext.map_store(mapName, key1, ..., value)`
- **Mapping load**: emit `tos.ciphertext.map_load(mapName, key1, ...)`

### 6. `~/tolang/cryptolib.go` — ABI Encoding

Add `"ciphertext"` case to ABI encode/decode:
- Encode: 2 consecutive 32-byte words (commitment, handle)
- Decode: consume 2 × 32-byte slots, concatenate as `"0x" + 128 hex`

### 7. `~/tolang/tol/sema/sema_test.go` — Compiler Tests

- `ciphertext` accepted as state variable, param, return type
- `ciphertext` in `mapping(agent => ciphertext)` works
- `ciphertext` rejected as mapping key
- `==` / `!=` work; `<`, `>`, `+`, `-` rejected

## Sample Contract (Confidential TRC20)

```tol
pragma tolang 0.4.0;

contract ConfidentialToken {
    mapping(agent => ciphertext) balances;
    mapping(agent => bytes32) publicKeys;

    event PublicKeyRegistered(agent indexed account, bytes32 pubkey);
    event ConfidentialTransfer(agent indexed from, agent indexed to);

    function registerPublicKey(bytes32 pubkey) external {
        set publicKeys[msg.sender] = pubkey;
        emit PublicKeyRegistered(msg.sender, pubkey);
    }

    function transfer(
        agent to,
        bytes32 commitment,
        bytes32 senderHandle,
        bytes32 receiverHandle,
        bytes32 sourceCommitment,
        bytes ctValidityProof,
        bytes commitmentEqProof,
        bytes rangeProof
    ) external {
        let senderPub: bytes32 = publicKeys[msg.sender];
        let receiverPub: bytes32 = publicKeys[to];
        require(senderPub != bytes32(0), "Sender not registered");
        require(receiverPub != bytes32(0), "Receiver not registered");

        let t: bytes32 = tos.transcript.new("confidential-transfer");

        require(
            tos.proof.verify_ct_validity(commitment, senderHandle, receiverHandle, senderPub, receiverPub, ctValidityProof, t),
            "Bad CT validity proof"
        );

        let senderCt: ciphertext = balances[msg.sender];
        let transferCt: ciphertext = tos.ciphertext.from_parts(commitment, senderHandle);
        let newSenderCt: ciphertext = tos.ciphertext.sub(senderCt, transferCt);

        require(
            tos.proof.verify_commitment_eq(senderPub, newSenderCt, sourceCommitment, commitmentEqProof, t),
            "Bad commitment eq proof"
        );
        require(tos.proof.verify_range(sourceCommitment, rangeProof, t), "Bad range proof");

        set balances[msg.sender] = newSenderCt;

        let recvTransferCt: ciphertext = tos.ciphertext.from_parts(commitment, receiverHandle);
        let recvBalance: ciphertext = balances[to];
        set balances[to] = tos.ciphertext.add(recvBalance, recvTransferCt);

        emit ConfidentialTransfer(msg.sender, to);
    }

    function balanceOf(agent account) external view returns (ciphertext) {
        return balances[account];
    }
}
```

## Implementation Sequence

| Phase | Work | Repo |
|-------|------|------|
| 1 | `lvm_crypto.go`: hex helpers + `tos.ciphertext.*` (15 functions) | gtos |
| 2 | `lvm_crypto.go`: `tos.ristretto.*` (10 functions + 2 constants) | gtos |
| 3 | `lvm_crypto.go`: `tos.scalar.*` (10 functions + 2 constants) | gtos |
| 4 | `lvm_crypto.go`: `tos.proof.*` (24 functions: 10 verify + 3 ctor + 11 accessor) | gtos |
| 5 | `lvm_crypto.go`: `tos.transcript.*` (7 functions) + `tos.crypto.*` (3 functions) | gtos |
| 6 | `lvm_crypto_test.go`: full test coverage for all 71 functions | gtos |
| 7 | Sema: `ciphertext` type + lowering: `storageKindCiphertext` + ABI encode/decode | tolang |
| 8 | Compiler tests + sample ConfidentialToken contract + integration test | both |

## Verification

```bash
# gtos: build
go build ./core/vm/... ./crypto/priv/... ./core/priv/...

# gtos: crypto primitive tests
go test -v ./core/vm/ -run "TestCiphertext|TestRistretto|TestScalar|TestProof|TestTranscript|TestCrypto" -timeout 120s

# gtos: full suite
go test -p 48 ./... -timeout 600s

# tolang: compiler tests
cd ~/tolang && go test ./tol/sema/... ./...

# integration: compile + deploy sample contract
cd ~/tolang && go run ./cmd/tolc examples/confidential_token/ConfidentialToken.tol
```
