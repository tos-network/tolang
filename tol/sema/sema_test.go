package sema

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tos-network/tolang/tol/ast"
)

// mapResolver is a simple in-memory FileResolver used by tests.
// It maps canonical file names to source bytes.
type mapResolver struct {
	files map[string][]byte
}

func newMapResolver(files map[string][]byte) *mapResolver {
	return &mapResolver{files: files}
}

func (r *mapResolver) Resolve(importingFile, importPath, importName string) ([]byte, string, error) {
	// Use the import path directly as the canonical name.
	src, ok := r.files[importPath]
	if !ok {
		return nil, "", fmt.Errorf("file not found: %q", importPath)
	}
	return src, importPath, nil
}

func TestCheckMinimal(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
		},
	}
	typed, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if typed == nil || typed.AST == nil {
		t.Fatalf("expected typed module")
	}
}

func TestCheckAllowsConstructorParams(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Constructor: &ast.ConstructorDecl{
				Params: []ast.FieldDecl{
					{Name: "owner", Type: "address"},
				},
			},
		},
	}
	typed, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if typed == nil || typed.AST == nil || typed.AST.Contract == nil {
		t.Fatalf("expected typed module")
	}
}

func TestCheckRejectsDuplicates(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					{Name: "x", Type: "u256"},
					{Name: "x", Type: "u256"},
				},
			},
			Functions: []ast.FunctionDecl{
				{Name: "transfer"},
				{Name: "transfer"},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
}

func TestCheckBreakContinueOutsideLoop(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{Kind: "break"},
						{Kind: "continue"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
}

func TestCheckSetTargetMustBeAssignable(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind: "binary",
								Op:   "+",
								Left: &ast.Expr{Kind: "ident", Value: "a"},
								Right: &ast.Expr{
									Kind:  "ident",
									Value: "b",
								},
							},
							Expr: &ast.Expr{Kind: "number", Value: "1"},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
}

func TestCheckRejectsDuplicateLocalLetInSameScope(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{Kind: "let", Name: "x", Type: "u256", Expr: &ast.Expr{Kind: "number", Value: "1"}},
						{Kind: "let", Name: "x", Type: "u256", Expr: &ast.Expr{Kind: "number", Value: "2"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2028") {
		t.Fatalf("expected TOL2028, got: %v", diags)
	}
}

func TestCheckRejectsLocalLetCollidingWithParamInSameScope(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Params: []ast.FieldDecl{
						{Name: "x", Type: "u256"},
					},
					Body: []ast.Statement{
						{Kind: "let", Name: "x", Type: "u256", Expr: &ast.Expr{Kind: "number", Value: "1"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2028") {
		t.Fatalf("expected TOL2028, got: %v", diags)
	}
}

func TestCheckAllowsLocalShadowingInNestedScope(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{Kind: "let", Name: "x", Type: "u256", Expr: &ast.Expr{Kind: "number", Value: "1"}},
						{
							Kind: "if",
							Cond: &ast.Expr{Kind: "ident", Value: "true"},
							Then: []ast.Statement{
								{Kind: "let", Name: "x", Type: "u256", Expr: &ast.Expr{Kind: "number", Value: "2"}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsSetTargetReservedLiteralIdent(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "true"},
							Expr:   &ast.Expr{Kind: "number", Value: "1"},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2008") {
		t.Fatalf("expected TOL2008, got: %v", diags)
	}
}

func TestCheckRejectsSetTargetSelectorMemberExpr(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "mark",
					Modifiers: []string{"public"},
				},
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind:   "member",
								Member: "selector",
								Object: &ast.Expr{
									Kind:   "member",
									Member: "mark",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "this",
									},
								},
							},
							Expr: &ast.Expr{Kind: "number", Value: "1"},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2008") {
		t.Fatalf("expected TOL2008, got: %v", diags)
	}
}

func TestCheckRejectsAssignExprTargetSelectorMemberExpr(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "mark",
					Modifiers: []string{"public"},
				},
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "assign",
								Left: &ast.Expr{
									Kind:   "member",
									Member: "selector",
									Object: &ast.Expr{
										Kind:   "member",
										Member: "mark",
										Object: &ast.Expr{
											Kind:  "ident",
											Value: "this",
										},
									},
								},
								Right: &ast.Expr{Kind: "number", Value: "1"},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2008") {
		t.Fatalf("expected TOL2008, got: %v", diags)
	}
}

func TestCheckRejectsInvalidSelectorOverride(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:             "f",
					SelectorOverride: "0x123",
					Modifiers:        []string{"public"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2010") {
		t.Fatalf("expected TOL2010, got: %v", diags)
	}
}

func TestCheckRejectsDuplicatePublicExternalSelector(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:             "a",
					SelectorOverride: "0x11111111",
					Modifiers:        []string{"public"},
				},
				{
					Name:             "b",
					SelectorOverride: "0x11111111",
					Modifiers:        []string{"external"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2011") {
		t.Fatalf("expected TOL2011, got: %v", diags)
	}
}

func TestCheckRejectsSelectorOverrideOnNonExternalFunction(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:             "f",
					SelectorOverride: "0x11111111",
					Modifiers:        []string{"internal"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2027") {
		t.Fatalf("expected TOL2027, got: %v", diags)
	}
}

func TestCheckAcceptsSelectorOverrideOnExternalFunction(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:             "f",
					SelectorOverride: "0x11111111",
					Modifiers:        []string{"external"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckAcceptsSelectorBuiltinNonLiteral(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind:  "ident",
								Value: "x",
							},
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "selector",
								},
								Args: []*ast.Expr{
									{Kind: "ident", Value: "sig"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckAcceptsSelectorBuiltinNonLiteralWithParenCallee(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind:  "ident",
								Value: "x",
							},
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind: "paren",
									Left: &ast.Expr{
										Kind:  "ident",
										Value: "selector",
									},
								},
								Args: []*ast.Expr{
									{Kind: "ident", Value: "sig"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsSelectorBuiltinEmptyLiteral(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind:  "ident",
								Value: "x",
							},
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "selector",
								},
								Args: []*ast.Expr{
									{Kind: "string", Value: "\"\""},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2012") {
		t.Fatalf("expected TOL2012, got: %v", diags)
	}
}

func TestCheckRejectsSelectorBuiltinMalformedLiteral(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind:  "ident",
								Value: "x",
							},
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "selector",
								},
								Args: []*ast.Expr{
									{Kind: "string", Value: "\"transfer\""},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2012") {
		t.Fatalf("expected TOL2012, got: %v", diags)
	}
}

func TestCheckRejectsSelectorBuiltinMalformedArgListLiteral(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind:  "ident",
								Value: "x",
							},
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "selector",
								},
								Args: []*ast.Expr{
									{Kind: "string", Value: "\"f(,)\""},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2012") {
		t.Fatalf("expected TOL2012, got: %v", diags)
	}
}

func TestCheckRejectsSelectorBuiltinNonCanonicalSpacingLiteral(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind:  "ident",
								Value: "x",
							},
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "selector",
								},
								Args: []*ast.Expr{
									{Kind: "string", Value: "\"transfer(address, u256)\""},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2012") {
		t.Fatalf("expected TOL2012, got: %v", diags)
	}
}

func TestCheckRejectsSelectorBuiltinLeadingWhitespaceLiteral(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind:  "ident",
								Value: "x",
							},
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "selector",
								},
								Args: []*ast.Expr{
									{Kind: "string", Value: "\" transfer(address,u256)\""},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2012") {
		t.Fatalf("expected TOL2012, got: %v", diags)
	}
}

func TestCheckRejectsSelectorBuiltinNameWhitespaceBeforeParen(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind:  "ident",
								Value: "x",
							},
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "selector",
								},
								Args: []*ast.Expr{
									{Kind: "string", Value: "\"transfer (address,u256)\""},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2012") {
		t.Fatalf("expected TOL2012, got: %v", diags)
	}
}

func TestCheckRejectsSelectorBuiltinInvalidFunctionNameLiteral(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind:  "ident",
								Value: "x",
							},
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "selector",
								},
								Args: []*ast.Expr{
									{Kind: "string", Value: "\"1f(u256)\""},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2012") {
		t.Fatalf("expected TOL2012, got: %v", diags)
	}
}

func TestCheckRejectsSelectorMemberUnknownTarget(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "x"},
							Expr: &ast.Expr{
								Kind:   "member",
								Member: "selector",
								Object: &ast.Expr{
									Kind:   "member",
									Member: "missing",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "this",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2013") {
		t.Fatalf("expected TOL2013, got: %v", diags)
	}
}

func TestCheckRejectsSelectorMemberNonExternalTarget(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "hidden",
					Modifiers: []string{"internal"},
				},
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "x"},
							Expr: &ast.Expr{
								Kind:   "member",
								Member: "selector",
								Object: &ast.Expr{
									Kind:   "member",
									Member: "hidden",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "Demo",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2013") {
		t.Fatalf("expected TOL2013, got: %v", diags)
	}
}

func TestCheckAcceptsSelectorMemberExternalTarget(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "pub",
					Modifiers: []string{"public"},
				},
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "x"},
							Expr: &ast.Expr{
								Kind:   "member",
								Member: "selector",
								Object: &ast.Expr{
									Kind:   "member",
									Member: "pub",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "this",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckAcceptsSelectorMemberExternalTargetWithParens(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "pub",
					Modifiers: []string{"public"},
				},
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "x"},
							Expr: &ast.Expr{
								Kind:   "member",
								Member: "selector",
								Object: &ast.Expr{
									Kind: "paren",
									Left: &ast.Expr{
										Kind:   "member",
										Member: "pub",
										Object: &ast.Expr{
											Kind: "paren",
											Left: &ast.Expr{
												Kind:  "ident",
												Value: "this",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsCallingSelectorBuiltinResult(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind: "call",
									Callee: &ast.Expr{
										Kind:  "ident",
										Value: "selector",
									},
									Args: []*ast.Expr{
										{Kind: "string", Value: "\"transfer(address,u256)\""},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2013") {
		t.Fatalf("expected TOL2013, got: %v", diags)
	}
}

func TestCheckRejectsCallingSelectorMemberResult(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "pub",
					Modifiers: []string{"public"},
				},
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Member: "selector",
									Object: &ast.Expr{
										Kind:   "member",
										Member: "pub",
										Object: &ast.Expr{
											Kind:  "ident",
											Value: "this",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2013") {
		t.Fatalf("expected TOL2013, got: %v", diags)
	}
}

func TestCheckRejectsUnknownFunctionModifier(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "f",
					Modifiers: []string{"onlyOwner"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	// TOL2038 = unknown modifier (not declared in contract and not a built-in keyword).
	if !strings.Contains(diags.Error(), "TOL2038") {
		t.Fatalf("expected TOL2038, got: %v", diags)
	}
}

func TestCheckRejectsReservedFunctionNameSelector(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "selector",
					Body: []ast.Statement{
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2033") {
		t.Fatalf("expected TOL2033, got: %v", diags)
	}
}

func TestCheckRejectsReservedFunctionNameThis(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "this",
					Body: []ast.Statement{
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2033") {
		t.Fatalf("expected TOL2033, got: %v", diags)
	}
}

func TestCheckRejectsReservedFunctionNamePrefixTol(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "__tol_internal",
					Body: []ast.Statement{
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2033") {
		t.Fatalf("expected TOL2033, got: %v", diags)
	}
}

func TestCheckRejectsReservedEventAndStorageNames(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					{Name: "selector", Type: "u256"},
					{Name: "this", Type: "u256"},
					{Name: "__tol_internal", Type: "u256"},
				},
			},
			Events: []ast.EventDecl{
				{Name: "selector"},
				{Name: "this"},
				{Name: "__tol_internal"},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2033") {
		t.Fatalf("expected TOL2033, got: %v", diags)
	}
}

func TestCheckRejectsReservedContractName(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "this",
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2033") {
		t.Fatalf("expected TOL2033, got: %v", diags)
	}
}

func TestCheckRejectsReservedContractNameSelector(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "selector",
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2033") {
		t.Fatalf("expected TOL2033, got: %v", diags)
	}
}

func TestCheckRejectsReservedContractNamePrefixTol(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "__tol_demo",
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2033") {
		t.Fatalf("expected TOL2033, got: %v", diags)
	}
}

func TestCheckRejectsConflictingVisibilityModifiers(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "f",
					Modifiers: []string{"public", "external"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2015") {
		t.Fatalf("expected TOL2015, got: %v", diags)
	}
}

func TestCheckRejectsDuplicateVisibilityModifier(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "f",
					Modifiers: []string{"public", "public"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2015") {
		t.Fatalf("expected TOL2015, got: %v", diags)
	}
}

func TestCheckRejectsConflictingMutabilityModifiers(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "f",
					Modifiers: []string{"view", "payable"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2015") {
		t.Fatalf("expected TOL2015, got: %v", diags)
	}
}

func TestCheckRejectsDuplicateMutabilityModifier(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "f",
					Modifiers: []string{"view", "view"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2015") {
		t.Fatalf("expected TOL2015, got: %v", diags)
	}
}

func TestCheckRejectsDuplicateFunctionParams(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Params: []ast.FieldDecl{
						{Name: "x", Type: "u256"},
						{Name: "x", Type: "u256"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2016") {
		t.Fatalf("expected TOL2016, got: %v", diags)
	}
}

func TestCheckRejectsDuplicateConstructorParams(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Constructor: &ast.ConstructorDecl{
				Params: []ast.FieldDecl{
					{Name: "owner", Type: "address"},
					{Name: "owner", Type: "address"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2016") {
		t.Fatalf("expected TOL2016, got: %v", diags)
	}
}

func TestCheckRejectsUnknownConstructorModifier(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Constructor: &ast.ConstructorDecl{
				Modifiers: []string{"onlyOwner"},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2014") {
		t.Fatalf("expected TOL2014, got: %v", diags)
	}
}

func TestCheckRejectsConflictingConstructorVisibility(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Constructor: &ast.ConstructorDecl{
				Modifiers: []string{"public", "internal"},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2015") {
		t.Fatalf("expected TOL2015, got: %v", diags)
	}
}

func TestCheckRejectsDuplicateConstructorVisibility(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Constructor: &ast.ConstructorDecl{
				Modifiers: []string{"public", "public"},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2015") {
		t.Fatalf("expected TOL2015, got: %v", diags)
	}
}

func TestCheckAcceptsConstructorPayableModifier(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Constructor: &ast.ConstructorDecl{
				Modifiers: []string{"payable"},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsReturnValueInVoidFunction(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "return",
							Expr: &ast.Expr{Kind: "number", Value: "1"},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2017") {
		t.Fatalf("expected TOL2017, got: %v", diags)
	}
}

func TestCheckRejectsMissingReturnValueInNonVoidFunction(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Returns: []ast.FieldDecl{
						{Name: "ok", Type: "bool"},
					},
					Body: []ast.Statement{
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2017") {
		t.Fatalf("expected TOL2017, got: %v", diags)
	}
}

func TestCheckRejectsNonVoidFunctionWithoutAnyReturnStmt(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Returns: []ast.FieldDecl{
						{Name: "ok", Type: "bool"},
					},
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "x",
							Type: "u256",
							Expr: &ast.Expr{Kind: "number", Value: "1"},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2017") {
		t.Fatalf("expected TOL2017, got: %v", diags)
	}
}

func TestCheckRejectsNonVoidFunctionMissingReturnOnSomePath(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Params: []ast.FieldDecl{
						{Name: "x", Type: "u256"},
					},
					Returns: []ast.FieldDecl{
						{Name: "ok", Type: "bool"},
					},
					Body: []ast.Statement{
						{
							Kind: "if",
							Cond: &ast.Expr{Kind: "binary", Op: ">", Left: &ast.Expr{Kind: "ident", Value: "x"}, Right: &ast.Expr{Kind: "number", Value: "0"}},
							Then: []ast.Statement{
								{Kind: "return", Expr: &ast.Expr{Kind: "number", Value: "1"}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2017") {
		t.Fatalf("expected TOL2017, got: %v", diags)
	}
}

func TestCheckAcceptsNonVoidFunctionAllPathsReturnOrRevert(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Params: []ast.FieldDecl{
						{Name: "x", Type: "u256"},
					},
					Returns: []ast.FieldDecl{
						{Name: "ok", Type: "bool"},
					},
					Body: []ast.Statement{
						{
							Kind: "if",
							Cond: &ast.Expr{Kind: "binary", Op: ">", Left: &ast.Expr{Kind: "ident", Value: "x"}, Right: &ast.Expr{Kind: "number", Value: "0"}},
							Then: []ast.Statement{
								{Kind: "return", Expr: &ast.Expr{Kind: "number", Value: "1"}},
							},
							Else: []ast.Statement{
								{Kind: "revert", Expr: &ast.Expr{Kind: "string", Value: "\"NO\""}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckAcceptsNonVoidFunctionInfiniteWhileWithGuaranteedReturn(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Returns: []ast.FieldDecl{
						{Name: "out", Type: "u256"},
					},
					Body: []ast.Statement{
						{
							Kind: "while",
							Cond: &ast.Expr{Kind: "ident", Value: "true"},
							Body: []ast.Statement{
								{Kind: "return", Expr: &ast.Expr{Kind: "number", Value: "1"}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckAcceptsNonVoidFunctionInfiniteForWithGuaranteedRevert(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Returns: []ast.FieldDecl{
						{Name: "out", Type: "u256"},
					},
					Body: []ast.Statement{
						{
							Kind: "for",
							Body: []ast.Statement{
								{Kind: "revert", Expr: &ast.Expr{Kind: "string", Value: "\"X\""}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckAcceptsNonVoidFunctionInfiniteForTrueWithGuaranteedReturn(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Returns: []ast.FieldDecl{
						{Name: "out", Type: "u256"},
					},
					Body: []ast.Statement{
						{
							Kind: "for",
							Cond: &ast.Expr{Kind: "ident", Value: "true"},
							Body: []ast.Statement{
								{Kind: "return", Expr: &ast.Expr{Kind: "number", Value: "7"}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsUnreachableStmtAfterReturn(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{Kind: "return"},
						{Kind: "let", Name: "x", Type: "u256", Expr: &ast.Expr{Kind: "number", Value: "1"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2030") {
		t.Fatalf("expected TOL2030, got: %v", diags)
	}
}

func TestCheckRejectsUnreachableStmtAfterTerminatingIf(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "if",
							Cond: &ast.Expr{Kind: "ident", Value: "flag"},
							Then: []ast.Statement{{Kind: "return"}},
							Else: []ast.Statement{{Kind: "revert", Expr: &ast.Expr{Kind: "string", Value: "\"X\""}}},
						},
						{Kind: "let", Name: "x", Type: "u256", Expr: &ast.Expr{Kind: "number", Value: "1"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2030") {
		t.Fatalf("expected TOL2030, got: %v", diags)
	}
}

func TestCheckRejectsUnreachableStmtAfterTerminatingInfiniteWhile(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "while",
							Cond: &ast.Expr{Kind: "ident", Value: "true"},
							Body: []ast.Statement{
								{Kind: "return"},
							},
						},
						{Kind: "let", Name: "x", Type: "u256", Expr: &ast.Expr{Kind: "number", Value: "1"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2030") {
		t.Fatalf("expected TOL2030, got: %v", diags)
	}
}

func TestCheckAllowsStmtAfterNonTerminatingIf(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "if",
							Cond: &ast.Expr{Kind: "ident", Value: "flag"},
							Then: []ast.Statement{{Kind: "return"}},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsUnreachableStmtAfterBreakInLoop(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "while",
							Cond: &ast.Expr{Kind: "ident", Value: "true"},
							Body: []ast.Statement{
								{Kind: "break"},
								{Kind: "let", Name: "x", Type: "u256", Expr: &ast.Expr{Kind: "number", Value: "1"}},
							},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2030") {
		t.Fatalf("expected TOL2030, got: %v", diags)
	}
}

func TestCheckRejectsUnreachableStmtAfterContinueInLoop(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "while",
							Cond: &ast.Expr{Kind: "ident", Value: "true"},
							Body: []ast.Statement{
								{Kind: "continue"},
								{Kind: "let", Name: "x", Type: "u256", Expr: &ast.Expr{Kind: "number", Value: "1"}},
							},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2030") {
		t.Fatalf("expected TOL2030, got: %v", diags)
	}
}

func TestCheckRejectsConstructorReturnValue(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Constructor: &ast.ConstructorDecl{
				Body: []ast.Statement{
					{
						Kind: "return",
						Expr: &ast.Expr{Kind: "number", Value: "1"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2017") {
		t.Fatalf("expected TOL2017, got: %v", diags)
	}
}

func TestCheckRejectsFallbackReturnValue(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Fallback: &ast.FallbackDecl{
				Body: []ast.Statement{
					{
						Kind: "return",
						Expr: &ast.Expr{Kind: "number", Value: "1"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2017") {
		t.Fatalf("expected TOL2017, got: %v", diags)
	}
}

func TestCheckRejectsDuplicateTopLevelSupportDeclName(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		SkippedTopDecls: []ast.SkippedTopDecl{
			{Kind: "interface", Name: "ICommon"},
			{Kind: "library", Name: "ICommon"},
		},
		Contract: &ast.ContractDecl{
			Name: "Demo",
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2026") {
		t.Fatalf("expected TOL2026, got: %v", diags)
	}
}

func TestCheckRejectsContractTopLevelNameCollision(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		SkippedTopDecls: []ast.SkippedTopDecl{
			{Kind: "interface", Name: "Demo"},
		},
		Contract: &ast.ContractDecl{
			Name: "Demo",
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2026") {
		t.Fatalf("expected TOL2026, got: %v", diags)
	}
}

func TestCheckDuplicateFunctionDoesNotCascadeDuplicateSelectorNoise(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Params: []ast.FieldDecl{
						{Name: "x", Type: "u256"},
					},
					Modifiers: []string{"public"},
					Body:      []ast.Statement{{Kind: "return"}},
				},
				{
					Name: "f",
					Params: []ast.FieldDecl{
						{Name: "x", Type: "u256"},
					},
					Modifiers: []string{"public"},
					Body:      []ast.Statement{{Kind: "return"}},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	msg := diags.Error()
	if !strings.Contains(msg, "TOL2004") {
		t.Fatalf("expected duplicate function error, got: %v", diags)
	}
	if strings.Contains(msg, "TOL2011") {
		t.Fatalf("unexpected duplicate selector noise for duplicate function: %v", diags)
	}
}

func TestCheckRejectsReservedTopLevelSupportDeclName(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		SkippedTopDecls: []ast.SkippedTopDecl{
			{Kind: "interface", Name: "selector"},
		},
		Contract: &ast.ContractDecl{
			Name: "Demo",
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2033") {
		t.Fatalf("expected TOL2033, got: %v", diags)
	}
}

func TestCheckRejectsDuplicateSkippedContractDeclName(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			SkippedDecls: []ast.SkippedContractDecl{
				{Kind: "enum", Name: "Common"},
				{Kind: "modifier", Name: "Common"},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2026") {
		t.Fatalf("expected TOL2026, got: %v", diags)
	}
}

func TestCheckRejectsReservedSkippedContractDeclName(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			SkippedDecls: []ast.SkippedContractDecl{
				{Kind: "error", Name: "selector"},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2033") {
		t.Fatalf("expected TOL2033, got: %v", diags)
	}
}

func TestCheckRejectsSkippedContractDeclNameCollisionWithFunction(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			SkippedDecls: []ast.SkippedContractDecl{
				{Kind: "enum", Name: "run"},
			},
			Functions: []ast.FunctionDecl{
				{Name: "run"},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2026") {
		t.Fatalf("expected TOL2026, got: %v", diags)
	}
}

func TestCheckRejectsContractNameCollisionWithSkippedContractDecl(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			SkippedDecls: []ast.SkippedContractDecl{
				{Kind: "modifier", Name: "Demo"},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2026") {
		t.Fatalf("expected TOL2026, got: %v", diags)
	}
}

func TestCheckRejectsPartialNestedMappingIndex(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					{Name: "allowances", Type: "mapping(address => mapping(address => u256))"},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Params: []ast.FieldDecl{
						{Name: "owner", Type: "address"},
					},
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "got"},
							Expr: &ast.Expr{
								Kind: "index",
								Object: &ast.Expr{
									Kind:  "ident",
									Value: "allowances",
								},
								Index: &ast.Expr{Kind: "ident", Value: "owner"},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2018") {
		t.Fatalf("expected TOL2018, got: %v", diags)
	}
}

func TestCheckRejectsStorageScalarIndexRead(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					{Name: "total", Type: "u256"},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Params: []ast.FieldDecl{
						{Name: "i", Type: "u256"},
					},
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "got"},
							Expr: &ast.Expr{
								Kind: "index",
								Object: &ast.Expr{
									Kind:  "ident",
									Value: "total",
								},
								Index: &ast.Expr{Kind: "ident", Value: "i"},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2018") {
		t.Fatalf("expected TOL2018, got: %v", diags)
	}
}

func TestCheckAcceptsStorageArrayPushLengthAndIndex(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					{Name: "xs", Type: "u256[]"},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Params: []ast.FieldDecl{
						{Name: "v", Type: "u256"},
						{Name: "i", Type: "u256"},
					},
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Member: "push",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "xs",
									},
								},
								Args: []*ast.Expr{
									{Kind: "ident", Value: "v"},
								},
							},
						},
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "a"},
							Expr: &ast.Expr{
								Kind:   "member",
								Member: "length",
								Object: &ast.Expr{
									Kind:  "ident",
									Value: "xs",
								},
							},
						},
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "b"},
							Expr: &ast.Expr{
								Kind: "index",
								Object: &ast.Expr{
									Kind:  "ident",
									Value: "xs",
								},
								Index: &ast.Expr{Kind: "ident", Value: "i"},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsSetStorageArrayLengthTarget(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					{Name: "xs", Type: "u256[]"},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind:   "member",
								Member: "length",
								Object: &ast.Expr{
									Kind:  "ident",
									Value: "xs",
								},
							},
							Expr: &ast.Expr{Kind: "number", Value: "1"},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2018") {
		t.Fatalf("expected TOL2018, got: %v", diags)
	}
}

func TestCheckRejectsFunctionCallArityMismatch(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "sum",
					Params: []ast.FieldDecl{
						{Name: "a", Type: "u256"},
						{Name: "b", Type: "u256"},
					},
					Body: []ast.Statement{{Kind: "return"}},
				},
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "sum"},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2019") {
		t.Fatalf("expected TOL2019, got: %v", diags)
	}
}

func TestCheckAcceptsFunctionCallArityMatch(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "sum",
					Params: []ast.FieldDecl{
						{Name: "a", Type: "u256"},
						{Name: "b", Type: "u256"},
					},
					Body: []ast.Statement{{Kind: "return"}},
				},
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "sum"},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
									{Kind: "number", Value: "2"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsThisMemberFunctionCallArityMismatch(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "sum",
					Params: []ast.FieldDecl{
						{Name: "a", Type: "u256"},
						{Name: "b", Type: "u256"},
					},
					Body: []ast.Statement{{Kind: "return"}},
				},
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Member: "sum",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "this",
									},
								},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2019") {
		t.Fatalf("expected TOL2019, got: %v", diags)
	}
}

func TestCheckRejectsContractMemberFunctionCallArityMismatch(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "sum",
					Params: []ast.FieldDecl{
						{Name: "a", Type: "u256"},
						{Name: "b", Type: "u256"},
					},
					Body: []ast.Statement{{Kind: "return"}},
				},
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Member: "sum",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "Demo",
									},
								},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2019") {
		t.Fatalf("expected TOL2019, got: %v", diags)
	}
}

func TestCheckRejectsUnknownThisMemberFunctionCallTarget(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Member: "missing",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "this",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2031") {
		t.Fatalf("expected TOL2031, got: %v", diags)
	}
}

func TestCheckRejectsUnknownContractMemberFunctionCallTarget(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Member: "missing",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "Demo",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2031") {
		t.Fatalf("expected TOL2031, got: %v", diags)
	}
}

func TestCheckRejectsThisMemberCallToNonExternalFunction(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "sum",
					Modifiers: []string{"internal"},
					Body:      []ast.Statement{{Kind: "return"}},
				},
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Member: "sum",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "this",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2032") {
		t.Fatalf("expected TOL2032, got: %v", diags)
	}
}

func TestCheckRejectsContractMemberCallToNonExternalFunction(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "sum",
					Modifiers: []string{"internal"},
					Body:      []ast.Statement{{Kind: "return"}},
				},
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Member: "sum",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "Demo",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2032") {
		t.Fatalf("expected TOL2032, got: %v", diags)
	}
}

func TestCheckRejectsDirectCallToExternalFunction(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "sum",
					Modifiers: []string{"external"},
					Body:      []ast.Statement{{Kind: "return"}},
				},
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "sum",
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2032") {
		t.Fatalf("expected TOL2032, got: %v", diags)
	}
}

func TestCheckRejectsInvalidAssignmentExprTarget(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "for",
							Init: &ast.Statement{
								Kind: "let",
								Name: "i",
								Type: "u256",
								Expr: &ast.Expr{Kind: "number", Value: "0"},
							},
							Cond: &ast.Expr{
								Kind:  "binary",
								Op:    "<",
								Left:  &ast.Expr{Kind: "ident", Value: "i"},
								Right: &ast.Expr{Kind: "number", Value: "3"},
							},
							Post: &ast.Expr{
								Kind: "assign",
								Op:   "=",
								Left: &ast.Expr{
									Kind:  "binary",
									Op:    "+",
									Left:  &ast.Expr{Kind: "ident", Value: "i"},
									Right: &ast.Expr{Kind: "number", Value: "1"},
								},
								Right: &ast.Expr{Kind: "number", Value: "1"},
							},
							Body: []ast.Statement{{Kind: "break"}},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2008") {
		t.Fatalf("expected TOL2008, got: %v", diags)
	}
}

func TestCheckRejectsAssignExprInLetInitializer(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "x",
							Type: "u256",
							Expr: &ast.Expr{
								Kind: "assign",
								Op:   "=",
								Left: &ast.Expr{Kind: "ident", Value: "a"},
								Right: &ast.Expr{
									Kind:  "number",
									Value: "1",
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020, got: %v", diags)
	}
}

func TestCheckRejectsNonCallAssignExprStatement(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind:  "binary",
								Op:    "+",
								Left:  &ast.Expr{Kind: "number", Value: "1"},
								Right: &ast.Expr{Kind: "number", Value: "2"},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020, got: %v", diags)
	}
}

func TestCheckRejectsUnknownStatementKind(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{Kind: "unknown_stmt"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

func TestCheckRejectsUnknownExpressionKind(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "set",
							Target: &ast.Expr{
								Kind:  "ident",
								Value: "x",
							},
							Expr: &ast.Expr{
								Kind:  "mystery_expr",
								Value: "x",
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

func TestCheckRejectsSelectorBuiltinExprStatement(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "selector",
								},
								Args: []*ast.Expr{
									{Kind: "string", Value: "\"transfer(address,u256)\""},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

func TestCheckRejectsForPostNonCallAssignExpr(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "for",
							Init: &ast.Statement{
								Kind: "let",
								Name: "i",
								Type: "u256",
								Expr: &ast.Expr{Kind: "number", Value: "0"},
							},
							Cond: &ast.Expr{
								Kind:  "binary",
								Op:    "<",
								Left:  &ast.Expr{Kind: "ident", Value: "i"},
								Right: &ast.Expr{Kind: "number", Value: "3"},
							},
							Post: &ast.Expr{
								Kind:  "binary",
								Op:    "+",
								Left:  &ast.Expr{Kind: "ident", Value: "i"},
								Right: &ast.Expr{Kind: "number", Value: "1"},
							},
							Body: []ast.Statement{{Kind: "break"}},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020, got: %v", diags)
	}
}

func TestCheckAcceptsCallExprStatementAndForPostAssign(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "tick",
					Body: []ast.Statement{{Kind: "return"}},
				},
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "tick"},
							},
						},
						{
							Kind: "for",
							Init: &ast.Statement{
								Kind: "let",
								Name: "i",
								Type: "u256",
								Expr: &ast.Expr{Kind: "number", Value: "0"},
							},
							Cond: &ast.Expr{
								Kind:  "binary",
								Op:    "<",
								Left:  &ast.Expr{Kind: "ident", Value: "i"},
								Right: &ast.Expr{Kind: "number", Value: "3"},
							},
							Post: &ast.Expr{
								Kind: "assign",
								Op:   "=",
								Left: &ast.Expr{Kind: "ident", Value: "i"},
								Right: &ast.Expr{
									Kind:  "binary",
									Op:    "+",
									Left:  &ast.Expr{Kind: "ident", Value: "i"},
									Right: &ast.Expr{Kind: "number", Value: "1"},
								},
							},
							Body: []ast.Statement{{Kind: "break"}},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsRequireWithoutExpr(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{Kind: "require"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

func TestCheckRejectsAssignExprInRequireExpr(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "require",
							Text: "\"BAD\"",
							Expr: &ast.Expr{
								Kind: "assign",
								Op:   "=",
								Left: &ast.Expr{Kind: "ident", Value: "x"},
								Right: &ast.Expr{
									Kind:  "number",
									Value: "1",
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020, got: %v", diags)
	}
}

func TestCheckRejectsAssignExprInAssertExpr(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "assert",
							Text: "\"BAD\"",
							Expr: &ast.Expr{
								Kind: "assign",
								Op:   "=",
								Left: &ast.Expr{Kind: "ident", Value: "x"},
								Right: &ast.Expr{
									Kind:  "number",
									Value: "1",
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020, got: %v", diags)
	}
}

func TestCheckRejectsRequireMissingMessage(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "require",
							Expr: &ast.Expr{Kind: "ident", Value: "ok"},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

func TestCheckRejectsAssertMissingMessage(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "assert",
							Expr: &ast.Expr{Kind: "ident", Value: "ok"},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

func TestCheckRejectsRequireNonLiteralMessage(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "require",
							Expr: &ast.Expr{Kind: "ident", Value: "ok"},
							Text: "BAD",
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

func TestCheckRejectsAssertNonLiteralMessage(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "assert",
							Expr: &ast.Expr{Kind: "ident", Value: "ok"},
							Text: "BAD",
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

func TestCheckAcceptsRequireAssertLiteralMessage(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "require",
							Expr: &ast.Expr{Kind: "ident", Value: "ok"},
							Text: "\"BAD\"",
						},
						{
							Kind: "assert",
							Expr: &ast.Expr{Kind: "ident", Value: "ok"},
							Text: "\"BAD\"",
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsEmitNonCallExpr(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind:  "ident",
								Value: "x",
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

func TestCheckRejectsAssignExprInEmitPayload(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "Tick",
								},
								Args: []*ast.Expr{
									{
										Kind: "assign",
										Op:   "=",
										Left: &ast.Expr{Kind: "ident", Value: "x"},
										Right: &ast.Expr{
											Kind:  "number",
											Value: "1",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020, got: %v", diags)
	}
}

func TestCheckRejectsEmitMemberCallPayload(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Member: "Tick",
									Object: &ast.Expr{
										Kind:  "ident",
										Value: "obj",
									},
								},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

func TestCheckRejectsEmitSelectorBuiltinPayload(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "selector",
								},
								Args: []*ast.Expr{
									{Kind: "string", Value: "\"transfer(address,u256)\""},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

func TestCheckAcceptsEmitCallExpr(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Events: []ast.EventDecl{
				{
					Name: "Tick",
					Params: []ast.FieldDecl{
						{Name: "a", Type: "u256"},
					},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "Tick",
								},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsEmitDeclaredEventArityMismatch(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Events: []ast.EventDecl{
				{
					Name: "Tick",
					Params: []ast.FieldDecl{
						{Name: "a", Type: "u256"},
						{Name: "b", Type: "u256"},
					},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "Tick",
								},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2023") {
		t.Fatalf("expected TOL2023, got: %v", diags)
	}
}

func TestCheckAcceptsEmitDeclaredEventArityMatch(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Events: []ast.EventDecl{
				{
					Name: "Tick",
					Params: []ast.FieldDecl{
						{Name: "a", Type: "u256"},
						{Name: "b", Type: "u256"},
					},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "Tick",
								},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
									{Kind: "number", Value: "2"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsDuplicateEventDeclarations(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Events: []ast.EventDecl{
				{Name: "Tick", Params: []ast.FieldDecl{{Name: "a", Type: "u256"}}},
				{Name: "Tick", Params: []ast.FieldDecl{{Name: "b", Type: "u256"}}},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2024") {
		t.Fatalf("expected TOL2024, got: %v", diags)
	}
}

func TestCheckRejectsDuplicateEventParams(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Events: []ast.EventDecl{
				{
					Name: "Tick",
					Params: []ast.FieldDecl{
						{Name: "a", Type: "u256"},
						{Name: "a", Type: "u256"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2016") {
		t.Fatalf("expected TOL2016, got: %v", diags)
	}
}

func TestCheckRejectsEventWithMoreThanThreeIndexedFields(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Events: []ast.EventDecl{
				{
					Name: "TooManyIndexed",
					Params: []ast.FieldDecl{
						{Name: "a", Type: "u256", Indexed: true},
						{Name: "b", Type: "u256", Indexed: true},
						{Name: "c", Type: "u256", Indexed: true},
						{Name: "d", Type: "u256", Indexed: true},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

func TestCheckRejectsDuplicateFunctionReturnNames(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Returns: []ast.FieldDecl{
						{Name: "ok", Type: "bool"},
						{Name: "ok", Type: "bool"},
					},
					Body: []ast.Statement{
						{Kind: "return", Expr: &ast.Expr{Kind: "number", Value: "1"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2016") {
		t.Fatalf("expected TOL2016, got: %v", diags)
	}
}

func TestCheckRejectsFunctionParamReturnNameCollision(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Params: []ast.FieldDecl{
						{Name: "x", Type: "u256"},
					},
					Returns: []ast.FieldDecl{
						{Name: "x", Type: "u256"},
					},
					Body: []ast.Statement{
						{Kind: "return", Expr: &ast.Expr{Kind: "number", Value: "1"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2029") {
		t.Fatalf("expected TOL2029, got: %v", diags)
	}
}

func TestCheckAcceptsFunctionDistinctParamAndReturnNames(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Params: []ast.FieldDecl{
						{Name: "x", Type: "u256"},
					},
					Returns: []ast.FieldDecl{
						{Name: "out", Type: "u256"},
					},
					Body: []ast.Statement{
						{Kind: "return", Expr: &ast.Expr{Kind: "number", Value: "1"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsEmitUnknownDeclaredEventSet(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Events: []ast.EventDecl{
				{Name: "Tick", Params: []ast.FieldDecl{{Name: "a", Type: "u256"}}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "Other",
								},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2025") {
		t.Fatalf("expected TOL2025, got: %v", diags)
	}
}

func TestCheckRejectsEmitWhenNoEventsDeclared(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "Tick",
								},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2025") {
		t.Fatalf("expected TOL2025, got: %v", diags)
	}
}

func TestCheckRejectsEventFunctionNameCollision(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Events: []ast.EventDecl{
				{Name: "Tick", Params: []ast.FieldDecl{{Name: "a", Type: "u256"}}},
			},
			Functions: []ast.FunctionDecl{
				{Name: "Tick"},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2026") {
		t.Fatalf("expected TOL2026, got: %v", diags)
	}
}

func TestCheckRejectsFunctionStorageNameCollision(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					{Name: "run", Type: "u256"},
				},
			},
			Functions: []ast.FunctionDecl{
				{Name: "run"},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2026") {
		t.Fatalf("expected TOL2026, got: %v", diags)
	}
}

func TestCheckRejectsRevertNonStringPayload(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "revert",
							Expr: &ast.Expr{
								Kind:  "ident",
								Value: "err",
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2022") {
		t.Fatalf("expected TOL2022, got: %v", diags)
	}
}

func TestCheckAcceptsRevertCustomErrorPayload(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "revert",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "InsufficientBalance",
								},
								Args: []*ast.Expr{
									{Kind: "ident", Value: "a"},
									{Kind: "ident", Value: "b"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckAcceptsRevertDeclaredCustomErrorPayload(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			SkippedDecls: []ast.SkippedContractDecl{
				{Kind: "error", Name: "InsufficientBalance"},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "revert",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "InsufficientBalance",
								},
								Args: []*ast.Expr{
									{Kind: "ident", Value: "a"},
									{Kind: "ident", Value: "b"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsRevertUndeclaredCustomErrorPayloadWhenErrorDeclsPresent(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Errors: []ast.ErrorDecl{
				{Name: "KnownError", Params: []ast.FieldDecl{}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "revert",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "UnknownError",
								},
								Args: []*ast.Expr{{Kind: "number", Value: "1"}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2022") {
		t.Fatalf("expected TOL2022, got: %v", diags)
	}
}

func TestCheckRejectsRevertInvalidStringLiteralPayload(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "revert",
							Expr: &ast.Expr{
								Kind:  "string",
								Value: "BAD",
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2022") {
		t.Fatalf("expected TOL2022, got: %v", diags)
	}
}

func TestCheckRejectsAssignExprInRevertPayload(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "revert",
							Expr: &ast.Expr{
								Kind: "assign",
								Op:   "=",
								Left: &ast.Expr{Kind: "ident", Value: "x"},
								Right: &ast.Expr{
									Kind:  "number",
									Value: "1",
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020, got: %v", diags)
	}
}

func TestCheckAcceptsRevertStringOrEmpty(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "a",
					Body: []ast.Statement{
						{Kind: "revert"},
					},
				},
				{
					Name: "b",
					Body: []ast.Statement{
						{
							Kind: "revert",
							Expr: &ast.Expr{
								Kind:  "string",
								Value: "\"ERR\"",
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsNestedAssignInExprStatementCallArg(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "foo",
								},
								Args: []*ast.Expr{
									{
										Kind: "assign",
										Op:   "=",
										Left: &ast.Expr{Kind: "ident", Value: "x"},
										Right: &ast.Expr{
											Kind:  "number",
											Value: "1",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020, got: %v", diags)
	}
}

func TestCheckRejectsNestedAssignInForPostCallArg(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "run",
					Body: []ast.Statement{
						{
							Kind: "for",
							Init: &ast.Statement{
								Kind: "let",
								Name: "i",
								Type: "u256",
								Expr: &ast.Expr{Kind: "number", Value: "0"},
							},
							Cond: &ast.Expr{
								Kind:  "binary",
								Op:    "<",
								Left:  &ast.Expr{Kind: "ident", Value: "i"},
								Right: &ast.Expr{Kind: "number", Value: "3"},
							},
							Post: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:  "ident",
									Value: "tick",
								},
								Args: []*ast.Expr{
									{
										Kind: "assign",
										Op:   "=",
										Left: &ast.Expr{Kind: "ident", Value: "i"},
										Right: &ast.Expr{
											Kind:  "number",
											Value: "1",
										},
									},
								},
							},
							Body: []ast.Statement{{Kind: "break"}},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2020") {
		t.Fatalf("expected TOL2020, got: %v", diags)
	}
}

// TOL_TEST: test block validation tests

func TestCheckAllowsTestBlockInTestFile(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Tests: []ast.TestDecl{
			{
				Name: "MyTests",
				Fns: []ast.TestFn{
					{Name: "test_something", Body: nil},
				},
			},
		},
	}
	// filename ends with _test.tol - valid
	_, diags := Check("my_contract_test.tol", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsTestBlockInNonTestFile(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Tests: []ast.TestDecl{
			{
				Name: "MyTests",
				Fns: []ast.TestFn{
					{Name: "test_something", Body: nil},
				},
			},
		},
	}
	// filename does NOT end with _test.tol
	_, diags := Check("my_contract.tol", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2035") {
		t.Fatalf("expected TOL2035, got: %v", diags)
	}
}

func TestCheckRejectsTestFnWithoutTestPrefix(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Tests: []ast.TestDecl{
			{
				Name: "MyTests",
				Fns: []ast.TestFn{
					{Name: "check_something", Body: nil},
				},
			},
		},
	}
	_, diags := Check("my_contract_test.tol", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2036") {
		t.Fatalf("expected TOL2036, got: %v", diags)
	}
}

func TestCheckAllowsTestOnlyFileWithoutContract(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Tests: []ast.TestDecl{
			{
				Name: "Suite",
				Fns: []ast.TestFn{
					{Name: "test_basic", Body: nil},
				},
			},
		},
	}
	_, diags := Check("suite_test.tol", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckRejectsNilLiteralInTestFnBody(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Tests: []ast.TestDecl{
			{
				Name: "Suite",
				Fns: []ast.TestFn{
					{
						Name: "test_nil_forbidden",
						Body: []ast.Statement{
							{
								Kind: "expr",
								Expr: &ast.Expr{Kind: "ident", Value: "nil"},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("suite_test.tol", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
	if !strings.Contains(diags.Error(), "source-level nil is not allowed in TOL") {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

// --- Modifier sema tests ---

func TestCheckModifierDeclaredAndUsed(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Modifiers: []ast.ModifierDecl{
				{
					Name: "onlyOwner",
					Body: []ast.Statement{
						{Kind: "require", Expr: &ast.Expr{Kind: "ident", Value: "cond"}, Text: `"not owner"`},
						{Kind: "placeholder"},
					},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:      "doThing",
					Modifiers: []string{"public", "onlyOwner"},
					Body:      []ast.Statement{{Kind: "return"}},
				},
			},
		},
	}
	typed, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if typed == nil {
		t.Fatalf("expected typed module")
	}
}

func TestCheckModifierUnknownNameRejected(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "doThing",
					Modifiers: []string{"public", "nonExistent"},
					Body:      []ast.Statement{{Kind: "return"}},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics")
	}
	if !strings.Contains(diags.Error(), "TOL2038") {
		t.Fatalf("expected TOL2038, got: %v", diags)
	}
}

func TestCheckModifierMissingPlaceholderRejected(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Modifiers: []ast.ModifierDecl{
				{
					Name: "noPlaceholder",
					Body: []ast.Statement{
						{Kind: "require", Expr: &ast.Expr{Kind: "ident", Value: "cond"}, Text: `"fail"`},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for modifier missing placeholder")
	}
	if !strings.Contains(diags.Error(), "TOL2040") {
		t.Fatalf("expected TOL2040, got: %v", diags)
	}
}

func TestCheckModifierDuplicatePlaceholderRejected(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Modifiers: []ast.ModifierDecl{
				{
					Name: "doublePlaceholder",
					Body: []ast.Statement{
						{Kind: "placeholder"},
						{Kind: "placeholder"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for modifier with 2 placeholders")
	}
	if !strings.Contains(diags.Error(), "TOL2040") {
		t.Fatalf("expected TOL2040, got: %v", diags)
	}
}

func TestCheckModifierDuplicateDeclRejected(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Modifiers: []ast.ModifierDecl{
				{Name: "onlyOwner", Body: []ast.Statement{{Kind: "placeholder"}}},
				{Name: "onlyOwner", Body: []ast.Statement{{Kind: "placeholder"}}},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for duplicate modifier declaration")
	}
	if !strings.Contains(diags.Error(), "TOL2039") {
		t.Fatalf("expected TOL2039, got: %v", diags)
	}
}

func TestCheckModifierNameCollisionWithSkippedDecl(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name:         "Demo",
			SkippedDecls: []ast.SkippedContractDecl{{Kind: "enum", Name: "Common"}},
			Modifiers: []ast.ModifierDecl{
				{Name: "Common", Body: []ast.Statement{{Kind: "placeholder"}}},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for modifier/enum name collision")
	}
	if !strings.Contains(diags.Error(), "TOL2026") {
		t.Fatalf("expected TOL2026, got: %v", diags)
	}
}


// =============================================================================
// M3: Inheritance, C3 linearization, interface conformance, super call tests.
// =============================================================================

// TestCheckInterfaceConformanceOK: contract properly implements interface.
func TestCheckInterfaceConformanceOK(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Interfaces: []ast.InterfaceDecl{
			{
				Name: "IToken",
				Functions: []ast.FuncSigDecl{
					{
						Name:      "transfer",
						Params:    []ast.FieldDecl{{Name: "to", Type: "address"}, {Name: "amount", Type: "u256"}},
						Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
						Modifiers: []string{"public"},
					},
				},
			},
		},
		Contract: &ast.ContractDecl{
			Name:  "Token",
			Bases: []string{"IToken"},
			Functions: []ast.FunctionDecl{
				{
					Name:      "transfer",
					Params:    []ast.FieldDecl{{Name: "to", Type: "address"}, {Name: "amount", Type: "u256"}},
					Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "true"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

// TestCheckInterfaceNotImplemented: contract missing required interface function.
func TestCheckInterfaceNotImplemented(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Interfaces: []ast.InterfaceDecl{
			{
				Name: "IToken",
				Functions: []ast.FuncSigDecl{
					{
						Name:      "transfer",
						Params:    []ast.FieldDecl{{Name: "to", Type: "address"}, {Name: "amount", Type: "u256"}},
						Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
						Modifiers: []string{"public"},
					},
				},
			},
		},
		Contract: &ast.ContractDecl{
			Name:      "Token",
			Bases:     []string{"IToken"},
			Functions: []ast.FunctionDecl{}, // missing transfer
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected interface-not-implemented error")
	}
	if !strings.Contains(diags.Error(), "TOL2044") {
		t.Fatalf("expected TOL2044, got: %v", diags)
	}
	if !strings.Contains(diags.Error(), "transfer") {
		t.Fatalf("expected 'transfer' in error, got: %v", diags)
	}
}

// TestCheckOverrideSignatureMismatch: contract function has wrong signature vs interface.
func TestCheckOverrideSignatureMismatch(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Interfaces: []ast.InterfaceDecl{
			{
				Name: "IToken",
				Functions: []ast.FuncSigDecl{
					{
						Name:      "transfer",
						Params:    []ast.FieldDecl{{Name: "to", Type: "address"}, {Name: "amount", Type: "u256"}},
						Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
						Modifiers: []string{"public"},
					},
				},
			},
		},
		Contract: &ast.ContractDecl{
			Name:  "Token",
			Bases: []string{"IToken"},
			Functions: []ast.FunctionDecl{
				{
					// Wrong: param type u128 instead of u256.
					Name:      "transfer",
					Params:    []ast.FieldDecl{{Name: "to", Type: "address"}, {Name: "amount", Type: "u128"}},
					Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "true"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected override signature mismatch error")
	}
	if !strings.Contains(diags.Error(), "TOL2045") {
		t.Fatalf("expected TOL2045, got: %v", diags)
	}
}

// TestCheckC3LinearizationSimple: single inheritance, C3 returns base name.
func TestCheckC3LinearizationSimple(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Interfaces: []ast.InterfaceDecl{
			{
				Name:      "IBase",
				Functions: []ast.FuncSigDecl{{Name: "foo", Params: nil, Returns: nil, Modifiers: []string{"public"}}},
			},
		},
		Contract: &ast.ContractDecl{
			Name:  "Child",
			Bases: []string{"IBase"},
			Functions: []ast.FunctionDecl{
				{
					Name:      "foo",
					Params:    nil,
					Returns:   nil,
					Modifiers: []string{"public"},
					Body:      []ast.Statement{{Kind: "return"}},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

// TestCheckC3DiamondInheritance: diamond pattern with two interfaces.
// A <- B <- Diamond
// A <- C <- Diamond
// Diamond is B, C
// L(Diamond) = Diamond + merge([B, A], [C, A], [B, C])
//            = [Diamond, B, C, A]
// All interfaces are leaves so no C3 conflict.
func TestCheckC3DiamondInheritance(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Interfaces: []ast.InterfaceDecl{
			{Name: "IA", Functions: []ast.FuncSigDecl{{Name: "foo", Modifiers: []string{"public"}}}},
			{Name: "IB", Functions: []ast.FuncSigDecl{{Name: "bar", Modifiers: []string{"public"}}}},
		},
		Contract: &ast.ContractDecl{
			Name:  "Diamond",
			Bases: []string{"IA", "IB"},
			Functions: []ast.FunctionDecl{
				{Name: "foo", Modifiers: []string{"public"}, Body: []ast.Statement{{Kind: "return"}}},
				{Name: "bar", Modifiers: []string{"public"}, Body: []ast.Statement{{Kind: "return"}}},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

// TestCheckInheritanceCycleDetection: base cannot include self.
func TestCheckInheritanceCycleDetection(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name:  "Cyclic",
			Bases: []string{"Cyclic"}, // self-reference
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected cycle detection error")
	}
	if !strings.Contains(diags.Error(), "TOL2042") {
		t.Fatalf("expected TOL2042, got: %v", diags)
	}
}

// TestCheckSuperCallValid: super.fn() in a contract with bases should be allowed.
func TestCheckSuperCallValid(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Interfaces: []ast.InterfaceDecl{
			{
				Name: "IBase",
				Functions: []ast.FuncSigDecl{
					{Name: "setup", Modifiers: []string{"internal"}},
				},
			},
		},
		Contract: &ast.ContractDecl{
			Name:  "Child",
			Bases: []string{"IBase"},
			Functions: []ast.FunctionDecl{
				{
					Name:      "setup",
					Modifiers: []string{"internal"},
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Object: &ast.Expr{Kind: "ident", Value: "super"},
									Member: "setup",
								},
								Args: []*ast.Expr{},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for valid super call: %v", diags)
	}
}

// TestCheckSuperCallWithoutBases: super.fn() with no bases is an error.
func TestCheckSuperCallWithoutBases(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name:  "Standalone",
			Bases: nil,
			Functions: []ast.FunctionDecl{
				{
					Name:      "setup",
					Modifiers: []string{"internal"},
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Object: &ast.Expr{Kind: "ident", Value: "super"},
									Member: "setup",
								},
								Args: []*ast.Expr{},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for super call without bases")
	}
	if !strings.Contains(diags.Error(), "TOL2046") {
		t.Fatalf("expected TOL2046, got: %v", diags)
	}
}

// TestCheckUnknownBaseAllowed: bases that are not in the module (cross-file) are allowed.
func TestCheckUnknownBaseAllowed(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name:  "Child",
			Bases: []string{"ExternalBase"}, // not in this module
			Functions: []ast.FunctionDecl{
				{
					Name:      "foo",
					Modifiers: []string{"public"},
					Body:      []ast.Statement{{Kind: "return"}},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for cross-file base: %v", diags)
	}
}

// TestCheckMultipleInterfacesConformance: contract implements both interfaces.
func TestCheckMultipleInterfacesConformance(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Interfaces: []ast.InterfaceDecl{
			{
				Name: "IOwnable",
				Functions: []ast.FuncSigDecl{
					{Name: "owner", Returns: []ast.FieldDecl{{Name: "addr", Type: "address"}}, Modifiers: []string{"public"}},
				},
			},
			{
				Name: "IToken",
				Functions: []ast.FuncSigDecl{
					{Name: "transfer", Params: []ast.FieldDecl{{Name: "to", Type: "address"}, {Name: "amount", Type: "u256"}}, Returns: []ast.FieldDecl{{Name: "ok", Type: "bool"}}, Modifiers: []string{"public"}},
				},
			},
		},
		Contract: &ast.ContractDecl{
			Name:  "Token",
			Bases: []string{"IToken", "IOwnable"},
			Functions: []ast.FunctionDecl{
				{
					Name:      "transfer",
					Params:    []ast.FieldDecl{{Name: "to", Type: "address"}, {Name: "amount", Type: "u256"}},
					Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
					Modifiers: []string{"public"},
					Body:      []ast.Statement{{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "true"}}},
				},
				{
					Name:      "owner",
					Returns:   []ast.FieldDecl{{Name: "addr", Type: "address"}},
					Modifiers: []string{"public"},
					Body:      []ast.Statement{{Kind: "return", Expr: &ast.Expr{Kind: "number", Value: "0"}}},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

// ──────────────────────────────────────────────────────────────
// error and enum declaration tests
// ──────────────────────────────────────────────────────────────

func TestCheckErrorDeclArityMismatch(t *testing.T) {
	// Declare error Unauthorized(caller: address) and call revert Unauthorized(a, b) → arity error.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Errors: []ast.ErrorDecl{
				{Name: "Unauthorized", Params: []ast.FieldDecl{{Name: "caller", Type: "address"}}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "revert",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "Unauthorized"},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
									{Kind: "number", Value: "2"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for arity mismatch")
	}
	if !strings.Contains(diags.Error(), "TOL2019") {
		t.Fatalf("expected TOL2019 (call arity), got: %v", diags)
	}
}

func TestCheckErrorDeclArityCorrect(t *testing.T) {
	// Declare error Unauthorized(caller: address) and call revert Unauthorized(addr) → OK.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Errors: []ast.ErrorDecl{
				{Name: "Unauthorized", Params: []ast.FieldDecl{{Name: "caller", Type: "address"}}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "revert",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "Unauthorized"},
								Args:   []*ast.Expr{{Kind: "number", Value: "0"}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckErrorDeclUndeclaredRejected(t *testing.T) {
	// Declare error Foo() and revert with NotDeclared() → error.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Errors: []ast.ErrorDecl{
				{Name: "Foo", Params: []ast.FieldDecl{}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "revert",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "NotDeclared"},
								Args:   []*ast.Expr{},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for undeclared error")
	}
	if !strings.Contains(diags.Error(), "TOL2022") {
		t.Fatalf("expected TOL2022, got: %v", diags)
	}
}

func TestCheckDuplicateErrorName(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Errors: []ast.ErrorDecl{
				{Name: "Foo", Params: []ast.FieldDecl{}},
				{Name: "Foo", Params: []ast.FieldDecl{}},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for duplicate error name")
	}
	if !strings.Contains(diags.Error(), "TOL2026") {
		t.Fatalf("expected TOL2026 (name collision), got: %v", diags)
	}
}

func TestCheckDuplicateEnumName(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Enums: []ast.EnumDecl{
				{Name: "State", Members: []string{"Active"}},
				{Name: "State", Members: []string{"Inactive"}},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for duplicate enum name")
	}
	if !strings.Contains(diags.Error(), "TOL2026") {
		t.Fatalf("expected TOL2026 (name collision), got: %v", diags)
	}
}

func TestCheckErrorAndEnumNameCollision(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Errors: []ast.ErrorDecl{
				{Name: "Conflict", Params: []ast.FieldDecl{}},
			},
			Enums: []ast.EnumDecl{
				{Name: "Conflict", Members: []string{"A"}},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for error/enum name collision")
	}
	if !strings.Contains(diags.Error(), "TOL2026") {
		t.Fatalf("expected TOL2026 (name collision), got: %v", diags)
	}
}

func TestCheckEnumInvalidMemberAccess(t *testing.T) {
	// State.InvalidMember should fail sema.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Enums: []ast.EnumDecl{
				{Name: "State", Members: []string{"Active", "Inactive"}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "s",
							Type: "u8",
							Expr: &ast.Expr{
								Kind:   "member",
								Object: &ast.Expr{Kind: "ident", Value: "State"},
								Member: "BadMember",
							},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for invalid enum member")
	}
	if !strings.Contains(diags.Error(), "BadMember") {
		t.Fatalf("expected error mentioning 'BadMember', got: %v", diags)
	}
}

func TestCheckEnumValidMemberAccess(t *testing.T) {
	// State.Active should pass sema.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Enums: []ast.EnumDecl{
				{Name: "State", Members: []string{"Active", "Inactive"}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "s",
							Type: "u8",
							Expr: &ast.Expr{
								Kind:   "member",
								Object: &ast.Expr{Kind: "ident", Value: "State"},
								Member: "Active",
							},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

// ---------------------------------------------------------------------------
// Library declaration sema tests
// ---------------------------------------------------------------------------

func TestCheckLibraryDeclarationOK(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Libraries: []ast.LibraryDecl{
			{
				Name: "MathLib",
				Functions: []ast.FunctionDecl{
					{
						Name:      "add",
						Modifiers: []string{"internal", "pure"},
						Params:    []ast.FieldDecl{{Name: "a", Type: "u256"}, {Name: "b", Type: "u256"}},
						Returns:   []ast.FieldDecl{{Name: "r", Type: "u256"}},
						Body:      []ast.Statement{{Kind: "return", Expr: &ast.Expr{Kind: "number", Value: "0"}}},
					},
				},
			},
		},
		Contract: &ast.ContractDecl{Name: "Demo"},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckLibraryRejectsExternalFunction(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Libraries: []ast.LibraryDecl{
			{
				Name: "MathLib",
				Functions: []ast.FunctionDecl{
					{
						Name:      "add",
						Modifiers: []string{"external"},
						Params:    []ast.FieldDecl{{Name: "a", Type: "u256"}},
						Body:      []ast.Statement{{Kind: "return"}},
					},
				},
			},
		},
		Contract: &ast.ContractDecl{Name: "Demo"},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for external library function")
	}
	if !strings.Contains(diags.Error(), "TOL2015") {
		t.Fatalf("expected TOL2015, got: %v", diags)
	}
}

func TestCheckLibraryRejectsPublicFunction(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Libraries: []ast.LibraryDecl{
			{
				Name: "MathLib",
				Functions: []ast.FunctionDecl{
					{
						Name:      "add",
						Modifiers: []string{"public"},
						Params:    []ast.FieldDecl{{Name: "a", Type: "u256"}},
						Body:      []ast.Statement{{Kind: "return"}},
					},
				},
			},
		},
		Contract: &ast.ContractDecl{Name: "Demo"},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for public library function")
	}
	if !strings.Contains(diags.Error(), "TOL2015") {
		t.Fatalf("expected TOL2015, got: %v", diags)
	}
}

func TestCheckLibraryNameCollisionWithInterface(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Interfaces: []ast.InterfaceDecl{
			{Name: "ICommon"},
		},
		Libraries: []ast.LibraryDecl{
			{Name: "ICommon"},
		},
		Contract: &ast.ContractDecl{Name: "Demo"},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected name collision error")
	}
	if !strings.Contains(diags.Error(), "TOL2026") {
		t.Fatalf("expected TOL2026, got: %v", diags)
	}
}

func TestCheckLibraryCallArityOK(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Libraries: []ast.LibraryDecl{
			{
				Name: "MathLib",
				Functions: []ast.FunctionDecl{
					{
						Name:      "add",
						Modifiers: []string{"internal", "pure"},
						Params:    []ast.FieldDecl{{Name: "a", Type: "u256"}, {Name: "b", Type: "u256"}},
						Returns:   []ast.FieldDecl{{Name: "r", Type: "u256"}},
						Body:      []ast.Statement{{Kind: "return", Expr: &ast.Expr{Kind: "number", Value: "0"}}},
					},
				},
			},
		},
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Returns:   []ast.FieldDecl{{Name: "result", Type: "u256"}},
					Body: []ast.Statement{
						{
							Kind: "return",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Object: &ast.Expr{Kind: "ident", Value: "MathLib"},
									Member: "add",
								},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
									{Kind: "number", Value: "2"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckLibraryCallArityMismatch(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Libraries: []ast.LibraryDecl{
			{
				Name: "MathLib",
				Functions: []ast.FunctionDecl{
					{
						Name:      "add",
						Modifiers: []string{"internal", "pure"},
						Params:    []ast.FieldDecl{{Name: "a", Type: "u256"}, {Name: "b", Type: "u256"}},
						Returns:   []ast.FieldDecl{{Name: "r", Type: "u256"}},
						Body:      []ast.Statement{{Kind: "return", Expr: &ast.Expr{Kind: "number", Value: "0"}}},
					},
				},
			},
		},
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "r",
							Type: "u256",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Object: &ast.Expr{Kind: "ident", Value: "MathLib"},
									Member: "add",
								},
								// Pass only 1 arg instead of 2.
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
								},
							},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected arity mismatch error")
	}
	if !strings.Contains(diags.Error(), "TOL2019") {
		t.Fatalf("expected TOL2019, got: %v", diags)
	}
}

func TestCheckUsingDeclValidOK(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Libraries: []ast.LibraryDecl{
			{
				Name: "MathLib",
				Functions: []ast.FunctionDecl{
					{
						Name:      "double",
						Modifiers: []string{"internal", "pure"},
						Params:    []ast.FieldDecl{{Name: "x", Type: "u256"}},
						Returns:   []ast.FieldDecl{{Name: "r", Type: "u256"}},
						Body:      []ast.Statement{{Kind: "return", Expr: &ast.Expr{Kind: "number", Value: "0"}}},
					},
				},
			},
		},
		Contract: &ast.ContractDecl{
			Name: "Demo",
			UsingDecls: []ast.UsingDecl{
				{Library: "MathLib", Type: "u256"},
			},
			Functions: []ast.FunctionDecl{
				{Name: "run", Modifiers: []string{"public"}, Body: []ast.Statement{{Kind: "return"}}},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestCheckUsingDeclUnknownLibraryRejected(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			UsingDecls: []ast.UsingDecl{
				{Library: "NonExistentLib", Type: "u256"},
			},
			Functions: []ast.FunctionDecl{
				{Name: "run", Modifiers: []string{"public"}, Body: []ast.Statement{{Kind: "return"}}},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for unknown library in using decl")
	}
	if !strings.Contains(diags.Error(), "TOL2031") {
		t.Fatalf("expected TOL2031, got: %v", diags)
	}
}

func TestCheckDeleteValidTarget(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{Kind: "let", Name: "x", Type: "u256", Expr: &ast.Expr{Kind: "number", Value: "5"}},
						{Kind: "delete", Expr: &ast.Expr{Kind: "ident", Value: "x"}},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for valid delete: %v", diags)
	}
}

func TestCheckDeleteInvalidTarget(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						// delete on a literal is invalid
						{Kind: "delete", Expr: &ast.Expr{Kind: "number", Value: "5"}},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for invalid delete target")
	}
	if !strings.Contains(diags.Error(), "TOL2061") {
		t.Fatalf("expected TOL2061, got: %v", diags)
	}
}

func TestCheckUncheckedBlock(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{Kind: "let", Name: "x", Type: "u256", Expr: &ast.Expr{Kind: "number", Value: "0"}},
						{
							Kind: "unchecked",
							Body: []ast.Statement{
								{
									Kind:   "set",
									Target: &ast.Expr{Kind: "ident", Value: "x"},
									Expr: &ast.Expr{
										Kind: "binary",
										Op:   "+",
										Left: &ast.Expr{Kind: "ident", Value: "x"},
										Right: &ast.Expr{Kind: "number", Value: "1"},
									},
								},
							},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for valid unchecked block: %v", diags)
	}
}

func TestCheckTernaryExpr(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "x",
							Type: "u256",
							Expr: &ast.Expr{
								Kind: "ternary",
								Args: []*ast.Expr{
									{Kind: "binary", Op: "==", Left: &ast.Expr{Kind: "number", Value: "1"}, Right: &ast.Expr{Kind: "number", Value: "1"}},
									{Kind: "number", Value: "42"},
									{Kind: "number", Value: "0"},
								},
							},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for valid ternary: %v", diags)
	}
}

// ---------------------------------------------------------------------------
// Effect-check tests (Pass 1: pure/view/payable enforcement, Pass 4: msg.value)
// ---------------------------------------------------------------------------

func TestCheckPureFunctionRejectsStorageRead(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{{Name: "balance", Type: "u256"}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:      "getBalance",
					Modifiers: []string{"public", "pure"},
					Returns:   []ast.FieldDecl{{Name: "v", Type: "u256"}},
					Body: []ast.Statement{
						{
							Kind: "return",
							Expr: &ast.Expr{Kind: "ident", Value: "balance"},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for pure function reading storage")
	}
	if !strings.Contains(diags.Error(), "TOL2050") {
		t.Fatalf("expected TOL2050, got: %v", diags)
	}
}

func TestCheckPureFunctionRejectsEmit(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Events: []ast.EventDecl{
				{Name: "Transfer", Params: []ast.FieldDecl{{Name: "amount", Type: "u256"}}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:      "doEmit",
					Modifiers: []string{"public", "pure"},
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "Transfer"},
								Args:   []*ast.Expr{{Kind: "number", Value: "1"}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for pure function emitting event")
	}
	if !strings.Contains(diags.Error(), "TOL2053") {
		t.Fatalf("expected TOL2053, got: %v", diags)
	}
}

func TestCheckViewFunctionRejectsStorageWrite(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{{Name: "counter", Type: "u256"}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:      "badView",
					Modifiers: []string{"public", "view"},
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "counter"},
							Expr:   &ast.Expr{Kind: "number", Value: "1"},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for view function writing storage")
	}
	if !strings.Contains(diags.Error(), "TOL2055") {
		t.Fatalf("expected TOL2055, got: %v", diags)
	}
}

func TestCheckViewFunctionRejectsEmit(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Events: []ast.EventDecl{
				{Name: "Log", Params: []ast.FieldDecl{{Name: "v", Type: "u256"}}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:      "badView",
					Modifiers: []string{"public", "view"},
					Body: []ast.Statement{
						{
							Kind: "emit",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "Log"},
								Args:   []*ast.Expr{{Kind: "number", Value: "42"}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for view function emitting event")
	}
	if !strings.Contains(diags.Error(), "TOL2056") {
		t.Fatalf("expected TOL2056, got: %v", diags)
	}
}

func TestCheckViewFunctionAllowsStorageRead(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{{Name: "counter", Type: "u256"}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:      "getCounter",
					Modifiers: []string{"public", "view"},
					Returns:   []ast.FieldDecl{{Name: "v", Type: "u256"}},
					Body: []ast.Statement{
						{
							Kind: "return",
							Expr: &ast.Expr{Kind: "ident", Value: "counter"},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	// View function is allowed to read storage — should produce no effect errors.
	// (It may produce other errors; we only check that TOL2055/TOL2050 are absent.)
	for _, d := range diags {
		if d.Code == "TOL2055" || d.Code == "TOL2050" {
			t.Fatalf("unexpected storage-read/write error in view function: %v", d)
		}
	}
}

func TestCheckNonPayableMsgValueRejected(t *testing.T) {
	// A view function (which is non-payable) that reads msg.value should get TOL2058.
	// Regular public functions without view/pure are allowed to read msg.value
	// (they may guard it manually); only view functions are restricted.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "getValue",
					Modifiers: []string{"public", "view"},
					Returns:   []ast.FieldDecl{{Name: "v", Type: "u256"}},
					Body: []ast.Statement{
						{
							Kind: "return",
							Expr: &ast.Expr{
								Kind:   "member",
								Object: &ast.Expr{Kind: "ident", Value: "msg"},
								Member: "value",
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for non-payable view function accessing msg.value")
	}
	if !strings.Contains(diags.Error(), "TOL2058") {
		t.Fatalf("expected TOL2058, got: %v", diags)
	}
}

func TestCheckPayableMsgValueAllowed(t *testing.T) {
	// A payable function should be allowed to read msg.value without any error.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "deposit",
					Modifiers: []string{"public", "payable"},
					Returns:   []ast.FieldDecl{{Name: "v", Type: "u256"}},
					Body: []ast.Statement{
						{
							Kind: "return",
							Expr: &ast.Expr{
								Kind:   "member",
								Object: &ast.Expr{Kind: "ident", Value: "msg"},
								Member: "value",
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	for _, d := range diags {
		if d.Code == "TOL2058" {
			t.Fatalf("unexpected TOL2058 for payable function accessing msg.value: %v", d)
		}
	}
}

// TestCheckDuplicateSelectorRejected exercises the existing selector uniqueness
// check (TOL2011) which was already implemented in sema.go.
func TestCheckDuplicateSelectorRejected(t *testing.T) {
	// Two public functions with the same name+types produce the same selector.
	// The existing sema check uses selectorDispatchKey which hashes the signature.
	// Two functions with different names but the same @selector override also collide.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:             "foo",
					Modifiers:        []string{"public"},
					SelectorOverride: "0xaabbccdd",
					Body:             []ast.Statement{{Kind: "return"}},
				},
				{
					Name:             "bar",
					Modifiers:        []string{"public"},
					SelectorOverride: "0xaabbccdd",
					Body:             []ast.Statement{{Kind: "return"}},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics for duplicate selector override")
	}
	if !strings.Contains(diags.Error(), "TOL2011") {
		t.Fatalf("expected TOL2011, got: %v", diags)
	}
}

// =============================================================================
// bytes/string dynamic operations (M3) sema tests
// =============================================================================

// TestCheckBytesConcatAccepted verifies that bytes.concat(a, b) passes sema.
func TestCheckBytesConcatAccepted(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "c",
							Type: "bytes",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Object: &ast.Expr{Kind: "ident", Value: "bytes"},
									Member: "concat",
								},
								Args: []*ast.Expr{
									{Kind: "string", Value: `"0xaa"`},
									{Kind: "string", Value: `"0xbb"`},
								},
							},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for bytes.concat: %v", diags)
	}
}

// TestCheckStringConcatAccepted verifies that string.concat(a, b) passes sema.
func TestCheckStringConcatAccepted(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "c",
							Type: "string",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Object: &ast.Expr{Kind: "ident", Value: "string"},
									Member: "concat",
								},
								Args: []*ast.Expr{
									{Kind: "string", Value: `"hello"`},
									{Kind: "string", Value: `" world"`},
								},
							},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for string.concat: %v", diags)
	}
}

// TestCheckBytesUnsupportedBuiltinRejected verifies that bytes.unknown(...) produces an error.
func TestCheckBytesUnsupportedBuiltinRejected(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "c",
							Type: "bytes",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Object: &ast.Expr{Kind: "ident", Value: "bytes"},
									Member: "unknown",
								},
								Args: []*ast.Expr{},
							},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for bytes.unknown builtin")
	}
	if !strings.Contains(diags.Error(), "TOL2021") {
		t.Fatalf("expected TOL2021, got: %v", diags)
	}
}

// TestCheckLengthMemberAccepted verifies that expr.length passes sema.
func TestCheckLengthMemberAccepted(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "data",
							Type: "bytes",
							Expr: &ast.Expr{Kind: "string", Value: `"0xaabb"`},
						},
						{
							Kind: "let",
							Name: "n",
							Type: "u256",
							Expr: &ast.Expr{
								Kind:   "member",
								Object: &ast.Expr{Kind: "ident", Value: "data"},
								Member: "length",
							},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for .length member: %v", diags)
	}
}

// TestCheckSliceExprAccepted verifies that expr[start:end] passes sema.
func TestCheckSliceExprAccepted(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "data",
							Type: "bytes",
							Expr: &ast.Expr{Kind: "string", Value: `"0xaabbccdd"`},
						},
						{
							Kind: "let",
							Name: "sl",
							Type: "bytes",
							Expr: &ast.Expr{
								Kind:   "slice",
								Object: &ast.Expr{Kind: "ident", Value: "data"},
								Args: []*ast.Expr{
									{Kind: "number", Value: "1"},
									{Kind: "number", Value: "3"},
								},
							},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for slice expr: %v", diags)
	}
}

// =============================================================================
// Try/catch error handling sema tests
// =============================================================================

func TestCheckTryCatchValidExternal(t *testing.T) {
	// A try/catch with a call expression as its target should pass sema.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "try",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "someCall"},
								Args:   []*ast.Expr{},
							},
							Body: []ast.Statement{},
							Catches: []ast.CatchClause{
								{Kind: "", Body: []ast.Statement{}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for valid try/catch: %v", diags)
	}
}

func TestCheckTryCatchDuplicateCatch(t *testing.T) {
	// Two bare catch clauses should produce a TOL2063 error.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "try",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "someCall"},
								Args:   []*ast.Expr{},
							},
							Body: []ast.Statement{},
							Catches: []ast.CatchClause{
								{Kind: "", Body: []ast.Statement{}},
								{Kind: "", Body: []ast.Statement{}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for duplicate bare catch clauses")
	}
	if !strings.Contains(diags.Error(), "TOL2063") {
		t.Fatalf("expected TOL2063 error, got: %v", diags)
	}
}

func TestCheckTryCatchNonCallTarget(t *testing.T) {
	// A try statement with a non-call expression should produce a TOL2062 error.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "try",
							Expr: &ast.Expr{
								Kind:  "ident",
								Value: "notACall",
							},
							Body:    []ast.Statement{},
							Catches: []ast.CatchClause{{Kind: "", Body: []ast.Statement{}}},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for non-call try target")
	}
	if !strings.Contains(diags.Error(), "TOL2062") {
		t.Fatalf("expected TOL2062 error, got: %v", diags)
	}
}

func TestCheckTryCatchPanicValid(t *testing.T) {
	// A try/catch with a valid Panic(code: u256) clause should pass sema.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "try",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "someCall"},
								Args:   []*ast.Expr{},
							},
							Body: []ast.Statement{},
							Catches: []ast.CatchClause{
								{Kind: "Panic", ParamName: "code", ParamType: "u256", Body: []ast.Statement{}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for valid Panic catch: %v", diags)
	}
}

func TestCheckTryCatchPanicWrongType(t *testing.T) {
	// A Panic clause with a non-u256 param type should produce an error.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "try",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "someCall"},
								Args:   []*ast.Expr{},
							},
							Body: []ast.Statement{},
							Catches: []ast.CatchClause{
								{Kind: "Panic", ParamName: "code", ParamType: "string", Body: []ast.Statement{}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for Panic clause with non-u256 type")
	}
}

func TestCheckTryCatchDuplicatePanic(t *testing.T) {
	// Two Panic clauses should produce a TOL2063 error.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "try",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "someCall"},
								Args:   []*ast.Expr{},
							},
							Body: []ast.Statement{},
							Catches: []ast.CatchClause{
								{Kind: "Panic", ParamName: "code", ParamType: "u256", Body: []ast.Statement{}},
								{Kind: "Panic", ParamName: "code2", ParamType: "u256", Body: []ast.Statement{}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for duplicate Panic catch clauses")
	}
	if !strings.Contains(diags.Error(), "TOL2063") {
		t.Fatalf("expected TOL2063 error, got: %v", diags)
	}
}

func TestCheckTryNewExprValid(t *testing.T) {
	// A try/catch with a new expression as its target should pass sema.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "try",
							Expr: &ast.Expr{
								Kind:  "new",
								Value: "Foo",
								Args:  []*ast.Expr{},
							},
							Body: []ast.Statement{},
							Catches: []ast.CatchClause{
								{Kind: "", Body: []ast.Statement{}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for try/new: %v", diags)
	}
}

// --- Struct declaration tests ---

func TestCheckStructDeclValid(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Structs: []ast.StructDecl{
				{
					Name: "Point",
					Fields: []ast.FieldDecl{
						{Name: "x", Type: "u256"},
						{Name: "y", Type: "address"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for valid struct: %v", diags)
	}
}

func TestCheckStructDeclDuplicateName(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Structs: []ast.StructDecl{
				{Name: "Point", Fields: []ast.FieldDecl{{Name: "x", Type: "u256"}}},
				{Name: "Point", Fields: []ast.FieldDecl{{Name: "y", Type: "u256"}}},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected duplicate struct error")
	}
	if !strings.Contains(diags.Error(), "TOL2064") {
		t.Fatalf("expected TOL2064, got: %v", diags)
	}
}

func TestCheckStructFieldUnknownType(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Structs: []ast.StructDecl{
				{
					Name: "Bad",
					Fields: []ast.FieldDecl{
						{Name: "x", Type: "NotAType"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected unknown type error")
	}
	if !strings.Contains(diags.Error(), "TOL2065") {
		t.Fatalf("expected TOL2065, got: %v", diags)
	}
}

func TestCheckStructAsParamType(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Structs: []ast.StructDecl{
				{
					Name:   "Point",
					Fields: []ast.FieldDecl{{Name: "x", Type: "u256"}},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:    "setPoint",
					Params:  []ast.FieldDecl{{Name: "p", Type: "Point"}},
					Returns: []ast.FieldDecl{{Name: "out", Type: "Point"}},
					Body: []ast.Statement{
						{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "p"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for struct param/return: %v", diags)
	}
}

func TestCheckStructLiteralValid(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Structs: []ast.StructDecl{
				{
					Name: "Point",
					Fields: []ast.FieldDecl{
						{Name: "x", Type: "u256"},
						{Name: "y", Type: "u256"},
					},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:    "mk",
					Returns: []ast.FieldDecl{{Name: "p", Type: "Point"}},
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "p",
							Type: "Point",
							Expr: &ast.Expr{
								Kind:  "struct_lit",
								Value: "Point",
								StructFields: []ast.StructFieldInit{
									{Name: "x", Expr: &ast.Expr{Kind: "number", Value: "1"}},
									{Name: "y", Expr: &ast.Expr{Kind: "number", Value: "2"}},
								},
							},
						},
						{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "p"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for valid struct literal: %v", diags)
	}
}

func TestCheckStructLiteralMissingField(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Structs: []ast.StructDecl{
				{
					Name: "Point",
					Fields: []ast.FieldDecl{
						{Name: "x", Type: "u256"},
						{Name: "y", Type: "u256"},
					},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:    "mk",
					Returns: []ast.FieldDecl{{Name: "p", Type: "Point"}},
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "p",
							Type: "Point",
							Expr: &ast.Expr{
								Kind:  "struct_lit",
								Value: "Point",
								StructFields: []ast.StructFieldInit{
									{Name: "x", Expr: &ast.Expr{Kind: "number", Value: "1"}},
									// missing y
								},
							},
						},
						{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "p"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for missing field in struct literal")
	}
	if !strings.Contains(diags.Error(), "TOL2066") {
		t.Fatalf("expected TOL2066, got: %v", diags)
	}
}

func TestCheckStructLiteralUnknownField(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Structs: []ast.StructDecl{
				{
					Name:   "Point",
					Fields: []ast.FieldDecl{{Name: "x", Type: "u256"}},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:    "mk",
					Returns: []ast.FieldDecl{{Name: "p", Type: "Point"}},
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "p",
							Type: "Point",
							Expr: &ast.Expr{
								Kind:  "struct_lit",
								Value: "Point",
								StructFields: []ast.StructFieldInit{
									{Name: "x", Expr: &ast.Expr{Kind: "number", Value: "1"}},
									{Name: "z", Expr: &ast.Expr{Kind: "number", Value: "9"}}, // unknown
								},
							},
						},
						{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "p"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for unknown field in struct literal")
	}
	if !strings.Contains(diags.Error(), "TOL2067") {
		t.Fatalf("expected TOL2067, got: %v", diags)
	}
}

func TestCheckFieldAccessOnNonStruct(t *testing.T) {
	// Field access (member expression) on a non-struct variable is allowed by the
	// current sema (it's not type-checked at the value level). This test confirms
	// that the sema correctly accepts member access syntax (it doesn't know the
	// runtime type of the variable, so it cannot reject it at this stage).
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name: "f",
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "v",
							Type: "u256",
							Expr: &ast.Expr{Kind: "number", Value: "0"},
						},
						{
							Kind: "let",
							Name: "x",
							Type: "u256",
							Expr: &ast.Expr{
								Kind:   "member",
								Object: &ast.Expr{Kind: "ident", Value: "v"},
								Member: "someField",
							},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	// Sema doesn't type-track value types, so member access is not rejected
	// (it will fail at runtime if invalid). We just verify no sema panic/crash.
	_ = diags
}

// --- Abstract contract tests ---

// TestCheckAbstractContractAccepted: abstract contract with a virtual stub function
// (Virtual==true, Body==nil) should produce no errors.
func TestCheckAbstractContractAccepted(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name:     "Base",
			Abstract: true,
			Functions: []ast.FunctionDecl{
				{
					Name:      "transfer",
					Params:    []ast.FieldDecl{{Name: "to", Type: "address"}, {Name: "amount", Type: "u256"}},
					Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
					Modifiers: []string{"public"},
					Virtual:   true,
					Body:      nil, // abstract stub — no body
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for valid abstract contract: %v", diags)
	}
}

// TestCheckAbstractFunctionInConcreteContractRejected: a non-abstract contract that
// declares a function with Virtual==true and Body==nil must be rejected with TOL2069.
func TestCheckAbstractFunctionInConcreteContractRejected(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name:     "Token",
			Abstract: false,
			Functions: []ast.FunctionDecl{
				{
					Name:      "transfer",
					Params:    []ast.FieldDecl{{Name: "to", Type: "address"}, {Name: "amount", Type: "u256"}},
					Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
					Modifiers: []string{"public"},
					Virtual:   true,
					Body:      nil, // missing body — error in non-abstract contract
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for abstract function in concrete contract")
	}
	if !strings.Contains(diags.Error(), "TOL2069") {
		t.Fatalf("expected TOL2069, got: %v", diags)
	}
	if !strings.Contains(diags.Error(), "transfer") {
		t.Fatalf("expected 'transfer' in error message, got: %v", diags)
	}
}

// TestCheckConcreteContractMustImplementAbstractFunctions: a concrete contract that
// inherits from an abstract contract but does not implement all abstract stubs must
// be rejected with TOL2070.
func TestCheckConcreteContractMustImplementAbstractFunctions(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		AbstractContracts: []ast.ContractDecl{
			{
				Name:     "Base",
				Abstract: true,
				Functions: []ast.FunctionDecl{
					{
						Name:      "transfer",
						Params:    []ast.FieldDecl{{Name: "to", Type: "address"}, {Name: "amount", Type: "u256"}},
						Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
						Modifiers: []string{"public"},
						Virtual:   true,
						Body:      nil,
					},
				},
			},
		},
		Contract: &ast.ContractDecl{
			Name:      "Token",
			Abstract:  false,
			Bases:     []string{"Base"},
			Functions: []ast.FunctionDecl{}, // missing transfer implementation
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for missing abstract function implementation")
	}
	if !strings.Contains(diags.Error(), "TOL2070") {
		t.Fatalf("expected TOL2070, got: %v", diags)
	}
	if !strings.Contains(diags.Error(), "transfer") {
		t.Fatalf("expected 'transfer' in error message, got: %v", diags)
	}
}

// TestCheckConcreteContractImplementsAllAbstractFunctions: a concrete contract that
// properly implements all abstract stubs should produce no errors.
func TestCheckConcreteContractImplementsAllAbstractFunctions(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		AbstractContracts: []ast.ContractDecl{
			{
				Name:     "Base",
				Abstract: true,
				Functions: []ast.FunctionDecl{
					{
						Name:      "transfer",
						Params:    []ast.FieldDecl{{Name: "to", Type: "address"}, {Name: "amount", Type: "u256"}},
						Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
						Modifiers: []string{"public"},
						Virtual:   true,
						Body:      nil,
					},
				},
			},
		},
		Contract: &ast.ContractDecl{
			Name:     "Token",
			Abstract: false,
			Bases:    []string{"Base"},
			Functions: []ast.FunctionDecl{
				{
					Name:      "transfer",
					Params:    []ast.FieldDecl{{Name: "to", Type: "address"}, {Name: "amount", Type: "u256"}},
					Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
					Modifiers: []string{"public"},
					Override:  true,
					Body: []ast.Statement{
						{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "true"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for contract implementing all abstract functions: %v", diags)
	}
}

// indexExpr builds a nested index expression: arr[k0][k1]...[kN].
func indexExpr(base *ast.Expr, keys ...*ast.Expr) *ast.Expr {
	cur := base
	for _, k := range keys {
		cur = &ast.Expr{Kind: "index", Object: cur, Index: k}
	}
	return cur
}

func identExpr(name string) *ast.Expr { return &ast.Expr{Kind: "ident", Value: name} }
func numExpr(v string) *ast.Expr      { return &ast.Expr{Kind: "number", Value: v} }

func TestCheckNestedArrayIndexAccepted(t *testing.T) {
	// u256[][] arr; fn f(i: u256, j: u256) { set got = arr[i][j]; }
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{{Name: "arr", Type: "u256[][]"}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:   "f",
					Params: []ast.FieldDecl{{Name: "i", Type: "u256"}, {Name: "j", Type: "u256"}},
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: identExpr("got"),
							Expr:   indexExpr(identExpr("arr"), identExpr("i"), identExpr("j")),
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for nested array index read: %v", diags)
	}
}

func TestCheckNestedMappingAccepted(t *testing.T) {
	// mapping(address => mapping(address => u256)) m; fn f(a: address, b: address) { set got = m[a][b]; }
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{{Name: "m", Type: "mapping(address => mapping(address => u256))"}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:   "f",
					Params: []ast.FieldDecl{{Name: "a", Type: "address"}, {Name: "b", Type: "address"}},
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: identExpr("got"),
							Expr:   indexExpr(identExpr("m"), identExpr("a"), identExpr("b")),
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for nested mapping read: %v", diags)
	}
}

func TestCheckMappingToArrayAccepted(t *testing.T) {
	// mapping(address => u256[]) bal; fn f(addr: address, i: u256) { set got = bal[addr][i]; }
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{{Name: "bal", Type: "mapping(address => u256[])"}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:   "f",
					Params: []ast.FieldDecl{{Name: "addr", Type: "address"}, {Name: "i", Type: "u256"}},
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: identExpr("got"),
							Expr:   indexExpr(identExpr("bal"), identExpr("addr"), identExpr("i")),
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for mapping-to-array read: %v", diags)
	}
}

func TestCheckTripleIndexRejected(t *testing.T) {
	// u256[] arr; fn f(i: u256, j: u256) { set got = arr[i][j]; } — too deep
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{{Name: "arr", Type: "u256[]"}},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:   "f",
					Params: []ast.FieldDecl{{Name: "i", Type: "u256"}, {Name: "j", Type: "u256"}},
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: identExpr("got"),
							Expr:   indexExpr(identExpr("arr"), identExpr("i"), identExpr("j")),
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for over-indexed 1D array, got none")
	}
	if !strings.Contains(diags.Error(), "TOL2018") {
		t.Fatalf("expected TOL2018, got: %v", diags)
	}
}

// --- Overloading tests ---

func TestCheckOverloadDistinctParamTypes(t *testing.T) {
	// Two functions with the same name but different parameter counts must be allowed.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "compute",
					Params:    []ast.FieldDecl{{Name: "x", Type: "u256"}},
					Returns:   []ast.FieldDecl{{Name: "r", Type: "u256"}},
					Modifiers: []string{"public"},
					Body:      []ast.Statement{{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "x"}}},
				},
				{
					Name:   "compute",
					Params: []ast.FieldDecl{{Name: "x", Type: "u256"}, {Name: "y", Type: "u256"}},
					Returns: []ast.FieldDecl{{Name: "r", Type: "u256"}},
					Modifiers: []string{"public"},
					Body: []ast.Statement{{Kind: "return", Expr: &ast.Expr{
						Kind:  "binary",
						Op:    "+",
						Left:  &ast.Expr{Kind: "ident", Value: "x"},
						Right: &ast.Expr{Kind: "ident", Value: "y"},
					}}},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for overloaded functions with distinct param counts: %v", diags)
	}
}

func TestCheckOverloadSameSignatureRejected(t *testing.T) {
	// Two functions with the same name AND the same parameter types must be rejected.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "compute",
					Params:    []ast.FieldDecl{{Name: "x", Type: "u256"}},
					Returns:   []ast.FieldDecl{{Name: "r", Type: "u256"}},
					Modifiers: []string{"public"},
					Body:      []ast.Statement{{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "x"}}},
				},
				{
					Name:      "compute",
					Params:    []ast.FieldDecl{{Name: "a", Type: "u256"}},
					Returns:   []ast.FieldDecl{{Name: "r", Type: "u256"}},
					Modifiers: []string{"public"},
					Body:      []ast.Statement{{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "a"}}},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for functions with duplicate name and same parameter types")
	}
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "compute") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected duplicate function diagnostic to mention 'compute', got: %v", diags)
	}
}

func TestCheckOverloadCallResolution(t *testing.T) {
	// Calling an overloaded function with different arities must not trigger arity errors.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "compute",
					Params:    []ast.FieldDecl{{Name: "x", Type: "u256"}},
					Returns:   []ast.FieldDecl{{Name: "r", Type: "u256"}},
					Modifiers: []string{"public"},
					Body:      []ast.Statement{{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "x"}}},
				},
				{
					Name:   "compute",
					Params: []ast.FieldDecl{{Name: "x", Type: "u256"}, {Name: "y", Type: "u256"}},
					Returns: []ast.FieldDecl{{Name: "r", Type: "u256"}},
					Modifiers: []string{"public"},
					Body:      []ast.Statement{{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "x"}}},
				},
				{
					Name:      "caller",
					Params:    []ast.FieldDecl{},
					Returns:   []ast.FieldDecl{{Name: "r", Type: "u256"}},
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "let",
							Name: "a",
							Type: "u256",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "compute"},
								Args:   []*ast.Expr{{Kind: "number", Value: "1"}},
							},
						},
						{
							Kind: "let",
							Name: "b",
							Type: "u256",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "compute"},
								Args:   []*ast.Expr{{Kind: "number", Value: "2"}, {Kind: "number", Value: "3"}},
							},
						},
						{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "a"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for overloaded call resolution: %v", diags)
	}
}

// --- Dynamic array parameter/return tests ---

// TestCheckArrayParamAccepted verifies that a function with a dynamic array parameter
// (u256[]) is accepted by the sema checker.
func TestCheckArrayParamAccepted(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "sumArr",
					Params:    []ast.FieldDecl{{Name: "arr", Type: "u256[]"}},
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for array param function: %v", diags)
	}
}

// TestCheckArrayReturnAccepted verifies that a function returning a dynamic array
// (u256[]) is accepted by the sema checker.
func TestCheckArrayReturnAccepted(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "getArr",
					Params:    []ast.FieldDecl{},
					Returns:   []ast.FieldDecl{{Name: "result", Type: "u256[]"}},
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "result"}},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for array return function: %v", diags)
	}
}

// TestCheckConstructorArrayParamAccepted verifies that a constructor with an address[]
// parameter is accepted by the sema checker (array ABI decode is now supported).
func TestCheckConstructorArrayParamAccepted(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Constructor: &ast.ConstructorDecl{
				Params: []ast.FieldDecl{
					{Name: "tokens", Type: "address[]"},
				},
				Body: []ast.Statement{
					{Kind: "return"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for constructor with address[] param: %v", diags)
	}
}

// typeMinMaxExpr builds the AST for type(T).bound.
func typeMinMaxExpr(typeName, bound string) *ast.Expr {
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

// TestCheckTypeBoundsAccepted verifies that type(uN).min/max and type(iN).min/max
// are accepted by the sema checker.
func TestCheckTypeBoundsAccepted(t *testing.T) {
	cases := []struct {
		typeName string
		bound    string
	}{
		{"u8", "min"}, {"u8", "max"},
		{"u256", "min"}, {"u256", "max"},
		{"u128", "min"}, {"u128", "max"},
		{"i8", "min"}, {"i8", "max"},
		{"i256", "min"}, {"i256", "max"},
	}
	for _, tc := range cases {
		m := &ast.Module{
			Version: "0.2.0",
			Contract: &ast.ContractDecl{
				Name: "Demo",
				Functions: []ast.FunctionDecl{
					{
						Name:      "f",
						Modifiers: []string{"public"},
						Returns:   []ast.FieldDecl{{Name: "v", Type: tc.typeName}},
						Body: []ast.Statement{
							{
								Kind: "return",
								Expr: typeMinMaxExpr(tc.typeName, tc.bound),
							},
						},
					},
				},
			},
		}
		_, diags := Check("<test>", m)
		if diags.HasErrors() {
			t.Errorf("type(%s).%s: unexpected diagnostics: %v", tc.typeName, tc.bound, diags)
		}
	}
}

// TestCheckTypeBoundsRejectsNonIntegerType verifies that type(bool).max and
// type(address).max are rejected with TOL2034.
func TestCheckTypeBoundsRejectsNonIntegerType(t *testing.T) {
	invalidTypes := []string{"bool", "address", "bytes", "bytes32", "string"}
	for _, typName := range invalidTypes {
		for _, bound := range []string{"min", "max"} {
			m := &ast.Module{
				Version: "0.2.0",
				Contract: &ast.ContractDecl{
					Name: "Demo",
					Functions: []ast.FunctionDecl{
						{
							Name:      "f",
							Modifiers: []string{"public"},
							Body: []ast.Statement{
								{
									Kind: "return",
									Expr: typeMinMaxExpr(typName, bound),
								},
							},
						},
					},
				},
			}
			_, diags := Check("<test>", m)
			if !diags.HasErrors() {
				t.Errorf("type(%s).%s: expected TOL2034 diagnostic but got none", typName, bound)
				continue
			}
			found := false
			for _, d := range diags {
				if d.Code == "TOL2034" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("type(%s).%s: expected TOL2034 diagnostic, got: %v", typName, bound, diags)
			}
		}
	}
}

// TestCheckDoWhileValid verifies that a do/while statement with a valid
// boolean condition is accepted.
func TestCheckDoWhileValid(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "run",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{Kind: "let", Name: "i", Type: "u256", Expr: &ast.Expr{Kind: "number", Value: "0"}},
						{
							Kind: "dowhile",
							Cond: &ast.Expr{Kind: "binary", Op: "<",
								Left:  &ast.Expr{Kind: "ident", Value: "i"},
								Right: &ast.Expr{Kind: "number", Value: "10"},
							},
							Body: []ast.Statement{
								{Kind: "set", Target: &ast.Expr{Kind: "ident", Value: "i"},
									Expr: &ast.Expr{Kind: "binary", Op: "+",
										Left:  &ast.Expr{Kind: "ident", Value: "i"},
										Right: &ast.Expr{Kind: "number", Value: "1"},
									},
								},
							},
						},
						{Kind: "return"},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for do/while: %v", diags)
	}
}

// TestCheckReceivePayableValid verifies that a receive() payable declaration
// is accepted by sema.
func TestCheckReceivePayableValid(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Wallet",
			Receive: &ast.ReceiveDecl{
				Body: []ast.Statement{{Kind: "return"}},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for receive(): %v", diags)
	}
}

// TestCheckTransferOnPayableAddressOK verifies that calling .transfer() on an
// "address payable" variable is accepted with no diagnostics.
func TestCheckTransferOnPayableAddressOK(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Wallet",
			Functions: []ast.FunctionDecl{
				{
					Name: "sendFunds",
					Params: []ast.FieldDecl{
						{Name: "recipient", Type: "address payable"},
						{Name: "amount", Type: "u256"},
					},
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Object: &ast.Expr{Kind: "ident", Value: "recipient"},
									Member: "transfer",
								},
								Args: []*ast.Expr{
									{Kind: "ident", Value: "amount"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for .transfer() on address payable: %v", diags)
	}
}

// TestCheckTransferOnPlainAddressRejected verifies that calling .transfer() on a
// plain "address" variable is rejected with TOL2085.
func TestCheckTransferOnPlainAddressRejected(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Wallet",
			Functions: []ast.FunctionDecl{
				{
					Name: "badSend",
					Params: []ast.FieldDecl{
						{Name: "recipient", Type: "address"},
						{Name: "amount", Type: "u256"},
					},
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind:   "member",
									Object: &ast.Expr{Kind: "ident", Value: "recipient"},
									Member: "transfer",
								},
								Args: []*ast.Expr{
									{Kind: "ident", Value: "amount"},
								},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatal("expected TOL2085 for .transfer() on plain address, got no errors")
	}
	if !strings.Contains(diags.Error(), "TOL2085") {
		t.Fatalf("expected TOL2085 in diagnostics, got: %v", diags)
	}
}

// TestCheckPayableCastAllowsTransfer verifies that payable(addr).transfer(amount)
// is accepted (the payable() cast makes it address payable).
func TestCheckPayableCastAllowsTransfer(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Wallet",
			Functions: []ast.FunctionDecl{
				{
					Name:      "sendViaPayable",
					Params:    []ast.FieldDecl{{Name: "recipient", Type: "address"}, {Name: "amount", Type: "u256"}},
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind: "call",
								Callee: &ast.Expr{
									Kind: "member",
									Object: &ast.Expr{
										Kind:  "call",
										Callee: &ast.Expr{Kind: "ident", Value: "payable"},
										Args:  []*ast.Expr{{Kind: "ident", Value: "recipient"}},
									},
									Member: "transfer",
								},
								Args: []*ast.Expr{{Kind: "ident", Value: "amount"}},
							},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for payable(addr).transfer(): %v", diags)
	}
}

// ── bytes_eq / string_eq and == rejection ───────────────────────────────────

// TestBytesEqBuiltinAccepted verifies that bytes_eq(a, b) is accepted (arity 2).
func TestBytesEqBuiltinAccepted(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "C",
			Functions: []ast.FunctionDecl{{
				Name:      "check",
				Params:    []ast.FieldDecl{{Name: "a", Type: "bytes"}, {Name: "b", Type: "bytes"}},
				Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
				Modifiers: []string{"public", "pure"},
				Body: []ast.Statement{
					{Kind: "let", Name: "ok", Type: "bool", Expr: &ast.Expr{
						Kind:   "call",
						Callee: &ast.Expr{Kind: "ident", Value: "bytes_eq"},
						Args:   []*ast.Expr{{Kind: "ident", Value: "a"}, {Kind: "ident", Value: "b"}},
					}},
					{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "ok"}},
				},
			}},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for bytes_eq: %v", diags)
	}
}

// TestStringEqBuiltinAccepted verifies that string_eq(a, b) is accepted (arity 2).
func TestStringEqBuiltinAccepted(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "C",
			Functions: []ast.FunctionDecl{{
				Name:      "check",
				Params:    []ast.FieldDecl{{Name: "a", Type: "string"}, {Name: "b", Type: "string"}},
				Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
				Modifiers: []string{"public", "pure"},
				Body: []ast.Statement{
					{Kind: "let", Name: "ok", Type: "bool", Expr: &ast.Expr{
						Kind:   "call",
						Callee: &ast.Expr{Kind: "ident", Value: "string_eq"},
						Args:   []*ast.Expr{{Kind: "ident", Value: "a"}, {Kind: "ident", Value: "b"}},
					}},
					{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "ok"}},
				},
			}},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics for string_eq: %v", diags)
	}
}

// TestBytesEqualityOperatorRejected verifies that bytes == bytes emits TOL2086.
func TestBytesEqualityOperatorRejected(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "C",
			Functions: []ast.FunctionDecl{{
				Name:      "check",
				Params:    []ast.FieldDecl{{Name: "a", Type: "bytes"}, {Name: "b", Type: "bytes"}},
				Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
				Modifiers: []string{"public", "pure"},
				Body: []ast.Statement{
					{Kind: "let", Name: "ok", Type: "bool", Expr: &ast.Expr{
						Kind:  "binary",
						Op:    "==",
						Left:  &ast.Expr{Kind: "ident", Value: "a"},
						Right: &ast.Expr{Kind: "ident", Value: "b"},
					}},
					{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "ok"}},
				},
			}},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatal("expected TOL2086 for bytes == bytes, but got no errors")
	}
	found := false
	for _, d := range diags {
		if d.Code == "TOL2086" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected TOL2086, got: %v", diags)
	}
}

// TestStringInequalityOperatorRejected verifies that string != string emits TOL2086.
func TestStringInequalityOperatorRejected(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "C",
			Functions: []ast.FunctionDecl{{
				Name:      "check",
				Params:    []ast.FieldDecl{{Name: "a", Type: "string"}, {Name: "b", Type: "string"}},
				Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
				Modifiers: []string{"public", "pure"},
				Body: []ast.Statement{
					{Kind: "let", Name: "ok", Type: "bool", Expr: &ast.Expr{
						Kind:  "binary",
						Op:    "!=",
						Left:  &ast.Expr{Kind: "ident", Value: "a"},
						Right: &ast.Expr{Kind: "ident", Value: "b"},
					}},
					{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "ok"}},
				},
			}},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatal("expected TOL2086 for string != string, but got no errors")
	}
	found := false
	for _, d := range diags {
		if d.Code == "TOL2086" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected TOL2086, got: %v", diags)
	}
}

// TestBytesEqArityRejected verifies that bytes_eq with wrong arity emits an error.
func TestBytesEqArityRejected(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "C",
			Functions: []ast.FunctionDecl{{
				Name:      "check",
				Params:    []ast.FieldDecl{{Name: "a", Type: "bytes"}},
				Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
				Modifiers: []string{"public", "pure"},
				Body: []ast.Statement{
					{Kind: "let", Name: "ok", Type: "bool", Expr: &ast.Expr{
						Kind:   "call",
						Callee: &ast.Expr{Kind: "ident", Value: "bytes_eq"},
						Args:   []*ast.Expr{{Kind: "ident", Value: "a"}}, // only 1 arg, should be 2
					}},
					{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "ok"}},
				},
			}},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatal("expected arity error for bytes_eq(a), but got no errors")
	}
}

// TestCircularImportDetected verifies that a mutual import (A imports B, B imports A)
// emits TOL2095 instead of a confusing "name not found" error.
func TestCircularImportDetected(t *testing.T) {
	// a.tol imports IFoo from b.tol; b.tol imports IBar from a.tol — direct cycle.
	bSrc := []byte(`pragma tolang 0.2.0;
import IBar from "a.tol";
interface IFoo {}
`)

	resolver := newMapResolver(map[string][]byte{
		"b.tol": bSrc,
		// a.tol is the root file being compiled; b.tol imports it back via "a.tol".
		"a.tol": []byte(`pragma tolang 0.2.0;
import IFoo from "b.tol";
contract A {}
`),
	})

	aMod := &ast.Module{
		Version: "0.2.0",
		Imports: []ast.ImportDecl{
			{Name: "IFoo", Path: "b.tol", Line: 2},
		},
		Contract: &ast.ContractDecl{
			Name: "A",
		},
	}

	_, diags := CheckWithResolver("a.tol", aMod, resolver)
	if !diags.HasErrors() {
		t.Fatal("expected TOL2095 for circular import, but got no errors")
	}
	found := false
	for _, d := range diags {
		if d.Code == "TOL2095" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected TOL2095, got: %v", diags)
	}
}

// TestBytesStorageSlotEqualityRejected verifies that comparing a bytes storage slot with == emits TOL2086.
func TestBytesStorageSlotEqualityRejected(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "C",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{{Name: "data", Type: "bytes"}},
			},
			Functions: []ast.FunctionDecl{{
				Name:      "check",
				Params:    []ast.FieldDecl{{Name: "b", Type: "bytes"}},
				Returns:   []ast.FieldDecl{{Name: "ok", Type: "bool"}},
				Modifiers: []string{"public", "view"},
				Body: []ast.Statement{
					{Kind: "let", Name: "ok", Type: "bool", Expr: &ast.Expr{
						Kind:  "binary",
						Op:    "==",
						Left:  &ast.Expr{Kind: "ident", Value: "data"},
						Right: &ast.Expr{Kind: "ident", Value: "b"},
					}},
					{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "ok"}},
				},
			}},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatal("expected TOL2086 for storage bytes == bytes, but got no errors")
	}
	found := false
	for _, d := range diags {
		if d.Code == "TOL2086" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected TOL2086, got: %v", diags)
	}
}

func TestEffectDeclaredOK(t *testing.T) {
	// A function with correct @effects (reads storage.x, writes/emits/calls empty) should not emit TOL2200.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					{Name: "x", Type: "u256"},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:      "getX",
					Modifiers: []string{"public", "view"},
					Returns:   []ast.FieldDecl{{Name: "v", Type: "u256"}},
					Body: []ast.Statement{
						{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "x"}},
					},
					Doc: &ast.DocMeta{
						Effects: &ast.EffectDecl{
							Reads:  []string{"storage.x"},
							Writes: []string{},
							Emits:  []string{},
							Calls:  []ast.CallRef{},
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	for _, d := range diags {
		if d.Code == "TOL2200" {
			t.Errorf("unexpected TOL2200: %s", d.Message)
		}
	}
}

func TestEffectUndeclaredWrite(t *testing.T) {
	// Writing storage.x without declaring it must fire TOL2200.
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					{Name: "x", Type: "u256"},
				},
			},
			Functions: []ast.FunctionDecl{
				{
					Name:      "setX",
					Modifiers: []string{"public"},
					Params:    []ast.FieldDecl{{Name: "v", Type: "u256"}},
					Body: []ast.Statement{
						{
							Kind:   "set",
							Target: &ast.Expr{Kind: "ident", Value: "x"},
							Expr:   &ast.Expr{Kind: "ident", Value: "v"},
						},
					},
					Doc: &ast.DocMeta{
						Effects: &ast.EffectDecl{
							Reads:  []string{},
							Writes: []string{}, // x not declared here — should trigger TOL2200
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	found := false
	for _, d := range diags {
		if d.Code == "TOL2200" {
			found = true
		}
	}
	if !found {
		t.Error("expected TOL2200 for undeclared write to storage.x")
	}
}

func TestEffectEmptyCallsViolated(t *testing.T) {
	// Declaring calls:[] but making an external call must fire TOL2204.
	callsEmpty := []ast.CallRef{} // non-nil but empty = "calls: []"
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{
					Name:      "bad",
					Modifiers: []string{"public"},
					Body: []ast.Statement{
						{
							Kind: "expr",
							Expr: &ast.Expr{
								Kind:   "call",
								Callee: &ast.Expr{Kind: "ident", Value: "call"},
								Args:   []*ast.Expr{},
							},
						},
					},
					Doc: &ast.DocMeta{
						Effects: &ast.EffectDecl{
							Calls: callsEmpty,
						},
					},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	found := false
	for _, d := range diags {
		if d.Code == "TOL2204" {
			found = true
		}
	}
	if !found {
		t.Error("expected TOL2204 for violated calls:[]")
	}
}

// --- User-defined value type (UDVT) tests ---

func TestUDVTValidElementaryUnderlying(t *testing.T) {
	// Valid: underlying is an elementary value type.
	// Note: the lexer normalizes Solidity aliases (uint256→u256, int128→i128 etc.) before
	// the AST reaches sema, so we only need to test the canonical internal forms here.
	for _, underlying := range []string{"u256", "u8", "i128", "bool", "address", "bytes32", "bytes1"} {
		m := &ast.Module{
			Version: "0.2.0",
			TypeDecls: []ast.TypeDecl{
				{Name: "MyType", Underlying: underlying},
			},
			Contract: &ast.ContractDecl{
				Name: "Demo",
				Functions: []ast.FunctionDecl{
					{Name: "run", Modifiers: []string{"public"}, Body: []ast.Statement{{Kind: "return"}}},
				},
			},
		}
		_, diags := Check("<test>", m)
		for _, d := range diags {
			if d.Code == "TOL2096" {
				t.Errorf("unexpected TOL2096 for underlying type %q: %s", underlying, d.Message)
			}
		}
	}
}

func TestUDVTInvalidUnderlyingArray(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		TypeDecls: []ast.TypeDecl{
			{Name: "MyArr", Underlying: "uint256[]"},
		},
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{Name: "run", Modifiers: []string{"public"}, Body: []ast.Statement{{Kind: "return"}}},
			},
		},
	}
	_, diags := Check("<test>", m)
	found := false
	for _, d := range diags {
		if d.Code == "TOL2096" {
			found = true
		}
	}
	if !found {
		t.Error("expected TOL2096 for UDVT with array underlying type")
	}
}

func TestUDVTInvalidUnderlyingMapping(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		TypeDecls: []ast.TypeDecl{
			{Name: "MyMap", Underlying: "mapping(address=>uint256)"},
		},
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Functions: []ast.FunctionDecl{
				{Name: "run", Modifiers: []string{"public"}, Body: []ast.Statement{{Kind: "return"}}},
			},
		},
	}
	_, diags := Check("<test>", m)
	found := false
	for _, d := range diags {
		if d.Code == "TOL2096" {
			found = true
		}
	}
	if !found {
		t.Error("expected TOL2096 for UDVT with mapping underlying type")
	}
}

// =============================================================================
// Task #12 — Contract body, state var modifiers, UDVT sema tests
// =============================================================================

// TestCheckUDVTInContract verifies that type X is Y; inside a contract body is accepted.
func TestCheckUDVTInContract(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			TypeDecls: []ast.TypeDecl{
				// Use TOL canonical form (u256, not uint256) since the lexer normalizes.
				{Name: "Price", Underlying: "u256"},
				{Name: "Status", Underlying: "u8"},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected error for UDVT in contract: %v", diags)
	}
}

// TestCheckMappingKeyEnum verifies that enum types are accepted as mapping keys.
func TestCheckMappingKeyEnum(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Enums: []ast.EnumDecl{
				{Name: "Status", Members: []string{"Active", "Inactive"}},
			},
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					// Use TOL canonical form u256 (uint256 is normalized by the lexer at parse time).
					{Name: "statusMap", Type: "mapping(Status => u256)"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected error for mapping with enum key: %v", diags)
	}
}

// TestCheckMappingKeyStringRejected verifies that string is still rejected as a mapping key.
func TestCheckMappingKeyStringRejected(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					{Name: "strMap", Type: "mapping(string => uint256)"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if !diags.HasErrors() {
		t.Fatalf("expected error for mapping with string key")
	}
}

// TestCheckStateVarVisibilityOK verifies that visibility modifiers on state vars are accepted.
func TestCheckStateVarVisibilityOK(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Contract: &ast.ContractDecl{
			Name: "Demo",
			Storage: &ast.StorageDecl{
				Slots: []ast.StorageSlot{
					// Use TOL canonical form u256 (uint256 is normalized by the lexer at parse time).
					{Name: "totalSupply", Type: "u256", Visibility: "public"},
					{Name: "owner", Type: "address", Visibility: "private"},
					{Name: "counter", Type: "u256", Visibility: "internal"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected error for state vars with visibility: %v", diags)
	}
}


// ── Task #11: Top-level declarations and all import forms ─────────────────────

// TestCheckTopLevelDeclarationsNoContract verifies that a file with only top-level
// declarations (free functions, constants, enums, errors) and no contract is valid.
func TestCheckTopLevelDeclarationsNoContract(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		FreeFunctions: []ast.FunctionDecl{
			{
				Name:      "helper",
				Params:    []ast.FieldDecl{{Name: "x", Type: "u256"}},
				Returns:   []ast.FieldDecl{{Name: "y", Type: "u256"}},
				Modifiers: []string{"internal", "pure"},
				Body:      []ast.Statement{{Kind: "return", Expr: &ast.Expr{Kind: "ident", Value: "x"}}},
			},
		},
		Constants: []ast.ConstantDecl{
			{Name: "MAX", Type: "u256", Value: &ast.Expr{Kind: "number", Value: "9999"}},
		},
		Enums: []ast.EnumDecl{
			{Name: "Status", Members: []string{"Active", "Inactive"}},
		},
		Errors: []ast.ErrorDecl{
			{Name: "NotAllowed", Params: []ast.FieldDecl{}},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors for top-level-only module: %v", diags)
	}
}

// TestCheckTopLevelEventsNoContract verifies that a file with top-level events and no contract is valid.
func TestCheckTopLevelEventsNoContract(t *testing.T) {
	m := &ast.Module{
		Version: "0.2.0",
		Events: []ast.EventDecl{
			{
				Name: "Transfer",
				Params: []ast.FieldDecl{
					{Name: "from", Type: "address", Indexed: true},
					{Name: "to", Type: "address", Indexed: true},
					{Name: "value", Type: "u256"},
				},
			},
		},
	}
	_, diags := Check("<test>", m)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors for module with only top-level events: %v", diags)
	}
}

// TestResolveImportsNamedSymbols verifies that import { A, B as C } from "path";
// merges the named interfaces/libraries from the referenced module.
func TestResolveImportsNamedSymbols(t *testing.T) {
	libSrc := []byte(`pragma tolang 0.2.0;
interface IFoo { function foo() external; }
interface IBar { function bar() external; }
contract Unused {}
`)

	resolver := newMapResolver(map[string][]byte{
		"lib.tol": libSrc,
	})

	m := &ast.Module{
		Version: "0.2.0",
		Imports: []ast.ImportDecl{
			{
				Path: "lib.tol",
				Named: []ast.ImportAlias{
					{Name: "IFoo", Alias: ""},
					{Name: "IBar", Alias: "MyBar"},
				},
				Line: 2,
			},
		},
		Contract: &ast.ContractDecl{Name: "Main"},
	}

	_, diags := CheckWithResolver("<test>", m, resolver)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors for named import: %v", diags)
	}
	// After resolving, IFoo and IBar (as MyBar) should be in m.Interfaces.
	found := map[string]bool{}
	for _, iface := range m.Interfaces {
		found[iface.Name] = true
	}
	if !found["IFoo"] {
		t.Error("expected IFoo to be imported")
	}
	if !found["MyBar"] {
		t.Error("expected IBar to be imported as MyBar")
	}
}

// TestResolveImportsStarSyntax verifies that import * as Lib from "path"; merges
// all interfaces and libraries from the referenced module (with alias prefix).
func TestResolveImportsStarSyntax(t *testing.T) {
	libSrc := []byte(`pragma tolang 0.2.0;
interface IFoo { function foo() external; }
library SafeMath { function add(u256 a, u256 b) internal pure returns (u256 c) { return a; } }
contract Unused {}
`)

	resolver := newMapResolver(map[string][]byte{
		"lib.tol": libSrc,
	})

	m := &ast.Module{
		Version: "0.2.0",
		Imports: []ast.ImportDecl{
			{
				Path:   "lib.tol",
				Alias:  "Lib",
				IsStar: true,
				Line:   2,
			},
		},
		Contract: &ast.ContractDecl{Name: "Main"},
	}

	_, diags := CheckWithResolver("<test>", m, resolver)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors for star import: %v", diags)
	}
	// After resolving, IFoo and SafeMath should be in m.Interfaces/m.Libraries
	// with "Lib." prefix.
	foundIface := false
	for _, iface := range m.Interfaces {
		if iface.Name == "Lib.IFoo" {
			foundIface = true
		}
	}
	if !foundIface {
		t.Errorf("expected Lib.IFoo in interfaces after star import, got: %v", m.Interfaces)
	}
	foundLib := false
	for _, lib := range m.Libraries {
		if lib.Name == "Lib.SafeMath" {
			foundLib = true
		}
	}
	if !foundLib {
		t.Errorf("expected Lib.SafeMath in libraries after star import, got: %v", m.Libraries)
	}
}

// TestResolveImportsBareNoError verifies that import "path"; (no alias) is accepted
// as a side-effect-only import without producing "no entity found" errors.
func TestResolveImportsBareNoError(t *testing.T) {
	libSrc := []byte(`pragma tolang 0.2.0;
interface IFoo { function foo() external; }
contract Impl {}
`)

	resolver := newMapResolver(map[string][]byte{
		"lib.tol": libSrc,
	})

	m := &ast.Module{
		Version: "0.2.0",
		Imports: []ast.ImportDecl{
			// Bare import: Name="", Alias="", IsStar=false, Named=nil
			{Path: "lib.tol", Line: 2},
		},
		Contract: &ast.ContractDecl{Name: "Main"},
	}

	_, diags := CheckWithResolver("<test>", m, resolver)
	if diags.HasErrors() {
		t.Fatalf("bare import should not produce errors, got: %v", diags)
	}
}
