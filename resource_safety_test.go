package lua

import (
	"fmt"
	"strings"
	"testing"
)

func TestSparseArrayWriteFallsBackToHashWithoutMaterializingHoles(t *testing.T) {
	tbl := newLTable(0, 0)
	key := MaxArrayHoleGrowth + 1024
	tbl.RawSetInt(key, LTrue)

	if got := tbl.RawGetInt(key); got != LTrue {
		t.Fatalf("unexpected sparse value: got=%v want=%v", got, LTrue)
	}
	if got := len(tbl.array); got != 0 {
		t.Fatalf("expected sparse write to stay out of array part, len=%d", got)
	}
	if got := tbl.Len(); got != 0 {
		t.Fatalf("unexpected table length for sparse hash-only write: %d", got)
	}
}

func TestSparseNumericHashKeyNextDoesNotLoop(t *testing.T) {
	tbl := newLTable(0, 0)
	key := lu256FromInt(MaxArrayHoleGrowth + 1024)
	tbl.RawSet(key, LTrue)

	k1, v1 := tbl.Next(LNil)
	if k1 != key || v1 != LTrue {
		t.Fatalf("unexpected first next result: key=%v value=%v", k1, v1)
	}
	k2, v2 := tbl.Next(k1)
	if k2 != LNil || v2 != LNil {
		t.Fatalf("expected iteration to terminate after sparse numeric hash key, got key=%v value=%v", k2, v2)
	}
}

func TestTableLenFailsWhenHostScanExceedsGasLimit(t *testing.T) {
	L := NewState()
	defer L.Close()

	tbl := newLTable(0, 0)
	tbl.array = make([]LValue, 128)
	for i := range tbl.array {
		tbl.array[i] = LNil
	}
	tbl.array[0] = LTrue
	L.SetGlobal("t", tbl)
	L.SetGasLimit(20)

	err := L.DoString(`return #t`)
	if err == nil {
		t.Fatal("expected gas limit error from table length scan")
	}
	if !strings.Contains(err.Error(), "gas limit exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNextFailsWhenHostScanExceedsGasLimit(t *testing.T) {
	L := NewState()
	defer L.Close()

	tbl := newLTable(0, 0)
	tbl.array = make([]LValue, 128)
	for i := range tbl.array {
		tbl.array[i] = LNil
	}
	tbl.array[127] = LTrue
	L.SetGlobal("t", tbl)
	L.SetGasLimit(20)

	err := L.DoString(`return next(t)`)
	if err == nil {
		t.Fatal("expected gas limit error from next() scan")
	}
	if !strings.Contains(err.Error(), "gas limit exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTableRemoveFailsWhenShiftWorkExceedsGasLimit(t *testing.T) {
	L := NewState()
	defer L.Close()

	tbl := L.CreateTable(128, 0)
	for i := 1; i <= 128; i++ {
		tbl.RawSetInt(i, lu256FromInt(i))
	}
	L.SetGlobal("t", tbl)
	L.SetGasLimit(20)

	err := L.DoString(`return table.remove(t, 1)`)
	if err == nil {
		t.Fatal("expected gas limit error from table.remove shift")
	}
	if !strings.Contains(err.Error(), "gas limit exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTableInsertFailsWhenShiftWorkExceedsGasLimit(t *testing.T) {
	L := NewState()
	defer L.Close()

	tbl := L.CreateTable(128, 0)
	for i := 1; i <= 128; i++ {
		tbl.RawSetInt(i, lu256FromInt(i))
	}
	L.SetGlobal("t", tbl)
	L.SetGasLimit(20)

	err := L.DoString(`table.insert(t, 1, 0)`)
	if err == nil {
		t.Fatal("expected gas limit error from table.insert shift")
	}
	if !strings.Contains(err.Error(), "gas limit exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHashBuiltinsFailWhenInputWorkExceedsGasLimit(t *testing.T) {
	tests := []struct {
		name string
		call string
	}{
		{name: "sha256", call: "sha256"},
		{name: "keccak256", call: "keccak256"},
		{name: "ripemd160", call: "ripemd160"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			L.SetGasLimit(20)

			script := fmt.Sprintf(`local s = "0x" .. string.rep("aa", 4096); return %s(s)`, tc.call)
			err := L.DoString(script)
			if err == nil {
				t.Fatal("expected gas limit error from large hash input")
			}
			if !strings.Contains(err.Error(), "gas limit exceeded") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestHashBuiltinsAllowSmallInputsWithinGasBudget(t *testing.T) {
	tests := []struct {
		name string
		call string
	}{
		{name: "sha256", call: "sha256"},
		{name: "keccak256", call: "keccak256"},
		{name: "ripemd160", call: "ripemd160"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			L := NewState()
			defer L.Close()
			L.SetGasLimit(20)

			script := fmt.Sprintf(`return %s("0x1234")`, tc.call)
			if err := L.DoString(script); err != nil {
				t.Fatalf("unexpected error on small input: %v", err)
			}
		})
	}
}

func TestABIDecodeFailsWhenCalldataWorkExceedsGasLimit(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(20)

	err := L.DoString(`local d = "0x" .. string.rep("00", 4096); return __tol_abi_decode_params(d, "u256")`)
	if err == nil {
		t.Fatal("expected gas limit error from abi decode")
	}
	if !strings.Contains(err.Error(), "gas limit exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStringByteFailsWhenReturningTooManyValuesForGasBudget(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(20)

	err := L.DoString(`local s = string.rep("a", 256); return string.byte(s, 1, 256)`)
	if err == nil {
		t.Fatal("expected gas limit error from string.byte")
	}
	if !strings.Contains(err.Error(), "gas limit exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStringByteAllowsSmallResultSetWithinGasBudget(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(20)

	if err := L.DoString(`return string.byte("abc", 1, 3)`); err != nil {
		t.Fatalf("unexpected error on small string.byte call: %v", err)
	}
	if got := L.GetTop(); got != 3 {
		t.Fatalf("unexpected stack result count: got=%d want=3", got)
	}
}

func TestPatternBacktrackingFailsWhenHostWorkExceedsGasLimit(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(20)

	err := L.DoString(`local s = string.rep("a", 24); return string.find(s, "^a*a*a*a*a*a*a*a*a*a*a*a*b$")`)
	if err == nil {
		t.Fatal("expected gas limit error from regex backtracking")
	}
	if !strings.Contains(err.Error(), "gas limit exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPatternSearchAllowsSmallInputsWithinGasBudget(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(20)

	if err := L.DoString(`return string.find("abcdef", "cd")`); err != nil {
		t.Fatalf("unexpected error on small pattern search: %v", err)
	}
}
