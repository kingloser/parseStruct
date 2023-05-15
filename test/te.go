package main

import (
	"fmt"
)

func main() {
	var a int
	var b int
	var e int
	c := 12
	a = c
	b = a + 1
	e = test(c, a)

	fmt.Println(a)
	fmt.Println(b)
	fmt.Println(c)

}
func test(m int, n int) int {

	return m + n
}
