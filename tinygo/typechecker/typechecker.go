// Package typechecker implements semantic analysis and type checking for tinygo.
// It builds a symbol table (scope chain), resolves identifiers, checks types,
// and verifies return statements.
package typechecker

import (
	"fmt"
	"tinygo/ast"
)

// ----------------------------------------------------------------------------
// Types
// ----------------------------------------------------------------------------

// Type represents a data type in the tinygo type system.
type Type string

const (
	TypeInt    Type = "int"
	TypeString Type = "string"
	TypeBool   Type = "bool"
	TypeVoid   Type = "void"
	TypeAny    Type = "any" // used for fmt functions, etc.
	TypeUnknown Type = ""
)

// ----------------------------------------------------------------------------
// Scope / Symbol Table
// ----------------------------------------------------------------------------

// Symbol holds info about a declared name.
type Symbol struct {
	Name string
	Type Type
	Kind string // "var", "func"
}

// Scope is a linked-list node of symbol tables.
type Scope struct {
	symbols map[string]*Symbol
	parent  *Scope
}

func newScope(parent *Scope) *Scope {
	return &Scope{symbols: make(map[string]*Symbol), parent: parent}
}

func (s *Scope) define(sym *Symbol) {
	s.symbols[sym.Name] = sym
}

func (s *Scope) lookup(name string) (*Symbol, bool) {
	if sym, ok := s.symbols[name]; ok {
		return sym, true
	}
	if s.parent != nil {
		return s.parent.lookup(name)
	}
	return nil, false
}

// ----------------------------------------------------------------------------
// TypeChecker
// ----------------------------------------------------------------------------

// TypeChecker walks the AST, resolves names, and checks types.
type TypeChecker struct {
	scope       *Scope
	currentFunc *ast.FuncDecl
	errors      []string
}

// New creates a new TypeChecker with a pre-populated global scope.
func New() *TypeChecker {
	global := newScope(nil)

	// Pre-declare built-in functions
	builtins := []struct {
		name string
		ret  Type
	}{
		{"println", TypeVoid},
		{"print", TypeVoid},
		{"len", TypeInt},
	}
	for _, b := range builtins {
		global.define(&Symbol{Name: b.name, Type: b.ret, Kind: "func"})
	}

	return &TypeChecker{scope: global}
}

// Check performs type checking on the parsed file.
func (tc *TypeChecker) Check(file *ast.File) []string {
	// First pass: register all top-level func names so they can be called
	// before their declaration (mutual recursion).
	for _, d := range file.Decls {
		switch decl := d.(type) {
		case *ast.FuncDecl:
			ret := Type(decl.ReturnType)
			if ret == "" {
				ret = TypeVoid
			}
			tc.scope.define(&Symbol{Name: decl.Name, Type: ret, Kind: "func"})
		case *ast.VarDeclTop:
			t := tc.inferType(decl.Value)
			if decl.Type != "" {
				t = Type(decl.Type)
			}
			tc.scope.define(&Symbol{Name: decl.Name, Type: t, Kind: "var"})
		}
	}

	// Second pass: check bodies
	for _, d := range file.Decls {
		tc.checkDecl(d)
	}
	return tc.errors
}

func (tc *TypeChecker) errorf(format string, args ...interface{}) {
	tc.errors = append(tc.errors, fmt.Sprintf(format, args...))
}

func (tc *TypeChecker) pushScope() {
	tc.scope = newScope(tc.scope)
}

func (tc *TypeChecker) popScope() {
	tc.scope = tc.scope.parent
}

// ----------------------------------------------------------------------------
// Declarations
// ----------------------------------------------------------------------------

func (tc *TypeChecker) checkDecl(d ast.Decl) {
	switch decl := d.(type) {
	case *ast.FuncDecl:
		tc.checkFuncDecl(decl)
	case *ast.VarDeclTop:
		if decl.Value != nil {
			tc.inferType(decl.Value)
		}
	}
}

func (tc *TypeChecker) checkFuncDecl(fn *ast.FuncDecl) {
	prev := tc.currentFunc
	tc.currentFunc = fn
	tc.pushScope()

	// Register parameters
	for _, p := range fn.Params {
		tc.scope.define(&Symbol{Name: p.Name, Type: Type(p.Type), Kind: "var"})
	}

	tc.checkBlock(fn.Body)
	tc.popScope()
	tc.currentFunc = prev
}

// ----------------------------------------------------------------------------
// Statements
// ----------------------------------------------------------------------------

func (tc *TypeChecker) checkBlock(block *ast.BlockStmt) {
	if block == nil {
		return
	}
	tc.pushScope()
	for _, s := range block.Stmts {
		tc.checkStmt(s)
	}
	tc.popScope()
}

func (tc *TypeChecker) checkStmt(s ast.Stmt) {
	switch stmt := s.(type) {
	case *ast.VarDeclStmt:
		t := tc.inferType(stmt.Value)
		if stmt.Type != "" {
			t = Type(stmt.Type)
		}
		tc.scope.define(&Symbol{Name: stmt.Name, Type: t, Kind: "var"})

	case *ast.AssignStmt:
		if ident, ok := stmt.Target.(*ast.Ident); ok {
			if _, found := tc.scope.lookup(ident.Name); !found {
				tc.errorf("undefined variable %q", ident.Name)
			}
		}
		tc.inferType(stmt.Value)

	case *ast.IncDecStmt:
		if ident, ok := stmt.Target.(*ast.Ident); ok {
			if _, found := tc.scope.lookup(ident.Name); !found {
				tc.errorf("undefined variable %q", ident.Name)
			}
		}

	case *ast.IfStmt:
		cond := tc.inferType(stmt.Cond)
		if cond != TypeBool && cond != TypeAny && cond != TypeUnknown {
			tc.errorf("if condition must be bool, got %s", cond)
		}
		tc.checkBlock(stmt.Then)
		if stmt.Else != nil {
			switch e := stmt.Else.(type) {
			case *ast.BlockStmt:
				tc.checkBlock(e)
			case *ast.IfStmt:
				tc.checkStmt(e)
			}
		}

	case *ast.ForStmt:
		tc.pushScope()
		if stmt.Init != nil {
			tc.checkStmt(stmt.Init)
		}
		if stmt.Cond != nil {
			tc.inferType(stmt.Cond)
		}
		if stmt.Post != nil {
			tc.checkStmt(stmt.Post)
		}
		// Check body without extra scope push (checkBlock pushes its own)
		if stmt.Body != nil {
			for _, s := range stmt.Body.Stmts {
				tc.checkStmt(s)
			}
		}
		tc.popScope()

	case *ast.ReturnStmt:
		if tc.currentFunc == nil {
			tc.errorf("return outside function")
			return
		}
		expected := Type(tc.currentFunc.ReturnType)
		if expected == "" {
			expected = TypeVoid
		}
		if stmt.Value == nil {
			if expected != TypeVoid {
				tc.errorf("function %q expects return type %s, got nothing", tc.currentFunc.Name, expected)
			}
		} else {
			got := tc.inferType(stmt.Value)
			if expected != TypeVoid && got != expected && got != TypeAny && expected != TypeAny {
				tc.errorf("function %q: return type mismatch: expected %s, got %s",
					tc.currentFunc.Name, expected, got)
			}
		}

	case *ast.ExprStmt:
		tc.inferType(stmt.Expr)

	case *ast.BlockStmt:
		tc.checkBlock(stmt)
	}
}

// ----------------------------------------------------------------------------
// Expression type inference
// ----------------------------------------------------------------------------

// inferType returns the inferred type of an expression.
func (tc *TypeChecker) inferType(e ast.Expr) Type {
	if e == nil {
		return TypeVoid
	}
	switch expr := e.(type) {
	case *ast.IntLit:
		return TypeInt
	case *ast.StringLit:
		return TypeString
	case *ast.BoolLit:
		return TypeBool
	case *ast.Ident:
		sym, ok := tc.scope.lookup(expr.Name)
		if !ok {
			tc.errorf("undefined identifier %q", expr.Name)
			return TypeUnknown
		}
		return sym.Type

	case *ast.SelectorExpr:
		// e.g. fmt.Println — treat as Any
		return TypeAny

	case *ast.CallExpr:
		// Infer return type of callee
		var retType Type
		switch fn := expr.Func.(type) {
		case *ast.Ident:
			sym, ok := tc.scope.lookup(fn.Name)
			if !ok {
				tc.errorf("undefined function %q", fn.Name)
				return TypeUnknown
			}
			retType = sym.Type
		case *ast.SelectorExpr:
			retType = TypeAny // fmt.Println etc.
		default:
			retType = TypeAny
		}
		for _, arg := range expr.Args {
			tc.inferType(arg)
		}
		return retType

	case *ast.BinaryExpr:
		lt := tc.inferType(expr.Left)
		rt := tc.inferType(expr.Right)
		// comparison operators always produce bool
		switch expr.Op {
		case "==", "!=", "<", ">", "<=", ">=", "&&", "||":
			return TypeBool
		}
		// arithmetic
		if lt == TypeInt && rt == TypeInt {
			return TypeInt
		}
		if lt == TypeString && expr.Op == "+" {
			return TypeString
		}
		if lt == TypeAny || rt == TypeAny {
			return TypeAny
		}
		if lt != TypeUnknown && rt != TypeUnknown && lt != rt {
			tc.errorf("type mismatch in binary expr: %s %s %s", lt, expr.Op, rt)
		}
		if lt != TypeUnknown {
			return lt
		}
		return rt

	case *ast.UnaryExpr:
		t := tc.inferType(expr.Operand)
		if expr.Op == "!" {
			return TypeBool
		}
		return t
	}
	return TypeUnknown
}
