// Package ast defines the Abstract Syntax Tree (AST) nodes for the tinygo compiler.
package ast

import "fmt"

// ----------------------------------------------------------------------------
// Interfaces
// ----------------------------------------------------------------------------

// Node is the base interface for all AST nodes.
type Node interface {
	nodeType() string
	String() string
}

// Expr is a node that produces a value.
type Expr interface {
	Node
	exprNode()
}

// Stmt is a node that represents a statement.
type Stmt interface {
	Node
	stmtNode()
}

// Decl is a top-level declaration.
type Decl interface {
	Node
	declNode()
}

// ----------------------------------------------------------------------------
// File (top-level unit)
// ----------------------------------------------------------------------------

// File represents a parsed Go source file.
type File struct {
	Package string
	Imports []string
	Decls   []Decl
}

func (f *File) nodeType() string { return "File" }
func (f *File) String() string {
	return fmt.Sprintf("File{package=%s, imports=%v, decls=%d}", f.Package, f.Imports, len(f.Decls))
}

// ----------------------------------------------------------------------------
// Declarations
// ----------------------------------------------------------------------------

// FuncDecl represents a function declaration: func name(params) returnType { body }
type FuncDecl struct {
	Name       string
	Params     []Field
	ReturnType string // "" means no return value
	Body       *BlockStmt
}

// Field is a parameter name + type pair.
type Field struct {
	Name string
	Type string
}

func (f *FuncDecl) nodeType() string { return "FuncDecl" }
func (f *FuncDecl) declNode()        {}
func (f *FuncDecl) String() string {
	return fmt.Sprintf("FuncDecl{name=%s, params=%v, ret=%q}", f.Name, f.Params, f.ReturnType)
}

// VarDeclStmt represents a top-level var declaration used as Decl.
type VarDeclTop struct {
	Name  string
	Type  string // may be empty if inferred
	Value Expr
}

func (v *VarDeclTop) nodeType() string { return "VarDeclTop" }
func (v *VarDeclTop) declNode()        {}
func (v *VarDeclTop) String() string {
	return fmt.Sprintf("VarDeclTop{%s %s}", v.Name, v.Type)
}

// ----------------------------------------------------------------------------
// Statements
// ----------------------------------------------------------------------------

// BlockStmt is a list of statements surrounded by braces.
type BlockStmt struct {
	Stmts []Stmt
}

func (b *BlockStmt) nodeType() string { return "BlockStmt" }
func (b *BlockStmt) stmtNode()        {}
func (b *BlockStmt) String() string   { return fmt.Sprintf("Block(%d stmts)", len(b.Stmts)) }

// VarDeclStmt represents a short var declaration: name := expr  or  var name type = expr
type VarDeclStmt struct {
	Name  string
	Type  string // may be empty (inferred)
	Value Expr
}

func (v *VarDeclStmt) nodeType() string { return "VarDeclStmt" }
func (v *VarDeclStmt) stmtNode()        {}
func (v *VarDeclStmt) String() string {
	return fmt.Sprintf("VarDecl{%s %s}", v.Name, v.Type)
}

// AssignStmt is a simple assignment: name = expr
type AssignStmt struct {
	Target Expr // can be Ident or IndexExpr
	Value  Expr
	Op     string // "=", "+=", "-=", etc.
}

func (a *AssignStmt) nodeType() string { return "AssignStmt" }
func (a *AssignStmt) stmtNode()        {}
func (a *AssignStmt) String() string {
	return fmt.Sprintf("Assign{%v %s %v}", a.Target, a.Op, a.Value)
}

// IncDecStmt is an increment/decrement: name++ or name--
type IncDecStmt struct {
	Target Expr
	Op     string // "++" or "--"
}

func (i *IncDecStmt) nodeType() string { return "IncDecStmt" }
func (i *IncDecStmt) stmtNode()        {}
func (i *IncDecStmt) String() string {
	return fmt.Sprintf("IncDec{%v %s}", i.Target, i.Op)
}

// IfStmt represents an if/else statement.
type IfStmt struct {
	Cond Expr
	Then *BlockStmt
	Else Stmt // nil, *BlockStmt, or *IfStmt (else if)
}

func (i *IfStmt) nodeType() string { return "IfStmt" }
func (i *IfStmt) stmtNode()        {}
func (i *IfStmt) String() string   { return fmt.Sprintf("If{cond=%v}", i.Cond) }

// ForStmt represents a for loop: for init; cond; post { body }
// Or an infinite loop: for { body }
// Or a while loop: for cond { body }
type ForStmt struct {
	Init Stmt // may be nil
	Cond Expr // may be nil (infinite loop)
	Post Stmt // may be nil
	Body *BlockStmt
}

func (f *ForStmt) nodeType() string { return "ForStmt" }
func (f *ForStmt) stmtNode()        {}
func (f *ForStmt) String() string   { return "For{...}" }

// ReturnStmt is a return statement.
type ReturnStmt struct {
	Value Expr // may be nil for bare return
}

func (r *ReturnStmt) nodeType() string { return "ReturnStmt" }
func (r *ReturnStmt) stmtNode()        {}
func (r *ReturnStmt) String() string {
	return fmt.Sprintf("Return{%v}", r.Value)
}

// ExprStmt wraps an expression used as a statement (e.g., a function call).
type ExprStmt struct {
	Expr Expr
}

func (e *ExprStmt) nodeType() string { return "ExprStmt" }
func (e *ExprStmt) stmtNode()        {}
func (e *ExprStmt) String() string   { return fmt.Sprintf("ExprStmt{%v}", e.Expr) }

// ----------------------------------------------------------------------------
// Expressions
// ----------------------------------------------------------------------------

// Ident is a simple identifier: x, main, fmt
type Ident struct {
	Name string
}

func (i *Ident) nodeType() string { return "Ident" }
func (i *Ident) exprNode()        {}
func (i *Ident) String() string   { return i.Name }

// IntLit is an integer literal: 42
type IntLit struct {
	Value int64
}

func (i *IntLit) nodeType() string { return "IntLit" }
func (i *IntLit) exprNode()        {}
func (i *IntLit) String() string   { return fmt.Sprintf("%d", i.Value) }

// StringLit is a string literal: "hello"
type StringLit struct {
	Value string
}

func (s *StringLit) nodeType() string { return "StringLit" }
func (s *StringLit) exprNode()        {}
func (s *StringLit) String() string   { return fmt.Sprintf("%q", s.Value) }

// BoolLit is a boolean literal: true, false
type BoolLit struct {
	Value bool
}

func (b *BoolLit) nodeType() string { return "BoolLit" }
func (b *BoolLit) exprNode()        {}
func (b *BoolLit) String() string {
	if b.Value {
		return "true"
	}
	return "false"
}

// BinaryExpr is: left op right
type BinaryExpr struct {
	Left  Expr
	Op    string
	Right Expr
}

func (b *BinaryExpr) nodeType() string { return "BinaryExpr" }
func (b *BinaryExpr) exprNode()        {}
func (b *BinaryExpr) String() string {
	return fmt.Sprintf("(%v %s %v)", b.Left, b.Op, b.Right)
}

// UnaryExpr is: op operand  (e.g., !x, -x)
type UnaryExpr struct {
	Op      string
	Operand Expr
}

func (u *UnaryExpr) nodeType() string { return "UnaryExpr" }
func (u *UnaryExpr) exprNode()        {}
func (u *UnaryExpr) String() string {
	return fmt.Sprintf("(%s%v)", u.Op, u.Operand)
}

// CallExpr is a function call: fn(args...)
type CallExpr struct {
	Func Expr // could be Ident or SelectorExpr
	Args []Expr
}

func (c *CallExpr) nodeType() string { return "CallExpr" }
func (c *CallExpr) exprNode()        {}
func (c *CallExpr) String() string {
	return fmt.Sprintf("Call{%v(%d args)}", c.Func, len(c.Args))
}

// SelectorExpr is a dotted expression: x.y  (e.g., fmt.Println)
type SelectorExpr struct {
	X   Expr
	Sel string
}

func (s *SelectorExpr) nodeType() string { return "SelectorExpr" }
func (s *SelectorExpr) exprNode()        {}
func (s *SelectorExpr) String() string {
	return fmt.Sprintf("%v.%s", s.X, s.Sel)
}
