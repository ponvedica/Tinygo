// Package ownership implements the formal Ore ownership state machine.
//
// This is a deterministic state machine with explicit transitions. Every
// operation on every variable goes through a named transition method.
// No implicit logic exists — rules are encoded as a table.
//
// ─── State Transition Table ─────────────────────────────────────────────────
//
//	Operation              │ Pre-state            │ Post-state
//	───────────────────────┼──────────────────────┼───────────────────────────
//	let x = <expr>         │ Uninitialized        │ Owned
//	let y = x   (move)     │ x: Owned             │ x: Moved, y: Owned
//	&x                     │ Owned                │ BorrowedImmutable
//	&mut x                 │ Owned                │ BorrowedMutable
//	scope exit (borrow)    │ BorrowedImm/Mut      │ Owned   (borrow released)
//	scope exit (owned)     │ Owned                │ → emit Drop
//	use of x               │ Moved                │ → ERROR
//	assign to immutable    │ any                  │ → ERROR
//
// ────────────────────────────────────────────────────────────────────────────
package ownership

import (
	"fmt"

	"goforust/ast"
)

// ─────────────────────────────────────────────────────────────────────────────
// State machine types
// ─────────────────────────────────────────────────────────────────────────────

// OwnershipState is the lifecycle state of a variable's value.
type OwnershipState int

const (
	Uninitialized     OwnershipState = iota // declared but no value yet
	Owned                                   // value is live and owned
	Moved                                   // value was transferred; variable is dead
	BorrowedImmutable                       // &x: shared, read-only reference live
	BorrowedMutable                         // &mut x: exclusive mutable reference live
	Dropped                                 // value was explicitly dropped
)

func (s OwnershipState) String() string {
	switch s {
	case Uninitialized:
		return "Uninitialized"
	case Owned:
		return "Owned"
	case Moved:
		return "Moved"
	case BorrowedImmutable:
		return "BorrowedImmutable"
	case BorrowedMutable:
		return "BorrowedMutable"
	case Dropped:
		return "Dropped"
	}
	return "Unknown"
}

// Symbol represents one variable in the symbol table.
type Symbol struct {
	Name    string
	State   OwnershipState
	Mutable bool
	Scope   int // depth at which it was declared
	Line    int // for error messages
	Col     int
}

// ─────────────────────────────────────────────────────────────────────────────
// Scope stack
// ─────────────────────────────────────────────────────────────────────────────

// scopeFrame is one level of the lexical scope stack.
type scopeFrame map[string]*Symbol

// Checker holds the scope stack and diagnostics.
type Checker struct {
	scopeStack  []scopeFrame
	diagnostics []Diagnostic
}

// Diagnostic is an ownership error with source location.
type Diagnostic struct {
	Line    int
	Col     int
	Message string
	Hint    string
}

// New creates a Checker with the global scope pre-pushed.
func New() *Checker {
	c := &Checker{}
	c.pushScope()
	return c
}

// Errors returns all diagnostics as plain strings (for backward compat).
func (c *Checker) Errors() []string {
	out := make([]string, len(c.diagnostics))
	for i, d := range c.diagnostics {
		out[i] = d.fullMessage()
	}
	return out
}

// Diagnostics returns structured diagnostics.
func (c *Checker) Diagnostics() []Diagnostic { return c.diagnostics }

func (d Diagnostic) fullMessage() string {
	if d.Line > 0 {
		return fmt.Sprintf("ownership error (L%d:%d): %s", d.Line, d.Col, d.Message)
	}
	return "ownership error: " + d.Message
}

// ─────────────────────────────────────────────────────────────────────────────
// Scope operations
// ─────────────────────────────────────────────────────────────────────────────

func (c *Checker) pushScope() { c.scopeStack = append(c.scopeStack, make(scopeFrame)) }

func (c *Checker) popScope() []string {
	frame := c.scopeStack[len(c.scopeStack)-1]
	c.scopeStack = c.scopeStack[:len(c.scopeStack)-1]
	// Collect owned-but-not-moved variables to drop.
	var drops []string
	for name, sym := range frame {
		if sym.State == Owned {
			drops = append(drops, name)
		}
	}
	return drops
}

func (c *Checker) currentDepth() int { return len(c.scopeStack) }

// define adds a symbol to the current (innermost) scope.
func (c *Checker) define(sym *Symbol) {
	sym.Scope = c.currentDepth()
	c.scopeStack[len(c.scopeStack)-1][sym.Name] = sym
}

// lookup searches the scope stack from innermost to outermost.
func (c *Checker) lookup(name string) *Symbol {
	for i := len(c.scopeStack) - 1; i >= 0; i-- {
		if sym, ok := c.scopeStack[i][name]; ok {
			return sym
		}
	}
	return nil
}

func (c *Checker) errorf(line, col int, msg string, args ...interface{}) {
	c.diagnostics = append(c.diagnostics, Diagnostic{
		Line:    line,
		Col:     col,
		Message: fmt.Sprintf(msg, args...),
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// State transition methods (the heart of the state machine)
// ─────────────────────────────────────────────────────────────────────────────

// transitionLet: Uninitialized → Owned.
// Called when a new variable is bound.
func (c *Checker) transitionLet(name string, mutable bool, line, col int) {
	c.define(&Symbol{
		Name:    name,
		State:   Owned,
		Mutable: mutable,
		Line:    line,
		Col:     col,
	})
}

// transitionUse: reads a variable's current state.
// consumesOwnership = true means this is a by-value use (move).
// Returns whether the use is legal.
func (c *Checker) transitionUse(name string, consumesOwnership bool, line, col int) bool {
	sym := c.lookup(name)
	if sym == nil {
		return true // unknown name (function, etc.) — not our concern
	}
	switch sym.State {
	case Moved:
		c.errorf(line, col, "use of moved value `%s`\n  note: `%s` was moved earlier (declared at L%d)",
			name, name, sym.Line)
		return false
	case Dropped:
		c.errorf(line, col, "use of dropped value `%s`", name)
		return false
	case Uninitialized:
		c.errorf(line, col, "use of possibly uninitialized variable `%s`", name)
		return false
	}
	// Legal use: if consuming ownership, transition to Moved.
	if consumesOwnership && sym.State == Owned {
		sym.State = Moved
	}
	return true
}

// transitionAssign: verifies the target is mutable and not moved.
func (c *Checker) transitionAssign(name string, line, col int) bool {
	sym := c.lookup(name)
	if sym == nil {
		return true
	}
	if !sym.Mutable {
		c.errorf(line, col, "cannot assign to immutable variable `%s`\n  help: consider declaring with `let mut %s`",
			name, name)
		return false
	}
	if sym.State == Moved {
		c.errorf(line, col, "cannot assign to moved variable `%s`", name)
		return false
	}
	return true
}

// transitionBorrow: Owned → BorrowedImmutable.
func (c *Checker) transitionBorrow(name string, line, col int) {
	sym := c.lookup(name)
	if sym == nil {
		return
	}
	if sym.State == Moved {
		c.errorf(line, col, "cannot borrow moved value `%s`", name)
		return
	}
	sym.State = BorrowedImmutable
}

// transitionMutBorrow: Owned → BorrowedMutable.
func (c *Checker) transitionMutBorrow(name string, line, col int) {
	sym := c.lookup(name)
	if sym == nil {
		return
	}
	if sym.State == Moved {
		c.errorf(line, col, "cannot borrow moved value `%s` as mutable", name)
		return
	}
	if !sym.Mutable {
		c.errorf(line, col, "cannot borrow `%s` as mutable, as it is not declared as mutable\n  help: declare with `let mut %s`",
			name, name)
		return
	}
	sym.State = BorrowedMutable
}

// transitionDrop: explicitly marks a variable as Dropped.
func (c *Checker) transitionDrop(name string) {
	sym := c.lookup(name)
	if sym != nil {
		sym.State = Dropped
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Top-level analysis entry
// ─────────────────────────────────────────────────────────────────────────────

// Check analyses the whole file.
func (c *Checker) Check(file *ast.File) {
	// Pre-declare top-level function names so recursive calls are legal.
	for _, d := range file.Decls {
		if fn, ok := d.(*ast.FnDecl); ok {
			c.define(&Symbol{Name: fn.Name, State: Owned, Mutable: false})
		}
	}
	for _, d := range file.Decls {
		c.checkDecl(d)
	}
}

func (c *Checker) checkDecl(d ast.Decl) {
	switch decl := d.(type) {
	case *ast.FnDecl:
		c.checkFn(decl)
	case *ast.StructDecl:
		// No ownership checking needed at struct definition.
	case *ast.ImplBlock:
		for _, m := range decl.Methods {
			c.checkFn(m)
		}
	}
}

func (c *Checker) checkFn(fn *ast.FnDecl) {
	c.pushScope()
	for _, p := range fn.Params {
		mutable := p.Type != nil && p.Type.Mode == ast.MutBorrow
		c.transitionLet(p.Name, mutable, 0, 0)
	}
	c.checkBlock(fn.Body)
	c.popScope() // drops handled inside checkBlock
}

func (c *Checker) checkBlock(block *ast.BlockStmt) {
	if block == nil {
		return
	}
	c.pushScope()
	for _, s := range block.Stmts {
		c.checkStmt(s)
	}
	// Auto-insert DropStmt for owned, non-moved vars at scope exit.
	drops := c.popScope()
	for _, name := range drops {
		block.Stmts = append(block.Stmts, &ast.DropStmt{VarName: name})
		c.transitionDrop(name)
	}
}

func (c *Checker) checkStmt(s ast.Stmt) {
	switch stmt := s.(type) {
	case *ast.LetDecl:
		if stmt.Value != nil {
			c.checkExpr(stmt.Value, true)
		}
		c.transitionLet(stmt.Name, stmt.Mutable, 0, 0)

	case *ast.AssignStmt:
		if ident, ok := stmt.Target.(*ast.Ident); ok {
			c.transitionAssign(ident.Name, 0, 0)
		}
		c.checkExpr(stmt.Value, true)

	case *ast.IncDecStmt:
		c.checkExpr(stmt.Target, false)

	case *ast.ExprStmt:
		c.checkExpr(stmt.Expr, true)

	case *ast.ReturnStmt:
		if stmt.Value != nil {
			c.checkExpr(stmt.Value, true)
		}

	case *ast.IfStmt:
		c.checkExpr(stmt.Cond, false)
		c.checkBlock(stmt.Then)
		if stmt.Else != nil {
			c.checkStmt(stmt.Else)
		}

	case *ast.WhileStmt:
		c.checkExpr(stmt.Cond, false)
		c.checkBlock(stmt.Body)

	case *ast.ForStmt:
		if stmt.Init != nil {
			c.checkStmt(stmt.Init)
		}
		if stmt.Cond != nil {
			c.checkExpr(stmt.Cond, false)
		}
		if stmt.Post != nil {
			c.checkStmt(stmt.Post)
		}
		c.checkBlock(stmt.Body)

	case *ast.BlockStmt:
		c.checkBlock(stmt)

	case *ast.DropStmt:
		c.transitionDrop(stmt.VarName)
	}
}

func (c *Checker) checkExpr(e ast.Expr, consumesOwnership bool) {
	if e == nil {
		return
	}
	switch expr := e.(type) {
	case *ast.Ident:
		// Passing by value = move. Passing by borrow (&) = does NOT consume.
		c.transitionUse(expr.Name, consumesOwnership, 0, 0)

	case *ast.BorrowExpr:
		// &x does not move x — it borrows.
		if ident, ok := expr.Operand.(*ast.Ident); ok {
			if expr.Mode == ast.MutBorrow {
				c.transitionMutBorrow(ident.Name, 0, 0)
			} else {
				c.transitionBorrow(ident.Name, 0, 0)
			}
		} else {
			c.checkExpr(expr.Operand, false)
		}

	case *ast.MoveExpr:
		c.checkExpr(expr.Operand, true)

	case *ast.BinaryExpr:
		c.checkExpr(expr.Left, false)
		c.checkExpr(expr.Right, false)

	case *ast.UnaryExpr:
		c.checkExpr(expr.Operand, false)

	case *ast.CallExpr:
		c.checkExpr(expr.Func, false)
		for _, arg := range expr.Args {
			if _, isBorrow := arg.(*ast.BorrowExpr); isBorrow {
				c.checkExpr(arg, false) // borrows don't move
			} else {
				c.checkExpr(arg, true) // by-value args move
			}
		}

	case *ast.SelectorExpr:
		c.checkExpr(expr.X, false)

	case *ast.StructLit:
		for _, f := range expr.Fields {
			c.checkExpr(f.Value, true)
		}

	case *ast.IntLit, *ast.StringLit, *ast.BoolLit:
		// Literals are always fresh — no ownership concern.
	}
}
