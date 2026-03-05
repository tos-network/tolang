package lua

import (
	"fmt"
	"strings"

	"github.com/tos-network/tolang/ast"
)

// IRInstruction is a normalized representation of one VM instruction.
// It keeps decoded operands and original op encoding kind.
type IRInstruction struct {
	Op   int
	Type opType
	A    int
	B    int
	C    int
	Bx   int
	Sbx  int
	Raw  uint32
}

// IRFunction mirrors FunctionProto but stores decoded instructions.
type IRFunction struct {
	SourceName      string
	LineDefined     int
	LastLineDefined int
	NumUpvalues     uint8
	NumParameters   uint8
	IsVarArg        uint8
	NumUsedRegs     uint8

	Instructions []IRInstruction
	Constants    []LValue
	Functions    []*IRFunction

	DbgSourcePositions []int
	DbgLocals          []*DbgLocalInfo
	DbgCalls           []DbgCall
	DbgUpvalues        []string
}

// IRProgram is a compiled unit root.
type IRProgram struct {
	Name string
	Root *IRFunction
}

func (p *IRProgram) String() string {
	if p == nil || p.Root == nil {
		return "<nil ir>"
	}
	var b strings.Builder
	writeIRFunction(&b, p.Root, 0)
	return b.String()
}

// BuildIRFromChunk compiles a Lua AST chunk to the IR layer.
func BuildIRFromChunk(chunk []ast.Stmt, name string) (*IRProgram, error) {
	return buildIRFromChunk(chunk, name)
}

// buildIRFromChunk is the internal implementation.
func buildIRFromChunk(chunk []ast.Stmt, name string) (*IRProgram, error) {
	root, err := compileASTToIRDirect(chunk, name)
	if err != nil {
		return nil, err
	}
	if name == "" && root != nil {
		name = root.SourceName
	}
	return &IRProgram{Name: name, Root: root}, nil
}

// BuildIRFromProto decodes a FunctionProto into IR.
func BuildIRFromProto(proto *FunctionProto, name string) *IRProgram {
	if proto == nil {
		return &IRProgram{Name: name, Root: nil}
	}
	if name == "" {
		name = proto.SourceName
	}
	return &IRProgram{Name: name, Root: irFromProto(proto)}
}

// CompileIR lowers IR back to executable FunctionProto bytecode.
func CompileIR(program *IRProgram) (*FunctionProto, error) {
	if program == nil || program.Root == nil {
		return nil, fmt.Errorf("nil IR program")
	}
	proto := protoFromIR(program.Root)
	if program.Name != "" {
		proto.SourceName = program.Name
	}
	hydrateStringConstants(proto)
	return proto, nil
}

func irFromProto(p *FunctionProto) *IRFunction {
	irf := &IRFunction{
		SourceName:         p.SourceName,
		LineDefined:        p.LineDefined,
		LastLineDefined:    p.LastLineDefined,
		NumUpvalues:        p.NumUpvalues,
		NumParameters:      p.NumParameters,
		IsVarArg:           p.IsVarArg,
		NumUsedRegs:        p.NumUsedRegisters,
		Instructions:       make([]IRInstruction, 0, len(p.Code)),
		Constants:          append([]LValue(nil), p.Constants...),
		Functions:          make([]*IRFunction, 0, len(p.FunctionPrototypes)),
		DbgSourcePositions: append([]int(nil), p.DbgSourcePositions...),
		DbgLocals:          cloneDbgLocals(p.DbgLocals),
		DbgCalls:           append([]DbgCall(nil), p.DbgCalls...),
		DbgUpvalues:        append([]string(nil), p.DbgUpvalues...),
	}
	for i := 0; i < len(p.Code); i++ {
		inst := p.Code[i]
		op := opGetOpCode(inst)
		ins := IRInstruction{Op: op, Type: opProps[op].Type, A: opGetArgA(inst), Raw: 0}
		switch ins.Type {
		case opTypeABC:
			ins.B = opGetArgB(inst)
			ins.C = opGetArgC(inst)
		case opTypeABx:
			ins.Bx = opGetArgBx(inst)
		case opTypeASbx:
			ins.Sbx = opGetArgSbx(inst)
		}
		irf.Instructions = append(irf.Instructions, ins)
		if op == OP_SETLIST && ins.C == 0 && i+1 < len(p.Code) {
			i++
			irf.Instructions = append(irf.Instructions, IRInstruction{
				Op: -1, Type: opType(-1), A: -1, B: -1, C: -1, Bx: -1, Sbx: 0, Raw: p.Code[i],
			})
		}
	}
	for _, child := range p.FunctionPrototypes {
		irf.Functions = append(irf.Functions, irFromProto(child))
	}
	return irf
}

func protoFromIR(irf *IRFunction) *FunctionProto {
	p := &FunctionProto{
		SourceName:         irf.SourceName,
		LineDefined:        irf.LineDefined,
		LastLineDefined:    irf.LastLineDefined,
		NumUpvalues:        irf.NumUpvalues,
		NumParameters:      irf.NumParameters,
		IsVarArg:           irf.IsVarArg,
		NumUsedRegisters:   irf.NumUsedRegs,
		Code:               make([]uint32, 0, len(irf.Instructions)),
		Constants:          append([]LValue(nil), irf.Constants...),
		FunctionPrototypes: make([]*FunctionProto, 0, len(irf.Functions)),
		DbgSourcePositions: append([]int(nil), irf.DbgSourcePositions...),
		DbgLocals:          cloneDbgLocals(irf.DbgLocals),
		DbgCalls:           append([]DbgCall(nil), irf.DbgCalls...),
		DbgUpvalues:        append([]string(nil), irf.DbgUpvalues...),
	}
	for _, ins := range irf.Instructions {
		if ins.Op < 0 {
			p.Code = append(p.Code, ins.Raw)
			continue
		}
		switch ins.Type {
		case opTypeABC:
			p.Code = append(p.Code, opCreateABC(ins.Op, ins.A, ins.B, ins.C))
		case opTypeABx:
			p.Code = append(p.Code, opCreateABx(ins.Op, ins.A, ins.Bx))
		case opTypeASbx:
			p.Code = append(p.Code, opCreateASbx(ins.Op, ins.A, ins.Sbx))
		}
	}
	for _, child := range irf.Functions {
		p.FunctionPrototypes = append(p.FunctionPrototypes, protoFromIR(child))
	}
	return p
}

func cloneDbgLocals(src []*DbgLocalInfo) []*DbgLocalInfo {
	out := make([]*DbgLocalInfo, 0, len(src))
	for _, li := range src {
		if li == nil {
			out = append(out, nil)
			continue
		}
		cp := *li
		out = append(out, &cp)
	}
	return out
}

func writeIRFunction(b *strings.Builder, f *IRFunction, depth int) {
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(b, "%sfunction %q params=%d regs=%d up=%d\n", indent, f.SourceName, f.NumParameters, f.NumUsedRegs, f.NumUpvalues)
	for pc, ins := range f.Instructions {
		if ins.Op < 0 {
			fmt.Fprintf(b, "%s  [%03d] RAW      0x%08x\n", indent, pc+1, ins.Raw)
			continue
		}
		name := "INVALID"
		if ins.Op >= 0 && ins.Op <= opCodeMax {
			name = opProps[ins.Op].Name
		}
		switch ins.Type {
		case opTypeABC:
			fmt.Fprintf(b, "%s  [%03d] %-8s A=%d B=%d C=%d\n", indent, pc+1, name, ins.A, ins.B, ins.C)
		case opTypeABx:
			fmt.Fprintf(b, "%s  [%03d] %-8s A=%d Bx=%d\n", indent, pc+1, name, ins.A, ins.Bx)
		case opTypeASbx:
			fmt.Fprintf(b, "%s  [%03d] %-8s A=%d sBx=%d\n", indent, pc+1, name, ins.A, ins.Sbx)
		}
	}
	for _, child := range f.Functions {
		writeIRFunction(b, child, depth+1)
	}
}

func hydrateStringConstants(proto *FunctionProto) {
	if proto == nil {
		return
	}
	proto.stringConstants = make([]string, 0, len(proto.Constants))
	for _, c := range proto.Constants {
		s := ""
		if sc, ok := c.(LString); ok {
			s = string(sc)
		}
		proto.stringConstants = append(proto.stringConstants, s)
	}
	for _, child := range proto.FunctionPrototypes {
		hydrateStringConstants(child)
	}
}
