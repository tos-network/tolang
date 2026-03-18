package sema

import (
	"fmt"
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
//   - @pay: amount must not be empty (TOL2308), recipient must not be empty (TOL2309)
//   - delegation.verify() only inside @delegated functions (TOL2310)
//   - escrow/release/slash only inside payable functions (TOL2311)
// moduleCaps are top-level capabilities declared outside any contract (shared across file).
func checkAgentNativeDecls(filename string, c *ast.ContractDecl, moduleCaps []ast.CapabilityDecl, diags *diag.Diagnostics) {
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

	// TOL2316: account contract must declare validate(bytes32,bytes) -> bool.
	if c.IsAccount {
		hasValidate := false
		for _, fn := range c.Functions {
			if fn.Name != "validate" {
				continue
			}
			// Match: validate(bytes32,bytes) or validate(bytes32,bytes) returns (bool)
			if len(fn.Params) == 2 {
				p0 := strings.TrimSpace(fn.Params[0].Type)
				p1 := strings.TrimSpace(fn.Params[1].Type)
				if (p0 == "bytes32" || p0 == "b256") && p1 == "bytes" {
					hasValidate = true
					break
				}
			}
		}
		if !hasValidate {
			*diags = append(*diags, diag.Diagnostic{
				Code:     diag.CodeAgentMissingValidate,
				Message:  fmt.Sprintf("account contract '%s' should declare validate(bytes32 tx_hash, bytes sig) returning bool (TOL2316)", c.Name),
				Span:     defaultSpan(filename),
				Severity: diag.SeverityWarning,
			})
		}
	}
}

// checkAgentBodyCalls walks statement bodies to find:
//   - agent(non-address-literal) casts (TOL2306)
//   - delegation.verify() outside @delegated functions (TOL2310)
//   - escrow/release/slash outside payable functions (TOL2311)
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
		// Check delegation.verify/consume arity (TOL2310).
		// The new 5-arg forms are allowed in any function; only flag if called with
		// the old 2-arg (nonce, sig) signature which is no longer supported.
		if callee.Kind == "member" {
			obj := callee.Object
			for obj != nil && obj.Kind == "paren" {
				obj = obj.Left
			}
			if obj != nil && obj.Kind == "ident" &&
				strings.TrimSpace(obj.Value) == "delegation" {
				method := strings.TrimSpace(callee.Member)
				switch method {
				case "verify", "consume":
					if len(e.Args) != 5 {
						*diags = append(*diags, diag.Diagnostic{
							Code:    diag.CodeAgentDelegateVerifyOutside,
							Message: fmt.Sprintf("delegation.%s() requires exactly 5 arguments: (sig, principal, scope_hash, expiry_ms, nonce)", method),
							Span:    defaultSpan(filename),
						})
					}
				case "revoke":
					if len(e.Args) != 2 {
						*diags = append(*diags, diag.Diagnostic{
							Code:    diag.CodeAgentDelegateVerifyOutside,
							Message: "delegation.revoke() requires exactly 2 arguments: (principal, nonce)",
							Span:    defaultSpan(filename),
						})
					}
				}
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
		// Check agent(expr) cast: numeric/bool literal is definitely not an agent (TOL2306).
		if name == "agent" && len(e.Args) == 1 {
			arg := e.Args[0]
			for arg != nil && arg.Kind == "paren" {
				arg = arg.Left
			}
			if arg != nil && isNonAgentLiteralExpr(arg) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeAgentCastNonAgent,
					Message: "agent(expr): argument is a non-agent literal; agent cast requires an agent expression",
					Span:    defaultSpan(filename),
				})
			}
		}
	})
}

// isNonAgentLiteralExpr returns true when expr is obviously not an address:
// decimal integer literals and bool keywords. Hex literals might be valid 20-byte addresses.
func isNonAgentLiteralExpr(e *ast.Expr) bool {
	if e == nil {
		return false
	}
	if e.Kind == "number" {
		// Literal 0 is the canonical zero/null agent (agent(0) idiom).
		if e.Value == "0" {
			return false
		}
		return true // any other decimal integer is not a valid agent
	}
	if e.Kind == "ident" && (e.Value == "true" || e.Value == "false") {
		return true // boolean is never a valid address
	}
	return false
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

// sliceContains returns true if s appears in slice.
func sliceContains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
