// Package bytecode provides the AST → bytecode emitter.
//
// Emitter walks a fully type-checked, ownership-analysed AST and
// emits a flat []Instruction (a Program). It does NOT perform any
// semantic analysis — it assumes the AST is already valid.
//
// Control flow (if/while) uses two-pass backpatching:
//
//	emit OpJmpFalse with JmpTo=-1  →  remember index
//	after emitting body, patch JmpTo to current length.
package bytecode

import (
	"fmt"

	"goforust/ast"
)

// ─────────────────────────────────────────────────────────────────────────────
// Emitter
// ─────────────────────────────────────────────────────────────────────────────

// Emitter translates a validated AST into a Program.
type Emitter struct {
	prog   *Program
	fnName string // current function being emitted
}

// NewEmitter creates a fresh Emitter.
func NewEmitter() *Emitter {
	return &Emitter{prog: NewProgram()}
}

// Emit compiles an ast.File and returns the resulting Program.
func (e *Emitter) Emit(file *ast.File) *Program {
	// Two-pass: first pass registers all function entry points,
	// then emits a CALL main + HALT, then all function bodies.

	// Emit bootstrap: CALL main, HALT
	e.emit(Instruction{Op: OpCall, StrArg: "main", IntArg: 0, Comment: "bootstrap → main"})
	e.emit(Instruction{Op: OpHalt, Comment: "end of program"})

	// Emit all function and struct declarations.
	for _, decl := range file.Decls {
		e.emitDecl(decl)
	}

	return e.prog
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

func (e *Emitter) emit(ins Instruction) int {
	idx := len(e.prog.Instructions)
	e.prog.Instructions = append(e.prog.Instructions, ins)
	return idx
}

// patch sets the JmpTo of the instruction at index idx.
func (e *Emitter) patch(idx int, target int) {
	e.prog.Instructions[idx].JmpTo = target
}

// current returns the index of the *next* instruction to be emitted.
func (e *Emitter) current() int {
	return len(e.prog.Instructions)
}

// ─────────────────────────────────────────────────────────────────────────────
// Declarations
// ─────────────────────────────────────────────────────────────────────────────

func (e *Emitter) emitDecl(d ast.Decl) {
	switch decl := d.(type) {
	case *ast.FnDecl:
		e.emitFn(decl)
	case *ast.StructDecl:
		// Structs have no runtime representation in the VM yet.
	case *ast.ImplBlock:
		for _, m := range decl.Methods {
			e.emitFn(m)
		}
	}
}

func (e *Emitter) emitFn(fn *ast.FnDecl) {
	prev := e.fnName
	e.fnName = fn.Name

	// Register this function's entry point.
	entry := e.current()
	e.prog.Functions[fn.Name] = entry

	// Function prologue: store all parameters from the stack.
	// Caller pushed args left-to-right; we store them in param order.
	for i := len(fn.Params) - 1; i >= 0; i-- {
		e.emit(Instruction{Op: OpStore, StrArg: fn.Params[i].Name,
			Comment: fmt.Sprintf("param %s", fn.Params[i].Name)})
	}

	// Emit body.
	if fn.Body != nil {
		e.emitBlock(fn.Body)
	}

	// Implicit return void if no explicit return.
	e.emit(Instruction{Op: OpReturn, Comment: fmt.Sprintf("end of %s", fn.Name)})

	e.fnName = prev
}

// ─────────────────────────────────────────────────────────────────────────────
// Statements
// ─────────────────────────────────────────────────────────────────────────────

func (e *Emitter) emitBlock(block *ast.BlockStmt) {
	if block == nil {
		return
	}
	for _, s := range block.Stmts {
		e.emitStmt(s)
	}
}

func (e *Emitter) emitStmt(s ast.Stmt) {
	switch stmt := s.(type) {
	case *ast.LetDecl:
		e.emitLetDecl(stmt)

	case *ast.AssignStmt:
		e.emitExpr(stmt.Value)
		if stmt.Op != "=" {
			// Compound assignment: e.g. x += 5 → load x, push 5, add, store x
			e.emitLoadTarget(stmt.Target)
			e.emit(Instruction{Op: compoundOp(stmt.Op), Comment: stmt.Op})
		}
		e.emitStoreTarget(stmt.Target)

	case *ast.IncDecStmt:
		e.emitExpr(stmt.Target)
		e.emit(Instruction{Op: OpPushInt, IntArg: 1})
		if stmt.Op == "++" {
			e.emit(Instruction{Op: OpAdd, Comment: "inc"})
		} else {
			e.emit(Instruction{Op: OpSub, Comment: "dec"})
		}
		e.emitStoreTarget(stmt.Target)

	case *ast.ExprStmt:
		if call, isCall := stmt.Expr.(*ast.CallExpr); isCall {
			// Determine if this is a void call (println, or user fn with no return).
			fnName := ""
			if ident, ok := call.Func.(*ast.Ident); ok {
				fnName = ident.Name
			}
			e.emitExpr(stmt.Expr)
			// println emits OpPrintln which pushes nothing — no pop needed.
			// Regular calls push a return value — pop it.
			if fnName != "println" {
				e.emit(Instruction{Op: OpPop, Comment: "discard call result"})
			}
		} else {
			e.emitExpr(stmt.Expr)
			e.emit(Instruction{Op: OpPop, Comment: "discard expr"})
		}

	case *ast.ReturnStmt:
		if stmt.Value != nil {
			e.emitExpr(stmt.Value)
		}
		e.emit(Instruction{Op: OpReturn})

	case *ast.IfStmt:
		e.emitIf(stmt)

	case *ast.WhileStmt:
		e.emitWhile(stmt)

	case *ast.ForStmt:
		e.emitFor(stmt)

	case *ast.BlockStmt:
		e.emitBlock(stmt)

	case *ast.DropStmt:
		// Drop: free the heap value and mark as dropped.
		e.emit(Instruction{Op: OpDrop, StrArg: stmt.VarName,
			Comment: fmt.Sprintf("drop %s", stmt.VarName)})
	}
}

func (e *Emitter) emitLetDecl(stmt *ast.LetDecl) {
	if stmt.Value != nil {
		// If value is a plain identifier (non-borrow), emit a move.
		if ident, ok := stmt.Value.(*ast.Ident); ok {
			e.emit(Instruction{Op: OpLoad, StrArg: ident.Name})
			e.emit(Instruction{Op: OpMove, StrArg: ident.Name,
				Comment: fmt.Sprintf("move %s → %s", ident.Name, stmt.Name)})
		} else {
			e.emitExpr(stmt.Value)
		}
	} else {
		e.emit(Instruction{Op: OpPushInt, IntArg: 0, Comment: "zero init"})
	}
	e.emit(Instruction{Op: OpStore, StrArg: stmt.Name,
		Comment: fmt.Sprintf("let %s", stmt.Name)})
}

func (e *Emitter) emitIf(stmt *ast.IfStmt) {
	// Emit condition.
	e.emitExpr(stmt.Cond)

	// Emit JmpFalse with placeholder.
	jmpFalse := e.emit(Instruction{Op: OpJmpFalse, JmpTo: -1, Comment: "if false → else"})

	// Emit then-block.
	e.emitBlock(stmt.Then)

	if stmt.Else != nil {
		// Jmp over else.
		jmpOver := e.emit(Instruction{Op: OpJmp, JmpTo: -1, Comment: "skip else"})
		// Patch JmpFalse to here (start of else).
		e.patch(jmpFalse, e.current())
		e.emitStmt(stmt.Else)
		// Patch JmpOver to after else.
		e.patch(jmpOver, e.current())
	} else {
		// Patch JmpFalse to after then-block.
		e.patch(jmpFalse, e.current())
	}
}

func (e *Emitter) emitWhile(stmt *ast.WhileStmt) {
	loopTop := e.current()

	// Emit condition.
	e.emitExpr(stmt.Cond)

	// JmpFalse exits loop.
	jmpExit := e.emit(Instruction{Op: OpJmpFalse, JmpTo: -1, Comment: "while exit"})

	// Emit body.
	e.emitBlock(stmt.Body)

	// Unconditional jump back to loop top.
	e.emit(Instruction{Op: OpJmp, JmpTo: loopTop, Comment: "while back-edge"})

	// Patch exit.
	e.patch(jmpExit, e.current())
}

func (e *Emitter) emitFor(stmt *ast.ForStmt) {
	// Init.
	if stmt.Init != nil {
		e.emitStmt(stmt.Init)
	}

	loopTop := e.current()

	// Condition.
	var jmpExit int
	if stmt.Cond != nil {
		e.emitExpr(stmt.Cond)
		jmpExit = e.emit(Instruction{Op: OpJmpFalse, JmpTo: -1, Comment: "for exit"})
	}

	// Body.
	e.emitBlock(stmt.Body)

	// Post.
	if stmt.Post != nil {
		e.emitStmt(stmt.Post)
	}

	e.emit(Instruction{Op: OpJmp, JmpTo: loopTop, Comment: "for back-edge"})

	if stmt.Cond != nil {
		e.patch(jmpExit, e.current())
	}
}

// emitLoadTarget loads the current value of an assignment target.
// Used for compound assignments like x += 5.
func (e *Emitter) emitLoadTarget(target ast.Expr) {
	if ident, ok := target.(*ast.Ident); ok {
		e.emit(Instruction{Op: OpLoad, StrArg: ident.Name})
	}
}

// emitStoreTarget stores TOS into an assignment target.
func (e *Emitter) emitStoreTarget(target ast.Expr) {
	if ident, ok := target.(*ast.Ident); ok {
		e.emit(Instruction{Op: OpStore, StrArg: ident.Name})
	}
}

// compoundOp converts "+=" etc. to the base arithmetic opcode.
func compoundOp(op string) OpCode {
	switch op {
	case "+=":
		return OpAdd
	case "-=":
		return OpSub
	case "*=":
		return OpMul
	case "/=":
		return OpDiv
	}
	return OpAdd
}

// ─────────────────────────────────────────────────────────────────────────────
// Expressions
// ─────────────────────────────────────────────────────────────────────────────

func (e *Emitter) emitExpr(expr ast.Expr) {
	if expr == nil {
		return
	}
	switch ex := expr.(type) {
	case *ast.IntLit:
		e.emit(Instruction{Op: OpPushInt, IntArg: ex.Value})

	case *ast.StringLit:
		e.emit(Instruction{Op: OpPushStr, StrArg: ex.Value})

	case *ast.BoolLit:
		e.emit(Instruction{Op: OpPushBool, BoolArg: ex.Value})

	case *ast.Ident:
		e.emit(Instruction{Op: OpLoad, StrArg: ex.Name})

	case *ast.BorrowExpr:
		if ident, ok := ex.Operand.(*ast.Ident); ok {
			op := OpBorrow
			if ex.Mode == ast.MutBorrow {
				op = OpBorrowMut
			}
			e.emit(Instruction{Op: op, StrArg: ident.Name})
		} else {
			e.emitExpr(ex.Operand)
		}

	case *ast.MoveExpr:
		if ident, ok := ex.Operand.(*ast.Ident); ok {
			e.emit(Instruction{Op: OpLoad, StrArg: ident.Name})
			e.emit(Instruction{Op: OpMove, StrArg: ident.Name, Comment: "explicit move"})
		} else {
			e.emitExpr(ex.Operand)
		}

	case *ast.UnaryExpr:
		e.emitExpr(ex.Operand)
		switch ex.Op {
		case "-":
			e.emit(Instruction{Op: OpNeg})
		case "!":
			e.emit(Instruction{Op: OpNot})
		}

	case *ast.BinaryExpr:
		e.emitExpr(ex.Left)
		e.emitExpr(ex.Right)
		e.emit(Instruction{Op: binaryOp(ex.Op), Comment: ex.Op})

	case *ast.CallExpr:
		e.emitCall(ex)

	case *ast.SelectorExpr:
		// For now treat pkg::fn or obj.field as a simple load by concatenated name.
		e.emit(Instruction{Op: OpLoad, StrArg: selectorName(ex)})

	case *ast.StructLit:
		// Struct literals emit each field value in order.
		for _, f := range ex.Fields {
			e.emitExpr(f.Value)
		}
	}
}

func (e *Emitter) emitCall(c *ast.CallExpr) {
	fnName := ""
	argCount := int64(len(c.Args))

	switch fn := c.Func.(type) {
	case *ast.Ident:
		fnName = fn.Name
	case *ast.SelectorExpr:
		fnName = selectorName(fn)
	}

	// Built-in: println
	if fnName == "println" {
		for _, arg := range c.Args {
			e.emitExpr(arg)
		}
		e.emit(Instruction{Op: OpPrintln, IntArg: argCount, Comment: "println"})
		return
	}

	// Regular call: push args left-to-right, then CALL.
	for _, arg := range c.Args {
		e.emitExpr(arg)
	}
	e.emit(Instruction{Op: OpCall, StrArg: fnName, IntArg: argCount})
}

func binaryOp(op string) OpCode {
	switch op {
	case "+":
		return OpAdd
	case "-":
		return OpSub
	case "*":
		return OpMul
	case "/":
		return OpDiv
	case "%":
		return OpMod
	case "==":
		return OpEq
	case "!=":
		return OpNeq
	case "<":
		return OpLt
	case ">":
		return OpGt
	case "<=":
		return OpLeq
	case ">=":
		return OpGeq
	case "&&":
		return OpAnd
	case "||":
		return OpOr
	}
	return OpNop
}

func selectorName(s *ast.SelectorExpr) string {
	if ident, ok := s.X.(*ast.Ident); ok {
		return ident.Name + s.Sep + s.Sel
	}
	return s.Sel
}
