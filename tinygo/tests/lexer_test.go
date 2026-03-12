// Unit tests for the tinygo lexer.
// Tests verify token types and literals for all major token categories.
package lexer_test

import (
	"testing"

	"tinygo/lexer"
)

type tokExpect struct {
	typ lexer.TokenType
	lit string
}

func tokenize(src string) []lexer.Token {
	l := lexer.New([]byte(src))
	return l.Tokenize()
}

// expectTokens asserts that the lexed tokens match the expected list
// (ignoring the final EOF).
func expectTokens(t *testing.T, src string, want []tokExpect) {
	t.Helper()
	got := tokenize(src)
	// strip EOF
	if len(got) > 0 && got[len(got)-1].Type == lexer.EOF {
		got = got[:len(got)-1]
	}
	if len(got) != len(want) {
		t.Fatalf("token count mismatch: got %d, want %d\ntokens: %v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Type != w.typ {
			t.Errorf("token[%d] type: got %s, want %s (src=%q)", i, g.Type, w.typ, src)
		}
		if w.lit != "" && g.Literal != w.lit {
			t.Errorf("token[%d] literal: got %q, want %q", i, g.Literal, w.lit)
		}
	}
}

// ----------------------------------------------------------------------------
// Tests
// ----------------------------------------------------------------------------

func TestKeywords(t *testing.T) {
	expectTokens(t, "package main", []tokExpect{
		{lexer.PACKAGE, "package"},
		{lexer.IDENT, "main"},
	})
}

func TestIdentifiers(t *testing.T) {
	expectTokens(t, "foo bar _baz", []tokExpect{
		{lexer.IDENT, "foo"},
		{lexer.IDENT, "bar"},
		{lexer.IDENT, "_baz"},
	})
}

func TestIntLiterals(t *testing.T) {
	expectTokens(t, "0 42 1000", []tokExpect{
		{lexer.INT, "0"},
		{lexer.INT, "42"},
		{lexer.INT, "1000"},
	})
}

func TestStringLiteral(t *testing.T) {
	tokens := tokenize(`"hello, world"`)
	if len(tokens) < 1 || tokens[0].Type != lexer.STRING {
		t.Fatalf("expected STRING token, got %v", tokens)
	}
	if tokens[0].Literal != "hello, world" {
		t.Errorf("string literal: got %q, want %q", tokens[0].Literal, "hello, world")
	}
}

func TestStringEscape(t *testing.T) {
	tokens := tokenize(`"line1\nline2"`)
	if tokens[0].Literal != "line1\nline2" {
		t.Errorf("escaped string: got %q, want %q", tokens[0].Literal, "line1\nline2")
	}
}

func TestBoolLiterals(t *testing.T) {
	expectTokens(t, "true false", []tokExpect{
		{lexer.TRUE, "true"},
		{lexer.FALSE, "false"},
	})
}

func TestOperators(t *testing.T) {
	expectTokens(t, "+ - * / % == != < > <= >= && || := =", []tokExpect{
		{lexer.PLUS, "+"},
		{lexer.MINUS, "-"},
		{lexer.STAR, "*"},
		{lexer.SLASH, "/"},
		{lexer.PERCENT, "%"},
		{lexer.EQ, "=="},
		{lexer.NEQ, "!="},
		{lexer.LT, "<"},
		{lexer.GT, ">"},
		{lexer.LEQ, "<="},
		{lexer.GEQ, ">="},
		{lexer.AND, "&&"},
		{lexer.OR, "||"},
		{lexer.DEFINE, ":="},
		{lexer.ASSIGN, "="},
	})
}

func TestIncDec(t *testing.T) {
	expectTokens(t, "i++ j--", []tokExpect{
		{lexer.IDENT, "i"},
		{lexer.INC, "++"},
		{lexer.IDENT, "j"},
		{lexer.DEC, "--"},
	})
}

func TestDelimiters(t *testing.T) {
	expectTokens(t, "( ) { } [ ] , ; . :", []tokExpect{
		{lexer.LPAREN, "("},
		{lexer.RPAREN, ")"},
		{lexer.LBRACE, "{"},
		{lexer.RBRACE, "}"},
		{lexer.LBRACKET, "["},
		{lexer.RBRACKET, "]"},
		{lexer.COMMA, ","},
		{lexer.SEMICOLON, ";"},
		{lexer.DOT, "."},
		{lexer.COLON, ":"},
	})
}

func TestSingleLineComment(t *testing.T) {
	// Comment should be entirely skipped
	expectTokens(t, "a // this is a comment\nb", []tokExpect{
		{lexer.IDENT, "a"},
		{lexer.IDENT, "b"},
	})
}

func TestBlockComment(t *testing.T) {
	expectTokens(t, "a /* block comment */ b", []tokExpect{
		{lexer.IDENT, "a"},
		{lexer.IDENT, "b"},
	})
}

func TestLineNumbers(t *testing.T) {
	tokens := tokenize("a\nb\nc")
	expected := []int{1, 2, 3}
	var got []int
	for _, tok := range tokens {
		if tok.Type == lexer.EOF {
			break
		}
		got = append(got, tok.Line)
	}
	if len(got) != len(expected) {
		t.Fatalf("line count: got %d, want %d", len(got), len(expected))
	}
	for i, line := range expected {
		if got[i] != line {
			t.Errorf("token[%d] line: got %d, want %d", i, got[i], line)
		}
	}
}

func TestFuncDecl(t *testing.T) {
	src := `func add(a int, b int) int { return a }`
	expectTokens(t, src, []tokExpect{
		{lexer.FUNC, "func"},
		{lexer.IDENT, "add"},
		{lexer.LPAREN, "("},
		{lexer.IDENT, "a"},
		{lexer.IDENT, "int"},
		{lexer.COMMA, ","},
		{lexer.IDENT, "b"},
		{lexer.IDENT, "int"},
		{lexer.RPAREN, ")"},
		{lexer.IDENT, "int"},
		{lexer.LBRACE, "{"},
		{lexer.RETURN, "return"},
		{lexer.IDENT, "a"},
		{lexer.RBRACE, "}"},
	})
}

func TestPeek(t *testing.T) {
	l := lexer.New([]byte("x y"))
	p := l.Peek()
	n := l.NextToken()
	if p.Literal != n.Literal {
		t.Errorf("Peek() = %q, NextToken() = %q — should be equal", p.Literal, n.Literal)
	}
}

func TestEOF(t *testing.T) {
	tokens := tokenize("")
	if len(tokens) != 1 || tokens[0].Type != lexer.EOF {
		t.Errorf("empty source should produce single EOF token, got %v", tokens)
	}
}
