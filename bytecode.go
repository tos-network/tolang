package lua

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/tos-network/tolang/parse"
)

var bytecodeMagic = [4]byte{'T', 'O', 'L', 'B'}

// BytecodeFormatVersion is the binary format version for tolang bytecode.
const BytecodeFormatVersion uint16 = 1

const (
	bcConstNil uint8 = iota
	bcConstBool
	bcConstNumber
	bcConstString
	bcConstAgent
)

func bytecodeVMID() string {
	return fmt.Sprintf("pkg=%s-%s;lua=%s;numbit=%d;opmax=%d",
		PackageName, PackageVersion, LuaVersion, LUint256Bit, opCodeMax)
}

// IsBytecode reports whether the input starts with tolang bytecode magic bytes.
func IsBytecode(data []byte) bool {
	if len(data) < len(bytecodeMagic) {
		return false
	}
	for i := range bytecodeMagic {
		if data[i] != bytecodeMagic[i] {
			return false
		}
	}
	return true
}

// EncodeFunctionProto serializes an executable function prototype into a deterministic bytecode blob.
func EncodeFunctionProto(proto *FunctionProto) ([]byte, error) {
	if proto == nil {
		return nil, fmt.Errorf("nil function proto")
	}
	var payload bytes.Buffer
	if err := writeProto(&payload, proto); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(payload.Bytes())

	var buf bytes.Buffer
	buf.Write(bytecodeMagic[:])
	if err := writeU16(&buf, BytecodeFormatVersion); err != nil {
		return nil, err
	}
	if err := writeString(&buf, bytecodeVMID()); err != nil {
		return nil, err
	}
	if err := writeU32(&buf, uint32(payload.Len())); err != nil {
		return nil, err
	}
	if _, err := buf.Write(payload.Bytes()); err != nil {
		return nil, err
	}
	if _, err := buf.Write(sum[:]); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecodeFunctionProto deserializes a bytecode blob into an executable function prototype.
func DecodeFunctionProto(data []byte) (*FunctionProto, error) {
	return decodeFunctionProtoV2(data)
}

func decodeFunctionProtoV2(data []byte) (*FunctionProto, error) {
	r := &byteReader{b: data}
	var magic [4]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return nil, fmt.Errorf("invalid bytecode header: %w", err)
	}
	if magic != bytecodeMagic {
		return nil, fmt.Errorf("invalid bytecode magic")
	}
	version, err := readU16(r)
	if err != nil {
		return nil, fmt.Errorf("invalid bytecode version: %w", err)
	}
	if version != BytecodeFormatVersion {
		return nil, fmt.Errorf("unsupported bytecode version: got=%d want=%d", version, BytecodeFormatVersion)
	}
	vmID, err := readString(r)
	if err != nil {
		return nil, fmt.Errorf("invalid bytecode vm id: %w", err)
	}
	wantVMID := bytecodeVMID()
	if vmID != wantVMID {
		return nil, fmt.Errorf("bytecode vm mismatch: got=%q want=%q", vmID, wantVMID)
	}
	payloadLen, err := readU32(r)
	if err != nil {
		return nil, fmt.Errorf("invalid bytecode payload length: %w", err)
	}
	remaining := len(data) - r.n
	if payloadLen > uint32(remaining) || remaining-int(payloadLen) < sha256.Size {
		return nil, fmt.Errorf("invalid bytecode payload size")
	}
	payload := data[r.n : r.n+int(payloadLen)]
	r.n += int(payloadLen)
	want := data[r.n : r.n+sha256.Size]
	r.n += sha256.Size
	if r.n != len(data) {
		return nil, fmt.Errorf("trailing bytes in bytecode")
	}
	got := sha256.Sum256(payload)
	if !bytes.Equal(got[:], want) {
		return nil, fmt.Errorf("bytecode checksum mismatch")
	}
	pr := &byteReader{b: payload}
	proto, err := readProto(pr)
	if err != nil {
		return nil, err
	}
	if pr.n != len(payload) {
		return nil, fmt.Errorf("trailing bytes in bytecode payload")
	}
	hydrateStringConstants(proto)
	if err := validateDecodedProto(proto); err != nil {
		return nil, err
	}
	return proto, nil
}

// CompileSourceToBytecode compiles Lua source into deterministic bytecode.
func CompileSourceToBytecode(source []byte, name string) ([]byte, error) {
	source = stripShebang(source)
	chunk, err := parse.Parse(bytes.NewReader(source), name)
	if err != nil {
		return nil, err
	}
	program, err := buildIRFromChunk(chunk, name)
	if err != nil {
		return nil, err
	}
	proto, err := CompileIR(program)
	if err != nil {
		return nil, err
	}
	return EncodeFunctionProto(proto)
}

func stripShebang(src []byte) []byte {
	if len(src) == 0 || src[0] != '#' {
		return src
	}
	if i := bytes.IndexByte(src, '\n'); i >= 0 {
		return src[i+1:]
	}
	return []byte{}
}

func writeProto(w io.Writer, p *FunctionProto) error {
	if err := writeString(w, p.SourceName); err != nil {
		return err
	}
	if err := writeI32(w, int32(p.LineDefined)); err != nil {
		return err
	}
	if err := writeI32(w, int32(p.LastLineDefined)); err != nil {
		return err
	}
	if err := writeU8(w, p.NumUpvalues); err != nil {
		return err
	}
	if err := writeU8(w, p.NumParameters); err != nil {
		return err
	}
	if err := writeU8(w, p.IsVarArg); err != nil {
		return err
	}
	if err := writeU8(w, p.NumUsedRegisters); err != nil {
		return err
	}

	if err := writeU32(w, uint32(len(p.Code))); err != nil {
		return err
	}
	for _, inst := range p.Code {
		if err := writeU32(w, inst); err != nil {
			return err
		}
	}

	if err := writeU32(w, uint32(len(p.Constants))); err != nil {
		return err
	}
	for _, c := range p.Constants {
		if err := writeConst(w, c); err != nil {
			return err
		}
	}

	if err := writeU32(w, uint32(len(p.FunctionPrototypes))); err != nil {
		return err
	}
	for _, child := range p.FunctionPrototypes {
		if err := writeProto(w, child); err != nil {
			return err
		}
	}

	if err := writeU32(w, uint32(len(p.DbgSourcePositions))); err != nil {
		return err
	}
	for _, n := range p.DbgSourcePositions {
		if err := writeI32(w, int32(n)); err != nil {
			return err
		}
	}

	if err := writeU32(w, uint32(len(p.DbgLocals))); err != nil {
		return err
	}
	for _, li := range p.DbgLocals {
		if li == nil {
			if err := writeU8(w, 0); err != nil {
				return err
			}
			continue
		}
		if err := writeU8(w, 1); err != nil {
			return err
		}
		if err := writeString(w, li.Name); err != nil {
			return err
		}
		if err := writeI32(w, int32(li.Reg)); err != nil {
			return err
		}
		if err := writeU8(w, li.Attr); err != nil {
			return err
		}
		if err := writeI32(w, int32(li.StartPc)); err != nil {
			return err
		}
		if err := writeI32(w, int32(li.EndPc)); err != nil {
			return err
		}
	}

	if err := writeU32(w, uint32(len(p.DbgCalls))); err != nil {
		return err
	}
	for _, dc := range p.DbgCalls {
		if err := writeString(w, dc.Name); err != nil {
			return err
		}
		if err := writeI32(w, int32(dc.Pc)); err != nil {
			return err
		}
	}

	if err := writeU32(w, uint32(len(p.DbgUpvalues))); err != nil {
		return err
	}
	for _, name := range p.DbgUpvalues {
		if err := writeString(w, name); err != nil {
			return err
		}
	}
	return nil
}

func readProto(r *byteReader) (*FunctionProto, error) {
	source, err := readString(r)
	if err != nil {
		return nil, err
	}
	lineDefined, err := readI32(r)
	if err != nil {
		return nil, err
	}
	lastLineDefined, err := readI32(r)
	if err != nil {
		return nil, err
	}
	nUp, err := readU8(r)
	if err != nil {
		return nil, err
	}
	nParam, err := readU8(r)
	if err != nil {
		return nil, err
	}
	isVarArg, err := readU8(r)
	if err != nil {
		return nil, err
	}
	nReg, err := readU8(r)
	if err != nil {
		return nil, err
	}

	codeLen, err := readU32(r)
	if err != nil {
		return nil, err
	}
	code := make([]uint32, codeLen)
	for i := range code {
		v, err := readU32(r)
		if err != nil {
			return nil, err
		}
		code[i] = v
	}

	constLen, err := readU32(r)
	if err != nil {
		return nil, err
	}
	consts := make([]LValue, constLen)
	for i := range consts {
		v, err := readConst(r)
		if err != nil {
			return nil, err
		}
		consts[i] = v
	}

	childLen, err := readU32(r)
	if err != nil {
		return nil, err
	}
	children := make([]*FunctionProto, childLen)
	for i := range children {
		cp, err := readProto(r)
		if err != nil {
			return nil, err
		}
		children[i] = cp
	}

	dbgPosLen, err := readU32(r)
	if err != nil {
		return nil, err
	}
	dbgPos := make([]int, dbgPosLen)
	for i := range dbgPos {
		n, err := readI32(r)
		if err != nil {
			return nil, err
		}
		dbgPos[i] = int(n)
	}

	dbgLocalLen, err := readU32(r)
	if err != nil {
		return nil, err
	}
	dbgLocals := make([]*DbgLocalInfo, dbgLocalLen)
	for i := range dbgLocals {
		present, err := readU8(r)
		if err != nil {
			return nil, err
		}
		if present == 0 {
			continue
		}
		name, err := readString(r)
		if err != nil {
			return nil, err
		}
		reg, err := readI32(r)
		if err != nil {
			return nil, err
		}
		attr, err := readU8(r)
		if err != nil {
			return nil, err
		}
		startPc, err := readI32(r)
		if err != nil {
			return nil, err
		}
		endPc, err := readI32(r)
		if err != nil {
			return nil, err
		}
		dbgLocals[i] = &DbgLocalInfo{Name: name, Reg: int(reg), Attr: attr, StartPc: int(startPc), EndPc: int(endPc)}
	}

	dbgCallLen, err := readU32(r)
	if err != nil {
		return nil, err
	}
	dbgCalls := make([]DbgCall, dbgCallLen)
	for i := range dbgCalls {
		name, err := readString(r)
		if err != nil {
			return nil, err
		}
		pc, err := readI32(r)
		if err != nil {
			return nil, err
		}
		dbgCalls[i] = DbgCall{Name: name, Pc: int(pc)}
	}

	dbgUpLen, err := readU32(r)
	if err != nil {
		return nil, err
	}
	dbgUp := make([]string, dbgUpLen)
	for i := range dbgUp {
		s, err := readString(r)
		if err != nil {
			return nil, err
		}
		dbgUp[i] = s
	}

	p := &FunctionProto{
		SourceName:         source,
		LineDefined:        int(lineDefined),
		LastLineDefined:    int(lastLineDefined),
		NumUpvalues:        nUp,
		NumParameters:      nParam,
		IsVarArg:           isVarArg,
		NumUsedRegisters:   nReg,
		Code:               code,
		Constants:          consts,
		FunctionPrototypes: children,
		DbgSourcePositions: dbgPos,
		DbgLocals:          dbgLocals,
		DbgCalls:           dbgCalls,
		DbgUpvalues:        dbgUp,
	}
	return p, nil
}

func validateDecodedProto(p *FunctionProto) error {
	for i := 0; i < len(p.Code); i++ {
		inst := p.Code[i]
		op := opGetOpCode(inst)
		if op < 0 || op > opCodeMax {
			return fmt.Errorf("invalid opcode %d at pc %d", op, i)
		}
		if op == OP_CLOSURE {
			bx := opGetArgBx(inst)
			if bx < 0 || bx >= len(p.FunctionPrototypes) {
				return fmt.Errorf("invalid closure prototype index %d at pc %d", bx, i)
			}
		}
		if op == OP_SETLIST && opGetArgC(inst) == 0 {
			if i+1 >= len(p.Code) {
				return fmt.Errorf("missing SETLIST extra word at pc %d", i)
			}
			i++
		}
	}
	for _, child := range p.FunctionPrototypes {
		if err := validateDecodedProto(child); err != nil {
			return err
		}
	}
	return nil
}

func writeConst(w io.Writer, v LValue) error {
	switch lv := v.(type) {
	case *LNilType:
		return writeU8(w, bcConstNil)
	case LBool:
		if err := writeU8(w, bcConstBool); err != nil {
			return err
		}
		if lv {
			return writeU8(w, 1)
		}
		return writeU8(w, 0)
	case LUint256:
		if err := writeU8(w, bcConstNumber); err != nil {
			return err
		}
		var buf [32]byte
		binary.LittleEndian.PutUint64(buf[0:], lv.lo)
		binary.LittleEndian.PutUint64(buf[8:], lv.ml)
		binary.LittleEndian.PutUint64(buf[16:], lv.mh)
		binary.LittleEndian.PutUint64(buf[24:], lv.hi)
		_, err := w.Write(buf[:])
		return err
	case LString:
		if err := writeU8(w, bcConstString); err != nil {
			return err
		}
		return writeString(w, string(lv))
	case LAgent:
		if err := writeU8(w, bcConstAgent); err != nil {
			return err
		}
		return writeString(w, string(lv))
	default:
		if v == LNil {
			return writeU8(w, bcConstNil)
		}
		return fmt.Errorf("unsupported constant type: %T", v)
	}
}

func readConst(r *byteReader) (LValue, error) {
	tag, err := readU8(r)
	if err != nil {
		return nil, err
	}
	switch tag {
	case bcConstNil:
		return LNil, nil
	case bcConstBool:
		b, err := readU8(r)
		if err != nil {
			return nil, err
		}
		return LBool(b != 0), nil
	case bcConstNumber:
		var buf [32]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return nil, fmt.Errorf("truncated number constant: %w", err)
		}
		return LUint256{
			lo: binary.LittleEndian.Uint64(buf[0:]),
			ml: binary.LittleEndian.Uint64(buf[8:]),
			mh: binary.LittleEndian.Uint64(buf[16:]),
			hi: binary.LittleEndian.Uint64(buf[24:]),
		}, nil
	case bcConstString:
		s, err := readString(r)
		if err != nil {
			return nil, err
		}
		return LString(s), nil
	case bcConstAgent:
		s, err := readString(r)
		if err != nil {
			return nil, err
		}
		addr, err := parseAgentString(s)
		if err != nil {
			return nil, err
		}
		return addr, nil
	default:
		return nil, fmt.Errorf("unknown constant tag: %d", tag)
	}
}

type byteReader struct {
	b []byte
	n int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.n >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.n:])
	r.n += n
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func writeU8(w io.Writer, v uint8) error {
	_, err := w.Write([]byte{v})
	return err
}

func writeU16(w io.Writer, v uint16) error {
	return binary.Write(w, binary.BigEndian, v)
}

func writeU32(w io.Writer, v uint32) error {
	return binary.Write(w, binary.BigEndian, v)
}

func writeI32(w io.Writer, v int32) error {
	return binary.Write(w, binary.BigEndian, v)
}

func writeString(w io.Writer, s string) error {
	if err := writeU32(w, uint32(len(s))); err != nil {
		return err
	}
	_, err := io.WriteString(w, s)
	return err
}

func readU8(r *byteReader) (uint8, error) {
	var b [1]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	return b[0], nil
}

func readU16(r *byteReader) (uint16, error) {
	var v uint16
	if err := binary.Read(r, binary.BigEndian, &v); err != nil {
		return 0, err
	}
	return v, nil
}

func readU32(r *byteReader) (uint32, error) {
	var v uint32
	if err := binary.Read(r, binary.BigEndian, &v); err != nil {
		return 0, err
	}
	return v, nil
}

func readI32(r *byteReader) (int32, error) {
	var v int32
	if err := binary.Read(r, binary.BigEndian, &v); err != nil {
		return 0, err
	}
	return v, nil
}

func readString(r *byteReader) (string, error) {
	n, err := readU32(r)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	if n > uint32(len(r.b)-r.n) {
		return "", io.ErrUnexpectedEOF
	}
	start := r.n
	r.n += int(n)
	return string(r.b[start:r.n]), nil
}
