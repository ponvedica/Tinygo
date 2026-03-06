// Package vm implements the Ore stack-based virtual machine.
//
// Architecture:
//   - Operand stack   []Value
//   - Local variables map[string]Value  (per call frame)
//   - Call frame stack []Frame
//   - Instruction pointer (ip) into Program.Instructions
//
// Ownership safety at runtime:
//
//	OpMove  → sets locals[name].Kind = KindMoved; any subsequent OpLoad → error
//	OpDrop  → sets locals[name].Kind = KindDropped; any subsequent OpLoad → error
//	OpBorrow → pushes a Ref value that records which variable it points to
//
// The VM does NOT re-run borrow/ownership static checks — those run at compile
// time. Runtime ownership enforcement is a second line of defence for bugs in
// the emitter.
package vm

import (
	"fmt"
	"os"
	"strings"

	"goforust/bytecode"
)

// ─────────────────────────────────────────────────────────────────────────────
// Value
// ─────────────────────────────────────────────────────────────────────────────

// ValueKind classifies what a Value holds.
type ValueKind int

const (
	KindInt ValueKind = iota
	KindStr           // heap-allocated string
	KindBool
	KindRef     // shared reference: points to a local name in a frame
	KindMutRef  // mutable reference
	KindMoved   // tombstone: value was moved away
	KindDropped // tombstone: value was dropped/freed
	KindUnit    // unit/void return
)

// Value is the runtime representation of an Ore value.
type Value struct {
	Kind ValueKind
	IVal int64
	SVal string
	BVal bool
	// For KindRef/KindMutRef: the variable name in the enclosing frame.
	RefTo string
}

var unit = Value{Kind: KindUnit}

func (v Value) String() string {
	switch v.Kind {
	case KindInt:
		return fmt.Sprintf("%d", v.IVal)
	case KindStr:
		return v.SVal
	case KindBool:
		if v.BVal {
			return "true"
		}
		return "false"
	case KindRef:
		return fmt.Sprintf("&%s", v.RefTo)
	case KindMutRef:
		return fmt.Sprintf("&mut %s", v.RefTo)
	case KindMoved:
		return "<moved>"
	case KindDropped:
		return "<dropped>"
	case KindUnit:
		return "()"
	}
	return "?"
}

// ─────────────────────────────────────────────────────────────────────────────
// Call frame
// ─────────────────────────────────────────────────────────────────────────────

// Frame is one activation record on the call stack.
type Frame struct {
	FnName  string
	Locals  map[string]Value
	RetAddr int // instruction index to return to
}

func newFrame(fnName string, retAddr int) *Frame {
	return &Frame{FnName: fnName, Locals: make(map[string]Value), RetAddr: retAddr}
}

// ─────────────────────────────────────────────────────────────────────────────
// VM
// ─────────────────────────────────────────────────────────────────────────────

// VM executes an Ore bytecode Program.
type VM struct {
	prog      *bytecode.Program
	stack     []Value
	callStack []*Frame
	ip        int
	maxStack  int // high-watermark for diagnostics
}

// New creates a VM ready to run prog.
func New(prog *bytecode.Program) *VM {
	return &VM{prog: prog}
}

// Run executes the program starting at instruction 0.
// It returns a non-nil error if execution fails.
func (vm *VM) Run() error {
	// Push a synthetic top-level frame.
	vm.callStack = append(vm.callStack, newFrame("<main>", -1))
	vm.ip = 0

	for vm.ip < len(vm.prog.Instructions) {
		ins := vm.prog.Instructions[vm.ip]
		vm.ip++

		// Track stack depth.
		if len(vm.stack) > vm.maxStack {
			vm.maxStack = len(vm.stack)
		}

		if err := vm.exec(ins); err != nil {
			return err
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Dispatcher
// ─────────────────────────────────────────────────────────────────────────────

func (vm *VM) exec(ins bytecode.Instruction) error {
	switch ins.Op {
	case bytecode.OpNop:
		// nothing

	// ── Stack ──────────────────────────────────────────────────────────────
	case bytecode.OpPushInt:
		vm.push(Value{Kind: KindInt, IVal: ins.IntArg})

	case bytecode.OpPushStr:
		vm.push(Value{Kind: KindStr, SVal: ins.StrArg})

	case bytecode.OpPushBool:
		vm.push(Value{Kind: KindBool, BVal: ins.BoolArg})

	case bytecode.OpPop:
		vm.pop()

	// ── Variables ───────────────────────────────────────────────────────────
	case bytecode.OpLoad:
		v, err := vm.loadVar(ins.StrArg)
		if err != nil {
			return vm.runtimeErr(ins, err.Error())
		}
		vm.push(v)

	case bytecode.OpStore:
		v := vm.pop()
		vm.setVar(ins.StrArg, v)

	// ── Ownership ───────────────────────────────────────────────────────────
	case bytecode.OpMove:
		// Mark source as Moved — future loads will error.
		vm.setVar(ins.StrArg, Value{Kind: KindMoved, SVal: ins.StrArg})

	case bytecode.OpBorrow:
		vm.push(Value{Kind: KindRef, RefTo: ins.StrArg})

	case bytecode.OpBorrowMut:
		vm.push(Value{Kind: KindMutRef, RefTo: ins.StrArg})

	case bytecode.OpDrop:
		// Free the value and mark as Dropped.
		// (In this VM strings are GC-managed by Go, so no actual free needed.)
		vm.setVar(ins.StrArg, Value{Kind: KindDropped, SVal: ins.StrArg})

	// ── Arithmetic ──────────────────────────────────────────────────────────
	case bytecode.OpAdd:
		r, l := vm.pop(), vm.pop()
		if l.Kind == KindStr || r.Kind == KindStr {
			vm.push(Value{Kind: KindStr, SVal: l.SVal + r.SVal})
		} else {
			vm.push(Value{Kind: KindInt, IVal: l.IVal + r.IVal})
		}

	case bytecode.OpSub:
		r, l := vm.pop(), vm.pop()
		vm.push(Value{Kind: KindInt, IVal: l.IVal - r.IVal})

	case bytecode.OpMul:
		r, l := vm.pop(), vm.pop()
		vm.push(Value{Kind: KindInt, IVal: l.IVal * r.IVal})

	case bytecode.OpDiv:
		r, l := vm.pop(), vm.pop()
		if r.IVal == 0 {
			return vm.runtimeErr(ins, "division by zero")
		}
		vm.push(Value{Kind: KindInt, IVal: l.IVal / r.IVal})

	case bytecode.OpMod:
		r, l := vm.pop(), vm.pop()
		if r.IVal == 0 {
			return vm.runtimeErr(ins, "modulo by zero")
		}
		vm.push(Value{Kind: KindInt, IVal: l.IVal % r.IVal})

	case bytecode.OpNeg:
		v := vm.pop()
		vm.push(Value{Kind: KindInt, IVal: -v.IVal})

	// ── Comparison ──────────────────────────────────────────────────────────
	case bytecode.OpEq:
		r, l := vm.pop(), vm.pop()
		vm.push(Value{Kind: KindBool, BVal: valEqual(l, r)})

	case bytecode.OpNeq:
		r, l := vm.pop(), vm.pop()
		vm.push(Value{Kind: KindBool, BVal: !valEqual(l, r)})

	case bytecode.OpLt:
		r, l := vm.pop(), vm.pop()
		vm.push(Value{Kind: KindBool, BVal: l.IVal < r.IVal})

	case bytecode.OpGt:
		r, l := vm.pop(), vm.pop()
		vm.push(Value{Kind: KindBool, BVal: l.IVal > r.IVal})

	case bytecode.OpLeq:
		r, l := vm.pop(), vm.pop()
		vm.push(Value{Kind: KindBool, BVal: l.IVal <= r.IVal})

	case bytecode.OpGeq:
		r, l := vm.pop(), vm.pop()
		vm.push(Value{Kind: KindBool, BVal: l.IVal >= r.IVal})

	// ── Logic ───────────────────────────────────────────────────────────────
	case bytecode.OpAnd:
		r, l := vm.pop(), vm.pop()
		vm.push(Value{Kind: KindBool, BVal: l.BVal && r.BVal})

	case bytecode.OpOr:
		r, l := vm.pop(), vm.pop()
		vm.push(Value{Kind: KindBool, BVal: l.BVal || r.BVal})

	case bytecode.OpNot:
		v := vm.pop()
		vm.push(Value{Kind: KindBool, BVal: !v.BVal})

	// ── Control flow ────────────────────────────────────────────────────────
	case bytecode.OpJmp:
		vm.ip = ins.JmpTo

	case bytecode.OpJmpFalse:
		cond := vm.pop()
		if !cond.BVal {
			vm.ip = ins.JmpTo
		}

	// ── Functions ────────────────────────────────────────────────────────────
	case bytecode.OpCall:
		return vm.execCall(ins)

	case bytecode.OpReturn:
		return vm.execReturn(ins)

	// ── Built-ins ────────────────────────────────────────────────────────────
	case bytecode.OpPrintln:
		vm.execPrintln(int(ins.IntArg))

	// ── Halt ────────────────────────────────────────────────────────────────
	case bytecode.OpHalt:
		vm.ip = len(vm.prog.Instructions) // stop the loop
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Call / Return
// ─────────────────────────────────────────────────────────────────────────────

func (vm *VM) execCall(ins bytecode.Instruction) error {
	fnName := ins.StrArg
	argCount := int(ins.IntArg)

	// Find function entry.
	entry, ok := vm.prog.Functions[fnName]
	if !ok {
		return vm.runtimeErr(ins, "undefined function: %s", fnName)
	}

	// Pop args from TOS (they were pushed left-to-right).
	args := make([]Value, argCount)
	for i := argCount - 1; i >= 0; i-- {
		args[i] = vm.pop()
	}

	// Push new frame.
	frame := newFrame(fnName, vm.ip) // return to ip (after the CALL)
	vm.callStack = append(vm.callStack, frame)

	// Jump to function entry.
	vm.ip = entry

	// The function prologue (emitted by Emitter) will STORE each param.
	// We push args back onto the stack so prologue can pop them.
	for _, a := range args {
		vm.push(a)
	}

	return nil
}

func (vm *VM) execReturn(ins bytecode.Instruction) error {
	// Pop return value (if any on stack).
	var retVal Value
	if len(vm.stack) > 0 {
		retVal = vm.pop()
	} else {
		retVal = unit
	}

	// Pop the current frame.
	if len(vm.callStack) <= 1 {
		// Returning from top-level frame — halt.
		vm.ip = len(vm.prog.Instructions)
		return nil
	}

	frame := vm.callStack[len(vm.callStack)-1]
	vm.callStack = vm.callStack[:len(vm.callStack)-1]

	// Restore ip to the return address.
	vm.ip = frame.RetAddr

	// Push return value for the caller.
	if retVal.Kind != KindUnit {
		vm.push(retVal)
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Built-in: println
// ─────────────────────────────────────────────────────────────────────────────

func (vm *VM) execPrintln(n int) {
	if n == 0 {
		fmt.Println()
		return
	}
	// Pop n values from the stack.
	vals := make([]Value, n)
	for i := n - 1; i >= 0; i-- {
		vals[i] = vm.pop()
	}
	parts := make([]string, n)
	for i, v := range vals {
		parts[i] = v.String()
	}
	fmt.Println(strings.Join(parts, " "))
}

// ─────────────────────────────────────────────────────────────────────────────
// Stack / frame operations
// ─────────────────────────────────────────────────────────────────────────────

func (vm *VM) push(v Value) {
	vm.stack = append(vm.stack, v)
}

func (vm *VM) pop() Value {
	if len(vm.stack) == 0 {
		fmt.Fprintln(os.Stderr, "vm: stack underflow")
		os.Exit(1)
	}
	v := vm.stack[len(vm.stack)-1]
	vm.stack = vm.stack[:len(vm.stack)-1]
	return v
}

func (vm *VM) currentFrame() *Frame {
	return vm.callStack[len(vm.callStack)-1]
}

// loadVar looks up a variable in the current frame.
// It returns an error if the variable has been moved or dropped.
func (vm *VM) loadVar(name string) (Value, error) {
	frame := vm.currentFrame()
	v, ok := frame.Locals[name]
	if !ok {
		// The variable may be a function name — treat as a callable.
		if _, isFn := vm.prog.Functions[name]; isFn {
			return Value{Kind: KindStr, SVal: name}, nil
		}
		return Value{}, fmt.Errorf("undefined variable: %s", name)
	}
	switch v.Kind {
	case KindMoved:
		return Value{}, fmt.Errorf("runtime ownership violation: use of moved value `%s`\n  note: value was moved earlier in this scope", name)
	case KindDropped:
		return Value{}, fmt.Errorf("runtime ownership violation: use of dropped value `%s`", name)
	}
	return v, nil
}

func (vm *VM) setVar(name string, v Value) {
	vm.currentFrame().Locals[name] = v
}

// ─────────────────────────────────────────────────────────────────────────────
// Error helpers
// ─────────────────────────────────────────────────────────────────────────────

func (vm *VM) runtimeErr(ins bytecode.Instruction, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("runtime error at ip=%d [%s]: %s", vm.ip-1, ins.Op, msg)
}

func valEqual(a, b Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case KindInt:
		return a.IVal == b.IVal
	case KindStr:
		return a.SVal == b.SVal
	case KindBool:
		return a.BVal == b.BVal
	}
	return false
}
