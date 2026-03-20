package lua

import (
	"fmt"
	"strings"
	"testing"
)

func TestStringFormatRejectsOversizeOutput(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(1000)

	script := fmt.Sprintf(`return string.format("%%%ds", "x")`, maxStringResultBytes+1)
	errorIfScriptNotFail(t, L, script, `string\.format: output too large`)
}

func TestConcatRejectsOversizeOutput(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(1000)

	errorIfScriptNotFail(t, L, `
local a = string.rep("a", 600000)
local b = string.rep("b", 500000)
return a .. b
`, `concat: output too large`)
}

func TestTableConcatRejectsOversizeOutput(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(1000)

	errorIfScriptNotFail(t, L, `
local a = string.rep("a", 600000)
local b = string.rep("b", 500000)
return table.concat({a, b})
`, `concat: output too large`)
}

func TestTolStringConcatRejectsOversizeOutput(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(1000)

	errorIfScriptNotFail(t, L, `
local a = string.rep("a", 600000)
local b = string.rep("b", 500000)
return __tol_str_concat(a, b)
`, `__tol_str_concat: output too large`)
}

func TestTolBytesConcatRejectsOversizeOutput(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(1000)

	errorIfScriptNotFail(t, L, `
local a = "0x" .. string.rep("a", 600000)
local b = "0x" .. string.rep("b", 500000)
return __tol_bytes_concat(a, b)
`, `__tol_bytes_concat: output too large`)
}

func TestStringFormatQuotedStringRejectsOversizeOutput(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(1000)

	script := fmt.Sprintf(`return string.format("%%q", string.rep("\\\\", %d))`, maxStringResultBytes/2)
	errorIfScriptNotFail(t, L, script, `string\.format: output too large`)
}

func TestStringConcatAllowsBoundarySizedOutput(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(1000)

	partA := maxStringResultBytes / 2
	partB := maxStringResultBytes - partA
	script := fmt.Sprintf(`
local a = string.rep("a", %d)
local b = string.rep("b", %d)
return string.len(a .. b)
`, partA, partB)
	if err := L.DoString(script); err != nil {
		t.Fatalf("unexpected error at size limit: %v", err)
	}
	got := L.Get(-1).String()
	want := fmt.Sprintf("%d", maxStringResultBytes)
	if got != want {
		t.Fatalf("unexpected boundary concat len: got=%q want=%q", got, want)
	}
}

func TestStringFormatAllowsBoundaryWidth(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(1000)

	script := fmt.Sprintf(`return string.len(string.format("%%%ds", "x"))`, maxStringResultBytes)
	if err := L.DoString(script); err != nil {
		t.Fatalf("unexpected format error at size limit: %v", err)
	}
	if got := L.Get(-1).String(); got != fmt.Sprintf("%d", maxStringResultBytes) {
		t.Fatalf("unexpected boundary format len: got=%q", got)
	}
}

func TestStringResultLimitErrorMessageIncludesLimit(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(1000)

	err := L.DoString(`return __tol_str_concat(string.rep("a", 600000), string.rep("b", 500000))`)
	if err == nil {
		t.Fatal("expected oversize concat error")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%d", maxStringResultBytes)) {
		t.Fatalf("expected limit in error, got: %v", err)
	}
}
