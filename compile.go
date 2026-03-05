package lua

import (
	"fmt"
	"reflect"

	"github.com/tos-network/tolang/ast"
)

/* internal constants & structs  {{{ */

const maxRegisters = 200

type expContextType int

const (
	ecGlobal expContextType = iota
	ecUpvalue
	ecLocal
	ecTable
	ecVararg
	ecMethod
	ecNone
)

const regNotDefined = opMaxArgsA + 1
const labelNoJump = 0

type expcontext struct {
	ctype expContextType
	reg   int
	// varargopt >= 0: wants varargopt+1 results, i.e  a = func()
	// varargopt = -1: ignore results             i.e  func()
	// varargopt = -2: receive all results        i.e  a = {func()}
	varargopt int
}

type assigncontext struct {
	ec       *expcontext
	keyrk    int
	valuerk  int
	keyks    bool
	needmove bool
}

type lblabels struct {
	t int
	f int
	e int
	b bool
}

type constLValueExpr struct {
	ast.ExprBase

	Value LValue
}

// }}}

/* utilities {{{ */
var _ecnone0 = &expcontext{ecNone, regNotDefined, 0}
var _ecnonem1 = &expcontext{ecNone, regNotDefined, -1}
var _ecnonem2 = &expcontext{ecNone, regNotDefined, -2}
var ecfuncdef = &expcontext{ecMethod, regNotDefined, 0}

func ecupdate(ec *expcontext, ctype expContextType, reg, varargopt int) {
	if ec == _ecnone0 || ec == _ecnonem1 || ec == _ecnonem2 {
		panic("can not update ec cache")
	}
	ec.ctype = ctype
	ec.reg = reg
	ec.varargopt = varargopt
}

func ecnone(varargopt int) *expcontext {
	switch varargopt {
	case 0:
		return _ecnone0
	case -1:
		return _ecnonem1
	case -2:
		return _ecnonem2
	}
	return &expcontext{ecNone, regNotDefined, varargopt}
}

func shouldmove(ec *expcontext, reg int) bool {
	return ec.ctype == ecLocal && ec.reg != regNotDefined && ec.reg != reg
}

func sline(pos ast.PositionHolder) int {
	return pos.Line()
}

func eline(pos ast.PositionHolder) int {
	line := pos.LastLine()
	if line == 0 {
		return pos.Line()
	}
	return line
}

func savereg(ec *expcontext, reg int) int {
	if ec.ctype != ecLocal || ec.reg == regNotDefined {
		return reg
	}
	return ec.reg
}

func raiseCompileError(context *funcContext, line int, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	panic(&CompileError{context: context, Line: line, Message: msg})
}

func isVarArgReturnExpr(expr ast.Expr) bool {
	switch ex := expr.(type) {
	case *ast.FuncCallExpr:
		return !ex.AdjustRet
	case *ast.Comma3Expr:
		return !ex.AdjustRet
	}
	return false
}

func lnumberValue(expr ast.Expr) (LUint256, bool) {
	if ex, ok := expr.(*ast.NumberExpr); ok {
		lv, err := parseNumber(ex.Value)
		if err != nil {
			return LUint256Zero, false
		}
		return lv, true
	} else if ex, ok := expr.(*constLValueExpr); ok {
		return ex.Value.(LUint256), true
	}
	return LUint256Zero, false
}

/* utilities }}} */

type gotoLabelDesc struct { // {{{
	Id                 int
	Name               string
	Pc                 int
	Line               int
	NumActiveLocalVars int
}

func newLabelDesc(id int, name string, pc, line, n int) *gotoLabelDesc {
	return &gotoLabelDesc{
		Id:                 id,
		Name:               name,
		Pc:                 pc,
		Line:               line,
		NumActiveLocalVars: n,
	}
}

func (l *gotoLabelDesc) SetNumActiveLocalVars(n int) {
	l.NumActiveLocalVars = n
} // }}}

type CompileError struct { // {{{
	context *funcContext
	Line    int
	Message string
}

func (e *CompileError) Error() string {
	return fmt.Sprintf("compile error near line(%v) %v: %v", e.Line, e.context.Func.SourceName, e.Message)
} // }}}

type codeStore struct { // {{{
	codes            []IRInstruction
	lines            []int
	pc               int
	loadNilBarrier   bool // true after any JMP/branch; prevents unsafe LOADNIL merging (Bug #522)
}

func (cd *codeStore) Add(inst IRInstruction, line int) {
	if l := len(cd.codes); l <= 0 || cd.pc == l {
		cd.codes = append(cd.codes, inst)
		cd.lines = append(cd.lines, line)
	} else {
		cd.codes[cd.pc] = inst
		cd.lines[cd.pc] = line
	}
	cd.pc++
}

func (cd *codeStore) AddABC(op int, a int, b int, c int, line int) {
	cd.Add(IRInstruction{
		Op:   op,
		Type: opTypeABC,
		A:    a,
		B:    b,
		C:    c,
		Bx:   -1,
		Sbx:  0,
	}, line)
}

func (cd *codeStore) AddABx(op int, a int, bx int, line int) {
	cd.Add(IRInstruction{
		Op:   op,
		Type: opTypeABx,
		A:    a,
		B:    -1,
		C:    -1,
		Bx:   bx,
		Sbx:  0,
	}, line)
}

func (cd *codeStore) AddASbx(op int, a int, sbx int, line int) {
	cd.Add(IRInstruction{
		Op:   op,
		Type: opTypeASbx,
		A:    a,
		B:    -1,
		C:    -1,
		Bx:   -1,
		Sbx:  sbx,
	}, line)
	if op == OP_JMP {
		// Any branch instruction marks the LOADNIL merge barrier: a LOADNIL
		// emitted after this point must NOT be merged with a LOADNIL that was
		// emitted before this branch, because the earlier LOADNIL may be a
		// jump target from inside the branch expression (Bug #522).
		cd.loadNilBarrier = true
	}
}

func (cd *codeStore) AddRawWord(word uint32, line int) {
	cd.Add(IRInstruction{
		Op: -1, Type: opType(-1), A: -1, B: -1, C: -1, Bx: -1, Sbx: 0, Raw: word,
	}, line)
}

func (cd *codeStore) PropagateKMV(top int, save *int, reg *int, inc int) {
	lastinst := cd.Last()
	if lastinst.A >= top {
		switch lastinst.Op {
		case OP_LOADK:
			cindex := lastinst.Bx
			if cindex <= opMaxIndexRk {
				cd.Pop()
				*save = opRkAsk(cindex)
				return
			}
		case OP_MOVE:
			cd.Pop()
			*save = lastinst.B
			return
		}
	}
	*save = *reg
	*reg = *reg + inc
}

func (cd *codeStore) PropagateMV(top int, save *int, reg *int, inc int) {
	lastinst := cd.Last()
	if lastinst.A >= top {
		switch lastinst.Op {
		case OP_MOVE:
			cd.Pop()
			*save = lastinst.B
			return
		}
	}
	*save = *reg
	*reg = *reg + inc
}

func (cd *codeStore) AddLoadNil(a, b, line int) {
	// The peephole merges consecutive LOADNIL instructions that cover adjacent
	// register ranges into a single instruction, e.g. three separate `x = nil`
	// statements for sequential locals become one LOADNIL covering all three.
	//
	// Bug #522 safety guard: when a JMP was emitted since the last LOADNIL
	// (loadNilBarrier == true), the previous LOADNIL may be a branch target.
	// Extending it would clobber extra registers on ALL paths through it, not
	// just the current one.  In that case always emit a fresh instruction.
	last := cd.Last()
	if !cd.loadNilBarrier && last.Op == OP_LOADNIL && (last.B+1) == a {
		cd.SetB(cd.LastPC(), b)
	} else {
		cd.AddABC(OP_LOADNIL, a, b, 0, line)
	}
	cd.loadNilBarrier = false // reset after each LOADNIL
}

func (cd *codeStore) SetOpCode(pc int, v int) {
	cd.codes[pc].Op = v
}

func (cd *codeStore) SetA(pc int, v int) {
	cd.codes[pc].A = v
}

func (cd *codeStore) SetB(pc int, v int) {
	cd.codes[pc].B = v
}

func (cd *codeStore) SetC(pc int, v int) {
	cd.codes[pc].C = v
}

func (cd *codeStore) SetBx(pc int, v int) {
	cd.codes[pc].Bx = v
}

func (cd *codeStore) SetSbx(pc int, v int) {
	cd.codes[pc].Sbx = v
}

func (cd *codeStore) At(pc int) IRInstruction {
	return cd.codes[pc]
}

func (cd *codeStore) List() []IRInstruction {
	return cd.codes[:cd.pc]
}

func (cd *codeStore) PosList() []int {
	return cd.lines[:cd.pc]
}

func (cd *codeStore) LastPC() int {
	return cd.pc - 1
}

func (cd *codeStore) Last() IRInstruction {
	if cd.pc == 0 {
		return IRInstruction{Op: -1, Type: opTypeABC, A: -1, B: -1, C: -1, Bx: -1, Sbx: 0}
	}
	return cd.codes[cd.pc-1]
}

func (cd *codeStore) Pop() {
	cd.pc--
} /* }}} Code */

/* {{{ VarNamePool */

type varNamePoolValue struct {
	Index int
	Name  string
	Attr  ast.LocalAttr
}

type varNamePool struct {
	names  []string
	attrs  []ast.LocalAttr
	offset int
}

func newVarNamePool(offset int) *varNamePool {
	return &varNamePool{
		names:  make([]string, 0, 16),
		attrs:  make([]ast.LocalAttr, 0, 16),
		offset: offset,
	}
}

func (vp *varNamePool) Names() []string {
	return vp.names
}

func (vp *varNamePool) List() []varNamePoolValue {
	result := make([]varNamePoolValue, len(vp.names), len(vp.names))
	for i, name := range vp.names {
		result[i].Index = i + vp.offset
		result[i].Name = name
		result[i].Attr = vp.attrs[i]
	}
	return result
}

func (vp *varNamePool) LastIndex() int {
	return vp.offset + len(vp.names)
}

func (vp *varNamePool) Find(name string) int {
	for i := len(vp.names) - 1; i >= 0; i-- {
		if vp.names[i] == name {
			return i + vp.offset
		}
	}
	return -1
}

func (vp *varNamePool) RegisterUnique(name string) int {
	index := vp.Find(name)
	if index < 0 {
		return vp.Register(name)
	}
	return index
}

func (vp *varNamePool) Register(name string) int {
	return vp.RegisterWithAttr(name, ast.LocalAttrNone)
}

func (vp *varNamePool) RegisterWithAttr(name string, attr ast.LocalAttr) int {
	vp.names = append(vp.names, name)
	vp.attrs = append(vp.attrs, attr)
	return len(vp.names) - 1 + vp.offset
}

func (vp *varNamePool) Attr(index int) ast.LocalAttr {
	i := index - vp.offset
	if i < 0 || i >= len(vp.attrs) {
		return ast.LocalAttrNone
	}
	return vp.attrs[i]
}

/* }}} VarNamePool */

/* FuncContext {{{ */

type codeBlock struct {
	LocalVars      *varNamePool
	BreakLabel     int
	Parent         *codeBlock
	RefUpvalue     bool
	LineStart      int
	LastLine       int
	labels         map[string]*gotoLabelDesc
	firstGotoIndex int
}

func newCodeBlock(localvars *varNamePool, blabel int, parent *codeBlock, pos ast.PositionHolder, firstGotoIndex int) *codeBlock {
	bl := &codeBlock{localvars, blabel, parent, false, 0, 0, map[string]*gotoLabelDesc{}, firstGotoIndex}
	if pos != nil {
		bl.LineStart = pos.Line()
		bl.LastLine = pos.LastLine()
	}
	return bl
}

func (b *codeBlock) AddLabel(label *gotoLabelDesc) *gotoLabelDesc {
	if old, ok := b.labels[label.Name]; ok {
		return old
	}
	b.labels[label.Name] = label
	return nil
}

func (b *codeBlock) GetLabel(label string) *gotoLabelDesc {
	if v, ok := b.labels[label]; ok {
		return v
	}
	return nil
}

func (b *codeBlock) LocalVarsCount() int {
	count := 0
	for block := b; block != nil; block = block.Parent {
		count += len(block.LocalVars.Names())
	}
	return count
}

func (b *codeBlock) HasToBeClosed() bool {
	for _, v := range b.LocalVars.List() {
		if v.Attr == ast.LocalAttrClose {
			return true
		}
	}
	return false
}

type funcContext struct {
	Func            *IRFunction
	Code            *codeStore
	Parent          *funcContext
	Upvalues        *varNamePool
	Block           *codeBlock
	Blocks          []*codeBlock
	regTop          int
	labelId         int
	labelPc         map[int]int
	gotosCount      int
	unresolvedGotos map[int]*gotoLabelDesc
}

func newFuncContext(sourcename string, parent *funcContext) *funcContext {
	fc := &funcContext{
		Func: &IRFunction{
			SourceName:         sourcename,
			Instructions:       make([]IRInstruction, 0, 1024),
			Constants:          make([]LValue, 0, 32),
			Functions:          make([]*IRFunction, 0, 16),
			DbgSourcePositions: make([]int, 0, 128),
			DbgLocals:          make([]*DbgLocalInfo, 0, 16),
			DbgCalls:           make([]DbgCall, 0, 128),
			DbgUpvalues:        make([]string, 0, 16),
		},
		Code:            &codeStore{codes: make([]IRInstruction, 0, 1024), lines: make([]int, 0, 1024)},
		Parent:          parent,
		Upvalues:        newVarNamePool(0),
		Block:           newCodeBlock(newVarNamePool(0), labelNoJump, nil, nil, 0),
		regTop:          0,
		labelId:         1,
		labelPc:         map[int]int{},
		gotosCount:      0,
		unresolvedGotos: map[int]*gotoLabelDesc{},
	}
	fc.Blocks = []*codeBlock{fc.Block}
	return fc
}

func (fc *funcContext) CheckUnresolvedGoto() {
	for i := fc.Block.firstGotoIndex; i < fc.gotosCount; i++ {
		gotoLabel, ok := fc.unresolvedGotos[i]
		if !ok {
			continue
		}
		raiseCompileError(fc, fc.Func.LastLineDefined, "no visible label '%s' for <goto> at line %d", gotoLabel.Name, gotoLabel.Line)
	}
}

func (fc *funcContext) AddUnresolvedGoto(label *gotoLabelDesc) {
	fc.unresolvedGotos[fc.gotosCount] = label
	fc.gotosCount++
}

func (fc *funcContext) AddNamedLabel(label *gotoLabelDesc) {
	if old := fc.Block.AddLabel(label); old != nil {
		raiseCompileError(fc, label.Line+1, "label '%s' already defined on line %d", label.Name, old.Line)
	}
	fc.SetLabelPc(label.Id, label.Pc)
}

func (fc *funcContext) GetNamedLabel(name string) *gotoLabelDesc {
	return fc.Block.GetLabel(name)
}

func (fc *funcContext) ResolveGoto(from, to *gotoLabelDesc, index int) {
	if from.NumActiveLocalVars < to.NumActiveLocalVars {
		varName := fc.Block.LocalVars.Names()[len(fc.Block.LocalVars.Names())-1]
		raiseCompileError(fc, to.Line+1, "<goto %s> at line %d jumps into the scope of local '%s'", to.Name, from.Line, varName)
	}
	fc.Code.SetSbx(from.Pc, to.Id)
	delete(fc.unresolvedGotos, index)
}

func (fc *funcContext) FindLabel(block *codeBlock, gotoLabel *gotoLabelDesc, i int) bool {
	target := block.GetLabel(gotoLabel.Name)
	if target != nil {
		if gotoLabel.NumActiveLocalVars > target.NumActiveLocalVars && (block.RefUpvalue || block.HasToBeClosed()) {
			fc.Code.SetA(gotoLabel.Pc-1, target.NumActiveLocalVars)
		}
		fc.ResolveGoto(gotoLabel, target, i)
		return true
	}
	return false
}

func (fc *funcContext) ResolveCurrentBlockGotosWithParentBlock() {
	blockActiveLocalVars := fc.Block.Parent.LocalVarsCount()
	for i := fc.Block.firstGotoIndex; i < fc.gotosCount; i++ {
		gotoLabel, ok := fc.unresolvedGotos[i]
		if !ok {
			continue
		}
		if gotoLabel.NumActiveLocalVars > blockActiveLocalVars {
			if fc.Block.RefUpvalue || fc.Block.HasToBeClosed() {
				fc.Code.SetA(gotoLabel.Pc-1, blockActiveLocalVars)
			}
			gotoLabel.SetNumActiveLocalVars(blockActiveLocalVars)
		}
		fc.FindLabel(fc.Block.Parent, gotoLabel, i)
	}
}

func (fc *funcContext) ResolveForwardGoto(target *gotoLabelDesc) {
	for i := fc.Block.firstGotoIndex; i <= fc.gotosCount; i++ {
		gotoLabel, ok := fc.unresolvedGotos[i]
		if !ok {
			continue
		}
		if gotoLabel.Name == target.Name {
			fc.ResolveGoto(gotoLabel, target, i)
		}
	}
}

func (fc *funcContext) NewLabel() int {
	ret := fc.labelId
	fc.labelId++
	return ret
}

func (fc *funcContext) SetLabelPc(label int, pc int) {
	fc.labelPc[label] = pc
}

func (fc *funcContext) GetLabelPc(label int) int {
	return fc.labelPc[label]
}

func (fc *funcContext) ConstIndex(value LValue) int {
	ctype := value.Type()
	for i, lv := range fc.Func.Constants {
		if lv.Type() == ctype && lv == value {
			return i
		}
	}
	fc.Func.Constants = append(fc.Func.Constants, value)
	v := len(fc.Func.Constants) - 1
	if v > opMaxArgBx {
		raiseCompileError(fc, fc.Func.LineDefined, "too many constants")
	}
	return v
}
func (fc *funcContext) BlockLocalVarsCount() int {
	count := 0
	for block := fc.Block; block != nil; block = block.Parent {
		count += len(block.LocalVars.Names())
	}
	return count
}

func (fc *funcContext) RegisterLocalVar(name string) int {
	return fc.RegisterLocalVarWithAttr(name, ast.LocalAttrNone)
}

func (fc *funcContext) RegisterLocalVarWithAttr(name string, attr ast.LocalAttr) int {
	ret := fc.Block.LocalVars.RegisterWithAttr(name, attr)
	fc.Func.DbgLocals = append(fc.Func.DbgLocals, &DbgLocalInfo{
		Name:    name,
		Reg:     ret,
		Attr:    uint8(attr),
		StartPc: fc.Code.LastPC() + 1,
	})
	fc.SetRegTop(fc.RegTop() + 1)
	return ret
}

func (fc *funcContext) FindLocalVarAndBlock(name string) (int, *codeBlock) {
	for block := fc.Block; block != nil; block = block.Parent {
		if index := block.LocalVars.Find(name); index > -1 {
			return index, block
		}
	}
	return -1, nil
}

func (fc *funcContext) FindLocalVar(name string) int {
	idx, _ := fc.FindLocalVarAndBlock(name)
	return idx
}

func (fc *funcContext) LocalVars() []varNamePoolValue {
	result := make([]varNamePoolValue, 0, 32)
	for _, block := range fc.Blocks {
		result = append(result, block.LocalVars.List()...)
	}
	return result
}

func (fc *funcContext) EnterBlock(blabel int, pos ast.PositionHolder) {
	fc.Block = newCodeBlock(newVarNamePool(fc.RegTop()), blabel, fc.Block, pos, fc.gotosCount)
	fc.Blocks = append(fc.Blocks, fc.Block)
}

func (fc *funcContext) CloseUpvalues() int {
	n := -1
	if fc.Block.RefUpvalue || fc.Block.HasToBeClosed() {
		n = fc.Block.Parent.LocalVars.LastIndex()
		fc.Code.AddABC(OP_CLOSE, n, 0, 0, fc.Block.LastLine)
	}
	return n
}

func (fc *funcContext) LeaveBlock() int {
	closed := fc.CloseUpvalues()
	fc.EndScope()

	if fc.Block.Parent != nil {
		fc.ResolveCurrentBlockGotosWithParentBlock()
	}
	fc.Block = fc.Block.Parent
	fc.SetRegTop(fc.Block.LocalVars.LastIndex())
	return closed
}

func (fc *funcContext) EndScope() {
	for _, vr := range fc.Block.LocalVars.List() {
		for i := len(fc.Func.DbgLocals) - 1; i >= 0; i-- {
			li := fc.Func.DbgLocals[i]
			if li.Reg == vr.Index && li.EndPc == 0 {
				li.EndPc = fc.Code.LastPC()
				break
			}
		}
	}
}

func (fc *funcContext) SetRegTop(top int) {
	if top > maxRegisters {
		raiseCompileError(fc, fc.Func.LineDefined, "too many local variables")
	}
	fc.regTop = top
}

func (fc *funcContext) RegTop() int {
	return fc.regTop
}

/* FuncContext }}} */

func compileChunk(context *funcContext, chunk []ast.Stmt, untilFollows bool) { // {{{
	for i, stmt := range chunk {
		lastStmt := true
		for j := i + 1; j < len(chunk); j++ {
			_, ok := chunk[j].(*ast.LabelStmt)
			if !ok {
				lastStmt = false
				break
			}
		}
		compileStmt(context, stmt, lastStmt && !untilFollows)
	}
} // }}}

func compileBlock(context *funcContext, chunk []ast.Stmt) { // {{{
	if len(chunk) == 0 {
		return
	}
	ph := &ast.Node{}
	ph.SetLine(sline(chunk[0]))
	ph.SetLastLine(eline(chunk[len(chunk)-1]))
	context.EnterBlock(labelNoJump, ph)
	for i, stmt := range chunk {
		lastStmt := true
		for j := i + 1; j < len(chunk); j++ {
			_, ok := chunk[j].(*ast.LabelStmt)
			if !ok {
				lastStmt = false
				break
			}
		}
		compileStmt(context, stmt, lastStmt)
	}
	context.LeaveBlock()
} // }}}

func compileStmt(context *funcContext, stmt ast.Stmt, isLastStmt bool) { // {{{
	switch st := stmt.(type) {
	case *ast.AssignStmt:
		compileAssignStmt(context, st)
	case *ast.LocalAssignStmt:
		compileLocalAssignStmt(context, st)
	case *ast.FuncCallStmt:
		compileFuncCallExpr(context, context.RegTop(), st.Expr.(*ast.FuncCallExpr), ecnone(-1))
	case *ast.DoBlockStmt:
		context.EnterBlock(labelNoJump, st)
		compileChunk(context, st.Stmts, false)
		context.LeaveBlock()
	case *ast.WhileStmt:
		compileWhileStmt(context, st)
	case *ast.RepeatStmt:
		compileRepeatStmt(context, st)
	case *ast.FuncDefStmt:
		compileFuncDefStmt(context, st)
	case *ast.ReturnStmt:
		compileReturnStmt(context, st)
	case *ast.IfStmt:
		compileIfStmt(context, st)
	case *ast.BreakStmt:
		compileBreakStmt(context, st)
	case *ast.NumberForStmt:
		compileNumberForStmt(context, st)
	case *ast.GenericForStmt:
		compileGenericForStmt(context, st)
	case *ast.LabelStmt:
		compileLabelStmt(context, st, isLastStmt)
	case *ast.GotoStmt:
		compileGotoStmt(context, st)
	}
} // }}}

func compileAssignStmtLeft(context *funcContext, stmt *ast.AssignStmt) (int, []*assigncontext) { // {{{
	reg := context.RegTop()
	acs := make([]*assigncontext, 0, len(stmt.Lhs))
	for _, lhs := range stmt.Lhs {
		switch st := lhs.(type) {
		case *ast.IdentExpr:
			identtype := getIdentRefType(context, context, st)
			ec := &expcontext{identtype, regNotDefined, 0}
			switch identtype {
			case ecGlobal:
				context.ConstIndex(LString(st.Value))
			case ecUpvalue:
				context.Upvalues.RegisterUnique(st.Value)
			case ecLocal:
				localReg, localBlock := context.FindLocalVarAndBlock(st.Value)
				if localReg < 0 || localBlock == nil {
					panic("unreachable: local identifier not found")
				}
				if localBlock.LocalVars.Attr(localReg) == ast.LocalAttrConst {
					raiseCompileError(context, sline(st), "attempt to assign to const local '%s'", st.Value)
				}
				ec.reg = localReg
			}
			acs = append(acs, &assigncontext{ec, 0, 0, false, false})
		case *ast.AttrGetExpr:
			ac := &assigncontext{&expcontext{ecTable, regNotDefined, 0}, 0, 0, false, false}
			compileExprWithKMVPropagation(context, st.Object, &reg, &ac.ec.reg)
			ac.keyrk = reg
			reg += compileExpr(context, reg, st.Key, ecnone(0))
			if _, ok := st.Key.(*ast.StringExpr); ok {
				ac.keyks = true
			}
			acs = append(acs, ac)

		default:
			panic("invalid left expression.")
		}
	}
	return reg, acs
} // }}}

func compileAssignStmtRight(context *funcContext, stmt *ast.AssignStmt, reg int, acs []*assigncontext) (int, []*assigncontext) { // {{{
	lennames := len(stmt.Lhs)
	lenexprs := len(stmt.Rhs)
	namesassigned := 0

	for namesassigned < lennames {
		ac := acs[namesassigned]
		ec := ac.ec
		var expr ast.Expr = nil
		if namesassigned >= lenexprs {
			expr = &ast.NilExpr{}
			expr.SetLine(sline(stmt.Lhs[namesassigned]))
			expr.SetLastLine(eline(stmt.Lhs[namesassigned]))
		} else if isVarArgReturnExpr(stmt.Rhs[namesassigned]) && (lenexprs-namesassigned-1) <= 0 {
			varargopt := lennames - namesassigned - 1
			regstart := reg
			reginc := compileExpr(context, reg, stmt.Rhs[namesassigned], ecnone(varargopt))
			reg += reginc
			for i := namesassigned; i < namesassigned+int(reginc); i++ {
				acs[i].needmove = true
				if acs[i].ec.ctype == ecTable {
					acs[i].valuerk = regstart + (i - namesassigned)
				}
			}
			namesassigned = lennames
			continue
		}

		if expr == nil {
			expr = stmt.Rhs[namesassigned]
		}
		idx := reg
		// Bug #440: in a multi-assignment `a, b = expr_a, expr_b` the compiler
		// must evaluate ALL RHS expressions before writing any LHS.  When there
		// are multiple local-variable targets we therefore force each RHS into a
		// fresh temporary register (ecnone) rather than directly into the local's
		// register.  Without this, expr_b could read a local that expr_a already
		// overwrote.  The deferred MOVE loop in compileAssignStmt then copies each
		// temporary to its final destination once all RHS values are captured.
		// Single-assignment is unaffected: a single `a = expr` is fine as before.
		evalEC := ec
		if lennames > 1 && ec.ctype == ecLocal {
			evalEC = ecnone(0)
		}
		reginc := compileExpr(context, reg, expr, evalEC)
		if ec.ctype == ecTable {
			if _, ok := expr.(*ast.LogicalOpExpr); !ok {
				context.Code.PropagateKMV(context.RegTop(), &ac.valuerk, &reg, reginc)
			} else {
				ac.valuerk = idx
				reg += reginc
			}
		} else {
			ac.needmove = reginc != 0
			reg += reginc
		}
		namesassigned += 1
	}

	rightreg := reg - 1

	// extra right exprs
	for i := namesassigned; i < lenexprs; i++ {
		varargopt := -1
		if i != lenexprs-1 {
			varargopt = 0
		}
		reg += compileExpr(context, reg, stmt.Rhs[i], ecnone(varargopt))
	}
	return rightreg, acs
} // }}}

func compileAssignStmt(context *funcContext, stmt *ast.AssignStmt) { // {{{
	code := context.Code
	lennames := len(stmt.Lhs)
	reg, acs := compileAssignStmtLeft(context, stmt)
	reg, acs = compileAssignStmtRight(context, stmt, reg, acs)

	for i := lennames - 1; i >= 0; i-- {
		ex := stmt.Lhs[i]
		switch acs[i].ec.ctype {
		case ecLocal:
			if acs[i].needmove {
				code.AddABC(OP_MOVE, context.FindLocalVar(ex.(*ast.IdentExpr).Value), reg, 0, sline(ex))
				reg -= 1
			}
		case ecGlobal:
			code.AddABx(OP_SETGLOBAL, reg, context.ConstIndex(LString(ex.(*ast.IdentExpr).Value)), sline(ex))
			reg -= 1
		case ecUpvalue:
			code.AddABC(OP_SETUPVAL, reg, context.Upvalues.RegisterUnique(ex.(*ast.IdentExpr).Value), 0, sline(ex))
			reg -= 1
		case ecTable:
			opcode := OP_SETTABLE
			if acs[i].keyks {
				opcode = OP_SETTABLEKS
			}
			code.AddABC(opcode, acs[i].ec.reg, acs[i].keyrk, acs[i].valuerk, sline(ex))
			if !opIsK(acs[i].valuerk) {
				reg -= 1
			}
		}
	}
} // }}}

func compileRegAssignment(context *funcContext, names []string, exprs []ast.Expr, reg int, nvars int, line int) { // {{{
	lennames := len(names)
	lenexprs := len(exprs)
	namesassigned := 0
	ec := &expcontext{}

	for namesassigned < lennames && namesassigned < lenexprs {
		if isVarArgReturnExpr(exprs[namesassigned]) && (lenexprs-namesassigned-1) <= 0 {

			varargopt := nvars - namesassigned
			ecupdate(ec, ecVararg, reg, varargopt-1)
			compileExpr(context, reg, exprs[namesassigned], ec)
			reg += varargopt
			namesassigned = lennames
		} else {
			ecupdate(ec, ecLocal, reg, 0)
			compileExpr(context, reg, exprs[namesassigned], ec)
			reg += 1
			namesassigned += 1
		}
	}

	// extra left names
	if lennames > namesassigned {
		restleft := lennames - namesassigned - 1
		context.Code.AddLoadNil(reg, reg+restleft, line)
		reg += restleft
	}

	// extra right exprs
	for i := namesassigned; i < lenexprs; i++ {
		varargopt := -1
		if i != lenexprs-1 {
			varargopt = 0
		}
		ecupdate(ec, ecNone, reg, varargopt)
		reg += compileExpr(context, reg, exprs[i], ec)
	}
} // }}}

func compileLocalAssignStmt(context *funcContext, stmt *ast.LocalAssignStmt) { // {{{
	attrs := stmt.Attrs
	if len(attrs) != len(stmt.Names) {
		attrs = make([]ast.LocalAttr, len(stmt.Names))
	}
	reg := context.RegTop()
	if len(stmt.Names) == 1 && len(stmt.Exprs) == 1 {
		if _, ok := stmt.Exprs[0].(*ast.FunctionExpr); ok {
			context.RegisterLocalVarWithAttr(stmt.Names[0], attrs[0])
			compileRegAssignment(context, stmt.Names, stmt.Exprs, reg, len(stmt.Names), sline(stmt))
			return
		}
	}

	compileRegAssignment(context, stmt.Names, stmt.Exprs, reg, len(stmt.Names), sline(stmt))
	for i, name := range stmt.Names {
		context.RegisterLocalVarWithAttr(name, attrs[i])
	}
} // }}}

func compileReturnStmt(context *funcContext, stmt *ast.ReturnStmt) { // {{{
	lenexprs := len(stmt.Exprs)
	code := context.Code
	reg := context.RegTop()
	a := reg
	lastisvaarg := false

	if lenexprs == 1 {
		switch ex := stmt.Exprs[0].(type) {
		case *ast.IdentExpr:
			if idx := context.FindLocalVar(ex.Value); idx > -1 {
				code.AddABC(OP_RETURN, idx, 2, 0, sline(stmt))
				return
			}
		case *ast.FuncCallExpr:
			if ex.AdjustRet { // return (func())
				reg += compileExpr(context, reg, ex, ecnone(0))
			} else {
				reg += compileExpr(context, reg, ex, ecnone(-2))
				code.SetOpCode(code.LastPC(), OP_TAILCALL)
			}
			code.AddABC(OP_RETURN, a, 0, 0, sline(stmt))
			return
		}
	}

	for i, expr := range stmt.Exprs {
		if i == lenexprs-1 && isVarArgReturnExpr(expr) {
			compileExpr(context, reg, expr, ecnone(-2))
			lastisvaarg = true
		} else {
			reg += compileExpr(context, reg, expr, ecnone(0))
		}
	}
	count := reg - a + 1
	if lastisvaarg {
		count = 0
	}
	context.Code.AddABC(OP_RETURN, a, count, 0, sline(stmt))
} // }}}

func compileIfStmt(context *funcContext, stmt *ast.IfStmt) { // {{{
	thenlabel := context.NewLabel()
	elselabel := context.NewLabel()
	endlabel := context.NewLabel()

	compileBranchCondition(context, context.RegTop(), stmt.Condition, thenlabel, elselabel, false)
	context.SetLabelPc(thenlabel, context.Code.LastPC())
	compileBlock(context, stmt.Then)
	if len(stmt.Else) > 0 {
		context.Code.AddASbx(OP_JMP, 0, endlabel, sline(stmt))
	}
	context.SetLabelPc(elselabel, context.Code.LastPC())
	if len(stmt.Else) > 0 {
		compileBlock(context, stmt.Else)
		context.SetLabelPc(endlabel, context.Code.LastPC())
	}

} // }}}

func compileBranchCondition(context *funcContext, reg int, expr ast.Expr, thenlabel, elselabel int, hasnextcond bool) { // {{{
	// TODO folding constants?
	code := context.Code
	flip := 0
	jumplabel := elselabel
	if hasnextcond {
		flip = 1
		jumplabel = thenlabel
	}

	switch ex := expr.(type) {
	case *ast.FalseExpr, *ast.NilExpr:
		if !hasnextcond {
			code.AddASbx(OP_JMP, 0, elselabel, sline(expr))
			return
		}
	case *ast.TrueExpr, *ast.NumberExpr, *ast.StringExpr:
		if !hasnextcond {
			return
		}
	case *ast.UnaryNotOpExpr:
		compileBranchCondition(context, reg, ex.Expr, elselabel, thenlabel, !hasnextcond)
		return
	case *ast.LogicalOpExpr:
		switch ex.Operator {
		case "and":
			nextcondlabel := context.NewLabel()
			compileBranchCondition(context, reg, ex.Lhs, nextcondlabel, elselabel, false)
			context.SetLabelPc(nextcondlabel, context.Code.LastPC())
			compileBranchCondition(context, reg, ex.Rhs, thenlabel, elselabel, hasnextcond)
		case "or":
			nextcondlabel := context.NewLabel()
			compileBranchCondition(context, reg, ex.Lhs, thenlabel, nextcondlabel, true)
			context.SetLabelPc(nextcondlabel, context.Code.LastPC())
			compileBranchCondition(context, reg, ex.Rhs, thenlabel, elselabel, hasnextcond)
		}
		return
	case *ast.RelationalOpExpr:
		compileRelationalOpExprAux(context, reg, ex, flip, jumplabel)
		return
	}

	a := reg
	compileExprWithMVPropagation(context, expr, &reg, &a)
	code.AddABC(OP_TEST, a, 0, 0^flip, sline(expr))
	code.AddASbx(OP_JMP, 0, jumplabel, sline(expr))
} // }}}

func compileWhileStmt(context *funcContext, stmt *ast.WhileStmt) { // {{{
	thenlabel := context.NewLabel()
	elselabel := context.NewLabel()
	condlabel := context.NewLabel()

	context.SetLabelPc(condlabel, context.Code.LastPC())
	compileBranchCondition(context, context.RegTop(), stmt.Condition, thenlabel, elselabel, false)
	context.SetLabelPc(thenlabel, context.Code.LastPC())
	context.EnterBlock(elselabel, stmt)
	compileChunk(context, stmt.Stmts, false)
	context.CloseUpvalues()
	context.Code.AddASbx(OP_JMP, 0, condlabel, eline(stmt))
	context.LeaveBlock()
	context.SetLabelPc(elselabel, context.Code.LastPC())
} // }}}

func compileRepeatStmt(context *funcContext, stmt *ast.RepeatStmt) { // {{{
	initlabel := context.NewLabel()
	thenlabel := context.NewLabel()
	elselabel := context.NewLabel()

	context.SetLabelPc(initlabel, context.Code.LastPC())
	context.SetLabelPc(elselabel, context.Code.LastPC())
	context.EnterBlock(thenlabel, stmt)
	compileChunk(context, stmt.Stmts, true)
	compileBranchCondition(context, context.RegTop(), stmt.Condition, thenlabel, elselabel, false)

	context.SetLabelPc(thenlabel, context.Code.LastPC())
	n := context.LeaveBlock()

	if n > -1 {
		label := context.NewLabel()
		context.Code.AddASbx(OP_JMP, 0, label, eline(stmt))
		context.SetLabelPc(elselabel, context.Code.LastPC())
		context.Code.AddABC(OP_CLOSE, n, 0, 0, eline(stmt))
		context.Code.AddASbx(OP_JMP, 0, initlabel, eline(stmt))
		context.SetLabelPc(label, context.Code.LastPC())
	}

} // }}}

func compileBreakStmt(context *funcContext, stmt *ast.BreakStmt) { // {{{
	for block := context.Block; block != nil; block = block.Parent {
		if label := block.BreakLabel; label != labelNoJump {
			if block.RefUpvalue || block.HasToBeClosed() {
				context.Code.AddABC(OP_CLOSE, block.Parent.LocalVars.LastIndex(), 0, 0, sline(stmt))
			}
			context.Code.AddASbx(OP_JMP, 0, label, sline(stmt))
			return
		}
	}
	raiseCompileError(context, sline(stmt), "no loop to break")
} // }}}

func compileFuncDefStmt(context *funcContext, stmt *ast.FuncDefStmt) { // {{{
	if stmt.Name.Func == nil {
		reg := context.RegTop()
		var treg, kreg int
		compileExprWithKMVPropagation(context, stmt.Name.Receiver, &reg, &treg)
		kreg = loadRk(context, &reg, stmt.Func, LString(stmt.Name.Method))
		compileExpr(context, reg, stmt.Func, ecfuncdef)
		context.Code.AddABC(OP_SETTABLE, treg, kreg, reg, sline(stmt.Name.Receiver))
	} else {
		astmt := &ast.AssignStmt{Lhs: []ast.Expr{stmt.Name.Func}, Rhs: []ast.Expr{stmt.Func}}
		astmt.SetLine(sline(stmt.Func))
		astmt.SetLastLine(eline(stmt.Func))
		compileAssignStmt(context, astmt)
	}
} // }}}

func compileNumberForStmt(context *funcContext, stmt *ast.NumberForStmt) { // {{{
	code := context.Code
	endlabel := context.NewLabel()
	ec := &expcontext{}

	context.EnterBlock(endlabel, stmt)
	reg := context.RegTop()
	rindex := context.RegisterLocalVar("(for index)")
	ecupdate(ec, ecLocal, rindex, 0)
	compileExpr(context, reg, stmt.Init, ec)

	reg = context.RegTop()
	rlimit := context.RegisterLocalVar("(for limit)")
	ecupdate(ec, ecLocal, rlimit, 0)
	compileExpr(context, reg, stmt.Limit, ec)

	reg = context.RegTop()
	rstep := context.RegisterLocalVar("(for step)")
	if stmt.Step == nil {
		stmt.Step = &ast.NumberExpr{Value: "1"}
		stmt.Step.SetLine(sline(stmt.Init))
	}
	ecupdate(ec, ecLocal, rstep, 0)
	compileExpr(context, reg, stmt.Step, ec)

	code.AddASbx(OP_FORPREP, rindex, 0, sline(stmt))

	context.RegisterLocalVar(stmt.Name)

	bodypc := code.LastPC()
	compileChunk(context, stmt.Stmts, false)

	context.LeaveBlock()

	flpc := code.LastPC()
	code.AddASbx(OP_FORLOOP, rindex, bodypc-(flpc+1), sline(stmt))

	context.SetLabelPc(endlabel, code.LastPC())
	code.SetSbx(bodypc, flpc-bodypc)

} // }}}

func compileGenericForStmt(context *funcContext, stmt *ast.GenericForStmt) { // {{{
	code := context.Code
	endlabel := context.NewLabel()
	bodylabel := context.NewLabel()
	fllabel := context.NewLabel()
	nnames := len(stmt.Names)

	context.EnterBlock(endlabel, stmt)
	rgen := context.RegisterLocalVar("(for generator)")
	context.RegisterLocalVar("(for state)")
	context.RegisterLocalVar("(for control)")

	compileRegAssignment(context, stmt.Names, stmt.Exprs, context.RegTop()-3, 3, sline(stmt))

	code.AddASbx(OP_JMP, 0, fllabel, sline(stmt))

	for _, name := range stmt.Names {
		context.RegisterLocalVar(name)
	}

	context.SetLabelPc(bodylabel, code.LastPC())
	compileChunk(context, stmt.Stmts, false)

	context.LeaveBlock()

	context.SetLabelPc(fllabel, code.LastPC())
	code.AddABC(OP_TFORLOOP, rgen, 0, nnames, sline(stmt))
	code.AddASbx(OP_JMP, 0, bodylabel, sline(stmt))

	context.SetLabelPc(endlabel, code.LastPC())
} // }}}

func compileLabelStmt(context *funcContext, stmt *ast.LabelStmt, isLastStmt bool) { // {{{
	labelId := context.NewLabel()
	label := newLabelDesc(labelId, stmt.Name, context.Code.LastPC(), sline(stmt), context.BlockLocalVarsCount())
	context.AddNamedLabel(label)
	if isLastStmt {
		label.SetNumActiveLocalVars(context.Block.Parent.LocalVarsCount())
	}
	context.ResolveForwardGoto(label)
} // }}}

func compileGotoStmt(context *funcContext, stmt *ast.GotoStmt) { // {{{
	context.Code.AddABC(OP_CLOSE, 0, 0, 0, sline(stmt))
	context.Code.AddASbx(OP_JMP, 0, labelNoJump, sline(stmt))
	label := newLabelDesc(-1, stmt.Label, context.Code.LastPC(), sline(stmt), context.BlockLocalVarsCount())
	context.AddUnresolvedGoto(label)
	context.FindLabel(context.Block, label, context.gotosCount-1)
} // }}}

func compileExpr(context *funcContext, reg int, expr ast.Expr, ec *expcontext) int { // {{{
	code := context.Code
	sreg := savereg(ec, reg)
	sused := 1
	if sreg < reg {
		sused = 0
	}

	switch ex := expr.(type) {
	case *ast.StringExpr:
		code.AddABx(OP_LOADK, sreg, context.ConstIndex(LString(ex.Value)), sline(ex))
		return sused
	case *ast.NumberExpr:
		num, err := parseNumber(ex.Value)
		if err != nil {
			raiseCompileError(context, sline(ex), "invalid uint256 number literal: %s", ex.Value)
		}
		code.AddABx(OP_LOADK, sreg, context.ConstIndex(num), sline(ex))
		return sused
	case *constLValueExpr:
		code.AddABx(OP_LOADK, sreg, context.ConstIndex(ex.Value), sline(ex))
		return sused
	case *ast.NilExpr:
		code.AddLoadNil(sreg, sreg, sline(ex))
		return sused
	case *ast.FalseExpr:
		code.AddABC(OP_LOADBOOL, sreg, 0, 0, sline(ex))
		return sused
	case *ast.TrueExpr:
		code.AddABC(OP_LOADBOOL, sreg, 1, 0, sline(ex))
		return sused
	case *ast.IdentExpr:
		switch getIdentRefType(context, context, ex) {
		case ecGlobal:
			code.AddABx(OP_GETGLOBAL, sreg, context.ConstIndex(LString(ex.Value)), sline(ex))
		case ecUpvalue:
			code.AddABC(OP_GETUPVAL, sreg, context.Upvalues.RegisterUnique(ex.Value), 0, sline(ex))
		case ecLocal:
			b := context.FindLocalVar(ex.Value)
			code.AddABC(OP_MOVE, sreg, b, 0, sline(ex))
		}
		return sused
	case *ast.Comma3Expr:
		if context.Func.IsVarArg == 0 {
			raiseCompileError(context, sline(ex), "cannot use '...' outside a vararg function")
		}
		context.Func.IsVarArg &= ^VarArgNeedsArg
		code.AddABC(OP_VARARG, sreg, 2+ec.varargopt, 0, sline(ex))
		if context.RegTop() > (sreg+2+ec.varargopt) || ec.varargopt < -1 {
			return 0
		}
		return (sreg + 1 + ec.varargopt) - reg
	case *ast.AttrGetExpr:
		a := sreg
		b := reg
		compileExprWithMVPropagation(context, ex.Object, &reg, &b)
		c := reg
		compileExprWithKMVPropagation(context, ex.Key, &reg, &c)
		opcode := OP_GETTABLE
		if _, ok := ex.Key.(*ast.StringExpr); ok {
			opcode = OP_GETTABLEKS
		}
		code.AddABC(opcode, a, b, c, sline(ex))
		return sused
	case *ast.TableExpr:
		compileTableExpr(context, reg, ex, ec)
		return 1
	case *ast.ArithmeticOpExpr:
		compileArithmeticOpExpr(context, reg, ex, ec)
		return sused
	case *ast.StringConcatOpExpr:
		compileStringConcatOpExpr(context, reg, ex, ec)
		return sused
	case *ast.UnaryMinusOpExpr, *ast.UnaryNotOpExpr, *ast.UnaryLenOpExpr, *ast.UnaryBitNotOpExpr:
		compileUnaryOpExpr(context, reg, ex, ec)
		return sused
	case *ast.RelationalOpExpr:
		compileRelationalOpExpr(context, reg, ex, ec)
		return sused
	case *ast.LogicalOpExpr:
		compileLogicalOpExpr(context, reg, ex, ec)
		return sused
	case *ast.FuncCallExpr:
		return compileFuncCallExpr(context, reg, ex, ec)
	case *ast.FunctionExpr:
		childcontext := newFuncContext(context.Func.SourceName, context)
		compileFunctionExpr(childcontext, ex, ec)
		protono := len(context.Func.Functions)
		context.Func.Functions = append(context.Func.Functions, childcontext.Func)
		code.AddABx(OP_CLOSURE, sreg, protono, sline(ex))
		for _, upvalue := range childcontext.Upvalues.List() {
			localidx, block := context.FindLocalVarAndBlock(upvalue.Name)
			if localidx > -1 {
				code.AddABC(OP_MOVE, 0, localidx, 0, sline(ex))
				block.RefUpvalue = true
			} else {
				upvalueidx := context.Upvalues.Find(upvalue.Name)
				if upvalueidx < 0 {
					upvalueidx = context.Upvalues.RegisterUnique(upvalue.Name)
				}
				code.AddABC(OP_GETUPVAL, 0, upvalueidx, 0, sline(ex))
			}
		}
		return sused
	default:
		panic(fmt.Sprintf("expr %v not implemented.", reflect.TypeOf(ex).Elem().Name()))
	}

} // }}}

func compileExprWithPropagation(context *funcContext, expr ast.Expr, reg *int, save *int, propergator func(int, *int, *int, int)) { // {{{
	reginc := compileExpr(context, *reg, expr, ecnone(0))
	if _, ok := expr.(*ast.LogicalOpExpr); ok {
		*save = *reg
		*reg = *reg + reginc
	} else {
		propergator(context.RegTop(), save, reg, reginc)
	}
} // }}}

func compileExprWithKMVPropagation(context *funcContext, expr ast.Expr, reg *int, save *int) { // {{{
	compileExprWithPropagation(context, expr, reg, save, context.Code.PropagateKMV)
} // }}}

func compileExprWithMVPropagation(context *funcContext, expr ast.Expr, reg *int, save *int) { // {{{
	compileExprWithPropagation(context, expr, reg, save, context.Code.PropagateMV)
} // }}}

func constFold(exp ast.Expr) ast.Expr { // {{{
	switch expr := exp.(type) {
	case *ast.ArithmeticOpExpr:
		lvalue, lisconst := lnumberValue(constFold(expr.Lhs))
		rvalue, risconst := lnumberValue(constFold(expr.Rhs))
		if lisconst && risconst {
			switch expr.Operator {
			case "+":
				return &constLValueExpr{Value: lu256Add(lvalue, rvalue)}
			case "-":
				return &constLValueExpr{Value: lu256Sub(lvalue, rvalue)}
			case "*":
				return &constLValueExpr{Value: lu256Mul(lvalue, rvalue)}
			case "/":
				if lu256IsZero(rvalue) {
					return expr // division by zero: skip folding, let runtime raise error
				}
				return &constLValueExpr{Value: lu256Div(lvalue, rvalue)}
			case "%":
				if lu256IsZero(rvalue) {
					return expr // division by zero: skip folding, let runtime raise error
				}
				return &constLValueExpr{Value: lu256Mod(lvalue, rvalue)}
			case "//":
				if lu256IsZero(rvalue) {
					return expr // division by zero: skip folding, let runtime raise error
				}
				return &constLValueExpr{Value: lu256FloorDiv(lvalue, rvalue)}
			case "^":
				return &constLValueExpr{Value: lu256Pow(lvalue, rvalue)}
			case "&":
				return &constLValueExpr{Value: lu256Band(lvalue, rvalue)}
			case "|":
				return &constLValueExpr{Value: lu256Bor(lvalue, rvalue)}
			case "~":
				return &constLValueExpr{Value: lu256Bxor(lvalue, rvalue)}
			case "<<":
				return &constLValueExpr{Value: lu256Shl(lvalue, lu256ShiftAmount(rvalue))}
			case ">>":
				return &constLValueExpr{Value: lu256Shr(lvalue, lu256ShiftAmount(rvalue))}
			default:
				panic(fmt.Sprintf("unknown binop: %v", expr.Operator))
			}
		} else {
			return expr
		}
	case *ast.UnaryMinusOpExpr:
		expr.Expr = constFold(expr.Expr)
		return expr
	case *ast.UnaryBitNotOpExpr:
		expr.Expr = constFold(expr.Expr)
		if v, ok := lnumberValue(expr.Expr); ok {
			return &constLValueExpr{Value: lu256Bnot(v)}
		}
		return expr
	default:

		return exp
	}
} // }}}

func compileFunctionExpr(context *funcContext, funcexpr *ast.FunctionExpr, ec *expcontext) { // {{{
	context.Func.LineDefined = sline(funcexpr)
	context.Func.LastLineDefined = eline(funcexpr)
	if len(funcexpr.ParList.Names) > maxRegisters {
		raiseCompileError(context, context.Func.LineDefined, "register overflow")
	}
	context.Func.NumParameters = uint8(len(funcexpr.ParList.Names))
	if ec.ctype == ecMethod {
		context.Func.NumParameters += 1
		context.RegisterLocalVar("self")
	}
	for _, name := range funcexpr.ParList.Names {
		context.RegisterLocalVar(name)
	}
	if funcexpr.ParList.HasVargs {
		if CompatVarArg {
			context.Func.IsVarArg = VarArgHasArg | VarArgNeedsArg
			if context.Parent != nil {
				context.RegisterLocalVar("arg")
			}
		}
		context.Func.IsVarArg |= VarArgIsVarArg
	}

	compileChunk(context, funcexpr.Stmts, false)

	context.Code.AddABC(OP_RETURN, 0, 1, 0, eline(funcexpr))
	context.EndScope()
	context.CheckUnresolvedGoto()
	context.Func.Instructions = context.Code.List()
	context.Func.DbgSourcePositions = context.Code.PosList()
	context.Func.DbgUpvalues = context.Upvalues.Names()
	context.Func.NumUpvalues = uint8(len(context.Func.DbgUpvalues))
	patchCode(context)
} // }}}

func compileTableExpr(context *funcContext, reg int, ex *ast.TableExpr, ec *expcontext) { // {{{
	code := context.Code
	/*
		tablereg := savereg(ec, reg)
		if tablereg == reg {
			reg += 1
		}
	*/
	tablereg := reg
	reg++
	code.AddABC(OP_NEWTABLE, tablereg, 0, 0, sline(ex))
	tablepc := code.LastPC()
	regbase := reg

	arraycount := 0
	lastvararg := false
	for i, field := range ex.Fields {
		islast := i == len(ex.Fields)-1
		if field.Key == nil {
			if islast && isVarArgReturnExpr(field.Value) {
				reg += compileExpr(context, reg, field.Value, ecnone(-2))
				lastvararg = true
			} else {
				reg += compileExpr(context, reg, field.Value, ecnone(0))
				arraycount += 1
			}
		} else {
			regorg := reg
			b := reg
			compileExprWithKMVPropagation(context, field.Key, &reg, &b)
			c := reg
			compileExprWithKMVPropagation(context, field.Value, &reg, &c)
			opcode := OP_SETTABLE
			if _, ok := field.Key.(*ast.StringExpr); ok {
				opcode = OP_SETTABLEKS
			}
			code.AddABC(opcode, tablereg, b, c, sline(ex))
			reg = regorg
		}
		flush := arraycount % FieldsPerFlush
		if (arraycount != 0 && (flush == 0 || islast)) || lastvararg {
			reg = regbase
			num := flush
			if num == 0 {
				num = FieldsPerFlush
			}
			c := (arraycount-1)/FieldsPerFlush + 1
			b := num
			if islast && isVarArgReturnExpr(field.Value) {
				b = 0
			}
			line := field.Value
			if field.Key != nil {
				line = field.Key
			}
			if c > 511 {
				c = 0
			}
			code.AddABC(OP_SETLIST, tablereg, b, c, sline(line))
			if c == 0 {
				code.AddRawWord(uint32(c), sline(line))
			}
		}
	}
	code.SetB(tablepc, int2Fb(arraycount))
	code.SetC(tablepc, int2Fb(len(ex.Fields)-arraycount))
	if shouldmove(ec, tablereg) {
		code.AddABC(OP_MOVE, ec.reg, tablereg, 0, sline(ex))
	}
} // }}}

func compileArithmeticOpExpr(context *funcContext, reg int, expr *ast.ArithmeticOpExpr, ec *expcontext) { // {{{
	exp := constFold(expr)
	if ex, ok := exp.(*constLValueExpr); ok {
		exp.SetLine(sline(expr))
		compileExpr(context, reg, ex, ec)
		return
	}
	expr, _ = exp.(*ast.ArithmeticOpExpr)
	a := savereg(ec, reg)
	b := reg
	compileExprWithKMVPropagation(context, expr.Lhs, &reg, &b)
	c := reg
	compileExprWithKMVPropagation(context, expr.Rhs, &reg, &c)

	op := 0
	switch expr.Operator {
	case "+":
		op = OP_ADD
	case "-":
		op = OP_SUB
	case "*":
		op = OP_MUL
	case "/":
		op = OP_DIV
	case "//":
		op = OP_IDIV
	case "%":
		op = OP_MOD
	case "^":
		op = OP_POW
	case "&":
		op = OP_BAND
	case "|":
		op = OP_BOR
	case "~":
		op = OP_BXOR
	case "<<":
		op = OP_SHL
	case ">>":
		op = OP_SHR
	}
	context.Code.AddABC(op, a, b, c, sline(expr))
} // }}}

func compileStringConcatOpExpr(context *funcContext, reg int, expr *ast.StringConcatOpExpr, ec *expcontext) { // {{{
	code := context.Code
	crange := 1
	for current := expr.Rhs; current != nil; {
		if ex, ok := current.(*ast.StringConcatOpExpr); ok {
			crange += 1
			current = ex.Rhs
		} else {
			current = nil
		}
	}
	a := savereg(ec, reg)
	basereg := reg
	reg += compileExpr(context, reg, expr.Lhs, ecnone(0))
	reg += compileExpr(context, reg, expr.Rhs, ecnone(0))
	for pc := code.LastPC(); pc != 0 && code.At(pc).Op == OP_CONCAT; pc-- {
		code.Pop()
	}
	code.AddABC(OP_CONCAT, a, basereg, basereg+crange, sline(expr))
} // }}}

func compileUnaryOpExpr(context *funcContext, reg int, expr ast.Expr, ec *expcontext) { // {{{
	opcode := 0
	code := context.Code
	var operandexpr ast.Expr
	switch ex := expr.(type) {
	case *ast.UnaryMinusOpExpr:
		exp := constFold(ex)
		if lvexpr, ok := exp.(*constLValueExpr); ok {
			exp.SetLine(sline(expr))
			compileExpr(context, reg, lvexpr, ec)
			return
		}
		ex, _ = exp.(*ast.UnaryMinusOpExpr)
		operandexpr = ex.Expr
		opcode = OP_UNM
	case *ast.UnaryNotOpExpr:
		switch ex.Expr.(type) {
		case *ast.TrueExpr:
			code.AddABC(OP_LOADBOOL, savereg(ec, reg), 0, 0, sline(expr))
			return
		case *ast.FalseExpr, *ast.NilExpr:
			code.AddABC(OP_LOADBOOL, savereg(ec, reg), 1, 0, sline(expr))
			return
		default:
			opcode = OP_NOT
			operandexpr = ex.Expr
		}
	case *ast.UnaryLenOpExpr:
		opcode = OP_LEN
		operandexpr = ex.Expr
	case *ast.UnaryBitNotOpExpr:
		exp := constFold(ex)
		if lvexpr, ok := exp.(*constLValueExpr); ok {
			exp.SetLine(sline(expr))
			compileExpr(context, reg, lvexpr, ec)
			return
		}
		ex, _ = exp.(*ast.UnaryBitNotOpExpr)
		opcode = OP_BNOT
		operandexpr = ex.Expr
	}

	a := savereg(ec, reg)
	b := reg
	compileExprWithMVPropagation(context, operandexpr, &reg, &b)
	code.AddABC(opcode, a, b, 0, sline(expr))
} // }}}

func compileRelationalOpExprAux(context *funcContext, reg int, expr *ast.RelationalOpExpr, flip int, label int) { // {{{
	code := context.Code
	b := reg
	compileExprWithKMVPropagation(context, expr.Lhs, &reg, &b)
	c := reg
	compileExprWithKMVPropagation(context, expr.Rhs, &reg, &c)
	switch expr.Operator {
	case "<":
		code.AddABC(OP_LT, 0^flip, b, c, sline(expr))
	case ">":
		code.AddABC(OP_LT, 0^flip, c, b, sline(expr))
	case "<=":
		code.AddABC(OP_LE, 0^flip, b, c, sline(expr))
	case ">=":
		code.AddABC(OP_LE, 0^flip, c, b, sline(expr))
	case "==":
		code.AddABC(OP_EQ, 0^flip, b, c, sline(expr))
	case "~=":
		code.AddABC(OP_EQ, 1^flip, b, c, sline(expr))
	}
	code.AddASbx(OP_JMP, 0, label, sline(expr))
} // }}}

func compileRelationalOpExpr(context *funcContext, reg int, expr *ast.RelationalOpExpr, ec *expcontext) { // {{{
	a := savereg(ec, reg)
	code := context.Code
	jumplabel := context.NewLabel()
	compileRelationalOpExprAux(context, reg, expr, 1, jumplabel)
	code.AddABC(OP_LOADBOOL, a, 0, 1, sline(expr))
	context.SetLabelPc(jumplabel, code.LastPC())
	code.AddABC(OP_LOADBOOL, a, 1, 0, sline(expr))
} // }}}

func compileLogicalOpExpr(context *funcContext, reg int, expr *ast.LogicalOpExpr, ec *expcontext) { // {{{
	a := savereg(ec, reg)
	code := context.Code
	endlabel := context.NewLabel()
	lb := &lblabels{context.NewLabel(), context.NewLabel(), endlabel, false}
	nextcondlabel := context.NewLabel()
	if expr.Operator == "and" {
		compileLogicalOpExprAux(context, reg, expr.Lhs, ec, nextcondlabel, endlabel, false, lb)
		context.SetLabelPc(nextcondlabel, code.LastPC())
		compileLogicalOpExprAux(context, reg, expr.Rhs, ec, endlabel, endlabel, false, lb)
	} else {
		compileLogicalOpExprAux(context, reg, expr.Lhs, ec, endlabel, nextcondlabel, true, lb)
		context.SetLabelPc(nextcondlabel, code.LastPC())
		compileLogicalOpExprAux(context, reg, expr.Rhs, ec, endlabel, endlabel, false, lb)
	}

	if lb.b {
		context.SetLabelPc(lb.f, code.LastPC())
		code.AddABC(OP_LOADBOOL, a, 0, 1, sline(expr))
		context.SetLabelPc(lb.t, code.LastPC())
		code.AddABC(OP_LOADBOOL, a, 1, 0, sline(expr))
	}

	lastinst := code.Last()
	if lastinst.Op == OP_JMP && lastinst.Sbx == endlabel {
		code.Pop()
	}

	context.SetLabelPc(endlabel, code.LastPC())
} // }}}

func compileLogicalOpExprAux(context *funcContext, reg int, expr ast.Expr, ec *expcontext, thenlabel, elselabel int, hasnextcond bool, lb *lblabels) { // {{{
	// TODO folding constants?
	code := context.Code
	flip := 0
	jumplabel := elselabel
	if hasnextcond {
		flip = 1
		jumplabel = thenlabel
	}

	switch ex := expr.(type) {
	case *ast.FalseExpr:
		if elselabel == lb.e {
			code.AddASbx(OP_JMP, 0, lb.f, sline(expr))
			lb.b = true
		} else {
			code.AddASbx(OP_JMP, 0, elselabel, sline(expr))
		}
		return
	case *ast.NilExpr:
		if elselabel == lb.e {
			compileExpr(context, reg, expr, ec)
			code.AddASbx(OP_JMP, 0, lb.e, sline(expr))
		} else {
			code.AddASbx(OP_JMP, 0, elselabel, sline(expr))
		}
		return
	case *ast.TrueExpr:
		if thenlabel == lb.e {
			code.AddASbx(OP_JMP, 0, lb.t, sline(expr))
			lb.b = true
		} else {
			code.AddASbx(OP_JMP, 0, thenlabel, sline(expr))
		}
		return
	case *ast.NumberExpr, *ast.StringExpr:
		if thenlabel == lb.e {
			compileExpr(context, reg, expr, ec)
			code.AddASbx(OP_JMP, 0, lb.e, sline(expr))
		} else {
			code.AddASbx(OP_JMP, 0, thenlabel, sline(expr))
		}
		return
	case *ast.LogicalOpExpr:
		switch ex.Operator {
		case "and":
			nextcondlabel := context.NewLabel()
			compileLogicalOpExprAux(context, reg, ex.Lhs, ec, nextcondlabel, elselabel, false, lb)
			context.SetLabelPc(nextcondlabel, context.Code.LastPC())
			compileLogicalOpExprAux(context, reg, ex.Rhs, ec, thenlabel, elselabel, hasnextcond, lb)
		case "or":
			nextcondlabel := context.NewLabel()
			compileLogicalOpExprAux(context, reg, ex.Lhs, ec, thenlabel, nextcondlabel, true, lb)
			context.SetLabelPc(nextcondlabel, context.Code.LastPC())
			compileLogicalOpExprAux(context, reg, ex.Rhs, ec, thenlabel, elselabel, hasnextcond, lb)
		}
		return
	case *ast.RelationalOpExpr:
		if thenlabel == elselabel {
			flip ^= 1
			jumplabel = lb.t
			lb.b = true
		} else if thenlabel == lb.e {
			jumplabel = lb.t
			lb.b = true
		} else if elselabel == lb.e {
			jumplabel = lb.f
			lb.b = true
		}
		compileRelationalOpExprAux(context, reg, ex, flip, jumplabel)
		return
	}

	a := reg
	sreg := savereg(ec, a)
	isLastAnd := elselabel == lb.e && thenlabel != elselabel
	isLastOr := thenlabel == lb.e && hasnextcond

	if ident, ok := expr.(*ast.IdentExpr); ok && (isLastAnd || isLastOr) && getIdentRefType(context, context, ident) == ecLocal {
		b := context.FindLocalVar(ident.Value)
		op := OP_TESTSET
		if sreg == b {
			op = OP_TEST
		}
		code.AddABC(op, sreg, b, 0^flip, sline(expr))
	} else if !hasnextcond && thenlabel == elselabel {
		reg += compileExpr(context, reg, expr, &expcontext{ec.ctype, intMax(a, sreg), ec.varargopt})
		last := context.Code.Last()
		if last.Op == OP_MOVE && last.A == a {
			context.Code.SetA(context.Code.LastPC(), sreg)
		} else {
			context.Code.AddABC(OP_MOVE, sreg, a, 0, sline(expr))
		}
	} else {
		reg += compileExpr(context, reg, expr, ecnone(0))
		if !hasnextcond {
			code.AddABC(OP_TEST, a, 0, 0^flip, sline(expr))
		} else {
			code.AddABC(OP_TESTSET, sreg, a, 0^flip, sline(expr))
		}
	}
	code.AddASbx(OP_JMP, 0, jumplabel, sline(expr))
} // }}}

// argsReferenceLocalReg returns true if any top-level IdentExpr in args
// refers to a local variable currently allocated at register targetReg.
// This is used to guard the function-call register-aliasing optimisation:
// when an argument reads the same register that we are about to overwrite
// with the function reference, the optimisation is unsafe.
func argsReferenceLocalReg(context *funcContext, args []ast.Expr, targetReg int) bool {
	for _, arg := range args {
		if ident, ok := arg.(*ast.IdentExpr); ok {
			if context.FindLocalVar(ident.Value) == targetReg {
				return true
			}
		}
	}
	return false
}

func compileFuncCallExpr(context *funcContext, reg int, expr *ast.FuncCallExpr, ec *expcontext) int { // {{{
	funcreg := reg
	if ec.ctype == ecLocal && ec.reg == (int(context.Func.NumParameters)-1) &&
		!argsReferenceLocalReg(context, expr.Args, ec.reg) {
		funcreg = ec.reg
		reg = ec.reg
	}
	argc := len(expr.Args)
	islastvararg := false
	name := "(anonymous)"

	if expr.Func != nil { // hoge.func()
		reg += compileExpr(context, reg, expr.Func, ecnone(0))
		name = getExprName(context, expr.Func)
	} else { // hoge:method()
		b := reg
		compileExprWithMVPropagation(context, expr.Receiver, &reg, &b)
		c := loadRk(context, &reg, expr, LString(expr.Method))
		context.Code.AddABC(OP_SELF, funcreg, b, c, sline(expr))
		// increments a register for an implicit "self"
		reg = b + 1
		reg2 := funcreg + 2
		if reg2 > reg {
			reg = reg2
		}
		argc += 1
		name = string(expr.Method)
	}

	for i, ar := range expr.Args {
		islastvararg = (i == len(expr.Args)-1) && isVarArgReturnExpr(ar)
		if islastvararg {
			compileExpr(context, reg, ar, ecnone(-2))
		} else {
			reg += compileExpr(context, reg, ar, ecnone(0))
		}
	}
	b := argc + 1
	if islastvararg {
		b = 0
	}
	context.Code.AddABC(OP_CALL, funcreg, b, ec.varargopt+2, sline(expr))
	context.Func.DbgCalls = append(context.Func.DbgCalls, DbgCall{Pc: context.Code.LastPC(), Name: name})

	if ec.varargopt == 0 && shouldmove(ec, funcreg) {
		context.Code.AddABC(OP_MOVE, ec.reg, funcreg, 0, sline(expr))
		return 1
	}
	if context.RegTop() > (funcreg+2+ec.varargopt) || ec.varargopt < -1 {
		return 0
	}
	return ec.varargopt + 1
} // }}}

func loadRk(context *funcContext, reg *int, expr ast.Expr, cnst LValue) int { // {{{
	cindex := context.ConstIndex(cnst)
	if cindex <= opMaxIndexRk {
		return opRkAsk(cindex)
	} else {
		ret := *reg
		*reg++
		context.Code.AddABx(OP_LOADK, ret, cindex, sline(expr))
		return ret
	}
} // }}}

func getIdentRefType(context *funcContext, current *funcContext, expr *ast.IdentExpr) expContextType { // {{{
	if current == nil {
		return ecGlobal
	} else if current.FindLocalVar(expr.Value) > -1 {
		if current == context {
			return ecLocal
		}
		return ecUpvalue
	}
	return getIdentRefType(context, current.Parent, expr)
} // }}}

func getExprName(context *funcContext, expr ast.Expr) string { // {{{
	switch ex := expr.(type) {
	case *ast.IdentExpr:
		return ex.Value
	case *ast.AttrGetExpr:
		switch kex := ex.Key.(type) {
		case *ast.StringExpr:
			return kex.Value
		}
		return "?"
	}
	return "?"
} // }}}

func patchCode(context *funcContext) { // {{{
	maxreg := 1
	if np := int(context.Func.NumParameters); np > 1 {
		maxreg = np
	}
	moven := 0
	code := context.Code.List()
	for pc := 0; pc < len(code); pc++ {
		inst := code[pc]
		curop := inst.Op
		switch curop {
		case OP_CLOSURE:
			pc += int(context.Func.Functions[inst.Bx].NumUpvalues)
			moven = 0
			continue
		case OP_SETGLOBAL, OP_SETUPVAL, OP_EQ, OP_LT, OP_LE, OP_TEST,
			OP_TAILCALL, OP_RETURN, OP_FORPREP, OP_FORLOOP, OP_TFORLOOP,
			OP_SETLIST, OP_CLOSE:
			/* nothing to do */
		case OP_CALL:
			if reg := inst.A + inst.C - 2; reg > maxreg {
				maxreg = reg
			}
		case OP_VARARG:
			if reg := inst.A + inst.B - 1; reg > maxreg {
				maxreg = reg
			}
		case OP_SELF:
			if reg := inst.A + 1; reg > maxreg {
				maxreg = reg
			}
		case OP_LOADNIL:
			if reg := inst.B; reg > maxreg {
				maxreg = reg
			}
		case OP_JMP: // jump to jump optimization
			distance := 0
			count := 0 // avoiding infinite loops
			for jmp := inst; jmp.Op == OP_JMP && count < 5; jmp = context.Code.At(pc + distance + 1) {
				d := context.GetLabelPc(jmp.Sbx) - pc
				if d > opMaxArgSbx {
					if distance == 0 {
						raiseCompileError(context, context.Func.LineDefined, "too long to jump.")
					}
					break
				}
				distance = d
				count++
			}
			if distance == 0 {
				context.Code.SetOpCode(pc, OP_NOP)
			} else {
				context.Code.SetSbx(pc, distance)
			}
		default:
			if reg := inst.A; reg > maxreg {
				maxreg = reg
			}
		}

		// bulk move optimization(reducing op dipatch costs)
		if curop == OP_MOVE {
			moven++
		} else {
			if moven > 1 {
				context.Code.SetOpCode(pc-moven, OP_MOVEN)
				context.Code.SetC(pc-moven, intMin(moven-1, opMaxArgsC))
			}
			moven = 0
		}
	}
	maxreg++
	if maxreg > maxRegisters {
		raiseCompileError(context, context.Func.LineDefined, "register overflow(too many local variables)")
	}
	context.Func.NumUsedRegs = uint8(maxreg)
} // }}}

func compileASTToIRDirect(chunk []ast.Stmt, name string) (root *IRFunction, err error) { // {{{
	defer func() {
		if rcv := recover(); rcv != nil {
			if _, ok := rcv.(*CompileError); ok {
				err = rcv.(error)
			} else {
				panic(rcv)
			}
		}
	}()
	err = nil
	parlist := &ast.ParList{HasVargs: true, Names: []string{}}
	funcexpr := &ast.FunctionExpr{ParList: parlist, Stmts: chunk}
	if len(chunk) > 0 {
		funcexpr.SetLastLine(sline(chunk[0]))
		funcexpr.SetLastLine(eline(chunk[len(chunk)-1]) + 1)
	}
	context := newFuncContext(name, nil)
	compileFunctionExpr(context, funcexpr, ecnone(0))
	root = context.Func
	return
} // }}}

// Compile compiles AST statements using a multi-stage pipeline:
// AST -> IR -> bytecode(FunctionProto).
func Compile(chunk []ast.Stmt, name string) (proto *FunctionProto, err error) {
	program, err := buildIRFromChunk(chunk, name)
	if err != nil {
		return nil, err
	}
	return CompileIR(program)
}
