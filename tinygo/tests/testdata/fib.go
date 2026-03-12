package main

import "fmt"

func fib(n int) int {
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
	result := fib(10)
	fmt.Println(result)
}
