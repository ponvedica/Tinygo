// Package lifetime implements static lifetime analysis for the Ore compiler.
//
// Lifetime analysis answers the question: does every reference live long enough?
//
// Algorithm (inspired by Rust NLL — Non-Lexical Lifetimes, simplified):
//  1. Assign each reference a unique lifetime ID at its definition point.
//  2. Build a constraint graph: if reference 'a flows into a context expecting 'b,
//     add edge 'a: 'b (read: a must outlive b).
//  3. Resolve all lifetime parameters in function signatures.
//  4. Detect dangling references: a reference whose source is already out of scope.
//
// This is a simplified but meaningful lifetime analyzer —
// it catches the most common dangling-reference patterns statically.
package lifetime

import (
	"fmt"

	"goforust/ast"
)

// ─────────────────────────────────────────────────────────────────────────────
// Lifetime IDs and constraints
// ─────────────────────────────────────────────────────────────────────────────

// LifetimeID uniquely identifies one reference lifetime.
type LifetimeID string

const (
	LifetimeStatic  LifetimeID = "'static" // lives forever
	LifetimeUnknown LifetimeID = "'_"      // anonymous / inferred
)

// Constraint represents 'a: 'b — lifetime a must outlive lifetime b.
type Constraint struct {
	Longer  LifetimeID
	Shorter LifetimeID
	Reason  string
}

// VarLifetime maps a variable name to its assigned lifetime.
type VarLifetime struct {
	Name       string
	Lifetime   LifetimeID
	ScopeDepth int // scope depth where it was declared (smaller = longer lived)
}

// ─────────────────────────────────────────────────────────────────────────────
// Analyzer
// ─────────────────────────────────────────────────────────────────────────────

// Analyzer performs static lifetime analysis on the AST.
type Analyzer struct {
	counter     int
	constraints []Constraint
	varLifetime map[string]*VarLifetime // varName → lifetime info
	scopeDepth  int
	errors      []string
	// currentFnLifetimes holds the declared lifetime params of the current fn.
	currentFnLifetimes map[string]LifetimeID
}

// New creates a new lifetime Analyzer.
func New() *Analyzer {
	return &Analyzer{
		varLifetime:        make(map[string]*VarLifetime),
		currentFnLifetimes: make(map[string]LifetimeID),
	}
}

// Errors returns all lifetime analysis errors.
func (a *Analyzer) Errors() []string { return a.errors }

// Constraints returns the built constraint graph (for debugging / testing).
func (a *Analyzer) Constraints() []Constraint { return a.constraints }

// Analyze runs lifetime analysis on a whole file.
func (a *Analyzer) Analyze(file *ast.File) {
	for _, d := range file.Decls {
		a.analyzeDecl(d)
	}
	a.solveConstraints()
}

func (a *Analyzer) errorf(format string, args ...interface{}) {
	a.errors = append(a.errors, fmt.Sprintf("lifetime error: "+format, args...))
}

// freshLifetime generates a unique anonymous lifetime ID.
func (a *Analyzer) freshLifetime() LifetimeID {
	a.counter++
	return LifetimeID(fmt.Sprintf("'lt%d", a.counter))
}

func (a *Analyzer) addConstraint(longer, shorter LifetimeID, reason string) {
	a.constraints = append(a.constraints, Constraint{
		Longer:  longer,
		Shorter: shorter,
		Reason:  reason,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Declarations
// ─────────────────────────────────────────────────────────────────────────────

func (a *Analyzer) analyzeDecl(d ast.Decl) {
	switch decl := d.(type) {
	case *ast.FnDecl:
		a.analyzeFn(decl)
	case *ast.ImplBlock:
		for _, m := range decl.Methods {
			a.analyzeFn(m)
		}
	}
}

func (a *Analyzer) analyzeFn(fn *ast.FnDecl) {
	// Create a fresh scope for this function.
	a.scopeDepth++
	prevFnLifetimes := a.currentFnLifetimes
	a.currentFnLifetimes = make(map[string]LifetimeID)

	// Map declared lifetime params to IDs.
	for _, lt := range fn.Lifetimes {
		// Named lifetimes like 'a get a stable ID from their annotation.
		a.currentFnLifetimes[lt.Name] = LifetimeID(lt.Name)
	}

	// Register parameters and their lifetimes.
	for _, p := range fn.Params {
		lt := a.lifetimeForType(p.Type)
		a.varLifetime[p.Name] = &VarLifetime{
			Name:       p.Name,
			Lifetime:   lt,
			ScopeDepth: a.scopeDepth,
		}
	}

	// Verify return type lifetime constraint.
	if fn.ReturnType != nil && fn.ReturnType.Lifetime != "" {
		retLT := LifetimeID(fn.ReturnType.Lifetime)
		// The returned lifetime must be valid for all corresponding param lifetimes.
		for _, p := range fn.Params {
			if p.Type != nil && p.Type.Lifetime == fn.ReturnType.Lifetime {
				paramLT := a.lifetimeForType(p.Type)
				// returned ref must live at most as long as the param ref source
				a.addConstraint(paramLT, retLT,
					fmt.Sprintf("fn %s: return lifetime %s must not outlive param %s (%s)",
						fn.Name, fn.ReturnType.Lifetime, p.Name, paramLT))
			}
		}
	}

	a.analyzeBlock(fn.Body, fn.ReturnType)
	a.scopeDepth--
	a.currentFnLifetimes = prevFnLifetimes
}

// ─────────────────────────────────────────────────────────────────────────────
// Blocks and statements
// ─────────────────────────────────────────────────────────────────────────────

func (a *Analyzer) analyzeBlock(block *ast.BlockStmt, fnReturnType *ast.TypeExpr) {
	if block == nil {
		return
	}
	a.scopeDepth++
	for _, s := range block.Stmts {
		a.analyzeStmt(s, fnReturnType)
	}
	// Variables declared in this scope go out of scope here.
	// Any references that point to them are now dangling.
	for name, vl := range a.varLifetime {
		if vl.ScopeDepth == a.scopeDepth {
			// Check that no longer-lived reference points at this variable.
			a.checkNoDanglingRef(name, vl)
			delete(a.varLifetime, name)
		}
	}
	a.scopeDepth--
}

func (a *Analyzer) analyzeStmt(s ast.Stmt, fnRet *ast.TypeExpr) {
	switch stmt := s.(type) {
	case *ast.LetDecl:
		lt := a.lifetimeForType(stmt.Type)
		if stmt.Value != nil {
			valLT := a.analyzeExpr(stmt.Value)
			// If we're binding a reference, the source must live at least as long.
			if stmt.Type != nil && stmt.Type.Mode != ast.Owned {
				a.addConstraint(valLT, lt,
					fmt.Sprintf("let %s: source must outlive binding", stmt.Name))
				// Detect immediate dangling: source is already out of scope.
				a.checkLTValid(valLT, stmt.Name)
			}
		}
		a.varLifetime[stmt.Name] = &VarLifetime{
			Name:       stmt.Name,
			Lifetime:   lt,
			ScopeDepth: a.scopeDepth,
		}

	case *ast.ReturnStmt:
		if stmt.Value != nil {
			valLT := a.analyzeExpr(stmt.Value)
			if fnRet != nil && fnRet.Mode != ast.Owned {
				// Returned reference must not be a local variable.
				retLT := a.lifetimeForType(fnRet)
				a.addConstraint(valLT, retLT, "return: value must outlive declared return lifetime")
				a.checkReturnRef(valLT)
			}
		}

	case *ast.ExprStmt:
		a.analyzeExpr(stmt.Expr)

	case *ast.AssignStmt:
		a.analyzeExpr(stmt.Value)

	case *ast.IfStmt:
		a.analyzeExpr(stmt.Cond)
		a.analyzeBlock(stmt.Then, fnRet)
		if stmt.Else != nil {
			a.analyzeStmt(stmt.Else, fnRet)
		}

	case *ast.WhileStmt:
		a.analyzeExpr(stmt.Cond)
		a.analyzeBlock(stmt.Body, fnRet)

	case *ast.ForStmt:
		if stmt.Init != nil {
			a.analyzeStmt(stmt.Init, fnRet)
		}
		if stmt.Cond != nil {
			a.analyzeExpr(stmt.Cond)
		}
		if stmt.Post != nil {
			a.analyzeStmt(stmt.Post, fnRet)
		}
		a.analyzeBlock(stmt.Body, fnRet)

	case *ast.BlockStmt:
		a.analyzeBlock(stmt, fnRet)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Expressions
// ─────────────────────────────────────────────────────────────────────────────

// analyzeExpr returns the lifetime associated with the expression.
func (a *Analyzer) analyzeExpr(e ast.Expr) LifetimeID {
	if e == nil {
		return LifetimeUnknown
	}
	switch expr := e.(type) {
	case *ast.Ident:
		if vl, ok := a.varLifetime[expr.Name]; ok {
			return vl.Lifetime
		}
		return LifetimeStatic // functions are 'static

	case *ast.BorrowExpr:
		// The lifetime of &x is tied to the lifetime of x.
		innerLT := a.analyzeExpr(expr.Operand)
		if expr.Lifetime != "" {
			named := LifetimeID(expr.Lifetime)
			a.addConstraint(innerLT, named,
				fmt.Sprintf("borrow %s: source must outlive named lifetime", expr.Lifetime))
			return named
		}
		return innerLT

	case *ast.CallExpr:
		for _, arg := range expr.Args {
			a.analyzeExpr(arg)
		}
		return LifetimeUnknown

	case *ast.BinaryExpr:
		a.analyzeExpr(expr.Left)
		a.analyzeExpr(expr.Right)
		return LifetimeUnknown

	case *ast.UnaryExpr:
		return a.analyzeExpr(expr.Operand)

	case *ast.MoveExpr:
		return a.analyzeExpr(expr.Operand)

	case *ast.SelectorExpr:
		return a.analyzeExpr(expr.X)

	case *ast.StructLit:
		for _, f := range expr.Fields {
			a.analyzeExpr(f.Value)
		}
		return LifetimeUnknown

	case *ast.IntLit, *ast.StringLit, *ast.BoolLit:
		return LifetimeStatic
	}
	return LifetimeUnknown
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers and constraint solving
// ─────────────────────────────────────────────────────────────────────────────

// lifetimeForType returns the lifetime ID for a type annotation.
func (a *Analyzer) lifetimeForType(t *ast.TypeExpr) LifetimeID {
	if t == nil || t.Mode == ast.Owned {
		return a.freshLifetime() // owned values get a scope-bound lifetime
	}
	if t.Lifetime != "" {
		// Named lifetime — check if it's a declared param.
		if id, ok := a.currentFnLifetimes[t.Lifetime]; ok {
			return id
		}
		return LifetimeID(t.Lifetime)
	}
	return a.freshLifetime()
}

// checkLTValid checks that a lifetime is still valid in the current scope.
func (a *Analyzer) checkLTValid(lt LifetimeID, bindingName string) {
	// If lifetime is already out of scope we would have cleaned it up.
	// For named lifetimes, check they are registered.
	if lt == LifetimeUnknown {
		a.errorf("cannot determine lifetime for binding %q — possible dangling reference", bindingName)
	}
}

// checkReturnRef verifies a returned reference's lifetime is not a local.
func (a *Analyzer) checkReturnRef(lt LifetimeID) {
	// If the lifetime was generated at the current scope, it's a local.
	for _, vl := range a.varLifetime {
		if vl.Lifetime == lt && vl.ScopeDepth >= a.scopeDepth {
			a.errorf("cannot return reference to local variable — it will be dropped")
		}
	}
}

// checkNoDanglingRef verifies no reference to `name` escapes its scope.
func (a *Analyzer) checkNoDanglingRef(name string, vl *VarLifetime) {
	for _, c := range a.constraints {
		if c.Shorter == vl.Lifetime {
			// Something (c.Longer) must outlive this variable — but the variable is
			// being dropped. This is only a problem if c.Longer is a longer-lived binding.
			for _, other := range a.varLifetime {
				if other.Lifetime == c.Longer && other.ScopeDepth < vl.ScopeDepth {
					a.errorf("reference to %q escapes its scope — dangling reference would be created", name)
				}
			}
		}
	}
}

// solveConstraints verifies all constraints are satisfiable.
// Simple check: no constraint should have Longer == Shorter (trivial cycle indicating error).
func (a *Analyzer) solveConstraints() {
	for _, c := range a.constraints {
		if c.Longer == c.Shorter {
			a.errorf("lifetime cycle detected: %s: %s (%s)", c.Longer, c.Shorter, c.Reason)
		}
	}
}
