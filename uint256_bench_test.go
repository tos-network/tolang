package lua

import (
	"math/big"
	"testing"
)

// lu256ToBigTest converts LUint256 to *big.Int for use in reference benchmarks.
func lu256ToBigTest(n LUint256) *big.Int {
	b := new(big.Int)
	b.SetBits([]big.Word{big.Word(n.lo), big.Word(n.ml), big.Word(n.mh), big.Word(n.hi)})
	return b
}

// bigToLU256Test converts *big.Int to LUint256 for use in reference benchmarks.
func bigToLU256Test(b *big.Int) LUint256 {
	words := b.Bits()
	var lo, ml, mh, hi uint64
	if len(words) > 0 {
		lo = uint64(words[0])
	}
	if len(words) > 1 {
		ml = uint64(words[1])
	}
	if len(words) > 2 {
		mh = uint64(words[2])
	}
	if len(words) > 3 {
		hi = uint64(words[3])
	}
	return LUint256{lo: lo, ml: ml, mh: mh, hi: hi}
}

// Benchmark native uint64 hot paths vs big.Int bridge paths.

var (
	benchA = LUint256{lo: 0xDEADBEEFCAFEBABE, ml: 0x1234567890ABCDEF, mh: 0xFEDCBA9876543210, hi: 0x0123456789ABCDEF}
	benchB = LUint256{lo: 0x0102030405060708, ml: 0x090A0B0C0D0E0F10, mh: 0x1112131415161718, hi: 0x191A1B1C1D1E1F20}
)

// --- hot paths (math/bits, no allocation) ---

func BenchmarkU256Add(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = lu256Add(benchA, benchB)
	}
}

func BenchmarkU256Sub(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = lu256Sub(benchA, benchB)
	}
}

func BenchmarkU256Mul(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = lu256Mul(benchA, benchB)
	}
}

func BenchmarkU256Cmp(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = lu256Cmp(benchA, benchB)
	}
}

func BenchmarkU256Shl(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = lu256Shl(benchA, 37)
	}
}

func BenchmarkU256Shr(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = lu256Shr(benchA, 37)
	}
}

func BenchmarkU256Band(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = lu256Band(benchA, benchB)
	}
}

// --- big.Int bridge paths ---

// 1-word divisor (common in contracts: 10^18, 2^n, etc.)
func BenchmarkU256Div64(b *testing.B) {
	d := LUint256{lo: 1_000_000_000_000_000_000} // 10^18
	for i := 0; i < b.N; i++ {
		_ = lu256Div(benchA, d)
	}
}

// 2-word divisor
func BenchmarkU256Div128(b *testing.B) {
	d := LUint256{lo: 0x0102030405060708, ml: 0x090A0B0C0D0E0F10}
	for i := 0; i < b.N; i++ {
		_ = lu256Div(benchA, d)
	}
}

// 4-word divisor
func BenchmarkU256Div256(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = lu256Div(benchA, benchB)
	}
}

func BenchmarkU256Mod(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = lu256Mod(benchA, benchB)
	}
}

func BenchmarkU256String(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = benchA.String()
	}
}

// --- reference: raw big.Int add for comparison ---

func BenchmarkBigIntAdd(b *testing.B) {
	a := lu256ToBigTest(benchA)
	bb := lu256ToBigTest(benchB)
	mod := new(big.Int).Lsh(big.NewInt(1), 256)
	r := new(big.Int)
	for i := 0; i < b.N; i++ {
		r.Add(a, bb)
		r.Mod(r, mod)
	}
}

func BenchmarkBigIntMul(b *testing.B) {
	a := lu256ToBigTest(benchA)
	bb := lu256ToBigTest(benchB)
	mod := new(big.Int).Lsh(big.NewInt(1), 256)
	r := new(big.Int)
	for i := 0; i < b.N; i++ {
		r.Mul(a, bb)
		r.Mod(r, mod)
	}
}

// --- VM-level: execute Lua arithmetic loop ---

func BenchmarkVMArithLoop(b *testing.B) {
	const script = `
local x = 1
for i = 1, 1000 do
  x = x + i
  x = x * 2
  x = x - i
end
`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		L := NewState()
		if err := L.DoString(script); err != nil {
			b.Fatal(err)
		}
		L.Close()
	}
}

// TestDivModCorrectness cross-checks native div/mod against big.Int for a
// range of divisor sizes (1-word, 2-word, 3-word, 4-word).
func TestDivModCorrectness(t *testing.T) {
	type tc struct {
		a, b LUint256
	}
	cases := []tc{
		// 1-word divisor
		{LUint256{lo: 0xFFFFFFFFFFFFFFFF, ml: 0xFFFFFFFFFFFFFFFF, mh: 0xFFFFFFFFFFFFFFFF, hi: 0xFFFFFFFFFFFFFFFF}, LUint256{lo: 7}},
		{LUint256{lo: 100}, LUint256{lo: 7}},
		{LUint256{lo: 0, ml: 1}, LUint256{lo: 3}},
		// 2-word divisor
		{LUint256{lo: 0xDEADBEEFCAFEBABE, ml: 0x1234567890ABCDEF, mh: 0xFEDCBA9876543210, hi: 0x01}, LUint256{lo: 0x0102030405060708, ml: 0x090A0B0C0D0E0F10}},
		{LUint256{lo: 0, ml: 0, mh: 1}, LUint256{lo: 3, ml: 5}},
		// 3-word divisor
		{LUint256{lo: 0xDEAD, ml: 0xBEEF, mh: 0xCAFE, hi: 0xBABE}, LUint256{lo: 1, ml: 2, mh: 3}},
		// 4-word divisor (quotient = 1)
		{LUint256{lo: 0xDEADBEEFCAFEBABE, ml: 0x1234567890ABCDEF, mh: 0xFEDCBA9876543210, hi: 0x0123456789ABCDEF},
			LUint256{lo: 0x0102030405060708, ml: 0x090A0B0C0D0E0F10, mh: 0x1112131415161718, hi: 0x091A1B1C1D1E1F20}},
		// Edge: a just above b
		{LUint256{lo: 8}, LUint256{lo: 7}},
		// Large shift normalisation (top word has leading zeros)
		{LUint256{lo: 0xFFFFFFFFFFFFFFFF, ml: 0xFFFFFFFFFFFFFFFF, mh: 0xFFFFFFFFFFFFFFFF, hi: 0xFFFFFFFFFFFFFFFF},
			LUint256{lo: 0, ml: 1}}, // divisor = 2^64
	}
	for _, c := range cases {
		wantQ := bigToLU256Test(new(big.Int).Quo(lu256ToBigTest(c.a), lu256ToBigTest(c.b)))
		wantR := bigToLU256Test(new(big.Int).Mod(lu256ToBigTest(c.a), lu256ToBigTest(c.b)))
		gotQ, gotR := lu256DivMod(c.a, c.b)
		if gotQ != wantQ || gotR != wantR {
			t.Errorf("divmod(%v, %v):\n  got  q=%v r=%v\n  want q=%v r=%v",
				c.a, c.b, gotQ, gotR, wantQ, wantR)
		}
	}
}
