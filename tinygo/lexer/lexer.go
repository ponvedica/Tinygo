// Package lexer implements the tokenizer (lexer) for the tinygo compiler.
package lexer

import (
	"fmt"
	"strings"
)

// Lexer holds state for scanning a Go source file.
type Lexer struct {
	src  []byte
	pos  int // current position in src
	line int
	col  int
	// peek buffer
	peeked []Token
}

// New creates a new Lexer from source bytes.
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

// NextToken returns the next token and advances the lexer.
func (l *Lexer) NextToken() Token {
	if len(l.peeked) > 0 {
		tok := l.peeked[0]
		l.peeked = l.peeked[1:]
		return tok
	}
	return l.next()
}

// Tokenize returns all tokens from the source.
func (l *Lexer) Tokenize() []Token {
	var tokens []Token
	for {
		tok := l.NextToken()
		tokens = append(tokens, tok)
		if tok.Type == EOF {
			break
		}
	}
	return tokens
}

// next reads the next token from the source.
func (l *Lexer) next() Token {
	l.skipWhitespaceAndComments()

	if l.pos >= len(l.src) {
		return l.tok(EOF, "")
	}

	ch := l.src[l.pos]

	// String literal
	if ch == '"' {
		return l.readString()
	}

	// Number literal
	if isDigit(ch) {
		return l.readInt()
	}

	// Identifier or keyword
	if isLetter(ch) {
		return l.readIdent()
	}

	// Operators and delimiters
	return l.readSymbol()
}

func (l *Lexer) readIdent() Token {
	start := l.pos
	scol := l.col
	sline := l.line
	for l.pos < len(l.src) && (isLetter(l.src[l.pos]) || isDigit(l.src[l.pos])) {
		l.pos++
		l.col++
	}
	lit := string(l.src[start:l.pos])
	tt := LookupIdent(lit)
	return Token{Type: tt, Literal: lit, Line: sline, Col: scol}
}

func (l *Lexer) readInt() Token {
	start := l.pos
	scol := l.col
	sline := l.line
	for l.pos < len(l.src) && isDigit(l.src[l.pos]) {
		l.pos++
		l.col++
	}
	return Token{Type: INT, Literal: string(l.src[start:l.pos]), Line: sline, Col: scol}
}

func (l *Lexer) readString() Token {
	sline := l.line
	scol := l.col
	l.pos++ // skip opening "
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
	return Token{Type: STRING, Literal: sb.String(), Line: sline, Col: scol}
}

func (l *Lexer) readSymbol() Token {
	ch := l.src[l.pos]
	// Look ahead for two-char operators
	var next byte
	if l.pos+1 < len(l.src) {
		next = l.src[l.pos+1]
	}

	advance1 := func(tt TokenType, lit string) Token {
		tok := Token{Type: tt, Literal: lit, Line: l.line, Col: l.col}
		l.pos++
		l.col++
		return tok
	}
	advance2 := func(tt TokenType, lit string) Token {
		tok := Token{Type: tt, Literal: lit, Line: l.line, Col: l.col}
		l.pos += 2
		l.col += 2
		return tok
	}

	switch ch {
	case '+':
		if next == '+' {
			return advance2(INC, "++")
		}
		if next == '=' {
			return advance2(PLUSEQ, "+=")
		}
		return advance1(PLUS, "+")
	case '-':
		if next == '-' {
			return advance2(DEC, "--")
		}
		if next == '=' {
			return advance2(MINUSEQ, "-=")
		}
		return advance1(MINUS, "-")
	case '*':
		if next == '=' {
			return advance2(STAREQ, "*=")
		}
		return advance1(STAR, "*")
	case '/':
		if next == '=' {
			return advance2(SLASHEQ, "/=")
		}
		return advance1(SLASH, "/")
	case '%':
		return advance1(PERCENT, "%")
	case '=':
		if next == '=' {
			return advance2(EQ, "==")
		}
		return advance1(ASSIGN, "=")
	case '!':
		if next == '=' {
			return advance2(NEQ, "!=")
		}
		return advance1(BANG, "!")
	case '<':
		if next == '=' {
			return advance2(LEQ, "<=")
		}
		return advance1(LT, "<")
	case '>':
		if next == '=' {
			return advance2(GEQ, ">=")
		}
		return advance1(GT, ">")
	case '&':
		if next == '&' {
			return advance2(AND, "&&")
		}
		return advance1(AMP, "&")
	case '|':
		if next == '|' {
			return advance2(OR, "||")
		}
		return advance1(PIPE, "|")
	case ':':
		if next == '=' {
			return advance2(DEFINE, ":=")
		}
		return advance1(COLON, ":")
	case '(':
		return advance1(LPAREN, "(")
	case ')':
		return advance1(RPAREN, ")")
	case '{':
		return advance1(LBRACE, "{")
	case '}':
		return advance1(RBRACE, "}")
	case '[':
		return advance1(LBRACKET, "[")
	case ']':
		return advance1(RBRACKET, "]")
	case ',':
		return advance1(COMMA, ",")
	case ';':
		return advance1(SEMICOLON, ";")
	case '.':
		return advance1(DOT, ".")
	default:
		tok := Token{Type: ILLEGAL, Literal: fmt.Sprintf("%c", ch), Line: l.line, Col: l.col}
		l.pos++
		l.col++
		return tok
	}
}

// skipWhitespaceAndComments skips spaces, tabs, newlines, and // comments.
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
		case ch == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/':
			// Single-line comment: skip until newline
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
		case ch == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*':
			// Block comment
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
