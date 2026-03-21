package lexer

type Type int

const (
	TokenIllegal Type = iota
	TokenEOF
	TokenIdent
	TokenNumber
	TokenString
	TokenHexString
	TokenLParen
	TokenRParen
	TokenLBrace
	TokenRBrace
	TokenLBracket
	TokenRBracket
	TokenColon
	TokenSemicolon
	TokenComma
	TokenDot
	TokenArrow
	TokenFatArrow
	TokenAssign
	TokenEq
	TokenNe
	TokenLT
	TokenLE
	TokenGT
	TokenGE
	TokenBang
	TokenAndAnd
	TokenOrOr
	TokenPlus
	TokenMinus
	TokenStar
	TokenPow // **
	TokenSlash
	TokenPercent
	TokenAt
	TokenBitAnd
	TokenBitOr
	TokenBitXor
	TokenBitNot
	TokenShl
	TokenSar        // >> arithmetic shift right (signed-aware, preserves sign bit)
	TokenPlusAssign
	TokenMinusAssign
	TokenMulAssign
	TokenDivAssign
	TokenModAssign
	TokenAndAssign
	TokenOrAssign
	TokenXorAssign
	TokenShlAssign
	TokenSarAssign  // >>= arithmetic shift right assign
	TokenPlusPlus
	TokenMinusMinus
	TokenKwImport
	TokenKwContract
	TokenKwInterface
	TokenKwLibrary
	TokenKwImmutable
	TokenKwEvent
	TokenKwFunction    // "function" (replaces old "fn")
	TokenKwReturns     // "returns"
	TokenKwMapping     // "mapping"
	TokenKwType        // "type"
	TokenKwAs          // "as"
	TokenKwPragma      // "pragma"
	TokenKwPackage     // "package"
	TokenKwTransient   // "transient"
	TokenKwStruct      // "struct"
	TokenReserved      // reserved keywords (Solidity subset + Agent-Native future)
	TokenKwConstructor
	TokenKwFallback
	TokenKwError
	TokenKwEnum
	TokenKwModifier
	TokenKwLet
	TokenKwSet
	TokenKwIf
	TokenKwElse
	TokenKwDo
	TokenKwWhile
	TokenKwFor
	TokenKwBreak
	TokenKwContinue
	TokenKwReturn
	TokenKwRequire
	TokenKwAssert
	TokenKwRevert
	TokenKwEmit
	TokenKwTest
	TokenKwConstant
	TokenQuestion
	TokenDocComment // /// ... or /** ... */
	TokenSubDenom   // tomi | gtomi | seconds | minutes | hours | days | weeks | years (tos handled as ident + SubDenomMultiplier)
	TokenShr        // >>> logical shift right (always zero-fills high bits)
	TokenShrAssign  // >>>= logical shift right assign
	// Category 8/9/10/11/12: promoted keywords
	TokenKwTry        // "try"
	TokenKwCatch      // "catch" — promoted from contextual
	TokenKwAbstract   // "abstract"
	TokenKwAnonymous  // "anonymous"
	TokenKwAgent      // "agent"
	TokenKwBool       // "bool"
	TokenKwCalldata   // "calldata"
	TokenKwDelete     // "delete"
	TokenKwExternal   // "external"
	TokenKwFalse      // "false"
	TokenKwGlobal     // "global"
	TokenKwIndexed    // "indexed"
	TokenKwInternal   // "internal"
	TokenKwIs         // "is"
	TokenKwMemory     // "memory"
	TokenKwNew        // "new"
	TokenKwDeploy     // "deploy" (alias for "new" in expression context)
	TokenKwOverride   // "override"
	TokenKwPayable    // "payable"
	TokenKwPrivate    // "private"
	TokenKwPublic     // "public"
	TokenKwPure       // "pure"
	TokenKwReceive    // "receive"
	TokenKwStorage    // "storage" (as keyword, not StorageDecl)
	TokenKwString     // "string"
	TokenKwTrue       // "true"
	TokenKwUnchecked  // "unchecked"
	TokenKwUno        // "uno"
	TokenKwUsing      // "using"
	TokenKwView       // "view"
	TokenKwVirtual    // "virtual"
	TokenUnicodeString // unicode"..." literal
)

func (t Type) String() string {
	switch t {
	case TokenIllegal:
		return "ILLEGAL"
	case TokenEOF:
		return "EOF"
	case TokenIdent:
		return "IDENT"
	case TokenNumber:
		return "NUMBER"
	case TokenString:
		return "STRING"
	case TokenHexString:
		return "HEX_STRING"
	case TokenLParen:
		return "("
	case TokenRParen:
		return ")"
	case TokenLBrace:
		return "{"
	case TokenRBrace:
		return "}"
	case TokenLBracket:
		return "["
	case TokenRBracket:
		return "]"
	case TokenColon:
		return ":"
	case TokenSemicolon:
		return ";"
	case TokenComma:
		return ","
	case TokenDot:
		return "."
	case TokenArrow:
		return "->"
	case TokenFatArrow:
		return "=>"
	case TokenAssign:
		return "="
	case TokenEq:
		return "=="
	case TokenNe:
		return "!="
	case TokenLT:
		return "<"
	case TokenLE:
		return "<="
	case TokenGT:
		return ">"
	case TokenGE:
		return ">="
	case TokenBang:
		return "!"
	case TokenAndAnd:
		return "&&"
	case TokenOrOr:
		return "||"
	case TokenPlus:
		return "+"
	case TokenMinus:
		return "-"
	case TokenStar:
		return "*"
	case TokenPow:
		return "**"
	case TokenSlash:
		return "/"
	case TokenPercent:
		return "%"
	case TokenAt:
		return "@"
	case TokenBitAnd:
		return "&"
	case TokenBitOr:
		return "|"
	case TokenBitXor:
		return "^"
	case TokenBitNot:
		return "~"
	case TokenShl:
		return "<<"
	case TokenSar:
		return ">>"
	case TokenShr:
		return ">>>"
	case TokenPlusAssign:
		return "+="
	case TokenMinusAssign:
		return "-="
	case TokenMulAssign:
		return "*="
	case TokenDivAssign:
		return "/="
	case TokenModAssign:
		return "%="
	case TokenAndAssign:
		return "&="
	case TokenOrAssign:
		return "|="
	case TokenXorAssign:
		return "^="
	case TokenShlAssign:
		return "<<="
	case TokenSarAssign:
		return ">>="
	case TokenShrAssign:
		return ">>>="
	case TokenPlusPlus:
		return "++"
	case TokenMinusMinus:
		return "--"
	case TokenKwImport:
		return "import"
	case TokenKwContract:
		return "contract"
	case TokenKwInterface:
		return "interface"
	case TokenKwLibrary:
		return "library"
	case TokenKwImmutable:
		return "immutable"
	case TokenKwEvent:
		return "event"
	case TokenKwFunction:
		return "function"
	case TokenKwReturns:
		return "returns"
	case TokenKwMapping:
		return "mapping"
	case TokenKwType:
		return "type"
	case TokenKwAs:
		return "as"
	case TokenKwPragma:
		return "pragma"
	case TokenKwPackage:
		return "package"
	case TokenKwTransient:
		return "transient"
	case TokenKwStruct:
		return "struct"
	case TokenReserved:
		return "RESERVED"
	case TokenKwConstructor:
		return "constructor"
	case TokenKwFallback:
		return "fallback"
	case TokenKwError:
		return "error"
	case TokenKwEnum:
		return "enum"
	case TokenKwModifier:
		return "modifier"
	case TokenKwTest:
		return "test"
	case TokenKwConstant:
		return "constant"
	case TokenQuestion:
		return "?"
	case TokenDocComment:
		return "DOC_COMMENT"
	case TokenSubDenom:
		return "SUBDENOM"
	case TokenKwTry:
		return "try"
	case TokenKwCatch:
		return "catch"
	case TokenKwAbstract:
		return "abstract"
	case TokenKwAnonymous:
		return "anonymous"
	case TokenKwAgent:
		return "agent"
	case TokenKwBool:
		return "bool"
	case TokenKwCalldata:
		return "calldata"
	case TokenKwDelete:
		return "delete"
	case TokenKwExternal:
		return "external"
	case TokenKwFalse:
		return "false"
	case TokenKwGlobal:
		return "global"
	case TokenKwIndexed:
		return "indexed"
	case TokenKwInternal:
		return "internal"
	case TokenKwIs:
		return "is"
	case TokenKwMemory:
		return "memory"
	case TokenKwNew:
		return "new"
	case TokenKwDeploy:
		return "deploy"
	case TokenKwOverride:
		return "override"
	case TokenKwPayable:
		return "payable"
	case TokenKwPrivate:
		return "private"
	case TokenKwPublic:
		return "public"
	case TokenKwPure:
		return "pure"
	case TokenKwReceive:
		return "receive"
	case TokenKwStorage:
		return "storage"
	case TokenKwString:
		return "string"
	case TokenKwTrue:
		return "true"
	case TokenKwUnchecked:
		return "unchecked"
	case TokenKwUno:
		return "uno"
	case TokenKwUsing:
		return "using"
	case TokenKwView:
		return "view"
	case TokenKwVirtual:
		return "virtual"
	case TokenUnicodeString:
		return "UNICODE_STRING"
	default:
		return "UNKNOWN"
	}
}

type Position struct {
	Offset int
	Line   int
	Column int
}

type Token struct {
	Type    Type
	Literal string
	Start   Position
	End     Position
}

// SubDenomMultiplier returns the compile-time multiplier for denomination
// suffixes on numeric literals. Returns "" if the string is not a denomination.
func SubDenomMultiplier(lit string) string {
	switch lit {
	case "tomi":
		return "1"
	case "gtomi":
		return "1000000000"
	case "tos":
		return "1000000000000000000"
	case "seconds":
		return "1"
	case "minutes":
		return "60"
	case "hours":
		return "3600"
	case "days":
		return "86400"
	case "weeks":
		return "604800"
	case "years":
		// Deprecated: Solidity 4820. Use 365 days instead.
		return "31536000"
	}
	return ""
}

func keywordType(lit string) Type {
	switch lit {
	case "import":
		return TokenKwImport
	case "contract":
		return TokenKwContract
	case "interface":
		return TokenKwInterface
	case "library":
		return TokenKwLibrary
	case "immutable":
		return TokenKwImmutable
	case "event":
		return TokenKwEvent
	case "function":
		return TokenKwFunction
	case "returns":
		return TokenKwReturns
	case "mapping":
		return TokenKwMapping
	case "type":
		return TokenKwType
	case "as":
		return TokenKwAs
	case "pragma":
		return TokenKwPragma
	case "package":
		return TokenKwPackage
	case "transient":
		return TokenKwTransient
	case "struct":
		return TokenKwStruct
	case "constructor":
		return TokenKwConstructor
	case "fallback":
		return TokenKwFallback
	case "error":
		return TokenKwError
	case "enum":
		return TokenKwEnum
	case "modifier":
		return TokenKwModifier
	case "test":
		return TokenKwTest
	case "constant":
		return TokenKwConstant
	case "let":
		return TokenKwLet
	case "set":
		return TokenKwSet
	case "if":
		return TokenKwIf
	case "else":
		return TokenKwElse
	case "do":
		return TokenKwDo
	case "while":
		return TokenKwWhile
	case "for":
		return TokenKwFor
	case "break":
		return TokenKwBreak
	case "continue":
		return TokenKwContinue
	case "return":
		return TokenKwReturn
	case "require":
		return TokenKwRequire
	case "assert":
		return TokenKwAssert
	case "revert":
		return TokenKwRevert
	case "emit":
		return TokenKwEmit
	case "tomi", "gtomi", "seconds", "minutes", "hours", "days", "weeks", "years":
		return TokenSubDenom
	// Category 12: promoted keywords
	case "try":
		return TokenKwTry
	case "catch":
		return TokenKwCatch
	case "abstract":
		return TokenKwAbstract
	case "anonymous":
		return TokenKwAnonymous
	case "bool":
		return TokenKwBool
	case "calldata":
		return TokenKwCalldata
	case "delete":
		return TokenKwDelete
	case "external":
		return TokenKwExternal
	case "false":
		return TokenKwFalse
	case "global":
		return TokenKwGlobal
	case "indexed":
		return TokenKwIndexed
	case "internal":
		return TokenKwInternal
	case "is":
		return TokenKwIs
	case "memory":
		return TokenKwMemory
	case "new":
		return TokenKwNew
	case "deploy":
		return TokenKwDeploy
	case "override":
		return TokenKwOverride
	case "payable":
		return TokenKwPayable
	case "private":
		return TokenKwPrivate
	case "public":
		return TokenKwPublic
	case "pure":
		return TokenKwPure
	case "receive":
		return TokenKwReceive
	case "storage":
		return TokenKwStorage
	case "string":
		return TokenKwString
	case "true":
		return TokenKwTrue
	case "unchecked":
		return TokenKwUnchecked
	case "uno":
		return TokenKwUno
	case "using":
		return TokenKwUsing
	case "view":
		return TokenKwView
	case "virtual":
		return TokenKwVirtual
	// Reserved keywords: Solidity-inherited (useful subset) + Agent-Native future keywords.
	// Only reserve words likely to become language-level keywords (control flow, type
	// system, concurrency). Domain nouns (oracle, guardian, task, etc.) are NOT reserved
	// because they are commonly used as variable/parameter names.
	case "byte", "case", "default", "final", "implements", "in",
		"match", "null", "sizeof", "static", "switch", "typedef", "typeof", "var",
		// Agent-Native reserved: concurrency primitives + verification keywords.
		"async", "await", "spawn",
		"intent",
		"attest", "stream":
		return TokenReserved
	default:
		return TokenIdent
	}
}
