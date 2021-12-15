package main

import (
	"encoding/json"
	"fmt"
)

type Outer struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func main() {
	text := `{"type":"login","data": {"foo":"bar","code":1}}`
	a := []byte(text)
	o := Outer{}
	_ = json.Unmarshal(a, &o)
	fmt.Print(string(o.Data))
	a[30] = 'x'
	fmt.Print(string(a))
	fmt.Print(string(o.Data))
}
