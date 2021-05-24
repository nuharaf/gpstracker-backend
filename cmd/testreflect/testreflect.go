package main

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type MyStruct struct {
	Field1 string `validate:"required"`
	Field2 string `validate:"required"`
}

func main() {
	o := &MyStruct{Field1: "test123", Field2: "test456"}
	s, _ := json.Marshal(o)
	fmt.Print(string(s))
	t := reflect.TypeOf(o)
	n := reflect.New(t.Elem())
	json.Unmarshal(s, &n)
	fmt.Print(n)
}
