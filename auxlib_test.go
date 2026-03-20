package lua

import (
	"testing"
)

func TestToStringMetaNameFallbackStable(t *testing.T) {
	L := NewState()
	defer L.Close()

	mt := L.NewTable()
	L.SetField(mt, "__name", LString("Foo"))

	tbl := L.NewTable()
	L.SetMetatable(tbl, mt)
	if got := L.ToStringMeta(tbl); got != LString("Foo") {
		t.Fatalf("unexpected table tostring meta fallback: got=%v want=Foo", got)
	}
	if got := L.ToStringMeta(tbl); got != LString("Foo") {
		t.Fatalf("expected repeated table tostring meta fallback to stay stable, got=%v want=Foo", got)
	}

	ud := L.NewUserData()
	L.SetMetatable(ud, mt)
	if got := L.ToStringMeta(ud); got != LString("Foo") {
		t.Fatalf("unexpected userdata tostring meta fallback: got=%v want=Foo", got)
	}
	if got := L.ToStringMeta(ud); got != LString("Foo") {
		t.Fatalf("expected repeated userdata tostring meta fallback to stay stable, got=%v want=Foo", got)
	}
}

func TestCheckInt(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Push(lu256FromInt(10))
		errorIfNotEqual(t, 10, L.CheckInt(2))
		L.Push(LString("aaa"))
		L.CheckInt(3)
		return 0
	}, "uint256 expected, got string")
}

func TestCheckInt64(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Push(lu256FromInt(10))
		errorIfNotEqual(t, int64(10), L.CheckInt64(2))
		L.Push(LString("aaa"))
		L.CheckInt64(3)
		return 0
	}, "uint256 expected, got string")
}

func TestCheckUint256(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Push(lu256FromInt(10))
		errorIfNotEqual(t, lu256FromInt(10), L.CheckUint256(2))
		L.Push(LString("11"))
		errorIfNotEqual(t, lu256FromInt(11), L.CheckUint256(3))
		L.Push(LString("aaa"))
		L.CheckUint256(4)
		return 0
	}, "uint256 expected, got string")
}

func TestCheckString(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Push(LString("aaa"))
		errorIfNotEqual(t, "aaa", L.CheckString(2))
		L.Push(lu256FromInt(10))
		errorIfNotEqual(t, "10", L.CheckString(3))
		L.Push(L.NewTable())
		L.CheckString(4)
		return 0
	}, "string expected, got table")
}

func TestCheckBool(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Push(LTrue)
		errorIfNotEqual(t, true, L.CheckBool(2))
		L.Push(lu256FromInt(10))
		L.CheckBool(3)
		return 0
	}, "boolean expected, got uint256")
}

func TestCheckTable(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		tbl := L.NewTable()
		L.Push(tbl)
		errorIfNotEqual(t, tbl, L.CheckTable(2))
		L.Push(lu256FromInt(10))
		L.CheckTable(3)
		return 0
	}, "table expected, got uint256")
}

func TestCheckFunction(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		fn := L.NewFunction(func(l *LState) int { return 0 })
		L.Push(fn)
		errorIfNotEqual(t, fn, L.CheckFunction(2))
		L.Push(lu256FromInt(10))
		L.CheckFunction(3)
		return 0
	}, "function expected, got uint256")
}

func TestCheckUserData(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		ud := L.NewUserData()
		L.Push(ud)
		errorIfNotEqual(t, ud, L.CheckUserData(2))
		L.Push(lu256FromInt(10))
		L.CheckUserData(3)
		return 0
	}, "userdata expected, got uint256")
}

func TestCheckType(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Push(lu256FromInt(10))
		L.CheckType(2, LTUint256)
		L.CheckType(2, LTString)
		return 0
	}, "string expected, got uint256")
}

func TestCheckTypes(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Push(lu256FromInt(10))
		L.CheckTypes(2, LTString, LTBool, LTUint256)
		L.CheckTypes(2, LTString, LTBool)
		return 0
	}, "string or boolean expected, got uint256")
}

func TestCheckOption(t *testing.T) {
	opts := []string{
		"opt1",
		"opt2",
		"opt3",
	}
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Push(LString("opt1"))
		errorIfNotEqual(t, 0, L.CheckOption(2, opts))
		L.Push(LString("opt5"))
		L.CheckOption(3, opts)
		return 0
	}, "invalid option: opt5 \\(must be one of opt1,opt2,opt3\\)")
}

func TestOptInt(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		errorIfNotEqual(t, 99, L.OptInt(1, 99))
		L.Push(lu256FromInt(10))
		errorIfNotEqual(t, 10, L.OptInt(2, 99))
		L.Push(LString("aaa"))
		L.OptInt(3, 99)
		return 0
	}, "uint256 expected, got string")
}

func TestOptInt64(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		errorIfNotEqual(t, int64(99), L.OptInt64(1, int64(99)))
		L.Push(lu256FromInt(10))
		errorIfNotEqual(t, int64(10), L.OptInt64(2, int64(99)))
		L.Push(LString("aaa"))
		L.OptInt64(3, int64(99))
		return 0
	}, "uint256 expected, got string")
}

func TestOptUint256(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		errorIfNotEqual(t, lu256FromInt(99), L.OptUint256(1, lu256FromInt(99)))
		L.Push(lu256FromInt(10))
		errorIfNotEqual(t, lu256FromInt(10), L.OptUint256(2, lu256FromInt(99)))
		L.Push(LString("aaa"))
		L.OptUint256(3, lu256FromInt(99))
		return 0
	}, "uint256 expected, got string")
}

func TestOptString(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		errorIfNotEqual(t, "bbb", L.OptString(1, "bbb"))
		L.Push(LString("aaa"))
		errorIfNotEqual(t, "aaa", L.OptString(2, "bbb"))
		L.Push(lu256FromInt(10))
		L.OptString(3, "bbb")
		return 0
	}, "string expected, got uint256")
}

func TestOptBool(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		errorIfNotEqual(t, true, L.OptBool(1, true))
		L.Push(LTrue)
		errorIfNotEqual(t, true, L.OptBool(2, false))
		L.Push(lu256FromInt(10))
		L.OptBool(3, false)
		return 0
	}, "boolean expected, got uint256")
}

func TestOptTable(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		deftbl := L.NewTable()
		errorIfNotEqual(t, deftbl, L.OptTable(1, deftbl))
		tbl := L.NewTable()
		L.Push(tbl)
		errorIfNotEqual(t, tbl, L.OptTable(2, deftbl))
		L.Push(lu256FromInt(10))
		L.OptTable(3, deftbl)
		return 0
	}, "table expected, got uint256")
}

func TestOptFunction(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		deffn := L.NewFunction(func(l *LState) int { return 0 })
		errorIfNotEqual(t, deffn, L.OptFunction(1, deffn))
		fn := L.NewFunction(func(l *LState) int { return 0 })
		L.Push(fn)
		errorIfNotEqual(t, fn, L.OptFunction(2, deffn))
		L.Push(lu256FromInt(10))
		L.OptFunction(3, deffn)
		return 0
	}, "function expected, got uint256")
}

func TestOptUserData(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		defud := L.NewUserData()
		errorIfNotEqual(t, defud, L.OptUserData(1, defud))
		ud := L.NewUserData()
		L.Push(ud)
		errorIfNotEqual(t, ud, L.OptUserData(2, defud))
		L.Push(lu256FromInt(10))
		L.OptUserData(3, defud)
		return 0
	}, "userdata expected, got uint256")
}
