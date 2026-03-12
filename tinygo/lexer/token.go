// Package lexer defines the token types and Token struct for the tinygo compiler.
package lexer

import "fmt"

// TokenType represents the type of a lexical token.
type TokenType int

const (
	// Special
	ILLEGAL TokenType = iota
	EOF

	// Literals
	IDENT  // main, x, foo
	INT    // 42
	STRING // "hello"

	// Keywords
	PACKAGE
	IMPORT
	FUNC
	VAR
	CONST
	TYPE
	IF
	ELSE
	FOR
	RETURN
	TRUE
	FALSE

	// Operators
	ASSIGN    // =
	DEFINE    // :=
	PLUS      // +
	MINUS     // -
	STAR      // *
	SLASH     // /
	PERCENT   // %
	BANG      // !
	AMP       // &
	PIPE      // |
	EQ        // ==
	NEQ       // !=
	LT        // <
	GT        // >
	LEQ       // <=
	GEQ       // >=
	AND       // &&
	OR        // ||
	PLUSEQ    // +=
	MINUSEQ   // -=
	STAREQ    // *=
	SLASHEQ   // /=
	INC       // ++
	DEC       // --

	// Delimiters
	LPAREN    // (
	RPAREN    // )
	LBRACE    // {
	RBRACE    // }
	LBRACKET  // [
	RBRACKET  // ]
	COMMA     // ,
	SEMICOLON // ;
	COLON     // :
	DOT       // .
	NEWLINE   // \n (used internally, then discarded)
)

// keywords maps keyword strings to their token types.
var keywords = map[string]TokenType{
	"package": PACKAGE,
	"import":  IMPORT,
	"func":    FUNC,
	"var":     VAR,
	"const":   CONST,
	"type":    TYPE,
	"if":      IF,
	"else":    ELSE,
	"for":     FOR,
	"return":  RETURN,
	"true":    TRUE,
	"false":   FALSE,
}

// LookupIdent returns the token type for an identifier (keyword or IDENT).
func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return IDENT
}

// Token represents a single lexical token with its type, literal value, and position.
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Col     int
}

func (t Token) String() string {
	return fmt.Sprintf("Token{%s %q L%d:C%d}", t.Type, t.Literal, t.Line, t.Col)
}

// String returns a human-readable name for a TokenType.
func (tt TokenType) String() string {
	names := map[TokenType]string{
		ILLEGAL:   "ILLEGAL",
		EOF:       "EOF",
		IDENT:     "IDENT",
		INT:       "INT",
		STRING:    "STRING",
		PACKAGE:   "PACKAGE",
		IMPORT:    "IMPORT",
		FUNC:      "FUNC",
		VAR:       "VAR",
		CONST:     "CONST",
		TYPE:      "TYPE",
		IF:        "IF",
		ELSE:      "ELSE",
		FOR:       "FOR",
		RETURN:    "RETURN",
		TRUE:      "TRUE",
		FALSE:     "FALSE",
		ASSIGN:    "=",
		DEFINE:    ":=",
		PLUS:      "+",
		MINUS:     "-",
		STAR:      "*",
		SLASH:     "/",
		PERCENT:   "%",
		BANG:      "!",
		AMP:       "&",
		PIPE:      "|",
		EQ:        "==",
		NEQ:       "!=",
		LT:        "<",
		GT:        ">",
		LEQ:       "<=",
		GEQ:       ">=",
		AND:       "&&",
		OR:        "||",
		PLUSEQ:    "+=",
		MINUSEQ:   "-=",
		STAREQ:    "*=",
		SLASHEQ:   "/=",
		INC:       "++",
		DEC:       "--",
		LPAREN:    "(",
		RPAREN:    ")",
		LBRACE:    "{",
		RBRACE:    "}",
		LBRACKET:  "[",
		RBRACKET:  "]",
		COMMA:     ",",
		SEMICOLON: ";",
		COLON:     ":",
		DOT:       ".",
		NEWLINE:   "NEWLINE",
	}
	if s, ok := names[tt]; ok {
		return s
	}
	return "UNKNOWN"
}
