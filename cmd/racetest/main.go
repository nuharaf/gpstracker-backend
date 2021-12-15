package main

import (
	"fmt"
	"sync"
)

type My struct {
	a uint64

	w sync.WaitGroup
}

func (m *My) Run() {
	go func() {
		fmt.Print(m.a)
		m.w.Done()
	}()
}

func (m *My) Run2() {
	go func() {
		m.a = 10
		m.w.Done()
	}()
}

func main() {

	m := &My{a: 345678, w: sync.WaitGroup{}}
	m.w.Add(2)
	m.Run()
	m.a = 1
	m.Run()

	m.w.Wait()

}
