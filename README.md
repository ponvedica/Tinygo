# Tinygo - Go compiler using Go

## Abstract

This project presents the design and implementation of tinygo, a lightweight Go compiler written entirely in Go. The compiler implements a complete compilation pipeline that translates a subset of Go into native executables via an intermediate C representation. The architecture follows a traditional multi-stage compiler design, comprising lexical analysis, parsing, semantic analysis, intermediate representation generation, and code generation.

The compilation process begins with a lexer that tokenizes the input source code into a sequence of lexical tokens. These tokens are then processed by a recursive-descent parser with Pratt parsing for expressions, which constructs an Abstract Syntax Tree (AST) representing the syntactic structure of the program. A type checker performs semantic analysis by maintaining a hierarchical symbol table, resolving identifiers, verifying type consistency, and validating function return types.

Following semantic validation, the AST is lowered into an Intermediate Representation (IR) that abstracts away parsing details while preserving type information and program structure. This IR enables clearer separation between semantic analysis and backend code generation. The code generation module then traverses the checked AST and produces equivalent C source code, mapping Go constructs such as variables, functions, control flow statements, and expressions into corresponding C constructs. The generated C program is subsequently compiled using GCC to produce a native executable.

The compiler supports core language features, including variable declarations, arithmetic and boolean expressions, conditional statements, loops, function declarations, and basic built-in functions such as printing and string length operations. The modular architecture allows future extensions such as optimization passes, additional language features, and alternative backends like LLVM or assembly.

Overall, tinygo demonstrates the fundamental principles of compiler construction while providing a practical implementation of a simplified Go compiler pipeline, illustrating how high-level language constructs can be systematically transformed into executable machine code.
