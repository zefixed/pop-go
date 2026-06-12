package main

import "fmt"

type statement []int

func withExtra(s statement, k int) statement {
	new := make(statement, len(s), len(s)+1)
	copy(new, s)
	return append(new, k)
}

func main() {
	fmt.Println(withExtra(statement{1, 2}, 3))
}
