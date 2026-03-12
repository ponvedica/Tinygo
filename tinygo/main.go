// tinygo — a Go compiler written in Go, from scratch.
//
// Architecture (inspired by babygo & tinygo-org/tinygo):
//
//   Source (.go)
//     │
//     ▼
//   Lexer  (lexer/)        — tokenize source bytes into Tokens
//     │
//     ▼
//   Parser (parser/)       — recursive-descent + Pratt → AST (ast/)
//     │
//     ▼
//   TypeChecker (typechecker/) — scope chain, type inference, error reporting
//     │
//     ▼
//   IR lowering (ir/)      — typed MetaExpr nodes, IRBlock/IRStmt tree
//     │
//     ▼
//   CodeGen (codegen/)     — emit C source → gcc → native binary
//
// Usage:
//   tinygo build [-o output] <file.go>
//   tinygo lex   <file.go>         — dump tokens
//   tinygo parse <file.go>         — dump AST
//   tinygo emit  <file.go>         — dump generated C
package main

import (
	"fmt"
	"os"
	"strings"

	"tinygo/ast"
	"tinygo/codegen"
	"tinygo/lexer"
	"tinygo/parser"
	"tinygo/typechecker"
)

const version = "0.1.0"

func usage() {
	fmt.Fprintf(os.Stderr, `tinygo — a Go compiler written in Go (v%s)

Usage:
  tinygo build [-o <output>] <file.go>    compile and link
  tinygo lex   <file.go>                  dump lexed tokens
  tinygo parse <file.go>                  dump parsed AST
  tinygo emit  <file.go>                  dump generated C source
  tinygo version                          show version

`, version)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("tinygo %s\n", version)

	case "lex":
		if len(os.Args) < 3 {
			fatalf("lex: missing <file.go>")
		}
		runLex(os.Args[2])

	case "parse":
		if len(os.Args) < 3 {
			fatalf("parse: missing <file.go>")
		}
		runParse(os.Args[2])

	case "emit":
		if len(os.Args) < 3 {
			fatalf("emit: missing <file.go>")
		}
		runEmit(os.Args[2])

	case "build":
		args := os.Args[2:]
		output := "a.out"
		if len(args) >= 2 && args[0] == "-o" {
			output = args[1]
			args = args[2:]
		}
		if len(args) < 1 {
			fatalf("build: missing <file.go>")
		}
		runBuild(args[0], output)

	default:
		// Treat bare file argument as implicit "build"
		if len(os.Args) == 2 {
			runBuild(os.Args[1], "a.out")
		} else {
			usage()
			os.Exit(1)
		}
	}
}

// ----------------------------------------------------------------------------
// Pipeline helpers
// ----------------------------------------------------------------------------

// readSource reads a Go source file and returns its bytes.
func readSource(path string) []byte {
	src, err := os.ReadFile(path)
	if err != nil {
		fatalf("cannot read %s: %v", path, err)
	}
	return src
}

// runLex prints all tokens from the source file.
func runLex(path string) {
	src := readSource(path)
	l := lexer.New(src)
	for _, tok := range l.Tokenize() {
		fmt.Println(tok)
	}
}

// runParse prints the full recursive AST.
func runParse(path string) {
	src := readSource(path)
	l := lexer.New(src)
	p := parser.New(l)
	file, err := p.ParseFile()
	if err != nil || len(p.Errors()) > 0 {
		for _, e := range p.Errors() {
			fmt.Fprintln(os.Stderr, "parse error:", e)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "fatal:", err)
		}
		os.Exit(1)
	}
	dumpNode(file, 0)
}

// runEmit runs the full pipeline through codegen but only dumps the C source.
func runEmit(path string) {
	src := readSource(path)

	// Lex
	l := lexer.New(src)

	// Parse
	p := parser.New(l)
	file, err := p.ParseFile()
	if parseErr(p, err) {
		os.Exit(1)
	}

	// Type check
	tc := typechecker.New()
	errs := tc.Check(file)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "type error:", e)
		}
		os.Exit(1)
	}

	// Emit C
	g := codegen.New()
	fmt.Print(g.DumpC(file))
}

// runBuild runs the full compilation pipeline to produce a native binary.
func runBuild(path, output string) {
	src := readSource(path)

	// Lex
	l := lexer.New(src)

	// Parse
	p := parser.New(l)
	file, err := p.ParseFile()
	if parseErr(p, err) {
		os.Exit(1)
	}

	// Type check
	tc := typechecker.New()
	errs := tc.Check(file)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "type error:", e)
		}
		os.Exit(1)
	}

	// Code generation (emit C + compile via gcc)
	g := codegen.New()
	if err := g.Generate(file, output); err != nil {
		fatalf("codegen failed: %v", err)
	}

	fmt.Printf("built: %s\n", output)
}

func parseErr(p *parser.Parser, err error) bool {
	had := false
	for _, e := range p.Errors() {
		fmt.Fprintln(os.Stderr, "parse error:", e)
		had = true
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal parse error:", err)
		had = true
	}
	return had
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "tinygo: "+format+"\n", args...)
	os.Exit(1)
}

// dumpNode recursively walks any AST node and prints it with indentation.
func dumpNode(node ast.Node, depth int) {
	pad := strings.Repeat("  ", depth)
	fmt.Printf("%s%s\n", pad, node)
	switch n := node.(type) {
	case *ast.File:
		for _, d := range n.Decls {
			dumpNode(d, depth+1)
		}
	case *ast.FuncDecl:
		if n.Body != nil {
			dumpNode(n.Body, depth+1)
		}
	case *ast.VarDeclTop:
		if n.Value != nil {
			dumpNode(n.Value, depth+1)
		}
	case *ast.BlockStmt:
		for _, s := range n.Stmts {
			dumpNode(s, depth+1)
		}
	case *ast.VarDeclStmt:
		if n.Value != nil {
			dumpNode(n.Value, depth+1)
		}
	case *ast.AssignStmt:
		dumpNode(n.Target, depth+1)
		dumpNode(n.Value, depth+1)
	case *ast.IncDecStmt:
		dumpNode(n.Target, depth+1)
	case *ast.IfStmt:
		dumpNode(n.Cond, depth+1)
		if n.Then != nil {
			dumpNode(n.Then, depth+1)
		}
		if n.Else != nil {
			dumpNode(n.Else, depth+1)
		}
	case *ast.ForStmt:
		if n.Init != nil {
			dumpNode(n.Init, depth+1)
		}
		if n.Cond != nil {
			dumpNode(n.Cond, depth+1)
		}
		if n.Post != nil {
			dumpNode(n.Post, depth+1)
		}
		if n.Body != nil {
			dumpNode(n.Body, depth+1)
		}
	case *ast.ReturnStmt:
		if n.Value != nil {
			dumpNode(n.Value, depth+1)
		}
	case *ast.ExprStmt:
		dumpNode(n.Expr, depth+1)
	case *ast.BinaryExpr:
		dumpNode(n.Left, depth+1)
		dumpNode(n.Right, depth+1)
	case *ast.UnaryExpr:
		dumpNode(n.Operand, depth+1)
	case *ast.CallExpr:
		dumpNode(n.Func, depth+1)
		for _, arg := range n.Args {
			dumpNode(arg, depth+1)
		}
	case *ast.SelectorExpr:
		dumpNode(n.X, depth+1)
	// Leaf nodes (Ident, IntLit, StringLit, BoolLit) — already printed above
	}
}
