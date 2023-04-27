package main

import "fmt"

type mp struct{
    cp  mn
    
}

type mn struct{
    cp  string
    
}
func main() {
    fmt.Println("Hello world!")
	var dd mp 
	fmt.Println(dd)
	fmt.Println(dd.cp)
}