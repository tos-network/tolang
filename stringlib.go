package lua

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tos-network/tolang/pm"
)

const emptyLString LString = LString("")

func OpenString(L *LState) int {
	var mod *LTable
	//_, ok := L.G.builtinMts[int(LTString)]
	//if !ok {
	mod = L.RegisterModule(StringLibName, strFuncs).(*LTable)
	gmatch := L.NewClosure(strGmatch, L.NewFunction(strGmatchIter))
	mod.RawSetString("gmatch", gmatch)
	mod.RawSetString("gfind", gmatch)
	mod.RawSetString("__index", mod)
	L.G.builtinMts[int(LTString)] = mod
	//}
	L.Push(mod)
	return 1
}

var strFuncs = map[string]LGFunction{
	"byte": strByte,
	"char": strChar,
	// dump REMOVED: serializes function bytecode, security risk
	"find":    strFind,
	"format":  strFormat,
	"gsub":    strGsub,
	"len":     strLen,
	"lower":   strLower,
	"match":   strMatch,
	"rep":     strRep,
	"reverse": strReverse,
	"sub":     strSub,
	"upper":   strUpper,
}

// luaIdxFromU256 converts a LUint256 to a Lua-style integer index (int64).
// Positive values are returned as-is (capped at math.MaxInt64 for huge values).
// Values with the high bit set are treated as two's-complement negative (int64(v.lo)).
func luaIdxFromU256(v LUint256) int64 {
	const maxInt64 uint64 = 1<<63 - 1
	if lu256IsNeg(v) {
		// Two's-complement negative: cast low word as signed.
		return int64(v.lo)
	}
	// Non-negative: if any upper word is set or lo > INT64_MAX, it is a huge
	// positive that is always out of string bounds — cap at MaxInt64.
	if v.hi != 0 || v.mh != 0 || v.ml != 0 || v.lo > maxInt64 {
		return int64(maxInt64)
	}
	return int64(v.lo)
}

// luaPosrelat implements Lua 5.4's posrelat: converts a 1-based Lua index
// (possibly negative for "from end") to a 1-based position within [1, l].
// Returns 0 when the negative index is too far from the start.
func luaPosrelat(pos, l int64) int64 {
	if pos >= 0 {
		return pos
	}
	if -pos > l {
		return 0
	}
	return l + pos + 1
}

func strByte(L *LState) int {
	str := L.CheckString(1)
	l := int64(len(str))

	// Default start index is 1.
	var rawI int64 = 1
	if L.GetTop() >= 2 && L.Get(2) != LNil {
		rawI = luaIdxFromU256(L.CheckUint256(2))
	}
	posi := luaPosrelat(rawI, l)

	// Default end index is posi (the pre-clamp posrelat result — matches Lua 5.4).
	rawJ := posi
	if L.GetTop() >= 3 && L.Get(3) != LNil {
		rawJ = luaIdxFromU256(L.CheckUint256(3))
	}
	pose := luaPosrelat(rawJ, l)

	// Clamp to valid range.
	if posi < 1 {
		posi = 1
	}
	if pose > l {
		pose = l
	}
	// Empty interval → no values (nil).
	if posi > pose {
		return 0
	}

	for i := posi - 1; i < pose; i++ {
		L.chargeGas(1)
		L.Push(lu256FromInt(int(str[i])))
	}
	return int(pose - posi + 1)
}

func findPatternMatches(L *LState, pattern string, src []byte, offset, limit int) ([]*pm.MatchData, error) {
	return pm.FindWithStep(pattern, src, offset, limit, func() {
		L.chargeGas(1)
	})
}

func strChar(L *LState) int {
	top := L.GetTop()
	bytes := make([]byte, top)
	for i := 1; i <= top; i++ {
		L.chargeGas(1)
		v := L.CheckInt(i)
		if v < 0 || v > 255 {
			L.ArgError(i, "value out of range")
		}
		bytes[i-1] = byte(v)
	}
	L.Push(LString(string(bytes)))
	return 1
}

func strFind(L *LState) int {
	str := L.CheckString(1)
	pattern := L.CheckString(2)
	l := len(str)

	// Compute init position (handles negative indices via LUint256 signed conversion).
	var initV LUint256
	if L.GetTop() >= 3 && L.Get(3) != LNil {
		initV = L.CheckUint256(3)
	} else {
		initV = LUint256One
	}
	init := luaU256Index2StringIndex(str, initV, true)

	if len(pattern) == 0 {
		// Empty pattern: init must be <= len(str) (0-based) i.e. init <= l.
		if init > l {
			L.Push(LNil)
			return 1
		}
		L.Push(lu256FromInt(init + 1))
		L.Push(lu256FromInt(init))
		return 2
	}

	plain := false
	if L.GetTop() >= 4 {
		plain = LVAsBool(L.Get(4))
	}

	if plain {
		pos := strings.Index(str[init:], pattern)
		if pos < 0 {
			L.Push(LNil)
			return 1
		}
		L.Push(lu256FromInt(init + pos + 1))
		L.Push(lu256FromInt(init + pos + len(pattern)))
		return 2
	}

	mds, err := findPatternMatches(L, pattern, unsafeFastStringToReadOnlyBytes(str), init, 1)
	if err != nil {
		L.RaiseError(err.Error())
	}
	if len(mds) == 0 {
		L.Push(LNil)
		return 1
	}
	md := mds[0]
	L.Push(lu256FromInt(md.Capture(0) + 1))
	L.Push(lu256FromInt(md.Capture(1)))
	for i := 2; i < md.CaptureLength(); i += 2 {
		if md.IsPosCapture(i) {
			L.Push(lu256FromInt(md.Capture(i)))
		} else {
			L.Push(LString(str[md.Capture(i):md.Capture(i+1)]))
		}
	}
	return md.CaptureLength()/2 + 1
}

func hasUnsupportedFloatFormatVerb(format string) bool {
	inVerb := false
	for i := 0; i < len(format); i++ {
		ch := format[i]
		if !inVerb {
			if ch == '%' {
				if i+1 < len(format) && format[i+1] == '%' {
					i++
					continue
				}
				inVerb = true
			}
			continue
		}
		switch ch {
		case 'e', 'E', 'f', 'F', 'g', 'G':
			return true
		case 'd', 'i', 'o', 'u', 'x', 'X', 'c', 'q', 's':
			inVerb = false
		default:
			// keep scanning until we hit a terminal verb
		}
	}
	return false
}

// luaQuoteString implements Lua 5.4's addquoted: wraps s in double-quotes,
// escaping backslash, double-quote, and newline as backslash sequences, and
// control characters (0x00–0x1F, 0x7F) as decimal escapes (\ddd or \d).
func luaQuoteString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\' || c == '\n':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c < 0x20 || c == 0x7f: // control character
			// If the next byte is an ASCII digit, use 3-digit form to avoid ambiguity.
			if i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
				b.WriteString(fmt.Sprintf("\\%03d", int(c)))
			} else {
				b.WriteString(fmt.Sprintf("\\%d", int(c)))
			}
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// fmtGetUint256 extracts a LUint256 from a format argument, raising an error
// if it cannot be converted.
func fmtGetUint256(L *LState, arg LValue, argN int) LUint256 {
	if v, ok := arg.(LUint256); ok {
		return v
	}
	if s, ok := arg.(LString); ok {
		if v, err := parseUint256(string(s)); err == nil {
			return v
		}
	}
	L.ArgError(argN, "number expected, got "+arg.Type().String())
	return LUint256Zero
}

func strFormat(L *LState) int {
	str := L.CheckString(1)
	if hasUnsupportedFloatFormatVerb(str) {
		L.RaiseError("string.format: float format verbs are not supported in uint256 mode")
	}

	var buf strings.Builder
	var outLen int64
	argIdx := 2 // next argument position (1-based in Lua stack)
	i := 0
	for i < len(str) {
		if str[i] != '%' {
			addStringResultBytes(L, "string.format", &outLen, 1)
			buf.WriteByte(str[i])
			i++
			continue
		}
		i++ // skip '%'
		if i >= len(str) {
			break
		}
		if str[i] == '%' {
			addStringResultBytes(L, "string.format", &outLen, 1)
			buf.WriteByte('%')
			i++
			continue
		}
		// Collect the verb: flags, width, precision, verb letter.
		verbStart := i - 1 // position of the leading '%'
		// Scan flags: -, +, space, #, 0
		for i < len(str) && strings.ContainsRune("-+ #0", rune(str[i])) {
			i++
		}
		// Width (optional digits)
		for i < len(str) && str[i] >= '0' && str[i] <= '9' {
			i++
		}
		// Precision (optional .digits)
		if i < len(str) && str[i] == '.' {
			i++
			for i < len(str) && str[i] >= '0' && str[i] <= '9' {
				i++
			}
		}
		if i >= len(str) {
			break
		}
		verb := str[i]
		i++
		verbFmt := str[verbStart:i] // e.g. "%5d" or "%-10s"
		if width := formatVerbWidth(verbFmt); width > maxStringResultBytes {
			L.RaiseError("string.format: output too large (%d bytes, limit %d)", width, maxStringResultBytes)
		}

		switch verb {
		case 's':
			// FMT-%s: use tostring (calls __tostring metamethod if available,
			// falls back to the value's String() method for nil, bool, etc.).
			arg := L.Get(argIdx)
			argIdx++
			s := L.ToStringMeta(arg).String()
			// FMT-NUL: raise error if string contains NUL and format has a
			// width or precision specifier (e.g. "%10s" or "%.5s").
			// Plain "%s" (no width/precision) is allowed to contain NUL bytes.
			hasSizeSpec := len(verbFmt) > 2 && strings.ContainsAny(verbFmt[1:len(verbFmt)-1], "0123456789.")
			if hasSizeSpec && strings.ContainsRune(s, 0) {
				L.ArgError(argIdx-1, "string contains zeros")
			}
			formatted := fmt.Sprintf(verbFmt, s)
			addStringResultBytes(L, "string.format", &outLen, len(formatted))
			buf.WriteString(formatted)
		case 'c':
			// FMT-%c: use raw byte, not rune (avoids UTF-8 multi-byte for n>127).
			arg := L.Get(argIdx)
			argIdx++
			var n int
			if iv, ok := arg.(LUint256); ok {
				var ok2 bool
				n, ok2 = lu256ToInt(iv)
				if !ok2 {
					L.ArgError(argIdx-1, "number out of range")
				}
			} else {
				L.ArgError(argIdx-1, "number expected")
			}
			// Build the single-byte string and apply width/flags by replacing
			// the terminal 'c' with 's' in the format verb.
			charStr := string([]byte{byte(n)})
			if verbFmt == "%c" {
				addStringResultBytes(L, "string.format", &outLen, len(charStr))
				buf.WriteString(charStr)
			} else {
				// Replace trailing 'c' with 's' to apply padding via fmt.Sprintf.
				sFmt := verbFmt[:len(verbFmt)-1] + "s"
				formatted := fmt.Sprintf(sFmt, charStr)
				addStringResultBytes(L, "string.format", &outLen, len(formatted))
				buf.WriteString(formatted)
			}
		case 'd', 'i':
			// Signed decimal: use int64 of the low 64 bits (matches Lua 5.4 lua_Integer semantics).
			arg := L.Get(argIdx)
			argIdx++
			v := fmtGetUint256(L, arg, argIdx-1)
			goFmt := verbFmt[:len(verbFmt)-1] + "d"
			formatted := fmt.Sprintf(goFmt, int64(v.lo))
			addStringResultBytes(L, "string.format", &outLen, len(formatted))
			buf.WriteString(formatted)
		case 'u':
			// Unsigned decimal: Go has no %u; use %d with uint64 (same output for non-negative).
			arg := L.Get(argIdx)
			argIdx++
			v := fmtGetUint256(L, arg, argIdx-1)
			goFmt := verbFmt[:len(verbFmt)-1] + "d"
			formatted := fmt.Sprintf(goFmt, v.lo)
			addStringResultBytes(L, "string.format", &outLen, len(formatted))
			buf.WriteString(formatted)
		case 'o', 'x', 'X':
			// Octal/hex: use uint64 of low 64 bits.
			arg := L.Get(argIdx)
			argIdx++
			v := fmtGetUint256(L, arg, argIdx-1)
			// Replace the verb in verbFmt (verb is last char, keep flags/width/prec).
			goFmt := verbFmt[:len(verbFmt)-1] + string(verb)
			formatted := fmt.Sprintf(goFmt, v.lo)
			addStringResultBytes(L, "string.format", &outLen, len(formatted))
			buf.WriteString(formatted)
		case 'q':
			// Lua-style quoting: strings get addquoted, integers get decimal literal.
			arg := L.Get(argIdx)
			argIdx++
			switch sv := arg.(type) {
			case LString:
				quotedLen := luaQuotedLen(string(sv))
				addStringResultBytes(L, "string.format", &outLen, int(quotedLen))
				buf.WriteString(luaQuoteString(string(sv)))
			case LUint256:
				// Lua 5.4 formats integers as signed decimal (or hex for INT64_MIN).
				n := int64(sv.lo)
				const int64Min int64 = -1 << 63
				if n == int64Min {
					formatted := fmt.Sprintf("0x%016x", uint64(n))
					addStringResultBytes(L, "string.format", &outLen, len(formatted))
					buf.WriteString(formatted)
				} else {
					formatted := fmt.Sprintf("%d", n)
					addStringResultBytes(L, "string.format", &outLen, len(formatted))
					buf.WriteString(formatted)
				}
			case *LNilType:
				addStringResultBytes(L, "string.format", &outLen, len("nil"))
				buf.WriteString("nil")
			case LBool:
				if bool(sv) {
					addStringResultBytes(L, "string.format", &outLen, len("true"))
					buf.WriteString("true")
				} else {
					addStringResultBytes(L, "string.format", &outLen, len("false"))
					buf.WriteString("false")
				}
			default:
				L.ArgError(argIdx-1, "value has no literal form")
			}
		default:
			// Unknown verb — propagate as-is (will produce Go's %!verb(...) error).
			arg := L.Get(argIdx)
			argIdx++
			formatted := fmt.Sprintf(verbFmt, arg)
			addStringResultBytes(L, "string.format", &outLen, len(formatted))
			buf.WriteString(formatted)
		}
	}

	L.Push(LString(buf.String()))
	return 1
}

func formatVerbWidth(verbFmt string) int {
	i := 1
	for i < len(verbFmt)-1 && strings.ContainsRune("-+ #0", rune(verbFmt[i])) {
		i++
	}
	widthStart := i
	for i < len(verbFmt)-1 && verbFmt[i] >= '0' && verbFmt[i] <= '9' {
		i++
	}
	if widthStart == i {
		return 0
	}
	width, err := strconv.Atoi(verbFmt[widthStart:i])
	if err != nil {
		return maxStringResultBytes + 1
	}
	return width
}

func luaQuotedLen(s string) int64 {
	total := int64(2) // opening and closing quote
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\' || c == '\n':
			total += 2
		case c < 0x20 || c == 0x7f:
			if i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
				total += 4
			} else if c >= 100 {
				total += 4
			} else if c >= 10 {
				total += 3
			} else {
				total += 2
			}
		default:
			total++
		}
	}
	return total
}

func strGsub(L *LState) int {
	str := L.CheckString(1)
	pat := L.CheckString(2)
	L.CheckTypes(3, LTString, LTTable, LTFunction)
	repl := L.CheckAny(3)
	limit := L.OptInt(4, -1)

	mds, err := findPatternMatches(L, pat, unsafeFastStringToReadOnlyBytes(str), 0, limit)
	if err != nil {
		L.RaiseError(err.Error())
	}
	if len(mds) == 0 {
		L.SetTop(1)
		L.Push(LUint256Zero)
		return 2
	}
	var out string
	switch lv := repl.(type) {
	case LString:
		out = strGsubStr(L, str, string(lv), mds)
	case *LTable:
		out = strGsubTable(L, str, lv, mds)
	case *LFunction:
		out = strGsubFunc(L, str, lv, mds)
	}
	// SEC-2: cap gsub output to prevent OOM via large replacement expansions.
	if int64(len(out)) > maxStringResultBytes {
		L.RaiseError("string.gsub: output too large (%d bytes, limit %d)", len(out), maxStringResultBytes)
		return 0
	}
	L.Push(LString(out))
	L.Push(lu256FromInt(len(mds)))
	return 2
}

type replaceInfo struct {
	Indicies []int
	String   string
}

func checkCaptureIndex(L *LState, m *pm.MatchData, idx int) {
	if idx <= 2 {
		return
	}
	if idx >= m.CaptureLength() {
		L.RaiseError("invalid capture index")
	}
}

func capturedString(L *LState, m *pm.MatchData, str string, idx int) string {
	checkCaptureIndex(L, m, idx)
	if idx >= m.CaptureLength() && idx == 2 {
		idx = 0
	}
	if m.IsPosCapture(idx) {
		return fmt.Sprint(m.Capture(idx))
	} else {
		return str[m.Capture(idx):m.Capture(idx+1)]
	}

}

func strGsubDoReplace(str string, info []replaceInfo) string {
	offset := 0
	buf := []byte(str)
	for _, replace := range info {
		oldlen := len(buf)
		b1 := append([]byte(""), buf[0:offset+replace.Indicies[0]]...)
		b2 := []byte("")
		index2 := offset + replace.Indicies[1]
		if index2 <= len(buf) {
			b2 = append(b2, buf[index2:len(buf)]...)
		}
		buf = append(b1, replace.String...)
		buf = append(buf, b2...)
		offset += len(buf) - oldlen
	}
	return string(buf)
}

func strGsubStr(L *LState, str string, repl string, matches []*pm.MatchData) string {
	infoList := make([]replaceInfo, 0, len(matches))
	for _, match := range matches {
		start, end := match.Capture(0), match.Capture(1)
		sc := newFlagScanner('%', "", "", repl)
		for c, eos := sc.Next(); !eos; c, eos = sc.Next() {
			if !sc.ChangeFlag {
				if sc.HasFlag {
					if c >= '0' && c <= '9' {
						sc.AppendString(capturedString(L, match, str, 2*(int(c)-48)))
					} else {
						sc.AppendChar('%')
						sc.AppendChar(c)
					}
					sc.HasFlag = false
				} else {
					sc.AppendChar(c)
				}
			}
		}
		infoList = append(infoList, replaceInfo{[]int{start, end}, sc.String()})
	}

	return strGsubDoReplace(str, infoList)
}

func strGsubTable(L *LState, str string, repl *LTable, matches []*pm.MatchData) string {
	infoList := make([]replaceInfo, 0, len(matches))
	for _, match := range matches {
		idx := 0
		if match.CaptureLength() > 2 { // has captures
			idx = 2
		}
		var value LValue
		if match.IsPosCapture(idx) {
			value = L.GetTable(repl, lu256FromInt(match.Capture(idx)))
		} else {
			value = L.GetField(repl, str[match.Capture(idx):match.Capture(idx+1)])
		}
		if !LVIsFalse(value) {
			infoList = append(infoList, replaceInfo{[]int{match.Capture(0), match.Capture(1)}, LVAsString(value)})
		}
	}
	return strGsubDoReplace(str, infoList)
}

func strGsubFunc(L *LState, str string, repl *LFunction, matches []*pm.MatchData) string {
	infoList := make([]replaceInfo, 0, len(matches))
	for _, match := range matches {
		start, end := match.Capture(0), match.Capture(1)
		L.Push(repl)
		nargs := 0
		if match.CaptureLength() > 2 { // has captures
			for i := 2; i < match.CaptureLength(); i += 2 {
				if match.IsPosCapture(i) {
					L.Push(lu256FromInt(match.Capture(i)))
				} else {
					L.Push(LString(capturedString(L, match, str, i)))
				}
				nargs++
			}
		} else {
			L.Push(LString(capturedString(L, match, str, 0)))
			nargs++
		}
		L.Call(nargs, 1)
		ret := L.reg.Pop()
		if !LVIsFalse(ret) {
			infoList = append(infoList, replaceInfo{[]int{start, end}, LVAsString(ret)})
		}
	}
	return strGsubDoReplace(str, infoList)
}

type strMatchData struct {
	str     string
	pos     int
	matches []*pm.MatchData
}

func strGmatchIter(L *LState) int {
	md := L.CheckUserData(1).Value.(*strMatchData)
	str := md.str
	matches := md.matches
	idx := md.pos
	md.pos += 1
	if idx == len(matches) {
		return 0
	}
	L.Push(L.Get(1))
	match := matches[idx]
	if match.CaptureLength() == 2 {
		L.Push(LString(str[match.Capture(0):match.Capture(1)]))
		return 1
	}

	for i := 2; i < match.CaptureLength(); i += 2 {
		if match.IsPosCapture(i) {
			L.Push(lu256FromInt(match.Capture(i)))
		} else {
			L.Push(LString(str[match.Capture(i):match.Capture(i+1)]))
		}
	}
	return match.CaptureLength()/2 - 1
}

func strGmatch(L *LState) int {
	str := L.CheckString(1)
	pattern := L.CheckString(2)
	mds, err := findPatternMatches(L, pattern, []byte(str), 0, -1)
	if err != nil {
		L.RaiseError(err.Error())
	}
	L.Push(L.Get(UpvalueIndex(1)))
	ud := L.NewUserData()
	ud.Value = &strMatchData{str, 0, mds}
	L.Push(ud)
	return 2
}

func strLen(L *LState) int {
	str := L.CheckString(1)
	L.Push(lu256FromInt(len(str)))
	return 1
}

func strLower(L *LState) int {
	str := L.CheckString(1)
	chargeChunkedWorkGas(L, len(str), cryptoLinearWorkGasChunkBytes)
	L.Push(LString(strings.ToLower(str)))
	return 1
}

func strMatch(L *LState) int {
	str := L.CheckString(1)
	pattern := L.CheckString(2)
	offset := L.OptInt(3, 1)
	l := len(str)
	if offset < 0 {
		offset = l + offset + 1
	}
	offset--
	if offset < 0 {
		offset = 0
	}

	mds, err := findPatternMatches(L, pattern, unsafeFastStringToReadOnlyBytes(str), offset, 1)
	if err != nil {
		L.RaiseError(err.Error())
	}
	if len(mds) == 0 {
		L.Push(LNil)
		return 0
	}
	md := mds[0]
	nsubs := md.CaptureLength() / 2
	switch nsubs {
	case 1:
		L.Push(LString(str[md.Capture(0):md.Capture(1)]))
		return 1
	default:
		for i := 2; i < md.CaptureLength(); i += 2 {
			if md.IsPosCapture(i) {
				L.Push(lu256FromInt(md.Capture(i)))
			} else {
				L.Push(LString(str[md.Capture(i):md.Capture(i+1)]))
			}
		}
		return nsubs - 1
	}
}

func strRep(L *LState) int {
	str := L.CheckString(1)
	n := L.CheckInt(2)
	sep := L.OptString(3, "")
	if n <= 0 {
		L.Push(emptyLString)
	} else {
		// SEC-2: guard against OOM/CPU DoS via huge repetition counts.
		// Use int64 arithmetic to avoid overflow before the comparison.
		outputLen := int64(len(str))*int64(n) + int64(len(sep))*int64(intMax(0, n-1))
		if outputLen > maxStringResultBytes {
			L.RaiseError("string.rep: output too large (%d bytes, limit %d)", outputLen, maxStringResultBytes)
			return 0
		}
		if sep == "" {
			L.Push(LString(strings.Repeat(str, n)))
		} else {
			// Build with separator between repetitions.
			var buf strings.Builder
			for i := 0; i < n; i++ {
				if i > 0 {
					buf.WriteString(sep)
				}
				buf.WriteString(str)
			}
			L.Push(LString(buf.String()))
		}
	}
	return 1
}

func strReverse(L *LState) int {
	str := L.CheckString(1)
	bts := []byte(str)
	chargeChunkedWorkGas(L, len(bts), cryptoLinearWorkGasChunkBytes)
	out := make([]byte, len(bts))
	for i, j := 0, len(bts)-1; j >= 0; i, j = i+1, j-1 {
		out[i] = bts[j]
	}
	L.Push(LString(string(out)))
	return 1
}

func strSub(L *LState) int {
	str := L.CheckString(1)
	start := luaU256Index2StringIndex(str, L.CheckUint256(2), true)
	l := len(str)
	var endV LUint256
	if L.GetTop() >= 3 && L.Get(3) != LNil {
		endV = L.CheckUint256(3)
	} else {
		// Default end = -1 (last byte): represent as two's-complement -1 in uint256.
		endV = LUint256{lo: ^uint64(0), ml: ^uint64(0), mh: ^uint64(0), hi: ^uint64(0)}
	}
	end := luaU256Index2StringIndex(str, endV, false)
	if start >= l || end < start {
		L.Push(emptyLString)
	} else {
		L.Push(LString(str[start:end]))
	}
	return 1
}

func strUpper(L *LState) int {
	str := L.CheckString(1)
	chargeChunkedWorkGas(L, len(str), cryptoLinearWorkGasChunkBytes)
	L.Push(LString(strings.ToUpper(str)))
	return 1
}

func luaIndex2StringIndex(str string, i int, start bool) int {
	if start && i != 0 {
		i -= 1
	}
	l := len(str)
	if i < 0 {
		i = l + i + 1
	}
	i = intMax(0, i)
	if !start && i > l {
		i = l
	}
	return i
}

// luaU256ToStringPos converts a LUint256 index argument to a Go int,
// handling Lua negative indices (large uint256 values with top bits set).
func luaU256ToStringPos(v LUint256) (int, bool) {
	if v.hi>>63 == 0 {
		// Non-negative: use normal conversion.
		return lu256ToInt(v)
	}
	// Treat as two's-complement negative: must have all upper words = 0xFFFF...FFFF.
	if v.hi != ^uint64(0) || v.mh != ^uint64(0) || v.ml != ^uint64(0) {
		return 0, false
	}
	return int(int64(v.lo)), true
}

// luaU256Index2StringIndex converts a LUint256 index to a byte offset, handling
// negative Lua indices. Falls back to lu256ToInt behaviour for non-negative values.
func luaU256Index2StringIndex(str string, v LUint256, start bool) int {
	i, ok := luaU256ToStringPos(v)
	if !ok {
		// Out of range (enormous positive or too-negative): clamp to boundary.
		if v.hi>>63 == 0 {
			// Very large positive — clamp to end.
			if start {
				return len(str)
			}
			return len(str)
		}
		// Very negative — clamp to 0.
		return 0
	}
	return luaIndex2StringIndex(str, i, start)
}

//
