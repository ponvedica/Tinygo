package lexer_test

import (
	"testing"

	"tinygo/ast"
	"tinygo/lexer"
	"tinygo/parser"
)

func parseFile(t *testing.T, src string) *ast.File {
	t.Helper()
	l := lexer.New([]byte(src))
	p := parser.New(l)
	file, err := p.ParseFile()
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	for _, e := range p.Errors() {
		t.Errorf("parse error: %s", e)
	}
	return file
}

// ----------------------------------------------------------------------------
// Package clause
// ----------------------------------------------------------------------------

func TestPackageClause(t *testing.T) {
	file := parseFile(t, "package main")
	if file.Package != "main" {
		t.Errorf("Package: got %q, want %q", file.Package, "main")
	}
}

// ----------------------------------------------------------------------------
// Imports
// ----------------------------------------------------------------------------

func TestSingleImport(t *testing.T) {
	file := parseFile(t, `package main
import "fmt"
`)
	if len(file.Imports) != 1 || file.Imports[0] != "fmt" {
		t.Errorf("Imports: got %v, want [fmt]", file.Imports)
	}
}

func TestGroupedImport(t *testing.T) {
	file := parseFile(t, `package main
import (
	"fmt"
	"os"
)
`)
	if len(file.Imports) != 2 {
		t.Errorf("Import count: got %d, want 2", len(file.Imports))
	}
}

// ----------------------------------------------------------------------------
// Function declarations
// ----------------------------------------------------------------------------

func TestEmptyFunc(t *testing.T) {
	file := parseFile(t, `package p
func hello() {
}`)
	if len(file.Decls) != 1 {
		t.Fatalf("Decls: got %d, want 1", len(file.Decls))
	}
	fn, ok := file.Decls[0].(*ast.FuncDecl)
	if !ok {
		t.Fatalf("Decl[0] is not *FuncDecl")
	}
	if fn.Name != "hello" {
		t.Errorf("FuncDecl.Name: got %q, want %q", fn.Name, "hello")
	}
	if fn.ReturnType != "" {
		t.Errorf("FuncDecl.ReturnType: got %q, want empty", fn.ReturnType)
	}
}

func TestFuncWithParams(t *testing.T) {
	file := parseFile(t, `package p
func add(a int, b int) int {
	return a
}`)
	fn := file.Decls[0].(*ast.FuncDecl)
	if len(fn.Params) != 2 {
		t.Errorf("Params: got %d, want 2", len(fn.Params))
	}
	if fn.Params[0].Name != "a" || fn.Params[0].Type != "int" {
		t.Errorf("Param[0]: got %+v", fn.Params[0])
	}
	if fn.ReturnType != "int" {
		t.Errorf("ReturnType: got %q, want int", fn.ReturnType)
	}
}

// ----------------------------------------------------------------------------
// Statements
// ----------------------------------------------------------------------------

func TestVarDeclShort(t *testing.T) {
	file := parseFile(t, `package p
func main() {
	x := 42
}`)
	fn := file.Decls[0].(*ast.FuncDecl)
	if len(fn.Body.Stmts) != 1 {
		t.Fatalf("Stmts: got %d, want 1", len(fn.Body.Stmts))
	}
	vd, ok := fn.Body.Stmts[0].(*ast.VarDeclStmt)
	if !ok {
		t.Fatalf("Stmt[0]: want *VarDeclStmt, got %T", fn.Body.Stmts[0])
	}
	if vd.Name != "x" {
		t.Errorf("VarDecl.Name: got %q, want x", vd.Name)
	}
	lit, ok := vd.Value.(*ast.IntLit)
	if !ok {
		t.Fatalf("VarDecl.Value: want *IntLit, got %T", vd.Value)
	}
	if lit.Value != 42 {
		t.Errorf("IntLit.Value: got %d, want 42", lit.Value)
	}
}

func TestReturnStatement(t *testing.T) {
	file := parseFile(t, `package p
func f() int {
	return 7
}`)
	fn := file.Decls[0].(*ast.FuncDecl)
	ret, ok := fn.Body.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("Stmt: want *ReturnStmt, got %T", fn.Body.Stmts[0])
	}
	lit := ret.Value.(*ast.IntLit)
	if lit.Value != 7 {
		t.Errorf("Return value: got %d, want 7", lit.Value)
	}
}

func TestIfElse(t *testing.T) {
	file := parseFile(t, `package p
func f(x int) {
	if x > 0 {
		return
	} else {
		return
	}
}`)
	fn := file.Decls[0].(*ast.FuncDecl)
	ifStmt, ok := fn.Body.Stmts[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("Stmt: want *IfStmt, got %T", fn.Body.Stmts[0])
	}
	if ifStmt.Else == nil {
		t.Error("IfStmt.Else: expected non-nil else block")
	}
}

func TestForCStyle(t *testing.T) {
	file := parseFile(t, `package p
func f() {
	for i := 0; i < 10; i++ {
		return
	}
}`)
	fn := file.Decls[0].(*ast.FuncDecl)
	forStmt, ok := fn.Body.Stmts[0].(*ast.ForStmt)
	if !ok {
		t.Fatalf("Stmt: want *ForStmt, got %T", fn.Body.Stmts[0])
	}
	if forStmt.Init == nil {
		t.Error("ForStmt.Init: expected non-nil")
	}
	if forStmt.Cond == nil {
		t.Error("ForStmt.Cond: expected non-nil")
	}
	if forStmt.Post == nil {
		t.Error("ForStmt.Post: expected non-nil")
	}
}

// ----------------------------------------------------------------------------
// Expressions
// ----------------------------------------------------------------------------

func TestBinaryExpr(t *testing.T) {
	file := parseFile(t, `package p
func f() int {
	return 1 + 2 * 3
}`)
	fn := file.Decls[0].(*ast.FuncDecl)
	ret := fn.Body.Stmts[0].(*ast.ReturnStmt)
	// Pratt parsing: * binds tighter → (1 + (2 * 3))
	bin, ok := ret.Value.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("Return expr: want *BinaryExpr, got %T", ret.Value)
	}
	if bin.Op != "+" {
		t.Errorf("BinaryExpr.Op: got %q, want +", bin.Op)
	}
	right, ok := bin.Right.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("Right: want *BinaryExpr (2*3), got %T", bin.Right)
	}
	if right.Op != "*" {
		t.Errorf("Right.Op: got %q, want *", right.Op)
	}
}

func TestCallExpr(t *testing.T) {
	file := parseFile(t, `package p
import "fmt"
func main() {
	fmt.Println("hi")
}`)
	fn := file.Decls[0].(*ast.FuncDecl)
	expr := fn.Body.Stmts[0].(*ast.ExprStmt)
	call, ok := expr.Expr.(*ast.CallExpr)
	if !ok {
		t.Fatalf("Expr: want *CallExpr, got %T", expr.Expr)
	}
	if len(call.Args) != 1 {
		t.Errorf("Args: got %d, want 1", len(call.Args))
	}
}

func TestSelectorExpr(t *testing.T) {
	file := parseFile(t, `package p
import "fmt"
func main() {
	fmt.Println("x")
}`)
	fn := file.Decls[0].(*ast.FuncDecl)
	expr := fn.Body.Stmts[0].(*ast.ExprStmt)
	call := expr.Expr.(*ast.CallExpr)
	sel, ok := call.Func.(*ast.SelectorExpr)
	if !ok {
		t.Fatalf("Call.Func: want *SelectorExpr, got %T", call.Func)
	}
	if sel.Sel != "Println" {
		t.Errorf("Selector: got %q, want Println", sel.Sel)
	}
}

func TestUnaryNegation(t *testing.T) {
	file := parseFile(t, `package p
func f() int {
	return -5
}`)
	fn := file.Decls[0].(*ast.FuncDecl)
	ret := fn.Body.Stmts[0].(*ast.ReturnStmt)
	u, ok := ret.Value.(*ast.UnaryExpr)
	if !ok {
		t.Fatalf("Return: want *UnaryExpr, got %T", ret.Value)
	}
	if u.Op != "-" {
		t.Errorf("UnaryExpr.Op: got %q, want -", u.Op)
	}
}

// ----------------------------------------------------------------------------
// Integration: full fibonacci source
// ----------------------------------------------------------------------------

func TestFibSource(t *testing.T) {
	src := `package main

import "fmt"

func fib(n int) int {
	if n <= 1 {
		return n
	}
	a := 0
	b := 1
	i := 2
	for i <= n {
		c := a + b
		a = b
		b = c
		i = i + 1
	}
	return b
}

func main() {
	result := fib(10)
	fmt.Println(result)
}`
	file := parseFile(t, src)
	if len(file.Decls) != 2 {
		t.Errorf("Decls: got %d, want 2 (fib, main)", len(file.Decls))
	}
}
