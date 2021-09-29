package main

import "fmt"

type I interface {
	M()
}

type T struct {
}

func (t *T) M() {}

func main() {
	var i I
	fmt.Println(i == nil)
	var t *T
	i = t

	fmt.Println(i == nil)
}
