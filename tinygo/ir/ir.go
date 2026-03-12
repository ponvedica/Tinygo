// Package ir defines the Intermediate Representation (IR) for the tinygo compiler.
//
// Inspired by babygo's internal/ir package, this IR sits between the parsed AST
// and the code generator. The semantic analysis (typechecker) lowers the typed AST
// into these IR nodes, which the codegen then walks to emit C (or assembly).
//
// Benefits of the IR layer:
//   - Codegen is decoupled from parsing details (positions, raw token literals).
//   - Optimisation passes can operate on IR without touching the AST.
//   - Easy to swap backends (C, x86-64, LLVM IR) by implementing a new codegen pass.
package ir

import "tinygo/ast"

// ----------------------------------------------------------------------------
// Package container (mirrors babygo's PkgContainer)
// ----------------------------------------------------------------------------

// Package represents one compiled Go package.
type Package struct {
	Name  string
	Path  string
	Files []*File
	Funcs []*Func
	Vars  []*GlobalVar
}

// File is one source file within a package.
type File struct {
	Name string
	Pkg  *Package
}

// ----------------------------------------------------------------------------
// Declarations
// ----------------------------------------------------------------------------

// Func is a compiled function ready for code generation.
type Func struct {
	Name       string
	Params     []*Param
	ReturnType string
	Body       *IRBlock
	// LocalVars is the complete set of locals declared in this function (for
	// stack-frame allocation in assembly backends).
	LocalVars []*LocalVar
}

// Param is a function parameter.
type Param struct {
	Name string
	Type string
}

// GlobalVar is a package-level variable.
type GlobalVar struct {
	Name  string
	Type  string
	Value MetaExpr // nil if zero-initialised
}

// LocalVar is a variable declared inside a function body.
type LocalVar struct {
	Name string
	Type string
}

// ----------------------------------------------------------------------------
// IR Statements
// ----------------------------------------------------------------------------

// IRBlock is a sequence of IR statements.
type IRBlock struct {
	Stmts []IRStmt
}

// IRStmt is any IR-level statement.
type IRStmt interface {
	irStmt()
}

// IRVarDecl declares a (possibly initialised) local variable.
type IRVarDecl struct {
	Name  string
	Type  string
	Value MetaExpr // may be nil
}

func (*IRVarDecl) irStmt() {}

// IRAssign is an assignment: target op= value
type IRAssign struct {
	Target MetaExpr
	Op     string // "=", "+=", "-=", "*=", "/="
	Value  MetaExpr
}

func (*IRAssign) irStmt() {}

// IRIncDec is target++ or target--
type IRIncDec struct {
	Target MetaExpr
	Op     string // "++" | "--"
}

func (*IRIncDec) irStmt() {}

// IRReturn is a return statement.
type IRReturn struct {
	Value MetaExpr // nil = bare return
}

func (*IRReturn) irStmt() {}

// IRExprStmt is a statement containing only an expression (e.g., a call).
type IRExprStmt struct {
	Expr MetaExpr
}

func (*IRExprStmt) irStmt() {}

// IRIf is an if/else-if/else chain.
type IRIf struct {
	Cond MetaExpr
	Then *IRBlock
	Else IRStmt // nil | *IRBlock | *IRIf
}

func (*IRIf) irStmt() {}

// IRFor is a for loop: for Init; Cond; Post { Body }
// Any of Init/Cond/Post may be nil (infinite / while loop).
type IRFor struct {
	Init IRStmt  // *IRVarDecl | *IRAssign | *IRIncDec | nil
	Cond MetaExpr
	Post IRStmt
	Body *IRBlock
}

func (*IRFor) irStmt() {}

// IRBlock itself can appear as a statement (nested scopes).
func (*IRBlock) irStmt() {}

// ----------------------------------------------------------------------------
// Meta Expressions (typed IR expression nodes)
// Mirrors babygo's MetaExpr interface.
// ----------------------------------------------------------------------------

// MetaExpr is a typed IR expression.
type MetaExpr interface {
	GetType() string
	irExpr()
}

// MetaIntLit is an integer literal.
type MetaIntLit struct {
	Value int64
	Type  string // "int"
}

func (m *MetaIntLit) GetType() string { return m.Type }
func (m *MetaIntLit) irExpr()         {}

// MetaStringLit is a string literal.
type MetaStringLit struct {
	Value string
}

func (m *MetaStringLit) GetType() string { return "string" }
func (m *MetaStringLit) irExpr()         {}

// MetaBoolLit is a boolean literal.
type MetaBoolLit struct {
	Value bool
}

func (m *MetaBoolLit) GetType() string { return "bool" }
func (m *MetaBoolLit) irExpr()         {}

// MetaIdent references a variable or function.
type MetaIdent struct {
	Name string
	Type string
}

func (m *MetaIdent) GetType() string { return m.Type }
func (m *MetaIdent) irExpr()         {}

// MetaSelector is a pkg.name or obj.field expression.
type MetaSelector struct {
	X    MetaExpr
	Sel  string
	Type string
}

func (m *MetaSelector) GetType() string { return m.Type }
func (m *MetaSelector) irExpr()         {}

// MetaBinaryExpr is left op right.
type MetaBinaryExpr struct {
	Left  MetaExpr
	Op    string
	Right MetaExpr
	Type  string
}

func (m *MetaBinaryExpr) GetType() string { return m.Type }
func (m *MetaBinaryExpr) irExpr()         {}

// MetaUnaryExpr is op operand.
type MetaUnaryExpr struct {
	Op      string
	Operand MetaExpr
	Type    string
}

func (m *MetaUnaryExpr) GetType() string { return m.Type }
func (m *MetaUnaryExpr) irExpr()         {}

// MetaCall is a function or method call.
type MetaCall struct {
	Func MetaExpr
	Args []MetaExpr
	Type string // return type
}

func (m *MetaCall) GetType() string { return m.Type }
func (m *MetaCall) irExpr()         {}

// ----------------------------------------------------------------------------
// AST → IR lowering helper (walks raw ast.Expr into MetaExpr).
// The typechecker calls this after resolving types.
// ----------------------------------------------------------------------------

// LowerExpr converts a typed ast.Expr into a MetaExpr.
// resolvedType is provided by the type-checker.
func LowerExpr(e ast.Expr, resolvedType string) MetaExpr {
	if e == nil {
		return nil
	}
	switch expr := e.(type) {
	case *ast.IntLit:
		return &MetaIntLit{Value: expr.Value, Type: "int"}
	case *ast.StringLit:
		return &MetaStringLit{Value: expr.Value}
	case *ast.BoolLit:
		return &MetaBoolLit{Value: expr.Value}
	case *ast.Ident:
		return &MetaIdent{Name: expr.Name, Type: resolvedType}
	case *ast.SelectorExpr:
		x := LowerExpr(expr.X, "")
		return &MetaSelector{X: x, Sel: expr.Sel, Type: resolvedType}
	case *ast.BinaryExpr:
		l := LowerExpr(expr.Left, "")
		r := LowerExpr(expr.Right, "")
		return &MetaBinaryExpr{Left: l, Op: expr.Op, Right: r, Type: resolvedType}
	case *ast.UnaryExpr:
		op := LowerExpr(expr.Operand, "")
		return &MetaUnaryExpr{Op: expr.Op, Operand: op, Type: resolvedType}
	case *ast.CallExpr:
		fn := LowerExpr(expr.Func, "")
		var args []MetaExpr
		for _, a := range expr.Args {
			args = append(args, LowerExpr(a, ""))
		}
		return &MetaCall{Func: fn, Args: args, Type: resolvedType}
	}
	return &MetaIdent{Name: "/* unknown */", Type: resolvedType}
}
