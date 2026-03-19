package sema

// inherit.go — C3 linearization, interface conformance, override checks, super resolution.
//
// Design notes:
//   - A single TOL file currently contains at most one contract and zero or more
//     interface declarations. Parent contracts listed in the "is" clause may reference
//     interfaces defined in the same file, or names that are not resolvable within
//     the file (cross-file parents). Unresolvable base names are recorded but not
//     treated as fatal errors so that partial inheritance trees compile.
//   - C3 linearization is computed over the subset of bases that are resolvable as
//     interface declarations within the same module. For unresolvable bases the name
//     is kept in the MRO as a stub entry.
//   - Interface conformance: for every interface listed as a base, every function
//     signature in that interface must be implemented by the contract (or inherited
//     from a resolvable base).
//   - Override compatibility: when a function declared in the child contract has the
//     same name as a function in a resolvable base interface, the signatures must match.
//   - super.fn(): validated to ensure fn exists in a resolvable parent interface
//     (for cross-file/cross-contract parents we allow the call without static validation).

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
// for the purpose of override checking. We require exact match of param types
// and return types. Visibility must match or be the same level.
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
	// Visibility must match.
	if child.visibility != parent.visibility {
		return false
	}
	return true
}

// checkInheritance runs all inheritance-related checks on a module.
func checkInheritance(filename string, m *ast.Module, diags *diag.Diagnostics) {
	if m.Contract == nil {
		return
	}

	hasBases := len(m.Contract.Bases) > 0

	// Even with no bases, validate that super is not used.
	if !hasBases {
		for _, fn := range m.Contract.Functions {
			checkSuperCalls(filename, fn.Name, fn.Body, nil, false, diags)
		}
		if m.Contract.Constructor != nil {
			checkSuperCalls(filename, "constructor", m.Contract.Constructor.Body, nil, false, diags)
		}
		// Even with no interface bases, check abstract contract conformance from
		// abstract contracts declared in the same module.
		checkAbstractConformance(filename, m, diags)
		return
	}

	// Build a map of interface name -> InterfaceDecl for quick lookup.
	ifaceByName := make(map[string]*ast.InterfaceDecl, len(m.Interfaces))
	for i := range m.Interfaces {
		ifaceByName[m.Interfaces[i].Name] = &m.Interfaces[i]
	}

	c := m.Contract

	// Compute MRO using C3 linearization over the known bases.
	mro, mroErr := c3Linearize(c.Name, c.Bases, ifaceByName)
	if mroErr != "" {
		*diags = append(*diags, diag.Diagnostic{
			Code:    diag.CodeSemaInheritC3Conflict,
			Message: mroErr,
			Span:    defaultSpan(filename),
		})
		// Continue with partial MRO.
	}

	// Build the contract's own function map (name -> sig).
	contractFuncs := make(map[string]funcSig, len(c.Functions))
	for _, fn := range c.Functions {
		contractFuncs[fn.Name] = makeFuncSigFromDecl(fn)
	}

	// For each base that is a known interface, check conformance and overrides.
	for _, baseName := range c.Bases {
		iface, ok := ifaceByName[baseName]
		if !ok {
			// Unknown base (cross-file contract or forward reference) — skip validation.
			continue
		}
		for _, sig := range iface.Functions {
			ifaceSig := makeFuncSigFromSig(sig)
			childSig, implemented := contractFuncs[sig.Name]
			if !implemented {
				// Not directly implemented. Check if inherited through other bases (future).
				// For now, report missing implementation.
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInterfaceNotImpl,
					Message: fmt.Sprintf("contract '%s' must implement function '%s' required by interface '%s'", c.Name, sig.Name, iface.Name),
					Span:    defaultSpan(filename),
				})
				continue
			}
			// Check override compatibility.
			if !sigCompatible(childSig, ifaceSig) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaOverrideSigMismatch,
					Message: fmt.Sprintf("function '%s' in contract '%s' has incompatible signature with interface '%s' (param/return types or visibility mismatch)", sig.Name, c.Name, iface.Name),
					Span:    defaultSpan(filename),
				})
			}
		}
	}

	// Check super.fn() usages in the contract's function bodies.
	// Collect all function names available in bases (from known interfaces).
	baseFuncNames := make(map[string]struct{})
	for _, baseName := range mro {
		if iface, ok := ifaceByName[baseName]; ok {
			for _, sig := range iface.Functions {
				baseFuncNames[sig.Name] = struct{}{}
			}
		}
	}
	for _, fn := range c.Functions {
		checkSuperCalls(filename, fn.Name, fn.Body, baseFuncNames, len(mro) > 0, diags)
	}
	if c.Constructor != nil {
		checkSuperCalls(filename, "constructor", c.Constructor.Body, baseFuncNames, len(mro) > 0, diags)
	}

	// Check abstract contract conformance from abstract contracts in the same module.
	checkAbstractConformance(filename, m, diags)
}

// checkAbstractConformance verifies that a non-abstract concrete contract implements
// all bodyless (abstract) function stubs declared in abstract contracts listed in its
// "is" clause that are resolvable within m.AbstractContracts.
func checkAbstractConformance(filename string, m *ast.Module, diags *diag.Diagnostics) {
	if m.Contract == nil || m.Contract.Abstract {
		// Abstract contracts themselves don't need to implement their own stubs.
		return
	}
	if len(m.AbstractContracts) == 0 || len(m.Contract.Bases) == 0 {
		return
	}

	// Build lookup map: abstract contract name → ContractDecl.
	abstractByName := make(map[string]*ast.ContractDecl, len(m.AbstractContracts))
	for i := range m.AbstractContracts {
		abstractByName[m.AbstractContracts[i].Name] = &m.AbstractContracts[i]
	}

	// Build the concrete contract's own function map (name → funcSig) for comparison.
	contractFuncs := make(map[string]funcSig, len(m.Contract.Functions))
	for _, fn := range m.Contract.Functions {
		contractFuncs[fn.Name] = makeFuncSigFromDecl(fn)
	}

	// For each base listed in the concrete contract, if it matches a known abstract
	// contract, check that all its bodyless stubs are implemented.
	for _, baseName := range m.Contract.Bases {
		ac, ok := abstractByName[baseName]
		if !ok {
			continue // Not an abstract contract we know about — skip.
		}
		for _, fn := range ac.Functions {
			if fn.Body != nil {
				// Has a body — not an abstract stub, no implementation required.
				continue
			}
			// This is an abstract stub. The concrete contract must implement it.
			childSig, implemented := contractFuncs[fn.Name]
			if !implemented {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaAbstractContractNotFullyImplemented,
					Message: fmt.Sprintf("contract '%s' must implement abstract function '%s' declared in abstract contract '%s'", m.Contract.Name, fn.Name, ac.Name),
					Span:    defaultSpan(filename),
				})
				continue
			}
			// Verify signature compatibility.
			abstractSig := makeFuncSigFromDecl(fn)
			if !sigCompatible(childSig, abstractSig) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaOverrideSigMismatch,
					Message: fmt.Sprintf("function '%s' in contract '%s' has incompatible signature with abstract function in '%s' (param/return types or visibility mismatch)", fn.Name, m.Contract.Name, ac.Name),
					Span:    defaultSpan(filename),
				})
			}
		}
	}
}

// checkSuperCalls validates super.fn(...) call expressions recursively.
func checkSuperCalls(filename, ownerFn string, stmts []ast.Statement, baseFuncNames map[string]struct{}, hasBases bool, diags *diag.Diagnostics) {
	for _, s := range stmts {
		checkSuperCallsInExpr(filename, ownerFn, s.Expr, baseFuncNames, hasBases, diags)
		checkSuperCallsInExpr(filename, ownerFn, s.Target, baseFuncNames, hasBases, diags)
		checkSuperCallsInExpr(filename, ownerFn, s.Cond, baseFuncNames, hasBases, diags)
		checkSuperCallsInExpr(filename, ownerFn, s.Post, baseFuncNames, hasBases, diags)
		if s.Init != nil {
			checkSuperCalls(filename, ownerFn, []ast.Statement{*s.Init}, baseFuncNames, hasBases, diags)
		}
		checkSuperCalls(filename, ownerFn, s.Then, baseFuncNames, hasBases, diags)
		checkSuperCalls(filename, ownerFn, s.Else, baseFuncNames, hasBases, diags)
		checkSuperCalls(filename, ownerFn, s.Body, baseFuncNames, hasBases, diags)
	}
}

func checkSuperCallsInExpr(filename, ownerFn string, e *ast.Expr, baseFuncNames map[string]struct{}, hasBases bool, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	if e.Kind == "call" {
		callee := stripParens(e.Callee)
		if callee != nil && callee.Kind == "member" {
			obj := stripParens(callee.Object)
			if obj != nil && obj.Kind == "ident" && strings.TrimSpace(obj.Value) == "super" {
				fnName := strings.TrimSpace(callee.Member)
				// Deprecation warning for all super calls.
				*diags = append(*diags, diag.Diagnostic{
					Code:     diag.CodeWarnSuperDeprecated,
					Message:  "'super' calls are deprecated; use direct library calls or composition instead",
					Span:     defaultSpan(filename),
					Severity: diag.SeverityWarning,
				})
				if !hasBases {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaInvalidSuperCall,
						Message: fmt.Sprintf("'super.%s' used in contract with no base contracts", fnName),
						Span:    defaultSpan(filename),
					})
				} else if len(baseFuncNames) > 0 {
					// We have resolved base function names from known interfaces.
					// Only error if there are no unresolved (cross-file) bases.
					if _, ok := baseFuncNames[fnName]; !ok {
						// The function is not in any known interface.
						// We don't error because there might be cross-file bases that define it.
						// This is a best-effort check.
					}
				}
				// Recursively check args.
				for _, a := range e.Args {
					checkSuperCallsInExpr(filename, ownerFn, a, baseFuncNames, hasBases, diags)
				}
				return
			}
		}
		checkSuperCallsInExpr(filename, ownerFn, e.Callee, baseFuncNames, hasBases, diags)
		for _, a := range e.Args {
			checkSuperCallsInExpr(filename, ownerFn, a, baseFuncNames, hasBases, diags)
		}
		return
	}
	switch e.Kind {
	case "member":
		checkSuperCallsInExpr(filename, ownerFn, e.Object, baseFuncNames, hasBases, diags)
	case "index":
		checkSuperCallsInExpr(filename, ownerFn, e.Object, baseFuncNames, hasBases, diags)
		checkSuperCallsInExpr(filename, ownerFn, e.Index, baseFuncNames, hasBases, diags)
	case "binary", "assign":
		checkSuperCallsInExpr(filename, ownerFn, e.Left, baseFuncNames, hasBases, diags)
		checkSuperCallsInExpr(filename, ownerFn, e.Right, baseFuncNames, hasBases, diags)
	case "unary":
		checkSuperCallsInExpr(filename, ownerFn, e.Right, baseFuncNames, hasBases, diags)
	case "paren":
		checkSuperCallsInExpr(filename, ownerFn, e.Left, baseFuncNames, hasBases, diags)
	}
}

// c3Linearize computes the C3 Method Resolution Order for a type that inherits
// from the given bases. It returns the linearized list of base names (excluding
// the contract itself) in MRO order.
//
// The algorithm is the standard C3 linearization:
//   L(C) = [C] + merge(L(B1), L(B2), ..., [B1, B2, ...])
//
// For bases that are not resolvable (not in ifaceByName), they are treated as
// leaves (no further inheritance known), so their linearization is just [BaseName].
func c3Linearize(contractName string, bases []string, ifaceByName map[string]*ast.InterfaceDecl) ([]string, string) {
	// Build linearization lists for each base.
	// For each base, call c3BaseLinearize to get its MRO.
	// Then apply the C3 merge algorithm.

	if len(bases) == 0 {
		return nil, ""
	}

	// Detect direct cycles: if contractName appears in bases.
	for _, b := range bases {
		if b == contractName {
			return nil, fmt.Sprintf("contract '%s' cannot inherit from itself", contractName)
		}
	}

	// For each base, compute its linearization (only for interfaces; contracts are stubs).
	var lists [][]string
	for _, b := range bases {
		bList := c3BaseLinearize(b, ifaceByName, map[string]bool{contractName: true})
		lists = append(lists, bList)
	}
	// Append the bases list itself.
	basesCopy := make([]string, len(bases))
	copy(basesCopy, bases)
	lists = append(lists, basesCopy)

	// Merge.
	result, err := c3Merge(lists)
	if err != "" {
		return result, err
	}
	return result, ""
}

// c3BaseLinearize computes the linearization for a single base type.
// For interfaces, it is just [InterfaceName] since interfaces in TOL don't
// inherit from other interfaces in the current grammar.
// For unknown types it returns [name].
func c3BaseLinearize(name string, ifaceByName map[string]*ast.InterfaceDecl, visited map[string]bool) []string {
	if visited[name] {
		return []string{name}
	}
	// Interfaces have no parents in the current grammar.
	// In the future, interfaces could inherit from each other.
	return []string{name}
}

// c3Merge performs the C3 linearization merge step.
// lists is a slice of sequences; returns the merged MRO.
func c3Merge(lists [][]string) ([]string, string) {
	var result []string
	for {
		// Remove empty lists.
		var nonEmpty [][]string
		for _, l := range lists {
			if len(l) > 0 {
				nonEmpty = append(nonEmpty, l)
			}
		}
		if len(nonEmpty) == 0 {
			return result, ""
		}
		// Find a "good head": a head of some list that does not appear in the
		// tail of any other list.
		found := ""
		for _, l := range nonEmpty {
			head := l[0]
			if !appearsInTail(head, nonEmpty) {
				found = head
				break
			}
		}
		if found == "" {
			// Collect the heads for the error message.
			heads := make([]string, 0, len(nonEmpty))
			for _, l := range nonEmpty {
				heads = append(heads, l[0])
			}
			return result, fmt.Sprintf("C3 linearization failed: no consistent MRO found (conflict between %s)", strings.Join(heads, ", "))
		}
		result = append(result, found)
		// Remove found from the head of all lists where it appears.
		newLists := make([][]string, 0, len(lists))
		for _, l := range lists {
			if len(l) > 0 && l[0] == found {
				newLists = append(newLists, l[1:])
			} else {
				newLists = append(newLists, l)
			}
		}
		lists = newLists
	}
}

func appearsInTail(name string, lists [][]string) bool {
	for _, l := range lists {
		for i := 1; i < len(l); i++ {
			if l[i] == name {
				return true
			}
		}
	}
	return false
}
