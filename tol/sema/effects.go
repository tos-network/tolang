package sema

// effects.go — verification passes for:
//   Pass 1: effect checks (pure/view/payable enforcement)
//   Pass 3: uninitialized variable reads
//   Pass 4: msg.value in non-payable functions (folded into Pass 1)
//
// Pass 2 (selector uniqueness) is already handled in sema.go via
// selectorDispatchKey / selectorSeen; no new code needed there.

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/diag"
)

// effectKind classifies which effect restrictions apply to a function.
type effectKind int

const (
	effectNone    effectKind = iota // no restriction (internal/private, no pure/view)
	effectView                      // view: no writes, no emit, no state-changing calls
	effectPure                      // pure: no reads or writes, no env, no emit, no external calls
	effectPayable                   // payable: no additional restrictions
)

// functionEffectKind derives the effectKind for a function from its modifiers.
func functionEffectKind(modifiers []string) effectKind {
	for _, m := range modifiers {
		switch m {
		case "pure":
			return effectPure
		case "view":
			return effectView
		case "payable":
			return effectPayable
		}
	}
	return effectNone
}

// isPayableFunction returns true when the function has the payable modifier.
func isPayableFunction(modifiers []string) bool {
	for _, m := range modifiers {
		if m == "payable" {
			return true
		}
	}
	return false
}

// externalStateCallNames is the set of builtin call names that perform
// state-changing or externally-observable operations.
var externalStateCallNames = map[string]bool{
	"call":         true,
	"create":       true,
	"create2":      true,
	"createx":      true,
	"create2x":     true,
	"transfer":     true,
	"delegatecall": true,
}

// staticCallNames are allowed in view (read-only external calls).
var staticCallNames = map[string]bool{
	"staticcall": true,
}

// checkFunctionEffects enforces pure/view/payable constraints for fn.
// storageSlotNames is the set of declared storage slot names in the contract.
func checkFunctionEffects(filename string, fn ast.FunctionDecl, storageSlotNames map[string]bool, diags *diag.Diagnostics) {
	ek := functionEffectKind(fn.Modifiers)
	payable := isPayableFunction(fn.Modifiers)

	switch ek {
	case effectPure:
		checkPureStmts(filename, fn.Name, fn.Params, fn.Body, storageSlotNames, diags)
	case effectView:
		// view functions cannot write storage, emit, or perform state-changing calls.
		// They also cannot access msg.value (since they cannot receive ETH, and
		// msg.value would always be 0 in a view call; flagging it avoids confusion).
		checkViewStmts(filename, fn.Name, fn.Params, fn.Body, storageSlotNames, diags)
		if !payable {
			checkMsgValueInStmts(filename, fn.Name, fn.Params, fn.Body, diags)
		}
	case effectPayable:
		// payable: no restrictions beyond normal checks (msg.value is fine here)
	default:
		// effectNone: standard function without pure/view/payable.
		// No additional effect restrictions applied.
	}

	// P5: validate declared @effects and @gas upper if present.
	checkDeclaredEffects(filename, fn, storageSlotNames, diags)
	checkGasUpperBound(filename, fn, storageSlotNames, diags)
	checkTotalCostBound(filename, fn, diags)
}

// --------------------------------------------------------------------------
// pure enforcement
// --------------------------------------------------------------------------

func checkPureStmts(filename, fnName string, params []ast.FieldDecl, stmts []ast.Statement, storageSlotNames map[string]bool, diags *diag.Diagnostics) {
	localNames := buildLocalNameSet(params)
	for _, s := range stmts {
		checkPureStmt(filename, fnName, &localNames, s, storageSlotNames, diags)
	}
}

func checkPureStmt(filename, fnName string, localNames *map[string]bool, s ast.Statement, storageSlotNames map[string]bool, diags *diag.Diagnostics) {
	switch s.Kind {
	case "emit":
		*diags = append(*diags, diag.Diagnostic{
			Code:    diag.CodeSemaPureFunctionEmit,
			Message: fmt.Sprintf("pure function '%s' must not emit events", fnName),
			Span:    defaultSpan(filename),
		})
	case "set":
		// Check if the set target is a storage variable (write).
		if s.Target != nil && isStorageExpr(s.Target, *localNames, storageSlotNames) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaPureFunctionStorageWrite,
				Message: fmt.Sprintf("pure function '%s' must not write to storage", fnName),
				Span:    defaultSpan(filename),
			})
		}
		checkPureExpr(filename, fnName, *localNames, s.Target, storageSlotNames, false, diags)
		checkPureExpr(filename, fnName, *localNames, s.Expr, storageSlotNames, false, diags)
	case "let":
		checkPureExpr(filename, fnName, *localNames, s.Expr, storageSlotNames, false, diags)
		declareLocal(localNames, s.Name)
	case "let-tuple":
		checkPureExpr(filename, fnName, *localNames, s.Expr, storageSlotNames, false, diags)
		for _, n := range s.Names {
			declareLocal(localNames, n)
		}
	default:
		if s.Init != nil {
			checkPureStmt(filename, fnName, localNames, *s.Init, storageSlotNames, diags)
		}
		checkPureExpr(filename, fnName, *localNames, s.Expr, storageSlotNames, false, diags)
		checkPureExpr(filename, fnName, *localNames, s.Target, storageSlotNames, false, diags)
		checkPureExpr(filename, fnName, *localNames, s.Cond, storageSlotNames, false, diags)
		checkPureExpr(filename, fnName, *localNames, s.Post, storageSlotNames, false, diags)
		for _, sub := range s.Then {
			checkPureStmt(filename, fnName, localNames, sub, storageSlotNames, diags)
		}
		for _, sub := range s.Else {
			checkPureStmt(filename, fnName, localNames, sub, storageSlotNames, diags)
		}
		for _, sub := range s.Body {
			checkPureStmt(filename, fnName, localNames, sub, storageSlotNames, diags)
		}
	}
}

// checkPureExpr recursively checks that e obeys pure semantics.
// targetContext=true means we are inspecting an assignment target (set/assign LHS);
// in that context a storage ident is a write (already reported by checkPureStmt).
func checkPureExpr(filename, fnName string, localNames map[string]bool, e *ast.Expr, storageSlotNames map[string]bool, targetContext bool, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	switch e.Kind {
	case "ident":
		name := strings.TrimSpace(e.Value)
		if !targetContext && isStorageSlot(name, localNames, storageSlotNames) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaPureFunctionStorageRead,
				Message: fmt.Sprintf("pure function '%s' must not read storage slot '%s'", fnName, name),
				Span:    defaultSpan(filename),
			})
		}
	case "member":
		if scope, _, ok := envMemberScopeKey(e); ok {
			if scope == "msg" || scope == "tx" || scope == "block" {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaPureFunctionEnvRead,
					Message: fmt.Sprintf("pure function '%s' must not read environment globals ('%s.*')", fnName, scope),
					Span:    defaultSpan(filename),
				})
			}
			// gas.left() is also forbidden in pure
			if scope == "gas" {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaPureFunctionEnvRead,
					Message: fmt.Sprintf("pure function '%s' must not call gas.left()", fnName),
					Span:    defaultSpan(filename),
				})
			}
			return // don't recurse into env member object
		}
		// If the object is a storage slot, that's a storage read.
		if e.Object != nil && isStorageExpr(e.Object, localNames, storageSlotNames) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaPureFunctionStorageRead,
				Message: fmt.Sprintf("pure function '%s' must not read storage slot via member access", fnName),
				Span:    defaultSpan(filename),
			})
		}
		checkPureExpr(filename, fnName, localNames, e.Object, storageSlotNames, false, diags)
	case "index":
		if isStorageExpr(e.Object, localNames, storageSlotNames) {
			if targetContext {
				// write detected; caller already reported
			} else {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaPureFunctionStorageRead,
					Message: fmt.Sprintf("pure function '%s' must not read from storage via index access", fnName),
					Span:    defaultSpan(filename),
				})
			}
		}
		checkPureExpr(filename, fnName, localNames, e.Object, storageSlotNames, targetContext, diags)
		checkPureExpr(filename, fnName, localNames, e.Index, storageSlotNames, false, diags)
	case "call":
		// Check callee for external call builtins.
		callee := stripParens(e.Callee)
		if callee != nil && callee.Kind == "ident" {
			name := strings.TrimSpace(callee.Value)
			if externalStateCallNames[name] || staticCallNames[name] {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaPureFunctionExternalCall,
					Message: fmt.Sprintf("pure function '%s' must not perform external calls ('%s')", fnName, name),
					Span:    defaultSpan(filename),
				})
			}
		}
		// Check gas.left() call: callee is member gas.left.
		if scope, key, ok := envMemberScopeKey(e.Callee); ok {
			_ = key
			if scope == "msg" || scope == "tx" || scope == "block" || scope == "gas" {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaPureFunctionEnvRead,
					Message: fmt.Sprintf("pure function '%s' must not read environment globals via call", fnName),
					Span:    defaultSpan(filename),
				})
				return
			}
		}
		checkPureExpr(filename, fnName, localNames, e.Callee, storageSlotNames, false, diags)
		for _, a := range e.Args {
			checkPureExpr(filename, fnName, localNames, a, storageSlotNames, false, diags)
		}
	case "binary", "assign":
		checkPureExpr(filename, fnName, localNames, e.Left, storageSlotNames, false, diags)
		checkPureExpr(filename, fnName, localNames, e.Right, storageSlotNames, false, diags)
	case "unary":
		checkPureExpr(filename, fnName, localNames, e.Right, storageSlotNames, false, diags)
	case "paren":
		checkPureExpr(filename, fnName, localNames, e.Left, storageSlotNames, targetContext, diags)
	}
}

// --------------------------------------------------------------------------
// view enforcement
// --------------------------------------------------------------------------

func checkViewStmts(filename, fnName string, params []ast.FieldDecl, stmts []ast.Statement, storageSlotNames map[string]bool, diags *diag.Diagnostics) {
	localNames := buildLocalNameSet(params)
	for _, s := range stmts {
		checkViewStmt(filename, fnName, &localNames, s, storageSlotNames, diags)
	}
}

func checkViewStmt(filename, fnName string, localNames *map[string]bool, s ast.Statement, storageSlotNames map[string]bool, diags *diag.Diagnostics) {
	switch s.Kind {
	case "emit":
		*diags = append(*diags, diag.Diagnostic{
			Code:    diag.CodeSemaViewFunctionEmit,
			Message: fmt.Sprintf("view function '%s' must not emit events", fnName),
			Span:    defaultSpan(filename),
		})
	case "set":
		if s.Target != nil && isStorageExpr(s.Target, *localNames, storageSlotNames) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaViewFunctionStorageWrite,
				Message: fmt.Sprintf("view function '%s' must not write to storage", fnName),
				Span:    defaultSpan(filename),
			})
		}
		checkViewExpr(filename, fnName, *localNames, s.Target, storageSlotNames, true, diags)
		checkViewExpr(filename, fnName, *localNames, s.Expr, storageSlotNames, false, diags)
	case "let":
		checkViewExpr(filename, fnName, *localNames, s.Expr, storageSlotNames, false, diags)
		declareLocal(localNames, s.Name)
	case "let-tuple":
		checkViewExpr(filename, fnName, *localNames, s.Expr, storageSlotNames, false, diags)
		for _, n := range s.Names {
			declareLocal(localNames, n)
		}
	default:
		if s.Init != nil {
			checkViewStmt(filename, fnName, localNames, *s.Init, storageSlotNames, diags)
		}
		checkViewExpr(filename, fnName, *localNames, s.Expr, storageSlotNames, false, diags)
		checkViewExpr(filename, fnName, *localNames, s.Target, storageSlotNames, false, diags)
		checkViewExpr(filename, fnName, *localNames, s.Cond, storageSlotNames, false, diags)
		checkViewExpr(filename, fnName, *localNames, s.Post, storageSlotNames, false, diags)
		for _, sub := range s.Then {
			checkViewStmt(filename, fnName, localNames, sub, storageSlotNames, diags)
		}
		for _, sub := range s.Else {
			checkViewStmt(filename, fnName, localNames, sub, storageSlotNames, diags)
		}
		for _, sub := range s.Body {
			checkViewStmt(filename, fnName, localNames, sub, storageSlotNames, diags)
		}
	}
}

func checkViewExpr(filename, fnName string, localNames map[string]bool, e *ast.Expr, storageSlotNames map[string]bool, targetContext bool, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	switch e.Kind {
	case "index":
		if targetContext && isStorageExpr(e.Object, localNames, storageSlotNames) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaViewFunctionStorageWrite,
				Message: fmt.Sprintf("view function '%s' must not write to storage via index access", fnName),
				Span:    defaultSpan(filename),
			})
		}
		checkViewExpr(filename, fnName, localNames, e.Object, storageSlotNames, targetContext, diags)
		checkViewExpr(filename, fnName, localNames, e.Index, storageSlotNames, false, diags)
	case "call":
		callee := stripParens(e.Callee)
		if callee != nil && callee.Kind == "ident" {
			name := strings.TrimSpace(callee.Value)
			if externalStateCallNames[name] {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaViewFunctionStateCall,
					Message: fmt.Sprintf("view function '%s' must not perform state-changing calls ('%s')", fnName, name),
					Span:    defaultSpan(filename),
				})
			}
		}
		checkViewExpr(filename, fnName, localNames, e.Callee, storageSlotNames, false, diags)
		for _, a := range e.Args {
			checkViewExpr(filename, fnName, localNames, a, storageSlotNames, false, diags)
		}
	case "member":
		if _, _, ok := envMemberScopeKey(e); ok {
			return // env reads are OK in view
		}
		checkViewExpr(filename, fnName, localNames, e.Object, storageSlotNames, false, diags)
	case "binary", "assign":
		checkViewExpr(filename, fnName, localNames, e.Left, storageSlotNames, false, diags)
		checkViewExpr(filename, fnName, localNames, e.Right, storageSlotNames, false, diags)
	case "unary":
		checkViewExpr(filename, fnName, localNames, e.Right, storageSlotNames, false, diags)
	case "paren":
		checkViewExpr(filename, fnName, localNames, e.Left, storageSlotNames, targetContext, diags)
	}
}

// --------------------------------------------------------------------------
// non-payable msg.value check
// --------------------------------------------------------------------------

// checkMsgValueInStmts reports an error when a non-payable function accesses msg.value.
func checkMsgValueInStmts(filename, fnName string, params []ast.FieldDecl, stmts []ast.Statement, diags *diag.Diagnostics) {
	localNames := buildLocalNameSet(params)
	for _, s := range stmts {
		checkMsgValueStmt(filename, fnName, &localNames, s, diags)
	}
}

func checkMsgValueStmt(filename, fnName string, localNames *map[string]bool, s ast.Statement, diags *diag.Diagnostics) {
	switch s.Kind {
	case "let":
		checkMsgValueExpr(filename, fnName, s.Expr, diags)
		declareLocal(localNames, s.Name)
	case "let-tuple":
		checkMsgValueExpr(filename, fnName, s.Expr, diags)
		for _, n := range s.Names {
			declareLocal(localNames, n)
		}
	default:
		if s.Init != nil {
			checkMsgValueStmt(filename, fnName, localNames, *s.Init, diags)
		}
		checkMsgValueExpr(filename, fnName, s.Expr, diags)
		checkMsgValueExpr(filename, fnName, s.Target, diags)
		checkMsgValueExpr(filename, fnName, s.Cond, diags)
		checkMsgValueExpr(filename, fnName, s.Post, diags)
		for _, sub := range s.Then {
			checkMsgValueStmt(filename, fnName, localNames, sub, diags)
		}
		for _, sub := range s.Else {
			checkMsgValueStmt(filename, fnName, localNames, sub, diags)
		}
		for _, sub := range s.Body {
			checkMsgValueStmt(filename, fnName, localNames, sub, diags)
		}
	}
}

func checkMsgValueExpr(filename, fnName string, e *ast.Expr, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	if scope, key, ok := envMemberScopeKey(e); ok {
		if scope == "msg" && key == "value" {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaNonPayableMsgValue,
				Message: fmt.Sprintf("non-payable function '%s' must not access 'msg.value'", fnName),
				Span:    defaultSpan(filename),
			})
		}
		return
	}
	switch e.Kind {
	case "call":
		checkMsgValueExpr(filename, fnName, e.Callee, diags)
		for _, a := range e.Args {
			checkMsgValueExpr(filename, fnName, a, diags)
		}
	case "member":
		checkMsgValueExpr(filename, fnName, e.Object, diags)
	case "index":
		checkMsgValueExpr(filename, fnName, e.Object, diags)
		checkMsgValueExpr(filename, fnName, e.Index, diags)
	case "binary", "assign":
		checkMsgValueExpr(filename, fnName, e.Left, diags)
		checkMsgValueExpr(filename, fnName, e.Right, diags)
	case "unary":
		checkMsgValueExpr(filename, fnName, e.Right, diags)
	case "paren":
		checkMsgValueExpr(filename, fnName, e.Left, diags)
	}
}

// --------------------------------------------------------------------------
// Pass 3: uninitialized variable reads
// --------------------------------------------------------------------------

// checkUninitializedReads emits TOL2060 when a local declared with `let x: T;`
// (no initializer) is read in an expression before being assigned on all paths.
// This is a conservative linear (non-path-sensitive) check: a variable is
// considered "initialized" once a `set x = ...` or `let x: T = expr` is seen
// in the same flat statement sequence. It does not track branches.
func checkUninitializedReads(filename, fnName string, params []ast.FieldDecl, stmts []ast.Statement, diags *diag.Diagnostics, knownStructs ...map[string][]ast.FieldDecl) {
	uninit := map[string]bool{}
	var structs map[string][]ast.FieldDecl
	if len(knownStructs) > 0 {
		structs = knownStructs[0]
	}
	// params are always initialized
	checkUninitStmts(filename, fnName, uninit, stmts, diags, structs)
}

func checkUninitStmts(filename, fnName string, uninit map[string]bool, stmts []ast.Statement, diags *diag.Diagnostics, knownStructs map[string][]ast.FieldDecl) {
	for _, s := range stmts {
		switch s.Kind {
		case "let":
			if s.Expr == nil {
				// `let x: T;` with no initializer.
				// Only mark as uninitialized when the type does NOT have a
				// language-defined default.
				// Default-initializable types (u*/i*, bool, address, string, bytes*,
				// and named struct types) are zero-initialized by the lowering pass.
				name := strings.TrimSpace(s.Name)
				if name != "" {
					localType := normalizeSelectorType(s.Type)
					_, isKnownStruct := knownStructs[localType]
					if !isDefaultInitializableTOLType(localType) && !isKnownStruct {
						uninit[name] = true
					}
				}
			} else {
				checkUninitExpr(filename, fnName, uninit, s.Expr, diags)
				// After initialization, the name is now assigned.
				name := strings.TrimSpace(s.Name)
				if name != "" {
					delete(uninit, name)
				}
			}
		case "set":
			// If the target is a plain ident, it becomes initialized.
			// First check the RHS expression for reads of uninitialized vars.
			checkUninitExpr(filename, fnName, uninit, s.Expr, diags)
			// Then mark the target as initialized (if it is a plain ident).
			if s.Target != nil {
				tgt := stripParens(s.Target)
				if tgt != nil && tgt.Kind == "ident" {
					name := strings.TrimSpace(tgt.Value)
					if name != "" {
						delete(uninit, name)
					}
				}
			}
			checkUninitExpr(filename, fnName, uninit, s.Target, diags)
		case "let-tuple":
			checkUninitExpr(filename, fnName, uninit, s.Expr, diags)
			for _, n := range s.Names {
				delete(uninit, strings.TrimSpace(n))
			}
		default:
			if s.Init != nil {
				checkUninitStmts(filename, fnName, uninit, []ast.Statement{*s.Init}, diags, knownStructs)
			}
			checkUninitExpr(filename, fnName, uninit, s.Expr, diags)
			checkUninitExpr(filename, fnName, uninit, s.Target, diags)
			checkUninitExpr(filename, fnName, uninit, s.Cond, diags)
			checkUninitExpr(filename, fnName, uninit, s.Post, diags)
			// Recurse into nested blocks (conservatively: don't propagate
			// writes in branches back to the outer scope).
			checkUninitStmts(filename, fnName, uninit, s.Then, diags, knownStructs)
			checkUninitStmts(filename, fnName, uninit, s.Else, diags, knownStructs)
			checkUninitStmts(filename, fnName, uninit, s.Body, diags, knownStructs)
		}
	}
}

func checkUninitExpr(filename, fnName string, uninit map[string]bool, e *ast.Expr, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	switch e.Kind {
	case "ident":
		name := strings.TrimSpace(e.Value)
		if uninit[name] {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaUninitializedRead,
				Message: fmt.Sprintf("function '%s': variable '%s' may be read before initialization", fnName, name),
				Span:    defaultSpan(filename),
			})
		}
	case "call":
		checkUninitExpr(filename, fnName, uninit, e.Callee, diags)
		for _, a := range e.Args {
			checkUninitExpr(filename, fnName, uninit, a, diags)
		}
	case "member":
		checkUninitExpr(filename, fnName, uninit, e.Object, diags)
	case "index":
		checkUninitExpr(filename, fnName, uninit, e.Object, diags)
		checkUninitExpr(filename, fnName, uninit, e.Index, diags)
	case "binary", "assign":
		checkUninitExpr(filename, fnName, uninit, e.Left, diags)
		checkUninitExpr(filename, fnName, uninit, e.Right, diags)
	case "unary":
		checkUninitExpr(filename, fnName, uninit, e.Right, diags)
	case "paren":
		checkUninitExpr(filename, fnName, uninit, e.Left, diags)
	}
}

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

// buildLocalNameSet returns a set containing all param names.
func buildLocalNameSet(params []ast.FieldDecl) map[string]bool {
	m := map[string]bool{}
	for _, p := range params {
		n := strings.TrimSpace(p.Name)
		if n != "" {
			m[n] = true
		}
	}
	return m
}

// declareLocal adds name to the local name set.
func declareLocal(localNames *map[string]bool, name string) {
	n := strings.TrimSpace(name)
	if n != "" {
		(*localNames)[n] = true
	}
}

// isStorageSlot returns true if name refers to a storage slot (not a local).
func isStorageSlot(name string, localNames map[string]bool, storageSlotNames map[string]bool) bool {
	if name == "" {
		return false
	}
	if localNames[name] {
		return false
	}
	return storageSlotNames[name]
}

// isStorageExpr returns true if e (after stripping parens) is a storage
// slot reference or an index/member chain rooted at a storage slot name.
func isStorageExpr(e *ast.Expr, localNames map[string]bool, storageSlotNames map[string]bool) bool {
	root := stripParens(e)
	if root == nil {
		return false
	}
	switch root.Kind {
	case "ident":
		return isStorageSlot(strings.TrimSpace(root.Value), localNames, storageSlotNames)
	case "index":
		return isStorageExpr(root.Object, localNames, storageSlotNames)
	case "member":
		return isStorageExpr(root.Object, localNames, storageSlotNames)
	default:
		return false
	}
}

// ─── Doc-effects validation (P5) ────────────────────────────────────────────

// InferredEffects holds the effects actually observed in a function's body.
type InferredEffects struct {
	Reads  []string   // canonical storage slot refs read
	Writes []string   // canonical storage slot refs written
	Emits  []string   // event names emitted
	Calls  []CallSite // external call sites
}

// CallSite is a single inferred external call site.
type CallSite struct {
	Selector string // 4-byte hex, if statically known; "" = unknown
	Target   string // address expression (informational; may be "dynamic")
}

// CoverageResult is the outcome of callRefCovers.
type CoverageResult int

const (
	coverNotCovered CoverageResult = iota
	coverCovered                   // normal match
	coverWildcard                  // wildcard match — marks non_composable
)

// callRefCovers returns whether a declared CallRef covers an inferred CallSite.
func callRefCovers(declared ast.CallRef, site CallSite) CoverageResult {
	if declared.Wildcard {
		return coverWildcard
	}
	// If declared has a selector, site selector must match.
	if declared.Selector != "" {
		if site.Selector == "" {
			return coverNotCovered // declared specific, site unknown
		}
		if declared.Selector != site.Selector {
			return coverNotCovered
		}
	}
	// No selector restriction (or selectors match).
	return coverCovered
}

// inferEffectsFromBody performs a shallow walk of the function body AST to
// collect storage reads/writes, emit events, and external calls.
// This is a best-effort inference; it does not resolve dynamic expressions.
func inferEffectsFromBody(body []ast.Statement, storageSlots map[string]bool, params map[string]bool) InferredEffects {
	var inf InferredEffects
	for _, stmt := range body {
		inferFromStmt(stmt, storageSlots, params, &inf)
	}
	// Deduplicate.
	inf.Reads = deduplicateStrings(inf.Reads)
	inf.Writes = deduplicateStrings(inf.Writes)
	inf.Emits = deduplicateStrings(inf.Emits)
	return inf
}

func inferFromStmt(s ast.Statement, slots map[string]bool, params map[string]bool, inf *InferredEffects) {
	switch s.Kind {
	case "set":
		// set storage.slot[key] = ... writes storage slot
		if s.Target != nil {
			if ref := storageRefFromExpr(s.Target, slots, params); ref != "" {
				inf.Writes = append(inf.Writes, ref)
			}
		}
		inferFromExpr(s.Expr, slots, params, inf)
	case "let", "let-tuple":
		inferFromExpr(s.Expr, slots, params, inf)
	case "return":
		inferFromExpr(s.Expr, slots, params, inf)
	case "expr":
		inferFromExpr(s.Expr, slots, params, inf)
	case "emit":
		if s.Name != "" {
			inf.Emits = append(inf.Emits, s.Name)
		} else if s.Text != "" {
			inf.Emits = append(inf.Emits, s.Text)
		}
	case "if":
		inferFromExpr(s.Cond, slots, params, inf)
		for _, s2 := range s.Then {
			inferFromStmt(s2, slots, params, inf)
		}
		for _, s2 := range s.Else {
			inferFromStmt(s2, slots, params, inf)
		}
	case "while", "dowhile":
		inferFromExpr(s.Cond, slots, params, inf)
		for _, s2 := range s.Body {
			inferFromStmt(s2, slots, params, inf)
		}
	case "for":
		if s.Init != nil {
			inferFromStmt(*s.Init, slots, params, inf)
		}
		inferFromExpr(s.Cond, slots, params, inf)
		inferFromExpr(s.Post, slots, params, inf)
		for _, s2 := range s.Body {
			inferFromStmt(s2, slots, params, inf)
		}
	case "block", "unchecked":
		for _, s2 := range s.Body {
			inferFromStmt(s2, slots, params, inf)
		}
	case "require", "assert", "revert":
		inferFromExpr(s.Expr, slots, params, inf)
	}
}

func inferFromExpr(e *ast.Expr, slots map[string]bool, params map[string]bool, inf *InferredEffects) {
	if e == nil {
		return
	}
	switch e.Kind {
	case "index":
		// If base is a storage slot, this is a read.
		if ref := storageRefFromExpr(e, slots, params); ref != "" {
			inf.Reads = append(inf.Reads, ref)
		} else {
			inferFromExpr(e.Object, slots, params, inf)
			inferFromExpr(e.Index, slots, params, inf)
		}
	case "member":
		if ref := storageRefFromExpr(e, slots, params); ref != "" {
			inf.Reads = append(inf.Reads, ref)
		} else {
			inferFromExpr(e.Object, slots, params, inf)
		}
	case "ident":
		// Direct storage slot read
		name := strings.TrimSpace(e.Value)
		if slots[name] {
			inf.Reads = append(inf.Reads, "storage."+name)
		}
	case "call":
		// Detect external calls via call()/create()/transfer() builtins.
		callee := stripParens(e.Callee)
		if callee != nil && callee.Kind == "ident" {
			name := strings.TrimSpace(callee.Value)
			switch name {
			case "call", "delegatecall", "staticcall", "create", "create2", "createx", "create2x", "transfer", "send":
				inf.Calls = append(inf.Calls, CallSite{Target: "dynamic"})
			}
		}
		inferFromExpr(e.Callee, slots, params, inf)
		for _, arg := range e.Args {
			inferFromExpr(arg, slots, params, inf)
		}
	case "binary", "assign":
		inferFromExpr(e.Left, slots, params, inf)
		inferFromExpr(e.Right, slots, params, inf)
	case "unary":
		inferFromExpr(e.Right, slots, params, inf)
	case "paren":
		inferFromExpr(e.Left, slots, params, inf)
	}
}

// storageRefFromExpr tries to derive a canonical storage ref string from an expression.
// Returns "" if the expression is not a recognized storage access pattern.
//
// For nested mappings like allowances[from][spender], the returned ref uses
// comma-separated keys: storage.allowances[from,spender] (per TOL_EFFECTS spec §3.4).
func storageRefFromExpr(e *ast.Expr, slots map[string]bool, params map[string]bool) string {
	if e == nil {
		return ""
	}
	switch e.Kind {
	case "ident":
		name := strings.TrimSpace(e.Value)
		if slots[name] {
			return "storage." + name
		}
	case "index":
		base := storageRefFromExpr(e.Object, slots, params)
		if base != "" {
			keyRef := resolveKeyRef(e.Index, params)
			// If base already ends with ']' (i.e. the object is itself an
			// indexed storage ref), merge the keys with a comma rather than
			// appending a new bracket pair.  This converts the chained form
			//   storage.allowances[from][spender]
			// into the spec-canonical comma-separated form
			//   storage.allowances[from,spender]
			if strings.HasSuffix(base, "]") {
				return base[:len(base)-1] + "," + keyRef + "]"
			}
			return base + "[" + keyRef + "]"
		}
	case "member":
		if e.Object != nil && e.Object.Kind == "ident" {
			if strings.TrimSpace(e.Object.Value) == "storage" {
				field := strings.TrimSpace(e.Member)
				if field != "" {
					return "storage." + field
				}
			}
		}
	}
	return ""
}

func deduplicateStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := ss[:0]
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// resolveKeyRef maps an index-key expression to a canonical key name.
//
// Recognized forms:
//   - msg.sender           → "caller"
//   - this                 → "this"
//   - address(this)        → "this"
//   - <param> (in params)  → param name (as-is)
//   - anything else        → "?" (dynamic — only covered by [*] in @effects)
func resolveKeyRef(e *ast.Expr, params map[string]bool) string {
	if e == nil {
		return "?"
	}
	e = stripParens(e)
	if e == nil {
		return "?"
	}
	switch e.Kind {
	case "ident":
		name := strings.TrimSpace(e.Value)
		if name == "this" {
			return "this"
		}
		if params != nil && params[name] {
			return name
		}
		return "?"
	case "member":
		if e.Object != nil && e.Object.Kind == "ident" &&
			strings.TrimSpace(e.Object.Value) == "msg" &&
			strings.TrimSpace(e.Member) == "sender" {
			return "caller"
		}
		return "?"
	case "call":
		// address(this) → "this"
		if e.Callee != nil && e.Callee.Kind == "ident" &&
			strings.TrimSpace(e.Callee.Value) == "agent" &&
			len(e.Args) == 1 {
			if resolveKeyRef(e.Args[0], params) == "this" {
				return "this"
			}
		}
		return "?"
	default:
		return "?"
	}
}

// buildParamSet returns a set of function parameter names.
func buildParamSet(params []ast.FieldDecl) map[string]bool {
	m := make(map[string]bool, len(params))
	for _, p := range params {
		if name := strings.TrimSpace(p.Name); name != "" {
			m[name] = true
		}
	}
	return m
}

// storageRefMsg returns a human-readable TOL2200 message, with a hint for [?] (dynamic) keys.
func storageRefMsg(fnName, ref, direction string) string {
	if strings.HasSuffix(ref, "[?]") {
		slotBase := ref[:strings.LastIndex(ref, "[")]
		return fmt.Sprintf(
			"undeclared effect in function '%s': '%s' accesses storage with a dynamic key; "+
				"add '%s[*]' to @effects %s, or use a recognized key expression "+
				"(parameter name, msg.sender → [caller], this → [this])",
			fnName, ref, slotBase, direction)
	}
	return fmt.Sprintf(
		"undeclared effect in function '%s': storage ref '%s' is %s but not in @effects %s",
		fnName, ref, direction, direction)
}

// checkDeclaredEffects validates declared @effects against inferred effects for a function.
// Emits TOL2200/2204/2205 diagnostics as appropriate.
func checkDeclaredEffects(filename string, fn ast.FunctionDecl, storageSlots map[string]bool, diags *diag.Diagnostics) {
	if fn.Doc == nil || fn.Doc.Effects == nil {
		return
	}
	decl := fn.Doc.Effects
	params := buildParamSet(fn.Params)
	inf := inferEffectsFromBody(fn.Body, storageSlots, params)

	span := diag.Span{File: filename}

	// Check Reads.
	for _, r := range inf.Reads {
		if !isRefCovered(r, decl.Reads) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeEffectUndeclared,
				Message: storageRefMsg(fn.Name, r, "reads"),
				Span:    span,
			})
		}
	}

	// Check Writes.
	for _, w := range inf.Writes {
		if !isRefCovered(w, decl.Writes) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeEffectUndeclared,
				Message: storageRefMsg(fn.Name, w, "writes"),
				Span:    span,
			})
		}
	}

	// Check Emits.
	for _, ev := range inf.Emits {
		if !stringSliceContains(decl.Emits, ev) {
			*diags = append(*diags, diag.Diagnostic{
				Code: diag.CodeEffectUndeclared,
				Message: fmt.Sprintf(
					"undeclared effect in function '%s': event '%s' is emitted but not in @effects emits",
					fn.Name, ev),
				Span: span,
			})
		}
	}

	// Check Calls (three states: nil=not declared, empty=declared empty, non-empty=declared).
	if decl.Calls != nil {
		if len(decl.Calls) == 0 {
			// Declared empty: no calls allowed.
			if len(inf.Calls) > 0 {
				*diags = append(*diags, diag.Diagnostic{
					Code: diag.CodeEffectEmptyCalls,
					Message: fmt.Sprintf(
						"function '%s' declares '@effects calls: []' but an external call was found in the implementation",
						fn.Name),
					Span: span,
				})
			}
		} else {
			// Non-empty: each inferred call site must be covered by a declared CallRef.
			nonComposable := false
			for _, site := range inf.Calls {
				covered := false
				for _, ref := range decl.Calls {
					result := callRefCovers(ref, site)
					if result == coverCovered {
						covered = true
						break
					}
					if result == coverWildcard {
						covered = true
						nonComposable = true
						break
					}
				}
				if !covered {
					*diags = append(*diags, diag.Diagnostic{
						Code: diag.CodeEffectUndeclared,
						Message: fmt.Sprintf(
							"function '%s': external call site (selector=%q) not covered by any declared @effects calls ref",
							fn.Name, site.Selector),
						Span: span,
					})
				}
			}
			_ = nonComposable // used in P6 ABI output

			// TOL2205: declared selector with no matching inferred call site.
			for _, ref := range decl.Calls {
				if ref.Selector == "" || ref.Wildcard {
					continue
				}
				found := false
				for _, site := range inf.Calls {
					if site.Selector == ref.Selector {
						found = true
						break
					}
				}
				if !found {
					*diags = append(*diags, diag.Diagnostic{
						Code: diag.CodeEffectSelectorNoSite,
						Message: fmt.Sprintf(
							"function '%s': @effects calls declares selector %s but no matching call site found in IR",
							fn.Name, ref.Selector),
						Span: span,
					})
				}
			}
		}
	}
}

// isRefCovered returns true if ref is covered by any entry in declared.
// Wildcard '*' in declared covers anything with the same slot prefix.
func isRefCovered(ref string, declared []string) bool {
	for _, d := range declared {
		if d == ref {
			return true
		}
		// Wildcard: "storage.slot[*]" covers "storage.slot[anything]"
		if strings.HasSuffix(d, "[*]") {
			prefix := d[:len(d)-3] // "storage.slot"
			if strings.HasPrefix(ref, prefix+"[") {
				return true
			}
		}
	}
	return false
}

func stringSliceContains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// Gas cost constants for static estimation (TOLANG v0.2, GTOS VM).
const (
	gasCostSStore   = uint64(20000)
	gasCostSLoad    = uint64(2100)
	gasCostLogBase  = uint64(375)
	gasCostLogTopic = uint64(375)
	gasCostInstr    = uint64(1)
)

// checkGasUpperBound validates @gas upper against a simple static cost estimate.
// Emits TOL2201 or TOL2202 diagnostics.
// This is a conservative, best-effort check.
//
// Two forms of @gas upper are handled:
//
//  1. Concrete: `@gas upper: 12345` — gas.Upper > 0, gas.Expr == "".
//     The body is estimated with bounds-aware loop unrolling. TOL2201 if the
//     body cannot be bounded; TOL2202 if declared < estimated.
//
//  2. Parametric: `@gas upper: 8200 + positions_len * 420` — gas.Expr != "".
//     The expression is evaluated by substituting @bounds variable values and
//     CallRef.max_gas fields. TOL2201 is emitted if any identifier in the
//     expression cannot be resolved. No numerical comparison against the body
//     estimate is performed: the parametric expression IS the declared bound.
func checkGasUpperBound(filename string, fn ast.FunctionDecl, storageSlots map[string]bool, diags *diag.Diagnostics) {
	if fn.Doc == nil || fn.Doc.Gas == nil {
		return
	}
	gas := fn.Doc.Gas
	span := diag.Span{File: filename}

	if gas.Expr != "" {
		// Parametric expression path: evaluate the expression by substituting
		// @bounds variable values and CallRef.max_gas fields. Emit TOL2201 if
		// any identifier cannot be resolved (function is effectively unbounded).
		// No comparison against the body gas estimate is performed — the
		// parametric expression is the developer-declared upper bound.
		var calls []ast.CallRef
		if fn.Doc.Effects != nil {
			calls = fn.Doc.Effects.Calls
		}
		_, ok := evalGasExpr(gas.Expr, fn.Doc.Bounds, calls)
		if !ok {
			*diags = append(*diags, diag.Diagnostic{
				Code: diag.CodeEffectGasUnbounded,
				Message: fmt.Sprintf(
					"cannot verify @gas upper in function '%s': parametric expression contains an unresolved identifier; declare all bound variables with @bounds or provide CallRef.max_gas values",
					fn.Name),
				Span: span,
			})
		}
		return
	}

	// Concrete path: declared is a plain integer upper bound.
	declared := gas.Upper

	// Estimate gas from body, threading @bounds through for loop unrolling.
	estimated := estimateGas(fn.Body, storageSlots, fn.Doc.Bounds)

	if estimated == ^uint64(0) {
		// Unbounded (loop without known bound).
		*diags = append(*diags, diag.Diagnostic{
			Code: diag.CodeEffectGasUnbounded,
			Message: fmt.Sprintf(
				"cannot verify @gas upper in function '%s': function contains an unbounded loop or dynamic iteration; declare loop bounds with @bounds or remove @gas upper",
				fn.Name),
			Span: span,
		})
		return
	}

	if declared < estimated {
		*diags = append(*diags, diag.Diagnostic{
			Code: diag.CodeEffectGasTooLow,
			Message: fmt.Sprintf(
				"@gas upper too low in function '%s': declared %d, inferred conservative upper bound: %d; raise @gas upper to at least %d",
				fn.Name, declared, estimated, estimated),
			Span: span,
		})
	}
}

// estimateGas returns a conservative gas estimate for a function body.
// Returns ^uint64(0) (MaxUint64) if any unbounded loop is detected.
// bounds may be nil; when non-nil it is used to bound for/while loops.
func estimateGas(body []ast.Statement, slots map[string]bool, bounds *ast.BoundsDecl) uint64 {
	total := uint64(0)
	for _, stmt := range body {
		cost := estimateStmtGas(stmt, slots, bounds)
		if cost == ^uint64(0) {
			return ^uint64(0)
		}
		total += cost
	}
	return total
}

// loopVarFromStmt extracts the loop variable name from a for-loop Init
// statement, which is expected to be a "let" statement.
// Returns "" if no loop variable can be determined.
func loopVarFromStmt(init *ast.Statement) string {
	if init == nil {
		return ""
	}
	if init.Kind == "let" {
		return strings.TrimSpace(init.Name)
	}
	return ""
}

// boundsMaxIter returns the maximum number of iterations for a loop variable
// given the @bounds constraints.  It returns 0, false when no matching
// constraint is found, and N, true when a matching constraint is found.
// For Op "<"  : effective max iterations = Value
// For Op "<=" : effective max iterations = Value + 1
// For Op "==" : effective max iterations = Value
// When multiple constraints match the same ident, the minimum bound is used.
func boundsMaxIter(varName string, bounds *ast.BoundsDecl) (uint64, bool) {
	if bounds == nil || varName == "" {
		return 0, false
	}
	found := false
	var minVal uint64
	for _, c := range bounds.Constraints {
		if strings.TrimSpace(c.Ident) != varName {
			continue
		}
		var iters uint64
		switch c.Op {
		case "<":
			iters = c.Value
		case "<=":
			iters = c.Value + 1
		case "==":
			iters = c.Value
		default:
			continue
		}
		if !found || iters < minVal {
			minVal = iters
			found = true
		}
	}
	return minVal, found
}

func estimateStmtGas(s ast.Statement, slots map[string]bool, bounds *ast.BoundsDecl) uint64 {
	switch s.Kind {
	case "set":
		base := gasCostInstr
		if s.Target != nil && storageRefFromExpr(s.Target, slots, nil) != "" {
			base = gasCostSStore
		}
		return base + estimateExprGas(s.Expr, slots)
	case "emit":
		// Conservative: assume 1 topic.
		return gasCostLogBase + gasCostLogTopic
	case "if":
		thenCost := estimateGas(s.Then, slots, bounds)
		elseCost := estimateGas(s.Else, slots, bounds)
		// Conservative: take max.
		if thenCost > elseCost {
			return thenCost
		}
		return elseCost
	case "while", "dowhile":
		// Attempt to infer the iteration count from @bounds via the loop
		// condition's left-hand-side variable (e.g. `while i <= 99`).
		// If a bound is found, the loop cost is maxIter × bodyCost.
		// If no bound is found, treat the loop as unbounded.
		loopVar := loopVarFromCond(s.Cond)
		if loopVar != "" && bounds != nil {
			if maxIter, ok := boundsMaxIter(loopVar, bounds); ok {
				bodyCost := estimateGas(s.Body, slots, bounds)
				if bodyCost != ^uint64(0) {
					// Overflow-safe multiplication: clamp to MaxUint64.
					if maxIter > 0 && bodyCost > math.MaxUint64/maxIter {
						return ^uint64(0)
					}
					return maxIter * bodyCost
				}
			}
		}
		// No bound found — treat as unbounded.
		return ^uint64(0)
	case "for":
		// Attempt to infer the iteration count from the condition's RHS
		// (e.g. `for let i = 0; i < n; ...` with `@bounds n <= 10`).
		// Supports both identifier bounds (looked up in @bounds) and numeric
		// literal upper bounds in the condition directly.
		maxIter, ok := maxIterFromForCond(s.Cond, bounds)
		if ok {
			bodyCost := estimateGas(s.Body, slots, bounds)
			if bodyCost != ^uint64(0) {
				// Overflow-safe multiplication: clamp to MaxUint64.
				if maxIter > 0 && bodyCost > math.MaxUint64/maxIter {
					return ^uint64(0)
				}
				return maxIter * bodyCost
			}
		}
		// No bound found — treat as unbounded.
		return ^uint64(0)
	case "block", "unchecked":
		return estimateGas(s.Body, slots, bounds)
	case "let", "let-tuple":
		return gasCostInstr + estimateExprGas(s.Expr, slots)
	case "return":
		return gasCostInstr + estimateExprGas(s.Expr, slots)
	case "expr":
		return gasCostInstr + estimateExprGas(s.Expr, slots)
	default:
		return gasCostInstr
	}
}

func estimateExprGas(e *ast.Expr, slots map[string]bool) uint64 {
	if e == nil {
		return 0
	}
	switch e.Kind {
	case "index":
		if storageRefFromExpr(e, slots, nil) != "" {
			return gasCostSLoad
		}
		return gasCostInstr
	case "member":
		if storageRefFromExpr(e, slots, nil) != "" {
			return gasCostSLoad
		}
		return gasCostInstr
	case "ident":
		name := strings.TrimSpace(e.Value)
		if slots[name] {
			return gasCostSLoad
		}
		return gasCostInstr
	case "binary", "assign":
		return gasCostInstr + estimateExprGas(e.Left, slots) + estimateExprGas(e.Right, slots)
	default:
		return gasCostInstr
	}
}

// ─── Loop-bound helpers ──────────────────────────────────────────────────────

// loopVarFromCond extracts the left-hand-side identifier from a simple binary
// loop condition such as `i < N`, `i <= N`, or `i != N`.
// Returns "" if the condition is nil or not a simple recognised form.
func loopVarFromCond(cond *ast.Expr) string {
	if cond == nil {
		return ""
	}
	if cond.Kind != "binary" {
		return ""
	}
	switch cond.Op {
	case "<", "<=", "!=":
		// OK — recognised loop-termination operators.
	default:
		return ""
	}
	left := cond.Left
	if left == nil {
		return ""
	}
	// Strip a single layer of parentheses.
	if left.Kind == "paren" && left.Left != nil {
		left = left.Left
	}
	if left.Kind != "ident" {
		return ""
	}
	return strings.TrimSpace(left.Value)
}

// maxIterFromForCond infers the maximum iteration count for a for-loop from its
// condition expression.  It inspects the right-hand side of the condition:
//   - If the RHS is an identifier, the function looks it up in @bounds.
//   - If the RHS is a numeric literal, the literal value is used directly.
//
// Returns (0, false) if the iteration count cannot be determined.
func maxIterFromForCond(cond *ast.Expr, bounds *ast.BoundsDecl) (uint64, bool) {
	if cond == nil || cond.Kind != "binary" {
		return 0, false
	}
	switch cond.Op {
	case "<", "<=", "!=":
		// OK — recognised loop-termination operators.
	default:
		return 0, false
	}
	rhs := cond.Right
	if rhs == nil {
		return 0, false
	}
	// Strip parens.
	if rhs.Kind == "paren" && rhs.Left != nil {
		rhs = rhs.Left
	}
	switch rhs.Kind {
	case "ident":
		// Look up the RHS variable in @bounds.
		name := strings.TrimSpace(rhs.Value)
		return boundsMaxIter(name, bounds)
	case "number":
		// Literal upper bound in the condition itself (e.g. `i < 100`).
		v, err := strconv.ParseUint(strings.TrimSpace(rhs.Value), 10, 64)
		if err != nil {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

// ─── Parametric @gas expression evaluator ───────────────────────────────────

// evalGasExpr evaluates a parametric @gas upper expression such as
// "8200 + positions_len * 420 + OracleCap.max_gas" by:
//
//  1. Substituting identifiers from the @bounds declaration.
//  2. Substituting "<Cap>.max_gas" references from the declared CallRefs.
//  3. Evaluating the resulting arithmetic expression (+, *, parentheses).
//
// Returns (value, true) on success, or (0, false) if any identifier cannot be
// resolved (indicating the expression is unbounded or malformed).
func evalGasExpr(expr string, bounds *ast.BoundsDecl, calls []ast.CallRef) (uint64, bool) {
	tokens, ok := tokenizeGasExpr(expr)
	if !ok {
		return 0, false
	}
	p := &gasExprParser{tokens: tokens, pos: 0}
	val, ok := p.parseAddExpr(bounds, calls)
	if !ok {
		return 0, false
	}
	// Ensure all tokens were consumed.
	if p.pos < len(p.tokens) {
		return 0, false
	}
	return val, true
}

// gasToken is a single token in a @gas expression.
type gasToken struct {
	kind string // "int", "ident", "plus", "star", "lparen", "rparen"
	text string
}

// tokenizeGasExpr splits a gas expression string into tokens.
// Identifiers may contain dots (e.g. "OracleCap.max_gas").
func tokenizeGasExpr(expr string) ([]gasToken, bool) {
	var tokens []gasToken
	i := 0
	runes := []rune(expr)
	n := len(runes)
	for i < n {
		ch := runes[i]
		if unicode.IsSpace(ch) {
			i++
			continue
		}
		switch ch {
		case '+':
			tokens = append(tokens, gasToken{kind: "plus", text: "+"})
			i++
		case '*':
			tokens = append(tokens, gasToken{kind: "star", text: "*"})
			i++
		case '(':
			tokens = append(tokens, gasToken{kind: "lparen", text: "("})
			i++
		case ')':
			tokens = append(tokens, gasToken{kind: "rparen", text: ")"})
			i++
		default:
			if unicode.IsDigit(ch) {
				j := i
				for j < n && unicode.IsDigit(runes[j]) {
					j++
				}
				tokens = append(tokens, gasToken{kind: "int", text: string(runes[i:j])})
				i = j
			} else if unicode.IsLetter(ch) || ch == '_' {
				// Identifiers may contain letters, digits, underscores, and dots.
				j := i
				for j < n && (unicode.IsLetter(runes[j]) || unicode.IsDigit(runes[j]) || runes[j] == '_' || runes[j] == '.') {
					j++
				}
				tokens = append(tokens, gasToken{kind: "ident", text: string(runes[i:j])})
				i = j
			} else {
				// Unknown character — return failure.
				return nil, false
			}
		}
	}
	return tokens, true
}

// gasExprParser is a minimal recursive-descent parser for gas expressions.
// Grammar (with standard precedence):
//
//	addExpr  = mulExpr ( '+' mulExpr )*
//	mulExpr  = primary ( '*' primary )*
//	primary  = INT | IDENT | '(' addExpr ')'
type gasExprParser struct {
	tokens []gasToken
	pos    int
}

func (p *gasExprParser) peek() *gasToken {
	if p.pos >= len(p.tokens) {
		return nil
	}
	return &p.tokens[p.pos]
}

func (p *gasExprParser) consume() gasToken {
	t := p.tokens[p.pos]
	p.pos++
	return t
}

func (p *gasExprParser) parseAddExpr(bounds *ast.BoundsDecl, calls []ast.CallRef) (uint64, bool) {
	left, ok := p.parseMulExpr(bounds, calls)
	if !ok {
		return 0, false
	}
	for {
		t := p.peek()
		if t == nil || t.kind != "plus" {
			break
		}
		p.consume()
		right, ok := p.parseMulExpr(bounds, calls)
		if !ok {
			return 0, false
		}
		// Overflow-safe addition.
		if left > math.MaxUint64-right {
			left = math.MaxUint64
		} else {
			left += right
		}
	}
	return left, true
}

func (p *gasExprParser) parseMulExpr(bounds *ast.BoundsDecl, calls []ast.CallRef) (uint64, bool) {
	left, ok := p.parsePrimary(bounds, calls)
	if !ok {
		return 0, false
	}
	for {
		t := p.peek()
		if t == nil || t.kind != "star" {
			break
		}
		p.consume()
		right, ok := p.parsePrimary(bounds, calls)
		if !ok {
			return 0, false
		}
		// Overflow-safe multiplication.
		if left > 0 && right > math.MaxUint64/left {
			left = math.MaxUint64
		} else {
			left *= right
		}
	}
	return left, true
}

func (p *gasExprParser) parsePrimary(bounds *ast.BoundsDecl, calls []ast.CallRef) (uint64, bool) {
	t := p.peek()
	if t == nil {
		return 0, false
	}
	switch t.kind {
	case "int":
		p.consume()
		v, err := strconv.ParseUint(t.text, 10, 64)
		if err != nil {
			return 0, false
		}
		return v, true
	case "ident":
		p.consume()
		return resolveGasIdent(t.text, bounds, calls)
	case "lparen":
		p.consume() // consume '('
		val, ok := p.parseAddExpr(bounds, calls)
		if !ok {
			return 0, false
		}
		closing := p.peek()
		if closing == nil || closing.kind != "rparen" {
			return 0, false
		}
		p.consume() // consume ')'
		return val, true
	}
	return 0, false
}

// resolveGasIdent resolves an identifier token from a @gas expression.
//
// Resolution order:
//  1. Plain identifier (e.g. "n") — looked up in @bounds constraints.
//  2. Cap.max_gas pattern (e.g. "OracleCap.max_gas") — looked up in CallRefs
//     by matching the Cap field.
//
// Returns (0, false) if the identifier cannot be resolved.
func resolveGasIdent(name string, bounds *ast.BoundsDecl, calls []ast.CallRef) (uint64, bool) {
	// Check for the "<Cap>.max_gas" pattern.
	if idx := strings.LastIndex(name, ".max_gas"); idx > 0 {
		capName := name[:idx]
		for _, c := range calls {
			if c.Cap == capName {
				return c.MaxGas, true
			}
		}
		return 0, false
	}

	// Plain identifier — look up in @bounds.
	if bounds != nil {
		for _, bc := range bounds.Constraints {
			if bc.Ident == name {
				return bc.Value, true
			}
		}
	}
	return 0, false
}

// checkTotalCostBound validates @total_cost(max: N) against @gas upper and @pay amount.
// Emits TOL2209 (warning) if gas_upper * 10gwei + pay_amount > declared max.
func checkTotalCostBound(filename string, fn ast.FunctionDecl, diags *diag.Diagnostics) {
	if fn.Doc == nil || fn.Doc.TotalCostMax == "" {
		return
	}
	maxWei, err := strconv.ParseUint(fn.Doc.TotalCostMax, 10, 64)
	if err != nil {
		// Non-integer max; skip numeric check.
		return
	}
	span := diag.Span{File: filename}

	var gasUpper uint64
	if fn.Doc.Gas != nil && fn.Doc.Gas.Upper > 0 {
		gasUpper = fn.Doc.Gas.Upper
	}
	var payAmount uint64
	if fn.Doc.PayAmount != "" {
		if v, pErr := strconv.ParseUint(fn.Doc.PayAmount, 10, 64); pErr == nil {
			payAmount = v
		}
	}

	// gweiPerGas = 10_000_000_000 (10 gwei per gas unit)
	const checkGweiPerGas = uint64(10_000_000_000)
	gasCost := gasUpper * checkGweiPerGas
	total := gasCost + payAmount
	// Overflow guard: if overflow occurred, values are too large to compare.
	if gasCost/checkGweiPerGas != gasUpper {
		return
	}
	if total < gasCost {
		return
	}
	if total > maxWei {
		*diags = append(*diags, diag.Diagnostic{
			Code: diag.CodeEffectTotalCostExceeded,
			Message: fmt.Sprintf(
				"@total_cost(max: %d) in function '%s' is exceeded: gas_upper(%d) * 10gwei + pay(%d) = %d wei > %d",
				maxWei, fn.Name, gasUpper, payAmount, total, maxWei),
			Span:     span,
			Severity: diag.SeverityWarning,
		})
	}
}
