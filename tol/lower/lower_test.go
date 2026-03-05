package lower

import (
	"testing"

	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/sema"
)

func TestFromTypedBuildsProgram(t *testing.T) {
	typed := &sema.TypedModule{
		AST: &ast.Module{
			Version: "0.2",
			Contract: &ast.ContractDecl{
				Name: "Demo",
				Storage: &ast.StorageDecl{
					Slots: []ast.StorageSlot{
						{Name: "total_supply", Type: "u256"},
					},
				},
				Functions: []ast.FunctionDecl{
					{
						Name:             "transfer",
						SelectorOverride: "0x1234abcd",
						Params: []ast.FieldDecl{
							{Name: "to", Type: "address"},
							{Name: "amount", Type: "u256"},
						},
						Body: []ast.Statement{
							{Kind: "return"},
						},
					},
				},
				Constructor: &ast.ConstructorDecl{},
				Fallback:    &ast.FallbackDecl{},
			},
		},
	}

	prog, err := FromTyped(typed)
	if err != nil {
		t.Fatalf("unexpected lower error: %v", err)
	}
	if prog == nil {
		t.Fatalf("expected lowered program")
	}
	if prog.ContractName != "Demo" {
		t.Fatalf("unexpected contract name: %s", prog.ContractName)
	}
	if len(prog.StorageSlots) != 1 || prog.StorageSlots[0].Name != "total_supply" {
		t.Fatalf("unexpected storage slots: %#v", prog.StorageSlots)
	}
	if len(prog.Functions) != 1 || prog.Functions[0].Name != "transfer" {
		t.Fatalf("unexpected functions: %#v", prog.Functions)
	}
	if prog.Functions[0].SelectorOverride != "0x1234abcd" {
		t.Fatalf("unexpected function selector override: %q", prog.Functions[0].SelectorOverride)
	}
	if !prog.HasConstructor || !prog.HasFallback {
		t.Fatalf("unexpected ctor/fallback flags: ctor=%v fb=%v", prog.HasConstructor, prog.HasFallback)
	}
}

func TestFromTypedClonesConstructorBody(t *testing.T) {
	typed := &sema.TypedModule{
		AST: &ast.Module{
			Version: "0.2",
			Contract: &ast.ContractDecl{
				Name: "Demo",
				Constructor: &ast.ConstructorDecl{
					Params: []ast.FieldDecl{
						{Name: "owner", Type: "address"},
					},
					Body: []ast.Statement{
						{Kind: "set", Name: "x"},
					},
				},
			},
		},
	}

	prog, err := FromTyped(typed)
	if err != nil {
		t.Fatalf("unexpected lower error: %v", err)
	}
	if prog == nil || !prog.HasConstructor {
		t.Fatalf("expected constructor flag")
	}
	if len(prog.ConstructorParams) != 1 || prog.ConstructorParams[0].Name != "owner" {
		t.Fatalf("unexpected constructor params: %#v", prog.ConstructorParams)
	}
	if len(prog.ConstructorBody) != 1 || prog.ConstructorBody[0].Kind != "set" {
		t.Fatalf("unexpected constructor body: %#v", prog.ConstructorBody)
	}
}

func TestFromTypedIncludesEvents(t *testing.T) {
	typed := &sema.TypedModule{
		AST: &ast.Module{
			Version: "0.2",
			Contract: &ast.ContractDecl{
				Name: "Demo",
				Events: []ast.EventDecl{
					{
						Name: "Transfer",
						Params: []ast.FieldDecl{
							{Name: "from", Type: "address", Indexed: true},
							{Name: "to", Type: "address", Indexed: true},
							{Name: "amount", Type: "u256", Indexed: false},
						},
					},
				},
			},
		},
	}

	prog, err := FromTyped(typed)
	if err != nil {
		t.Fatalf("unexpected lower error: %v", err)
	}
	if prog == nil {
		t.Fatalf("expected lowered program")
	}
	if len(prog.Events) != 1 {
		t.Fatalf("unexpected events length: %d", len(prog.Events))
	}
	if prog.Events[0].Name != "Transfer" {
		t.Fatalf("unexpected event name: %s", prog.Events[0].Name)
	}
	if len(prog.Events[0].Params) != 3 {
		t.Fatalf("unexpected event params: %#v", prog.Events[0].Params)
	}
	if !prog.Events[0].Params[0].Indexed || !prog.Events[0].Params[1].Indexed || prog.Events[0].Params[2].Indexed {
		t.Fatalf("unexpected event indexed flags: %#v", prog.Events[0].Params)
	}
}

// TestFromTypedSlotInitializer verifies that inline state var initializers
// generate constructor preamble statements.
func TestFromTypedSlotInitializer(t *testing.T) {
	typed := &sema.TypedModule{
		AST: &ast.Module{
			Version: "0.2",
			Contract: &ast.ContractDecl{
				Name: "Demo",
				Storage: &ast.StorageDecl{
					Slots: []ast.StorageSlot{
						{
							Name: "x",
							Type: "u256",
							InitExpr: &ast.Expr{
								Kind:  "number",
								Value: "1",
							},
						},
					},
				},
			},
		},
	}

	prog, err := FromTyped(typed)
	if err != nil {
		t.Fatalf("unexpected lower error: %v", err)
	}
	// A synthesized constructor should exist.
	if !prog.HasConstructor {
		t.Fatalf("expected HasConstructor=true from slot initializer")
	}
	if len(prog.ConstructorBody) != 1 {
		t.Fatalf("expected 1 constructor body stmt, got %d", len(prog.ConstructorBody))
	}
	stmt := prog.ConstructorBody[0]
	if stmt.Kind != "set" {
		t.Fatalf("expected stmt kind=set, got %q", stmt.Kind)
	}
	if stmt.Target == nil || stmt.Target.Value != "x" {
		t.Fatalf("expected set target 'x', got %#v", stmt.Target)
	}
}
