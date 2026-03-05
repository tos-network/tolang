package lexer

import (
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"
)

type Lexer struct {
	src  []byte
	idx  int
	line int
	col  int
}

func New(src []byte) *Lexer {
	return &Lexer{
		src:  src,
		line: 1,
		col:  1,
	}
}

func (l *Lexer) Next() Token {
	l.skipSpaceAndOrdinaryComments()
	start := l.pos()
	if l.eof() {
		return Token{Type: TokenEOF, Start: start, End: start}
	}

	// Intercept doc comments before the main character switch.
	if l.peek() == '/' {
		if l.peekN(1) == '/' && l.peekN(2) == '/' {
			return l.readTripleSlashDoc(start)
		}
		if l.peekN(1) == '*' && l.peekN(2) == '*' {
			return l.readBlockDoc(start)
		}
	}

	ch := l.peek()
	switch ch {
	case '(':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenLParen, Literal: "(", Start: start, End: end}
	case ')':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenRParen, Literal: ")", Start: start, End: end}
	case '{':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenLBrace, Literal: "{", Start: start, End: end}
	case '}':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenRBrace, Literal: "}", Start: start, End: end}
	case '[':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenLBracket, Literal: "[", Start: start, End: end}
	case ']':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenRBracket, Literal: "]", Start: start, End: end}
	case ':':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenColon, Literal: ":", Start: start, End: end}
	case ';':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenSemicolon, Literal: ";", Start: start, End: end}
	case ',':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenComma, Literal: ",", Start: start, End: end}
	case '.':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenDot, Literal: ".", Start: start, End: end}
	case '-':
		if l.peekN(1) == '>' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenArrow, Literal: "->", Start: start, End: end}
		}
		if l.peekN(1) == '-' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenMinusMinus, Literal: "--", Start: start, End: end}
		}
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenMinusAssign, Literal: "-=", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenMinus, Literal: "-", Start: start, End: end}
	case '=':
		if l.peekN(1) == '>' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenFatArrow, Literal: "=>", Start: start, End: end}
		}
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenEq, Literal: "==", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenAssign, Literal: "=", Start: start, End: end}
	case '!':
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenNe, Literal: "!=", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenBang, Literal: "!", Start: start, End: end}
	case '<':
		if l.peekN(1) == '<' {
			if l.peekN(2) == '=' {
				l.advance()
				l.advance()
				l.advance()
				end := l.lastPos()
				return Token{Type: TokenShlAssign, Literal: "<<=", Start: start, End: end}
			}
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenShl, Literal: "<<", Start: start, End: end}
		}
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenLE, Literal: "<=", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenLT, Literal: "<", Start: start, End: end}
	case '>':
		if l.peekN(1) == '>' {
			// >>> and >>>= are logical (unsigned) right shift — distinct from >> / >>= (arithmetic).
			if l.peekN(2) == '>' {
				if l.peekN(3) == '=' {
					l.advance()
					l.advance()
					l.advance()
					l.advance()
					end := l.lastPos()
					return Token{Type: TokenShrAssign, Literal: ">>>=", Start: start, End: end}
				}
				l.advance()
				l.advance()
				l.advance()
				end := l.lastPos()
				return Token{Type: TokenShr, Literal: ">>>", Start: start, End: end}
			}
			if l.peekN(2) == '=' {
				l.advance()
				l.advance()
				l.advance()
				end := l.lastPos()
				return Token{Type: TokenSarAssign, Literal: ">>=", Start: start, End: end}
			}
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenSar, Literal: ">>", Start: start, End: end}
		}
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenGE, Literal: ">=", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenGT, Literal: ">", Start: start, End: end}
	case '&':
		if l.peekN(1) == '&' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenAndAnd, Literal: "&&", Start: start, End: end}
		}
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenAndAssign, Literal: "&=", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenBitAnd, Literal: "&", Start: start, End: end}
	case '|':
		if l.peekN(1) == '|' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenOrOr, Literal: "||", Start: start, End: end}
		}
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenOrAssign, Literal: "|=", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenBitOr, Literal: "|", Start: start, End: end}
	case '^':
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenXorAssign, Literal: "^=", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenBitXor, Literal: "^", Start: start, End: end}
	case '~':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenBitNot, Literal: "~", Start: start, End: end}
	case '+':
		if l.peekN(1) == '+' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenPlusPlus, Literal: "++", Start: start, End: end}
		}
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenPlusAssign, Literal: "+=", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenPlus, Literal: "+", Start: start, End: end}
	case '*':
		if l.peekN(1) == '*' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenPow, Literal: "**", Start: start, End: end}
		}
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenMulAssign, Literal: "*=", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenStar, Literal: "*", Start: start, End: end}
	case '/':
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenDivAssign, Literal: "/=", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenSlash, Literal: "/", Start: start, End: end}
	case '%':
		if l.peekN(1) == '=' {
			l.advance()
			l.advance()
			end := l.lastPos()
			return Token{Type: TokenModAssign, Literal: "%=", Start: start, End: end}
		}
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenPercent, Literal: "%", Start: start, End: end}
	case '@':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenAt, Literal: "@", Start: start, End: end}
	case '?':
		l.advance()
		end := l.lastPos()
		return Token{Type: TokenQuestion, Literal: "?", Start: start, End: end}
	case '"', '\'':
		lit, lexErr := l.readString(ch)
		end := l.lastPos()
		if lexErr != "" {
			return Token{Type: TokenIllegal, Literal: lexErr, Start: start, End: end}
		}
		return Token{Type: TokenString, Literal: lit, Start: start, End: end}
	}

	if isIdentStart(ch) {
		lit := l.readIdent()
		lit = normalizeTypeAlias(lit)
		// Check for hex string literal: hex"..." or hex'...'
		if lit == "hex" && !l.eof() && (l.peek() == '"' || l.peek() == '\'') {
			hexLit, err := l.readHexString(l.peek())
			end := l.lastPos()
			if err != nil {
				// Emit as ILLEGAL with the error message as literal.
				return Token{Type: TokenIllegal, Literal: err.Error(), Start: start, End: end}
			}
			return Token{Type: TokenHexString, Literal: hexLit, Start: start, End: end}
		}
		// Check for unicode string literal: unicode"..." or unicode'...'
		if lit == "unicode" && !l.eof() && (l.peek() == '"' || l.peek() == '\'') {
			quote := l.peek()
			strLit, lexErr := l.readString(quote)
			end := l.lastPos()
			if lexErr != "" {
				return Token{Type: TokenIllegal, Literal: lexErr, Start: start, End: end}
			}
			return Token{Type: TokenUnicodeString, Literal: strLit, Start: start, End: end}
		}
		end := l.lastPos()
		return Token{Type: keywordType(lit), Literal: lit, Start: start, End: end}
	}

	if isDigit(ch) {
		lit := l.readNumber()
		end := l.lastPos()
		// 12.11: detect octal literals: leading zero followed by more digits (not hex)
		if len(lit) >= 2 && lit[0] == '0' && lit[1] >= '0' && lit[1] <= '9' {
			return Token{Type: TokenIllegal, Literal: "octal literal not allowed: " + lit, Start: start, End: end}
		}
		return Token{Type: TokenNumber, Literal: lit, Start: start, End: end}
	}

	l.advance()
	end := l.lastPos()
	return Token{Type: TokenIllegal, Literal: string([]byte{ch}), Start: start, End: end}
}

// skipSpaceAndOrdinaryComments skips whitespace, ordinary // line comments (not ///),
// and ordinary /* */ block comments (not /**). Doc comments (/// and /**) are NOT consumed
// here; they are intercepted in Next() and emitted as TokenDocComment.
func (l *Lexer) skipSpaceAndOrdinaryComments() {
	for !l.eof() {
		ch := l.peek()
		if isSpace(ch) {
			l.advance()
			continue
		}
		// Ordinary // line comment: two slashes but NOT three (not ///).
		if ch == '/' && l.peekN(1) == '/' && l.peekN(2) != '/' {
			for !l.eof() && l.peek() != '\n' {
				l.advance()
			}
			continue
		}
		// Ordinary /* */ block comment: /* but NOT /** (not followed by a second *).
		if ch == '/' && l.peekN(1) == '*' && l.peekN(2) != '*' {
			l.advance() // /
			l.advance() // *
			for !l.eof() {
				if l.peek() == '*' && l.peekN(1) == '/' {
					l.advance() // *
					l.advance() // /
					break
				}
				l.advance()
			}
			continue
		}
		break
	}
}

// readTripleSlashDoc reads all consecutive /// lines (including empty /// lines) into a
// single TokenDocComment. A true blank line (not starting with ///) ends the block.
func (l *Lexer) readTripleSlashDoc(start Position) Token {
	var buf strings.Builder
	for {
		// Positioned at the '/' of '///'. Read to end of line (excluding newline).
		lineStart := l.idx
		for !l.eof() && l.peek() != '\n' {
			l.advance()
		}
		buf.Write(l.src[lineStart:l.idx])

		// Consume the newline if present.
		if !l.eof() && l.peek() == '\n' {
			buf.WriteByte('\n')
			l.advance()
		}

		// Look ahead using raw index arithmetic (no state mutation):
		// skip horizontal whitespace, then check for ///.
		i := l.idx
		for i < len(l.src) && (l.src[i] == ' ' || l.src[i] == '\t') {
			i++
		}
		if i+2 < len(l.src) && l.src[i] == '/' && l.src[i+1] == '/' && l.src[i+2] == '/' {
			// Next line is ///: advance past the leading horizontal whitespace and continue.
			for l.idx < i {
				l.advance()
			}
			continue
		}
		break
	}
	end := l.lastPos()
	return Token{Type: TokenDocComment, Literal: buf.String(), Start: start, End: end}
}

// readBlockDoc reads a /** ... */ block into a single TokenDocComment.
// Positioned at the '/' of '/**' on entry.
func (l *Lexer) readBlockDoc(start Position) Token {
	blockStart := l.idx
	l.advance() // /
	l.advance() // first *
	// Now at the second '*' (part of /**). Read until */ closes the block.
	for !l.eof() {
		if l.peek() == '*' && l.peekN(1) == '/' {
			l.advance() // *
			l.advance() // /
			break
		}
		l.advance()
	}
	end := l.lastPos()
	return Token{Type: TokenDocComment, Literal: string(l.src[blockStart:l.idx]), Start: start, End: end}
}

func (l *Lexer) readIdent() string {
	start := l.idx
	for !l.eof() && isIdentPart(l.peek()) {
		l.advance()
	}
	return string(l.src[start:l.idx])
}

func (l *Lexer) readNumber() string {
	start := l.idx
	// Handle 0x/0X hex prefix: read hex digits with optional '_' separators.
	if l.peek() == '0' && l.idx+1 < len(l.src) {
		next := l.src[l.idx+1]
		if next == 'x' || next == 'X' {
			l.advance() // '0'
			l.advance() // 'x'/'X'
			prefix := string(l.src[start:l.idx]) // "0x" or "0X"
			var hexBuf strings.Builder
			for !l.eof() && (isHexDigit(l.peek()) || l.peek() == '_') {
				ch := l.peek()
				l.advance()
				if ch != '_' {
					hexBuf.WriteByte(ch)
				}
			}
			return prefix + hexBuf.String()
		}
	}
	// Decimal: digits with optional '_' separators (stripped), optional '.' (for
	// version literals like 0.2.0), and optional 'e'/'E' exponent (scientific
	// notation, only when no '.' has appeared yet).
	var buf strings.Builder
	hasDot := false
	for !l.eof() {
		ch := l.peek()
		switch {
		case isDigit(ch):
			buf.WriteByte(ch)
			l.advance()
		case ch == '_' && !hasDot:
			// Underscore digit separator: skip (strip from output).
			l.advance()
		case ch == '.':
			hasDot = true
			buf.WriteByte(ch)
			l.advance()
		case (ch == 'e' || ch == 'E') && !hasDot && buf.Len() > 0:
			// Scientific notation exponent: consume 'e'/'E' and its digits.
			buf.WriteByte(ch)
			l.advance()
			for !l.eof() && (isDigit(l.peek()) || l.peek() == '_') {
				ch2 := l.peek()
				l.advance()
				if ch2 != '_' {
					buf.WriteByte(ch2)
				}
			}
			return buf.String()
		default:
			return buf.String()
		}
	}
	return buf.String()
}

func isHexDigit(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

// readString reads a string literal delimited by quote (either '"' or '\'').
// It validates escape sequences according to the TOL escape rules:
//
//	\n  \r  \t  \\  \'  \"  \0  \xHH  \uHHHH  \<newline>
//
// On success it returns (normalized-literal, "") where the normalized literal
// still carries the surrounding quotes and is in a form that strconv.Unquote
// can decode (TOL-specific escapes are translated to Go equivalents).
// On error it returns ("", errorMessage).
func (l *Lexer) readString(quote byte) (string, string) {
	l.advance() // consume opening quote
	var buf strings.Builder
	buf.WriteByte(quote) // re-emit opening quote in normalised form
	for !l.eof() {
		ch := l.peek()
		if ch == quote {
			l.advance()
			buf.WriteByte(quote) // closing quote
			return buf.String(), ""
		}
		if ch == '\n' || ch == '\r' {
			// Unescaped newline inside string literal — terminate (error).
			return "", "unterminated string literal: unescaped newline"
		}
		if ch != '\\' {
			l.advance()
			buf.WriteByte(ch)
			continue
		}
		// Backslash: read the escape sequence.
		l.advance() // consume '\'
		if l.eof() {
			return "", "unterminated escape sequence in string literal"
		}
		esc := l.peek()
		l.advance() // consume the character after '\'
		switch esc {
		case 'n':
			buf.WriteString(`\n`)
		case 'r':
			buf.WriteString(`\r`)
		case 't':
			buf.WriteString(`\t`)
		case '\\':
			buf.WriteString(`\\`)
		case '\'':
			// Single-quote escape: valid in both quote styles.
			// In a double-quoted Go string literal, '\'' is not a valid Go escape,
			// but a bare ' is fine. Store as a literal single-quote char.
			if quote == '"' {
				buf.WriteByte('\'')
			} else {
				buf.WriteString(`\'`)
			}
		case '"':
			buf.WriteString(`\"`)
		case '0':
			// \0 → null byte; convert to \x00 so strconv.Unquote can handle it.
			buf.WriteString(`\x00`)
		case 'x':
			// \xHH — two hex digits.
			h1, h2, ok := l.readTwoHex()
			if !ok {
				return "", "invalid \\x escape: expected two hex digits"
			}
			buf.WriteString(fmt.Sprintf(`\x%02x`, h1*16+h2))
		case 'u':
			// \uHHHH — four hex digits (Unicode code point, UTF-8 encoded).
			r, ok := l.readFourHex()
			if !ok {
				return "", "invalid \\u escape: expected four hex digits"
			}
			// Encode the Unicode code point as UTF-8 in the buffer.
			// We emit a Go-compatible \u escape so strconv.Unquote can decode it.
			if r > 0x10FFFF || (r >= 0xD800 && r <= 0xDFFF) {
				return "", fmt.Sprintf("invalid Unicode code point U+%04X in \\u escape", r)
			}
			var runeBuf [utf8.UTFMax]byte
			n := utf8.EncodeRune(runeBuf[:], rune(r))
			for i := 0; i < n; i++ {
				buf.WriteByte(runeBuf[i])
			}
		case '\n':
			// \<newline> — line continuation: ignore both the backslash and the newline.
		case '\r':
			// \<CR> — also skip (handles \r\n line endings).
			if !l.eof() && l.peek() == '\n' {
				l.advance() // consume \n of \r\n pair
			}
		default:
			return "", fmt.Sprintf("unknown escape sequence '\\%c' in string literal", esc)
		}
	}
	return "", "unterminated string literal"
}

// readTwoHex reads two hex digits and returns their values (high nibble, low nibble) and ok.
func (l *Lexer) readTwoHex() (uint8, uint8, bool) {
	if l.eof() {
		return 0, 0, false
	}
	h1 := hexVal(l.peek())
	if h1 < 0 {
		return 0, 0, false
	}
	l.advance()
	if l.eof() {
		return 0, 0, false
	}
	h2 := hexVal(l.peek())
	if h2 < 0 {
		return 0, 0, false
	}
	l.advance()
	return uint8(h1), uint8(h2), true
}

// readFourHex reads four hex digits and returns the resulting uint32 value and ok.
func (l *Lexer) readFourHex() (uint32, bool) {
	var r uint32
	for i := 0; i < 4; i++ {
		if l.eof() {
			return 0, false
		}
		v := hexVal(l.peek())
		if v < 0 {
			return 0, false
		}
		l.advance()
		r = r*16 + uint32(v)
	}
	return r, true
}

// hexVal returns the numeric value (0-15) of a hex digit, or -1 if not a hex digit.
func hexVal(ch byte) int {
	switch {
	case ch >= '0' && ch <= '9':
		return int(ch - '0')
	case ch >= 'a' && ch <= 'f':
		return int(ch-'a') + 10
	case ch >= 'A' && ch <= 'F':
		return int(ch-'A') + 10
	}
	return -1
}

// readHexString reads a hex string literal after the leading 'hex' identifier has
// already been consumed.  The cursor is positioned at the opening quote character.
// It advances past the closing quote and returns the decoded bytes as a lowercase
// hexadecimal string (no "0x" prefix, no underscores) or an error if the content
// is invalid.  The Token.Literal for TokenHexString is therefore a plain lowercase
// hex string whose length is always even (two chars per byte).
func (l *Lexer) readHexString(quote byte) (string, error) {
	l.advance() // opening quote
	var content strings.Builder
	// hexCount tracks how many hex digits have been read since the last '_' (or
	// since the start).  Solidity requires underscores to appear only between
	// complete byte pairs, i.e. hexCount must equal exactly 2 before an '_' is
	// accepted.
	hexCount := 0
	for !l.eof() {
		ch := l.peek()
		if ch == quote {
			// Trailing underscore check: the previous character was '_', which
			// means hexCount was reset to 0 without any following hex digits.
			// We detect this via the total content length: if a '_' was the
			// last thing consumed, hexCount is 0 but we just started a new
			// pair — we track this with a separate flag instead.
			l.advance() // closing quote
			break
		}
		if ch == '_' {
			// Leading underscore: no hex digits consumed yet at all.
			if content.Len() == 0 && hexCount == 0 {
				l.advance()
				return "", fmt.Errorf("hex string literal: leading underscore separator")
			}
			// Consecutive underscores: hexCount would be 0 from a previous '_' reset.
			if hexCount == 0 {
				l.advance()
				return "", fmt.Errorf("hex string literal: consecutive underscores")
			}
			// Underscore must appear between complete byte pairs (hexCount == 2).
			if hexCount != 2 {
				l.advance()
				return "", fmt.Errorf("hex string literal: underscore must appear between byte pairs")
			}
			// Peek ahead: if the next character after this '_' is the closing
			// quote, it is a trailing underscore.
			l.advance() // consume '_'
			if !l.eof() && l.peek() == quote {
				return "", fmt.Errorf("hex string literal: trailing underscore separator")
			}
			hexCount = 0
			continue
		}
		if isHexDigit(ch) {
			content.WriteByte(ch)
			hexCount++
			l.advance()
			continue
		}
		// Invalid character inside hex string.
		l.advance()
		return "", fmt.Errorf("invalid character '%c' in hex string literal", ch)
	}
	raw := content.String()
	if len(raw)%2 != 0 {
		return "", fmt.Errorf("hex string literal has odd number of hex digits (%d); must be even", len(raw))
	}
	// Validate that it's valid hex (isHexDigit guarantees this, but run through
	// hex.DecodeString to normalise to lowercase as a canonical form).
	if len(raw) > 0 {
		if _, err := hex.DecodeString(raw); err != nil {
			return "", fmt.Errorf("invalid hex string literal: %v", err)
		}
	}
	return strings.ToLower(raw), nil
}

// PeekNextNonDoc scans ahead to return the next non-doc-comment token without
// consuming any tokens from the lexer. The lexer state is unchanged after the call.
func (l *Lexer) PeekNextNonDoc() Token {
	saved := Lexer{src: l.src, idx: l.idx, line: l.line, col: l.col}
	for {
		tok := saved.Next()
		if tok.Type != TokenDocComment {
			return tok
		}
	}
}

// PeekToken returns the next token without consuming it.
// The lexer state is restored after the peek so that the next call to Next()
// will return the same token.
func (l *Lexer) PeekToken() Token {
	savedIdx := l.idx
	savedLine := l.line
	savedCol := l.col
	tok := l.Next()
	l.idx = savedIdx
	l.line = savedLine
	l.col = savedCol
	return tok
}

// PeekSecond returns the second upcoming token (skipping doc comments) without
// consuming any tokens. Used for two-token lookahead disambiguation.
func (l *Lexer) PeekSecond() Token {
	savedIdx := l.idx
	savedLine := l.line
	savedCol := l.col
	// Consume the first non-doc token.
	for {
		tok := l.Next()
		if tok.Type != TokenDocComment {
			break
		}
	}
	// Peek the second non-doc token.
	var second Token
	for {
		second = l.Next()
		if second.Type != TokenDocComment {
			break
		}
	}
	// Restore lexer state.
	l.idx = savedIdx
	l.line = savedLine
	l.col = savedCol
	return second
}

func (l *Lexer) eof() bool {
	return l.idx >= len(l.src)
}

func (l *Lexer) peek() byte {
	return l.src[l.idx]
}

func (l *Lexer) peekN(n int) byte {
	if l.idx+n >= len(l.src) {
		return 0
	}
	return l.src[l.idx+n]
}

func (l *Lexer) advance() {
	if l.eof() {
		return
	}
	ch := l.src[l.idx]
	l.idx++
	if ch == '\n' {
		l.line++
		l.col = 1
		return
	}
	l.col++
}

func (l *Lexer) pos() Position {
	return Position{
		Offset: l.idx,
		Line:   l.line,
		Column: l.col,
	}
}

func (l *Lexer) lastPos() Position {
	if l.col <= 1 {
		return Position{
			Offset: l.idx,
			Line:   l.line - 1,
			Column: 1,
		}
	}
	return Position{
		Offset: l.idx,
		Line:   l.line,
		Column: l.col - 1,
	}
}

func isSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || ch == '$'
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || isDigit(ch)
}

// normalizeTypeAlias maps Solidity-compatible uint/int type names to their
// canonical TOL equivalents (u/i prefix). This normalization happens at lex
// time so that the parser and semantic analysis only ever see the canonical
// forms.
//
//	uint8..uint256  →  u8..u256  (32 variants, multiples of 8)
//	int8..int256    →  i8..i256  (32 variants, multiples of 8)
//	uint            →  u256      (bare alias)
//	int             →  i256      (bare alias)
func normalizeTypeAlias(lit string) string {
	if lit == "uint" {
		return "u256"
	}
	if lit == "int" {
		return "i256"
	}
	if strings.HasPrefix(lit, "uint") {
		rest := lit[4:] // digits after "uint"
		if isIntWidth(rest) {
			return "u" + rest
		}
	}
	if strings.HasPrefix(lit, "int") {
		rest := lit[3:] // digits after "int"
		if isIntWidth(rest) {
			return "i" + rest
		}
	}
	return lit
}

// isIntWidth returns true if s is a decimal string representing a valid integer
// type width: 8, 16, 24, ..., 256 (multiples of 8 up to 256).
func isIntWidth(s string) bool {
	if len(s) == 0 || len(s) > 3 {
		return false
	}
	n := 0
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return false
		}
		n = n*10 + int(c-'0')
	}
	return n >= 8 && n <= 256 && n%8 == 0
}
