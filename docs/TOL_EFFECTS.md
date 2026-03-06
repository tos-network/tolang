# TOL Effect Annotation System

**Status:** Design v0.3 — pre-implementation
**Scope:** NatSpec-style `@effects` / `@gas` / `@bounds` doc-comment annotations on function
declarations
**Goal:** Make every externally-visible TOL function carry machine-readable, compiler-verified
metadata that allows an Agent (or any caller) to reason about side-effects, event emission,
external calls, and gas cost *before* executing the function.

---

## 1. Motivation

A TOL function today exposes its signature (name, parameter types, return types) in the `.toc` ABI.
That is sufficient for *calling* the function but not for *reasoning* about it:

| Question an Agent needs to answer | Current answer |
|-----------------------------------|---------------|
| Which storage slots does this write? | Must read source or simulate |
| Which events will be emitted? | Must read source or simulate |
| Will it make an external call? To whom? | Unknown |
| What permission is needed to authorize that call? | Unknown |
| What is the worst-case gas cost? | Unknown |
| What input constraints make the gas bound hold? | Unknown |

The effect annotation system adds a **compiler-verified metadata block** alongside the function,
embedded in the `.toc` ABI JSON and bound to the `BytecodeHash`, making it trustworthy without
re-reading the source.

---

## 2. Design Principles

1. **Optional** — a function without annotations compiles and runs exactly as before.
2. **Declarative upper-bound** — the declared set must be a *superset* of what the implementation
   actually does. The compiler verifies `effects_actual ⊆ effects_declared`.
3. **Comment-first** — annotations live in `///` or `/** */` doc comments, never in the function
   signature. The signature stays clean.
4. **Compiler-verified, chain-trusted** — verification happens at compile time; the chain trusts
   the `.toc` because the `BytecodeHash` covers the compiled bytecode that was verified against
   the declaration.
5. **Capability-first for external calls** — `@effects calls` expresses not only *what* is called
   but *how it is authorized* and *what resource limits apply*, so an Agent can reason about safety
   and budget without simulating execution.
6. **Bounds-conditional gas** — `@gas upper` is only verified when all inputs affecting cost are
   bounded; missing bounds cause `@gas upper` to be rejected, not silently accepted.

---

## 3. Syntax

### 3.1 Single-line style (`///`)

```tol
/// @notice Transfers `amount` tokens from caller to `to`.
/// @param  to     Recipient agent.
/// @param  amount Number of tokens (u256).
/// @effects reads:  storage.balances[caller], storage.allowances[caller, to]
/// @effects writes: storage.balances[caller], storage.balances[to]
/// @effects emits:  Transfer(caller, to, amount)
/// @effects calls:  []
/// @gas     upper:  50000
fn transfer(to: agent, amount: u256) -> (ok: bool) external {
    ...
}
```

### 3.2 Block style (`/** */`)

```tol
/**
 * @notice Transfers `amount` tokens from caller to `to`.
 * @param  to     Recipient agent.
 * @param  amount Number of tokens (u256).
 * @effects reads:  storage.balances[caller], storage.allowances[caller, to]
 * @effects writes: storage.balances[caller], storage.balances[to]
 * @effects emits:  Transfer(caller, to, amount)
 * @effects calls:  []
 * @gas     upper:  50000
 */
fn transfer(to: agent, amount: u256) -> (ok: bool) external {
    ...
}
```

### 3.3 Supported tags

| Tag | Key | Value format | Required? |
|-----|-----|-------------|-----------|
| `@notice` | — | Free text | No |
| `@param` | param name | Free text | No |
| `@return` | return name | Free text | No |
| `@effects` | `reads` | Comma-separated storage refs | No |
| `@effects` | `writes` | Comma-separated storage refs | No |
| `@effects` | `emits` | Comma-separated event refs | No |
| `@effects` | `calls` | Comma-separated call-cap refs, or `[]` | No |
| `@bounds` | — | Comma-separated bound constraints | No |
| `@gas` | `upper` | Decimal integer or parametric expression | No |

Multiple `@effects` lines with the same key are merged (union).

### 3.4 Storage reference syntax

Storage refs use `caller` and `this` with distinct meanings:

| Keyword | Meaning |
|---------|---------|
| `caller` | `msg.sender` — the external agent that called this function |
| `this` | The current contract's own agent identity |
| `<param>` | A function parameter name used as a mapping key |
| `*` | Wildcard: any key in this slot's key-space |

```
storage.<slot>                          entire slot (scalar)
storage.<slot>[caller]                  mapping key = msg.sender
storage.<slot>[this]                    mapping key = contract agent identity
storage.<slot>[<param>]                 mapping key = function parameter
storage.<slot>[*]                       any key (wildcard)
storage.<slot>[caller, <param>]         nested mapping: outer=caller, inner=param
storage.<slot>[*, *]                    any key at any nesting level (widest wildcard)
```

### 3.5 Event reference syntax

```
EventName                               event is emitted (any arguments)
EventName(arg1, arg2, ...)              informational: names which values are emitted
```

### 3.6 Call-cap reference syntax (capability + limits)

External calls must be expressed as structured capability references, not bare names.
This design is intentional: even when the VM does not yet enforce capabilities, the metadata
records *what authorization would be required* so Agents can reason about it.

**Empty (no external calls):**
```
[]
```

**Single structured call-cap item (key:value pairs):**
```
cap:<CapName> iface:<InterfaceName> selector:<0xHEX> max_gas:<N> max_calls:<N> max_depth:<N>
```

All fields except `max_gas` are optional in the annotation; `cap` and `selector` together form the
verifiable anchor.

**Examples:**
```
/// @effects calls: cap:OracleCap iface:IOracle selector:0x12345678 max_gas:3000 max_calls:1 max_depth:1
/// @effects calls: cap:TokenCap  iface:IERC20  selector:0xa9059cbb max_gas:5000 max_calls:1 max_depth:1
/// @effects calls: *                            // wildcard — any call; discouraged; marks function non_composable
```

Multiple calls are expressed as multiple `@effects calls:` lines (one per call site):

```tol
/// @effects calls: cap:OracleCap iface:IOracle selector:0x12345678 max_gas:3000 max_calls:1 max_depth:1
/// @effects calls: cap:VaultCap  iface:IVault  selector:0x2e1a7d4d max_gas:8000 max_calls:1 max_depth:1
```

### 3.7 `@bounds` tag

`@bounds` declares the constraints on inputs or loop counts that must hold for `@gas upper` to be
valid. Without the relevant bounds, `@gas upper` cannot be statically verified and is rejected.

```tol
/// @bounds positions_len <= 64, data_len <= 256
/// @gas     upper: 8200 + positions_len * 420 + OracleCap.max_gas
```

Bound constraint syntax:
```
<ident> <= <N>       variable (param or loop counter) has an integer upper bound
<ident> == <N>       variable is a known constant
```

`@gas upper` may be a parametric expression using bound variables and `<CapName>.max_gas`:
```
upper: 8200 + positions_len * 420 + OracleCap.max_gas
```

The compiler evaluates this expression by substituting declared bound values and summing with
`max_gas` from each declared call-cap to produce a concrete upper bound for verification.

### 3.8 Binding rules

A doc-comment block binds to the **immediately following declaration** according to these rules:

1. A `///` line or a `/** */` block immediately before a `fn`, `interface fn`, `constructor`,
   `fallback`, `receive`, `event`, `struct`, or `contract` declaration is bound to that declaration.
2. Intervening blank lines are allowed; the binding is not broken by whitespace.
3. Any non-comment, non-whitespace token between the doc-comment and the declaration **breaks**
   the binding — the comment is treated as a free-standing doc comment and no annotation is
   extracted.
4. Only the **closest** doc-comment block immediately before the declaration is bound; earlier
   blocks are ignored for annotation purposes.

---

## 4. AST Changes

### 4.1 New types in `tol/ast/ast.go`

```go
// DocMeta holds structured NatSpec-style metadata parsed from a /// or /** */ doc comment.
// All fields are optional; nil means "not declared".
type DocMeta struct {
    Notice  string      // @notice text (free-form)
    Params  []DocParam  // @param entries, one per parameter
    Returns []DocParam  // @return entries, one per return value
    Effects *EffectDecl // @effects entries; nil = no effects declared
    Bounds  *BoundsDecl // @bounds entries; nil = no bounds declared
    Gas     *GasDecl    // @gas entries; nil = no gas bound declared
}

// DocParam is a single @param or @return entry.
type DocParam struct {
    Name string
    Text string
}

// EffectDecl is the structured representation of @effects annotations.
type EffectDecl struct {
    Reads  []string    // canonical storage slot refs declared as read
    Writes []string    // canonical storage slot refs declared as written
    Emits  []string    // event refs declared as emitted
    Calls  []CallRef   // capability-based external call refs; nil = not declared
}

// CallRef is a single structured external call declaration.
// It encodes authorization (Cap, Iface, Selector) and resource limits.
type CallRef struct {
    Cap      string // capability type name (e.g. "OracleCap"); "" = wildcard
    Iface    string // interface type name (e.g. "IOracle"); informational
    Selector string // 4-byte hex selector (e.g. "0x12345678"); "" = any
    MaxGas   uint64 // gas budget for this call; 0 = not specified
    MaxCalls uint32 // maximum number of times this call may occur; 0 = not specified
    MaxDepth uint32 // maximum call-stack depth from this point; 0 = not specified
    Wildcard bool   // true when declared as "*"
}

// BoundsDecl holds the @bounds constraints.
type BoundsDecl struct {
    Constraints []BoundConstraint
}

// BoundConstraint is a single bound: "ident <= N" or "ident == N".
type BoundConstraint struct {
    Ident string
    Op    string // "<=" or "=="
    Value uint64
}

// GasDecl holds the @gas upper bound declaration.
type GasDecl struct {
    Upper     uint64 // concrete upper bound (after substituting bounds); 0 if parametric
    Expr      string // raw parametric expression string (e.g. "8200 + len*420 + OracleCap.max_gas")
    Evaluated uint64 // compiler-evaluated concrete value (set during sema); 0 if UNBOUNDED
}
```

### 4.2 `FunctionDecl` extension

```go
type FunctionDecl struct {
    Name             string
    SelectorOverride string
    Params           []FieldDecl
    Returns          []FieldDecl
    Modifiers        []string
    Body             []Statement
    Virtual          bool
    Override         bool
    Doc              *DocMeta   // nil if no doc comment present  ← NEW
}
```

The same `Doc *DocMeta` field is added to:
- `FuncSigDecl` (interface function signatures)
- `ConstructorDecl`
- `FallbackDecl`
- `ReceiveDecl`

---

## 5. Lexer Changes

### 5.1 New token type

```go
TokenDocComment  // /// ... or /** ... */
```

`TokenDocComment.Literal` contains the **raw text** of the comment block, including all `///`
prefixes and `/** */` delimiters, with newlines preserved. The parser is responsible for extracting
individual tag lines.

### 5.2 Recognition rules

| Input | Current behaviour | New behaviour |
|-------|-------------------|---------------|
| `// comment` | Discarded in `skipSpaceAndComments` | Unchanged — discarded |
| `/// comment` | Discarded | Emitted as `TokenDocComment` |
| `/** ... */` | Discarded | Emitted as `TokenDocComment` |
| `/* ... */` (single `*`) | Discarded | Unchanged — discarded |

`skipSpaceAndComments` is split into two helpers:

- `skipSpaceAndOrdinaryComments` — skips whitespace, `//` (non-`///`), and any `/* */` that does
  **not** start with `/**`.
- The main `Next()` loop checks for `///` and `/**` before falling through to
  `skipSpaceAndOrdinaryComments`.

```go
func (l *Lexer) Next() Token {
    l.skipSpaceAndOrdinaryComments()
    start := l.pos()
    if l.eof() { return Token{Type: TokenEOF, ...} }

    if l.peek() == '/' {
        if l.peekN(1) == '/' && l.peekN(2) == '/' {
            return l.readTripleSlashDoc(start)   // consumes all consecutive /// lines
        }
        if l.peekN(1) == '*' && l.peekN(2) == '*' {
            return l.readBlockDoc(start)          // consumes /** ... */
        }
    }
    // ... rest of existing switch unchanged
}
```

`readTripleSlashDoc` reads all **consecutive** `///` lines — including **empty `///` lines**
(a line that contains only `///` with no following text) — into a single `TokenDocComment`.
Empty `///` lines act as blank paragraph separators within the doc block but do not break
the token boundary. A true blank line (a line that does not start with `///`) **does** break
the consecutive sequence and ends the token.

```
Input:                          TokenDocComment.Literal:
  /// @notice hello              "/// @notice hello\n"
  ///                            "/// @notice hello\n///\n"  (empty /// continues block)
  /// @effects reads: s.x        "/// @notice hello\n///\n/// @effects reads: s.x\n"
  <blank line>                   ← sequence ends here
  fn f() {}
```

**Parser-level merge (recommended):** If the parser encounters **multiple consecutive**
`TokenDocComment` tokens without any intervening non-comment tokens (e.g., a blank line broke
one `///` block into two), it should merge them by concatenating the raw literals before calling
`parseDocMeta`. This handles edge cases where formatting tools or editors insert blank lines
inside doc blocks.

---

## 6. Parser Changes

### 6.1 Doc-comment accumulation

The parser maintains a field:

```go
type Parser struct {
    ...
    pendingDoc *ast.DocMeta // set when a TokenDocComment is consumed; cleared on use or non-doc token
}
```

When the parser encounters a `TokenDocComment` token, it:
1. Parses the raw text into `*ast.DocMeta` via `parseDocMeta`.
2. If `pendingDoc` is already non-nil (consecutive doc-comment tokens), merges by re-running
   `parseDocMeta` on the concatenated raw text of both blocks.
3. Stores the result in `pendingDoc`.

When the parser begins parsing a declaration (`fn`, `constructor`, `fallback`, `receive`,
`interface fn`), it:
1. Takes `pendingDoc` and attaches it to the declaration's `Doc` field.
2. Sets `pendingDoc = nil`.

If any non-whitespace, non-doc-comment token is consumed between the doc-comment and the
declaration, `pendingDoc` is set to `nil` without error.

### 6.2 Doc-comment parsing (`parseDocMeta`)

```
Input:  raw string of a doc-comment block
Output: *ast.DocMeta

Algorithm:
  1. Split into lines.
  2. Strip leading whitespace + "* " (block style) or "/// " (line style) prefix.
  3. Strip trailing inline comments: text after " //" on a tag line is ignored.
  4. For each line matching /^\s*@(\w+)\s*((\w+)\s*:\s*)?(.*)$/:
       - @notice <text>            → DocMeta.Notice
       - @param <name> <text>      → DocMeta.Params
       - @return <name> <text>     → DocMeta.Returns
       - @effects reads:  <refs>   → EffectDecl.Reads  (parse comma-separated refs, canonicalize)
       - @effects writes: <refs>   → EffectDecl.Writes
       - @effects emits:  <refs>   → EffectDecl.Emits
       - @effects calls:  <refs>   → EffectDecl.Calls  (parse CallRef list; "[]" → empty []CallRef{})
       - @bounds <constraints>     → BoundsDecl.Constraints
       - @gas upper: <expr>        → GasDecl.Expr (or GasDecl.Upper if purely numeric)
  5. Unknown tags are stored in a raw []DocTag slice for forward compatibility.
```

### 6.3 Effect ref canonicalization during parsing

All storage refs are immediately canonicalized:
- Whitespace around `,`, `[`, `]` is stripped.
- `self` is rejected with a parse error directing the author to use `caller` or `this`.
- Key names are normalized to lowercase.

Example: `storage.allowances[caller , to]` → `storage.allowances[caller,to]`

### 6.4 CallRef parsing

A `@effects calls: []` line produces `EffectDecl.Calls = []CallRef{}` (empty, non-nil).
A missing `@effects calls:` line leaves `EffectDecl.Calls = nil` (not declared).

Each `@effects calls: <item>` line is parsed as **key:value pairs**:

```
cap:<Name>        → CallRef.Cap
iface:<Name>      → CallRef.Iface
selector:<0xHEX>  → CallRef.Selector (validated: must be 4-byte hex)
max_gas:<N>       → CallRef.MaxGas
max_calls:<N>     → CallRef.MaxCalls
max_depth:<N>     → CallRef.MaxDepth
*                 → CallRef.Wildcard = true
```

### 6.5 Precise `parseDocMeta` state machine

```
State: LINE_START
  consume leading whitespace
  if "///" → strip "///" prefix, advance to TAG_SCAN
  if " * " | " *\n" | "/**" | "*/" → strip block-comment decoration, advance to TAG_SCAN
  else → advance to TAG_SCAN (bare line in block comment)

State: TAG_SCAN
  if line matches /^\s*@(\w+)(.*)$/ → transition to TAG_BODY(tag=\1, rest=\2)
  else → treat as continuation of DocMeta.Notice (or ignore if Notice already set)
  advance to LINE_START

State: TAG_BODY(tag, rest)
  switch tag:
    "notice"  → DocMeta.Notice = strings.TrimSpace(rest)
    "param"   → split rest on first whitespace: name=first, text=remainder
                 append DocParam{Name,Text} to DocMeta.Params
    "return"  → same as @param but append to DocMeta.Returns
    "effects" → split rest on ":": key=first-word, value=after-colon
                 switch key:
                   "reads"  → parse comma-refs → canonicalize → append to EffectDecl.Reads
                   "writes" → parse comma-refs → canonicalize → append to EffectDecl.Writes
                   "emits"  → parse comma-refs → append to EffectDecl.Emits
                   "calls"  → if value=="[]" → set EffectDecl.Calls=[]CallRef{}
                               else parse CallRef (see §6.4) → append to EffectDecl.Calls
    "bounds"  → split on "," → for each: parse "<ident> <op> <N>"
                 append BoundConstraint to BoundsDecl.Constraints
    "gas"     → split rest on ":": key=first-word, value=after-colon
                 if key=="upper": if value is purely /^[0-9]+$/ → GasDecl.Upper=N
                                  else → GasDecl.Expr=TrimSpace(value)
    default   → append DocTag{Key:tag, Value:rest} to raw unknown tags

Comma-ref parser (for reads/writes/emits):
  split on "," (but not inside brackets)
  for each token: TrimSpace → canonicalize → reject "self" with error
  return []string

CallRef parser (for calls item):
  if TrimSpace(item) == "*" → return CallRef{Wildcard:true}
  tokenize on whitespace
  for each token matching /^(\w+):(.+)$/:
    key=\1, val=\2
    switch key:
      "cap"       → CallRef.Cap = val
      "iface"     → CallRef.Iface = val
      "selector"  → validate 0x[0-9a-fA-F]{8} → CallRef.Selector = val
      "max_gas"   → parse uint64 → CallRef.MaxGas
      "max_calls" → parse uint32 → CallRef.MaxCalls
      "max_depth" → parse uint32 → CallRef.MaxDepth
      else        → ignore (forward-compat)
  return CallRef
```

### 6.6 CallRef coverage judgment function

The sema layer uses the following three-state function to determine whether a declared `CallRef`
covers an inferred `CallSite`:

```
CoverageResult = COVERED | NOT_COVERED | WILDCARD_COVERED

func callRefCovers(declared CallRef, site CallSite) CoverageResult:
    // Wildcard declared: covers everything, but marks function non_composable
    if declared.Wildcard:
        return WILDCARD_COVERED

    // If selector declared and site selector known: must match exactly
    if declared.Selector != "" and site.Selector != "":
        if declared.Selector != site.Selector:
            return NOT_COVERED

    // If selector declared but site selector unknown (dynamic): declared does not cover
    if declared.Selector != "" and site.Selector == "":
        return NOT_COVERED

    // If no selector declared: covers any selector (cap-only declaration)
    return COVERED

Coverage algorithm for a function's Calls dimension:
    for each site in inferred.Calls:
        matched = false
        wildcard = false
        for each ref in declared.Calls:
            result = callRefCovers(ref, site)
            if result == COVERED:
                matched = true; break
            if result == WILDCARD_COVERED:
                matched = true; wildcard = true; break
        if not matched:
            emit TOL2200 (undeclared call site)
        if wildcard:
            mark function as non_composable = true

    for each ref in declared.Calls where ref.Selector != "":
        if no site in inferred.Calls has site.Selector == ref.Selector:
            emit TOL2205 (declared selector with no matching IR call site)
```

The **three states** are:
- `COVERED` — the declared ref covers this site; no further action.
- `NOT_COVERED` — selector mismatch or dynamic target against declared selector; emit TOL2200.
- `WILDCARD_COVERED` — site is covered but marks the function as `non_composable: true` in the
  ABI JSON (see §9).

---

## 7. Sema Validation

### 7.1 Effect inference

The sema layer extends the existing `pure`/`view` effect tracking (in `tol/sema/effects.go`) to
produce an `InferredEffects` struct per function:

```go
type InferredEffects struct {
    Reads  []string  // canonical storage slot refs actually read in the IR
    Writes []string  // canonical storage slot refs actually written
    Emits  []string  // event names actually emitted
    Calls  []CallSite // external call sites found in the IR
}

type CallSite struct {
    Selector string // 4-byte hex, if statically known
    Target   string // agent expression (may be "dynamic")
}
```

### 7.2 Declared vs. inferred check

For every function with non-nil `Doc.Effects`:

```
For each dimension D in {Reads, Writes, Emits}:
    for each ref in inferred[D]:
        if ref is not covered by any entry in declared[D]:
            emit TOL2200

For each call site in inferred.Calls:
    if EffectDecl.Calls == nil:
        skip (calls not declared — no verification)
    if EffectDecl.Calls == []CallRef{} (declared empty):
        if any call site exists: emit TOL2204
    else:
        use callRefCovers() for each (declared, site) pair (§6.6)
```

**Coverage rules for storage refs:**

| Declared ref | Covers inferred ref |
|-------------|---------------------|
| `storage.balances[*]` | `storage.balances[caller]`, `storage.balances[to]`, any key |
| `storage.balances[caller]` | `storage.balances[caller]` only |
| `storage.allowances[caller,*]` | `storage.allowances[caller,to]`, `storage.allowances[caller,x]` |
| Wildcard `*` (calls) | any external call site (but sets `non_composable: true`) |

Inferred refs **not** in the declared set produce TOL2200 and print the diff:

```
TOL2200: undeclared effect in function 'transfer'
         actual writes: storage.total_supply
         declared writes: storage.balances[caller], storage.balances[to]
         storage.total_supply is written but not covered by any declared writes ref
         hint: add 'storage.total_supply' to @effects writes
```

Declared refs **not** in the inferred set are permitted (safe over-approximation).
The compiler may emit a hint (not error) for severe over-approximation under `-Weffects-overapprox`.

### 7.3 Gas upper-bound verification

**Step 1 — Determine if the function is statically bounded:**

A function is **statically bounded** when every loop in the body has a provable finite iteration
count.  The compiler recognises the following bounded-loop forms:

- `for let i = 0; i < N; ...` where `N` is a numeric literal in the condition — the literal
  value is used as the iteration count directly.
- `for let i = 0; i < v; ...` where `v` is an identifier declared in `@bounds` — the declared
  bound value is used as the iteration count.
- `while v <= N` (or `while v < N`, `while v != N`) where `v` is declared in `@bounds` — the
  bound value is used as the iteration count.
- `do { ... } while (v <= N)` — same rule as `while`.

If any loop cannot be bounded using one of the above forms and no corresponding `@bounds`
constraint exists, the function is **UNBOUNDED**.

**Step 2 — Evaluate concrete `@gas upper`:**

When `@gas upper: N` (a plain integer) is declared:

If the function is UNBOUNDED → **TOL2201** error:
```
TOL2201: cannot verify @gas upper: function 'settle' contains an unbounded loop
         or dynamic iteration not covered by @bounds
         declare loop bounds with @bounds or remove @gas upper
```

If the function is bounded:
- Compute `gas_inferred` using the conservative static cost model (see §7.4), threading the
  `@bounds` declaration through loop unrolling.
- If `gas_declared < gas_inferred` → **TOL2202** error:
```
TOL2202: @gas upper too low in function 'settle'
         declared:  50000
         inferred conservative upper bound: 61200
         raise @gas upper to at least 61200 or reduce the function's cost
```

**Step 3 — Evaluate parametric `@gas upper`:**

When `@gas upper` is a parametric expression (e.g. `8200 + positions_len * 420 + OracleCap.max_gas`),
the compiler validates that the expression can be fully evaluated:

- Each plain identifier in the expression must be declared in `@bounds`.  Its value is taken from
  the bound constraint (`<` and `<=` use the bound value; `==` uses the exact value).
- Each `<Cap>.max_gas` term must match a `CallRef.Cap` name in the declared `@effects calls`.
  The `CallRef.MaxGas` value is substituted.
- The expression is evaluated using `+`, `*`, and parentheses with standard operator precedence
  (`*` before `+`).

If any identifier cannot be resolved → **TOL2201** error:
```
TOL2201: cannot verify @gas upper: parametric expression contains an unresolved identifier
```

When all identifiers resolve, the parametric expression is accepted as the declared upper bound
without further comparison against the body gas estimate. The developer takes responsibility for
the correctness of the bound.

**Conservative sum rule:** When multiple `CallRef` entries are declared, `@gas upper` evaluation
sums *all* their `MaxGas` values — regardless of whether the underlying call sites are on mutually
exclusive branches. This is **intentionally conservative**: an Agent can safely budget by the
declared upper bound without needing to understand internal control flow.

A future annotation `@gas path: worstpath` (not yet implemented) may allow the compiler to use
path-sensitive analysis to produce a tighter bound on branching call sites. The current model keeps
this interface open: `@gas upper` is the conservative single-path budget; `@gas path:` is a
reserved key for the per-path refinement.

### 7.4 Conservative gas cost model

| Operation | Cost (gas units) |
|-----------|-----------------|
| Each `sstore` (any value) | `SSTORE_COST` = 20000 |
| Each `sload` | `SLOAD_COST` = 2100 |
| Each `log0`/`log1`/`log2`/`log3`/`log4` | `LOG_BASE` + `LOG_TOPIC` × n\_topics |
| Each external call | sum of all declared `CallRef.MaxGas` (conservative — see §7.3) |
| Each instruction (default) | 1 |
| Loop body (bounded by `N`) | `N` × body\_gas |
| `LOG_BASE` | 375 |
| `LOG_TOPIC` | 375 |

This model is deliberately conservative (over-estimates). Future versions may refine it.

**VM binding:** `SSTORE_COST`, `SLOAD_COST`, `LOG_BASE`, and `LOG_TOPIC` are **static estimation
parameters specific to TOLANG v0.2** and the GTOS VM configuration. They are not raw EVM opcode
gas values — they are deliberate conservative overestimates calibrated to the GTOS metering model.
Future VM versions may update these constants.

The `.toc` format includes a `gas_model` top-level field in the ABI JSON to record which cost
model version was used during compilation. It lets Agents verify that the `gas_upper` values in
the ABI were computed under the same model version as the VM they are targeting:

```json
{
  "gas_model": { "version": "tolang/0.2.0", "sload": 2100, "sstore": 20000, "log_base": 375 }
}
```

---

## 8. Canonicalization

All storage refs — both declared and inferred — are stored in canonical form before comparison.
This prevents spurious mismatches from formatting differences.

**Canonical form rules:**

1. No spaces inside `[` / `]` or around `,`.
2. Keyword `caller` = `msg.sender`; keyword `this` = contract's own agent identity. Both are preserved
   as-is (not collapsed).
3. Slot name and key names are lowercase.
4. Multiple keys in nested mappings use `,` as separator: `[caller,to]`.
5. `*` wildcards are kept as-is.

**Examples:**

| Input | Canonical form |
|-------|---------------|
| `storage.balances [ caller ]` | `storage.balances[caller]` |
| `storage.allowances[caller, to]` | `storage.allowances[caller,to]` |
| `storage.Balances[*]` | `storage.balances[*]` |

**Scope of v0.2 storage refs:**

- Storage refs are resolved against the **current contract's storage slots only** — first-level
  names declared in `storage { slot <name>: ... }`.
- **Cross-contract storage references are not supported.** It is not possible to declare
  `@effects reads: OtherContract.storage.balances[...]` in v0.2; such refs are rejected with a
  parse error. This avoids ambiguity when two imported contracts define slots with the same name.
- **Struct-path storage refs** (e.g., `storage.user[caller].balance` for a struct-valued slot)
  are a **future extension**. In v0.2 all refs stop at the slot name level. When struct-path refs
  are introduced, the canonicalization grammar will be extended; existing canonical refs (which
  have no `.field` suffix) will continue to be valid under the extended rules.

---

## 9. Output: ABI JSON Extension

The `.toc` ABI JSON (§4 of `docs/toc-format.md`) is extended per function.

### 9.1 Full example

```json
{
  "functions": [
    {
      "name": "transfer",
      "visibility": "external",
      "selector": "0xf6136730",
      "params": ["agent", "u256"],
      "returns": ["bool"],
      "doc": {
        "notice": "Transfers `amount` tokens from caller to `to`.",
        "effects": {
          "reads":  ["storage.balances[caller]", "storage.allowances[caller,to]"],
          "writes": ["storage.balances[caller]", "storage.balances[to]"],
          "emits":  ["Transfer"],
          "calls":  []
        },
        "gas_upper": 50000
      }
    },
    {
      "name": "settle",
      "visibility": "external",
      "selector": "0xabcd1234",
      "params": ["u256"],
      "returns": [],
      "doc": {
        "effects": {
          "reads":  ["storage.positions[*]"],
          "writes": ["storage.balances[*]"],
          "emits":  ["Settled"],
          "calls": [
            {
              "cap": "OracleCap",
              "iface": "IOracle",
              "selector": "0x12345678",
              "max_gas": 3000,
              "max_calls": 1,
              "max_depth": 1
            }
          ]
        },
        "bounds": ["positions_len<=64"],
        "gas_upper": 14600
      }
    },
    {
      "name": "emergencyDrain",
      "visibility": "external",
      "selector": "0xdeadbeef",
      "params": [],
      "returns": [],
      "doc": {
        "effects": {
          "calls": [{ "wildcard": true }]
        },
        "non_composable": true
      }
    }
  ]
}
```

### 9.2 Field presence rules

| JSON field | Emitted when |
|------------|-------------|
| `doc` | Any doc annotation is present on the function |
| `doc.notice` | `@notice` is non-empty |
| `doc.effects` | At least one `@effects` tag is present |
| `doc.effects.reads` | `@effects reads:` is present (even if empty) |
| `doc.effects.writes` | `@effects writes:` is present |
| `doc.effects.emits` | `@effects emits:` is present |
| `doc.effects.calls` | `@effects calls:` is present (`[]` when declared empty) |
| `doc.bounds` | `@bounds` is present |
| `doc.gas_upper` | `@gas upper` is present and successfully verified |
| `doc.non_composable` | `@effects calls: *` wildcard is present; always `true` when emitted |

**`non_composable: true`** is a fast-path signal for tooling and Agents: a function tagged
`non_composable` may make arbitrary external calls and should not be treated as safe for
automated invocation without additional authorization review. Tooling that processes `.toc` files
does not need to inspect the `calls` array to detect this condition.

### 9.3 Trust model

An Agent that has verified the `.toc` `BytecodeHash` can trust all `doc.*` fields, because:

1. The compiler verified `effects_actual ⊆ effects_declared` against the compiled bytecode.
2. The `BytecodeHash` in the `.toc` covers exactly that compiled bytecode.
3. Therefore: any `doc.effects` in the ABI JSON is as trustworthy as the bytecode itself.

An Agent may additionally verify:
- The compiler version via `Artifact.Compiler`.
- The cost model version via the optional `gas_model` field in the `.toc` (see §7.4).
- An optional publisher signature over `keccak256(canonical_abi_json)` (future extension).

---

## 10. Diagnostic Codes

| Code | Condition |
|------|-----------|
| TOL2200 | Inferred effect not covered by declared effect set |
| TOL2201 | `@gas upper` declared but function is UNBOUNDED (missing bounds for loops or dynamic inputs) |
| TOL2202 | Declared `@gas upper` is less than the inferred conservative upper bound |
| TOL2204 | `@effects calls: []` declared but an external call was found in the implementation |
| TOL2205 | `@effects calls:` declared with a `selector` but no matching call site found in the IR |

---

## 11. Complete Example

```tol
source tolang 0.2.0

contract TRC20 {
    storage {
        slot balances:     mapping(agent => u256);
        slot allowances:   mapping(agent => mapping(agent => u256));
        slot total_supply: u256;
    }

    event Transfer(from: agent indexed, to: agent indexed, value: u256)
    event Approval(owner: agent indexed, spender: agent indexed, value: u256)

    /**
     * @notice Returns the token balance of `owner`.
     * @param  owner   The agent to query.
     * @return balance The token balance.
     * @effects reads:  storage.balances[owner]
     * @effects writes: []
     * @effects emits:  []
     * @effects calls:  []
     * @gas     upper:  2500
     */
    fn balanceOf(owner: agent) -> (balance: u256) public view {
        return balances[owner];
    }

    /**
     * @notice Transfers `amount` tokens from caller to `to`.
     * @effects reads:  storage.balances[caller], storage.allowances[caller,to]
     * @effects writes: storage.balances[caller], storage.balances[to]
     * @effects emits:  Transfer
     * @effects calls:  []
     * @gas     upper:  50000
     */
    fn transfer(to: agent, amount: u256) -> (ok: bool) external {
        require(balances[msg.sender] >= amount, "INSUFFICIENT_BALANCE");
        set balances[msg.sender] -= amount;
        set balances[to] += amount;
        emit Transfer(msg.sender, to, amount);
        return true;
    }
}
```

ABI JSON entry for `transfer`:

```json
{
  "name": "transfer",
  "visibility": "external",
  "selector": "0xf6136730",
  "params": ["agent", "u256"],
  "returns": ["bool"],
  "doc": {
    "effects": {
      "reads":  ["storage.balances[caller]", "storage.allowances[caller,to]"],
      "writes": ["storage.balances[caller]", "storage.balances[to]"],
      "emits":  ["Transfer"],
      "calls":  []
    },
    "gas_upper": 50000
  }
}
```

An Agent calling this contract reads the ABI JSON and knows — before sending any transaction:
- Exactly which storage keys may change.
- Which event will be emitted.
- No external calls (no re-entrancy risk).
- Maximum gas to budget.

---

## 12. Implementation Plan

| Phase | Files | Deliverable |
|-------|-------|-------------|
| **P1** | `tol/lexer/lexer.go`, `tol/lexer/token.go` | `TokenDocComment`; `///` and `/** */` recognition; empty `///` line handling |
| **P2** | `tol/ast/ast.go` | `DocMeta`, `EffectDecl`, `CallRef`, `BoundsDecl`, `GasDecl`; `FunctionDecl.Doc` |
| **P3** | `tol/parser/parser.go` | `parseDocMeta` state machine; `parseCallRef`; binding + merge logic; canonicalization |
| **P4** | `tol/diag/diag.go` | TOL2200–TOL2205 diagnostic codes |
| **P5** | `tol/sema/effects.go` | Effect inference; declared vs. inferred check; `callRefCovers`; bounds + gas verification; `non_composable` flag |
| **P6** | `tol_toc.go` | `doc` field in ABI JSON output (`effects`, `bounds`, `gas_upper`, `non_composable`) |
| **P7** | `tol/parser/parser_test.go`, `tol/sema/sema_test.go`, `tol_api_test.go` | Full test coverage |

Each phase is independently testable. P1–P3 produce a correctly-populated `FunctionDecl.Doc`
without requiring any sema changes. P4–P5 build on the parsed AST. P6 requires P5.

**Recommended implementation order (by benefit/effort ratio):**

1. P1–P3 — get doc meta all the way through to AST; enables inspection tooling immediately.
2. P5 partial — effects `reads/writes/emits` verification first; `calls` existence + selector
   coverage second; `max_calls` loop-bound linkage last (most complex).
3. P5 gas — `UNBOUNDED` rejection + simple cost model (the table in §7.4); parametric `@gas`
   expressions last.
4. P6 — ABI JSON output; straightforward once P5 is done.
5. P7 — test coverage throughout.
