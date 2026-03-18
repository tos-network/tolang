# TOLANG Design Principles Compliance Audit

## Status Overview

| # | Issue | Severity | Principle | Decision | Status |
|---|-------|----------|-----------|----------|--------|
| 1 | `msg.value` silently rewritten in `payable(uno)` | CRITICAL | 3, 4 | **FIX** | TODO |
| 2 | `==`/`!=`/`<=`/`>=` on uno hides 150k gas crypto ops | CRITICAL | 4, 7, 14 | **FIX** | TODO |
| 3 | Three error modes (require/assert/revert) with no semantic distinction | CRITICAL | 9 | **FIX** | TODO |
| 4 | ABI spec is Draft 0.1, critical fields undefined | CRITICAL | 10 | **FIX** | TODO |
| 5 | oracle\<T\>/vote\<T\>/task\<T\> baked into compiler as DSL | MAJOR | 1, 8 | **FIX** | TODO |
| 6 | Inheritance system (434 lines) with zero agent use case | MAJOR | 1, 8 | **FIX** | TODO |
| 7 | Modifier guards invisible to @effects | MAJOR | 6, 13 | **FIX** | TODO |
| 8 | Two variable declaration syntaxes coexist | MODERATE | 5 | **FIX** | TODO |
| 9 | 9 annotation types with inconsistent syntax | MODERATE | 5, 8 | **DEFER** | — |
| 10 | Type aliases normalized at lex time | MINOR | 4 | **KEEP** | — |
| 11 | block.timestamp field naming inconsistency | MINOR | 2 | **FIX** | TODO |
| 12 | ++/--, do-while, sub-denominations (wei/gwei/ether) | MINOR | 1 | **KEEP** | — |
| 13 | Solidity reserved keywords (23 unused) | MINOR | 8 | **KEEP** | — |
| 14 | No formatter, no LSP, no source maps | MODERATE | 11 | **DEFER** | — |
| 15 | using-for syntax enables implicit operator overloading | MODERATE | 4, 6 | **FIX** | TODO |
| 16 | `set` keyword is optional for storage writes | MODERATE | 4, 5 | **FIX** | TODO |

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

**Plan:**

1. **Deprecate** the implicit `msg.value` rewrite in `payable(uno)` functions.
2. **Require** developers to write `msg.uno_value` explicitly in `payable(uno)` functions.
3. **Emit compiler error** (not warning) if `msg.value` is used inside `payable(uno)` — with a clear diagnostic: `"use msg.uno_value in payable(uno) functions; msg.value refers to plaintext TOS amount"`.
4. **Keep** `msg.value` available in `payable(uno)` to mean the plaintext gas/fee portion if needed, maintaining semantic consistency with non-uno payable.

**Files to change:**
- `tol/sema/sema.go` — add diagnostic rejecting `msg.value` in `payable(uno)` context
- `tol_ir_direct_lowering.go` — remove the silent `msg.value → msg.uno_value` rewrite
- Update example contracts to use `msg.uno_value`

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

**Plan:**

1. **Remove** operator desugaring for `==`, `!=`, `<=`, `>=` on uno type.
2. **Reject** these operators on uno at the sema level with a clear error: `"operator '==' not supported on uno type; use a.eq(b) to make the 150k gas proof verification explicit"`.
3. **Keep** only explicit method calls: `a.eq(b)`, `a.ne(b)`, `a.lte(b)`, `a.gte(b)`.
4. **Revert** the `!=` desugaring to `not(tos.ciphertext.eq(...))` that was added in the current session.

This means uno operators will be **fully consistent**: no operators allowed, all operations are method calls. The type becomes "method-only" which is the correct design for an encrypted type with non-trivial cost.

**Files to change:**
- `tol/sema/sema.go` — move `<=`, `>=`, `!=` back to the rejection list alongside `<`, `>`, `+`, `-`, `*`, `/`; keep `==` rejection too
- `tol_ir_direct_lowering.go` — remove the `==`/`!=`/`<=`/`>=` desugaring case for uno
- `tol_ir_direct_lowering_uno_test.go` — update tests: operator forms should fail; method forms should pass
- `tol/sema/sema_test.go` — update: `TestUnoLteGteOperator` should expect rejection
- `docs/grammar/TolangParser.g4` — update uno comment section

**Note:** The `lte`, `gte`, `ne` methods and their LVM runtime implementations in gtos remain valid and unchanged. Only the **operator sugar** is removed; the **method call path** stays.

---

## Issue 3: Three Error Modes with No Semantic Distinction

**Principles violated:** 9 (Uniform Error Model)

**Problem:**

The language provides `require()`, `assert()`, and `revert()` but does not document or enforce any semantic difference between them. All three compile to Lua `error()`. The `try-catch` compiles to Lua `pcall()` which catches all errors indiscriminately, including Lua runtime errors (stack overflow, OOM).

An agent cannot determine from the ABI whether a function failure is recoverable (invalid input) or indicates a bug (invariant violation).

**Decision: FIX**

**Plan:**

1. **Define and document** the semantic distinction:
   - `require(cond, msg)` — **precondition check** (caller error, recoverable). Used for input validation, permission checks, balance checks. Emits error with selector `Error(string)`.
   - `assert(cond)` — **invariant check** (bug indicator, should never trigger in correct code). Emits error with selector `Panic(uint256)` and panic code.
   - `revert CustomError(args)` — **explicit revert** with typed error. Emits custom error selector.
2. **Differentiate at codegen level:**
   - `require` → `error({selector=0x08c379a0, msg=...})` (Error(string) ABI)
   - `assert` → `error({selector=0x4e487b71, code=...})` (Panic(uint256) ABI)
   - `revert` → `error({selector=custom_selector, args=...})`
3. **Document in ABI spec** which functions use which error types.
4. **Restrict try-catch**: only catch typed errors, not Lua runtime panics. A Lua stack overflow should be uncatchable (it indicates a VM bug, not a contract error).

**Files to change:**
- `tol_ir_direct_lowering.go` — differentiate require/assert/revert codegen
- `docs/ABI_SPEC.md` — add error model section
- `docs/TOLANG_LANGUAGE_DESIGN_PRINCIPLES.md` — reference error model

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

**Plan:**

1. **Promote ABI spec to 1.0** with concrete schemas for all fields.
2. **Define `delegation_scope`** as a JSON object with fixed keys: `{ action: string, contract: address, expiry_ms: uint64, nonce: uint64 }`.
3. **Define `proof_schema`** as an enum: `"none" | "sigma-range-v1" | "transcript-binding-v1"`.
4. **Version `task_descriptor`** properly: `{ version: "1.0", states: [...], transitions: [...] }`.
5. **Add ABI compatibility guarantee**: once 1.0 is published, field additions are backward-compatible; removals and type changes require major version bump.

**Files to change:**
- `docs/ABI_SPEC.md` — full revision
- `tol_artifact.go` — validate emitted ABI against schema

---

## Issue 5: oracle\<T\>/vote\<T\>/task\<T\> Baked into Compiler as DSL

**Principles violated:** 1 (Purpose Before Features), 8 (Restrict Complexity)

**Problem:**

Three parameterized types — `oracle<T>`, `vote<T>`, `task<T>` — are implemented as compiler intrinsics with ~145 lines of special-case codegen and ~443 lines of sema validation (agent.go). The task type has a hardcoded 8-state state machine. Agents cannot define custom state machines or oracle patterns.

These are domain-specific abstractions for a specific market pattern, not fundamental language primitives.

**Decision: FIX (phased)**

**Plan:**

Phase 1 (short-term): Keep the current implementation but **document it as "built-in library"**, not as core language feature. Make it clear in the spec that these are convenience wrappers, not the only way to build agent protocols.

Phase 2 (medium-term): Extract oracle/vote/task into a **standard library** (`stdlib/oracle.tol`, `stdlib/task.tol`, `stdlib/vote.tol`) implemented as normal contracts with struct storage. The compiler recognizes these as "blessed" libraries but does not special-case their types.

Phase 3 (long-term): Allow user-defined state machines via a `@state_machine` annotation or struct-based pattern, making task\<T\> just one instance of a general pattern.

**Files to change (Phase 1):**
- `docs/AGENT-NATIVE.md` — clarify that oracle/vote/task are built-in convenience, not core primitives
- `docs/FEATURE_MATURITY_MATRIX.md` — reclassify as "Built-in Library"

---

## Issue 6: Inheritance System with Zero Agent Use Case

**Principles violated:** 1 (Purpose Before Features), 8 (Restrict Complexity)

**Problem:**

The inheritance system (`inherit.go`, 434 lines) implements full C3 linearization, virtual/override dispatch, super calls, and abstract contract validation. No agent protocol in the codebase uses polymorphic dispatch. Agents call fixed function selectors, not virtual methods.

**Decision: FIX (phased)**

**Plan:**

Phase 1 (short-term): **Deprecate** `virtual` and `super` keywords with compiler warnings. Document that inheritance is supported only for **interface implementation** (contract implements interface), not for polymorphic hierarchies.

Phase 2 (medium-term): **Restrict** inheritance to single-level interface implementation only: `contract Foo is IBar { }` where IBar must be an interface. Remove multi-level inheritance, C3 linearization, virtual, override, and super.

Phase 3 (long-term): Replace inheritance with **composition + capability references**. A contract that needs "base" behavior imports a library.

**Files to change (Phase 1):**
- `tol/sema/sema.go` — emit deprecation warning for `virtual`, `super`
- `docs/ARCHITECTURE.md` — document the deprecation path

---

## Issue 7: Modifier Guards Invisible to @effects

**Principles violated:** 6 (Auditability), 13 (Security Boundaries)

**Problem:**

A modifier like `onlyOwner` contains a `require(msg.sender == owner)` guard, but this guard is invisible to the `@effects` annotation system. An auditor reading `@effects writes: storage.balance` does not see the permission check. The effects system reports state mutations but not authority gates.

**Decision: FIX**

**Plan:**

1. **Extend @effects syntax** to support a `guards:` clause: `@effects guards: [onlyOwner], writes: [storage.balance]`.
2. **Emit compiler warning** if a function uses a modifier but does not declare its guard in @effects.
3. **Long-term**: consider replacing modifiers entirely with `@requires` capability checks, which ARE visible in the effects system and ABI.

**Files to change:**
- `tol/ast/ast.go` — add `Guards` field to EffectsAnnotation
- `tol/sema/sema.go` — validate guards reference declared modifiers
- `tol_ir_direct_lowering.go` — no change (modifier lowering stays the same)

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

**Decision: FIX (phased)**

**Plan:**

Phase 1: **Deprecate** the `let` keyword for variable declarations with a compiler warning: `"let x: T is deprecated; use T x instead"`. The type-first form is more widely understood and aligns with Solidity, C, Go, and Java conventions.

Phase 2: **Remove** `let` from the parser after one major version cycle.

**Rationale for keeping type-first over let:** Type-first is the dominant form in systems languages (C, Go, Java, Solidity, Rust's `let x: T`). The `let` form adds a colon and reverses the type/name order, creating an unnecessary alternative.

**Files to change (Phase 1):**
- `tol/parser/parser.go` — emit deprecation warning when parsing `let` declarations
- `docs/grammar/TolangParser.g4` — mark `let` as deprecated

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

---

## Issue 12: ++/--, do-while, Sub-Denominations

**Principles violated:** 1 (Purpose Before Features) — minor

**Problem:**

- `++`/`--` operators are syntactic sugar for `x = x + 1`.
- `do-while` is equivalent to `{ body } while (cond)`.
- Sub-denomination suffixes (`wei`, `gwei`, `ether`, `seconds`, `days`) are Solidity conventions.

**Decision: KEEP**

**Rationale:**

1. **Low complexity cost** — each is a few lines in the parser, trivial lowering.
2. **Developer expectation** — removing `++` from a C-family language would surprise every developer.
3. **Sub-denominations** — `days` and `hours` are useful for deadline expressions (`block.timestamp_ms + 7 days`). The unit suffixes are computed at compile time with zero runtime overhead.
4. **do-while** — rare but occasionally clearer than while for "execute at least once" patterns.

**Optional improvement:** Deprecate `wei`/`gwei`/`ether` since TOS is a single unit, but keep time suffixes.

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

**Plan:**

1. **Restrict using-for** to method-style calls only: `using SafeMath for u256` allows `x.add(y)` but NOT operator overloading of `+`.
2. **Reject** operator-form dispatch from using-for declarations at sema level.
3. This preserves the utility of using-for (attaching methods to types) while eliminating the implicit operator overloading problem.

**Files to change:**
- `tol/sema/sema.go` — reject operator dispatch from using-for bindings
- `docs/grammar/TolangParser.g4` — update using-for documentation

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

**Plan:**

1. **Make `set` mandatory** for all storage writes (state variable assignments, mapping writes, array element writes).
2. **Emit compiler error** if a storage write lacks the `set` keyword.
3. **Keep `set` optional** for local variable reassignment (local mutation is less dangerous than storage mutation).

This makes storage writes grep-able: `grep "set "` finds all state mutations. Auditors can verify storage safety without reading the full control flow.

**Files to change:**
- `tol/sema/sema.go` — add check: assignment to storage without `set` is an error
- `tol/parser/parser.go` — no change (parser already supports both forms)
- Update all example contracts to use `set` for storage writes
