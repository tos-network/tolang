package lua

import (
	"fmt"
	"strconv"
	"strings"
)

type LValueType int

const (
	LTNil LValueType = iota
	LTBool
	LTUint256
	LTString
	LTAddress
	LTFunction
	LTUserData
	LTTable
)

var lValueNames = [8]string{"nil", "boolean", "uint256", "string", "address", "function", "userdata", "table"}

func (vt LValueType) String() string {
	return lValueNames[int(vt)]
}

type LValue interface {
	String() string
	Type() LValueType
}

// LVIsFalse returns true if a given LValue is a nil or false otherwise false.
func LVIsFalse(v LValue) bool { return v == LNil || v == LFalse }

// LVIsFalse returns false if a given LValue is a nil or false otherwise true.
func LVAsBool(v LValue) bool { return v != LNil && v != LFalse }

// LVAsString returns string representation of a given LValue
// if the LValue is a string or number, otherwise an empty string.
func LVAsString(v LValue) string {
	switch sn := v.(type) {
	case LString, LUint256:
		return sn.String()
	default:
		return ""
	}
}

// LVCanConvToString returns true if a given LValue is a string or number
// otherwise false.
func LVCanConvToString(v LValue) bool {
	switch v.(type) {
	case LString, LUint256:
		return true
	default:
		return false
	}
}

// Lu256Cmp is the exported comparison function for LUint256.
// Returns -1 if a < b, 0 if a == b, +1 if a > b.
func Lu256Cmp(a, b LUint256) int { return lu256Cmp(a, b) }

// Lu256FromUint64 creates an LUint256 from a uint64.
func Lu256FromUint64(v uint64) LUint256 { return LUint256{lo: v} }

// LVAsUint256 tries to convert a given LValue to a uint256.
func LVAsUint256(v LValue) LUint256 {
	switch lv := v.(type) {
	case LUint256:
		return lv
	case LString:
		if num, err := parseNumber(string(lv)); err == nil {
			return num
		}
	}
	return LUint256Zero
}

type LNilType struct{}

func (nl *LNilType) String() string   { return "nil" }
func (nl *LNilType) Type() LValueType { return LTNil }

var LNil = LValue(&LNilType{})

type LBool bool

func (bl LBool) String() string {
	if bool(bl) {
		return "true"
	}
	return "false"
}
func (bl LBool) Type() LValueType { return LTBool }

var LTrue = LBool(true)
var LFalse = LBool(false)

type LString string

func (st LString) String() string   { return string(st) }
func (st LString) Type() LValueType { return LTString }

// fmt.Formatter interface
func (st LString) Format(f fmt.State, c rune) {
	switch c {
	case 'd', 'i':
		if nm, err := parseNumber(string(st)); err != nil {
			defaultFormat(nm, f, 'd')
		} else {
			defaultFormat(string(st), f, 's')
		}
	default:
		defaultFormat(string(st), f, c)
	}
}

type LAddress string

func (ad LAddress) String() string   { return string(ad) }
func (ad LAddress) Type() LValueType { return LTAddress }

// fmt.Formatter interface
func (ad LAddress) Format(f fmt.State, c rune) {
	defaultFormat(string(ad), f, c)
}

func (nm LUint256) String() string {
	if lu256IsZero(nm) {
		return "0"
	}
	const chunk = uint64(10_000_000_000_000_000_000) // 10^19
	d := LUint256{lo: chunk}
	var parts [5]uint64 // ceil(78/19) = 5
	n := 0
	v := nm
	for !lu256IsZero(v) {
		var r LUint256
		v, r = lu256DivMod(v, d)
		parts[n] = r.lo
		n++
	}
	var sb strings.Builder
	// Most significant chunk: no leading zeros.
	sb.WriteString(strconv.FormatUint(parts[n-1], 10))
	// Remaining chunks (from second-most to least significant): zero-pad to 19 digits.
	for i := n - 2; i >= 0; i-- {
		sb.WriteString(fmt.Sprintf("%019d", parts[i]))
	}
	return sb.String()
}

func (nm LUint256) Type() LValueType { return LTUint256 }

// fmt.Formatter interface
func (nm LUint256) Format(f fmt.State, c rune) {
	switch c {
	case 'q', 's':
		defaultFormat(nm.String(), f, c)
	case 'b', 'c', 'd', 'o', 'x', 'X', 'U':
		defaultFormat(int64(nm.lo), f, c)
	case 'i':
		defaultFormat(int64(nm.lo), f, 'd')
	case 'e', 'E', 'f', 'F', 'g', 'G':
		defaultFormat(nm.String(), f, 's')
	default:
		defaultFormat(nm.String(), f, c)
	}
}

type LTable struct {
	Metatable LValue

	array   []LValue
	dict    map[LValue]LValue
	strdict map[string]LValue
	keys    []LValue
	k2i     map[LValue]int
}

func (tb *LTable) String() string   { return "table" }
func (tb *LTable) Type() LValueType { return LTTable }

type LFunction struct {
	IsG       bool
	Env       *LTable
	Proto     *FunctionProto
	GFunction LGFunction
	Upvalues  []*Upvalue
}
type LGFunction func(*LState) int

func (fn *LFunction) String() string   { return "function" }
func (fn *LFunction) Type() LValueType { return LTFunction }

type Global struct {
	MainThread    *LState
	CurrentThread *LState
	Registry      *LTable
	Global        *LTable

	builtinMts map[int]LValue
	gccount    int32
}

type LState struct {
	G       *Global
	Env     *LTable
	Panic   func(*LState)
	Dead    bool
	Options Options

	reg          *registry
	stack        callFrameStack
	alloc        *allocator
	currentFrame *callFrame
	uvcache      *Upvalue
	hasErrorFunc bool
	mainLoop     func(*LState, *callFrame)

	// Gas metering: set via SetGasLimit before execution.
	gasLimit uint64
	gasUsed  uint64

	// Line hook: called once per executed instruction when non-nil.
	// The source string comes from FunctionProto.SourceName of the active frame.
	lineHook func(source string, line int)
}

// SetGasLimit configures the maximum number of VM instructions this LState
// may execute. Must be called before DoString/DoFile/Call. Zero means unlimited.
func (ls *LState) SetGasLimit(limit uint64) {
	ls.gasLimit = limit
	ls.gasUsed = 0
}

// GasUsed returns the number of VM instructions executed so far.
func (ls *LState) GasUsed() uint64 { return ls.gasUsed }

// SetLineHook installs a hook function that is called after each VM instruction
// with the FunctionProto.SourceName and the source line from DbgSourcePositions.
// Pass nil to disable.
func (ls *LState) SetLineHook(fn func(string, int)) {
	ls.lineHook = fn
}

type LUserData struct {
	Value     interface{}
	Env       *LTable
	Metatable LValue
}

func (ud *LUserData) String() string   { return "userdata" }
func (ud *LUserData) Type() LValueType { return LTUserData }
