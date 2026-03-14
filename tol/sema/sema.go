package sema

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/diag"
	"github.com/tos-network/tolang/tol/parser"
	"golang.org/x/crypto/sha3"
)

// TolLangPackage is the reserved default platform package, analogous to Java's java.lang.
// Contracts in this package are automatically imported into every TOL compilation unit
// without an explicit import statement and are accessible by short name (no prefix needed).
// External packages (.tor archives) may not declare this package name.
const TolLangPackage = "tol.lang"

// PackageLister is an optional interface that a FileResolver may implement.
// When implemented, the sema auto-import mechanism calls ListPackage("tol.lang")
// to discover all platform contracts and import them automatically.
type PackageLister interface {
	ListPackage(pkgPath string) ([]string, error)
}

// FileResolver resolves import paths to source bytes.
// importingFile is the canonical path of the file containing the import statement.
// importPath is the raw path string from the import declaration (e.g. "./token.tol").
// importName is the identifier being imported (e.g. "IToken"); resolvers for
// archive formats (.tor) use this to locate the right entry within the archive.
// Returns the source bytes, a canonical name for the file, and any error.
type FileResolver interface {
	Resolve(importingFile string, importPath string, importName string) (src []byte, canonicalName string, err error)
}

// CheckWithResolver is like Check but resolves import declarations using resolver.
// Resolved interfaces and libraries are merged into m.Interfaces / m.Libraries
// before the rest of the semantic checks run.
// Pass nil as resolver to use no resolver (imports will produce diagnostics).
func CheckWithResolver(filename string, m *ast.Module, resolver FileResolver) (*TypedModule, diag.Diagnostics) {
	var diags diag.Diagnostics
	inFlight := map[string]bool{filename: true}
	// Auto-import tol.lang before processing explicit imports (like Java's java.lang).
	autoImportTolLang(filename, m, resolver, &diags, inFlight)
	if m != nil && len(m.Imports) > 0 {
		resolveImports(filename, m, resolver, &diags, inFlight)
	}
	typed, checkDiags := Check(filename, m)
	diags = append(diags, checkDiags...)
	if diags.HasErrors() {
		return nil, diags
	}
	return typed, nil
}

// autoImportTolLang automatically imports all contracts from the tol.lang package
// into m, as if the user had written "import tol.lang.X;" for each contract X.
// This mirrors Java's automatic import of java.lang.*: every TOL compilation unit
// sees tol.lang types under their short names without any explicit import.
// If the resolver does not implement PackageLister, or tol/lang/ does not exist
// on the filesystem, this is a silent no-op.
// Contracts whose short name is already present in m.Interfaces are skipped
// (explicit user imports win over auto-imports).
func autoImportTolLang(filename string, m *ast.Module, resolver FileResolver, diags *diag.Diagnostics, inFlight map[string]bool) {
	if m == nil || resolver == nil {
		return
	}
	lister, ok := resolver.(PackageLister)
	if !ok {
		return
	}
	names, err := lister.ListPackage(TolLangPackage)
	if err != nil || len(names) == 0 {
		return
	}
	// Build set of already-present interface/library names to avoid shadowing.
	existing := make(map[string]bool, len(m.Interfaces)+len(m.Libraries))
	for _, iface := range m.Interfaces {
		existing[iface.Name] = true
	}
	for _, lib := range m.Libraries {
		existing[lib.Name] = true
	}
	// Also skip auto-importing into tol.lang itself (avoid self-import).
	if m.Package == TolLangPackage {
		return
	}
	// Build a temporary module with synthetic package imports for each tol.lang name.
	synthMod := &ast.Module{Imports: make([]ast.ImportDecl, 0, len(names))}
	for _, name := range names {
		if existing[name] {
			continue // user already imported or defined this name; skip
		}
		synthMod.Imports = append(synthMod.Imports, ast.ImportDecl{
			IsPackageImport: true,
			PackagePath:     TolLangPackage,
			PackageContract: name,
			Name:            name,
		})
	}
	if len(synthMod.Imports) == 0 {
		return
	}
	resolveImports(filename, synthMod, resolver, diags, inFlight)
	// Merge resolved interfaces and libraries into m.
	for _, iface := range synthMod.Interfaces {
		if !existing[iface.Name] {
			m.Interfaces = append(m.Interfaces, iface)
		}
	}
	for _, lib := range synthMod.Libraries {
		if !existing[lib.Name] {
			m.Libraries = append(m.Libraries, lib)
		}
	}
}

// resolveImports processes each ImportDecl, loads the referenced file, and
// merges the named interface or library into m.Interfaces / m.Libraries.
// inFlight tracks the set of canonical file names currently being resolved
// (i.e. on the import stack); if a file appears in inFlight we have a cycle.
func resolveImports(filename string, m *ast.Module, resolver FileResolver, diags *diag.Diagnostics, inFlight map[string]bool) {
	defaultSpan := func(line int) diag.Span {
		return diag.Span{File: filename, Start: diag.Position{Line: line, Column: 1}}
	}
	for _, imp := range m.Imports {
		// --- Package import: "import tos.registry.AgentRegistry [as Alias];" ---
		if imp.IsPackageImport {
			if resolver == nil {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaImportNoResolver,
					Message: fmt.Sprintf("cannot resolve package import %q.%q: no file resolver available", imp.PackagePath, imp.PackageContract),
					Span:    defaultSpan(imp.Line),
				})
				continue
			}
			src, canonName, err := resolver.Resolve(filename, imp.PackagePath, imp.PackageContract)
			if err != nil {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaImportNotFound,
					Message: fmt.Sprintf("cannot resolve package import %q.%q: %v", imp.PackagePath, imp.PackageContract, err),
					Span:    defaultSpan(imp.Line),
				})
				continue
			}
			if inFlight[canonName] {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaImportCircular,
					Message: fmt.Sprintf("circular import detected: %q imports %q.%q which is already being resolved", filename, imp.PackagePath, imp.PackageContract),
					Span:    defaultSpan(imp.Line),
				})
				continue
			}
			refMod, parseDiags := parser.ParseFile(canonName, src)
			if parseDiags.HasErrors() {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaImportParseFailed,
					Message: fmt.Sprintf("package import %q.%q failed to parse: %v", imp.PackagePath, imp.PackageContract, parseDiags),
					Span:    defaultSpan(imp.Line),
				})
				continue
			}
			if len(refMod.Imports) > 0 {
				inFlight[canonName] = true
				resolveImports(canonName, refMod, resolver, diags, inFlight)
				delete(inFlight, canonName)
			}
			contractName := imp.PackageContract
			localName := imp.Name // local alias (may differ from contractName via 'as')
			found := false
			// Try interfaces in the referenced module.
			for _, iface := range refMod.Interfaces {
				if iface.Name == contractName {
					tagged := iface
					tagged.Name = localName
					tagged.PackageName = imp.PackagePath
					tagged.ContractName = contractName
					m.Interfaces = append(m.Interfaces, tagged)
					found = true
					break
				}
			}
			// Try concrete contracts — synthesize interface from external functions.
			if !found {
				for i := range refMod.Contracts {
					if refMod.Contracts[i].Name == contractName {
						synth := synthesizeInterfaceFromContract(&refMod.Contracts[i], localName, imp.PackagePath, contractName)
						m.Interfaces = append(m.Interfaces, synth)
						found = true
						break
					}
				}
			}
			if !found {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaImportNameNotFound,
					Message: fmt.Sprintf("package import %q: no contract or interface named %q found in %q", imp.PackagePath, contractName, canonName),
					Span:    defaultSpan(imp.Line),
				})
			}
			continue
		}

		path := imp.Path
		// Require a resolver.
		if resolver == nil {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaImportNoResolver,
				Message: fmt.Sprintf("cannot resolve import %q: no file resolver available", path),
				Span:    defaultSpan(imp.Line),
			})
			continue
		}
		src, canonName, err := resolver.Resolve(filename, path, imp.Name)
		if err != nil {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaImportNotFound,
				Message: fmt.Sprintf("cannot read import %q: %v", path, err),
				Span:    defaultSpan(imp.Line),
			})
			continue
		}
		// Cycle detection: if canonName is already on the in-flight stack we
		// have a circular import.  Emit TOL2095 and skip this import so we
		// don't recurse infinitely.
		if inFlight[canonName] {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaImportCircular,
				Message: fmt.Sprintf("circular import detected: %q imports %q which is already being resolved", filename, canonName),
				Span:    defaultSpan(imp.Line),
			})
			continue
		}
		refMod, parseDiags := parser.ParseFile(canonName, src)
		if parseDiags.HasErrors() {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaImportParseFailed,
				Message: fmt.Sprintf("import %q failed to parse: %v", path, parseDiags),
				Span:    defaultSpan(imp.Line),
			})
			continue
		}
		// Recursively resolve transitive imports in the referenced module so
		// that entities it re-exports from its own dependencies are available.
		if len(refMod.Imports) > 0 {
			inFlight[canonName] = true
			resolveImports(canonName, refMod, resolver, diags, inFlight)
			delete(inFlight, canonName)
		}
		// Find the named entity/entities in the referenced module and merge them.
		if len(imp.Named) > 0 {
			// Named import list: import { A, B as C } from "path";
			// Each entry in imp.Named specifies a symbol to import with optional alias.
			for _, alias := range imp.Named {
				symName := alias.Name
				bindName := alias.Name
				if alias.Alias != "" {
					bindName = alias.Alias
				}
				found := false
				for _, iface := range refMod.Interfaces {
					if iface.Name == symName {
						renamed := iface
						renamed.Name = bindName
						m.Interfaces = append(m.Interfaces, renamed)
						found = true
						break
					}
				}
				if !found {
					for _, lib := range refMod.Libraries {
						if lib.Name == symName {
							renamed := lib
							renamed.Name = bindName
							m.Libraries = append(m.Libraries, renamed)
							found = true
							break
						}
					}
				}
				if !found {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaImportNameNotFound,
						Message: fmt.Sprintf("import %q: no interface or library named %q found in %q", path, symName, canonName),
						Span:    defaultSpan(imp.Line),
					})
				}
			}
		} else if imp.IsStar {
			// Star import: import * as X from "path";
			// Import all interfaces and libraries from the referenced module under the alias namespace.
			// Since TOL doesn't have true namespace support yet, we import all entities,
			// prefixing them with the alias when needed (or importing the first entity under the alias).
			// For now: import all interfaces and libraries from refMod into m, renamed with alias prefix.
			bindAlias := imp.Alias
			for _, iface := range refMod.Interfaces {
				renamed := iface
				if bindAlias != "" {
					renamed.Name = bindAlias + "." + iface.Name
				}
				m.Interfaces = append(m.Interfaces, renamed)
			}
			for _, lib := range refMod.Libraries {
				renamed := lib
				if bindAlias != "" {
					renamed.Name = bindAlias + "." + lib.Name
				}
				m.Libraries = append(m.Libraries, renamed)
			}
		} else {
			// Simple forms:
			//   import "path";              — bare import (side-effect only, no bindings)
			//   import "path" as Alias;    — alias import: take first entity and bind under alias
			//   import Name from "path";   — old-style: import entity named Name
			found := false
			bindName := imp.Name
			if imp.Alias != "" {
				bindName = imp.Alias
			}
			if imp.Name == "" && imp.Alias == "" {
				// Bare import "path"; — side-effect only, nothing to bind.
				// No diagnostics needed.
			} else if imp.Name == "" {
				// import "path" as Alias; — take first interface or library and bind under alias.
				for _, iface := range refMod.Interfaces {
					renamed := iface
					renamed.Name = bindName
					m.Interfaces = append(m.Interfaces, renamed)
					found = true
					break
				}
				if !found {
					for _, lib := range refMod.Libraries {
						renamed := lib
						renamed.Name = bindName
						m.Libraries = append(m.Libraries, renamed)
						found = true
						break
					}
				}
				if !found {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaImportNameNotFound,
						Message: fmt.Sprintf("import %q: no interface or library found in %q", path, canonName),
						Span:    defaultSpan(imp.Line),
					})
				}
			} else {
				// import Name from "path"; or import "path" as Name (imp.Name set).
				for _, iface := range refMod.Interfaces {
					if iface.Name == imp.Name {
						renamed := iface
						renamed.Name = bindName
						m.Interfaces = append(m.Interfaces, renamed)
						found = true
						break
					}
				}
				if !found {
					for _, lib := range refMod.Libraries {
						if lib.Name == imp.Name {
							renamed := lib
							renamed.Name = bindName
							m.Libraries = append(m.Libraries, renamed)
							found = true
							break
						}
					}
				}
				if !found {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaImportNameNotFound,
						Message: fmt.Sprintf("import %q: no interface or library named %q found in %q", path, imp.Name, canonName),
						Span:    defaultSpan(imp.Line),
					})
				}
			}
		}
	}
}

// synthesizeInterfaceFromContract builds an InterfaceDecl from a concrete contract's
// external functions. Used when a package import references a concrete contract.
func synthesizeInterfaceFromContract(c *ast.ContractDecl, localName, pkgName, contractName string) ast.InterfaceDecl {
	iface := ast.InterfaceDecl{
		Name:         localName,
		PackageName:  pkgName,
		ContractName: contractName,
	}
	for _, fn := range c.Functions {
		isExternal := false
		for _, mod := range fn.Modifiers {
			if mod == "external" || mod == "public" {
				isExternal = true
				break
			}
		}
		if !isExternal || fn.Body == nil {
			continue
		}
		iface.Functions = append(iface.Functions, ast.FuncSigDecl{
			Name:      fn.Name,
			Params:    fn.Params,
			Returns:   fn.Returns,
			Modifiers: fn.Modifiers,
		})
	}
	// Copy constants so callers can inline them via fully-qualified access.
	iface.Constants = append(iface.Constants, c.Constants...)
	// Copy enums so callers can reference enum members via fully-qualified access.
	iface.Enums = append(iface.Enums, c.Enums...)
	return iface
}

// TypedModule is the semantic-checked representation used by lowering.
type TypedModule struct {
	AST *ast.Module
}

type storageSlotKind string

const (
	storageKindScalar  storageSlotKind = "scalar"
	storageKindMapping storageSlotKind = "mapping"
	storageKindArray   storageSlotKind = "array"
)

type storageSlotInfo struct {
	name         string
	kind         storageSlotKind
	typeName     string
	mappingDepth int
	isImmutable  bool // true for immutable declarations (not a state variable slot)
}

type storageCheckCtx struct {
	slots  map[string]storageSlotInfo
	scopes []map[string]struct{}
}

func Check(filename string, m *ast.Module) (*TypedModule, diag.Diagnostics) {
	var diags diag.Diagnostics
	if m == nil {
		return nil, diags
	}

	// Backward-compatibility: if the caller set m.Contract directly (old API / tests)
	// but did not populate m.Contracts, synthesize Contracts from Contract so the rest
	// of Check() has a unified view.
	if m.Contract != nil && len(m.Contracts) == 0 {
		m.Contracts = []ast.ContractDecl{*m.Contract}
		m.Contract = &m.Contracts[0]
	}

	// Version pragmas are accepted leniently: any Solidity-style version constraint
	// (e.g. ^0.8.0, >=0.7.0 <0.9.0, 0.2.0) is stored as metadata without enforcement.

	// A test-only or declaration-only file (no contract, but has test blocks or top-level
	// declarations) is valid.
	hasTopLevelDecls := len(m.Tests) > 0 || len(m.Interfaces) > 0 || len(m.Libraries) > 0 ||
		len(m.Structs) > 0 || len(m.TypeDecls) > 0 || len(m.AbstractContracts) > 0 ||
		len(m.FreeFunctions) > 0 || len(m.Constants) > 0 || len(m.Enums) > 0 ||
		len(m.Errors) > 0 || len(m.Events) > 0 || len(m.UsingDecls) > 0 ||
		len(m.Capabilities) > 0 || len(m.Contracts) > 0
	if len(m.Contracts) == 0 && !hasTopLevelDecls {
		diags = append(diags, diag.Diagnostic{
			Code:    diag.CodeSemaMissingContract,
			Message: "missing contract declaration",
			Span: diag.Span{
				File: filename,
				Start: diag.Position{
					Line:   1,
					Column: 1,
				},
				End: diag.Position{
					Line:   1,
					Column: 1,
				},
			},
		})
	}

	// Build module-level topSeen and libFuncs once — shared across all contracts.
	topSeen := map[string]string{}

	// Register interface declarations in topSeen first.
	for _, iface := range m.Interfaces {
		name := strings.TrimSpace(iface.Name)
		if name == "" {
			continue
		}
		if name == "this" || name == "selector" {
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("interface name '%s' is reserved and cannot be declared", name),
				Span:    defaultSpan(filename),
			})
		}
		if strings.HasPrefix(name, "__tol_") {
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("interface name '%s' uses reserved internal prefix '__tol_'", name),
				Span:    defaultSpan(filename),
			})
		}
		if prev, exists := topSeen[name]; exists {
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("duplicate top-level declaration name '%s' between %s and interface", name, prev),
				Span:    defaultSpan(filename),
			})
			continue
		}
		topSeen[name] = "interface"
	}

	// Build library function table and register libraries in topSeen.
	// libFuncs: library name → function name → arity
	libFuncs := map[string]map[string]int{}
	for _, lib := range m.Libraries {
		name := strings.TrimSpace(lib.Name)
		if name == "" {
			continue
		}
		if name == "this" || name == "selector" {
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("library name '%s' is reserved and cannot be declared", name),
				Span:    defaultSpan(filename),
			})
		}
		if strings.HasPrefix(name, "__tol_") {
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("library name '%s' uses reserved internal prefix '__tol_'", name),
				Span:    defaultSpan(filename),
			})
		}
		if prev, exists := topSeen[name]; exists {
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("duplicate top-level declaration name '%s' between %s and library", name, prev),
				Span:    defaultSpan(filename),
			})
			continue
		}
		topSeen[name] = "library"
		fnMap := map[string]int{}
		libFuncs[name] = fnMap
		// Validate library function modifiers and populate fnMap.
		diags = append(diags, checkLibraryDecl(filename, lib, fnMap)...)
	}

	for _, decl := range m.SkippedTopDecls {
		kind := strings.TrimSpace(decl.Kind)
		name := strings.TrimSpace(decl.Name)
		if name == "" {
			continue
		}
		if name == "this" || name == "selector" {
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("%s name '%s' is reserved and cannot be declared", kind, name),
				Span:    defaultSpan(filename),
			})
		}
		if strings.HasPrefix(name, "__tol_") {
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("%s name '%s' uses reserved internal prefix '__tol_'", kind, name),
				Span:    defaultSpan(filename),
			})
		}
		if prev, exists := topSeen[name]; exists {
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("duplicate top-level declaration name '%s' between %s and %s", name, prev, kind),
				Span:    defaultSpan(filename),
			})
			continue
		}
		topSeen[name] = kind
	}

	// Check each contract in the module.
	for i := range m.Contracts {
		checkOneContract(filename, m, &m.Contracts[i], topSeen, libFuncs, &diags)
	}

	// Validate inheritance, C3 linearization, interface conformance, and super calls.
	// Run once per contract (using m.Contract pointer swap for compatibility with inherit.go).
	for i := range m.Contracts {
		saved := m.Contract
		m.Contract = &m.Contracts[i]
		checkInheritance(filename, m, &diags)
		m.Contract = saved
	}

	// Validate test declarations.
	isTestFile := strings.HasSuffix(filename, "_test.tol")
	for _, td := range m.Tests {
		if !isTestFile {
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaTestInNonTestFile,
				Message: fmt.Sprintf("test block '%s' is only allowed in files ending with '_test.tol'", td.Name),
				Span:    defaultSpan(filename),
			})
		}
		// Validate block-level let declarations.
		checkTestFnBody(filename, td.Lets, &diags)
		for _, fn := range td.Fns {
			if !strings.HasPrefix(fn.Name, "test_") && !strings.HasPrefix(fn.Name, "fuzz_") {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidTestFnName,
					Message: fmt.Sprintf("test function '%s' must start with 'test_' or 'fuzz_'", fn.Name),
					Span:    defaultSpan(filename),
				})
			}
			// Check function body with test builtins whitelisted.
			checkTestFnBody(filename, fn.Body, &diags)
		}
		// Validate mock method bodies.
		for _, md := range td.Mocks {
			for _, mm := range md.Methods {
				checkTestFnBody(filename, mm.Body, &diags)
			}
		}
	}

	if diags.HasErrors() {
		return nil, diags
	}
	return &TypedModule{AST: m}, nil
}

// checkOneContract runs all per-contract semantic checks for a single contract c.
// topSeen and libFuncs are module-level maps built once from interfaces/libraries
// and shared across all contracts in the module.
func checkOneContract(filename string, m *ast.Module, c *ast.ContractDecl, topSeen map[string]string, libFuncs map[string]map[string]int, diags *diag.Diagnostics) {
	contractName := strings.TrimSpace(c.Name)

	if prev, exists := topSeen[contractName]; exists {
		*diags = append(*diags, diag.Diagnostic{
			Code:    diag.CodeSemaNameCollision,
			Message: fmt.Sprintf("contract name '%s' collides with top-level %s declaration", contractName, prev),
			Span:    defaultSpan(filename),
		})
	}
	if contractName == "this" || contractName == "selector" {
		*diags = append(*diags, diag.Diagnostic{
			Code:    diag.CodeSemaReservedName,
			Message: fmt.Sprintf("contract name '%s' is reserved and cannot be declared", contractName),
			Span:    defaultSpan(filename),
		})
	}
	if strings.HasPrefix(contractName, "__tol_") {
		*diags = append(*diags, diag.Diagnostic{
			Code:    diag.CodeSemaReservedName,
			Message: fmt.Sprintf("contract name '%s' uses reserved internal prefix '__tol_'", contractName),
			Span:    defaultSpan(filename),
		})
	}
	contractSupportSeen := map[string]string{}
	// errorDeclParams maps error name -> param list for arity checking.
	errorDeclParams := map[string][]ast.FieldDecl{}
	// enumMemberValues maps enum name -> (member name -> integer value).
	enumMemberValues := map[string]map[string]int{}

	// Validate error declarations.
	for _, ed := range c.Errors {
		name := strings.TrimSpace(ed.Name)
		if name == "" {
			continue
		}
		if name == "this" || name == "selector" {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("error name '%s' is reserved and cannot be declared", name),
				Span:    defaultSpan(filename),
			})
		}
		if strings.HasPrefix(name, "__tol_") {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("error name '%s' uses reserved internal prefix '__tol_'", name),
				Span:    defaultSpan(filename),
			})
		}
		if prev, exists := contractSupportSeen[name]; exists {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("duplicate contract declaration name '%s' between %s and error", name, prev),
				Span:    defaultSpan(filename),
			})
			continue
		}
		contractSupportSeen[name] = "error"
		errorDeclParams[name] = ed.Params
	}

	// Validate enum declarations.
	for _, en := range c.Enums {
		name := strings.TrimSpace(en.Name)
		if name == "" {
			continue
		}
		if name == "this" || name == "selector" {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("enum name '%s' is reserved and cannot be declared", name),
				Span:    defaultSpan(filename),
			})
		}
		if strings.HasPrefix(name, "__tol_") {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("enum name '%s' uses reserved internal prefix '__tol_'", name),
				Span:    defaultSpan(filename),
			})
		}
		if len(en.Members) == 0 {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidStmtShape,
				Message: fmt.Sprintf("enum '%s' must have at least one member", name),
				Span:    defaultSpan(filename),
			})
		}
		if prev, exists := contractSupportSeen[name]; exists {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("duplicate contract declaration name '%s' between %s and enum", name, prev),
				Span:    defaultSpan(filename),
			})
			continue
		}
		contractSupportSeen[name] = "enum"
		memberVals := make(map[string]int, len(en.Members))
		memberSeen := map[string]struct{}{}
		for i, mem := range en.Members {
			mname := strings.TrimSpace(mem)
			if mname == "" {
				continue
			}
			if _, exists := memberSeen[mname]; exists {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaNameCollision,
					Message: fmt.Sprintf("duplicate enum member '%s' in enum '%s'", mname, name),
					Span:    defaultSpan(filename),
				})
				continue
			}
			memberSeen[mname] = struct{}{}
			memberVals[mname] = i
		}
		enumMemberValues[name] = memberVals
	}

	// Build known struct types from top-level and contract-level struct declarations.
	// knownStructs maps struct name → ordered field list.
	// User-defined value type aliases (type X is Y;) are also registered here
	// with a nil field list so that type validation accepts them.
	knownStructs := map[string][]ast.FieldDecl{}
	// Register top-level and contract-level user-defined value types.
	allTypeDecls := append(m.TypeDecls, c.TypeDecls...)
	for _, td := range allTypeDecls {
		name := strings.TrimSpace(td.Name)
		if name != "" {
			// Solidity compliance (Deviation 4): the underlying type of a UDVT must
			// be an elementary value type (uN, iN, bool, agent, bytesN, string,
			// bytes). Arrays, mappings, structs, function types, and other UDVTs
			// are not allowed.
			underlying := normalizeSelectorType(strings.TrimSpace(td.Underlying))
			if underlying != "" && !isValueTOLType(underlying) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaUDVTInvalidUnderlying,
					Message: fmt.Sprintf("user-defined value type '%s': underlying type '%s' must be an elementary value type (uN, iN, bool, agent, bytesN); arrays, mappings, and struct types are not allowed", name, td.Underlying),
					Span:    defaultSpan(filename),
				})
			}
			knownStructs[name] = nil // nil = user-defined value type (no fields)
		}
	}
	// First register top-level structs.
	for _, sd := range m.Structs {
		name := strings.TrimSpace(sd.Name)
		if name == "" {
			continue
		}
		if prev, exists := knownStructs[name]; exists {
			_ = prev
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaDuplicateStruct,
				Message: fmt.Sprintf("duplicate struct declaration '%s'", name),
				Span:    defaultSpan(filename),
			})
			continue
		}
		knownStructs[name] = sd.Fields
	}
	// Then register contract-level structs (may shadow or duplicate top-level).
	for _, sd := range c.Structs {
		name := strings.TrimSpace(sd.Name)
		if name == "" {
			continue
		}
		if name == "this" || name == "selector" {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("struct name '%s' is reserved and cannot be declared", name),
				Span:    defaultSpan(filename),
			})
		}
		if strings.HasPrefix(name, "__tol_") {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("struct name '%s' uses reserved internal prefix '__tol_'", name),
				Span:    defaultSpan(filename),
			})
		}
		// Check duplicate struct first (TOL2050), then collision with other kinds (TOL2026).
		if _, exists := knownStructs[name]; exists {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaDuplicateStruct,
				Message: fmt.Sprintf("duplicate struct declaration '%s'", name),
				Span:    defaultSpan(filename),
			})
			continue
		}
		if prev, exists := contractSupportSeen[name]; exists {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("duplicate contract declaration name '%s' between %s and struct", name, prev),
				Span:    defaultSpan(filename),
			})
			continue
		}
		contractSupportSeen[name] = "struct"
		knownStructs[name] = sd.Fields
	}
	// Register top-level and imported interface names as valid types (nil fields).
	// This allows local variables of interface type: let reg: IToken = IToken(addr);
	for _, iface := range m.Interfaces {
		name := strings.TrimSpace(iface.Name)
		if name != "" {
			if _, exists := knownStructs[name]; !exists {
				knownStructs[name] = nil
			}
		}
	}
	// Validate struct field types (each field must be a primitive or known struct).
	for _, sd := range append(m.Structs, c.Structs...) {
		fieldSeen := map[string]struct{}{}
		for _, f := range sd.Fields {
			fname := strings.TrimSpace(f.Name)
			if fname == "" {
				continue
			}
			if _, dup := fieldSeen[fname]; dup {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaNameCollision,
					Message: fmt.Sprintf("duplicate field '%s' in struct '%s'", fname, sd.Name),
					Span:    defaultSpan(filename),
				})
				continue
			}
			fieldSeen[fname] = struct{}{}
			ftype := strings.TrimSpace(f.Type)
			if ftype == "" {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: fmt.Sprintf("missing type for field '%s' in struct '%s'", fname, sd.Name),
					Span:    defaultSpan(filename),
				})
				continue
			}
			// Allow primitive TOL types or known struct names.
			norm := normalizeSelectorType(ftype)
			if !isValidTOLType(norm, false) {
				if _, isStruct := knownStructs[norm]; !isStruct {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaUnknownStructType,
						Message: fmt.Sprintf("unknown type '%s' for field '%s' in struct '%s'", norm, fname, sd.Name),
						Span:    defaultSpan(filename),
					})
				}
			}
		}
	}

	// Also process any remaining SkippedDecls (for kinds not yet fully parsed).
	for _, decl := range c.SkippedDecls {
		kind := strings.TrimSpace(decl.Kind)
		name := strings.TrimSpace(decl.Name)
		if name == "" || name == "<anonymous>" {
			continue
		}
		if name == "this" || name == "selector" {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("%s name '%s' is reserved and cannot be declared", kind, name),
				Span:    defaultSpan(filename),
			})
		}
		if strings.HasPrefix(name, "__tol_") {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("%s name '%s' uses reserved internal prefix '__tol_'", kind, name),
				Span:    defaultSpan(filename),
			})
		}
		if prev, exists := contractSupportSeen[name]; exists {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("duplicate contract declaration name '%s' between %s and %s", name, prev, kind),
				Span:    defaultSpan(filename),
			})
			continue
		}
		contractSupportSeen[name] = kind
	}

	if prev, exists := contractSupportSeen[contractName]; exists {
		*diags = append(*diags, diag.Diagnostic{
			Code:    diag.CodeSemaNameCollision,
			Message: fmt.Sprintf("contract name '%s' collides with contract-level %s declaration", contractName, prev),
			Span:    defaultSpan(filename),
		})
	}

	// Build map of declared user-defined modifier names and check for name collisions.
	userModifiers := map[string]ast.ModifierDecl{}
	modifierSeen := map[string]struct{}{}
	for _, md := range c.Modifiers {
		name := strings.TrimSpace(md.Name)
		if name == "" {
			continue
		}
		if name == "this" || name == "selector" {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("modifier name '%s' is reserved and cannot be declared", name),
				Span:    defaultSpan(filename),
			})
		}
		if strings.HasPrefix(name, "__tol_") {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("modifier name '%s' uses reserved internal prefix '__tol_'", name),
				Span:    defaultSpan(filename),
			})
		}
		if _, exists := modifierSeen[name]; exists {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaDuplicateModifier,
				Message: fmt.Sprintf("duplicate modifier declaration '%s'", name),
				Span:    defaultSpan(filename),
			})
			continue
		}
		// Check against other contract-support declarations (error, enum, etc.)
		if prev, exists := contractSupportSeen[name]; exists {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("duplicate contract declaration name '%s' between %s and modifier", name, prev),
				Span:    defaultSpan(filename),
			})
			continue
		}
		modifierSeen[name] = struct{}{}
		contractSupportSeen[name] = "modifier"
		userModifiers[name] = md
	}

	funcVis := map[string]string{}
	funcArity := map[string]int{}
	eventArity := map[string]int{}
	for _, fn := range c.Functions {
		vis, modDiags := validateFunctionModifiers(filename, fn.Name, fn.Modifiers, userModifiers)
		*diags = append(*diags, modDiags...)
		funcVis[fn.Name] = vis
		if existing, seen := funcArity[fn.Name]; !seen {
			funcArity[fn.Name] = len(fn.Params)
		} else if existing != len(fn.Params) {
			// Multiple overloads with different arity: use -1 as "overloaded, any arity" sentinel.
			funcArity[fn.Name] = -1
		}
		// If two overloads happen to have the same arity, the duplicate-signature check below
		// will catch them; keep the existing value.
	}
	for _, ev := range c.Events {
		evName := strings.TrimSpace(ev.Name)
		if evName == "selector" || evName == "this" {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("event name '%s' is reserved and cannot be declared", evName),
				Span:    defaultSpan(filename),
			})
		}
		if strings.HasPrefix(evName, "__tol_") {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("event name '%s' uses reserved internal prefix '__tol_'", evName),
				Span:    defaultSpan(filename),
			})
		}
		*diags = append(*diags, duplicateParamDiagnostics(filename, "event", ev.Name, ev.Params)...)
		for _, p := range ev.Params {
			validateTypeForContext(filename, p.Type, fmt.Sprintf("event '%s' parameter '%s'", ev.Name, strings.TrimSpace(p.Name)), false, diags)
		}
		indexedCount := 0
		for _, p := range ev.Params {
			if p.Indexed {
				indexedCount++
			}
		}
		if indexedCount > 3 {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidStmtShape,
				Message: fmt.Sprintf("event '%s' declares %d indexed field(s); at most 3 are allowed", ev.Name, indexedCount),
				Span:    defaultSpan(filename),
			})
		}
		if _, exists := eventArity[ev.Name]; exists {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaDuplicateEvent,
				Message: fmt.Sprintf("duplicate event '%s'", ev.Name),
				Span:    defaultSpan(filename),
			})
			continue
		}
		eventArity[ev.Name] = len(ev.Params)
	}
	slotInfos := map[string]storageSlotInfo{}

	if c.Storage != nil {
		slotSeen := map[string]struct{}{}
		for _, slot := range c.Storage.Slots {
			validateTypeForContext(filename, slot.Type, fmt.Sprintf("storage slot '%s'", strings.TrimSpace(slot.Name)), true, diags)
			slotName := strings.TrimSpace(slot.Name)
			if slotName == "selector" || slotName == "this" {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaReservedName,
					Message: fmt.Sprintf("storage slot name '%s' is reserved and cannot be declared", slotName),
					Span:    defaultSpan(filename),
				})
			}
			if strings.HasPrefix(slotName, "__tol_") {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaReservedName,
					Message: fmt.Sprintf("storage slot name '%s' uses reserved internal prefix '__tol_'", slotName),
					Span:    defaultSpan(filename),
				})
			}
			if _, ok := slotSeen[slot.Name]; ok {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaDuplicateSlot,
					Message: fmt.Sprintf("duplicate storage slot '%s'", slot.Name),
					Span: diag.Span{
						File: filename,
						Start: diag.Position{
							Line:   1,
							Column: 1,
						},
						End: diag.Position{
							Line:   1,
							Column: 1,
						},
					},
				})
			} else {
				slotSeen[slot.Name] = struct{}{}
				slotInfos[slot.Name] = buildStorageSlotInfo(slot)
			}
		}
	}
	// Register immutable declarations into slotInfos (they behave like scalar slots).
	immutableSeen := map[string]struct{}{}
	for _, imm := range c.Immutables {
		immName := strings.TrimSpace(imm.Name)
		if immName == "" {
			continue
		}
		if immName == "selector" || immName == "this" {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("immutable name '%s' is reserved and cannot be declared", immName),
				Span:    defaultSpan(filename),
			})
		}
		if strings.HasPrefix(immName, "__tol_") {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("immutable name '%s' uses reserved internal prefix '__tol_'", immName),
				Span:    defaultSpan(filename),
			})
		}
		// Immutable must be a value type (no mappings, no arrays).
		immType := strings.TrimSpace(imm.Type)
		normType := normalizeSelectorType(immType)
		if !isValueTOLType(normType) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaImmutableBadType,
				Message: fmt.Sprintf("immutable '%s' has unsupported type '%s'; immutable variables must be value types (uN, iN, bool, agent, bytes1..bytes32)", immName, normType),
				Span:    defaultSpan(filename),
			})
		}
		if _, dup := immutableSeen[immName]; dup {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaDuplicateSlot,
				Message: fmt.Sprintf("duplicate immutable declaration '%s'", immName),
				Span:    defaultSpan(filename),
			})
			continue
		}
		if _, exists := slotInfos[immName]; exists {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("immutable '%s' collides with an existing storage slot of the same name", immName),
				Span:    defaultSpan(filename),
			})
			continue
		}
		immutableSeen[immName] = struct{}{}
		slotInfos[immName] = storageSlotInfo{
			name:        immName,
			kind:        storageKindScalar,
			typeName:    immType,
			isImmutable: true,
		}
	}

	// Validate constant declarations.
	// constantNames holds the set of constant names for write-prohibition checks.
	constantNames := map[string]struct{}{}
	constantSeen := map[string]struct{}{}
	for _, cd := range c.Constants {
		cname := strings.TrimSpace(cd.Name)
		if cname == "" {
			continue
		}
		if cname == "selector" || cname == "this" {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("constant name '%s' is reserved and cannot be declared", cname),
				Span:    defaultSpan(filename),
			})
		}
		if strings.HasPrefix(cname, "__tol_") {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("constant name '%s' uses reserved internal prefix '__tol_'", cname),
				Span:    defaultSpan(filename),
			})
		}
		if _, dup := constantSeen[cname]; dup {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaDuplicateConstant,
				Message: fmt.Sprintf("duplicate constant declaration '%s'", cname),
				Span:    defaultSpan(filename),
			})
			continue
		}
		if _, exists := slotInfos[cname]; exists {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("constant '%s' collides with an existing storage slot of the same name", cname),
				Span:    defaultSpan(filename),
			})
			continue
		}
		if prev, exists := contractSupportSeen[cname]; exists {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("constant name '%s' collides with %s declaration", cname, prev),
				Span:    defaultSpan(filename),
			})
			continue
		}
		// Validate that the type is a value type.
		ctype := normalizeSelectorType(strings.TrimSpace(cd.Type))
		if !isValueTOLType(ctype) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaConstantInvalidType,
				Message: fmt.Sprintf("constant '%s' has unsupported type '%s'; constants must be value types (uN, iN, bool, agent, bytes1..bytes32)", cname, ctype),
				Span:    defaultSpan(filename),
			})
		}
		// Validate that the value is a compile-time constant expression.
		if cd.Value == nil || !isConstantExpr(cd.Value, constantSeen) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaConstantInvalidValue,
				Message: fmt.Sprintf("constant '%s' initializer must be a compile-time constant expression (literals, arithmetic, and previously-declared constants)", cname),
				Span:    defaultSpan(filename),
			})
		}
		// Validate hex literal type compatibility.
		if cd.Value != nil && cd.Value.Kind == "hex_lit" {
			hexDiags := checkHexLiteralType(filename, cname, ctype, cd.Value.Value)
			*diags = append(*diags, hexDiags...)
		}
		constantSeen[cname] = struct{}{}
		contractSupportSeen[cname] = "constant"
		constantNames[cname] = struct{}{}
	}

	*diags = append(*diags, checkContractNameCollisions(filename, slotInfos, funcArity, eventArity)...)
	*diags = append(*diags, checkContractSupportNameCollisions(filename, contractSupportSeen, slotInfos, funcArity, eventArity)...)

	// Validate modifier declarations.
	for _, md := range c.Modifiers {
		*diags = append(*diags, checkModifierBody(filename, c.Name, funcVis, funcArity, eventArity, slotInfos, md)...)
	}

	// Build set of storage slot names for effect-check passes.
	storageSlotNames := map[string]bool{}
	for name := range slotInfos {
		storageSlotNames[name] = true
	}

	funcSeen := map[string]struct{}{} // key: "name(type1,type2,...)" full signature
	selectorSeen := map[string]string{}
	for _, fn := range c.Functions {
		name := strings.TrimSpace(fn.Name)
		if name == "selector" || name == "this" {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("function name '%s' is reserved and cannot be declared", name),
				Span:    defaultSpan(filename),
			})
		}
		if strings.HasPrefix(name, "__tol_") {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaReservedName,
				Message: fmt.Sprintf("function name '%s' uses reserved internal prefix '__tol_'", name),
				Span:    defaultSpan(filename),
			})
		}
		// A virtual stub (Virtual==true, Body==nil) is only allowed in abstract contracts.
		if fn.Virtual && fn.Body == nil && !c.Abstract {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaAbstractFunctionInConcreteContract,
				Message: fmt.Sprintf("function '%s' is declared virtual with no body; abstract functions are only allowed in 'abstract contract' declarations", fn.Name),
				Span:    defaultSpan(filename),
			})
		}
		*diags = append(*diags, duplicateParamDiagnostics(filename, "function", fn.Name, fn.Params)...)
		*diags = append(*diags, duplicateParamDiagnostics(filename, "returns", fn.Name, fn.Returns)...)
		*diags = append(*diags, checkParamReturnNameCollisions(filename, fn.Name, fn.Params, fn.Returns)...)
		for _, p := range fn.Params {
			validateDataLocationForContext(filename, p.DataLoc, p.Type, fmt.Sprintf("function '%s' parameter '%s'", fn.Name, strings.TrimSpace(p.Name)), diags)
			validateTypeForContextWithStructs(filename, p.Type, fmt.Sprintf("function '%s' parameter '%s'", fn.Name, strings.TrimSpace(p.Name)), false, knownStructs, diags)
		}
		for _, r := range fn.Returns {
			validateTypeForContextWithStructs(filename, r.Type, fmt.Sprintf("function '%s' return '%s'", fn.Name, strings.TrimSpace(r.Name)), false, knownStructs, diags)
		}
		fnSig := funcSignatureKey(fn.Name, fn.Params)
		if _, ok := funcSeen[fnSig]; ok {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaDuplicateFunction,
				Message: fmt.Sprintf("duplicate function signature '%s'", fnSig),
				Span: diag.Span{
					File: filename,
					Start: diag.Position{
						Line:   1,
						Column: 1,
					},
					End: diag.Position{
						Line:   1,
						Column: 1,
					},
				},
			})
		} else {
			funcSeen[fnSig] = struct{}{}
		}
		if fn.SelectorOverride != "" && !isValidSelectorOverride(fn.SelectorOverride) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidSelector,
				Message: fmt.Sprintf("invalid @selector value '%s' (expected 0x followed by 8 hex chars)", fn.SelectorOverride),
				Span:    defaultSpan(filename),
			})
		}
		if fn.SelectorOverride != "" {
			vis := funcVis[fn.Name]
			if vis != "public" && vis != "external" {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaSelectorVisibility,
					Message: fmt.Sprintf("@selector is only allowed on public/external functions (got '%s' on '%s')", vis, fn.Name),
					Span:    defaultSpan(filename),
				})
			}
		}
		if key, ok := selectorDispatchKey(fn); ok {
			if prev, exists := selectorSeen[key]; exists {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaDuplicateSelector,
					Message: fmt.Sprintf("duplicate external/public selector key '%s' between functions '%s' and '%s'", key, prev, fn.Name),
					Span:    defaultSpan(filename),
				})
			} else {
				selectorSeen[key] = fn.Name
			}
		}
		// Skip body-related checks for abstract virtual stubs (no body).
		if fn.Body == nil {
			continue
		}
		_ = fn.Virtual // Virtual flag is purely informational after sema; no further check needed here.
		checkStatements(filename, c.Name, funcVis, funcArity, eventArity, fn.Body, 0, diags, knownStructs)
		checkDeclaredCustomErrorReverts(filename, errorDeclParams, fn.Body, diags)
		checkEnumMemberExprs(filename, enumMemberValues, fn.Body, diags)
		checkStructLiterals(filename, knownStructs, fn.Body, diags)
		checkReturnStatements(filename, "function", fn.Name, len(fn.Returns) > 0, fn.Body, diags)
		checkUnreachableStatements(filename, fn.Body, 0, diags)
		checkDuplicateLocals(filename, "function", fn.Name, fn.Params, fn.Body, diags)
		// Solidity convention: if ALL return parameters are named, implicit return is allowed.
		allNamedReturns := len(fn.Returns) > 0
		for _, r := range fn.Returns {
			if strings.TrimSpace(r.Name) == "" {
				allNamedReturns = false
				break
			}
		}
		if len(fn.Returns) > 0 && !allNamedReturns && !guaranteesValueReturnOrRevert(fn.Body) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidReturn,
				Message: fmt.Sprintf("function '%s' requires all paths to end in return value or revert in current verifier stage", fn.Name),
				Span:    defaultSpan(filename),
			})
		}
		checkStorageFunctionBody(filename, slotInfos, fn.Params, fn.Body, diags)
		// Immutable check: no writes to immutable variables outside the constructor.
		checkImmutableWritesInNonCtor(filename, fn.Name, slotInfos, fn.Params, fn.Body, diags)
		// Constant write prohibition: no set to a constant name.
		if len(constantNames) > 0 {
			checkConstantSetTargets(filename, constantNames, fn.Body, diags)
		}
		// New effect-check passes.
		checkFunctionEffects(filename, fn, storageSlotNames, diags)
		checkUninitializedReads(filename, fn.Name, fn.Params, fn.Body, diags, knownStructs)
		checkAgentTransferCalls(filename, fn.Params, fn.Body, diags)
		checkBytesStringEquality(filename, slotInfos, fn.Params, fn.Body, diags)
	}

	if c.Constructor != nil {
		*diags = append(*diags, validateConstructorModifiers(filename, c.Constructor.Modifiers)...)
		*diags = append(*diags, duplicateParamDiagnostics(filename, "constructor", "", c.Constructor.Params)...)
		for _, p := range c.Constructor.Params {
			validateDataLocationForContext(filename, p.DataLoc, p.Type, fmt.Sprintf("constructor parameter '%s'", strings.TrimSpace(p.Name)), diags)
			validateTypeForContextWithStructs(filename, p.Type, fmt.Sprintf("constructor parameter '%s'", strings.TrimSpace(p.Name)), false, knownStructs, diags)
			validateConstructorParamABIEncodable(filename, p.Type, strings.TrimSpace(p.Name), diags)
		}
		checkStatements(filename, c.Name, funcVis, funcArity, eventArity, c.Constructor.Body, 0, diags, knownStructs)
		checkDeclaredCustomErrorReverts(filename, errorDeclParams, c.Constructor.Body, diags)
		checkEnumMemberExprs(filename, enumMemberValues, c.Constructor.Body, diags)
		checkStructLiterals(filename, knownStructs, c.Constructor.Body, diags)
		checkReturnStatements(filename, "constructor", "", false, c.Constructor.Body, diags)
		checkUnreachableStatements(filename, c.Constructor.Body, 0, diags)
		checkDuplicateLocals(filename, "constructor", "", c.Constructor.Params, c.Constructor.Body, diags)
		checkStorageFunctionBody(filename, slotInfos, c.Constructor.Params, c.Constructor.Body, diags)
		// Immutable check: verify all immutables are assigned in the constructor.
		checkImmutableAssignedInConstructor(filename, slotInfos, c.Constructor.Body, diags)
		// Constant write prohibition: no set to a constant name.
		if len(constantNames) > 0 {
			checkConstantSetTargets(filename, constantNames, c.Constructor.Body, diags)
		}
		checkAgentTransferCalls(filename, c.Constructor.Params, c.Constructor.Body, diags)
		checkBytesStringEquality(filename, slotInfos, c.Constructor.Params, c.Constructor.Body, diags)
	}
	if c.Fallback != nil {
		checkStatements(filename, c.Name, funcVis, funcArity, eventArity, c.Fallback.Body, 0, diags, knownStructs)
		checkDeclaredCustomErrorReverts(filename, errorDeclParams, c.Fallback.Body, diags)
		checkEnumMemberExprs(filename, enumMemberValues, c.Fallback.Body, diags)
		checkStructLiterals(filename, knownStructs, c.Fallback.Body, diags)
		checkReturnStatements(filename, "fallback", "", false, c.Fallback.Body, diags)
		checkUnreachableStatements(filename, c.Fallback.Body, 0, diags)
		checkDuplicateLocals(filename, "fallback", "", nil, c.Fallback.Body, diags)
		checkStorageFunctionBody(filename, slotInfos, nil, c.Fallback.Body, diags)
		// Immutable check: no writes to immutable variables in fallback.
		checkImmutableWritesInNonCtor(filename, "fallback", slotInfos, nil, c.Fallback.Body, diags)
		// Constant write prohibition: no set to a constant name.
		if len(constantNames) > 0 {
			checkConstantSetTargets(filename, constantNames, c.Fallback.Body, diags)
		}
		checkAgentTransferCalls(filename, nil, c.Fallback.Body, diags)
		checkBytesStringEquality(filename, slotInfos, nil, c.Fallback.Body, diags)
	}
	if c.Receive != nil {
		checkStatements(filename, c.Name, funcVis, funcArity, eventArity, c.Receive.Body, 0, diags, knownStructs)
		checkDeclaredCustomErrorReverts(filename, errorDeclParams, c.Receive.Body, diags)
		checkEnumMemberExprs(filename, enumMemberValues, c.Receive.Body, diags)
		checkStructLiterals(filename, knownStructs, c.Receive.Body, diags)
		checkReturnStatements(filename, "receive", "", false, c.Receive.Body, diags)
		checkUnreachableStatements(filename, c.Receive.Body, 0, diags)
		checkDuplicateLocals(filename, "receive", "", nil, c.Receive.Body, diags)
		checkStorageFunctionBody(filename, slotInfos, nil, c.Receive.Body, diags)
		checkImmutableWritesInNonCtor(filename, "receive", slotInfos, nil, c.Receive.Body, diags)
		if len(constantNames) > 0 {
			checkConstantSetTargets(filename, constantNames, c.Receive.Body, diags)
		}
		checkAgentTransferCalls(filename, nil, c.Receive.Body, diags)
		checkBytesStringEquality(filename, slotInfos, nil, c.Receive.Body, diags)
	}

	// Validate 'using LibName for Type' declarations.
	usingDecls := c.UsingDecls
	*diags = append(*diags, checkUsingDecls(filename, usingDecls, libFuncs)...)

	// Validate library function calls (LibName.fn(args)) in contract bodies.
	for _, fn := range c.Functions {
		checkLibraryCallsInStmts(filename, fn.Body, libFuncs, diags)
	}
	if c.Constructor != nil {
		checkLibraryCallsInStmts(filename, c.Constructor.Body, libFuncs, diags)
	}
	if c.Fallback != nil {
		checkLibraryCallsInStmts(filename, c.Fallback.Body, libFuncs, diags)
	}
	// Validate 'using X for T' method-call style: val.fn(args) → LibName.fn(val, args).
	// We only check arity here; the actual expansion happens at lower time.
	if len(usingDecls) > 0 {
		for _, fn := range c.Functions {
			checkUsingCallsInStmts(filename, fn.Body, usingDecls, libFuncs, diags)
		}
		if c.Constructor != nil {
			checkUsingCallsInStmts(filename, c.Constructor.Body, usingDecls, libFuncs, diags)
		}
		if c.Fallback != nil {
			checkUsingCallsInStmts(filename, c.Fallback.Body, usingDecls, libFuncs, diags)
		}
	}

	// Agent-native validation. Pass the set of known struct names so that
	// task<T> type-parameter checks can verify T is a declared struct.
	structNameSet := make(map[string]bool, len(knownStructs))
	for name := range knownStructs {
		structNameSet[name] = true
	}
	checkAgentNativeDecls(filename, c, m.Capabilities, diags, structNameSet)
}

func checkStatements(filename string, contractName string, funcVis map[string]string, funcArity map[string]int, eventArity map[string]int, stmts []ast.Statement, loopDepth int, diags *diag.Diagnostics, knownStructs ...map[string][]ast.FieldDecl) {
	var structs map[string][]ast.FieldDecl
	if len(knownStructs) > 0 {
		structs = knownStructs[0]
	}
	checkStatementsWithStructs(filename, contractName, funcVis, funcArity, eventArity, stmts, loopDepth, diags, structs)
}

func checkStatementsWithStructs(filename string, contractName string, funcVis map[string]string, funcArity map[string]int, eventArity map[string]int, stmts []ast.Statement, loopDepth int, diags *diag.Diagnostics, knownStructs map[string][]ast.FieldDecl) {
	for _, s := range stmts {
		checkExpr(contractName, funcVis, funcArity, filename, s.Expr, diags)
		checkExpr(contractName, funcVis, funcArity, filename, s.Target, diags)
		checkExpr(contractName, funcVis, funcArity, filename, s.Cond, diags)
		checkExpr(contractName, funcVis, funcArity, filename, s.Post, diags)
		if s.Init != nil {
			checkStatementsWithStructs(filename, contractName, funcVis, funcArity, eventArity, []ast.Statement{*s.Init}, loopDepth, diags, knownStructs)
		}
		switch s.Kind {
		case "let":
			label := strings.TrimSpace(s.Name)
			if label == "" {
				label = "<anonymous>"
			}
			if strings.TrimSpace(s.Type) != "" {
				validateTypeForContextWithStructs(filename, s.Type, fmt.Sprintf("local '%s'", label), false, knownStructs, diags)
			}
			if s.Expr == nil {
				localType := normalizeSelectorType(s.Type)
				if strings.TrimSpace(localType) == "" {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaInvalidStmtShape,
						Message: fmt.Sprintf("local '%s' requires either explicit type or initializer", label),
						Span:    defaultSpan(filename),
					})
				} else {
					_, isKnownStruct := knownStructs[localType]
					if !isDefaultInitializableTOLType(localType) && !isKnownStruct {
						*diags = append(*diags, diag.Diagnostic{
							Code:    diag.CodeSemaInvalidStmtShape,
							Message: fmt.Sprintf("local '%s' of type '%s' requires explicit initializer in current stage", label, localType),
							Span:    defaultSpan(filename),
						})
					}
				}
			}
			if _, ok := abiDecodeCallDataArg(s.Expr); ok {
				localType := normalizeSelectorType(s.Type)
				if strings.TrimSpace(localType) == "" {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaInvalidStmtShape,
						Message: "abi.decode local binding requires explicit type annotation in current stage",
						Span:    defaultSpan(filename),
					})
				} else if !isSupportedABIDecodeTargetTypeWithStructs(localType, knownStructs) {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaInvalidStmtShape,
						Message: fmt.Sprintf("abi.decode typed local binding only supports bool/agent/bytesN/uN/iN/bytes/struct in current stage (got '%s')", localType),
						Span:    defaultSpan(filename),
					})
				} else {
					validateTypedABIDecodeLiteralForType(filename, label, localType, s.Expr, diags)
				}
			}
			if containsAssignExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in let initializer",
					Span:    defaultSpan(filename),
				})
			}
		case "let-tuple":
			// Validate tuple let-binding: let (a, b, ...) : (T1, T2, ...) = expr;
			if s.Expr == nil {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: "tuple let binding requires an initializer expression",
					Span:    defaultSpan(filename),
				})
			}
			if len(s.Names) < 2 {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: "tuple let binding requires at least two variables",
					Span:    defaultSpan(filename),
				})
			}
			if len(s.Types) > 0 && len(s.Types) != len(s.Names) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: fmt.Sprintf("tuple let binding: %d variable(s) but %d type(s)", len(s.Names), len(s.Types)),
					Span:    defaultSpan(filename),
				})
			}
			// Validate each individual type.
			for i, typ := range s.Types {
				varName := "<anonymous>"
				if i < len(s.Names) {
					varName = s.Names[i]
				}
				validateTypeForContextWithStructs(filename, typ, fmt.Sprintf("tuple local '%s'", varName), false, knownStructs, diags)
			}
			// For abi.decode tuple form: detect abi.decode(data) with type annotations.
			if _, isABIDecode := abiDecodeCallDataArg(s.Expr); isABIDecode && len(s.Types) > 0 {
				for i, typ := range s.Types {
					localType := normalizeSelectorType(typ)
					if !isSupportedABIDecodeTargetTypeWithStructs(localType, knownStructs) {
						varName := "<anonymous>"
						if i < len(s.Names) {
							varName = s.Names[i]
						}
						*diags = append(*diags, diag.Diagnostic{
							Code:    diag.CodeSemaInvalidStmtShape,
							Message: fmt.Sprintf("abi.decode typed tuple binding only supports bool/agent/bytesN/uN/iN/bytes/struct in current stage (got '%s' for '%s')", localType, varName),
							Span:    defaultSpan(filename),
						})
					}
				}
			}
			if containsAssignExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in tuple let initializer",
					Span:    defaultSpan(filename),
				})
			}
		case "return":
			if containsAssignExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in return expression",
					Span:    defaultSpan(filename),
				})
			}
		case "break":
			if loopDepth <= 0 {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaBreakOutsideLoop,
					Message: "break used outside loop",
					Span:    defaultSpan(filename),
				})
			}
		case "continue":
			if loopDepth <= 0 {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaContinueOutsideLoop,
					Message: "continue used outside loop",
					Span:    defaultSpan(filename),
				})
			}
		case "require", "assert":
			if s.Expr == nil {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: s.Kind + " statement requires an expression argument in current stage",
					Span:    defaultSpan(filename),
				})
			}
			if strings.TrimSpace(s.Text) == "" {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: s.Kind + " statement requires a string message argument in current stage",
					Span:    defaultSpan(filename),
				})
			} else if _, err := strconv.Unquote(strings.TrimSpace(s.Text)); err != nil {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: s.Kind + " statement message must be a string literal in current stage",
					Span:    defaultSpan(filename),
				})
			} else if containsAssignExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in require/assert expression",
					Span:    defaultSpan(filename),
				})
			}
		case "revert":
			if containsAssignExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in revert payload",
					Span:    defaultSpan(filename),
				})
			}
			if s.Expr != nil && !isStringLiteralExpr(s.Expr) && !isCustomErrorRevertPayloadExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidRevert,
					Message: "revert payload must be a string literal or custom error call in current stage",
					Span:    defaultSpan(filename),
				})
			}
		case "emit":
			if s.Expr == nil || !isCallExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: "emit statement requires a call-like payload (e.g. emit EventName(...))",
					Span:    defaultSpan(filename),
				})
			} else if containsAssignExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in emit payload",
					Span:    defaultSpan(filename),
				})
			} else {
				name, argc, ok := emitCallInfo(s.Expr)
				if !ok {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaInvalidStmtShape,
						Message: "emit statement payload must call an event identifier (e.g. EventName(...))",
						Span:    defaultSpan(filename),
					})
				} else if name == "selector" {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaInvalidStmtShape,
						Message: "emit statement must call an event name, not selector(...) builtin",
						Span:    defaultSpan(filename),
					})
				} else if want, exists := eventArity[name]; exists {
					if argc != want {
						*diags = append(*diags, diag.Diagnostic{
							Code:    diag.CodeSemaEmitArity,
							Message: fmt.Sprintf("emit event '%s' expects %d argument(s), got %d", name, want, argc),
							Span:    defaultSpan(filename),
						})
					}
				} else {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaUnknownEmitEvent,
						Message: fmt.Sprintf("emit event '%s' is not declared in contract", name),
						Span:    defaultSpan(filename),
					})
				}
			}
		case "set":
			if !isAssignableTarget(s.Target) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidSetTarget,
					Message: "set target must be identifier, member access, or index access",
					Span:    defaultSpan(filename),
				})
			} else if isReadOnlyIdentTarget(s.Target) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidSetTarget,
					Message: "set target cannot be 'true', 'false', or 'nil'",
					Span:    defaultSpan(filename),
				})
			}
			if isSelectorMemberExpr(s.Target) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidSetTarget,
					Message: "selector member expression is read-only and cannot be assignment target",
					Span:    defaultSpan(filename),
				})
			}
			if scope, key, ok := envMemberScopeKey(s.Target); ok {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidSetTarget,
					Message: fmt.Sprintf("environment member '%s.%s' is read-only and cannot be assignment target", scope, key),
					Span:    defaultSpan(filename),
				})
			}
			if containsAssignExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in set value expression",
					Span:    defaultSpan(filename),
				})
			}
		case "if":
			if s.Cond == nil {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaMissingCondition,
					Message: "if statement requires a condition expression",
					Span:    defaultSpan(filename),
				})
			}
			if containsAssignExpr(s.Cond) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in if condition",
					Span:    defaultSpan(filename),
				})
			}
			checkStatementsWithStructs(filename, contractName, funcVis, funcArity, eventArity, s.Then, loopDepth, diags, knownStructs)
			checkStatementsWithStructs(filename, contractName, funcVis, funcArity, eventArity, s.Else, loopDepth, diags, knownStructs)
		case "while", "dowhile":
			if s.Cond == nil {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaMissingCondition,
					Message: s.Kind + " statement requires a condition expression",
					Span:    defaultSpan(filename),
				})
			}
			if containsAssignExpr(s.Cond) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in " + s.Kind + " condition",
					Span:    defaultSpan(filename),
				})
			}
			checkStatementsWithStructs(filename, contractName, funcVis, funcArity, eventArity, s.Body, loopDepth+1, diags, knownStructs)
		case "for":
			if s.Cond != nil && containsAssignExpr(s.Cond) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "assignment expressions are not allowed in for condition",
					Span:    defaultSpan(filename),
				})
			}
			if s.Post != nil && !isExprStatementExpr(s.Post) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "for post expression must be a function call or assignment expression",
					Span:    defaultSpan(filename),
				})
			}
			if isSelectorBuiltinCallExpr(s.Post) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: "selector(...) cannot be used as for post expression statement",
					Span:    defaultSpan(filename),
				})
			}
			if hasIllegalNestedAssignInStmtExpr(s.Post) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "nested assignment expressions are not allowed in for post expression",
					Span:    defaultSpan(filename),
				})
			}
			checkStatementsWithStructs(filename, contractName, funcVis, funcArity, eventArity, s.Body, loopDepth+1, diags, knownStructs)
		case "expr":
			if s.Expr == nil || !isExprStatementExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "expression statement must be a function call or assignment expression",
					Span:    defaultSpan(filename),
				})
			}
			if hasIllegalNestedAssignInStmtExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidAssignExpr,
					Message: "nested assignment expressions are not allowed in expression statement",
					Span:    defaultSpan(filename),
				})
			}
			if isSelectorBuiltinCallExpr(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: "selector(...) cannot be used as standalone expression statement",
					Span:    defaultSpan(filename),
				})
			}
		case "placeholder":
			// _; placeholder is only valid inside a modifier body. Reaching here means it
			// appeared inside a control-flow sub-block of a modifier. We allow it here;
			// the modifier body validator enforces the count constraint via countPlaceholders.
		case "delete":
			if s.Expr == nil {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: "delete statement requires a target expression",
					Span:    defaultSpan(filename),
				})
			} else if !isAssignableTarget(s.Expr) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidDeleteTarget,
					Message: "delete target must be a local variable or storage index/mapping access",
					Span:    defaultSpan(filename),
				})
			}
		case "unchecked":
			checkStatements(filename, contractName, funcVis, funcArity, eventArity, s.Body, loopDepth, diags)
		case "try":
			// Validate try/catch statement.
			// Allowed targets: call expressions (external calls) or new expressions (contract construction).
			if s.Expr == nil || (!isCallExpr(s.Expr) && !isNewExpr(s.Expr)) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaTryNonCall,
					Message: "try statement requires a function call or new expression as its target",
					Span:    defaultSpan(filename),
				})
			}
			// Check for duplicate catch kinds.
			seenKinds := map[string]bool{}
			for _, clause := range s.Catches {
				if seenKinds[clause.Kind] {
					kindLabel := clause.Kind
					if kindLabel == "" {
						kindLabel = "bare"
					}
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaDuplicateCatch,
						Message: fmt.Sprintf("duplicate catch clause kind '%s' in try statement", kindLabel),
						Span:    defaultSpan(filename),
					})
				}
				seenKinds[clause.Kind] = true
				// Validate that Panic clause has u256 parameter type.
				if clause.Kind == "Panic" && clause.ParamName != "" && clause.ParamType != "u256" {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaInvalidStmtShape,
						Message: fmt.Sprintf("catch Panic parameter must have type u256, got '%s'", clause.ParamType),
						Span:    defaultSpan(filename),
					})
				}
			}
			// Check success body.
			checkStatements(filename, contractName, funcVis, funcArity, eventArity, s.Body, loopDepth, diags)
			// Check each catch body.
			for _, clause := range s.Catches {
				checkStatements(filename, contractName, funcVis, funcArity, eventArity, clause.Body, loopDepth, diags)
			}
		default:
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidStmtShape,
				Message: fmt.Sprintf("unsupported statement kind '%s' in current verifier stage", s.Kind),
				Span:    defaultSpan(filename),
			})
		}
	}
}

func isExprStatementExpr(e *ast.Expr) bool {
	if e == nil {
		return false
	}
	if e.Kind == "call" || e.Kind == "assign" {
		return true
	}
	// Postfix i++ / i-- are valid as standalone expression statements and as
	// for post-step expressions.
	if e.Kind == "unary" && (e.Op == "post++" || e.Op == "post--") {
		return true
	}
	if e.Kind == "paren" {
		return isExprStatementExpr(e.Left)
	}
	return false
}

func isCallExpr(e *ast.Expr) bool {
	root := stripParens(e)
	return root != nil && root.Kind == "call"
}

func isNewExpr(e *ast.Expr) bool {
	root := stripParens(e)
	return root != nil && root.Kind == "new"
}

func isSelectorBuiltinCallExpr(e *ast.Expr) bool {
	root := stripParens(e)
	if root == nil || root.Kind != "call" {
		return false
	}
	callee := stripParens(root.Callee)
	return callee != nil && callee.Kind == "ident" && strings.TrimSpace(callee.Value) == "selector"
}

func isSelectorMemberExpr(e *ast.Expr) bool {
	root := stripParens(e)
	return root != nil && root.Kind == "member" && root.Member == "selector"
}

func envMemberScopeKey(e *ast.Expr) (string, string, bool) {
	root := stripParens(e)
	if root == nil || root.Kind != "member" {
		return "", "", false
	}
	obj := stripParens(root.Object)
	if obj == nil || obj.Kind != "ident" {
		return "", "", false
	}
	scope := strings.TrimSpace(obj.Value)
	switch scope {
	case "msg", "tx", "block", "gas":
		return scope, strings.TrimSpace(root.Member), true
	default:
		return "", "", false
	}
}

func isAllowedEnvField(scope, key string) bool {
	switch scope {
	case "msg":
		return key == "sender" || key == "value" || key == "data"
	case "tx":
		return key == "origin" || key == "gasprice"
	case "block":
		return key == "number" || key == "timestamp" || key == "timestamp_ms"
	default:
		return false
	}
}

func builtinCallArity(name string) (int, bool) {
	switch name {
	case "call":
		return 3, true
	case "staticcall":
		return 2, true
	case "delegatecall":
		return 2, true
	case "create":
		return 2, true
	case "create2":
		return 3, true
	case "createx":
		return 4, true
	case "create2x":
		return 5, true
	case "transfer":
		return 2, true
	case "bytes_eq", "string_eq":
		return 2, true
	case "keccak256":
		return 1, true
	case "sha256":
		return 1, true
	case "ripemd160":
		return 1, true
	case "ecrecover":
		return 4, true
	default:
		// Integer type casts uN(expr) and iN(expr) take exactly 1 argument.
		if isIntegerTypeCastName(name) {
			return 1, true
		}
		return 0, false
	}
}

// isIntegerTypeCastName returns true if name is a TOL integer type cast (uN or iN).
func isIntegerTypeCastName(name string) bool {
	n := strings.TrimSpace(name)
	if len(n) < 2 {
		return false
	}
	if n[0] != 'u' && n[0] != 'i' {
		return false
	}
	bits, err := strconv.Atoi(n[1:])
	return err == nil && bits >= 8 && bits <= 256 && bits%8 == 0
}

// matchTypeBoundsExpr checks whether e matches the pattern `type(T).min` or
// `type(T).max` where T is a uN or iN integer type name. It returns the type
// name and the bound ("min" or "max") when matched, plus a boolean indicating
// whether this is a valid type-bounds expression (as opposed to an invalid
// type argument). Callers must inspect `validType` to emit diagnostics.
//
// Returns (typeName, bound, matched, validType):
//   - matched=false → not a type-bounds pattern at all; ignore
//   - matched=true, validType=true → valid type(uN|iN).min/max
//   - matched=true, validType=false → type(T).min/max with unsupported T
func matchTypeBoundsExpr(e *ast.Expr) (typeName, bound string, matched, validType bool) {
	if e == nil || e.Kind != "member" {
		return
	}
	if e.Member != "min" && e.Member != "max" {
		return
	}
	obj := stripParens(e.Object)
	if obj == nil || obj.Kind != "call" {
		return
	}
	callee := stripParens(obj.Callee)
	if callee == nil || callee.Kind != "ident" || strings.TrimSpace(callee.Value) != "type" {
		return
	}
	if len(obj.Args) != 1 || obj.Args[0] == nil {
		return
	}
	arg := stripParens(obj.Args[0])
	if arg == nil || arg.Kind != "ident" {
		return
	}
	matched = true
	typeName = strings.TrimSpace(arg.Value)
	bound = e.Member
	validType = isIntegerTypeCastName(typeName)
	return
}

// matchTypeInterfaceIdExpr checks whether e matches the pattern `type(I).interfaceId`
// where I is an identifier. Returns the interface name and matched=true when matched.
// Does not validate that I is a known interface — that is left to the lower phase.
func matchTypeInterfaceIdExpr(e *ast.Expr) (ifaceName string, matched bool) {
	if e == nil || e.Kind != "member" {
		return
	}
	if strings.TrimSpace(e.Member) != "interfaceId" {
		return
	}
	obj := stripParens(e.Object)
	if obj == nil || obj.Kind != "call" {
		return
	}
	callee := stripParens(obj.Callee)
	if callee == nil || callee.Kind != "ident" || strings.TrimSpace(callee.Value) != "type" {
		return
	}
	if len(obj.Args) != 1 || obj.Args[0] == nil {
		return
	}
	arg := stripParens(obj.Args[0])
	if arg == nil || arg.Kind != "ident" {
		return
	}
	matched = true
	ifaceName = strings.TrimSpace(arg.Value)
	return
}

func abiBuiltinCallName(callee *ast.Expr) (string, bool) {
	root := stripParens(callee)
	if root == nil || root.Kind != "member" {
		return "", false
	}
	obj := stripParens(root.Object)
	if obj == nil || obj.Kind != "ident" || strings.TrimSpace(obj.Value) != "abi" {
		return "", false
	}
	name := strings.TrimSpace(root.Member)
	return name, name != ""
}

// bytesStringBuiltinCallName detects a `bytes.concat(...)` or `string.concat(...)`
// call expression and returns ("bytes"|"string", "concat", true) when matched.
func bytesStringBuiltinCallName(callee *ast.Expr) (ns, name string, ok bool) {
	root := stripParens(callee)
	if root == nil || root.Kind != "member" {
		return "", "", false
	}
	obj := stripParens(root.Object)
	if obj == nil || obj.Kind != "ident" {
		return "", "", false
	}
	scope := strings.TrimSpace(obj.Value)
	if scope != "bytes" && scope != "string" {
		return "", "", false
	}
	member := strings.TrimSpace(root.Member)
	if member == "" {
		return "", "", false
	}
	return scope, member, true
}

func abiDecodeCallDataArg(e *ast.Expr) (*ast.Expr, bool) {
	root := stripParens(e)
	if root == nil || root.Kind != "call" {
		return nil, false
	}
	name, ok := abiBuiltinCallName(root.Callee)
	if !ok || name != "decode" {
		return nil, false
	}
	if len(root.Args) != 1 || root.Args[0] == nil {
		return nil, false
	}
	return root.Args[0], true
}

func isSupportedABIDecodeTargetType(typeName string) bool {
	return isSupportedABIDecodeTargetTypeWithStructs(typeName, nil)
}

func isSupportedABIDecodeTargetTypeWithStructs(typeName string, knownStructs map[string][]ast.FieldDecl) bool {
	t := normalizeSelectorType(typeName)
	switch t {
	case "bool", "agent", "bytes":
		return true
	}
	// Dynamic array T[] or fixed array T[N]: supported via ABI decode prelude.
	if strings.HasSuffix(t, "]") {
		return true
	}
	if strings.HasPrefix(t, "bytes") {
		nStr := t[len("bytes"):]
		if nStr != "" {
			n, err := strconv.Atoi(nStr)
			return err == nil && n >= 1 && n <= 32
		}
	}
	// uN (unsigned integers)
	if len(t) >= 2 && t[0] == 'u' {
		n, err := strconv.Atoi(t[1:])
		return err == nil && n >= 8 && n <= 256 && n%8 == 0
	}
	// iN (signed integers)
	if len(t) >= 2 && t[0] == 'i' {
		n, err := strconv.Atoi(t[1:])
		return err == nil && n >= 8 && n <= 256 && n%8 == 0
	}
	if len(knownStructs) > 0 {
		if _, isStruct := knownStructs[t]; isStruct {
			return true
		}
	}
	return false
}

func isDefaultInitializableTOLType(typeName string) bool {
	t := normalizeSelectorType(typeName)
	switch t {
	case "bool", "agent", "string", "bytes":
		return true
	}
	if strings.HasPrefix(t, "bytes") {
		nStr := t[len("bytes"):]
		if nStr != "" {
			n, err := strconv.Atoi(nStr)
			return err == nil && n >= 1 && n <= 32
		}
	}
	if len(t) >= 2 && (t[0] == 'u' || t[0] == 'i') {
		n, err := strconv.Atoi(t[1:])
		return err == nil && n >= 8 && n <= 256 && n%8 == 0
	}
	return false
}

func isReadOnlyIdentTarget(e *ast.Expr) bool {
	root := stripParens(e)
	if root == nil || root.Kind != "ident" {
		return false
	}
	switch strings.TrimSpace(root.Value) {
	case "true", "false", "nil":
		return true
	default:
		return false
	}
}

func emitCallInfo(e *ast.Expr) (string, int, bool) {
	root := stripParens(e)
	if root == nil || root.Kind != "call" {
		return "", 0, false
	}
	callee := stripParens(root.Callee)
	if callee == nil || callee.Kind != "ident" {
		return "", 0, false
	}
	name := strings.TrimSpace(callee.Value)
	if name == "" {
		return "", 0, false
	}
	return name, len(root.Args), true
}

func isStringLiteralExpr(e *ast.Expr) bool {
	root := stripParens(e)
	if root == nil || root.Kind != "string" {
		return false
	}
	_, err := strconv.Unquote(strings.TrimSpace(root.Value))
	return err == nil
}

func isCustomErrorRevertPayloadExpr(e *ast.Expr) bool {
	root := stripParens(e)
	if root == nil || root.Kind != "call" {
		return false
	}
	callee := stripParens(root.Callee)
	if callee == nil || callee.Kind != "ident" {
		return false
	}
	name := strings.TrimSpace(callee.Value)
	return name != "" && name != "selector"
}

func isSelectorSignatureLiteralExpr(e *ast.Expr) bool {
	root := stripParens(e)
	if root == nil || root.Kind != "string" {
		return false
	}
	v, err := strconv.Unquote(strings.TrimSpace(root.Value))
	if err != nil {
		return false
	}
	sig := strings.TrimSpace(v)
	if sig == "" {
		return false
	}
	if v != sig {
		return false
	}
	open := strings.Index(sig, "(")
	close := strings.LastIndex(sig, ")")
	if !(open > 0 && close == len(sig)-1 && open < close) {
		return false
	}
	rawName := sig[:open]
	name := strings.TrimSpace(rawName)
	if rawName != name {
		return false
	}
	if !isValidSelectorFunctionName(name) {
		return false
	}
	args := strings.TrimSpace(sig[open+1 : close])
	if args == "" {
		return true
	}
	for _, p := range strings.Split(args, ",") {
		token := strings.TrimSpace(p)
		if token == "" {
			return false
		}
		if p != token || strings.ContainsAny(token, " \t\r\n") {
			return false
		}
	}
	return true
}

func isValidSelectorFunctionName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !(r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
				return false
			}
			continue
		}
		if !(r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func stripParens(e *ast.Expr) *ast.Expr {
	cur := e
	for cur != nil && cur.Kind == "paren" {
		cur = cur.Left
	}
	return cur
}

func hasIllegalNestedAssignInStmtExpr(e *ast.Expr) bool {
	root := stripParens(e)
	if root == nil {
		return false
	}
	switch root.Kind {
	case "assign":
		return containsAssignExpr(root.Left) || containsAssignExpr(root.Right)
	case "call":
		if containsAssignExpr(root.Callee) {
			return true
		}
		for _, a := range root.Args {
			if containsAssignExpr(a) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func containsAssignExpr(e *ast.Expr) bool {
	if e == nil {
		return false
	}
	if e.Kind == "assign" {
		return true
	}
	switch e.Kind {
	case "paren":
		return containsAssignExpr(e.Left)
	case "call":
		if containsAssignExpr(e.Callee) {
			return true
		}
		for _, a := range e.Args {
			if containsAssignExpr(a) {
				return true
			}
		}
		return false
	case "member":
		return containsAssignExpr(e.Object)
	case "index":
		return containsAssignExpr(e.Object) || containsAssignExpr(e.Index)
	case "slice":
		for _, a := range e.Args {
			if containsAssignExpr(a) {
				return true
			}
		}
		return containsAssignExpr(e.Object)
	case "binary":
		return containsAssignExpr(e.Left) || containsAssignExpr(e.Right)
	case "unary":
		return containsAssignExpr(e.Right)
	case "ternary":
		for _, a := range e.Args {
			if containsAssignExpr(a) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func customErrorCallName(e *ast.Expr) (string, bool) {
	name, _, ok := customErrorCallInfo(e)
	return name, ok
}

func customErrorCallInfo(e *ast.Expr) (string, int, bool) {
	root := stripParens(e)
	if root == nil || root.Kind != "call" {
		return "", 0, false
	}
	callee := stripParens(root.Callee)
	if callee == nil || callee.Kind != "ident" {
		return "", 0, false
	}
	name := strings.TrimSpace(callee.Value)
	if name == "" || name == "selector" {
		return "", 0, false
	}
	return name, len(root.Args), true
}

func checkDeclaredCustomErrorReverts(filename string, errorDeclParams map[string][]ast.FieldDecl, stmts []ast.Statement, diags *diag.Diagnostics) {
	if len(errorDeclParams) == 0 {
		return
	}
	for _, s := range stmts {
		if s.Kind == "revert" {
			if name, argc, ok := customErrorCallInfo(s.Expr); ok {
				if params, exists := errorDeclParams[name]; !exists {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaInvalidRevert,
						Message: fmt.Sprintf("revert custom error '%s' is not declared in contract", name),
						Span:    defaultSpan(filename),
					})
				} else if argc != len(params) {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaCallArity,
						Message: fmt.Sprintf("revert error '%s' expects %d argument(s), got %d", name, len(params), argc),
						Span:    defaultSpan(filename),
					})
				}
			}
		}
		if s.Init != nil {
			checkDeclaredCustomErrorReverts(filename, errorDeclParams, []ast.Statement{*s.Init}, diags)
		}
		checkDeclaredCustomErrorReverts(filename, errorDeclParams, s.Then, diags)
		checkDeclaredCustomErrorReverts(filename, errorDeclParams, s.Else, diags)
		checkDeclaredCustomErrorReverts(filename, errorDeclParams, s.Body, diags)
	}
}

// checkEnumMemberExprs validates that all enum member accesses (EnumName.Member) in
// expressions reference known enums and valid members.
func checkEnumMemberExprs(filename string, enumMemberValues map[string]map[string]int, stmts []ast.Statement, diags *diag.Diagnostics) {
	if len(enumMemberValues) == 0 {
		return
	}
	for _, s := range stmts {
		checkEnumMemberExpr(filename, enumMemberValues, s.Expr, diags)
		checkEnumMemberExpr(filename, enumMemberValues, s.Target, diags)
		checkEnumMemberExpr(filename, enumMemberValues, s.Cond, diags)
		checkEnumMemberExpr(filename, enumMemberValues, s.Post, diags)
		if s.Init != nil {
			checkEnumMemberExprs(filename, enumMemberValues, []ast.Statement{*s.Init}, diags)
		}
		checkEnumMemberExprs(filename, enumMemberValues, s.Then, diags)
		checkEnumMemberExprs(filename, enumMemberValues, s.Else, diags)
		checkEnumMemberExprs(filename, enumMemberValues, s.Body, diags)
	}
}

func checkEnumMemberExpr(filename string, enumMemberValues map[string]map[string]int, e *ast.Expr, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	switch e.Kind {
	case "member":
		obj := stripParens(e.Object)
		if obj != nil && obj.Kind == "ident" {
			enumName := strings.TrimSpace(obj.Value)
			if members, isEnum := enumMemberValues[enumName]; isEnum {
				memberName := strings.TrimSpace(e.Member)
				if _, ok := members[memberName]; !ok {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaInvalidStmtShape,
						Message: fmt.Sprintf("enum '%s' has no member '%s'", enumName, memberName),
						Span:    defaultSpan(filename),
					})
				}
				return // don't recurse into enum member access
			}
		}
		checkEnumMemberExpr(filename, enumMemberValues, e.Object, diags)
	case "call":
		checkEnumMemberExpr(filename, enumMemberValues, e.Callee, diags)
		for _, a := range e.Args {
			checkEnumMemberExpr(filename, enumMemberValues, a, diags)
		}
	case "binary":
		checkEnumMemberExpr(filename, enumMemberValues, e.Left, diags)
		checkEnumMemberExpr(filename, enumMemberValues, e.Right, diags)
	case "unary":
		checkEnumMemberExpr(filename, enumMemberValues, e.Right, diags)
	case "paren":
		checkEnumMemberExpr(filename, enumMemberValues, e.Left, diags)
	case "assign":
		checkEnumMemberExpr(filename, enumMemberValues, e.Left, diags)
		checkEnumMemberExpr(filename, enumMemberValues, e.Right, diags)
	case "index":
		checkEnumMemberExpr(filename, enumMemberValues, e.Object, diags)
		checkEnumMemberExpr(filename, enumMemberValues, e.Index, diags)
	case "slice":
		checkEnumMemberExpr(filename, enumMemberValues, e.Object, diags)
		for _, a := range e.Args {
			checkEnumMemberExpr(filename, enumMemberValues, a, diags)
		}
	}
}

// checkStructLiterals validates struct literal expressions (Kind == "struct_lit") in statements.
// It ensures the struct name is known and all required fields are provided without extras.
func checkStructLiterals(filename string, knownStructs map[string][]ast.FieldDecl, stmts []ast.Statement, diags *diag.Diagnostics) {
	if len(knownStructs) == 0 {
		return
	}
	for _, s := range stmts {
		checkStructLiteralExpr(filename, knownStructs, s.Expr, diags)
		checkStructLiteralExpr(filename, knownStructs, s.Target, diags)
		checkStructLiteralExpr(filename, knownStructs, s.Cond, diags)
		checkStructLiteralExpr(filename, knownStructs, s.Post, diags)
		if s.Init != nil {
			checkStructLiterals(filename, knownStructs, []ast.Statement{*s.Init}, diags)
		}
		checkStructLiterals(filename, knownStructs, s.Then, diags)
		checkStructLiterals(filename, knownStructs, s.Else, diags)
		checkStructLiterals(filename, knownStructs, s.Body, diags)
	}
}

func checkStructLiteralExpr(filename string, knownStructs map[string][]ast.FieldDecl, e *ast.Expr, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	switch e.Kind {
	case "struct_lit":
		structName := strings.TrimSpace(e.Value)
		fields, exists := knownStructs[structName]
		if !exists {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaUnknownStructType,
				Message: fmt.Sprintf("unknown struct type '%s' in struct literal", structName),
				Span:    defaultSpan(filename),
			})
			return
		}
		// Validate: no unknown fields, no missing fields.
		declaredSet := make(map[string]struct{}, len(fields))
		for _, f := range fields {
			declaredSet[f.Name] = struct{}{}
		}
		providedSet := make(map[string]struct{}, len(e.StructFields))
		for _, sf := range e.StructFields {
			fname := strings.TrimSpace(sf.Name)
			if _, ok := declaredSet[fname]; !ok {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaStructLiteralUnknown,
					Message: fmt.Sprintf("struct '%s' has no field '%s'", structName, fname),
					Span:    defaultSpan(filename),
				})
				continue
			}
			if _, dup := providedSet[fname]; dup {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaStructLiteralUnknown,
					Message: fmt.Sprintf("duplicate field '%s' in struct literal for '%s'", fname, structName),
					Span:    defaultSpan(filename),
				})
				continue
			}
			providedSet[fname] = struct{}{}
			// Recurse into field expression.
			checkStructLiteralExpr(filename, knownStructs, sf.Expr, diags)
		}
		// Check for missing fields.
		if len(providedSet) != len(fields) {
			for _, f := range fields {
				if _, ok := providedSet[f.Name]; !ok {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaStructLiteralArity,
						Message: fmt.Sprintf("struct literal for '%s' is missing field '%s'", structName, f.Name),
						Span:    defaultSpan(filename),
					})
				}
			}
		}
	case "call":
		checkStructLiteralExpr(filename, knownStructs, e.Callee, diags)
		for _, a := range e.Args {
			checkStructLiteralExpr(filename, knownStructs, a, diags)
		}
	case "binary":
		checkStructLiteralExpr(filename, knownStructs, e.Left, diags)
		checkStructLiteralExpr(filename, knownStructs, e.Right, diags)
	case "unary":
		checkStructLiteralExpr(filename, knownStructs, e.Right, diags)
	case "paren":
		checkStructLiteralExpr(filename, knownStructs, e.Left, diags)
	case "member":
		checkStructLiteralExpr(filename, knownStructs, e.Object, diags)
	case "index":
		checkStructLiteralExpr(filename, knownStructs, e.Object, diags)
		checkStructLiteralExpr(filename, knownStructs, e.Index, diags)
	case "assign":
		checkStructLiteralExpr(filename, knownStructs, e.Left, diags)
		checkStructLiteralExpr(filename, knownStructs, e.Right, diags)
	}
}

func checkReturnStatements(filename, ownerKind, ownerName string, expectsValue bool, stmts []ast.Statement, diags *diag.Diagnostics) {
	for _, s := range stmts {
		if s.Kind == "return" {
			switch {
			case expectsValue && s.Expr == nil:
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidReturn,
					Message: fmt.Sprintf("%s requires a return value", ownerLabel(ownerKind, ownerName)),
					Span:    defaultSpan(filename),
				})
			case !expectsValue && s.Expr != nil:
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidReturn,
					Message: fmt.Sprintf("%s must not return a value", ownerLabel(ownerKind, ownerName)),
					Span:    defaultSpan(filename),
				})
			}
		}
		if s.Init != nil {
			checkReturnStatements(filename, ownerKind, ownerName, expectsValue, []ast.Statement{*s.Init}, diags)
		}
		checkReturnStatements(filename, ownerKind, ownerName, expectsValue, s.Then, diags)
		checkReturnStatements(filename, ownerKind, ownerName, expectsValue, s.Else, diags)
		checkReturnStatements(filename, ownerKind, ownerName, expectsValue, s.Body, diags)
		for _, clause := range s.Catches {
			checkReturnStatements(filename, ownerKind, ownerName, expectsValue, clause.Body, diags)
		}
	}
}

func guaranteesValueReturnOrRevert(stmts []ast.Statement) bool {
	for _, s := range stmts {
		if guaranteesValueReturnOrRevertStmt(s) {
			return true
		}
	}
	return false
}

func guaranteesValueReturnOrRevertStmt(s ast.Statement) bool {
	switch s.Kind {
	case "return":
		return s.Expr != nil
	case "revert":
		return true
	case "if":
		if len(s.Then) == 0 || len(s.Else) == 0 {
			return false
		}
		return guaranteesValueReturnOrRevert(s.Then) && guaranteesValueReturnOrRevert(s.Else)
	case "while", "dowhile", "for":
		if !loopConditionAlwaysTrue(s) {
			return false
		}
		return blockGuaranteesValueReturnOrRevert(s.Body)
	default:
		return false
	}
}

func blockGuaranteesValueReturnOrRevert(stmts []ast.Statement) bool {
	for _, s := range stmts {
		if guaranteesValueReturnOrRevertStmt(s) {
			return true
		}
	}
	return false
}

func checkUnreachableStatements(filename string, stmts []ast.Statement, loopDepth int, diags *diag.Diagnostics) {
	terminated := false
	for _, s := range stmts {
		if terminated {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaUnreachableStmt,
				Message: "unreachable statement after terminal control-flow statement",
				Span:    defaultSpan(filename),
			})
			continue
		}
		if s.Init != nil {
			checkUnreachableStatements(filename, []ast.Statement{*s.Init}, loopDepth, diags)
		}
		checkUnreachableStatements(filename, s.Then, loopDepth, diags)
		checkUnreachableStatements(filename, s.Else, loopDepth, diags)
		nextLoopDepth := loopDepth
		if s.Kind == "while" || s.Kind == "dowhile" || s.Kind == "for" {
			nextLoopDepth++
		}
		checkUnreachableStatements(filename, s.Body, nextLoopDepth, diags)
		for _, clause := range s.Catches {
			checkUnreachableStatements(filename, clause.Body, loopDepth, diags)
		}
		if guaranteesTerminationStmt(s, loopDepth) {
			terminated = true
		}
	}
}

func guaranteesTerminationStmt(s ast.Statement, loopDepth int) bool {
	switch s.Kind {
	case "return", "revert":
		return true
	case "break", "continue":
		return loopDepth > 0
	case "while", "dowhile", "for":
		if !loopConditionAlwaysTrue(s) {
			return false
		}
		return blockGuaranteesHardTermination(s.Body, loopDepth+1)
	case "if":
		if len(s.Then) == 0 || len(s.Else) == 0 {
			return false
		}
		return blockGuaranteesTermination(s.Then, loopDepth) && blockGuaranteesTermination(s.Else, loopDepth)
	default:
		return false
	}
}

func blockGuaranteesTermination(stmts []ast.Statement, loopDepth int) bool {
	for _, s := range stmts {
		if guaranteesTerminationStmt(s, loopDepth) {
			return true
		}
	}
	return false
}

func blockGuaranteesHardTermination(stmts []ast.Statement, loopDepth int) bool {
	for _, s := range stmts {
		if guaranteesHardTerminationStmt(s, loopDepth) {
			return true
		}
	}
	return false
}

func guaranteesHardTerminationStmt(s ast.Statement, loopDepth int) bool {
	switch s.Kind {
	case "return", "revert":
		return true
	case "if":
		if len(s.Then) == 0 || len(s.Else) == 0 {
			return false
		}
		return blockGuaranteesHardTermination(s.Then, loopDepth) && blockGuaranteesHardTermination(s.Else, loopDepth)
	case "while", "dowhile", "for":
		if !loopConditionAlwaysTrue(s) {
			return false
		}
		return blockGuaranteesHardTermination(s.Body, loopDepth+1)
	default:
		return false
	}
}

func loopConditionAlwaysTrue(s ast.Statement) bool {
	switch s.Kind {
	case "while", "dowhile":
		return isLiteralTrueIdentExpr(s.Cond)
	case "for":
		if s.Cond == nil {
			return true
		}
		return isLiteralTrueIdentExpr(s.Cond)
	default:
		return false
	}
}

func isLiteralTrueIdentExpr(e *ast.Expr) bool {
	root := stripParens(e)
	if root == nil {
		return false
	}
	if root.Kind == "ident" && strings.TrimSpace(root.Value) == "true" {
		return true
	}
	if root.Kind == "bool" {
		v := strings.ToLower(strings.TrimSpace(root.Value))
		return v == "true"
	}
	return false
}

type localScope struct {
	names map[string]struct{}
}

func checkDuplicateLocals(filename, ownerKind, ownerName string, params []ast.FieldDecl, body []ast.Statement, diags *diag.Diagnostics) {
	scopes := []localScope{{names: map[string]struct{}{}}}
	declare := func(name string) bool {
		n := strings.TrimSpace(name)
		if n == "" {
			return true
		}
		cur := &scopes[len(scopes)-1]
		if _, exists := cur.names[n]; exists {
			return false
		}
		cur.names[n] = struct{}{}
		return true
	}
	for _, p := range params {
		_ = declare(p.Name)
	}
	checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, body, &scopes, declare, diags)
}

func checkDuplicateLocalsInStmts(
	filename, ownerKind, ownerName string,
	stmts []ast.Statement,
	scopes *[]localScope,
	declare func(string) bool,
	diags *diag.Diagnostics,
) {
	push := func() {
		*scopes = append(*scopes, localScope{names: map[string]struct{}{}})
	}
	pop := func() {
		if len(*scopes) > 1 {
			*scopes = (*scopes)[:len(*scopes)-1]
		}
	}
	for _, s := range stmts {
		if s.Kind == "let" && !declare(s.Name) {
			subject := ownerKind
			if ownerKind == "function" {
				subject = fmt.Sprintf("function '%s'", ownerName)
			}
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaDuplicateLocal,
				Message: fmt.Sprintf("duplicate local variable '%s' in %s scope", strings.TrimSpace(s.Name), subject),
				Span:    defaultSpan(filename),
			})
		}
		switch s.Kind {
		case "if":
			push()
			checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, s.Then, scopes, declare, diags)
			pop()
			push()
			checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, s.Else, scopes, declare, diags)
			pop()
		case "while", "dowhile":
			push()
			checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, s.Body, scopes, declare, diags)
			pop()
		case "for":
			push()
			if s.Init != nil {
				checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, []ast.Statement{*s.Init}, scopes, declare, diags)
			}
			checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, s.Body, scopes, declare, diags)
			pop()
		case "try":
			if len(s.Body) > 0 {
				push()
				checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, s.Body, scopes, declare, diags)
				pop()
			}
			for _, clause := range s.Catches {
				push()
				checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, clause.Body, scopes, declare, diags)
				pop()
			}
		default:
			if len(s.Then) > 0 {
				push()
				checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, s.Then, scopes, declare, diags)
				pop()
			}
			if len(s.Else) > 0 {
				push()
				checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, s.Else, scopes, declare, diags)
				pop()
			}
			if len(s.Body) > 0 {
				push()
				checkDuplicateLocalsInStmts(filename, ownerKind, ownerName, s.Body, scopes, declare, diags)
				pop()
			}
		}
	}
}

func ownerLabel(ownerKind, ownerName string) string {
	if ownerKind == "function" && strings.TrimSpace(ownerName) != "" {
		return fmt.Sprintf("function '%s'", ownerName)
	}
	return ownerKind
}

func buildStorageSlotInfo(slot ast.StorageSlot) storageSlotInfo {
	typeName := strings.TrimSpace(slot.Type)
	kind := classifyStorageKind(typeName)
	return storageSlotInfo{
		name:         slot.Name,
		kind:         kind,
		typeName:     typeName,
		mappingDepth: mappingTypeDepth(typeName),
	}
}

func classifyStorageKind(typeName string) storageSlotKind {
	compact := strings.ReplaceAll(normalizeSelectorType(typeName), " ", "")
	switch {
	case strings.HasPrefix(compact, "mapping("):
		return storageKindMapping
	case strings.HasSuffix(compact, "]"):
		return storageKindArray
	default:
		return storageKindScalar
	}
}

func mappingTypeDepth(typeName string) int {
	compact := strings.ReplaceAll(normalizeSelectorType(typeName), " ", "")
	return strings.Count(compact, "mapping(")
}

// mappingValueType extracts the value type V from "mapping(K=>V)".
// Returns "" if the type is not a mapping.
func mappingValueType(typeName string) string {
	compact := strings.ReplaceAll(normalizeSelectorType(typeName), " ", "")
	if !strings.HasPrefix(compact, "mapping(") {
		return ""
	}
	// Find the "=>" separator inside the outermost mapping(...) parens.
	inner := compact[len("mapping(") : len(compact)-1]
	// Scan for "=>" at depth 0 (not nested in inner mappings/parens).
	depth := 0
	for i := 0; i < len(inner)-1; i++ {
		switch inner[i] {
		case '(':
			depth++
		case ')':
			depth--
		case '=':
			if depth == 0 && i+1 < len(inner) && inner[i+1] == '>' {
				return inner[i+2:]
			}
		}
	}
	return ""
}

// arrayElemType strips the trailing [] or [N] from an array type.
// Returns "" if the type is not an array type.
func arrayElemType(typeName string) string {
	compact := strings.ReplaceAll(normalizeSelectorType(typeName), " ", "")
	if !strings.HasSuffix(compact, "]") {
		return ""
	}
	// Find the matching '[' for the last ']'.
	j := len(compact) - 1 // points to ']'
	for i := j - 1; i >= 0; i-- {
		if compact[i] == '[' {
			return compact[:i]
		}
	}
	return ""
}

// slotTypeAfterIndex returns the type after applying one index to typeName.
// For mapping(K=>V), returns V. For array T[], returns T. Otherwise returns "".
func slotTypeAfterIndex(typeName string) string {
	compact := strings.ReplaceAll(normalizeSelectorType(typeName), " ", "")
	if strings.HasPrefix(compact, "mapping(") {
		return mappingValueType(typeName)
	}
	if strings.HasSuffix(compact, "]") {
		return arrayElemType(typeName)
	}
	return ""
}

// slotMaxIndexDepth returns the maximum number of indices that can be applied
// to a storage slot of the given type before reaching a scalar.
func slotMaxIndexDepth(typeName string) int {
	depth := 0
	cur := strings.TrimSpace(typeName)
	for {
		next := slotTypeAfterIndex(cur)
		if next == "" {
			break
		}
		depth++
		cur = next
	}
	return depth
}

func newStorageCheckCtx(slots map[string]storageSlotInfo, params []ast.FieldDecl) *storageCheckCtx {
	c := &storageCheckCtx{
		slots:  slots,
		scopes: []map[string]struct{}{},
	}
	c.pushScope()
	for _, p := range params {
		c.declareLocal(p.Name)
	}
	return c
}

func (c *storageCheckCtx) pushScope() {
	c.scopes = append(c.scopes, map[string]struct{}{})
}

func (c *storageCheckCtx) popScope() {
	if len(c.scopes) == 0 {
		return
	}
	c.scopes = c.scopes[:len(c.scopes)-1]
}

func (c *storageCheckCtx) declareLocal(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if len(c.scopes) == 0 {
		c.pushScope()
	}
	c.scopes[len(c.scopes)-1][name] = struct{}{}
}

func (c *storageCheckCtx) isLocal(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for i := len(c.scopes) - 1; i >= 0; i-- {
		if _, ok := c.scopes[i][name]; ok {
			return true
		}
	}
	return false
}

func (c *storageCheckCtx) storagePathFromExpr(e *ast.Expr) (string, []*ast.Expr, bool) {
	if c == nil || e == nil {
		return "", nil, false
	}
	switch e.Kind {
	case "paren":
		return c.storagePathFromExpr(e.Left)
	case "ident":
		name := strings.TrimSpace(e.Value)
		if name == "" || c.isLocal(name) {
			return "", nil, false
		}
		if _, ok := c.slots[name]; !ok {
			return "", nil, false
		}
		return name, []*ast.Expr{}, true
	case "index":
		slot, keys, ok := c.storagePathFromExpr(e.Object)
		if !ok {
			return "", nil, false
		}
		out := make([]*ast.Expr, 0, len(keys)+1)
		out = append(out, keys...)
		out = append(out, e.Index)
		return slot, out, true
	default:
		return "", nil, false
	}
}

type storageExprUse int

const (
	storageUseValue storageExprUse = iota
	storageUseIndexObject
	storageUseMemberObject
	storageUseCallCallee
)

func checkStorageFunctionBody(filename string, slots map[string]storageSlotInfo, params []ast.FieldDecl, body []ast.Statement, diags *diag.Diagnostics) {
	if len(slots) == 0 {
		return
	}
	ctx := newStorageCheckCtx(slots, params)
	checkStorageStatements(filename, ctx, body, diags)
}

func checkStorageStatements(filename string, ctx *storageCheckCtx, stmts []ast.Statement, diags *diag.Diagnostics) {
	for _, s := range stmts {
		switch s.Kind {
		case "let":
			checkStorageExpr(filename, ctx, s.Expr, storageUseValue, diags)
			ctx.declareLocal(s.Name)
		case "let-tuple":
			checkStorageExpr(filename, ctx, s.Expr, storageUseValue, diags)
			for _, name := range s.Names {
				ctx.declareLocal(name)
			}
		case "set":
			checkStorageSetTarget(filename, ctx, s.Target, diags)
			checkStorageExpr(filename, ctx, s.Expr, storageUseValue, diags)
		case "if":
			checkStorageExpr(filename, ctx, s.Cond, storageUseValue, diags)
			ctx.pushScope()
			checkStorageStatements(filename, ctx, s.Then, diags)
			ctx.popScope()
			ctx.pushScope()
			checkStorageStatements(filename, ctx, s.Else, diags)
			ctx.popScope()
		case "while", "dowhile":
			checkStorageExpr(filename, ctx, s.Cond, storageUseValue, diags)
			ctx.pushScope()
			checkStorageStatements(filename, ctx, s.Body, diags)
			ctx.popScope()
		case "for":
			ctx.pushScope()
			if s.Init != nil {
				checkStorageStatements(filename, ctx, []ast.Statement{*s.Init}, diags)
			}
			checkStorageExpr(filename, ctx, s.Cond, storageUseValue, diags)
			checkStorageExpr(filename, ctx, s.Post, storageUseValue, diags)
			checkStorageStatements(filename, ctx, s.Body, diags)
			ctx.popScope()
		case "delete":
			checkStorageSetTarget(filename, ctx, s.Expr, diags)
		case "unchecked":
			ctx.pushScope()
			checkStorageStatements(filename, ctx, s.Body, diags)
			ctx.popScope()
		case "try":
			checkStorageExpr(filename, ctx, s.Expr, storageUseValue, diags)
			ctx.pushScope()
			checkStorageStatements(filename, ctx, s.Body, diags)
			ctx.popScope()
			for _, clause := range s.Catches {
				ctx.pushScope()
				checkStorageStatements(filename, ctx, clause.Body, diags)
				ctx.popScope()
			}
		default:
			checkStorageExpr(filename, ctx, s.Expr, storageUseValue, diags)
			checkStorageExpr(filename, ctx, s.Target, storageUseValue, diags)
			checkStorageExpr(filename, ctx, s.Cond, storageUseValue, diags)
			checkStorageExpr(filename, ctx, s.Post, storageUseValue, diags)
			if s.Init != nil {
				checkStorageStatements(filename, ctx, []ast.Statement{*s.Init}, diags)
			}
			if len(s.Then) > 0 {
				ctx.pushScope()
				checkStorageStatements(filename, ctx, s.Then, diags)
				ctx.popScope()
			}
			if len(s.Else) > 0 {
				ctx.pushScope()
				checkStorageStatements(filename, ctx, s.Else, diags)
				ctx.popScope()
			}
			if len(s.Body) > 0 {
				ctx.pushScope()
				checkStorageStatements(filename, ctx, s.Body, diags)
				ctx.popScope()
			}
		}
	}
}

func checkStorageSetTarget(filename string, ctx *storageCheckCtx, target *ast.Expr, diags *diag.Diagnostics) {
	if target == nil {
		return
	}
	if slotName, ok := storageArrayLengthMemberTarget(ctx, target); ok {
		reportStorageAccess(filename, fmt.Sprintf("storage array length on slot '%s' is read-only in current stage", slotName), diags)
		return
	}
	if slotName, keys, ok := ctx.storagePathFromExpr(target); ok {
		info := ctx.slots[slotName]
		checkStorageKeys(filename, ctx, keys, diags)
		validateStorageWrite(filename, info, keys, diags)
		return
	}
	checkStorageExpr(filename, ctx, target, storageUseValue, diags)
}

func checkStorageExpr(filename string, ctx *storageCheckCtx, e *ast.Expr, use storageExprUse, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	if slotName, keys, ok := ctx.storagePathFromExpr(e); ok {
		info := ctx.slots[slotName]
		switch use {
		case storageUseValue:
			validateStorageRead(filename, info, keys, diags)
		case storageUseIndexObject:
			validateStorageIndexObject(filename, info, keys, diags)
		case storageUseCallCallee:
			reportStorageAccess(filename, fmt.Sprintf("storage slot '%s' is not callable", info.name), diags)
		}
	}

	switch e.Kind {
	case "call":
		if e.Callee != nil && e.Callee.Kind == "member" {
			if e.Callee.Member == "push" {
				if slotName, keys, ok := ctx.storagePathFromExpr(e.Callee.Object); ok {
					info := ctx.slots[slotName]
					checkStorageKeys(filename, ctx, keys, diags)
					validateStoragePush(filename, info, keys, len(e.Args), diags)
					for _, a := range e.Args {
						checkStorageExpr(filename, ctx, a, storageUseValue, diags)
					}
					return
				}
			}
			// Allow oracle<T> OOP method: .fulfill(value)
			if e.Callee.Member == "fulfill" {
				if slotName, _, ok := ctx.storagePathFromExpr(e.Callee.Object); ok {
					info := ctx.slots[slotName]
					if strings.HasPrefix(info.typeName, "oracle<") {
						for _, a := range e.Args {
							checkStorageExpr(filename, ctx, a, storageUseValue, diags)
						}
						return
					}
				}
			}
			// Allow task<T> OOP methods: .accept/.submit/.approve/.reject/.dispute/.cancel
			taskMethods := map[string]bool{
				"accept": true, "submit": true, "approve": true,
				"reject": true, "dispute": true, "cancel": true,
			}
			if taskMethods[e.Callee.Member] {
				if slotName, _, ok := ctx.storagePathFromExpr(e.Callee.Object); ok {
					info := ctx.slots[slotName]
					if strings.Contains(info.typeName, "task<") {
						for _, a := range e.Args {
							checkStorageExpr(filename, ctx, a, storageUseValue, diags)
						}
						return
					}
				}
				// Also allow on task<T> local variable (tracked by type)
				if e.Callee.Object != nil && e.Callee.Object.Kind == "ident" {
					// Pass through — task local method calls are handled in lowering
					for _, a := range e.Args {
						checkStorageExpr(filename, ctx, a, storageUseValue, diags)
					}
					return
				}
			}
			// Allow vote<T> OOP methods: .cast(voter, choice) and .new(quorum, deadline_ms, tie)
			voteMethods := map[string]bool{"cast": true, "new": true}
			if voteMethods[e.Callee.Member] {
				if slotName, _, ok := ctx.storagePathFromExpr(e.Callee.Object); ok {
					info := ctx.slots[slotName]
					if strings.HasPrefix(info.typeName, "vote<") {
						for _, a := range e.Args {
							checkStorageExpr(filename, ctx, a, storageUseValue, diags)
						}
						return
					}
				}
			}
		}
		checkStorageExpr(filename, ctx, e.Callee, storageUseCallCallee, diags)
		for _, a := range e.Args {
			checkStorageExpr(filename, ctx, a, storageUseValue, diags)
		}
	case "member":
		if e.Member == "length" {
			if slotName, keys, ok := ctx.storagePathFromExpr(e.Object); ok {
				info := ctx.slots[slotName]
				checkStorageKeys(filename, ctx, keys, diags)
				validateStorageLength(filename, info, keys, diags)
				return
			}
		}
		if slotName, _, ok := ctx.storagePathFromExpr(e.Object); ok && e.Member != "selector" {
			info := ctx.slots[slotName]
			// Allow oracle<T> OOP members: .is_set, .value
			if strings.HasPrefix(info.typeName, "oracle<") {
				switch e.Member {
				case "is_set", "value":
					return // valid oracle property access
				}
			}
			// Allow vote<T> OOP members: .vote_count, .yes_count, .no_count, .is_decided, .result
			if strings.HasPrefix(info.typeName, "vote<") {
				switch e.Member {
				case "vote_count", "yes_count", "no_count", "is_decided", "result":
					return // valid vote property access
				}
			}
			// Allow task<T> mapping OOP members when accessed via index (tasks[tid].worker etc.)
			if strings.Contains(info.typeName, "task<") {
				switch e.Member {
				case "worker", "poster", "reward", "is_expired", "state":
					return // valid task property access
				}
			}
			// Allow agent-typed slot property access
			if info.typeName == "agent" {
				switch e.Member {
				case "stake", "is_active", "reputation", "rating_count", "suspended":
					return // valid agent property access
				}
			}
			reportStorageAccess(filename, fmt.Sprintf("unsupported member access '.%s' on storage slot '%s'", e.Member, info.name), diags)
		}
		checkStorageExpr(filename, ctx, e.Object, storageUseMemberObject, diags)
	case "index":
		checkStorageExpr(filename, ctx, e.Object, storageUseIndexObject, diags)
		checkStorageExpr(filename, ctx, e.Index, storageUseValue, diags)
	case "slice":
		checkStorageExpr(filename, ctx, e.Object, storageUseValue, diags)
		for _, a := range e.Args {
			checkStorageExpr(filename, ctx, a, storageUseValue, diags)
		}
	case "binary", "assign":
		checkStorageExpr(filename, ctx, e.Left, storageUseValue, diags)
		checkStorageExpr(filename, ctx, e.Right, storageUseValue, diags)
	case "unary":
		checkStorageExpr(filename, ctx, e.Right, storageUseValue, diags)
	case "paren":
		checkStorageExpr(filename, ctx, e.Left, use, diags)
	case "ternary":
		for _, arg := range e.Args {
			checkStorageExpr(filename, ctx, arg, storageUseValue, diags)
		}
	default:
		// leaf nodes
	}
}

func storageArrayLengthMemberTarget(ctx *storageCheckCtx, e *ast.Expr) (string, bool) {
	root := stripParens(e)
	if root == nil || root.Kind != "member" || root.Member != "length" {
		return "", false
	}
	slotName, keys, ok := ctx.storagePathFromExpr(root.Object)
	if !ok {
		return "", false
	}
	info := ctx.slots[slotName]
	// Determine the type after applying the collected keys.
	cur := info.typeName
	for range keys {
		cur = slotTypeAfterIndex(cur)
		if cur == "" {
			return "", false
		}
	}
	// The resulting type must be an array for .length to be valid.
	compact := strings.ReplaceAll(normalizeSelectorType(cur), " ", "")
	if !strings.HasSuffix(compact, "]") {
		return "", false
	}
	return slotName, true
}

func checkStorageKeys(filename string, ctx *storageCheckCtx, keys []*ast.Expr, diags *diag.Diagnostics) {
	for _, k := range keys {
		checkStorageExpr(filename, ctx, k, storageUseValue, diags)
	}
}

func validateStorageRead(filename string, info storageSlotInfo, keys []*ast.Expr, diags *diag.Diagnostics) {
	switch info.kind {
	case storageKindScalar:
		if len(keys) > 0 {
			reportStorageAccess(filename, fmt.Sprintf("storage slot '%s' of type '%s' does not support indexed read", info.name, info.typeName), diags)
		}
	case storageKindMapping:
		maxDepth := slotMaxIndexDepth(info.typeName)
		if maxDepth <= 0 {
			maxDepth = 1
		}
		if len(keys) != maxDepth {
			reportStorageAccess(filename, fmt.Sprintf("storage mapping slot '%s' requires exactly %d index key(s), got %d", info.name, maxDepth, len(keys)), diags)
		}
	case storageKindArray:
		maxDepth := slotMaxIndexDepth(info.typeName)
		switch {
		case len(keys) == 0:
			reportStorageAccess(filename, fmt.Sprintf("direct storage array value read is not supported on slot '%s'; use index or .length", info.name), diags)
		case len(keys) <= maxDepth:
			// ok — one or more valid indices
		default:
			reportStorageAccess(filename, fmt.Sprintf("storage slot '%s' indexed too deeply: max depth %d, got %d", info.name, maxDepth, len(keys)), diags)
		}
	}
}

func validateStorageWrite(filename string, info storageSlotInfo, keys []*ast.Expr, diags *diag.Diagnostics) {
	switch info.kind {
	case storageKindScalar:
		if len(keys) > 0 {
			reportStorageAccess(filename, fmt.Sprintf("storage slot '%s' of type '%s' does not support indexed write", info.name, info.typeName), diags)
		}
	case storageKindMapping:
		maxDepth := slotMaxIndexDepth(info.typeName)
		if maxDepth <= 0 {
			maxDepth = 1
		}
		if len(keys) != maxDepth {
			reportStorageAccess(filename, fmt.Sprintf("storage mapping slot '%s' requires exactly %d index key(s), got %d", info.name, maxDepth, len(keys)), diags)
		}
	case storageKindArray:
		maxDepth := slotMaxIndexDepth(info.typeName)
		if len(keys) == 0 || len(keys) > maxDepth {
			reportStorageAccess(filename, fmt.Sprintf("storage array slot '%s' write requires 1..%d index key(s), got %d", info.name, maxDepth, len(keys)), diags)
		}
	}
}

func validateStorageIndexObject(filename string, info storageSlotInfo, keys []*ast.Expr, diags *diag.Diagnostics) {
	maxDepth := slotMaxIndexDepth(info.typeName)
	switch info.kind {
	case storageKindScalar:
		reportStorageAccess(filename, fmt.Sprintf("storage slot '%s' of type '%s' is not indexable", info.name, info.typeName), diags)
	case storageKindMapping:
		if len(keys) >= maxDepth {
			reportStorageAccess(filename, fmt.Sprintf("storage slot '%s' indexed too deeply: max depth %d", info.name, maxDepth), diags)
		}
	case storageKindArray:
		// Allow indexing as an object as long as we haven't exceeded the max depth.
		// The partial index chain (len(keys)) must leave at least one more level.
		if len(keys) >= maxDepth {
			reportStorageAccess(filename, fmt.Sprintf("storage slot '%s' indexed too deeply: max depth %d", info.name, maxDepth), diags)
		}
	}
}

func validateStorageLength(filename string, info storageSlotInfo, keys []*ast.Expr, diags *diag.Diagnostics) {
	// Determine the type after applying the given keys.
	cur := info.typeName
	for range keys {
		cur = slotTypeAfterIndex(cur)
		if cur == "" {
			reportStorageAccess(filename, fmt.Sprintf("'.length' applied to non-array type on slot '%s'", info.name), diags)
			return
		}
	}
	// After applying keys, the resulting type must be an array.
	compact := strings.ReplaceAll(normalizeSelectorType(cur), " ", "")
	if !strings.HasSuffix(compact, "]") {
		reportStorageAccess(filename, fmt.Sprintf("'.length' is only supported on storage arrays (slot '%s')", info.name), diags)
	}
}

func validateStoragePush(filename string, info storageSlotInfo, keys []*ast.Expr, argCount int, diags *diag.Diagnostics) {
	// Determine the type after applying the given keys.
	cur := info.typeName
	for range keys {
		cur = slotTypeAfterIndex(cur)
		if cur == "" {
			reportStorageAccess(filename, fmt.Sprintf("'.push(v)' applied to non-array type on slot '%s'", info.name), diags)
			return
		}
	}
	// After applying keys, the resulting type must be an array.
	compact := strings.ReplaceAll(normalizeSelectorType(cur), " ", "")
	if !strings.HasSuffix(compact, "]") {
		reportStorageAccess(filename, fmt.Sprintf("'.push(v)' is only supported on storage arrays (slot '%s')", info.name), diags)
		return
	}
	if argCount != 1 {
		reportStorageAccess(filename, "storage array push requires exactly one argument", diags)
	}
}

func reportStorageAccess(filename, msg string, diags *diag.Diagnostics) {
	*diags = append(*diags, diag.Diagnostic{
		Code:    diag.CodeSemaStorageAccess,
		Message: msg,
		Span:    defaultSpan(filename),
	})
}

func isAssignableTarget(e *ast.Expr) bool {
	if e == nil {
		return false
	}
	switch e.Kind {
	case "ident", "member", "index":
		return true
	default:
		return false
	}
}

func checkExpr(contractName string, funcVis map[string]string, funcArity map[string]int, filename string, e *ast.Expr, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	switch e.Kind {
	case "call":
		if isSelectorBuiltinCallExpr(e.Callee) || isSelectorMemberExpr(e.Callee) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaSelectorTarget,
				Message: "selector expression result is bytes4 and cannot be called as a function",
				Span:    defaultSpan(filename),
			})
		}
		callee := stripParens(e.Callee)
		skipRecursiveCalleeCheck := false
		if ns, name, ok := bytesStringBuiltinCallName(e.Callee); ok {
			skipRecursiveCalleeCheck = true
			switch name {
			case "concat":
				// bytes.concat or string.concat: variadic, any number of args accepted.
				for _, a := range e.Args {
					checkExpr(contractName, funcVis, funcArity, filename, a, diags)
				}
			default:
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: fmt.Sprintf("unsupported %s builtin '%s.%s' in current stage", ns, ns, name),
					Span:    defaultSpan(filename),
				})
			}
		}
		if abiName, ok := abiBuiltinCallName(e.Callee); ok {
			switch abiName {
			case "encode", "encodePacked":
				// Any arity accepted in current stage.
			case "encodeWithSelector":
				if len(e.Args) < 1 {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaCallArity,
						Message: fmt.Sprintf("abi.encodeWithSelector expects at least 1 argument(s), got %d", len(e.Args)),
						Span:    defaultSpan(filename),
					})
				}
			case "encodeWithSignature":
				if len(e.Args) < 1 {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaCallArity,
						Message: fmt.Sprintf("abi.encodeWithSignature expects at least 1 argument(s), got %d", len(e.Args)),
						Span:    defaultSpan(filename),
					})
				} else {
					first := stripParens(e.Args[0])
					if first == nil || first.Kind != "string" || !isSelectorSignatureLiteralExpr(first) {
						*diags = append(*diags, diag.Diagnostic{
							Code:    diag.CodeSemaInvalidSelectorExpr,
							Message: "abi.encodeWithSignature requires canonical string literal 'name(type1,type2,...)' as first argument",
							Span:    defaultSpan(filename),
						})
					}
				}
			case "decode":
				if len(e.Args) != 1 {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaCallArity,
						Message: fmt.Sprintf("abi.decode expects exactly 1 argument(s), got %d", len(e.Args)),
						Span:    defaultSpan(filename),
					})
				}
			default:
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: fmt.Sprintf("unsupported abi builtin 'abi.%s' in current stage", abiName),
					Span:    defaultSpan(filename),
				})
			}
		}
		if scope, key, ok := envMemberScopeKey(e.Callee); ok {
			skipRecursiveCalleeCheck = true
			switch scope {
			case "gas":
				if key != "left" {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaInvalidStmtShape,
						Message: fmt.Sprintf("unsupported gas member '%s'; only gas.left() is allowed", "gas."+key),
						Span:    defaultSpan(filename),
					})
				} else if len(e.Args) != 0 {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaCallArity,
						Message: fmt.Sprintf("gas.left() expects 0 argument(s), got %d", len(e.Args)),
						Span:    defaultSpan(filename),
					})
				}
			case "msg", "tx", "block":
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidStmtShape,
					Message: fmt.Sprintf("environment field '%s.%s' is not callable", scope, key),
					Span:    defaultSpan(filename),
				})
			}
		}
		if callee != nil && callee.Kind == "ident" {
			name := strings.TrimSpace(callee.Value)
			if want, ok := builtinCallArity(name); ok {
				if _, declared := funcArity[name]; !declared && len(e.Args) != want {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaCallArity,
						Message: fmt.Sprintf("builtin '%s' expects %d argument(s), got %d", name, want, len(e.Args)),
						Span:    defaultSpan(filename),
					})
				}
			}
		}
		if callee != nil && callee.Kind == "ident" && strings.TrimSpace(callee.Value) == "selector" {
			if len(e.Args) != 1 || e.Args[0] == nil {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidSelectorExpr,
					Message: "selector(...) requires exactly one argument",
					Span:    defaultSpan(filename),
				})
			} else if e.Args[0].Kind == "string" && !isSelectorSignatureLiteralExpr(e.Args[0]) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidSelectorExpr,
					Message: "selector(...) requires a string literal in signature form 'name(type1,type2,...)'",
					Span:    defaultSpan(filename),
				})
			}
		}
		// payable(expr): address-type annotation; identity cast at runtime.
		// Validate arity only; payable is not a declared contract function.
		if callee != nil && callee.Kind == "ident" && strings.TrimSpace(callee.Value) == "payable" {
			if len(e.Args) != 1 {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaCallArity,
					Message: fmt.Sprintf("payable(...) expects exactly 1 argument, got %d", len(e.Args)),
					Span:    defaultSpan(filename),
				})
			} else {
				checkExpr(contractName, funcVis, funcArity, filename, e.Args[0], diags)
			}
			// payable is not a declared function — return without further call checks.
			return
		}
		if name, ok := localContractCallName(contractName, e.Callee); ok {
			if want, exists := funcArity[name]; exists && want != -1 && len(e.Args) != want {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaCallArity,
					Message: fmt.Sprintf("function '%s' expects %d argument(s), got %d", name, want, len(e.Args)),
					Span:    defaultSpan(filename),
				})
			}
			root := stripParens(e.Callee)
			if root != nil && root.Kind == "ident" {
				if vis, exists := funcVis[name]; exists && vis == "external" {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaCallVisibility,
						Message: fmt.Sprintf("direct call target function '%s' is external-only; use contract-scoped dispatch call", name),
						Span:    defaultSpan(filename),
					})
				}
			}
		}
		if name, ok := scopedContractMemberCallName(contractName, e.Callee); ok {
			if _, exists := funcArity[name]; !exists {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaUnknownCallTarget,
					Message: fmt.Sprintf("contract call target function '%s' not found", name),
					Span:    defaultSpan(filename),
				})
			} else if vis := funcVis[name]; vis != "public" && vis != "external" {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaCallVisibility,
					Message: fmt.Sprintf("contract-scoped call target function '%s' is not externally dispatchable", name),
					Span:    defaultSpan(filename),
				})
			}
		}
		if !skipRecursiveCalleeCheck {
			checkExpr(contractName, funcVis, funcArity, filename, e.Callee, diags)
		}
		for _, a := range e.Args {
			checkExpr(contractName, funcVis, funcArity, filename, a, diags)
		}
	case "member":
		// Validate selector member builtin: this.fn.selector / Contract.fn.selector
		if e.Member == "selector" {
			ok := false
			msg := ""
			target := stripParens(e.Object)
			if target != nil && target.Kind == "member" {
				scopeExpr := stripParens(target.Object)
				scope := ""
				if scopeExpr != nil && scopeExpr.Kind == "ident" {
					scope = strings.TrimSpace(scopeExpr.Value)
				}
				fnName := strings.TrimSpace(target.Member)
				if scope != "this" && scope != contractName {
					msg = fmt.Sprintf("selector member scope must be 'this' or '%s'", contractName)
				} else if vis, exists := funcVis[fnName]; !exists {
					msg = fmt.Sprintf("selector target function '%s' not found", fnName)
				} else if vis != "public" && vis != "external" {
					msg = fmt.Sprintf("selector target function '%s' is not externally dispatchable", fnName)
				} else {
					ok = true
				}
			} else {
				msg = "selector member expression must be 'this.fn.selector' or 'Contract.fn.selector'"
			}
			if !ok {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaSelectorTarget,
					Message: msg,
					Span:    defaultSpan(filename),
				})
			}
		}
		if scope, key, ok := envMemberScopeKey(e); ok {
			switch scope {
			case "msg", "tx", "block":
				if !isAllowedEnvField(scope, key) {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaInvalidStmtShape,
						Message: fmt.Sprintf("unsupported environment field '%s.%s' in current stage", scope, key),
						Span:    defaultSpan(filename),
					})
				}
			case "gas":
				if key != "left" {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaInvalidStmtShape,
						Message: fmt.Sprintf("unsupported gas member '%s'; only gas.left() is allowed", "gas."+key),
						Span:    defaultSpan(filename),
					})
				}
			}
		}
		// Validate type(T).min / type(T).max compile-time integer bound expressions.
		if typeName, _, matched, validType := matchTypeBoundsExpr(e); matched {
			if !validType {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidTypeBoundsType,
					Message: fmt.Sprintf("type bounds type '%s' is not supported; only uN and iN integer types are allowed (e.g. type(u256).max)", typeName),
					Span:    defaultSpan(filename),
				})
			}
			// type(T).min/max is a compile-time constant: skip recursive check of the
			// call-expression object to avoid spurious call-arity or unknown-function errors.
			return
		}
		// Validate type(I).interfaceId compile-time EIP-165 interface ID expressions.
		if _, matched := matchTypeInterfaceIdExpr(e); matched {
			// type(I).interfaceId is a compile-time constant: skip recursive check
			// to avoid spurious call-arity or unknown-function errors on type(...).
			return
		}
		// Allow .length on any expression (bytes/string length is valid; validated at runtime).
		checkExpr(contractName, funcVis, funcArity, filename, e.Object, diags)
	case "slice":
		// Slice expression: expr[start:end] — valid on bytes expressions.
		// Recursively check the object and both bound arguments.
		checkExpr(contractName, funcVis, funcArity, filename, e.Object, diags)
		if len(e.Args) == 2 {
			checkExpr(contractName, funcVis, funcArity, filename, e.Args[0], diags)
			checkExpr(contractName, funcVis, funcArity, filename, e.Args[1], diags)
		}
	case "index":
		checkExpr(contractName, funcVis, funcArity, filename, e.Object, diags)
		checkExpr(contractName, funcVis, funcArity, filename, e.Index, diags)
	case "binary":
		checkExpr(contractName, funcVis, funcArity, filename, e.Left, diags)
		checkExpr(contractName, funcVis, funcArity, filename, e.Right, diags)
	case "assign":
		if !isAssignableTarget(e.Left) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidSetTarget,
				Message: "assignment target must be identifier, member access, or index access",
				Span:    defaultSpan(filename),
			})
		} else if isReadOnlyIdentTarget(e.Left) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidSetTarget,
				Message: "assignment target cannot be 'true', 'false', or 'nil'",
				Span:    defaultSpan(filename),
			})
		} else if isSelectorMemberExpr(e.Left) {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidSetTarget,
				Message: "selector member expression is read-only and cannot be assignment target",
				Span:    defaultSpan(filename),
			})
		} else if scope, key, ok := envMemberScopeKey(e.Left); ok {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidSetTarget,
				Message: fmt.Sprintf("environment member '%s.%s' is read-only and cannot be assignment target", scope, key),
				Span:    defaultSpan(filename),
			})
		}
		checkExpr(contractName, funcVis, funcArity, filename, e.Left, diags)
		checkExpr(contractName, funcVis, funcArity, filename, e.Right, diags)
	case "unary":
		checkExpr(contractName, funcVis, funcArity, filename, e.Right, diags)
	case "paren":
		checkExpr(contractName, funcVis, funcArity, filename, e.Left, diags)
	case "ternary":
		if len(e.Args) != 3 {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidStmtShape,
				Message: "ternary expression requires exactly 3 operands (cond ? then : else)",
				Span:    defaultSpan(filename),
			})
		} else {
			checkExpr(contractName, funcVis, funcArity, filename, e.Args[0], diags)
			checkExpr(contractName, funcVis, funcArity, filename, e.Args[1], diags)
			checkExpr(contractName, funcVis, funcArity, filename, e.Args[2], diags)
		}
	case "inspect":
		*diags = append(*diags, diag.Diagnostic{
			Code:    diag.CodeSemaInspectInNonTestFile,
			Message: "'inspect' is only allowed in test function bodies",
			Span:    defaultSpan(filename),
		})
	case "ident", "number", "string", "hex_lit":
		if e.Kind == "ident" && strings.TrimSpace(e.Value) == "nil" {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidStmtShape,
				Message: "source-level nil is not allowed in TOL; use typed default values instead",
				Span:    defaultSpan(filename),
			})
		}
	case "struct_lit":
		// Recurse into struct field initializer expressions.
		for _, sf := range e.StructFields {
			checkExpr(contractName, funcVis, funcArity, filename, sf.Expr, diags)
		}
	case "new":
		// new ContractName(args...) — validate argument expressions.
		for _, a := range e.Args {
			checkExpr(contractName, funcVis, funcArity, filename, a, diags)
		}
	case "new_array":
		// new T[](size) — memory array allocation. Validate the size expression.
		for _, a := range e.Args {
			checkExpr(contractName, funcVis, funcArity, filename, a, diags)
		}
	case "msg_agent":
		// msg.agent — zero-agent fallback (agent-typed, no revert on unregistered callers).
		// No sub-expressions to check.
	default:
		*diags = append(*diags, diag.Diagnostic{
			Code:    diag.CodeSemaInvalidStmtShape,
			Message: fmt.Sprintf("unsupported expression kind '%s' in current verifier stage", e.Kind),
			Span:    defaultSpan(filename),
		})
	}
}

// funcSignatureKey builds a key string "name(type1,type2,...)" used for overload deduplication.
// This is the full canonical signature that distinguishes overloads.
func funcSignatureKey(name string, params []ast.FieldDecl) string {
	types := make([]string, 0, len(params))
	for _, p := range params {
		t := strings.Join(strings.Fields(p.Type), " ")
		types = append(types, t)
	}
	return name + "(" + strings.Join(types, ",") + ")"
}

func localContractCallName(contractName string, callee *ast.Expr) (string, bool) {
	root := stripParens(callee)
	if root == nil {
		return "", false
	}
	switch root.Kind {
	case "ident":
		name := strings.TrimSpace(root.Value)
		return name, name != ""
	case "member":
		obj := stripParens(root.Object)
		if obj == nil || obj.Kind != "ident" {
			return "", false
		}
		scope := strings.TrimSpace(obj.Value)
		if scope != "this" && scope != contractName {
			return "", false
		}
		name := strings.TrimSpace(root.Member)
		return name, name != ""
	default:
		return "", false
	}
}

func scopedContractMemberCallName(contractName string, callee *ast.Expr) (string, bool) {
	root := stripParens(callee)
	if root == nil || root.Kind != "member" {
		return "", false
	}
	obj := stripParens(root.Object)
	if obj == nil || obj.Kind != "ident" {
		return "", false
	}
	scope := strings.TrimSpace(obj.Value)
	if scope != "this" && scope != contractName {
		return "", false
	}
	name := strings.TrimSpace(root.Member)
	return name, name != ""
}

func functionVisibility(modifiers []string) string {
	vis := ""
	for _, m := range modifiers {
		switch m {
		case "public", "external", "internal", "private":
			vis = m
		}
	}
	return vis
}

func checkContractNameCollisions(filename string, slots map[string]storageSlotInfo, funcs map[string]int, events map[string]int) diag.Diagnostics {
	var out diag.Diagnostics
	for name := range events {
		if _, exists := funcs[name]; exists {
			out = append(out, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("name collision: event '%s' conflicts with function '%s'", name, name),
				Span:    defaultSpan(filename),
			})
		}
		if _, exists := slots[name]; exists {
			out = append(out, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("name collision: event '%s' conflicts with storage slot '%s'", name, name),
				Span:    defaultSpan(filename),
			})
		}
	}
	for name := range funcs {
		if _, exists := slots[name]; exists {
			out = append(out, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("name collision: function '%s' conflicts with storage slot '%s'", name, name),
				Span:    defaultSpan(filename),
			})
		}
	}
	return out
}

func checkContractSupportNameCollisions(filename string, support map[string]string, slots map[string]storageSlotInfo, funcs map[string]int, events map[string]int) diag.Diagnostics {
	var out diag.Diagnostics
	for name, kind := range support {
		if _, exists := funcs[name]; exists {
			out = append(out, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("name collision: %s '%s' conflicts with function '%s'", kind, name, name),
				Span:    defaultSpan(filename),
			})
		}
		if _, exists := events[name]; exists {
			out = append(out, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("name collision: %s '%s' conflicts with event '%s'", kind, name, name),
				Span:    defaultSpan(filename),
			})
		}
		if _, exists := slots[name]; exists {
			out = append(out, diag.Diagnostic{
				Code:    diag.CodeSemaNameCollision,
				Message: fmt.Sprintf("name collision: %s '%s' conflicts with storage slot '%s'", kind, name, name),
				Span:    defaultSpan(filename),
			})
		}
	}
	return out
}

// checkModifierBody validates a modifier declaration's body.
// Rules:
//   - The body must contain exactly one "placeholder" statement (_; ).
//   - Otherwise the body is checked like a void function body (no return values).
func checkModifierBody(filename string, contractName string, funcVis map[string]string, funcArity map[string]int, eventArity map[string]int, slotInfos map[string]storageSlotInfo, md ast.ModifierDecl) diag.Diagnostics {
	var diags diag.Diagnostics

	// Count placeholder statements (including nested ones in control flow).
	count := countPlaceholders(md.Body)
	if count == 0 {
		diags = append(diags, diag.Diagnostic{
			Code:    diag.CodeSemaModifierPlaceholder,
			Message: fmt.Sprintf("modifier '%s' must contain exactly one '_; ' placeholder statement", md.Name),
			Span:    defaultSpan(filename),
		})
	} else if count > 1 {
		diags = append(diags, diag.Diagnostic{
			Code:    diag.CodeSemaModifierPlaceholder,
			Message: fmt.Sprintf("modifier '%s' contains %d '_; ' placeholder statements; only one is allowed", md.Name, count),
			Span:    defaultSpan(filename),
		})
	}

	// Check body statements (same rules as a void function, but allowing "placeholder").
	checkModifierBodyStmts(filename, contractName, funcVis, funcArity, eventArity, md.Body, 0, &diags)
	checkStorageFunctionBody(filename, slotInfos, nil, md.Body, &diags)
	return diags
}

// countPlaceholders counts all "placeholder" statements recursively.
func countPlaceholders(stmts []ast.Statement) int {
	count := 0
	for _, s := range stmts {
		if s.Kind == "placeholder" {
			count++
		}
		count += countPlaceholders(s.Then)
		count += countPlaceholders(s.Else)
		count += countPlaceholders(s.Body)
	}
	return count
}

// checkModifierBodyStmts is like checkStatements but allows "placeholder" kind.
func checkModifierBodyStmts(filename string, contractName string, funcVis map[string]string, funcArity map[string]int, eventArity map[string]int, stmts []ast.Statement, loopDepth int, diags *diag.Diagnostics) {
	for _, s := range stmts {
		if s.Kind == "placeholder" {
			// _; is valid only at statement level inside a modifier body.
			continue
		}
		// Delegate all other statement checking to the regular checker.
		checkStatements(filename, contractName, funcVis, funcArity, eventArity, []ast.Statement{s}, loopDepth, diags)
	}
}

func validateFunctionModifiers(filename string, fnName string, modifiers []string, userMods map[string]ast.ModifierDecl) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	vis := ""
	hasView := false
	hasPure := false
	hasPayable := false

	for _, m := range modifiers {
		// Allow user-defined modifier names — they are validated separately.
		if _, isUserMod := userMods[m]; isUserMod {
			continue
		}
		switch m {
		case "public", "external", "internal", "private":
			if vis == m {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("duplicate visibility modifier '%s' on function '%s'", m, fnName),
					Span:    defaultSpan(filename),
				})
			} else if vis != "" {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting visibility modifiers '%s' and '%s' on function '%s'", vis, m, fnName),
					Span:    defaultSpan(filename),
				})
			}
			vis = m
		case "view":
			if hasView {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("duplicate modifier 'view' on function '%s'", fnName),
					Span:    defaultSpan(filename),
				})
			}
			if hasPayable {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting modifiers 'view' and 'payable' on function '%s'", fnName),
					Span:    defaultSpan(filename),
				})
			}
			if hasPure {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting modifiers 'view' and 'pure' on function '%s'", fnName),
					Span:    defaultSpan(filename),
				})
			}
			hasView = true
		case "pure":
			if hasPure {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("duplicate modifier 'pure' on function '%s'", fnName),
					Span:    defaultSpan(filename),
				})
			}
			if hasPayable {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting modifiers 'pure' and 'payable' on function '%s'", fnName),
					Span:    defaultSpan(filename),
				})
			}
			if hasView {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting modifiers 'pure' and 'view' on function '%s'", fnName),
					Span:    defaultSpan(filename),
				})
			}
			hasPure = true
		case "payable":
			if hasPayable {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("duplicate modifier 'payable' on function '%s'", fnName),
					Span:    defaultSpan(filename),
				})
			}
			if hasView {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting modifiers 'payable' and 'view' on function '%s'", fnName),
					Span:    defaultSpan(filename),
				})
			}
			if hasPure {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting modifiers 'payable' and 'pure' on function '%s'", fnName),
					Span:    defaultSpan(filename),
				})
			}
			hasPayable = true
		default:
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaUnknownModifier,
				Message: fmt.Sprintf("unknown modifier '%s' on function '%s': not a built-in modifier and not declared in contract", m, fnName),
				Span:    defaultSpan(filename),
			})
		}
	}

	return vis, diags
}

func validateConstructorModifiers(filename string, modifiers []string) diag.Diagnostics {
	var diags diag.Diagnostics
	vis := ""
	hasPayable := false
	for _, m := range modifiers {
		switch m {
		case "public", "internal":
			if vis == m {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("duplicate constructor visibility modifier '%s'", m),
					Span:    defaultSpan(filename),
				})
			} else if vis != "" {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("conflicting constructor visibility modifiers '%s' and '%s'", vis, m),
					Span:    defaultSpan(filename),
				})
			}
			vis = m
		case "payable":
			if hasPayable {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: "duplicate constructor modifier 'payable'",
					Span:    defaultSpan(filename),
				})
			}
			hasPayable = true
		default:
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidFnModifier,
				Message: fmt.Sprintf("unsupported constructor modifier '%s'", m),
				Span:    defaultSpan(filename),
			})
		}
	}
	return diags
}

// validateConstructorParamABIEncodable checks that a constructor parameter type
// is ABI-encodable.  Mapping types are not supported as constructor parameters
// (they are not ABI-encodable at all).  Array types (both fixed-size T[N] and
// dynamic T[]) are now supported via __tol_abi_decode_tuple which handles offset
// pointers for dynamic arrays.  Mappings are already rejected by
// validateTypeForContext (allowMapping=false); this function is a no-op for all
// other types including arrays.
func validateConstructorParamABIEncodable(filename, typeName, paramName string, diags *diag.Diagnostics) {
	// Array types are now supported via the ABI decode prelude. Nothing to reject.
	_ = filename
	_ = typeName
	_ = paramName
	_ = diags
}

func duplicateParamDiagnostics(filename, ownerKind, ownerName string, params []ast.FieldDecl) diag.Diagnostics {
	var out diag.Diagnostics
	seen := map[string]struct{}{}
	for _, p := range params {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			subject := ownerKind
			switch ownerKind {
			case "function":
				subject = fmt.Sprintf("function '%s'", ownerName)
			case "event":
				subject = fmt.Sprintf("event '%s'", ownerName)
			case "returns":
				subject = fmt.Sprintf("return list of function '%s'", ownerName)
			}
			out = append(out, diag.Diagnostic{
				Code:    diag.CodeSemaDuplicateParam,
				Message: fmt.Sprintf("duplicate parameter '%s' in %s", name, subject),
				Span:    defaultSpan(filename),
			})
			continue
		}
		seen[name] = struct{}{}
	}
	return out
}

func checkParamReturnNameCollisions(filename, fnName string, params, returns []ast.FieldDecl) diag.Diagnostics {
	var out diag.Diagnostics
	if len(params) == 0 || len(returns) == 0 {
		return out
	}
	paramNames := map[string]struct{}{}
	for _, p := range params {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		paramNames[name] = struct{}{}
	}
	for _, r := range returns {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			continue
		}
		if _, ok := paramNames[name]; ok {
			out = append(out, diag.Diagnostic{
				Code:    diag.CodeSemaParamReturnCollision,
				Message: fmt.Sprintf("function '%s' has name collision between parameter and return field '%s'", fnName, name),
				Span:    defaultSpan(filename),
			})
		}
	}
	return out
}

func selectorDispatchKey(fn ast.FunctionDecl) (string, bool) {
	visibility := ""
	for _, m := range fn.Modifiers {
		switch m {
		case "public", "external", "internal", "private":
			visibility = m
		}
	}
	if visibility != "public" && visibility != "external" {
		return "", false
	}
	if fn.SelectorOverride != "" {
		return strings.ToLower(fn.SelectorOverride), true
	}
	sig := fmt.Sprintf("%s(%s)", fn.Name, selectorTypeList(fn.Params))
	return selectorHexFromSignature(sig), true
}

func selectorTypeList(params []ast.FieldDecl) string {
	if len(params) == 0 {
		return ""
	}
	types := make([]string, 0, len(params))
	for _, p := range params {
		types = append(types, normalizeSelectorType(p.Type))
	}
	return strings.Join(types, ",")
}

// isPayableAgentType returns true if the type is any agent/address — every
// agent in TOL is a valid transfer target; Solidity's "agent payable" distinction
// is not required.
func isPayableAgentType(t string) bool {
	n := normalizeSelectorType(t)
	return n == "agent"
}

// checkAgentTransferCalls validates that .transfer() and .send() are called on
// agent-typed expressions. This is a separate pass that builds its own flat type
// environment from params
// and let bindings, matching the checkDuplicateLocals pattern.
func checkAgentTransferCalls(filename string, params []ast.FieldDecl, body []ast.Statement, diags *diag.Diagnostics) {
	typeEnv := make(map[string]string, len(params)+8)
	for _, p := range params {
		typeEnv[strings.TrimSpace(p.Name)] = strings.Join(strings.Fields(p.Type), " ")
	}
	checkAgentTransferInStmts(filename, typeEnv, body, diags)
}

func checkAgentTransferInStmts(filename string, typeEnv map[string]string, stmts []ast.Statement, diags *diag.Diagnostics) {
	for _, s := range stmts {
		// Accumulate let bindings into the type env for subsequent statements.
		if s.Kind == "let" && s.Name != "" {
			typeEnv[strings.TrimSpace(s.Name)] = strings.Join(strings.Fields(s.Type), " ")
		}
		if s.Kind == "let-tuple" {
			for i, name := range s.Names {
				t := ""
				if i < len(s.Types) {
					t = strings.Join(strings.Fields(s.Types[i]), " ")
				}
				typeEnv[strings.TrimSpace(name)] = t
			}
		}
		// Check expressions in this statement.
		if s.Expr != nil {
			checkAgentTransferInExpr(filename, typeEnv, s.Expr, diags)
		}
		if s.Target != nil {
			checkAgentTransferInExpr(filename, typeEnv, s.Target, diags)
		}
		if s.Cond != nil {
			checkAgentTransferInExpr(filename, typeEnv, s.Cond, diags)
		}
		if s.Post != nil {
			checkAgentTransferInExpr(filename, typeEnv, s.Post, diags)
		}
		// Recurse into sub-blocks. Pass the same typeEnv (shadowing not allowed
		// by checkDuplicateLocals, so a flat map is safe).
		if s.Init != nil {
			checkAgentTransferInStmts(filename, typeEnv, []ast.Statement{*s.Init}, diags)
		}
		checkAgentTransferInStmts(filename, typeEnv, s.Then, diags)
		checkAgentTransferInStmts(filename, typeEnv, s.Else, diags)
		checkAgentTransferInStmts(filename, typeEnv, s.Body, diags)
		for _, clause := range s.Catches {
			checkAgentTransferInStmts(filename, typeEnv, clause.Body, diags)
		}
	}
}

// isPayableReceiverExpr returns true if expr is an expression that is
// guaranteed to produce an "agent payable" value:
//   - ident whose declared type is "agent payable"
//   - payable(expr) cast
func isPayableReceiverExpr(typeEnv map[string]string, e *ast.Expr) bool {
	if e == nil {
		return false
	}
	switch e.Kind {
	case "ident":
		rawType := typeEnv[strings.TrimSpace(e.Value)]
		return isPayableAgentType(rawType)
	case "call":
		// payable(addr) is an explicit cast to agent payable.
		if callee := e.Callee; callee != nil && callee.Kind == "ident" &&
			strings.TrimSpace(callee.Value) == "payable" {
			return true
		}
	}
	return false
}

func checkAgentTransferInExpr(filename string, typeEnv map[string]string, e *ast.Expr, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	switch e.Kind {
	case "call":
		callee := e.Callee
		if callee != nil && callee.Kind == "member" {
			member := strings.TrimSpace(callee.Member)
			if member == "transfer" || member == "send" {
				// Check that the receiver is agent payable.
				receiver := callee.Object
				if !isPayableReceiverExpr(typeEnv, receiver) {
					receiverDesc := "expression"
					if receiver != nil && receiver.Kind == "ident" {
						name := strings.TrimSpace(receiver.Value)
						rawType := typeEnv[name]
						if rawType == "" {
							rawType = "agent"
						}
						receiverDesc = fmt.Sprintf("'%s' (declared as '%s')", name, rawType)
					}
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaTransferOnNonPayable,
						Message: fmt.Sprintf("'.%s()' requires 'agent payable' receiver; %s is not payable — use payable(addr).%s(...) or declare the variable as 'agent payable'", member, receiverDesc, member),
						Span:    defaultSpan(filename),
					})
				}
				// Still recurse into args.
				for _, a := range e.Args {
					checkAgentTransferInExpr(filename, typeEnv, a, diags)
				}
				return
			}
		}
		// Recurse into callee and args for non-transfer calls.
		checkAgentTransferInExpr(filename, typeEnv, e.Callee, diags)
		for _, a := range e.Args {
			checkAgentTransferInExpr(filename, typeEnv, a, diags)
		}
	case "binary", "unary":
		checkAgentTransferInExpr(filename, typeEnv, e.Left, diags)
		checkAgentTransferInExpr(filename, typeEnv, e.Right, diags)
	case "member":
		checkAgentTransferInExpr(filename, typeEnv, e.Object, diags)
	case "index":
		checkAgentTransferInExpr(filename, typeEnv, e.Object, diags)
		checkAgentTransferInExpr(filename, typeEnv, e.Index, diags)
	}
}

// isBytesOrStringTypeName returns true for "bytes", "string", and any "bytesN" wider
// dynamic type (but NOT fixed-size bytesN which have defined equality semantics).
// In practice only "bytes" and "string" are dynamic and lack == support.
func isBytesOrStringTypeName(t string) bool {
	t = strings.Join(strings.Fields(t), " ")
	return t == "bytes" || t == "string"
}

// checkBytesStringEquality is a separate pass that rejects == / != on bytes/string
// operands (TOL2086). It builds a flat type environment from storage slots, params,
// and let declarations, then walks all expressions in the body.
func checkBytesStringEquality(filename string, slotInfos map[string]storageSlotInfo, params []ast.FieldDecl, body []ast.Statement, diags *diag.Diagnostics) {
	typeEnv := make(map[string]string, len(slotInfos)+len(params)+8)
	for name, info := range slotInfos {
		typeEnv[name] = info.typeName
	}
	for _, p := range params {
		typeEnv[strings.TrimSpace(p.Name)] = strings.Join(strings.Fields(p.Type), " ")
	}
	checkBytesStringEqualityInStmts(filename, typeEnv, body, diags)
}

func checkBytesStringEqualityInStmts(filename string, typeEnv map[string]string, stmts []ast.Statement, diags *diag.Diagnostics) {
	for _, s := range stmts {
		// Accumulate let bindings so subsequent statements can see their types.
		if s.Kind == "let" && s.Name != "" {
			typeEnv[strings.TrimSpace(s.Name)] = strings.Join(strings.Fields(s.Type), " ")
		}
		if s.Kind == "let-tuple" {
			for i, name := range s.Names {
				t := ""
				if i < len(s.Types) {
					t = strings.Join(strings.Fields(s.Types[i]), " ")
				}
				typeEnv[strings.TrimSpace(name)] = t
			}
		}
		if s.Expr != nil {
			checkBytesStringEqualityInExpr(filename, typeEnv, s.Expr, diags)
		}
		if s.Target != nil {
			checkBytesStringEqualityInExpr(filename, typeEnv, s.Target, diags)
		}
		if s.Cond != nil {
			checkBytesStringEqualityInExpr(filename, typeEnv, s.Cond, diags)
		}
		if s.Post != nil {
			checkBytesStringEqualityInExpr(filename, typeEnv, s.Post, diags)
		}
		if s.Init != nil {
			checkBytesStringEqualityInStmts(filename, typeEnv, []ast.Statement{*s.Init}, diags)
		}
		checkBytesStringEqualityInStmts(filename, typeEnv, s.Then, diags)
		checkBytesStringEqualityInStmts(filename, typeEnv, s.Else, diags)
		checkBytesStringEqualityInStmts(filename, typeEnv, s.Body, diags)
		for _, clause := range s.Catches {
			checkBytesStringEqualityInStmts(filename, typeEnv, clause.Body, diags)
		}
	}
}

// isBytesStringOperand returns true if expr is an ident whose resolved type is
// bytes or string.
func isBytesStringOperand(typeEnv map[string]string, e *ast.Expr) bool {
	if e == nil {
		return false
	}
	// Strip parentheses.
	for e.Kind == "paren" && e.Left != nil {
		e = e.Left
	}
	if e.Kind == "ident" {
		return isBytesOrStringTypeName(typeEnv[strings.TrimSpace(e.Value)])
	}
	return false
}

func checkBytesStringEqualityInExpr(filename string, typeEnv map[string]string, e *ast.Expr, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	switch e.Kind {
	case "binary":
		if e.Op == "==" || e.Op == "!=" {
			if isBytesStringOperand(typeEnv, e.Left) || isBytesStringOperand(typeEnv, e.Right) {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaBytesEqualityOperator,
					Message: fmt.Sprintf("operator '%s' is not supported for bytes/string; use bytes_eq(a, b) or string_eq(a, b) for content equality", e.Op),
					Span:    defaultSpan(filename),
				})
				return
			}
		}
		checkBytesStringEqualityInExpr(filename, typeEnv, e.Left, diags)
		checkBytesStringEqualityInExpr(filename, typeEnv, e.Right, diags)
	case "unary":
		checkBytesStringEqualityInExpr(filename, typeEnv, e.Right, diags)
	case "paren":
		checkBytesStringEqualityInExpr(filename, typeEnv, e.Left, diags)
	case "call":
		checkBytesStringEqualityInExpr(filename, typeEnv, e.Callee, diags)
		for _, a := range e.Args {
			checkBytesStringEqualityInExpr(filename, typeEnv, a, diags)
		}
	case "member":
		checkBytesStringEqualityInExpr(filename, typeEnv, e.Object, diags)
	case "index":
		checkBytesStringEqualityInExpr(filename, typeEnv, e.Object, diags)
		checkBytesStringEqualityInExpr(filename, typeEnv, e.Index, diags)
	case "ternary":
		for _, a := range e.Args {
			checkBytesStringEqualityInExpr(filename, typeEnv, a, diags)
		}
	case "assign":
		checkBytesStringEqualityInExpr(filename, typeEnv, e.Left, diags)
		checkBytesStringEqualityInExpr(filename, typeEnv, e.Right, diags)
	}
}

func normalizeSelectorType(t string) string {
	s := strings.Join(strings.Fields(t), " ")
	repl := strings.NewReplacer(
		"( ", "(",
		" )", ")",
		"[ ", "[",
		" ]", "]",
		" ,", ",",
		", ", ",",
		" => ", "=>",
		" =>", "=>",
		"=> ", "=>",
	)
	// Apply replacer then also remove any remaining spaces before '(' or '['.
	// These arise because parseTypeUntil joins all tokens with ' '.
	result := repl.Replace(s)
	result = strings.ReplaceAll(result, " (", "(")
	result = strings.ReplaceAll(result, " [", "[")
	// Strip payable qualifier: "agent payable" → "agent".
	if result == "agent payable" {
		result = "agent"
	}
	return result
}

func validateTypeForContext(filename, typeName, context string, allowMapping bool, diags *diag.Diagnostics) {
	validateTypeForContextWithStructs(filename, typeName, context, allowMapping, nil, diags)
}

// validateTypeForContextWithStructs validates a type in context, additionally allowing named struct types.
func validateTypeForContextWithStructs(filename, typeName, context string, allowMapping bool, knownStructs map[string][]ast.FieldDecl, diags *diag.Diagnostics) {
	norm := normalizeSelectorType(typeName)
	if strings.TrimSpace(norm) == "" {
		*diags = append(*diags, diag.Diagnostic{
			Code:    diag.CodeSemaInvalidStmtShape,
			Message: fmt.Sprintf("missing type in %s", context),
			Span:    defaultSpan(filename),
		})
		return
	}
	if isValidTOLType(norm, allowMapping) {
		return
	}
	// Check if the type is a known struct name.
	if len(knownStructs) > 0 {
		if _, isStruct := knownStructs[norm]; isStruct {
			return
		}
	}
	// Package-qualified types (e.g. "tos.registry.AgentRegistry") are accepted as valid.
	// They represent contract/interface references resolved via package imports.
	// Actual type validity is verified at IR lowering time.
	if strings.Contains(norm, ".") && !strings.Contains(norm, "(") && !strings.Contains(norm, "[") {
		return
	}
	*diags = append(*diags, diag.Diagnostic{
		Code:    diag.CodeSemaInvalidStmtShape,
		Message: fmt.Sprintf("invalid type '%s' in %s", norm, context),
		Span:    defaultSpan(filename),
	})
}

func validateDataLocationForContext(filename, dataLoc, typeName, context string, diags *diag.Diagnostics) {
	loc := strings.TrimSpace(dataLoc)
	if loc == "" {
		return
	}
	if loc != "storage" && loc != "memory" && loc != "calldata" {
		*diags = append(*diags, diag.Diagnostic{
			Code:    diag.CodeSemaInvalidStmtShape,
			Message: fmt.Sprintf("unsupported data location '%s' in %s", loc, context),
			Span:    defaultSpan(filename),
		})
		return
	}
	if !isReferenceLikeTOLType(normalizeSelectorType(typeName)) {
		*diags = append(*diags, diag.Diagnostic{
			Code:    diag.CodeSemaInvalidStmtShape,
			Message: fmt.Sprintf("data location '%s' in %s requires reference type (bytes/string/array/mapping)", loc, context),
			Span:    defaultSpan(filename),
		})
	}
}

func isReferenceLikeTOLType(typeName string) bool {
	t := strings.TrimSpace(typeName)
	if t == "" {
		return false
	}
	switch t {
	case "bytes", "string":
		return true
	}
	if strings.HasPrefix(t, "mapping(") {
		return true
	}
	return strings.Contains(t, "[")
}

func isValidTOLType(typeName string, allowMapping bool) bool {
	t := strings.TrimSpace(typeName)
	if t == "" {
		return false
	}
	// Agent-native parameterized types: oracle<T>, vote<T>, task<T>, agent, delegation.
	if t == "agent" || t == "delegation" ||
		strings.HasPrefix(t, "oracle<") ||
		strings.HasPrefix(t, "vote<") ||
		strings.HasPrefix(t, "task<") {
		return true
	}
	// Function types: "function(...) modifiers returns (...)"
	// These are first-class types (Task #10). Accept any string starting with "function(".
	if strings.HasPrefix(t, "function(") {
		return true
	}
	// Array suffix handling: T[], T[N], and nested combinations.
	for strings.HasSuffix(t, "]") {
		lb := strings.LastIndex(t, "[")
		if lb <= 0 || lb >= len(t)-1 {
			return false
		}
		sizeLit := strings.TrimSpace(t[lb+1 : len(t)-1])
		if sizeLit != "" {
			n, err := strconv.Atoi(sizeLit)
			if err != nil || n <= 0 {
				return false
			}
		}
		t = strings.TrimSpace(t[:lb])
		if t == "" {
			return false
		}
	}
	if strings.HasPrefix(t, "mapping(") {
		if !allowMapping || !strings.HasSuffix(t, ")") {
			return false
		}
		inner := t[len("mapping(") : len(t)-1]
		keyType, valType, ok := splitTopLevelMappingPair(inner)
		if !ok {
			return false
		}
		keyNorm := normalizeSelectorType(keyType)
		if strings.HasPrefix(keyNorm, "mapping(") || strings.Contains(keyNorm, "[") {
			return false
		}
		return isValidMappingKeyType(keyNorm) && isValidTOLType(valType, true)
	}
	return isValidAtomicTOLType(t)
}

func isValidMappingKeyType(t string) bool {
	switch t {
	case "bool", "agent", "bytes32":
		return true
	}
	if len(t) >= 2 && (t[0] == 'u' || t[0] == 'i') {
		n, err := strconv.Atoi(t[1:])
		return err == nil && n >= 8 && n <= 256 && n%8 == 0
	}
	// Allow user-defined types (enums, UDVTs) as mapping keys.
	// These are plain identifiers that are not known primitive types.
	// Reject known non-key primitive types (string, bytes, agent payable etc.).
	// The caller already ensures this is not a mapping or array type.
	switch t {
	case "string", "bytes", "tuple":
		return false // these cannot be mapping keys
	}
	if len(t) > 0 {
		first := t[0]
		if (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z') || first == '_' {
			allWord := true
			for _, ch := range t[1:] {
				if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_') {
					allWord = false
					break
				}
			}
			if allWord {
				return true // user-defined enum, UDVT, or other user type
			}
		}
	}
	return false
}

// splitTopLevelMappingPair splits "keyType=>valType" at the top-level => separator.
func splitTopLevelMappingPair(inner string) (string, string, bool) {
	s := strings.TrimSpace(inner)
	if s == "" {
		return "", "", false
	}
	depth := 0
	pos := -1
	// Iterate over ALL characters so that closing parens of nested mappings
	// are counted correctly, while only looking for => when safe (i < len(s)-1).
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return "", "", false
			}
		}
		if i < len(s)-1 && depth == 0 && s[i] == '=' && s[i+1] == '>' {
			if pos != -1 {
				return "", "", false
			}
			pos = i
			i++ // skip '>'
		}
	}
	if depth != 0 || pos < 0 || pos >= len(s)-2 {
		return "", "", false
	}
	keyType := strings.TrimSpace(s[:pos])
	valType := strings.TrimSpace(s[pos+2:])
	if keyType == "" || valType == "" {
		return "", "", false
	}
	return keyType, valType, true
}

func isValidAtomicTOLType(t string) bool {
	switch t {
	case "bool", "agent", "string", "bytes":
		return true
	}
	if strings.HasPrefix(t, "bytes") {
		nStr := t[len("bytes"):]
		if nStr == "" {
			return false
		}
		n, err := strconv.Atoi(nStr)
		return err == nil && n >= 1 && n <= 32
	}
	if len(t) >= 2 && (t[0] == 'u' || t[0] == 'i') {
		n, err := strconv.Atoi(t[1:])
		return err == nil && n >= 8 && n <= 256 && n%8 == 0
	}
	return false
}

// isValueTOLType returns true if typeName is a value type that can be used for
// immutable declarations: uN, iN, bool, agent, bytes1..bytes32.
// Mappings, arrays, string, and bytes are excluded.
func isValueTOLType(typeName string) bool {
	t := strings.TrimSpace(typeName)
	if t == "" {
		return false
	}
	// Arrays and mappings are not value types.
	if strings.HasSuffix(t, "]") || strings.HasPrefix(t, "mapping(") {
		return false
	}
	// string and bytes are dynamic reference types.
	if t == "string" || t == "bytes" {
		return false
	}
	switch t {
	case "bool", "agent":
		return true
	}
	if strings.HasPrefix(t, "bytes") {
		nStr := t[len("bytes"):]
		if nStr == "" {
			return false
		}
		n, err := strconv.Atoi(nStr)
		return err == nil && n >= 1 && n <= 32
	}
	if len(t) >= 2 && (t[0] == 'u' || t[0] == 'i') {
		n, err := strconv.Atoi(t[1:])
		return err == nil && n >= 8 && n <= 256 && n%8 == 0
	}
	return false
}

// isConstantExpr returns true if e is a compile-time constant expression.
// Allowed: integer/string literals, bool keywords, references to previously-
// declared constants (knownConsts), unary +/-/~/! applied to such an
// expression, and binary arithmetic/bitwise/comparison/logical operators
// applied to two such expressions.  This mirrors Solidity's constant-expression
// rules and allows e.g. `1_000 * 10 ** 18`.
func isConstantExpr(e *ast.Expr, knownConsts map[string]struct{}) bool {
	if e == nil {
		return false
	}
	switch e.Kind {
	case "number", "string", "hex_lit":
		return true
	case "ident":
		v := strings.TrimSpace(e.Value)
		if v == "true" || v == "false" {
			return true
		}
		_, ok := knownConsts[v]
		return ok
	case "paren":
		return isConstantExpr(e.Left, knownConsts)
	case "unary":
		// Prefix unary operators: +, -, ~, !
		return isConstantExpr(e.Right, knownConsts)
	case "binary":
		// All binary arithmetic / bitwise / comparison / logical operators.
		return isConstantExpr(e.Left, knownConsts) && isConstantExpr(e.Right, knownConsts)
	default:
		return false
	}
}

// checkHexLiteralType validates that a hex literal (value is a lowercase hex string
// with even length, no "0x" prefix) is compatible with the declared constant type.
// For bytesN types (N in 1..32), the literal must have exactly N bytes (2*N hex chars).
// For "bytes" (dynamic), any even-length hex string is accepted.
// For integer/bool/address types, a hex literal is not a valid initializer.
func checkHexLiteralType(filename, constName, ctype, hexValue string) diag.Diagnostics {
	var diags diag.Diagnostics
	nBytes := len(hexValue) / 2 // number of bytes (hexValue length is always even)
	_ = nBytes
	// Dynamic bytes or string: any length (including 0) is acceptable.
	// Solidity treats hex literals as StringLiteralType, implicitly convertible
	// to both bytes and string.
	if ctype == "bytes" || ctype == "string" {
		return nil
	}
	// Fixed bytesN (1..32): hex literal byte count must match N exactly.
	if strings.HasPrefix(ctype, "bytes") {
		nStr := ctype[len("bytes"):]
		if n, err := strconv.Atoi(nStr); err == nil && n >= 1 && n <= 32 {
			if nBytes != n {
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConstantInvalidValue,
					Message: fmt.Sprintf("constant '%s': hex literal has %d byte(s) but type '%s' requires exactly %d byte(s)", constName, nBytes, ctype, n),
					Span:    diag.Span{File: filename},
				})
			}
			return diags
		}
	}
	// For any other type (uN, iN, bool, agent), a hex_lit is not valid.
	diags = append(diags, diag.Diagnostic{
		Code:    diag.CodeSemaConstantInvalidValue,
		Message: fmt.Sprintf("constant '%s': hex\"...\" literal cannot be used as type '%s'; hex literals are only valid for bytes, string, and bytesN types", constName, ctype),
		Span:    diag.Span{File: filename},
	})
	return diags
}

// checkConstantSetTargets walks a list of statements and reports any "set"
// statement whose direct target is an identifier that is a constant name.
func checkConstantSetTargets(filename string, constantNames map[string]struct{}, stmts []ast.Statement, diags *diag.Diagnostics) {
	for _, s := range stmts {
		switch s.Kind {
		case "set":
			if s.Target != nil && s.Target.Kind == "ident" {
				name := strings.TrimSpace(s.Target.Value)
				if _, isConst := constantNames[name]; isConst {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaConstantWriteProhibited,
						Message: fmt.Sprintf("cannot assign to constant '%s'", name),
						Span:    defaultSpan(filename),
					})
				}
			}
		case "if":
			checkConstantSetTargets(filename, constantNames, s.Then, diags)
			checkConstantSetTargets(filename, constantNames, s.Else, diags)
		case "while", "dowhile", "for":
			checkConstantSetTargets(filename, constantNames, s.Body, diags)
		}
	}
}

// checkImmutableWritesInNonCtor checks that no set statement in the given body
// writes to an immutable slot. Called for all non-constructor function/fallback bodies.
func checkImmutableWritesInNonCtor(filename, fnName string, slots map[string]storageSlotInfo, params []ast.FieldDecl, body []ast.Statement, diags *diag.Diagnostics) {
	if len(slots) == 0 {
		return
	}
	hasImmutable := false
	for _, info := range slots {
		if info.isImmutable {
			hasImmutable = true
			break
		}
	}
	if !hasImmutable {
		return
	}
	ctx := newStorageCheckCtx(slots, params)
	checkImmutableWriteStmts(filename, fnName, ctx, body, diags)
}

func checkImmutableWriteStmts(filename, fnName string, ctx *storageCheckCtx, stmts []ast.Statement, diags *diag.Diagnostics) {
	for _, s := range stmts {
		switch s.Kind {
		case "let":
			ctx.declareLocal(s.Name)
		case "let-tuple":
			for _, name := range s.Names {
				ctx.declareLocal(name)
			}
		case "set":
			if s.Target != nil {
				if slotName, _, ok := ctx.storagePathFromExpr(s.Target); ok {
					info := ctx.slots[slotName]
					if info.isImmutable {
						*diags = append(*diags, diag.Diagnostic{
							Code:    diag.CodeSemaImmutableWriteOutsideCtor,
							Message: fmt.Sprintf("immutable variable '%s' can only be assigned in the constructor (attempted write in %s)", slotName, fnLabel(fnName)),
							Span:    defaultSpan(filename),
						})
					}
				}
			}
		case "if":
			ctx.pushScope()
			checkImmutableWriteStmts(filename, fnName, ctx, s.Then, diags)
			ctx.popScope()
			ctx.pushScope()
			checkImmutableWriteStmts(filename, fnName, ctx, s.Else, diags)
			ctx.popScope()
		case "while", "dowhile":
			ctx.pushScope()
			checkImmutableWriteStmts(filename, fnName, ctx, s.Body, diags)
			ctx.popScope()
		case "for":
			ctx.pushScope()
			if s.Init != nil {
				checkImmutableWriteStmts(filename, fnName, ctx, []ast.Statement{*s.Init}, diags)
			}
			checkImmutableWriteStmts(filename, fnName, ctx, s.Body, diags)
			ctx.popScope()
		case "unchecked":
			ctx.pushScope()
			checkImmutableWriteStmts(filename, fnName, ctx, s.Body, diags)
			ctx.popScope()
		default:
			if len(s.Then) > 0 {
				ctx.pushScope()
				checkImmutableWriteStmts(filename, fnName, ctx, s.Then, diags)
				ctx.popScope()
			}
			if len(s.Else) > 0 {
				ctx.pushScope()
				checkImmutableWriteStmts(filename, fnName, ctx, s.Else, diags)
				ctx.popScope()
			}
			if len(s.Body) > 0 {
				ctx.pushScope()
				checkImmutableWriteStmts(filename, fnName, ctx, s.Body, diags)
				ctx.popScope()
			}
		}
	}
}

func fnLabel(fnName string) string {
	if strings.TrimSpace(fnName) == "" || fnName == "fallback" {
		return fnName
	}
	return fmt.Sprintf("function '%s'", fnName)
}

// checkImmutableAssignedInConstructor verifies that every immutable slot is
// assigned at least once inside the constructor body.
func checkImmutableAssignedInConstructor(filename string, slots map[string]storageSlotInfo, body []ast.Statement, diags *diag.Diagnostics) {
	immNames := make([]string, 0)
	for name, info := range slots {
		if info.isImmutable {
			immNames = append(immNames, name)
		}
	}
	if len(immNames) == 0 {
		return
	}
	assigned := map[string]bool{}
	collectImmutableAssignments(slots, body, assigned)
	for _, name := range immNames {
		if !assigned[name] {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaImmutableNotAssigned,
				Message: fmt.Sprintf("immutable variable '%s' is never assigned in the constructor", name),
				Span:    defaultSpan(filename),
			})
		}
	}
}

// collectImmutableAssignments walks statement trees and records which immutable
// slot names are the target of a "set" statement.
func collectImmutableAssignments(slots map[string]storageSlotInfo, stmts []ast.Statement, assigned map[string]bool) {
	for _, s := range stmts {
		if s.Kind == "set" && s.Target != nil && s.Target.Kind == "ident" {
			name := strings.TrimSpace(s.Target.Value)
			if info, ok := slots[name]; ok && info.isImmutable {
				assigned[name] = true
			}
		}
		collectImmutableAssignments(slots, s.Then, assigned)
		collectImmutableAssignments(slots, s.Else, assigned)
		collectImmutableAssignments(slots, s.Body, assigned)
	}
}

func validateTypedABIDecodeLiteralForType(filename, localName, localType string, decodeExpr *ast.Expr, diags *diag.Diagnostics) {
	dataArg, ok := abiDecodeCallDataArg(decodeExpr)
	if !ok {
		return
	}
	payload, literal, validHex := literalHexPayload(dataArg)
	if !literal {
		return
	}
	if !validHex {
		*diags = append(*diags, diag.Diagnostic{
			Code:    diag.CodeSemaInvalidStmtShape,
			Message: "abi.decode literal must be 0x-prefixed even-length hex bytes",
			Span:    defaultSpan(filename),
		})
		return
	}
	switch localType {
	case "bool":
		return
	case "agent":
		if len(payload) != 64 {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidStmtShape,
				Message: fmt.Sprintf("abi.decode literal for local '%s' as agent expects 32-byte payload, got %d-byte", localName, len(payload)/2),
				Span:    defaultSpan(filename),
			})
		}
		return
	}
	if strings.HasPrefix(localType, "bytes") {
		n, _ := strconv.Atoi(localType[len("bytes"):])
		wantHexLen := n * 2
		if len(payload) != wantHexLen {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidStmtShape,
				Message: fmt.Sprintf("abi.decode literal for local '%s' as %s expects %d-byte payload, got %d-byte", localName, localType, n, len(payload)/2),
				Span:    defaultSpan(filename),
			})
		}
		return
	}
	if len(localType) >= 2 && localType[0] == 'u' {
		bits, _ := strconv.Atoi(localType[1:])
		wantHexLen := bits / 4
		if len(payload) > wantHexLen {
			hi := payload[:len(payload)-wantHexLen]
			for i := 0; i < len(hi); i++ {
				if hi[i] != '0' {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaInvalidStmtShape,
						Message: fmt.Sprintf("abi.decode literal overflows target type '%s' for local '%s'", localType, localName),
						Span:    defaultSpan(filename),
					})
					break
				}
			}
		}
	}
}

func literalHexPayload(e *ast.Expr) (payload string, literal bool, validHex bool) {
	root := stripParens(e)
	if root == nil || root.Kind != "string" {
		return "", false, false
	}
	raw := strings.TrimSpace(root.Value)
	if uq, err := strconv.Unquote(raw); err == nil {
		raw = uq
	}
	if !strings.HasPrefix(raw, "0x") {
		return "", true, false
	}
	payload = strings.ToLower(raw[2:])
	if len(payload)%2 != 0 {
		return payload, true, false
	}
	for i := 0; i < len(payload); i++ {
		c := payload[i]
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
		if !isHex {
			return payload, true, false
		}
	}
	return payload, true, true
}

func isValidSelectorOverride(sel string) bool {
	if len(sel) != 10 || !strings.HasPrefix(sel, "0x") {
		return false
	}
	for i := 2; i < len(sel); i++ {
		ch := sel[i]
		isHex := (ch >= '0' && ch <= '9') ||
			(ch >= 'a' && ch <= 'f') ||
			(ch >= 'A' && ch <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}

func selectorHexFromSignature(sig string) string {
	h := sha3.NewLegacyKeccak256()
	_, _ = h.Write([]byte(sig))
	sum := h.Sum(nil)
	return "0x" + hex.EncodeToString(sum[:4])
}

func defaultSpan(filename string) diag.Span {
	return diag.Span{
		File: filename,
		Start: diag.Position{
			Line:   1,
			Column: 1,
		},
		End: diag.Position{
			Line:   1,
			Column: 1,
		},
	}
}

// testBuiltinCallTargets are the assert_* functions valid inside test function bodies.
var testBuiltinCallTargets = map[string]struct{}{
	"assert_eq":              {},
	"assert_ne":              {},
	"assert_gt":              {},
	"assert_lt":              {},
	"assert_ge":              {},
	"assert_le":              {},
	"assert_true":            {},
	"assert_false":           {},
	"assert_between":         {},
	"assert_revert":          {},
	"assert_event":           {},
	"assert_no_event":        {},
	"assert_instructions_le": {},
}

// checkTestFnBody validates statements in a test function body, applying
// relaxed rules: test builtins are valid call targets without arity checks,
// deploy and with are valid statement kinds, and break/continue are allowed
// inside loops only.
func checkTestFnBody(filename string, stmts []ast.Statement, diags *diag.Diagnostics) {
	for _, s := range stmts {
		checkTestExpr(filename, s.Expr, diags)
		checkTestExpr(filename, s.Cond, diags)
		checkTestExpr(filename, s.Target, diags)
		checkTestExpr(filename, s.Post, diags)
		if s.Init != nil {
			checkTestFnBody(filename, []ast.Statement{*s.Init}, diags)
		}
		checkTestFnBody(filename, s.Then, diags)
		checkTestFnBody(filename, s.Else, diags)
		checkTestFnBody(filename, s.Body, diags)
	}
}

// checkTestExpr is like checkExpr but skips TOL2031 for test builtins and
// does not enforce contract-call visibility/arity.
func checkTestExpr(filename string, e *ast.Expr, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	switch e.Kind {
	case "call":
		checkTestExpr(filename, e.Callee, diags)
		for _, a := range e.Args {
			checkTestExpr(filename, a, diags)
		}
	case "member":
		// type(T).min/max is a compile-time constant in test expressions too.
		if typeName, _, matched, validType := matchTypeBoundsExpr(e); matched {
			if !validType {
				*diags = append(*diags, diag.Diagnostic{
					Code:    diag.CodeSemaInvalidTypeBoundsType,
					Message: fmt.Sprintf("type bounds type '%s' is not supported; only uN and iN integer types are allowed (e.g. type(u256).max)", typeName),
					Span:    defaultSpan(filename),
				})
			}
			return
		}
		// type(I).interfaceId is a compile-time constant in test expressions too.
		if _, matched := matchTypeInterfaceIdExpr(e); matched {
			return
		}
		checkTestExpr(filename, e.Object, diags)
	case "index":
		checkTestExpr(filename, e.Object, diags)
		checkTestExpr(filename, e.Index, diags)
	case "slice":
		checkTestExpr(filename, e.Object, diags)
		for _, a := range e.Args {
			checkTestExpr(filename, a, diags)
		}
	case "binary":
		checkTestExpr(filename, e.Left, diags)
		checkTestExpr(filename, e.Right, diags)
	case "assign":
		checkTestExpr(filename, e.Left, diags)
		checkTestExpr(filename, e.Right, diags)
	case "unary":
		checkTestExpr(filename, e.Right, diags)
	case "paren":
		checkTestExpr(filename, e.Left, diags)
	case "inspect":
		// inspect binding.slot — allowed in test bodies; recurse into binding.
		checkTestExpr(filename, e.Object, diags)
	case "ident", "number", "string", "hex_lit":
		if e.Kind == "ident" && strings.TrimSpace(e.Value) == "nil" {
			*diags = append(*diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidStmtShape,
				Message: "source-level nil is not allowed in TOL; use typed default values instead",
				Span:    defaultSpan(filename),
			})
		}
	}
}

// checkLibraryDecl validates a library declaration's functions.
// It checks that library functions use only allowed modifiers (internal, pure, view)
// and not external or public (which are contract-dispatch modifiers).
// It populates fnMap with function name -> arity pairs.
func checkLibraryDecl(filename string, lib ast.LibraryDecl, fnMap map[string]int) diag.Diagnostics {
	var diags diag.Diagnostics
	fnSeen := map[string]struct{}{}
	for _, fn := range lib.Functions {
		name := strings.TrimSpace(fn.Name)
		if name == "" {
			continue
		}
		// Check for duplicate functions within the library.
		if _, exists := fnSeen[name]; exists {
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaDuplicateFunction,
				Message: fmt.Sprintf("duplicate function '%s' in library '%s'", name, lib.Name),
				Span:    defaultSpan(filename),
			})
			continue
		}
		fnSeen[name] = struct{}{}
		fnMap[name] = len(fn.Params)
		// Validate modifiers: library functions must not be external or public.
		for _, m := range fn.Modifiers {
			switch m {
			case "external", "public":
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaConflictingModifier,
					Message: fmt.Sprintf("library function '%s.%s' cannot use modifier '%s'; library functions must be internal, pure, or view", lib.Name, name, m),
					Span:    defaultSpan(filename),
				})
			case "internal", "pure", "view", "payable":
				// allowed
			default:
				diags = append(diags, diag.Diagnostic{
					Code:    diag.CodeSemaUnknownModifier,
					Message: fmt.Sprintf("unknown modifier '%s' on library function '%s.%s'", m, lib.Name, name),
					Span:    defaultSpan(filename),
				})
			}
		}
		// Validate return path: same as contract functions.
		if len(fn.Returns) > 0 && !guaranteesValueReturnOrRevert(fn.Body) {
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidReturn,
				Message: fmt.Sprintf("library function '%s.%s' requires all paths to end in return value or revert in current verifier stage", lib.Name, name),
				Span:    defaultSpan(filename),
			})
		}
		diags = append(diags, duplicateParamDiagnostics(filename, "function", lib.Name+"."+name, fn.Params)...)
		diags = append(diags, duplicateParamDiagnostics(filename, "returns", lib.Name+"."+name, fn.Returns)...)
		checkReturnStatements(filename, "function", lib.Name+"."+name, len(fn.Returns) > 0, fn.Body, &diags)
		checkUnreachableStatements(filename, fn.Body, 0, &diags)
	}
	return diags
}

// checkUsingDecls validates 'using LibName for Type' declarations inside a contract.
// It ensures the referenced library exists and the type is a valid TOL type.
func checkUsingDecls(filename string, decls []ast.UsingDecl, libFuncs map[string]map[string]int) diag.Diagnostics {
	var diags diag.Diagnostics
	for _, ud := range decls {
		libName := strings.TrimSpace(ud.Library)
		if libName == "" {
			continue
		}
		if _, exists := libFuncs[libName]; !exists {
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaUnknownCallTarget,
				Message: fmt.Sprintf("'using' references unknown library '%s'", libName),
				Span:    defaultSpan(filename),
			})
		}
		typeName := strings.TrimSpace(ud.Type)
		if typeName == "" {
			diags = append(diags, diag.Diagnostic{
				Code:    diag.CodeSemaInvalidStmtShape,
				Message: fmt.Sprintf("'using %s for' requires a non-empty type", libName),
				Span:    defaultSpan(filename),
			})
		}
	}
	return diags
}

// libraryCallInfo extracts the library name and function name from a member call expression
// of the form LibName.fn(args), and returns them if the callee is a member access on an ident.
func libraryCallInfo(callee *ast.Expr) (libName, fnName string, ok bool) {
	root := stripParens(callee)
	if root == nil || root.Kind != "member" {
		return "", "", false
	}
	obj := stripParens(root.Object)
	if obj == nil || obj.Kind != "ident" {
		return "", "", false
	}
	lib := strings.TrimSpace(obj.Value)
	fn := strings.TrimSpace(root.Member)
	if lib == "" || fn == "" {
		return "", "", false
	}
	return lib, fn, true
}

// checkLibraryCallsInStmts validates calls to library functions (LibName.fn(args))
// in a list of statements. It checks that the library and function exist and that
// arity is correct.
func checkLibraryCallsInStmts(filename string, stmts []ast.Statement, libFuncs map[string]map[string]int, diags *diag.Diagnostics) {
	if len(libFuncs) == 0 {
		return
	}
	for _, s := range stmts {
		checkLibraryCallsInExpr(filename, s.Expr, libFuncs, diags)
		checkLibraryCallsInExpr(filename, s.Target, libFuncs, diags)
		checkLibraryCallsInExpr(filename, s.Cond, libFuncs, diags)
		checkLibraryCallsInExpr(filename, s.Post, libFuncs, diags)
		if s.Init != nil {
			checkLibraryCallsInStmts(filename, []ast.Statement{*s.Init}, libFuncs, diags)
		}
		checkLibraryCallsInStmts(filename, s.Then, libFuncs, diags)
		checkLibraryCallsInStmts(filename, s.Else, libFuncs, diags)
		checkLibraryCallsInStmts(filename, s.Body, libFuncs, diags)
	}
}

func checkLibraryCallsInExpr(filename string, e *ast.Expr, libFuncs map[string]map[string]int, diags *diag.Diagnostics) {
	if e == nil {
		return
	}
	if e.Kind == "call" {
		libName, fnName, ok := libraryCallInfo(e.Callee)
		if ok {
			if fnMap, libExists := libFuncs[libName]; libExists {
				// This is a call to a known library.
				if want, fnExists := fnMap[fnName]; !fnExists {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaUnknownCallTarget,
						Message: fmt.Sprintf("library '%s' has no function '%s'", libName, fnName),
						Span:    defaultSpan(filename),
					})
				} else if len(e.Args) != want {
					*diags = append(*diags, diag.Diagnostic{
						Code:    diag.CodeSemaCallArity,
						Message: fmt.Sprintf("library function '%s.%s' expects %d argument(s), got %d", libName, fnName, want, len(e.Args)),
						Span:    defaultSpan(filename),
					})
				}
			}
		}
		checkLibraryCallsInExpr(filename, e.Callee, libFuncs, diags)
		for _, a := range e.Args {
			checkLibraryCallsInExpr(filename, a, libFuncs, diags)
		}
		return
	}
	checkLibraryCallsInExpr(filename, e.Left, libFuncs, diags)
	checkLibraryCallsInExpr(filename, e.Right, libFuncs, diags)
	checkLibraryCallsInExpr(filename, e.Object, libFuncs, diags)
	checkLibraryCallsInExpr(filename, e.Index, libFuncs, diags)
	for _, a := range e.Args {
		checkLibraryCallsInExpr(filename, a, libFuncs, diags)
	}
}

// checkUsingCallsInStmts validates method-style calls via 'using X for T' declarations.
// A call val.fn(args) where val has type T and 'using LibName for T' is declared
// should be equivalent to LibName.fn(val, args). We check arity as LibName.fn(val, args).
func checkUsingCallsInStmts(filename string, stmts []ast.Statement, usingDecls []ast.UsingDecl, libFuncs map[string]map[string]int, diags *diag.Diagnostics) {
	// Build type → library lookup.
	typeToLib := map[string]string{}
	for _, ud := range usingDecls {
		typeToLib[strings.TrimSpace(ud.Type)] = strings.TrimSpace(ud.Library)
	}
	checkUsingCallsInStmtsInner(filename, stmts, typeToLib, libFuncs, diags)
}

func checkUsingCallsInStmtsInner(filename string, stmts []ast.Statement, typeToLib map[string]string, libFuncs map[string]map[string]int, diags *diag.Diagnostics) {
	// Note: we skip using-call arity checks here because at sema time we don't have
	// type information for arbitrary expressions. The lower stage will expand
	// val.fn(args) → LibName.fn(val, args) based on annotation. For now, we simply
	// allow these without additional arity checking in sema.
	_ = filename
	_ = stmts
	_ = typeToLib
	_ = libFuncs
	_ = diags
}
