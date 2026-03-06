// go-for-rust: Ore language compiler — serious compiler engineering edition.
//
// Pipeline:
//
//	Source → Lexer → AST → sema.Analyze (ownership+borrow+lifetime) → bytecode.Emit → vm.Run
//
// Commands:
//
//	run    <file.ore>           ← PRIMARY: compile → bytecode → VM execution
//	dis    <file.ore>           ← disassemble bytecode (human-readable)
//	check  <file.ore>           ← static analysis only (all 3 passes), Rustc-style errors
//	emit-c <file.ore>           ← old C transpiler (kept for reference)
//	lex    <file.ore>           ← dump tokens
//	parse  <file.ore>           ← dump AST
//	version
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"goforust/ast"
	"goforust/bytecode"
	"goforust/codegen"
	"goforust/lexer"
	"goforust/parser"
	"goforust/sema"
	"goforust/vm"
)

const version = "0.2.0"

func usage() {
	fmt.Fprintf(os.Stderr, `goforust — Ore language compiler v%s (Rust-inspired, written in Go)

Commands:
  run    <file.ore>   ← compile to bytecode and run in the Ore VM  [primary]
  dis    <file.ore>   ← show disassembled bytecode instructions
  check  <file.ore>   ← ownership / borrow / lifetime analysis only
  emit-c <file.ore>   ← dump generated C (transpiler path, for reference)
  lex    <file.ore>   ← dump tokenizer output
  parse  <file.ore>   ← dump parsed AST
  version

`, version)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("goforust %s\n", version)

	case "lex":
		needArg("lex")
		cmdLex(os.Args[2])

	case "parse":
		needArg("parse")
		cmdParse(os.Args[2])

	case "check":
		needArg("check")
		cmdCheck(os.Args[2])

	case "run":
		needArg("run")
		cmdRun(os.Args[2])

	case "dis":
		needArg("dis")
		cmdDis(os.Args[2])

	case "emit-c":
		needArg("emit-c")
		cmdEmitC(os.Args[2])

	// kept for backward compat — aliases run
	case "build":
		args := os.Args[2:]
		out := "a.out"
		if len(args) >= 2 && args[0] == "-o" {
			out = args[1]
			args = args[2:]
		}
		if len(args) == 0 {
			die("build: missing <file.ore>")
		}
		cmdBuild(args[0], out)

	default:
		usage()
		os.Exit(1)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Command implementations
// ─────────────────────────────────────────────────────────────────────────────

func cmdLex(path string) {
	l := lexer.New(readSrc(path))
	for _, tok := range l.Tokenize() {
		fmt.Println(tok)
	}
}

func cmdParse(path string) {
	file := mustParse(path, readSrc(path))
	fmt.Println(file)
	for _, d := range file.Decls {
		fmt.Println(" ", d)
	}
}

func cmdCheck(path string) {
	src := readSrc(path)
	file := mustParse(path, src)
	result := sema.Analyze(file, src, path)

	// Print all diagnostics with color + source context.
	fmt.Print(result.PrintAll(isTerminal()))

	if result.HasErrors() {
		os.Exit(1)
	}
	fmt.Printf("\x1b[32m✓\x1b[0m All checks passed (%s)\n", filepath.Base(path))
}

func cmdRun(path string) {
	src := readSrc(path)
	file := mustParse(path, src)

	// Semantic analysis.
	result := sema.Analyze(file, src, path)
	if result.HasErrors() {
		fmt.Print(result.PrintAll(isTerminal()))
		os.Exit(1)
	}

	// Emit bytecode.
	emitter := bytecode.NewEmitter()
	prog := emitter.Emit(file)

	// Run on the Ore VM.
	machine := vm.New(prog)
	if err := machine.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func cmdDis(path string) {
	src := readSrc(path)
	file := mustParse(path, src)

	result := sema.Analyze(file, src, path)
	if result.HasErrors() {
		fmt.Print(result.PrintAll(isTerminal()))
		os.Exit(1)
	}

	emitter := bytecode.NewEmitter()
	prog := emitter.Emit(file)
	fmt.Print(bytecode.Disassemble(prog))
}

func cmdEmitC(path string) {
	src := readSrc(path)
	file := mustParse(path, src)

	result := sema.Analyze(file, src, path)
	if result.HasErrors() {
		fmt.Print(result.PrintAll(isTerminal()))
		os.Exit(1)
	}

	g := codegen.New()
	fmt.Print(g.DumpC(file))
}

func cmdBuild(path, out string) {
	src := readSrc(path)
	file := mustParse(path, src)

	result := sema.Analyze(file, src, path)
	if result.HasErrors() {
		fmt.Print(result.PrintAll(isTerminal()))
		os.Exit(1)
	}

	g := codegen.New()
	if err := g.Generate(file, out); err != nil {
		die("codegen: %v", err)
	}
	fmt.Printf("built (C backend): %s\n", out)
}

// ─────────────────────────────────────────────────────────────────────────────
// Shared pipeline stages
// ─────────────────────────────────────────────────────────────────────────────

func readSrc(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		die("cannot read %s: %v", path, err)
	}
	return b
}

// mustParse lexes + parses an Ore file. Exits on parse errors.
func mustParse(path string, src []byte) *ast.File {
	l := lexer.New(src)
	p := parser.New(l)
	file, err := p.ParseFile()
	hadErr := false
	for _, e := range p.Errors() {
		fmt.Fprintf(os.Stderr, "parse error: %s\n", e)
		hadErr = true
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse error: %v\n", err)
		hadErr = true
	}
	if hadErr {
		os.Exit(1)
	}
	return file
}

// isTerminal returns true when stdout appears to be a real TTY (enables color).
func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// ─────────────────────────────────────────────────────────────────────────────
// Misc
// ─────────────────────────────────────────────────────────────────────────────

func needArg(cmd string) {
	if len(os.Args) < 3 {
		die("%s: missing <file.ore>", cmd)
	}
}

func die(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "goforust: "+format+"\n", a...)
	os.Exit(1)
}
