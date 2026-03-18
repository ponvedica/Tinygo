
# TinyGo — A Go Compiler Written in Go

A complete, from-scratch Go compiler that translates a subset of Go source code into native executables.

---

## What is this?

TinyGo is a hand-crafted compiler that implements every classical stage of compilation — from reading raw `.go` source files all the way to producing a runnable native binary. It does this by transpiling Go to C and invoking `gcc` to compile the final output.

The project is built entirely in Go with no external dependencies, and every component — the lexer, parser, type checker, IR, and code generator — is written from scratch.

---

## How it works

```
Source (.go)  →  Lexer  →  Parser  →  Type Checker  →  IR  →  Code Generator  →  gcc  →  Binary
```

- **Lexer** — Reads the source file and breaks it into tokens (keywords, identifiers, operators, literals)
- **Parser** — Builds an Abstract Syntax Tree using recursive-descent parsing with Pratt expression parsing for correct operator precedence
- **Type Checker** — Walks the AST, resolves variable scopes, infers types, and validates return statements
- **IR Lowering** — Converts the typed AST into intermediate representation nodes, decoupled from parsing details
- **Code Generator** — Emits valid C source code, maps Go types to C equivalents, and translates `fmt.Println` to `printf`
- **GCC** — Compiles the generated C into a native executable

---

## What it can compile

TinyGo supports a practical subset of Go including:

- Functions with parameters and return types
- Variables (`var`, `:=`), assignments, and compound operators (`+=`, `-=`, `*=`)
- `if` / `else if` / `else` statements
- `for` loops — C-style, while-style, and infinite
- `return` statements
- All arithmetic, comparison, and logical operators
- `fmt.Println`, `fmt.Print`, `fmt.Printf`
- Types: `int`, `string`, `bool`

---

## Try it

```bash
# Build the compiler
go build -o tinygo .

# Compile and run a Go file
tinygo build -o hello tests/testdata/hello.go
./hello

# Inspect individual pipeline stages
tinygo lex   tests/testdata/fib.go      # dump tokens
tinygo parse tests/testdata/fib.go      # dump AST
tinygo emit  tests/testdata/fib.go      # dump generated C
```

---

## Demo

A browser-based playground is included. Start the server and open `demo.html` to write and compile Go code interactively in the browser.

```bash
go build -o server ./cmd/server/
./server        # runs on http://localhost:8080
```

---
