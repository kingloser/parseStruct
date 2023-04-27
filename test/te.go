package main

import (
	"fmt"
)

func main() {
	fmt.Println("-----")
	if false {
		err := test()
		fmt.Println("----->", err)
	}

}
func test() error {
	var err error
	a := err
	return a
}
