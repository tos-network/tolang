// Package format implements canonical formatting for TOL source files.
//
// The formatter parses source into an AST, then walks the AST to emit
// canonically formatted code. Style is opinionated and not configurable:
//   - 4-space indentation
//   - Opening brace on same line
//   - Semicolons after statements
//   - One blank line between top-level declarations
//   - Doc comments (///) preserved and attached to declarations
package format

import (
	"fmt"
	"strings"

	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/parser"
)

const indent = "    " // 4 spaces

// Format parses the TOL source and returns canonically formatted output.
// If the source has parse errors, the first error is returned.
func Format(source []byte, filename string) ([]byte, error) {
	mod, diags := parser.ParseFile(filename, source)
	if diags.HasErrors() {
		return nil, fmt.Errorf("%s", diags.Error())
	}
	f := &formatter{}
	f.formatModule(mod)
	return []byte(f.buf.String()), nil
}

type formatter struct {
	buf   strings.Builder
	depth int // current indentation depth
}

func (f *formatter) indent() string {
	return strings.Repeat(indent, f.depth)
}

func (f *formatter) writef(format string, args ...interface{}) {
	fmt.Fprintf(&f.buf, format, args...)
}

func (f *formatter) writeIndent() {
	f.buf.WriteString(f.indent())
}

func (f *formatter) writeLine(s string) {
	f.writeIndent()
	f.buf.WriteString(s)
	f.buf.WriteByte('\n')
}

// --- Module ---

func (f *formatter) formatModule(mod *ast.Module) {
	if mod.Version != "" {
		f.writef("pragma tolang %s;\n", mod.Version)
	}

	if mod.Package != "" {
		f.writef("\npackage %s;\n", mod.Package)
	}

	for _, imp := range mod.Imports {
		f.buf.WriteByte('\n')
		f.formatImport(&imp)
	}

	// Top-level type declarations
	for _, td := range mod.TypeDecls {
		f.buf.WriteByte('\n')
		f.writef("type %s is %s;\n", td.Name, td.Underlying)
	}

	// Top-level enums
	for _, en := range mod.Enums {
		f.buf.WriteByte('\n')
		f.formatEnum(&en, 0)
	}

	// Top-level constants
	for _, c := range mod.Constants {
		f.buf.WriteByte('\n')
		f.formatConstant(&c, 0)
	}

	// Top-level errors
	for _, e := range mod.Errors {
		f.buf.WriteByte('\n')
		f.formatError(&e, 0)
	}

	// Top-level events
	for _, ev := range mod.Events {
		f.buf.WriteByte('\n')
		f.formatEvent(&ev, 0)
	}

	// Top-level using declarations
	for _, u := range mod.UsingDecls {
		f.buf.WriteByte('\n')
		f.writef("using %s for %s;\n", u.Library, u.Type)
	}

	// Top-level structs
	for _, s := range mod.Structs {
		f.buf.WriteByte('\n')
		f.formatStruct(&s, 0)
	}

	// Top-level capabilities
	for _, cap := range mod.Capabilities {
		f.buf.WriteByte('\n')
		f.writef("capability %s;\n", cap.Name)
	}

	// Top-level free functions
	for _, fn := range mod.FreeFunctions {
		f.buf.WriteByte('\n')
		f.formatFunction(&fn, 0)
	}

	// Interfaces
	for _, iface := range mod.Interfaces {
		f.buf.WriteByte('\n')
		f.formatInterface(&iface)
	}

	// Libraries
	for _, lib := range mod.Libraries {
		f.buf.WriteByte('\n')
		f.formatLibrary(&lib)
	}

	// Abstract contracts
	for _, c := range mod.AbstractContracts {
		f.buf.WriteByte('\n')
		f.formatContract(&c)
	}

	// Concrete contracts
	for _, c := range mod.Contracts {
		f.buf.WriteByte('\n')
		f.formatContract(&c)
	}

	// Legacy single contract (if Contracts is empty)
	if len(mod.Contracts) == 0 && mod.Contract != nil {
		f.buf.WriteByte('\n')
		f.formatContract(mod.Contract)
	}

	// Tests
	for _, t := range mod.Tests {
		f.buf.WriteByte('\n')
		f.formatTest(&t)
	}
}

// --- Import ---

func (f *formatter) formatImport(imp *ast.ImportDecl) {
	if imp.IsPackageImport {
		s := imp.PackagePath + "." + imp.PackageContract
		if imp.Name != "" && imp.Name != imp.PackageContract {
			f.writef("import %s as %s;\n", s, imp.Name)
		} else {
			f.writef("import %s;\n", s)
		}
		return
	}
	if imp.IsStar {
		f.writef("import * as %s from %q;\n", imp.Name, imp.Path)
		return
	}
	if len(imp.Named) > 0 {
		parts := make([]string, len(imp.Named))
		for i, n := range imp.Named {
			if n.Alias != "" {
				parts[i] = fmt.Sprintf("%s as %s", n.Name, n.Alias)
			} else {
				parts[i] = n.Name
			}
		}
		f.writef("import { %s } from %q;\n", strings.Join(parts, ", "), imp.Path)
		return
	}
	if imp.Alias != "" {
		f.writef("import %q as %s;\n", imp.Path, imp.Alias)
		return
	}
	if imp.Name != "" {
		f.writef("import %s from %q;\n", imp.Name, imp.Path)
		return
	}
	f.writef("import %q;\n", imp.Path)
}

// --- Interface ---

func (f *formatter) formatInterface(iface *ast.InterfaceDecl) {
	f.writef("interface %s {\n", iface.Name)
	f.depth++

	for _, td := range iface.TypeDecls {
		f.writeLine(fmt.Sprintf("type %s is %s;", td.Name, td.Underlying))
	}
	for _, s := range iface.Structs {
		f.formatStruct(&s, f.depth)
	}
	for _, en := range iface.Enums {
		f.formatEnum(&en, f.depth)
	}
	for _, c := range iface.Constants {
		f.formatConstant(&c, f.depth)
	}
	for _, u := range iface.UsingDecls {
		f.writeLine(fmt.Sprintf("using %s for %s;", u.Library, u.Type))
	}
	for _, e := range iface.Errors {
		f.formatError(&e, f.depth)
	}
	for _, ev := range iface.Events {
		f.formatEvent(&ev, f.depth)
	}

	for i, fn := range iface.Functions {
		if i > 0 || len(iface.Events)+len(iface.Errors)+len(iface.Enums)+len(iface.Structs)+len(iface.TypeDecls)+len(iface.Constants)+len(iface.UsingDecls) > 0 {
			f.buf.WriteByte('\n')
		}
		f.formatFuncSig(&fn)
	}

	f.depth--
	f.writeLine("}")
}

func (f *formatter) formatFuncSig(fn *ast.FuncSigDecl) {
	if fn.Doc != nil {
		f.writeDocComment(fn.Doc)
	}
	f.writeIndent()
	f.buf.WriteString("function ")
	f.buf.WriteString(fn.Name)
	f.buf.WriteByte('(')
	f.writeParams(fn.Params)
	f.buf.WriteByte(')')
	f.writeModifiers(fn.Modifiers)
	if fn.PayableAsset != "" {
		// already included in modifiers as "payable(uno)" etc.
	}
	if len(fn.Returns) > 0 {
		f.buf.WriteString(" returns (")
		f.writeReturns(fn.Returns)
		f.buf.WriteByte(')')
	}
	f.buf.WriteString(";\n")
}

// --- Library ---

func (f *formatter) formatLibrary(lib *ast.LibraryDecl) {
	f.writef("library %s {\n", lib.Name)
	f.depth++

	needSep := false
	for _, td := range lib.TypeDecls {
		f.writeLine(fmt.Sprintf("type %s is %s;", td.Name, td.Underlying))
		needSep = true
	}
	for _, s := range lib.Structs {
		if needSep {
			f.buf.WriteByte('\n')
		}
		f.formatStruct(&s, f.depth)
		needSep = true
	}
	for _, en := range lib.Enums {
		if needSep {
			f.buf.WriteByte('\n')
		}
		f.formatEnum(&en, f.depth)
		needSep = true
	}
	for _, c := range lib.Constants {
		f.formatConstant(&c, f.depth)
		needSep = true
	}
	for _, u := range lib.UsingDecls {
		f.writeLine(fmt.Sprintf("using %s for %s;", u.Library, u.Type))
		needSep = true
	}
	for _, e := range lib.Errors {
		f.formatError(&e, f.depth)
		needSep = true
	}
	for _, ev := range lib.Events {
		f.formatEvent(&ev, f.depth)
		needSep = true
	}

	for i, fn := range lib.Functions {
		if i > 0 || needSep {
			f.buf.WriteByte('\n')
		}
		f.formatFunction(&fn, f.depth)
	}

	f.depth--
	f.writeLine("}")
}

// --- Contract ---

func (f *formatter) formatContract(c *ast.ContractDecl) {
	prefix := "contract"
	if c.Abstract {
		prefix = "abstract contract"
	}
	if c.IsAccount {
		prefix = "account contract"
	}

	f.buf.WriteString(prefix)
	f.writef(" %s", c.Name)

	if len(c.BaseSpecifiers) > 0 {
		f.buf.WriteString(" is ")
		for i, bs := range c.BaseSpecifiers {
			if i > 0 {
				f.buf.WriteString(", ")
			}
			f.buf.WriteString(bs.Name)
			if len(bs.Args) > 0 {
				f.buf.WriteByte('(')
				for j, arg := range bs.Args {
					if j > 0 {
						f.buf.WriteString(", ")
					}
					f.formatExpr(arg)
				}
				f.buf.WriteByte(')')
			}
		}
	} else if len(c.Bases) > 0 {
		f.buf.WriteString(" is ")
		f.buf.WriteString(strings.Join(c.Bases, ", "))
	}

	f.buf.WriteString(" {\n")
	f.depth++

	needSep := false

	// Manifest
	if c.Manifest != nil {
		if needSep {
			f.buf.WriteByte('\n')
		}
		f.formatManifest(c.Manifest)
		needSep = true
	}

	// Storage
	if c.Storage != nil && len(c.Storage.Slots) > 0 {
		if needSep {
			f.buf.WriteByte('\n')
		}
		for _, slot := range c.Storage.Slots {
			f.formatStorageSlot(&slot)
		}
		needSep = true
	}

	// Immutables
	for _, imm := range c.Immutables {
		if needSep {
			f.buf.WriteByte('\n')
			needSep = false
		}
		f.writeLine(fmt.Sprintf("immutable %s: %s;", imm.Name, imm.Type))
		needSep = true
	}

	// Type declarations
	for _, td := range c.TypeDecls {
		if needSep {
			f.buf.WriteByte('\n')
			needSep = false
		}
		f.writeLine(fmt.Sprintf("type %s is %s;", td.Name, td.Underlying))
		needSep = true
	}

	// Constants
	if len(c.Constants) > 0 {
		if needSep {
			f.buf.WriteByte('\n')
		}
		for _, con := range c.Constants {
			f.formatConstant(&con, f.depth)
		}
		needSep = true
	}

	// Enums
	for _, en := range c.Enums {
		if needSep {
			f.buf.WriteByte('\n')
		}
		f.formatEnum(&en, f.depth)
		needSep = true
	}

	// Structs
	for _, s := range c.Structs {
		if needSep {
			f.buf.WriteByte('\n')
		}
		f.formatStruct(&s, f.depth)
		needSep = true
	}

	// Using declarations
	for _, u := range c.UsingDecls {
		if needSep {
			f.buf.WriteByte('\n')
			needSep = false
		}
		f.writeLine(fmt.Sprintf("using %s for %s;", u.Library, u.Type))
		needSep = true
	}

	// Errors
	if len(c.Errors) > 0 {
		if needSep {
			f.buf.WriteByte('\n')
		}
		for _, e := range c.Errors {
			f.formatError(&e, f.depth)
		}
		needSep = true
	}

	// Events
	if len(c.Events) > 0 {
		if needSep {
			f.buf.WriteByte('\n')
		}
		for _, ev := range c.Events {
			f.formatEvent(&ev, f.depth)
		}
		needSep = true
	}

	// Capabilities
	for _, cap := range c.Capabilities {
		if needSep {
			f.buf.WriteByte('\n')
			needSep = false
		}
		f.writeLine(fmt.Sprintf("capability %s;", cap.Name))
		needSep = true
	}

	// Purposes
	for _, p := range c.Purposes {
		if needSep {
			f.buf.WriteByte('\n')
			needSep = false
		}
		f.writeLine(fmt.Sprintf("purpose %s;", p.Name))
		needSep = true
	}

	// Modifiers
	for _, mod := range c.Modifiers {
		if needSep {
			f.buf.WriteByte('\n')
		}
		f.formatModifier(&mod)
		needSep = true
	}

	// Constructor
	if c.Constructor != nil {
		if needSep {
			f.buf.WriteByte('\n')
		}
		f.formatConstructor(c.Constructor)
		needSep = true
	}

	// Functions
	for _, fn := range c.Functions {
		if needSep {
			f.buf.WriteByte('\n')
		}
		f.formatFunction(&fn, f.depth)
		needSep = true
	}

	// Fallback
	if c.Fallback != nil {
		if needSep {
			f.buf.WriteByte('\n')
		}
		f.formatFallback(c.Fallback)
		needSep = true
	}

	// Receive
	if c.Receive != nil {
		if needSep {
			f.buf.WriteByte('\n')
		}
		f.formatReceive(c.Receive)
		needSep = true
	}

	f.depth--
	f.writeLine("}")
}

// --- Manifest ---

func (f *formatter) formatManifest(m *ast.ManifestDecl) {
	f.writeLine("manifest {")
	f.depth++
	for _, field := range m.Fields {
		f.writeIndent()
		if field.IsArray {
			f.writef("%s: [%s];\n", field.Key, strings.Join(field.Array, ", "))
		} else {
			// If value looks like it should be quoted (has spaces, or is a known string field)
			f.writef("%s: %s;\n", field.Key, formatManifestValue(field))
		}
	}
	f.depth--
	f.writeLine("}")
}

func formatManifestValue(field ast.ManifestField) string {
	// Value already includes surrounding quotes for strings (from the lexer).
	return field.Value
}

// --- Storage ---

func (f *formatter) formatStorageSlot(slot *ast.StorageSlot) {
	f.writeIndent()
	if slot.IsTransient {
		f.buf.WriteString("transient ")
	}
	if slot.Visibility != "" && slot.Visibility != "internal" {
		f.buf.WriteString(slot.Visibility)
		f.buf.WriteByte(' ')
	}
	if slot.Override {
		f.buf.WriteString("override ")
	}
	f.buf.WriteString(slot.Type)
	f.buf.WriteByte(' ')
	f.buf.WriteString(slot.Name)
	if slot.InitExpr != nil {
		f.buf.WriteString(" = ")
		f.formatExpr(slot.InitExpr)
	}
	f.buf.WriteString(";\n")
}

// --- Event ---

func (f *formatter) formatEvent(ev *ast.EventDecl, depth int) {
	oldDepth := f.depth
	f.depth = depth
	f.writeIndent()
	f.buf.WriteString("event ")
	f.buf.WriteString(ev.Name)
	f.buf.WriteByte('(')
	for i, p := range ev.Params {
		if i > 0 {
			f.buf.WriteString(", ")
		}
		f.buf.WriteString(p.Type)
		f.buf.WriteByte(' ')
		f.buf.WriteString(p.Name)
		if p.Indexed {
			f.buf.WriteString(" indexed")
		}
	}
	f.buf.WriteByte(')')
	if ev.Anonymous {
		f.buf.WriteString(" anonymous")
	}
	f.buf.WriteByte('\n')
	f.depth = oldDepth
}

// --- Error ---

func (f *formatter) formatError(e *ast.ErrorDecl, depth int) {
	oldDepth := f.depth
	f.depth = depth
	f.writeIndent()
	f.buf.WriteString("error ")
	f.buf.WriteString(e.Name)
	f.buf.WriteByte('(')
	f.writeParams(e.Params)
	f.buf.WriteString(");\n")
	f.depth = oldDepth
}

// --- Enum ---

func (f *formatter) formatEnum(en *ast.EnumDecl, depth int) {
	oldDepth := f.depth
	f.depth = depth
	f.writeLine(fmt.Sprintf("enum %s { %s }", en.Name, strings.Join(en.Members, ", ")))
	f.depth = oldDepth
}

// --- Struct ---

func (f *formatter) formatStruct(s *ast.StructDecl, depth int) {
	oldDepth := f.depth
	f.depth = depth
	f.writeLine(fmt.Sprintf("struct %s {", s.Name))
	f.depth++
	for _, field := range s.Fields {
		f.writeLine(fmt.Sprintf("%s: %s;", field.Name, field.Type))
	}
	f.depth--
	f.writeLine("}")
	f.depth = oldDepth
}

// --- Constant ---

func (f *formatter) formatConstant(c *ast.ConstantDecl, depth int) {
	oldDepth := f.depth
	f.depth = depth
	f.writeIndent()
	f.writef("constant %s: %s = ", c.Name, c.Type)
	if c.Value != nil {
		f.formatExpr(c.Value)
	}
	f.buf.WriteString(";\n")
	f.depth = oldDepth
}

// --- Modifier ---

func (f *formatter) formatModifier(mod *ast.ModifierDecl) {
	f.writeIndent()
	f.buf.WriteString("modifier ")
	f.buf.WriteString(mod.Name)
	if len(mod.Params) > 0 {
		f.buf.WriteByte('(')
		f.writeParams(mod.Params)
		f.buf.WriteByte(')')
	}
	if mod.Virtual {
		f.buf.WriteString(" virtual")
	}
	if mod.Override {
		f.buf.WriteString(" override")
	}
	if mod.Abstract {
		f.buf.WriteString(";\n")
		return
	}
	f.buf.WriteString(" {\n")
	f.depth++
	f.formatStatements(mod.Body)
	f.depth--
	f.writeLine("}")
}

// --- Constructor ---

func (f *formatter) formatConstructor(c *ast.ConstructorDecl) {
	if c.Doc != nil {
		f.writeDocComment(c.Doc)
	}
	f.writeIndent()
	f.buf.WriteString("constructor(")
	f.writeParams(c.Params)
	f.buf.WriteByte(')')
	f.writeModifiers(c.Modifiers)
	if c.PayableAsset != "" {
		// payable(uno) etc. should already be in modifiers
	}
	f.buf.WriteString(" {\n")
	f.depth++
	f.formatStatements(c.Body)
	f.depth--
	f.writeLine("}")
}

// --- Function ---

func (f *formatter) formatFunction(fn *ast.FunctionDecl, depth int) {
	oldDepth := f.depth
	f.depth = depth
	if fn.Doc != nil {
		f.writeDocComment(fn.Doc)
	}
	if fn.SelectorOverride != "" {
		f.writeLine(fmt.Sprintf("@selector(%q)", fn.SelectorOverride))
	}
	f.writeIndent()
	f.buf.WriteString("function ")
	f.buf.WriteString(fn.Name)
	f.buf.WriteByte('(')
	f.writeParams(fn.Params)
	f.buf.WriteByte(')')
	f.writeModifiers(fn.Modifiers)
	if fn.Virtual {
		f.buf.WriteString(" virtual")
	}
	if fn.Override {
		f.buf.WriteString(" override")
	}
	if len(fn.Returns) > 0 {
		f.buf.WriteString(" returns (")
		f.writeReturns(fn.Returns)
		f.buf.WriteByte(')')
	}
	f.buf.WriteString(" {\n")
	f.depth++
	f.formatStatements(fn.Body)
	f.depth--
	f.writeLine("}")
	f.depth = oldDepth
}

// --- Fallback ---

func (f *formatter) formatFallback(fb *ast.FallbackDecl) {
	if fb.Doc != nil {
		f.writeDocComment(fb.Doc)
	}
	f.writeLine("fallback {")
	f.depth++
	f.formatStatements(fb.Body)
	f.depth--
	f.writeLine("}")
}

// --- Receive ---

func (f *formatter) formatReceive(r *ast.ReceiveDecl) {
	if r.Doc != nil {
		f.writeDocComment(r.Doc)
	}
	f.writeIndent()
	f.buf.WriteString("receive() external payable")
	if r.PayableAsset != "" {
		// The parser stores "payable(uno)" as modifier + PayableAsset="uno"
		// but we emit it directly
	}
	if len(r.Body) == 0 {
		f.buf.WriteString(" {}\n")
	} else {
		f.buf.WriteString(" {\n")
		f.depth++
		f.formatStatements(r.Body)
		f.depth--
		f.writeLine("}")
	}
}

// --- Doc Comments ---

func (f *formatter) writeDocComment(doc *ast.DocMeta) {
	if doc.Notice != "" {
		for _, line := range strings.Split(doc.Notice, "\n") {
			f.writeLine("/// @notice " + line)
		}
	}
	for _, p := range doc.Params {
		f.writeLine(fmt.Sprintf("/// @param  %s %s", p.Name, p.Text))
	}
	for _, r := range doc.Returns {
		if r.Name != "" {
			f.writeLine(fmt.Sprintf("/// @return %s %s", r.Name, r.Text))
		} else {
			f.writeLine(fmt.Sprintf("/// @return %s", r.Text))
		}
	}
	if doc.Effects != nil {
		eff := doc.Effects
		if len(eff.Reads) > 0 {
			f.writeLine(fmt.Sprintf("/// @effects(reads:  %s)", strings.Join(eff.Reads, ", ")))
		} else if eff.Reads != nil {
			f.writeLine("/// @effects(reads:  [])")
		}
		if len(eff.Writes) > 0 {
			f.writeLine(fmt.Sprintf("/// @effects(writes: %s)", strings.Join(eff.Writes, ", ")))
		} else if eff.Writes != nil {
			f.writeLine("/// @effects(writes: [])")
		}
		if len(eff.Emits) > 0 {
			f.writeLine(fmt.Sprintf("/// @effects(emits:  %s)", strings.Join(eff.Emits, ", ")))
		} else if eff.Emits != nil {
			f.writeLine("/// @effects(emits:  [])")
		}
		if len(eff.Calls) > 0 {
			parts := make([]string, len(eff.Calls))
			for i, c := range eff.Calls {
				if c.Wildcard {
					parts[i] = "*"
				} else {
					parts[i] = fmt.Sprintf("%s.%s.%s", c.Cap, c.Iface, c.Selector)
				}
			}
			f.writeLine(fmt.Sprintf("/// @effects(calls:  %s)", strings.Join(parts, ", ")))
		} else if eff.Calls != nil {
			f.writeLine("/// @effects(calls:  [])")
		}
	}
	if doc.Gas != nil {
		if doc.Gas.Expr != "" {
			f.writeLine(fmt.Sprintf("/// @gas(upper: %s)", doc.Gas.Expr))
		} else {
			f.writeLine(fmt.Sprintf("/// @gas(upper: %d)", doc.Gas.Upper))
		}
	}
}

// --- Helpers for params/returns/modifiers ---

func (f *formatter) writeParams(params []ast.FieldDecl) {
	for i, p := range params {
		if i > 0 {
			f.buf.WriteString(", ")
		}
		f.buf.WriteString(p.Type)
		if p.DataLoc != "" {
			f.buf.WriteByte(' ')
			f.buf.WriteString(p.DataLoc)
		}
		if p.Name != "" {
			f.buf.WriteByte(' ')
			f.buf.WriteString(p.Name)
		}
	}
}

func (f *formatter) writeReturns(returns []ast.FieldDecl) {
	for i, r := range returns {
		if i > 0 {
			f.buf.WriteString(", ")
		}
		f.buf.WriteString(r.Type)
		if r.DataLoc != "" {
			f.buf.WriteByte(' ')
			f.buf.WriteString(r.DataLoc)
		}
		if r.Name != "" {
			f.buf.WriteByte(' ')
			f.buf.WriteString(r.Name)
		}
	}
}

func (f *formatter) writeModifiers(mods []string) {
	for _, m := range mods {
		f.buf.WriteByte(' ')
		f.buf.WriteString(m)
	}
}

// --- Statements ---

func (f *formatter) formatStatements(stmts []Statement) {
	for _, s := range stmts {
		f.formatStatement(&s)
	}
}

type Statement = ast.Statement

func (f *formatter) formatStatement(s *Statement) {
	switch s.Kind {
	case "let":
		f.writeIndent()
		f.buf.WriteString(s.Type)
		f.buf.WriteByte(' ')
		f.buf.WriteString(s.Name)
		if s.Expr != nil {
			f.buf.WriteString(" = ")
			f.formatExpr(s.Expr)
		}
		f.buf.WriteString(";\n")

	case "let-tuple":
		f.writeIndent()
		f.buf.WriteByte('(')
		for i, name := range s.Names {
			if i > 0 {
				f.buf.WriteString(", ")
			}
			if i < len(s.Types) && s.Types[i] != "" {
				f.buf.WriteString(s.Types[i])
				f.buf.WriteByte(' ')
			}
			f.buf.WriteString(name)
		}
		f.buf.WriteString(") = ")
		if s.Expr != nil {
			f.formatExpr(s.Expr)
		}
		f.buf.WriteString(";\n")

	case "set":
		f.writeIndent()
		f.buf.WriteString("set ")
		if s.Target != nil {
			f.formatExpr(s.Target)
		} else {
			f.buf.WriteString(s.Name)
		}
		if s.Op != "" && s.Op != "++" && s.Op != "--" {
			f.buf.WriteByte(' ')
			f.buf.WriteString(s.Op)
			f.buf.WriteByte(' ')
			if s.Expr != nil {
				f.formatExpr(s.Expr)
			}
		} else if s.Op == "++" || s.Op == "--" {
			f.buf.WriteString(s.Op)
		} else {
			f.buf.WriteString(" = ")
			if s.Expr != nil {
				f.formatExpr(s.Expr)
			}
		}
		f.buf.WriteString(";\n")

	case "return":
		f.writeIndent()
		f.buf.WriteString("return")
		if s.Expr != nil {
			f.buf.WriteByte(' ')
			f.formatExpr(s.Expr)
		}
		f.buf.WriteString(";\n")

	case "if":
		f.writeIndent()
		f.buf.WriteString("if (")
		if s.Cond != nil {
			f.formatExpr(s.Cond)
		}
		f.buf.WriteString(") {\n")
		f.depth++
		f.formatStatements(s.Then)
		f.depth--
		if len(s.Else) > 0 {
			// Check if single else-if
			if len(s.Else) == 1 && s.Else[0].Kind == "if" {
				f.writeIndent()
				f.buf.WriteString("} else ")
				// Write the else-if inline (without indent)
				f.formatIfInline(&s.Else[0])
			} else {
				f.writeLine("} else {")
				f.depth++
				f.formatStatements(s.Else)
				f.depth--
				f.writeLine("}")
			}
		} else {
			f.writeLine("}")
		}

	case "for":
		f.writeIndent()
		f.buf.WriteString("for (")
		if s.Init != nil {
			f.formatStatementInline(s.Init)
		}
		f.buf.WriteString("; ")
		if s.Cond != nil {
			f.formatExpr(s.Cond)
		}
		f.buf.WriteString("; ")
		if s.Post != nil {
			f.formatExpr(s.Post)
		}
		f.buf.WriteString(") {\n")
		f.depth++
		f.formatStatements(s.Body)
		f.depth--
		f.writeLine("}")

	case "while":
		f.writeIndent()
		f.buf.WriteString("while (")
		if s.Cond != nil {
			f.formatExpr(s.Cond)
		}
		f.buf.WriteString(") {\n")
		f.depth++
		f.formatStatements(s.Body)
		f.depth--
		f.writeLine("}")

	case "do-while":
		f.writeLine("do {")
		f.depth++
		f.formatStatements(s.Body)
		f.depth--
		f.writeIndent()
		f.buf.WriteString("} while (")
		if s.Cond != nil {
			f.formatExpr(s.Cond)
		}
		f.buf.WriteString(");\n")

	case "emit":
		f.writeIndent()
		f.buf.WriteString("emit ")
		if s.Expr != nil {
			f.formatExpr(s.Expr)
		} else {
			f.buf.WriteString(s.Name)
		}
		f.buf.WriteString(";\n")

	case "revert":
		f.writeIndent()
		f.buf.WriteString("revert ")
		if s.Expr != nil {
			f.formatExpr(s.Expr)
		} else {
			f.buf.WriteString(s.Text)
		}
		f.buf.WriteString(";\n")

	case "require":
		// Parser stores: Expr = condition, Text = message string (with quotes).
		f.writeIndent()
		f.buf.WriteString("require(")
		if s.Expr != nil {
			f.formatExpr(s.Expr)
		}
		if s.Text != "" && s.Text != `""` {
			f.buf.WriteString(", ")
			f.buf.WriteString(s.Text)
		}
		f.buf.WriteString(");\n")

	case "assert":
		// Parser stores: Expr = condition, Text = message (optional).
		f.writeIndent()
		f.buf.WriteString("assert(")
		if s.Expr != nil {
			f.formatExpr(s.Expr)
		}
		f.buf.WriteString(");\n")

	case "call":
		f.writeIndent()
		if s.Expr != nil {
			f.formatExpr(s.Expr)
		}
		f.buf.WriteString(";\n")

	case "expr":
		f.writeIndent()
		if s.Expr != nil {
			f.formatExpr(s.Expr)
		}
		f.buf.WriteString(";\n")

	case "break":
		f.writeLine("break;")

	case "continue":
		f.writeLine("continue;")

	case "delete":
		f.writeIndent()
		f.buf.WriteString("delete ")
		if s.Expr != nil {
			f.formatExpr(s.Expr)
		} else {
			f.buf.WriteString(s.Name)
		}
		f.buf.WriteString(";\n")

	case "block":
		f.writeLine("{")
		f.depth++
		f.formatStatements(s.Body)
		f.depth--
		f.writeLine("}")

	case "unchecked":
		f.writeLine("unchecked {")
		f.depth++
		f.formatStatements(s.Body)
		f.depth--
		f.writeLine("}")

	case "try-catch":
		f.writeIndent()
		f.buf.WriteString("try ")
		if s.Expr != nil {
			f.formatExpr(s.Expr)
		}
		f.buf.WriteString(" {\n")
		f.depth++
		f.formatStatements(s.Body)
		f.depth--
		for _, cc := range s.Catches {
			f.writeIndent()
			if cc.Kind == "" {
				f.buf.WriteString("} catch {")
			} else {
				f.buf.WriteString("} catch ")
				f.buf.WriteString(cc.Kind)
				if cc.ParamName != "" {
					f.buf.WriteByte('(')
					if cc.ParamType != "" {
						f.buf.WriteString(cc.ParamType)
						f.buf.WriteByte(' ')
					}
					f.buf.WriteString(cc.ParamName)
					f.buf.WriteByte(')')
				}
				f.buf.WriteString(" {")
			}
			f.buf.WriteByte('\n')
			f.depth++
			f.formatStatements(cc.Body)
			f.depth--
		}
		f.writeLine("}")

	case "placeholder":
		f.writeLine("_;")

	case "deploy":
		// deploy ContractName(args...) -> binding;
		f.writeIndent()
		f.buf.WriteString("deploy ")
		if s.Expr != nil {
			f.formatExpr(s.Expr)
		}
		if s.Name != "" {
			f.buf.WriteString(" -> ")
			f.buf.WriteString(s.Name)
		}
		f.buf.WriteString(";\n")

	case "with":
		// with msg.sender = expr { body }
		f.writeIndent()
		f.buf.WriteString("with ")
		if s.Cond != nil {
			f.formatExpr(s.Cond)
		}
		f.buf.WriteString(" {\n")
		f.depth++
		f.formatStatements(s.Body)
		f.depth--
		f.writeLine("}")

	case "assert_revert":
		// assert_revert("msg") { body }
		f.writeIndent()
		f.buf.WriteString("assert_revert")
		if s.Expr != nil {
			f.buf.WriteByte('(')
			f.formatExpr(s.Expr)
			f.buf.WriteByte(')')
		}
		f.buf.WriteString(" {\n")
		f.depth++
		f.formatStatements(s.Body)
		f.depth--
		f.writeLine("}")

	case "assert_all":
		// assert_all { body }
		f.writeLine("assert_all {")
		f.depth++
		f.formatStatements(s.Body)
		f.depth--
		f.writeLine("}")

	case "assert_instructions_le":
		// assert_instructions_le expr { body }
		f.writeIndent()
		f.buf.WriteString("assert_instructions_le ")
		if s.Expr != nil {
			f.formatExpr(s.Expr)
		}
		f.buf.WriteString(" {\n")
		f.depth++
		f.formatStatements(s.Body)
		f.depth--
		f.writeLine("}")

	default:
		// Fallback: emit as raw text if available
		if s.Text != "" {
			f.writeLine(s.Text + ";")
		}
	}
}

// formatIfInline writes an if statement without leading indent (for else-if chains).
func (f *formatter) formatIfInline(s *Statement) {
	f.buf.WriteString("if (")
	if s.Cond != nil {
		f.formatExpr(s.Cond)
	}
	f.buf.WriteString(") {\n")
	f.depth++
	f.formatStatements(s.Then)
	f.depth--
	if len(s.Else) > 0 {
		if len(s.Else) == 1 && s.Else[0].Kind == "if" {
			f.writeIndent()
			f.buf.WriteString("} else ")
			f.formatIfInline(&s.Else[0])
		} else {
			f.writeLine("} else {")
			f.depth++
			f.formatStatements(s.Else)
			f.depth--
			f.writeLine("}")
		}
	} else {
		f.writeLine("}")
	}
}

// formatStatementInline writes a statement without newline/indent (for for-loop init).
func (f *formatter) formatStatementInline(s *Statement) {
	switch s.Kind {
	case "let":
		f.buf.WriteString(s.Type)
		f.buf.WriteByte(' ')
		f.buf.WriteString(s.Name)
		if s.Expr != nil {
			f.buf.WriteString(" = ")
			f.formatExpr(s.Expr)
		}
	case "set":
		if s.Target != nil {
			f.formatExpr(s.Target)
		} else {
			f.buf.WriteString(s.Name)
		}
		if s.Op != "" {
			f.buf.WriteByte(' ')
			f.buf.WriteString(s.Op)
			f.buf.WriteByte(' ')
		} else {
			f.buf.WriteString(" = ")
		}
		if s.Expr != nil {
			f.formatExpr(s.Expr)
		}
	case "expr":
		if s.Expr != nil {
			f.formatExpr(s.Expr)
		}
	}
}

// --- Expressions ---

func (f *formatter) formatExpr(e *ast.Expr) {
	if e == nil {
		return
	}
	switch e.Kind {
	case "number":
		f.buf.WriteString(e.Value)

	case "string":
		// Value already includes surrounding quotes from the lexer.
		f.buf.WriteString(e.Value)

	case "hex_string":
		f.buf.WriteString(e.Value)

	case "ident":
		f.buf.WriteString(e.Value)

	case "bool":
		f.buf.WriteString(e.Value)

	case "binary":
		f.formatExpr(e.Left)
		f.buf.WriteByte(' ')
		f.buf.WriteString(e.Op)
		f.buf.WriteByte(' ')
		f.formatExpr(e.Right)

	case "unary":
		f.buf.WriteString(e.Op)
		f.formatExpr(e.Left)

	case "call":
		f.formatExpr(e.Callee)
		if len(e.Options) > 0 {
			f.buf.WriteByte('{')
			for i, opt := range e.Options {
				if i > 0 {
					f.buf.WriteString(", ")
				}
				f.buf.WriteString(opt.Key)
				f.buf.WriteString(": ")
				f.formatExpr(opt.Value)
			}
			f.buf.WriteByte('}')
		}
		f.buf.WriteByte('(')
		for i, arg := range e.Args {
			if i > 0 {
				f.buf.WriteString(", ")
			}
			f.formatExpr(arg)
		}
		f.buf.WriteByte(')')

	case "named_call":
		f.formatExpr(e.Callee)
		if len(e.Options) > 0 {
			f.buf.WriteByte('{')
			for i, opt := range e.Options {
				if i > 0 {
					f.buf.WriteString(", ")
				}
				f.buf.WriteString(opt.Key)
				f.buf.WriteString(": ")
				f.formatExpr(opt.Value)
			}
			f.buf.WriteByte('}')
		}
		f.buf.WriteString("({")
		for i, arg := range e.NamedArgs {
			if i > 0 {
				f.buf.WriteString(", ")
			}
			f.buf.WriteString(arg.Name)
			f.buf.WriteString(": ")
			f.formatExpr(arg.Expr)
		}
		f.buf.WriteString("})")

	case "member":
		f.formatExpr(e.Object)
		f.buf.WriteByte('.')
		f.buf.WriteString(e.Member)

	case "index":
		f.formatExpr(e.Object)
		f.buf.WriteByte('[')
		f.formatExpr(e.Index)
		f.buf.WriteByte(']')

	case "paren":
		f.buf.WriteByte('(')
		f.formatExpr(e.Left)
		f.buf.WriteByte(')')

	case "assign":
		f.formatExpr(e.Left)
		f.buf.WriteByte(' ')
		if e.Op != "" {
			f.buf.WriteString(e.Op)
		} else {
			f.buf.WriteByte('=')
		}
		f.buf.WriteByte(' ')
		f.formatExpr(e.Right)

	case "ternary":
		f.formatExpr(e.Left)
		f.buf.WriteString(" ? ")
		f.formatExpr(e.Right)
		// The ternary "else" is typically stored in Args[0]
		if len(e.Args) > 0 {
			f.buf.WriteString(" : ")
			f.formatExpr(e.Args[0])
		}

	case "tuple":
		f.buf.WriteByte('(')
		for i, arg := range e.Args {
			if i > 0 {
				f.buf.WriteString(", ")
			}
			f.formatExpr(arg)
		}
		f.buf.WriteByte(')')

	case "array_lit":
		f.buf.WriteByte('[')
		for i, arg := range e.Args {
			if i > 0 {
				f.buf.WriteString(", ")
			}
			f.formatExpr(arg)
		}
		f.buf.WriteByte(']')

	case "struct_lit":
		f.buf.WriteString(e.Value)
		f.buf.WriteString("({")
		for i, sf := range e.StructFields {
			if i > 0 {
				f.buf.WriteString(", ")
			}
			f.buf.WriteString(sf.Name)
			f.buf.WriteString(": ")
			f.formatExpr(sf.Expr)
		}
		f.buf.WriteString("})")

	case "new":
		f.buf.WriteString("new ")
		f.buf.WriteString(e.Value)
		if len(e.Args) > 0 || e.Callee != nil {
			f.buf.WriteByte('(')
			for i, arg := range e.Args {
				if i > 0 {
					f.buf.WriteString(", ")
				}
				f.formatExpr(arg)
			}
			f.buf.WriteByte(')')
		}

	case "type_cast":
		f.buf.WriteString(e.Value)
		f.buf.WriteByte('(')
		if e.Left != nil {
			f.formatExpr(e.Left)
		} else if len(e.Args) > 0 {
			f.formatExpr(e.Args[0])
		}
		f.buf.WriteByte(')')

	default:
		// Unknown expression kind — emit value if available
		if e.Value != "" {
			f.buf.WriteString(e.Value)
		}
	}
}

// --- Test ---

func (f *formatter) formatTest(t *ast.TestDecl) {
	f.writef("test %s {\n", t.Name)
	f.depth++

	// Block-level let declarations
	for _, s := range t.Lets {
		f.formatStatement(&s)
	}
	if len(t.Lets) > 0 {
		f.buf.WriteByte('\n')
	}

	// Setup
	if t.SetupSuite != nil {
		f.writeLine("setup_suite {")
		f.depth++
		f.formatStatements(t.SetupSuite.Body)
		f.depth--
		f.writeLine("}")
		f.buf.WriteByte('\n')
	}
	if t.Setup != nil {
		f.writeLine("setup {")
		f.depth++
		f.formatStatements(t.Setup.Body)
		f.depth--
		f.writeLine("}")
		f.buf.WriteByte('\n')
	}

	// Mocks
	for _, mock := range t.Mocks {
		f.writeIndent()
		f.writef("mock %s", mock.Name)
		if mock.Interface != "" {
			f.writef(": %s", mock.Interface)
		}
		f.buf.WriteString(" {\n")
		f.depth++
		for _, m := range mock.Methods {
			f.writeIndent()
			f.buf.WriteString("function ")
			f.buf.WriteString(m.Name)
			f.buf.WriteByte('(')
			f.writeParams(m.Params)
			f.buf.WriteByte(')')
			if len(m.Returns) > 0 {
				f.buf.WriteString(" returns (")
				f.writeReturns(m.Returns)
				f.buf.WriteByte(')')
			}
			f.buf.WriteString(" {\n")
			f.depth++
			f.formatStatements(m.Body)
			f.depth--
			f.writeLine("}")
		}
		f.depth--
		f.writeLine("}")
		f.buf.WriteByte('\n')
	}

	// Test functions
	for i, fn := range t.Fns {
		if i > 0 {
			f.buf.WriteByte('\n')
		}
		f.writeIndent()
		f.buf.WriteString("function ")
		f.buf.WriteString(fn.Name)
		f.buf.WriteByte('(')
		f.writeParams(fn.Params)
		f.buf.WriteString(") {\n")
		f.depth++
		f.formatStatements(fn.Body)
		f.depth--
		f.writeLine("}")
	}

	// Teardown
	if t.Teardown != nil {
		f.buf.WriteByte('\n')
		f.writeLine("teardown {")
		f.depth++
		f.formatStatements(t.Teardown.Body)
		f.depth--
		f.writeLine("}")
	}
	if t.TeardownSuite != nil {
		f.buf.WriteByte('\n')
		f.writeLine("teardown_suite {")
		f.depth++
		f.formatStatements(t.TeardownSuite.Body)
		f.depth--
		f.writeLine("}")
	}

	f.depth--
	f.writeLine("}")
}
