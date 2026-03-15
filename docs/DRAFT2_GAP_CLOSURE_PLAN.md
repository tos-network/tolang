# AGENT_PROTOCOL_DRAFT2.tol Feature Gap Closure — COMPLETED

**Status:** All gaps closed
**Completed:** 2026-03-15
**Commit:** `8d35f4f feat: AGENT_PROTOCOL_DRAFT2 feature gap closure + event emit protocol overhaul`

## Context

`docs/AGENT_PROTOCOL_DRAFT2.tol` is the agent-native TOL syntax target — a complete agent
economy protocol covering registry, reputation, task escrow, dispute resolution, prediction
markets, and reward vaults.

Seven feature groups were identified as missing from the compiler. **All have been implemented.**

---

## Implementation Summary

### G1 — Syntax Quick-Wins

All four sub-gaps completed:

| Sub-gap | Feature | Implementation |
|---------|---------|----------------|
| G1a | `deploy` keyword | `TokenKwDeploy` in lexer; `parseDeployStatement()` + expression-level deploy in parser |
| G1b | `manifest {}` extensions | `parseManifestDecl` accepts `;`/`,` separators, numeric values, array values `[A, B]` |
| G1c | `escrow`/`release`/`slash` optional purpose | Sema allows 2-3 args for escrow/release, 2-4 for slash; omitted purpose defaults to 0 |
| G1d | `@pay(amount)` bare form | `parsePayTag` handles bare form; `DocMeta.PayIsBare` field tracks it |

### G2 — Top-level `capability` Declarations

Completed. `Module.Capabilities` field in AST; `parseModule()` parses top-level `capability`;
sema merges module-level and contract-level capabilities; lowering propagates both.

### G3 — `oracle<T>` OOP Member Interface

Completed. Sema whitelists `.fulfill()`, `.is_set`, `.value` on oracle-typed storage slots.
`lowerOracleSlotExpr()` in IR lowering emits calls to `__tol_oracle_fulfill`,
`__tol_oracle_is_set`, `__tol_oracle_value` prelude helpers.

### G4 — `task<T>` OOP Interface

Completed in all three phases:

| Phase | Feature | Implementation |
|-------|---------|----------------|
| G4a | `mapping(K => task<T>)` storage type | Sema accepts; lowering emits sub-slot hashes (poster/worker/reward/deadline/data) |
| G4b | `tasks[tid].method()` calls | `lowerTaskMappingCallExpr()` handles `.accept()`, `.submit()`, `.approve()`, `.reject()`, `.dispute()`, `.cancel()`, `.new()` |
| G4b | `tasks[tid].property` reads | `lowerTaskMappingMemberExpr()` handles `.worker`, `.poster`, `.reward`, `.deadline`, `.is_expired`, `.state` |
| G4c | `task<T>` local variable handles | `loweringCtx.taskLocals` tracks task-typed locals; method/property calls on locals work |

### G5 — `agent` Property Access

Completed. `lowerAgentPropertyExpr()` handles `.stake`, `.is_active`, `.reputation`,
`.rating_count`, `.suspended` on `agent(expr)` calls, agent-typed locals, and agent-typed
storage slots. Backed by `__tol_agent_prop` prelude helper calling `tos.agentload`.

---

## Files Modified

| File | Changes |
|------|---------|
| `tol/lexer/token.go` | `TokenKwDeploy` |
| `tol/ast/ast.go` | `Module.Capabilities`; `ManifestField.IsArray/Array`; `DocMeta.PayIsBare` |
| `tol/parser/parser.go` | `deploy` keyword; top-level `capability`; manifest extensions; `@pay` bare form |
| `tol/sema/sema.go` | Module-cap merge; oracle/task/agent member whitelists |
| `tol/sema/agent.go` | Escrow arity relaxation; module cap propagation; `@pay` bare TOL2309 skip |
| `tol/lower/lower.go` | Module capability propagation |
| `tol_ir_direct_lowering.go` | `lowerOracleSlotExpr`, `lowerTaskMappingCallExpr`, `lowerTaskMappingMemberExpr`, `lowerAgentPropertyExpr`; extended prelude |
| `tol_artifact.go` | Manifest array/numeric value serialization |

---

## Verification

```bash
# All gaps closed — DRAFT2 compiles successfully
go run ./cmd/tolang compile --emit toc -o /tmp/draft2.toc docs/AGENT_PROTOCOL_DRAFT2.tol
```
