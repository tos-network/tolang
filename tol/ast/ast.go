package ast

import (
	"fmt"
	"strings"
)

// StructDecl is a struct type declaration.
// Syntax: struct Foo { field: T; field2: T2; }
type StructDecl struct {
	Name   string
	Fields []FieldDecl // reuse existing FieldDecl{Name, Type string}
}

// CapabilityDecl is an agent-native capability declaration inside a contract.
// Syntax: capability Foo;
// The capability name is resolved at runtime via tos.capabilitybit("Foo").
type CapabilityDecl struct {
	Name string
	Line int
}

// PurposeDecl is an agent-native purpose declaration inside a contract.
// Syntax: purpose WorkEscrow;
// The compiler assigns ordinals 0–255 in declaration order.
type PurposeDecl struct {
	Name string
	Line int
}

// ManifestField is one key-value pair in a manifest block.
type ManifestField struct {
	Key     string
	Value   string   // string or number literal value (as text, quotes stripped by parser for strings)
	IsArray bool     // true when value is an array literal: [A, B, ...]
	Array   []string // array elements (ident or string values); valid when IsArray==true
}

// ManifestDecl is a manifest block declaration inside a contract.
// Syntax: manifest { name: "TaskBoard", version: "1.0.0" }
type ManifestDecl struct {
	Fields []ManifestField
	Line   int
}

// ImportAlias is a single symbol in a named import list with an optional local alias.
// Syntax: import { A, B as BB } from "path";
// When Alias is empty, the symbol is imported under its original Name.
type ImportAlias struct {
	Name  string // original symbol name in the source module
	Alias string // local alias (empty means use Name as-is)
}

// ImportDecl is an import statement at the top of a TOL source file.
// Supported forms:
//
//	import "path";                         — bare import (side-effect only)
//	import "path" as Alias;               — namespace alias
//	import Name from "path";              — old TOL-style (still accepted)
//	import { A, B } from "path";          — named imports
//	import { A as AA, B } from "path";    — named imports with per-symbol aliases
//	import * as X from "path";            — namespace import (star)
//	import tos.registry.AgentRegistry;    — package import (dotted, NEW)
//	import tos.registry.AgentRegistry as IRegistry; — package import with alias (NEW)
type ImportDecl struct {
	Name   string        // identifier to import (old-style "import Name from") or star-alias, or local alias for package imports
	Alias  string        // as-alias (e.g. import "path" as Alias;)
	Path   string        // import path string literal
	Line   int           // source line
	Named  []ImportAlias // named imports from "import { A, B as BB } from "path";"
	IsStar bool          // true for "import * as X from "path";"
	// Package import fields (IsPackageImport == true):
	IsPackageImport bool   // true for "import tos.registry.AgentRegistry;" form
	PackagePath     string // package path component, e.g. "tos.registry"
	PackageContract string // contract/interface name, e.g. "AgentRegistry"
}

// TypeDecl is a user-defined value type declaration at the top level.
// Syntax: type MyInt is uint256;
// The named type is a transparent alias for the underlying primitive type.
type TypeDecl struct {
	Name       string // the new type name (e.g. "MyInt")
	Underlying string // the underlying primitive type (e.g. "uint256")
}

// Module is the root node for a TOL source file.
type Module struct {
	Version           string
	Package           string          // declared package path, e.g. "tos.registry"; empty if not declared
	Imports           []ImportDecl    // import declarations
	SkippedTopDecls   []SkippedTopDecl
	Interfaces        []InterfaceDecl // parsed interface declarations
	Libraries         []LibraryDecl   // parsed library declarations
	Structs           []StructDecl    // top-level struct declarations
	TypeDecls         []TypeDecl      // user-defined value type declarations (type X is Y;)
	AbstractContracts []ContractDecl  // abstract contract declarations (may precede the concrete contract)
	// Top-level free declarations (outside any contract/library)
	FreeFunctions []FunctionDecl  // free functions at file level
	Constants     []ConstantDecl  // top-level compile-time constants
	Enums         []EnumDecl      // top-level enum declarations
	Errors        []ErrorDecl     // top-level error declarations
	Events        []EventDecl     // top-level event declarations
	UsingDecls    []UsingDecl     // top-level using-for declarations
	Capabilities  []CapabilityDecl // top-level capability declarations (shared across all contracts in file)
	Contract      *ContractDecl   // primary (first) concrete contract; kept for backward compatibility
	Contracts     []ContractDecl  // all concrete contract declarations in declaration order
	Tests         []TestDecl
}

// PrimaryContract returns the first concrete contract in the module, or nil if none.
// Use this in all code that previously accessed mod.Contract directly.
func (m *Module) PrimaryContract() *ContractDecl {
	if len(m.Contracts) > 0 {
		return &m.Contracts[0]
	}
	return m.Contract
}

// LibraryDecl is a top-level library declaration.
// Libraries support the full contractMember set except state vars, constructor, fallback, receive.
type LibraryDecl struct {
	Name       string
	Functions  []FunctionDecl
	Events     []EventDecl
	Errors     []ErrorDecl
	Enums      []EnumDecl
	Structs    []StructDecl
	TypeDecls  []TypeDecl
	Constants  []ConstantDecl
	UsingDecls []UsingDecl
}

// UsingDecl is a using-for declaration inside a contract: using LibName for Type;
type UsingDecl struct {
	Library string // the library name
	Type    string // the target type (e.g. "u256", "address")
}

// TestDecl is a test block declaration.
type TestDecl struct {
	Name          string
	Lets          []Statement // block-level let declarations (accessible in setup and test functions)
	SetupSuite    *TestLifecycleFn
	Setup         *TestLifecycleFn
	Teardown      *TestLifecycleFn
	TeardownSuite *TestLifecycleFn
	Fns           []TestFn
	Mocks         []MockDecl
}

// TestLifecycleFn is a setup/teardown lifecycle function inside a test block.
type TestLifecycleFn struct {
	Returns []FieldDecl // named bindings produced (for setup)
	Params  []FieldDecl // bindings consumed (for teardown)
	Body    []Statement
}

// TestFn is a single test function (name must start with "test_" or "fuzz_").
type TestFn struct {
	Name       string
	Params     []FieldDecl // receives bindings from setup
	Tags       []string    // from #[tag("slow")]
	Skip       bool        // from #[skip]
	Body       []Statement
	Cases      *CasesTable // non-nil when @cases attribute is present
	Fuzz       bool        // true when @fuzz attribute present
	FuzzCount  int         // 0 → runner uses Runner.FuzzCount default (100)
	Timeout    int         // milliseconds; 0 = no limit
}

// MockDecl is an inline stub contract declared in a test block.
type MockDecl struct {
	Name      string
	Interface string       // name after ':' (informational; not validated)
	Methods   []MockMethod
}

// MockMethod is a single method stub inside a mock contract.
type MockMethod struct {
	Name    string
	Params  []FieldDecl
	Returns []FieldDecl
	Body    []Statement
}

// CasesTable holds parameterized test data from the @cases { | col | ... } block.
type CasesTable struct {
	Columns []string  // header column names
	Rows    [][]*Expr // one Expr per column per row
}

type SkippedTopDecl struct {
	Kind string
	Name string
}

// ModifierDecl is a modifier declaration inside a contract.
// The body contains ordinary statements plus zero or one Statement with Kind="placeholder" (_; ).
// Body is nil when the modifier is abstract (declared with ';' instead of a block, 11.3).
type ModifierDecl struct {
	Name     string
	Params   []FieldDecl // optional parameter list (11.1)
	Virtual  bool        // true when "virtual" is present (11.2)
	Override bool        // true when "override" is present (11.2)
	Abstract bool        // true when body is ';' instead of block (11.3)
	Body     []Statement
}

// InterfaceDecl is a top-level interface declaration.
// Interfaces may contain function signatures, events, errors, enums, UDVTs, structs, and using declarations.
// Interfaces may not contain state variables or constructors.
type InterfaceDecl struct {
	Name        string
	Functions   []FuncSigDecl
	Events      []EventDecl
	Errors      []ErrorDecl
	Enums       []EnumDecl
	Structs     []StructDecl
	TypeDecls   []TypeDecl
	UsingDecls  []UsingDecl
	Constants   []ConstantDecl // compile-time constants (populated for package imports from contracts)
	// Package origin (set when imported via "import tos.pkg.Contract;"):
	PackageName  string // origin package path, e.g. "tos.registry"; empty for file imports
	ContractName string // concrete contract name in the package, e.g. "AgentRegistry"
}

// FuncSigDecl is a function signature without a body (used in interface declarations).
type FuncSigDecl struct {
	Name      string
	Params    []FieldDecl
	Returns   []FieldDecl
	Modifiers []string
	Doc       *DocMeta // structured doc comment metadata (optional)
}

// ErrorDecl is a custom error declaration inside a contract.
// Syntax: error Unauthorized(address caller, uint256 value);
type ErrorDecl struct {
	Name   string
	Params []FieldDecl // error parameters
}

// EnumDecl is an enum declaration inside a contract.
// Syntax: enum State { Active, Inactive, Paused }
type EnumDecl struct {
	Name    string
	Members []string // member names in declaration order (index 0, 1, 2, ...)
}

// ImmutableDecl is an immutable variable declaration inside a contract.
// Syntax: immutable name: Type;
// The variable must be assigned exactly in the constructor and is read-only elsewhere.
type ImmutableDecl struct {
	Name string
	Type string
}

// ConstantDecl is a compile-time constant declaration inside a contract.
// Syntax: constant NAME: TYPE = LITERAL;
// The type must be a value type (uN, iN, bool, address, bytes1..bytes32).
// The value must be a compile-time literal (integer, hex, or bool).
type ConstantDecl struct {
	Name  string
	Type  string
	Value *Expr // must be a literal expression (Kind == "number", "string", or bool ident)
}

// BaseSpecifier is an entry in the inheritance clause: Base or Base(arg1, arg2).
type BaseSpecifier struct {
	Name string   // base contract name
	Args []*Expr  // optional constructor arguments (nil means no args provided)
}

// ContractDecl is a contract declaration node.
type ContractDecl struct {
	Name           string
	Abstract       bool            // true if declared as "abstract contract"
	IsAccount      bool            // true for "account contract" declarations (AA wallet marker)
	Bases          []string        // direct parent names, in declaration order (e.g. "is A, B")
	BaseSpecifiers []BaseSpecifier // full inheritance specifiers with optional constructor args
	SkippedDecls   []SkippedContractDecl
	UsingDecls     []UsingDecl    // using LibName for Type; declarations
	Constants      []ConstantDecl // compile-time constant declarations
	TypeDecls      []TypeDecl     // user-defined value type declarations (type X is Y;) inside contract
	Storage        *StorageDecl
	Immutables     []ImmutableDecl // immutable variable declarations
	Events         []EventDecl
	Errors         []ErrorDecl
	Enums          []EnumDecl
	Structs        []StructDecl   // struct declarations inside contract
	Modifiers      []ModifierDecl
	Functions      []FunctionDecl
	Constructor    *ConstructorDecl
	Fallback       *FallbackDecl
	Receive        *ReceiveDecl
	// Agent-native declarations
	Capabilities []CapabilityDecl // capability declarations (resolved at runtime via tos.capabilitybit)
	Purposes     []PurposeDecl    // purpose declarations (ordinals assigned in declaration order)
	Manifest     *ManifestDecl    // optional manifest block
}

type SkippedContractDecl struct {
	Kind string
	Name string
}

type StorageDecl struct {
	Slots []StorageSlot
}

type StorageSlot struct {
	Name        string
	Type        string
	IsTransient bool   // true for EIP-1153 transient storage (tload/tstore)
	Visibility  string // "public", "private", "internal", or "" (default internal)
	Override    bool   // true when "override" modifier is present
	InitExpr    *Expr  // optional inline initializer: uint256 x = 1;
}

type EventDecl struct {
	Name      string
	Params    []FieldDecl
	Anonymous bool // true when "anonymous" modifier is present (9.1)
}

type FunctionDecl struct {
	Name             string
	SelectorOverride string
	Params           []FieldDecl
	Returns          []FieldDecl
	Modifiers        []string
	Body             []Statement
	Virtual          bool     // true when "virtual" modifier is present
	Override         bool     // true when "override" modifier is present
	Doc              *DocMeta // structured doc comment metadata (optional)
}

type ConstructorDecl struct {
	Params    []FieldDecl
	Modifiers []string
	Body      []Statement
	Doc       *DocMeta // structured doc comment metadata (optional)
}

type FallbackDecl struct {
	Body []Statement
	Doc  *DocMeta // structured doc comment metadata (optional)
}

// ReceiveDecl is a receive() payable function declaration inside a contract.
// Syntax: receive() payable { body }
// Receives plain ETH transfers (msg.data is empty). Must have no parameters.
type ReceiveDecl struct {
	Body []Statement
	Doc  *DocMeta // structured doc comment metadata (optional)
}

type FieldDecl struct {
	Name    string
	Type    string
	DataLoc string
	Indexed bool
}

// DocParam is a single @param or @return entry.
type DocParam struct {
	Name string
	Text string
}

// DocMeta holds structured NatSpec-style metadata parsed from a /// or /** */ doc comment.
type DocMeta struct {
	Notice  string
	Params  []DocParam
	Returns []DocParam
	Effects *EffectDecl
	Bounds  *BoundsDecl
	Gas     *GasDecl
	// Agent-native annotations
	RequiresCap  []string // @requires(caller: X) — list of capability names
	HasPay       bool     // true when @pay(...) annotation is present
	PayAmount    string   // @pay(amount=expr) — amount expression text (literal if known)
	PayRecipient string   // @pay(recipient=expr) — recipient expression text
	PayIsBare    bool     // true when @pay(expr) bare form used (no named keys)
	Delegated      bool     // @delegated — function accepts delegated calls
	Verifiable     bool     // @verifiable — function result is verifiable off-chain
	VerifiableStub bool     // true for auto-generated verify_* stub functions
	// @quota annotation
	QuotaCalls string // @quota(calls: N) — calls per purchased bundle
	QuotaPrice string // @quota(price: M) — price per bundle in micro-TOS
	// @total_cost annotation
	TotalCostMax string // @total_cost(max: N) — declared maximum total cost in wei
}

// EffectDecl is the structured representation of @effects annotations.
type EffectDecl struct {
	Reads  []string
	Writes []string
	Emits  []string
	Calls  []CallRef // nil = not declared; len==0 = declared empty
}

// CallRef is a single structured external call declaration.
type CallRef struct {
	Cap      string
	Iface    string
	Selector string
	MaxGas   uint64
	MaxCalls uint32
	MaxDepth uint32
	Wildcard bool
}

// BoundsDecl holds @bounds constraints.
type BoundsDecl struct {
	Constraints []BoundConstraint
}

// BoundConstraint is one bound: "ident <= N" or "ident == N".
type BoundConstraint struct {
	Ident string
	Op    string
	Value uint64
}

// GasDecl holds the @gas upper bound declaration.
type GasDecl struct {
	Upper     uint64
	Expr      string
	Evaluated uint64
}

// CatchClause is one catch arm in a try/catch statement.
type CatchClause struct {
	Kind      string // "" = bare, "Error" = string reason, "bytes" = raw data
	ParamName string // bound variable name (if any)
	ParamType string // "string" or "bytes" (if any)
	Body      []Statement
}

type Statement struct {
	Kind    string
	Name    string
	Names   []string // used by "let-tuple": ordered list of variable names
	Type    string
	Types   []string // used by "let-tuple": per-variable type annotations
	Text    string
	Line    int // source line (1-based); 0 = unknown
	Op      string // used by "set": empty = plain assign, "+=" etc = compound assign, "++" / "--" = inc/dec
	Expr    *Expr
	Target  *Expr
	Cond    *Expr
	Init    *Statement
	Post    *Expr
	Then    []Statement
	Else    []Statement
	Body    []Statement
	Catches []CatchClause // used by "try": catch clauses
}

// StructFieldInit is a single field initializer in a struct literal: field: expr
type StructFieldInit struct {
	Name string
	Expr *Expr
}

// CallOption is a single key-value option in a call options block: {gas: X, value: Y}.
// Used by the call expression syntax: token.transfer{value: 1 ether}(recipient).
type CallOption struct {
	Key   string // "gas" or "value"
	Value *Expr
}

// NamedArg is a single named argument in a named-arg call: f({to: alice, amount: 100}).
// Used by the call expression syntax: transfer({to: alice, amount: 100}).
type NamedArg struct {
	Name string
	Expr *Expr
}

type Expr struct {
	Kind         string
	Value        string
	Op           string
	Left         *Expr
	Right        *Expr
	Callee       *Expr
	Args         []*Expr
	NamedArgs    []NamedArg    // named call arguments: {name: expr, ...}; used when Kind == "named_call"
	Options      []CallOption  // call options block: {gas: X, value: Y}; non-nil when Options block present
	Object       *Expr
	Member       string
	Index        *Expr
	StructFields []StructFieldInit // used when Kind == "struct_lit"
}

func (m *Module) String() string {
	if m == nil {
		return "<nil>"
	}
	out := fmt.Sprintf("source tolang %s\n", m.Version)
	for _, imp := range m.Imports {
		out += fmt.Sprintf("import %s from %q;\n", imp.Name, imp.Path)
	}
	if m.Contract == nil && len(m.Tests) == 0 {
		return out + "<no contract>"
	}
	for _, d := range m.SkippedTopDecls {
		out += fmt.Sprintf("%s %s { ... }\n", d.Kind, d.Name)
	}

	for _, iface := range m.Interfaces {
		out += fmt.Sprintf("interface %s { // fns=%d\n", iface.Name, len(iface.Functions))
		out += "}\n"
	}

	for _, lib := range m.Libraries {
		out += fmt.Sprintf("library %s { // fns=%d\n", lib.Name, len(lib.Functions))
		out += "}\n"
	}

	if m.Contract != nil {
		bases := ""
		if len(m.Contract.Bases) > 0 {
			bases = " is " + strings.Join(m.Contract.Bases, ", ")
		}
		prefix := "contract"
		if m.Contract.Abstract {
			prefix = "abstract contract"
		}
		out += fmt.Sprintf("%s %s%s {\n", prefix, m.Contract.Name, bases)

		if m.Contract.Storage != nil {
			for _, slot := range m.Contract.Storage.Slots {
				prefix := ""
				if slot.IsTransient {
					prefix = "transient "
				}
				out += fmt.Sprintf("  %s%s %s;\n", prefix, slot.Type, slot.Name)
			}
		}

		for _, imm := range m.Contract.Immutables {
			out += fmt.Sprintf("  immutable %s: %s;\n", imm.Name, imm.Type)
		}

		for _, d := range m.Contract.SkippedDecls {
			out += fmt.Sprintf("  %s %s { ... }\n", d.Kind, d.Name)
		}

		for _, mod := range m.Contract.Modifiers {
			out += fmt.Sprintf("  modifier %s { ... } // stmts=%d\n", mod.Name, len(mod.Body))
		}

		for _, ev := range m.Contract.Events {
			out += fmt.Sprintf("  event %s(", ev.Name)
			for i, p := range ev.Params {
				if i > 0 {
					out += ", "
				}
				out += fmt.Sprintf("%s: %s", p.Name, p.Type)
				if p.Indexed {
					out += " indexed"
				}
			}
			out += ")\n"
		}

		for _, fn := range m.Contract.Functions {
			if fn.SelectorOverride != "" {
				out += fmt.Sprintf("  @selector(%q)\n", fn.SelectorOverride)
			}
			out += fmt.Sprintf("  function %s(", fn.Name)
			for i, p := range fn.Params {
				if i > 0 {
					out += ", "
				}
				out += fmt.Sprintf("%s %s", p.Type, p.Name)
				if p.DataLoc != "" {
					out += " " + p.DataLoc
				}
			}
			out += ")"
			if len(fn.Returns) > 0 {
				out += " returns ("
				for i, r := range fn.Returns {
					if i > 0 {
						out += ", "
					}
					out += fmt.Sprintf("%s %s", r.Type, r.Name)
				}
				out += ")"
			}
			for _, mod := range fn.Modifiers {
				out += " " + mod
			}
			out += fmt.Sprintf(" { ... } // stmts=%d\n", len(fn.Body))
		}

		if m.Contract.Constructor != nil {
			out += "  constructor("
			for i, p := range m.Contract.Constructor.Params {
				if i > 0 {
					out += ", "
				}
				out += fmt.Sprintf("%s: %s", p.Name, p.Type)
				if p.DataLoc != "" {
					out += " " + p.DataLoc
				}
			}
			out += ")"
			for _, mod := range m.Contract.Constructor.Modifiers {
				out += " " + mod
			}
			out += fmt.Sprintf(" { ... } // stmts=%d\n", len(m.Contract.Constructor.Body))
		}

		if m.Contract.Fallback != nil {
			out += fmt.Sprintf("  fallback { ... } // stmts=%d\n", len(m.Contract.Fallback.Body))
		}

		if m.Contract.Receive != nil {
			out += fmt.Sprintf("  receive() payable { ... } // stmts=%d\n", len(m.Contract.Receive.Body))
		}

		out += "}\n"
	}

	for _, td := range m.Tests {
		out += fmt.Sprintf("test %s { ... } // fns=%d\n", td.Name, len(td.Fns))
	}

	return out
}
