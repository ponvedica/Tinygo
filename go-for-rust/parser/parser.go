// Package parser implements the Ore language recursive-descent parser.
// Key additions over a plain parser:
//   - Parses `let [mut] name: Type = expr`
//   - Parses lifetime generic params: <'a, 'b>
//   - Parses type annotations: &T, &mut T, &'a T, &'a mut T
//   - Parses borrow expressions: &expr, &mut expr
//   - Parses fn, struct, impl declarations
//   - Pratt precedence climbing for expressions
package parser

import (
	"fmt"
	"strconv"

	"goforust/ast"
	"goforust/lexer"
)

// Parser holds the lexer and a two-token lookahead.
type Parser struct {
	l      *lexer.Lexer
	cur    lexer.Token
	peek   lexer.Token
	errors []string
}

// New creates a Parser pre-loaded with the first two tokens.
func New(l *lexer.Lexer) *Parser {
	p := &Parser{l: l}
	p.advance()
	p.advance()
	return p
}

// Errors returns any parse errors.
func (p *Parser) Errors() []string { return p.errors }

func (p *Parser) advance() {
	p.cur = p.peek
	p.peek = p.l.NextToken()
}

func (p *Parser) curIs(tt lexer.TokenType) bool  { return p.cur.Type == tt }
func (p *Parser) peekIs(tt lexer.TokenType) bool { return p.peek.Type == tt }

func (p *Parser) expect(tt lexer.TokenType) (lexer.Token, error) {
	if p.cur.Type != tt {
		err := fmt.Errorf("L%d:%d: expected %s, got %s (%q)",
			p.cur.Line, p.cur.Col, tt, p.cur.Type, p.cur.Literal)
		p.errors = append(p.errors, err.Error())
		return p.cur, err
	}
	tok := p.cur
	p.advance()
	return tok, nil
}

// expectOpt consumes a token if present, returns whether it was present.
func (p *Parser) expectOpt(tt lexer.TokenType) bool {
	if p.curIs(tt) {
		p.advance()
		return true
	}
	return false
}

// ────────────────────────────────────────────────────────────────────────────
// File
// ────────────────────────────────────────────────────────────────────────────

// ParseFile parses a top-level Ore source file.
func (p *Parser) ParseFile() (*ast.File, error) {
	file := &ast.File{}

	// optional: mod <name>;
	if p.curIs(lexer.MOD) {
		p.advance()
		name, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, err
		}
		file.Module = name.Literal
		p.expectOpt(lexer.SEMICOLON)
	}

	for !p.curIs(lexer.EOF) {
		decl, err := p.parseDecl()
		if err != nil {
			return nil, err
		}
		if decl != nil {
			file.Decls = append(file.Decls, decl)
		}
	}
	return file, nil
}

func (p *Parser) parseDecl() (ast.Decl, error) {
	// Skip pub keyword
	if p.curIs(lexer.PUB) {
		p.advance()
	}
	switch p.cur.Type {
	case lexer.FN:
		return p.parseFnDecl()
	case lexer.STRUCT:
		return p.parseStructDecl()
	case lexer.IMPL:
		return p.parseImplBlock()
	default:
		err := fmt.Errorf("L%d: unexpected token at top level: %s", p.cur.Line, p.cur.Type)
		p.errors = append(p.errors, err.Error())
		p.advance()
		return nil, err
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Declarations
// ────────────────────────────────────────────────────────────────────────────

// parseFnDecl parses: fn name<'a>(params) -> RetType { body }
func (p *Parser) parseFnDecl() (*ast.FnDecl, error) {
	p.advance() // consume 'fn'

	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, err
	}

	// Generic lifetime params: <'a, 'b>
	var lifetimes []ast.LifetimeParam
	if p.curIs(lexer.LT) {
		p.advance()
		for !p.curIs(lexer.GT) && !p.curIs(lexer.EOF) {
			if p.curIs(lexer.LIFETIME) {
				lifetimes = append(lifetimes, ast.LifetimeParam{Name: p.cur.Literal})
				p.advance()
			}
			p.expectOpt(lexer.COMMA)
		}
		if _, err := p.expect(lexer.GT); err != nil {
			return nil, err
		}
	}

	// Parameters
	if _, err := p.expect(lexer.LPAREN); err != nil {
		return nil, err
	}
	var params []ast.Field
	for !p.curIs(lexer.RPAREN) && !p.curIs(lexer.EOF) {
		// &self or self shorthand
		if p.curIs(lexer.SELF) {
			params = append(params, ast.Field{Name: "self", Type: &ast.TypeExpr{Name: "Self", Mode: ast.Owned}})
			p.advance()
			p.expectOpt(lexer.COMMA)
			continue
		}
		if p.curIs(lexer.AMP) {
			// &self
			p.advance()
			if p.curIs(lexer.SELF) {
				params = append(params, ast.Field{Name: "self", Type: &ast.TypeExpr{Name: "Self", Mode: ast.Shared}})
				p.advance()
				p.expectOpt(lexer.COMMA)
				continue
			}
		}
		if p.curIs(lexer.AMPMUT) {
			// &mut self
			p.advance()
			if p.curIs(lexer.SELF) {
				params = append(params, ast.Field{Name: "self", Type: &ast.TypeExpr{Name: "Self", Mode: ast.MutBorrow}})
				p.advance()
				p.expectOpt(lexer.COMMA)
				continue
			}
		}

		paramName, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.COLON); err != nil {
			return nil, err
		}
		paramType, err := p.parseTypeExpr()
		if err != nil {
			return nil, err
		}
		params = append(params, ast.Field{Name: paramName.Literal, Type: paramType})
		p.expectOpt(lexer.COMMA)
	}
	if _, err := p.expect(lexer.RPAREN); err != nil {
		return nil, err
	}

	// Return type: -> Type
	var retType *ast.TypeExpr
	if p.curIs(lexer.ARROW) {
		p.advance()
		retType, err = p.parseTypeExpr()
		if err != nil {
			return nil, err
		}
	}

	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}

	return &ast.FnDecl{
		Name:       name.Literal,
		Lifetimes:  lifetimes,
		Params:     params,
		ReturnType: retType,
		Body:       body,
	}, nil
}

// parseStructDecl parses: struct Name { field: Type, ... }
func (p *Parser) parseStructDecl() (*ast.StructDecl, error) {
	p.advance() // consume 'struct'
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.LBRACE); err != nil {
		return nil, err
	}
	var fields []ast.Field
	for !p.curIs(lexer.RBRACE) && !p.curIs(lexer.EOF) {
		fieldName, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.COLON); err != nil {
			return nil, err
		}
		fieldType, err := p.parseTypeExpr()
		if err != nil {
			return nil, err
		}
		fields = append(fields, ast.Field{Name: fieldName.Literal, Type: fieldType})
		p.expectOpt(lexer.COMMA)
	}
	if _, err := p.expect(lexer.RBRACE); err != nil {
		return nil, err
	}
	return &ast.StructDecl{Name: name.Literal, Fields: fields}, nil
}

// parseImplBlock parses: impl<'a> TypeName { fn ... }
func (p *Parser) parseImplBlock() (*ast.ImplBlock, error) {
	p.advance() // consume 'impl'

	var lifetimes []ast.LifetimeParam
	if p.curIs(lexer.LT) {
		p.advance()
		for !p.curIs(lexer.GT) && !p.curIs(lexer.EOF) {
			if p.curIs(lexer.LIFETIME) {
				lifetimes = append(lifetimes, ast.LifetimeParam{Name: p.cur.Literal})
				p.advance()
			}
			p.expectOpt(lexer.COMMA)
		}
		if _, err := p.expect(lexer.GT); err != nil {
			return nil, err
		}
	}

	typeName, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.LBRACE); err != nil {
		return nil, err
	}
	var methods []*ast.FnDecl
	for !p.curIs(lexer.RBRACE) && !p.curIs(lexer.EOF) {
		if p.curIs(lexer.PUB) {
			p.advance()
		}
		if p.curIs(lexer.FN) {
			fn, err := p.parseFnDecl()
			if err != nil {
				return nil, err
			}
			methods = append(methods, fn)
		} else {
			p.advance()
		}
	}
	if _, err := p.expect(lexer.RBRACE); err != nil {
		return nil, err
	}
	return &ast.ImplBlock{TypeName: typeName.Literal, Lifetimes: lifetimes, Methods: methods}, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Type expressions
// ────────────────────────────────────────────────────────────────────────────

// parseTypeExpr parses a type annotation: Int, &String, &mut String, &'a String, &'a mut String
func (p *Parser) parseTypeExpr() (*ast.TypeExpr, error) {
	// Reference type
	if p.curIs(lexer.AMPMUT) {
		p.advance()
		name := p.parseBaseTypeName()
		return &ast.TypeExpr{Name: name, Mode: ast.MutBorrow}, nil
	}
	if p.curIs(lexer.AMP) {
		p.advance()
		lifetime := ""
		if p.curIs(lexer.LIFETIME) {
			lifetime = p.cur.Literal
			p.advance()
		}
		isMut := false
		if p.curIs(lexer.MUT) {
			isMut = true
			p.advance()
		}
		name := p.parseBaseTypeName()
		mode := ast.Shared
		if isMut {
			mode = ast.MutBorrow
		}
		return &ast.TypeExpr{Name: name, Mode: mode, Lifetime: lifetime}, nil
	}
	// Owned type
	name := p.parseBaseTypeName()
	return &ast.TypeExpr{Name: name, Mode: ast.Owned}, nil
}

func (p *Parser) parseBaseTypeName() string {
	switch p.cur.Type {
	case lexer.TY_INT:
		p.advance()
		return "Int"
	case lexer.TY_STRING:
		p.advance()
		return "String"
	case lexer.TY_BOOL:
		p.advance()
		return "Bool"
	case lexer.IDENT:
		name := p.cur.Literal
		p.advance()
		return name
	}
	return "Unknown"
}

// ────────────────────────────────────────────────────────────────────────────
// Statements
// ────────────────────────────────────────────────────────────────────────────

func (p *Parser) parseBlock() (*ast.BlockStmt, error) {
	if _, err := p.expect(lexer.LBRACE); err != nil {
		return nil, err
	}
	block := &ast.BlockStmt{}
	for !p.curIs(lexer.RBRACE) && !p.curIs(lexer.EOF) {
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		if stmt != nil {
			block.Stmts = append(block.Stmts, stmt)
		}
		p.expectOpt(lexer.SEMICOLON)
	}
	if _, err := p.expect(lexer.RBRACE); err != nil {
		return nil, err
	}
	return block, nil
}

func (p *Parser) parseStmt() (ast.Stmt, error) {
	switch p.cur.Type {
	case lexer.LET:
		return p.parseLetDecl()
	case lexer.RETURN:
		return p.parseReturn()
	case lexer.IF:
		return p.parseIf()
	case lexer.WHILE:
		return p.parseWhile()
	case lexer.FOR:
		return p.parseFor()
	case lexer.LBRACE:
		return p.parseBlock()
	case lexer.DROP:
		return p.parseExplicitDrop()
	default:
		return p.parseSimpleStmt()
	}
}

func (p *Parser) parseLetDecl() (*ast.LetDecl, error) {
	p.advance() // consume 'let'
	mutable := false
	if p.curIs(lexer.MUT) {
		mutable = true
		p.advance()
	}
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, err
	}
	var typeAnnotation *ast.TypeExpr
	if p.curIs(lexer.COLON) {
		p.advance()
		typeAnnotation, err = p.parseTypeExpr()
		if err != nil {
			return nil, err
		}
	}
	var value ast.Expr
	if p.curIs(lexer.ASSIGN) {
		p.advance()
		value, err = p.parseExpr(0)
		if err != nil {
			return nil, err
		}
	}
	return &ast.LetDecl{
		Name:    name.Literal,
		Mutable: mutable,
		Type:    typeAnnotation,
		Value:   value,
	}, nil
}

func (p *Parser) parseReturn() (*ast.ReturnStmt, error) {
	p.advance() // consume 'return'
	if p.curIs(lexer.SEMICOLON) || p.curIs(lexer.RBRACE) {
		return &ast.ReturnStmt{}, nil
	}
	val, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}
	return &ast.ReturnStmt{Value: val}, nil
}

func (p *Parser) parseIf() (*ast.IfStmt, error) {
	p.advance() // consume 'if'
	cond, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}
	then, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	stmt := &ast.IfStmt{Cond: cond, Then: then}
	if p.curIs(lexer.ELSE) {
		p.advance()
		if p.curIs(lexer.IF) {
			elseIf, err := p.parseIf()
			if err != nil {
				return nil, err
			}
			stmt.Else = elseIf
		} else {
			elseBlock, err := p.parseBlock()
			if err != nil {
				return nil, err
			}
			stmt.Else = elseBlock
		}
	}
	return stmt, nil
}

func (p *Parser) parseWhile() (*ast.WhileStmt, error) {
	p.advance()
	cond, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.WhileStmt{Cond: cond, Body: body}, nil
}

func (p *Parser) parseFor() (*ast.ForStmt, error) {
	p.advance()
	init, err := p.parseSimpleStmt()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.SEMICOLON); err != nil {
		return nil, err
	}
	cond, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(lexer.SEMICOLON); err != nil {
		return nil, err
	}
	post, err := p.parseSimpleStmt()
	if err != nil {
		return nil, err
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.ForStmt{Init: init, Cond: cond, Post: post, Body: body}, nil
}

func (p *Parser) parseExplicitDrop() (*ast.DropStmt, error) {
	p.advance() // consume 'drop'
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, err
	}
	return &ast.DropStmt{VarName: name.Literal}, nil
}

func (p *Parser) parseSimpleStmt() (ast.Stmt, error) {
	expr, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}
	switch p.cur.Type {
	case lexer.ASSIGN, lexer.PLUSEQ, lexer.MINUSEQ, lexer.STAREQ, lexer.SLASHEQ:
		op := p.cur.Literal
		p.advance()
		val, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		return &ast.AssignStmt{Target: expr, Op: op, Value: val}, nil
	case lexer.INC:
		p.advance()
		return &ast.IncDecStmt{Target: expr, Op: "++"}, nil
	case lexer.DEC:
		p.advance()
		return &ast.IncDecStmt{Target: expr, Op: "--"}, nil
	}
	return &ast.ExprStmt{Expr: expr}, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Pratt expression parser
// ────────────────────────────────────────────────────────────────────────────

const (
	precLowest = 0
	precOr     = 1
	precAnd    = 2
	precEq     = 3
	precCmp    = 4
	precAdd    = 5
	precMul    = 6
	precUnary  = 7
	precCall   = 8
	precSel    = 9
)

func infixPrec(tt lexer.TokenType) int {
	switch tt {
	case lexer.OR:
		return precOr
	case lexer.AND:
		return precAnd
	case lexer.EQ, lexer.NEQ:
		return precEq
	case lexer.LT, lexer.GT, lexer.LEQ, lexer.GEQ:
		return precCmp
	case lexer.PLUS, lexer.MINUS:
		return precAdd
	case lexer.STAR, lexer.SLASH, lexer.PERCENT:
		return precMul
	case lexer.LPAREN:
		return precCall
	case lexer.DOT, lexer.DCOLON:
		return precSel
	}
	return precLowest
}

func (p *Parser) parseExpr(minPrec int) (ast.Expr, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		prec := infixPrec(p.cur.Type)
		if prec <= minPrec {
			break
		}
		switch p.cur.Type {
		case lexer.LPAREN:
			// Call
			p.advance()
			var args []ast.Expr
			for !p.curIs(lexer.RPAREN) && !p.curIs(lexer.EOF) {
				arg, err := p.parseExpr(0)
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
				p.expectOpt(lexer.COMMA)
			}
			if _, err := p.expect(lexer.RPAREN); err != nil {
				return nil, err
			}
			left = &ast.CallExpr{Func: left, Args: args}
		case lexer.DOT:
			p.advance()
			sel, err := p.expect(lexer.IDENT)
			if err != nil {
				return nil, err
			}
			left = &ast.SelectorExpr{X: left, Sel: sel.Literal, Sep: "."}
		case lexer.DCOLON:
			p.advance()
			sel, err := p.expect(lexer.IDENT)
			if err != nil {
				return nil, err
			}
			left = &ast.SelectorExpr{X: left, Sel: sel.Literal, Sep: "::"}
		default:
			op := p.cur.Literal
			p.advance()
			right, err := p.parseExpr(prec)
			if err != nil {
				return nil, err
			}
			left = &ast.BinaryExpr{Left: left, Op: op, Right: right}
		}
	}
	return left, nil
}

func (p *Parser) parsePrimary() (ast.Expr, error) {
	// Borrow: &expr or &mut expr
	if p.curIs(lexer.AMP) {
		p.advance()
		lifetime := ""
		if p.curIs(lexer.LIFETIME) {
			lifetime = p.cur.Literal
			p.advance()
		}
		isMut := p.curIs(lexer.MUT)
		if isMut {
			p.advance()
		}
		operand, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		mode := ast.Shared
		if isMut {
			mode = ast.MutBorrow
		}
		return &ast.BorrowExpr{Mode: mode, Lifetime: lifetime, Operand: operand}, nil
	}
	if p.curIs(lexer.AMPMUT) {
		p.advance()
		operand, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &ast.BorrowExpr{Mode: ast.MutBorrow, Operand: operand}, nil
	}
	// Explicit move
	if p.curIs(lexer.MOVE) {
		p.advance()
		operand, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &ast.MoveExpr{Operand: operand}, nil
	}
	// Unary
	if p.curIs(lexer.MINUS) {
		p.advance()
		op, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: "-", Operand: op}, nil
	}
	if p.curIs(lexer.BANG) {
		p.advance()
		op, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: "!", Operand: op}, nil
	}
	// Grouping
	if p.curIs(lexer.LPAREN) {
		p.advance()
		e, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.RPAREN); err != nil {
			return nil, err
		}
		return e, nil
	}
	// Int literal
	if p.curIs(lexer.INT_LIT) {
		val, err := strconv.ParseInt(p.cur.Literal, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("L%d: invalid int %q", p.cur.Line, p.cur.Literal)
		}
		p.advance()
		return &ast.IntLit{Value: val}, nil
	}
	// String literal
	if p.curIs(lexer.STR_LIT) {
		lit := p.cur.Literal
		p.advance()
		return &ast.StringLit{Value: lit}, nil
	}
	// Bool literal
	if p.curIs(lexer.BOOL_LIT) {
		val := p.cur.Literal == "true"
		p.advance()
		return &ast.BoolLit{Value: val}, nil
	}
	// Identifier — could be a plain ident or start of a struct literal.
	// Only treat `Name {` as a struct literal when the brace is on the SAME LINE
	// as the identifier — identical to how Rust resolves this ambiguity.
	if p.curIs(lexer.IDENT) {
		identTok := p.cur
		p.advance()
		// Struct literal: Name { field: val, ... } — only when on same line.
		if p.curIs(lexer.LBRACE) && p.cur.Line == identTok.Line {
			return p.parseStructLit(identTok.Literal)
		}
		return &ast.Ident{Name: identTok.Literal}, nil
	}
	err := fmt.Errorf("L%d:%d: unexpected token in expression: %s (%q)",
		p.cur.Line, p.cur.Col, p.cur.Type, p.cur.Literal)
	p.errors = append(p.errors, err.Error())
	return nil, err
}

func (p *Parser) parseStructLit(typeName string) (*ast.StructLit, error) {
	p.advance() // consume {
	var fields []ast.StructFieldVal
	for !p.curIs(lexer.RBRACE) && !p.curIs(lexer.EOF) {
		name, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.COLON); err != nil {
			return nil, err
		}
		val, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		fields = append(fields, ast.StructFieldVal{Name: name.Literal, Value: val})
		p.expectOpt(lexer.COMMA)
	}
	if _, err := p.expect(lexer.RBRACE); err != nil {
		return nil, err
	}
	return &ast.StructLit{TypeName: typeName, Fields: fields}, nil
}
