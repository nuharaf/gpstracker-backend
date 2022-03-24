package main

import (
	"fmt"
	"time"
)

func main() {
	t := "2022-01-01T00:00:00+07:00"
	ti, err := time.Parse(time.RFC3339, t)
	fmt.Println(err)
	fmt.Print(ti)
}
