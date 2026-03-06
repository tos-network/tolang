// ANTLR4 Lexer Grammar for the TOL (TOS Object Language) language.
//
// TOL v0.3 / v0.4 (agent-native extension) — see docs/TOL_SPEC.md and
// docs/AGENT-NATIVE.md for the full language specification.
//
// Alignment policy (see docs/grammar/diff.md):
//   - All Solidity reserved keywords are reserved here as well.
//   - TOL-specific reserved keywords are marked "// TOL-specific".
//   - Keywords that are currently CONTEXTUAL in the production lexer
//     (tol/lexer/lexer.go) are marked "// production: contextual".
//     The production lexer emits them as TokenIdent; the parser disambiguates
//     by position.  They will be promoted to fully reserved over time.
//
// Usage:
//   antlr4 -Dlanguage=Go TolangLexer.g4 TolangParser.g4

lexer grammar TolangLexer;

// ============================================================
// Reserved Keywords
// Solidity-aligned unless marked "// TOL-specific".
// ============================================================

Abstract    : 'abstract'    ; // production: contextual
// Address token removed — 'address' is a deprecated alias for 'agent'.
// The production lexer still accepts 'address' for backward compatibility
// and normalises it to 'agent' via normalizeSelectorType().
Anonymous   : 'anonymous'   ; // production: contextual
As          : 'as'          ;
Assert      : 'assert'      ; // TOL-specific (Solidity: contextual builtin)
Bool        : 'bool'        ; // production: contextual
Break       : 'break'       ;
Bytes       : 'bytes'       ; // production: contextual
Calldata    : 'calldata'    ; // production: contextual
Catch       : 'catch'       ; // production: contextual
Constant    : 'constant'    ;
Constructor : 'constructor' ;
Continue    : 'continue'    ;
Contract    : 'contract'    ;
Delete      : 'delete'      ; // production: contextual
Deploy      : 'deploy'      ; // TOL-specific: alias for 'new'; production: TokenKwDeploy
Do          : 'do'          ;
Else        : 'else'        ;
Emit        : 'emit'        ;
Enum        : 'enum'        ;
Error       : 'error'       ;
Event       : 'event'       ;
External    : 'external'    ; // production: contextual
Fallback    : 'fallback'    ;
False       : 'false'       ; // production: contextual
For         : 'for'         ;
From        : 'from'        ; // production: NOT a keyword token — emitted as TokenIdent
                               // The production lexer checks (p.cur.Literal == "from") directly.
Function    : 'function'    ;
Global      : 'global'      ; // production: contextual
If          : 'if'          ;
Immutable   : 'immutable'   ;
Import      : 'import'      ;
Indexed     : 'indexed'     ; // production: contextual
Interface   : 'interface'   ;
Internal    : 'internal'    ; // production: contextual
Is          : 'is'          ; // production: contextual
Let         : 'let'         ; // TOL-specific (Solidity: future reserved)
Library     : 'library'     ;
Mapping     : 'mapping'     ;
Memory      : 'memory'      ; // production: contextual
Modifier    : 'modifier'    ;
New         : 'new'         ; // production: contextual
Override    : 'override'    ; // production: contextual
Payable     : 'payable'     ; // production: contextual
Package     : 'package'     ; // TOL package namespace declaration
Pragma      : 'pragma'      ;
Private     : 'private'     ; // production: contextual
Public      : 'public'      ; // production: contextual
Pure        : 'pure'        ; // production: contextual
Receive     : 'receive'     ; // production: contextual
Require     : 'require'     ; // TOL-specific (Solidity: contextual builtin)
Return      : 'return'      ;
Returns     : 'returns'     ;
Revert      : 'revert'      ;
Set         : 'set'         ; // TOL-specific; explicit storage-write keyword
Storage     : 'storage'     ; // data-location keyword (NOT a block wrapper)
String      : 'string'      ; // production: contextual
Struct      : 'struct'      ;
Test        : 'test'        ; // TOL-specific; first-class test block keyword
Transient   : 'transient'   ; // EIP-1153
True        : 'true'        ; // production: contextual
Try         : 'try'         ; // production: contextual
Type        : 'type'        ;
Unchecked   : 'unchecked'   ; // production: contextual
Unicode     : 'unicode'     ; // string-literal prefix (production: not yet implemented)
Using       : 'using'       ; // production: contextual
View        : 'view'        ; // production: contextual
Virtual     : 'virtual'     ; // production: contextual
While       : 'while'       ;

// ============================================================
// Reserved keywords — future use (Solidity-aligned)
//
// The production lexer (tol/lexer/token.go keywordType()) emits these as
// TokenReserved; the parser (tol/parser/parser.go) rejects them with
// diagnostic TOL1001 if used as identifiers.
// This matches Solidity's ReservedKeywords rule exactly.
// ============================================================

ReservedKeywords
    : 'after' | 'alias' | 'apply' | 'auto' | 'byte' | 'case' | 'copyof'
    | 'default' | 'define' | 'final' | 'implements' | 'in' | 'inline'
    | 'macro' | 'match' | 'mutable' | 'null' | 'of' | 'partial' | 'promise'
    | 'reference' | 'relocatable' | 'sealed' | 'sizeof' | 'static'
    | 'supports' | 'switch' | 'typedef' | 'typeof' | 'var'
    ;

// ============================================================
// Numeric Type Tokens
//
// IMPLEMENTATION NOTE:
//   The production lexer (tol/lexer/lexer.go) emits ALL identifiers as
//   TokenIdent, including "u256", "i8", "bytes32".  The ANTLR4 grammar
//   is stricter: it rejects invalid types at lex time.
//
//   The production lexer normalises Solidity uint/int aliases to the
//   canonical TOL u/i prefix via normalizeTypeAlias().
//
// Semantic ranges: u/i: 8,16,...,256 (multiples of 8); bytes: 1..32.
// ============================================================

UnsignedIntegerType
    // Canonical TOL form: u8 .. u256
    : 'u' ( '8' | '16' | '24' | '32' | '40' | '48' | '56' | '64'
          | '72' | '80' | '88' | '96' | '104' | '112' | '120' | '128'
          | '136' | '144' | '152' | '160' | '168' | '176' | '184' | '192'
          | '200' | '208' | '216' | '224' | '232' | '240' | '248' | '256' )
    // Solidity-compatible aliases: uint8..uint256 — normalised to u8..u256
    | 'uint' ( '8' | '16' | '24' | '32' | '40' | '48' | '56' | '64'
             | '72' | '80' | '88' | '96' | '104' | '112' | '120' | '128'
             | '136' | '144' | '152' | '160' | '168' | '176' | '184' | '192'
             | '200' | '208' | '216' | '224' | '232' | '240' | '248' | '256' )
    | 'uint256'
    | 'uint'
    ;

SignedIntegerType
    // Canonical TOL form: i8 .. i256
    : 'i' ( '8' | '16' | '24' | '32' | '40' | '48' | '56' | '64'
          | '72' | '80' | '88' | '96' | '104' | '112' | '120' | '128'
          | '136' | '144' | '152' | '160' | '168' | '176' | '184' | '192'
          | '200' | '208' | '216' | '224' | '232' | '240' | '248' | '256' )
    // Solidity-compatible aliases: int8..int256 — normalised to i8..i256
    | 'int' ( '8' | '16' | '24' | '32' | '40' | '48' | '56' | '64'
            | '72' | '80' | '88' | '96' | '104' | '112' | '120' | '128'
            | '136' | '144' | '152' | '160' | '168' | '176' | '184' | '192'
            | '200' | '208' | '216' | '224' | '232' | '240' | '248' | '256' )
    | 'int256'
    | 'int'
    ;

FixedBytesType
    : 'bytes' ( '1' | '2' | '3' | '4' | '5' | '6' | '7' | '8'
              | '9' | '10' | '11' | '12' | '13' | '14' | '15' | '16'
              | '17' | '18' | '19' | '20' | '21' | '22' | '23' | '24'
              | '25' | '26' | '27' | '28' | '29' | '30' | '31' | '32' )
    ;

// Fixed-point types (Solidity-aligned; NOT yet implemented in the production compiler).
// Included here so grammar tooling can parse Solidity source that uses fixed-point.
Fixed
    : 'fixed'
    | 'fixed' [1-9][0-9]* 'x' [1-9][0-9]*
    ;

Ufixed
    : 'ufixed'
    | 'ufixed' [1-9][0-9]+ 'x' [1-9][0-9]+
    ;

// ============================================================
// Literal Tokens
// ============================================================

// DecimalNumber
//   - Integer literals with optional underscore separators and scientific notation.
//   - Decimal fractions (1.5, 1.5e10) are included for Solidity alignment;
//     the production lexer currently accepts integers only.
//   - Version literal form (0.2.0) uses the dot-separated alt (longest-match wins).
//
// IMPORTANT: the production lexer's readNumber() emits "0.2.0" as ONE token.
DecimalNumber
    : ( DecimalDigits | DecimalDigits '.' DecimalDigits )
      ( [eE] '-'? DecimalDigits )?         // optional signed exponent (Solidity-aligned)
    | [0-9]+ ('.' [0-9]+)+                 // version literal (0.2.0)
    ;

fragment DecimalDigits
    : [0-9] ('_'? [0-9])*
    ;

// Hex literals: 0x / 0X prefix, optional underscore separators between digits.
HexNumber
    : '0' [xX] [0-9a-fA-F] ([0-9a-fA-F_]* [0-9a-fA-F])?
    ;

// Octal literals: scanned but DISALLOWED in TOL (aligned with Solidity).
// The production lexer will emit a TokenIllegal for octal inputs.
OctalNumber
    : '0' [0-9]+ ('.' [0-9]+)?
    ;

// Standard string literals: double-quoted or single-quoted.
StringLiteral
    : '"'  DoubleStringChar* '"'
    | '\'' SingleStringChar* '\''
    ;

// Unicode string literals: unicode"..." or unicode'...'
// Aligned with Solidity's UnicodeStringLiteral.
// The production lexer does not yet distinguish these; they are accepted
// as ordinary StringLiterals and the unicode prefix is ignored.
UnicodeStringLiteral
    : Unicode ( '"' DoubleUnicodeChar* '"'
              | '\'' SingleUnicodeChar* '\'' )
    ;

// Hex string literal: hex"deadbeef" or hex'DE_AD_BE_EF'
// Content (minus '_') must be an even number of hex digits.
HexString
    : 'hex' ( '"'  EvenHexDigits? '"'
             | '\'' EvenHexDigits? '\'' )
    ;

fragment EvenHexDigits
    : HexDigitPair ( '_'? HexDigitPair )*
    ;

fragment HexDigitPair
    : [0-9a-fA-F] [0-9a-fA-F]
    ;

fragment DoubleStringChar
    : ~["\\\r\n]
    | EscapeSequence
    ;

fragment SingleStringChar
    : ~['\\\r\n]
    | EscapeSequence
    ;

fragment DoubleUnicodeChar
    : ~["\r\n\\]
    | EscapeSequence
    ;

fragment SingleUnicodeChar
    : ~['\r\n\\]
    | EscapeSequence
    ;

// EscapeSequence — complete set accepted by TOL.
//   Supported: \n \r \t \\ \' \" \0 \xHH \uHHHH \<LF> \<CR><LF>
// TOL supports \0 (NUL) and line-continuation escapes (\<newline>),
// which Solidity does not.
fragment EscapeSequence
    : '\\' [nrt\\'\"0]
    | '\\' 'x' HexDigit HexDigit
    | '\\' 'u' HexDigit HexDigit HexDigit HexDigit
    | '\\' '\n'            // line continuation (LF) — TOL-specific
    | '\\' '\r' '\n'?      // line continuation (CR / CR+LF) — TOL-specific
    ;

fragment HexDigit : [0-9a-fA-F] ;

// ============================================================
// Identifier
//
// Contextual keywords — valid Identifier tokens in certain positions:
//   (all keywords listed in the reserved section above that are
//    marked "production: contextual")
//
// Additionally, the following have no reserved token and remain
// purely contextual:
//   agent  capability  cases  fuzz  inspect  mock  nil  oracle
//   selector  setup  setup_suite  skip  tag  task  teardown
//   teardown_suite  timeout  tolang  vote  with
//
// 'agent' is TOL's primary identity type — equivalent to Solidity's 'address'
// but semantically richer (carries .stake, .is_active, .reputation etc.).
// The deprecated 'address' and 'address payable' spellings are still accepted
// and silently normalised to 'agent' throughout the pipeline.
//
// Note: 'deploy' was contextual; promoted to Deploy token (TokenKwDeploy).
// ============================================================

Identifier
    : IdentifierStart IdentifierPart*
    ;

fragment IdentifierStart : [a-zA-Z_$] ;
fragment IdentifierPart  : [a-zA-Z0-9_$] ;

// ============================================================
// Punctuation
// ============================================================

LParen    : '('  ;
RParen    : ')'  ;
LBrace    : '{'  ;
RBrace    : '}'  ;
LBracket  : '['  ;
RBracket  : ']'  ;
Semicolon : ';'  ;
Colon     : ':'  ;
Comma     : ','  ;
Dot       : '.'  ;
Question  : '?'  ;
At        : '@'  ; // TOL-specific: attribute annotations (@skip, @tag, @effects, …)
Star      : '*'  ; // used for multiplication, 'import *', and 'using … for *'

// Arrows
Arrow    : '->'  ; // binding arrow in deploy / setup returns
FatArrow : '=>'  ; // mapping key=>value separator

// ============================================================
// Operators (longest-match first)
// ============================================================

// Compound assignment
PlusAssign  : '+='  ;
MinusAssign : '-='  ;
MulAssign   : '*='  ;
DivAssign   : '/='  ;
ModAssign   : '%='  ;
AndAssign   : '&='  ;
OrAssign    : '|='  ;
XorAssign   : '^='  ;
ShlAssign   : '<<=' ;
SarAssign   : '>>=' ;   // arithmetic right-shift assign
ShrAssign   : '>>>=' ;  // logical right-shift assign — TOL / Solidity

// Power
Pow : '**' ;

// Increment / decrement
PlusPlus   : '++' ;
MinusMinus : '--' ;

// Comparison (two-char before single-char)
Eq  : '==' ;
Ne  : '!=' ;
Le  : '<=' ;
Ge  : '>=' ;
Shl : '<<' ;
Sar : '>>' ;   // arithmetic right shift
Shr : '>>>' ;  // logical right shift (zero-fill) — TOL / Solidity

// Logical
AndAnd : '&&' ;
OrOr   : '||' ;

// Assignment
Assign : '=' ;

// Arithmetic / bitwise (single-char; Pow and PlusPlus declared above)
Plus    : '+'  ;
Minus   : '-'  ;
Slash   : '/'  ;
Percent : '%'  ;
BitAnd  : '&'  ;
BitOr   : '|'  ;
BitXor  : '^'  ;
BitNot  : '~'  ;
Bang    : '!'  ;
Lt      : '<'  ;
Gt      : '>'  ;

// ============================================================
// Sub-denomination unit suffixes
//
// These identifiers are emitted as TokenSubDenom by the production lexer
// when they immediately follow a numeric literal.
//   Value: wei (×1), gwei (×1e9), ether (×1e18)
//   Time:  seconds (×1), minutes (×60), hours (×3600),
//          days (×86400), weeks (×604800)
// ============================================================

SubDenomination
    : 'wei' | 'gwei' | 'ether'
    | 'seconds' | 'minutes' | 'hours' | 'days' | 'weeks'
    | 'years'   // DEPRECATED: use 365 days instead
    ;

// ============================================================
// Whitespace and Comments
//
// ORDERING NOTE (ANTLR longest-match):
//   DocLineComment  must precede LineComment  (both start with '//')
//   DocBlockComment must precede BlockComment (both start with '/*')
//
// Doc comments are NOT sent to the hidden channel — they are emitted as
// visible tokens so the parser can bind them to the immediately following
// declaration (TokenDocComment in the production lexer).
//
// Production lexer behaviour (tol/lexer/lexer.go):
//   '///'  → TokenDocComment  (triple-slash; any following content on same line)
//   '/**'  → TokenDocComment  (block; terminated by '*/')
//   '//'   → skipped           (regular line comment)
//   '/*'   → skipped           (regular block comment, NOT '/**')
// ============================================================

Whitespace      : [ \t\r\n\u000C]+ -> skip ;
DocLineComment  : '///' ~[\r\n]*    ;                  // triple-slash — emitted as TokenDocComment
DocBlockComment : '/**' .*? '*/'    ;                  // block doc   — emitted as TokenDocComment
LineComment     : '//' ~[\r\n]*     -> channel(HIDDEN) ; // must appear AFTER DocLineComment
BlockComment    : '/*' .*? '*/'     -> channel(HIDDEN) ; // must appear AFTER DocBlockComment
