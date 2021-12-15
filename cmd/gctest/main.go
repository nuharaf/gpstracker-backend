package main

import (
	"encoding/json"
	"fmt"
)

type Msg struct {
	MsgType int
	Payload uint
}

func main() {
	m := parse([]byte{})
	fmt.Print(m)

}

func parse(data []byte) Msg {
	m := Msg{}
	m.MsgType = 1
	_ = json.Unmarshal(data, &m)
	return m
}
