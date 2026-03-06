# TOL System Architecture

End-to-end narrative covering the TOL compilation pipeline (tolang) and smart contract
creation/call execution (gtos).

---

## 1. System Overview

Two repositories work together:

| Repo | Role |
|------|------|
| **tolang** (`github.com/tos-network/tolang`) | TOL language compiler toolchain — source → bytecode + ABI + `.tor` packages |
| **gtos** (`github.com/tos-network/gtos`) | TOS blockchain node — deploys and executes `.tor` packages on the Lua VM |

The tolang VM is a heavily modified Lua 5.4 subset embedded inside gtos.
`gtos/core/lvm/lvm.go` imports `github.com/tos-network/tolang` and calls `Execute()`.

---

## 2. Compilation Pipeline (tolang)

```
.tol source
  └─ Lexer           tol/lexer/lexer.go
       └─ Token stream
            └─ Parser          tol/parser/parser.go
                 └─ ast.Module
                      └─ Sema              tol/sema/sema.go
                           └─ TypedModule
                                └─ Lowering        tol/lower/lower.go
                                     └─ lower.Program  (backend-agnostic IR)
                                          ├─ BuildIRFromLowered()       (runtime mode)
                                          │    tol_ir_direct_lowering.go
                                          │         └─ IRProgram
                                          │              └─ CompileIR()  compile.go
                                          │                   └─ FunctionProto
                                          │                        └─ EncodeFunctionProto()  bytecode.go
                                          │                             └─ TOLB blob
                                          │                                  └─ EncodeArtifact()  tol_artifact.go
                                          │                                       └─ .toc file
                                          └─ BuildIRFromLoweredInit()   (init mode, if constructor)
                                               tol_ir_direct_lowering.go
                                                    └─ (same chain → init .toc)

Multiple .toc + .abi  →  CompilePackage()  tol_package.go  →  .tor (ZIP)
```

### 2.1 Lexer (`tol/lexer/lexer.go`)

Tokenises UTF-8 TOL source. Produces a flat token stream including:
- Identifiers, keywords (Solidity-aligned: `function`, `returns`, `mapping`, `struct`,
  `transient`, `type`, `as`, `pragma`, `contract`, `interface`, etc.)
- Literals: integer, string, agent, selector (`0x...` 4-byte), doc-comments (`///`, `/** */`)
- 21 Solidity reserved words mapped to `TokenReserved` (emits `TOL1001` if used as identifiers)

### 2.2 Parser (`tol/parser/parser.go`)

Recursive descent parser. Produces `ast.Module` containing:
- `Package` string (from `package` declaration)
- `Contracts []ContractDecl` — each with functions, events, storage vars, constructor, etc.
- `Interfaces []InterfaceDecl`
- Pending `DocMeta` accumulated from `///`/`/** */` blocks and bound to the next declaration

### 2.3 Semantic Analysis (`tol/sema/sema.go`)

`sema.Check()` / `sema.CheckWithResolver()` performs:
- Type checking (uint256, agent, bool, string, bytes, mapping, array, struct, enum, tuple)
- Visibility and mutability checks
- Inheritance / C3 linearisation / interface conformance
- `using LibName for Type` library dispatch resolution
- `@effects`/`@bounds`/`@gas` annotation inference and validation (effects.go)
- Import resolution via `sema.FileResolver` (OS filesystem, GitHub, `.toc`/`.tor` import)

Returns a `TypedModule` (reuses `ast.Module` with types resolved).

### 2.4 Lowering (`tol/lower/lower.go`)

`lower.FromTypedContract()` converts the typed AST to a `lower.Program`:
- Flattens inheritance (constructor super-calls, modifier expansion)
- Resolves storage slot names to `__tol_s_<name>` constants
- Lowers TOL syntax to a restricted statement/expression IR
- Emits `lower.Program.Functions`, `.StorageSlots`, `.Events`, `.HasConstructor`, etc.

### 2.5 IR Lowering (`tol_ir_direct_lowering.go`)

`buildDirectIRFromLowered()` converts a `lower.Program` to a Lua AST chunk (`[]luast.Stmt`),
then wraps it into an `IRProgram` via `buildIRFromChunk()`.

Two modes controlled by `bootstrapMode`:

| Mode | Entry point function | Purpose |
|------|---------------------|---------|
| `bootstrapModeRuntime` | `BuildIRFromLowered()` | Emits `tos.oninvoke` dispatch; no constructor |
| `bootstrapModeInit` | `BuildIRFromLoweredInit()` | Emits `tos.oncreate` with constructor; no dispatch |

The generated chunk includes:
- Selector / ABI preludes (helper functions for encoding/decoding)
- Host preludes (`__tol_env_get`, `__tol_storage`, etc.)
- Event helper functions (`__tol_emit_<Name>`)
- Storage constant locals (`local __tol_s_<name> = "<hex32>"`)
- One Lua function per TOL function
- The bootstrap: either `tos.oninvoke = function(sel) ... end` or `tos.oncreate = function() ... end`

### 2.6 Codegen (`compile.go`)

`CompileIR()` compiles an `IRProgram` (Lua AST) to a `FunctionProto` (register-based VM bytecode).

### 2.7 Artifact Creation (`tol_artifact.go`)

`EncodeArtifact()` serialises a compiled artifact to `.toc` binary (see FILE_FORMATS.md §2).

Post-compile analysis (`analyzeBytecodeMetadata`):
- Walks the FunctionProto tree to compute `MaxStackSlots` and detect backward jumps (`ContainsUnboundedLoop`).

`CompileArtifactWithOptions()` is the all-in-one helper:
source → sema → lower → IR → bytecode → `EncodeArtifact`.

### 2.8 Interface Generation (`tol_interface.go`)

`BuildInterfaceWithOptions()` renders a `.abi` text file from the parsed `ast.Module`.
Only `public`/`external` functions and events are included.

### 2.9 Package Assembly (`tol_package.go`)

`CompilePackage()` orchestrates the full pipeline for one `.tol` source file:

1. Parse module → collect all contracts.
2. For each contract: compile runtime `.toc`; if it has a constructor or storage initialisers,
   also compile an init `.toc` (init/runtime split — see §3).
3. Generate `.abi` for each contract.
4. Build `manifest.json` with `name`, `version`, `contracts`, and optionally
   `main_contract` + `init_code`.
5. Pack everything into a deterministic ZIP archive.
6. Optionally sign with Ed25519 (`PackageOptions.SigningKey`).

---

## 3. Init / Runtime Split

### Motivation

EVM-style constructor-at-deploy separation: constructor logic runs once at deploy time and
is never callable from a normal call. This keeps the on-chain runtime artifact lean and
makes re-entrancy into the constructor impossible.

### Runtime `.toc`

Contains all contract functions and `tos.oninvoke` dispatch. No constructor.
Stored on-chain via `StateDB.SetCode`. Executed on every call.

**Generated Lua skeleton:**
```lua
-- preludes, helper functions, storage constants
local __tol_s_balances = "0x..."

function transfer(to, amount) ... end
function balanceOf(account) ... end

tos = tos or {}
tos.oninvoke = function(selector)
  if selector == "0xa9059cbb" then return transfer(...) end
  if selector == "0x70a08231" then return balanceOf(...) end
  -- fallback / receive handling
end
```

### Init `.toc`

Contains the constructor function and `tos.oncreate`. Not stored on-chain.
Executed exactly once at deploy time with `IsCreate = true`.

**Generated Lua skeleton:**
```lua
-- preludes, helper functions, storage constants
local __tol_s_owner = "0x..."

function __tol_constructor(initialSupply)
  -- reads tos.calldata, ABI-decodes args
  tos.set(__tol_s_totalSupply, initialSupply)
  tos.set(__tol_s_owner, tos.caller)
end

tos = tos or {}
tos.oncreate = function() return __tol_constructor() end
```

### When is an init `.toc` generated?

`contractNeedsInitCode()` returns true when:
- The contract declares a `constructor(...)`, OR
- Any storage slot has an initialiser expression.

---

## 4. Contract Deployment Flow (gtos)

### 4.1 Transaction routing

```
Deploy transaction (contractCreation = true)
  gtos/core/state_transition.go
    └─ lvm.Create(caller, pkgBytes, ctorArgs, gas, value, nonce)
```

Deployment calldata layout:
```
tx.data = [.tor ZIP bytes (variable)] [ABI-encoded constructor args (variable)]
```

`SplitDeployDataAndConstructorArgs()` locates the ZIP EOCD (End of Central Directory)
signature by scanning backwards, validates the ZIP structure, and returns `(pkgBytes, ctorArgs)`.

### 4.2 `lvm.Create()` steps

```
lvm.Create(caller, pkgBytes, ctorArgs, gas, value, nonce)
  │
  ├─ Reject if not .tor  (raw .toc deployment is not accepted)
  ├─ Reject if len(pkgBytes) > params.MaxCodeSize
  ├─ contractAddr = crypto.CreateAddress(callerAddr, nonce)
  ├─ Collision check: nonce != 0 or code != empty  → ErrContractAddressCollision
  ├─ lua.DecodePackage(pkgBytes)  → validate manifest + all .toc/.abi entries
  ├─ Validate dispatch tag uniqueness (keccak256("pkg:" + name)[:4]) across contracts
  │
  ├─ [if main_contract + init_code present — init/runtime split]
  │    ├─ Validate main_contract exists in contracts and has a .toc path
  │    ├─ Validate init_code is NOT listed in contracts
  │    ├─ DecodeArtifact(pkg.Files[init_code])  → initArtifactBytecode
  │    └─ buildRuntimePackage()
  │         strips init_code file + main_contract/init_code/publisher_key/signature manifest fields
  │         → runtimePkgBytes
  │
  ├─ [else] runtimePkgBytes = pkgBytes  (legacy: no constructor)
  │
  ├─ Charge code storage gas: len(runtimePkgBytes) × 200 gas/byte
  ├─ StateDB.Snapshot()
  ├─ Transfer value:  callerAddr → contractAddr  (if value > 0)
  ├─ StateDB.CreateAccount(contractAddr)
  ├─ StateDB.SetNonce(contractAddr, 1)
  ├─ StateDB.SetCode(contractAddr, runtimePkgBytes)   ← stored on-chain
  ├─ StateDB.SetNonce(callerAddr, nonce+1)
  │
  └─ [if init/runtime split]
       Execute(stateDB, blockCtx, chainConfig, ctorCtx{IsCreate:true, Data:ctorArgs},
               initArtifactBytecode, gas)
         → loads init module → tos.oncreate defined
         → tolDispatch(IsCreate=true) → tos.oncreate()
              → __tol_constructor() reads tos.calldata → ABI-decodes ctorArgs → initialises storage
       On error: StateDB.RevertToSnapshot()
```

### 4.3 `buildRuntimePackage()` details

Parses the manifest retaining only `name`, `version`, `contracts` (drops `main_contract`,
`init_code`, `package`, `publisher_key`, `signature`).
Removes the `init_code` file from the files map.
Re-encodes via `lua.EncodePackage()` → produces a clean runtime-only package.

---

## 5. Contract Call Flow (gtos)

### 5.1 Transaction routing

```
Call transaction (contractCreation = false)
  gtos/core/state_transition.go
    └─ lvm.Call(caller, contractAddr, calldata, gas, value)
```

### 5.2 `lvm.Call()` steps

```
lvm.Call(caller, contractAddr, input, gas, value)
  │
  ├─ Depth check: l.depth > callCreateDepth (1024) → ErrDepth
  ├─ Transfer value:  callerAddr → contractAddr  (if value > 0)
  ├─ StateDB.Snapshot()
  ├─ code = StateDB.GetCode(contractAddr)   ← on-chain .tor bytes
  └─ Execute(stateDB, blockCtx, chainConfig, ctx, code, gas)
       → see §5.3
       On error: StateDB.RevertToSnapshot()
```

### 5.3 `Execute()` routing

```
Execute(stateDB, blockCtx, chainConfig, ctx, src, gasLimit)
  │
  ├─ if IsPackage(src) [.tor ZIP magic]
  │    └─ executePackage()
  │         ├─ dispatchTag = ctx.Data[:4]
  │         ├─ DecodePackage(src)
  │         ├─ For each contract in manifest:
  │         │    tag = keccak256("pkg:" + contract.Name)[:4]
  │         │    if tag == dispatchTag:
  │         │       art = DecodeArtifact(pkg.Files[toc_path])
  │         │       childCtx.Data = ctx.Data[4:]   ← strip dispatch tag
  │         │       return Execute(..., art.Bytecode, gasLimit)
  │         └─ error if no contract matched
  │
  ├─ if IsArtifact(src) [.toc TOC\0 magic]  ← internal/test path only; production always uses .tor
  │    └─ DecodeArtifact(src) → src = art.Bytecode
  │
  ├─ Setup Lua state:
  │    L = lua.NewState(...)
  │    L.SetGasLimit(gasLimit)
  │
  ├─ Build tos table (see §5.4)
  ├─ Register all tos.* primitives
  │
  ├─ Execute Lua module (src = TOLB bytecode or Lua source):
  │    L.DoByteCode(src) / L.DoString(src)
  │    → defines contract functions + sets tos.oninvoke or tos.oncreate
  │
  └─ Post-module dispatch (tolDispatch):
       if ctx.IsCreate:
           fn = tos.oncreate  (Lua closure)
           L.PCall(fn, 0, ...)
       else:
           fn = tos.oninvoke  (Lua closure)
           selector = "0x" + hex(ctx.Data[:4])  (or nil if len < 4)
           L.PCall(fn, selector, ...)
               → looks up selector in dispatch table
               → ABI-decodes calldata from tos.calldata
               → calls contract function
               → tos.result(abiEncoded) → capturedResult
```

`tolDispatch` is a no-op for raw Lua contracts (where `tos.oncreate`/`tos.oninvoke`
is a Go function, `IsG=true`).

### 5.4 `tos` table context

Execute populates the `tos` global table before running the module:

| Field | Type | Value |
|-------|------|-------|
| `tos.caller` | string | `ctx.From.Hex()` — immediate msg.sender |
| `tos.value` | uint256 | `ctx.Value` in wei |
| `tos.calldata` | string | `"0x" + hex(ctx.Data)` — full calldata for this frame |
| `tos.msg.sender` | string | same as `tos.caller` |
| `tos.msg.value` | uint256 | same as `tos.value` |
| `tos.msg.data` | string | `"0x" + hex(ctx.Data)` |
| `tos.msg.sig` | string | `"0x" + hex(ctx.Data[:4])` or `"0x"` |
| `tos.tx.origin` | string | `ctx.TxOrigin.Hex()` — original EOA, constant |
| `tos.tx.gasprice` | uint256 | `ctx.TxPrice` |
| `tos.block.number` | uint256 | `blockCtx.BlockNumber` |
| `tos.block.timestamp` | uint256 | `blockCtx.Time` |
| `tos.block.coinbase` | string | `blockCtx.Coinbase.Hex()` |
| `tos.block.chainid` | uint256 | `chainConfig.ChainID` |
| `tos.block.gaslimit` | uint256 | `blockCtx.GasLimit` |
| `tos.block.basefee` | uint256 | `blockCtx.BaseFee` |

After the tos table is built, it is set as the Lua global `tos`.

TOL-compiled code accesses these via `__tol_env_get(scope, key)`, a generated helper
that reads `tos[scope][key]`. Unit test code may also set `msg`/`tx`/`block` globals
directly (test compatibility path in `__tol_env_get`).

---

## 6. Calldata Layout

### 6.1 Deploy transaction (`tx.data`)

```
[.tor ZIP bytes (variable)] [ABI-encoded constructor args (variable)]
```

The boundary is found by `SplitDeployDataAndConstructorArgs()`, which scans for the ZIP
EOCD (`PK\x05\x06`) signature and validates the candidate ZIP.

### 6.2 Call to `.tor` contract (normal call)

```
[4B pkg dispatch tag] [4B function selector] [ABI-encoded args...]
```

- **Dispatch tag** = `keccak256("pkg:" + contractName)[:4]` — routes to the right contract in the package.
- `executePackage()` strips the 4-byte dispatch tag; the remaining bytes (`selector + args`) become `ctx.Data` for the inner `Execute` call.
- The inner contract sees `tos.calldata = "0x" + hex(selector + args)`.

### 6.3 Call to bare `.toc` artifact (internal path only)

```
[4B function selector] [ABI-encoded args...]
```

- **Production deployment only accepts `.tor`** — `lvm.Create()` rejects any input that is not a valid ZIP package.
- This path is reached internally after `executePackage()` has already stripped the 4-byte dispatch tag and extracted the individual `.toc` artifact bytecode, or in unit tests that run `.toc` directly.
- `tolDispatch` extracts `selector = "0x" + hex(data[:4])` and calls `tos.oninvoke(selector)`.

### 6.4 Receive call (no selector)

If `len(ctx.Data) < 4`, `tolDispatch` passes `nil` as the selector to `tos.oninvoke`.
The generated dispatch table maps `nil` to `__tol_receive()` (if `receive() payable` is declared).

---

## 7. Gas Model

### 7.1 Runtime gas (gtos `lvm.go`)

These are the actual costs charged by the gtos runtime:

| Operation | Cost |
|-----------|------|
| Storage read (`tos.get`) | 100 gas |
| Storage write (`tos.set`) | 5,000 gas |
| Balance query (`tos.balance`) | 400 gas |
| Value transfer (`tos.transfer`/`tos.send`) | 2,300 gas |
| Log emission base (`tos.emit`) | 375 gas |
| Log topic (indexed param, each) | 375 gas |
| Log data (per byte) | 8 gas |
| Contract creation base (`gasDeploy`) | 32,000 gas |
| Code storage (`gasDeployByte`) | 200 gas/byte of runtime package |
| VM opcode (per Lua instruction) | 1 gas |
| Child call gas | Deducted from parent budget |

Nested `tos.call` depth limit: 8 (independent of the outer `callCreateDepth` of 1024).

### 7.2 Gas accounting invariant

```
vmGasUsed + totalChildGas + primGasCharged ≤ gasLimit
```

`chargePrimGas(cost)` adjusts the VM opcode ceiling (`L.SetGasLimit`) after each primitive
charge to enforce the invariant. A VM opcode that would exceed the ceiling triggers
`"lua: gas limit exceeded"`.

### 7.3 Compile-time gas model (ABI `gas_model` field)

The `abi_json.gas_model` embedded in `.toc` contains the constants used for **static
`@gas upper` estimation** at compile time:

| Field | Value |
|-------|-------|
| `sload` | 2,100 |
| `sstore` | 20,000 |
| `log_base` | 375 |

These are intentionally more conservative than the runtime costs to give agents a worst-case
budget estimate. The runtime always uses the gtos constants in §7.1, not the ABI values.
Old compiled contracts are not invalidated when runtime gas constants change.

---

## 8. Storage Model

### 8.1 Compile-time slot names (tolang)

At compile time, `keccak256("tol.slot.<ContractName>.<name>")` is computed for each storage
variable and emitted as a Lua constant `local __tol_s_<name> = "<32-byte hex>"`.

The runtime `.toc` stores and reads using these keys:
```lua
tos.set(__tol_s_totalSupply, newValue)
local v = tos.get(__tol_s_totalSupply)
```

### 8.2 Runtime slot derivation (gtos)

`tos.get(key)` / `tos.set(key, val)` in gtos map the key string to an EVM storage slot:

```go
StorageSlot(key) = keccak256("gtos.lua.storage." + key)
```

The key passed is the hex32 string from the compile-time constant (e.g., `"0xabcd..."`),
so the effective slot is: `keccak256("gtos.lua.storage." + keccak256("tol.slot.MyContract.totalSupply"))`.

### 8.3 Mapping slots

```go
MapSlot(mapName, keys[]) = keccak256(keccak256(key_N) || prev_slot)
```

Base: `keccak256("gtos.lua.map." + mapName)`. Each key is hashed before mixing.

### 8.4 Array slots

```go
ArrLenSlot(name) = keccak256("gtos.lua.arr." + name)
ArrElemSlot(base, i) = keccak256(base || uint64_BE(i))
```

### 8.5 String slots

```go
StrLenSlot(name) = keccak256("gtos.lua.str." + name)
StrChunkSlot(base, i) = keccak256(base || uint32_BE(i))
```

Strings are stored as a length slot and 32-byte chunks.

---

## 9. Contradictions & Notes

### 9.1 Gas model discrepancy (compile-time vs. runtime)

The compile-time `gas_model` in ABI JSON (`sload=2100, sstore=20000`) differs significantly
from the actual gtos runtime costs (`gasSLoad=100, gasSStore=5000`). This is intentional:
- The ABI values are conservative upper bounds used by the static estimator for `@gas upper` annotations.
- Runtime costs may be tuned independently.
- Agents and tools should use ABI `gas_upper` for budgeting, not the `gas_model` constants directly.

### 9.2 Two independent version numbers

- **TOLB format version** = 1 (internal bytecode blob inside `.toc`)
- **Artifact format version** = 1 (`.toc` container format)

Both are validated independently on decode. A version mismatch in either layer rejects the artifact.

### 9.3 Dispatch tag is separate from function selector

Calling a contract inside a `.tor` requires two 4-byte prefixes:
1. **Package dispatch tag** = `keccak256("pkg:" + contractName)[:4]` — routes to the right `.toc`.
2. **Function selector** = `keccak256("<name>(<types>)")[:4]` — routes to the right function.

The compiler validates dispatch tag uniqueness per package. Callers must prepend both
when constructing calldata for a `.tor` contract.

### 9.4 Runtime package strips more than just init_code

`buildRuntimePackage()` retains only `name`, `version`, and `contracts` from the manifest.
The following fields are dropped: `main_contract`, `init_code`, `package`, `publisher_key`,
`signature`. Ed25519 signatures are for off-chain distribution
integrity; they are not verified on-chain.

### 9.5 tos.oncreate is a Go function for raw Lua contracts

`tolDispatch` skips dispatch when `tos.oncreate` or `tos.oninvoke` is a Go function (`IsG=true`).
This is the raw Lua contract path where dispatch happens inline in the module body.
TOL-compiled contracts always set these to Lua closures.

### 9.6 tolDispatch passes only the selector to tos.oninvoke

```go
selector = "0x" + hex(ctx.Data[:4])   // or nil if len(Data) < 4
L.PCall(oninvoke_fn, selector)
```

The full calldata is NOT passed as an argument. The TOL-generated `tos.oninvoke` reads
`tos.calldata` directly to decode arguments. This keeps the Lua dispatch table lightweight.
