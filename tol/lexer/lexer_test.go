package lexer

import (
	"strings"
	"testing"
)

// tokenize returns all tokens (excluding EOF) from src.
func tokenize(src string) []Token {
	l := New([]byte(src))
	var toks []Token
	for {
		tok := l.Next()
		if tok.Type == TokenEOF {
			break
		}
		toks = append(toks, tok)
	}
	return toks
}

// firstTok returns the first token from src.
func firstTok(src string) Token {
	return New([]byte(src)).Next()
}

// --- Ordinary comment tests (must still be discarded) ---

func TestOrdinaryLineCommentDiscarded(t *testing.T) {
	// A plain // comment must not produce a token; first token should be 'function'.
	toks := tokenize("// hi\nfunction f() {}")
	if len(toks) == 0 || toks[0].Type != TokenKwFunction {
		t.Fatalf("expected first token 'fn', got %v", toks)
	}
}

func TestOrdinaryBlockCommentDiscarded(t *testing.T) {
	// A plain /* */ comment must not produce a token.
	toks := tokenize("/* hello */\nfunction f() {}")
	if len(toks) == 0 || toks[0].Type != TokenKwFunction {
		t.Fatalf("expected first token 'fn', got %v", toks)
	}
}

// --- TokenDocComment: triple-slash ---

func TestTripleSlashProducesDocComment(t *testing.T) {
	tok := firstTok("/// @notice hi\nfunction f() {}")
	if tok.Type != TokenDocComment {
		t.Fatalf("expected TokenDocComment, got %v (%q)", tok.Type, tok.Literal)
	}
	if !strings.Contains(tok.Literal, "@notice hi") {
		t.Fatalf("expected @notice hi in literal, got %q", tok.Literal)
	}
}

func TestTripleSlashMultiLine(t *testing.T) {
	src := "/// @notice hello\n/// @param x foo\nfunction f() {}"
	tok := firstTok(src)
	if tok.Type != TokenDocComment {
		t.Fatalf("expected TokenDocComment, got %v", tok.Type)
	}
	if !strings.Contains(tok.Literal, "@notice hello") {
		t.Fatalf("expected @notice hello in literal, got %q", tok.Literal)
	}
	if !strings.Contains(tok.Literal, "@param x foo") {
		t.Fatalf("expected @param x foo in literal, got %q", tok.Literal)
	}
	// Should be a single token (both lines merged).
	toks := tokenize(src)
	if toks[0].Type != TokenDocComment {
		t.Fatalf("first token should be DOC_COMMENT")
	}
	if toks[1].Type != TokenKwFunction {
		t.Fatalf("second token should be fn, got %v", toks[1].Type)
	}
}

func TestTripleSlashEmptyLinesMerged(t *testing.T) {
	// An empty /// line must be merged into the same TokenDocComment as adjacent /// lines.
	src := "/// @notice hi\n///\n/// @effects reads: storage.x\nfunction f() {}"
	toks := tokenize(src)
	if toks[0].Type != TokenDocComment {
		t.Fatalf("expected TokenDocComment, got %v", toks[0].Type)
	}
	// Entire three-line block must be in one token.
	lit := toks[0].Literal
	if !strings.Contains(lit, "@notice hi") {
		t.Fatalf("missing @notice hi in %q", lit)
	}
	if !strings.Contains(lit, "@effects reads") {
		t.Fatalf("missing @effects reads in %q", lit)
	}
	// The empty /// line must appear in the literal.
	lines := strings.Split(strings.TrimRight(lit, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines in merged literal, got %d: %q", len(lines), lit)
	}
	if strings.TrimSpace(lines[1]) != "///" {
		t.Fatalf("expected empty /// line as second line, got %q", lines[1])
	}
	// Next real token must be 'fn'.
	if toks[1].Type != TokenKwFunction {
		t.Fatalf("expected fn after doc comment, got %v", toks[1].Type)
	}
}

func TestBlankLineBreaksTripleSlashBlock(t *testing.T) {
	// A true blank line (no ///) must end the block → two separate TokenDocComment tokens.
	src := "/// a\n\n/// b\nfunction f() {}"
	toks := tokenize(src)
	if len(toks) < 2 {
		t.Fatalf("expected at least 2 tokens, got %d", len(toks))
	}
	if toks[0].Type != TokenDocComment {
		t.Fatalf("first token should be DOC_COMMENT, got %v", toks[0].Type)
	}
	if toks[1].Type != TokenDocComment {
		t.Fatalf("second token should be DOC_COMMENT (blank line broke the block), got %v", toks[1].Type)
	}
	if !strings.Contains(toks[0].Literal, "/// a") {
		t.Fatalf("first doc comment should contain '/// a', got %q", toks[0].Literal)
	}
	if !strings.Contains(toks[1].Literal, "/// b") {
		t.Fatalf("second doc comment should contain '/// b', got %q", toks[1].Literal)
	}
	if toks[2].Type != TokenKwFunction {
		t.Fatalf("third token should be fn, got %v", toks[2].Type)
	}
}

// --- TokenDocComment: block style /** */ ---

func TestBlockDocProducesDocComment(t *testing.T) {
	src := "/** @notice hi */\nfunction f() {}"
	tok := firstTok(src)
	if tok.Type != TokenDocComment {
		t.Fatalf("expected TokenDocComment, got %v (%q)", tok.Type, tok.Literal)
	}
	if !strings.Contains(tok.Literal, "@notice hi") {
		t.Fatalf("expected @notice hi in literal, got %q", tok.Literal)
	}
}

func TestBlockDocMultiLine(t *testing.T) {
	src := "/**\n * @notice hello\n * @param x foo\n */\nfunction f() {}"
	toks := tokenize(src)
	if toks[0].Type != TokenDocComment {
		t.Fatalf("expected TokenDocComment, got %v", toks[0].Type)
	}
	lit := toks[0].Literal
	if !strings.Contains(lit, "@notice hello") {
		t.Fatalf("expected @notice hello in literal, got %q", lit)
	}
	if !strings.Contains(lit, "@param x foo") {
		t.Fatalf("expected @param x foo in literal, got %q", lit)
	}
	if toks[1].Type != TokenKwFunction {
		t.Fatalf("second token should be fn, got %v", toks[1].Type)
	}
}

func TestSingleStarBlockCommentDiscarded(t *testing.T) {
	// /* ... */ with a single star must NOT produce a TokenDocComment.
	toks := tokenize("/* not a doc comment */\nfunction f() {}")
	if toks[0].Type != TokenKwFunction {
		t.Fatalf("expected fn (ordinary block comment discarded), got %v (%q)", toks[0].Type, toks[0].Literal)
	}
}

func TestEmptyBlockDoc(t *testing.T) {
	// /**/ is treated as an empty /** */ doc comment.
	src := "/**/function f() {}"
	toks := tokenize(src)
	if toks[0].Type != TokenDocComment {
		t.Fatalf("expected TokenDocComment for /**/, got %v", toks[0].Type)
	}
}

// --- Position tracking ---

func TestDocCommentPosition(t *testing.T) {
	src := "/// @notice hi\nfunction f() {}"
	tok := firstTok(src)
	if tok.Start.Line != 1 {
		t.Fatalf("expected doc comment on line 1, got %d", tok.Start.Line)
	}
	if tok.Start.Column != 1 {
		t.Fatalf("expected doc comment at column 1, got %d", tok.Start.Column)
	}
}

// --- Literal content preservation ---

func TestDocCommentLiteralPreservesNewlines(t *testing.T) {
	src := "/// line1\n/// line2\nfunction f() {}"
	tok := firstTok(src)
	if tok.Type != TokenDocComment {
		t.Fatalf("expected TokenDocComment")
	}
	if !strings.Contains(tok.Literal, "\n") {
		t.Fatalf("expected newline preserved in literal, got %q", tok.Literal)
	}
}

func TestDocCommentFollowedByDeclaration(t *testing.T) {
	// Verify that after a doc comment token, the parser (via lexer) sees the fn keyword.
	src := "/// @effects calls: []\nfunction transfer() {}"
	toks := tokenize(src)
	if toks[0].Type != TokenDocComment {
		t.Fatalf("expected TokenDocComment first, got %v", toks[0].Type)
	}
	if toks[1].Type != TokenKwFunction {
		t.Fatalf("expected fn second, got %v", toks[1].Type)
	}
}
