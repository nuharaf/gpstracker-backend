package main

import (
	"encoding/json"
	"fmt"
)

type MyStruct struct {
	Val int
	Key string
}

func main() {

	s := make([]MyStruct, 2)

	s[0] = MyStruct{99, "wow"}
	s[1] = MyStruct{98, "tsk"}
	fmt.Print(s)
	text := `[{"Key" :"hello"},{"Key":"foo","Val":0}]`
	_ = json.Unmarshal([]byte(text), &s)
	fmt.Print(s)

}
