# Plan: AGENT_PROTOCOL_DRAFT2.tol Feature Gap Closure

## Context

`docs/AGENT_PROTOCOL_DRAFT2.tol` is the aspirational agent-native TOL syntax target.
Seven feature groups are missing from the current compiler. This plan closes all gaps
so the DRAFT2 contracts compile and run correctly.

Missing features confirmed by compilation test + audit:
1. Top-level `capability` declarations (outside contracts)
2. `oracle<T>` OOP member interface (`.fulfill()`, `.is_set`, `.value`)
3. `task<T>` OOP interface (methods + properties + `mapping(K=>task<T>)` type)
4. `agent` property access (`.stake`, `.is_active`, `.reputation`, `.rating_count`)
5. `manifest {}` format extensions (`;` separator, numeric values, array values)
6. `deploy Contract(...)` keyword (alias for `new`)
7. `escrow`/`release`/`slash` optional-purpose 2-arg form + `@pay(amount)` bare form

---

## Implementation Groups

### G1 — Syntax Quick-Wins (no semantic dependency)

#### G1a: `deploy` keyword

**Lexer** `tol/lexer/token.go`
- Add `TokenKwDeploy` constant after `TokenKwNew`
- Add `"deploy": TokenKwDeploy` to keyword map

**Parser** `tol/parser/parser.go`
- In `parsePrefixExpr`, add `case lexer.TokenKwDeploy:` that does identical work to `TokenKwNew`:
  consumes token, reads contract name ident, calls `parseCallArgs()`, returns
  `&ast.Expr{Kind: "new", Value: name, Args: args}` — reuse existing "new" AST node.
- No lowering changes needed; `case "new":` in `tolExprToLua` already handles it.

#### G1b: `manifest {}` format extensions

Current: only string literals + comma separators.
Target: `;` OR `,` separators; number literals; array-of-idents `[A, B]`.

**AST** `tol/ast/ast.go`
- Extend `ManifestField`:
  ```go
  type ManifestField struct {
      Key      string
      Value    string   // string literal or number literal (as text)
      IsArray  bool     // true when value is an array like [A, B]
      Array    []string // array elements (ident or string values)
  }
  ```

**Parser** `tol/parser/parser.go` — `parseManifestDecl()` (line 5597)
- Accept `;` or `,` as field separator (check both `TokenSemicolon` and `TokenComma`)
- For value: instead of only `TokenString`, also accept:
  - `TokenNumber` → store as string in `Value`
  - `TokenLBracket` → consume `[`, collect comma-separated idents/strings into `Array`, set `IsArray=true`

**Artifact** `tol_artifact.go` — manifest population (line 574)
- For `IsArray` fields: serialize `Array` as JSON array string → store as `extra[key]`
- For numeric `Value`: strip no quotes (already stored as text) → map to `extra[key]`

#### G1c: `escrow`/`release`/`slash` optional purpose

Currently: `escrow(agent, amount, purpose)` — 3 args mandatory.
Target: `escrow(agent, amount)` — purpose defaults to 0 (first declared purpose or literal 0).

**Sema** `tol/sema/agent.go` — `checkAgentBodyCalls()`
- Change TOL2311 check to allow 2 or 3 args for escrow/release
- Allow 3 or 4 args for slash (4th arg = purpose)

**Lowering** `tol_ir_direct_lowering.go` — `lowerAgentNativeCallExpr()`
- For `escrow`/`release`: if `len(e.Args) == 2`, emit `__tol_escrow(agent, amount, 0)`
- For `slash`: if `len(e.Args) == 3`, emit `__tol_slash(agent, amount, recipient, 0)`

#### G1d: `@pay(amount_expr)` bare form

Currently: `@pay(amount=X, recipient=Y)` with named keys required.
Target: `@pay(10_000_000)` → treat as amount only, recipient defaults to `fee_recipient` or left blank.

**Parser** `tol/parser/parser.go` — `parsePayTag()` (line 5532)
- Before splitting on `,`, check if rest (stripped of parens) contains no `=`:
  if so, treat the whole expression as the amount, set `meta.PayAmount = rest`, `meta.HasPay = true`
- Set `meta.PayIsBare = true` to distinguish from the named form.

**AST** `tol/ast/ast.go` — `DocMeta`
- Add `PayIsBare bool` field: true when `@pay(expr)` bare form is used (no named keys).

**Sema** `tol/sema/agent.go` — `checkAgentNativeDecls()`
- Skip TOL2309 (empty recipient) when `fn.Doc.PayIsBare` is true.

---

### G2 — Top-level `capability` Declarations

**AST** `tol/ast/ast.go`
- Add to `Module` struct (after `UsingDecls` field, line 101):
  ```go
  Capabilities []CapabilityDecl  // top-level capability declarations (shared across contracts)
  ```

**Parser** `tol/parser/parser.go` — `parseModule()` top-level loop (line 128)
- Add branch in the `else if` chain for contextual keyword `"capability"`:
  ```go
  } else if p.cur.Type == lexer.TokenIdent && p.cur.Literal == "capability" {
      cd := p.parseCapabilityDecl()
      if cd != nil {
          mod.Capabilities = append(mod.Capabilities, *cd)
      }
  ```
- Reuse existing `parseCapabilityDecl()` (line 5553) unchanged.

**Sema** `tol/sema/sema.go` — `checkOneContract()`
- Merge `m.Capabilities` into the effective capability set passed to `checkAgentNativeDecls`.
- Strategy: add `extraCaps []ast.CapabilityDecl` parameter to `checkAgentNativeDecls`; union
  with `c.Capabilities` for lookup.

**Sema** `tol/sema/agent.go` — `checkAgentNativeDecls()`
- Update signature: `checkAgentNativeDecls(filename string, c *ast.ContractDecl, moduleCaps []ast.CapabilityDecl, diags *diag.Diagnostics, knownStructNames map[string]bool)`
- Seed `capNames` from both `c.Capabilities` and `moduleCaps`.
- Duplicate check only within the combined set (no cross-contract collision needed).

**Lower** `tol/lower/lower.go` — `FromTypedContract()`
- Before building `out.Capabilities` from `c.Capabilities`, also prepend any module-level
  capabilities from `typed.AST.Capabilities` (avoid duplicates).

---

### G3 — `oracle<T>` OOP Member Interface

**Design:** When `e.Object` is an oracle storage slot and `e.Member` is a known oracle
property/method, intercept before the generic storage-access error.

#### G3a: Sema — whitelist oracle members

`tol/sema/sema.go` — `checkStorageExpr()` case `"member"` (line 3185)
- Before the generic "unsupported member access" error, check if the slot type starts
  with `"oracle<"` and the member is one of `{"is_set", "value"}` → allow (return early).
- In case `"call"` handler: check if callee is member `.fulfill` on oracle slot → allow.

#### G3b: Lowering — `lowerOracleSlotExpr()`

New function in `tol_ir_direct_lowering.go` (insert near `lowerStorageLengthMemberExpr`):

```
func lowerOracleSlotExpr(ctx, e) (luast.Expr, bool, error)
```

Handles two patterns:

**Member read** (`e.Kind == "member"`, object is oracle slot):
- `.is_set` → `__tol_oracle_is_set(__tol_s_<name>_set)`
- `.value`  → `__tol_oracle_value(__tol_s_<name>_val)`

**Method call** (`e.Kind == "call"`, callee is member `.fulfill` on oracle slot):
- `.fulfill(v)` → `__tol_oracle_fulfill(__tol_s_<name>_val, __tol_s_<name>_set, v)`

Detection: use `ctx.storagePathFromExpr(obj)` to get `(slotName, [], true)`, then
check `ctx.env.storageByName[slotName].typ` starts with `"oracle<"`.

The slot constant names (`__tol_s_<name>_val`, `__tol_s_<name>_set`) are already emitted
by `buildAgentNativePrelude()`.

Wire into `tolExprToLua()`:
- In `case "member":` — call `lowerOracleSlotExpr` before generic fallback
- In `case "call":` — call `lowerOracleSlotExpr` before `lowerAgentNativeCallExpr`

---

### G4 — `task<T>` OOP Interface

Most complex feature. Implemented in two phases.

#### G4a: Accept `mapping(K => task<T>)` as valid storage type

**Sema** `tol/sema/sema.go` — `isValidTOLType()` (line 4516) and
`classifyStorageSlotKind()` in `tol_ir_direct_lowering.go`:
- `isValidTOLType("mapping(u256=>task<bytes32>)")` already returns true because mapping is
  accepted and inner task type skips the inner check.
- Validate: if mapping value type is `task<T>`, the compound type is valid.
- The existing `classifyStorageSlotKind` classifies it as `storageKindMapping` — correct.

**Prelude** `buildAgentNativePrelude()` in `tol_ir_direct_lowering.go`:
- For each slot whose type is `mapping(K => task<T>)` (detect with `strings.Contains(typ, "task<")`),
  emit OOP sub-slot hashes in addition to the base hash:
  ```lua
  local __tol_s_tasks_base     = "0x..."  -- keccak256("tol.task.C.tasks")
  local __tol_s_tasks_poster   = "0x..."  -- keccak256("tol.task.C.tasks.poster")
  local __tol_s_tasks_worker   = "0x..."  -- keccak256("tol.task.C.tasks.worker")
  local __tol_s_tasks_reward   = "0x..."  -- keccak256("tol.task.C.tasks.reward")
  local __tol_s_tasks_deadline = "0x..."  -- keccak256("tol.task.C.tasks.deadline")
  local __tol_s_tasks_data     = "0x..."  -- keccak256("tol.task.C.tasks.data")
  ```
  Field values stored at `__tol_mkey(tid, field_base)`.

  Also emit `__tol_task_new` Lua helper:
  ```lua
  local __tol_task_new = function(task_base, poster_base, worker_base, reward_base, ddl_base, data_base, poster, reward, ddl)
    local tid = keccak256(tostring(poster) .. tostring(block and block.number or 0))
    local state_slot = __tol_mkey(tid, task_base)
    __tol_sstore(state_slot, 1)  -- Open
    __tol_sstore(__tol_mkey(tid, poster_base), poster)
    __tol_sstore(__tol_mkey(tid, reward_base), reward)
    __tol_sstore(__tol_mkey(tid, ddl_base), ddl)
    return tid
  end
  ```

#### G4b: `tasks[tid].method()` — direct mapping-element method calls (Phase 1)

New function `lowerTaskMappingCallExpr(ctx, e)` in `tol_ir_direct_lowering.go`:

Pattern: `e.Kind == "call"`, `e.Callee.Kind == "member"`, `e.Callee.Object` is an index
expression where base is a `mapping(K=>task<T>)` slot.

Detection:
1. `ctx.storagePathFromExpr(e.Callee.Object)` → `(slotName, [tidExpr], true)`
2. `ctx.env.storageByName[slotName].typ` contains `"task<"` (mapping-of-task)
3. Dispatch on `e.Callee.Member`:

| Method | State transition | Extra stores | Lua emitted |
|--------|-----------------|--------------|-------------|
| `.accept(worker)` | 1→2 | worker_base ← worker | `__tol_task_transition(base, tid, 1, 2, worker); __tol_sstore(__tol_mkey(tid, worker_base), worker)` |
| `.submit(data)` | 2→3 | data_base ← data | `__tol_task_transition(base, tid, 2, 3, nil); __tol_sstore(...)` |
| `.approve()` | 3→4 | — | `__tol_task_transition(base, tid, 3, 4, nil)` |
| `.reject()` | 3→5 | — | `__tol_task_transition(base, tid, 3, 5, nil)` |
| `.dispute()` | 3→6 or 5→6 | — | state-agnostic: `__tol_task_transition(base, tid, cur, 6, nil)` (runtime check) |
| `.cancel()` | {1,2,3,6}→7 | — | state-agnostic cancel |

Also handle property reads `tasks[tid].property` (member on indexed mapping-of-task):
- `.worker` → `__tol_sload(__tol_mkey(tid, __tol_s_tasks_worker))`
- `.poster` → `__tol_sload(__tol_mkey(tid, __tol_s_tasks_poster))`
- `.reward` → `__tol_sload(__tol_mkey(tid, __tol_s_tasks_reward))`
- `.is_expired` → `__tol_sload(__tol_mkey(tid, __tol_s_tasks_deadline)) < (block and block.timestamp or 0)`

Also handle `task<T>.new(poster, reward, ddl)` static constructor call:
- Pattern: `e.Kind == "call"`, `e.Callee.Kind == "member"`, `e.Callee.Object.Kind == "ident"`,
  value matches `"task<...>"` ident — NOTE: parser sees `task` as ident + `<T>` as type args
- Emit: `__tol_task_new(__tol_s_tasks_base, __tol_s_tasks_poster, ..., poster, reward, ddl)`

Wire into `tolExprToLua()`: call `lowerTaskMappingCallExpr` in `case "call":` and `case "member":`.

#### G4c: `task<T>` local variable handle (Phase 2)

`task<bytes32> t = tasks[task_id]` → `local t = {_base=__tol_s_tasks_base, _tid=task_id}`

Requirements:
- Sema: accept `task<T>` as a valid local variable type (already returns true in `isValidTOLType`)
- Lower `loweringCtx`: track local variables of type `task<T>` with their source slot name
  → add `taskLocals map[string]string` (localName → slotName) to `loweringCtx`
- Lower assignment `task<T> t = tasks[tid]`:
  - Emit `local t = {_base=__tol_s_<slotName>_base, _tid=<tidExpr>}`
  - Record `ctx.taskLocals["t"] = "tasks"`
- Lower `t.method()` and `t.property`:
  - In `lowerTaskMappingCallExpr`, also detect when object is an ident that is in `ctx.taskLocals`
  - Emit same patterns as G4b but with `t._base` and `t._tid` as the base/tid values

**Sema** `tol/sema/sema.go` — local type declarations:
- In `checkStatements`, for `let`-style local of type `task<T>`: suppress "invalid local type" error.
  (Already handled: `isValidTOLType("task<bytes32>")` returns true.)

---

### G5 — `agent` Property Access

**Design:** `agent(expr).property` or `localAgentVar.property` → calls to `tos.agentload(addr, field)`.

#### G5a: Lua prelude helper

Add to `buildAgentNativePrelude()`:
```lua
local __tol_agent_prop = tos and type(tos.agentload)=="function" and function(addr, field)
  local v = tos.agentload(addr, field)
  return v ~= nil and v or 0
end or function(addr, field) return 0 end

local __tol_MIN_AGENT_STAKE = tos and type(tos.min_agent_stake)=="function" and tos.min_agent_stake() or 0
```

#### G5b: `lowerAgentPropertyExpr()` in `tol_ir_direct_lowering.go`

New function. Detection: `e.Kind == "member"` where `e.Object` is a call to `agent()` or
an ident of declared `agent` type (track agent-typed locals like task locals).

Property → Lua mapping:
| Property | Lua emitted |
|----------|-------------|
| `.stake` | `__tol_agent_prop(addr, "stake")` |
| `.is_active` | `(__tol_agent_prop(addr,"registered")~=0 and __tol_agent_prop(addr,"suspended")==0 and __tol_agent_prop(addr,"stake")>=__tol_MIN_AGENT_STAKE)` |
| `.reputation` | `__tol_agent_prop(addr, "reputation")` |
| `.rating_count` | `__tol_agent_prop(addr, "rating_count")` |
| `.suspended` | `__tol_agent_prop(addr, "suspended") ~= 0` |

Wire into `tolExprToLua()` `case "member":` — call before `lowerEnvMemberExpr`.

#### G5c: Sema

`tol/sema/sema.go` — in `checkStorageExpr` case `"member"`:
- When object resolves to an `agent`-typed slot or is a call to `agent(...)`, whitelist the known
  property names (`.stake`, `.is_active`, `.reputation`, `.rating_count`, `.suspended`).

---

## Files to Modify

| File | Changes |
|------|---------|
| `tol/lexer/token.go` | Add `TokenKwDeploy` |
| `tol/ast/ast.go` | `Module.Capabilities []CapabilityDecl`; extend `ManifestField`; add `DocMeta.PayIsBare` |
| `tol/parser/parser.go` | `deploy` keyword; top-level `capability`; manifest extensions; `@pay` bare form |
| `tol/sema/sema.go` | Module-cap merge; oracle/task/agent member whitelists |
| `tol/sema/agent.go` | Relax escrow arity; module cap propagation; `@pay` bare form TOL2309 skip |
| `tol/lower/lower.go` | Propagate module capabilities |
| `tol_ir_direct_lowering.go` | `lowerOracleSlotExpr`, `lowerTaskMappingCallExpr`, `lowerAgentPropertyExpr`; extended prelude (task sub-slots, `__tol_task_new`, `__tol_agent_prop`); escrow 2-arg fallback |
| `tol_artifact.go` | Manifest array/numeric value serialization |

---

## Implementation Order

1. **G1 (Quick-wins)**: deploy, manifest extensions, escrow arity, @pay bare — independent, no deps
2. **G2 (Top-level caps)**: AST + parser + sema merge + lower propagation
3. **G3 (oracle OOP)**: sema whitelist + lowering — depends on existing oracle prelude
4. **G4a+G4b (task mapping OOP)**: sema type acceptance + prelude sub-slots + mapping-method lowering
5. **G5 (agent props)**: prelude helper + lowering + sema whitelist
6. **G4c (task local handles)**: type tracking in loweringCtx — defer to last, depends on G4b

---

## Verification

```bash
cd ~/tolang

# Full build
go build ./...

# Existing tests must stay green
go test ./...

# Compile the DRAFT2 file (target: zero errors)
go run ./cmd/tolang compile --emit toc -o /tmp/draft2.toc docs/AGENT_PROTOCOL_DRAFT2.tol

# Spot-check ABI output
strings /tmp/draft2.toc | grep -E "requires_capability|manifest|verifiable|delegated"

# Integration contracts per feature group
# G1: deploy keyword
cat > /tmp/g1.tol <<'EOF'
pragma tolang 0.2;
contract Factory {
    function make() external returns (address) {
        return deploy Child();
    }
}
contract Child {}
EOF
go run ./cmd/tolang compile --emit toc -o /dev/null /tmp/g1.tol

# G3: oracle OOP
cat > /tmp/g3.tol <<'EOF'
pragma tolang 0.2;
contract OTest {
    capability Resolver;
    oracle<uint256> price;
    /// @requires(caller: Resolver)
    function resolve(uint256 v) external { price.fulfill(v); }
    function isSet() external view returns (bool) { return price.is_set; }
    function get() external view returns (uint256) { return price.value; }
}
EOF
go run ./cmd/tolang compile --emit toc -o /dev/null /tmp/g3.tol

# G4: task OOP
cat > /tmp/g4.tol <<'EOF'
pragma tolang 0.2;
contract TTest {
    mapping(uint256 => task<bytes32>) tasks;
    uint256 next_id;
    function post(bytes32 spec) external payable returns (uint256 tid) {
        tid = next_id; next_id = next_id + 1;
        tasks[tid] = task<bytes32>.new(msg.sender, msg.value, 0);
    }
    function accept(uint256 tid) external { tasks[tid].accept(msg.sender); }
    function approve(uint256 tid) external { tasks[tid].approve(); }
}
EOF
go run ./cmd/tolang compile --emit toc -o /dev/null /tmp/g4.tol

# G5: agent properties
cat > /tmp/g5.tol <<'EOF'
pragma tolang 0.2;
contract ATest {
    function isActive() external view returns (bool) {
        return agent(msg.sender).is_active;
    }
    function stake() external view returns (uint256) {
        return agent(msg.sender).stake;
    }
}
EOF
go run ./cmd/tolang compile --emit toc -o /dev/null /tmp/g5.tol
```

---

## Key Implementation Notes

### G4c: `loweringCtx` extensions for task locals

```go
type loweringCtx struct {
    // ... existing fields ...
    taskLocals  map[string]string // local var name → storage slot name (for task<T> locals)
    agentLocals map[string]bool   // local var name → true (for agent-typed locals)
}
```

When lowering `let` statement with Type `task<T>` and Expr `tasks[tid]`:
- Detect: `s.Type` starts with `"task<"` and the initializer is a storage index path
- Emit: `local t = {_base=__tol_s_tasks_base, _tid=<tidExpr>}`
- Record: `ctx.taskLocals["t"] = "tasks"`

When lowering member access `t.property` or `t.method()`:
- Check if `e.Object` is an ident in `ctx.taskLocals`
- If yes, treat `t._base` and `t._tid` as the base and tid, forwarding to the same lowering
  as `tasks[t._tid].property`

### G2: Module capability scope

Module-level capabilities declared with `capability Foo;` at the file top-level are visible
to all contracts in the same file. The sema check unions them with contract-level capabilities
for `@requires` resolution. The lowering prepends them to `out.Capabilities` so the runtime
prelude emits the `__tol_cap_Foo` locals they need.

### G3: Oracle slot detection in lowering

Oracle slots are detected in `lowerOracleSlotExpr` by:
1. Getting the storage path from the object expression
2. Checking `ctx.env.storageByName[slotName].typ` starts with `"oracle<"`
3. The slot constants `__tol_s_<name>_val` and `__tol_s_<name>_set` are pre-emitted

### G1b: Manifest array serialization in tol_artifact.go

For `IsArray` manifest fields (e.g. `capabilities: [Registrar]`):
- Serialize `Array` as a JSON array string: `"[\"Registrar\"]"`
- Store in `extra[key]`

For numeric manifest fields (e.g. `sla_uptime: 9900`):
- Store the raw text (no quote stripping needed since it was a number token)
- Store in `extra[key]`
