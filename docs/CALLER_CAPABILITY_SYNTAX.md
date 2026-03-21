# Caller Capability Syntax

**Status: IMPLEMENTED (2026-03-21)**

## Purpose

Compiler-enforced caller capability syntax via `@requires(caller: Cap)`.
Moves common access-control patterns out of hand-rolled contract logic and
into the language, sema, lowering, artifact, and metadata layers.

---

## 1. Syntax

```solidity
contract Treasury {
    capability OwnerCap;
    capability SpenderCap;

    /// @requires(caller: OwnerCap)
    function withdraw(u256 amount) public {
        // only callable by holders of OwnerCap
    }

    /// @requires(caller: SpenderCap)
    function spend(agent recipient, u256 amount) public {
        // only callable by holders of SpenderCap
    }

    function balance() public view returns (u256 bal) {
        // callable by anyone — no @requires
        return total;
    }
}
```

**Supported forms:**
- `@requires(caller: CapName)` — named key form
- `@requires CapName` — bare form (shorthand)

Capabilities are declared with `capability CapName;` at contract level or
module level.

---

## 2. Semantic Rules

### Sema validation (`tol/sema/agent.go`)

- Each `@requires(caller: X)` reference is validated against the set of
  declared `capability` names in the contract and module
- Unknown capability → **TOL2302** error:
  `"@requires references undeclared capability 'X' (declare it with 'capability X;')"`
- Multiple `@requires` on the same function are allowed (all must be satisfied)
- `@requires` composes independently with `@effects`, `@verifiable`,
  `@delegated`, `@pay`, `@quota`

### Interaction with other annotations

| Annotation | Interaction with @requires |
|-----------|---------------------------|
| `@effects` | Independent — @effects documents side effects; @requires documents access control |
| `@verifiable` | Independent — @verifiable is for pure/view ZK stubs |
| `@delegated` | Independent — @delegated marks delegation-compatible functions |
| `@pay` | Independent — @pay documents outbound payment |
| `@quota` | @quota preamble runs after @requires preamble |

---

## 3. Compiler Pipeline

| Stage | What happens | File |
|-------|-------------|------|
| **Lexer** | `///` tokenized as `TokenDocComment` | `tol/lexer/lexer.go` |
| **Parser** | `parseRequiresTag()` extracts cap names into `DocMeta.RequiresCap` | `tol/parser/parser.go:6083` |
| **AST** | `DocMeta.RequiresCap []string` stores capability names | `tol/ast/ast.go:359` |
| **Sema** | `checkAgentNativeDecls()` validates each cap is declared | `tol/sema/agent.go:68-77` |
| **Lower** | `Function.Doc` passed through unchanged | `tol/lower/lower.go:339` |
| **IR codegen** | `buildRequiresCapPreamble()` emits Lua preamble | `tol_ir_direct_lowering.go:2935-2958` |
| **ABI** | `RequiresCapability` field populated in `.toc` | `tol_artifact.go:719` |
| **Metadata** | `FunctionMeta.RequiresCapability []string` | `metadata/metadata.go:44` |

---

## 4. Lowering Model

`buildRequiresCapPreamble()` emits the following Lua code per capability:

```lua
if not (tos and type(tos.hascapability)=="function"
        and tos.hascapability(msg.sender, __tol_cap_CapName)) then
    error("CapabilityDenied:CapName")
end
```

The preamble is prepended to the function body before any other logic.
Order: @requires → @pay → @quota → function body.

### Runtime contract

- `tos.hascapability(caller, capName)` is a host function provided by the
  GTOS LVM (or stubbed in the test harness)
- Returns `true` if the caller holds the capability, `false` otherwise
- If `tos.hascapability` is not defined (e.g., raw Lua mode), the check
  fails closed — access is denied

---

## 5. ABI / Metadata Representation

In the `.toc` artifact:

```json
{
  "name": "withdraw",
  "selector": "0x...",
  "requires_capability": "OwnerCap",
  "visibility": "public",
  "mutability": "nonpayable"
}
```

In extracted `FunctionMeta`:

```json
{
  "name": "withdraw",
  "requires_capability": ["OwnerCap"],
  "effects": null,
  "verifiable": false,
  "delegated": false
}
```

Agents and tools inspect `requires_capability` in the ABI to determine
authority requirements before execution.

---

## 6. Failure Behavior

| Failure | Error |
|---------|-------|
| Unknown capability at compile time | TOL2302: `"@requires references undeclared capability 'X'"` |
| Capability denied at runtime | Lua error: `"CapabilityDenied:CapName"` |
| `tos.hascapability` not available | Fails closed — treated as capability denied |

---

## 7. Migration Path

Existing hand-rolled patterns:

```solidity
function withdraw(u256 amount) public {
    require(msg.sender == owner, "NOT_OWNER");
    // ...
}
```

Migrate to:

```solidity
capability OwnerCap;

/// @requires(caller: OwnerCap)
function withdraw(u256 amount) public {
    // ...
}
```

Benefits of migration:
- Capability names are compile-time validated
- ABI/metadata exposes authority requirements to agents and tools
- Consistent error format (`CapabilityDenied:X`)
- Future: optimizer can hoist checks, compose with delegation

---

## 8. Test Coverage

| Test | File | What it proves |
|------|------|----------------|
| `TestRequiresCapabilityCompileAndABI` | `stdlib_runtime_test.go` | Compile + ABI emission with `requires_capability` field |
| `TestRequiresCapabilityUnknownCapRejected` | `stdlib_runtime_test.go` | Sema rejects unknown capabilities (TOL2302) |
| `TestRequiresCapabilityRuntimePreamble` | `stdlib_runtime_test.go` | Runtime preamble: denied without hook, granted with hook, denied when hook returns false |

---

## Related Documents

- `docs/AGENT_NATIVE_STDLIB_2046.md`
- `docs/TOLANG_SHORTCOMINGS.md`
- `docs/STDLIB_CAPABILITY_ANALYSIS.md`
- `docs/TOLANG_LANGUAGE_DESIGN_PRINCIPLES.md`
