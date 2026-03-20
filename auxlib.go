package lua

import (
	"fmt"
	"sort"
	"strings"
)

/* checkType {{{ */

func (ls *LState) CheckAny(n int) LValue {
	if n > ls.GetTop() {
		ls.ArgError(n, "value expected")
	}
	return ls.Get(n)
}

func (ls *LState) CheckInt(n int) int {
	v := ls.Get(n)
	if intv, ok := v.(LUint256); ok {
		if out, ok := lu256ToInt(intv); ok {
			return out
		}
		ls.ArgError(n, "number out of range")
	}
	ls.TypeError(n, LTUint256)
	return 0
}

func (ls *LState) CheckInt64(n int) int64 {
	v := ls.Get(n)
	if intv, ok := v.(LUint256); ok {
		if out, ok := lu256ToInt64(intv); ok {
			return out
		}
		ls.ArgError(n, "number out of range")
	}
	ls.TypeError(n, LTUint256)
	return 0
}

func (ls *LState) CheckUint256(n int) LUint256 {
	v := ls.Get(n)
	if lv, ok := v.(LUint256); ok {
		return lv
	}
	if lv, ok := v.(LString); ok {
		if num, err := parseNumber(string(lv)); err == nil {
			return num
		}
	}
	ls.TypeError(n, LTUint256)
	return LUint256Zero
}

func (ls *LState) CheckString(n int) string {
	v := ls.Get(n)
	if lv, ok := v.(LString); ok {
		return string(lv)
	} else if LVCanConvToString(v) {
		return ls.ToString(n)
	}
	ls.TypeError(n, LTString)
	return ""
}

func (ls *LState) CheckAgent(n int) LAgent {
	v := ls.Get(n)
	switch lv := v.(type) {
	case LAgent:
		addr, err := parseAgentString(string(lv))
		if err != nil {
			ls.ArgError(n, err.Error())
		}
		return addr
	case LString:
		addr, err := parseAgentString(string(lv))
		if err != nil {
			ls.ArgError(n, err.Error())
		}
		return addr
	default:
		ls.TypeError(n, LTAgent)
	}
	return LAgent("")
}

func (ls *LState) CheckBool(n int) bool {
	v := ls.Get(n)
	if lv, ok := v.(LBool); ok {
		return bool(lv)
	}
	ls.TypeError(n, LTBool)
	return false
}

func (ls *LState) CheckTable(n int) *LTable {
	v := ls.Get(n)
	if lv, ok := v.(*LTable); ok {
		return lv
	}
	ls.TypeError(n, LTTable)
	return nil
}

func (ls *LState) CheckFunction(n int) *LFunction {
	v := ls.Get(n)
	if lv, ok := v.(*LFunction); ok {
		return lv
	}
	ls.TypeError(n, LTFunction)
	return nil
}

func (ls *LState) CheckUserData(n int) *LUserData {
	v := ls.Get(n)
	if lv, ok := v.(*LUserData); ok {
		return lv
	}
	ls.TypeError(n, LTUserData)
	return nil
}

func (ls *LState) CheckType(n int, typ LValueType) {
	v := ls.Get(n)
	if v.Type() != typ {
		ls.TypeError(n, typ)
	}
}

func (ls *LState) CheckTypes(n int, typs ...LValueType) {
	vt := ls.Get(n).Type()
	for _, typ := range typs {
		if vt == typ {
			return
		}
	}
	buf := []string{}
	for _, typ := range typs {
		buf = append(buf, typ.String())
	}
	ls.ArgError(n, strings.Join(buf, " or ")+" expected, got "+ls.Get(n).Type().String())
}

func (ls *LState) CheckOption(n int, options []string) int {
	str := ls.CheckString(n)
	for i, v := range options {
		if v == str {
			return i
		}
	}
	ls.ArgError(n, fmt.Sprintf("invalid option: %s (must be one of %s)", str, strings.Join(options, ",")))
	return 0
}

/* }}} */

/* optType {{{ */

func (ls *LState) OptInt(n int, d int) int {
	v := ls.Get(n)
	if v == LNil {
		return d
	}
	if intv, ok := v.(LUint256); ok {
		if out, ok := lu256ToInt(intv); ok {
			return out
		}
		ls.ArgError(n, "number out of range")
	}
	ls.TypeError(n, LTUint256)
	return 0
}

func (ls *LState) OptInt64(n int, d int64) int64 {
	v := ls.Get(n)
	if v == LNil {
		return d
	}
	if intv, ok := v.(LUint256); ok {
		if out, ok := lu256ToInt64(intv); ok {
			return out
		}
		ls.ArgError(n, "number out of range")
	}
	ls.TypeError(n, LTUint256)
	return 0
}

func (ls *LState) OptUint256(n int, d LUint256) LUint256 {
	v := ls.Get(n)
	if v == LNil {
		return d
	}
	if lv, ok := v.(LUint256); ok {
		return lv
	}
	ls.TypeError(n, LTUint256)
	return LUint256Zero
}

func (ls *LState) OptString(n int, d string) string {
	v := ls.Get(n)
	if v == LNil {
		return d
	}
	if lv, ok := v.(LString); ok {
		return string(lv)
	}
	ls.TypeError(n, LTString)
	return ""
}

func (ls *LState) OptAgent(n int, d LAgent) LAgent {
	v := ls.Get(n)
	if v == LNil {
		return d
	}
	switch lv := v.(type) {
	case LAgent:
		addr, err := parseAgentString(string(lv))
		if err != nil {
			ls.ArgError(n, err.Error())
		}
		return addr
	case LString:
		addr, err := parseAgentString(string(lv))
		if err != nil {
			ls.ArgError(n, err.Error())
		}
		return addr
	default:
		ls.TypeError(n, LTAgent)
	}
	return LAgent("")
}

func (ls *LState) OptBool(n int, d bool) bool {
	v := ls.Get(n)
	if v == LNil {
		return d
	}
	if lv, ok := v.(LBool); ok {
		return bool(lv)
	}
	ls.TypeError(n, LTBool)
	return false
}

func (ls *LState) OptTable(n int, d *LTable) *LTable {
	v := ls.Get(n)
	if v == LNil {
		return d
	}
	if lv, ok := v.(*LTable); ok {
		return lv
	}
	ls.TypeError(n, LTTable)
	return nil
}

func (ls *LState) OptFunction(n int, d *LFunction) *LFunction {
	v := ls.Get(n)
	if v == LNil {
		return d
	}
	if lv, ok := v.(*LFunction); ok {
		return lv
	}
	ls.TypeError(n, LTFunction)
	return nil
}

func (ls *LState) OptUserData(n int, d *LUserData) *LUserData {
	v := ls.Get(n)
	if v == LNil {
		return d
	}
	if lv, ok := v.(*LUserData); ok {
		return lv
	}
	ls.TypeError(n, LTUserData)
	return nil
}

/* }}} */

/* error operations {{{ */

func (ls *LState) ArgError(n int, message string) {
	ls.RaiseError("bad argument #%v to %v (%v)", n, ls.rawFrameFuncName(ls.currentFrame), message)
}

func (ls *LState) TypeError(n int, typ LValueType) {
	ls.RaiseError("bad argument #%v to %v (%v expected, got %v)", n, ls.rawFrameFuncName(ls.currentFrame), typ.String(), ls.Get(n).Type().String())
}

/* }}} */

/* debug operations {{{ */

func (ls *LState) Where(level int) string {
	return ls.where(level, false)
}

/* }}} */

/* table operations {{{ */

func (ls *LState) FindTable(obj *LTable, n string, size int) LValue {
	names := strings.Split(n, ".")
	curobj := obj
	for _, name := range names {
		if curobj.Type() != LTTable {
			return LNil
		}
		nextobj := ls.RawGet(curobj, LString(name))
		if nextobj == LNil {
			tb := ls.CreateTable(0, size)
			ls.RawSet(curobj, LString(name), tb)
			curobj = tb
		} else if nextobj.Type() != LTTable {
			return LNil
		} else {
			curobj = nextobj.(*LTable)
		}
	}
	return curobj
}

/* }}} */

/* register operations {{{ */

func (ls *LState) RegisterModule(name string, funcs map[string]LGFunction) LValue {
	tb := ls.FindTable(ls.Get(RegistryIndex).(*LTable), "_LOADED", 1)
	mod := ls.GetField(tb, name)
	if mod.Type() != LTTable {
		newmod := ls.FindTable(ls.Get(GlobalsIndex).(*LTable), name, len(funcs))
		if newmodtb, ok := newmod.(*LTable); !ok {
			ls.RaiseError("name conflict for module(%v)", name)
		} else {
			for _, fname := range sortedLGFunctionKeys(funcs) {
				fn := funcs[fname]
				newmodtb.RawSetString(fname, ls.NewFunction(fn))
			}
			ls.SetField(tb, name, newmodtb)
			return newmodtb
		}
	}
	return mod
}

func (ls *LState) SetFuncs(tb *LTable, funcs map[string]LGFunction, upvalues ...LValue) *LTable {
	for _, fname := range sortedLGFunctionKeys(funcs) {
		fn := funcs[fname]
		tb.RawSetString(fname, ls.NewClosure(fn, upvalues...))
	}
	return tb
}

func sortedLGFunctionKeys(funcs map[string]LGFunction) []string {
	keys := make([]string, 0, len(funcs))
	for name := range funcs {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return keys
}

/* }}} */

/* metatable operations {{{ */

func (ls *LState) NewTypeMetatable(typ string) *LTable {
	regtable := ls.Get(RegistryIndex)
	mt := ls.GetField(regtable, typ)
	if tb, ok := mt.(*LTable); ok {
		return tb
	}
	mtnew := ls.NewTable()
	ls.SetField(regtable, typ, mtnew)
	return mtnew
}

func (ls *LState) GetMetaField(obj LValue, event string) LValue {
	return ls.metaOp1(obj, event)
}

func (ls *LState) GetTypeMetatable(typ string) LValue {
	return ls.GetField(ls.Get(RegistryIndex), typ)
}

func (ls *LState) CallMeta(obj LValue, event string) LValue {
	op := ls.metaOp1(obj, event)
	if op.Type() == LTFunction {
		ls.reg.Push(op)
		ls.reg.Push(obj)
		ls.Call(1, 1)
		return ls.reg.Pop()
	}
	return LNil
}

/* }}} */

/* load and function call operations {{{ */

func (ls *LState) LoadString(source string) (*LFunction, error) {
	return ls.Load(strings.NewReader(source), "<string>")
}

// LoadIR compiles IR into bytecode and returns an executable Lua function.
func (ls *LState) LoadIR(program *IRProgram) (*LFunction, error) {
	proto, err := CompileIR(program)
	if err != nil {
		return nil, newApiErrorE(ApiErrorSyntax, err)
	}
	return newLFunctionL(proto, ls.currentEnv(), 0), nil
}

// LoadBytecode loads a precompiled bytecode blob.
func (ls *LState) LoadBytecode(data []byte) (*LFunction, error) {
	proto, err := DecodeFunctionProto(data)
	if err != nil {
		return nil, newApiErrorE(ApiErrorSyntax, err)
	}
	return newLFunctionL(proto, ls.currentEnv(), 0), nil
}

func (ls *LState) DoString(source string) error {
	if fn, err := ls.LoadString(source); err != nil {
		return err
	} else {
		ls.Push(fn)
		return ls.PCall(0, MultRet, nil)
	}
}

// DoBytecode executes precompiled bytecode.
func (ls *LState) DoBytecode(data []byte) error {
	fn, err := ls.LoadBytecode(data)
	if err != nil {
		return err
	}
	ls.Push(fn)
	return ls.PCall(0, MultRet, nil)
}

/* }}} */

/* Tolang original APIs {{{ */

// ToStringMeta returns string representation of given LValue.
// This method calls the `__tostring` meta method if defined.
// If __tostring is absent but __name is in the metatable, falls back to the
// stable type label in __name. Unlike stock Lua, TOL must never expose host
// pointer addresses in contract-visible strings.
func (ls *LState) ToStringMeta(lv LValue) LValue {
	if fn, ok := ls.metaOp1(lv, "__tostring").(*LFunction); ok {
		ls.Push(fn)
		ls.Push(lv)
		ls.Call(1, 1)
		result := ls.reg.Pop()
		if _, ok := result.(LString); !ok {
			ls.RaiseError("'__tostring' must return a string")
		}
		return result
	}
	// If the metatable has __name, use it as the type description.
	if name, ok := ls.metaOp1(lv, "__name").(LString); ok {
		return LString(string(name))
	}
	return LString(lv.String())
}

// Set a module loader to the package.preload table.
func (ls *LState) PreloadModule(name string, loader LGFunction) {
	preload := ls.GetField(ls.GetField(ls.Get(EnvironIndex), "package"), "preload")
	if _, ok := preload.(*LTable); !ok {
		ls.RaiseError("package.preload must be a table")
	}
	ls.SetField(preload, name, ls.NewFunction(loader))
}

/* }}} */

//
