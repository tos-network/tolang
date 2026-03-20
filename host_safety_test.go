package lua

import (
	"fmt"
	"strings"
	"testing"
)

func TestTableSortFailsWhenHostWorkExceedsGasLimit(t *testing.T) {
	L := NewState()
	defer L.Close()

	tbl := L.CreateTable(256, 0)
	for i := 256; i >= 1; i-- {
		tbl.RawSetInt(257-i, lu256FromInt(i))
	}
	L.SetGlobal("t", tbl)
	L.SetGasLimit(20)

	err := L.DoString(`table.sort(t)`)
	if err == nil {
		t.Fatal("expected gas limit error from table.sort")
	}
	if !strings.Contains(err.Error(), "gas limit exceeded") {
		t.Fatalf("expected gas limit exceeded, got: %v", err)
	}
	if L.GasUsed() <= 20 {
		t.Fatalf("expected host sort work to consume gas, got %d", L.GasUsed())
	}
}

func TestTableSortSucceedsWithSufficientGas(t *testing.T) {
	L := NewState()
	defer L.Close()

	tbl := L.CreateTable(256, 0)
	for i := 256; i >= 1; i-- {
		tbl.RawSetInt(257-i, lu256FromInt(i))
	}
	L.SetGlobal("t", tbl)
	L.SetGasLimit(1 << 20)

	if err := L.DoString(`table.sort(t)`); err != nil {
		t.Fatalf("unexpected table.sort error: %v", err)
	}
	if got := tbl.RawGetInt(1); got != lu256FromInt(1) {
		t.Fatalf("unexpected first sorted value: %v", got)
	}
	if got := tbl.RawGetInt(256); got != lu256FromInt(256) {
		t.Fatalf("unexpected last sorted value: %v", got)
	}
	if L.GasUsed() == 0 {
		t.Fatal("expected table.sort host work to consume gas")
	}
}

func TestSetLineHookDisabledByDefault(t *testing.T) {
	L := NewState()
	defer L.Close()

	defer func() {
		rcv := recover()
		if rcv == nil {
			t.Fatal("expected SetLineHook to panic when host hooks are disabled")
		}
		if !strings.Contains(fmt.Sprint(rcv), "AllowHostHooks") {
			t.Fatalf("unexpected panic: %v", rcv)
		}
	}()

	L.SetLineHook(func(string, int) {})
}

func TestSetLineHookAllowedWhenOptedIn(t *testing.T) {
	L := NewState(Options{AllowHostHooks: true})
	defer L.Close()

	hits := 0
	L.SetLineHook(func(string, int) { hits++ })
	if err := L.DoString(`local x = 1; x = x + 1`); err != nil {
		t.Fatalf("unexpected DoString error with line hook enabled: %v", err)
	}
	if hits == 0 {
		t.Fatal("expected line hook to be invoked")
	}
}
