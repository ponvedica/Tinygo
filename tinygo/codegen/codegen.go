// Package codegen implements Go-to-C code generation for the tinygo compiler.
// It walks the type-checked AST and emits valid C source code, then shells
// out to gcc to produce a native executable.
package codegen

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"tinygo/ast"
)

// Generator holds code generation state.
type Generator struct {
	buf     strings.Builder
	indent  int
	imports []string
}

// New creates a new Generator.
func New() *Generator {
	return &Generator{}
}

// Generate walks the AST, emits C, writes to a temp file, and compiles with gcc.
// outputPath is the desired output binary path.
func (g *Generator) Generate(file *ast.File, outputPath string) error {
	// Preamble
	g.emit("#include <stdio.h>\n")
	g.emit("#include <stdlib.h>\n")
	g.emit("#include <string.h>\n")
	g.emit("\n")

	// Type aliases
	g.emit("typedef long      _goInt;\n")
	g.emit("typedef char*     _goString;\n")
	g.emit("typedef int       _goBool;\n")
	g.emit("#define true  1\n")
	g.emit("#define false 0\n")
	g.emit("\n")

	// Forward-declare all functions
	for _, d := range file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok {
			g.emitFuncSignature(fn)
			g.emit(";\n")
		}
	}
	g.emit("\n")

	// Generate declarations
	for _, d := range file.Decls {
		g.genDecl(d)
		g.emit("\n")
	}

	// Write C source to temp file
	tmpFile, err := os.CreateTemp("", "tinygo_*.c")
	if err != nil {
		return fmt.Errorf("failed to create temp C file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(g.buf.String()); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write C source: %w", err)
	}
	tmpFile.Close()

	// Compile with gcc
	cmd := exec.Command("gcc", "-o", outputPath, tmpPath, "-lm")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Also dump the generated C for debugging
		fmt.Fprintf(os.Stderr, "\n--- Generated C ---\n%s\n---\n", g.buf.String())
		return fmt.Errorf("gcc compilation failed: %w", err)
	}
	return nil
}

// DumpC returns the generated C source without compiling.
func (g *Generator) DumpC(file *ast.File) string {
	g.buf.Reset()
	g.emit("#include <stdio.h>\n")
	g.emit("#include <stdlib.h>\n")
	g.emit("#include <string.h>\n\n")
	g.emit("typedef long      _goInt;\n")
	g.emit("typedef char*     _goString;\n")
	g.emit("typedef int       _goBool;\n")
	g.emit("#define true  1\n")
	g.emit("#define false 0\n\n")
	for _, d := range file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok {
			g.emitFuncSignature(fn)
			g.emit(";\n")
		}
	}
	g.emit("\n")
	for _, d := range file.Decls {
		g.genDecl(d)
		g.emit("\n")
	}
	return g.buf.String()
}

func (g *Generator) emit(s string) {
	g.buf.WriteString(s)
}

func (g *Generator) emitIndent() {
	for i := 0; i < g.indent; i++ {
		g.buf.WriteString("    ")
	}
}

func (g *Generator) emitLine(s string) {
	g.emitIndent()
	g.buf.WriteString(s)
	g.buf.WriteByte('\n')
}

// ----------------------------------------------------------------------------
// Declarations
// ----------------------------------------------------------------------------

func (g *Generator) genDecl(d ast.Decl) {
	switch decl := d.(type) {
	case *ast.FuncDecl:
		g.genFuncDecl(decl)
	case *ast.VarDeclTop:
		cType := goTypeToCType(decl.Type)
		if decl.Value != nil {
			g.emit(fmt.Sprintf("%s %s = %s;\n", cType, decl.Name, g.genExpr(decl.Value)))
		} else {
			g.emit(fmt.Sprintf("%s %s;\n", cType, decl.Name))
		}
	}
}

func (g *Generator) emitFuncSignature(fn *ast.FuncDecl) {
	retType := "void"
	if fn.ReturnType != "" {
		retType = goTypeToCType(fn.ReturnType)
	}
	// Use main as-is for interoperability
	name := fn.Name
	if name == "main" {
		retType = "int"
	}

	var params []string
	for _, p := range fn.Params {
		params = append(params, fmt.Sprintf("%s %s", goTypeToCType(p.Type), p.Name))
	}
	paramStr := strings.Join(params, ", ")
	if paramStr == "" {
		paramStr = "void"
	}
	g.emit(fmt.Sprintf("%s %s(%s)", retType, name, paramStr))
}

func (g *Generator) genFuncDecl(fn *ast.FuncDecl) {
	g.emitFuncSignature(fn)
	g.emit(" {\n")
	g.indent++
	g.genBlock(fn.Body)
	if fn.Name == "main" {
		g.emitLine("return 0;")
	}
	g.indent--
	g.emit("}\n")
}

// ----------------------------------------------------------------------------
// Statements
// ----------------------------------------------------------------------------

func (g *Generator) genBlock(block *ast.BlockStmt) {
	if block == nil {
		return
	}
	for _, s := range block.Stmts {
		g.genStmt(s)
	}
}

func (g *Generator) genStmt(s ast.Stmt) {
	switch stmt := s.(type) {
	case *ast.VarDeclStmt:
		cType := "long" // default int
		if stmt.Type != "" {
			cType = goTypeToCType(stmt.Type)
		} else if stmt.Value != nil {
			cType = g.inferCType(stmt.Value)
		}
		if stmt.Value != nil {
			g.emitLine(fmt.Sprintf("%s %s = %s;", cType, stmt.Name, g.genExpr(stmt.Value)))
		} else {
			g.emitLine(fmt.Sprintf("%s %s;", cType, stmt.Name))
		}

	case *ast.AssignStmt:
		g.emitLine(fmt.Sprintf("%s %s %s;", g.genExpr(stmt.Target), stmt.Op, g.genExpr(stmt.Value)))

	case *ast.IncDecStmt:
		g.emitLine(fmt.Sprintf("%s%s;", g.genExpr(stmt.Target), stmt.Op))

	case *ast.ExprStmt:
		g.emitLine(fmt.Sprintf("%s;", g.genExpr(stmt.Expr)))

	case *ast.ReturnStmt:
		if stmt.Value != nil {
			g.emitLine(fmt.Sprintf("return %s;", g.genExpr(stmt.Value)))
		} else {
			g.emitLine("return;")
		}

	case *ast.IfStmt:
		g.emitIndent()
		g.emit(fmt.Sprintf("if (%s) {\n", g.genExpr(stmt.Cond)))
		g.indent++
		g.genBlock(stmt.Then)
		g.indent--
		if stmt.Else != nil {
			switch e := stmt.Else.(type) {
			case *ast.BlockStmt:
				g.emitLine("} else {")
				g.indent++
				g.genBlock(e)
				g.indent--
				g.emitLine("}")
			case *ast.IfStmt:
				g.emitIndent()
				g.emit("} else ")
				// Re-generate the else-if inline
				g.emitElseIf(e)
			}
		} else {
			g.emitLine("}")
		}

	case *ast.ForStmt:
		if stmt.Init == nil && stmt.Cond == nil && stmt.Post == nil {
			// infinite loop
			g.emitLine("while (1) {")
		} else if stmt.Init == nil && stmt.Post == nil {
			// while loop
			g.emitIndent()
			g.emit(fmt.Sprintf("while (%s) {\n", g.genExpr(stmt.Cond)))
		} else {
			// C-style for
			initStr := g.genSimpleStmtInline(stmt.Init)
			condStr := ""
			if stmt.Cond != nil {
				condStr = g.genExpr(stmt.Cond)
			}
			postStr := g.genSimpleStmtInline(stmt.Post)
			g.emitIndent()
			g.emit(fmt.Sprintf("for (%s; %s; %s) {\n", initStr, condStr, postStr))
		}
		g.indent++
		g.genBlock(stmt.Body)
		g.indent--
		g.emitLine("}")

	case *ast.BlockStmt:
		g.emitLine("{")
		g.indent++
		g.genBlock(stmt)
		g.indent--
		g.emitLine("}")
	}
}

func (g *Generator) emitElseIf(stmt *ast.IfStmt) {
	g.emit(fmt.Sprintf("if (%s) {\n", g.genExpr(stmt.Cond)))
	g.indent++
	g.genBlock(stmt.Then)
	g.indent--
	if stmt.Else != nil {
		switch e := stmt.Else.(type) {
		case *ast.BlockStmt:
			g.emitLine("} else {")
			g.indent++
			g.genBlock(e)
			g.indent--
			g.emitLine("}")
		case *ast.IfStmt:
			g.emitIndent()
			g.emit("} else ")
			g.emitElseIf(e)
		}
	} else {
		g.emitLine("}")
	}
}

// genSimpleStmtInline generates a statement as a single-line C expression (for for-loop header).
func (g *Generator) genSimpleStmtInline(s ast.Stmt) string {
	if s == nil {
		return ""
	}
	switch stmt := s.(type) {
	case *ast.VarDeclStmt:
		cType := "long"
		if stmt.Type != "" {
			cType = goTypeToCType(stmt.Type)
		} else if stmt.Value != nil {
			cType = g.inferCType(stmt.Value)
		}
		if stmt.Value != nil {
			return fmt.Sprintf("%s %s = %s", cType, stmt.Name, g.genExpr(stmt.Value))
		}
		return fmt.Sprintf("%s %s", cType, stmt.Name)
	case *ast.AssignStmt:
		return fmt.Sprintf("%s %s %s", g.genExpr(stmt.Target), stmt.Op, g.genExpr(stmt.Value))
	case *ast.IncDecStmt:
		return fmt.Sprintf("%s%s", g.genExpr(stmt.Target), stmt.Op)
	case *ast.ExprStmt:
		return g.genExpr(stmt.Expr)
	}
	return ""
}

// ----------------------------------------------------------------------------
// Expressions
// ----------------------------------------------------------------------------

func (g *Generator) genExpr(e ast.Expr) string {
	if e == nil {
		return ""
	}
	switch expr := e.(type) {
	case *ast.IntLit:
		return fmt.Sprintf("%dL", expr.Value)
	case *ast.StringLit:
		return fmt.Sprintf("%q", expr.Value)
	case *ast.BoolLit:
		if expr.Value {
			return "1"
		}
		return "0"
	case *ast.Ident:
		return expr.Name
	case *ast.BinaryExpr:
		return fmt.Sprintf("(%s %s %s)", g.genExpr(expr.Left), expr.Op, g.genExpr(expr.Right))
	case *ast.UnaryExpr:
		return fmt.Sprintf("(%s%s)", expr.Op, g.genExpr(expr.Operand))
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", g.genExpr(expr.X), expr.Sel)
	case *ast.CallExpr:
		return g.genCall(expr)
	}
	return "/* unknown */"
}

// genCall maps Go calls (including fmt.XX) to C equivalents.
func (g *Generator) genCall(c *ast.CallExpr) string {
	// Detect fmt.Println, fmt.Printf, etc.
	if sel, ok := c.Func.(*ast.SelectorExpr); ok {
		if ident, ok2 := sel.X.(*ast.Ident); ok2 && ident.Name == "fmt" {
			switch sel.Sel {
			case "Println":
				return g.genFmtPrintln(c.Args, true)
			case "Print":
				return g.genFmtPrintln(c.Args, false)
			case "Printf":
				return g.genFmtPrintf(c.Args)
			}
		}
	}

	// Built-in functions
	if ident, ok := c.Func.(*ast.Ident); ok {
		switch ident.Name {
		case "println":
			return g.genFmtPrintln(c.Args, true)
		case "print":
			return g.genFmtPrintln(c.Args, false)
		case "len":
			if len(c.Args) == 1 {
				return fmt.Sprintf("strlen(%s)", g.genExpr(c.Args[0]))
			}
		}
	}

	// Regular function call
	fnName := g.genExpr(c.Func)
	var args []string
	for _, a := range c.Args {
		args = append(args, g.genExpr(a))
	}
	return fmt.Sprintf("%s(%s)", fnName, strings.Join(args, ", "))
}

// genFmtPrintln generates printf calls for fmt.Println/fmt.Print.
func (g *Generator) genFmtPrintln(args []ast.Expr, newline bool) string {
	if len(args) == 0 {
		if newline {
			return `printf("\n")`
		}
		return `printf("")`
	}

	// Build a combined format string
	var fmtParts []string
	var cArgs []string

	for _, a := range args {
		switch arg := a.(type) {
		case *ast.StringLit:
			fmtParts = append(fmtParts, "%s")
			cArgs = append(cArgs, fmt.Sprintf("%q", arg.Value))
		case *ast.IntLit:
			fmtParts = append(fmtParts, "%ld")
			cArgs = append(cArgs, fmt.Sprintf("%dL", arg.Value))
		case *ast.BoolLit:
			fmtParts = append(fmtParts, "%d")
			if arg.Value {
				cArgs = append(cArgs, "1")
			} else {
				cArgs = append(cArgs, "0")
			}
		default:
			// Runtime type — we emit %ld (int) as default; strings use %s
			fmtParts = append(fmtParts, "%ld")
			cArgs = append(cArgs, g.genExpr(a))
		}
	}

	fmtStr := strings.Join(fmtParts, " ")
	if newline {
		fmtStr += "\\n"
	}

	allArgs := append([]string{fmt.Sprintf("%q", fmtStr)}, cArgs...)
	return fmt.Sprintf("printf(%s)", strings.Join(allArgs, ", "))
}

func (g *Generator) genFmtPrintf(args []ast.Expr) string {
	if len(args) == 0 {
		return `printf("")`
	}
	var cArgs []string
	for _, a := range args {
		cArgs = append(cArgs, g.genExpr(a))
	}
	return fmt.Sprintf("printf(%s)", strings.Join(cArgs, ", "))
}

// ----------------------------------------------------------------------------
// Type mapping
// ----------------------------------------------------------------------------

func goTypeToCType(t string) string {
	switch t {
	case "int", "int64", "int32":
		return "_goInt"
	case "string":
		return "_goString"
	case "bool":
		return "_goBool"
	case "void", "":
		return "void"
	}
	return "_goInt" // fallback
}

func (g *Generator) inferCType(e ast.Expr) string {
	switch e.(type) {
	case *ast.StringLit:
		return "_goString"
	case *ast.BoolLit:
		return "_goBool"
	case *ast.IntLit:
		return "_goInt"
	}
	return "_goInt"
}
