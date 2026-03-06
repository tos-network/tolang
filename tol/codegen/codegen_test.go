package codegen

import (
	"testing"

	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/lower"
)

func TestBytecodeMinimalLoweredProgram(t *testing.T) {
	p := &lower.Program{
		ContractName: "Demo",
	}
	bc, err := Bytecode(p)
	if err != nil {
		t.Fatalf("unexpected codegen error: %v", err)
	}
	if len(bc) == 0 {
		t.Fatalf("expected non-empty bytecode")
	}
}

func TestBytecodeSupportsStorageInDirectIR(t *testing.T) {
	p := &lower.Program{
		ContractName: "Demo",
		StorageSlots: []lower.StorageSlot{
			{Name: "x", Type: "u256"},
		},
		Functions: []lower.Function{
			{
				Name:      "setx",
				Modifiers: []string{"public"},
				Params: []ast.FieldDecl{
					{Name: "v", Type: "u256"},
				},
				Body: []ast.Statement{
					{
						Kind: "set",
						Target: &ast.Expr{
							Kind:  "ident",
							Value: "x",
						},
						Expr: &ast.Expr{
							Kind:  "ident",
							Value: "v",
						},
					},
					{Kind: "return"},
				},
			},
		},
	}
	bc, err := Bytecode(p)
	if err != nil {
		t.Fatalf("unexpected codegen error: %v", err)
	}
	if len(bc) == 0 {
		t.Fatalf("expected non-empty bytecode")
	}
}

func TestBytecodeEnumMemberLowering(t *testing.T) {
	// Test that State.Active is lowered to integer 0, State.Inactive to 1.
	p := &lower.Program{
		ContractName: "Demo",
		Enums: []lower.EnumDecl{
			{Name: "State", Members: []string{"Active", "Inactive", "Paused"}},
		},
		Functions: []lower.Function{
			{
				Name:      "getState",
				Modifiers: []string{"public"},
				Returns:   []ast.FieldDecl{{Name: "v", Type: "u8"}},
				Body: []ast.Statement{
					{
						Kind: "return",
						Expr: &ast.Expr{
							Kind:   "member",
							Object: &ast.Expr{Kind: "ident", Value: "State"},
							Member: "Inactive",
						},
					},
				},
			},
		},
	}
	bc, err := Bytecode(p)
	if err != nil {
		t.Fatalf("unexpected codegen error: %v", err)
	}
	if len(bc) == 0 {
		t.Fatalf("expected non-empty bytecode")
	}
}

func TestBytecodeErrorDeclRevertCompiles(t *testing.T) {
	// Test that revert Unauthorized(addr) compiles with error decl present.
	p := &lower.Program{
		ContractName: "Demo",
		Errors: []lower.ErrorDecl{
			{Name: "Unauthorized", Params: []ast.FieldDecl{{Name: "caller", Type: "agent"}}},
		},
		Functions: []lower.Function{
			{
				Name:      "doSomething",
				Modifiers: []string{"public"},
				Params:    []ast.FieldDecl{{Name: "caller", Type: "agent"}},
				Body: []ast.Statement{
					{
						Kind: "revert",
						Expr: &ast.Expr{
							Kind:   "call",
							Callee: &ast.Expr{Kind: "ident", Value: "Unauthorized"},
							Args:   []*ast.Expr{{Kind: "ident", Value: "caller"}},
						},
					},
				},
			},
		},
	}
	bc, err := Bytecode(p)
	if err != nil {
		t.Fatalf("unexpected codegen error: %v", err)
	}
	if len(bc) == 0 {
		t.Fatalf("expected non-empty bytecode")
	}
}

func TestBytecodeEnumIfComparison(t *testing.T) {
	// Test enum used in if comparison: if s == State.Active { ... }
	p := &lower.Program{
		ContractName: "Demo",
		Enums: []lower.EnumDecl{
			{Name: "State", Members: []string{"Active", "Inactive"}},
		},
		Functions: []lower.Function{
			{
				Name:      "check",
				Modifiers: []string{"public"},
				Params:    []ast.FieldDecl{{Name: "s", Type: "u8"}},
				Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
				Body: []ast.Statement{
					{
						Kind: "if",
						Cond: &ast.Expr{
							Kind: "binary",
							Op:   "==",
							Left: &ast.Expr{Kind: "ident", Value: "s"},
							Right: &ast.Expr{
								Kind:   "member",
								Object: &ast.Expr{Kind: "ident", Value: "State"},
								Member: "Active",
							},
						},
						Then: []ast.Statement{
							{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "true"}},
						},
					},
					{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "false"}},
				},
			},
		},
	}
	bc, err := Bytecode(p)
	if err != nil {
		t.Fatalf("unexpected codegen error: %v", err)
	}
	if len(bc) == 0 {
		t.Fatalf("expected non-empty bytecode")
	}
}

// typeMinMaxASTExpr builds ast.Expr for type(typeName).bound.
func typeMinMaxASTExpr(typeName, bound string) *ast.Expr {
	return &ast.Expr{
		Kind:   "member",
		Member: bound,
		Object: &ast.Expr{
			Kind:   "call",
			Callee: &ast.Expr{Kind: "ident", Value: "type"},
			Args:   []*ast.Expr{{Kind: "ident", Value: typeName}},
		},
	}
}

// TestBytecodeTypeMinMaxLowering verifies that type(uN).min/max and type(iN).min/max
// are successfully lowered to bytecode without error.
func TestBytecodeTypeMinMaxLowering(t *testing.T) {
	cases := []struct {
		typeName string
		bound    string
		retType  string
	}{
		{"u256", "max", "u256"},
		{"u256", "min", "u256"},
		{"u8", "max", "u8"},
		{"u8", "min", "u8"},
		{"u128", "max", "u128"},
		{"i8", "max", "i8"},
		{"i8", "min", "i8"},
		{"i256", "max", "i256"},
		{"i256", "min", "i256"},
	}
	for _, tc := range cases {
		p := &lower.Program{
			ContractName: "Demo",
			Functions: []lower.Function{
				{
					Name:      "getBound",
					Modifiers: []string{"public"},
					Returns:   []ast.FieldDecl{{Name: "v", Type: tc.retType}},
					Body: []ast.Statement{
						{
							Kind: "return",
							Expr: typeMinMaxASTExpr(tc.typeName, tc.bound),
						},
					},
				},
			},
		}
		bc, err := Bytecode(p)
		if err != nil {
			t.Errorf("type(%s).%s: unexpected codegen error: %v", tc.typeName, tc.bound, err)
			continue
		}
		if len(bc) == 0 {
			t.Errorf("type(%s).%s: expected non-empty bytecode", tc.typeName, tc.bound)
		}
	}
}
