package lua

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"math/bits"
	"strconv"
	"strings"
)

var (
	errInvalidNumber  = errors.New("invalid number")
	errNegativeNumber = errors.New("negative numbers are not supported")
	errNumberOverflow = errors.New("number out of uint256 range")
)

// ─── Parse helpers ──────────────────────────────────────────────────────────

func parseUint256(number string) (LUint256, error) {
	s := strings.TrimSpace(number)
	if s == "" {
		return LUint256Zero, errInvalidNumber
	}
	base := 10
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s, base = s[2:], 16
	} else if strings.HasPrefix(s, "0b") || strings.HasPrefix(s, "0B") {
		s, base = s[2:], 2
	}
	// Note: leading zeros are NOT treated as octal (unlike C). "00010" == 10.
	return parseUint256Base(s, base)
}

func parseUint256Base(number string, base int) (LUint256, error) {
	// Do NOT TrimSpace here: "0x 2" should be invalid, not silently parsed as 2.
	// Callers (parseUint256) do their own trimming before stripping the prefix.
	s := number
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	if s == "" || s[0] == '-' {
		return LUint256Zero, errInvalidNumber
	}
	baseU := LUint256{lo: uint64(base)}
	result := LUint256Zero
	for _, ch := range []byte(s) {
		var digit uint64
		switch {
		case '0' <= ch && ch <= '9':
			digit = uint64(ch - '0')
		case 'a' <= ch && ch <= 'f':
			digit = uint64(ch-'a') + 10
		case 'A' <= ch && ch <= 'F':
			digit = uint64(ch-'A') + 10
		default:
			return LUint256Zero, errInvalidNumber
		}
		if digit >= uint64(base) {
			return LUint256Zero, errInvalidNumber
		}
		result = lu256Add(lu256Mul(result, baseU), LUint256{lo: digit})
	}
	return result, nil
}

// ─── Conversion helpers ──────────────────────────────────────────────────────

func lu256FromInt(v int) LUint256 {
	if v <= 0 {
		return LUint256Zero
	}
	return LUint256{lo: uint64(v)}
}

func lu256FromInt64(v int64) LUint256 {
	if v <= 0 {
		return LUint256Zero
	}
	return LUint256{lo: uint64(v)}
}

func lu256FromUint64(v uint64) LUint256 {
	return LUint256{lo: v}
}

func lu256ToInt(v LUint256) (int, bool) {
	if v.hi != 0 || v.mh != 0 || v.ml != 0 {
		return 0, false
	}
	if v.lo > uint64(^uint(0)>>1) {
		return 0, false
	}
	return int(v.lo), true
}

func lu256ToInt64(v LUint256) (int64, bool) {
	if v.hi != 0 || v.mh != 0 || v.ml != 0 {
		return 0, false
	}
	const maxInt64 uint64 = 1<<63 - 1
	if v.lo > maxInt64 {
		return 0, false
	}
	return int64(v.lo), true
}

// ─── Comparison / predicate ──────────────────────────────────────────────────

func lu256Cmp(a, b LUint256) int {
	if a.hi != b.hi {
		if a.hi > b.hi {
			return 1
		}
		return -1
	}
	if a.mh != b.mh {
		if a.mh > b.mh {
			return 1
		}
		return -1
	}
	if a.ml != b.ml {
		if a.ml > b.ml {
			return 1
		}
		return -1
	}
	if a.lo != b.lo {
		if a.lo > b.lo {
			return 1
		}
		return -1
	}
	return 0
}

func lu256IsZero(v LUint256) bool {
	return v == LUint256{}
}

// lu256IsNeg reports whether v is "negative" in EVM two's complement uint256
// semantics: a value with bit 255 set (v >= 2^255) is considered negative.
func lu256IsNeg(v LUint256) bool {
	return v.hi>>63 != 0
}

// lu256SCmp compares a and b as signed 256-bit two's-complement integers.
// Returns -1, 0, or 1 as a < b, a == b, or a > b.
func lu256SCmp(a, b LUint256) int {
	aNeg := lu256IsNeg(a)
	bNeg := lu256IsNeg(b)
	if aNeg != bNeg {
		if aNeg {
			return -1
		}
		return 1
	}
	return lu256Cmp(a, b)
}

// lu256ToSignedInt converts a LUint256 to a signed Go int, treating values
// with the top bit set as negative (two's complement interpretation).
// This is used for string index arguments where Lua uses negative indices.
// Returns (0, false) if the value is too large to represent as a signed int.
func lu256ToSignedInt(v LUint256) (int, bool) {
	// If top bit is not set, treat as normal non-negative value.
	if v.hi>>63 == 0 {
		return lu256ToInt(v)
	}
	// Top bit is set: interpret as two's complement negative.
	// Only supported if the upper 3 words are all 0xFFFF...FFFF (i.e. the value
	// fits in int64 as a negative number: v = 2^256 + signedValue where signedValue < 0).
	if v.hi != ^uint64(0) || v.mh != ^uint64(0) || v.ml != ^uint64(0) {
		// Too far from zero to be a useful signed int.
		return 0, false
	}
	// lo word as signed int64 (will be negative since top bit of lo is set if lo > maxInt64,
	// or it may be any value — the sign is determined by the upper words).
	return int(int64(v.lo)), true
}

// ─── Pure uint64 arithmetic (hot path) ──────────────────────────────────────

func lu256Add(a, b LUint256) LUint256 {
	lo, c0 := bits.Add64(a.lo, b.lo, 0)
	ml, c1 := bits.Add64(a.ml, b.ml, c0)
	mh, c2 := bits.Add64(a.mh, b.mh, c1)
	hi, _ := bits.Add64(a.hi, b.hi, c2)
	return LUint256{lo: lo, ml: ml, mh: mh, hi: hi}
}

func lu256Sub(a, b LUint256) LUint256 {
	lo, b0 := bits.Sub64(a.lo, b.lo, 0)
	ml, b1 := bits.Sub64(a.ml, b.ml, b0)
	mh, b2 := bits.Sub64(a.mh, b.mh, b1)
	hi, _ := bits.Sub64(a.hi, b.hi, b2)
	return LUint256{lo: lo, ml: ml, mh: mh, hi: hi}
}

// lu256Mul computes a*b mod 2^256 using schoolbook multiplication.
func lu256Mul(a, b LUint256) LUint256 {
	// word 0
	h0, r0 := bits.Mul64(a.lo, b.lo)

	// word 1 partial products: (a.lo*b.ml) and (a.ml*b.lo)
	h01, p01 := bits.Mul64(a.lo, b.ml)
	h10, p10 := bits.Mul64(a.ml, b.lo)

	r1, c := bits.Add64(h0, p01, 0)
	r1, c = bits.Add64(r1, p10, c)
	carry1 := h01 + h10 + c // carry into word 2 (fits since max = 2*(2^64-1)+1 < 2^65)

	// word 2 partial products: (a.lo*b.mh), (a.ml*b.ml), (a.mh*b.lo)
	h02, p02 := bits.Mul64(a.lo, b.mh)
	h11, p11 := bits.Mul64(a.ml, b.ml)
	h20, p20 := bits.Mul64(a.mh, b.lo)

	r2, c := bits.Add64(carry1, p02, 0)
	r2, c = bits.Add64(r2, p11, c)
	r2, c = bits.Add64(r2, p20, c)
	carry2 := h02 + h11 + h20 + c

	// word 3: only low 64 bits matter (high bits >= 2^256 are discarded)
	_, p03 := bits.Mul64(a.lo, b.hi)
	_, p12 := bits.Mul64(a.ml, b.mh)
	_, p21 := bits.Mul64(a.mh, b.ml)
	_, p30 := bits.Mul64(a.hi, b.lo)
	r3 := carry2 + p03 + p12 + p21 + p30

	return LUint256{lo: r0, ml: r1, mh: r2, hi: r3}
}

func lu256Band(a, b LUint256) LUint256 {
	return LUint256{lo: a.lo & b.lo, ml: a.ml & b.ml, mh: a.mh & b.mh, hi: a.hi & b.hi}
}

func lu256Bor(a, b LUint256) LUint256 {
	return LUint256{lo: a.lo | b.lo, ml: a.ml | b.ml, mh: a.mh | b.mh, hi: a.hi | b.hi}
}

func lu256Bxor(a, b LUint256) LUint256 {
	return LUint256{lo: a.lo ^ b.lo, ml: a.ml ^ b.ml, mh: a.mh ^ b.mh, hi: a.hi ^ b.hi}
}

func lu256Bnot(a LUint256) LUint256 {
	return LUint256{lo: ^a.lo, ml: ^a.ml, mh: ^a.mh, hi: ^a.hi}
}

func lu256ShiftAmount(v LUint256) uint {
	if v.hi != 0 || v.mh != 0 || v.ml != 0 || v.lo >= 256 {
		return 256
	}
	return uint(v.lo)
}

func lu256Shl(a LUint256, n uint) LUint256 {
	if n >= 256 {
		return LUint256{}
	}
	if n == 0 {
		return a
	}
	words := [4]uint64{a.lo, a.ml, a.mh, a.hi}
	var result [4]uint64
	wordShift := n / 64
	bitShift := n % 64
	if bitShift == 0 {
		for i := wordShift; i < 4; i++ {
			result[i] = words[i-wordShift]
		}
	} else {
		for i := wordShift; i < 4; i++ {
			result[i] = words[i-wordShift] << bitShift
			if i > wordShift {
				result[i] |= words[i-wordShift-1] >> (64 - bitShift)
			}
		}
	}
	return LUint256{lo: result[0], ml: result[1], mh: result[2], hi: result[3]}
}

func lu256Shr(a LUint256, n uint) LUint256 {
	if n >= 256 {
		return LUint256{}
	}
	if n == 0 {
		return a
	}
	words := [4]uint64{a.lo, a.ml, a.mh, a.hi}
	var result [4]uint64
	wordShift := n / 64
	bitShift := n % 64
	if bitShift == 0 {
		for i := uint(0); i < 4-wordShift; i++ {
			result[i] = words[i+wordShift]
		}
	} else {
		for i := uint(0); i < 4-wordShift; i++ {
			result[i] = words[i+wordShift] >> bitShift
			if i+wordShift+1 < 4 {
				result[i] |= words[i+wordShift+1] << (64 - bitShift)
			}
		}
	}
	return LUint256{lo: result[0], ml: result[1], mh: result[2], hi: result[3]}
}

// ─── div / mod: native zero-allocation implementation ────────────────────────

func lu256Div(a, b LUint256) LUint256      { q, _ := lu256DivMod(a, b); return q }
func lu256FloorDiv(a, b LUint256) LUint256 { return lu256Div(a, b) }
func lu256Mod(a, b LUint256) LUint256      { _, r := lu256DivMod(a, b); return r }

// lu256DivMod returns (a/b, a%b) using native arithmetic (zero allocation).
func lu256DivMod(a, b LUint256) (quo, rem LUint256) {
	if lu256IsZero(b) {
		panic("u256: division by zero")
	}
	switch lu256Cmp(a, b) {
	case -1:
		return LUint256Zero, a
	case 0:
		return LUint256One, LUint256Zero
	}
	// Fast path: 1-word divisor → 4 × bits.Div64 in sequence.
	if b.hi == 0 && b.mh == 0 && b.ml == 0 {
		q3, r := bits.Div64(0, a.hi, b.lo)
		q2, r := bits.Div64(r, a.mh, b.lo)
		q1, r := bits.Div64(r, a.ml, b.lo)
		q0, r := bits.Div64(r, a.lo, b.lo)
		return LUint256{lo: q0, ml: q1, mh: q2, hi: q3}, LUint256{lo: r}
	}
	// General: Knuth Algorithm D for 2–4 word divisors.
	return u256KnuthD(a, b)
}

// u256word returns the i-th 64-bit word (0 = least significant) of n.
func u256word(n LUint256, i int) uint64 {
	switch i {
	case 0:
		return n.lo
	case 1:
		return n.ml
	case 2:
		return n.mh
	default:
		return n.hi
	}
}

// u256KnuthD implements Knuth TAOCP Vol.2 §4.3.1 Algorithm D for 256-bit
// unsigned division.  Zero allocation; precondition: b has ≥ 2 significant
// words (b.hi != 0 || b.mh != 0 || b.ml != 0).
func u256KnuthD(u, d LUint256) (quo, rem LUint256) {
	// dLen = number of significant 64-bit words in d (2, 3 or 4).
	dLen := 4
	if d.hi == 0 {
		dLen = 3
		if d.mh == 0 {
			dLen = 2
		}
	}

	// Normalize: shift left so MSB of dn[dLen-1] is 1.
	shift := uint(bits.LeadingZeros64(u256word(d, dLen-1)))

	var dn [4]uint64
	if shift == 0 {
		for i := 0; i < dLen; i++ {
			dn[i] = u256word(d, i)
		}
	} else {
		for i := dLen - 1; i > 0; i-- {
			dn[i] = (u256word(d, i) << shift) | (u256word(d, i-1) >> (64 - shift))
		}
		dn[0] = d.lo << shift
	}

	// Normalized dividend (one extra overflow word).
	var un [5]uint64
	if shift == 0 {
		un[4], un[3], un[2], un[1], un[0] = 0, u.hi, u.mh, u.ml, u.lo
	} else {
		un[4] = u.hi >> (64 - shift)
		un[3] = (u.hi << shift) | (u.mh >> (64 - shift))
		un[2] = (u.mh << shift) | (u.ml >> (64 - shift))
		un[1] = (u.ml << shift) | (u.lo >> (64 - shift))
		un[0] = u.lo << shift
	}

	var q [4]uint64
	for j := 4 - dLen; j >= 0; j-- {
		// Estimate quotient digit q̂.
		var qhat, rhat uint64
		if un[j+dLen] >= dn[dLen-1] {
			qhat = ^uint64(0) // max; add-back will correct
			// rhat = 0: holiman-style — refinement loop still converges (≤2 iters).
		} else {
			qhat, rhat = bits.Div64(un[j+dLen], un[j+dLen-1], dn[dLen-1])
		}
		// D3: refine q̂ (runs at most twice per Knuth's theorem).
		for dn[dLen-2] != 0 {
			hi, lo := bits.Mul64(qhat, dn[dLen-2])
			if hi < rhat || (hi == rhat && lo <= un[j+dLen-2]) {
				break
			}
			qhat--
			var carry uint64
			rhat, carry = bits.Add64(rhat, dn[dLen-1], 0)
			if carry != 0 {
				break // rhat overflowed → q̂ is now tight
			}
		}
		// D4: multiply and subtract.
		// borrow is a full 64-bit accumulated value (p_hi + underflow),
		// so we fold it into lo before calling Sub64 (which requires 0/1).
		var borrow uint64
		for i := 0; i < dLen; i++ {
			hi, lo := bits.Mul64(qhat, dn[i])
			var carry uint64
			lo, carry = bits.Add64(lo, borrow, 0) // fold borrow into lo
			hi += carry                             // propagate carry to hi
			var underflow uint64
			un[j+i], underflow = bits.Sub64(un[j+i], lo, 0)
			borrow = hi + underflow
		}
		var overflow uint64
		un[j+dLen], overflow = bits.Sub64(un[j+dLen], borrow, 0)
		// D5: add back if we over-subtracted.
		if overflow != 0 {
			qhat--
			var carry uint64
			for i := 0; i < dLen; i++ {
				un[j+i], carry = bits.Add64(un[j+i], dn[i], carry)
			}
			un[j+dLen], _ = bits.Add64(un[j+dLen], 0, carry)
		}
		q[j] = qhat
	}

	// D8: unnormalize remainder.
	var rw [4]uint64
	if shift == 0 {
		for i := 0; i < dLen; i++ {
			rw[i] = un[i]
		}
	} else {
		for i := 0; i < dLen-1; i++ {
			rw[i] = (un[i] >> shift) | (un[i+1] << (64 - shift))
		}
		rw[dLen-1] = un[dLen-1] >> shift
	}
	return LUint256{lo: q[0], ml: q[1], mh: q[2], hi: q[3]},
		LUint256{lo: rw[0], ml: rw[1], mh: rw[2], hi: rw[3]}
}

func lu256Pow(base, exp LUint256) LUint256 {
	if lu256IsZero(exp) {
		return LUint256One
	}
	result := LUint256One
	for !lu256IsZero(exp) {
		if exp.lo&1 == 1 {
			result = lu256Mul(result, base)
		}
		base = lu256Mul(base, base)
		exp = lu256Shr(exp, 1)
	}
	return result
}

// ─── New signed / mask primitives (zero allocation, no math/big) ─────────────

// lu256MaskN zeros bits at positions [n, 255]. n must be in [0, 256].
func lu256MaskN(a LUint256, n int) LUint256 {
	if n >= 256 {
		return a
	}
	if n <= 0 {
		return LUint256Zero
	}
	w := [4]uint64{a.lo, a.ml, a.mh, a.hi}
	wordIdx := n / 64
	bitInWord := uint(n % 64)
	if bitInWord != 0 {
		w[wordIdx] &= (uint64(1) << bitInWord) - 1
		wordIdx++
	}
	for i := wordIdx; i < 4; i++ {
		w[i] = 0
	}
	return LUint256{lo: w[0], ml: w[1], mh: w[2], hi: w[3]}
}

// lu256BitN reports whether bit n (0-indexed from LSB) is set.
func lu256BitN(a LUint256, n int) bool {
	w := [4]uint64{a.lo, a.ml, a.mh, a.hi}
	return (w[n/64]>>(uint(n%64)))&1 == 1
}

// lu256NegN is two's-complement negation mod 2^n.
func lu256NegN(a LUint256, n int) LUint256 {
	return lu256MaskN(lu256Sub(LUint256Zero, a), n)
}

// lu256SignExtendN fills bits [n, 255] with the value of bit n-1.
func lu256SignExtendN(a LUint256, n int) LUint256 {
	if n >= 256 || !lu256BitN(a, n-1) {
		return a
	}
	allOnes := LUint256{lo: ^uint64(0), ml: ^uint64(0), mh: ^uint64(0), hi: ^uint64(0)}
	high := lu256Bnot(lu256MaskN(allOnes, n)) // 1s in [n..255]
	return lu256Bor(a, high)
}

// lu256SignedCmpN compares a and b as N-bit signed integers (-1, 0, 1).
func lu256SignedCmpN(a, b LUint256, n int) int {
	aNeg := lu256BitN(a, n-1)
	bNeg := lu256BitN(b, n-1)
	if aNeg != bNeg {
		if aNeg {
			return -1
		}
		return 1
	}
	return lu256Cmp(a, b)
}

// lu256SignedDivN performs truncating-toward-zero signed division.
func lu256SignedDivN(a, b LUint256, n int) LUint256 {
	aNeg := lu256BitN(a, n-1)
	bNeg := lu256BitN(b, n-1)
	absA := a
	if aNeg {
		absA = lu256NegN(a, n)
	}
	absB := b
	if bNeg {
		absB = lu256NegN(b, n)
	}
	q, _ := lu256DivMod(absA, absB)
	q = lu256MaskN(q, n)
	if aNeg != bNeg {
		q = lu256NegN(q, n)
	}
	return q
}

// lu256SignedModN performs truncating signed modulo (sign follows dividend).
func lu256SignedModN(a, b LUint256, n int) LUint256 {
	aNeg := lu256BitN(a, n-1)
	bNeg := lu256BitN(b, n-1)
	absA := a
	if aNeg {
		absA = lu256NegN(a, n)
	}
	absB := b
	if bNeg {
		absB = lu256NegN(b, n)
	}
	_, r := lu256DivMod(absA, absB)
	r = lu256MaskN(r, n)
	if aNeg {
		r = lu256NegN(r, n)
	}
	return r
}

// lu256SarN is arithmetic right shift of an N-bit signed value.
func lu256SarN(a LUint256, shift uint, n int) LUint256 {
	if !lu256BitN(a, n-1) {
		return lu256MaskN(lu256Shr(a, shift), n)
	}
	if int(shift) >= n {
		return lu256MaskN(LUint256{lo: ^uint64(0), ml: ^uint64(0), mh: ^uint64(0), hi: ^uint64(0)}, n)
	}
	return lu256MaskN(lu256Shr(lu256SignExtendN(a, n), shift), n)
}

// lu256ToBytes32BE encodes v as a 32-byte big-endian array.
func lu256ToBytes32BE(v LUint256) [32]byte {
	var b [32]byte
	binary.BigEndian.PutUint64(b[0:], v.hi)
	binary.BigEndian.PutUint64(b[8:], v.mh)
	binary.BigEndian.PutUint64(b[16:], v.ml)
	binary.BigEndian.PutUint64(b[24:], v.lo)
	return b
}

// lu256ToBig converts a LUint256 to a *big.Int.
func lu256ToBig(n LUint256) *big.Int {
	b := new(big.Int)
	b.SetBits([]big.Word{big.Word(n.lo), big.Word(n.ml), big.Word(n.mh), big.Word(n.hi)})
	return b
}

// bigToLU256 converts a *big.Int to LUint256 (mod 2^256; panics on negative).
func bigToLU256(b *big.Int) (LUint256, error) {
	if b.Sign() < 0 {
		return LUint256Zero, errNegativeNumber
	}
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
	if len(words) > 4 {
		return LUint256Zero, errNumberOverflow
	}
	return LUint256{lo: lo, ml: ml, mh: mh, hi: hi}, nil
}

// lu256FromHex64 parses a 64-character hex string (no 0x prefix) into LUint256.
func lu256FromHex64(s string) (LUint256, error) {
	if len(s) != 64 {
		return LUint256Zero, fmt.Errorf("expected 64 hex chars, got %d", len(s))
	}
	hi, err := strconv.ParseUint(s[0:16], 16, 64)
	if err != nil {
		return LUint256Zero, err
	}
	mh, err := strconv.ParseUint(s[16:32], 16, 64)
	if err != nil {
		return LUint256Zero, err
	}
	ml, err := strconv.ParseUint(s[32:48], 16, 64)
	if err != nil {
		return LUint256Zero, err
	}
	lo, err := strconv.ParseUint(s[48:64], 16, 64)
	if err != nil {
		return LUint256Zero, err
	}
	return LUint256{lo: lo, ml: ml, mh: mh, hi: hi}, nil
}
