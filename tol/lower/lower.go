package lower

import (
	"fmt"
	"strings"

	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/diag"
	"github.com/tos-network/tolang/tol/sema"
)

// ConstantDecl is the lowered form of a compile-time constant declaration.
// The Value field holds the AST literal expression (Kind == "number" or "ident" true/false).
type ConstantDecl struct {
	Name  string
	Type  string
	Value *ast.Expr
}

// ErrorDecl is the lowered form of a custom error declaration.
type ErrorDecl struct {
	Name   string
	Params []ast.FieldDecl
}

// EnumDecl is the lowered form of an enum declaration.
type EnumDecl struct {
	Name    string
	Members []string // member names in declaration order (index = value)
}

// StructDecl is the lowered form of a struct declaration.
type StructDecl struct {
	Name   string
	Fields []ast.FieldDecl // field names and types in declaration order
}

// InterfaceDecl is the lowered form of an interface declaration.
// It carries the function signatures needed to compute EIP-165 interface IDs.
type InterfaceDecl struct {
	Name         string
	Functions    []InterfaceFuncSig
	Enums        []EnumDecl     // enum declarations (propagated from package imports)
	Constants    []ConstantDecl // compile-time constants (propagated from package imports)
	PackageName  string // origin package path, e.g. "tos.registry"; empty for file imports
	ContractName string // concrete contract name in the package, e.g. "AgentRegistry"
}

// InterfaceFuncSig is a single function signature within an interface.
type InterfaceFuncSig struct {
	Name   string
	Params []ast.FieldDecl
}

// Program is the backend-agnostic lowered form.
type Program struct {
	ContractName      string
	PackageName       string // declared package path from source, e.g. "tos.registry"
	StorageSlots      []StorageSlot
	Events            []Event
	Errors            []ErrorDecl    // custom error declarations
	Enums             []EnumDecl     // enum declarations
	Structs           []StructDecl   // struct declarations
	Constants         []ConstantDecl // compile-time constant declarations
	Interfaces        []InterfaceDecl // interface declarations (for type(I).interfaceId)
	TypeAliases       []TypeAlias   // user-defined value type declarations (type X is Y;)
	Functions         []Function
	Libraries         []Library   // library declarations (functions only, no storage)
	UsingDecls        []UsingDecl // using LibName for Type directives from the contract
	HasConstructor    bool
	ConstructorParams []ast.FieldDecl
	ConstructorBody   []ast.Statement
	HasFallback       bool
	FallbackBody      []ast.Statement
	HasReceive        bool
	ReceiveBody       []ast.Statement
	// Agent-native declarations
	Capabilities []string          // ordered capability names (resolved at runtime via tos.capabilitybit)
	Purposes     []string          // ordered purpose names (index = bit ordinal, compile-time constant)
	Manifest     map[string]string // manifest key→value pairs
}

// Library is the lowered form of a library declaration.
type Library struct {
	Name      string
	Functions []Function
}

// UsingDecl is the lowered form of a using-for declaration.
type UsingDecl struct {
	Library string
	Type    string
}

// TypeAlias is the lowered form of a user-defined value type declaration.
// Syntax: type MyInt is uint256; → TypeAlias{Name: "MyInt", Underlying: "uint256"}
// The alias is a transparent alias: MyInt is treated identically to uint256.
type TypeAlias struct {
	Name       string // the user-defined type name (e.g. "MyInt")
	Underlying string // the underlying primitive type (e.g. "uint256")
}

type StorageSlot struct {
	Name        string
	Type        string
	IsImmutable bool // true for immutable declarations (uses "tol.immutable.*" slot prefix)
	IsTransient bool // true for EIP-1153 transient storage (TLOAD/TSTORE)
}

type Event struct {
	Name   string
	Params []ast.FieldDecl
}

type Function struct {
	Name             string
	LuaName          string // Lua-level function name; equals Name for non-overloads, mangled for overloads.
	SelectorOverride string
	Params           []ast.FieldDecl
	Returns          []ast.FieldDecl
	Modifiers        []string
	Body             []ast.Statement
	Doc              *ast.DocMeta // structured doc comment metadata (optional); carries @requires, @pay, etc.
}

// MangledLuaName returns a Lua-safe mangled name for a function given its parameter types.
// All overloads get a mangled name of the form "name__type1_type2_..." to avoid collisions.
// Type names are normalized: spaces removed, "[]" → "_arr", "mapping(...)" → "_map".
func MangledLuaName(name string, params []ast.FieldDecl) string {
	if len(params) == 0 {
		return name + "__"
	}
	parts := make([]string, 0, len(params))
	for _, p := range params {
		parts = append(parts, mangleTypeName(p.Type))
	}
	return name + "__" + strings.Join(parts, "_")
}

// mangleTypeName converts a TOL type string into a Lua-identifier-safe fragment.
func mangleTypeName(t string) string {
	// Normalize whitespace first.
	s := strings.Join(strings.Fields(t), "")
	// Replace mapping(...) with _map.
	for strings.Contains(s, "mapping(") {
		start := strings.Index(s, "mapping(")
		depth := 0
		end := start
		for i := start; i < len(s); i++ {
			if s[i] == '(' {
				depth++
			} else if s[i] == ')' {
				depth--
				if depth == 0 {
					end = i + 1
					break
				}
			}
		}
		s = s[:start] + "map" + s[end:]
	}
	// Replace [] with _arr.
	s = strings.ReplaceAll(s, "[]", "_arr")
	// Remove remaining non-identifier characters (parens, commas, etc.).
	var out []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			out = append(out, c)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

// FromTypedContract lowers a single concrete contract from the typed module into a Program.
func FromTypedContract(typed *sema.TypedModule, c *ast.ContractDecl) (*Program, error) {
	if typed == nil || typed.AST == nil || c == nil {
		return nil, fmt.Errorf("[%s] invalid typed module or nil contract", diag.CodeLowerNotImplemented)
	}
	out := &Program{
		ContractName: c.Name,
	}
	if c.Storage != nil {
		out.StorageSlots = make([]StorageSlot, 0, len(c.Storage.Slots)+len(c.Immutables))
		for _, s := range c.Storage.Slots {
			out.StorageSlots = append(out.StorageSlots, StorageSlot{
				Name:        s.Name,
				Type:        normalizeType(s.Type),
				IsTransient: s.IsTransient,
			})
		}
	} else if len(c.Immutables) > 0 {
		out.StorageSlots = make([]StorageSlot, 0, len(c.Immutables))
	}
	// Lower immutable declarations as storage slots (IsImmutable=true).
	for _, imm := range c.Immutables {
		out.StorageSlots = append(out.StorageSlots, StorageSlot{
			Name:        imm.Name,
			Type:        normalizeType(imm.Type),
			IsImmutable: true,
		})
	}
	out.Events = make([]Event, 0, len(c.Events))
	for _, ev := range c.Events {
		out.Events = append(out.Events, Event{
			Name:   ev.Name,
			Params: cloneFields(ev.Params),
		})
	}

	out.Errors = make([]ErrorDecl, 0, len(c.Errors))
	for _, ed := range c.Errors {
		out.Errors = append(out.Errors, ErrorDecl{
			Name:   ed.Name,
			Params: cloneFields(ed.Params),
		})
	}

	out.Enums = make([]EnumDecl, 0, len(c.Enums))
	for _, en := range c.Enums {
		out.Enums = append(out.Enums, EnumDecl{
			Name:    en.Name,
			Members: cloneStrings(en.Members),
		})
	}

	// Lower struct declarations: top-level first, then contract-level.
	allStructs := append(typed.AST.Structs, c.Structs...)
	out.Structs = make([]StructDecl, 0, len(allStructs))
	for _, sd := range allStructs {
		out.Structs = append(out.Structs, StructDecl{
			Name:   sd.Name,
			Fields: cloneFields(sd.Fields),
		})
	}

	// Lower constant declarations.
	out.Constants = make([]ConstantDecl, 0, len(c.Constants))
	for _, cd := range c.Constants {
		out.Constants = append(out.Constants, ConstantDecl{
			Name:  cd.Name,
			Type:  normalizeType(cd.Type),
			Value: cd.Value, // AST literal expr; safe to share (read-only at lower stage)
		})
	}

	// Lower user-defined value type declarations (type X is Y;).
	out.TypeAliases = make([]TypeAlias, 0, len(typed.AST.TypeDecls))
	for _, td := range typed.AST.TypeDecls {
		out.TypeAliases = append(out.TypeAliases, TypeAlias{
			Name:       td.Name,
			Underlying: normalizeType(td.Underlying),
		})
	}

	// Lower interface declarations (for type(I).interfaceId compile-time constant).
	out.Interfaces = make([]InterfaceDecl, 0, len(typed.AST.Interfaces))
	for _, iface := range typed.AST.Interfaces {
		fns := make([]InterfaceFuncSig, 0, len(iface.Functions))
		for _, fn := range iface.Functions {
			fns = append(fns, InterfaceFuncSig{
				Name:   fn.Name,
				Params: cloneFields(fn.Params),
			})
		}
		// Propagate constants from the AST interface (populated for package imports).
		consts := make([]ConstantDecl, 0, len(iface.Constants))
		for _, cd := range iface.Constants {
			consts = append(consts, ConstantDecl{Name: cd.Name, Type: cd.Type, Value: cd.Value})
		}
		// Propagate enums from the AST interface (populated for package imports).
		enums := make([]EnumDecl, 0, len(iface.Enums))
		for _, ed := range iface.Enums {
			members := make([]string, len(ed.Members))
			copy(members, ed.Members)
			enums = append(enums, EnumDecl{Name: ed.Name, Members: members})
		}
		out.Interfaces = append(out.Interfaces, InterfaceDecl{
			Name:         iface.Name,
			Functions:    fns,
			Enums:        enums,
			Constants:    consts,
			PackageName:  iface.PackageName,  // carry package origin
			ContractName: iface.ContractName, // carry concrete contract name
		})
	}
	// Copy the package name from the source module.
	out.PackageName = typed.AST.Package

	// Build modifier lookup map for expansion.
	modMap := make(map[string]ast.ModifierDecl, len(c.Modifiers))
	for _, md := range c.Modifiers {
		modMap[md.Name] = md
	}

	// First pass: count how many concrete (non-abstract) functions share each name.
	overloadCount := make(map[string]int, len(c.Functions))
	for _, fn := range c.Functions {
		if fn.Body == nil {
			continue
		}
		overloadCount[fn.Name]++
	}

	out.Functions = make([]Function, 0, len(c.Functions))
	for _, fn := range c.Functions {
		// Skip abstract (bodyless) function stubs — they have no implementation to lower.
		if fn.Body == nil {
			continue
		}
		expandedBody, err := expandModifiers(fn.Body, fn.Modifiers, modMap)
		if err != nil {
			return nil, fmt.Errorf("[%s] expanding modifiers for function '%s': %w", diag.CodeLowerNotImplemented, fn.Name, err)
		}
		// Desugar compound-assignment and inc/dec statements.
		expandedBody = desugarStatements(expandedBody)
		// Strip user-defined modifier names from the modifiers list (keep built-in keywords only).
		builtinMods := filterBuiltinModifiers(fn.Modifiers, modMap)
		// Compute the Lua-level name. If multiple functions share the same name (overloads),
		// all of them get a mangled name to avoid Lua global collisions.
		luaName := fn.Name
		if overloadCount[fn.Name] > 1 {
			luaName = MangledLuaName(fn.Name, fn.Params)
		}
		out.Functions = append(out.Functions, Function{
			Name:             fn.Name,
			LuaName:          luaName,
			SelectorOverride: fn.SelectorOverride,
			Params:           cloneFields(fn.Params),
			Returns:          cloneFields(fn.Returns),
			Modifiers:        builtinMods,
			Body:             expandedBody,
			Doc:              fn.Doc,
		})
	}
	// Collect inline state variable initializers as constructor preamble statements.
	var slotInitStmts []ast.Statement
	if c.Storage != nil {
		for _, s := range c.Storage.Slots {
			if s.InitExpr != nil {
				slotInitStmts = append(slotInitStmts, ast.Statement{
					Kind:   "set",
					Target: &ast.Expr{Kind: "ident", Value: s.Name},
					Expr:   cloneExpr(s.InitExpr),
				})
			}
		}
	}
	out.HasConstructor = c.Constructor != nil || len(slotInitStmts) > 0
	if c.Constructor != nil {
		// Constructors do not support user-defined modifiers in current milestone.
		out.ConstructorParams = cloneFields(c.Constructor.Params)
		// Prepend slot initializers before the explicit constructor body.
		ctorBody := desugarStatements(cloneStatements(c.Constructor.Body))
		out.ConstructorBody = append(slotInitStmts, ctorBody...)
	} else if len(slotInitStmts) > 0 {
		// No explicit constructor but we have slot initializers: synthesize a constructor.
		out.ConstructorBody = slotInitStmts
	}
	out.HasFallback = c.Fallback != nil
	if c.Fallback != nil {
		out.FallbackBody = desugarStatements(cloneStatements(c.Fallback.Body))
	}
	out.HasReceive = c.Receive != nil
	if c.Receive != nil {
		out.ReceiveBody = desugarStatements(cloneStatements(c.Receive.Body))
	}

	// Lower library declarations.
	out.Libraries = make([]Library, 0, len(typed.AST.Libraries))
	for _, lib := range typed.AST.Libraries {
		lowLib := Library{Name: lib.Name}
		for _, fn := range lib.Functions {
			lowLib.Functions = append(lowLib.Functions, Function{
				Name:      fn.Name,
				Params:    cloneFields(fn.Params),
				Returns:   cloneFields(fn.Returns),
				Modifiers: append([]string(nil), fn.Modifiers...),
				Body:      desugarStatements(cloneStatements(fn.Body)),
			})
		}
		out.Libraries = append(out.Libraries, lowLib)
	}

	// Lower using-for declarations.
	out.UsingDecls = make([]UsingDecl, 0, len(c.UsingDecls))
	for _, ud := range c.UsingDecls {
		out.UsingDecls = append(out.UsingDecls, UsingDecl{
			Library: strings.TrimSpace(ud.Library),
			Type:    strings.TrimSpace(ud.Type),
		})
	}

	// Lower agent-native declarations.
	// Module-level capabilities (from typed.AST.Capabilities) come first, then contract-level.
	seen := make(map[string]bool)
	out.Capabilities = make([]string, 0, len(typed.AST.Capabilities)+len(c.Capabilities))
	for _, cd := range typed.AST.Capabilities {
		if !seen[cd.Name] {
			out.Capabilities = append(out.Capabilities, cd.Name)
			seen[cd.Name] = true
		}
	}
	for _, cd := range c.Capabilities {
		if !seen[cd.Name] {
			out.Capabilities = append(out.Capabilities, cd.Name)
			seen[cd.Name] = true
		}
	}
	out.Purposes = make([]string, 0, len(c.Purposes))
	for _, pd := range c.Purposes {
		out.Purposes = append(out.Purposes, pd.Name)
	}
	if c.Manifest != nil {
		out.Manifest = make(map[string]string, len(c.Manifest.Fields))
		for _, f := range c.Manifest.Fields {
			if f.IsArray {
				out.Manifest[f.Key] = "[" + strings.Join(f.Array, ",") + "]"
			} else {
				out.Manifest[f.Key] = f.Value
			}
		}
	}

	return out, nil
}

// FromTypedAll lowers all concrete contracts in the typed module, returning one Program per contract.
func FromTypedAll(typed *sema.TypedModule) ([]*Program, error) {
	if typed == nil || typed.AST == nil {
		return nil, fmt.Errorf("[%s] invalid typed module", diag.CodeLowerNotImplemented)
	}
	progs := make([]*Program, 0, len(typed.AST.Contracts))
	for i := range typed.AST.Contracts {
		p, err := FromTypedContract(typed, &typed.AST.Contracts[i])
		if err != nil {
			return nil, err
		}
		progs = append(progs, p)
	}
	return progs, nil
}

// FromTyped lowers the primary (first) contract in the typed module.
// Use FromTypedContract or FromTypedAll for multi-contract files.
func FromTyped(typed *sema.TypedModule) (*Program, error) {
	if typed == nil || typed.AST == nil {
		return nil, fmt.Errorf("[%s] invalid typed module", diag.CodeLowerNotImplemented)
	}
	c := typed.AST.PrimaryContract()
	if c == nil {
		return nil, fmt.Errorf("[%s] no contract in module", diag.CodeLowerNotImplemented)
	}
	return FromTypedContract(typed, c)
}

// expandModifiers expands user-defined modifier applications onto a function body.
// Multiple modifiers are applied left-to-right (outermost first): the leftmost modifier
// wraps the next, and so on, with the innermost wrapping the original function body.
//
// For each modifier, its body is cloned and the single "placeholder" statement (_; )
// is replaced by the inner body. The result is the expanded body.
func expandModifiers(fnBody []ast.Statement, modifiers []string, modMap map[string]ast.ModifierDecl) ([]ast.Statement, error) {
	// Collect user-defined modifier names from the modifiers list, preserving order.
	var userModNames []string
	for _, m := range modifiers {
		if _, ok := modMap[m]; ok {
			userModNames = append(userModNames, m)
		}
	}
	if len(userModNames) == 0 {
		return cloneStatements(fnBody), nil
	}

	// Start with the original function body.
	inner := cloneStatements(fnBody)

	// Apply modifiers in reverse order (rightmost modifier is innermost wrapper).
	for i := len(userModNames) - 1; i >= 0; i-- {
		name := userModNames[i]
		md := modMap[name]
		expanded, err := injectBody(cloneStatements(md.Body), inner)
		if err != nil {
			return nil, fmt.Errorf("modifier '%s': %w", name, err)
		}
		inner = expanded
	}
	return inner, nil
}

// injectBody replaces all "placeholder" statements in modifierBody with the provided inner body.
// It returns the resulting statement list.
func injectBody(modifierBody []ast.Statement, inner []ast.Statement) ([]ast.Statement, error) {
	result := make([]ast.Statement, 0, len(modifierBody)+len(inner))
	for _, s := range modifierBody {
		if s.Kind == "placeholder" {
			result = append(result, inner...)
			continue
		}
		// Recursively inject into sub-bodies.
		injected, err := injectIntoStmt(s, inner)
		if err != nil {
			return nil, err
		}
		result = append(result, injected)
	}
	return result, nil
}

// injectIntoStmt recursively replaces placeholder statements inside a statement's sub-bodies.
func injectIntoStmt(s ast.Statement, inner []ast.Statement) (ast.Statement, error) {
	if len(s.Then) > 0 || len(s.Else) > 0 || len(s.Body) > 0 {
		var err error
		if len(s.Then) > 0 {
			s.Then, err = injectBody(s.Then, inner)
			if err != nil {
				return s, err
			}
		}
		if len(s.Else) > 0 {
			s.Else, err = injectBody(s.Else, inner)
			if err != nil {
				return s, err
			}
		}
		if len(s.Body) > 0 {
			s.Body, err = injectBody(s.Body, inner)
			if err != nil {
				return s, err
			}
		}
	}
	return s, nil
}

// filterBuiltinModifiers returns the modifier strings that are built-in keywords (not user-defined).
func filterBuiltinModifiers(modifiers []string, modMap map[string]ast.ModifierDecl) []string {
	var out []string
	for _, m := range modifiers {
		if _, isUser := modMap[m]; !isUser {
			out = append(out, m)
		}
	}
	return out
}

// cloneExpr deep-copies an expression tree so that the result does not share
// any pointer nodes with the original.  A nil input returns nil.
func cloneExpr(e *ast.Expr) *ast.Expr {
	if e == nil {
		return nil
	}
	out := *e // shallow copy of the struct value
	out.Left = cloneExpr(e.Left)
	out.Right = cloneExpr(e.Right)
	out.Callee = cloneExpr(e.Callee)
	out.Object = cloneExpr(e.Object)
	out.Index = cloneExpr(e.Index)
	if len(e.Args) > 0 {
		out.Args = make([]*ast.Expr, len(e.Args))
		for i, a := range e.Args {
			out.Args[i] = cloneExpr(a)
		}
	}
	if len(e.StructFields) > 0 {
		out.StructFields = make([]ast.StructFieldInit, len(e.StructFields))
		for i, sf := range e.StructFields {
			out.StructFields[i] = ast.StructFieldInit{Name: sf.Name, Expr: cloneExpr(sf.Expr)}
		}
	}
	return &out
}

// compoundOpToBinaryOp maps a compound-assignment operator to its underlying
// binary operator string, e.g. "+=" → "+".
func compoundOpToBinaryOp(op string) string {
	switch op {
	case "+=":
		return "+"
	case "-=":
		return "-"
	case "*=":
		return "*"
	case "/=":
		return "/"
	case "%=":
		return "%"
	case "&=":
		return "&"
	case "|=":
		return "|"
	case "^=":
		return "^"
	case "<<=":
		return "<<"
	case ">>=":
		return ">>"
	case ">>>=":
		return ">>>"
	}
	return op
}

// desugarStmt expands a compound-assignment or inc/dec statement into an
// equivalent plain assignment statement.  All other statements are returned
// as-is (with their sub-bodies recursively desugared).
func desugarStmt(s ast.Statement) ast.Statement {
	if s.Kind == "set" && s.Op != "" {
		op := s.Op
		if op == "++" || op == "--" {
			// set x++; → set x = x + 1;
			binOp := "+"
			if op == "--" {
				binOp = "-"
			}
			one := &ast.Expr{Kind: "number", Value: "1"}
			rhs := &ast.Expr{Kind: "binary", Op: binOp, Left: cloneExpr(s.Target), Right: one}
			return ast.Statement{
				Kind:   "set",
				Target: s.Target,
				Expr:   rhs,
				Line:   s.Line,
			}
		}
		// compound assignment: set x += expr; → set x = x + expr;
		binOp := compoundOpToBinaryOp(op)
		rhs := &ast.Expr{Kind: "binary", Op: binOp, Left: cloneExpr(s.Target), Right: s.Expr}
		return ast.Statement{
			Kind:   "set",
			Target: s.Target,
			Expr:   rhs,
			Line:   s.Line,
		}
	}
	// Recursively desugar sub-bodies.
	if len(s.Then) > 0 {
		s.Then = desugarStatements(s.Then)
	}
	if len(s.Else) > 0 {
		s.Else = desugarStatements(s.Else)
	}
	if len(s.Body) > 0 {
		s.Body = desugarStatements(s.Body)
	}
	if s.Init != nil {
		desugared := desugarStmt(*s.Init)
		s.Init = &desugared
	}
	return s
}

// desugarStatements applies desugarStmt to every statement in the slice.
func desugarStatements(stmts []ast.Statement) []ast.Statement {
	if len(stmts) == 0 {
		return stmts
	}
	out := make([]ast.Statement, len(stmts))
	for i, s := range stmts {
		out[i] = desugarStmt(s)
	}
	return out
}

func cloneFields(in []ast.FieldDecl) []ast.FieldDecl {
	if len(in) == 0 {
		return nil
	}
	out := make([]ast.FieldDecl, len(in))
	copy(out, in)
	return out
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneStatements(in []ast.Statement) []ast.Statement {
	if len(in) == 0 {
		return nil
	}
	out := make([]ast.Statement, len(in))
	copy(out, in)
	return out
}

func normalizeType(t string) string {
	return strings.Join(strings.Fields(t), " ")
}
