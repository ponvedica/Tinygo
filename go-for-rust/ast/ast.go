// Package ast defines the ownership-aware Abstract Syntax Tree for the Ore language.
//
// Key extensions over a plain AST:
//   - TypeExpr carries OwnershipMode (Owned | Shared | MutBorrow) and an optional lifetime
//   - LetDecl distinguishes immutable vs mutable bindings
//   - BorrowExpr / MoveExpr mark reference-taking and explicit moves
//   - DropStmt is inserted by the ownership checker at scope exits
//   - StructDecl and ImplBlock support user-defined types
//   - LifetimeParam represents generic lifetime parameters on fn/impl
package ast

import (
	"fmt"
	"strings"
)

// ────────────────────────────────────────────────────────────────────────────
// Ownership mode
// ────────────────────────────────────────────────────────────────────────────

// OwnershipMode classifies how a value is held.
type OwnershipMode int

const (
	Owned     OwnershipMode = iota // value is owned; moving transfers ownership
	Shared                         // &T  — immutable borrow
	MutBorrow                      // &mut T — exclusive mutable borrow
)

func (m OwnershipMode) String() string {
	switch m {
	case Owned:
		return "owned"
	case Shared:
		return "&"
	case MutBorrow:
		return "&mut"
	}
	return "?"
}

// ────────────────────────────────────────────────────────────────────────────
// TypeExpr — type with optional lifetime and ownership mode
// ────────────────────────────────────────────────────────────────────────────

// TypeExpr is the parsed representation of a type annotation.
// Examples:
//
//	Int               → {Name:"Int", Mode:Owned}
//	&String           → {Name:"String", Mode:Shared}
//	&mut String       → {Name:"String", Mode:MutBorrow}
//	&'a String        → {Name:"String", Mode:Shared, Lifetime:"'a"}
//	&'a mut String    → {Name:"String", Mode:MutBorrow, Lifetime:"'a"}
type TypeExpr struct {
	Name     string        // base type name: Int, String, Bool, or user struct
	Mode     OwnershipMode // how it is held
	Lifetime string        // "'a", "'static", or "" if none
}

func (t *TypeExpr) String() string {
	if t == nil {
		return "()"
	}
	switch t.Mode {
	case Shared:
		if t.Lifetime != "" {
			return fmt.Sprintf("&%s %s", t.Lifetime, t.Name)
		}
		return "&" + t.Name
	case MutBorrow:
		if t.Lifetime != "" {
			return fmt.Sprintf("&%s mut %s", t.Lifetime, t.Name)
		}
		return "&mut " + t.Name
	}
	return t.Name
}

// ────────────────────────────────────────────────────────────────────────────
// Node interfaces
// ────────────────────────────────────────────────────────────────────────────

type Node interface {
	nodeType() string
	String() string
}

type Expr interface {
	Node
	exprNode()
}

type Stmt interface {
	Node
	stmtNode()
}

type Decl interface {
	Node
	declNode()
}

// ────────────────────────────────────────────────────────────────────────────
// File
// ────────────────────────────────────────────────────────────────────────────

// File is the top-level parsed unit, one .ore source file.
type File struct {
	Module string // mod name
	Decls  []Decl
}

func (f *File) nodeType() string { return "File" }
func (f *File) String() string {
	return fmt.Sprintf("File{mod=%s, decls=%d}", f.Module, len(f.Decls))
}

// ────────────────────────────────────────────────────────────────────────────
// Declarations
// ────────────────────────────────────────────────────────────────────────────

// LifetimeParam is a generic lifetime parameter: <'a>
type LifetimeParam struct {
	Name string // "'a"
}

// Field is a named + typed slot in a struct or function parameter.
type Field struct {
	Name string
	Type *TypeExpr
}

func (f Field) String() string {
	return fmt.Sprintf("%s: %s", f.Name, f.Type)
}

// FnDecl is a function declaration.
//
//	fn foo<'a>(x: &'a String, y: Int) -> Int { ... }
type FnDecl struct {
	Name       string
	Lifetimes  []LifetimeParam // generic lifetime params
	Params     []Field
	ReturnType *TypeExpr // nil → ()
	Body       *BlockStmt
}

func (f *FnDecl) nodeType() string { return "FnDecl" }
func (f *FnDecl) declNode()        {}
func (f *FnDecl) String() string {
	lts := ""
	if len(f.Lifetimes) > 0 {
		lt := make([]string, len(f.Lifetimes))
		for i, l := range f.Lifetimes {
			lt[i] = l.Name
		}
		lts = "<" + strings.Join(lt, ", ") + ">"
	}
	return fmt.Sprintf("FnDecl{%s%s ret=%s}", f.Name, lts, f.ReturnType)
}

// StructDecl is a struct definition.
//
//	struct Point { x: Int, y: Int }
type StructDecl struct {
	Name   string
	Fields []Field
}

func (s *StructDecl) nodeType() string { return "StructDecl" }
func (s *StructDecl) declNode()        {}
func (s *StructDecl) String() string {
	return fmt.Sprintf("StructDecl{%s, %d fields}", s.Name, len(s.Fields))
}

// ImplBlock defines methods on a type.
//
//	impl Point { fn area(self: &Self) -> Int { ... } }
type ImplBlock struct {
	TypeName  string
	Lifetimes []LifetimeParam
	Methods   []*FnDecl
}

func (i *ImplBlock) nodeType() string { return "ImplBlock" }
func (i *ImplBlock) declNode()        {}
func (i *ImplBlock) String() string {
	return fmt.Sprintf("ImplBlock{%s, %d methods}", i.TypeName, len(i.Methods))
}

// ────────────────────────────────────────────────────────────────────────────
// Statements
// ────────────────────────────────────────────────────────────────────────────

// BlockStmt is a { ... } block containing a sequence of statements.
type BlockStmt struct {
	Stmts []Stmt
}

func (b *BlockStmt) nodeType() string { return "BlockStmt" }
func (b *BlockStmt) stmtNode()        {}
func (b *BlockStmt) String() string   { return fmt.Sprintf("Block(%d stmts)", len(b.Stmts)) }

// LetDecl is a local variable declaration.
//
//	let x: Int = 42;
//	let mut s: String = "hi";
type LetDecl struct {
	Name    string
	Mutable bool
	Type    *TypeExpr // may be nil (inferred)
	Value   Expr      // may be nil (uninitialized, rare)
}

func (l *LetDecl) nodeType() string { return "LetDecl" }
func (l *LetDecl) stmtNode()        {}
func (l *LetDecl) String() string {
	mut := ""
	if l.Mutable {
		mut = "mut "
	}
	return fmt.Sprintf("Let{%s%s: %v}", mut, l.Name, l.Type)
}

// AssignStmt is target op= value.
type AssignStmt struct {
	Target Expr
	Op     string // "=", "+=", "-=", etc.
	Value  Expr
}

func (a *AssignStmt) nodeType() string { return "AssignStmt" }
func (a *AssignStmt) stmtNode()        {}
func (a *AssignStmt) String() string {
	return fmt.Sprintf("Assign{%v %s %v}", a.Target, a.Op, a.Value)
}

// IncDecStmt is target++ or target--.
type IncDecStmt struct {
	Target Expr
	Op     string
}

func (i *IncDecStmt) nodeType() string { return "IncDecStmt" }
func (i *IncDecStmt) stmtNode()        {}
func (i *IncDecStmt) String() string {
	return fmt.Sprintf("IncDec{%v%s}", i.Target, i.Op)
}

// ReturnStmt is a return statement.
type ReturnStmt struct {
	Value Expr // nil = bare return
}

func (r *ReturnStmt) nodeType() string { return "ReturnStmt" }
func (r *ReturnStmt) stmtNode()        {}
func (r *ReturnStmt) String() string   { return fmt.Sprintf("Return{%v}", r.Value) }

// ExprStmt wraps a call (or any expression) used as a statement.
type ExprStmt struct {
	Expr Expr
}

func (e *ExprStmt) nodeType() string { return "ExprStmt" }
func (e *ExprStmt) stmtNode()        {}
func (e *ExprStmt) String() string   { return fmt.Sprintf("ExprStmt{%v}", e.Expr) }

// IfStmt is an if/else-if/else chain.
type IfStmt struct {
	Cond Expr
	Then *BlockStmt
	Else Stmt // nil | *BlockStmt | *IfStmt
}

func (i *IfStmt) nodeType() string { return "IfStmt" }
func (i *IfStmt) stmtNode()        {}
func (i *IfStmt) String() string   { return fmt.Sprintf("If{%v}", i.Cond) }

// WhileStmt is while cond { body }.
type WhileStmt struct {
	Cond Expr
	Body *BlockStmt
}

func (w *WhileStmt) nodeType() string { return "WhileStmt" }
func (w *WhileStmt) stmtNode()        {}
func (w *WhileStmt) String() string   { return fmt.Sprintf("While{%v}", w.Cond) }

// ForStmt is for init; cond; post { body }.
type ForStmt struct {
	Init Stmt
	Cond Expr
	Post Stmt
	Body *BlockStmt
}

func (f *ForStmt) nodeType() string { return "ForStmt" }
func (f *ForStmt) stmtNode()        {}
func (f *ForStmt) String() string   { return "For{...}" }

// DropStmt is compiler-inserted at end of scope for owned variables.
// It is NOT written in source; the ownership checker inserts it during analysis.
type DropStmt struct {
	VarName string
}

func (d *DropStmt) nodeType() string { return "DropStmt" }
func (d *DropStmt) stmtNode()        {}
func (d *DropStmt) String() string   { return fmt.Sprintf("Drop{%s}", d.VarName) }

// ────────────────────────────────────────────────────────────────────────────
// Expressions
// ────────────────────────────────────────────────────────────────────────────

// Ident is a variable or function name.
type Ident struct {
	Name string
}

func (i *Ident) nodeType() string { return "Ident" }
func (i *Ident) exprNode()        {}
func (i *Ident) String() string   { return i.Name }

// IntLit is an integer literal.
type IntLit struct{ Value int64 }

func (i *IntLit) nodeType() string { return "IntLit" }
func (i *IntLit) exprNode()        {}
func (i *IntLit) String() string   { return fmt.Sprintf("%d", i.Value) }

// StringLit is a string literal.
type StringLit struct{ Value string }

func (s *StringLit) nodeType() string { return "StringLit" }
func (s *StringLit) exprNode()        {}
func (s *StringLit) String() string   { return fmt.Sprintf("%q", s.Value) }

// BoolLit is true or false.
type BoolLit struct{ Value bool }

func (b *BoolLit) nodeType() string { return "BoolLit" }
func (b *BoolLit) exprNode()        {}
func (b *BoolLit) String() string {
	if b.Value {
		return "true"
	}
	return "false"
}

// BorrowExpr is &expr or &mut expr — taking a reference.
// OwnershipMode must be Shared or MutBorrow.
type BorrowExpr struct {
	Mode     OwnershipMode
	Lifetime string // "'a" or ""
	Operand  Expr
}

func (b *BorrowExpr) nodeType() string { return "BorrowExpr" }
func (b *BorrowExpr) exprNode()        {}
func (b *BorrowExpr) String() string {
	prefix := "&"
	if b.Mode == MutBorrow {
		prefix = "&mut "
	}
	return fmt.Sprintf("(%s%v)", prefix, b.Operand)
}

// MoveExpr is an explicit `move expr` — forces ownership transfer.
type MoveExpr struct {
	Operand Expr
}

func (m *MoveExpr) nodeType() string { return "MoveExpr" }
func (m *MoveExpr) exprNode()        {}
func (m *MoveExpr) String() string   { return fmt.Sprintf("(move %v)", m.Operand) }

// BinaryExpr is left op right.
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

// UnaryExpr is op operand.
type UnaryExpr struct {
	Op      string
	Operand Expr
}

func (u *UnaryExpr) nodeType() string { return "UnaryExpr" }
func (u *UnaryExpr) exprNode()        {}
func (u *UnaryExpr) String() string {
	return fmt.Sprintf("(%s%v)", u.Op, u.Operand)
}

// CallExpr is fn(args...).
type CallExpr struct {
	Func Expr
	Args []Expr
}

func (c *CallExpr) nodeType() string { return "CallExpr" }
func (c *CallExpr) exprNode()        {}
func (c *CallExpr) String() string {
	return fmt.Sprintf("Call{%v(%d args)}", c.Func, len(c.Args))
}

// SelectorExpr is obj.field or pkg::name.
type SelectorExpr struct {
	X   Expr
	Sel string
	Sep string // "." or "::"
}

func (s *SelectorExpr) nodeType() string { return "SelectorExpr" }
func (s *SelectorExpr) exprNode()        {}
func (s *SelectorExpr) String() string {
	return fmt.Sprintf("%v%s%s", s.X, s.Sep, s.Sel)
}

// StructLit is a struct literal: Point { x: 1, y: 2 }
type StructLit struct {
	TypeName string
	Fields   []StructFieldVal
}

type StructFieldVal struct {
	Name  string
	Value Expr
}

func (s *StructLit) nodeType() string { return "StructLit" }
func (s *StructLit) exprNode()        {}
func (s *StructLit) String() string {
	return fmt.Sprintf("StructLit{%s{...}}", s.TypeName)
}
