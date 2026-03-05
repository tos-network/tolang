package lua

import (
	"os"
	"strconv"
)

var CompatVarArg = true
var FieldsPerFlush = 50
var RegistrySize = 256 * 20
var RegistryGrowStep = 32
var CallStackSize = 256
var MaxTableGetLoop = 100
var MaxArrayIndex = 67108864

// LUint256 is a 256-bit unsigned integer stored as four 64-bit words in
// little-endian order: lo holds bits 0–63, ml bits 64–127, mh bits 128–191,
// hi bits 192–255.  Simple operations (add, sub, bitwise, shifts, compare)
// are implemented natively with math/bits; complex operations (div, mod, pow)
// bridge to math/big.Int.
type LUint256 struct {
	lo, ml, mh, hi uint64
}

const LUint256Bit = 256

var LUint256Zero = LUint256{}
var LUint256One = LUint256{lo: 1}

const LuaVersion = "Lua 5.1 (uint256)"

var LuaPath = "LUA_PATH"
var LuaLDir string
var LuaPathDefault string
var LuaOS string
var LuaDirSep string
var LuaPathSep = ";"
var LuaPathMark = "?"
var LuaExecDir = "!"
var LuaIgMark = "-"

func init() {
	if strconv.IntSize != 64 {
		panic("tolang deterministic VM requires a 64-bit architecture (int size must be 64)")
	}
	if os.PathSeparator == '/' { // unix-like
		LuaOS = "unix"
		LuaLDir = "/usr/local/share/lua/5.1"
		LuaDirSep = "/"
		LuaPathDefault = "./?.lua;" + LuaLDir + "/?.lua;" + LuaLDir + "/?/init.lua"
	} else { // windows
		LuaOS = "windows"
		LuaLDir = "!\\lua"
		LuaDirSep = "\\"
		LuaPathDefault = ".\\?.lua;" + LuaLDir + "\\?.lua;" + LuaLDir + "\\?\\init.lua"
	}
}
