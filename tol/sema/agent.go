package sema

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/diag"
)

// checkAgentNativeDecls validates agent-native declarations in a contract:
//   - No duplicate capabilities (TOL2300)
//   - No duplicate purposes (TOL2301)
//   - Max 256 purposes (TOL2312)
//   - @requires references only declared capabilities (TOL2302)
//   - @verifiable only on pure or view functions (TOL2314)
//   - oracle<T>: T must be a value type (TOL2303)
//   - vote<T>: T must be numeric uint/int/bool (TOL2304)
//   - task<T>: T should be a struct type (TOL2305)
//   - @pay: amount must not be empty (TOL2308), recipient must not be empty (TOL2309)
//   - delegation.verify() only inside @delegated functions (TOL2310)
//   - escrow/release/slash only inside payable functions (TOL2311)
// moduleCaps are top-level capabilities declared outside any contract (shared across file).
func checkAgentNativeDecls(filename string, c *ast.ContractDecl, moduleCaps []ast.CapabilityDecl, diags *diag.Diagnostics, knownStructNames map[string]bool) {
	// Build capability name set for lookup: union of module-level + contract-level.
	capNames := make(map[string]bool, len(c.Capabilities)+len(moduleCaps))
	for _, cd := range moduleCaps {
		capNames[cd.Name] = true
	}
	for _, cd := range c.Capabilities {
		if capNames[cd.Name] {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeAgentCapabilityDup,
				Message: fmt.Sprintf("capability '%s' already declared in this contract", cd.Name),
				Span:    diag.Span{File: filename, Start: diag.Position{Line: cd.Line}},
			})
			continue
		}
		capNames[cd.Name] = true
	}

	// Build purpose name set; enforce max 256 ordinals.
	purNames := make(map[string]bool, len(c.Purposes))
	for i, pd := range c.Purposes {
		if purNames[pd.Name] {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeAgentPurposeDup,
				Message: fmt.Sprintf("purpose '%s' already declared in this contract", pd.Name),
				Span:    diag.Span{File: filename, Start: diag.Position{Line: pd.Line}},
			})
			continue
		}
		purNames[pd.Name] = true
		if i >= 256 {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeAgentPurposeOrdinalOverflow,
				Message: fmt.Sprintf("purpose '%s' exceeds max 256 purposes per contract (ordinal %d)", pd.Name, i),
				Span:    diag.Span{File: filename, Start: diag.Position{Line: pd.Line}},
			})
		}
	}

	// Check storage slot type parameters (TOL2303/2304/2305).
	if c.Storage != nil {
		for _, slot := range c.Storage.Slots {
			t := strings.TrimSpace(slot.Type)
			inner := extractAgentInnerType(t)
			switch {
			case strings.HasPrefix(t, "oracle<"):
				// TOL2303: inner type must be a value type (no mapping/array).
				if inner == "" || !isValueTOLType(inner) {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeAgentOracleInvalidType,
						Message: fmt.Sprintf("oracle<%s>: type parameter must be a value type (uint/int/bool/address/bytesN), got '%s'", inner, inner),
						Span:    defaultSpan(filename),
					})
				}
			case strings.HasPrefix(t, "vote<"):
				// TOL2304: inner type must be numeric (uint/int/bool).
				if inner == "" || !isNumericTOLType(inner) {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeAgentVoteInvalidType,
						Message: fmt.Sprintf("vote<%s>: type parameter must be a numeric type (uint/int/bool), got '%s'", inner, inner),
						Span:    defaultSpan(filename),
					})
				}
			case strings.HasPrefix(t, "task<"):
				// TOL2305: inner type should be a struct (warning-level: emit diagnostic but don't fail).
				if inner != "" && len(knownStructNames) > 0 && !knownStructNames[inner] {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeAgentTaskInvalidType,
						Message: fmt.Sprintf("task<%s>: type parameter '%s' is not a known struct type", inner, inner),
						Span:    defaultSpan(filename),
					})
				}
			}
		}
	}

	// Validate per-function annotations.
	for _, fn := range c.Functions {
		if fn.Doc == nil {
			// Still need to check body even without doc (for TOL2310/2311).
			isPayable := sliceContains(fn.Modifiers, "payable")
			checkAgentBodyCalls(filename, fn.Body, false, isPayable, diags)
			continue
		}
		// @requires: each named capability must be declared in this contract (TOL2302).
		for _, capName := range fn.Doc.RequiresCap {
			if !capNames[capName] {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeAgentRequiresUnknownCap,
					Message: fmt.Sprintf("@requires references undeclared capability '%s' (declare it with 'capability %s;')", capName, capName),
					Span:    defaultSpan(filename),
				})
			}
		}
		// @verifiable: function must be pure or view (TOL2314).
		if fn.Doc.Verifiable {
			isViewOrPure := sliceContains(fn.Modifiers, "pure") || sliceContains(fn.Modifiers, "view")
			if !isViewOrPure {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeAgentVerifiableNonPure,
					Message: fmt.Sprintf("@verifiable on function '%s' requires 'pure' or 'view' modifier", fn.Name),
					Span:    defaultSpan(filename),
				})
			}
		}
		// @pay: when declared, amount must be non-empty (TOL2308).
		// Recipient (TOL2309) is only required when using named-key form (not bare form).
		if fn.Doc.HasPay {
			if fn.Doc.PayAmount == "" {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeAgentPayBadAmount,
					Message: fmt.Sprintf("@pay on function '%s': amount= expression must not be empty", fn.Name),
					Span:    defaultSpan(filename),
				})
			}
			if !fn.Doc.PayIsBare && fn.Doc.PayRecipient == "" {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeAgentPayBadRecipient,
					Message: fmt.Sprintf("@pay on function '%s': recipient= expression must not be empty", fn.Name),
					Span:    defaultSpan(filename),
				})
			}
		}
		// Body walks for TOL2310/2311.
		isPayable := sliceContains(fn.Modifiers, "payable")
		checkAgentBodyCalls(filename, fn.Body, fn.Doc.Delegated, isPayable, diags)
	}
}

// validTaskTransitions is the set of allowed (from, to) state pairs for task<T>.
// States: None=0, Open=1, Accepted=2, Submitted=3, Approved=4, Rejected=5, Disputed=6, Cancelled=7.
var validTaskTransitions = map[[2]uint64]bool{
	{0, 1}: true, // None → Open      (task_post)
	{1, 2}: true, // Open → Accepted
	{2, 3}: true, // Accepted → Submitted
	{3, 4}: true, // Submitted → Approved
	{3, 5}: true, // Submitted → Rejected
	{3, 6}: true, // Submitted → Disputed
	{6, 4}: true, // Disputed → Approved
	{6, 5}: true, // Disputed → Rejected
	{1, 7}: true, // Open → Cancelled
	{2, 7}: true, // Accepted → Cancelled
	{3, 7}: true, // Submitted → Cancelled
	{6, 7}: true, // Disputed → Cancelled
}

// checkAgentBodyCalls walks statement bodies to find:
//   - agent(non-address-literal) casts (TOL2306)
//   - delegation.verify() outside @delegated functions (TOL2310)
//   - escrow/release/slash outside payable functions (TOL2311)
//   - __tol_task_transition with invalid literal from/to states (TOL2315)
func checkAgentBodyCalls(filename string, stmts []ast.Statement, isDelegated, isPayable bool, diags *diag.Diagnostics) {
	walkStmtExprs(stmts, func(e *ast.Expr) {
		if e == nil || e.Kind != "call" {
			return
		}
		callee := e.Callee
		for callee != nil && callee.Kind == "paren" {
			callee = callee.Left
		}
		if callee == nil {
			return
		}
		// Check delegation.verify() outside @delegated (TOL2310).
		if !isDelegated && callee.Kind == "member" {
			obj := callee.Object
			for obj != nil && obj.Kind == "paren" {
				obj = obj.Left
			}
			if obj != nil && obj.Kind == "ident" &&
				strings.TrimSpace(obj.Value) == "delegation" &&
				strings.TrimSpace(callee.Member) == "verify" {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeAgentDelegateVerifyOutside,
					Message: "delegation.verify() called outside a @delegated function",
					Span:    defaultSpan(filename),
				})
			}
		}
		if callee.Kind != "ident" {
			return
		}
		name := strings.TrimSpace(callee.Value)
		// Check escrow/release/slash arity: allow 2 or 3 args for escrow/release, 3 or 4 for slash.
		switch name {
		case "escrow", "release":
			if len(e.Args) < 2 || len(e.Args) > 3 {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeAgentEscrowNonPayable,
					Message: fmt.Sprintf("%s() requires 2 or 3 arguments (agent, amount[, purpose])", name),
					Span:    defaultSpan(filename),
				})
			}
		case "slash":
			if len(e.Args) < 2 || len(e.Args) > 4 {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeAgentEscrowNonPayable,
					Message: "slash() requires 2 to 4 arguments (agent, amount[, recipient[, purpose]])",
					Span:    defaultSpan(filename),
				})
			}
		}
		// Check escrow outside payable (TOL2311).
		// release/slash operate on already-escrowed funds and can be called from non-payable functions.
		if !isPayable && name == "escrow" {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeAgentEscrowNonPayable,
				Message: fmt.Sprintf("%s() called outside a payable function", name),
				Span:    defaultSpan(filename),
			})
		}
		// Check agent(expr) cast: numeric/bool literal is definitely not an address (TOL2306).
		if name == "agent" && len(e.Args) == 1 {
			arg := e.Args[0]
			for arg != nil && arg.Kind == "paren" {
				arg = arg.Left
			}
			if arg != nil && isNonAddressLiteralExpr(arg) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeAgentCastNonAddress,
					Message: "agent(expr): argument is a non-address literal; agent cast requires an address expression",
					Span:    defaultSpan(filename),
				})
			}
		}
		// Check __tol_task_transition literal from/to state values (TOL2315).
		if name == "__tol_task_transition" && len(e.Args) >= 4 {
			fromLit, fromOk := literalUint64(e.Args[2])
			toLit, toOk := literalUint64(e.Args[3])
			if fromOk && fromLit > 7 {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeAgentTaskInvalidTransition,
					Message: fmt.Sprintf("__tol_task_transition: from_state %d is out of range (0–7)", fromLit),
					Span:    defaultSpan(filename),
				})
			} else if toOk && toLit > 7 {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeAgentTaskInvalidTransition,
					Message: fmt.Sprintf("__tol_task_transition: to_state %d is out of range (0–7)", toLit),
					Span:    defaultSpan(filename),
				})
			} else if fromOk && toOk && !validTaskTransitions[[2]uint64{fromLit, toLit}] {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeAgentTaskInvalidTransition,
					Message: fmt.Sprintf("__tol_task_transition: transition %d→%d is not a valid task state transition", fromLit, toLit),
					Span:    defaultSpan(filename),
				})
			}
		}
	})
}

// isNonAddressLiteralExpr returns true when expr is obviously not an address:
// decimal integer literals and bool keywords. Hex literals might be valid 20-byte addresses.
func isNonAddressLiteralExpr(e *ast.Expr) bool {
	if e == nil {
		return false
	}
	if e.Kind == "number" {
		return true // decimal integer is never a valid address
	}
	if e.Kind == "ident" && (e.Value == "true" || e.Value == "false") {
		return true // boolean is never a valid address
	}
	return false
}

// literalUint64 attempts to read a decimal integer literal from an expression.
// Strips parens first. Returns (value, true) on success.
func literalUint64(e *ast.Expr) (uint64, bool) {
	for e != nil && e.Kind == "paren" {
		e = e.Left
	}
	if e == nil || e.Kind != "number" {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimSpace(e.Value), 10, 64)
	return n, err == nil
}

// walkStmtExprs calls visit for every Expr reachable from stmts (recursive).
func walkStmtExprs(stmts []ast.Statement, visit func(*ast.Expr)) {
	for i := range stmts {
		walkStmtExpr(&stmts[i], visit)
	}
}

func walkStmtExpr(s *ast.Statement, visit func(*ast.Expr)) {
	if s == nil {
		return
	}
	if s.Expr != nil {
		walkExpr(s.Expr, visit)
	}
	if s.Cond != nil {
		walkExpr(s.Cond, visit)
	}
	if s.Target != nil {
		walkExpr(s.Target, visit)
	}
	if s.Post != nil {
		walkExpr(s.Post, visit)
	}
	if s.Init != nil {
		walkStmtExpr(s.Init, visit)
	}
	walkStmtExprs(s.Then, visit)
	walkStmtExprs(s.Else, visit)
	walkStmtExprs(s.Body, visit)
	for _, catch := range s.Catches {
		walkStmtExprs(catch.Body, visit)
	}
}

func walkExpr(e *ast.Expr, visit func(*ast.Expr)) {
	if e == nil {
		return
	}
	visit(e)
	walkExpr(e.Left, visit)
	walkExpr(e.Right, visit)
	walkExpr(e.Callee, visit)
	walkExpr(e.Object, visit)
	walkExpr(e.Index, visit)
	for _, a := range e.Args {
		walkExpr(a, visit)
	}
	for _, f := range e.StructFields {
		walkExpr(f.Expr, visit)
	}
	for _, o := range e.Options {
		walkExpr(o.Value, visit)
	}
	for _, n := range e.NamedArgs {
		walkExpr(n.Expr, visit)
	}
}

// extractAgentInnerType extracts T from "oracle<T>", "vote<T>", "task<T>".
func extractAgentInnerType(typeStr string) string {
	open := strings.Index(typeStr, "<")
	if open < 0 {
		return ""
	}
	close := strings.LastIndex(typeStr, ">")
	if close <= open {
		return ""
	}
	return strings.TrimSpace(typeStr[open+1 : close])
}

// isNumericTOLType returns true for uint/int/bool types acceptable as vote<T> parameters.
func isNumericTOLType(t string) bool {
	t = strings.TrimSpace(t)
	if t == "bool" {
		return true
	}
	if len(t) >= 2 && (t[0] == 'u' || t[0] == 'i') {
		n, err := strconv.Atoi(t[1:])
		return err == nil && n >= 8 && n <= 256 && n%8 == 0
	}
	return false
}

// sliceContains returns true if s appears in slice.
func sliceContains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
