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
	if p == nil {
		return fmt.Errorf("nil function proto")
	}
	if len(p.stringConstants) != len(p.Constants) {
		return fmt.Errorf("invalid string constant cache")
	}
	if int(p.NumParameters) > int(p.NumUsedRegisters) {
		return fmt.Errorf("invalid function proto: num parameters %d exceeds registers %d", p.NumParameters, p.NumUsedRegisters)
	}
	for i, li := range p.DbgLocals {
		if li == nil {
			continue
		}
		if li.Reg < 0 || li.Reg >= int(p.NumUsedRegisters) {
			return fmt.Errorf("invalid debug local register %d at index %d", li.Reg, i)
		}
	}
	for i := 0; i < len(p.Code); i++ {
		extraConsumed, err := validateDecodedInstruction(p, i)
		if err != nil {
			return err
		}
		i += extraConsumed
	}
	for i, child := range p.FunctionPrototypes {
		if child == nil {
			return fmt.Errorf("nil child function proto at index %d", i)
		}
		if err := validateDecodedProto(child); err != nil {
			return err
		}
	}
	return nil
}

func validateDecodedInstruction(p *FunctionProto, pc int) (int, error) {
	inst := p.Code[pc]
	op := opGetOpCode(inst)
	if op < 0 || op > opCodeMax {
		return 0, fmt.Errorf("invalid opcode %d at pc %d", op, pc)
	}

	numRegs := int(p.NumUsedRegisters)
	numConsts := len(p.Constants)
	numUpvalues := int(p.NumUpvalues)
	codeLen := len(p.Code)

	validateReg := func(reg int, what string) error {
		if reg < 0 || reg >= numRegs {
			return fmt.Errorf("invalid %s register %d at pc %d", what, reg, pc)
		}
		return nil
	}
	validateRegRange := func(start, end int, what string) error {
		if start < 0 || end < start || end >= numRegs {
			return fmt.Errorf("invalid %s register range %d..%d at pc %d", what, start, end, pc)
		}
		return nil
	}
	validateConst := func(idx int, what string) error {
		if idx < 0 || idx >= numConsts {
			return fmt.Errorf("invalid %s constant index %d at pc %d", what, idx, pc)
		}
		return nil
	}
	validateStringConst := func(idx int, what string) error {
		if err := validateConst(idx, what); err != nil {
			return err
		}
		if _, ok := p.Constants[idx].(LString); !ok {
			return fmt.Errorf("invalid %s string constant index %d at pc %d", what, idx, pc)
		}
		return nil
	}
	validateUpvalue := func(idx int, what string) error {
		if idx < 0 || idx >= numUpvalues {
			return fmt.Errorf("invalid %s upvalue index %d at pc %d", what, idx, pc)
		}
		return nil
	}
	validateRK := func(arg int, what string) error {
		if opIsK(arg) {
			return validateConst(opIndexK(arg), what)
		}
		return validateReg(arg, what)
	}
	validateRKString := func(arg int, what string) error {
		if !opIsK(arg) {
			return fmt.Errorf("%s must use a constant string at pc %d", what, pc)
		}
		return validateStringConst(opIndexK(arg), what)
	}
	validateHasNext := func(what string) error {
		if pc+1 >= codeLen {
			return fmt.Errorf("%s at pc %d requires a following instruction", what, pc)
		}
		return nil
	}
	validateJumpTarget := func(fromPC, sbx int, what string) error {
		target := fromPC + 1 + sbx
		if target < 0 || target >= codeLen {
			return fmt.Errorf("invalid %s jump target %d at pc %d", what, target, fromPC)
		}
		return nil
	}

	A := opGetArgA(inst)
	B := opGetArgB(inst)
	C := opGetArgC(inst)
	Bx := opGetArgBx(inst)
	Sbx := opGetArgSbx(inst)

	switch op {
	case OP_MOVE:
		if err := validateReg(A, "MOVE destination"); err != nil {
			return 0, err
		}
		if err := validateReg(B, "MOVE source"); err != nil {
			return 0, err
		}
	case OP_MOVEN:
		if err := validateReg(A, "MOVEN destination"); err != nil {
			return 0, err
		}
		if err := validateReg(B, "MOVEN source"); err != nil {
			return 0, err
		}
		if pc+C >= codeLen {
			return 0, fmt.Errorf("MOVEN at pc %d missing %d trailing MOVE instruction(s)", pc, C)
		}
		for j := 0; j < C; j++ {
			extra := p.Code[pc+1+j]
			if opGetOpCode(extra) != OP_MOVE {
				return 0, fmt.Errorf("MOVEN at pc %d expects MOVE at pc %d", pc, pc+1+j)
			}
			if err := validateReg(opGetArgA(extra), "MOVEN trailing destination"); err != nil {
				return 0, err
			}
			if err := validateReg(opGetArgB(extra), "MOVEN trailing source"); err != nil {
				return 0, err
			}
		}
		return C, nil
	case OP_LOADK:
		if err := validateReg(A, "LOADK destination"); err != nil {
			return 0, err
		}
		if err := validateConst(Bx, "LOADK"); err != nil {
			return 0, err
		}
	case OP_LOADBOOL:
		if err := validateReg(A, "LOADBOOL destination"); err != nil {
			return 0, err
		}
		if C != 0 {
			if err := validateHasNext("LOADBOOL"); err != nil {
				return 0, err
			}
		}
	case OP_LOADNIL:
		if err := validateRegRange(A, B, "LOADNIL"); err != nil {
			return 0, err
		}
	case OP_GETUPVAL:
		if err := validateReg(A, "GETUPVAL destination"); err != nil {
			return 0, err
		}
		if err := validateUpvalue(B, "GETUPVAL"); err != nil {
			return 0, err
		}
	case OP_GETGLOBAL:
		if err := validateReg(A, "GETGLOBAL destination"); err != nil {
			return 0, err
		}
		if err := validateStringConst(Bx, "GETGLOBAL"); err != nil {
			return 0, err
		}
	case OP_GETTABLE:
		if err := validateReg(A, "GETTABLE destination"); err != nil {
			return 0, err
		}
		if err := validateReg(B, "GETTABLE base"); err != nil {
			return 0, err
		}
		if err := validateRK(C, "GETTABLE key"); err != nil {
			return 0, err
		}
	case OP_GETTABLEKS:
		if err := validateReg(A, "GETTABLEKS destination"); err != nil {
			return 0, err
		}
		if err := validateReg(B, "GETTABLEKS base"); err != nil {
			return 0, err
		}
		if err := validateRKString(C, "GETTABLEKS key"); err != nil {
			return 0, err
		}
	case OP_SETGLOBAL:
		if err := validateReg(A, "SETGLOBAL source"); err != nil {
			return 0, err
		}
		if err := validateStringConst(Bx, "SETGLOBAL"); err != nil {
			return 0, err
		}
	case OP_SETUPVAL:
		if err := validateReg(A, "SETUPVAL source"); err != nil {
			return 0, err
		}
		if err := validateUpvalue(B, "SETUPVAL"); err != nil {
			return 0, err
		}
	case OP_SETTABLE:
		if err := validateReg(A, "SETTABLE base"); err != nil {
			return 0, err
		}
		if err := validateRK(B, "SETTABLE key"); err != nil {
			return 0, err
		}
		if err := validateRK(C, "SETTABLE value"); err != nil {
			return 0, err
		}
	case OP_SETTABLEKS:
		if err := validateReg(A, "SETTABLEKS base"); err != nil {
			return 0, err
		}
		if err := validateRKString(B, "SETTABLEKS key"); err != nil {
			return 0, err
		}
		if err := validateRK(C, "SETTABLEKS value"); err != nil {
			return 0, err
		}
	case OP_NEWTABLE:
		if err := validateReg(A, "NEWTABLE destination"); err != nil {
			return 0, err
		}
	case OP_SELF:
		if err := validateRegRange(A, A+1, "SELF destination"); err != nil {
			return 0, err
		}
		if err := validateReg(B, "SELF receiver"); err != nil {
			return 0, err
		}
		if err := validateRKString(C, "SELF method"); err != nil {
			return 0, err
		}
	case OP_ADD, OP_SUB, OP_MUL, OP_DIV, OP_MOD, OP_POW, OP_BAND, OP_BOR, OP_BXOR, OP_SHL, OP_SHR, OP_IDIV:
		if err := validateReg(A, "arithmetic destination"); err != nil {
			return 0, err
		}
		if err := validateRK(B, "arithmetic lhs"); err != nil {
			return 0, err
		}
		if err := validateRK(C, "arithmetic rhs"); err != nil {
			return 0, err
		}
	case OP_UNM, OP_LEN, OP_BNOT:
		if err := validateReg(A, "unary destination"); err != nil {
			return 0, err
		}
		if err := validateRK(B, "unary operand"); err != nil {
			return 0, err
		}
	case OP_NOT:
		if err := validateReg(A, "NOT destination"); err != nil {
			return 0, err
		}
		if err := validateReg(B, "NOT source"); err != nil {
			return 0, err
		}
	case OP_CONCAT:
		if err := validateReg(A, "CONCAT destination"); err != nil {
			return 0, err
		}
		if err := validateRegRange(B, C, "CONCAT operand"); err != nil {
			return 0, err
		}
	case OP_JMP:
		if err := validateJumpTarget(pc, Sbx, "JMP"); err != nil {
			return 0, err
		}
	case OP_EQ, OP_LT, OP_LE:
		if err := validateRK(B, "comparison lhs"); err != nil {
			return 0, err
		}
		if err := validateRK(C, "comparison rhs"); err != nil {
			return 0, err
		}
		if err := validateHasNext(opProps[op].Name); err != nil {
			return 0, err
		}
	case OP_TEST:
		if err := validateReg(A, "TEST source"); err != nil {
			return 0, err
		}
		if err := validateHasNext("TEST"); err != nil {
			return 0, err
		}
	case OP_TESTSET:
		if err := validateReg(A, "TESTSET destination"); err != nil {
			return 0, err
		}
		if err := validateReg(B, "TESTSET source"); err != nil {
			return 0, err
		}
		if err := validateHasNext("TESTSET"); err != nil {
			return 0, err
		}
	case OP_CALL:
		if err := validateReg(A, "CALL base"); err != nil {
			return 0, err
		}
		if B > 0 {
			if err := validateRegRange(A, A+B-1, "CALL argument"); err != nil {
				return 0, err
			}
		}
		if C > 1 {
			if err := validateRegRange(A, A+C-2, "CALL destination"); err != nil {
				return 0, err
			}
		}
	case OP_TAILCALL:
		if err := validateReg(A, "TAILCALL base"); err != nil {
			return 0, err
		}
		if B > 0 {
			if err := validateRegRange(A, A+B-1, "TAILCALL argument"); err != nil {
				return 0, err
			}
		}
	case OP_RETURN:
		if err := validateReg(A, "RETURN base"); err != nil {
			return 0, err
		}
		if B > 1 {
			if err := validateRegRange(A, A+B-2, "RETURN value"); err != nil {
				return 0, err
			}
		}
	case OP_FORLOOP:
		if err := validateRegRange(A, A+3, "FORLOOP"); err != nil {
			return 0, err
		}
		if err := validateJumpTarget(pc, Sbx, "FORLOOP"); err != nil {
			return 0, err
		}
	case OP_FORPREP:
		if err := validateRegRange(A, A+2, "FORPREP"); err != nil {
			return 0, err
		}
		if err := validateJumpTarget(pc, Sbx, "FORPREP"); err != nil {
			return 0, err
		}
	case OP_TFORLOOP:
		if err := validateRegRange(A, A+3+C, "TFORLOOP"); err != nil {
			return 0, err
		}
		if err := validateHasNext("TFORLOOP"); err != nil {
			return 0, err
		}
		helper := p.Code[pc+1]
		if opGetOpCode(helper) != OP_JMP {
			return 0, fmt.Errorf("TFORLOOP at pc %d expects JMP helper at pc %d", pc, pc+1)
		}
		if err := validateJumpTarget(pc+1, opGetArgSbx(helper), "TFORLOOP helper"); err != nil {
			return 0, err
		}
		return 1, nil
	case OP_SETLIST:
		if err := validateReg(A, "SETLIST table"); err != nil {
			return 0, err
		}
		if B > 0 {
			if err := validateRegRange(A+1, A+B, "SETLIST source"); err != nil {
				return 0, err
			}
		}
		if C == 0 {
			if err := validateHasNext("SETLIST"); err != nil {
				return 0, err
			}
			return 1, nil
		}
	case OP_CLOSURE:
		if err := validateReg(A, "CLOSURE destination"); err != nil {
			return 0, err
		}
		if Bx < 0 || Bx >= len(p.FunctionPrototypes) {
			return 0, fmt.Errorf("invalid closure prototype index %d at pc %d", Bx, pc)
		}
		child := p.FunctionPrototypes[Bx]
		if child == nil {
			return 0, fmt.Errorf("nil closure prototype at index %d for pc %d", Bx, pc)
		}
		nup := int(child.NumUpvalues)
		if nup > 0 && pc+nup >= codeLen {
			return 0, fmt.Errorf("CLOSURE at pc %d missing %d upvalue binding instruction(s)", pc, nup)
		}
		for j := 0; j < nup; j++ {
			extra := p.Code[pc+1+j]
			switch opGetOpCode(extra) {
			case OP_MOVE:
				if err := validateReg(opGetArgB(extra), "CLOSURE upvalue source"); err != nil {
					return 0, err
				}
			case OP_GETUPVAL:
				if err := validateUpvalue(opGetArgB(extra), "CLOSURE captured upvalue"); err != nil {
					return 0, err
				}
			default:
				return 0, fmt.Errorf("invalid CLOSURE upvalue binding opcode %d at pc %d", opGetOpCode(extra), pc+1+j)
			}
		}
		return nup, nil
	case OP_VARARG:
		if err := validateReg(A, "VARARG destination"); err != nil {
			return 0, err
		}
		if B > 1 {
			if err := validateRegRange(A, A+B-2, "VARARG destination"); err != nil {
				return 0, err
			}
		}
	case OP_CLOSE, OP_NOP:
		return 0, nil
	}
	return 0, nil
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
