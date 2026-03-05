package lua

import (
	"strings"
	"testing"
)

func TestMathMaxTableArgument(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`res = math.max({2, 7, 5})`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := L.GetGlobal("res")
	num, ok := got.(LUint256)
	if !ok {
		t.Fatalf("expected number result, got %T", got)
	}
	if lu256Cmp(num, lu256FromInt(7)) != 0 {
		t.Fatalf("unexpected max result: got=%s want=7", num.String())
	}
}

func TestMathMinTableArgument(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`res = math.min({2, 7, 5})`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := L.GetGlobal("res")
	num, ok := got.(LUint256)
	if !ok {
		t.Fatalf("expected number result, got %T", got)
	}
	if lu256Cmp(num, lu256FromInt(2)) != 0 {
		t.Fatalf("unexpected min result: got=%s want=2", num.String())
	}
}

func TestMathMaxTableArgumentRejectsEmpty(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`res = math.max({})`)
	if err == nil {
		t.Fatalf("expected error for empty table")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMathMaxTableArgumentRejectsNonNumber(t *testing.T) {
	L := NewState()
	defer L.Close()

	err := L.DoString(`res = math.max({1, "x"})`)
	if err == nil {
		t.Fatalf("expected error for non-number table element")
	}
	if !strings.Contains(err.Error(), "contain only numbers") {
		t.Fatalf("unexpected error: %v", err)
	}
}
