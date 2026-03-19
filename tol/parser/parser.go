package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/diag"
	"github.com/tos-network/tolang/tol/lexer"
)

type Parser struct {
	filename           string
	lex                *lexer.Lexer
	cur                lexer.Token
	diags              diag.Diagnostics
	structNames        map[string]struct{} // known struct names for struct literal disambiguation
	inAbstractContract bool               // true when parsing the body of an abstract contract
	pendingDoc         *ast.DocMeta       // accumulated from TokenDocComment; cleared after binding
}

func ParseFile(filename string, src []byte) (*ast.Module, diag.Diagnostics) {
	return parseFileWith(filename, src, false)
}

// ParseFileStrict parses in strict mode: rejects Solidity type aliases (uint256 → u256 etc.).
func ParseFileStrict(filename string, src []byte) (*ast.Module, diag.Diagnostics) {
	return parseFileWith(filename, src, true)
}

func parseFileWith(filename string, src []byte, strict bool) (*ast.Module, diag.Diagnostics) {
	lex := lexer.New(src)
	lex.Strict = strict
	p := &Parser{
		filename:    filename,
		lex:         lex,
		structNames: map[string]struct{}{},
	}
	p.next()
	mod := p.parseModule()
	return mod, p.diags
}

// extractPragmaVersion extracts the first version number from pragma tokens.
// It handles Solidity-style version constraints:
//
//	0.2.0          plain version
//	^0.8.0         caret
//	~0.8.0         tilde
//	>=0.7.0        comparison
//	>=0.7.0 <0.9.0 conjunction
//	0.2.0 || 0.3.0 disjunction
//
// The first dot-separated numeric token (skipping the language name and operators) is returned.
func extractPragmaVersion(tokens []string) string {
	// Semver range operator literals produced by the TOL lexer.
	rangeOps := map[string]bool{
		"^": true, "~": true,
		">=": true, "<=": true, ">": true, "<": true, "=": true,
		"||": true, "-": true,
	}
	for i, tok := range tokens {
		if rangeOps[tok] {
			continue
		}
		// Skip the first token when it is a pure language name (e.g. "tolang", "solidity").
		if i == 0 {
			allAlpha := true
			for _, ch := range tok {
				if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')) {
					allAlpha = false
					break
				}
			}
			if allAlpha {
				continue
			}
		}
		// A version token contains at least one digit and at least one dot.
		hasDigit, hasDot := false, false
		for _, ch := range tok {
			if ch >= '0' && ch <= '9' {
				hasDigit = true
			}
			if ch == '.' {
				hasDot = true
			}
		}
		if hasDigit && hasDot {
			return tok
		}
	}
	return ""
}

func (p *Parser) parseModule() *ast.Module {
	mod := &ast.Module{}

	// Version header: pragma tolang MAJOR.MINOR.PATCH;
	// Also accepts: pragma solidity <version-tokens>; (leniently)
	if !p.expect(lexer.TokenKwPragma, diag.CodeParseUnexpected, "expected 'pragma tolang <version>;' version header") {
		return mod
	}
	var pragmaTokens []string
	for p.cur.Type != lexer.TokenSemicolon && p.cur.Type != lexer.TokenEOF {
		pragmaTokens = append(pragmaTokens, p.cur.Literal)
		p.next()
	}
	if p.cur.Type == lexer.TokenSemicolon {
		p.next()
	}
	// Extract the version number from pragma tokens.
	// Accepts Solidity-style constraints: ^0.8.0, >=0.7.0 <0.9.0, 0.2, 0.2.0, etc.
	// Strategy: skip the language name (first purely-alpha token) and range operators,
	// then take the first token that looks like a version number (digits + dots).
	mod.Version = extractPragmaVersion(pragmaTokens)

	// Optional package declaration: package tolang.registry;
	if p.cur.Type == lexer.TokenKwPackage {
		p.next() // consume 'package'
		mod.Package = p.parseDottedName()
		if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after package declaration") {
			return mod
		}
	}

	// Parse import declarations (must come before other top-level declarations).
	for p.cur.Type == lexer.TokenKwImport {
		imp := p.parseImportDecl()
		if imp != nil {
			mod.Imports = append(mod.Imports, *imp)
		}
	}

	// Parse top-level declarations: interfaces, libraries, structs, abstract contracts,
	// free functions, constants, enums, errors, events, and using declarations.
	// These may appear in any order before the (optional) concrete contract.
	for {
		if p.cur.Type == lexer.TokenKwInterface {
			iface := p.parseInterfaceDecl()
			if iface != nil {
				mod.Interfaces = append(mod.Interfaces, *iface)
			}
		} else if p.cur.Type == lexer.TokenKwLibrary {
			lib := p.parseLibraryDecl()
			if lib != nil {
				mod.Libraries = append(mod.Libraries, *lib)
			}
		} else if p.cur.Type == lexer.TokenKwStruct {
			// "struct" keyword
			sd := p.parseStructDecl()
			if sd != nil {
				p.structNames[sd.Name] = struct{}{}
				mod.Structs = append(mod.Structs, *sd)
			}
		} else if p.cur.Type == lexer.TokenKwType {
			// "type Name is UnderlyingType;" — user-defined value type declaration
			td := p.parseTypeDecl()
			if td != nil {
				mod.TypeDecls = append(mod.TypeDecls, *td)
			}
		} else if p.cur.Type == lexer.TokenKwFunction {
			// Free function at file level
			fn := p.parseFunctionDecl("")
			if fn != nil {
				mod.FreeFunctions = append(mod.FreeFunctions, *fn)
			}
		} else if p.cur.Type == lexer.TokenKwError {
			// Top-level error declaration
			ed := p.parseErrorDecl()
			if ed != nil {
				mod.Errors = append(mod.Errors, *ed)
			}
		} else if p.cur.Type == lexer.TokenKwEnum {
			// Top-level enum declaration
			en := p.parseEnumDecl()
			if en != nil {
				mod.Enums = append(mod.Enums, *en)
			}
		} else if p.cur.Type == lexer.TokenKwEvent {
			// Top-level event declaration
			ev := p.parseEventDecl()
			if ev != nil {
				mod.Events = append(mod.Events, *ev)
			}
		} else if p.cur.Type == lexer.TokenKwConstant {
			// Top-level constant: constant NAME: TYPE = VALUE;
			cd := p.parseConstantDecl()
			if cd != nil {
				mod.Constants = append(mod.Constants, *cd)
			}
		} else if p.cur.Type == lexer.TokenKwUsing || (p.cur.Type == lexer.TokenIdent && p.cur.Literal == "using") {
			// Top-level using-for declaration: using LibName for Type;
			ud := p.parseUsingDecl()
			if ud != nil {
				mod.UsingDecls = append(mod.UsingDecls, *ud)
			}
		} else if p.cur.Type == lexer.TokenIdent && p.cur.Literal == "capability" {
			// Top-level capability declaration: capability Foo;
			cd := p.parseCapabilityDecl()
			if cd != nil {
				mod.Capabilities = append(mod.Capabilities, *cd)
			}
		} else if p.cur.Type == lexer.TokenKwAbstract {
			// Look-ahead: "abstract contract <Name> { ... }" → abstract contract decl.
			saved := p.cur
			p.next() // consume "abstract"
			if p.cur.Type == lexer.TokenKwContract {
				ac := p.parseContractDecl(true)
				if ac != nil {
					mod.AbstractContracts = append(mod.AbstractContracts, *ac)
				}
			} else {
				// Not a contract keyword — put the token back by re-adding it as a
				// diagnostic and break out of the loop.
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnexpected,
					Message: fmt.Sprintf("expected 'contract' after 'abstract', got '%s'", p.cur.Literal),
					Span:    p.span(saved),
				})
				break
			}
		} else if p.isTypeStart() {
			// Peek ahead: type-first constant at top level: uint256 constant MAX = 1000000;
			nxt := p.peekTok()
			if nxt.Type == lexer.TokenKwConstant {
				cd := p.parseConstantDeclTypeFirst()
				if cd != nil {
					mod.Constants = append(mod.Constants, *cd)
				}
			} else {
				break
			}
		} else {
			break
		}
	}

	// A module may have test blocks before any contract declaration.
	for p.cur.Type == lexer.TokenKwTest {
		td := p.parseTestDecl()
		if td != nil {
			mod.Tests = append(mod.Tests, *td)
		}
	}

	// Parse zero or more concrete contract declarations (multi-contract files).
	// Interfaces, abstract contracts, and libraries may be interspersed between contracts.
	for {
		switch p.cur.Type {
		case lexer.TokenKwContract:
			concrete := p.parseContractDecl(false)
			if concrete != nil {
				mod.Contracts = append(mod.Contracts, *concrete)
				// Maintain backward-compatible mod.Contract pointing to the first contract.
				if mod.Contract == nil {
					mod.Contract = &mod.Contracts[0]
				} else {
					// Re-point to the slice element (slice may have been reallocated).
					mod.Contract = &mod.Contracts[0]
				}
			}
		case lexer.TokenIdent:
			// "account contract <Name> { ... }" — account modifier on contract.
			if p.cur.Literal == "account" {
				saved := p.cur
				p.next() // consume "account"
				if p.cur.Type == lexer.TokenKwContract {
					concrete := p.parseContractDecl(false)
					if concrete != nil {
						concrete.IsAccount = true
						mod.Contracts = append(mod.Contracts, *concrete)
						if mod.Contract == nil {
							mod.Contract = &mod.Contracts[0]
						} else {
							mod.Contract = &mod.Contracts[0]
						}
					}
				} else {
					p.addDiag(diag.Diagnostic{
						Code:    diag.CodeParseUnexpected,
						Message: fmt.Sprintf("expected 'contract' after 'account', got '%s'", p.cur.Literal),
						Span:    p.span(saved),
					})
					goto afterContracts
				}
			} else {
				goto afterContracts
			}
		case lexer.TokenKwInterface:
			iface := p.parseInterfaceDecl()
			if iface != nil {
				mod.Interfaces = append(mod.Interfaces, *iface)
			}
		case lexer.TokenKwLibrary:
			lib := p.parseLibraryDecl()
			if lib != nil {
				mod.Libraries = append(mod.Libraries, *lib)
			}
		case lexer.TokenKwAbstract:
			saved := p.cur
			p.next() // consume "abstract"
			if p.cur.Type == lexer.TokenKwContract {
				ac := p.parseContractDecl(true)
				if ac != nil {
					mod.AbstractContracts = append(mod.AbstractContracts, *ac)
				}
			} else {
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnexpected,
					Message: fmt.Sprintf("expected 'contract' after 'abstract', got '%s'", p.cur.Literal),
					Span:    p.span(saved),
				})
				goto afterContracts
			}
		case lexer.TokenKwTest:
			// Test blocks may follow each contract declaration.
			for p.cur.Type == lexer.TokenKwTest {
				td := p.parseTestDecl()
				if td != nil {
					mod.Tests = append(mod.Tests, *td)
				}
			}
			continue
		default:
			goto afterContracts
		}
		// Test blocks may also appear between any top-level declarations.
		for p.cur.Type == lexer.TokenKwTest {
			td := p.parseTestDecl()
			if td != nil {
				mod.Tests = append(mod.Tests, *td)
			}
		}
	}
afterContracts:

	// If no contracts were found, check whether the file had other top-level declarations.
	if len(mod.Contracts) == 0 {
		if p.cur.Type == lexer.TokenEOF {
			return mod
		}
		hasTopDecls := len(mod.Tests) > 0 || len(mod.AbstractContracts) > 0 ||
			len(mod.Interfaces) > 0 || len(mod.Libraries) > 0 ||
			len(mod.FreeFunctions) > 0 || len(mod.Constants) > 0 ||
			len(mod.Enums) > 0 || len(mod.Errors) > 0 ||
			len(mod.Events) > 0 || len(mod.UsingDecls) > 0 ||
			len(mod.Structs) > 0 || len(mod.TypeDecls) > 0
		if hasTopDecls {
			if p.cur.Type != lexer.TokenEOF {
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnexpected,
					Message: fmt.Sprintf("unexpected token '%s' after declarations", p.cur.Literal),
					Span:    p.span(p.cur),
				})
			}
			return mod
		}
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected 'contract' declaration",
			Span:    p.span(p.cur),
		})
		return mod
	}

	if p.cur.Type != lexer.TokenEOF {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: fmt.Sprintf("unexpected token '%s' after contract declaration", p.cur.Literal),
			Span:    p.span(p.cur),
		})
	}

	return mod
}

// parseContractDecl parses a contract declaration (abstract or concrete).
// The 'abstract' keyword has already been consumed when isAbstract=true;
// the 'contract' keyword has NOT been consumed yet.
func (p *Parser) parseContractDecl(isAbstract bool) *ast.ContractDecl {
	if !p.expect(lexer.TokenKwContract, diag.CodeParseUnexpected, "expected 'contract' declaration") {
		return nil
	}

	contractName := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected contract name") {
		return nil
	}
	c := &ast.ContractDecl{Name: contractName.Literal, Abstract: isAbstract}

	// Parse optional inheritance specification: is Base1, Base2(arg1, arg2), ...
	if p.cur.Type == lexer.TokenKwIs || (p.cur.Type == lexer.TokenIdent && p.cur.Literal == "is") {
		p.next() // consume "is"
		for {
			if p.cur.Type != lexer.TokenIdent {
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnexpected,
					Message: "expected base contract name after 'is'",
					Span:    p.span(p.cur),
				})
				break
			}
			baseName := p.cur.Literal
			p.next()
			spec := ast.BaseSpecifier{Name: baseName}
			// Optional constructor arguments: Base(arg1, arg2)
			if p.cur.Type == lexer.TokenLParen {
				p.next() // consume '('
				var args []*ast.Expr
				for p.cur.Type != lexer.TokenRParen && p.cur.Type != lexer.TokenEOF {
					arg, ok := p.parseExpression(map[lexer.Type]bool{
						lexer.TokenComma:  true,
						lexer.TokenRParen: true,
					})
					if !ok {
						break
					}
					args = append(args, arg)
					if p.cur.Type == lexer.TokenComma {
						p.next()
					}
				}
				if p.cur.Type == lexer.TokenRParen {
					p.next()
				}
				spec.Args = args
			}
			c.Bases = append(c.Bases, baseName)
			c.BaseSpecifiers = append(c.BaseSpecifiers, spec)
			if p.cur.Type == lexer.TokenComma {
				p.next() // consume ','
				continue
			}
			break
		}
	}

	if !p.expect(lexer.TokenLBrace, diag.CodeParseUnexpected, "expected '{' after contract name") {
		return nil
	}

	p.inAbstractContract = isAbstract
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		p.parseContractMember(c)
	}
	p.inAbstractContract = false
	if !p.expect(lexer.TokenRBrace, diag.CodeParseUnexpected, "expected '}' to close contract body") {
		return nil
	}
	return c
}

func (p *Parser) parseTestDecl() *ast.TestDecl {
	if !p.expect(lexer.TokenKwTest, diag.CodeParseUnexpected, "expected 'test'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected test block name") {
		return nil
	}
	if !p.expect(lexer.TokenLBrace, diag.CodeParseUnexpected, "expected '{' after test name") {
		return nil
	}

	td := &ast.TestDecl{Name: nameTok.Literal}
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		p.parseTestMember(td)
	}
	if !p.expect(lexer.TokenRBrace, diag.CodeParseUnexpected, "expected '}' to close test block") {
		return nil
	}
	return td
}

func (p *Parser) parseTestMember(td *ast.TestDecl) {
	// Handle attribute annotations before test functions: #[skip] #[tag("...")]
	tags, skip, hasCases, hasFuzz, fuzzCount, timeoutMs := p.parseTestAttributes()

	// Contextual keywords: setup, teardown, setup_suite, teardown_suite, fn
	if p.cur.Type == lexer.TokenKwFunction {
		fn := p.parseTestFn(tags, skip, hasCases, hasFuzz, fuzzCount, timeoutMs)
		if fn != nil {
			td.Fns = append(td.Fns, *fn)
		}
		return
	}

	// Block-level variable declarations: Type name = expr;
	// Must check before the TokenIdent gate because promoted type keywords
	// (string, bool, agent, uno) are not TokenIdent.
	if p.isTypeStart() && (p.peek().Type == lexer.TokenIdent || p.peek().Type == lexer.TokenLBracket) {
		// Exclude test-block contextual keywords (setup, teardown, mock, etc.)
		if p.cur.Type != lexer.TokenIdent || !isTestBlockKeyword(p.cur.Literal) {
			stmt, ok := p.parseTypeFirstVarDecl()
			if ok {
				td.Lets = append(td.Lets, stmt)
			}
			return
		}
	}
	// Legacy let syntax (emits error with migration hint).
	if p.cur.Type == lexer.TokenKwLet {
		stmt, ok := p.parseLetStatement(lexer.TokenSemicolon)
		if ok {
			td.Lets = append(td.Lets, stmt)
		}
		return
	}

	if p.cur.Type != lexer.TokenIdent {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnsupported,
			Message: fmt.Sprintf("unexpected token '%s' in test block", p.cur.Literal),
			Span:    p.span(p.cur),
		})
		p.syncUnknownMember()
		return
	}

	kw := p.cur.Literal
	switch kw {
	case "setup", "setup_suite", "teardown", "teardown_suite":
		p.next()
		lf := p.parseTestLifecycleFn(kw)
		if lf == nil {
			return
		}
		switch kw {
		case "setup":
			td.Setup = lf
		case "setup_suite":
			td.SetupSuite = lf
		case "teardown":
			td.Teardown = lf
		case "teardown_suite":
			td.TeardownSuite = lf
		}
	case "mock":
		md := p.parseMockDecl()
		if md != nil {
			td.Mocks = append(td.Mocks, *md)
		}
	default:
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnsupported,
			Message: fmt.Sprintf("unsupported test block member '%s'", kw),
			Span:    p.span(p.cur),
		})
		p.syncUnknownMember()
	}
}

func (p *Parser) parseTestAttributes() (tags []string, skip, hasCases, hasFuzz bool, fuzzCount int, timeoutMs int) {
	// Attributes are not lexed as a single token; '#' is TokenAt-adjacent but
	// in TOL we use '@'. For test blocks we handle #[...] as a special form
	// by treating '#' as illegal. Since we don't have a '#' token, attributes
	// like #[skip] and #[tag("...")] are parsed via '@' convention instead:
	//   @skip, @tag("...")
	for p.cur.Type == lexer.TokenAt {
		p.next() // consume '@'
		if p.cur.Type != lexer.TokenIdent {
			break
		}
		attr := p.cur.Literal
		p.next()
		switch attr {
		case "skip":
			skip = true
		case "cases":
			hasCases = true
		case "fuzz":
			hasFuzz = true
			// optional: @fuzz(count=N)
			if p.cur.Type == lexer.TokenLParen {
				p.next() // consume '('
				// expect ident "count"
				if p.cur.Type == lexer.TokenIdent && p.cur.Literal == "count" {
					p.next() // consume "count"
					if p.cur.Type == lexer.TokenAssign {
						p.next() // consume '='
						if p.cur.Type == lexer.TokenNumber {
							if n, err := strconv.Atoi(p.cur.Literal); err == nil {
								fuzzCount = n
							}
							p.next()
						}
					}
				}
				if p.cur.Type == lexer.TokenRParen {
					p.next()
				}
			}
		case "timeout":
			if p.cur.Type == lexer.TokenLParen {
				p.next() // consume '('
				if p.cur.Type == lexer.TokenNumber {
					if n, err := strconv.Atoi(p.cur.Literal); err == nil {
						timeoutMs = n
					}
					p.next()
				}
				if p.cur.Type == lexer.TokenRParen {
					p.next()
				}
			}
		case "tag":
			if p.cur.Type == lexer.TokenLParen {
				p.next()
				if p.cur.Type == lexer.TokenString {
					tag, err := strconv.Unquote(p.cur.Literal)
					if err != nil {
						tag = p.cur.Literal
					}
					tags = append(tags, tag)
					p.next()
				}
				if p.cur.Type == lexer.TokenRParen {
					p.next()
				}
			}
		default:
			// Unknown attribute; skip the optional (...)
			if p.cur.Type == lexer.TokenLParen {
				_ = p.consumePaired(lexer.TokenLParen, lexer.TokenRParen, "attribute args")
			}
		}
	}
	return tags, skip, hasCases, hasFuzz, fuzzCount, timeoutMs
}

func (p *Parser) parseTestLifecycleFn(kw string) *ast.TestLifecycleFn {
	lf := &ast.TestLifecycleFn{}
	isSetup := kw == "setup" || kw == "setup_suite"

	if isSetup {
		// setup returns (Type binding) { ... }  OR  setup -> (binding: Type) { ... } (legacy)
		if p.cur.Type == lexer.TokenKwReturns || p.cur.Type == lexer.TokenArrow {
			p.next()
			returns, ok := p.parseFieldList(false, false)
			if !ok {
				return nil
			}
			lf.Returns = returns
		}
	} else {
		// teardown (binding: Type) { ... }
		if p.cur.Type == lexer.TokenLParen {
			params, ok := p.parseFieldList(false, false)
			if !ok {
				return nil
			}
			lf.Params = params
		}
	}

	body, ok := p.parseTestStatementBlock(kw + " body")
	if !ok {
		return nil
	}
	lf.Body = body
	return lf
}

func (p *Parser) parseTestFn(tags []string, skip, hasCases, hasFuzz bool, fuzzCount int, timeoutMs int) *ast.TestFn {
	if !p.expect(lexer.TokenKwFunction, diag.CodeParseUnexpected, "expected 'function'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected test function name") {
		return nil
	}
	params, ok := p.parseFieldList(false, false)
	if !ok {
		return nil
	}
	body, ok := p.parseTestStatementBlock("test function body")
	if !ok {
		return nil
	}
	fn := &ast.TestFn{
		Name:      nameTok.Literal,
		Params:    params,
		Tags:      tags,
		Skip:      skip,
		Body:      body,
		Fuzz:      hasFuzz,
		FuzzCount: fuzzCount,
		Timeout:   timeoutMs,
	}
	if hasCases {
		fn.Cases = p.parseCasesTable()
	}
	return fn
}

// parseMockDecl parses:
//
//	mock ContractName [: InterfaceName] {
//	  fn methodName(params...) [-> (returns...)] { ... }
//	  ...
//	}
func (p *Parser) parseMockDecl() *ast.MockDecl {
	p.next() // consume "mock"
	if p.cur.Type != lexer.TokenIdent {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected mock contract name",
			Span:    p.span(p.cur),
		})
		return nil
	}
	md := &ast.MockDecl{Name: p.cur.Literal}
	p.next()
	if p.cur.Type == lexer.TokenColon {
		p.next()
		if p.cur.Type == lexer.TokenIdent {
			md.Interface = p.cur.Literal
			p.next()
		}
	}
	if !p.expect(lexer.TokenLBrace, diag.CodeParseUnexpected, "expected '{' to open mock block") {
		return nil
	}
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		if p.cur.Type != lexer.TokenKwFunction {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("expected 'function' in mock block, got '%s'", p.cur.Literal),
				Span:    p.span(p.cur),
			})
			p.syncUnknownMember()
			continue
		}
		p.next() // consume 'function'
		if p.cur.Type != lexer.TokenIdent {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected mock method name",
				Span:    p.span(p.cur),
			})
			return nil
		}
		mm := ast.MockMethod{Name: p.cur.Literal}
		p.next()
		params, ok := p.parseFieldList(false, false)
		if !ok {
			return nil
		}
		mm.Params = params
		if p.cur.Type == lexer.TokenKwReturns {
			p.next()
			returns, ok := p.parseFieldList(false, false)
			if !ok {
				return nil
			}
			mm.Returns = returns
		}
		body, ok := p.parseTestStatementBlock("mock method body")
		if !ok {
			return nil
		}
		mm.Body = body
		md.Methods = append(md.Methods, mm)
	}
	if !p.expect(lexer.TokenRBrace, diag.CodeParseUnexpected, "expected '}' to close mock block") {
		return nil
	}
	return md
}

// parseCasesTable parses:
//
//	cases {
//	  | col1 | col2 |
//	  | expr | expr |
//	  ...
//	}
func (p *Parser) parseCasesTable() *ast.CasesTable {
	// Expect the ident "cases"
	if p.cur.Type != lexer.TokenIdent || p.cur.Literal != "cases" {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected 'cases' block after @cases test function",
			Span:    p.span(p.cur),
		})
		return nil
	}
	p.next()

	if !p.expect(lexer.TokenLBrace, diag.CodeParseUnexpected, "expected '{' to open cases block") {
		return nil
	}

	ct := &ast.CasesTable{}

	// Parse header row: | col1 | col2 | ...
	if p.cur.Type != lexer.TokenBitOr {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected '|' to start cases header row",
			Span:    p.span(p.cur),
		})
		return nil
	}
	p.next() // consume leading |
	for p.cur.Type == lexer.TokenIdent {
		ct.Columns = append(ct.Columns, p.cur.Literal)
		p.next()
		if p.cur.Type == lexer.TokenBitOr {
			p.next() // consume separator/trailing |
		} else {
			break
		}
	}

	// Parse data rows: | expr | expr | ...
	// Each row starts with a leading |; expressions are separated by | and
	// terminated by a trailing |. The trailing | is detected by checking that
	// the next token after consuming | is another | (next row) or } (end block).
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		if p.cur.Type != lexer.TokenBitOr {
			break
		}
		p.next() // consume leading |
		var row []*ast.Expr
		for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
			expr, ok := p.parseExpression(map[lexer.Type]bool{
				lexer.TokenBitOr:  true,
				lexer.TokenRBrace: true,
			})
			if !ok {
				break
			}
			row = append(row, expr)
			if p.cur.Type == lexer.TokenBitOr {
				p.next() // consume separator or trailing |
				// If the next token is | (next row) or } (end block),
				// the trailing | we just consumed ends this row.
				if p.cur.Type == lexer.TokenBitOr || p.cur.Type == lexer.TokenRBrace || p.cur.Type == lexer.TokenEOF {
					break
				}
			} else {
				break
			}
		}
		ct.Rows = append(ct.Rows, row)
	}

	if !p.expect(lexer.TokenRBrace, diag.CodeParseUnexpected, "expected '}' to close cases block") {
		return nil
	}
	return ct
}

// parseTestStatementBlock parses a block that may contain deploy/with in addition
// to normal statements.
func (p *Parser) parseTestStatementBlock(what string) ([]ast.Statement, bool) {
	if p.cur.Type != lexer.TokenLBrace {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected '{' before " + what,
			Span:    p.span(p.cur),
		})
		return nil, false
	}
	p.next()

	stmts := []ast.Statement{}
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		var stmt ast.Statement
		var ok bool
		if p.cur.Type == lexer.TokenIdent || p.cur.Type == lexer.TokenKwDeploy {
			switch p.cur.Literal {
			case "deploy":
				startLine := p.cur.Start.Line
				p.next()
				stmt, ok = p.parseDeployStatement()
				if ok {
					stmt.Line = startLine
				}
			case "with":
				startLine := p.cur.Start.Line
				p.next()
				stmt, ok = p.parseWithStatement()
				if ok {
					stmt.Line = startLine
				}
			case "assert_all":
				startLine := p.cur.Start.Line
				p.next()
				body, bodyOk := p.parseTestStatementBlock("assert_all body")
				if !bodyOk {
					ok = false
					break
				}
				stmt, ok = ast.Statement{Kind: "assert_all", Body: body, Line: startLine}, true
			case "assert_instructions_le":
				startLine := p.cur.Start.Line
				p.next() // consume "assert_instructions_le"
				limitExpr, exprOk := p.parseExpression(map[lexer.Type]bool{lexer.TokenLBrace: true})
				if !exprOk {
					ok = false
					break
				}
				body, bodyOk := p.parseTestStatementBlock("assert_instructions_le body")
				if !bodyOk {
					ok = false
					break
				}
				stmt, ok = ast.Statement{Kind: "assert_instructions_le", Expr: limitExpr, Body: body, Line: startLine}, true
			case "assert_revert":
				stmt, ok = p.parseAssertRevertStatement()
			default:
				stmt, ok = p.parseStatement()
			}
		} else {
			stmt, ok = p.parseStatement()
		}
		if ok {
			stmts = append(stmts, stmt)
			continue
		}
		p.syncStatement()
	}
	if p.cur.Type == lexer.TokenEOF {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "unexpected EOF while parsing " + what,
			Span:    p.span(p.cur),
		})
		return nil, false
	}
	p.next()
	return stmts, true
}

// parseAssertRevertStatement parses:
//
//	assert_revert { ... }
//	assert_revert("msg") { ... }
//
// The "assert_revert" identifier has already been consumed by parseTestStatementBlock.
func (p *Parser) parseAssertRevertStatement() (ast.Statement, bool) {
	line := p.cur.Start.Line
	p.next() // consume "assert_revert"

	var msgExpr *ast.Expr
	if p.cur.Type == lexer.TokenLParen {
		p.next() // consume '('
		if p.cur.Type != lexer.TokenRParen {
			expr, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenRParen: true})
			if !ok {
				return ast.Statement{}, false
			}
			msgExpr = expr
		}
		if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after assert_revert message") {
			return ast.Statement{}, false
		}
	}

	body, ok := p.parseTestStatementBlock("assert_revert body")
	if !ok {
		return ast.Statement{}, false
	}
	return ast.Statement{
		Kind: "assert_revert",
		Expr: msgExpr,
		Body: body,
		Line: line,
	}, true
}

// parseDeployStatement parses: ContractName(args...) -> binding;
// The "deploy" keyword has already been consumed.
func (p *Parser) parseDeployStatement() (ast.Statement, bool) {
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected contract name after 'deploy'") {
		return ast.Statement{}, false
	}
	args := []*ast.Expr{}
	if p.cur.Type == lexer.TokenLParen {
		p.next()
		if p.cur.Type != lexer.TokenRParen {
			for {
				arg, ok := p.parseExpression(map[lexer.Type]bool{
					lexer.TokenComma:  true,
					lexer.TokenRParen: true,
				})
				if !ok {
					return ast.Statement{}, false
				}
				args = append(args, arg)
				if p.cur.Type == lexer.TokenComma {
					p.next()
					continue
				}
				break
			}
		}
		if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after deploy arguments") {
			return ast.Statement{}, false
		}
	}
	bindingName := ""
	if p.cur.Type == lexer.TokenArrow {
		p.next()
		bindTok := p.cur
		if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected binding name after '->'") {
			return ast.Statement{}, false
		}
		bindingName = bindTok.Literal
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after deploy statement") {
		return ast.Statement{}, false
	}
	expr := &ast.Expr{
		Kind:   "call",
		Callee: &ast.Expr{Kind: "ident", Value: nameTok.Literal},
		Args:   args,
	}
	return ast.Statement{
		Kind: "deploy",
		Name: bindingName,
		Expr: expr,
	}, true
}

// parseWithStatement parses: msg.sender = expr { stmts }
// The "with" keyword has already been consumed.
func (p *Parser) parseWithStatement() (ast.Statement, bool) {
	cond, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenLBrace: true})
	if !ok {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "unexpected EOF while parsing 'with' condition",
			Span:    p.span(p.cur),
		})
		return ast.Statement{}, false
	}
	body, ok := p.parseTestStatementBlock("with body")
	if !ok {
		return ast.Statement{}, false
	}
	return ast.Statement{
		Kind: "with",
		Cond: cond,
		Body: body,
	}, true
}

// parseInterfaceDecl parses:
//
//	interface IName {
//	  fn method(params...) [-> (returns...)] modifiers... ;
// parseImportDecl parses all supported import forms:
//
//	import "path";                    — bare import (side-effect only)
//	import "path" as Alias;          — namespace alias
//	import { A, B } from "path";     — named imports
//	import * as X from "path";       — namespace import (star)
//	import Identifier from "path";   — old TOL-style (still accepted)
// parseDottedName reads a dot-separated identifier sequence and returns the concatenated string.
// Example: "tos" "." "registry" "." "AgentRegistry" → "tolang.registry.AgentRegistry"
// Requires at least one identifier. Stops when next token is not a dot followed by an ident.
func (p *Parser) parseDottedName() string {
	if p.cur.Type != lexer.TokenIdent {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: fmt.Sprintf("expected identifier in dotted name, got '%s'", p.cur.Literal),
			Span:    p.span(p.cur),
		})
		return ""
	}
	result := p.cur.Literal
	p.next()
	for p.cur.Type == lexer.TokenDot {
		next := p.peek()
		if next.Type != lexer.TokenIdent {
			break
		}
		p.next() // consume '.'
		result += "." + p.cur.Literal
		p.next() // consume ident
	}
	return result
}

func (p *Parser) parseImportDecl() *ast.ImportDecl {
	line := p.cur.Start.Line
	if !p.expect(lexer.TokenKwImport, diag.CodeParseUnexpected, "expected 'import'") {
		return nil
	}

	// Form: import "path" [as Alias];
	if p.cur.Type == lexer.TokenString {
		pathTok := p.cur
		p.next()
		path, err := strconv.Unquote(pathTok.Literal)
		if err != nil {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("invalid import path %s", pathTok.Literal),
				Span:    p.span(pathTok),
			})
			return nil
		}
		alias := ""
		if p.cur.Type == lexer.TokenKwAs {
			p.next() // consume 'as'
			if p.cur.Type != lexer.TokenIdent {
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnexpected,
					Message: "expected identifier after 'as'",
					Span:    p.span(p.cur),
				})
				return nil
			}
			alias = p.cur.Literal
			p.next()
		}
		if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after import") {
			return nil
		}
		return &ast.ImportDecl{Path: path, Alias: alias, Line: line}
	}

	// Form: import * as X from "path";
	if p.cur.Type == lexer.TokenStar {
		p.next() // consume '*'
		if p.cur.Type != lexer.TokenKwAs {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected 'as' after '*' in import",
				Span:    p.span(p.cur),
			})
			return nil
		}
		p.next() // consume 'as'
		if p.cur.Type != lexer.TokenIdent {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected identifier after 'as' in import",
				Span:    p.span(p.cur),
			})
			return nil
		}
		alias := p.cur.Literal
		p.next()
		// consume 'from'
		if p.cur.Type != lexer.TokenIdent || p.cur.Literal != "from" {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("expected 'from' after alias in import, got '%s'", p.cur.Literal),
				Span:    p.span(p.cur),
			})
			return nil
		}
		p.next()
		pathTok := p.cur
		if !p.expect(lexer.TokenString, diag.CodeParseUnexpected, "expected string literal after 'from'") {
			return nil
		}
		if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after import") {
			return nil
		}
		path, err := strconv.Unquote(pathTok.Literal)
		if err != nil {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("invalid import path %s", pathTok.Literal),
				Span:    p.span(pathTok),
			})
			return nil
		}
		return &ast.ImportDecl{Path: path, Alias: alias, IsStar: true, Line: line}
	}

	// Form: import { A, B as BB } from "path";
	if p.cur.Type == lexer.TokenLBrace {
		p.next() // consume '{'
		var named []ast.ImportAlias
		for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
			if p.cur.Type != lexer.TokenIdent {
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnexpected,
					Message: fmt.Sprintf("expected identifier in named import list, got '%s'", p.cur.Literal),
					Span:    p.span(p.cur),
				})
				return nil
			}
			symName := p.cur.Literal
			p.next()
			// Optional per-symbol alias: A as AA
			symAlias := ""
			if p.cur.Type == lexer.TokenKwAs {
				p.next() // consume 'as'
				if p.cur.Type != lexer.TokenIdent {
					p.addDiag(diag.Diagnostic{
						Code:    diag.CodeParseUnexpected,
						Message: "expected identifier after 'as' in named import alias",
						Span:    p.span(p.cur),
					})
					return nil
				}
				symAlias = p.cur.Literal
				p.next()
			}
			named = append(named, ast.ImportAlias{Name: symName, Alias: symAlias})
			if p.cur.Type == lexer.TokenComma {
				p.next()
			}
		}
		if !p.expect(lexer.TokenRBrace, diag.CodeParseUnexpected, "expected '}' after named import list") {
			return nil
		}
		// consume 'from'
		if p.cur.Type != lexer.TokenIdent || p.cur.Literal != "from" {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("expected 'from' after named import list, got '%s'", p.cur.Literal),
				Span:    p.span(p.cur),
			})
			return nil
		}
		p.next()
		pathTok := p.cur
		if !p.expect(lexer.TokenString, diag.CodeParseUnexpected, "expected string literal after 'from'") {
			return nil
		}
		if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after import") {
			return nil
		}
		path, err := strconv.Unquote(pathTok.Literal)
		if err != nil {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("invalid import path %s", pathTok.Literal),
				Span:    p.span(pathTok),
			})
			return nil
		}
		return &ast.ImportDecl{Path: path, Named: named, Line: line}
	}

	// Package import form: import tolang.registry.AgentRegistry [as LocalAlias];
	// Detected when an identifier is followed by a dot (dotted path) and no 'from'.
	// Must have at least one dot: "import Foo;" without a dot is ambiguous with old style
	// but we require 'from' for old style, so if no 'from' follows, treat as package import.
	if p.cur.Type == lexer.TokenIdent {
		// Peek: if after the first ident there's a dot, it's a package import.
		// If there's a 'from' keyword, it's old style.
		firstIdent := p.cur.Literal
		savedCur := p.cur
		p.next() // consume first ident
		if p.cur.Type == lexer.TokenDot {
			// Package import path: read remaining dotted segments.
			full := firstIdent
			for p.cur.Type == lexer.TokenDot {
				next := p.peek()
				if next.Type != lexer.TokenIdent {
					break
				}
				p.next() // consume '.'
				full += "." + p.cur.Literal
				p.next() // consume ident
			}
			// Split on last dot: "tolang.registry.AgentRegistry" → pkg="tolang.registry", contract="AgentRegistry"
			idx := strings.LastIndex(full, ".")
			if idx <= 0 {
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnexpected,
					Message: "package import requires a dotted path with at least two segments (e.g. tolang.registry.AgentRegistry)",
					Span:    p.span(savedCur),
				})
				return nil
			}
			pkgPath := full[:idx]
			contractName := full[idx+1:]
			localName := contractName // default: local binding = contract name
			// Optional alias: import tolang.registry.AgentRegistry as IRegistry;
			if p.cur.Type == lexer.TokenKwAs {
				p.next() // consume 'as'
				if p.cur.Type != lexer.TokenIdent {
					p.addDiag(diag.Diagnostic{
						Code:    diag.CodeParseUnexpected,
						Message: "expected identifier after 'as' in package import",
						Span:    p.span(p.cur),
					})
					return nil
				}
				localName = p.cur.Literal
				p.next()
			}
			if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after package import") {
				return nil
			}
			return &ast.ImportDecl{
				Name:            localName,
				Line:            line,
				IsPackageImport: true,
				PackagePath:     pkgPath,
				PackageContract: contractName,
			}
		}
		// Not a dotted path — fall through to old-format handling.
		// Restore state: the first ident is already consumed, we need to check for 'from'.
		// At this point p.cur is whatever came after the first ident.
		if p.cur.Type != lexer.TokenIdent || p.cur.Literal != "from" {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("expected 'from' after import name, got '%s'", p.cur.Literal),
				Span:    p.span(p.cur),
			})
			return nil
		}
		p.next() // consume 'from'
		pathTok := p.cur
		if !p.expect(lexer.TokenString, diag.CodeParseUnexpected, "expected string literal after 'from'") {
			return nil
		}
		if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after import path") {
			return nil
		}
		path, err := strconv.Unquote(pathTok.Literal)
		if err != nil {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("invalid import path %s", pathTok.Literal),
				Span:    p.span(pathTok),
			})
			return nil
		}
		return &ast.ImportDecl{Name: firstIdent, Path: path, Line: line}
	}

	// Unknown import form.
	p.addDiag(diag.Diagnostic{
		Code:    diag.CodeParseUnexpected,
		Message: fmt.Sprintf("expected string, identifier, '{', or '*' after 'import', got '%s'", p.cur.Literal),
		Span:    p.span(p.cur),
	})
	return nil
}

//	  event Evt(params...);
//	  ...
//	}
func (p *Parser) parseInterfaceDecl() *ast.InterfaceDecl {
	if !p.expect(lexer.TokenKwInterface, diag.CodeParseUnexpected, "expected 'interface'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected interface name") {
		return nil
	}
	if !p.expect(lexer.TokenLBrace, diag.CodeParseUnexpected, "expected '{' after interface name") {
		return nil
	}
	iface := &ast.InterfaceDecl{Name: nameTok.Literal}
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		switch p.cur.Type {
		case lexer.TokenKwFunction:
			sig := p.parseFuncSigDecl()
			if sig != nil {
				iface.Functions = append(iface.Functions, *sig)
			}
		case lexer.TokenAt:
			// attribute before function sig
			selectorOverride, ok := p.parseFunctionAttributes()
			if !ok {
				p.syncUnknownMember()
				continue
			}
			if p.cur.Type != lexer.TokenKwFunction {
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnsupported,
					Message: "attributes are currently supported only before function declarations",
					Span:    p.span(p.cur),
				})
				p.syncUnknownMember()
				continue
			}
			sig := p.parseFuncSigDecl()
			if sig != nil {
				if selectorOverride != "" {
					// store selector override in modifiers for now
					sig.Modifiers = append(sig.Modifiers, "@selector("+selectorOverride+")")
				}
				iface.Functions = append(iface.Functions, *sig)
			}
		case lexer.TokenKwEvent:
			ev := p.parseEventDecl()
			if ev != nil {
				iface.Events = append(iface.Events, *ev)
			}
		case lexer.TokenKwError:
			ed := p.parseErrorDecl()
			if ed != nil {
				iface.Errors = append(iface.Errors, *ed)
			}
		case lexer.TokenKwEnum:
			en := p.parseEnumDecl()
			if en != nil {
				iface.Enums = append(iface.Enums, *en)
			}
		case lexer.TokenKwStruct:
			sd := p.parseStructDecl()
			if sd != nil {
				p.structNames[sd.Name] = struct{}{}
				iface.Structs = append(iface.Structs, *sd)
			}
		case lexer.TokenKwType:
			td := p.parseTypeDecl()
			if td != nil {
				iface.TypeDecls = append(iface.TypeDecls, *td)
			}
		default:
			// Handle contextual 'using' keyword.
			if p.cur.Type == lexer.TokenKwUsing || (p.cur.Type == lexer.TokenIdent && p.cur.Literal == "using") {
				ud := p.parseUsingDecl()
				if ud != nil {
					iface.UsingDecls = append(iface.UsingDecls, *ud)
				}
			} else {
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnsupported,
					Message: fmt.Sprintf("unsupported interface member starting at '%s'", p.cur.Literal),
					Span:    p.span(p.cur),
				})
				p.syncUnknownMember()
			}
		}
	}
	if !p.expect(lexer.TokenRBrace, diag.CodeParseUnexpected, "expected '}' to close interface body") {
		return nil
	}
	return iface
}

// parseFuncSigDecl parses a function signature (no body, ends with ';').
// The 'function' keyword has NOT yet been consumed.
// New Solidity order: function name(params) modifiers [returns (...)] ;
func (p *Parser) parseFuncSigDecl() *ast.FuncSigDecl {
	doc := p.takePendingDoc()
	if !p.expect(lexer.TokenKwFunction, diag.CodeParseUnexpected, "expected 'function'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected function name") {
		return nil
	}
	params, ok := p.parseFieldList(false, true)
	if !ok {
		return nil
	}
	// Modifiers come BEFORE returns in Solidity order.
	modifiers := p.parseModifiersUntilSemicolon()
	var returns []ast.FieldDecl
	if p.cur.Type == lexer.TokenKwReturns {
		p.next()
		ret, rok := p.parseFieldList(false, false)
		if !rok {
			return nil
		}
		returns = ret
		// Consume any remaining modifiers after returns (shouldn't normally happen)
		for p.cur.Type != lexer.TokenEOF &&
			p.cur.Type != lexer.TokenLBrace &&
			p.cur.Type != lexer.TokenSemicolon &&
			p.cur.Type != lexer.TokenRBrace {
			modifiers = append(modifiers, p.cur.Literal)
			p.next()
		}
	}
	if p.cur.Type == lexer.TokenSemicolon {
		p.next()
	} else if p.cur.Type == lexer.TokenLBrace {
		// Has a body - skip it
		_ = p.consumeBlock("interface function body")
	}
	var payableAsset string
	var cleanMods []string
	for _, mod := range modifiers {
		if asset, ok := ParsePayableAsset([]string{mod}); ok && asset != "" {
			cleanMods = append(cleanMods, "payable")
			payableAsset = asset
		} else {
			cleanMods = append(cleanMods, mod)
		}
	}
	return &ast.FuncSigDecl{
		Name:         nameTok.Literal,
		Params:       params,
		Returns:      returns,
		Modifiers:    cleanMods,
		PayableAsset: payableAsset,
		Doc:          doc,
	}
}

// parseModifiersUntilSemicolon collects modifier tokens until '{', ';', or 'returns' is seen.
func (p *Parser) parseModifiersUntilSemicolon() []string {
	var mods []string
	for p.cur.Type != lexer.TokenEOF &&
		p.cur.Type != lexer.TokenLBrace &&
		p.cur.Type != lexer.TokenSemicolon &&
		p.cur.Type != lexer.TokenRBrace &&
		p.cur.Type != lexer.TokenKwReturns {
		// Handle payable(asset) syntax.
		if p.cur.Type == lexer.TokenKwPayable && p.peek().Type == lexer.TokenLParen {
			p.next() // consume 'payable'
			p.next() // consume '('
			if p.cur.Type == lexer.TokenIdent || p.cur.Type == lexer.TokenKwUno {
				asset := p.cur.Literal
				p.next() // consume asset name
				if p.cur.Type == lexer.TokenRParen {
					p.next() // consume ')'
				}
				mods = append(mods, "payable("+asset+")")
			} else {
				if p.cur.Type == lexer.TokenRParen {
					p.next()
				}
				mods = append(mods, "payable")
			}
			continue
		}
		mods = append(mods, p.cur.Literal)
		p.next()
	}
	return mods
}

// syncUntilAfterSemicolon advances past the next ';'.
func (p *Parser) syncUntilAfterSemicolon() {
	for p.cur.Type != lexer.TokenSemicolon && p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		p.next()
	}
	if p.cur.Type == lexer.TokenSemicolon {
		p.next()
	}
}

func (p *Parser) parseSkippedTopDecl(mod *ast.Module) {
	kind := p.cur.Literal
	p.next()

	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, fmt.Sprintf("expected %s name", kind)) {
		return
	}

	if !p.consumeBlock(kind + " body") {
		return
	}

	mod.SkippedTopDecls = append(mod.SkippedTopDecls, ast.SkippedTopDecl{
		Kind: kind,
		Name: nameTok.Literal,
	})
}

// parseLibraryDecl parses:
//
//	library LibName {
//	  fn method(params...) [-> (returns...)] modifiers... { ... }
//	  ...
//	}
func (p *Parser) parseLibraryDecl() *ast.LibraryDecl {
	if !p.expect(lexer.TokenKwLibrary, diag.CodeParseUnexpected, "expected 'library'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected library name") {
		return nil
	}
	if !p.expect(lexer.TokenLBrace, diag.CodeParseUnexpected, "expected '{' after library name") {
		return nil
	}
	lib := &ast.LibraryDecl{Name: nameTok.Literal}
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		switch p.cur.Type {
		case lexer.TokenAt:
			selectorOverride, ok := p.parseFunctionAttributes()
			if !ok {
				p.syncUnknownMember()
				continue
			}
			if p.cur.Type != lexer.TokenKwFunction {
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnsupported,
					Message: "attributes are currently supported only before function declarations in library",
					Span:    p.span(p.cur),
				})
				p.syncUnknownMember()
				continue
			}
			fn := p.parseFunctionDecl(selectorOverride)
			if fn != nil {
				lib.Functions = append(lib.Functions, *fn)
			}
		case lexer.TokenKwFunction:
			fn := p.parseFunctionDecl("")
			if fn != nil {
				lib.Functions = append(lib.Functions, *fn)
			}
		case lexer.TokenKwEvent:
			ev := p.parseEventDecl()
			if ev != nil {
				lib.Events = append(lib.Events, *ev)
			}
		case lexer.TokenKwError:
			ed := p.parseErrorDecl()
			if ed != nil {
				lib.Errors = append(lib.Errors, *ed)
			}
		case lexer.TokenKwEnum:
			en := p.parseEnumDecl()
			if en != nil {
				lib.Enums = append(lib.Enums, *en)
			}
		case lexer.TokenKwStruct:
			sd := p.parseStructDecl()
			if sd != nil {
				p.structNames[sd.Name] = struct{}{}
				lib.Structs = append(lib.Structs, *sd)
			}
		case lexer.TokenKwType:
			td := p.parseTypeDecl()
			if td != nil {
				lib.TypeDecls = append(lib.TypeDecls, *td)
			}
		case lexer.TokenKwConstant:
			cd := p.parseConstantDecl()
			if cd != nil {
				lib.Constants = append(lib.Constants, *cd)
			}
		default:
			// Handle contextual 'using' keyword and type-first constant.
			if p.cur.Type == lexer.TokenKwUsing || (p.cur.Type == lexer.TokenIdent && p.cur.Literal == "using") {
				ud := p.parseUsingDecl()
				if ud != nil {
					lib.UsingDecls = append(lib.UsingDecls, *ud)
				}
			} else if p.isTypeStart() {
				// Type-first constant: uint256 constant MAX = 100;
				nxt := p.peekTok()
				if nxt.Type == lexer.TokenKwConstant {
					cd := p.parseConstantDeclTypeFirst()
					if cd != nil {
						lib.Constants = append(lib.Constants, *cd)
					}
				} else {
					p.addDiag(diag.Diagnostic{
						Code:    diag.CodeParseUnsupported,
						Message: fmt.Sprintf("unsupported library member starting at token '%s'; libraries do not support state variables", p.cur.Literal),
						Span:    p.span(p.cur),
					})
					p.syncUnknownMember()
				}
			} else {
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnsupported,
					Message: fmt.Sprintf("unsupported library member starting at token '%s'", p.cur.Literal),
					Span:    p.span(p.cur),
				})
				p.syncUnknownMember()
			}
		}
	}
	if !p.expect(lexer.TokenRBrace, diag.CodeParseUnexpected, "expected '}' to close library body") {
		return nil
	}
	return lib
}

// parseUsingDecl parses several forms of using directive:
//
//	using LibName for Type;
//	using LibName for *;
//	using LibName for Type global;
//	using { FuncA, FuncB } for Type;
//	using { add as + } for uint256;
//
// The "using" keyword (TokenKwUsing) has already been identified but NOT yet consumed.
func (p *Parser) parseUsingDecl() *ast.UsingDecl {
	p.next() // consume "using"

	var libName string

	// 10.1: braced list form: using { FuncA, FuncB as op } for Type;
	if p.cur.Type == lexer.TokenLBrace {
		p.next() // consume '{'
		var parts []string
		for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
			if !isIdentLike(p.cur.Type) {
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnexpected,
					Message: fmt.Sprintf("expected function name in 'using { ... }', got '%s'", p.cur.Literal),
					Span:    p.span(p.cur),
				})
				p.syncUntilAfterSemicolon()
				return nil
			}
			fnName := p.cur.Literal
			p.next()
			// 10.4: optional "as operator" alias
			if p.cur.Type == lexer.TokenKwAs {
				p.next() // consume "as"
				// operator token — accept any single token as the operator
				if p.cur.Type != lexer.TokenEOF {
					fnName = fnName + " as " + p.cur.Literal
					p.next()
				}
			}
			parts = append(parts, fnName)
			if p.cur.Type == lexer.TokenComma {
				p.next()
			}
		}
		if p.cur.Type == lexer.TokenRBrace {
			p.next() // consume '}'
		}
		libName = "{" + strings.Join(parts, ", ") + "}"
	} else {
		// Simple form: using LibName for Type;
		libTok := p.cur
		if !isIdentLike(p.cur.Type) {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected library name after 'using'",
				Span:    p.span(p.cur),
			})
			p.syncUntilAfterSemicolon()
			return nil
		}
		libName = libTok.Literal
		p.next()
	}

	// expect "for" keyword
	if p.cur.Type != lexer.TokenKwFor {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected 'for' in 'using LibName for Type'",
			Span:    p.span(p.cur),
		})
		p.syncUntilAfterSemicolon()
		return nil
	}
	p.next() // consume "for"

	// 10.2: wildcard type: using LibName for *;
	var typ string
	if p.cur.Type == lexer.TokenStar {
		typ = "*"
		p.next()
	} else {
		typ = p.parseTypeUntil(map[lexer.Type]bool{
			lexer.TokenSemicolon: true,
			lexer.TokenKwGlobal:  true,
		})
		if typ == "" {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected type in 'using LibName for Type'",
				Span:    p.span(p.cur),
			})
			p.syncUntilAfterSemicolon()
			return nil
		}
	}

	// 10.3: optional "global" modifier
	if p.cur.Type == lexer.TokenKwGlobal {
		p.next() // consume "global"
	}

	if p.cur.Type == lexer.TokenSemicolon {
		p.next()
	}
	return &ast.UsingDecl{Library: libName, Type: typ}
}

func (p *Parser) parseContractMember(contract *ast.ContractDecl) {
	// Handle 'using' keyword (now promoted to TokenKwUsing).
	if p.cur.Type == lexer.TokenKwUsing {
		ud := p.parseUsingDecl()
		if ud != nil {
			contract.UsingDecls = append(contract.UsingDecls, *ud)
		}
		return
	}

	// Handle 'struct' keyword inside contract.
	if p.cur.Type == lexer.TokenKwStruct {
		sd := p.parseStructDecl()
		if sd != nil {
			p.structNames[sd.Name] = struct{}{}
			contract.Structs = append(contract.Structs, *sd)
		}
		return
	}

	// Handle 'type' keyword inside contract: type X is Y; (UDVT)
	if p.cur.Type == lexer.TokenKwType {
		td := p.parseTypeDecl()
		if td != nil {
			contract.TypeDecls = append(contract.TypeDecls, *td)
		}
		return
	}

	// Handle 'receive' keyword (now promoted to TokenKwReceive): receive() payable { body }
	if p.cur.Type == lexer.TokenKwReceive || (p.cur.Type == lexer.TokenIdent && p.cur.Literal == "receive") {
		rc := p.parseReceiveDecl()
		if rc == nil {
			return
		}
		if contract.Receive != nil {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnsupported,
				Message: "multiple receive functions are not supported",
				Span:    p.span(p.cur),
			})
			return
		}
		contract.Receive = rc
		return
	}

	if p.cur.Type == lexer.TokenAt {
		selectorOverride, ok := p.parseFunctionAttributes()
		if !ok {
			return
		}
		if p.cur.Type != lexer.TokenKwFunction {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnsupported,
				Message: "attributes are currently supported only before function declarations",
				Span:    p.span(p.cur),
			})
			p.syncUnknownMember()
			return
		}
		fn := p.parseFunctionDecl(selectorOverride)
		if fn != nil {
			contract.Functions = append(contract.Functions, *fn)
		}
		return
	}

	// Handle agent-native contextual keywords: capability, purpose, manifest, agent.
	if p.cur.Type == lexer.TokenIdent || p.cur.Type == lexer.TokenKwAgent {
		switch p.cur.Literal {
		case "capability":
			cd := p.parseCapabilityDecl()
			if cd != nil {
				for _, existing := range contract.Capabilities {
					if existing.Name == cd.Name {
						p.addDiag(diag.Diagnostic{
							Code:    diag.CodeAgentCapabilityDup,
							Message: fmt.Sprintf("capability '%s' already declared in this contract", cd.Name),
							Span:    diag.Span{File: p.filename, Start: diag.Position{Line: cd.Line}},
						})
						return
					}
				}
				contract.Capabilities = append(contract.Capabilities, *cd)
			}
			return
		case "purpose":
			pd := p.parsePurposeDecl()
			if pd != nil {
				for _, existing := range contract.Purposes {
					if existing.Name == pd.Name {
						p.addDiag(diag.Diagnostic{
							Code:    diag.CodeAgentPurposeDup,
							Message: fmt.Sprintf("purpose '%s' already declared in this contract", pd.Name),
							Span:    diag.Span{File: p.filename, Start: diag.Position{Line: pd.Line}},
						})
						return
					}
				}
				contract.Purposes = append(contract.Purposes, *pd)
			}
			return
		case "manifest":
			md := p.parseManifestDecl()
			if md != nil {
				if contract.Manifest != nil {
					p.addDiag(diag.Diagnostic{
						Code:    diag.CodeAgentManifestDup,
						Message: "manifest block already declared in this contract",
						Span:    diag.Span{File: p.filename, Start: diag.Position{Line: md.Line}},
					})
				} else {
					contract.Manifest = md
				}
			}
			return
		case "agent":
			slot := p.parseAgentTypeSlot()
			if slot != nil {
				if contract.Storage == nil {
					contract.Storage = &ast.StorageDecl{}
				}
				contract.Storage.Slots = append(contract.Storage.Slots, *slot)
			}
			return
		}
	}

	switch p.cur.Type {
	case lexer.TokenKwTransient:
		// transient Type name; — EIP-1153 transient state variable at contract body level
		slot := p.parseStorageSlot()
		if slot != nil {
			if contract.Storage == nil {
				contract.Storage = &ast.StorageDecl{}
			}
			contract.Storage.Slots = append(contract.Storage.Slots, *slot)
		}
	case lexer.TokenKwEvent:
		ev := p.parseEventDecl()
		if ev != nil {
			contract.Events = append(contract.Events, *ev)
		}
	case lexer.TokenKwFunction:
		fn := p.parseFunctionDecl("")
		if fn != nil {
			contract.Functions = append(contract.Functions, *fn)
		}
	case lexer.TokenKwConstructor:
		ctor := p.parseConstructorDecl()
		if ctor == nil {
			return
		}
		if contract.Constructor != nil {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnsupported,
				Message: "multiple constructors are not supported",
				Span:    p.span(p.cur),
			})
			return
		}
		contract.Constructor = ctor
	case lexer.TokenKwFallback:
		fb := p.parseFallbackDecl()
		if fb == nil {
			return
		}
		if contract.Fallback != nil {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnsupported,
				Message: "multiple fallbacks are not supported",
				Span:    p.span(p.cur),
			})
			return
		}
		contract.Fallback = fb
	case lexer.TokenKwError:
		ed := p.parseErrorDecl()
		if ed != nil {
			contract.Errors = append(contract.Errors, *ed)
		}
	case lexer.TokenKwEnum:
		en := p.parseEnumDecl()
		if en != nil {
			contract.Enums = append(contract.Enums, *en)
		}
	case lexer.TokenKwModifier:
		md := p.parseModifierDecl()
		if md != nil {
			contract.Modifiers = append(contract.Modifiers, *md)
		}
	case lexer.TokenKwImmutable:
		imm := p.parseImmutableDecl()
		if imm != nil {
			contract.Immutables = append(contract.Immutables, *imm)
		}
	case lexer.TokenKwConstant:
		cd := p.parseConstantDecl()
		if cd != nil {
			contract.Constants = append(contract.Constants, *cd)
		}
	default:
		if p.isTypeStart() {
			// Peek ahead to detect type-first constant/immutable syntax:
			//   uint256 constant MAX = 1000000;
			//   uint256 immutable owner;
			nxt := p.peekTok()
			if nxt.Type == lexer.TokenKwConstant {
				cd := p.parseConstantDeclTypeFirst()
				if cd != nil {
					contract.Constants = append(contract.Constants, *cd)
				}
				return
			}
			if nxt.Type == lexer.TokenKwImmutable {
				imm := p.parseImmutableDeclTypeFirst()
				if imm != nil {
					contract.Immutables = append(contract.Immutables, *imm)
				}
				return
			}
			// Solidity-style state variable: Type name;
			slot := p.parseStorageSlot()
			if slot != nil {
				if contract.Storage == nil {
					contract.Storage = &ast.StorageDecl{}
				}
				contract.Storage.Slots = append(contract.Storage.Slots, *slot)
			}
			return
		}
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnsupported,
			Message: fmt.Sprintf("unsupported contract member starting at token '%s'", p.cur.Literal),
			Span:    p.span(p.cur),
		})
		p.syncUnknownMember()
	}
}

// collectAttrArgs collects the raw text of tokens between '(' and ')' (already past the '(').
// Returns the collected string and advances past ')'.
func (p *Parser) collectAttrArgs() string {
	var parts []string
	depth := 1
	for p.cur.Type != lexer.TokenEOF {
		if p.cur.Type == lexer.TokenLParen {
			depth++
		} else if p.cur.Type == lexer.TokenRParen {
			depth--
			if depth == 0 {
				p.next() // consume ')'
				break
			}
		}
		parts = append(parts, p.cur.Literal)
		p.next()
	}
	return strings.Join(parts, "")
}

func (p *Parser) parseFunctionAttributes() (string, bool) {
	selectorOverride := ""
	for p.cur.Type == lexer.TokenAt {
		if !p.expect(lexer.TokenAt, diag.CodeParseUnexpected, "expected '@'") {
			return "", false
		}
		attrName := p.cur
		if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected attribute name after '@'") {
			return "", false
		}
		// Check if attribute has parens or is bare.
		hasParen := p.cur.Type == lexer.TokenLParen
		if hasParen {
			p.next() // consume '('
		}

		switch attrName.Literal {
		case "selector":
			if !hasParen {
				break // @selector without parens is a no-op
			}
			if p.cur.Type != lexer.TokenString {
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnexpected,
					Message: "expected selector string literal",
					Span:    p.span(p.cur),
				})
				p.collectAttrArgs()
				continue
			}
			val, err := strconv.Unquote(p.cur.Literal)
			if err != nil {
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnexpected,
					Message: "invalid selector string literal",
					Span:    p.span(p.cur),
				})
			} else {
				if selectorOverride != "" {
					p.addDiag(diag.Diagnostic{
						Code:    diag.CodeParseUnsupported,
						Message: "duplicate @selector attribute",
						Span:    p.span(attrName),
					})
				}
				selectorOverride = val
			}
			p.next()
			// consume remaining to ')'
			for p.cur.Type != lexer.TokenRParen && p.cur.Type != lexer.TokenEOF {
				p.next()
			}
			if p.cur.Type == lexer.TokenRParen {
				p.next()
			}
			continue
		case "requires":
			// @requires(caller: CapName) — add to pendingDoc.
			if hasParen {
				rawArgs := p.collectAttrArgs()
				// Parse: "caller:CapName" or "caller: CapName"
				rawArgs = strings.TrimSpace(rawArgs)
				if strings.HasPrefix(rawArgs, "caller") {
					rest := strings.TrimSpace(strings.TrimPrefix(rawArgs, "caller"))
					rest = strings.TrimPrefix(rest, ":")
					rest = strings.TrimPrefix(rest, "=")
					capName := strings.TrimSpace(rest)
					if capName != "" {
						if p.pendingDoc == nil {
							p.pendingDoc = &ast.DocMeta{}
						}
						p.pendingDoc.RequiresCap = append(p.pendingDoc.RequiresCap, capName)
					}
				}
			}
			continue
		case "pay":
			// @pay(amount_expr) or @pay(amount=expr, recipient=expr)
			if hasParen {
				rawPay := p.collectAttrArgs()
				if p.pendingDoc == nil {
					p.pendingDoc = &ast.DocMeta{}
				}
				parsePayTag(p.pendingDoc, rawPay)
			}
			continue
		case "verifiable":
			if p.pendingDoc == nil {
				p.pendingDoc = &ast.DocMeta{}
			}
			p.pendingDoc.Verifiable = true
			if hasParen {
				p.collectAttrArgs()
			}
			continue
		case "delegated":
			if p.pendingDoc == nil {
				p.pendingDoc = &ast.DocMeta{}
			}
			p.pendingDoc.Delegated = true
			if hasParen {
				p.collectAttrArgs()
			}
			continue
		case "quota":
			// @quota(calls: N, price: M)
			if hasParen {
				rawQuota := p.collectAttrArgs()
				if p.pendingDoc == nil {
					p.pendingDoc = &ast.DocMeta{}
				}
				parseQuotaTag(p.pendingDoc, rawQuota)
			}
			continue
		case "total_cost":
			// @total_cost(max: N)
			if hasParen {
				rawTC := p.collectAttrArgs()
				if p.pendingDoc == nil {
					p.pendingDoc = &ast.DocMeta{}
				}
				parseTotalCostTag(p.pendingDoc, rawTC)
			}
			continue
		default:
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnsupported,
				Message: fmt.Sprintf("unsupported attribute '@%s'", attrName.Literal),
				Span:    p.span(attrName),
			})
			if hasParen {
				p.collectAttrArgs()
			}
			continue
		}
		if hasParen {
			if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after attribute") {
				return "", false
			}
		}
	}
	return selectorOverride, true
}

// parseErrorDecl parses a custom error declaration:
//   error ErrorName(param1: Type1, param2: Type2);
func (p *Parser) parseErrorDecl() *ast.ErrorDecl {
	if !p.expect(lexer.TokenKwError, diag.CodeParseUnexpected, "expected 'error'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected error name") {
		return nil
	}
	params, ok := p.parseFieldList(false, false)
	if !ok {
		return nil
	}
	if p.cur.Type == lexer.TokenSemicolon {
		p.next()
	} else {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected ';' after error declaration",
			Span:    p.span(p.cur),
		})
	}
	return &ast.ErrorDecl{
		Name:   nameTok.Literal,
		Params: params,
	}
}

// parseEnumDecl parses an enum declaration:
//   enum State { Active, Inactive, Paused }
func (p *Parser) parseEnumDecl() *ast.EnumDecl {
	if !p.expect(lexer.TokenKwEnum, diag.CodeParseUnexpected, "expected 'enum'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected enum name") {
		return nil
	}
	if !p.expect(lexer.TokenLBrace, diag.CodeParseUnexpected, "expected '{' after enum name") {
		return nil
	}
	var members []string
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		if p.cur.Type != lexer.TokenIdent {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("expected enum member name, got '%s'", p.cur.Literal),
				Span:    p.span(p.cur),
			})
			p.syncUntil(lexer.TokenComma, lexer.TokenRBrace, lexer.TokenEOF)
		} else {
			members = append(members, p.cur.Literal)
			p.next()
		}
		if p.cur.Type == lexer.TokenComma {
			p.next()
			continue
		}
		break
	}
	if !p.expect(lexer.TokenRBrace, diag.CodeParseUnexpected, "expected '}' to close enum body") {
		return nil
	}
	return &ast.EnumDecl{
		Name:    nameTok.Literal,
		Members: members,
	}
}

// parseStructDecl parses a struct declaration:
//
//	struct Foo { u256 x; address y; }
//
// The "struct" keyword has NOT yet been consumed.
func (p *Parser) parseStructDecl() *ast.StructDecl {
	if !p.expect(lexer.TokenKwStruct, diag.CodeParseUnexpected, "expected 'struct'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected struct name") {
		return nil
	}
	if !p.expect(lexer.TokenLBrace, diag.CodeParseUnexpected, "expected '{' after struct name") {
		return nil
	}
	var fields []ast.FieldDecl
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		// New Solidity format: Type fieldName;
		// Collect type tokens until we see a plain ident at depth 0 (field name) or ';'/'}'
		var typeTokens []string
		depthParen, depthBracket := 0, 0
		for p.cur.Type != lexer.TokenEOF && p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenSemicolon {
			if depthParen == 0 && depthBracket == 0 {
				if len(typeTokens) > 0 && p.cur.Type == lexer.TokenIdent {
					break // field name follows
				}
			}
			switch p.cur.Type {
			case lexer.TokenLParen:
				depthParen++
			case lexer.TokenRParen:
				if depthParen > 0 {
					depthParen--
				}
			case lexer.TokenLBracket:
				depthBracket++
			case lexer.TokenRBracket:
				if depthBracket > 0 {
					depthBracket--
				}
			}
			typeTokens = append(typeTokens, p.cur.Literal)
			p.next()
		}
		typ := normalizeGenericTypeString(joinTypeTokens(typeTokens))
		if typ == "" {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("expected field type in struct, got '%s'", p.cur.Literal),
				Span:    p.span(p.cur),
			})
			p.syncUntil(lexer.TokenSemicolon, lexer.TokenRBrace, lexer.TokenEOF)
			if p.cur.Type == lexer.TokenSemicolon {
				p.next()
			}
			continue
		}
		// Field name must be an identifier
		if p.cur.Type != lexer.TokenIdent {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("expected field name after type '%s' in struct", typ),
				Span:    p.span(p.cur),
			})
			p.syncUntil(lexer.TokenSemicolon, lexer.TokenRBrace, lexer.TokenEOF)
			if p.cur.Type == lexer.TokenSemicolon {
				p.next()
			}
			continue
		}
		fieldName := p.cur.Literal
		p.next() // consume field name
		if p.cur.Type == lexer.TokenSemicolon {
			p.next()
		}
		fields = append(fields, ast.FieldDecl{Name: fieldName, Type: typ})
	}
	if !p.expect(lexer.TokenRBrace, diag.CodeParseUnexpected, "expected '}' to close struct body") {
		return nil
	}
	return &ast.StructDecl{
		Name:   nameTok.Literal,
		Fields: fields,
	}
}

// parseTypeDecl parses a user-defined value type declaration:
//
//	type MyInt is uint256;
//
// The 'type' keyword has not been consumed yet.
func (p *Parser) parseTypeDecl() *ast.TypeDecl {
	if !p.expect(lexer.TokenKwType, diag.CodeParseUnexpected, "expected 'type'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected type name after 'type'") {
		return nil
	}
	// Expect "is" (now TokenKwIs).
	if p.cur.Type != lexer.TokenKwIs {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: fmt.Sprintf("expected 'is' after type name, got '%s'", p.cur.Literal),
			Span:    p.span(p.cur),
		})
		return nil
	}
	p.next() // consume "is"
	// Parse the underlying type (collect until ';')
	underlying := p.parseTypeUntil(map[lexer.Type]bool{lexer.TokenSemicolon: true})
	if underlying == "" {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected underlying type after 'is'",
			Span:    p.span(p.cur),
		})
		return nil
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after type declaration") {
		return nil
	}
	return &ast.TypeDecl{Name: nameTok.Literal, Underlying: underlying}
}

func (p *Parser) parseSkippedContractDecl(contract *ast.ContractDecl, kind string) {
	p.next() // skip keyword

	name := "<anonymous>"
	if p.cur.Type == lexer.TokenIdent {
		name = p.cur.Literal
		p.next()
	}

	if p.cur.Type == lexer.TokenLParen {
		if !p.consumePaired(lexer.TokenLParen, lexer.TokenRParen, kind+" parameter list") {
			return
		}
	}

	if p.cur.Type == lexer.TokenLBrace {
		_ = p.consumeBlock(kind + " body")
	} else if p.cur.Type == lexer.TokenSemicolon {
		p.next()
	} else {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected '{' or ';' after " + kind + " declaration",
			Span:    p.span(p.cur),
		})
	}

	contract.SkippedDecls = append(contract.SkippedDecls, ast.SkippedContractDecl{
		Kind: kind,
		Name: name,
	})
}

func (p *Parser) parseModifierDecl() *ast.ModifierDecl {
	if !p.expect(lexer.TokenKwModifier, diag.CodeParseUnexpected, "expected 'modifier'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected modifier name") {
		return nil
	}

	// 11.1: optional parameter list (modifier may omit the () entirely).
	var params []ast.FieldDecl
	if p.cur.Type == lexer.TokenLParen {
		var ok bool
		params, ok = p.parseFieldList(false, false)
		if !ok {
			return nil
		}
	}

	// 11.2: optional virtual / override after param list.
	isVirtual := false
	isOverride := false
	for p.cur.Type == lexer.TokenKwVirtual || p.cur.Type == lexer.TokenKwOverride {
		if p.cur.Type == lexer.TokenKwVirtual {
			isVirtual = true
		} else {
			isOverride = true
			// optional overrideSpecifier: override(Base1, Base2)
			if p.peek().Type == lexer.TokenLParen {
				p.next()
				_ = p.consumePaired(lexer.TokenLParen, lexer.TokenRParen, "override specifier")
				continue
			}
		}
		p.next()
	}

	// 11.3: abstract modifier body: semicolon instead of block.
	if p.cur.Type == lexer.TokenSemicolon {
		p.next()
		return &ast.ModifierDecl{
			Name:     nameTok.Literal,
			Params:   params,
			Virtual:  isVirtual,
			Override: isOverride,
			Abstract: true,
			Body:     nil,
		}
	}

	body, ok := p.parseModifierBody()
	if !ok {
		return nil
	}
	return &ast.ModifierDecl{
		Name:     nameTok.Literal,
		Params:   params,
		Virtual:  isVirtual,
		Override: isOverride,
		Abstract: false,
		Body:     body,
	}
}

// parseModifierBody parses the { ... } block of a modifier declaration.
// Inside a modifier body, _; (underscore semicolon) is parsed as a "placeholder" statement.
func (p *Parser) parseModifierBody() ([]ast.Statement, bool) {
	if p.cur.Type != lexer.TokenLBrace {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected '{' before modifier body",
			Span:    p.span(p.cur),
		})
		return nil, false
	}
	p.next()

	stmts := []ast.Statement{}
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		// Check for _; (placeholder statement)
		if p.cur.Type == lexer.TokenIdent && p.cur.Literal == "_" {
			line := p.cur.Start.Line
			p.next()
			if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after '_' in modifier body") {
				p.syncStatement()
				continue
			}
			stmts = append(stmts, ast.Statement{Kind: "placeholder", Line: line})
			continue
		}
		stmt, ok := p.parseStatement()
		if ok {
			stmts = append(stmts, stmt)
			continue
		}
		p.syncStatement()
	}
	if p.cur.Type == lexer.TokenEOF {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "unexpected EOF while parsing modifier body",
			Span:    p.span(p.cur),
		})
		return nil, false
	}
	p.next()
	return stmts, true
}

// isStateVariableModifier returns true if the identifier is a state variable modifier.
func isStateVariableModifier(lit string) bool {
	switch lit {
	case "public", "private", "internal", "override":
		return true
	}
	return false
}

// isStateVariableModifierToken returns true if the token type is a promoted state variable modifier keyword.
func isStateVariableModifierToken(t lexer.Type) bool {
	switch t {
	case lexer.TokenKwPublic, lexer.TokenKwPrivate, lexer.TokenKwInternal, lexer.TokenKwOverride:
		return true
	}
	return false
}

// parseStorageSlot parses a single storage variable declaration:
//
//	[transient] Type [public|private|internal] [override] name [= expr] ;
//
// Example:
//
//	u256 total_supply;
//	mapping(address => u256) balances;
//	transient u256 lock_status;
//	uint256 public totalSupply;
//	uint256 public override balance;
//	uint256 x = 1;
func (p *Parser) parseStorageSlot() *ast.StorageSlot {
	isTransient := false
	if p.cur.Type == lexer.TokenKwTransient {
		isTransient = true
		p.next()
	}
	// Use parseField to parse "Type name" — it stops at the first plain ident after type tokens.
	// The "name" returned by parseField may actually be a visibility/override modifier.
	field, ok := p.parseField(false, false)
	if !ok {
		p.syncUntil(lexer.TokenSemicolon, lexer.TokenRBrace, lexer.TokenEOF)
		if p.cur.Type == lexer.TokenSemicolon {
			p.next()
		}
		return nil
	}

	// Collect optional state variable modifiers: public, private, internal, override.
	// Since these are now promoted keywords, parseField may have absorbed them into field.Type
	// as ident-like tokens (e.g. "uint256 public"), or returned them as field.Name (legacy),
	// or the current token may be a modifier keyword after a complete type+name parse failed.
	visibility := ""
	isOverride := false

	// extractModifierFromType checks if the field type string ends with a visibility/override
	// modifier that was absorbed into the type during parsing (because the keywords are
	// isIdentLike). If so, strip the modifier from the type and record it.
	extractModifierFromType := func() {
		for {
			t := field.Type
			stripped := false
			for _, mod := range []string{"public", "private", "internal", "override"} {
				suffix := " " + mod
				if len(t) > len(suffix) && t[len(t)-len(suffix):] == suffix {
					field.Type = t[:len(t)-len(suffix)]
					switch mod {
					case "public", "private", "internal":
						visibility = mod
					case "override":
						isOverride = true
					}
					stripped = true
					break
				}
			}
			if !stripped {
				break
			}
		}
	}

	// Helper to consume a modifier token (promoted keyword or ident).
	consumeModifierToken := func() bool {
		switch p.cur.Type {
		case lexer.TokenKwPublic:
			visibility = "public"
			p.next()
			return true
		case lexer.TokenKwPrivate:
			visibility = "private"
			p.next()
			return true
		case lexer.TokenKwInternal:
			visibility = "internal"
			p.next()
			return true
		case lexer.TokenKwOverride:
			isOverride = true
			p.next()
			return true
		}
		if p.cur.Type == lexer.TokenIdent && isStateVariableModifier(p.cur.Literal) {
			switch p.cur.Literal {
			case "public", "private", "internal":
				visibility = p.cur.Literal
			case "override":
				isOverride = true
			}
			p.next()
			return true
		}
		return false
	}

	// Check if what parseField returned as "name" is actually a modifier (legacy path).
	if isStateVariableModifier(field.Name) {
		switch field.Name {
		case "public", "private", "internal":
			visibility = field.Name
		case "override":
			isOverride = true
		}
		// Continue consuming remaining modifiers from the stream (promoted or contextual).
		for isStateVariableModifierToken(p.cur.Type) || (p.cur.Type == lexer.TokenIdent && isStateVariableModifier(p.cur.Literal)) {
			consumeModifierToken()
		}
		// Now parse the actual variable name.
		if p.cur.Type != lexer.TokenIdent {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected variable name after storage variable modifiers",
				Span:    p.span(p.cur),
			})
			p.syncUntil(lexer.TokenSemicolon, lexer.TokenRBrace, lexer.TokenEOF)
			if p.cur.Type == lexer.TokenSemicolon {
				p.next()
			}
			return nil
		}
		field.Name = p.cur.Literal
		p.next()
	} else if field.Name != "" {
		// parseField returned a name but may have absorbed visibility modifiers into
		// the type string (because TokenKwPublic etc. are isIdentLike).
		// Extract any trailing visibility modifiers from the type.
		extractModifierFromType()
	} else if isStateVariableModifierToken(p.cur.Type) || (p.cur.Type == lexer.TokenIdent && isStateVariableModifier(p.cur.Literal)) {
		// parseField stopped before consuming a name because the next token is a
		// promoted state variable modifier keyword (e.g. TokenKwPublic). Consume
		// modifiers, then read the actual variable name.
		for isStateVariableModifierToken(p.cur.Type) || (p.cur.Type == lexer.TokenIdent && isStateVariableModifier(p.cur.Literal)) {
			consumeModifierToken()
		}
		if p.cur.Type != lexer.TokenIdent {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected variable name after storage variable modifiers",
				Span:    p.span(p.cur),
			})
			p.syncUntil(lexer.TokenSemicolon, lexer.TokenRBrace, lexer.TokenEOF)
			if p.cur.Type == lexer.TokenSemicolon {
				p.next()
			}
			return nil
		}
		field.Name = p.cur.Literal
		p.next()
	} else {
		// field.Name == "": parseField stopped before consuming a name (no modifier either).
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected variable name after type in storage declaration",
			Span:    p.span(p.cur),
		})
		p.syncUntil(lexer.TokenSemicolon, lexer.TokenRBrace, lexer.TokenEOF)
		if p.cur.Type == lexer.TokenSemicolon {
			p.next()
		}
		return nil
	}

	// Optional inline initializer: = expression
	var initExpr *ast.Expr
	if p.cur.Type == lexer.TokenAssign {
		p.next() // consume '='
		expr, exprOk := p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
		if !exprOk || expr == nil {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected expression after '=' in storage variable initializer",
				Span:    p.span(p.cur),
			})
		} else {
			initExpr = expr
		}
	}

	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after storage variable declaration") {
		return nil
	}
	return &ast.StorageSlot{
		Name:        field.Name,
		Type:        field.Type,
		IsTransient: isTransient,
		Visibility:  visibility,
		Override:    isOverride,
		InitExpr:    initExpr,
	}
}

func (p *Parser) parseEventDecl() *ast.EventDecl {
	if !p.expect(lexer.TokenKwEvent, diag.CodeParseUnexpected, "expected 'event'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected event name") {
		return nil
	}
	params, ok := p.parseFieldList(true, false)
	if !ok {
		return nil
	}
	// 9.1: optional anonymous modifier after ')'.
	anonymous := false
	if p.cur.Type == lexer.TokenKwAnonymous {
		anonymous = true
		p.next()
	}
	if p.cur.Type == lexer.TokenSemicolon {
		p.next()
	}
	ev := &ast.EventDecl{
		Name:      nameTok.Literal,
		Params:    params,
		Anonymous: anonymous,
	}
	return ev
}

func (p *Parser) parseFunctionDecl(selectorOverride string) *ast.FunctionDecl {
	doc := p.takePendingDoc()
	if !p.expect(lexer.TokenKwFunction, diag.CodeParseUnexpected, "expected 'function'") {
		return nil
	}
	nameTok := p.cur
	// Allow ident-like keywords (e.g. "deploy", "new") as function names.
	if isIdentLike(p.cur.Type) {
		p.next()
	} else if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected function name") {
		return nil
	}

	params, ok := p.parseFieldList(false, true)
	if !ok {
		return nil
	}

	// Solidity order: modifiers come BEFORE returns.
	rawMods := p.parseModifiersUntilBlock()

	// Optional returns clause (after modifiers, before body).
	var returns []ast.FieldDecl
	if p.cur.Type == lexer.TokenKwReturns {
		p.next()
		ret, rok := p.parseFieldList(false, false)
		if !rok {
			return nil
		}
		returns = ret
		// Consume any further modifiers after returns.
		extraMods := p.parseModifiersUntilBlock()
		rawMods = append(rawMods, extraMods...)
	}

	// Extract virtual/override/payableAsset from the raw modifier list.
	var isVirtual, isOverride bool
	var payableAsset string
	var modifiers []string
	for _, mod := range rawMods {
		switch mod {
		case "virtual":
			isVirtual = true
		case "override":
			isOverride = true
		default:
			// Normalize "payable(uno)" → modifier "payable" + PayableAsset "uno".
			if asset, ok := ParsePayableAsset([]string{mod}); ok && asset != "" {
				modifiers = append(modifiers, "payable")
				payableAsset = asset
			} else {
				modifiers = append(modifiers, mod)
			}
		}
	}

	// If the current token is not '{', the modifiers already consumed a ';' terminator —
	// this is a bodyless function declaration. In abstract contracts this is a valid
	// virtual stub; in concrete contracts sema will report an error (TOL2060).
	if p.cur.Type != lexer.TokenLBrace {
		return &ast.FunctionDecl{
			Name:             nameTok.Literal,
			SelectorOverride: selectorOverride,
			Params:           params,
			Returns:          returns,
			Modifiers:        modifiers,
			Body:             nil,
			Virtual:          isVirtual,
			Override:         isOverride,
			PayableAsset:     payableAsset,
			Doc:              doc,
		}
	}

	body, ok := p.parseStatementBlock("function body")
	if !ok {
		return nil
	}

	return &ast.FunctionDecl{
		Name:             nameTok.Literal,
		SelectorOverride: selectorOverride,
		Params:           params,
		Returns:          returns,
		Modifiers:        modifiers,
		Body:             body,
		Virtual:          isVirtual,
		Override:         isOverride,
		PayableAsset:     payableAsset,
		Doc:              doc,
	}
}

func (p *Parser) parseConstructorDecl() *ast.ConstructorDecl {
	doc := p.takePendingDoc()
	if !p.expect(lexer.TokenKwConstructor, diag.CodeParseUnexpected, "expected 'constructor'") {
		return nil
	}

	params := []ast.FieldDecl{}
	if p.cur.Type == lexer.TokenLParen {
		var ok bool
		params, ok = p.parseFieldList(false, true)
		if !ok {
			return nil
		}
	}

	rawMods := p.parseModifiersUntilBlock()
	var modifiers []string
	var payableAsset string
	for _, mod := range rawMods {
		if asset, ok := ParsePayableAsset([]string{mod}); ok && asset != "" {
			modifiers = append(modifiers, "payable")
			payableAsset = asset
		} else {
			modifiers = append(modifiers, mod)
		}
	}
	body, ok := p.parseStatementBlock("constructor body")
	if !ok {
		return nil
	}

	return &ast.ConstructorDecl{
		Params:       params,
		Modifiers:    modifiers,
		Body:         body,
		PayableAsset: payableAsset,
		Doc:          doc,
	}
}

func (p *Parser) parseFallbackDecl() *ast.FallbackDecl {
	doc := p.takePendingDoc()
	if !p.expect(lexer.TokenKwFallback, diag.CodeParseUnexpected, "expected 'fallback'") {
		return nil
	}
	if p.cur.Type == lexer.TokenLParen {
		params, ok := p.parseFieldList(false, false)
		if !ok {
			return nil
		}
		if len(params) > 0 {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnsupported,
				Message: "fallback parameters are not supported",
				Span:    p.span(p.cur),
			})
		}
	}
	_ = p.parseModifiersUntilBlock()
	body, ok := p.parseStatementBlock("fallback body")
	if !ok {
		return nil
	}
	return &ast.FallbackDecl{Body: body, Doc: doc}
}

// parseReceiveDecl parses: receive() payable { body }
// The "receive" identifier has already been identified but NOT yet consumed.
func (p *Parser) parseReceiveDecl() *ast.ReceiveDecl {
	doc := p.takePendingDoc()
	p.next() // consume "receive"
	// Expect empty parameter list: ()
	if !p.expect(lexer.TokenLParen, diag.CodeParseUnexpected, "expected '(' after 'receive'") {
		return nil
	}
	if p.cur.Type != lexer.TokenRParen {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnsupported,
			Message: "receive() does not accept parameters",
			Span:    p.span(p.cur),
		})
		// Skip to ')' or '{'.
		for p.cur.Type != lexer.TokenRParen && p.cur.Type != lexer.TokenLBrace && p.cur.Type != lexer.TokenEOF {
			p.next()
		}
	}
	if p.cur.Type == lexer.TokenRParen {
		p.next() // consume ')'
	}
	// Consume modifiers: we require "payable" (or "payable(uno)") but accept
	// any modifiers silently.
	mods := p.parseModifiersUntilBlock()
	hasPayable := false
	var payableAsset string
	for _, m := range mods {
		if m == "payable" {
			hasPayable = true
		} else if asset, ok := ParsePayableAsset([]string{m}); ok && asset != "" {
			hasPayable = true
			payableAsset = asset
		}
	}
	if !hasPayable {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnsupported,
			Message: "receive() must be declared payable",
			Span:    p.span(p.cur),
		})
	}
	body, ok := p.parseStatementBlock("receive body")
	if !ok {
		return nil
	}
	return &ast.ReceiveDecl{Body: body, PayableAsset: payableAsset, Doc: doc}
}

func (p *Parser) parseFieldList(allowIndexed, allowDataLoc bool) ([]ast.FieldDecl, bool) {
	if !p.expect(lexer.TokenLParen, diag.CodeParseUnexpected, "expected '('") {
		return nil, false
	}
	fields := []ast.FieldDecl{}
	if p.cur.Type == lexer.TokenRParen {
		p.next()
		return fields, true
	}

	for {
		field, ok := p.parseField(allowIndexed, allowDataLoc)
		if ok {
			fields = append(fields, field)
		} else {
			p.syncUntil(lexer.TokenComma, lexer.TokenRParen, lexer.TokenEOF)
		}
		if p.cur.Type == lexer.TokenComma {
			p.next()
			continue
		}
		break
	}
	if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')'") {
		return nil, false
	}
	return fields, true
}

// isFunctionTypeModifier returns true if the ident literal is a function type
// modifier keyword that can appear inside a function type: external, internal,
// public, private, view, pure, payable.
func isFunctionTypeModifier(lit string) bool {
	switch lit {
	case "external", "internal", "public", "private", "view", "pure", "payable":
		return true
	}
	return false
}

// parseField parses a single field/parameter in the new Solidity format:
//
//	Type [DataLoc] [indexed] [Name]
//
// (type comes first, name is optional and comes last)
func (p *Parser) parseField(allowIndexed, allowDataLoc bool) (ast.FieldDecl, bool) {
	var typeTokens []string
	depthParen := 0
	depthBracket := 0
	depthAngle := 0 // for generic type params
	// isFunctionType tracks whether the type being collected is a function type
	// (starts with "function" keyword). Function types can include modifiers
	// like "external", "internal", "view", "pure", "payable" and "returns".
	isFunctionType := false

	// Collect type tokens. Stop at dataloc keyword, indexed keyword, plain ident after type,
	// comma, or rparen.
	for p.cur.Type != lexer.TokenEOF {
		if depthParen == 0 && depthBracket == 0 && depthAngle == 0 {
			// Check for data location (memory, calldata, storage) — now promoted keywords.
			if allowDataLoc && isDataLocationKeyword(p.cur.Type) {
				break // type done, next is dataloc
			}
			// Also accept legacy ident form for data locations.
			if allowDataLoc && p.cur.Type == lexer.TokenIdent && isDataLocationToken(p.cur.Literal) {
				break // type done, next is dataloc
			}
			// Check for indexed — now promoted to TokenKwIndexed.
			if allowIndexed && p.cur.Type == lexer.TokenKwIndexed {
				break // type done, next is indexed
			}
			if p.cur.Type == lexer.TokenComma || p.cur.Type == lexer.TokenRParen {
				break // type done, no name
			}
			// A plain ident AFTER we already have type tokens = parameter name
			// (keywords like 'mapping', 'address' etc. can be part of the type)
			if len(typeTokens) > 0 && p.cur.Type == lexer.TokenIdent {
				// "payable" as plain ident is a type suffix: "agent payable"
				if p.cur.Literal == "payable" {
					// fall through to append (legacy: payable still as ident)
				} else if isFunctionType && isFunctionTypeModifier(p.cur.Literal) {
					// Function type modifiers (external, internal, view, pure, etc.) as idents.
					// fall through to append
				} else if len(typeTokens) > 0 && typeTokens[len(typeTokens)-1] == "." {
					// identifierPath: A.B.C — ident following '.' is part of the dotted type name.
					// fall through to append
				} else {
					// For simple cases: if we have type tokens and see a plain ident, it's the name.
					break // type done, next is name
				}
			}
			// A '.' at depth 0 after type tokens starts an identifierPath (e.g. IERC20.Token).
			// Don't break; let the token be appended so the next ident is consumed as part of the type.
			if len(typeTokens) > 0 && p.cur.Type == lexer.TokenDot {
				typeTokens = append(typeTokens, p.cur.Literal)
				p.next()
				continue
			}
			// Handle promoted keyword tokens after type tokens have started.
			if len(typeTokens) > 0 {
				// "payable" as keyword suffix: "agent payable"
				if p.cur.Type == lexer.TokenKwPayable {
					typeTokens = append(typeTokens, p.cur.Literal)
					p.next()
					continue
				}
				// Function type modifiers as keywords.
				if isFunctionType && isFunctionTypeModifierToken(p.cur.Type) {
					typeTokens = append(typeTokens, p.cur.Literal)
					p.next()
					continue
				}
				// State variable modifiers (public, private, internal, override) are never
				// part of a type; stop type collection so parseStorageSlot can handle them.
				if isStateVariableModifierToken(p.cur.Type) {
					break
				}
			}
			// When the type starts with 'function' keyword, track it so we can handle modifiers.
			if len(typeTokens) == 0 && p.cur.Type == lexer.TokenKwFunction {
				isFunctionType = true
			}
			// 'returns' keyword is part of a function type: function(...) returns (...)
			// Don't break when we're collecting a function type.
			if isFunctionType && p.cur.Type == lexer.TokenKwReturns {
				typeTokens = append(typeTokens, p.cur.Literal)
				p.next()
				continue
			}
			// A non-ident keyword token that is NOT a type keyword stops type collection
			// (unless we're inside parens/brackets, angle brackets, or it's a known type keyword).
			if len(typeTokens) > 0 && p.cur.Type != lexer.TokenIdent &&
				!isIdentLike(p.cur.Type) &&
				p.cur.Type != lexer.TokenKwMapping &&
				p.cur.Type != lexer.TokenLBracket &&
				p.cur.Type != lexer.TokenRBracket &&
				p.cur.Type != lexer.TokenLParen &&
				p.cur.Type != lexer.TokenRParen &&
				p.cur.Type != lexer.TokenLT && // generic type param open
				p.cur.Type != lexer.TokenGT {  // generic type param close
				break
			}
		}

		switch p.cur.Type {
		case lexer.TokenLParen:
			depthParen++
		case lexer.TokenRParen:
			if depthParen > 0 {
				depthParen--
			}
		case lexer.TokenLBracket:
			depthBracket++
		case lexer.TokenRBracket:
			if depthBracket > 0 {
				depthBracket--
			}
		case lexer.TokenLT:
			if depthAngle > 0 {
				depthAngle++
			}
		case lexer.TokenGT:
			if depthAngle > 0 {
				depthAngle--
			}
		}

		typeTokens = append(typeTokens, p.cur.Literal)
		p.next()
	}

	typ := normalizeGenericTypeString(joinTypeTokens(typeTokens))
	if typ == "" {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected parameter type",
			Span:    p.span(p.cur),
		})
		return ast.FieldDecl{}, false
	}

	// Optional data location (memory, calldata, storage — now as keyword tokens).
	dataLoc := ""
	if allowDataLoc {
		if isDataLocationKeyword(p.cur.Type) {
			dataLoc = p.cur.Literal
			p.next()
		} else if p.cur.Type == lexer.TokenIdent && isDataLocationToken(p.cur.Literal) {
			dataLoc = p.cur.Literal
			p.next()
		}
	}

	// Optional indexed (may appear before or after name) — now TokenKwIndexed.
	indexed := false
	if allowIndexed && p.cur.Type == lexer.TokenKwIndexed {
		indexed = true
		p.next()
	}

	// Optional name: a plain identifier that is not a terminator.
	// Also accept certain keyword tokens used as names (e.g., "storage" as param name is unusual
	// but we keep backward compat for plain TokenIdent names).
	name := ""
	if p.cur.Type == lexer.TokenIdent &&
		!isDataLocationToken(p.cur.Literal) &&
		p.cur.Literal != "indexed" {
		name = p.cur.Literal
		p.next()
	}

	// Optional indexed after name (Solidity style: "address from indexed")
	if allowIndexed && !indexed && p.cur.Type == lexer.TokenKwIndexed {
		indexed = true
		p.next()
	}

	return ast.FieldDecl{
		Name:    name,
		Type:    typ,
		DataLoc: dataLoc,
		Indexed: indexed,
	}, true
}

func isDataLocationToken(s string) bool {
	switch s {
	case "memory", "calldata", "storage":
		return true
	default:
		return false
	}
}

func (p *Parser) parseTypeUntil(stop map[lexer.Type]bool) string {
	var tokens []string
	depthParen := 0
	depthBracket := 0

	for p.cur.Type != lexer.TokenEOF {
		if depthParen == 0 && depthBracket == 0 && stop[p.cur.Type] {
			break
		}
		switch p.cur.Type {
		case lexer.TokenLParen:
			depthParen++
		case lexer.TokenRParen:
			if depthParen > 0 {
				depthParen--
			}
		case lexer.TokenLBracket:
			depthBracket++
		case lexer.TokenRBracket:
			if depthBracket > 0 {
				depthBracket--
			}
		}
		// Include keyword tokens that are valid in type positions (mapping, storage, etc.)
		// Their Literal is the keyword text.
		tokens = append(tokens, p.cur.Literal)
		p.next()
	}
	return joinTypeTokens(tokens)
}

// joinTypeTokens joins type token literals into a canonical type string
// without unnecessary spaces around punctuation. Rules:
//
//	No space before: [ ] ( ) , .
//	No space after:  [ ( , .
//	Everything else gets a space (e.g. "mapping(address => uint256)").
//
// It also strips optional key/value identifier names from mapping types,
// e.g. "mapping(address key => uint256 value)" becomes "mapping(address => uint256)".
func joinTypeTokens(tokens []string) string {
	tokens = stripMappingKeyValueNames(tokens)
	if len(tokens) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(tokens[0])
	for i := 1; i < len(tokens); i++ {
		prev, cur := tokens[i-1], tokens[i]
		noSpaceBefore := cur == "[" || cur == "]" || cur == "(" || cur == ")" || cur == "," || cur == "."
		noSpaceAfter := prev == "[" || prev == "(" || prev == "," || prev == "."
		if !noSpaceBefore && !noSpaceAfter {
			b.WriteByte(' ')
		}
		b.WriteString(cur)
	}
	return b.String()
}

// normalizeGenericTypeString removes spaces around '<' and '>' for generic type params.
func normalizeGenericTypeString(s string) string {
	s = strings.ReplaceAll(s, " < ", "<")
	s = strings.ReplaceAll(s, " >", ">")
	s = strings.ReplaceAll(s, "< ", "<")
	return s
}

// isTypelikeToken returns true if s is a type-like token (identifier or type keyword),
// as opposed to a punctuation symbol like "(", ")", "[", "]", ",", "=>", "mapping", etc.
func isTypelikeToken(s string) bool {
	switch s {
	case "(", ")", "[", "]", ",", ".", "=>", "":
		return false
	}
	return true
}

// stripMappingKeyValueNames removes the optional named key/value identifiers from
// mapping type tokens. E.g.:
//
//	["mapping", "(", "agent", "key", "=>", "u256", "value", ")"]
//	→ ["mapping", "(", "agent", "=>", "u256", ")"]
//
// A name is stripped only when two consecutive typeLike tokens appear before "=>"
// (strip the second one) or after "=>" and before ")" at depth 1 (strip the one
// before the closing paren when preceded by a typeLike token).
func stripMappingKeyValueNames(tokens []string) []string {
	// Quick check: does the token list contain "=>"? If not, nothing to strip.
	hasArrow := false
	for _, t := range tokens {
		if t == "=>" {
			hasArrow = true
			break
		}
	}
	if !hasArrow {
		return tokens
	}

	result := make([]string, 0, len(tokens))
	// Track mapping depth: increments on "mapping" token, decrements on matching ")".
	// We use a simple parenthesis depth counter within mapping contexts.
	inMapping := make(map[int]bool)
	depth := 0

	for _, tok := range tokens {
		switch tok {
		case "mapping":
			result = append(result, tok)
			depth++
			inMapping[depth] = true
		case "(":
			result = append(result, tok)
			if !inMapping[depth] {
				depth++
			}
		case ")":
			if inMapping[depth] {
				// Before closing a mapping paren, check if last two result tokens are both
				// typeLike — if so, strip the last one (it's the value name).
				n := len(result)
				if n >= 2 && isTypelikeToken(result[n-1]) && isTypelikeToken(result[n-2]) {
					result = result[:n-1]
				}
				inMapping[depth] = false
				depth--
				if depth < 0 {
					depth = 0
				}
			} else {
				if depth > 0 {
					depth--
				}
			}
			result = append(result, tok)
		case "=>":
			if depth > 0 && inMapping[depth] {
				// Before "=>", check if last two result tokens are both typeLike —
				// if so, strip the last one (it's the key name).
				n := len(result)
				if n >= 2 && isTypelikeToken(result[n-1]) && isTypelikeToken(result[n-2]) {
					result = result[:n-1]
				}
			}
			result = append(result, tok)
		default:
			result = append(result, tok)
		}
	}
	return result
}
func (p *Parser) parseModifiersUntilBlock() []string {
	var mods []string
	for p.cur.Type != lexer.TokenEOF && p.cur.Type != lexer.TokenLBrace {
		// A ';' terminates a bodyless (virtual stub) function declaration.
		// When inside an abstract contract this is valid; when outside, sema will
		// report the error (TOL2060). The parser handles the token in both cases.
		if p.cur.Type == lexer.TokenSemicolon {
			p.next()
			return mods
		}
		// Stop at 'returns' keyword so the caller can handle the returns clause.
		if p.cur.Type == lexer.TokenKwReturns {
			break
		}
		// Handle payable(asset) syntax: payable(uno), payable(cc), etc.
		// Collapsed into a single modifier string "payable(uno)".
		if p.cur.Type == lexer.TokenKwPayable && p.peek().Type == lexer.TokenLParen {
			p.next() // consume 'payable'
			p.next() // consume '('
			if p.cur.Type == lexer.TokenIdent || p.cur.Type == lexer.TokenKwUno {
				asset := p.cur.Literal
				p.next() // consume asset name
				if p.cur.Type == lexer.TokenRParen {
					p.next() // consume ')'
				}
				mods = append(mods, "payable("+asset+")")
			} else {
				// payable() with empty parens — treat as plain payable
				if p.cur.Type == lexer.TokenRParen {
					p.next()
				}
				mods = append(mods, "payable")
			}
			continue
		}
		mods = append(mods, p.cur.Literal)
		p.next()
	}
	return mods
}

// ParsePayableAsset extracts the asset type from a modifier list.
// Returns ("uno", true) for payable(uno), ("", true) for plain payable, ("", false) for non-payable.
func ParsePayableAsset(modifiers []string) (asset string, isPayable bool) {
	for _, m := range modifiers {
		if m == "payable" {
			return "", true
		}
		if strings.HasPrefix(m, "payable(") && strings.HasSuffix(m, ")") {
			return m[len("payable(") : len(m)-1], true
		}
	}
	return "", false
}

func (p *Parser) parseStatementBlock(what string) ([]ast.Statement, bool) {
	if p.cur.Type != lexer.TokenLBrace {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected '{' before " + what,
			Span:    p.span(p.cur),
		})
		return nil, false
	}
	p.next()

	stmts := []ast.Statement{}
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		stmt, ok := p.parseStatement()
		if ok {
			stmts = append(stmts, stmt)
			continue
		}
		p.syncStatement()
	}
	if p.cur.Type == lexer.TokenEOF {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "unexpected EOF while parsing " + what,
			Span:    p.span(p.cur),
		})
		return nil, false
	}
	p.next()
	return stmts, true
}

func (p *Parser) parseStatement() (ast.Statement, bool) {
	line := p.cur.Start.Line

	// Type-first tuple destructuring: (T1 a, T2 b) = expr;
	// Disambiguate from expression: if '(' followed by type-start followed by ident.
	if p.cur.Type == lexer.TokenLParen {
		peek1 := p.peek()
		if isTypeStartToken(peek1.Type, peek1.Literal) {
			peek2 := p.lex.PeekSecond()
			if peek2.Type == lexer.TokenIdent || isIdentLike(peek2.Type) {
				return p.parseTypeFirstTupleDecl()
			}
		}
	}
	// Solidity-style local variable declaration: Type name [= expr];
	// Disambiguate: if current token can start a type and the NEXT token is an
	// identifier, treat it as a type-first var decl. Also detect array types
	// (u256[] x, u256[3] x) where next is '[' and peek-second is ']' or number.
	// Key: `address(x)` does NOT trigger — next token is '(' not an ident.
	// Key: `balances[addr]` does NOT trigger — peek-second is ident, not ']'/number.
	if p.isTypeStart() &&
		p.cur.Type != lexer.TokenKwDelete &&
		p.cur.Type != lexer.TokenKwUnchecked &&
		p.cur.Type != lexer.TokenKwTry &&
		!(p.cur.Type == lexer.TokenIdent && (p.cur.Literal == "unchecked" || p.cur.Literal == "try")) {
		nxt := p.peek()
		if nxt.Type == lexer.TokenIdent {
			return p.parseTypeFirstVarDecl()
		}
		if nxt.Type == lexer.TokenLBracket {
			peek2 := p.lex.PeekSecond()
			if peek2.Type == lexer.TokenRBracket || peek2.Type == lexer.TokenNumber {
				return p.parseTypeFirstVarDecl()
			}
		}
	}
	switch p.cur.Type {
	case lexer.TokenSemicolon:
		p.next()
		return ast.Statement{}, false
	case lexer.TokenKwLet:
		return p.parseLetStatement(lexer.TokenSemicolon)
	case lexer.TokenKwSet:
		return p.parseSetStatement(lexer.TokenSemicolon)
	case lexer.TokenKwReturn:
		return p.parseReturnStatement()
	case lexer.TokenKwBreak:
		if !p.expect(lexer.TokenKwBreak, diag.CodeParseUnexpected, "expected 'break'") {
			return ast.Statement{}, false
		}
		if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after break") {
			return ast.Statement{}, false
		}
		return ast.Statement{Kind: "break", Line: line}, true
	case lexer.TokenKwContinue:
		if !p.expect(lexer.TokenKwContinue, diag.CodeParseUnexpected, "expected 'continue'") {
			return ast.Statement{}, false
		}
		if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after continue") {
			return ast.Statement{}, false
		}
		return ast.Statement{Kind: "continue", Line: line}, true
	case lexer.TokenKwRequire:
		return p.parseRequireAssertStatement("require", lexer.TokenKwRequire)
	case lexer.TokenKwAssert:
		return p.parseRequireAssertStatement("assert", lexer.TokenKwAssert)
	case lexer.TokenKwRevert:
		return p.parseUnaryCallLikeStatement("revert", lexer.TokenKwRevert)
	case lexer.TokenKwEmit:
		return p.parseUnaryCallLikeStatement("emit", lexer.TokenKwEmit)
	case lexer.TokenKwDelete:
		return p.parseDeleteStatement()
	case lexer.TokenKwIf:
		return p.parseIfStatement()
	case lexer.TokenKwDo:
		return p.parseDoWhileStatement()
	case lexer.TokenKwWhile:
		return p.parseWhileStatement()
	case lexer.TokenKwFor:
		return p.parseForStatement()
	case lexer.TokenPlusPlus, lexer.TokenMinusMinus:
		return p.parsePrefixIncDecStatement()
	// Promoted keywords with dedicated statement handlers:
	case lexer.TokenKwUnchecked:
		return p.parseUncheckedStatement()
	case lexer.TokenKwTry:
		return p.parseTryCatchStatement()
	case lexer.TokenIdent:
		return p.parseExprSemicolonStmt()
	default:
		return p.parseExprSemicolonStmt()
	}
}

// parseTypeFirstVarDecl parses a Solidity-style local variable declaration:
//
//	Type name [= expr];
//
// Examples:
//
//	uint256 x = 1;
//	address owner;
//	mapping(address => uint256) balances;
//
// This is equivalent to `let name: Type [= expr];` and produces the same AST node.
// Called when the parser detects a type token followed by an identifier.
func (p *Parser) parseTypeFirstVarDecl() (ast.Statement, bool) {
	line := p.cur.Start.Line
	// Parse the type using parseField (handles mapping(...), uint256[], address, etc.)
	field, ok := p.parseField(false, true) // allowIndexed=false, allowDataLoc=true
	if !ok {
		p.syncUntil(lexer.TokenSemicolon, lexer.TokenRBrace, lexer.TokenEOF)
		if p.cur.Type == lexer.TokenSemicolon {
			p.next()
		}
		return ast.Statement{}, false
	}
	if field.Name == "" {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected variable name after type in local variable declaration",
			Span:    p.span(p.cur),
		})
		p.syncUntil(lexer.TokenSemicolon, lexer.TokenRBrace, lexer.TokenEOF)
		if p.cur.Type == lexer.TokenSemicolon {
			p.next()
		}
		return ast.Statement{}, false
	}
	stmt := ast.Statement{
		Kind: "let",
		Name: field.Name,
		Type: field.Type,
		Line: line,
	}
	// Optional initializer: = expr
	if p.cur.Type == lexer.TokenAssign {
		p.next()
		expr, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
		if !ok {
			return ast.Statement{}, false
		}
		stmt.Expr = expr
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after local variable declaration") {
		return ast.Statement{}, false
	}
	return stmt, true
}

// parsePrefixIncDecStatement parses a standalone prefix ++x; or --x; statement.
// The ++ or -- token is consumed first, then the target expression, then ';'.
func (p *Parser) parsePrefixIncDecStatement() (ast.Statement, bool) {
	line := p.cur.Start.Line
	op := p.cur.Literal // "++" or "--"
	p.next()            // consume ++ or --
	target, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
	if !ok {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after "+op+" statement") {
		return ast.Statement{}, false
	}
	return ast.Statement{
		Kind:   "set",
		Op:     op,
		Target: target,
		Line:   line,
	}, true
}

func (p *Parser) parseDeleteStatement() (ast.Statement, bool) {
	line := p.cur.Start.Line
	p.next() // consume "delete" (TokenKwDelete)
	expr, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
	if !ok {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after delete statement") {
		return ast.Statement{}, false
	}
	return ast.Statement{Kind: "delete", Expr: expr, Line: line}, true
}

func (p *Parser) parseUncheckedStatement() (ast.Statement, bool) {
	line := p.cur.Start.Line
	p.next() // consume "unchecked"
	body, ok := p.parseStatementBlock("unchecked body")
	if !ok {
		return ast.Statement{}, false
	}
	return ast.Statement{Kind: "unchecked", Body: body, Line: line}, true
}

func (p *Parser) parseLetStatement(terminator lexer.Type) (ast.Statement, bool) {
	line := p.cur.Start.Line
	if !p.expect(lexer.TokenKwLet, diag.CodeParseUnexpected, "expected 'let'") {
		return ast.Statement{}, false
	}

	// Emit error with migration guidance, then parse anyway for error recovery.
	if p.cur.Type == lexer.TokenLParen {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "'let' is removed; use type-first tuple syntax: (T1 a, T2 b) = expr;",
			Span:    diag.Span{File: p.filename, Start: diag.Position{Line: line}},
		})
		return p.parseLetTupleStatement(terminator, line)
	}

	// Single variable: let x: T = expr → error, suggest T x = expr
	var hint string
	if p.cur.Type == lexer.TokenIdent {
		hint = fmt.Sprintf("'let' is removed; use type-first syntax: T %s = expr;", p.cur.Literal)
	} else {
		hint = "'let' is removed; use type-first syntax: T name = expr;"
	}
	p.addDiag(diag.Diagnostic{
		Code:    diag.CodeParseUnexpected,
		Message: hint,
		Span:    diag.Span{File: p.filename, Start: diag.Position{Line: line}},
	})

	// Parse the rest for error recovery.
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected variable name after 'let'") {
		return ast.Statement{}, false
	}
	stmt := ast.Statement{
		Kind: "let",
		Name: nameTok.Literal,
		Line: line,
	}

	if p.cur.Type == lexer.TokenColon {
		p.next()
		typ := p.parseTypeUntil(map[lexer.Type]bool{
			lexer.TokenAssign: true,
			terminator:        true,
		})
		if typ == "" {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected type in let statement",
				Span:    p.span(p.cur),
			})
			return ast.Statement{}, false
		}
		stmt.Type = typ
	}

	if p.cur.Type == lexer.TokenAssign {
		p.next()
		expr, ok := p.parseExpression(map[lexer.Type]bool{terminator: true})
		if !ok {
			return ast.Statement{}, false
		}
		stmt.Expr = expr
	}

	if !p.expect(terminator, diag.CodeParseUnexpected, "expected statement terminator after let") {
		return ast.Statement{}, false
	}
	return stmt, true
}

// parseLetTupleStatement parses: let (a, b, ...) : (T1, T2, ...) = expr;
// Called when 'let' is already consumed and cur is at '('.
func (p *Parser) parseLetTupleStatement(terminator lexer.Type, line int) (ast.Statement, bool) {
	// Consume '('
	if !p.expect(lexer.TokenLParen, diag.CodeParseUnexpected, "expected '(' in tuple let binding") {
		return ast.Statement{}, false
	}

	// Parse comma-separated variable names.
	var names []string
	for {
		if p.cur.Type != lexer.TokenIdent {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected variable name in tuple let binding",
				Span:    p.span(p.cur),
			})
			return ast.Statement{}, false
		}
		names = append(names, p.cur.Literal)
		p.next()
		if p.cur.Type == lexer.TokenComma {
			p.next()
			continue
		}
		break
	}
	if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after variable names in tuple let binding") {
		return ast.Statement{}, false
	}

	// Optional type annotation: : (T1, T2, ...)
	var types []string
	if p.cur.Type == lexer.TokenColon {
		p.next()
		if p.cur.Type != lexer.TokenLParen {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected '(' after ':' in tuple type annotation",
				Span:    p.span(p.cur),
			})
			return ast.Statement{}, false
		}
		p.next() // consume '('
		for {
			typ := p.parseTypeUntil(map[lexer.Type]bool{
				lexer.TokenComma:  true,
				lexer.TokenRParen: true,
			})
			if typ == "" {
				p.addDiag(diag.Diagnostic{
					Code:    diag.CodeParseUnexpected,
					Message: "expected type in tuple type annotation",
					Span:    p.span(p.cur),
				})
				return ast.Statement{}, false
			}
			types = append(types, typ)
			if p.cur.Type == lexer.TokenComma {
				p.next()
				continue
			}
			break
		}
		if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after tuple type list") {
			return ast.Statement{}, false
		}
	}

	// Require '='
	if !p.expect(lexer.TokenAssign, diag.CodeParseUnexpected, "expected '=' in tuple let binding") {
		return ast.Statement{}, false
	}

	expr, ok := p.parseExpression(map[lexer.Type]bool{terminator: true})
	if !ok {
		return ast.Statement{}, false
	}

	if !p.expect(terminator, diag.CodeParseUnexpected, "expected statement terminator after tuple let") {
		return ast.Statement{}, false
	}

	return ast.Statement{
		Kind:  "let-tuple",
		Names: names,
		Types: types,
		Line:  line,
		Expr:  expr,
	}, true
}

// parseTypeFirstTupleDecl parses a Solidity-style tuple variable declaration:
//
//	(T1 a, T2 b, ...) = expr;
//
// Each element is a type-name pair. Produces the same "let-tuple" AST node
// as the old let (a, b): (T1, T2) = expr syntax.
func (p *Parser) parseTypeFirstTupleDecl() (ast.Statement, bool) {
	line := p.cur.Start.Line
	if !p.expect(lexer.TokenLParen, diag.CodeParseUnexpected, "expected '(' in tuple declaration") {
		return ast.Statement{}, false
	}

	var names []string
	var types []string
	for {
		// Parse each element as a type-name pair using parseField.
		field, ok := p.parseField(false, false)
		if !ok || field.Type == "" {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected type and variable name in tuple declaration, e.g. (u256 a, bool b)",
				Span:    p.span(p.cur),
			})
			p.syncUntil(lexer.TokenSemicolon, lexer.TokenRBrace, lexer.TokenEOF)
			if p.cur.Type == lexer.TokenSemicolon {
				p.next()
			}
			return ast.Statement{}, false
		}
		if field.Name == "" {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("expected variable name after type '%s' in tuple declaration", field.Type),
				Span:    p.span(p.cur),
			})
			p.syncUntil(lexer.TokenSemicolon, lexer.TokenRBrace, lexer.TokenEOF)
			if p.cur.Type == lexer.TokenSemicolon {
				p.next()
			}
			return ast.Statement{}, false
		}
		types = append(types, field.Type)
		names = append(names, field.Name)
		if p.cur.Type == lexer.TokenComma {
			p.next()
			continue
		}
		break
	}
	if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after tuple declaration") {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenAssign, diag.CodeParseUnexpected, "expected '=' after tuple declaration") {
		return ast.Statement{}, false
	}
	expr, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
	if !ok {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after tuple declaration") {
		return ast.Statement{}, false
	}
	return ast.Statement{
		Kind:  "let-tuple",
		Names: names,
		Types: types,
		Line:  line,
		Expr:  expr,
	}, true
}

// compoundAssignOp maps compound-assignment token types to the operator string.
func compoundAssignOp(tt lexer.Type) (string, bool) {
	switch tt {
	case lexer.TokenPlusAssign:
		return "+=", true
	case lexer.TokenMinusAssign:
		return "-=", true
	case lexer.TokenMulAssign:
		return "*=", true
	case lexer.TokenDivAssign:
		return "/=", true
	case lexer.TokenModAssign:
		return "%=", true
	case lexer.TokenAndAssign:
		return "&=", true
	case lexer.TokenOrAssign:
		return "|=", true
	case lexer.TokenXorAssign:
		return "^=", true
	case lexer.TokenShlAssign:
		return "<<=", true
	case lexer.TokenSarAssign:
		return ">>=", true
	case lexer.TokenShrAssign:
		return ">>>=", true
	}
	return "", false
}

// isCompoundAssignToken returns true for any compound-assignment token type.
func isCompoundAssignToken(tt lexer.Type) bool {
	_, ok := compoundAssignOp(tt)
	return ok
}

func (p *Parser) parseSetStatement(terminator lexer.Type) (ast.Statement, bool) {
	line := p.cur.Start.Line
	if !p.expect(lexer.TokenKwSet, diag.CodeParseUnexpected, "expected 'set'") {
		return ast.Statement{}, false
	}

	// Build stop set for target expression: stop at '=', compound-assign operators, '++', '--'.
	targetStop := map[lexer.Type]bool{
		lexer.TokenAssign:      true,
		lexer.TokenPlusAssign:  true,
		lexer.TokenMinusAssign: true,
		lexer.TokenMulAssign:   true,
		lexer.TokenDivAssign:   true,
		lexer.TokenModAssign:   true,
		lexer.TokenAndAssign:   true,
		lexer.TokenOrAssign:    true,
		lexer.TokenXorAssign:   true,
		lexer.TokenShlAssign:   true,
		lexer.TokenSarAssign:   true,
		lexer.TokenShrAssign:   true,
		lexer.TokenPlusPlus:    true,
		lexer.TokenMinusMinus:  true,
	}
	target, ok := p.parseExpression(targetStop)
	if !ok {
		return ast.Statement{}, false
	}

	// Check for ++ / -- (inc/dec after target).
	if p.cur.Type == lexer.TokenPlusPlus || p.cur.Type == lexer.TokenMinusMinus {
		op := p.cur.Literal // "++" or "--"
		p.next()           // consume ++ or --
		if !p.expect(terminator, diag.CodeParseUnexpected, "expected statement terminator after "+op) {
			return ast.Statement{}, false
		}
		return ast.Statement{
			Kind:   "set",
			Op:     op,
			Target: target,
			Line:   line,
		}, true
	}

	// Check for compound assignment operators.
	if op, isCompound := compoundAssignOp(p.cur.Type); isCompound {
		p.next() // consume the compound-assign token
		value, ok := p.parseExpression(map[lexer.Type]bool{terminator: true})
		if !ok {
			return ast.Statement{}, false
		}
		if !p.expect(terminator, diag.CodeParseUnexpected, "expected statement terminator after "+op) {
			return ast.Statement{}, false
		}
		return ast.Statement{
			Kind:   "set",
			Op:     op,
			Target: target,
			Expr:   value,
			Line:   line,
		}, true
	}

	// Plain assignment: set x = expr;
	if !p.expect(lexer.TokenAssign, diag.CodeParseUnexpected, "expected '=' in set statement") {
		return ast.Statement{}, false
	}
	value, ok := p.parseExpression(map[lexer.Type]bool{terminator: true})
	if !ok {
		return ast.Statement{}, false
	}
	if !p.expect(terminator, diag.CodeParseUnexpected, "expected statement terminator after set") {
		return ast.Statement{}, false
	}
	return ast.Statement{
		Kind:   "set",
		Target: target,
		Expr:   value,
		Line:   line,
	}, true
}

func (p *Parser) parseReturnStatement() (ast.Statement, bool) {
	line := p.cur.Start.Line
	if !p.expect(lexer.TokenKwReturn, diag.CodeParseUnexpected, "expected 'return'") {
		return ast.Statement{}, false
	}
	if p.cur.Type == lexer.TokenSemicolon {
		p.next()
		return ast.Statement{Kind: "return", Line: line}, true
	}
	expr, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
	if !ok {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after return statement") {
		return ast.Statement{}, false
	}
	return ast.Statement{
		Kind: "return",
		Expr: expr,
		Line: line,
	}, true
}

func (p *Parser) parseUnaryCallLikeStatement(kind string, kw lexer.Type) (ast.Statement, bool) {
	line := p.cur.Start.Line
	if !p.expect(kw, diag.CodeParseUnexpected, "expected '"+kind+"'") {
		return ast.Statement{}, false
	}
	if p.cur.Type == lexer.TokenSemicolon {
		p.next()
		return ast.Statement{Kind: kind, Line: line}, true
	}
	expr, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
	if !ok {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after "+kind+" statement") {
		return ast.Statement{}, false
	}
	return ast.Statement{
		Kind: kind,
		Expr: expr,
		Line: line,
	}, true
}

// parseRequireAssertStatement parses: require(cond[, "msg"]); or assert(cond[, "msg"]);
// The second message argument is optional; when omitted, Text is set to "".
// The condition expression is stored in Stmt.Expr; the message string in Stmt.Text.
func (p *Parser) parseRequireAssertStatement(kind string, kw lexer.Type) (ast.Statement, bool) {
	line := p.cur.Start.Line
	if !p.expect(kw, diag.CodeParseUnexpected, "expected '"+kind+"'") {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenLParen, diag.CodeParseUnexpected, "expected '(' after '"+kind+"'") {
		return ast.Statement{}, false
	}
	cond, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenComma: true, lexer.TokenRParen: true})
	if !ok {
		return ast.Statement{}, false
	}
	msg := `""`
	if p.cur.Type == lexer.TokenComma {
		p.next() // consume ','
		if p.cur.Type != lexer.TokenString {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected string literal as " + kind + " message",
				Span:    p.span(p.cur),
			})
			return ast.Statement{}, false
		}
		msg = p.cur.Literal
		p.next()
	}
	if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after "+kind+" arguments") {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after "+kind+" statement") {
		return ast.Statement{}, false
	}
	return ast.Statement{Kind: kind, Expr: cond, Text: msg, Line: line}, true
}

func (p *Parser) parseExprSemicolonStmt() (ast.Statement, bool) {
	line := p.cur.Start.Line

	// Parse the expression. The expression parser handles plain '=' as an infix
	// operator (producing Kind="assign"), but compound-assign operators (+=, -=, etc.)
	// are not infix operators — the expression parser will stop before them.
	// We use a stop set that includes both ';' and all compound-assign operators so
	// that the LHS is parsed cleanly and we can detect the operator below.
	lhsStop := map[lexer.Type]bool{
		lexer.TokenSemicolon:   true,
		lexer.TokenPlusAssign:  true,
		lexer.TokenMinusAssign: true,
		lexer.TokenMulAssign:   true,
		lexer.TokenDivAssign:   true,
		lexer.TokenModAssign:   true,
		lexer.TokenAndAssign:   true,
		lexer.TokenOrAssign:    true,
		lexer.TokenXorAssign:   true,
		lexer.TokenShlAssign:   true,
		lexer.TokenSarAssign:   true,
		lexer.TokenShrAssign:   true,
	}
	expr, ok := p.parseExpression(lhsStop)
	if !ok {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "unexpected EOF while parsing expression statement",
			Span:    p.span(p.cur),
		})
		return ast.Statement{}, false
	}

	// Compound-assign operator: x += rhs; / x -= rhs; etc.
	// The expression above parsed the LHS; now consume the operator and RHS.
	if op, isCompound := compoundAssignOp(p.cur.Type); isCompound {
		p.next() // consume compound-assign token
		value, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
		if !ok {
			return ast.Statement{}, false
		}
		if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after "+op+" statement") {
			return ast.Statement{}, false
		}
		return ast.Statement{
			Kind:   "set",
			Op:     op,
			Target: expr,
			Expr:   value,
			Line:   line,
		}, true
	}

	// Postfix i++; / i--; → rewrite to set statement so the lowering path is
	// identical to "set i++;" (desugarStmt handles Op "++" / "--").
	if expr != nil && expr.Kind == "unary" && (expr.Op == "post++" || expr.Op == "post--") {
		op := expr.Op[4:] // strip "post" prefix → "++" or "--"
		if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after "+op+" statement") {
			return ast.Statement{}, false
		}
		return ast.Statement{Kind: "set", Op: op, Target: expr.Right, Line: line}, true
	}

	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after expression statement") {
		return ast.Statement{}, false
	}

	// Plain assignment (x = expr) produces Kind="assign" expression and is allowed
	// as an expression statement. Other expressions (calls) are also allowed.
	// The sema layer will reject any non-call, non-assign expression statements (TOL2020).
	return ast.Statement{
		Kind: "expr",
		Expr: expr,
		Line: line,
	}, true
}

// parseBlockOrSingleStatement parses either a block `{ ... }` or a single
// statement (8.1: non-block bodies for if/else/while/for).
func (p *Parser) parseBlockOrSingleStatement(what string) ([]ast.Statement, bool) {
	if p.cur.Type == lexer.TokenLBrace {
		return p.parseStatementBlock(what)
	}
	// Single statement (no braces).
	stmt, ok := p.parseStatement()
	if !ok {
		return nil, false
	}
	return []ast.Statement{stmt}, true
}

func (p *Parser) parseIfStatement() (ast.Statement, bool) {
	line := p.cur.Start.Line
	if !p.expect(lexer.TokenKwIf, diag.CodeParseUnexpected, "expected 'if'") {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenLParen, diag.CodeParseUnexpected, "expected '(' after 'if'") {
		return ast.Statement{}, false
	}
	cond, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenRParen: true})
	if !ok {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "unexpected EOF while parsing if condition",
			Span:    p.span(p.cur),
		})
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after if condition") {
		return ast.Statement{}, false
	}
	// 8.1: accept block or single statement
	thenBlock, ok := p.parseBlockOrSingleStatement("if body")
	if !ok {
		return ast.Statement{}, false
	}

	stmt := ast.Statement{
		Kind: "if",
		Cond: cond,
		Then: thenBlock,
		Line: line,
	}
	if p.cur.Type == lexer.TokenKwElse {
		p.next()
		if p.cur.Type == lexer.TokenKwIf {
			nested, ok := p.parseIfStatement()
			if !ok {
				return ast.Statement{}, false
			}
			stmt.Else = []ast.Statement{nested}
			return stmt, true
		}
		// 8.1: accept block or single statement for else
		elseBlock, ok := p.parseBlockOrSingleStatement("else body")
		if !ok {
			return ast.Statement{}, false
		}
		stmt.Else = elseBlock
	}
	return stmt, true
}

func (p *Parser) parseWhileStatement() (ast.Statement, bool) {
	line := p.cur.Start.Line
	if !p.expect(lexer.TokenKwWhile, diag.CodeParseUnexpected, "expected 'while'") {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenLParen, diag.CodeParseUnexpected, "expected '(' after 'while'") {
		return ast.Statement{}, false
	}
	cond, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenRParen: true})
	if !ok {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "unexpected EOF while parsing while condition",
			Span:    p.span(p.cur),
		})
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after while condition") {
		return ast.Statement{}, false
	}
	// 8.1: accept block or single statement
	body, ok := p.parseBlockOrSingleStatement("while body")
	if !ok {
		return ast.Statement{}, false
	}
	return ast.Statement{
		Kind: "while",
		Cond: cond,
		Body: body,
		Line: line,
	}, true
}

// parseDoWhileStatement parses: do { body } while (cond);
// The "do" keyword has already been identified as the current token.
func (p *Parser) parseDoWhileStatement() (ast.Statement, bool) {
	line := p.cur.Start.Line
	if !p.expect(lexer.TokenKwDo, diag.CodeParseUnexpected, "expected 'do'") {
		return ast.Statement{}, false
	}
	body, ok := p.parseStatementBlock("do body")
	if !ok {
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenKwWhile, diag.CodeParseUnexpected, "expected 'while' after do body") {
		return ast.Statement{}, false
	}
	// The condition is parenthesised: while (cond)
	// parseExpression with LBrace stop would not work here — the condition is wrapped in parens.
	// We parse it as a parenthesised expression by consuming '(' manually and stopping at ')'.
	if !p.expect(lexer.TokenLParen, diag.CodeParseUnexpected, "expected '(' after 'while' in do/while") {
		return ast.Statement{}, false
	}
	cond, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenRParen: true})
	if !ok {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "unexpected EOF while parsing do/while condition",
			Span:    p.span(p.cur),
		})
		return ast.Statement{}, false
	}
	if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' to close do/while condition") {
		return ast.Statement{}, false
	}
	if p.cur.Type == lexer.TokenSemicolon {
		p.next()
	}
	return ast.Statement{
		Kind: "dowhile",
		Cond: cond,
		Body: body,
		Line: line,
	}, true
}

func (p *Parser) parseForStatement() (ast.Statement, bool) {
	line := p.cur.Start.Line
	if !p.expect(lexer.TokenKwFor, diag.CodeParseUnexpected, "expected 'for'") {
		return ast.Statement{}, false
	}

	if !p.expect(lexer.TokenLParen, diag.CodeParseUnexpected, "expected '(' after 'for'") {
		return ast.Statement{}, false
	}

	var init *ast.Statement
	if p.cur.Type != lexer.TokenSemicolon {
		switch p.cur.Type {
		case lexer.TokenKwLet:
			s, ok := p.parseLetStatement(lexer.TokenSemicolon)
			if !ok {
				return ast.Statement{}, false
			}
			init = &s
		case lexer.TokenKwSet:
			s, ok := p.parseSetStatement(lexer.TokenSemicolon)
			if !ok {
				return ast.Statement{}, false
			}
			init = &s
		default:
			// Check for Solidity-style type-first variable declaration:
			// Type name [= expr]; — e.g. "uint256 i = 0" or "u256[] arr = ..."
			isTypeFirstDecl := false
			if p.isTypeStart() &&
				p.cur.Type != lexer.TokenKwDelete &&
				p.cur.Type != lexer.TokenKwUnchecked &&
				p.cur.Type != lexer.TokenKwTry {
				nxt := p.peek()
				if nxt.Type == lexer.TokenIdent {
					isTypeFirstDecl = true
				} else if nxt.Type == lexer.TokenLBracket {
					peek2 := p.lex.PeekSecond()
					if peek2.Type == lexer.TokenRBracket || peek2.Type == lexer.TokenNumber {
						isTypeFirstDecl = true
					}
				}
			}
			if isTypeFirstDecl {
				s, ok := p.parseTypeFirstVarDecl()
				if !ok {
					return ast.Statement{}, false
				}
				init = &s
			} else {
				expr, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
				if !ok {
					return ast.Statement{}, false
				}
				if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after for init expression") {
					return ast.Statement{}, false
				}
				s := ast.Statement{Kind: "expr", Expr: expr}
				init = &s
			}
		}
	} else {
		p.next()
	}

	var cond *ast.Expr
	if p.cur.Type != lexer.TokenSemicolon {
		var ok bool
		cond, ok = p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
		if !ok {
			return ast.Statement{}, false
		}
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after for condition") {
		return ast.Statement{}, false
	}

	var post *ast.Expr
	if p.cur.Type != lexer.TokenRParen {
		var ok bool
		post, ok = p.parseExpression(map[lexer.Type]bool{lexer.TokenRParen: true})
		if !ok {
			return ast.Statement{}, false
		}
	}
	if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after for post-expression") {
		return ast.Statement{}, false
	}

	// 8.1: accept block or single statement for for body
	body, ok := p.parseBlockOrSingleStatement("for body")
	if !ok {
		return ast.Statement{}, false
	}
	return ast.Statement{
		Kind: "for",
		Init: init,
		Cond: cond,
		Post: post,
		Body: body,
		Line: line,
	}, true
}

// parseTryCatchStatement parses:
//
//	try expr [returns (T x)] { body } catch { body }
//	try expr [returns (T x)] { body } catch Error(name: string) { body }
//	try expr [returns (T x)] { body } catch (name: bytes) { body }
//
// "try" is now TokenKwTry (promoted keyword).
// At least one catch clause is required (8.12).
func (p *Parser) parseTryCatchStatement() (ast.Statement, bool) {
	line := p.cur.Start.Line
	p.next() // consume "try" (TokenKwTry)

	// Parse the expression being tried. We stop at '{' and 'returns'.
	tryExpr, ok := p.parseExpression(map[lexer.Type]bool{
		lexer.TokenLBrace:  true,
		lexer.TokenKwReturns: true,
	})
	if !ok {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected expression after 'try'",
			Span:    p.span(p.cur),
		})
		return ast.Statement{}, false
	}

	// 8.11: optional returns clause: try expr returns (T x) { ... }
	if p.cur.Type == lexer.TokenKwReturns {
		p.next() // consume "returns"
		// Parse optional return variable list; discard — we only need to parse past it.
		if p.cur.Type == lexer.TokenLParen {
			_, _ = p.parseFieldList(false, true)
		}
	}

	// Parse success body.
	successBody, ok := p.parseStatementBlock("try body")
	if !ok {
		return ast.Statement{}, false
	}

	// 8.12: parse at least one catch clause.
	var catches []ast.CatchClause
	for p.cur.Type == lexer.TokenKwCatch {
		p.next() // consume "catch" (TokenKwCatch)

		clause, ok := p.parseCatchClause()
		if !ok {
			return ast.Statement{}, false
		}
		catches = append(catches, clause)
	}
	if len(catches) == 0 {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "try statement requires at least one catch clause",
			Span:    p.span(p.cur),
		})
	}

	return ast.Statement{
		Kind:    "try",
		Expr:    tryExpr,
		Body:    successBody,
		Catches: catches,
		Line:    line,
	}, true
}

// parseCatchClause parses the part after "catch" (TokenKwCatch) was consumed.
//
// Solidity-aligned form (8.13):
//
//	catch { body }                         → bare catch
//	catch (bytes memory err) { body }      → raw bytes catch
//	catch Error(string reason) {}          → named Error catch
//	catch Panic(uint256 code) {}           → named Panic catch
//	catch identifier? (paramList) { body } → general form
//
// Also accepts the legacy TOL colon form: catch Error(r: string) { }
// The optional identifier before '(' determines the kind ("Error", "Panic", etc.).
func (p *Parser) parseCatchClause() (ast.CatchClause, bool) {
	// 8.13: general form: catch [Identifier] [(paramList)] { body }
	// If the next token is neither '(' nor '{', it's an identifier qualifier.
	clauseKind := ""
	if isIdentLike(p.cur.Type) && p.cur.Type != lexer.TokenLBrace {
		clauseKind = p.cur.Literal
		p.next() // consume identifier (e.g. "Error", "Panic")
	}

	// Optional parameter list: (paramList)
	paramName := ""
	paramType := ""
	if p.cur.Type == lexer.TokenLParen {
		p.next() // consume '('
		if p.cur.Type != lexer.TokenRParen {
			// Peek ahead to detect legacy colon-separated form: "name: Type"
			// vs Solidity type-first form: "Type name".
			// If the token after the current token (an ident) is ':', it's legacy form.
			isLegacy := false
			if p.cur.Type == lexer.TokenIdent {
				// Save state for one-token lookahead.
				saved := p.lex.PeekToken()
				if saved.Type == lexer.TokenColon {
					isLegacy = true
				}
			}

			if isLegacy {
				// Legacy form: name: Type
				paramName = p.cur.Literal
				p.next() // consume name
				if !p.expect(lexer.TokenColon, diag.CodeParseUnexpected, "expected ':' in catch parameter") {
					return ast.CatchClause{}, false
				}
				paramType = p.parseTypeUntil(map[lexer.Type]bool{lexer.TokenRParen: true})
			} else {
				// Solidity type-first form: Type name
				field, ok := p.parseField(false, true)
				if ok {
					paramName = field.Name
					paramType = field.Type
				}
				// If there are more params, skip them.
				for p.cur.Type == lexer.TokenComma {
					p.next()
					_, _ = p.parseField(false, true)
				}
			}
		}
		if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after catch parameter") {
			return ast.CatchClause{}, false
		}
	}

	body, ok := p.parseStatementBlock("catch body")
	if !ok {
		return ast.CatchClause{}, false
	}

	// Determine kind: for backward compat with old "(name: bytes)" form,
	// map empty clauseKind+paramType="bytes" → Kind="bytes".
	kind := clauseKind
	if kind == "" && paramType == "bytes" {
		kind = "bytes"
	}

	return ast.CatchClause{
		Kind:      kind,
		ParamName: paramName,
		ParamType: paramType,
		Body:      body,
	}, true
}

const (
	exprPrecLowest  = 1
	exprPrecAssign  = 2
	exprPrecOr      = 3
	exprPrecAnd     = 4
	exprPrecBitOr   = 5
	exprPrecBitXor  = 6
	exprPrecBitAnd  = 7
	exprPrecCmp     = 8
	exprPrecShift   = 9
	exprPrecAdd     = 10
	exprPrecMul     = 11
	exprPrecPow     = 12 // ** — higher than *, right-associative
	exprPrecPrefix  = 13 // unary prefix operators
	exprPrecPostfix = 14 // call / member / index / ++ / --
)

func (p *Parser) parseExpression(stop map[lexer.Type]bool) (*ast.Expr, bool) {
	return p.parseExprPrec(exprPrecLowest, stop)
}

func (p *Parser) parseExprPrec(minPrec int, stop map[lexer.Type]bool) (*ast.Expr, bool) {
	left, ok := p.parsePrefixExpr(stop)
	if !ok {
		return nil, false
	}

	for {
		if p.cur.Type == lexer.TokenEOF || stop[p.cur.Type] {
			break
		}

		// Ternary operator: cond ? then : else
		// Parsed at lowest precedence (below all binary operators).
		if p.cur.Type == lexer.TokenQuestion && minPrec <= exprPrecLowest {
			p.next() // consume '?'
			// Parse 'then' branch stopping at ':'
			thenExpr, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenColon: true})
			if !ok {
				return nil, false
			}
			if !p.expect(lexer.TokenColon, diag.CodeParseUnexpected, "expected ':' in ternary expression") {
				return nil, false
			}
			elseExpr, ok := p.parseExprPrec(exprPrecLowest, stop)
			if !ok {
				return nil, false
			}
			left = &ast.Expr{Kind: "ternary", Args: []*ast.Expr{left, thenExpr, elseExpr}}
			break
		}

		if p.cur.Type == lexer.TokenLParen || p.cur.Type == lexer.TokenDot || p.cur.Type == lexer.TokenLBracket || p.cur.Type == lexer.TokenLBrace {
			if exprPrecPostfix < minPrec {
				break
			}
			left, ok = p.parsePostfixExpr(left)
			if !ok {
				return nil, false
			}
			continue
		}

		// Call options block: expr{gas: G, value: V}(args)
		// Only try this when '{' is not a stop token and the lookahead shows a
		// key-value pattern ('ident :') or empty braces ('}'). This avoids
		// accidentally consuming '{' that starts a block statement.
		if p.cur.Type == lexer.TokenLBrace && !stop[lexer.TokenLBrace] {
			nextTok := p.peek()
			if nextTok.Type == lexer.TokenIdent || nextTok.Type == lexer.TokenRBrace {
				if exprPrecPostfix < minPrec {
					break
				}
				left, ok = p.parsePostfixExpr(left)
				if !ok {
					return nil, false
				}
				continue
			}
		}

		// Postfix ++ / -- : only consume when not in the caller's stop set.
		// (parseSetStatement stops before ++ to handle it itself; other callers
		// like parseExprSemicolonStmt and the for post-step do not stop here.)
		if (p.cur.Type == lexer.TokenPlusPlus || p.cur.Type == lexer.TokenMinusMinus) && !stop[p.cur.Type] {
			if exprPrecPostfix < minPrec {
				break
			}
			op := p.cur.Literal // "++" or "--"
			p.next()
			left = &ast.Expr{Kind: "unary", Op: "post" + op, Right: left}
			continue
		}

		prec, rightAssoc := infixPrecedence(p.cur.Type)
		if prec < minPrec || prec == 0 {
			break
		}

		opTok := p.cur
		p.next()
		nextMin := prec + 1
		if rightAssoc {
			nextMin = prec
		}
		right, ok := p.parseExprPrec(nextMin, stop)
		if !ok {
			return nil, false
		}

		kind := "binary"
		if opTok.Type == lexer.TokenAssign {
			kind = "assign"
		}
		left = &ast.Expr{
			Kind:  kind,
			Op:    opTok.Literal,
			Left:  left,
			Right: right,
		}
	}
	return left, true
}

func (p *Parser) parsePrefixExpr(stop map[lexer.Type]bool) (*ast.Expr, bool) {
	if p.cur.Type == lexer.TokenEOF || stop[p.cur.Type] {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected expression",
			Span:    p.span(p.cur),
		})
		return nil, false
	}

	switch p.cur.Type {
	// 12.10: unicode string literal
	case lexer.TokenUnicodeString:
		tok := p.cur
		p.next()
		return &ast.Expr{Kind: "string", Value: tok.Literal}, true
	// Promoted keywords that appear as expressions: true, false, new
	case lexer.TokenKwTrue, lexer.TokenKwFalse:
		tok := p.cur
		p.next()
		return &ast.Expr{Kind: "ident", Value: tok.Literal}, true
	case lexer.TokenKwDeploy:
		p.next() // consume "deploy"
		if !isIdentLike(p.cur.Type) {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected contract name after 'deploy'",
				Span:    p.span(p.cur),
			})
			return nil, false
		}
		{
			deployName := p.cur.Literal
			p.next()
			if !p.expect(lexer.TokenLParen, diag.CodeParseUnexpected, "expected '(' after contract name in 'deploy' expression") {
				return nil, false
			}
			deployArgs := []*ast.Expr{}
			if p.cur.Type != lexer.TokenRParen {
				for {
					arg, ok := p.parseExpression(map[lexer.Type]bool{
						lexer.TokenComma:  true,
						lexer.TokenRParen: true,
					})
					if !ok {
						return nil, false
					}
					deployArgs = append(deployArgs, arg)
					if p.cur.Type == lexer.TokenComma {
						p.next()
					} else {
						break
					}
				}
			}
			if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' to close 'deploy' argument list") {
				return nil, false
			}
			return &ast.Expr{Kind: "new", Value: deployName, Args: deployArgs}, true
		}
	case lexer.TokenKwNew:
		p.next() // consume "new"
		if !isIdentLike(p.cur.Type) {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected contract name or type after 'new'",
				Span:    p.span(p.cur),
			})
			return nil, false
		}
		typeName := p.cur.Literal
		p.next() // consume type name
		// new T[](size) — memory array allocation.
		if p.cur.Type == lexer.TokenLBracket {
			p.next() // consume '['
			if !p.expect(lexer.TokenRBracket, diag.CodeParseUnexpected, "expected ']' in 'new T[](size)' expression") {
				return nil, false
			}
			if !p.expect(lexer.TokenLParen, diag.CodeParseUnexpected, "expected '(' after 'new T[]' for size argument") {
				return nil, false
			}
			sizeExpr, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenRParen: true})
			if !ok {
				return nil, false
			}
			if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' to close 'new T[](size)' expression") {
				return nil, false
			}
			return &ast.Expr{
				Kind:  "new_array",
				Value: typeName, // base element type
				Args:  []*ast.Expr{sizeExpr},
			}, true
		}
		// new Contract(args) — contract deployment.
		contractName := typeName
		if !p.expect(lexer.TokenLParen, diag.CodeParseUnexpected, "expected '(' after contract name in 'new' expression") {
			return nil, false
		}
		args := []*ast.Expr{}
		if p.cur.Type != lexer.TokenRParen {
			for {
				arg, ok := p.parseExpression(map[lexer.Type]bool{
					lexer.TokenComma:  true,
					lexer.TokenRParen: true,
				})
				if !ok {
					return nil, false
				}
				args = append(args, arg)
				if p.cur.Type == lexer.TokenComma {
					p.next()
				} else {
					break
				}
			}
		}
		if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' to close 'new' argument list") {
			return nil, false
		}
		return &ast.Expr{
			Kind:  "new",
			Value: contractName,
			Args:  args,
		}, true
	// Keywords that can appear in expression context (used as identifiers):
	case lexer.TokenKwAgent, lexer.TokenKwBool, lexer.TokenKwString, lexer.TokenKwUno,
		lexer.TokenKwPayable, lexer.TokenKwPure, lexer.TokenKwView,
		lexer.TokenKwPublic, lexer.TokenKwPrivate, lexer.TokenKwExternal, lexer.TokenKwInternal,
		lexer.TokenKwMemory, lexer.TokenKwCalldata, lexer.TokenKwStorage,
		lexer.TokenKwVirtual, lexer.TokenKwOverride, lexer.TokenKwGlobal,
		lexer.TokenKwReceive, lexer.TokenKwUsing, lexer.TokenKwIs, lexer.TokenKwAbstract:
		tok := p.cur
		p.next()
		return &ast.Expr{Kind: "ident", Value: tok.Literal}, true
	case lexer.TokenIdent:
		// inspect binding.slot — white-box storage slot read (test-only)
		if p.cur.Literal == "inspect" {
			p.next()
			bindingTok := p.cur
			if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected binding name after 'inspect'") {
				return nil, false
			}
			if !p.expect(lexer.TokenDot, diag.CodeParseUnexpected, "expected '.' after binding name in 'inspect'") {
				return nil, false
			}
			slotTok := p.cur
			if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected slot name after '.' in 'inspect'") {
				return nil, false
			}
			return &ast.Expr{
				Kind:   "inspect",
				Object: &ast.Expr{Kind: "ident", Value: bindingTok.Literal},
				Member: slotTok.Literal,
			}, true
		}
		// struct literal: StructName { field: expr, ... } — only when ident is a known struct name
		if _, isStruct := p.structNames[p.cur.Literal]; isStruct {
			// peek: if next token after ident is '{', parse as struct literal
			nameTok := p.cur
			p.next()
			if p.cur.Type == lexer.TokenLBrace {
				p.next() // consume '{'
				var fields []ast.StructFieldInit
				for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
					if p.cur.Type != lexer.TokenIdent {
						p.addDiag(diag.Diagnostic{
							Code:    diag.CodeParseUnexpected,
							Message: fmt.Sprintf("expected field name in struct literal, got '%s'", p.cur.Literal),
							Span:    p.span(p.cur),
						})
						break
					}
					fieldNameTok := p.cur
					p.next()
					if !p.expect(lexer.TokenColon, diag.CodeParseUnexpected, "expected ':' after field name in struct literal") {
						break
					}
					fieldExpr, ok := p.parseExpression(map[lexer.Type]bool{
						lexer.TokenComma:  true,
						lexer.TokenRBrace: true,
					})
					if !ok {
						break
					}
					fields = append(fields, ast.StructFieldInit{Name: fieldNameTok.Literal, Expr: fieldExpr})
					if p.cur.Type == lexer.TokenComma {
						p.next()
					}
				}
				if !p.expect(lexer.TokenRBrace, diag.CodeParseUnexpected, "expected '}' to close struct literal") {
					return nil, false
				}
				return &ast.Expr{
					Kind:         "struct_lit",
					Value:        nameTok.Literal,
					StructFields: fields,
				}, true
			}
			// Not a struct literal (no '{' after ident) — treat as plain ident
			return &ast.Expr{Kind: "ident", Value: nameTok.Literal}, true
		}
		tok := p.cur
		p.next()
		return &ast.Expr{Kind: "ident", Value: tok.Literal}, true
	case lexer.TokenNumber:
		tok := p.cur
		p.next()
		numVal := tok.Literal
		// SubDenomination: `1 tos`, `7 days`, etc. — compile-time constant fold.
		// "tos" is emitted as TokenIdent (not TokenSubDenom) because it is also
		// a valid package name; handle it contextually here.
		if p.cur.Type == lexer.TokenSubDenom || (p.cur.Type == lexer.TokenIdent && lexer.SubDenomMultiplier(p.cur.Literal) != "") {
			denomTok := p.cur
			mult := lexer.SubDenomMultiplier(denomTok.Literal)
			if denomTok.Literal == "years" {
				p.addDiag(diag.Diagnostic{
					Code:     diag.CodeWarnYearsUnit,
					Message:  `Using "years" as a unit denomination is deprecated. Use 365 days instead.`,
					Span:     p.span(denomTok),
					Severity: diag.SeverityWarning,
				})
			}
			p.next() // consume the denomination token
			if mult != "1" {
				numVal = mulDecimalStrings(numVal, mult)
			}
		}
		return &ast.Expr{Kind: "number", Value: numVal}, true
	case lexer.TokenString:
		tok := p.cur
		p.next()
		return &ast.Expr{Kind: "string", Value: tok.Literal}, true
	case lexer.TokenHexString:
		tok := p.cur
		p.next()
		// tok.Literal is a lowercase hex string (no "0x" prefix, no underscores).
		return &ast.Expr{Kind: "hex_lit", Value: tok.Literal}, true
	case lexer.TokenLParen:
		// Parse either a parenthesised expression or a tuple expression: (a, b, c).
		// A tuple is detected when a comma is encountered at depth 0 inside the parens.
		p.next()
		// Empty tuple: ()
		if p.cur.Type == lexer.TokenRParen {
			p.next()
			return &ast.Expr{Kind: "tuple", Args: []*ast.Expr{}}, true
		}
		first, ok := p.parseExpression(map[lexer.Type]bool{
			lexer.TokenRParen: true,
			lexer.TokenComma:  true,
		})
		if !ok {
			return nil, false
		}
		if p.cur.Type == lexer.TokenComma {
			// Tuple: collect additional elements.
			elems := []*ast.Expr{first}
			for p.cur.Type == lexer.TokenComma {
				p.next() // consume ','
				if p.cur.Type == lexer.TokenRParen {
					// trailing comma — allow empty slot (nil) for destructuring gaps
					elems = append(elems, nil)
					break
				}
				elem, ok := p.parseExpression(map[lexer.Type]bool{
					lexer.TokenRParen: true,
					lexer.TokenComma:  true,
				})
				if !ok {
					return nil, false
				}
				elems = append(elems, elem)
			}
			if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' to close tuple expression") {
				return nil, false
			}
			return &ast.Expr{Kind: "tuple", Args: elems}, true
		}
		// Single-element parenthesised expression.
		if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' to close expression") {
			return nil, false
		}
		return &ast.Expr{Kind: "paren", Left: first}, true
	case lexer.TokenLBracket:
		// Inline array expression: [expr, expr, ...]
		p.next() // consume '['
		var elems []*ast.Expr
		if p.cur.Type != lexer.TokenRBracket {
			for {
				elem, ok := p.parseExpression(map[lexer.Type]bool{
					lexer.TokenComma:    true,
					lexer.TokenRBracket: true,
				})
				if !ok {
					return nil, false
				}
				elems = append(elems, elem)
				if p.cur.Type == lexer.TokenComma {
					p.next()
					continue
				}
				break
			}
		}
		if !p.expect(lexer.TokenRBracket, diag.CodeParseUnexpected, "expected ']' to close inline array") {
			return nil, false
		}
		return &ast.Expr{Kind: "array_lit", Args: elems}, true
	case lexer.TokenPlus, lexer.TokenMinus, lexer.TokenBang, lexer.TokenBitNot:
		op := p.cur.Literal
		p.next()
		right, ok := p.parseExprPrec(exprPrecPrefix, stop)
		if !ok {
			return nil, false
		}
		return &ast.Expr{Kind: "unary", Op: op, Right: right}, true
	case lexer.TokenKwType:
		// type(I).interfaceId  or  type(T).min / type(T).max
		tok := p.cur
		p.next()
		return &ast.Expr{Kind: "ident", Value: tok.Literal}, true
	case lexer.TokenKwMapping:
		// mapping used as expression (shouldn't normally happen but be safe)
		tok := p.cur
		p.next()
		return &ast.Expr{Kind: "ident", Value: tok.Literal}, true
	default:
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: fmt.Sprintf("unexpected token '%s' in expression", p.cur.Literal),
			Span:    p.span(p.cur),
		})
		return nil, false
	}
}

// parseCallOptionsBlock parses a call options block: { key: value, key: value }
// The '{' has not been consumed yet. Returns the parsed options and true on success.
// Valid keys are "gas" and "value".
func (p *Parser) parseCallOptionsBlock() ([]ast.CallOption, bool) {
	if !p.expect(lexer.TokenLBrace, diag.CodeParseUnexpected, "expected '{' for call options") {
		return nil, false
	}
	var options []ast.CallOption
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		// Expect: ident ':' expr
		keyTok := p.cur
		if keyTok.Type != lexer.TokenIdent {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("expected option key (gas or value) in call options block, got '%s'", keyTok.Literal),
				Span:    p.span(keyTok),
			})
			return nil, false
		}
		p.next() // consume key
		if !p.expect(lexer.TokenColon, diag.CodeParseUnexpected, "expected ':' after call option key") {
			return nil, false
		}
		val, ok := p.parseExpression(map[lexer.Type]bool{
			lexer.TokenComma:  true,
			lexer.TokenRBrace: true,
		})
		if !ok {
			return nil, false
		}
		options = append(options, ast.CallOption{Key: keyTok.Literal, Value: val})
		if p.cur.Type == lexer.TokenComma {
			p.next()
			continue
		}
		break
	}
	if !p.expect(lexer.TokenRBrace, diag.CodeParseUnexpected, "expected '}' to close call options block") {
		return nil, false
	}
	return options, true
}

// isNamedArgBlock peeks ahead to determine if the current '{' starts a named-arg block.
// A named-arg block looks like: { ident : ... } or { } (empty).
// This is used to identify f({name: val, ...}) Solidity-style named call arguments.
func (p *Parser) isNamedArgBlock() bool {
	// p.cur is '{'; peek at the token immediately after '{'.
	next := p.peek()
	// Empty block → named arg (empty named args).
	if next.Type == lexer.TokenRBrace {
		return true
	}
	// ident followed by ':' → named arg key-value pair.
	if next.Type == lexer.TokenIdent {
		return p.lex.PeekSecond().Type == lexer.TokenColon
	}
	return false
}

// parseNamedArgBlock parses { name: expr, name: expr, ... } as named call arguments.
// The '{' has not been consumed yet.
func (p *Parser) parseNamedArgBlock() ([]ast.NamedArg, bool) {
	if !p.expect(lexer.TokenLBrace, diag.CodeParseUnexpected, "expected '{' for named arguments") {
		return nil, false
	}
	var namedArgs []ast.NamedArg
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		keyTok := p.cur
		if keyTok.Type != lexer.TokenIdent {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("expected argument name in named call, got '%s'", keyTok.Literal),
				Span:    p.span(keyTok),
			})
			return nil, false
		}
		p.next() // consume name
		if !p.expect(lexer.TokenColon, diag.CodeParseUnexpected, "expected ':' after argument name") {
			return nil, false
		}
		val, ok := p.parseExpression(map[lexer.Type]bool{
			lexer.TokenComma:  true,
			lexer.TokenRBrace: true,
		})
		if !ok {
			return nil, false
		}
		namedArgs = append(namedArgs, ast.NamedArg{Name: keyTok.Literal, Expr: val})
		if p.cur.Type == lexer.TokenComma {
			p.next()
			continue
		}
		break
	}
	if !p.expect(lexer.TokenRBrace, diag.CodeParseUnexpected, "expected '}' to close named argument block") {
		return nil, false
	}
	return namedArgs, true
}

func (p *Parser) parsePostfixExpr(left *ast.Expr) (*ast.Expr, bool) {
	switch p.cur.Type {
	case lexer.TokenLBrace:
		// Call options block: expr{gas: X, value: Y}(args)
		// The '{' must be followed by 'key: value' pairs and '}', then '('.
		// This is only valid when immediately followed by '(' (call options syntax).
		// We parse speculatively.
		options, ok := p.parseCallOptionsBlock()
		if !ok {
			return nil, false
		}
		// After the options block, expect '(' for the argument list.
		if p.cur.Type != lexer.TokenLParen {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("expected '(' after call options block, got '%s'", p.cur.Literal),
				Span:    p.span(p.cur),
			})
			return nil, false
		}
		p.next() // consume '('
		args := []*ast.Expr{}
		if p.cur.Type != lexer.TokenRParen {
			for {
				arg, ok := p.parseExpression(map[lexer.Type]bool{
					lexer.TokenComma:  true,
					lexer.TokenRParen: true,
				})
				if !ok {
					return nil, false
				}
				args = append(args, arg)
				if p.cur.Type == lexer.TokenComma {
					p.next()
					continue
				}
				break
			}
		}
		if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after argument list") {
			return nil, false
		}
		return &ast.Expr{Kind: "call", Callee: left, Args: args, Options: options}, true
	case lexer.TokenLParen:
		p.next()
		// Named call arguments: f({to: alice, amount: 100})
		// Detected when the argument list starts with '{' followed by 'ident :' or '}'.
		if p.cur.Type == lexer.TokenLBrace && p.isNamedArgBlock() {
			namedArgs, ok := p.parseNamedArgBlock()
			if !ok {
				return nil, false
			}
			if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after named argument block") {
				return nil, false
			}
			return &ast.Expr{Kind: "named_call", Callee: left, NamedArgs: namedArgs}, true
		}
		args := []*ast.Expr{}
		if p.cur.Type != lexer.TokenRParen {
			for {
				arg, ok := p.parseExpression(map[lexer.Type]bool{
					lexer.TokenComma:  true,
					lexer.TokenRParen: true,
				})
				if !ok {
					return nil, false
				}
				args = append(args, arg)
				if p.cur.Type == lexer.TokenComma {
					p.next()
					continue
				}
				break
			}
		}
		if !p.expect(lexer.TokenRParen, diag.CodeParseUnexpected, "expected ')' after argument list") {
			return nil, false
		}
		return &ast.Expr{Kind: "call", Callee: left, Args: args}, true
	case lexer.TokenDot:
		p.next()
		memberTok := p.cur
		// Allow keyword tokens as member names (e.g. token.address, IERC20.TransferEvent).
		// In Solidity, keywords like 'address', 'type', 'function' can appear as member names.
		if p.cur.Type == lexer.TokenEOF {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: "expected member name after '.'",
				Span:    p.span(p.cur),
			})
			return nil, false
		}
		p.next() // accept any non-EOF token as member name (including keywords like 'agent')
		memberName := memberTok.Literal
		// msg.agent → special Kind "msg_agent" (zero-agent fallback, no revert on unregistered).
		if memberName == "agent" &&
			left != nil && left.Kind == "ident" && strings.TrimSpace(left.Value) == "msg" {
			return &ast.Expr{Kind: "msg_agent"}, true
		}
		return &ast.Expr{Kind: "member", Object: left, Member: memberName}, true
	case lexer.TokenLBracket:
		p.next()
		// Check for slice notation: expr[start:end]
		startExpr, ok := p.parseExpression(map[lexer.Type]bool{
			lexer.TokenRBracket: true,
			lexer.TokenColon:    true,
		})
		if !ok {
			return nil, false
		}
		if p.cur.Type == lexer.TokenColon {
			p.next() // consume ':'
			endExpr, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenRBracket: true})
			if !ok {
				return nil, false
			}
			if !p.expect(lexer.TokenRBracket, diag.CodeParseUnexpected, "expected ']' after slice expression") {
				return nil, false
			}
			return &ast.Expr{Kind: "slice", Object: left, Args: []*ast.Expr{startExpr, endExpr}}, true
		}
		if !p.expect(lexer.TokenRBracket, diag.CodeParseUnexpected, "expected ']' after index expression") {
			return nil, false
		}
		return &ast.Expr{Kind: "index", Object: left, Index: startExpr}, true
	default:
		return left, true
	}
}

func infixPrecedence(tt lexer.Type) (int, bool) {
	switch tt {
	case lexer.TokenAssign:
		return exprPrecAssign, true
	case lexer.TokenOrOr:
		return exprPrecOr, false
	case lexer.TokenAndAnd:
		return exprPrecAnd, false
	case lexer.TokenBitOr:
		return exprPrecBitOr, false
	case lexer.TokenBitXor:
		return exprPrecBitXor, false
	case lexer.TokenBitAnd:
		return exprPrecBitAnd, false
	case lexer.TokenEq, lexer.TokenNe, lexer.TokenLT, lexer.TokenLE, lexer.TokenGT, lexer.TokenGE:
		return exprPrecCmp, false
	case lexer.TokenShl, lexer.TokenSar, lexer.TokenShr:
		return exprPrecShift, false
	case lexer.TokenPlus, lexer.TokenMinus:
		return exprPrecAdd, false
	case lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent:
		return exprPrecMul, false
	case lexer.TokenPow:
		return exprPrecPow, true // right-associative: 2 ** 3 ** 4 == 2 ** (3 ** 4)
	default:
		return 0, false
	}
}

func (p *Parser) syncStatement() {
	for p.cur.Type != lexer.TokenEOF && p.cur.Type != lexer.TokenRBrace {
		if p.cur.Type == lexer.TokenSemicolon {
			p.next()
			return
		}
		if p.cur.Type == lexer.TokenLBrace {
			_ = p.consumeBlock("invalid nested block")
			return
		}
		p.next()
	}
}

func (p *Parser) consumeBlock(what string) bool {
	if p.cur.Type != lexer.TokenLBrace {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected '{' before " + what,
			Span:    p.span(p.cur),
		})
		return false
	}
	p.next()
	depth := 1
	for p.cur.Type != lexer.TokenEOF {
		switch p.cur.Type {
		case lexer.TokenLBrace:
			depth++
		case lexer.TokenRBrace:
			depth--
			if depth == 0 {
				p.next()
				return true
			}
		}
		p.next()
	}
	p.addDiag(diag.Diagnostic{
		Code:    diag.CodeParseUnexpected,
		Message: "unexpected EOF while parsing " + what,
		Span:    p.span(p.cur),
	})
	return false
}

func (p *Parser) consumePaired(open, close lexer.Type, what string) bool {
	if p.cur.Type != open {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected opening token before " + what,
			Span:    p.span(p.cur),
		})
		return false
	}
	p.next()
	depth := 1
	for p.cur.Type != lexer.TokenEOF {
		switch p.cur.Type {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				p.next()
				return true
			}
		}
		p.next()
	}
	p.addDiag(diag.Diagnostic{
		Code:    diag.CodeParseUnexpected,
		Message: "unexpected EOF while parsing " + what,
		Span:    p.span(p.cur),
	})
	return false
}

func (p *Parser) syncUnknownMember() {
	for p.cur.Type != lexer.TokenEOF && p.cur.Type != lexer.TokenRBrace {
		if p.isContractMemberStart(p.cur.Type) {
			return
		}
		if p.cur.Type == lexer.TokenLBrace {
			_ = p.consumeBlock("unknown member")
			return
		}
		if p.cur.Type == lexer.TokenSemicolon {
			p.next()
			return
		}
		p.next()
	}
}

// isTypeStart reports whether the current token can begin a type expression,
// and therefore a state variable declaration at the contract body level.
func (p *Parser) isTypeStart() bool {
	return isTypeStartToken(p.cur.Type, p.cur.Literal)
}

// isTypeStartToken reports whether a token type/literal can begin a type expression.
// Static version of isTypeStart for use with peeked tokens.
func isTypeStartToken(t lexer.Type, literal string) bool {
	switch t {
	case lexer.TokenKwMapping, lexer.TokenIdent,
		lexer.TokenKwAgent, lexer.TokenKwBool, lexer.TokenKwString, lexer.TokenKwUno,
		lexer.TokenKwMemory, lexer.TokenKwCalldata, lexer.TokenKwStorage:
		return true
	}
	return false
}

// isTestBlockKeyword returns true for contextual keywords used in test blocks
// (setup, teardown, mock, etc.) that should not be treated as type names.
func isTestBlockKeyword(literal string) bool {
	switch literal {
	case "setup", "setup_suite", "teardown", "teardown_suite", "mock":
		return true
	}
	return false
}

// isIdentLike reports whether a token type can be used as an identifier in
// contexts where keywords are allowed as names (e.g. function names, variable
// names).  This is needed because several keywords were promoted from
// contextual (TokenIdent) to reserved token types but still appear as names
// in various positions.
func isIdentLike(t lexer.Type) bool {
	switch t {
	case lexer.TokenIdent,
		// Keywords that can appear as identifiers in various contexts
		lexer.TokenKwAbstract, lexer.TokenKwAgent, lexer.TokenKwAnonymous,
		lexer.TokenKwBool, lexer.TokenKwCalldata, lexer.TokenKwDelete, lexer.TokenKwUno,
		lexer.TokenKwExternal, lexer.TokenKwFalse, lexer.TokenKwGlobal,
		lexer.TokenKwIndexed, lexer.TokenKwInternal, lexer.TokenKwIs,
		lexer.TokenKwMemory, lexer.TokenKwNew, lexer.TokenKwDeploy, lexer.TokenKwOverride,
		lexer.TokenKwPayable, lexer.TokenKwPrivate, lexer.TokenKwPublic,
		lexer.TokenKwPure, lexer.TokenKwReceive, lexer.TokenKwStorage,
		lexer.TokenKwString, lexer.TokenKwTrue, lexer.TokenKwUnchecked,
		lexer.TokenKwUsing, lexer.TokenKwView, lexer.TokenKwVirtual,
		lexer.TokenKwTry, lexer.TokenKwCatch:
		return true
	}
	return false
}

// isDataLocationKeyword reports whether the current token is a data-location
// keyword (memory, calldata, storage), now that these are promoted.
func isDataLocationKeyword(t lexer.Type) bool {
	switch t {
	case lexer.TokenKwMemory, lexer.TokenKwCalldata, lexer.TokenKwStorage:
		return true
	}
	return false
}

// isFunctionTypeModifierToken reports whether the token type is a function-type
// modifier keyword (external, internal, view, pure, payable).
func isFunctionTypeModifierToken(t lexer.Type) bool {
	switch t {
	case lexer.TokenKwExternal, lexer.TokenKwInternal, lexer.TokenKwView,
		lexer.TokenKwPure, lexer.TokenKwPayable, lexer.TokenKwPublic, lexer.TokenKwPrivate:
		return true
	}
	return false
}

func (p *Parser) isContractMemberStart(tt lexer.Type) bool {
	switch tt {
	case lexer.TokenKwEvent,
		lexer.TokenKwFunction,
		lexer.TokenKwConstructor,
		lexer.TokenKwFallback,
		lexer.TokenKwError,
		lexer.TokenKwEnum,
		lexer.TokenKwModifier,
		lexer.TokenKwImmutable,
		lexer.TokenKwConstant,
		lexer.TokenKwStruct,
		lexer.TokenKwTransient,
		lexer.TokenKwMapping,
		lexer.TokenIdent,
		// Promoted keywords that can start contract members:
		lexer.TokenKwUsing, lexer.TokenKwReceive,
		lexer.TokenKwAgent, lexer.TokenKwBool, lexer.TokenKwString, lexer.TokenKwUno:
		return true
	default:
		return false
	}
}

// parseImmutableDecl parses an immutable variable declaration:
//
//	immutable NAME: TYPE;
func (p *Parser) parseImmutableDecl() *ast.ImmutableDecl {
	if !p.expect(lexer.TokenKwImmutable, diag.CodeParseUnexpected, "expected 'immutable'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected immutable variable name") {
		return nil
	}
	if !p.expect(lexer.TokenColon, diag.CodeParseUnexpected, "expected ':' after immutable variable name") {
		return nil
	}
	typ := p.parseTypeUntil(map[lexer.Type]bool{
		lexer.TokenSemicolon: true,
	})
	if typ == "" {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected type after ':' in immutable declaration",
			Span:    p.span(p.cur),
		})
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after immutable declaration") {
		return nil
	}
	return &ast.ImmutableDecl{Name: nameTok.Literal, Type: typ}
}

// parseConstantDecl parses a compile-time constant declaration:
//
//	constant NAME: TYPE = LITERAL;
func (p *Parser) parseConstantDecl() *ast.ConstantDecl {
	if !p.expect(lexer.TokenKwConstant, diag.CodeParseUnexpected, "expected 'constant'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected constant name") {
		return nil
	}
	if !p.expect(lexer.TokenColon, diag.CodeParseUnexpected, "expected ':' after constant name") {
		return nil
	}
	typ := p.parseTypeUntil(map[lexer.Type]bool{
		lexer.TokenAssign:    true,
		lexer.TokenSemicolon: true,
	})
	if typ == "" {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected type after ':' in constant declaration",
			Span:    p.span(p.cur),
		})
	}
	if !p.expect(lexer.TokenAssign, diag.CodeParseUnexpected, "expected '=' after type in constant declaration") {
		return nil
	}
	val, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
	if !ok || val == nil {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected literal value after '=' in constant declaration",
			Span:    p.span(p.cur),
		})
		return nil
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after constant declaration") {
		return nil
	}
	return &ast.ConstantDecl{Name: nameTok.Literal, Type: typ, Value: val}
}

// parseImmutableDeclTypeFirst parses a type-first immutable declaration:
//
//	Type immutable NAME;
//
// Example: uint256 immutable owner;
func (p *Parser) parseImmutableDeclTypeFirst() *ast.ImmutableDecl {
	// Parse the leading type (everything up to the 'immutable' keyword).
	typ := p.parseTypeUntil(map[lexer.Type]bool{
		lexer.TokenKwImmutable: true,
	})
	if typ == "" {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected type before 'immutable'",
			Span:    p.span(p.cur),
		})
		return nil
	}
	if !p.expect(lexer.TokenKwImmutable, diag.CodeParseUnexpected, "expected 'immutable'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected immutable variable name") {
		return nil
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after immutable declaration") {
		return nil
	}
	return &ast.ImmutableDecl{Name: nameTok.Literal, Type: typ}
}

// parseConstantDeclTypeFirst parses a type-first constant declaration:
//
//	Type constant NAME = LITERAL;
//
// Example: uint256 constant MAX_SUPPLY = 1000000;
func (p *Parser) parseConstantDeclTypeFirst() *ast.ConstantDecl {
	// Parse the leading type (everything up to the 'constant' keyword).
	typ := p.parseTypeUntil(map[lexer.Type]bool{
		lexer.TokenKwConstant: true,
	})
	if typ == "" {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected type before 'constant'",
			Span:    p.span(p.cur),
		})
		return nil
	}
	if !p.expect(lexer.TokenKwConstant, diag.CodeParseUnexpected, "expected 'constant'") {
		return nil
	}
	nameTok := p.cur
	if !p.expect(lexer.TokenIdent, diag.CodeParseUnexpected, "expected constant name") {
		return nil
	}
	if !p.expect(lexer.TokenAssign, diag.CodeParseUnexpected, "expected '=' after constant name") {
		return nil
	}
	val, ok := p.parseExpression(map[lexer.Type]bool{lexer.TokenSemicolon: true})
	if !ok || val == nil {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected literal value after '=' in constant declaration",
			Span:    p.span(p.cur),
		})
		return nil
	}
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after constant declaration") {
		return nil
	}
	return &ast.ConstantDecl{Name: nameTok.Literal, Type: typ, Value: val}
}

func (p *Parser) expect(tt lexer.Type, code, message string) bool {
	// When an identifier is expected but we see a reserved keyword, emit a
	// specific error rather than the generic "expected X" message.
	if tt == lexer.TokenIdent && p.cur.Type == lexer.TokenReserved {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: fmt.Sprintf("reserved keyword %q cannot be used as identifier", p.cur.Literal),
			Span:    p.span(p.cur),
		})
		return false
	}
	if p.cur.Type != tt {
		p.addDiag(diag.Diagnostic{
			Code:    code,
			Message: message,
			Span:    p.span(p.cur),
		})
		return false
	}
	p.next()
	return true
}

func (p *Parser) syncUntil(types ...lexer.Type) {
	for p.cur.Type != lexer.TokenEOF {
		for _, tt := range types {
			if p.cur.Type == tt {
				return
			}
		}
		p.next()
	}
}

// next advances to the next non-doc-comment token, accumulating any
// TokenDocComment tokens into pendingDoc along the way.
// If the incoming non-doc token is not a declaration preamble token,
// pendingDoc is discarded (binding only survives to the immediately
// following declaration).
func (p *Parser) next() {
	for {
		p.cur = p.lex.Next()
		if p.cur.Type != lexer.TokenDocComment {
			break
		}
		// Accumulate consecutive doc-comment tokens into pendingDoc.
		parsed := parseDocMeta(p.cur.Literal)
		if p.pendingDoc == nil {
			p.pendingDoc = parsed
		} else {
			// Merge: append all slices from parsed into pendingDoc.
			if parsed.Notice != "" {
				p.pendingDoc.Notice = parsed.Notice
			}
			p.pendingDoc.Params = append(p.pendingDoc.Params, parsed.Params...)
			p.pendingDoc.Returns = append(p.pendingDoc.Returns, parsed.Returns...)
			if parsed.Effects != nil {
				if p.pendingDoc.Effects == nil {
					p.pendingDoc.Effects = parsed.Effects
				} else {
					p.pendingDoc.Effects.Reads = append(p.pendingDoc.Effects.Reads, parsed.Effects.Reads...)
					p.pendingDoc.Effects.Writes = append(p.pendingDoc.Effects.Writes, parsed.Effects.Writes...)
					p.pendingDoc.Effects.Emits = append(p.pendingDoc.Effects.Emits, parsed.Effects.Emits...)
					if parsed.Effects.Calls != nil {
						p.pendingDoc.Effects.Calls = append(p.pendingDoc.Effects.Calls, parsed.Effects.Calls...)
					}
				}
			}
			if parsed.Bounds != nil {
				if p.pendingDoc.Bounds == nil {
					p.pendingDoc.Bounds = parsed.Bounds
				} else {
					p.pendingDoc.Bounds.Constraints = append(p.pendingDoc.Bounds.Constraints, parsed.Bounds.Constraints...)
				}
			}
			if parsed.Gas != nil {
				p.pendingDoc.Gas = parsed.Gas
			}
			// Agent-native annotation fields.
			p.pendingDoc.RequiresCap = append(p.pendingDoc.RequiresCap, parsed.RequiresCap...)
			if parsed.Delegated {
				p.pendingDoc.Delegated = true
			}
			if parsed.Verifiable {
				p.pendingDoc.Verifiable = true
			}
			if parsed.HasPay {
				p.pendingDoc.HasPay = true
			}
			if parsed.PayAmount != "" {
				p.pendingDoc.PayAmount = parsed.PayAmount
			}
			if parsed.PayRecipient != "" {
				p.pendingDoc.PayRecipient = parsed.PayRecipient
			}
			if parsed.QuotaCalls != "" {
				p.pendingDoc.QuotaCalls = parsed.QuotaCalls
			}
			if parsed.QuotaPrice != "" {
				p.pendingDoc.QuotaPrice = parsed.QuotaPrice
			}
			if parsed.TotalCostMax != "" {
				p.pendingDoc.TotalCostMax = parsed.TotalCostMax
			}
		}
	}
	// Clear pendingDoc if the token is not part of a declaration preamble.
	// Tokens that keep pendingDoc alive: TokenKwFunction, TokenKwConstructor,
	// TokenKwFallback, TokenKwEvent, TokenAt (for @selector before function),
	// and TokenKwInterface (doc on interface function sigs).
	p.clearPendingDocOnNonDecl()
}

// peekTok returns the next non-doc-comment token without consuming it.
// This is a one-token lookahead used to disambiguate syntax forms.
func (p *Parser) peekTok() lexer.Token {
	return p.lex.PeekNextNonDoc()
}

// peek returns the next token from the lexer without consuming it.
// This is used for one-token lookahead in disambiguation (e.g. type-first local var decl).
// Doc-comment tokens at statement level are rare; this returns the raw next token
// which is sufficient for the type-ident disambiguation needed in parseStatement.
func (p *Parser) peek() lexer.Token {
	return p.lex.PeekToken()
}

// takePendingDoc returns and clears pendingDoc.
func (p *Parser) takePendingDoc() *ast.DocMeta {
	d := p.pendingDoc
	p.pendingDoc = nil
	return d
}

// clearPendingDocOnNonDecl discards pendingDoc when a non-declaration token is consumed.
func (p *Parser) clearPendingDocOnNonDecl() {
	switch p.cur.Type {
	case lexer.TokenKwFunction, lexer.TokenKwConstructor, lexer.TokenKwFallback,
		lexer.TokenKwEvent, lexer.TokenKwInterface, lexer.TokenKwContract,
		lexer.TokenAt,              // @selector or other attributes before function
		lexer.TokenKwReceive,       // receive() payable { ... } — now a keyword
		lexer.TokenKwAbstract:      // abstract contract — now a keyword
		// do NOT clear — binding survives attribute tokens too
	default:
		p.pendingDoc = nil
	}
}

// parseDocMeta parses the raw text of a TokenDocComment into a *ast.DocMeta.
// Handles both /// and /** */ style raw literals.
func parseDocMeta(raw string) *ast.DocMeta {
	meta := &ast.DocMeta{}
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		// Strip /// prefix or block-comment decoration (* prefix).
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "///")
		line = strings.TrimPrefix(line, "/**")
		line = strings.TrimPrefix(line, "*/")
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Match @tag or @tag key: value or @tag(args)
		if !strings.HasPrefix(line, "@") {
			continue
		}
		line = line[1:] // strip @
		// Split into tag and rest.
		// Delimiters: space, tab, or '(' (for @requires(caller: X) style).
		splitIdx := strings.IndexAny(line, " \t(")
		var tag, rest string
		if splitIdx < 0 {
			tag = line
		} else {
			tag = line[:splitIdx]
			if line[splitIdx] == '(' {
				// Keep the '(' as part of rest so parseRequiresTag/parsePayTag can strip it.
				rest = strings.TrimSpace(line[splitIdx:])
			} else {
				rest = strings.TrimSpace(line[splitIdx+1:])
			}
		}
		switch tag {
		case "notice":
			meta.Notice = rest
		case "param":
			name, text, _ := strings.Cut(rest, " ")
			meta.Params = append(meta.Params, ast.DocParam{Name: strings.TrimSpace(name), Text: strings.TrimSpace(text)})
		case "return":
			name, text, _ := strings.Cut(rest, " ")
			meta.Returns = append(meta.Returns, ast.DocParam{Name: strings.TrimSpace(name), Text: strings.TrimSpace(text)})
		case "effects":
			parseEffectsTag(meta, rest)
		case "bounds":
			parseBoundsTag(meta, rest)
		case "gas":
			parseGasTag(meta, rest)
		case "requires":
			parseRequiresTag(meta, rest)
		case "pay":
			parsePayTag(meta, rest)
		case "delegated":
			meta.Delegated = true
		case "verifiable":
			meta.Verifiable = true
		case "quota":
			parseQuotaTag(meta, rest)
		case "total_cost":
			parseTotalCostTag(meta, rest)
		}
	}
	// Return nil if nothing was parsed.
	if meta.Notice == "" && len(meta.Params) == 0 && len(meta.Returns) == 0 &&
		meta.Effects == nil && meta.Bounds == nil && meta.Gas == nil &&
		len(meta.RequiresCap) == 0 && !meta.Delegated && !meta.Verifiable &&
		!meta.HasPay && meta.PayAmount == "" &&
		meta.QuotaCalls == "" && meta.TotalCostMax == "" {
		return nil
	}
	return meta
}

// parseEffectsTag parses one or more "@effects key: values" clauses from a single line.
// Multiple clauses may appear on one line separated by commas outside brackets:
//   @effects guards: [onlyOwner], writes: [storage.balance]
func parseEffectsTag(meta *ast.DocMeta, rest string) {
	// rest is like "reads:  storage.balances[caller], storage.x" or "calls: []"
	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		return
	}
	key := strings.TrimSpace(rest[:colonIdx])
	val := strings.TrimSpace(rest[colonIdx+1:])

	// When the value starts with '[', find the matching ']' and check if there
	// are additional clauses after it (e.g. "], writes: [...]").
	var tail string
	if strings.HasPrefix(val, "[") {
		bracketDepth := 0
		for i := 0; i < len(val); i++ {
			switch val[i] {
			case '[':
				bracketDepth++
			case ']':
				bracketDepth--
				if bracketDepth == 0 {
					// Everything after the closing bracket is a potential tail clause.
					after := strings.TrimSpace(val[i+1:])
					val = val[:i+1]
					if strings.HasPrefix(after, ",") {
						tail = strings.TrimSpace(after[1:])
					}
					goto done
				}
			}
		}
	}
done:

	// Strip surrounding brackets from val for bracket-enclosed lists: [storage.x] → storage.x
	// This is needed when multiple clauses appear on one line: guards: [onlyOwner], writes: [storage.x]
	strippedVal := val
	if strings.HasPrefix(strippedVal, "[") && strings.HasSuffix(strippedVal, "]") {
		strippedVal = strippedVal[1 : len(strippedVal)-1]
	}

	if meta.Effects == nil {
		meta.Effects = &ast.EffectDecl{}
	}
	switch key {
	case "reads":
		if val != "" && val != "[]" {
			meta.Effects.Reads = append(meta.Effects.Reads, parseCommaRefs(strippedVal)...)
		}
	case "writes":
		if val != "" && val != "[]" {
			meta.Effects.Writes = append(meta.Effects.Writes, parseCommaRefs(strippedVal)...)
		}
	case "emits":
		// Event names are case-sensitive (PascalCase); do NOT canonicalize.
		if val != "" && val != "[]" {
			for _, name := range strings.Split(strippedVal, ",") {
				name = strings.TrimSpace(name)
				if name != "" {
					meta.Effects.Emits = append(meta.Effects.Emits, name)
				}
			}
		}
	case "calls":
		if val == "[]" {
			if meta.Effects.Calls == nil {
				meta.Effects.Calls = []ast.CallRef{} // declared empty
			}
		} else if val != "" {
			cr := parseCallRef(val)
			meta.Effects.Calls = append(meta.Effects.Calls, cr)
		}
	case "guards":
		// Parse guard modifier names: guards: [onlyOwner, onlyAdmin] or guards: onlyOwner, onlyAdmin
		if val != "" && val != "[]" {
			for _, name := range strings.Split(strippedVal, ",") {
				name = strings.TrimSpace(name)
				if name != "" {
					meta.Effects.Guards = append(meta.Effects.Guards, name)
				}
			}
		}
	}
	// If there was a tail clause (multi-clause line), recursively parse it.
	if tail != "" {
		parseEffectsTag(meta, tail)
	}
}

// parseCommaRefs splits a comma-separated list of storage/event refs and canonicalizes each.
// Commas inside brackets are NOT treated as separators, so nested-mapping refs like
// "storage.allowances[caller,to]" are kept as a single item.
func parseCommaRefs(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])

	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = canonicalizeRef(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// canonicalizeRef normalizes a storage ref: lowercase, no spaces inside brackets.
func canonicalizeRef(ref string) string {
	// Remove spaces around [ ] ,
	var b strings.Builder
	for i := 0; i < len(ref); i++ {
		ch := ref[i]
		if ch == ' ' || ch == '\t' {
			continue
		}
		b.WriteByte(ch)
	}
	return strings.ToLower(b.String())
}

// parseCallRef parses a single @effects calls: item.
// Format: "cap:OracleCap iface:IOracle selector:0x12345678 max_gas:3000 max_calls:1 max_depth:1"
// or just "*" for wildcard.
func parseCallRef(s string) ast.CallRef {
	s = strings.TrimSpace(s)
	if s == "*" {
		return ast.CallRef{Wildcard: true}
	}
	var cr ast.CallRef
	fields := strings.Fields(s)
	for _, f := range fields {
		k, v, ok := strings.Cut(f, ":")
		if !ok {
			continue
		}
		switch k {
		case "cap":
			cr.Cap = v
		case "iface":
			cr.Iface = v
		case "selector":
			cr.Selector = v
		case "max_gas":
			fmt.Sscanf(v, "%d", &cr.MaxGas)
		case "max_calls":
			var n uint32
			fmt.Sscanf(v, "%d", &n)
			cr.MaxCalls = n
		case "max_depth":
			var n uint32
			fmt.Sscanf(v, "%d", &n)
			cr.MaxDepth = n
		}
	}
	return cr
}

// parseBoundsTag parses "@bounds ident <= N, ident2 == M".
func parseBoundsTag(meta *ast.DocMeta, rest string) {
	if meta.Bounds == nil {
		meta.Bounds = &ast.BoundsDecl{}
	}
	parts := strings.Split(rest, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		var bc ast.BoundConstraint
		if idx := strings.Index(p, "<="); idx >= 0 {
			bc.Ident = strings.TrimSpace(p[:idx])
			bc.Op = "<="
			fmt.Sscanf(strings.TrimSpace(p[idx+2:]), "%d", &bc.Value)
		} else if idx := strings.Index(p, "=="); idx >= 0 {
			bc.Ident = strings.TrimSpace(p[:idx])
			bc.Op = "=="
			fmt.Sscanf(strings.TrimSpace(p[idx+2:]), "%d", &bc.Value)
		} else {
			continue
		}
		if bc.Ident != "" {
			meta.Bounds.Constraints = append(meta.Bounds.Constraints, bc)
		}
	}
}

// parseGasTag parses "@gas upper: N" or "@gas upper: expr".
// The value after "upper:" is treated as a concrete integer only when the
// entire value string (after trimming whitespace) is a pure decimal integer.
// Any value that contains non-digit characters is stored as a parametric
// expression string in GasDecl.Expr.
func parseGasTag(meta *ast.DocMeta, rest string) {
	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		return
	}
	key := strings.TrimSpace(rest[:colonIdx])
	val := strings.TrimSpace(rest[colonIdx+1:])
	if key != "upper" {
		return
	}
	if meta.Gas == nil {
		meta.Gas = &ast.GasDecl{}
	}
	// Use strconv.ParseUint to ensure the entire value string is a pure
	// decimal integer (fmt.Sscanf would accept a leading integer even when
	// the string contains additional non-digit characters).
	if n, err := strconv.ParseUint(val, 10, 64); err == nil {
		meta.Gas.Upper = n
	} else {
		meta.Gas.Expr = val
	}
}

// parseRequiresTag parses "@requires(caller: X)" into meta.RequiresCap.
// Multiple @requires lines accumulate capability names.
func parseRequiresTag(meta *ast.DocMeta, rest string) {
	// rest is like "(caller: CapName)" or "caller: CapName"
	s := strings.TrimSpace(rest)
	s = strings.TrimPrefix(s, "(")
	s = strings.TrimSuffix(s, ")")
	colonIdx := strings.Index(s, ":")
	if colonIdx < 0 {
		// bare name: @requires CapName
		name := strings.TrimSpace(s)
		if name != "" {
			meta.RequiresCap = append(meta.RequiresCap, name)
		}
		return
	}
	// key: value — key should be "caller"
	val := strings.TrimSpace(s[colonIdx+1:])
	if val != "" {
		meta.RequiresCap = append(meta.RequiresCap, val)
	}
}

// parsePayTag parses "@pay(...)" into meta.PayAmount / PayRecipient.
// Supported forms:
//
//	@pay(amount=X, recipient=Y)     — named keys with '='
//	@pay(X)                         — bare amount (PayIsBare=true, PayRecipient left empty)
//	@pay(X, recipient: Y)           — positional amount + named recipient with ':'
func parsePayTag(meta *ast.DocMeta, rest string) {
	meta.HasPay = true
	s := strings.TrimSpace(rest)
	s = strings.TrimPrefix(s, "(")
	s = strings.TrimSuffix(s, ")")
	s = strings.TrimSpace(s)
	// Named-key form: contains '=' separator.
	if strings.Contains(s, "=") {
		for _, part := range strings.Split(s, ",") {
			part = strings.TrimSpace(part)
			k, v, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			switch strings.TrimSpace(k) {
			case "amount":
				meta.PayAmount = strings.TrimSpace(v)
			case "recipient":
				meta.PayRecipient = strings.TrimSpace(v)
			}
		}
		return
	}
	// Mixed positional + named form: @pay(X, recipient: Y)
	// Split on first ',' to get the positional amount and the rest.
	if idx := strings.Index(s, ","); idx >= 0 {
		amount := strings.TrimSpace(s[:idx])
		tail := strings.TrimSpace(s[idx+1:])
		// tail should be "recipient: Y" or "recipient = Y"
		k, v, ok := strings.Cut(tail, ":")
		if !ok {
			k, v, ok = strings.Cut(tail, "=")
		}
		if ok && strings.TrimSpace(k) == "recipient" {
			meta.PayAmount = amount
			meta.PayRecipient = strings.TrimSpace(v)
			return
		}
	}
	// Bare amount: @pay(X)
	meta.PayAmount = s
	meta.PayIsBare = true
}

// parseQuotaTag parses "@quota(calls: N, price: M)" into meta.QuotaCalls / QuotaPrice.
// Supported forms:
//
//	@quota(calls: 1000, price: 1000000)   — colon separators
//	@quota(calls=1000, price=1000000)     — equals separators
func parseQuotaTag(meta *ast.DocMeta, rest string) {
	s := strings.TrimSpace(rest)
	s = strings.TrimPrefix(s, "(")
	s = strings.TrimSuffix(s, ")")
	s = strings.TrimSpace(s)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		var k, v string
		var ok bool
		k, v, ok = strings.Cut(part, ":")
		if !ok {
			k, v, ok = strings.Cut(part, "=")
		}
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "calls":
			meta.QuotaCalls = strings.TrimSpace(v)
		case "price":
			meta.QuotaPrice = strings.TrimSpace(v)
		}
	}
}

// parseTotalCostTag parses "@total_cost(max: N)" into meta.TotalCostMax.
// Supported forms:
//
//	@total_cost(max: 500000)   — colon separator
//	@total_cost(max=500000)    — equals separator
func parseTotalCostTag(meta *ast.DocMeta, rest string) {
	s := strings.TrimSpace(rest)
	s = strings.TrimPrefix(s, "(")
	s = strings.TrimSuffix(s, ")")
	s = strings.TrimSpace(s)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		var k, v string
		var ok bool
		k, v, ok = strings.Cut(part, ":")
		if !ok {
			k, v, ok = strings.Cut(part, "=")
		}
		if !ok {
			continue
		}
		if strings.TrimSpace(k) == "max" {
			meta.TotalCostMax = strings.TrimSpace(v)
		}
	}
}

// parseCapabilityDecl parses "capability IDENT ;" at contract body level.
// The "capability" contextual keyword has NOT been consumed yet.
func (p *Parser) parseCapabilityDecl() *ast.CapabilityDecl {
	line := p.cur.Start.Line
	p.next() // consume 'capability'
	if p.cur.Type != lexer.TokenIdent {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected capability name (identifier) after 'capability'",
			Span:    p.span(p.cur),
		})
		p.syncUntilAfterSemicolon()
		return nil
	}
	name := p.cur.Literal
	p.next()
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after capability declaration") {
		return nil
	}
	return &ast.CapabilityDecl{Name: name, Line: line}
}

// parsePurposeDecl parses "purpose IDENT ;" at contract body level.
func (p *Parser) parsePurposeDecl() *ast.PurposeDecl {
	line := p.cur.Start.Line
	p.next() // consume 'purpose'
	if p.cur.Type != lexer.TokenIdent {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: "expected purpose name (identifier) after 'purpose'",
			Span:    p.span(p.cur),
		})
		p.syncUntilAfterSemicolon()
		return nil
	}
	name := p.cur.Literal
	p.next()
	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, "expected ';' after purpose declaration") {
		return nil
	}
	return &ast.PurposeDecl{Name: name, Line: line}
}

// parseManifestDecl parses a manifest block at contract body level.
// Supported value forms:
//   key: "string"    — string literal
//   key: 1234        — number literal
//   key: [A, B]      — array of idents/strings
// Separators: ',' or ';' (both accepted, both optional before '}'.
func (p *Parser) parseManifestDecl() *ast.ManifestDecl {
	line := p.cur.Start.Line
	p.next() // consume 'manifest'
	if !p.expect(lexer.TokenLBrace, diag.CodeParseUnexpected, "expected '{' after 'manifest'") {
		return nil
	}
	md := &ast.ManifestDecl{Line: line}
	for p.cur.Type != lexer.TokenRBrace && p.cur.Type != lexer.TokenEOF {
		// key must be an identifier
		if p.cur.Type != lexer.TokenIdent {
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("expected manifest key (identifier), got '%s'", p.cur.Literal),
				Span:    p.span(p.cur),
			})
			p.syncUntil(lexer.TokenRBrace, lexer.TokenEOF)
			break
		}
		key := p.cur.Literal
		p.next()
		if !p.expect(lexer.TokenColon, diag.CodeParseUnexpected, "expected ':' after manifest key") {
			p.syncUntil(lexer.TokenRBrace, lexer.TokenEOF)
			break
		}
		var field ast.ManifestField
		field.Key = key
		switch p.cur.Type {
		case lexer.TokenString:
			field.Value = p.cur.Literal
			p.next()
		case lexer.TokenNumber:
			field.Value = p.cur.Literal
			p.next()
		case lexer.TokenLBracket:
			// Array value: [A, B, ...]
			p.next() // consume '['
			field.IsArray = true
			for p.cur.Type != lexer.TokenRBracket && p.cur.Type != lexer.TokenEOF {
				switch p.cur.Type {
				case lexer.TokenIdent:
					field.Array = append(field.Array, p.cur.Literal)
				case lexer.TokenString:
					field.Array = append(field.Array, p.cur.Literal)
				default:
					p.addDiag(diag.Diagnostic{
						Code:    diag.CodeParseUnexpected,
						Message: fmt.Sprintf("expected identifier or string in manifest array, got '%s'", p.cur.Literal),
						Span:    p.span(p.cur),
					})
					p.syncUntil(lexer.TokenRBracket, lexer.TokenRBrace, lexer.TokenEOF)
					goto doneField
				}
				p.next()
				if p.cur.Type == lexer.TokenComma {
					p.next()
				}
			}
			if p.cur.Type == lexer.TokenRBracket {
				p.next() // consume ']'
			}
		default:
			p.addDiag(diag.Diagnostic{
				Code:    diag.CodeParseUnexpected,
				Message: fmt.Sprintf("expected string, number, or array for manifest value, got '%s'", p.cur.Literal),
				Span:    p.span(p.cur),
			})
			p.syncUntil(lexer.TokenRBrace, lexer.TokenEOF)
			break
		}
		md.Fields = append(md.Fields, field)
	doneField:
		// Accept ',' or ';' as separator (both optional before '}')
		if p.cur.Type == lexer.TokenComma || p.cur.Type == lexer.TokenSemicolon {
			p.next()
		}
	}
	if !p.expect(lexer.TokenRBrace, diag.CodeParseUnexpected, "expected '}' to close manifest block") {
		return nil
	}
	return md
}

// parseAgentTypeSlot parses an agent-native typed storage slot:
//
//	agent     name;
//
// The type keyword has NOT been consumed yet.
func (p *Parser) parseAgentTypeSlot() *ast.StorageSlot {
	typeName := p.cur.Literal // "agent"
	p.next()                  // consume the type keyword

	fullType := typeName
	if p.cur.Type == lexer.TokenLT {
		// Consume '<T>'
		p.next() // consume '<'
		var innerParts []string
		depth := 0
		for p.cur.Type != lexer.TokenEOF {
			if p.cur.Type == lexer.TokenLT {
				depth++
				innerParts = append(innerParts, "<")
				p.next()
				continue
			}
			if p.cur.Type == lexer.TokenGT {
				if depth == 0 {
					p.next() // consume '>'
					break
				}
				depth--
				innerParts = append(innerParts, ">")
				p.next()
				continue
			}
			innerParts = append(innerParts, p.cur.Literal)
			p.next()
		}
		fullType = typeName + "<" + strings.Join(innerParts, "") + ">"
	}

	// Parse optional array suffixes: [][3][], [N], etc.
	for p.cur.Type == lexer.TokenLBracket {
		p.next() // consume '['
		if p.cur.Type == lexer.TokenRBracket {
			fullType += "[]"
			p.next() // consume ']'
		} else {
			// fixed-size array [N]
			size := p.cur.Literal
			p.next()
			if p.cur.Type == lexer.TokenRBracket {
				p.next()
			}
			fullType += "[" + size + "]"
		}
	}

	// Parse optional visibility modifiers (public/private/internal/override)
	visibility := ""
	isOverride := false
	for {
		switch p.cur.Type {
		case lexer.TokenKwPublic:
			visibility = "public"
			p.next()
			continue
		case lexer.TokenKwPrivate:
			visibility = "private"
			p.next()
			continue
		case lexer.TokenKwInternal:
			visibility = "internal"
			p.next()
			continue
		case lexer.TokenKwOverride:
			isOverride = true
			p.next()
			continue
		}
		break
	}
	_ = isOverride // stored on slot via existing fields if needed

	if p.cur.Type != lexer.TokenIdent {
		p.addDiag(diag.Diagnostic{
			Code:    diag.CodeParseUnexpected,
			Message: fmt.Sprintf("expected variable name after agent type '%s', got '%s'", fullType, p.cur.Literal),
			Span:    p.span(p.cur),
		})
		p.syncUntil(lexer.TokenSemicolon, lexer.TokenRBrace, lexer.TokenEOF)
		if p.cur.Type == lexer.TokenSemicolon {
			p.next()
		}
		return nil
	}
	name := p.cur.Literal
	p.next()

	if !p.expect(lexer.TokenSemicolon, diag.CodeParseUnexpected, fmt.Sprintf("expected ';' after agent-native slot '%s'", name)) {
		return nil
	}
	return &ast.StorageSlot{
		Name:       name,
		Type:       fullType,
		Visibility: visibility,
	}
}

func (p *Parser) addDiag(d diag.Diagnostic) {
	p.diags = append(p.diags, d)
}

func (p *Parser) span(tok lexer.Token) diag.Span {
	return diag.Span{
		File: p.filename,
		Start: diag.Position{
			Line:   tok.Start.Line,
			Column: tok.Start.Column,
		},
		End: diag.Position{
			Line:   tok.End.Line,
			Column: tok.End.Column,
		},
	}
}

// mulDecimalStrings multiplies two non-negative decimal integer strings and
// returns their product as a decimal string. Both a and b must contain only
// ASCII digit characters (no leading zeros, no sign, no hex prefix). Used for
// SubDenomination compile-time constant folding (e.g. "1" * "1000000000000000000").
func mulDecimalStrings(a, b string) string {
	if a == "0" || b == "0" {
		return "0"
	}
	if a == "1" {
		return b
	}
	if b == "1" {
		return a
	}
	// Represent a as a slice of decimal digits (most-significant first).
	digitsA := make([]int, len(a))
	for i, ch := range a {
		digitsA[i] = int(ch - '0')
	}
	// Represent b as a slice of decimal digits.
	digitsB := make([]int, len(b))
	for i, ch := range b {
		digitsB[i] = int(ch - '0')
	}
	// Result has at most len(a)+len(b) digits.
	result := make([]int, len(a)+len(b))
	// Schoolbook long multiplication (grade-school algorithm).
	for i := len(digitsA) - 1; i >= 0; i-- {
		for j := len(digitsB) - 1; j >= 0; j-- {
			mul := digitsA[i] * digitsB[j]
			p1 := i + j
			p2 := i + j + 1
			sum := mul + result[p2]
			result[p2] = sum % 10
			result[p1] += sum / 10
		}
	}
	// Convert result to string, skipping leading zeros.
	var sb strings.Builder
	leadingZero := true
	for _, d := range result {
		if leadingZero && d == 0 {
			continue
		}
		leadingZero = false
		sb.WriteByte(byte('0' + d))
	}
	if sb.Len() == 0 {
		return "0"
	}
	return sb.String()
}
