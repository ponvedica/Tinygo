// Package parser implements a recursive-descent parser for the tinygo compiler.
// It uses Pratt parsing (top-down operator precedence) for expressions.
package parser

import (
	"fmt"
	"strconv"

	"tinygo/ast"
	"tinygo/lexer"
)

// Parser holds the lexer and current/peek tokens.
type Parser struct {
	l       *lexer.Lexer
	cur     lexer.Token
	peek    lexer.Token
	errors  []string
}

// New creates a Parser from a Lexer, pre-loading cur and peek.
func New(l *lexer.Lexer) *Parser {
	p := &Parser{l: l}
	p.advance() // load cur
	p.advance() // load peek
	return p
}

// Errors returns any parse errors collected.
func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) advance() {
	p.cur = p.peek
	p.peek = p.l.NextToken()
}

func (p *Parser) expect(tt lexer.TokenType) (lexer.Token, error) {
	if p.cur.Type != tt {
		err := fmt.Errorf("line %d:%d: expected %s, got %s (%q)",
			p.cur.Line, p.cur.Col, tt, p.cur.Type, p.cur.Literal)
		p.errors = append(p.errors, err.Error())
		return p.cur, err
	}
	tok := p.cur
	p.advance()
	return tok, nil
}

func (p *Parser) curIs(tt lexer.TokenType) bool  { return p.cur.Type == tt }
func (p *Parser) peekIs(tt lexer.TokenType) bool { return p.peek.Type == tt }

// ----------------------------------------------------------------------------
// Top-level
// ----------------------------------------------------------------------------

// ParseFile parses an entire source file into an *ast.File.
func (p *Parser) ParseFile() (*ast.File, error) {
	file := &ast.File{}

	// package clause
	if _, err := p.expect(lexer.PACKAGE); err != nil {
		return nil, err
	}
	nameTok, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, err
	}
	file.Package = nameTok.Literal

	// import(s)
	for p.curIs(lexer.IMPORT) {
		p.advance()
		if p.curIs(lexer.LPAREN) {
			p.advance()
			for !p.curIs(lexer.RPAREN) && !p.curIs(lexer.EOF) {
				if p.curIs(lexer.STRING) {
					file.Imports = append(file.Imports, p.cur.Literal)
					p.advance()
				} else {
					p.advance()
				}
			}
			p.advance() // consume )
		} else if p.curIs(lexer.STRING) {
			file.Imports = append(file.Imports, p.cur.Literal)
			p.advance()
		}
	}

	// top-level declarations
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
	switch p.cur.Type {
	case lexer.FUNC:
		return p.parseFuncDecl()
	case lexer.VAR:
		return p.parseVarDeclTop()
	default:
		err := fmt.Errorf("line %d:%d: unexpected token at top-level: %s (%q)",
			p.cur.Line, p.cur.Col, p.cur.Type, p.cur.Literal)
		p.errors = append(p.errors, err.Error())
		p.advance()
		return nil, err
	}
}

func (p *Parser) parseFuncDecl() (*ast.FuncDecl, error) {
	p.advance() // consume 'func'

	nameTok, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(lexer.LPAREN); err != nil {
		return nil, err
	}

	// Parse parameters
	var params []ast.Field
	for !p.curIs(lexer.RPAREN) && !p.curIs(lexer.EOF) {
		// name type, name type, ...
		pName, err := p.expect(lexer.IDENT)
		if err != nil {
			return nil, err
		}
		pType := p.parseTypeName()
		params = append(params, ast.Field{Name: pName.Literal, Type: pType})
		if p.curIs(lexer.COMMA) {
			p.advance()
		}
	}
	if _, err := p.expect(lexer.RPAREN); err != nil {
		return nil, err
	}

	// Optional return type
	var returnType string
	if !p.curIs(lexer.LBRACE) {
		returnType = p.parseTypeName()
	}

	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}

	return &ast.FuncDecl{
		Name:       nameTok.Literal,
		Params:     params,
		ReturnType: returnType,
		Body:       body,
	}, nil
}

func (p *Parser) parseVarDeclTop() (*ast.VarDeclTop, error) {
	p.advance() // consume 'var'
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, err
	}
	typeName := ""
	if !p.curIs(lexer.ASSIGN) && !p.curIs(lexer.EOF) {
		typeName = p.parseTypeName()
	}
	var val ast.Expr
	if p.curIs(lexer.ASSIGN) {
		p.advance()
		val, err = p.parseExpr(0)
		if err != nil {
			return nil, err
		}
	}
	return &ast.VarDeclTop{Name: name.Literal, Type: typeName, Value: val}, nil
}

func (p *Parser) parseTypeName() string {
	var t string
	switch p.cur.Type {
	case lexer.IDENT:
		t = p.cur.Literal
		p.advance()
	default:
		t = ""
	}
	return t
}

// ----------------------------------------------------------------------------
// Statements
// ----------------------------------------------------------------------------

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
	}
	if _, err := p.expect(lexer.RBRACE); err != nil {
		return nil, err
	}
	return block, nil
}

func (p *Parser) parseStmt() (ast.Stmt, error) {
	switch p.cur.Type {
	case lexer.VAR:
		return p.parseVarDeclStmt()
	case lexer.IF:
		return p.parseIfStmt()
	case lexer.FOR:
		return p.parseForStmt()
	case lexer.RETURN:
		return p.parseReturnStmt()
	case lexer.LBRACE:
		return p.parseBlock()
	default:
		return p.parseSimpleStmt()
	}
}

func (p *Parser) parseVarDeclStmt() (*ast.VarDeclStmt, error) {
	p.advance() // consume 'var'
	name, err := p.expect(lexer.IDENT)
	if err != nil {
		return nil, err
	}
	typeName := ""
	if !p.curIs(lexer.ASSIGN) && !p.curIs(lexer.RBRACE) && !p.curIs(lexer.EOF) {
		typeName = p.parseTypeName()
	}
	var val ast.Expr
	if p.curIs(lexer.ASSIGN) {
		p.advance()
		val, err = p.parseExpr(0)
		if err != nil {
			return nil, err
		}
	}
	return &ast.VarDeclStmt{Name: name.Literal, Type: typeName, Value: val}, nil
}

func (p *Parser) parseSimpleStmt() (ast.Stmt, error) {
	// Could be: expr, ident := expr, ident op= expr, ident++/--
	expr, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}

	switch p.cur.Type {
	case lexer.DEFINE:
		// Short variable declaration: ident := expr
		p.advance()
		ident, ok := expr.(*ast.Ident)
		if !ok {
			return nil, fmt.Errorf("line %d: left-hand side of := must be identifier", p.cur.Line)
		}
		val, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		return &ast.VarDeclStmt{Name: ident.Name, Value: val}, nil

	case lexer.ASSIGN, lexer.PLUSEQ, lexer.MINUSEQ, lexer.STAREQ, lexer.SLASHEQ:
		op := p.cur.Literal
		p.advance()
		val, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		return &ast.AssignStmt{Target: expr, Value: val, Op: op}, nil

	case lexer.INC:
		p.advance()
		return &ast.IncDecStmt{Target: expr, Op: "++"}, nil

	case lexer.DEC:
		p.advance()
		return &ast.IncDecStmt{Target: expr, Op: "--"}, nil
	}

	return &ast.ExprStmt{Expr: expr}, nil
}

func (p *Parser) parseIfStmt() (*ast.IfStmt, error) {
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
			elseIf, err := p.parseIfStmt()
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

func (p *Parser) parseForStmt() (*ast.ForStmt, error) {
	p.advance() // consume 'for'

	// for { } — infinite loop
	if p.curIs(lexer.LBRACE) {
		body, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		return &ast.ForStmt{Body: body}, nil
	}

	// We need to distinguish:
	//   for cond { }                    (while-loop)
	//   for init; cond; post { }       (c-style)
	//
	// Try to parse a simple stmt first.
	first, err := p.parseStmt()
	if err != nil {
		return nil, err
	}

	if p.curIs(lexer.SEMICOLON) {
		// c-style for loop
		p.advance() // consume ;

		var cond ast.Expr
		if !p.curIs(lexer.SEMICOLON) {
			cond, err = p.parseExpr(0)
			if err != nil {
				return nil, err
			}
		}
		if _, err := p.expect(lexer.SEMICOLON); err != nil {
			return nil, err
		}
		var post ast.Stmt
		if !p.curIs(lexer.LBRACE) {
			post, err = p.parseSimpleStmt()
			if err != nil {
				return nil, err
			}
		}
		body, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		return &ast.ForStmt{Init: first, Cond: cond, Post: post, Body: body}, nil
	}

	// while-style: first is actually the condition (wrapped in ExprStmt)
	// Unwrap from ExprStmt to get Expr
	var condExpr ast.Expr
	if exprStmt, ok := first.(*ast.ExprStmt); ok {
		condExpr = exprStmt.Expr
	} else {
		return nil, fmt.Errorf("line %d: expected expression as for condition", p.cur.Line)
	}
	body, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	return &ast.ForStmt{Cond: condExpr, Body: body}, nil
}

func (p *Parser) parseReturnStmt() (*ast.ReturnStmt, error) {
	p.advance() // consume 'return'
	if p.curIs(lexer.RBRACE) || p.curIs(lexer.EOF) {
		return &ast.ReturnStmt{}, nil
	}
	val, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}
	return &ast.ReturnStmt{Value: val}, nil
}

// ----------------------------------------------------------------------------
// Pratt expression parser
// ----------------------------------------------------------------------------

// precedence levels
const (
	precLowest  = 0
	precOr      = 1
	precAnd     = 2
	precEq      = 3
	precCmp     = 4
	precAdd     = 5
	precMul     = 6
	precUnary   = 7
	precCall    = 8
	precSelect  = 9
)

func tokenPrec(tt lexer.TokenType) int {
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
	case lexer.DOT:
		return precSelect
	}
	return precLowest
}

func (p *Parser) parseExpr(minPrec int) (ast.Expr, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}

	for {
		prec := tokenPrec(p.cur.Type)
		if prec <= minPrec {
			break
		}

		switch p.cur.Type {
		case lexer.LPAREN:
			// Call expression
			p.advance()
			var args []ast.Expr
			for !p.curIs(lexer.RPAREN) && !p.curIs(lexer.EOF) {
				arg, err := p.parseExpr(0)
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
				if p.curIs(lexer.COMMA) {
					p.advance()
				}
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
			left = &ast.SelectorExpr{X: left, Sel: sel.Literal}

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
	switch p.cur.Type {
	case lexer.INT:
		val, err := strconv.ParseInt(p.cur.Literal, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid int literal %q", p.cur.Line, p.cur.Literal)
		}
		p.advance()
		return &ast.IntLit{Value: val}, nil

	case lexer.STRING:
		lit := p.cur.Literal
		p.advance()
		return &ast.StringLit{Value: lit}, nil

	case lexer.TRUE:
		p.advance()
		return &ast.BoolLit{Value: true}, nil

	case lexer.FALSE:
		p.advance()
		return &ast.BoolLit{Value: false}, nil

	case lexer.IDENT:
		name := p.cur.Literal
		p.advance()
		return &ast.Ident{Name: name}, nil

	case lexer.LPAREN:
		p.advance()
		expr, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(lexer.RPAREN); err != nil {
			return nil, err
		}
		return expr, nil

	case lexer.MINUS:
		p.advance()
		operand, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: "-", Operand: operand}, nil

	case lexer.BANG:
		p.advance()
		operand, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: "!", Operand: operand}, nil

	default:
		err := fmt.Errorf("line %d:%d: unexpected token in expression: %s (%q)",
			p.cur.Line, p.cur.Col, p.cur.Type, p.cur.Literal)
		p.errors = append(p.errors, err.Error())
		return nil, err
	}
}
