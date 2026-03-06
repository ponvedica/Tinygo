// Package borrow implements the borrow checker for the Ore compiler.
//
// Rust borrow rules enforced:
//  1. You can have any number of shared (&T) borrows simultaneously.
//  2. You can have exactly ONE mutable (&mut T) borrow at a time.
//  3. Shared and mutable borrows cannot coexist for the same variable.
//  4. A variable cannot be moved while it is borrowed.
//  5. You cannot mutate through a shared borrow.
//  6. Borrows are scoped to the lexical block they appear in.
package borrow

import (
	"fmt"

	"goforust/ast"
)

// ─────────────────────────────────────────────────────────────────────────────
// Borrow records
// ─────────────────────────────────────────────────────────────────────────────

// BorrowKind classifies how a variable is being borrowed.
type BorrowKind int

const (
	SharedBorrow BorrowKind = iota // &T
	MutBorrow                      // &mut T
)

func (k BorrowKind) String() string {
	if k == SharedBorrow {
		return "&"
	}
	return "&mut"
}

// ActiveBorrow records one live borrow of a variable.
type ActiveBorrow struct {
	Kind    BorrowKind
	ScopeID int    // which scope depth owns this borrow
	Alias   string // the name that holds the borrow (for error messages)
}

// ─────────────────────────────────────────────────────────────────────────────
// Borrow table
// ─────────────────────────────────────────────────────────────────────────────

// BorrowTable tracks active borrows for all variables.
type BorrowTable struct {
	// borrows maps original variable name → list of active borrows.
	borrows map[string][]*ActiveBorrow
}

func newBorrowTable() *BorrowTable {
	return &BorrowTable{borrows: make(map[string][]*ActiveBorrow)}
}

// addShared registers a new shared borrow of `varName`.
// Returns an error if a mutable borrow is already active.
func (bt *BorrowTable) addShared(varName, alias string, scopeID int) error {
	for _, b := range bt.borrows[varName] {
		if b.Kind == MutBorrow {
			return fmt.Errorf("cannot borrow %q as immutable because it is also borrowed as mutable (by %q)", varName, b.Alias)
		}
	}
	bt.borrows[varName] = append(bt.borrows[varName], &ActiveBorrow{Kind: SharedBorrow, ScopeID: scopeID, Alias: alias})
	return nil
}

// addMut registers a new mutable borrow of `varName`.
// Returns an error if any borrow (shared or mutable) is already active.
func (bt *BorrowTable) addMut(varName, alias string, scopeID int) error {
	existing := bt.borrows[varName]
	if len(existing) > 0 {
		kind := existing[0].Kind
		return fmt.Errorf("cannot borrow %q as mutable because it is also borrowed as %s (by %q)", varName, kind, existing[0].Alias)
	}
	bt.borrows[varName] = append(bt.borrows[varName], &ActiveBorrow{Kind: MutBorrow, ScopeID: scopeID, Alias: alias})
	return nil
}

// releaseScope removes all borrows with the given scopeID (end of scope).
func (bt *BorrowTable) releaseScope(scopeID int) {
	for varName, bs := range bt.borrows {
		var kept []*ActiveBorrow
		for _, b := range bs {
			if b.ScopeID != scopeID {
				kept = append(kept, b)
			}
		}
		bt.borrows[varName] = kept
	}
}

// isBorrowed returns true if `varName` has any active borrow.
func (bt *BorrowTable) isBorrowed(varName string) bool {
	return len(bt.borrows[varName]) > 0
}

// isMutBorrowed returns true if `varName` has an active mutable borrow.
func (bt *BorrowTable) isMutBorrowed(varName string) bool {
	for _, b := range bt.borrows[varName] {
		if b.Kind == MutBorrow {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// Checker
// ─────────────────────────────────────────────────────────────────────────────

// Checker enforces borrow rules across the AST.
type Checker struct {
	bt     *BorrowTable
	scope  int // current scope depth (increments on push)
	errors []string
	// varMut tracks which variables were declared with `let mut`
	varMut map[string]bool
}

// New creates a borrow Checker.
func New() *Checker {
	return &Checker{
		bt:     newBorrowTable(),
		varMut: make(map[string]bool),
	}
}

// Errors returns any borrow errors found.
func (c *Checker) Errors() []string { return c.errors }

// Check runs the borrow checker on a file.
func (c *Checker) Check(file *ast.File) {
	for _, d := range file.Decls {
		c.checkDecl(d)
	}
}

func (c *Checker) errorf(format string, args ...interface{}) {
	c.errors = append(c.errors, fmt.Sprintf("borrow error: "+format, args...))
}

func (c *Checker) pushScope() int {
	c.scope++
	return c.scope
}

func (c *Checker) popScope(id int) {
	c.bt.releaseScope(id)
}

// ─────────────────────────────────────────────────────────────────────────────
// Declarations
// ─────────────────────────────────────────────────────────────────────────────

func (c *Checker) checkDecl(d ast.Decl) {
	switch decl := d.(type) {
	case *ast.FnDecl:
		c.checkFn(decl)
	case *ast.ImplBlock:
		for _, m := range decl.Methods {
			c.checkFn(m)
		}
	}
}

func (c *Checker) checkFn(fn *ast.FnDecl) {
	for _, p := range fn.Params {
		if p.Type != nil && p.Type.Mode == ast.MutBorrow {
			c.varMut[p.Name] = true
		}
	}
	c.checkBlock(fn.Body)
}

// ─────────────────────────────────────────────────────────────────────────────
// Statements
// ─────────────────────────────────────────────────────────────────────────────

func (c *Checker) checkBlock(block *ast.BlockStmt) {
	if block == nil {
		return
	}
	sid := c.pushScope()
	for _, s := range block.Stmts {
		c.checkStmt(s)
	}
	c.popScope(sid)
}

func (c *Checker) checkStmt(s ast.Stmt) {
	switch stmt := s.(type) {
	case *ast.LetDecl:
		if stmt.Mutable {
			c.varMut[stmt.Name] = true
		}
		if stmt.Value != nil {
			c.checkExpr(stmt.Value, stmt.Name)
		}

	case *ast.AssignStmt:
		// Assignment via mutable reference is only allowed if the target variable
		// was declared mutable.
		if ident, ok := stmt.Target.(*ast.Ident); ok {
			if !c.varMut[ident.Name] {
				c.errorf("cannot assign to immutable binding %q — declare with `let mut`", ident.Name)
			}
			// Cannot assign to a variable that is currently borrowed.
			if c.bt.isBorrowed(ident.Name) {
				c.errorf("cannot assign to %q because it is currently borrowed", ident.Name)
			}
		}
		c.checkExpr(stmt.Value, "")

	case *ast.ExprStmt:
		c.checkExpr(stmt.Expr, "")

	case *ast.ReturnStmt:
		if stmt.Value != nil {
			c.checkExpr(stmt.Value, "")
		}

	case *ast.IfStmt:
		c.checkExpr(stmt.Cond, "")
		c.checkBlock(stmt.Then)
		if stmt.Else != nil {
			c.checkStmt(stmt.Else)
		}

	case *ast.WhileStmt:
		c.checkExpr(stmt.Cond, "")
		c.checkBlock(stmt.Body)

	case *ast.ForStmt:
		if stmt.Init != nil {
			c.checkStmt(stmt.Init)
		}
		if stmt.Cond != nil {
			c.checkExpr(stmt.Cond, "")
		}
		if stmt.Post != nil {
			c.checkStmt(stmt.Post)
		}
		c.checkBlock(stmt.Body)

	case *ast.BlockStmt:
		c.checkBlock(stmt)

	case *ast.DropStmt:
		// If there's an active borrow when variable is dropped, that's an error.
		if c.bt.isBorrowed(stmt.VarName) {
			c.errorf("cannot drop %q while it is still borrowed", stmt.VarName)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Expressions
// ─────────────────────────────────────────────────────────────────────────────

// checkExpr walks an expression checking borrow rules.
// alias is the name of the binding receiving this expression (for error messages).
func (c *Checker) checkExpr(e ast.Expr, alias string) {
	if e == nil {
		return
	}
	switch expr := e.(type) {
	case *ast.BorrowExpr:
		// Determine what is being borrowed.
		varName := extractIdentName(expr.Operand)
		if varName == "" {
			c.checkExpr(expr.Operand, "")
			return
		}
		if alias == "" {
			alias = "_borrow_"
		}
		var err error
		if expr.Mode == ast.MutBorrow {
			// Mutable borrow: variable must itself be mutable.
			if !c.varMut[varName] {
				c.errorf("cannot borrow %q as mutable, as it is not declared as mutable", varName)
			}
			err = c.bt.addMut(varName, alias, c.scope)
		} else {
			err = c.bt.addShared(varName, alias, c.scope)
		}
		if err != nil {
			c.errorf("%s", err.Error())
		}

	case *ast.CallExpr:
		c.checkExpr(expr.Func, "")
		for _, arg := range expr.Args {
			c.checkExpr(arg, "")
		}

	case *ast.BinaryExpr:
		c.checkExpr(expr.Left, "")
		c.checkExpr(expr.Right, "")

	case *ast.UnaryExpr:
		c.checkExpr(expr.Operand, "")

	case *ast.MoveExpr:
		c.checkExpr(expr.Operand, "")
		// Moving a borrowed variable is not allowed.
		if varName := extractIdentName(expr.Operand); varName != "" {
			if c.bt.isBorrowed(varName) {
				c.errorf("cannot move %q because it is currently borrowed", varName)
			}
		}

	case *ast.SelectorExpr:
		c.checkExpr(expr.X, "")

	case *ast.StructLit:
		for _, f := range expr.Fields {
			c.checkExpr(f.Value, "")
		}

	case *ast.Ident, *ast.IntLit, *ast.StringLit, *ast.BoolLit:
		// Nothing to check here; ownership checker handles Ident.
	}
}

// extractIdentName returns the name of an identifier expression, or "".
func extractIdentName(e ast.Expr) string {
	if ident, ok := e.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}
