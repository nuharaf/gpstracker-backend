package main

import "fmt"

type BaseIf interface {
	GetName() string
}

type TypeA struct {
}

type TypeB struct {
}

func main() {
	var i BaseIf
	ta := TypeA{}
	i = ta
	tacopy = ta
	fmt.Println(ta)
	fmt.Println(ta)

}

func (a *TypeA) GetName() string {
	return "typeA"
}

func (a *TypeA) A_function() string {
	return "yes this is a"
}

func (b *TypeB) GetName() string {
	return "typeB"
}

func (b *TypeB) B_function() string {
	return "yes this is b"
}
