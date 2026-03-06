// Unit tests for all three static analysis passes of the Ore compiler.
// Tests verify that valid programs pass without errors,
// and that invalid programs produce the expected error messages.
package tests

import (
	"strings"
	"testing"

	"goforust/ast"
	"goforust/borrow"
	"goforust/lexer"
	"goforust/lifetime"
	"goforust/ownership"
	"goforust/parser"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

func parseOre(t *testing.T, src string) *ast.File {
	t.Helper()
	l := lexer.New([]byte(src))
	p := parser.New(l)
	file, err := p.ParseFile()
	for _, e := range p.Errors() {
		t.Errorf("parse error: %s", e)
	}
	if err != nil {
		t.Fatalf("parse fatal: %v", err)
	}
	return file
}

func expectOwnershipErrors(t *testing.T, src string, fragments ...string) {
	t.Helper()
	file := parseOre(t, src)
	oc := ownership.New()
	oc.Check(file)
	errs := oc.Errors()
	if len(fragments) == 0 && len(errs) > 0 {
		t.Errorf("expected no ownership errors, got: %v", errs)
	}
	for _, frag := range fragments {
		found := false
		for _, e := range errs {
			if strings.Contains(e, frag) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected ownership error containing %q, got: %v", frag, errs)
		}
	}
}

func expectBorrowErrors(t *testing.T, src string, fragments ...string) {
	t.Helper()
	file := parseOre(t, src)
	bc := borrow.New()
	bc.Check(file)
	errs := bc.Errors()
	if len(fragments) == 0 && len(errs) > 0 {
		t.Errorf("expected no borrow errors, got: %v", errs)
	}
	for _, frag := range fragments {
		found := false
		for _, e := range errs {
			if strings.Contains(e, frag) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected borrow error containing %q, got: %v", frag, errs)
		}
	}
}

func expectLifetimeErrors(t *testing.T, src string, fragments ...string) {
	t.Helper()
	file := parseOre(t, src)
	la := lifetime.New()
	la.Analyze(file)
	errs := la.Errors()
	if len(fragments) == 0 && len(errs) > 0 {
		t.Errorf("expected no lifetime errors, got: %v", errs)
	}
	for _, frag := range fragments {
		found := false
		for _, e := range errs {
			if strings.Contains(e, frag) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected lifetime error containing %q, got: %v", frag, errs)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Lexer tests
// ─────────────────────────────────────────────────────────────────────────────

func TestLexLifetime(t *testing.T) {
	src := `'a 'static 'b`
	l := lexer.New([]byte(src))
	tokens := l.Tokenize()
	if tokens[0].Type != lexer.LIFETIME || tokens[0].Literal != "'a" {
		t.Errorf("expected LIFETIME 'a, got %v", tokens[0])
	}
	if tokens[1].Type != lexer.LIFETIME || tokens[1].Literal != "'static" {
		t.Errorf("expected LIFETIME 'static, got %v", tokens[1])
	}
}

func TestLexAmpMut(t *testing.T) {
	src := `&mut x`
	l := lexer.New([]byte(src))
	tokens := l.Tokenize()
	if tokens[0].Type != lexer.AMPMUT {
		t.Errorf("expected AMPMUT, got %v", tokens[0])
	}
}

func TestLexArrow(t *testing.T) {
	src := `-> Int`
	l := lexer.New([]byte(src))
	tokens := l.Tokenize()
	if tokens[0].Type != lexer.ARROW {
		t.Errorf("expected ARROW, got %v", tokens[0])
	}
}

func TestLexKeywords(t *testing.T) {
	src := `let mut fn struct impl return if else while`
	l := lexer.New([]byte(src))
	toks := l.Tokenize()
	expected := []lexer.TokenType{
		lexer.LET, lexer.MUT, lexer.FN, lexer.STRUCT, lexer.IMPL,
		lexer.RETURN, lexer.IF, lexer.ELSE, lexer.WHILE,
	}
	for i, want := range expected {
		if toks[i].Type != want {
			t.Errorf("token[%d]: got %s, want %s", i, toks[i].Type, want)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Parser tests
// ─────────────────────────────────────────────────────────────────────────────

func TestParseLetDecl(t *testing.T) {
	src := `fn main() { let x: Int = 42; }`
	file := parseOre(t, src)
	fn := file.Decls[0].(*ast.FnDecl)
	let, ok := fn.Body.Stmts[0].(*ast.LetDecl)
	if !ok {
		t.Fatalf("expected LetDecl, got %T", fn.Body.Stmts[0])
	}
	if let.Name != "x" || let.Mutable {
		t.Errorf("LetDecl: got name=%s mutable=%v", let.Name, let.Mutable)
	}
}

func TestParseLetMut(t *testing.T) {
	src := `fn main() { let mut count: Int = 0; }`
	file := parseOre(t, src)
	fn := file.Decls[0].(*ast.FnDecl)
	let := fn.Body.Stmts[0].(*ast.LetDecl)
	if !let.Mutable {
		t.Error("expected Mutable=true for let mut")
	}
}

func TestParseBorrowExpr(t *testing.T) {
	src := `fn f() { let r: &Int = &x; }`
	file := parseOre(t, src)
	fn := file.Decls[0].(*ast.FnDecl)
	let := fn.Body.Stmts[0].(*ast.LetDecl)
	if let.Type.Mode != ast.Shared {
		t.Errorf("type mode: got %v, want Shared", let.Type.Mode)
	}
	borrow, ok := let.Value.(*ast.BorrowExpr)
	if !ok {
		t.Fatalf("value: want *BorrowExpr, got %T", let.Value)
	}
	if borrow.Mode != ast.Shared {
		t.Errorf("borrow mode: got %v, want Shared", borrow.Mode)
	}
}

func TestParseMutBorrowExpr(t *testing.T) {
	src := `fn f(x: &mut Int) { }`
	file := parseOre(t, src)
	fn := file.Decls[0].(*ast.FnDecl)
	if fn.Params[0].Type.Mode != ast.MutBorrow {
		t.Errorf("param mode: want MutBorrow, got %v", fn.Params[0].Type.Mode)
	}
}

func TestParseLifetimeParam(t *testing.T) {
	src := `fn longest<'a>(x: &'a String, y: &'a String) -> &'a String { return x; }`
	file := parseOre(t, src)
	fn := file.Decls[0].(*ast.FnDecl)
	if len(fn.Lifetimes) != 1 || fn.Lifetimes[0].Name != "'a" {
		t.Errorf("lifetimes: got %v, want ['a]", fn.Lifetimes)
	}
	if fn.ReturnType == nil || fn.ReturnType.Lifetime != "'a" {
		t.Errorf("return lifetime: got %v, want 'a", fn.ReturnType)
	}
}

func TestParseStruct(t *testing.T) {
	src := `struct Point { x: Int, y: Int }`
	file := parseOre(t, src)
	s, ok := file.Decls[0].(*ast.StructDecl)
	if !ok {
		t.Fatalf("expected StructDecl, got %T", file.Decls[0])
	}
	if s.Name != "Point" || len(s.Fields) != 2 {
		t.Errorf("StructDecl: name=%s fields=%d", s.Name, len(s.Fields))
	}
}

func TestParseWhile(t *testing.T) {
	src := `fn f() { while i < 10 { i = i + 1; } }`
	file := parseOre(t, src)
	fn := file.Decls[0].(*ast.FnDecl)
	_, ok := fn.Body.Stmts[0].(*ast.WhileStmt)
	if !ok {
		t.Fatalf("expected WhileStmt, got %T", fn.Body.Stmts[0])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Ownership checker tests
// ─────────────────────────────────────────────────────────────────────────────

func TestOwnership_ValidMove(t *testing.T) {
	// Moving a value into a function is fine — no use after.
	src := `
fn consume(s: String) { println(s); }
fn main() {
    let msg: String = "hi";
    consume(msg);
}`
	expectOwnershipErrors(t, src) // no errors expected
}

func TestOwnership_UseAfterMove(t *testing.T) {
	src := `
fn take(s: String) { println(s); }
fn main() {
    let s: String = "hello";
    take(s);
    println(s);
}`
	expectOwnershipErrors(t, src, "use of moved value", "s")
}

func TestOwnership_ImmutableAssign(t *testing.T) {
	src := `
fn main() {
    let x: Int = 5;
    x = 10;
}`
	expectOwnershipErrors(t, src, "immutable")
}

func TestOwnership_BorrowDontMove(t *testing.T) {
	// Passing by borrow should NOT move the value.
	src := `
fn read(s: &String) { println(s); }
fn main() {
    let msg: String = "hello";
    read(&msg);
    read(&msg);
}`
	expectOwnershipErrors(t, src) // no errors expected
}

// ─────────────────────────────────────────────────────────────────────────────
// Borrow checker tests
// ─────────────────────────────────────────────────────────────────────────────

func TestBorrow_SharedBorrowsOk(t *testing.T) {
	src := `
fn main() {
    let mut x: Int = 5;
    let r1: &Int = &x;
    let r2: &Int = &x;
    println(x);
}`
	expectBorrowErrors(t, src) // two shared borrows are fine
}

func TestBorrow_MutAndSharedConflict(t *testing.T) {
	src := `
fn main() {
    let mut x: Int = 5;
    let r1: &Int = &x;
    let r2: &mut Int = &mut x;
    println(x);
}`
	expectBorrowErrors(t, src, "cannot borrow", "mutable")
}

func TestBorrow_MutRequiresMutBinding(t *testing.T) {
	src := `
fn main() {
    let x: Int = 5;
    let r: &mut Int = &mut x;
}`
	expectBorrowErrors(t, src, "not declared as mutable")
}

func TestBorrow_AssignImmutableBinding(t *testing.T) {
	src := `
fn main() {
    let x: Int = 5;
    x = 10;
}`
	expectBorrowErrors(t, src, "immutable binding")
}

// ─────────────────────────────────────────────────────────────────────────────
// Lifetime analyzer tests
// ─────────────────────────────────────────────────────────────────────────────

func TestLifetime_ValidFn(t *testing.T) {
	src := `fn longest<'a>(x: &'a String, y: &'a String) -> &'a String { return x; }`
	expectLifetimeErrors(t, src) // no errors
}

func TestLifetime_ConstraintBuilt(t *testing.T) {
	src := `fn longest<'a>(x: &'a String, y: &'a String) -> &'a String { return x; }`
	file := parseOre(t, src)
	la := lifetime.New()
	la.Analyze(file)
	if len(la.Constraints()) == 0 {
		t.Error("expected at least one lifetime constraint to be built")
	}
}

func TestLifetime_LiteralsAreStatic(t *testing.T) {
	src := `fn main() { let x: Int = 42; }`
	file := parseOre(t, src)
	la := lifetime.New()
	la.Analyze(file)
	if len(la.Errors()) > 0 {
		t.Errorf("integer literal should have 'static lifetime, got errors: %v", la.Errors())
	}
}
