package lua

import (
	"strings"
	"testing"
)

func TestLStateIsClosed(t *testing.T) {
	L := NewState()
	L.Close()
	errorIfNotEqual(t, true, L.IsClosed())
}

func TestCallStackOverflowWhenFixed(t *testing.T) {
	L := NewState(Options{
		CallStackSize: 3,
	})
	defer L.Close()

	// expect fixed stack implementation by default (for backwards compatibility)
	stack := L.stack
	if _, ok := stack.(*fixedCallFrameStack); !ok {
		t.Errorf("expected fixed callframe stack by default")
	}

	errorIfScriptNotFail(t, L, `
    local function recurse(count)
      if count > 0 then
        recurse(count - 1)
      end
    end
    local function c()
      recurse(9)
    end
    c()
    `, "stack overflow")
}

func TestCallStackOverflowWhenAutoGrow(t *testing.T) {
	L := NewState(Options{
		CallStackSize:       3,
		MinimizeStackMemory: true,
	})
	defer L.Close()

	// expect auto growing stack implementation when MinimizeStackMemory is set
	stack := L.stack
	if _, ok := stack.(*autoGrowingCallFrameStack); !ok {
		t.Errorf("expected fixed callframe stack by default")
	}

	errorIfScriptNotFail(t, L, `
    local function recurse(count)
      if count > 0 then
        recurse(count - 1)
      end
    end
    local function c()
      recurse(9)
    end
    c()
    `, "stack overflow")
}

func TestSkipOpenLibs(t *testing.T) {
	L := NewState(Options{SkipOpenLibs: true})
	defer L.Close()
	// type() is only available when base lib is loaded
	errorIfScriptNotFail(t, L, `type("")`,
		"attempt to call a non-function object")
	L2 := NewState()
	defer L2.Close()
	errorIfScriptFail(t, L2, `type("")`)
}

func TestGetAndReplace(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LString("a"))
	L.Replace(1, LString("b"))
	L.Replace(0, LString("c"))
	errorIfNotEqual(t, LNil, L.Get(0))
	errorIfNotEqual(t, LNil, L.Get(-10))
	errorIfNotEqual(t, L.Env, L.Get(EnvironIndex))
	errorIfNotEqual(t, LString("b"), L.Get(1))
	L.Push(LString("c"))
	L.Push(LString("d"))
	L.Replace(-2, LString("e"))
	errorIfNotEqual(t, LString("e"), L.Get(-2))
	registry := L.NewTable()
	L.Replace(RegistryIndex, registry)
	L.G.Registry = registry
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Replace(RegistryIndex, LNil)
		return 0
	}, "registry must be a table")
	errorIfGFuncFail(t, L, func(L *LState) int {
		env := L.NewTable()
		L.Replace(EnvironIndex, env)
		errorIfNotEqual(t, env, L.Get(EnvironIndex))
		return 0
	})
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Replace(EnvironIndex, LNil)
		return 0
	}, "environment must be a table")
	errorIfGFuncFail(t, L, func(L *LState) int {
		gbl := L.NewTable()
		L.Replace(GlobalsIndex, gbl)
		errorIfNotEqual(t, gbl, L.G.Global)
		return 0
	})
	errorIfGFuncNotFail(t, L, func(L *LState) int {
		L.Replace(GlobalsIndex, LNil)
		return 0
	}, "_G must be a table")

	L2 := NewState()
	defer L2.Close()
	clo := L2.NewClosure(func(L2 *LState) int {
		L2.Replace(UpvalueIndex(1), lu256FromInt(3))
		errorIfNotEqual(t, lu256FromInt(3), L2.Get(UpvalueIndex(1)))
		return 0
	}, lu256FromInt(1), lu256FromInt(2))
	L2.SetGlobal("clo", clo)
	errorIfScriptFail(t, L2, `clo()`)
}

func TestRemove(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(LString("a"))
	L.Push(LString("b"))
	L.Push(LString("c"))

	L.Remove(4)
	errorIfNotEqual(t, LString("a"), L.Get(1))
	errorIfNotEqual(t, LString("b"), L.Get(2))
	errorIfNotEqual(t, LString("c"), L.Get(3))
	errorIfNotEqual(t, 3, L.GetTop())

	L.Remove(3)
	errorIfNotEqual(t, LString("a"), L.Get(1))
	errorIfNotEqual(t, LString("b"), L.Get(2))
	errorIfNotEqual(t, LNil, L.Get(3))
	errorIfNotEqual(t, 2, L.GetTop())
	L.Push(LString("c"))

	L.Remove(-10)
	errorIfNotEqual(t, LString("a"), L.Get(1))
	errorIfNotEqual(t, LString("b"), L.Get(2))
	errorIfNotEqual(t, LString("c"), L.Get(3))
	errorIfNotEqual(t, 3, L.GetTop())

	L.Remove(2)
	errorIfNotEqual(t, LString("a"), L.Get(1))
	errorIfNotEqual(t, LString("c"), L.Get(2))
	errorIfNotEqual(t, LNil, L.Get(3))
	errorIfNotEqual(t, 2, L.GetTop())
}

func TestToInt(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(lu256FromInt(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	errorIfNotEqual(t, 10, L.ToInt(1))
	errorIfNotEqual(t, 0, L.ToInt(2))
	errorIfNotEqual(t, 0, L.ToInt(3))
}

func TestToInt64(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(lu256FromInt(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	errorIfNotEqual(t, int64(10), L.ToInt64(1))
	errorIfNotEqual(t, int64(0), L.ToInt64(2))
	errorIfNotEqual(t, int64(0), L.ToInt64(3))
}

func TestToNumber(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(lu256FromInt(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	errorIfNotEqual(t, lu256FromInt(10), L.ToUint256(1))
	errorIfNotEqual(t, LUint256Zero, L.ToUint256(2))
	errorIfNotEqual(t, LUint256Zero, L.ToUint256(3))
}

func TestToString(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(lu256FromInt(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	errorIfNotEqual(t, "10", L.ToString(1))
	errorIfNotEqual(t, "99.9", L.ToString(2))
	errorIfNotEqual(t, "", L.ToString(3))
}

func TestToTable(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(lu256FromInt(10))
	L.Push(LString("99.9"))
	L.Push(L.NewTable())
	errorIfFalse(t, L.ToTable(1) == nil, "index 1 must be nil")
	errorIfFalse(t, L.ToTable(2) == nil, "index 2 must be nil")
	errorIfNotEqual(t, L.Get(3), L.ToTable(3))
}

func TestToFunction(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(lu256FromInt(10))
	L.Push(LString("99.9"))
	L.Push(L.NewFunction(func(L *LState) int { return 0 }))
	errorIfFalse(t, L.ToFunction(1) == nil, "index 1 must be nil")
	errorIfFalse(t, L.ToFunction(2) == nil, "index 2 must be nil")
	errorIfNotEqual(t, L.Get(3), L.ToFunction(3))
}

func TestToUserData(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Push(lu256FromInt(10))
	L.Push(LString("99.9"))
	L.Push(L.NewUserData())
	errorIfFalse(t, L.ToUserData(1) == nil, "index 1 must be nil")
	errorIfFalse(t, L.ToUserData(2) == nil, "index 2 must be nil")
	errorIfNotEqual(t, L.Get(3), L.ToUserData(3))
}

func TestObjLen(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfNotEqual(t, 3, L.ObjLen(LString("abc")))
	tbl := L.NewTable()
	tbl.Append(LTrue)
	tbl.Append(LTrue)
	errorIfNotEqual(t, 2, L.ObjLen(tbl))
	mt := L.NewTable()
	L.SetField(mt, "__len", L.NewFunction(func(L *LState) int {
		tbl := L.CheckTable(1)
		L.Push(lu256FromInt(tbl.Len() + 1))
		return 1
	}))
	L.SetMetatable(tbl, mt)
	errorIfNotEqual(t, 3, L.ObjLen(tbl))
	errorIfNotEqual(t, 0, L.ObjLen(lu256FromInt(10)))
}

func TestConcat(t *testing.T) {
	L := NewState()
	defer L.Close()
	errorIfNotEqual(t, "a1c", L.Concat(LString("a"), lu256FromInt(1), LString("c")))
}

func TestPCall(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.Register("f1", func(L *LState) int {
		panic("panic!")
		return 0
	})
	errorIfScriptNotFail(t, L, `f1()`, "panic!")
	L.Push(L.GetGlobal("f1"))
	err := L.PCall(0, 0, L.NewFunction(func(L *LState) int {
		L.Push(LString("by handler"))
		return 1
	}))
	errorIfFalse(t, strings.Contains(err.Error(), "by handler"), "")

	L.Push(L.GetGlobal("f1"))
	err = L.PCall(0, 0, L.NewFunction(func(L *LState) int {
		L.RaiseError("error!")
		return 1
	}))
	errorIfFalse(t, strings.Contains(err.Error(), "error!"), "")

	L.Push(L.GetGlobal("f1"))
	err = L.PCall(0, 0, L.NewFunction(func(L *LState) int {
		panic("panicc!")
		return 1
	}))
	errorIfFalse(t, strings.Contains(err.Error(), "panicc!"), "")

	// Issue #452, expected to be revert back to previous call stack after any error.
	currentFrame, currentTop, currentSp := L.currentFrame, L.GetTop(), L.stack.Sp()
	L.Push(L.GetGlobal("f1"))
	err = L.PCall(0, 0, nil)
	errorIfFalse(t, err != nil, "")
	errorIfFalse(t, L.currentFrame == currentFrame, "")
	errorIfFalse(t, L.GetTop() == currentTop, "")
	errorIfFalse(t, L.stack.Sp() == currentSp, "")

	currentFrame, currentTop, currentSp = L.currentFrame, L.GetTop(), L.stack.Sp()
	L.Push(L.GetGlobal("f1"))
	err = L.PCall(0, 0, L.NewFunction(func(L *LState) int {
		L.RaiseError("error!")
		return 1
	}))
	errorIfFalse(t, err != nil, "")
	errorIfFalse(t, L.currentFrame == currentFrame, "")
	errorIfFalse(t, L.GetTop() == currentTop, "")
	errorIfFalse(t, L.stack.Sp() == currentSp, "")
}

func TestCoroutineApi1(t *testing.T) {
	t.Skip("coroutine library removed")
}

func TestContextWithCroutine(t *testing.T) {
	t.Skip("context API removed; coroutine library removed")
}

func TestPCallAfterFail(t *testing.T) {
	L := NewState()
	defer L.Close()
	errFn := L.NewFunction(func(L *LState) int {
		L.RaiseError("error!")
		return 0
	})
	changeError := L.NewFunction(func(L *LState) int {
		L.Push(errFn)
		err := L.PCall(0, 0, nil)
		if err != nil {
			L.RaiseError("A New Error")
		}
		return 0
	})
	L.Push(changeError)
	err := L.PCall(0, 0, nil)
	errorIfFalse(t, strings.Contains(err.Error(), "A New Error"), "error not propogated correctly")
}

func TestRegistryFixedOverflow(t *testing.T) {
	state := NewState()
	defer state.Close()
	reg := state.reg
	expectedPanic := false
	// should be non auto grow by default
	errorIfFalse(t, reg.maxSize == 0, "state should default to non-auto growing implementation")
	// fill the stack and check we get a panic
	test := LString("test")
	for i := 0; i < len(reg.array); i++ {
		reg.Push(test)
	}
	defer func() {
		rcv := recover()
		if rcv != nil {
			if expectedPanic {
				errorIfFalse(t, rcv.(error).Error() != "registry overflow", "expected registry overflow exception, got "+rcv.(error).Error())
			} else {
				t.Errorf("did not expect registry overflow")
			}
		} else if expectedPanic {
			t.Errorf("expected registry overflow exception, but didn't get panic")
		}
	}()
	expectedPanic = true
	reg.Push(test)
}

func TestRegistryAutoGrow(t *testing.T) {
	state := NewState(Options{RegistryMaxSize: 300, RegistrySize: 200, RegistryGrowStep: 25})
	defer state.Close()
	expectedPanic := false
	defer func() {
		rcv := recover()
		if rcv != nil {
			if expectedPanic {
				errorIfFalse(t, rcv.(error).Error() != "registry overflow", "expected registry overflow exception, got "+rcv.(error).Error())
			} else {
				t.Errorf("did not expect registry overflow")
			}
		} else if expectedPanic {
			t.Errorf("expected registry overflow exception, but didn't get panic")
		}
	}()
	reg := state.reg
	test := LString("test")
	for i := 0; i < 300; i++ {
		reg.Push(test)
	}
	expectedPanic = true
	reg.Push(test)
}

// This test exposed a panic caused by accessing an unassigned var in the lua registry.
// The panic was caused by initCallFrame. It was calling resize() on the registry after it had written some values
// directly to the reg's array, but crucially, before it had updated "top". This meant when the resize occurred, the
// values beyond top where not copied, and were lost, leading to a later uninitialised value being found in the registry.
func TestUninitializedVarAccess(t *testing.T) {
	L := NewState(Options{
		RegistrySize:    128,
		RegistryMaxSize: 256,
	})
	defer L.Close()
	// This test needs to trigger a resize when the local vars are allocated, so we need it to
	// be 128 for the padding amount in the test function to work. If it's larger, we will need
	// more padding to force the error.
	errorIfNotEqual(t, cap(L.reg.array), 128)
	errorIfScriptFail(t, L, `
		local function test(arg1, arg2, arg3)
			-- padding to cause a registry resize when the local vars for this func are reserved
			local a0,b0,c0,d0,e0,f0,g0,h0,i0,j0,k0,l0,m0,n0,o0,p0,q0,r0,s0,t0,u0,v0,w0,x0,y0,z0
			local a1,b1,c1,d1,e1,f1,g1,h1,i1,j1,k1,l1,m1,n1,o1,p1,q1,r1,s1,t1,u1,v1,w1,x1,y1,z1
			local a2,b2,c2,d2,e2,f2,g2,h2,i2,j2,k2,l2,m2,n2,o2,p2,q2,r2,s2,t2,u2,v2,w2,x2,y2,z2
			local a3,b3,c3,d3,e3,f3,g3,h3,i3,j3,k3,l3,m3,n3,o3,p3,q3,r3,s3,t3,u3,v3,w3,x3,y3,z3
			local a4,b4,c4,d4,e4,f4,g4,h4,i4,j4,k4,l4,m4,n4,o4,p4,q4,r4,s4,t4,u4,v4,w4,x4,y4,z4
			if arg3 == nil then
				return 1
			end
			return 0
		end

		test(1,2)
	`)
}

func BenchmarkCallFrameStackPushPopAutoGrow(t *testing.B) {
	stack := newAutoGrowingCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		for i := 0; i < Iterations; i++ {
			stack.Pop()
		}
	}
}

func BenchmarkCallFrameStackPushPopFixed(t *testing.B) {
	stack := newFixedCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		for i := 0; i < Iterations; i++ {
			stack.Pop()
		}
	}
}

// this test will intentionally not incur stack growth in order to bench the performance when no allocations happen
func BenchmarkCallFrameStackPushPopShallowAutoGrow(t *testing.B) {
	stack := newAutoGrowingCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 8
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		for i := 0; i < Iterations; i++ {
			stack.Pop()
		}
	}
}

func BenchmarkCallFrameStackPushPopShallowFixed(t *testing.B) {
	stack := newFixedCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 8
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		for i := 0; i < Iterations; i++ {
			stack.Pop()
		}
	}
}

func BenchmarkCallFrameStackPushPopFixedNoInterface(t *testing.B) {
	stack := newFixedCallFrameStack(256).(*fixedCallFrameStack)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		for i := 0; i < Iterations; i++ {
			stack.Pop()
		}
	}
}

func BenchmarkCallFrameStackUnwindAutoGrow(t *testing.B) {
	stack := newAutoGrowingCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		stack.SetSp(0)
	}
}

func BenchmarkCallFrameStackUnwindFixed(t *testing.B) {
	stack := newFixedCallFrameStack(256)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		stack.SetSp(0)
	}
}

func BenchmarkCallFrameStackUnwindFixedNoInterface(t *testing.B) {
	stack := newFixedCallFrameStack(256).(*fixedCallFrameStack)

	t.ResetTimer()

	const Iterations = 256
	for j := 0; j < t.N; j++ {
		for i := 0; i < Iterations; i++ {
			stack.Push(callFrame{})
		}
		stack.SetSp(0)
	}
}

type registryTestHandler int

func (registryTestHandler) registryOverflow() {
	panic("registry overflow")
}

// test pushing and popping from the registry
func BenchmarkRegistryPushPopAutoGrow(t *testing.B) {
	al := newAllocator(32)
	sz := 256 * 20
	reg := newRegistry(registryTestHandler(0), sz/2, 64, sz, al)
	value := LString("test")

	t.ResetTimer()

	for j := 0; j < t.N; j++ {
		for i := 0; i < sz; i++ {
			reg.Push(value)
		}
		for i := 0; i < sz; i++ {
			reg.Pop()
		}
	}
}

func BenchmarkRegistryPushPopFixed(t *testing.B) {
	al := newAllocator(32)
	sz := 256 * 20
	reg := newRegistry(registryTestHandler(0), sz, 0, sz, al)
	value := LString("test")

	t.ResetTimer()

	for j := 0; j < t.N; j++ {
		for i := 0; i < sz; i++ {
			reg.Push(value)
		}
		for i := 0; i < sz; i++ {
			reg.Pop()
		}
	}
}

func BenchmarkRegistrySetTop(t *testing.B) {
	al := newAllocator(32)
	sz := 256 * 20
	reg := newRegistry(registryTestHandler(0), sz, 32, sz*2, al)

	t.ResetTimer()

	for j := 0; j < t.N; j++ {
		reg.SetTop(sz)
		reg.SetTop(0)
	}
}

func TestGasLimitExceeded(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(100)
	err := L.DoString(`while true do end`)
	if err == nil {
		t.Fatal("expected gas limit error, got nil")
	}
	if !strings.Contains(err.Error(), "gas limit exceeded") {
		t.Fatalf("expected 'gas limit exceeded' in error, got: %v", err)
	}
	if L.GasUsed() != 101 {
		t.Fatalf("expected GasUsed=101, got %d", L.GasUsed())
	}
}

func TestGasNotExceeded(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.SetGasLimit(10000)
	err := L.DoString(`local x = 0; for i = 1, 10 do x = x + i end`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if L.GasUsed() == 0 {
		t.Fatal("expected GasUsed > 0")
	}
}

func TestHexEscape(t *testing.T) {
	L := NewState()
	defer L.Close()

	// Basic \xNN round-trips.
	cases := []struct {
		expr string // a Lua string expression (not a statement)
		want string
	}{
		{`"\x00"`, "\x00"},                         // null byte
		{`"\xff"`, "\xff"},                         // 0xff
		{`"\xde\xad\xbe\xef"`, "\xde\xad\xbe\xef"}, // multi-byte
		{`"\x41\x42\x43"`, "ABC"},                  // ASCII via hex
		{`"\x0a"`, "\n"},                           // \x0a == newline
		{`"a\x20b"`, "a b"},                        // hex in middle of string
		{`"\xDE\xAD"`, "\xde\xad"},                 // uppercase hex digits
	}

	for _, tc := range cases {
		err := L.DoString(`_result = ` + tc.expr)
		if err != nil {
			t.Errorf("expr %q: unexpected error: %v", tc.expr, err)
			continue
		}
		got, ok := L.GetGlobal("_result").(LString)
		if !ok {
			t.Errorf("expr %q: result is not LString", tc.expr)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("expr %q: want %q, got %q", tc.expr, tc.want, string(got))
		}
	}

	// \x with non-hex digit must be a lexer error.
	if err := L.DoString(`local s = "\xGG"`); err == nil {
		t.Error(`"\xGG" should produce a lexer error but did not`)
	}
	// \x with only one hex digit must also error.
	if err := L.DoString(`local s = "\x4"`); err == nil {
		t.Error(`"\x4" (one digit) should produce a lexer error but did not`)
	}
}

func TestUnicodeEscape(t *testing.T) {
	L := NewState()
	defer L.Close()

	cases := []struct {
		expr string
		want string
	}{
		{`"\u{41}"`, "A"},
		{`"\u{03A9}"`, "Ω"},
		{`"\u{1F600}"`, "😀"},
	}

	for _, tc := range cases {
		if err := L.DoString(`_result = ` + tc.expr); err != nil {
			t.Fatalf("expr %q: unexpected error: %v", tc.expr, err)
		}
		got, ok := L.GetGlobal("_result").(LString)
		if !ok {
			t.Fatalf("expr %q: result is not LString", tc.expr)
		}
		if string(got) != tc.want {
			t.Fatalf("expr %q: want %q, got %q", tc.expr, tc.want, string(got))
		}
	}

	invalid := []string{
		`local s = "\u{}"`,
		`local s = "\u{110000}"`,
		`local s = "\u{D800}"`,
	}
	for _, src := range invalid {
		if err := L.DoString(src); err == nil {
			t.Fatalf("expected Unicode escape error for %q", src)
		}
	}
}

func TestZEscape(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString("_result = \"a\\z \t  b\""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := L.GetGlobal("_result").(LString)
	if !ok {
		t.Fatal("result is not LString")
	}
	if string(got) != "ab" {
		t.Fatalf("want %q, got %q", "ab", string(got))
	}

	src := "_result = \"a\\z\n  b\""
	if err := L.DoString(src); err != nil {
		t.Fatalf("unexpected multiline \\z error: %v", err)
	}
	got, ok = L.GetGlobal("_result").(LString)
	if !ok {
		t.Fatal("result is not LString")
	}
	if string(got) != "ab" {
		t.Fatalf("want %q, got %q", "ab", string(got))
	}
}

func TestLua54BitwiseAndIDiv(t *testing.T) {
	L := NewState()
	defer L.Close()

	src := `
		assert(7 // 2 == 3)
		assert((5 & 3) == 1)
		assert((5 | 2) == 7)
		assert((5 ~ 1) == 4)
		assert((1 << 8) == 256)
		assert((256 >> 8) == 1)
		assert((1 << 256) == 0)
		assert((~(~123)) == 123)
	`
	if err := L.DoString(src); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLocalAttributes(t *testing.T) {
	L := NewState()
	defer L.Close()

	// <const>: assignments after declaration must fail at compile time.
	if err := L.DoString(`
		local x<const> = 1
		x = 2
	`); err == nil {
		t.Fatal("expected const local assignment compile error")
	}

	// <close>: invoke __close exactly once when scope exits.
	if err := L.DoString(`
		closed = 0
		do
			local h<close> = setmetatable({}, {
				__close = function(self, err)
					assert(err == nil)
					closed = closed + 1
				end
			})
		end
		assert(closed == 1)
	`); err != nil {
		t.Fatalf("unexpected <close> error: %v", err)
	}

	// LIFO order for multiple to-be-closed locals in same scope.
	if err := L.DoString(`
		seq = ""
		do
			local a<close> = setmetatable({}, { __close = function() seq = seq .. "a" end })
			local b<close> = setmetatable({}, { __close = function() seq = seq .. "b" end })
		end
		assert(seq == "ba")
	`); err != nil {
		t.Fatalf("unexpected LIFO close error: %v", err)
	}

	// break should also trigger __close.
	if err := L.DoString(`
		closed = 0
		while true do
			local h <close> = setmetatable({}, { __close = function() closed = closed + 1 end })
			break
		end
		assert(closed == 1)
	`); err != nil {
		t.Fatalf("unexpected break close error: %v", err)
	}

	if err := L.DoString(`local x <foo> = 1`); err == nil {
		t.Fatal("expected unknown local attribute error")
	}
}

func TestNumberLiteralPolicyStillUint256(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`local x = 0xff; assert(x == 255)`); err != nil {
		t.Fatalf("hex integer should be accepted: %v", err)
	}
	if err := L.DoString(`local x = 1.5`); err == nil {
		t.Fatal("float literal should be rejected")
	}
	if err := L.DoString(`local x = 1e5; assert(x == 100000)`); err != nil {
		t.Fatalf("scientific notation literal should be accepted: %v", err)
	}
}
