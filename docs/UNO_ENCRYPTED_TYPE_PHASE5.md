# Implementation Plan: `uno` Encrypted Type (Phase 5 Contract Ciphertext Ops)

## Context

GTOS privacy (UNO) works at the protocol level (Shield/Transfer/Unshield) but smart contracts cannot manipulate encrypted values. This plan implements Phase 5 from the privacy roadmap (~72% → ~80%), adding the `uno` first-class encrypted type to the LVM and TOL compiler per `~/tolang/docs/LVM_HE_OPCODES_PLAN_V2.md`.

**Architecture**: TOL compiler desugars `a.add(b)` → `tos.ciphertext.add(a, b)` Lua calls. LVM registers `tos.ciphertext` sub-table with 22 Go functions. Tier 1 ops use existing `crypto/priv` ElGamal functions. Tier 2 ops verify ZK proof bundles from tx calldata (initially stubbed).

## Phase 1: gtos — ProofBundle type

**New file**: `core/vm/proof_bundle.go`

```go
type ProofEntry struct {
    Op         string      // "mul","div","rem","lt","gt","eq","min","max","select","verify_transfer","verify_eq"
    InputHash  common.Hash // keccak256(op || input_a || input_b)
    ResultData []byte      // 64B ciphertext or 1B bool
    Proof      []byte      // ZK proof bytes
}

type ProofBundle struct { entries []ProofEntry; cursor int }

func (pb *ProofBundle) Next(op string, inputs ...[]byte) (*ProofEntry, error)
func ExtractProofBundle(data []byte) (*ProofBundle, []byte)  // splits calldata, returns (bundle, stripped_abi_data)
```

Binary format: `[abi calldata] [0x50424E44 "PBND" magic] [count u16] [entries...]`
Entry: `[op_tag u8] [input_hash 32B] [result_len u16] [result...] [proof_len u16] [proof...]`

**New file**: `core/vm/proof_bundle_test.go` — encode/decode round-trip, empty bundle, malformed data, Next() mismatch/exhaustion.

**Verify**: `go test ./core/vm/ -run TestProofBundle`

## Phase 2: gtos — lvm_crypto.go Tier 1 (9 native functions)

**New file**: `core/vm/lvm_crypto.go`

Registration function:
```go
func registerCiphertextTable(L *lua.LState, tosTable *lua.LTable,
    chargePrimGas func(uint64), readonly bool, proofBundle *ProofBundle)
```

Creates `ctTable := L.NewTable()`, sets `L.SetField(tosTable, "ciphertext", ctTable)`.

| # | Function | Calls | Gas |
|---|----------|-------|-----|
| 1 | `add(a,b)` | `cryptopriv.AddCompressedCiphertexts(a64,b64)` | 8000 |
| 2 | `sub(a,b)` | `cryptopriv.SubCompressedCiphertexts(a64,b64)` | 8000 |
| 3 | `add_scalar(ct,n)` | `cryptopriv.AddAmountCompressed(ct64,n)` | 6000 |
| 4 | `sub_scalar(ct,n)` | `cryptopriv.SubAmountCompressed(ct64,n)` | 6000 |
| 5 | `mul_scalar(ct,n)` | n→scalar32, `cryptopriv.MulScalarCompressed(ct64,s32)` | 10000 |
| 6 | `div_scalar(ct,n)` | n→scalar32→invert, `MulScalarCompressed(ct64,inv32)` | 12000 |
| 7 | `zero()` | `corepriv.ZeroCiphertext()` → hex | 100 |
| 8 | `encrypt(pk,amt)` | `cryptopriv.Encrypt(pub32,amount)` | 15000 |
| 9 | `from_parts(c,h)` | concatenate two bytes32 hex → ciphertext hex | 100 |

Internal helpers: `parseCiphertextHex`, `ciphertextToHex`, `parseBytes32Hex`.

Key files used:
- `crypto/priv/elgamal.go` — `AddCompressedCiphertexts`, `SubCompressedCiphertexts`, `AddAmountCompressed`, `SubAmountCompressed`, `MulScalarCompressed`, `Encrypt`, `ZeroCiphertextCompressed`
- `core/priv/zero.go` — `ZeroCiphertext()`

## Phase 3: gtos — lvm_crypto.go Tier 2 + accessors + verification (13 functions)

**Same file**: `core/vm/lvm_crypto.go`

Tier 2 pattern: parse args → charge gas → `proofBundle.Next(op, a, b)` → verify proof → return result.

| # | Function | Proof verification | Gas |
|---|----------|--------------------|-----|
| 10 | `mul(a,b)` → uno | **STUBBED** (TODO: Sigma protocol) | 200000 |
| 11 | `div(a,b)` → uno | **STUBBED** | 200000 |
| 12 | `rem(a,b)` → uno | **STUBBED** | 200000 |
| 13 | `lt(a,b)` → bool | **STUBBED** (TODO: sub + range proof) | 160000 |
| 14 | `gt(a,b)` → bool | **STUBBED** | 160000 |
| 15 | `eq(a,b)` → bool | **STUBBED** (TODO: sub + zero check) | 150000 |
| 16 | `min(a,b)` → uno | **STUBBED** | 170000 |
| 17 | `max(a,b)` → uno | **STUBBED** | 170000 |
| 18 | `select(c,a,b)` → uno | **STUBBED** | 160000 |
| 19 | `commitment(ct)` → bytes32 | N/A (hex slice) | 100 |
| 20 | `handle(ct)` → bytes32 | N/A (hex slice) | 100 |
| 21 | `verify_transfer(ct,sPub,rPub)` → bool | **REAL**: `corepriv.VerifyCiphertextValidityProof` | 100000 |
| 22 | `verify_eq(ct,commit,pk)` → bool | **REAL**: `corepriv.VerifyCommitmentEqProof` | 100000 |

"STUBBED" means: InputHash is still enforced (prevents result substitution), but the ZK proof bytes are not cryptographically verified. Marked with `// TODO: implement ZK proof verification`.

Key files used:
- `core/priv/verify.go` — `VerifyCiphertextValidityProof` (line 25), `VerifyCommitmentEqProof` (line 44)

## Phase 4: gtos — Hook Execute() + tests

**Modified file**: `core/vm/lvm.go` (~line 889)

Reorder: call `ExtractProofBundle(ctx.Data)` BEFORE calldata hex encoding, use stripped data for `msg.data`/`tos.calldata`, pass bundle to `registerCiphertextTable`.

```go
proofBundle, strippedData := ExtractProofBundle(ctx.Data)
// ... use strippedData for calldataHex / msg.data ...
registerCiphertextTable(L, tosTable, chargePrimGas, ctx.Readonly, proofBundle)
```

**New file**: `core/vm/lvm_crypto_test.go`

Uses existing test helpers from `lvm_agent_test.go`: `newAgentTestState()`, `newBlockCtx()`, `runLua()`.
For Tier 2 tests: modified `runLuaWithBundle()` that appends proof bundle to `ctx.Data`.

Test cases: Tier 1 round-trips, Tier 2 with proof bundles, missing/wrong bundle reverts, verify_transfer/verify_eq with real proofs, gas checks, readonly allowed for reads.

**Verify**: `go test -v ./core/vm/ -run TestCt -timeout 120s`

## Phase 5: tolang — sema.go `uno` type recognition

**Modified file**: `tol/sema/sema.go`

| Function | Line | Change |
|----------|------|--------|
| `isValidAtomicTOLType` | 4737 | Add `"uno"` to switch |
| `isValueTOLType` | 4760 | Add `"uno"` to switch |
| `isDefaultInitializableTOLType` | 2149 | Add `"uno"` to switch |
| `isValidMappingKeyType` | 4663 | Do NOT add (uno can't be key) |
| `checkStorageExpr` | 3194 | Add uno method validation block |
| member access | 3259 | Add `commitment`/`handle` as uno properties |

Method validation: add `unoMethods map[string]bool` with all 18 instance methods, check callee type is `"uno"`.

Static methods: detect `uno.zero()`, `uno.encrypt(pk,amt)`, `uno.select(c,a,b)`, `uno.from_parts(c,h)` as type-qualified calls.

Return type helper:
```go
func unoMethodReturnType(method string) string {
    switch method {
    case "lt","gt","eq","verify_transfer","verify_eq": return "bool"
    case "commitment","handle": return "bytes32"
    default: return "uno"
    }
}
```

## Phase 6: tolang — tol_ir_direct_lowering.go desugaring + storage

**Modified file**: `tol_ir_direct_lowering.go`

**New function**: `lowerUnoMethodExpr(ctx, e) (luast.Expr, bool, error)`
- Instance methods: `a.add(b)` → `tos.ciphertext.add(a_lua, b_lua)` via `FuncCallExpr` with `AttrGetExpr` chain
- Static methods: `uno.zero()` → `tos.ciphertext.zero()` when `e.Callee.Object.Value == "uno"`

**Hook into `tolExprToLua`** (line 4248, "call" case): add before oracle intercept.

**Storage for uno**:
- Load: emit `tos.ciphertext.from_parts(__tol_sload(commit_slot), __tol_sload(handle_slot))`
- Store: emit two `__tol_sstore` calls (commitment slot + handle slot derived via `keccak256(slot .. ".h")`)
- Detect in `lowerStorageStoreStmt` (line 6311) and `lowerStorageLoadExpr` (line 6340)

**`defaultValueExprForType`** (line 2743): add `"uno"` → emit `tos.ciphertext.zero()` call.

**Binary ops**: `==`/`!=` on uno → desugar to `tos.ciphertext.eq(a,b)` / `not tos.ciphertext.eq(a,b)`. Reject `+`,`-`,`*`,`/`,`<`,`>` on uno.

## Phase 7: tolang — ABI encoding + tests

**Modified file**: `cryptolib.go`

`abiDecodeSlot` (line 202): `"uno"` consumes 2 consecutive 32B slots, concatenates as `"0x" + 128 hex`.
`abiEncodeSlot`: `"uno"` splits 128-hex into two 32B slots.

**Modified file**: `tol/sema/sema_test.go` — tests for uno type acceptance/rejection, method validation, operator restrictions.

**New file**: `tol_ir_direct_lowering_uno_test.go` — tests for Lua output correctness.

**Verify**: `cd ~/tolang && go test ./tol/sema/... ./...`

## Phase 8: Integration

Compile `examples/confidential_token/ConfidentialToken.tol` end-to-end.
Execute in test LVM with proof bundles.

**Verify**:
```bash
cd ~/tolang && go run ./cmd/tolc examples/confidential_token/ConfidentialToken.tol
cd ~/gtos && go test -p 96 ./... -timeout 600s
```

## Execution Order

Phases 1-4 (gtos) and Phases 5-7 (tolang) are **independent** and can be developed in parallel. Phase 8 requires both complete.

```
gtos:   Phase 1 → Phase 2 → Phase 3 → Phase 4
                                                  → Phase 8
tolang: Phase 5 → Phase 6 → Phase 7             ↗
```
