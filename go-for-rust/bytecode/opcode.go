// Package bytecode defines the Ore bytecode instruction set.
//
// The Ore VM is a simple stack machine. All values are pushed onto
// the operand stack; operations pop their inputs and push results.
//
// Ownership-aware opcodes:
//
//	OpMove    — mark source variable as Moved in the VM locals table
//	OpBorrow  — create a reference in the operand stack (no ownership change)
//	OpDrop    — call destructor for a variable (free heap memory)
//
// Control flow uses backpatching:
//
//	OpJmpFalse / OpJmp carry a JmpTo index into the instruction slice.
//	The emitter sets JmpTo = -1 as a placeholder, then patches it.
package bytecode

import (
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// OpCode definitions
// ─────────────────────────────────────────────────────────────────────────────

// OpCode is one byte identifying an instruction.
type OpCode uint8

const (
	// ── Stack / Literals ───────────────────────────────────────────────────
	OpNop      OpCode = iota // no-op
	OpPushInt                // push IntArg onto stack
	OpPushStr                // push StrArg onto stack
	OpPushBool               // push BoolArg onto stack

	// ── Variables ──────────────────────────────────────────────────────────
	OpLoad  // push copy of local[StrArg] onto stack
	OpStore // pop TOS → local[StrArg]
	OpPop   // discard top of stack

	// ── Arithmetic ─────────────────────────────────────────────────────────
	OpAdd // pop (r, l) push l+r  (Int)
	OpSub // pop (r, l) push l-r
	OpMul // pop (r, l) push l*r
	OpDiv // pop (r, l) push l/r
	OpMod // pop (r, l) push l%r
	OpNeg // pop x, push -x (unary)

	// ── Comparison ─────────────────────────────────────────────────────────
	OpEq  // pop (r, l) push l==r (Bool)
	OpNeq // pop (r, l) push l!=r
	OpLt  // pop (r, l) push l<r
	OpGt  // pop (r, l) push l>r
	OpLeq // pop (r, l) push l<=r
	OpGeq // pop (r, l) push l>=r

	// ── Logic ──────────────────────────────────────────────────────────────
	OpAnd // pop (r, l) push l&&r (Bool)
	OpOr  // pop (r, l) push l||r
	OpNot // pop x, push !x

	// ── Control flow ───────────────────────────────────────────────────────
	OpJmp      // unconditional jump to JmpTo
	OpJmpFalse // pop Bool; jump to JmpTo if false

	// ── Functions ──────────────────────────────────────────────────────────
	OpCall   // call function StrArg with IntArg args on stack
	OpReturn // return TOS (or unit if stack empty)

	// ── Built-ins ──────────────────────────────────────────────────────────
	OpPrintln // pop N values (count=IntArg), print with newline

	// ── Ownership / Borrow ─────────────────────────────────────────────────
	OpMove      // StrArg=varName: mark locals[StrArg] as Moved (ownership transfer)
	OpBorrow    // StrArg=varName: push a reference to locals[StrArg] (shared)
	OpBorrowMut // StrArg=varName: push a mutable reference to locals[StrArg]
	OpDrop      // StrArg=varName: free heap memory if needed, mark Dropped

	// ── VM control ─────────────────────────────────────────────────────────
	OpHalt // stop execution
)

// opNames maps each OpCode to a human-readable mnemonic.
var opNames = map[OpCode]string{
	OpNop:       "NOP",
	OpPushInt:   "PUSH_INT",
	OpPushStr:   "PUSH_STR",
	OpPushBool:  "PUSH_BOOL",
	OpLoad:      "LOAD",
	OpStore:     "STORE",
	OpPop:       "POP",
	OpAdd:       "ADD",
	OpSub:       "SUB",
	OpMul:       "MUL",
	OpDiv:       "DIV",
	OpMod:       "MOD",
	OpNeg:       "NEG",
	OpEq:        "EQ",
	OpNeq:       "NEQ",
	OpLt:        "LT",
	OpGt:        "GT",
	OpLeq:       "LEQ",
	OpGeq:       "GEQ",
	OpAnd:       "AND",
	OpOr:        "OR",
	OpNot:       "NOT",
	OpJmp:       "JMP",
	OpJmpFalse:  "JMP_FALSE",
	OpCall:      "CALL",
	OpReturn:    "RETURN",
	OpPrintln:   "PRINTLN",
	OpMove:      "MOVE",
	OpBorrow:    "BORROW",
	OpBorrowMut: "BORROW_MUT",
	OpDrop:      "DROP",
	OpHalt:      "HALT",
}

func (op OpCode) String() string {
	if s, ok := opNames[op]; ok {
		return s
	}
	return fmt.Sprintf("OP(%d)", int(op))
}

// ─────────────────────────────────────────────────────────────────────────────
// Instruction
// ─────────────────────────────────────────────────────────────────────────────

// Instruction is a single bytecode instruction.
type Instruction struct {
	Op      OpCode
	IntArg  int64  // integer argument (literal value, arg count, jump offset)
	StrArg  string // string argument (variable name, function name, string literal)
	BoolArg bool   // boolean argument
	JmpTo   int    // jump target index; -1 = unresolved (backpatch pending)
	Comment string // optional human-readable note (for disassembly)
}

// ─────────────────────────────────────────────────────────────────────────────
// Program
// ─────────────────────────────────────────────────────────────────────────────

// Program is the compiled representation of an Ore file.
type Program struct {
	Instructions []Instruction
	// Functions maps function name → entry instruction index.
	Functions map[string]int
}

// NewProgram creates an empty Program.
func NewProgram() *Program {
	return &Program{Functions: make(map[string]int)}
}

// ─────────────────────────────────────────────────────────────────────────────
// Disassembly
// ─────────────────────────────────────────────────────────────────────────────

// Disassemble returns a human-readable listing of a Program.
func Disassemble(prog *Program) string {
	var sb strings.Builder

	// Print function index for reference.
	if len(prog.Functions) > 0 {
		sb.WriteString("=== Functions ===\n")
		for name, idx := range prog.Functions {
			sb.WriteString(fmt.Sprintf("  %-20s → #%d\n", name, idx))
		}
		sb.WriteByte('\n')
	}

	sb.WriteString("=== Instructions ===\n")
	for i, ins := range prog.Instructions {
		sb.WriteString(DisassembleOne(i, ins))
	}
	return sb.String()
}

// DisassembleOne formats a single instruction.
func DisassembleOne(idx int, ins Instruction) string {
	line := fmt.Sprintf("%4d │ %-14s", idx, ins.Op)

	switch ins.Op {
	case OpPushInt:
		line += fmt.Sprintf(" %d", ins.IntArg)
	case OpPushStr:
		line += fmt.Sprintf(" %q", ins.StrArg)
	case OpPushBool:
		if ins.BoolArg {
			line += " true"
		} else {
			line += " false"
		}
	case OpLoad, OpStore, OpMove, OpBorrow, OpBorrowMut, OpDrop:
		line += fmt.Sprintf(" %s", ins.StrArg)
	case OpCall:
		line += fmt.Sprintf(" %s (%d args)", ins.StrArg, ins.IntArg)
	case OpJmp, OpJmpFalse:
		target := ins.JmpTo
		line += fmt.Sprintf(" →%d", target)
	case OpPrintln:
		line += fmt.Sprintf(" (n=%d)", ins.IntArg)
	}

	if ins.Comment != "" {
		line += fmt.Sprintf("   ; %s", ins.Comment)
	}

	return line + "\n"
}
