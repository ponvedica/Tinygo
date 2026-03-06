package main

import (
	"fmt"

	"tinygo/lexer"
)

// Fibonacci using a for loop — tests: var decl, :=, assignment, arithmetic, if/else
func fibonacci(n int) int {
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
	_ = lexer.Token{} // silence import
	result := fibonacci(10)
	fmt.Println(result)
}
