package main

import (
	"fmt"
	"time"
)

func main() {
	t := time.Now().UTC()
	fmt.Printf("%q", t)
	fmt.Printf("%x-%x-%x %x:%x:%x\n", t.Year()%100, int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
}
