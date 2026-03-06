package lua

import (
	gosha256 "crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/ripemd160"
	"golang.org/x/crypto/sha3"
)

// openCrypto registers deterministic crypto builtins as Lua globals.
// These are required for TOL canonical storage key derivation (spec §8.3).
func openCrypto(L *LState) {
	L.SetGlobal("keccak256", L.NewFunction(cryptoKeccak256))
	L.SetGlobal("sha256", L.NewFunction(cryptoSHA256))
	L.SetGlobal("ripemd160", L.NewFunction(cryptoRIPEMD160))
	L.SetGlobal("gas_left", L.NewFunction(cryptoGasLeft))
	L.SetGlobal("__tol_enc", L.NewFunction(cryptoTolEnc))
	L.SetGlobal("uint256_add_hex", L.NewFunction(cryptoUint256AddHex))
	L.SetGlobal("__tol_abi_decode_params", L.NewFunction(cryptoABIDecodeParams))

	// bytes/string dynamic operation helpers (spec §10, M3).
	L.SetGlobal("__tol_bytes_concat", L.NewFunction(cryptoBytesConcat))
	L.SetGlobal("__tol_str_concat", L.NewFunction(cryptoStrConcat))
	L.SetGlobal("__tol_bytes_length", L.NewFunction(cryptoBytesLength))
	L.SetGlobal("__tol_str_length", L.NewFunction(cryptoStrLength))
	L.SetGlobal("__tol_dyn_length", L.NewFunction(cryptoDynLength))
	L.SetGlobal("__tol_bytes_slice", L.NewFunction(cryptoBytesSlice))

	// Checked unsigned arithmetic helpers (spec §7.1, M5): revert with Panic(0x11) on overflow.
	L.SetGlobal("__tol_uint_checked_add", L.NewFunction(cryptoUintCheckedAdd))
	L.SetGlobal("__tol_uint_checked_sub", L.NewFunction(cryptoUintCheckedSub))
	L.SetGlobal("__tol_uint_checked_mul", L.NewFunction(cryptoUintCheckedMul))

	// Checked signed arithmetic helpers: revert with Panic(0x11) on overflow.
	L.SetGlobal("__tol_signed_checked_add", L.NewFunction(cryptoSignedCheckedAdd))
	L.SetGlobal("__tol_signed_checked_sub", L.NewFunction(cryptoSignedCheckedSub))
	L.SetGlobal("__tol_signed_checked_mul", L.NewFunction(cryptoSignedCheckedMul))

	// bytes/string byte-level index helper (spec §10, M4).
	L.SetGlobal("__tol_bytes_index", L.NewFunction(cryptoBytesIndex))

	// Signed integer arithmetic helpers (spec §7.1, M2).
	// These operate on N-bit two's complement values stored as non-negative big.Int decimals.
	L.SetGlobal("__tol_signed_add", L.NewFunction(cryptoSignedAdd))
	L.SetGlobal("__tol_signed_sub", L.NewFunction(cryptoSignedSub))
	L.SetGlobal("__tol_signed_mul", L.NewFunction(cryptoSignedMul))
	L.SetGlobal("__tol_signed_div", L.NewFunction(cryptoSignedDiv))
	L.SetGlobal("__tol_signed_mod", L.NewFunction(cryptoSignedMod))
	L.SetGlobal("__tol_signed_neg", L.NewFunction(cryptoSignedNeg))
	L.SetGlobal("__tol_signed_lt", L.NewFunction(cryptoSignedLt))
	L.SetGlobal("__tol_signed_gt", L.NewFunction(cryptoSignedGt))
	L.SetGlobal("__tol_signed_le", L.NewFunction(cryptoSignedLe))
	L.SetGlobal("__tol_signed_ge", L.NewFunction(cryptoSignedGe))
	L.SetGlobal("__tol_signed_trunc", L.NewFunction(cryptoSignedTrunc))
	L.SetGlobal("__tol_signed_from_u256", L.NewFunction(cryptoSignedFromU256))
	L.SetGlobal("__tol_signed_to_u256", L.NewFunction(cryptoSignedToU256))
	L.SetGlobal("__tol_signed_sar", L.NewFunction(cryptoSignedSar))
}

// cryptoKeccak256 implements keccak256(hex_input: string) -> bytes32_hex.
// hex_input must be "0x" followed by even hex chars; the raw bytes are hashed.
// Returns "0x" + 64 hex chars.
func cryptoKeccak256(L *LState) int {
	s := strings.TrimSpace(L.CheckString(1))
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		L.RaiseError("keccak256: input must start with 0x, got: %q", s)
	}
	data, err := hex.DecodeString(s[2:])
	if err != nil {
		L.RaiseError("keccak256: invalid hex input: %s", err)
	}
	h := sha3.NewLegacyKeccak256()
	h.Write(data)
	L.Push(LString("0x" + hex.EncodeToString(h.Sum(nil))))
	return 1
}

// cryptoSHA256 implements sha256(hex_input: string) -> bytes32_hex.
// hex_input must be "0x" followed by even hex chars; the raw bytes are hashed.
// Returns "0x" + 64 hex chars.
func cryptoSHA256(L *LState) int {
	s := strings.TrimSpace(L.CheckString(1))
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		L.RaiseError("sha256: input must start with 0x, got: %q", s)
	}
	data, err := hex.DecodeString(s[2:])
	if err != nil {
		L.RaiseError("sha256: invalid hex input: %s", err)
	}
	sum := gosha256.Sum256(data)
	L.Push(LString("0x" + hex.EncodeToString(sum[:])))
	return 1
}

// cryptoRIPEMD160 implements ripemd160(hex_input: string) -> bytes32_hex.
// hex_input must be "0x" followed by even hex chars; the raw bytes are hashed.
// EVM convention: RIPEMD-160 output is left-padded to 32 bytes.
func cryptoRIPEMD160(L *LState) int {
	s := strings.TrimSpace(L.CheckString(1))
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		L.RaiseError("ripemd160: input must start with 0x, got: %q", s)
	}
	data, err := hex.DecodeString(s[2:])
	if err != nil {
		L.RaiseError("ripemd160: invalid hex input: %s", err)
	}
	h := ripemd160.New()
	_, _ = h.Write(data)
	digest := h.Sum(nil) // 20 bytes
	var out [32]byte
	copy(out[32-len(digest):], digest)
	L.Push(LString("0x" + hex.EncodeToString(out[:])))
	return 1
}

// cryptoGasLeft implements gas_left() -> u64_decimal.
// When gas metering is disabled (limit=0), returns "0" in current stage.
func cryptoGasLeft(L *LState) int {
	if L.gasLimit == 0 {
		L.Push(LUint256Zero)
		return 1
	}
	if L.gasUsed >= L.gasLimit {
		L.Push(LUint256Zero)
		return 1
	}
	remaining := L.gasLimit - L.gasUsed
	L.Push(lu256FromUint64(remaining))
	return 1
}

// cryptoTolEnc implements __tol_enc(value) -> 64-char hex string (no 0x, 32 bytes).
// Encodes a TOL key value for canonical mapping slot derivation (spec §8.3):
//   - LAgent / LString "0x...": hex decode, right-align in 32 bytes
//   - LUint256 / LString decimal: big-endian 32-byte u256
//   - LBool: 32 zero bytes with LSB = 1 (true) or 0 (false)
func cryptoTolEnc(L *LState) int {
	v := L.CheckAny(1)
	encoded, err := tolEncodeKey(v)
	if err != nil {
		L.RaiseError("__tol_enc: %s", err)
	}
	L.Push(LString(encoded))
	return 1
}

// cryptoABIDecodeParams implements __tol_abi_decode_params(calldata, type1, type2, ...).
// Decodes ABI-encoded constructor calldata into typed Lua values.
// Each parameter occupies one 32-byte (64 hex char) slot in the calldata.
// Supported types: u8..u256, i8..i256, address, bool, bytes1..bytes32.
// Returns one value per type argument.
func cryptoABIDecodeParams(L *LState) int {
	n := L.GetTop()
	if n < 1 {
		L.RaiseError("__tol_abi_decode_params: calldata argument required")
	}
	calldataStr := strings.TrimSpace(L.CheckString(1))
	if !strings.HasPrefix(calldataStr, "0x") && !strings.HasPrefix(calldataStr, "0X") {
		L.RaiseError("__tol_abi_decode_params: calldata must be 0x-prefixed hex string, got: %q", calldataStr)
	}
	hexData := strings.ToLower(calldataStr[2:])
	if len(hexData)%2 != 0 {
		L.RaiseError("__tol_abi_decode_params: calldata hex must have even length")
	}
	// Validate hex characters
	for i, c := range hexData {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			L.RaiseError("__tol_abi_decode_params: invalid hex character at position %d", i)
		}
	}

	numTypes := n - 1 // types start at arg position 2
	numSlots := len(hexData) / 64
	if numTypes > numSlots && len(hexData) > 0 {
		L.RaiseError("__tol_abi_decode_params: calldata too short: need %d 32-byte slots, got %d", numTypes, numSlots)
	}

	for i := 0; i < numTypes; i++ {
		typ := strings.TrimSpace(L.CheckString(i + 2))
		slotStart := i * 64
		var slotHex string
		if slotStart+64 <= len(hexData) {
			slotHex = hexData[slotStart : slotStart+64]
		} else {
			// Zero-pad if calldata is shorter than expected
			slotHex = strings.Repeat("0", 64)
		}
		v, err := abiDecodeSlot(slotHex, typ)
		if err != nil {
			L.RaiseError("__tol_abi_decode_params: parameter %d type %q: %s", i+1, typ, err.Error())
		}
		L.Push(v)
	}
	return numTypes
}

// abiDecodeSlot decodes a single 32-byte (64 hex char) ABI slot into a Lua value.
func abiDecodeSlot(slotHex, typ string) (LValue, error) {
	if len(slotHex) != 64 {
		return nil, fmt.Errorf("internal: slot must be 64 hex chars, got %d", len(slotHex))
	}
	t := strings.TrimSpace(typ)

	switch t {
	case "bool":
		// Any non-zero byte in the slot → true
		allZero := true
		for _, c := range slotHex {
			if c != '0' {
				allZero = false
				break
			}
		}
		return LBool(!allZero), nil

	case "agent":
		// ABI pads address to 32 bytes (left-padded with zeros); return "0x" + full 64 hex
		return LString("0x" + slotHex), nil

	case "string", "bytes":
		// Dynamic types: only static pointer supported in this stage.
		// Return the raw slot as hex (offset pointer or empty bytes).
		return LString("0x" + slotHex), nil
	}

	// bytes1..bytes32 (right-padded in ABI encoding)
	if strings.HasPrefix(t, "bytes") {
		nStr := t[len("bytes"):]
		if nStr != "" {
			n, err := strconv.Atoi(nStr)
			if err != nil || n < 1 || n > 32 {
				return nil, fmt.Errorf("unsupported bytes type %q", t)
			}
			// bytes<N> is left-aligned (right-padded with zeros) in ABI encoding
			usedHex := slotHex[:n*2]
			return LString("0x" + usedHex), nil
		}
	}

	// u8..u256 and i8..i256
	var prefix byte
	var rest string
	if len(t) >= 2 {
		prefix = t[0]
		rest = t[1:]
	}
	if prefix == 'u' || prefix == 'i' {
		bits, err := strconv.Atoi(rest)
		if err != nil || bits < 8 || bits > 256 || bits%8 != 0 {
			return nil, fmt.Errorf("unsupported integer type %q", t)
		}
		// ABI encoding: right-aligned (left-padded) in 32 bytes.
		// Parse the 64-char hex slot directly.
		_ = bits // sign interpretation is two's-complement; bit pattern stored as-is
		v, err := lu256FromHex64(slotHex)
		if err != nil {
			return nil, err
		}
		return v, nil
	}

	return nil, fmt.Errorf("unsupported type %q in __tol_abi_decode_params", t)
}

// cryptoUint256AddHex implements uint256_add_hex(base_hex, offset) -> bytes32_hex.
// Adds a non-negative integer offset to a hex-encoded u256, wrapping mod 2^256.
// Used for array element slot computation: H(base_slot) + index.
func cryptoUint256AddHex(L *LState) int {
	baseStr := strings.TrimSpace(L.CheckString(1))
	if strings.HasPrefix(baseStr, "0x") || strings.HasPrefix(baseStr, "0X") {
		baseStr = baseStr[2:]
	}
	base, err := parseUint256Base(baseStr, 16)
	if err != nil {
		L.RaiseError("uint256_add_hex: invalid hex base: %q", baseStr)
	}
	var offset LUint256
	switch v := L.CheckAny(2).(type) {
	case LUint256:
		offset = v
	case LString:
		offset, err = parseUint256(string(v))
		if err != nil {
			L.RaiseError("uint256_add_hex: invalid string offset: %q", string(v))
		}
	default:
		L.RaiseError("uint256_add_hex: unsupported offset type %T", v)
	}
	result := lu256Add(base, offset) // wraps mod 2^256 naturally
	b := lu256ToBytes32BE(result)
	L.Push(LString("0x" + hex.EncodeToString(b[:])))
	return 1
}

// tolEncodeKey encodes a Lua value to a 64-char hex string (no 0x prefix) for
// use in TOL canonical storage key derivation per spec §8.3.
func tolEncodeKey(v LValue) (string, error) {
	var buf [32]byte
	switch val := v.(type) {
	case LBool:
		if bool(val) {
			buf[31] = 1
		}
		return hex.EncodeToString(buf[:]), nil
	case LAgent:
		return encodeHexTo32(string(val))
	case LString:
		s := strings.TrimSpace(string(val))
		if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			return encodeHexTo32(s)
		}
		return encodeDecimalTo32(s)
	case LUint256:
		return encodeDecimalTo32(val.String())
	default:
		return "", fmt.Errorf("unsupported key type %T", v)
	}
}

func encodeHexTo32(s string) (string, error) {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("invalid hex value: %s", err)
	}
	if len(b) > 32 {
		return "", fmt.Errorf("hex value exceeds 32 bytes (%d bytes)", len(b))
	}
	var buf [32]byte
	copy(buf[32-len(b):], b)
	return hex.EncodeToString(buf[:]), nil
}

func encodeDecimalTo32(s string) (string, error) {
	v, err := parseUint256(strings.TrimSpace(s))
	if err != nil {
		return "", fmt.Errorf("cannot parse as u256 decimal: %q", s)
	}
	b := lu256ToBytes32BE(v)
	return hex.EncodeToString(b[:]), nil
}

// =============================================================================
// bytes/string dynamic operation helpers (TOL M3, spec §10)
// =============================================================================

// cryptoBytesConcat implements __tol_bytes_concat(a, b, ...) -> bytes string.
// All arguments must be "0x"-prefixed hex strings. Strips the prefix from each,
// concatenates the hex digits, and prepends a single "0x".
func cryptoBytesConcat(L *LState) int {
	n := L.GetTop()
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		s := L.CheckString(i)
		if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			sb.WriteString(s[2:])
		} else {
			sb.WriteString(s)
		}
	}
	L.Push(LString("0x" + sb.String()))
	return 1
}

// cryptoStrConcat implements __tol_str_concat(a, b, ...) -> string.
// Concatenates arbitrary plain strings.
func cryptoStrConcat(L *LState) int {
	n := L.GetTop()
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		sb.WriteString(L.CheckString(i))
	}
	L.Push(LString(sb.String()))
	return 1
}

// cryptoBytesLength implements __tol_bytes_length(b) -> u256 decimal byte count.
// b must be a "0x"-prefixed hex string; returns (#hex_chars / 2) as a decimal LUint256.
func cryptoBytesLength(L *LState) int {
	s := L.CheckString(1)
	hexPart := s
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		hexPart = s[2:]
	}
	byteLen := len(hexPart) / 2
	L.Push(lu256FromInt(byteLen))
	return 1
}

// cryptoStrLength implements __tol_str_length(s) -> u256 decimal character count.
// Returns the number of bytes (characters) in the string as a decimal LUint256.
func cryptoStrLength(L *LState) int {
	s := L.CheckString(1)
	L.Push(lu256FromInt(len(s)))
	return 1
}

// cryptoDynLength implements __tol_dyn_length(v) -> u256 length.
// Handles:
//   - "0x"-prefixed hex bytes: byte count
//   - plain string: character count
//   - Lua table with _size field (memory array): returns _size
func cryptoDynLength(L *LState) int {
	v := L.Get(1)
	switch val := v.(type) {
	case LString:
		s := string(val)
		if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			byteLen := (len(s) - 2) / 2
			L.Push(lu256FromInt(byteLen))
		} else {
			L.Push(lu256FromInt(len(s)))
		}
		return 1
	case *LTable:
		// Memory array: _size field holds the fixed capacity.
		sizeVal := val.RawGetString("_size")
		if u, ok := sizeVal.(LUint256); ok {
			L.Push(u)
			return 1
		}
		L.Push(LUint256{}) // zero
		return 1
	default:
		L.ArgError(1, "bytes, string, or memory array expected")
		return 0
	}
}

// cryptoBytesSlice implements __tol_bytes_slice(b, start, end_) -> bytes string.
// Extracts bytes [start, end_) (0-indexed, exclusive end) from a "0x"-prefixed hex string.
// start and end_ are decimal LUint256 values.
func cryptoBytesSlice(L *LState) int {
	s := L.CheckString(1)
	startN := L.CheckUint256(2)
	endN := L.CheckUint256(3)

	hexPart := s
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		hexPart = s[2:]
	}

	byteLen := int64(len(hexPart) / 2)
	si := int64(startN.lo)
	ei := int64(endN.lo)

	if si < 0 {
		si = 0
	}
	if ei > byteLen {
		ei = byteLen
	}
	if si > ei {
		si = ei
	}

	// Each byte is 2 hex chars; byte i occupies hexPart[2*i : 2*i+2].
	hexStart := si * 2
	hexEnd := ei * 2
	slice := hexPart[hexStart:hexEnd]
	L.Push(LString("0x" + slice))
	return 1
}

// =============================================================================
// Signed integer arithmetic helpers (TOL M2, spec §7.1)
// =============================================================================
//
// All signed integer values in TOL are represented as two's-complement LUint256
// values. A value v represents a negative N-bit number when bit (N-1) is set.
// Operations are implemented using native uint256 primitives (zero allocation).

// signedBitWidth validates and returns the requested integer bit width.
func signedBitWidth(L *LState, argPos int) int {
	nv := L.CheckUint256(argPos)
	n, ok := lu256ToInt(nv)
	if !ok || n < 8 || n > 256 || n%8 != 0 {
		L.RaiseError("__tol_signed: invalid bit width %v (must be 8..256, multiple of 8)", nv)
	}
	return n
}

// cryptoSignedAdd implements __tol_signed_add(a, b, bits) -> signed add mod 2^bits.
// Two's-complement add/sub/mul are identical to unsigned; just mask to N bits.
func cryptoSignedAdd(L *LState) int {
	a := L.CheckUint256(1)
	b := L.CheckUint256(2)
	n := signedBitWidth(L, 3)
	L.Push(lu256MaskN(lu256Add(a, b), n))
	return 1
}

// cryptoSignedSub implements __tol_signed_sub(a, b, bits) -> signed sub mod 2^bits.
func cryptoSignedSub(L *LState) int {
	a := L.CheckUint256(1)
	b := L.CheckUint256(2)
	n := signedBitWidth(L, 3)
	L.Push(lu256MaskN(lu256Sub(a, b), n))
	return 1
}

// cryptoSignedMul implements __tol_signed_mul(a, b, bits) -> signed mul mod 2^bits.
func cryptoSignedMul(L *LState) int {
	a := L.CheckUint256(1)
	b := L.CheckUint256(2)
	n := signedBitWidth(L, 3)
	L.Push(lu256MaskN(lu256Mul(a, b), n))
	return 1
}

// cryptoSignedDiv implements __tol_signed_div(a, b, bits) -> signed division
// truncating toward zero. Reverts on division by zero and min_int / -1.
func cryptoSignedDiv(L *LState) int {
	a := L.CheckUint256(1)
	b := L.CheckUint256(2)
	n := signedBitWidth(L, 3)
	if lu256IsZero(lu256MaskN(b, n)) {
		L.RaiseError("signed integer division by zero")
	}
	// Check for min_signed / -1 overflow (spec §7.1).
	// min_signed for N bits = 1 << (N-1) in two's complement representation.
	minSigned := lu256MaskN(LUint256Zero, n)
	minSigned = lu256Bor(minSigned, lu256Shl(LUint256One, uint(n-1)))
	// -1 in N-bit two's complement = all ones in N bits = MaskN(allOnes, N)
	allOnes := LUint256{lo: ^uint64(0), ml: ^uint64(0), mh: ^uint64(0), hi: ^uint64(0)}
	negOne := lu256MaskN(allOnes, n)
	if lu256MaskN(a, n) == minSigned && lu256MaskN(b, n) == negOne {
		L.RaiseError("signed integer overflow: min_int / -1")
	}
	L.Push(lu256SignedDivN(a, b, n))
	return 1
}

// cryptoSignedMod implements __tol_signed_mod(a, b, bits) -> signed remainder
// where the result has the same sign as the dividend. Reverts on mod by zero.
func cryptoSignedMod(L *LState) int {
	a := L.CheckUint256(1)
	b := L.CheckUint256(2)
	n := signedBitWidth(L, 3)
	if lu256IsZero(lu256MaskN(b, n)) {
		L.RaiseError("signed integer modulo by zero")
	}
	L.Push(lu256SignedModN(a, b, n))
	return 1
}

// cryptoSignedNeg implements __tol_signed_neg(a, bits) -> -a mod 2^bits.
func cryptoSignedNeg(L *LState) int {
	a := L.CheckUint256(1)
	n := signedBitWidth(L, 2)
	L.Push(lu256NegN(a, n))
	return 1
}

// cryptoSignedLt implements __tol_signed_lt(a, b, bits) -> bool (signed a < b).
func cryptoSignedLt(L *LState) int {
	a := L.CheckUint256(1)
	b := L.CheckUint256(2)
	n := signedBitWidth(L, 3)
	L.Push(LBool(lu256SignedCmpN(a, b, n) < 0))
	return 1
}

// cryptoSignedGt implements __tol_signed_gt(a, b, bits) -> bool (signed a > b).
func cryptoSignedGt(L *LState) int {
	a := L.CheckUint256(1)
	b := L.CheckUint256(2)
	n := signedBitWidth(L, 3)
	L.Push(LBool(lu256SignedCmpN(a, b, n) > 0))
	return 1
}

// cryptoSignedLe implements __tol_signed_le(a, b, bits) -> bool (signed a <= b).
func cryptoSignedLe(L *LState) int {
	a := L.CheckUint256(1)
	b := L.CheckUint256(2)
	n := signedBitWidth(L, 3)
	L.Push(LBool(lu256SignedCmpN(a, b, n) <= 0))
	return 1
}

// cryptoSignedGe implements __tol_signed_ge(a, b, bits) -> bool (signed a >= b).
func cryptoSignedGe(L *LState) int {
	a := L.CheckUint256(1)
	b := L.CheckUint256(2)
	n := signedBitWidth(L, 3)
	L.Push(LBool(lu256SignedCmpN(a, b, n) >= 0))
	return 1
}

// cryptoSignedTrunc implements __tol_signed_trunc(a, bits) -> a truncated to N bits.
// Used for type casts like i8(expr): take the low N bits.
func cryptoSignedTrunc(L *LState) int {
	a := L.CheckUint256(1)
	n := signedBitWidth(L, 2)
	L.Push(lu256MaskN(a, n))
	return 1
}

// cryptoSignedFromU256 implements __tol_signed_from_u256(a, bits) -> reinterpret
// a u256 value as a signed N-bit integer (truncate to N bits).
func cryptoSignedFromU256(L *LState) int {
	a := L.CheckUint256(1)
	n := signedBitWidth(L, 2)
	L.Push(lu256MaskN(a, n))
	return 1
}

// cryptoSignedToU256 implements __tol_signed_to_u256(a, bits) -> sign-extend
// N-bit signed value to 256 bits.
func cryptoSignedToU256(L *LState) int {
	a := L.CheckUint256(1)
	n := signedBitWidth(L, 2)
	L.Push(lu256SignExtendN(a, n))
	return 1
}

// =============================================================================
// Checked arithmetic helpers (TOL M5, spec §7.1)
// These revert with Panic(0x11) on overflow, implementing Solidity 0.8 semantics.
// Panic code 0x11 = arithmetic overflow/underflow.
// =============================================================================

// panicOverflow raises a Lua error encoding Panic(0x11) — arithmetic overflow/underflow.
func panicOverflow(L *LState) {
	L.RaiseError("Panic(0x11)")
}

// lu256Bit returns the value of bit position pos (0-indexed from LSB) in v.
func lu256Bit(v LUint256, pos uint) bool {
	switch {
	case pos < 64:
		return (v.lo>>pos)&1 == 1
	case pos < 128:
		return (v.ml>>(pos-64))&1 == 1
	case pos < 192:
		return (v.mh>>(pos-128))&1 == 1
	default:
		return (v.hi>>(pos-192))&1 == 1
	}
}

// cryptoUintCheckedAdd implements __tol_uint_checked_add(a, b, bits) -> a+b or Panic(0x11).
// bits=256 means u256 (overflow when result < a due to wrap).
// bits<256 means uN: overflow when unmasked result != masked result.
func cryptoUintCheckedAdd(L *LState) int {
	a := L.CheckUint256(1)
	b := L.CheckUint256(2)
	n := int(L.CheckUint256(3).lo)
	result := lu256Add(a, b)
	if n == 256 {
		// u256: overflow when result < a (carry wrapped)
		if lu256Cmp(result, a) < 0 {
			panicOverflow(L)
		}
	} else {
		// uN (N < 256): overflow when upper bits are set after addition
		masked := lu256MaskN(result, n)
		if masked != result {
			panicOverflow(L)
		}
	}
	L.Push(result)
	return 1
}

// cryptoUintCheckedSub implements __tol_uint_checked_sub(a, b, bits) -> a-b or Panic(0x11).
// Underflow when b > a (for the N-bit masked values).
func cryptoUintCheckedSub(L *LState) int {
	a := L.CheckUint256(1)
	b := L.CheckUint256(2)
	n := int(L.CheckUint256(3).lo)
	aMasked := a
	bMasked := b
	if n < 256 {
		aMasked = lu256MaskN(a, n)
		bMasked = lu256MaskN(b, n)
	}
	if lu256Cmp(bMasked, aMasked) > 0 {
		panicOverflow(L)
	}
	L.Push(lu256Sub(a, b))
	return 1
}

// cryptoUintCheckedMul implements __tol_uint_checked_mul(a, b, bits) -> a*b or Panic(0x11).
// For uN (N<256): overflow when unmasked product != masked product.
// For u256: overflow when a != 0 and product/a != b.
func cryptoUintCheckedMul(L *LState) int {
	a := L.CheckUint256(1)
	b := L.CheckUint256(2)
	n := int(L.CheckUint256(3).lo)
	result := lu256Mul(a, b)
	if n < 256 {
		masked := lu256MaskN(result, n)
		if masked != result {
			panicOverflow(L)
		}
	} else {
		// u256 mul: overflow detection via division
		if !lu256IsZero(a) {
			q, _ := lu256DivMod(result, a)
			if q != b {
				panicOverflow(L)
			}
		}
	}
	L.Push(result)
	return 1
}

// cryptoSignedCheckedAdd implements __tol_signed_checked_add(a, b, bits).
// Overflow when both operands have the same sign but the result has the opposite sign.
func cryptoSignedCheckedAdd(L *LState) int {
	a := L.CheckUint256(1)
	b := L.CheckUint256(2)
	n := signedBitWidth(L, 3)
	result := lu256MaskN(lu256Add(a, b), n)
	signBit := uint(n - 1)
	aSign := lu256Bit(a, signBit)
	bSign := lu256Bit(b, signBit)
	rSign := lu256Bit(result, signBit)
	if aSign == bSign && rSign != aSign {
		panicOverflow(L)
	}
	L.Push(result)
	return 1
}

// cryptoSignedCheckedSub implements __tol_signed_checked_sub(a, b, bits).
// Overflow when a and b have different signs and the result has a different sign than a.
func cryptoSignedCheckedSub(L *LState) int {
	a := L.CheckUint256(1)
	b := L.CheckUint256(2)
	n := signedBitWidth(L, 3)
	result := lu256MaskN(lu256Sub(a, b), n)
	signBit := uint(n - 1)
	aSign := lu256Bit(a, signBit)
	bSign := lu256Bit(b, signBit)
	rSign := lu256Bit(result, signBit)
	if aSign != bSign && rSign != aSign {
		panicOverflow(L)
	}
	L.Push(result)
	return 1
}

// cryptoSignedCheckedMul implements __tol_signed_checked_mul(a, b, bits).
// Overflow detection: wrapping product masked back must equal original product via signed division.
func cryptoSignedCheckedMul(L *LState) int {
	a := L.CheckUint256(1)
	b := L.CheckUint256(2)
	n := signedBitWidth(L, 3)
	result := lu256MaskN(lu256Mul(a, b), n)
	aMasked := lu256MaskN(a, n)
	if !lu256IsZero(aMasked) {
		q := lu256SignedDivN(result, aMasked, n)
		if lu256MaskN(q, n) != lu256MaskN(b, n) {
			panicOverflow(L)
		}
	}
	L.Push(result)
	return 1
}

// cryptoBytesIndex implements __tol_bytes_index(b, i) -> bytes1.
// Extracts byte at 0-indexed position i from a "0x"-prefixed hex string.
// Returns a "0x"-prefixed 2-hex-char bytes1 value.
func cryptoBytesIndex(L *LState) int {
	s := L.CheckString(1)
	idxV := L.CheckUint256(2)

	hexPart := s
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		hexPart = s[2:]
	}
	byteLen := len(hexPart) / 2
	idx := int(idxV.lo)
	if idx < 0 || idx >= byteLen {
		L.RaiseError("bytes index %d out of range [0, %d)", idx, byteLen)
	}
	b := hexPart[idx*2 : idx*2+2]
	L.Push(LString("0x" + b))
	return 1
}

// cryptoSignedSar implements __tol_signed_sar(a, shift, bits) -> arithmetic right shift.
// For an N-bit signed integer a, shift it right by `shift` positions with sign extension.
// This is the Solidity SAR semantics for signed types: sign bit is preserved.
func cryptoSignedSar(L *LState) int {
	a := L.CheckUint256(1)
	shiftV := L.CheckUint256(2)
	n := signedBitWidth(L, 3)
	shift := lu256ShiftAmount(shiftV)
	L.Push(lu256SarN(a, shift, n))
	return 1
}
