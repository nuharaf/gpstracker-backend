package main

import (
	"fmt"
	"runtime/debug"
	"time"
)

func main() {
	debug.SetTraceback("all")
	defer handle()
	go ongoroutine()
	time.Sleep(10 * time.Second)

}

func ongoroutine() {
	fmt.Println("help")
	defer handle()
	panic("test")
}

func handle() {
	r := recover()
	if r != nil {
		fmt.Print("handled")
	}

}
