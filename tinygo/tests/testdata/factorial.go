package main

import "fmt"

func factorial(n int) int {
	result := 1
	i := 1
	for i <= n {
		result = result * i
		i = i + 1
	}
	return result
}

func main() {
	fmt.Println(factorial(5))
}
