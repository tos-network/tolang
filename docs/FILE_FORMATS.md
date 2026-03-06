# TOL File Formats

Definitive specification for `.toc`, `.abi`, `.tor`, and the internal TOLB bytecode blob.
All integers are big-endian unless noted otherwise.

---

## 1. Overview

| Ext   | Full name                  | Container     | Magic (hex)       | Purpose |
|-------|----------------------------|---------------|-------------------|---------|
| `.tol` | TOL Source               | UTF-8 text    | —                 | Source code |
| `.toc` | TOL Compiled Contract    | Binary        | `54 4F 43 00`     | Bytecode + ABI + metadata |
| `.abi` | TOL Interface            | UTF-8 text    | —                 | ABI interface declaration |
| `.tor` | TOL Package Archive      | ZIP           | `50 4B 03 04`     | Multi-artifact distribution package |

The `.tor` ZIP magic (`PK\x03\x04`) is the standard PKZIP local-file-header signature.

---

## 2. `.toc` — Compiled Artifact

### 2.1 Magic & version

| Offset | Length | Value | Description |
|--------|--------|-------|-------------|
| 0      | 4 B    | `54 4F 43 00` (`TOC\0`) | Magic |
| 4      | 2 B    | `0x0001` (uint16 BE) | `ArtifactFormatVersion = 1` |

### 2.2 Sequential fields

Fields follow immediately after the header, in this exact order:

```
[4B]  magic = TOC\0
[2B]  version = 1  (uint16 BE)
[str] compiler     e.g. "tolang/0.2.0"
[str] contract_name
[lb]  bytecode     (TOLB blob — see §5)
[lb]  abi_json     (UTF-8 JSON, may be 0 bytes)
[lb]  storage_layout_json  (UTF-8 JSON, may be 0 bytes)
[32B] source_hash  (keccak256 of source UTF-8, raw bytes)
[32B] bytecode_hash (keccak256 of bytecode bytes, raw bytes)
[4B]  max_stack_slots  (uint32 BE)
[4B]  bytecode_len     (uint32 BE — byte length of TOLB blob)
[1B]  contains_unbounded_loop  (0 or 1)
```

**Encoding conventions:**
- `[str]` — `uint32 BE` length prefix followed by UTF-8 bytes (no null terminator).
- `[lb]` — `uint32 BE` length prefix followed by raw bytes; length may be 0.
- No alignment padding between fields.

### 2.3 `abi_json` structure

```json
{
  "gas_model": {
    "version": "tolang/0.2.0",
    "sload": 2100,
    "sstore": 20000,
    "log_base": 375
  },
  "functions": [
    {
      "name": "transfer",
      "visibility": "public",
      "selector": "0xa9059cbb",
      "params": ["agent", "u256"],
      "returns": ["bool"],
      "doc": {
        "notice": "...",
        "effects": {
          "reads":  ["balances"],
          "writes": ["balances"],
          "emits":  ["Transfer"],
          "calls":  [{"cap": "max_calls=1", "iface": "IERC20", "selector": "0x...", "max_gas": 5000, "max_calls": 1}]
        },
        "bounds": ["amount>0"],
        "gas_upper": 30000,
        "non_composable": false
      }
    }
  ],
  "events": [
    { "name": "Transfer", "params": ["agent", "agent", "u256"] }
  ]
}
```

**Notes:**
- Only `public` and `external` functions appear in `functions`.
- `doc` is omitted when no `@effects`/`@bounds`/`@gas` annotations are present.
- `gas_model` contains the **compile-time** cost constants used for static gas estimation;
  actual runtime charges in gtos may differ (see §6 in ARCHITECTURE.md).
- ABI type strings are normalised (whitespace collapsed, no trailing spaces).
- Function `selector` is `keccak256("<name>(<types>)")[:4]` as `0x`-prefixed lowercase hex,
  unless `@selector("0x...")` overrides it in source.

### 2.4 `storage_layout_json` structure

```json
{
  "slots": [
    {
      "name": "totalSupply",
      "type": "u256",
      "canonical_hash": "0x..."
    }
  ]
}
```

`canonical_hash` = `keccak256("tol.slot.<ContractName>.<name>")` as `0x`-prefixed hex.

### 2.5 Validation on decode

1. Magic must equal `TOC\0` exactly.
2. `version` must equal `1`; any other value is rejected.
3. `contract_name` must be non-empty after trimming whitespace.
4. `bytecode` must be non-empty.
5. `keccak256(bytecode) == bytecode_hash` (embedded raw bytes, not hex).
6. `bytecode` must decode successfully as a valid TOLB blob (§5).
7. If `abi_json` is non-empty it must be valid JSON.
8. If `storage_layout_json` is non-empty it must be valid JSON.

---

## 3. `.abi` — Interface File

Pure UTF-8 text. No binary encoding, no compression.

### 3.1 Structure

```
pragma tolang <version>;

interface <Name> {
  @selector("0x...")   -- optional 4-byte selector hint
  function <name>(<Type> <arg>, ...) [modifiers] [returns (<Type> <ret>, ...)];

  event <name>(<Type> <arg> [indexed], ...);
}
```

**Example:**
```
pragma tolang 0.2;

interface ITRC20 {
  @selector("0xa9059cbb")
  function transfer(agent to, u256 amount) public returns (bool ok);

  event Transfer(agent from, agent to, u256 value);
}
```

### 3.2 Rules

- First non-blank, non-comment line must be `pragma <lang> <version>;`.
  `<lang>` is typically `tolang`; the validator accepts any single token.
- Exactly one `interface` block per file.
- Function declarations: must end with `;`; must contain a `(...)` parameter list;
  function names must be unique within the interface and must not collide with event names.
- `@selector("0x...")` may precede a `function` line; value must be `0x` followed by
  exactly 8 hex digits (case-insensitive).
- Event declarations: must end with `;`; at most 3 `indexed` parameters per event;
  `indexed` appears as a suffix after the parameter name.
- Comments: `--` introduces a single-line comment (stripped before parsing).
- The `function` keyword is canonical; the legacy `fn` alias is also accepted by the parser.

### 3.3 Generation

`BuildInterfaceWithOptions()` in `tol_interface.go` renders a `.abi` from a parsed
`ast.Module`. Only `public` and `external` functions are included.

---

## 4. `.tor` — Package Archive

Standard ZIP (PKZIP) with deterministic properties to allow reproducible builds.

### 4.1 ZIP properties

| Property | Value |
|----------|-------|
| Compression | `zip.Store` (no compression — method 0) |
| File order | Alphabetically sorted by entry path (manifest.json first) |
| Modification time | 1980-01-01 00:00:00 UTC (fixed) |
| ZIP comment | None |
| Extra fields | None |

### 4.2 Required entry: `manifest.json`

Always the first ZIP entry. UTF-8 JSON, no binary encoding.

```json
{
  "name": "trc20",
  "package": "com.example.trc20",
  "version": "0.1.0",
  "main_contract": "TRC20",
  "init_code": "init/TRC20_init.toc",
  "contracts": [
    {
      "name": "TRC20",
      "toc": "bytecode/TRC20.toc",
      "abi": "interfaces/ITRC20.abi"
    }
  ],
  "publisher_key": "...",
  "signature": "..."
}
```

**Required fields:** `name`, `version`, `contracts`.

**Optional fields:**
- `package` — fully-qualified dotted package path (from `package` declaration in source).
- `main_contract` — name of the contract with a constructor (for init/runtime split).
- `init_code` — path to the init artifact within the archive (omitted if no constructor).
- `publisher_key` / `signature` — Ed25519 signing fields (see §4.4); omitted for unsigned packages.

**Contracts array:** each entry must declare at least one of `toc` or `toi`.
Interface-only entries (no `toc`) are allowed for abstract interfaces.

### 4.3 Conventional file paths

| File | Path |
|------|------|
| Runtime artifact | `bytecode/<Name>.toc` |
| Init artifact | `init/<Name>_init.toc` |
| Interface | `interfaces/I<Name>.abi` |
| Source (optional) | `sources/<Name>.tol` |
| Top-level interface | `interfaces/<Name>.abi` |

### 4.4 Signing

Packages may be signed with Ed25519 for off-chain integrity verification.
**Signatures are stripped on-chain** before the runtime package is stored (see ARCHITECTURE.md §4).

**Signing payload:**
```
payload = keccak256(manifest_without_sig_fields || file1_contents || file2_contents || ...)
```

- `manifest_without_sig_fields` = manifest JSON with `"signature"` key removed.
- Files are concatenated in ascending alphabetical order by entry path.
- No delimiters between file contents.
- Algorithm: Ed25519; private key seed is 32 raw bytes.

**Embedded fields in manifest:**
- `"publisher_key"` — hex-encoded Ed25519 public key (64 hex chars = 32 bytes).
- `"signature"` — hex-encoded Ed25519 signature (128 hex chars = 64 bytes).

Unsigned packages (no `"signature"` field) are always accepted by the runtime.
A package with `"signature"` but missing `"publisher_key"` is rejected.

### 4.5 Validation on decode

1. ZIP magic `PK\x03\x04` at offset 0.
2. `manifest.json` entry must be present and contain valid JSON.
3. `name` and `version` fields must be non-empty.
4. `name` must not equal `tol.lang` or start with `tol.lang.` (reserved namespace).
5. All `toc` and `toi` paths referenced in `contracts` must exist as ZIP entries.
6. Duplicate contract names are rejected.
7. Dispatch tag collisions (`keccak256("pkg:" + name)[:4]`) between contracts are rejected.
8. If `"signature"` is present, Ed25519 verification must pass.
9. All `.toc` entries are decoded and validated.
10. All `.abi` entries are structurally validated.

### 4.6 Reserved namespace

`tol.lang` and any package name starting with `tol.lang.` are reserved for the TOS platform.
`CompilePackage` and `DecodePackage` both reject such names with error code `TOL4001`.

---

## 5. TOLB — Internal Bytecode Blob

TOLB is the binary encoding of a compiled Lua `FunctionProto`.
It is embedded inside `.toc` files as the `bytecode` field. It is **not** a standalone file format.

### 5.1 Layout

```
[4B]   magic = 54 4F 4C 42  ("TOLB")
[2B]   version = 1  (uint16 BE)  (BytecodeFormatVersion)
[str]  vm_id           (see §5.2)
[4B]   payload_len     (uint32 BE — byte count of FunctionProto payload)
[N B]  payload         (serialised FunctionProto — see §5.3)
[32B]  checksum        (SHA-256 of payload bytes)
```

`[str]` encoding: `uint32 BE` length prefix + UTF-8 bytes (same as `.toc`).

### 5.2 VM ID string

```
pkg=<PackageName>-<PackageVersion>;lua=<LuaVersion>;numbit=<LUint256Bit>;opmax=<opCodeMax>
```

Example: `pkg=tolang-0.2.0;lua=Lua 5.4;numbit=256;opmax=96`

The VM ID binds the bytecode to a specific compiler + VM configuration.
Decoding fails with an error if the runtime VM ID does not match exactly.

### 5.3 Payload — FunctionProto

The payload contains a recursively serialised `FunctionProto` (the top-level chunk function).
Nested function prototypes are embedded inline. The payload encoding is an internal format
defined in `bytecode.go`; it is not Lua 5.4 dump format.

### 5.4 Validation on decode

1. Magic must equal `TOLB` exactly.
2. `version` must equal `3`; any other value is rejected.
3. `vm_id` must match the decoding runtime's own VM ID string exactly.
4. `SHA-256(payload) == checksum`.
5. Payload must deserialise as a valid FunctionProto with no trailing bytes.
