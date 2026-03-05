package lua

import (
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"unsafe"
)

func intMin(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

func intMax(a, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}

func defaultFormat(v interface{}, f fmt.State, c rune) {
	buf := make([]string, 0, 10)
	buf = append(buf, "%")
	for i := 0; i < 128; i++ {
		if f.Flag(i) {
			buf = append(buf, string(rune(i)))
		}
	}

	if w, ok := f.Width(); ok {
		buf = append(buf, strconv.Itoa(w))
	}
	if p, ok := f.Precision(); ok {
		buf = append(buf, "."+strconv.Itoa(p))
	}
	buf = append(buf, string(c))
	format := strings.Join(buf, "")
	fmt.Fprintf(f, format, v)
}

type flagScanner struct {
	flag       byte
	start      string
	end        string
	buf        []byte
	str        string
	Length     int
	Pos        int
	HasFlag    bool
	ChangeFlag bool
}

func newFlagScanner(flag byte, start, end, str string) *flagScanner {
	return &flagScanner{flag, start, end, make([]byte, 0, len(str)), str, len(str), 0, false, false}
}

func (fs *flagScanner) AppendString(str string) { fs.buf = append(fs.buf, str...) }

func (fs *flagScanner) AppendChar(ch byte) { fs.buf = append(fs.buf, ch) }

func (fs *flagScanner) String() string { return string(fs.buf) }

func (fs *flagScanner) Next() (byte, bool) {
	c := byte('\000')
	fs.ChangeFlag = false
	if fs.Pos == fs.Length {
		if fs.HasFlag {
			fs.AppendString(fs.end)
		}
		return c, true
	}

	c = fs.str[fs.Pos]
	if c == fs.flag {
		if fs.Pos < (fs.Length-1) && fs.str[fs.Pos+1] == fs.flag {
			fs.HasFlag = false
			fs.AppendChar(fs.flag)
			fs.Pos += 2
			return fs.Next()
		} else if fs.Pos != fs.Length-1 {
			if fs.HasFlag {
				fs.AppendString(fs.end)
			}
			fs.AppendString(fs.start)
			fs.ChangeFlag = true
			fs.HasFlag = true
		}
	}
	fs.Pos++
	return c, false
}

func isInteger(v LUint256) bool {
	return true // any LUint256 is a valid uint256
}

func isArrayKey(v LUint256) bool {
	idx, ok := lu256ToInt(v)
	return ok && idx > 0 && idx < MaxArrayIndex
}

// parseNumber converts a number string to LUint256. Used by the source-code
// compiler. Accepts decimal, hex (0x prefix), and scientific notation
// (e.g. "1e10" → 10_000_000_000). Underscore separators are stripped by the
// lexer before this function is called.
func parseNumber(number string) (LUint256, error) {
	return parseNumberTonumber(number)
}

// parseNumberTonumber is like parseNumber but also accepts scientific notation
// (e.g. "2e2" → 200). Used exclusively by tonumber(); NOT used by the compiler
// so that source literals like "1e5" remain rejected.
func parseNumberTonumber(number string) (LUint256, error) {
	v, err := parseUint256(number)
	if err == nil {
		return v, nil
	}
	// Try scientific notation: e.g. "2e2" → 200, "1e10" → 10000000000.
	// Only non-negative integer results are accepted (TOL has no floats).
	s := strings.TrimSpace(number)
	eIdx := strings.IndexAny(s, "eE")
	if eIdx < 0 {
		return LUint256Zero, err
	}
	mantissaStr := s[:eIdx]
	expStr := s[eIdx+1:]
	// Mantissa must be a valid non-negative integer string.
	mantissa, err2 := parseUint256(mantissaStr)
	if err2 != nil {
		return LUint256Zero, err
	}
	// Exponent must be a non-negative integer.
	exp, err3 := strconv.ParseInt(expStr, 10, 64)
	if err3 != nil || exp < 0 {
		return LUint256Zero, err
	}
	// Compute mantissa * 10^exp using big.Int for intermediate work then convert.
	tenBig := big.NewInt(10)
	expBig := new(big.Int).Exp(tenBig, big.NewInt(exp), nil)
	mBig := lu256ToBig(mantissa)
	result := new(big.Int).Mul(mBig, expBig)
	if result.Sign() < 0 {
		return LUint256Zero, errNegativeNumber
	}
	return bigToLU256(result)
}

func int2Fb(val int) int {
	e := 0
	x := val
	for x >= 16 {
		x = (x + 1) >> 1
		e++
	}
	if x < 8 {
		return x
	}
	return ((e + 1) << 3) | (x - 8)
}

func strCmp(s1, s2 string) int {
	len1 := len(s1)
	len2 := len(s2)
	for i := 0; ; i++ {
		c1 := -1
		if i < len1 {
			c1 = int(s1[i])
		}
		c2 := -1
		if i != len2 {
			c2 = int(s2[i])
		}
		switch {
		case c1 < c2:
			return -1
		case c1 > c2:
			return +1
		case c1 < 0:
			return 0
		}
	}
}

func unsafeFastStringToReadOnlyBytes(s string) (bs []byte) {
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&bs))
	bh.Data = sh.Data
	bh.Cap = sh.Len
	bh.Len = sh.Len
	return
}
