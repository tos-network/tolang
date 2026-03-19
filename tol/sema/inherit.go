package sema

// inherit.go — interface implementation checks and legacy super rejection.
//
// TOL Phase 2 keeps only one role for the `is` clause: interface implementation.
// Base contracts, abstract base hierarchies, constructor-style base arguments, and
// C3 linearization are intentionally outside the supported model. Shared behavior
// should be expressed with composition and libraries, not base-contract dispatch.

import (
	"fmt"
	"strings"

	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/diag"
)

// funcSig is the canonical signature of a function used for comparison.
type funcSig struct {
	name       string
	paramTypes []string
	retTypes   []string
	visibility string
}

func makeFuncSig(name string, params, returns []ast.FieldDecl, modifiers []string) funcSig {
	pts := make([]string, len(params))
	for i, p := range params {
		pts[i] = normalizeSelectorType(p.Type)
	}
	rts := make([]string, len(returns))
	for i, r := range returns {
		rts[i] = normalizeSelectorType(r.Type)
	}
	vis := ""
	for _, m := range modifiers {
		switch m {
		case "public", "external", "internal", "private":
			vis = m
		}
	}
	return funcSig{name: name, paramTypes: pts, retTypes: rts, visibility: vis}
}

func makeFuncSigFromDecl(fn ast.FunctionDecl) funcSig {
	return makeFuncSig(fn.Name, fn.Params, fn.Returns, fn.Modifiers)
}

func makeFuncSigFromSig(sig ast.FuncSigDecl) funcSig {
	return makeFuncSig(sig.Name, sig.Params, sig.Returns, sig.Modifiers)
}

// sigCompatible reports whether two signatures are assignment-compatible
// for the purpose of interface conformance. We require exact match of param
// types, return types, and visibility.
func sigCompatible(child, parent funcSig) bool {
	if len(child.paramTypes) != len(parent.paramTypes) {
		return false
	}
	if len(child.retTypes) != len(parent.retTypes) {
		return false
	}
	for i, pt := range child.paramTypes {
		if pt != parent.paramTypes[i] {
			return false
		}
	}
	for i, rt := range child.retTypes {
		if rt != parent.retTypes[i] {
			return false
		}
	}
	return child.visibility == parent.visibility
}

// checkInheritance runs interface implementation checks on a module.
func checkInheritance(filename string, m *ast.Module, diags *diag.Diagnostics) {
	if m.Contract == nil {
		return
	}

	c := m.Contract

	ifaceByName := make(map[string]*ast.InterfaceDecl, len(m.Interfaces))
	for i := range m.Interfaces {
		ifaceByName[m.Interfaces[i].Name] = &m.Interfaces[i]
	}

	contractFuncs := make(map[string]funcSig, len(c.Functions))
	for _, fn := range c.Functions {
		contractFuncs[fn.Name] = makeFuncSigFromDecl(fn)
	}

	for _, baseName := range c.Bases {
		iface, ok := ifaceByName[baseName]
		if !ok {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaUnknownBase,
				Message: fmt.Sprintf("base '%s' is not a known interface; TOL contracts may only list interfaces in the 'is' clause", baseName),
				Span:    defaultSpan(filename),
			})
			continue
		}
		for _, sig := range iface.Functions {
			ifaceSig := makeFuncSigFromSig(sig)
			childSig, implemented := contractFuncs[sig.Name]
			if !implemented {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInterfaceNotImpl,
					Message: fmt.Sprintf("contract '%s' must implement function '%s' required by interface '%s'", c.Name, sig.Name, iface.Name),
					Span:    defaultSpan(filename),
				})
				continue
			}
			if !sigCompatible(childSig, ifaceSig) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaOverrideSigMismatch,
					Message: fmt.Sprintf("function '%s' in contract '%s' has incompatible signature with interface '%s' (param/return types or visibility mismatch)", sig.Name, c.Name, iface.Name),
					Span:    defaultSpan(filename),
				})
			}
		}
	}

	for _, fn := range c.Functions {
		checkSuperCalls(filename, fn.Body, diags)
	}
	if c.Constructor != nil {
		checkSuperCalls(filename, c.Constructor.Body, diags)
	}
}

// checkSuperCalls validates super.fn(...) call expressions recursively.
func checkSuperCalls(filename string, stmts []ast.Statement, diags *diag.Diagnostics) {
	for _, s := range stmts {
		checkSuperCallsInExpr(filename, s.Expr, diags)
		checkSuperCallsInExpr(filename, s.Target, diags)
		checkSuperCallsInExpr(filename, s.Cond, diags)
		checkSuperCallsInExpr(filename, s.Post, diags)
		if s.Init != nil {
			checkSuperCalls(filename, []ast.Statement{*s.Init}, diags)
		}
		checkSuperCalls(filename, s.Then, diags)
		checkSuperCalls(filename, s.Else, diags)
		checkSuperCalls(filename, s.Body, diags)
	}
}

func checkSuperCallsInExpr(filename string, e *ast.Expr, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	if e.Kind == "call" {
		callee := stripParens(e.Callee)
		if callee != nil && callee.Kind == "member" {
			obj := stripParens(callee.Object)
			if obj != nil && obj.Kind == "ident" && strings.TrimSpace(obj.Value) == "super" {
				fnName := strings.TrimSpace(callee.Member)
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidSuperCall,
					Message: fmt.Sprintf("'super.%s' is not supported; use explicit library/helper calls or composition instead", fnName),
					Span:    defaultSpan(filename),
				})
				for _, a := range e.Args {
					checkSuperCallsInExpr(filename, a, diags)
				}
				return
			}
		}
		checkSuperCallsInExpr(filename, e.Callee, diags)
		for _, a := range e.Args {
			checkSuperCallsInExpr(filename, a, diags)
		}
		return
	}
	switch e.Kind {
	case "member":
		checkSuperCallsInExpr(filename, e.Object, diags)
	case "index":
		checkSuperCallsInExpr(filename, e.Object, diags)
		checkSuperCallsInExpr(filename, e.Index, diags)
	case "binary", "assign":
		checkSuperCallsInExpr(filename, e.Left, diags)
		checkSuperCallsInExpr(filename, e.Right, diags)
	case "unary":
		checkSuperCallsInExpr(filename, e.Right, diags)
	case "paren":
		checkSuperCallsInExpr(filename, e.Left, diags)
	}
}
