// Package lexer defines all token types for the Ore language.
// Ore is a Rust-inspired language with ownership, borrowing, and lifetimes.
package lexer

import "fmt"

// TokenType identifies the category of a lexical token.
type TokenType int

const (
	// Special
	ILLEGAL TokenType = iota
	EOF

	// Literals
	IDENT    // variable and type names
	INT_LIT  // 42
	STR_LIT  // "hello"
	BOOL_LIT // true / false
	LIFETIME // 'a  'static  'b

	// ── Keywords ───────────────────────────────────────────────────────────
	LET      // let
	MUT      // mut
	FN       // fn
	RETURN   // return
	IF       // if
	ELSE     // else
	FOR      // for
	WHILE    // while
	STRUCT   // struct
	IMPL     // impl
	SELF     // self
	MOVE     // move
	COPY     // copy  (Copy-type annotation, future use)
	DROP     // drop  (explicit drop hint)
	TRUE     // true
	FALSE    // false
	PUB      // pub
	USE      // use
	MOD      // mod
	AS       // as

	// ── Primitive type keywords ─────────────────────────────────────────────
	TY_INT    // Int
	TY_STRING // String
	TY_BOOL   // Bool
	TY_UNIT   // ()

	// ── Operators ───────────────────────────────────────────────────────────
	ASSIGN  // =
	DEFINE  // :=  (future: we use 'let' instead, kept for compat)
	PLUS    // +
	MINUS   // -
	STAR    // *
	SLASH   // /
	PERCENT // %
	BANG    // !

	// Comparison
	EQ  // ==
	NEQ // !=
	LT  // <
	GT  // >
	LEQ // <=
	GEQ // >=

	// Logical
	AND // &&
	OR  // ||

	// Compound assignment
	PLUSEQ  // +=
	MINUSEQ // -=
	STAREQ  // *=
	SLASHEQ // /=

	// Increment/Decrement
	INC // ++
	DEC // --

	// ── Reference / Ownership tokens ────────────────────────────────────────
	AMP    // &       — shared borrow
	AMPMUT // &mut    — mutable borrow (scanned as one token)

	// ── Delimiters ──────────────────────────────────────────────────────────
	LPAREN    // (
	RPAREN    // )
	LBRACE    // {
	RBRACE    // }
	LBRACKET  // [
	RBRACKET  // ]
	COMMA     // ,
	SEMICOLON // ;
	COLON     // :
	DCOLON    // ::
	DOT       // .
	ARROW     // ->
	FATARROW  // =>
	HASH      // #  (future: attributes)
	TICK      // '  (start of lifetime — scanned within LIFETIME token)
	PIPE      // |
	AT        // @
)

// keywords maps identifier strings to their keyword token type.
var keywords = map[string]TokenType{
	"let":    LET,
	"mut":    MUT,
	"fn":     FN,
	"return": RETURN,
	"if":     IF,
	"else":   ELSE,
	"for":    FOR,
	"while":  WHILE,
	"struct": STRUCT,
	"impl":   IMPL,
	"self":   SELF,
	"move":   MOVE,
	"copy":   COPY,
	"drop":   DROP,
	"true":   TRUE,
	"false":  FALSE,
	"pub":    PUB,
	"use":    USE,
	"mod":    MOD,
	"as":     AS,
	"Int":    TY_INT,
	"String": TY_STRING,
	"Bool":   TY_BOOL,
}

// LookupIdent returns the token type for an identifier string.
func LookupIdent(s string) TokenType {
	if tt, ok := keywords[s]; ok {
		return tt
	}
	return IDENT
}

// Token is a single scanned token with type, raw literal, and source position.
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Col     int
}

func (t Token) String() string {
	return fmt.Sprintf("Token{%-10s %-12q L%d:C%d}", t.Type, t.Literal, t.Line, t.Col)
}

// String returns a human-readable name for a TokenType.
func (tt TokenType) String() string {
	names := map[TokenType]string{
		ILLEGAL:   "ILLEGAL",
		EOF:       "EOF",
		IDENT:     "IDENT",
		INT_LIT:   "INT_LIT",
		STR_LIT:   "STR_LIT",
		BOOL_LIT:  "BOOL_LIT",
		LIFETIME:  "LIFETIME",
		LET:       "let",
		MUT:       "mut",
		FN:        "fn",
		RETURN:    "return",
		IF:        "if",
		ELSE:      "else",
		FOR:       "for",
		WHILE:     "while",
		STRUCT:    "struct",
		IMPL:      "impl",
		SELF:      "self",
		MOVE:      "move",
		COPY:      "copy",
		DROP:      "drop",
		TRUE:      "true",
		FALSE:     "false",
		PUB:       "pub",
		USE:       "use",
		MOD:       "mod",
		AS:        "as",
		TY_INT:    "Int",
		TY_STRING: "String",
		TY_BOOL:   "Bool",
		TY_UNIT:   "()",
		ASSIGN:    "=",
		DEFINE:    ":=",
		PLUS:      "+",
		MINUS:     "-",
		STAR:      "*",
		SLASH:     "/",
		PERCENT:   "%",
		BANG:      "!",
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
		AMP:       "&",
		AMPMUT:    "&mut",
		LPAREN:    "(",
		RPAREN:    ")",
		LBRACE:    "{",
		RBRACE:    "}",
		LBRACKET:  "[",
		RBRACKET:  "]",
		COMMA:     ",",
		SEMICOLON: ";",
		COLON:     ":",
		DCOLON:    "::",
		DOT:       ".",
		ARROW:     "->",
		FATARROW:  "=>",
		HASH:      "#",
		TICK:      "'",
		PIPE:      "|",
		AT:        "@",
	}
	if s, ok := names[tt]; ok {
		return s
	}
	return fmt.Sprintf("TokenType(%d)", int(tt))
}
