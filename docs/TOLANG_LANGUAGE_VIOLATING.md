# TOLANG Design Principles Compliance Audit

## Status Overview

| # | Issue | Severity | Principle | Decision | Status |
|---|-------|----------|-----------|----------|--------|
| 1 | `msg.value` silently rewritten in `payable(uno)` | CRITICAL | 3, 4 | **FIX** | ✅ DONE |
| 2 | `==`/`!=`/`<=`/`>=` on uno hides 150k gas crypto ops | CRITICAL | 4, 7, 14 | **FIX** | ✅ DONE |
| 3 | Three error modes (require/assert/revert) with no semantic distinction | CRITICAL | 9 | **FIX** | ✅ DONE |
| 4 | ABI spec is Draft 0.1, critical fields undefined | CRITICAL | 10 | **FIX** | ✅ DONE |
| 5 | oracle\<T\>/vote\<T\>/task\<T\> baked into compiler as DSL | MAJOR | 1, 8 | **FIX** | ✅ DONE |
| 6 | Inheritance system (434 lines) with zero agent use case | MAJOR | 1, 8 | **FIX** | ✅ DONE (Phase 1) |
| 7 | Modifier guards invisible to @effects | MAJOR | 6, 13 | **FIX** | ✅ DONE |
| 8 | Two variable declaration syntaxes coexist | MODERATE | 5 | **FIX** | ✅ DONE |
| 9 | 9 annotation types with inconsistent syntax | MODERATE | 5, 8 | **DEFER** | — |
| 10 | Type aliases normalized at lex time | MINOR | 4 | **KEEP** | — |
| 11 | block.timestamp field naming inconsistency | MINOR | 2 | **FIX** | ✅ DONE |
| 12 | ++/--, do-while, sub-denominations (tomi/gtomi/tos) | MINOR | 1 | **KEEP** | ✅ wei/gwei/ether replaced |
| 13 | Solidity reserved keywords (23 unused) | MINOR | 8 | **KEEP** | — |
| 14 | No formatter, no LSP, no source maps | MODERATE | 11 | **DEFER** | — |
| 15 | using-for syntax enables implicit operator overloading | MODERATE | 4, 6 | **FIX** | ✅ DONE |
| 16 | `set` keyword is optional for storage writes | MODERATE | 4, 5 | **FIX** | ✅ DONE |

---

## Issue 1: `msg.value` Silently Rewritten in `payable(uno)`

**Principles violated:** 3 (Safety), 4 (Explicitness)

**Problem:**

In a `payable(uno)` function, `msg.value` is silently rewritten by the compiler to `msg.uno_value`. The developer writes identical source code for plaintext and encrypted payment, but the semantics are completely different:

```tol
function deposit() external payable      { total = total + msg.value; }  // msg.value = plaintext TOS
function deposit() external payable(uno) { bal = bal.add(msg.value); }   // msg.value = encrypted ciphertext
```

A reader cannot distinguish between plaintext and encrypted value access without checking the function header. An auditor must mentally track whether the enclosing function is `payable` or `payable(uno)` to understand what `msg.value` means.

**Decision: FIX**

**Status: ✅ DONE** — `msg.value` in `payable(uno)` now emits TOL2100 error. The silent `msg.value → msg.uno_value` rewrite in lowering removed. Example contracts updated to use `msg.uno_value` explicitly.

**Plan:**

1. **Deprecate** the implicit `msg.value` rewrite in `payable(uno)` functions.
2. **Require** developers to write `msg.uno_value` explicitly in `payable(uno)` functions.
3. **Emit compiler error** (not warning) if `msg.value` is used inside `payable(uno)` — with a clear diagnostic: `"use msg.uno_value in payable(uno) functions; msg.value refers to plaintext TOS amount"`.
4. **Keep** `msg.value` available in `payable(uno)` to mean the plaintext gas/fee portion if needed, maintaining semantic consistency with non-uno payable.

**Files changed:**
- `tol/sema/sema.go` — added `checkMsgValueInPayableUno()` with TOL2100 diagnostic
- `tol_ir_direct_lowering.go` — removed the silent `msg.value → msg.uno_value` rewrite
- `examples/confidential_vault/ConfidentialVault.tol` — updated to use `msg.uno_value`

---

## Issue 2: `==`/`!=`/`<=`/`>=` on uno Hides 150k Gas Crypto Operations

**Principles violated:** 4 (Explicitness), 7 (Resource-Aware), 14 (Predictable Performance)

**Problem:**

The operators `==`, `!=`, `<=`, `>=` on uno type are silently desugared to `tos.ciphertext.{eq,ne,lte,gte}(a, b)`. These are proof-bundle verified operations costing 150,000–160,000 gas each. The identical operator on a plaintext type costs negligible gas.

```tol
bool ok1 = x == y;  // plaintext: ~3 gas
bool ok2 = a == b;  // uno: 150,000 gas + proof bundle required
```

A developer cannot tell from the source code that `a == b` is a 150k gas cryptographic verification. This violates resource-awareness and cost predictability.

**Decision: FIX**

**Status: ✅ DONE** — All comparison operators (`==`, `!=`, `<=`, `>=`) on uno type now rejected at sema level. Lowering desugaring removed. Method calls (`a.eq(b)`, `a.lte(b)` etc.) remain the only way to perform encrypted comparisons.

**Plan:**

1. **Remove** operator desugaring for `==`, `!=`, `<=`, `>=` on uno type.
2. **Reject** these operators on uno at the sema level with a clear error: `"operator '==' not supported on uno type; use a.eq(b) to make the 150k gas proof verification explicit"`.
3. **Keep** only explicit method calls: `a.eq(b)`, `a.ne(b)`, `a.lte(b)`, `a.gte(b)`.
4. **Revert** the `!=` desugaring to `not(tos.ciphertext.eq(...))` that was added in the current session.

**Files changed:**
- `tol/sema/sema.go` — `==`, `!=`, `<=`, `>=` added to uno operator rejection list
- `tol_ir_direct_lowering.go` — removed desugaring block for uno operators
- `tol_ir_direct_lowering_uno_test.go` — operator tests now expect failure; method tests still pass
- `tol/sema/sema_test.go` — `TestUnoLteGteOperator` and `TestUnoEqOperator` expect rejection
- `docs/grammar/TolangParser.g4` — updated uno comment section

**Note:** The `lte`, `gte`, `ne` methods and their LVM runtime implementations in gtos remain valid and unchanged. Only the **operator sugar** is removed; the **method call path** stays.

---

## Issue 3: Three Error Modes with No Semantic Distinction

**Principles violated:** 9 (Uniform Error Model)

**Problem:**

The language provides `require()`, `assert()`, and `revert()` but does not document or enforce any semantic difference between them. All three compile to Lua `error()`. The `try-catch` compiles to Lua `pcall()` which catches all errors indiscriminately, including Lua runtime errors (stack overflow, OOM).

An agent cannot determine from the ABI whether a function failure is recoverable (invalid input) or indicates a bug (invariant violation).

**Decision: FIX**

**Status: ✅ DONE**

**What was done:**

1. **Differentiated codegen** in `tol_ir_direct_lowering.go`:
   - `require(cond, msg)` now lowers to `if not (cond) then error({selector="0x08c379a0", msg=msg}) end` — Error(string) ABI selector for precondition failures.
   - `assert(cond)` now lowers to `if not (cond) then error({selector="0x4e487b71", code=1}) end` — Panic(uint256) ABI selector for invariant violations.
   - `revert "msg"` now lowers to `error({selector="0x08c379a0", msg="msg"})` — Error(string) for plain reverts.
   - `revert CustomError(args)` now lowers to `error({selector="custom", data=<abi-encoded>})` — typed custom error.
2. **Documented error model** in `docs/ABI_SPEC.md` section 7.10 — covers all three error types, their selectors, runtime representation, try-catch interaction, and agent implications.
3. **try-catch limitation documented** — `pcall` catches all errors indiscriminately; filtering to only catch typed contract errors (selector-tagged tables) is deferred.
4. **Updated test infrastructure** — `extractApiRevertMsg` and `extractErrorMessage` helpers handle table-valued errors in test assertions.
5. **Added tests** — `TestRequireErrorSelector`, `TestAssertErrorSelector`, `TestRevertErrorSelector` verify correct selector and payload for each error type.

**Files changed:**
- `tol_ir_direct_lowering.go` — differentiated require/assert/revert lowering
- `docs/ABI_SPEC.md` — added Error Model section (7.10)
- `tol_api_test.go` — added 3 selector verification tests, updated existing error checks
- `trc20_test.go` — updated error message extraction for table errors
- `tol/test/runner.go` — updated `assert_revert` to handle table error values
- `testutils_test.go` — added `extractApiRevertMsg` helper

---

## Issue 4: ABI Spec Is Draft 0.1 with Critical Fields Undefined

**Principles violated:** 10 (Stable ABI)

**Problem:**

The ABI specification (`docs/ABI_SPEC.md`) is marked `Status: Draft, Version: 0.1`. Critical agent-facing fields are undefined:
- `delegation_scope` — "optional structured scope descriptor" with no schema
- `proof_schema` — "optional schema URI or structured object" with no decision
- `task_descriptor.state_machine` — hardcoded "tol-task-v1" with no version policy

Agents, SDKs, and indexers cannot rely on an unstable ABI.

**Decision: FIX**

**Status: ✅ DONE** — ABI spec promoted to Stable 1.0. All fields defined with concrete schemas. Error model (require/assert/revert selectors) documented. Compatibility guarantee section added.

**What was done:**

1. `docs/ABI_SPEC.md` — promoted to Stable 1.0; `delegation_scope` defined as `{ action, contract, expiry_ms, nonce }`; `proof_schema` defined as enum `"none" | "sigma-range-v1" | "transcript-binding-v1"`; `task_descriptor.state_machine` replaced with generic versioned schema; ABI Compatibility Guarantee section added; Error Model section added.
2. `tol_artifact.go` — TODO(ABI-1.0) review comments added for `ABIVersion`, `Errors`, `DelegationScope`, `ProofSchema`, `CallerKind` fields.

---

## Issue 5: oracle\<T\>/vote\<T\>/task\<T\> Baked into Compiler as DSL

**Principles violated:** 1 (Purpose Before Features), 8 (Restrict Complexity)

**Problem:**

Three parameterized types — `oracle<T>`, `vote<T>`, `task<T>` — are implemented as compiler intrinsics with ~145 lines of special-case codegen and ~443 lines of sema validation (agent.go). The task type has a hardcoded 8-state state machine. Agents cannot define custom state machines or oracle patterns.

These are domain-specific abstractions for a specific market pattern, not fundamental language primitives.

**Decision: FIX (phased)**

**Status: ✅ DONE** — All three phases completed in commit `866c2c2` (2026-03-18).

Skipped directly to Phase 3: removed all compiler intrinsics entirely and replaced with stdlib pattern contracts. ~1100 lines of special-case code deleted across parser, sema, and lowering. Zero new language features needed — the patterns are expressed using existing TOL primitives (struct, constant, require(), mapping, event).

**What was done:**

1. **Created `stdlib/` pattern contracts** — `stdlib/Oracle.tol` (write-once pattern), `stdlib/Vote.tol` (tally-and-threshold), `stdlib/Task.tol` (state machine with constants + require guards). All compile successfully.
2. **Removed lowering** (~530 lines from `tol_ir_direct_lowering.go`) — deleted 7 functions (`lowerOracleSlotExpr`, `lowerVoteSlotExpr`, `taskSlotForExpr`, `buildTaskFieldExpr`, `lowerTaskMappingStoreStmt`, `lowerTaskMappingMemberExpr`, `lowerTaskMappingCallExpr`), prelude generation (`__tol_oracle_*`, `__tol_vote_*`, `__tol_task_*`), call-site dispatches, `ctx.taskLocals` tracking.
3. **Removed sema validation** (~120 lines from `agent.go` + ~50 lines from `sema.go`) — deleted `validTaskTransitions`, type parameter checks (TOL2303/2304/2305), `__tol_task_transition` check (TOL2315), `extractAgentInnerType()`, `isNumericTOLType()`, `literalUint64()`, oracle/vote/task method and property validation.
4. **Narrowed parser** (~50 lines from `parser.go`) — `case "oracle","vote","task","agent"` → `case "agent"`; removed angle-bracket detection, local variable detection, and expression detection for oracle/vote/task.
5. **Updated docs** — `AGENT_PROTOCOL_DRAFT2.tol` (rewrote TaskEscrow + PredictionMarket with plain storage), `FEATURE_MATURITY_MATRIX.md`, `AGENT-NATIVE.md`, `TolangParser.g4`, `OracleResolver.tol`.
6. **Reserved diagnostic codes** — TOL2303, TOL2304, TOL2305, TOL2315 marked `// RESERVED`.

**Verification:** Build clean, all 11 test packages pass, stdlib compiles, `oracle<u256>` syntax correctly rejected as parse error, agent type unaffected.

---

## Issue 6: Inheritance System with Zero Agent Use Case

**Principles violated:** 1 (Purpose Before Features), 8 (Restrict Complexity)

**Problem:**

The inheritance system (`inherit.go`, 434 lines) implements full C3 linearization, virtual/override dispatch, super calls, and abstract contract validation. No agent protocol in the codebase uses polymorphic dispatch. Agents call fixed function selectors, not virtual methods.

**Decision: FIX (phased)**

**Status: ✅ DONE (Phase 1)** — Deprecation warnings implemented (2026-03-19).

**Plan:**

Phase 1 (short-term): **Deprecate** `virtual`, `override`, and `super` keywords with compiler warnings. Document that inheritance is supported only for **interface implementation** (contract implements interface), not for polymorphic hierarchies.

Phase 2 (medium-term): **Restrict** inheritance to single-level interface implementation only: `contract Foo is IBar { }` where IBar must be an interface. Remove multi-level inheritance, C3 linearization, virtual, override, and super.

Phase 3 (long-term): Replace inheritance with **composition + capability references**. A contract that needs "base" behavior imports a library.

**What was done (Phase 1):**

1. **Added deprecation warning codes** — TOL2317 (`virtual`), TOL2318 (`override`), TOL2319 (`super`) in `tol/diag/diag.go`.
2. **Emit warnings in sema** — `tol/sema/sema.go` emits `SeverityWarning` diagnostics when `virtual` or `override` modifiers are used on function declarations, modifier declarations, or storage slots.
3. **Emit warnings for super calls** — `tol/sema/inherit.go` emits TOL2319 warning whenever `super.method()` is detected.
4. **Interface implementation unaffected** — `contract Foo is IBar` where IBar is an interface does NOT emit warnings (this is the intended usage pattern).
5. **Warnings propagated to callers** — `Check()` and `CheckWithResolver()` now return warning-only diagnostics instead of discarding them. Callers already use `HasErrors()` for error detection, so warnings are non-breaking.
6. **Tests added** — `TestDeprecationWarningsVirtualOverrideSuper` (all three warnings emitted, no errors) and `TestDeprecationWarningInterfaceImplementationNoWarning` (no warnings for clean interface implementation).

**Files changed:**
- `tol/diag/diag.go` — added TOL2317, TOL2318, TOL2319 warning codes
- `tol/sema/sema.go` — deprecation warnings for virtual/override on functions, modifiers, storage slots; fixed `Check()`/`CheckWithResolver()` to return warnings
- `tol/sema/inherit.go` — deprecation warning for super calls
- `tol/sema/sema_test.go` — two new tests, one existing test updated

---

## Issue 7: Modifier Guards Invisible to @effects

**Principles violated:** 6 (Auditability), 13 (Security Boundaries)

**Problem:**

A modifier like `onlyOwner` contains a `require(msg.sender == owner)` guard, but this guard is invisible to the `@effects` annotation system. An auditor reading `@effects writes: storage.balance` does not see the permission check. The effects system reports state mutations but not authority gates.

**Decision: FIX**

**Status: ✅ DONE**

**What was done:**

1. **Extended `@effects` syntax** with a `guards:` clause: `@effects guards: [onlyOwner], writes: [storage.balance]`. The parser supports bracket-enclosed lists and multi-clause single-line format.
2. **Added `Guards []string` field** to `EffectDecl` in `tol/ast/ast.go`.
3. **Extended `parseEffectsTag`** in `tol/parser/parser.go` to parse the `guards:` key, strip brackets from all clause values, and handle multiple clauses on a single `@effects` line via recursive tail parsing.
4. **Added `checkEffectsGuards` validation** in `tol/sema/effects.go`: emits TOL2206 WARNING (not error) when a function uses a user-defined modifier but the `@effects` annotation does not declare it in a `guards:` clause. The warning only fires when `@effects` is present — functions without `@effects` are unaffected.
5. **Added 5 tests**: 2 parser tests (`TestParseDocMetaGuards`, `TestParseDocMetaGuardsMultiLine`) and 3 sema tests (`TestEffectsGuardsMissingWarning`, `TestEffectsGuardsDeclaredOK`, `TestEffectsGuardsNoEffectsAnnotation`).

**Files changed:**
- `tol/ast/ast.go` — added `Guards` field to `EffectDecl`
- `tol/diag/diag.go` — added `CodeEffectGuardMissing` (TOL2206)
- `tol/parser/parser.go` — extended `parseEffectsTag` with `guards:` key, bracket stripping, multi-clause tail parsing
- `tol/sema/effects.go` — added `checkEffectsGuards` function
- `tol/sema/sema.go` — built `userModNames` set and wired `checkEffectsGuards` call
- `tol/parser/parser_test.go` — 2 new parser tests
- `tol/sema/sema_test.go` — 3 new sema tests

**Long-term**: consider replacing modifiers entirely with `@requires` capability checks, which ARE visible in the effects system and ABI.

---

## Issue 8: Two Variable Declaration Syntaxes Coexist

**Principles violated:** 5 (Simple Semantics)

**Problem:**

Two syntaxes declare local variables:
```tol
let x: u256 = 1;   // TOL-native style
u256 x = 1;        // Solidity-compatible type-first style
```

Both produce identical AST. Having two syntaxes for the same semantics confuses new developers and increases parser complexity.

**Decision: FIX**

**Status: ✅ DONE** — Completed in one step (skipped phased approach).

Removed `let` entirely. `let` remains a reserved keyword (cannot be used as identifier). All variable declarations now use type-first syntax, aligned with Solidity.

**What was done:**

1. **Parser**: `let x: T = expr` → emits error `"'let' is removed; use type-first syntax: T x = expr;"`. Error recovery still parses the statement for better diagnostics.
2. **Tuple syntax**: Added Solidity-style `(T1 a, T2 b) = expr;` — replaces `let (a, b): (T1, T2) = expr;`. Both produce the same `"let-tuple"` AST node.
3. **Array type-first**: Extended type-first detection to recognize `u256[] x` and `u256[3] x` (peek-second heuristic: `]` or number → array type; ident → index expression).
4. **Test block variables**: Type-first declarations now work in test blocks (`test Suite { u256 x = 42; ... }`), with contextual keyword exclusion for `setup`/`teardown`/`mock`.
5. **All .tol files migrated**: 48 `let` statements across 12 example files converted.
6. **All Go test files migrated**: ~200+ embedded TOL strings updated across 6 test files.
7. **Grammar doc**: `letStatement` / `letTupleStatement` marked as REMOVED; `typeFirstTupleDecl` added.

**Files changed:**
- `tol/parser/parser.go` — `parseLetStatement` (error + recovery), `parseTypeFirstTupleDecl` (new), type-first array detection, test block type-first support
- `docs/grammar/TolangParser.g4` — updated rules
- `examples/` — 12 .tol files migrated
- `tol_api_test.go`, `trc20_test.go`, `package_system_verify_test.go`, `tol_compile_options_test.go`, `tol/parser/parser_test.go`, `tol/test/runner_test.go`, `tol/test/coverage_line_test.go` — embedded TOL strings updated

---

## Issue 9: 9 Annotation Types with Inconsistent Syntax

**Principles violated:** 5 (Simple Semantics), 8 (Restrict Complexity)

**Problem:**

Nine independent annotations exist with different syntax forms:
- `@effects reads: [...] writes: [...]` (multi-key, colon-separated)
- `@gas upper: 50000` (single key-value)
- `@requires(caller: Arbitrator)` (parenthesized named key)
- `@delegated` (bare, no payload)
- `@pay(expr)` (positional) or `@pay(amount=expr, recipient=expr)` (named keys)
- `@verifiable` (bare)
- `@quota(calls: N, price: M)` (parenthesized)
- `@bounds(...)` (parenthesized expressions)
- `@total_cost(max: N)` (parenthesized)

**Decision: DEFER**

**Rationale:** Unifying annotation syntax is desirable but low-urgency. The current annotations are compile-time metadata that do not affect runtime behavior. Standardizing syntax is a cosmetic improvement that can wait until the ABI spec stabilizes (Issue 4). Once the ABI schema is fixed, annotations can be unified to match.

**Future direction:** Consider a single `@metadata { ... }` block with structured JSON-like syntax, replacing all individual annotations.

---

## Issue 10: Type Aliases Normalized at Lex Time

**Principles violated:** 4 (Explicitness) — minor

**Problem:**

`normalizeTypeAlias()` in the lexer silently maps `uint256` to `u256`, `int128` to `i128`, etc. The AST never sees the original Solidity spelling.

**Decision: KEEP**

**Rationale:**

1. **Zero runtime impact** — normalization happens at lex time, before parsing.
2. **Migration necessity** — Solidity developers expect to write `uint256`; rejecting it would block adoption.
3. **Single point of normalization** — only one function (`normalizeTypeAlias`) handles all mappings; no leakage into parser/sema/codegen.
4. **Compiler diagnostics** could optionally emit an info-level hint: `"uint256 is accepted as alias for u256"`, giving visibility without breaking compatibility.

**Optional improvement:** Add a `--strict` compiler flag that rejects Solidity aliases and requires canonical TOL types only.

---

## Issue 11: block.timestamp Field Naming Inconsistency

**Principles violated:** 2 (Determinism)

**Problem:**

In `tol_ir_direct_lowering.go`, two different field names are used for the same value:
```go
local now = block and block.timestamp_ms or 0      // one location
local now_ms = block ~= nil and block.timestamp or 0  // another location
```

`timestamp_ms` vs `timestamp` — the same field with two names, creating confusion about whether the value is in milliseconds or seconds.

**Decision: FIX**

**Plan:**

Standardize on `block.timestamp_ms` everywhere. The TOS chain uses millisecond-resolution timestamps (genesis = `date +%s%3N`). The field name must reflect the unit.

**Files to change:**
- `tol_ir_direct_lowering.go` — find and replace all `block.timestamp` references to `block.timestamp_ms`

**Status: DONE** — Standardized on `block.timestamp_ms` in the lowered Lua runtime code (`tol_ir_direct_lowering.go`), default state globals (`state.go`), test runner defaults (`tol/test/runner.go`), and API tests (`tol_api_test.go`).

---

## Issue 12: ++/--, do-while, Sub-Denominations

**Principles violated:** 1 (Purpose Before Features) — minor

**Problem:**

- `++`/`--` operators are syntactic sugar for `x = x + 1`.
- `do-while` is equivalent to `{ body } while (cond)`.
- Sub-denomination suffixes (`tomi`, `gtomi`, `tos`, `seconds`, `days`) are native TOS denominations.

**Decision: KEEP**

**Rationale:**

1. **Low complexity cost** — each is a few lines in the parser, trivial lowering.
2. **Developer expectation** — removing `++` from a C-family language would surprise every developer.
3. **Sub-denominations** — `days` and `hours` are useful for deadline expressions (`block.timestamp_ms + 7 days`). The unit suffixes are computed at compile time with zero runtime overhead.
4. **do-while** — rare but occasionally clearer than while for "execute at least once" patterns.

Sub-denominations now use native TOS naming: `tomi` (base unit, 1), `gtomi` (1e9), `tos` (1e18).

**✅ Improvement applied:** `wei`, `gwei`, `ether` have been removed from the lexer. Using them now produces a parse error. Replaced with TOS-native denominations: `tomi`, `gtomi`, `tos`. Time units (`seconds`, `minutes`, `hours`, `days`, `weeks`) retained. `years` retained but marked deprecated (consistent with Solidity EIP-4820).

---

## Issue 13: Solidity Reserved Keywords (23 Unused)

**Principles violated:** 8 (Restrict Complexity) — minor

**Problem:**

23 Solidity-reserved keywords (`after`, `alias`, `apply`, `auto`, `byte`, `case`, `copyof`, `default`, `define`, `final`, `implements`, `in`, `inline`, `macro`, `match`, `mutable`, `null`, `of`, `partial`, `promise`, `reference`, `relocatable`, `sealed`, `sizeof`, `static`, `supports`, `switch`, `typedef`, `typeof`, `var`) are reserved in TOL but serve no purpose.

**Decision: KEEP**

**Rationale:**

Reserving keywords prevents developers from using them as identifiers, which would break forward compatibility if TOL ever adopts these keywords. This is standard practice (Solidity, Rust, Go all reserve unused keywords). The cost is zero at runtime and minimal in the lexer.

---

## Issue 14: No Formatter, No LSP, No Source Maps

**Principles violated:** 11 (Tooling)

**Problem:**

The compiler produces bytecode and ABI but lacks:
- `tolfmt` code formatter
- Language Server Protocol implementation
- Source maps for debugging
- Coverage tracking

**Decision: DEFER**

**Rationale:** These are important but are engineering tasks, not language design issues. They do not affect the correctness or safety of compiled contracts. They should be built after the language spec stabilizes (especially after Issues 1–4 are resolved).

**Priority order:** formatter > source maps > LSP > coverage.

---

## Issue 15: using-for Enables Implicit Operator Overloading

**Principles violated:** 4 (Explicitness), 6 (Auditability)

**Problem:**

`using SafeMath for u256` allows `x + y` to silently dispatch to `SafeMath.add(x, y)`. An auditor reading `x + y` cannot know whether this is native addition or a library call without checking all `using` declarations in scope.

**Decision: FIX**

**Status: DONE**

**What was done:**

1. Added `checkUsingDecls` validation in `tol/sema/sema.go` that detects `as OPERATOR` aliases in braced using-for declarations (e.g., `using { add as + } for u256;`).
2. Emits **TOL2101** error: `"operator 'OP' cannot dispatch through using-for binding; use explicit method call x.fn(y) instead"` for any of `+`, `-`, `*`, `/`, `%`, `==`, `!=`, `<`, `>`, `<=`, `>=`.
3. Method-style calls (`x.add(y)`) through `using SafeMath for u256` remain allowed.
4. Added diagnostic code `CodeSemaUsingForOperator = "TOL2101"` in `tol/diag/diag.go`.
5. Added tests: `TestCheckUsingForOperatorDispatchRejected`, `TestCheckUsingForMethodCallAllowed`, `TestCheckUsingForBracedWithoutOperatorAllowed`.

---

## Issue 16: `set` Keyword Is Optional for Storage Writes

**Principles violated:** 4 (Explicitness), 5 (Simple Semantics)

**Problem:**

The `set` keyword was designed to make storage writes explicit, but it is optional:
```tol
set balances[addr] = 100;  // explicit — intended design
balances[addr] = 100;      // also works — set is optional
```

This defeats the purpose. If `set` is optional, it provides no safety guarantee. Auditors cannot rely on "all storage writes are marked with set" because the language does not enforce it.

**Decision: FIX**

**Status: DONE**

**What was done:**

1. Added storage-write enforcement in `checkStorageStatements` (`tol/sema/sema.go`): when an `"expr"` statement contains an `"assign"` expression whose left-hand side resolves to a storage slot (via `storagePathFromExpr`), emits **TOL2102** error: `"storage write requires 'set' keyword: set VARIABLE = VALUE"`.
2. Local variable reassignment (`x = x + 1` where `x` is a local) does NOT require `set` -- the check only triggers when the assignment target is a known storage slot or indexed path into a storage slot.
3. Added diagnostic code `CodeSemaStorageWriteNeedsSet = "TOL2102"` in `tol/diag/diag.go`.
4. Added tests: `TestCheckStorageWriteWithoutSetRejected`, `TestCheckStorageWriteWithSetAllowed`, `TestCheckLocalAssignWithoutSetAllowed`, `TestCheckStorageMappingWriteWithoutSetRejected`.
5. Updated all example `.tol` files (`ConfidentialToken`, `PrivateAuction`, `PrivateOTC`, `PrivateVoting`, `PrivatePayroll`, `PrivatePrediction`) to use `set` for storage writes.
6. Updated `docs/AGENT_PROTOCOL_DRAFT2.tol` to use `set` for all storage writes.

Storage writes are now grep-able: `grep "set "` finds all state mutations.
