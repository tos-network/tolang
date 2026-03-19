package format

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tos-network/tolang/tol/parser"
)

func TestFormatTRC20RoundTrip(t *testing.T) {
	// Find examples relative to module root.
	src, err := os.ReadFile(filepath.Join("..", "..", "examples", "trc20_tol", "TRC20.tol"))
	if err != nil {
		t.Skipf("skipping: cannot read TRC20.tol: %v", err)
	}

	formatted, err := Format(src, "TRC20.tol")
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}
	if len(formatted) == 0 {
		t.Fatal("formatted output is empty")
	}

	// Verify the formatted output parses without errors.
	_, diags := parser.ParseFile("TRC20.tol", formatted)
	if diags.HasErrors() {
		t.Fatalf("formatted output has parse errors: %v", diags.Error())
	}
}

func TestFormatIdempotent(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "examples", "trc20_tol", "TRC20.tol"))
	if err != nil {
		t.Skipf("skipping: cannot read TRC20.tol: %v", err)
	}

	pass1, err := Format(src, "TRC20.tol")
	if err != nil {
		t.Fatalf("Format pass 1 failed: %v", err)
	}

	pass2, err := Format(pass1, "TRC20.tol")
	if err != nil {
		t.Fatalf("Format pass 2 failed: %v", err)
	}

	if string(pass1) != string(pass2) {
		t.Fatal("formatter is not idempotent")
	}
}

func TestFormatParseError(t *testing.T) {
	_, err := Format([]byte("this is not valid tol"), "bad.tol")
	if err == nil {
		t.Fatal("expected error for invalid source")
	}
}

func TestFormatBasicContract(t *testing.T) {
	src := []byte(`pragma tolang 0.2.0;
contract Foo {
  u256 x;
  function bar() public view returns (u256 r) {
    return x;
  }
}`)

	formatted, err := Format(src, "test.tol")
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	// Verify it re-parses.
	_, diags := parser.ParseFile("test.tol", formatted)
	if diags.HasErrors() {
		t.Fatalf("formatted output has parse errors: %v\n%s", diags.Error(), formatted)
	}
}

func TestFormatAllExamples(t *testing.T) {
	examplesDir := filepath.Join("..", "..", "examples")
	matches, err := filepath.Glob(filepath.Join(examplesDir, "*", "*.tol"))
	if err != nil {
		t.Skipf("skipping: %v", err)
	}
	for _, path := range matches {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("cannot read %s: %v", path, err)
			}
			formatted, err := Format(src, name)
			if err != nil {
				t.Fatalf("Format failed: %v", err)
			}
			// Check idempotency.
			pass2, err := Format(formatted, name)
			if err != nil {
				t.Fatalf("Format pass 2 failed: %v", err)
			}
			if string(formatted) != string(pass2) {
				t.Fatalf("not idempotent for %s", name)
			}
			// Check re-parsability.
			_, diags := parser.ParseFile(name, formatted)
			if diags.HasErrors() {
				t.Fatalf("formatted output has parse errors: %v", diags.Error())
			}
		})
	}
}
