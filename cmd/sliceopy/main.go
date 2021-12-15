package main

import "fmt"

func main() {
	a := []int{5, 6, 7, 8, 9}
	b := make([]int, 10)
	b = b[:len(a)]
	copy(b, a)
	fmt.Printf("%v\n", b)
}
