package main

import (
	"fmt"
	"reflect"
)

type MyStruct struct {
	A string
	B int
}

func main() {

	var a []*MyStruct
	t := reflect.TypeOf(a)
	fmt.Println(t)
	v := reflect.ValueOf(test)
	inType := v.Type().In(0)
	fmt.Println(inType)
	x := reflect.New(inType)
	fmt.Println(x)

}

func test(in *[]MyStruct) {
	var out []MyStruct
	out = *in
	s1 := MyStruct{A: "hello", B: 1}
	s2 := MyStruct{A: "world", B: 2}
	out = append(out, s1, s2)
	*in = out

}
