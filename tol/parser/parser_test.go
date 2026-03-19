package parser

import (
	"testing"
)

func TestParseMinimalModule(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil {
		t.Fatalf("expected contract in AST")
	}
	if mod.Version != "0.2.0" {
		t.Fatalf("unexpected version: %s", mod.Version)
	}
	if mod.Contract.Name != "Demo" {
		t.Fatalf("unexpected contract name: %s", mod.Contract.Name)
	}
}

func TestParseContractSubset(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
interface ITRC20 { function transfer(agent to, u256 amount) public; }
library MathX { function dummy() { } }
contract Demo {
  mapping(agent => u256) balances;
  u256 total_supply;

  event Transfer(agent from indexed, agent to indexed, u256 value)

  function transfer(agent to, u256 amount) public returns (bool ok) {
    if (amount > 0) { return true; }
    return false;
  }

  constructor(agent owner) public { }
  fallback { revert "UNKNOWN_SELECTOR"; }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil {
		t.Fatalf("expected contract in AST")
	}
	// Interface and library declarations are now fully parsed (not in SkippedTopDecls).
	if len(mod.SkippedTopDecls) != 0 {
		t.Fatalf("unexpected skipped top decl count: %d (want 0)", len(mod.SkippedTopDecls))
	}
	if len(mod.Interfaces) != 1 || mod.Interfaces[0].Name != "ITRC20" {
		t.Fatalf("unexpected interfaces: %#v", mod.Interfaces)
	}
	if len(mod.Libraries) != 1 || mod.Libraries[0].Name != "MathX" {
		t.Fatalf("unexpected libraries: %#v", mod.Libraries)
	}
	if mod.Contract.Storage == nil || len(mod.Contract.Storage.Slots) != 2 {
		t.Fatalf("unexpected storage parse result: %#v", mod.Contract.Storage)
	}
	if len(mod.Contract.Events) != 1 || mod.Contract.Events[0].Name != "Transfer" {
		t.Fatalf("unexpected events parse result: %#v", mod.Contract.Events)
	}
	if len(mod.Contract.Functions) != 1 || mod.Contract.Functions[0].Name != "transfer" {
		t.Fatalf("unexpected functions parse result: %#v", mod.Contract.Functions)
	}
	if len(mod.Contract.Functions[0].Body) != 2 {
		t.Fatalf("unexpected function body stmt count: %d", len(mod.Contract.Functions[0].Body))
	}
	if mod.Contract.Functions[0].Body[0].Kind != "if" {
		t.Fatalf("unexpected first stmt kind: %s", mod.Contract.Functions[0].Body[0].Kind)
	}
	if len(mod.Contract.Functions[0].Body[0].Then) != 1 || mod.Contract.Functions[0].Body[0].Then[0].Kind != "return" {
		t.Fatalf("unexpected if-then body: %#v", mod.Contract.Functions[0].Body[0].Then)
	}
	if mod.Contract.Functions[0].Body[1].Kind != "return" {
		t.Fatalf("unexpected second stmt kind: %s", mod.Contract.Functions[0].Body[1].Kind)
	}
	if mod.Contract.Constructor == nil {
		t.Fatalf("expected constructor")
	}
	if len(mod.Contract.Constructor.Body) != 0 {
		t.Fatalf("unexpected constructor body stmt count: %d", len(mod.Contract.Constructor.Body))
	}
	if mod.Contract.Fallback == nil {
		t.Fatalf("expected fallback")
	}
	if len(mod.Contract.Fallback.Body) != 1 || mod.Contract.Fallback.Body[0].Kind != "revert" {
		t.Fatalf("unexpected fallback body: %#v", mod.Contract.Fallback.Body)
	}
}

func TestParseFunctionSelectorAttribute(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  @selector("0x1234abcd")
  function ping(u256 a) public {
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil || len(mod.Contract.Functions) != 1 {
		t.Fatalf("unexpected parse result: %#v", mod)
	}
	fn := mod.Contract.Functions[0]
	if fn.Name != "ping" {
		t.Fatalf("unexpected function name: %s", fn.Name)
	}
	if fn.SelectorOverride != "0x1234abcd" {
		t.Fatalf("unexpected selector override: %q", fn.SelectorOverride)
	}
}

func TestParseSkippedContractDecls(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  error Unauthorized(agent sender);
  enum Mode { A, B }
  modifier onlyOwner() { _; }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil {
		t.Fatalf("expected contract")
	}
	// error and enum are now fully parsed; modifier is parsed into Modifiers.
	if len(mod.Contract.SkippedDecls) != 0 {
		t.Fatalf("unexpected skipped decl count: %d (want 0)", len(mod.Contract.SkippedDecls))
	}
	if len(mod.Contract.Errors) != 1 {
		t.Fatalf("unexpected error decl count: %d (want 1)", len(mod.Contract.Errors))
	}
	if mod.Contract.Errors[0].Name != "Unauthorized" {
		t.Fatalf("unexpected error name: %s", mod.Contract.Errors[0].Name)
	}
	if len(mod.Contract.Errors[0].Params) != 1 || mod.Contract.Errors[0].Params[0].Name != "sender" {
		t.Fatalf("unexpected error params: %#v", mod.Contract.Errors[0].Params)
	}
	if len(mod.Contract.Enums) != 1 {
		t.Fatalf("unexpected enum decl count: %d (want 1)", len(mod.Contract.Enums))
	}
	if mod.Contract.Enums[0].Name != "Mode" {
		t.Fatalf("unexpected enum name: %s", mod.Contract.Enums[0].Name)
	}
	if len(mod.Contract.Enums[0].Members) != 2 || mod.Contract.Enums[0].Members[0] != "A" || mod.Contract.Enums[0].Members[1] != "B" {
		t.Fatalf("unexpected enum members: %#v", mod.Contract.Enums[0].Members)
	}
	if len(mod.Contract.Modifiers) != 1 {
		t.Fatalf("unexpected modifier count: %d (want 1)", len(mod.Contract.Modifiers))
	}
	if mod.Contract.Modifiers[0].Name != "onlyOwner" {
		t.Fatalf("unexpected modifier name: %s", mod.Contract.Modifiers[0].Name)
	}
}

func TestParseMissingHeader(t *testing.T) {
	src := []byte(`contract Demo {}`)
	_, diags := ParseFile("<test>", src)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for missing tol header")
	}
}

func TestParseLoopStatements(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function run(u256 n) public {
    u256 i = 0;
    while (i < n) {
      if (i == 5) {
        break;
      } else {
        set i = i + 1;
        continue;
      }
    }
    for (u256 j = 0; j < n; j = j + 1) {
      emit Tick(j);
    }
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil || len(mod.Contract.Functions) != 1 {
		t.Fatalf("unexpected module parse result")
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 4 {
		t.Fatalf("unexpected top-level stmt count: %d", len(body))
	}
	if body[0].Kind != "let" || body[0].Name != "i" || body[0].Type != "u256" {
		t.Fatalf("unexpected let stmt: %#v", body[0])
	}
	if body[0].Expr == nil || body[0].Expr.Kind != "number" || body[0].Expr.Value != "0" {
		t.Fatalf("unexpected let init expr: %#v", body[0].Expr)
	}
	if body[1].Kind != "while" {
		t.Fatalf("expected while stmt, got: %s", body[1].Kind)
	}
	if body[1].Cond == nil || body[1].Cond.Kind != "binary" || body[1].Cond.Op != "<" {
		t.Fatalf("unexpected while cond: %#v", body[1].Cond)
	}
	if len(body[1].Body) != 1 || body[1].Body[0].Kind != "if" {
		t.Fatalf("unexpected while body: %#v", body[1].Body)
	}
	if body[2].Kind != "for" {
		t.Fatalf("expected for stmt, got: %s", body[2].Kind)
	}
	if body[2].Init == nil || body[2].Init.Kind != "let" || body[2].Init.Name != "j" {
		t.Fatalf("unexpected for init: %#v", body[2].Init)
	}
	if body[2].Cond == nil || body[2].Cond.Kind != "binary" || body[2].Cond.Op != "<" {
		t.Fatalf("unexpected for cond: %#v", body[2].Cond)
	}
	if body[2].Post == nil || body[2].Post.Kind != "assign" || body[2].Post.Op != "=" {
		t.Fatalf("unexpected for post: %#v", body[2].Post)
	}
	if len(body[2].Body) != 1 || body[2].Body[0].Kind != "emit" {
		t.Fatalf("unexpected for body: %#v", body[2].Body)
	}
	if body[2].Body[0].Expr == nil || body[2].Body[0].Expr.Kind != "call" {
		t.Fatalf("unexpected emit expr: %#v", body[2].Body[0].Expr)
	}
	if body[3].Kind != "return" {
		t.Fatalf("expected return stmt, got: %s", body[3].Kind)
	}
}

func TestParseExpressionPrecedence(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function run() public {
    u256 x = a + b * c;
    set x = (x + 1) * foo(2, arr[i]).v;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	if len(fn.Body) != 2 {
		t.Fatalf("unexpected stmt count: %d", len(fn.Body))
	}

	letExpr := fn.Body[0].Expr
	if letExpr == nil || letExpr.Kind != "binary" || letExpr.Op != "+" {
		t.Fatalf("unexpected let expr: %#v", letExpr)
	}
	if letExpr.Right == nil || letExpr.Right.Kind != "binary" || letExpr.Right.Op != "*" {
		t.Fatalf("expected multiplication on right side due precedence, got: %#v", letExpr.Right)
	}

	setStmt := fn.Body[1]
	if setStmt.Kind != "set" || setStmt.Expr == nil {
		t.Fatalf("unexpected set stmt: %#v", setStmt)
	}
	if setStmt.Expr.Kind != "binary" || setStmt.Expr.Op != "*" {
		t.Fatalf("unexpected set rhs expr: %#v", setStmt.Expr)
	}
	if setStmt.Expr.Right == nil || setStmt.Expr.Right.Kind != "member" || setStmt.Expr.Right.Member != "v" {
		t.Fatalf("unexpected chained member expr: %#v", setStmt.Expr.Right)
	}
}

func TestParseBitwiseAndShiftExpressions(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function run() public {
    u256 x = a | b & c ^ d;
    set x = ~x << 2 >> 1;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	if len(fn.Body) != 2 {
		t.Fatalf("unexpected stmt count: %d", len(fn.Body))
	}

	letExpr := fn.Body[0].Expr
	if letExpr == nil || letExpr.Kind != "binary" || letExpr.Op != "|" {
		t.Fatalf("unexpected bitwise root expr: %#v", letExpr)
	}
	if letExpr.Right == nil || letExpr.Right.Kind != "binary" || letExpr.Right.Op != "^" {
		t.Fatalf("unexpected right branch expr: %#v", letExpr.Right)
	}
	if letExpr.Right.Left == nil || letExpr.Right.Left.Kind != "binary" || letExpr.Right.Left.Op != "&" {
		t.Fatalf("unexpected bit-and expr: %#v", letExpr.Right.Left)
	}

	setExpr := fn.Body[1].Expr
	if setExpr == nil || setExpr.Kind != "binary" || setExpr.Op != ">>" {
		t.Fatalf("unexpected set expr root: %#v", setExpr)
	}
	if setExpr.Left == nil || setExpr.Left.Kind != "binary" || setExpr.Left.Op != "<<" {
		t.Fatalf("unexpected shift-left branch: %#v", setExpr.Left)
	}
	if setExpr.Left.Left == nil || setExpr.Left.Left.Kind != "unary" || setExpr.Left.Left.Op != "~" {
		t.Fatalf("unexpected unary bit-not branch: %#v", setExpr.Left.Left)
	}
}

// --- Modifier parsing tests ---

func TestParseModifierBasic(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  modifier onlyOwner {
    require(msg.sender == owner, "not owner");
    _;
  }
  function doThing() public onlyOwner {
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(mod.Contract.Modifiers) != 1 {
		t.Fatalf("expected 1 modifier, got %d", len(mod.Contract.Modifiers))
	}
	md := mod.Contract.Modifiers[0]
	if md.Name != "onlyOwner" {
		t.Fatalf("expected modifier name 'onlyOwner', got %q", md.Name)
	}
	// Body: require stmt + placeholder
	if len(md.Body) != 2 {
		t.Fatalf("expected 2 modifier body stmts, got %d", len(md.Body))
	}
	if md.Body[0].Kind != "require" {
		t.Fatalf("expected first stmt kind 'require', got %q", md.Body[0].Kind)
	}
	if md.Body[1].Kind != "placeholder" {
		t.Fatalf("expected second stmt kind 'placeholder', got %q", md.Body[1].Kind)
	}
	// Function should have onlyOwner in modifiers list.
	if len(mod.Contract.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(mod.Contract.Functions))
	}
	fn := mod.Contract.Functions[0]
	found := false
	for _, m := range fn.Modifiers {
		if m == "onlyOwner" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected function modifiers to contain 'onlyOwner', got %v", fn.Modifiers)
	}
}

func TestParseModifierWithParentheses(t *testing.T) {
	// TOL syntax allows optional () after modifier name.
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  modifier onlyOwner() {
    _;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(mod.Contract.Modifiers) != 1 {
		t.Fatalf("expected 1 modifier, got %d", len(mod.Contract.Modifiers))
	}
	if mod.Contract.Modifiers[0].Name != "onlyOwner" {
		t.Fatalf("unexpected modifier name: %s", mod.Contract.Modifiers[0].Name)
	}
}

func TestParseModifierPlaceholderFirst(t *testing.T) {
	// Placeholder can appear before or after other statements.
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  modifier noReentrancy {
    _;
    require(unlocked, "reentrant call");
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	md := mod.Contract.Modifiers[0]
	if len(md.Body) != 2 {
		t.Fatalf("expected 2 modifier body stmts, got %d", len(md.Body))
	}
	if md.Body[0].Kind != "placeholder" {
		t.Fatalf("expected first stmt 'placeholder', got %q", md.Body[0].Kind)
	}
	if md.Body[1].Kind != "require" {
		t.Fatalf("expected second stmt 'require', got %q", md.Body[1].Kind)
	}
}

func TestParseMultipleModifiers(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  modifier onlyOwner {
    require(msg.sender == owner, "not owner");
    _;
  }
  modifier whenNotPaused {
    require(paused == false, "paused");
    _;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(mod.Contract.Modifiers) != 2 {
		t.Fatalf("expected 2 modifiers, got %d", len(mod.Contract.Modifiers))
	}
	if mod.Contract.Modifiers[0].Name != "onlyOwner" {
		t.Fatalf("unexpected first modifier name: %s", mod.Contract.Modifiers[0].Name)
	}
	if mod.Contract.Modifiers[1].Name != "whenNotPaused" {
		t.Fatalf("unexpected second modifier name: %s", mod.Contract.Modifiers[1].Name)
	}
}

func TestParseTestAssertRevertBlock(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
test Suite {
  function test_revert() {
    assert_revert("boom") {
      revert "boom";
    }
  }
}
`)
	mod, diags := ParseFile("sample_test.tol", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(mod.Tests) != 1 || len(mod.Tests[0].Fns) != 1 {
		t.Fatalf("unexpected test parse shape: %#v", mod.Tests)
	}
	body := mod.Tests[0].Fns[0].Body
	if len(body) != 1 {
		t.Fatalf("unexpected statement count: %d", len(body))
	}
	if body[0].Kind != "assert_revert" {
		t.Fatalf("expected assert_revert statement, got %q", body[0].Kind)
	}
	if body[0].Expr == nil || body[0].Expr.Kind != "string" {
		t.Fatalf("expected assert_revert message expression, got %#v", body[0].Expr)
	}
	if len(body[0].Body) != 1 || body[0].Body[0].Kind != "revert" {
		t.Fatalf("unexpected assert_revert body: %#v", body[0].Body)
	}
}


// M3: Inheritance tests.

func TestParseContractSingleInheritance(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
interface IToken {
  function transfer(agent to, u256 amount) public returns (bool ok) ;
}
contract Token is IToken {
  function transfer(agent to, u256 amount) public returns (bool ok) {
    return true;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod.Contract == nil {
		t.Fatalf("expected contract")
	}
	if len(mod.Contract.Bases) != 1 || mod.Contract.Bases[0] != "IToken" {
		t.Fatalf("unexpected bases: %v", mod.Contract.Bases)
	}
	if len(mod.Interfaces) != 1 || mod.Interfaces[0].Name != "IToken" {
		t.Fatalf("unexpected interfaces: %v", mod.Interfaces)
	}
	if len(mod.Interfaces[0].Functions) != 1 || mod.Interfaces[0].Functions[0].Name != "transfer" {
		t.Fatalf("unexpected interface functions: %v", mod.Interfaces[0].Functions)
	}
}

func TestParseContractMultipleInheritance(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
interface IOwnable {
  function owner() public returns (agent addr) ;
}
interface IToken {
  function transfer(agent to, u256 amount) public returns (bool ok) ;
}
contract Token is IToken, IOwnable {
  function transfer(agent to, u256 amount) public returns (bool ok) {
    return true;
  }
  function owner() public returns (agent addr) {
    return 0;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod.Contract == nil {
		t.Fatalf("expected contract")
	}
	if len(mod.Contract.Bases) != 2 {
		t.Fatalf("expected 2 bases, got %d: %v", len(mod.Contract.Bases), mod.Contract.Bases)
	}
	if mod.Contract.Bases[0] != "IToken" || mod.Contract.Bases[1] != "IOwnable" {
		t.Fatalf("unexpected bases: %v", mod.Contract.Bases)
	}
	if len(mod.Interfaces) != 2 {
		t.Fatalf("expected 2 interfaces, got %d", len(mod.Interfaces))
	}
}

func TestParseInterfaceMultipleFunctions(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
interface IERC20 {
  function totalSupply() public returns (u256 supply) ;
  function balanceOf(agent owner) public returns (u256 balance) ;
  function transfer(agent to, u256 amount) public returns (bool ok) ;
}
contract Token is IERC20 {
  function totalSupply() public returns (u256 supply) { return 0; }
  function balanceOf(agent owner) public returns (u256 balance) { return 0; }
  function transfer(agent to, u256 amount) public returns (bool ok) { return false; }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(mod.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(mod.Interfaces))
	}
	if len(mod.Interfaces[0].Functions) != 3 {
		t.Fatalf("expected 3 interface functions, got %d", len(mod.Interfaces[0].Functions))
	}
}

func TestParseVirtualOverrideModifiers(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Base {
  function foo() public virtual returns (u256 v) { return 1; }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod.Contract == nil || len(mod.Contract.Functions) != 1 {
		t.Fatalf("expected one function")
	}
	fn := mod.Contract.Functions[0]
	if !fn.Virtual {
		t.Fatalf("expected Virtual=true")
	}
	if fn.Override {
		t.Fatalf("expected Override=false")
	}
	// "virtual" should not appear in Modifiers.
	for _, m := range fn.Modifiers {
		if m == "virtual" {
			t.Fatalf("'virtual' should not be in Modifiers, got: %v", fn.Modifiers)
		}
	}
}

func TestParseContractNoInheritance(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function foo() public { return; }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod.Contract == nil {
		t.Fatalf("expected contract")
	}
	if len(mod.Contract.Bases) != 0 {
		t.Fatalf("expected no bases, got: %v", mod.Contract.Bases)
	}
}

func TestParseErrorDecl(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  error Unauthorized(agent caller, u256 value);
  error Empty();
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil {
		t.Fatalf("expected contract")
	}
	if len(mod.Contract.Errors) != 2 {
		t.Fatalf("expected 2 error decls, got %d", len(mod.Contract.Errors))
	}
	if mod.Contract.Errors[0].Name != "Unauthorized" {
		t.Fatalf("unexpected error name: %s", mod.Contract.Errors[0].Name)
	}
	if len(mod.Contract.Errors[0].Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(mod.Contract.Errors[0].Params))
	}
	if mod.Contract.Errors[0].Params[0].Name != "caller" || mod.Contract.Errors[0].Params[0].Type != "agent" {
		t.Fatalf("unexpected first param: %#v", mod.Contract.Errors[0].Params[0])
	}
	if mod.Contract.Errors[0].Params[1].Name != "value" || mod.Contract.Errors[0].Params[1].Type != "u256" {
		t.Fatalf("unexpected second param: %#v", mod.Contract.Errors[0].Params[1])
	}
	if mod.Contract.Errors[1].Name != "Empty" || len(mod.Contract.Errors[1].Params) != 0 {
		t.Fatalf("unexpected empty error: %#v", mod.Contract.Errors[1])
	}
}

func TestParseEnumDecl(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  enum State { Active, Inactive, Paused }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil {
		t.Fatalf("expected contract")
	}
	if len(mod.Contract.Enums) != 1 {
		t.Fatalf("expected 1 enum decl, got %d", len(mod.Contract.Enums))
	}
	en := mod.Contract.Enums[0]
	if en.Name != "State" {
		t.Fatalf("unexpected enum name: %s", en.Name)
	}
	if len(en.Members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(en.Members))
	}
	if en.Members[0] != "Active" || en.Members[1] != "Inactive" || en.Members[2] != "Paused" {
		t.Fatalf("unexpected members: %v", en.Members)
	}
}

func TestParseDeleteStatement(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function run() public {
    u256 x = 5;
    delete x;
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil || len(mod.Contract.Functions) != 1 {
		t.Fatalf("unexpected parse result")
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(body))
	}
	if body[1].Kind != "delete" {
		t.Fatalf("expected 'delete' statement kind, got %q", body[1].Kind)
	}
	if body[1].Expr == nil || body[1].Expr.Kind != "ident" || body[1].Expr.Value != "x" {
		t.Fatalf("unexpected delete target: %#v", body[1].Expr)
	}
}

func TestParseUncheckedBlock(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function run() public {
    u256 x = 0;
    unchecked {
      set x = x + 1;
    }
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil || len(mod.Contract.Functions) != 1 {
		t.Fatalf("unexpected parse result")
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(body))
	}
	if body[1].Kind != "unchecked" {
		t.Fatalf("expected 'unchecked' statement kind, got %q", body[1].Kind)
	}
	if len(body[1].Body) != 1 {
		t.Fatalf("expected 1 statement in unchecked body, got %d", len(body[1].Body))
	}
	if body[1].Body[0].Kind != "set" {
		t.Fatalf("expected 'set' inside unchecked body, got %q", body[1].Body[0].Kind)
	}
}

func TestParseTernaryExpr(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function run() public returns (u256 out) {
    u256 x = 1 == 1 ? 42 : 0;
    return x;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil || len(mod.Contract.Functions) != 1 {
		t.Fatalf("unexpected parse result")
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(body))
	}
	letStmt := body[0]
	if letStmt.Kind != "let" {
		t.Fatalf("expected 'let' statement, got %q", letStmt.Kind)
	}
	if letStmt.Expr == nil || letStmt.Expr.Kind != "ternary" {
		t.Fatalf("expected ternary expression in let initializer, got %#v", letStmt.Expr)
	}
	if len(letStmt.Expr.Args) != 3 {
		t.Fatalf("expected 3 ternary args, got %d", len(letStmt.Expr.Args))
	}
}

func TestParseTryCatch(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function call_it() public {
    try f() {
    } catch {
    }
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil {
		t.Fatalf("expected contract")
	}
	fn := mod.Contract.Functions[0]
	if len(fn.Body) < 1 {
		t.Fatalf("expected at least 1 statement in body")
	}
	tryStmt := fn.Body[0]
	if tryStmt.Kind != "try" {
		t.Fatalf("expected 'try' statement, got '%s'", tryStmt.Kind)
	}
	if tryStmt.Expr == nil || tryStmt.Expr.Kind != "call" {
		t.Fatalf("expected call expression in try, got %v", tryStmt.Expr)
	}
	if len(tryStmt.Catches) != 1 {
		t.Fatalf("expected 1 catch clause, got %d", len(tryStmt.Catches))
	}
	if tryStmt.Catches[0].Kind != "" {
		t.Fatalf("expected bare catch clause kind, got '%s'", tryStmt.Catches[0].Kind)
	}
}

func TestParseTryCatchError(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function call_it() public {
    try f() {
    } catch Error(r: string) {
    }
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil {
		t.Fatalf("expected contract")
	}
	fn := mod.Contract.Functions[0]
	tryStmt := fn.Body[0]
	if tryStmt.Kind != "try" {
		t.Fatalf("expected 'try' statement, got '%s'", tryStmt.Kind)
	}
	if len(tryStmt.Catches) != 1 {
		t.Fatalf("expected 1 catch clause, got %d", len(tryStmt.Catches))
	}
	clause := tryStmt.Catches[0]
	if clause.Kind != "Error" {
		t.Fatalf("expected 'Error' catch clause kind, got '%s'", clause.Kind)
	}
	if clause.ParamName != "r" {
		t.Fatalf("expected param name 'r', got '%s'", clause.ParamName)
	}
	if clause.ParamType != "string" {
		t.Fatalf("expected param type 'string', got '%s'", clause.ParamType)
	}
}

func TestParseTryCatchRawBytes(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function call_it() public {
    try f() {
    } catch (data: bytes) {
    }
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	tryStmt := fn.Body[0]
	if tryStmt.Kind != "try" {
		t.Fatalf("expected 'try' statement, got '%s'", tryStmt.Kind)
	}
	if len(tryStmt.Catches) != 1 {
		t.Fatalf("expected 1 catch clause, got %d", len(tryStmt.Catches))
	}
	clause := tryStmt.Catches[0]
	if clause.Kind != "bytes" {
		t.Fatalf("expected 'bytes' catch clause kind, got '%s'", clause.Kind)
	}
	if clause.ParamName != "data" {
		t.Fatalf("expected param name 'data', got '%s'", clause.ParamName)
	}
}

func TestParseTryCatchPanic(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function call_it() public {
    try f() {
    } catch Panic(code: u256) {
    }
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	tryStmt := fn.Body[0]
	if tryStmt.Kind != "try" {
		t.Fatalf("expected 'try' statement, got '%s'", tryStmt.Kind)
	}
	if len(tryStmt.Catches) != 1 {
		t.Fatalf("expected 1 catch clause, got %d", len(tryStmt.Catches))
	}
	clause := tryStmt.Catches[0]
	if clause.Kind != "Panic" {
		t.Fatalf("expected 'Panic' catch clause kind, got '%s'", clause.Kind)
	}
	if clause.ParamName != "code" {
		t.Fatalf("expected param name 'code', got '%s'", clause.ParamName)
	}
	if clause.ParamType != "u256" {
		t.Fatalf("expected param type 'u256', got '%s'", clause.ParamType)
	}
}

func TestParseTryNew(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function create_it() public {
    try new Foo(1, 2) {
    } catch {
    }
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	tryStmt := fn.Body[0]
	if tryStmt.Kind != "try" {
		t.Fatalf("expected 'try' statement, got '%s'", tryStmt.Kind)
	}
	if tryStmt.Expr == nil || tryStmt.Expr.Kind != "new" {
		t.Fatalf("expected 'new' expression in try, got %v", tryStmt.Expr)
	}
	if tryStmt.Expr.Value != "Foo" {
		t.Fatalf("expected contract name 'Foo', got '%s'", tryStmt.Expr.Value)
	}
	if len(tryStmt.Expr.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(tryStmt.Expr.Args))
	}
	if len(tryStmt.Catches) != 1 {
		t.Fatalf("expected 1 catch clause, got %d", len(tryStmt.Catches))
	}
	if tryStmt.Catches[0].Kind != "" {
		t.Fatalf("expected bare catch clause, got '%s'", tryStmt.Catches[0].Kind)
	}
}

func TestParseStructDeclContractLevel(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  struct Point { u256 x; agent y; }
  function getPoint() public returns (Point p) {
    return p;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil {
		t.Fatalf("expected contract")
	}
	if len(mod.Contract.Structs) != 1 {
		t.Fatalf("expected 1 struct, got %d", len(mod.Contract.Structs))
	}
	sd := mod.Contract.Structs[0]
	if sd.Name != "Point" {
		t.Fatalf("unexpected struct name: %s", sd.Name)
	}
	if len(sd.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(sd.Fields))
	}
	if sd.Fields[0].Name != "x" || sd.Fields[0].Type != "u256" {
		t.Fatalf("unexpected field 0: %#v", sd.Fields[0])
	}
	if sd.Fields[1].Name != "y" || sd.Fields[1].Type != "agent" {
		t.Fatalf("unexpected field 1: %#v", sd.Fields[1])
	}
}

func TestParseStructDeclTopLevel(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
struct Pair { u256 a; u256 b; }
contract Demo {
  function sum(Pair p) public returns (u256 out) {
    return out;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(mod.Structs) != 1 {
		t.Fatalf("expected 1 top-level struct, got %d", len(mod.Structs))
	}
	sd := mod.Structs[0]
	if sd.Name != "Pair" {
		t.Fatalf("unexpected struct name: %s", sd.Name)
	}
	if len(sd.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d: %#v", len(sd.Fields), sd.Fields)
	}
}

func TestParseStructLiteralExpr(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  struct Point { u256 x; u256 y; }
  function mk() public returns (Point p) {
    Point p = Point { x: 1, y: 2 };
    return p;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil || mod.Contract == nil {
		t.Fatalf("expected contract")
	}
	fns := mod.Contract.Functions
	if len(fns) != 1 {
		t.Fatalf("expected 1 function")
	}
	body := fns[0].Body
	if len(body) < 1 {
		t.Fatalf("expected at least 1 statement")
	}
	letStmt := body[0]
	if letStmt.Kind != "let" {
		t.Fatalf("expected 'let' statement, got %s", letStmt.Kind)
	}
	if letStmt.Expr == nil {
		t.Fatalf("expected let initializer expression")
	}
	if letStmt.Expr.Kind != "struct_lit" {
		t.Fatalf("expected 'struct_lit' expression, got %s", letStmt.Expr.Kind)
	}
	if letStmt.Expr.Value != "Point" {
		t.Fatalf("unexpected struct name in literal: %s", letStmt.Expr.Value)
	}
	if len(letStmt.Expr.StructFields) != 2 {
		t.Fatalf("expected 2 struct fields, got %d", len(letStmt.Expr.StructFields))
	}
	if letStmt.Expr.StructFields[0].Name != "x" {
		t.Fatalf("unexpected first field name: %s", letStmt.Expr.StructFields[0].Name)
	}
	if letStmt.Expr.StructFields[1].Name != "y" {
		t.Fatalf("unexpected second field name: %s", letStmt.Expr.StructFields[1].Name)
	}
}

// TestParseAbstractContract verifies that "abstract contract" sets Abstract=true on the
// ContractDecl and that a bodyless function (ending with ';') is accepted and produces a
// FunctionDecl with Virtual==true and Body==nil.
func TestParseAbstractContract(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
abstract contract Foo {
    function bar() public virtual ;
    function baz() public returns (bool ok) { return true; }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil {
		t.Fatalf("expected non-nil module")
	}
	// Abstract contract goes into m.AbstractContracts.
	if len(mod.AbstractContracts) != 1 {
		t.Fatalf("expected 1 abstract contract, got %d", len(mod.AbstractContracts))
	}
	ac := mod.AbstractContracts[0]
	if ac.Name != "Foo" {
		t.Fatalf("unexpected abstract contract name: %s", ac.Name)
	}
	if !ac.Abstract {
		t.Fatalf("expected Abstract=true on abstract contract")
	}
	if len(ac.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(ac.Functions))
	}
	// First function: bodyless virtual stub.
	stub := ac.Functions[0]
	if stub.Name != "bar" {
		t.Fatalf("unexpected stub name: %s", stub.Name)
	}
	if !stub.Virtual {
		t.Fatalf("expected Virtual=true on stub function")
	}
	if stub.Body != nil {
		t.Fatalf("expected nil body on stub function, got %d stmts", len(stub.Body))
	}
	// Second function: has a body.
	impl := ac.Functions[1]
	if impl.Name != "baz" {
		t.Fatalf("unexpected impl name: %s", impl.Name)
	}
	if impl.Body == nil {
		t.Fatalf("expected non-nil body on implemented function")
	}
}

// TestParseAbstractContractWithConcreteContract verifies that an abstract contract
// followed by a concrete contract are both parsed correctly.
func TestParseAbstractContractWithConcreteContract(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
abstract contract Base {
    function transfer(agent to, u256 amount) public virtual returns (bool ok) ;
}
contract Token is Base {
    function transfer(agent to, u256 amount) public override returns (bool ok) {
        return true;
    }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod == nil {
		t.Fatalf("expected non-nil module")
	}
	if len(mod.AbstractContracts) != 1 {
		t.Fatalf("expected 1 abstract contract, got %d", len(mod.AbstractContracts))
	}
	if mod.AbstractContracts[0].Name != "Base" {
		t.Fatalf("unexpected abstract contract name: %s", mod.AbstractContracts[0].Name)
	}
	if mod.Contract == nil {
		t.Fatalf("expected concrete contract")
	}
	if mod.Contract.Name != "Token" {
		t.Fatalf("unexpected concrete contract name: %s", mod.Contract.Name)
	}
	if mod.Contract.Abstract {
		t.Fatalf("expected Abstract=false on concrete contract")
	}
	if len(mod.Contract.Bases) != 1 || mod.Contract.Bases[0] != "Base" {
		t.Fatalf("unexpected bases: %v", mod.Contract.Bases)
	}
	if len(mod.Contract.Functions) != 1 {
		t.Fatalf("expected 1 function in Token, got %d", len(mod.Contract.Functions))
	}
	fn := mod.Contract.Functions[0]
	if fn.Name != "transfer" {
		t.Fatalf("unexpected function name: %s", fn.Name)
	}
	if !fn.Override {
		t.Fatalf("expected Override=true")
	}
	if fn.Body == nil {
		t.Fatalf("expected non-nil body on concrete function")
	}
}

func TestParseDoWhileStatement(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function run() public {
    u256 i = 0;
    do {
      set i = i + 1;
    } while (i < 10);
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	if len(fn.Body) != 3 {
		t.Fatalf("expected 3 statements (let, dowhile, return), got %d", len(fn.Body))
	}
	dw := fn.Body[1]
	if dw.Kind != "dowhile" {
		t.Fatalf("expected dowhile, got %s", dw.Kind)
	}
	if dw.Cond == nil {
		t.Fatal("do/while condition must not be nil")
	}
	if len(dw.Body) != 1 {
		t.Fatalf("expected 1 body statement, got %d", len(dw.Body))
	}
}

func TestParseReceivePayable(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Wallet {
  receive() payable {
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod.Contract.Receive == nil {
		t.Fatal("expected non-nil Receive declaration")
	}
	if len(mod.Contract.Receive.Body) != 1 {
		t.Fatalf("expected 1 body statement, got %d", len(mod.Contract.Receive.Body))
	}
}

func TestParseReceiveNonPayableRejected(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Test {
  receive() {
    return;
  }
}
`)
	_, diags := ParseFile("<test>", src)
	if !diags.HasErrors() {
		t.Fatal("expected diagnostic for non-payable receive")
	}
}

func TestParseForWithParentheses(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract C {
  function run() public {
    for (u256 i = 0; i < 10; i = i + 1) {
      set x = i;
    }
    return;
  }
}`)
	m, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse error for parenthesized for: %v", diags)
	}
	body := m.Contract.Functions[0].Body
	if len(body) < 1 || body[0].Kind != "for" {
		t.Fatalf("expected for stmt, got: %v", body)
	}
	if body[0].Init == nil || body[0].Cond == nil || body[0].Post == nil {
		t.Fatalf("for stmt missing init/cond/post: %+v", body[0])
	}
}

func TestParseForWithAndWithoutParensEquivalent(t *testing.T) {
	srcNoParen := []byte(`pragma tolang 0.2.0;
contract C {
  function run() public {
    for (u256 i = 0; i < 10; i = i + 1) {
      set x = i;
    }
    return;
  }
}`)
	srcParen := []byte(`pragma tolang 0.2.0;
contract C {
  function run() public {
    for (u256 i = 0; i < 10; i = i + 1) {
      set x = i;
    }
    return;
  }
}`)
	m1, d1 := ParseFile("<test>", srcNoParen)
	m2, d2 := ParseFile("<test>", srcParen)
	if d1.HasErrors() {
		t.Fatalf("no-paren form failed: %v", d1)
	}
	if d2.HasErrors() {
		t.Fatalf("paren form failed: %v", d2)
	}
	s1 := m1.Contract.Functions[0].Body[0]
	s2 := m2.Contract.Functions[0].Body[0]
	if s1.Kind != "for" || s2.Kind != "for" {
		t.Fatal("expected for stmt in both forms")
	}
	// Both should produce identical AST shape.
	if s1.Init == nil || s2.Init == nil {
		t.Fatal("missing init")
	}
	if s1.Cond == nil || s2.Cond == nil {
		t.Fatal("missing cond")
	}
	if s1.Post == nil || s2.Post == nil {
		t.Fatal("missing post")
	}
}

func TestParseWhileWithParentheses(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract C {
  function run() public {
    while (x < 10) {
      set x = x + 1;
    }
    return;
  }
}`)
	m, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse error for parenthesized while: %v", diags)
	}
	body := m.Contract.Functions[0].Body
	if len(body) < 1 || body[0].Kind != "while" {
		t.Fatalf("expected while stmt, got: %v", body)
	}
	if body[0].Cond == nil {
		t.Fatal("while stmt missing cond")
	}
}

func TestParseWhileWithAndWithoutParensEquivalent(t *testing.T) {
	srcNoParen := []byte(`pragma tolang 0.2.0;
contract C {
  function run() public {
    while (x < 10) {
      set x = x + 1;
    }
    return;
  }
}`)
	srcParen := []byte(`pragma tolang 0.2.0;
contract C {
  function run() public {
    while (x < 10) {
      set x = x + 1;
    }
    return;
  }
}`)
	m1, d1 := ParseFile("<test>", srcNoParen)
	m2, d2 := ParseFile("<test>", srcParen)
	if d1.HasErrors() {
		t.Fatalf("no-paren while failed: %v", d1)
	}
	if d2.HasErrors() {
		t.Fatalf("paren while failed: %v", d2)
	}
	s1 := m1.Contract.Functions[0].Body[0]
	s2 := m2.Contract.Functions[0].Body[0]
	if s1.Kind != "while" || s2.Kind != "while" {
		t.Fatal("expected while stmt in both forms")
	}
	if s1.Cond == nil || s2.Cond == nil {
		t.Fatal("missing cond")
	}
}

func TestParsePostfixIncStatement(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract C {
  function run() public {
    i++;
    j--;
    return;
  }
}`)
	m, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse error for i++ / j--: %v", diags)
	}
	body := m.Contract.Functions[0].Body
	if body[0].Kind != "set" || body[0].Op != "++" {
		t.Fatalf("expected set++ for i++, got kind=%s op=%s", body[0].Kind, body[0].Op)
	}
	if body[1].Kind != "set" || body[1].Op != "--" {
		t.Fatalf("expected set-- for j--, got kind=%s op=%s", body[1].Kind, body[1].Op)
	}
}

func TestParsePrefixIncStatement(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract C {
  function run() public {
    ++i;
    --j;
    return;
  }
}`)
	m, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse error for ++i / --j: %v", diags)
	}
	body := m.Contract.Functions[0].Body
	if body[0].Kind != "set" || body[0].Op != "++" {
		t.Fatalf("expected set++ for ++i, got kind=%s op=%s", body[0].Kind, body[0].Op)
	}
	if body[1].Kind != "set" || body[1].Op != "--" {
		t.Fatalf("expected set-- for --j, got kind=%s op=%s", body[1].Kind, body[1].Op)
	}
}

func TestParseForPostStepPostfixInc(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract C {
  function run() public {
    for (u256 i = 0; i < 10; i++) {
      set x = i;
    }
    return;
  }
}`)
	m, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected parse error for for(;;i++): %v", diags)
	}
	forStmt := m.Contract.Functions[0].Body[0]
	if forStmt.Kind != "for" {
		t.Fatalf("expected for stmt, got %s", forStmt.Kind)
	}
	if forStmt.Post == nil {
		t.Fatal("for stmt missing post")
	}
	if forStmt.Post.Kind != "unary" || forStmt.Post.Op != "post++" {
		t.Fatalf("expected post++ expr, got kind=%s op=%s", forStmt.Post.Kind, forStmt.Post.Op)
	}
}

func TestParseDocMetaTripleSlash(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
    /// @notice Say hello.
    /// @effects reads: storage.x
    /// @effects writes: storage.y
    /// @effects calls: []
    /// @gas upper: 5000
    function foo() public view {}
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	if mod.Contract == nil || len(mod.Contract.Functions) == 0 {
		t.Fatal("expected function")
	}
	fn := mod.Contract.Functions[0]
	if fn.Doc == nil {
		t.Fatal("expected Doc to be set on function")
	}
	if fn.Doc.Notice != "Say hello." {
		t.Errorf("unexpected notice: %q", fn.Doc.Notice)
	}
	if fn.Doc.Effects == nil {
		t.Fatal("expected Effects")
	}
	if len(fn.Doc.Effects.Reads) == 0 || fn.Doc.Effects.Reads[0] != "storage.x" {
		t.Errorf("unexpected reads: %v", fn.Doc.Effects.Reads)
	}
	if len(fn.Doc.Effects.Writes) == 0 || fn.Doc.Effects.Writes[0] != "storage.y" {
		t.Errorf("unexpected writes: %v", fn.Doc.Effects.Writes)
	}
	if fn.Doc.Effects.Calls == nil || len(fn.Doc.Effects.Calls) != 0 {
		t.Errorf("expected empty calls slice, got %v", fn.Doc.Effects.Calls)
	}
	if fn.Doc.Gas == nil || fn.Doc.Gas.Upper != 5000 {
		t.Errorf("unexpected gas: %v", fn.Doc.Gas)
	}
}

func TestParseDocMetaBlockStyle(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
    /**
     * @notice Block style.
     * @effects reads: storage.balances[caller]
     * @effects calls: cap:OracleCap selector:0x12345678 max_gas:3000 max_calls:1
     */
    function bar() public {}
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	if fn.Doc == nil || fn.Doc.Effects == nil {
		t.Fatal("expected Doc.Effects")
	}
	if len(fn.Doc.Effects.Reads) == 0 {
		t.Errorf("expected reads")
	}
	if len(fn.Doc.Effects.Calls) != 1 {
		t.Errorf("expected 1 CallRef, got %d", len(fn.Doc.Effects.Calls))
	} else {
		cr := fn.Doc.Effects.Calls[0]
		if cr.Cap != "OracleCap" {
			t.Errorf("unexpected cap: %q", cr.Cap)
		}
		if cr.Selector != "0x12345678" {
			t.Errorf("unexpected selector: %q", cr.Selector)
		}
		if cr.MaxGas != 3000 {
			t.Errorf("unexpected max_gas: %d", cr.MaxGas)
		}
	}
}

func TestParseDocMetaWildcardCalls(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
    /// @effects calls: *
    function drain() public {}
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	if fn.Doc == nil || fn.Doc.Effects == nil || len(fn.Doc.Effects.Calls) != 1 {
		t.Fatal("expected 1 CallRef")
	}
	if !fn.Doc.Effects.Calls[0].Wildcard {
		t.Error("expected Wildcard=true")
	}
}

func TestParseDocMetaBoundsAndParametricGas(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
    /// @bounds n <= 100
    /// @gas upper: 5000
    function batch(u256 n) public {}
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	if fn.Doc == nil {
		t.Fatal("expected Doc")
	}
	if fn.Doc.Bounds == nil || len(fn.Doc.Bounds.Constraints) == 0 {
		t.Fatal("expected Bounds")
	}
	bc := fn.Doc.Bounds.Constraints[0]
	if bc.Ident != "n" || bc.Op != "<=" || bc.Value != 100 {
		t.Errorf("unexpected bound: %+v", bc)
	}
}

func TestNoDocMetaOnUnannotatedFn(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
    function plain() public {}
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	if fn.Doc != nil {
		t.Errorf("expected nil Doc on unannotated function, got %+v", fn.Doc)
	}
}

func TestDocMetaNotBoundAcrossBlankToken(t *testing.T) {
	// A doc comment followed by a let statement (not a fn) must NOT bind to the fn.
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
    // Regular comment — not a doc comment
    function plain() public {}
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	if fn.Doc != nil {
		t.Errorf("expected nil Doc when ordinary comment present, got %+v", fn.Doc)
	}
}

// ── Task #4: require/assert single-argument form ──────────────────────────────

func TestParseRequireNoMessage(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function run(bool ok) public {
    require(ok);
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for require(ok): %v", diags)
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(body))
	}
	req := body[0]
	if req.Kind != "require" {
		t.Fatalf("expected 'require' stmt, got %q", req.Kind)
	}
	if req.Text != `""` {
		t.Fatalf("expected empty message \"\", got %q", req.Text)
	}
	if req.Expr == nil {
		t.Fatal("expected non-nil condition expr")
	}
}

func TestParseAssertNoMessage(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function run(u256 x) public {
    assert(x > 0);
    assert(x < 100, "too large");
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(body))
	}
	// First assert: no message
	a0 := body[0]
	if a0.Kind != "assert" {
		t.Fatalf("expected 'assert', got %q", a0.Kind)
	}
	if a0.Text != `""` {
		t.Fatalf("expected empty message for first assert, got %q", a0.Text)
	}
	// Second assert: explicit message
	a1 := body[1]
	if a1.Kind != "assert" {
		t.Fatalf("expected 'assert', got %q", a1.Kind)
	}
	if a1.Text != `"too large"` {
		t.Fatalf("expected message \"too large\", got %q", a1.Text)
	}
}

// ── Task #5: Import syntax variants ──────────────────────────────────────────

func TestParseImportBare(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
import "./utils.tol";
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for bare import: %v", diags)
	}
	if len(mod.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(mod.Imports))
	}
	imp := mod.Imports[0]
	if imp.Path != "./utils.tol" {
		t.Fatalf("unexpected path: %q", imp.Path)
	}
	if imp.Alias != "" || imp.Name != "" || len(imp.Named) != 0 || imp.IsStar {
		t.Fatalf("bare import should have no alias/name/named/star: %+v", imp)
	}
}

func TestParseImportNamed(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
import { SafeMath, Address } from "./lib.tol";
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for named import: %v", diags)
	}
	if len(mod.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(mod.Imports))
	}
	imp := mod.Imports[0]
	if imp.Path != "./lib.tol" {
		t.Fatalf("unexpected path: %q", imp.Path)
	}
	if len(imp.Named) != 2 || imp.Named[0].Name != "SafeMath" || imp.Named[1].Name != "Address" {
		t.Fatalf("unexpected named imports: %v", imp.Named)
	}
}

func TestParseImportStar(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
import * as Lib from "./lib.tol";
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for star import: %v", diags)
	}
	if len(mod.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(mod.Imports))
	}
	imp := mod.Imports[0]
	if !imp.IsStar {
		t.Fatal("expected IsStar=true for 'import * as'")
	}
	if imp.Alias != "Lib" {
		t.Fatalf("unexpected alias: %q", imp.Alias)
	}
	if imp.Path != "./lib.tol" {
		t.Fatalf("unexpected path: %q", imp.Path)
	}
}

// ── Task #8: Type-first constant/immutable syntax ─────────────────────────────

func TestParseConstantTypeFirst(t *testing.T) {
	// Note: the lexer normalizes uint256 -> u256, so the AST type is "u256".
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  uint256 constant MAX_SUPPLY = 1000000;
  constant ZERO: agent = "0x0000000000000000000000000000000000000000000000000000000000000000";
  function run() public { return; }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for type-first constant: %v", diags)
	}
	if len(mod.Contract.Constants) != 2 {
		t.Fatalf("expected 2 constants, got %d", len(mod.Contract.Constants))
	}
	c0 := mod.Contract.Constants[0]
	if c0.Name != "MAX_SUPPLY" {
		t.Fatalf("unexpected constant name: %q", c0.Name)
	}
	if c0.Type != "u256" { // lexer normalizes uint256 -> u256
		t.Fatalf("unexpected constant type: %q", c0.Type)
	}
	if c0.Value == nil || c0.Value.Kind != "number" || c0.Value.Value != "1000000" {
		t.Fatalf("unexpected constant value: %+v", c0.Value)
	}
	c1 := mod.Contract.Constants[1]
	if c1.Name != "ZERO" {
		t.Fatalf("unexpected constant name: %q", c1.Name)
	}
	if c1.Type != "agent" {
		t.Fatalf("unexpected constant type: %q", c1.Type)
	}
}

func TestParseImmutableTypeFirst(t *testing.T) {
	// Note: the lexer normalizes uint256 -> u256, so the AST type is "u256".
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  uint256 immutable maxAmount;
  immutable owner: agent;
  constructor(agent o, uint256 m) public {
    set owner = o;
    set maxAmount = m;
  }
  function run() public { return; }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for type-first immutable: %v", diags)
	}
	if len(mod.Contract.Immutables) != 2 {
		t.Fatalf("expected 2 immutables, got %d", len(mod.Contract.Immutables))
	}
	i0 := mod.Contract.Immutables[0]
	if i0.Name != "maxAmount" {
		t.Fatalf("unexpected immutable name: %q", i0.Name)
	}
	if i0.Type != "u256" { // lexer normalizes uint256 -> u256
		t.Fatalf("unexpected immutable type: %q", i0.Type)
	}
	i1 := mod.Contract.Immutables[1]
	if i1.Name != "owner" {
		t.Fatalf("unexpected immutable name: %q", i1.Name)
	}
	if i1.Type != "agent" {
		t.Fatalf("unexpected immutable type: %q", i1.Type)
	}
}

func TestParseBothOldAndNewConstantForms(t *testing.T) {
	// Verify that both old and new constant syntax work together in the same contract.
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  constant OLD_STYLE: uint256 = 42;
  uint256 constant NEW_STYLE = 99;
  function run() public { return; }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for mixed constant forms: %v", diags)
	}
	if len(mod.Contract.Constants) != 2 {
		t.Fatalf("expected 2 constants, got %d", len(mod.Contract.Constants))
	}
	names := map[string]bool{}
	for _, c := range mod.Contract.Constants {
		names[c.Name] = true
	}
	if !names["OLD_STYLE"] || !names["NEW_STYLE"] {
		t.Fatalf("expected both OLD_STYLE and NEW_STYLE constants, got: %v", names)
	}
}

func TestTypeFirstLocalVarDecl(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  u256 total_supply;

  function run(u256 n) public {
    uint256 x = 1;
    uint256 y;
    agent owner = msg.sender;
    bool flag = true;
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for type-first local var decl: %v", diags)
	}
	if mod == nil || mod.Contract == nil || len(mod.Contract.Functions) != 1 {
		t.Fatalf("expected exactly one function")
	}
	body := mod.Contract.Functions[0].Body
	// Expect 5 statements: uint256 x=1, uint256 y, agent owner, bool flag, return
	if len(body) != 5 {
		t.Fatalf("expected 5 stmts in body, got %d: %#v", len(body), body)
	}

	// uint256 x = 1; — lexer normalizes uint256 → u256
	s0 := body[0]
	if s0.Kind != "let" {
		t.Fatalf("stmt[0] kind: want 'let', got '%s'", s0.Kind)
	}
	if s0.Name != "x" {
		t.Fatalf("stmt[0] name: want 'x', got '%s'", s0.Name)
	}
	if s0.Type != "u256" {
		t.Fatalf("stmt[0] type: want 'u256' (lexer normalizes uint256→u256), got '%s'", s0.Type)
	}
	if s0.Expr == nil || s0.Expr.Kind != "number" || s0.Expr.Value != "1" {
		t.Fatalf("stmt[0] init expr: want number(1), got %#v", s0.Expr)
	}

	// uint256 y; — no initializer; lexer normalizes uint256 → u256
	s1 := body[1]
	if s1.Kind != "let" {
		t.Fatalf("stmt[1] kind: want 'let', got '%s'", s1.Kind)
	}
	if s1.Name != "y" {
		t.Fatalf("stmt[1] name: want 'y', got '%s'", s1.Name)
	}
	if s1.Type != "u256" {
		t.Fatalf("stmt[1] type: want 'u256' (lexer normalizes uint256→u256), got '%s'", s1.Type)
	}
	if s1.Expr != nil {
		t.Fatalf("stmt[1] should have no initializer, got %#v", s1.Expr)
	}

	// agent owner = msg.sender;
	s2 := body[2]
	if s2.Kind != "let" {
		t.Fatalf("stmt[2] kind: want 'let', got '%s'", s2.Kind)
	}
	if s2.Name != "owner" {
		t.Fatalf("stmt[2] name: want 'owner', got '%s'", s2.Name)
	}
	if s2.Type != "agent" {
		t.Fatalf("stmt[2] type: want 'agent', got '%s'", s2.Type)
	}

	// bool flag = true;
	s3 := body[3]
	if s3.Kind != "let" {
		t.Fatalf("stmt[3] kind: want 'let', got '%s'", s3.Kind)
	}
	if s3.Name != "flag" {
		t.Fatalf("stmt[3] name: want 'flag', got '%s'", s3.Name)
	}
	if s3.Type != "bool" {
		t.Fatalf("stmt[3] type: want 'bool', got '%s'", s3.Type)
	}
}

// TestTypeFirstVarDeclWithMapping tests type-first decl coexistence with old let syntax.
// Also verifies that uint256 is normalized to u256 by the lexer.
func TestTypeFirstVarDeclWithMapping(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function run() public {
    u256 a = 0;
    uint256 b = 2;
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 3 {
		t.Fatalf("expected 3 stmts, got %d", len(body))
	}
	// let a: u256 = 0  (old syntax) — type as specified
	if body[0].Kind != "let" || body[0].Name != "a" || body[0].Type != "u256" {
		t.Fatalf("stmt[0] mismatch: %#v", body[0])
	}
	// uint256 b = 2  (new type-first syntax) — lexer normalizes uint256 → u256
	if body[1].Kind != "let" || body[1].Name != "b" || body[1].Type != "u256" {
		t.Fatalf("stmt[1] mismatch: %#v", body[1])
	}
	if body[1].Expr == nil || body[1].Expr.Kind != "number" || body[1].Expr.Value != "2" {
		t.Fatalf("stmt[1] init expr mismatch: %#v", body[1].Expr)
	}
}

// TestAgentCallNotTypeDecl tests that address(x) is NOT treated as a type-first var decl.
func TestAgentCallNotTypeDecl(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function run(agent x) public {
    agent y = address(x);
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 2 {
		t.Fatalf("expected 2 stmts, got %d", len(body))
	}
	// The address(x) should be parsed as a call expression in the initializer, not as a type-first decl.
	if body[0].Kind != "let" || body[0].Name != "y" {
		t.Fatalf("stmt[0] mismatch: %#v", body[0])
	}
}

// TestImplicitAssignStatement tests Task #3: bare assignment expressions as statements.
// x = 1; and compound assigns like x += 1; and member/index assigns.
func TestImplicitAssignStatement(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  u256 total_supply;

  function run(u256 n) public {
    u256 x = 0;
    x = 1;
    x += 2;
    x -= 1;
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for implicit assign: %v", diags)
	}
	body := mod.Contract.Functions[0].Body
	// Expect: let x, x=1, x+=2, x-=1, return = 5 stmts
	if len(body) != 5 {
		t.Fatalf("expected 5 stmts, got %d: %#v", len(body), body)
	}

	// x = 1; → expr stmt with assign expr
	s1 := body[1]
	if s1.Kind != "expr" {
		t.Fatalf("stmt[1] (x=1) kind: want 'expr', got '%s'", s1.Kind)
	}
	if s1.Expr == nil || s1.Expr.Kind != "assign" || s1.Expr.Op != "=" {
		t.Fatalf("stmt[1] expr mismatch: %#v", s1.Expr)
	}

	// x += 2; → set stmt with Op="+="
	s2 := body[2]
	if s2.Kind != "set" {
		t.Fatalf("stmt[2] (x+=2) kind: want 'set', got '%s'", s2.Kind)
	}
	if s2.Op != "+=" {
		t.Fatalf("stmt[2] op: want '+=', got '%s'", s2.Op)
	}
	if s2.Expr == nil || s2.Expr.Kind != "number" || s2.Expr.Value != "2" {
		t.Fatalf("stmt[2] rhs mismatch: %#v", s2.Expr)
	}

	// x -= 1; → set stmt with Op="-="
	s3 := body[3]
	if s3.Kind != "set" {
		t.Fatalf("stmt[3] (x-=1) kind: want 'set', got '%s'", s3.Kind)
	}
	if s3.Op != "-=" {
		t.Fatalf("stmt[3] op: want '-=', got '%s'", s3.Op)
	}
}

// TestImplicitAssignMemberAndIndex tests implicit assignment on member access and index expressions.
func TestImplicitAssignMemberAndIndex(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  mapping(agent => u256) balances;

  function run(agent who, u256 value) public {
    balances[who] = value;
    balances[who] -= value;
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for member/index assign: %v", diags)
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 3 {
		t.Fatalf("expected 3 stmts, got %d", len(body))
	}

	// balances[who] = value; → expr stmt with assign expr
	s0 := body[0]
	if s0.Kind != "expr" {
		t.Fatalf("stmt[0] (balances[who]=value) kind: want 'expr', got '%s'", s0.Kind)
	}
	if s0.Expr == nil || s0.Expr.Kind != "assign" {
		t.Fatalf("stmt[0] expr mismatch: %#v", s0.Expr)
	}

	// balances[who] -= value; → set stmt with Op="-="
	s1 := body[1]
	if s1.Kind != "set" {
		t.Fatalf("stmt[1] (balances[who]-=value) kind: want 'set', got '%s'", s1.Kind)
	}
	if s1.Op != "-=" {
		t.Fatalf("stmt[1] op: want '-=', got '%s'", s1.Op)
	}
}

// TestCallOptionsTwoOptions verifies that addr.call{gas: 2300, value: 1}("")
// parses as a call expression with two options in the AST.
func TestCallOptionsTwoOptions(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  agent addr;
  function run(agent recipient) public {
    addr.call{gas: 2300, value: 1}("");
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(body))
	}
	stmt := body[0]
	if stmt.Kind != "expr" {
		t.Fatalf("stmt kind: want 'expr', got '%s'", stmt.Kind)
	}
	callExpr := stmt.Expr
	if callExpr == nil || callExpr.Kind != "call" {
		t.Fatalf("stmt.Expr: want call expr, got %#v", callExpr)
	}
	if len(callExpr.Options) != 2 {
		t.Fatalf("Options count: want 2, got %d", len(callExpr.Options))
	}
	if callExpr.Options[0].Key != "gas" {
		t.Fatalf("Options[0].Key: want 'gas', got '%s'", callExpr.Options[0].Key)
	}
	if callExpr.Options[0].Value == nil || callExpr.Options[0].Value.Kind != "number" || callExpr.Options[0].Value.Value != "2300" {
		t.Fatalf("Options[0].Value: want number 2300, got %#v", callExpr.Options[0].Value)
	}
	if callExpr.Options[1].Key != "value" {
		t.Fatalf("Options[1].Key: want 'value', got '%s'", callExpr.Options[1].Key)
	}
	if callExpr.Options[1].Value == nil || callExpr.Options[1].Value.Kind != "number" || callExpr.Options[1].Value.Value != "1" {
		t.Fatalf("Options[1].Value: want number 1, got %#v", callExpr.Options[1].Value)
	}
	// The single argument is the string ""
	if len(callExpr.Args) != 1 {
		t.Fatalf("Args count: want 1, got %d", len(callExpr.Args))
	}
	if callExpr.Args[0].Kind != "string" {
		t.Fatalf("Args[0].Kind: want 'string', got '%s'", callExpr.Args[0].Kind)
	}
}

// TestCallOptionsSingleOption verifies that token.transfer{value: amount}(to)
// parses as a call expression with a single value option.
func TestCallOptionsSingleOption(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  agent token;
  function run(agent to, uint256 amount) public {
    token.transfer{value: amount}(to);
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(body))
	}
	callExpr := body[0].Expr
	if callExpr == nil || callExpr.Kind != "call" {
		t.Fatalf("stmt.Expr: want call expr, got %#v", callExpr)
	}
	if len(callExpr.Options) != 1 {
		t.Fatalf("Options count: want 1, got %d", len(callExpr.Options))
	}
	if callExpr.Options[0].Key != "value" {
		t.Fatalf("Options[0].Key: want 'value', got '%s'", callExpr.Options[0].Key)
	}
	if callExpr.Options[0].Value == nil || callExpr.Options[0].Value.Kind != "ident" || callExpr.Options[0].Value.Value != "amount" {
		t.Fatalf("Options[0].Value: want ident 'amount', got %#v", callExpr.Options[0].Value)
	}
	// Argument list: (to)
	if len(callExpr.Args) != 1 {
		t.Fatalf("Args count: want 1, got %d", len(callExpr.Args))
	}
	if callExpr.Args[0].Kind != "ident" || callExpr.Args[0].Value != "to" {
		t.Fatalf("Args[0]: want ident 'to', got %#v", callExpr.Args[0])
	}
}

// TestCallOptionsNone verifies that f() with no options block still works (regression test).
func TestCallOptionsNone(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  agent addr;
  function run() public {
    addr.call("");
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(body))
	}
	callExpr := body[0].Expr
	if callExpr == nil || callExpr.Kind != "call" {
		t.Fatalf("stmt.Expr: want call expr, got %#v", callExpr)
	}
	if len(callExpr.Options) != 0 {
		t.Fatalf("Options count: want 0 (no options block), got %d", len(callExpr.Options))
	}
	if len(callExpr.Args) != 1 {
		t.Fatalf("Args count: want 1, got %d", len(callExpr.Args))
	}
}

func TestParseImportNamedWithAlias(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
import { SafeMath as SM, Address } from "./lib.tol";
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for named import with alias: %v", diags)
	}
	if len(mod.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(mod.Imports))
	}
	imp := mod.Imports[0]
	if imp.Path != "./lib.tol" {
		t.Fatalf("unexpected path: %q", imp.Path)
	}
	if len(imp.Named) != 2 {
		t.Fatalf("expected 2 named imports, got %d", len(imp.Named))
	}
	if imp.Named[0].Name != "SafeMath" || imp.Named[0].Alias != "SM" {
		t.Fatalf("unexpected first named import: %+v", imp.Named[0])
	}
	if imp.Named[1].Name != "Address" || imp.Named[1].Alias != "" {
		t.Fatalf("unexpected second named import: %+v", imp.Named[1])
	}
}


// =============================================================================
// Task #12 — Contract body, state var modifiers, UDVT tests
// =============================================================================

// TestParseStateVarVisibility verifies visibility modifiers on state variables.
func TestParseStateVarVisibility(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
    uint256 public totalSupply;
    agent private owner;
    uint256 internal counter;
    uint256 public override balance;
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod.Contract.Storage == nil {
		t.Fatalf("expected storage")
	}
	slots := mod.Contract.Storage.Slots
	if len(slots) != 4 {
		t.Fatalf("expected 4 slots, got %d: %#v", len(slots), slots)
	}
	if slots[0].Name != "totalSupply" || slots[0].Visibility != "public" {
		t.Fatalf("slot[0]: name=%q visibility=%q", slots[0].Name, slots[0].Visibility)
	}
	if slots[1].Name != "owner" || slots[1].Visibility != "private" {
		t.Fatalf("slot[1]: name=%q visibility=%q", slots[1].Name, slots[1].Visibility)
	}
	if slots[2].Name != "counter" || slots[2].Visibility != "internal" {
		t.Fatalf("slot[2]: name=%q visibility=%q", slots[2].Name, slots[2].Visibility)
	}
	if slots[3].Name != "balance" || slots[3].Visibility != "public" || !slots[3].Override {
		t.Fatalf("slot[3]: name=%q visibility=%q override=%v", slots[3].Name, slots[3].Visibility, slots[3].Override)
	}
}

// TestParseStateVarInitialValue verifies inline initializer on state variables.
func TestParseStateVarInitialValue(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
    uint256 x = 1;
    uint256 public maxSupply = 1000000;
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if mod.Contract.Storage == nil {
		t.Fatalf("expected storage")
	}
	slots := mod.Contract.Storage.Slots
	if len(slots) != 2 {
		t.Fatalf("expected 2 slots, got %d", len(slots))
	}
	if slots[0].Name != "x" || slots[0].InitExpr == nil {
		t.Fatalf("slot[0]: name=%q initExpr=%v", slots[0].Name, slots[0].InitExpr)
	}
	if slots[1].Name != "maxSupply" || slots[1].Visibility != "public" || slots[1].InitExpr == nil {
		t.Fatalf("slot[1]: name=%q visibility=%q initExpr=%v", slots[1].Name, slots[1].Visibility, slots[1].InitExpr)
	}
}

// TestParseUDVTInContract verifies type X is Y; inside a contract body.
func TestParseUDVTInContract(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
    type Price is uint256;
    type Status is uint8;
    function foo(Price p) public returns (Price r) {
        return p;
    }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(mod.Contract.TypeDecls) != 2 {
		t.Fatalf("expected 2 type decls, got %d", len(mod.Contract.TypeDecls))
	}
	// The lexer normalizes uint256 → u256 and uint8 → u8 at lex time.
	if mod.Contract.TypeDecls[0].Name != "Price" || mod.Contract.TypeDecls[0].Underlying != "u256" {
		t.Fatalf("unexpected TypeDecl[0]: %+v", mod.Contract.TypeDecls[0])
	}
	if mod.Contract.TypeDecls[1].Name != "Status" || mod.Contract.TypeDecls[1].Underlying != "u8" {
		t.Fatalf("unexpected TypeDecl[1]: %+v", mod.Contract.TypeDecls[1])
	}
}

// TestParseInheritanceWithArgs verifies is Base(arg1, arg2) constructor args.
func TestParseInheritanceWithArgs(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo is Base(1, 2) {
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(mod.Contract.Bases) != 1 || mod.Contract.Bases[0] != "Base" {
		t.Fatalf("expected Bases=[Base], got %v", mod.Contract.Bases)
	}
	if len(mod.Contract.BaseSpecifiers) != 1 {
		t.Fatalf("expected 1 base specifier, got %d", len(mod.Contract.BaseSpecifiers))
	}
	spec := mod.Contract.BaseSpecifiers[0]
	if spec.Name != "Base" {
		t.Fatalf("expected spec.Name=Base, got %q", spec.Name)
	}
	if len(spec.Args) != 2 {
		t.Fatalf("expected 2 constructor args, got %d", len(spec.Args))
	}
}

// TestParseInterfaceFullBody verifies that interfaces accept struct, UDVT, using.
func TestParseInterfaceFullBody(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
interface IFull {
    struct Point { uint256 x; uint256 y; }
    type Price is uint256;
    event Transfer(agent from, agent to, uint256 value);
    error InsufficientBalance(agent account);
    enum Status { Active, Inactive }
    function transfer(agent to, uint256 amount) external returns (bool ok);
}
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(mod.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(mod.Interfaces))
	}
	iface := mod.Interfaces[0]
	if len(iface.Structs) != 1 || iface.Structs[0].Name != "Point" {
		t.Fatalf("expected 1 struct Point, got %#v", iface.Structs)
	}
	if len(iface.TypeDecls) != 1 || iface.TypeDecls[0].Name != "Price" {
		t.Fatalf("expected 1 type decl Price, got %#v", iface.TypeDecls)
	}
	if len(iface.Events) != 1 || iface.Events[0].Name != "Transfer" {
		t.Fatalf("expected 1 event Transfer, got %#v", iface.Events)
	}
	if len(iface.Errors) != 1 || iface.Errors[0].Name != "InsufficientBalance" {
		t.Fatalf("expected 1 error InsufficientBalance, got %#v", iface.Errors)
	}
	if len(iface.Enums) != 1 || iface.Enums[0].Name != "Status" {
		t.Fatalf("expected 1 enum Status, got %#v", iface.Enums)
	}
	if len(iface.Functions) != 1 || iface.Functions[0].Name != "transfer" {
		t.Fatalf("expected 1 function transfer, got %#v", iface.Functions)
	}
}

// TestParseLibraryFullBody verifies that libraries accept events, structs, enums, errors, UDVTs.
func TestParseLibraryFullBody(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
library MathLib {
    struct Fraction { uint256 num; uint256 denom; }
    type Fixed is uint256;
    event Computed(uint256 result);
    error Overflow();
    enum Rounding { Down, Up }
    uint256 constant MAX = 1000000;
    function add(uint256 a, uint256 b) internal pure returns (uint256 c) {
        return a + b;
    }
}
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(mod.Libraries) != 1 {
		t.Fatalf("expected 1 library, got %d", len(mod.Libraries))
	}
	lib := mod.Libraries[0]
	if lib.Name != "MathLib" {
		t.Fatalf("expected lib name MathLib, got %q", lib.Name)
	}
	if len(lib.Structs) != 1 || lib.Structs[0].Name != "Fraction" {
		t.Fatalf("expected 1 struct Fraction, got %#v", lib.Structs)
	}
	if len(lib.TypeDecls) != 1 || lib.TypeDecls[0].Name != "Fixed" {
		t.Fatalf("expected 1 type Fixed, got %#v", lib.TypeDecls)
	}
	if len(lib.Events) != 1 || lib.Events[0].Name != "Computed" {
		t.Fatalf("expected 1 event Computed, got %#v", lib.Events)
	}
	if len(lib.Errors) != 1 || lib.Errors[0].Name != "Overflow" {
		t.Fatalf("expected 1 error Overflow, got %#v", lib.Errors)
	}
	if len(lib.Enums) != 1 || lib.Enums[0].Name != "Rounding" {
		t.Fatalf("expected 1 enum Rounding, got %#v", lib.Enums)
	}
	if len(lib.Constants) != 1 || lib.Constants[0].Name != "MAX" {
		t.Fatalf("expected 1 constant MAX, got %#v", lib.Constants)
	}
	if len(lib.Functions) != 1 || lib.Functions[0].Name != "add" {
		t.Fatalf("expected 1 function add, got %#v", lib.Functions)
	}
}


// ── Task #11: Top-level declarations and all import forms ─────────────────────

// TestParseTopLevelFreeFunction verifies that a free function (outside any contract/library)
// at file level is parsed into mod.FreeFunctions.
func TestParseTopLevelFreeFunction(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
function helper(u256 x) internal pure returns (u256 result) {
  return x;
}
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for top-level free function: %v", diags)
	}
	if len(mod.FreeFunctions) != 1 {
		t.Fatalf("expected 1 free function, got %d", len(mod.FreeFunctions))
	}
	fn := mod.FreeFunctions[0]
	if fn.Name != "helper" {
		t.Fatalf("unexpected free function name: %q", fn.Name)
	}
	if len(fn.Params) != 1 || fn.Params[0].Name != "x" || fn.Params[0].Type != "u256" {
		t.Fatalf("unexpected params: %#v", fn.Params)
	}
	if len(fn.Returns) != 1 || fn.Returns[0].Name != "result" {
		t.Fatalf("unexpected returns: %#v", fn.Returns)
	}
}

// TestParseTopLevelConstant verifies that a constant declared at file level is parsed
// into mod.Constants.
func TestParseTopLevelConstant(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
uint256 constant MAX_SUPPLY = 1000000;
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for top-level constant: %v", diags)
	}
	if len(mod.Constants) != 1 {
		t.Fatalf("expected 1 top-level constant, got %d", len(mod.Constants))
	}
	c := mod.Constants[0]
	if c.Name != "MAX_SUPPLY" {
		t.Fatalf("unexpected constant name: %q", c.Name)
	}
	if c.Type != "u256" { // lexer normalizes uint256 → u256
		t.Fatalf("unexpected constant type: %q", c.Type)
	}
	if c.Value == nil || c.Value.Kind != "number" || c.Value.Value != "1000000" {
		t.Fatalf("unexpected constant value: %+v", c.Value)
	}
}

// TestParseTopLevelEnum verifies that an enum declared at file level is parsed
// into mod.Enums.
func TestParseTopLevelEnum(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
enum Status { Pending, Active, Closed }
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for top-level enum: %v", diags)
	}
	if len(mod.Enums) != 1 {
		t.Fatalf("expected 1 top-level enum, got %d", len(mod.Enums))
	}
	en := mod.Enums[0]
	if en.Name != "Status" {
		t.Fatalf("unexpected enum name: %q", en.Name)
	}
	if len(en.Members) != 3 || en.Members[0] != "Pending" || en.Members[1] != "Active" || en.Members[2] != "Closed" {
		t.Fatalf("unexpected enum members: %v", en.Members)
	}
}

// TestParseTopLevelError verifies that an error declared at file level is parsed
// into mod.Errors.
func TestParseTopLevelError(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
error InsufficientBalance(agent account, u256 needed);
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for top-level error: %v", diags)
	}
	if len(mod.Errors) != 1 {
		t.Fatalf("expected 1 top-level error, got %d", len(mod.Errors))
	}
	ed := mod.Errors[0]
	if ed.Name != "InsufficientBalance" {
		t.Fatalf("unexpected error name: %q", ed.Name)
	}
	if len(ed.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(ed.Params))
	}
	if ed.Params[0].Name != "account" || ed.Params[0].Type != "agent" {
		t.Fatalf("unexpected first param: %+v", ed.Params[0])
	}
	if ed.Params[1].Name != "needed" || ed.Params[1].Type != "u256" {
		t.Fatalf("unexpected second param: %+v", ed.Params[1])
	}
}

// TestParseTopLevelEvent verifies that an event declared at file level is parsed
// into mod.Events.
func TestParseTopLevelEvent(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
event Transfer(agent from indexed, agent to indexed, u256 value)
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for top-level event: %v", diags)
	}
	if len(mod.Events) != 1 {
		t.Fatalf("expected 1 top-level event, got %d", len(mod.Events))
	}
	ev := mod.Events[0]
	if ev.Name != "Transfer" {
		t.Fatalf("unexpected event name: %q", ev.Name)
	}
	if len(ev.Params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(ev.Params))
	}
	if ev.Params[0].Name != "from" || !ev.Params[0].Indexed {
		t.Fatalf("unexpected first param: %+v", ev.Params[0])
	}
	if ev.Params[1].Name != "to" || !ev.Params[1].Indexed {
		t.Fatalf("unexpected second param: %+v", ev.Params[1])
	}
	if ev.Params[2].Name != "value" || ev.Params[2].Indexed {
		t.Fatalf("unexpected third param: %+v", ev.Params[2])
	}
}

// TestParseTopLevelUDVT verifies that a user-defined value type at file level is already
// handled (via mod.TypeDecls).
func TestParseTopLevelUDVT(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
type Price is uint256;
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for top-level UDVT: %v", diags)
	}
	if len(mod.TypeDecls) != 1 {
		t.Fatalf("expected 1 top-level type decl, got %d", len(mod.TypeDecls))
	}
	td := mod.TypeDecls[0]
	if td.Name != "Price" {
		t.Fatalf("unexpected type name: %q", td.Name)
	}
	if td.Underlying != "u256" { // lexer normalizes uint256 → u256
		t.Fatalf("unexpected underlying type: %q", td.Underlying)
	}
}

// TestParseTopLevelUsing verifies that a using-for declaration at file level is parsed
// into mod.UsingDecls.
func TestParseTopLevelUsing(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
library SafeAdd { function add(u256 a, u256 b) internal pure returns (u256 c) { return a; } }
using SafeAdd for u256;
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for top-level using: %v", diags)
	}
	if len(mod.UsingDecls) != 1 {
		t.Fatalf("expected 1 top-level using decl, got %d", len(mod.UsingDecls))
	}
	ud := mod.UsingDecls[0]
	if ud.Library != "SafeAdd" {
		t.Fatalf("unexpected library name: %q", ud.Library)
	}
	if ud.Type != "u256" {
		t.Fatalf("unexpected type: %q", ud.Type)
	}
}

// TestParseTopLevelMixedDecls verifies that multiple different top-level declarations
// can coexist before a contract.
func TestParseTopLevelMixedDecls(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
uint256 constant FEE = 100;
enum Direction { Up, Down }
error BadInput();
event Log(u256 value)
function compute(u256 x) internal pure returns (u256 y) { return x; }
contract Demo {}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for mixed top-level decls: %v", diags)
	}
	if len(mod.Constants) != 1 || mod.Constants[0].Name != "FEE" {
		t.Fatalf("unexpected constants: %v", mod.Constants)
	}
	if len(mod.Enums) != 1 || mod.Enums[0].Name != "Direction" {
		t.Fatalf("unexpected enums: %v", mod.Enums)
	}
	if len(mod.Errors) != 1 || mod.Errors[0].Name != "BadInput" {
		t.Fatalf("unexpected errors: %v", mod.Errors)
	}
	if len(mod.Events) != 1 || mod.Events[0].Name != "Log" {
		t.Fatalf("unexpected events: %v", mod.Events)
	}
	if len(mod.FreeFunctions) != 1 || mod.FreeFunctions[0].Name != "compute" {
		t.Fatalf("unexpected free functions: %v", mod.FreeFunctions)
	}
}

// TestParseTopLevelOnlyNoContract verifies that a file with only top-level declarations
// and no contract is valid (does not require a contract declaration).
func TestParseTopLevelOnlyNoContract(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
uint256 constant MAX = 9999;
error NotAllowed();
function pure_fn(u256 a) internal pure returns (u256 b) { return a; }
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for top-level-only file: %v", diags)
	}
	if mod.Contract != nil {
		t.Fatalf("expected no contract")
	}
	if len(mod.Constants) != 1 || mod.Constants[0].Name != "MAX" {
		t.Fatalf("unexpected constants: %v", mod.Constants)
	}
	if len(mod.Errors) != 1 || mod.Errors[0].Name != "NotAllowed" {
		t.Fatalf("unexpected errors: %v", mod.Errors)
	}
	if len(mod.FreeFunctions) != 1 || mod.FreeFunctions[0].Name != "pure_fn" {
		t.Fatalf("unexpected free functions: %v", mod.FreeFunctions)
	}
}


// ─── Task #13 tests ───────────────────────────────────────────────────────────

// 6.2 — named mapping key/value identifiers are stripped from the canonical type string.
func TestNamedMappingKeyValue(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract C {
    mapping(agent key => uint256 value) balances;
    function run(mapping(agent key => uint256 value) storage m) external {}
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	slot := mod.Contract.Storage.Slots[0]
	const want = "mapping(agent => u256)"
	if slot.Type != want {
		t.Fatalf("slot type: want %q, got %q", want, slot.Type)
	}
	param := mod.Contract.Functions[0].Params[0]
	if param.Type != want {
		t.Fatalf("param type: want %q, got %q", want, param.Type)
	}
}

// 6.5 — dotted identifierPath as a parameter type.
func TestIdentifierPath(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract C {
    function run(IERC20.Token tok) external {}
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	param := mod.Contract.Functions[0].Params[0]
	if param.Type != "IERC20.Token" {
		t.Fatalf("param type: want %q, got %q", "IERC20.Token", param.Type)
	}
}

// 7.6 — delete as a keyword token.
func TestDeleteKeyword(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract C {
    uint256 count;
    function run() external {
        delete count;
    }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(body))
	}
	if body[0].Kind != "delete" {
		t.Fatalf("stmt kind: want 'delete', got %q", body[0].Kind)
	}
}

// 7.3 — inline array literal [1, 2, 3].
func TestInlineArrayExpression(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract C {
    function exec() external returns (uint256 val) {
        val = [10, 20, 30][1];
    }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(body))
	}
	// Statement is "expr" wrapping an "assign" expression.
	stmt := body[0]
	if stmt.Kind != "expr" {
		t.Fatalf("stmt kind: want 'expr', got %q", stmt.Kind)
	}
	assignExpr := stmt.Expr
	if assignExpr == nil || assignExpr.Kind != "assign" {
		t.Fatalf("assign expr: want 'assign', got %#v", assignExpr)
	}
	// The RHS of the assignment should be index(array_lit, 1).
	rhs := assignExpr.Right
	if rhs == nil || rhs.Kind != "index" {
		t.Fatalf("rhs kind: want 'index', got %#v", rhs)
	}
	if rhs.Object == nil || rhs.Object.Kind != "array_lit" {
		t.Fatalf("index object: want 'array_lit', got %#v", rhs.Object)
	}
	if len(rhs.Object.Args) != 3 {
		t.Fatalf("array_lit length: want 3, got %d", len(rhs.Object.Args))
	}
}

// 7.4 — tuple expression (a, b).
func TestTupleExpression(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract C {
    function pair() external returns (uint256 a, uint256 b) {
        return (1, 2);
    }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 1 || body[0].Kind != "return" {
		t.Fatalf("expected 1 return stmt, got %d stmts; kinds=%v", len(body), func() []string {
			var ks []string
			for _, s := range body {
				ks = append(ks, s.Kind)
			}
			return ks
		}())
	}
	expr := body[0].Expr
	if expr == nil || expr.Kind != "tuple" {
		t.Fatalf("return expr kind: want 'tuple', got %#v", expr)
	}
	if len(expr.Args) != 2 {
		t.Fatalf("tuple length: want 2, got %d", len(expr.Args))
	}
}

// 7.2 — named call arguments f({to: alice, amount: 100}).
func TestNamedCallArguments(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract C {
    function execute(agent alice) external {
        transfer({to: alice, amount: 100});
    }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(body))
	}
	expr := body[0].Expr
	if expr == nil || expr.Kind != "named_call" {
		t.Fatalf("expr kind: want 'named_call', got %#v", expr)
	}
	if len(expr.NamedArgs) != 2 {
		t.Fatalf("NamedArgs count: want 2, got %d", len(expr.NamedArgs))
	}
	if expr.NamedArgs[0].Name != "to" {
		t.Fatalf("NamedArgs[0].Name: want 'to', got %q", expr.NamedArgs[0].Name)
	}
	if expr.NamedArgs[1].Name != "amount" {
		t.Fatalf("NamedArgs[1].Name: want 'amount', got %q", expr.NamedArgs[1].Name)
	}
}

// 7.9 — .agent member access on keyword token.
func TestMemberAccessAgentKeyword(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract C {
    agent tok;
    function getAgent() external returns (agent a) {
        a = tok.agent;
    }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	body := mod.Contract.Functions[0].Body
	if len(body) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(body))
	}
	// Statement is "expr" wrapping an "assign" expression.
	stmt := body[0]
	if stmt.Kind != "expr" {
		t.Fatalf("stmt kind: want 'expr', got %q", stmt.Kind)
	}
	assignExpr := stmt.Expr
	if assignExpr == nil || assignExpr.Kind != "assign" {
		t.Fatalf("assign expr: want 'assign', got %#v", assignExpr)
	}
	// The RHS should be a member access with member "agent".
	rhs := assignExpr.Right
	if rhs == nil || rhs.Kind != "member" {
		t.Fatalf("rhs kind: want 'member', got %#v", rhs)
	}
	if rhs.Member != "agent" {
		t.Fatalf("member: want 'agent', got %q", rhs.Member)
	}
}

// 7.8 — type(T).min / type(T).max
func TestTypeMinMax(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract C {
    function limits() external returns (uint256 lo, uint256 hi) {
        lo = type(uint256).min;
        hi = type(uint256).max;
    }
}
`)
	_, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for type(T).min/.max: %v", diags)
	}
}

// 6.6 — function type as parameter type.
func TestFunctionTypeInParam(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract C {
    function invoke(function(agent, uint256) external returns (bool) fn) external {}
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for function type param: %v", diags)
	}
	param := mod.Contract.Functions[0].Params[0]
	if param.Name != "fn" {
		t.Fatalf("param name: want 'fn', got %q", param.Name)
	}
	wantType := "function(agent,u256) external returns(bool)"
	if param.Type != wantType {
		t.Fatalf("param type: want %q, got %q", wantType, param.Type)
	}
}


// --- Task #14 tests ---

// 8.1: Non-block if/else/while/for bodies.
func TestParseNonBlockIfBody(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function check(u256 x) public returns (bool ok) {
    if (x > 0) return true;
    else return false;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	if len(fn.Body) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(fn.Body))
	}
	ifStmt := fn.Body[0]
	if ifStmt.Kind != "if" {
		t.Fatalf("expected 'if' stmt, got '%s'", ifStmt.Kind)
	}
	if len(ifStmt.Then) != 1 || ifStmt.Then[0].Kind != "return" {
		t.Fatalf("expected single return in then, got %v", ifStmt.Then)
	}
	if len(ifStmt.Else) != 1 || ifStmt.Else[0].Kind != "return" {
		t.Fatalf("expected single return in else, got %v", ifStmt.Else)
	}
}

func TestParseNonBlockWhileBody(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function loop() public {
    u256 i = 0;
    while (i < 10) i++;
  }
}
`)
	_, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for non-block while: %v", diags)
	}
}

func TestParseNonBlockForBody(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function loop() public {
    for (u256 i = 0; i < 10; i++) return;
  }
}
`)
	_, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for non-block for: %v", diags)
	}
}

// 8.10: try/catch as reserved keywords, 8.11: try returns clause, 8.12: at least 1 catch.
func TestParseTryCatchReturns(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function call_it() public returns (bool ok) {
    try f() returns (bool r) {
      ok = r;
    } catch {
      ok = false;
    }
    return ok;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	tryStmt := fn.Body[0]
	if tryStmt.Kind != "try" {
		t.Fatalf("expected 'try' stmt, got '%s'", tryStmt.Kind)
	}
	if len(tryStmt.Catches) != 1 {
		t.Fatalf("expected 1 catch, got %d", len(tryStmt.Catches))
	}
}

// 9.1: anonymous event modifier.
func TestParseAnonymousEvent(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  event MyEvent(u256 val) anonymous;
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(mod.Contract.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(mod.Contract.Events))
	}
	ev := mod.Contract.Events[0]
	if !ev.Anonymous {
		t.Fatalf("expected anonymous event")
	}
}

// 9.2: indexed as reserved keyword in event parameter.
func TestParseIndexedKeyword(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  event Transfer(agent from indexed, agent to indexed, u256 value);
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	ev := mod.Contract.Events[0]
	if !ev.Params[0].Indexed {
		t.Fatalf("expected first param to be indexed")
	}
	if !ev.Params[1].Indexed {
		t.Fatalf("expected second param to be indexed")
	}
	if ev.Params[2].Indexed {
		t.Fatalf("expected third param to NOT be indexed")
	}
}

// 10.1: using { A, B } for Type
func TestParseUsingBracedList(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  using { add, sub } for u256;
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for using braced list: %v", diags)
	}
	if len(mod.Contract.UsingDecls) != 1 {
		t.Fatalf("expected 1 using decl, got %d", len(mod.Contract.UsingDecls))
	}
	ud := mod.Contract.UsingDecls[0]
	if ud.Type != "u256" {
		t.Fatalf("expected type 'u256', got '%s'", ud.Type)
	}
}

// 10.2: using LibName for *
func TestParseUsingWildcard(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  using SafeMath for *;
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for using wildcard: %v", diags)
	}
	ud := mod.Contract.UsingDecls[0]
	if ud.Type != "*" {
		t.Fatalf("expected type '*', got '%s'", ud.Type)
	}
}

// 10.3: using LibName for Type global
func TestParseUsingGlobal(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  using SafeMath for u256 global;
}
`)
	_, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for using global: %v", diags)
	}
}

// 11.1: modifier without param list
func TestParseModifierNoParens(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  modifier onlyOwner {
    _;
  }
  function doThing() public onlyOwner {
    return;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for modifier without parens: %v", diags)
	}
	if len(mod.Contract.Modifiers) != 1 {
		t.Fatalf("expected 1 modifier, got %d", len(mod.Contract.Modifiers))
	}
	m := mod.Contract.Modifiers[0]
	if m.Name != "onlyOwner" {
		t.Fatalf("expected modifier name 'onlyOwner', got '%s'", m.Name)
	}
}

// 11.2: modifier with virtual/override
func TestParseModifierVirtualOverride(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
abstract contract Base {
  modifier auth() virtual {
    _;
  }
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for virtual modifier: %v", diags)
	}
	if len(mod.AbstractContracts) != 1 {
		t.Fatalf("expected 1 abstract contract, got %d", len(mod.AbstractContracts))
	}
	m := mod.AbstractContracts[0].Modifiers[0]
	if !m.Virtual {
		t.Fatalf("expected modifier to be virtual")
	}
}

// 11.3: abstract modifier (semicolon body)
func TestParseModifierAbstract(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
abstract contract Base {
  modifier auth() virtual;
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for abstract modifier: %v", diags)
	}
	m := mod.AbstractContracts[0].Modifiers[0]
	if !m.Abstract {
		t.Fatalf("expected modifier to be abstract (semicolon body)")
	}
}

// 12.10: unicode string literal
func TestParseUnicodeStringLiteral(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function greet() public returns (string s) {
    s = unicode"Hello 🌍";
    return s;
  }
}
`)
	_, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for unicode string: %v", diags)
	}
}

// 12.11: octal literal detection
func TestParseOctalLiteralError(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function bad() public returns (u256 x) {
    x = 07;
    return x;
  }
}
`)
	_, diags := ParseFile("<test>", src)
	if !diags.HasErrors() {
		t.Fatalf("expected error for octal literal '07'")
	}
}

// Keyword promotion: true/false as keywords in expressions
func TestParseTrueFalseKeywords(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function check() public returns (bool ok) {
    ok = true;
    return false;
  }
}
`)
	_, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for true/false keywords: %v", diags)
	}
}

// Keyword promotion: new as keyword in expressions
func TestParseNewKeyword(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  function deploy() public {
    try new Token(1000) {
    } catch {
    }
  }
}
`)
	_, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for 'new' keyword: %v", diags)
	}
}

// Keyword promotion: agent as type keyword
func TestParseAgentKeyword(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
  agent owner;
  function getOwner() public returns (agent a) {
    return owner;
  }
}
`)
	_, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for 'address' keyword: %v", diags)
	}
}

// Keyword promotion: is as keyword in inheritance
func TestParseIsKeyword(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
interface IERC20 {
  function transfer(agent to, u256 amount) external returns (bool ok);
}
contract Token is IERC20 {
  function transfer(agent to, u256 amount) external returns (bool ok) {
    return true;
  }
}
`)
	_, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for 'is' keyword: %v", diags)
	}
}

func TestParseDocMetaGuards(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
    modifier onlyOwner() {
        require(msg.sender == owner);
        _;
    }
    /// @effects guards: [onlyOwner], writes: [storage.balance]
    function withdraw() public onlyOwner {}
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	if mod.Contract == nil || len(mod.Contract.Functions) == 0 {
		t.Fatal("expected function")
	}
	fn := mod.Contract.Functions[0]
	if fn.Doc == nil {
		t.Fatal("expected Doc to be set on function")
	}
	if fn.Doc.Effects == nil {
		t.Fatal("expected Effects")
	}
	if len(fn.Doc.Effects.Guards) != 1 || fn.Doc.Effects.Guards[0] != "onlyOwner" {
		t.Errorf("expected Guards=[onlyOwner], got %v", fn.Doc.Effects.Guards)
	}
	if len(fn.Doc.Effects.Writes) == 0 || fn.Doc.Effects.Writes[0] != "storage.balance" {
		t.Errorf("unexpected writes: %v", fn.Doc.Effects.Writes)
	}
}

func TestParseDocMetaGuardsMultiLine(t *testing.T) {
	src := []byte(`
pragma tolang 0.2.0;
contract Demo {
    modifier onlyOwner() { require(msg.sender == owner); _; }
    modifier whenActive() { require(active); _; }
    /// @effects guards: [onlyOwner, whenActive]
    /// @effects writes: [storage.x]
    function doStuff() public onlyOwner whenActive {}
}
`)
	mod, diags := ParseFile("<test>", src)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %v", diags)
	}
	fn := mod.Contract.Functions[0]
	if fn.Doc == nil || fn.Doc.Effects == nil {
		t.Fatal("expected Doc.Effects")
	}
	if len(fn.Doc.Effects.Guards) != 2 {
		t.Fatalf("expected 2 guards, got %d: %v", len(fn.Doc.Effects.Guards), fn.Doc.Effects.Guards)
	}
	if fn.Doc.Effects.Guards[0] != "onlyOwner" || fn.Doc.Effects.Guards[1] != "whenActive" {
		t.Errorf("unexpected guards: %v", fn.Doc.Effects.Guards)
	}
}
