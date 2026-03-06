// Package lexer implements the Ore language tokenizer.
// Key extensions over a vanilla lexer:
//   - Lifetime tokens: 'a  'static  'b  (tick + identifier)
//   - &mut scanned as a single AMPMUT token
//   - -> scanned as ARROW
//   - :: scanned as DCOLON
package lexer

import (
	"fmt"
	"strings"
)

// Lexer scans Ore source bytes into tokens.
type Lexer struct {
	src    []byte
	pos    int
	line   int
	col    int
	peeked []Token
}

// New creates a Lexer from source bytes.
func New(src []byte) *Lexer {
	return &Lexer{src: src, pos: 0, line: 1, col: 1}
}

// Peek returns the next token without consuming it.
func (l *Lexer) Peek() Token {
	if len(l.peeked) == 0 {
		l.peeked = append(l.peeked, l.next())
	}
	return l.peeked[0]
}

// NextToken consumes and returns the next token.
func (l *Lexer) NextToken() Token {
	if len(l.peeked) > 0 {
		tok := l.peeked[0]
		l.peeked = l.peeked[1:]
		return tok
	}
	return l.next()
}

// Tokenize returns all tokens (including EOF).
func (l *Lexer) Tokenize() []Token {
	var out []Token
	for {
		tok := l.NextToken()
		out = append(out, tok)
		if tok.Type == EOF {
			break
		}
	}
	return out
}

// next scans the next token.
func (l *Lexer) next() Token {
	l.skipWhitespaceAndComments()
	if l.pos >= len(l.src) {
		return l.tok(EOF, "")
	}

	ch := l.src[l.pos]

	// Lifetime: 'identifier  (e.g. 'a, 'static)
	if ch == '\'' {
		return l.readLifetime()
	}

	// String literal
	if ch == '"' {
		return l.readString()
	}

	// Number
	if isDigit(ch) {
		return l.readInt()
	}

	// Identifier / keyword
	if isLetter(ch) {
		return l.readIdent()
	}

	// Symbols
	return l.readSymbol()
}

// readLifetime reads a 'name lifetime token.
// If the tick is not followed by a letter, it emits a bare TICK.
func (l *Lexer) readLifetime() Token {
	sline, scol := l.line, l.col
	l.pos++ // consume '
	l.col++
	if l.pos < len(l.src) && isLetter(l.src[l.pos]) {
		start := l.pos
		for l.pos < len(l.src) && (isLetter(l.src[l.pos]) || isDigit(l.src[l.pos])) {
			l.pos++
			l.col++
		}
		return Token{Type: LIFETIME, Literal: "'" + string(l.src[start:l.pos]), Line: sline, Col: scol}
	}
	return Token{Type: TICK, Literal: "'", Line: sline, Col: scol}
}

func (l *Lexer) readIdent() Token {
	start := l.pos
	sline, scol := l.line, l.col
	for l.pos < len(l.src) && (isLetter(l.src[l.pos]) || isDigit(l.src[l.pos])) {
		l.pos++
		l.col++
	}
	lit := string(l.src[start:l.pos])
	tt := LookupIdent(lit)
	// Map true/false to BOOL_LIT
	if tt == TRUE || tt == FALSE {
		return Token{Type: BOOL_LIT, Literal: lit, Line: sline, Col: scol}
	}
	return Token{Type: tt, Literal: lit, Line: sline, Col: scol}
}

func (l *Lexer) readInt() Token {
	start := l.pos
	sline, scol := l.line, l.col
	for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
		l.pos++
		l.col++
	}
	return Token{Type: INT_LIT, Literal: string(l.src[start:l.pos]), Line: sline, Col: scol}
}

func (l *Lexer) readString() Token {
	sline, scol := l.line, l.col
	l.pos++ // skip "
	l.col++
	var sb strings.Builder
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == '"' {
			l.pos++
			l.col++
			break
		}
		if ch == '\\' && l.pos+1 < len(l.src) {
			l.pos++
			l.col++
			switch l.src[l.pos] {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case '"':
				sb.WriteByte('"')
			case '\\':
				sb.WriteByte('\\')
			default:
				sb.WriteByte('\\')
				sb.WriteByte(l.src[l.pos])
			}
			l.pos++
			l.col++
			continue
		}
		sb.WriteByte(ch)
		l.pos++
		l.col++
	}
	return Token{Type: STR_LIT, Literal: sb.String(), Line: sline, Col: scol}
}

func (l *Lexer) readSymbol() Token {
	ch := l.src[l.pos]
	var peek byte
	if l.pos+1 < len(l.src) {
		peek = l.src[l.pos+1]
	}
	// 3-char lookahead for &mut
	var peek2 byte
	if l.pos+2 < len(l.src) {
		peek2 = l.src[l.pos+2]
	}

	a1 := func(tt TokenType, lit string) Token {
		tok := Token{Type: tt, Literal: lit, Line: l.line, Col: l.col}
		l.pos++
		l.col++
		return tok
	}
	a2 := func(tt TokenType, lit string) Token {
		tok := Token{Type: tt, Literal: lit, Line: l.line, Col: l.col}
		l.pos += 2
		l.col += 2
		return tok
	}
	aN := func(tt TokenType, lit string, n int) Token {
		tok := Token{Type: tt, Literal: lit, Line: l.line, Col: l.col}
		l.pos += n
		l.col += n
		return tok
	}

	switch ch {
	// &mut  →  AMPMUT  (must check before & → AMP)
	case '&':
		// &mut followed by whitespace or letter: treat as AMPMUT
		if peek == 'm' && peek2 == 'u' && l.pos+3 < len(l.src) && l.src[l.pos+3] == 't' {
			// Verify char after 'mut' is not identifier char
			afterMut := byte(0)
			if l.pos+4 < len(l.src) {
				afterMut = l.src[l.pos+4]
			}
			if !isLetter(afterMut) && !isDigit(afterMut) {
				return aN(AMPMUT, "&mut", 4)
			}
		}
		if peek == '&' {
			return a2(AND, "&&")
		}
		return a1(AMP, "&")

	case '-':
		if peek == '>' {
			return a2(ARROW, "->")
		}
		if peek == '-' {
			return a2(DEC, "--")
		}
		if peek == '=' {
			return a2(MINUSEQ, "-=")
		}
		return a1(MINUS, "-")

	case '=':
		if peek == '=' {
			return a2(EQ, "==")
		}
		if peek == '>' {
			return a2(FATARROW, "=>")
		}
		return a1(ASSIGN, "=")

	case '+':
		if peek == '+' {
			return a2(INC, "++")
		}
		if peek == '=' {
			return a2(PLUSEQ, "+=")
		}
		return a1(PLUS, "+")

	case '*':
		if peek == '=' {
			return a2(STAREQ, "*=")
		}
		return a1(STAR, "*")

	case '/':
		if peek == '=' {
			return a2(SLASHEQ, "/=")
		}
		return a1(SLASH, "/")

	case '%':
		return a1(PERCENT, "%")

	case '!':
		if peek == '=' {
			return a2(NEQ, "!=")
		}
		return a1(BANG, "!")

	case '<':
		if peek == '=' {
			return a2(LEQ, "<=")
		}
		return a1(LT, "<")

	case '>':
		if peek == '=' {
			return a2(GEQ, ">=")
		}
		return a1(GT, ">")

	case '|':
		if peek == '|' {
			return a2(OR, "||")
		}
		return a1(PIPE, "|")

	case ':':
		if peek == ':' {
			return a2(DCOLON, "::")
		}
		if peek == '=' {
			return a2(DEFINE, ":=")
		}
		return a1(COLON, ":")

	case '(':
		return a1(LPAREN, "(")
	case ')':
		return a1(RPAREN, ")")
	case '{':
		return a1(LBRACE, "{")
	case '}':
		return a1(RBRACE, "}")
	case '[':
		return a1(LBRACKET, "[")
	case ']':
		return a1(RBRACKET, "]")
	case ',':
		return a1(COMMA, ",")
	case ';':
		return a1(SEMICOLON, ";")
	case '.':
		return a1(DOT, ".")
	case '#':
		return a1(HASH, "#")
	case '@':
		return a1(AT, "@")

	default:
		tok := Token{Type: ILLEGAL, Literal: fmt.Sprintf("%c", ch), Line: l.line, Col: l.col}
		l.pos++
		l.col++
		return tok
	}
}

// skipWhitespaceAndComments skips spaces, tabs, newlines, and // / /* */ comments.
func (l *Lexer) skipWhitespaceAndComments() {
	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\r':
			l.pos++
			l.col++
		case ch == '\n':
			l.pos++
			l.line++
			l.col = 1
		// Single-line comment
		case ch == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/':
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
		// Block comment
		case ch == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*':
			l.pos += 2
			for l.pos+1 < len(l.src) {
				if l.src[l.pos] == '*' && l.src[l.pos+1] == '/' {
					l.pos += 2
					break
				}
				if l.src[l.pos] == '\n' {
					l.line++
					l.col = 1
				} else {
					l.col++
				}
				l.pos++
			}
		default:
			return
		}
	}
}

func (l *Lexer) tok(tt TokenType, lit string) Token {
	return Token{Type: tt, Literal: lit, Line: l.line, Col: l.col}
}

func isLetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}
