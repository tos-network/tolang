package lua

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tos-network/tolang/parse"
)

func testScriptDir(t *testing.T, tests []string, directory string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatalf("chdir %s: %v", directory, err)
	}
	defer os.Chdir(wd)
	for _, script := range tests {
		script := script
		t.Run(script, func(t *testing.T) {
			fmt.Printf("testing %s/%s\n", directory, script)
			src, err := os.ReadFile(script)
			if err != nil {
				t.Fatal(err)
			}
			L := NewState(Options{
				RegistrySize:        1024 * 20,
				CallStackSize:       1024,
				IncludeGoStackTrace: true,
			})
			// Provide a no-op print for test scripts that use it as a banner.
			L.SetGlobal("print", L.NewFunction(func(L *LState) int { return 0 }))
			defer L.Close()
			if err := L.DoString(string(src)); err != nil {
				t.Error(err)
			}
		})
	}
}

// lua54Tests are the Lua 5.4 standard test suite files that are compatible
// with this VM (no floats, no io/os/coroutine/debug/require/collectgarbage).
var lua54Tests = []string{
	"all.lua",
	"api.lua",
	"attrib.lua",
	"big.lua",
	"bitwise.lua",
	"bwcoercion.lua",
	"calls.lua",
	"closure.lua",
	"code.lua",
	"constructs.lua",
	"errors.lua",
	"events.lua",
	"literals.lua",
	"locals.lua",
	"math.lua",
	"nextvar.lua",
	// pm.lua skipped: tests Lua 5.3.3+ empty-match gsub semantics and utf8 lib
	"sort.lua",
	"strings.lua",
	"tpack.lua",
	"utf8.lua",
	"vararg.lua",
	"verybig.lua",
}

func TestLua(t *testing.T) {
	testScriptDir(t, lua54Tests, "_lua-5.4.8-tests")
}

var numActiveUserDatas int32 = 0

type finalizerStub struct{ x byte }

func allocFinalizerUserData(L *LState) int {
	ud := L.NewUserData()
	atomic.AddInt32(&numActiveUserDatas, 1)
	a := finalizerStub{}
	ud.Value = &a
	runtime.SetFinalizer(&a, func(aa *finalizerStub) {
		atomic.AddInt32(&numActiveUserDatas, -1)
	})
	L.Push(ud)
	return 1
}

func sleep(L *LState) int {
	time.Sleep(time.Duration(L.CheckInt(1)) * time.Millisecond)
	return 0
}

func countFinalizers(L *LState) int {
	L.Push(lu256FromInt(int(numActiveUserDatas)))
	return 1
}

// TestLocalVarFree verifies that tables and user user datas which are no longer referenced by the lua script are
// correctly gc-ed. There was a bug in the upstream Lua VM where local vars were not being gc-ed in all circumstances.
func TestLocalVarFree(t *testing.T) {
	t.Skip("collectgarbage removed for deterministic VM")
	s := `
		function Test(a, b, c)
			local a = { v = allocFinalizer() }
			local b = { v = allocFinalizer() }
			return a
		end
		Test(1,2,3)
		for i = 1, 100 do
			collectgarbage()
			if countFinalizers() == 0 then
				return
			end
			sleep(100)
		end
		error("user datas not finalized after 100 gcs")
`
	L := NewState()
	L.SetGlobal("allocFinalizer", L.NewFunction(allocFinalizerUserData))
	L.SetGlobal("sleep", L.NewFunction(sleep))
	L.SetGlobal("countFinalizers", L.NewFunction(countFinalizers))
	defer L.Close()
	if err := L.DoString(s); err != nil {
		t.Error(err)
	}
}

func TestMergingLoadNilBug(t *testing.T) {
	// there was a bug where a multiple load nils were being incorrectly merged, and the following code exposed it
	s := `
    function test()
        local a = 0
        local b = 1
        local c = 2
        local d = 3
        local e = 4		-- reg 4
        local f = 5
        local g = 6
        local h = 7

        if e == 4 then
            e = nil		-- should clear reg 4, but clears regs 4-8 by mistake
        end
        if f == nil then
            error("bad f")
        end
        if g == nil then
            error("bad g")
        end
        if h == nil then
            error("bad h")
        end
    end

    test()
`

	L := NewState()
	defer L.Close()
	if err := L.DoString(s); err != nil {
		t.Error(err)
	}
}

func TestMergingLoadNil(t *testing.T) {
	// multiple nil assignments to consecutive registers should be merged
	s := `
		function test()
			local a = 0
			local b = 1
			local c = 2

			-- this should generate just one LOADNIL byte code instruction
			a = nil
			b = nil
			c = nil

			print(a,b,c)
		end

		test()`

	chunk, err := parse.Parse(strings.NewReader(s), "test")
	if err != nil {
		t.Fatal(err)
	}

	compiled, err := Compile(chunk, "test")
	if err != nil {
		t.Fatal(err)
	}

	if len(compiled.FunctionPrototypes) != 1 {
		t.Fatal("expected 1 function prototype")
	}

	// there should be exactly 1 LOADNIL instruction in the byte code generated for the above
	// anymore, and the LOADNIL merging is not working correctly
	count := 0
	for _, instr := range compiled.FunctionPrototypes[0].Code {
		if opGetOpCode(instr) == OP_LOADNIL {
			count++
		}
	}

	if count != 1 {
		t.Fatalf("expected 1 LOADNIL instruction, found %d", count)
	}
}
