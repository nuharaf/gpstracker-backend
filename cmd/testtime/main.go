package main

import (
	"fmt"
	"time"
)

func main() {
	// t := time.Now().UTC()
	// x := -10 * time.Hour
	// fmt.Print(t)
	// fmt.Print(t.Add(x))

	a := []byte{0x4d, 0xd8}
	bcdOffset := (uint16(a[0]) << 4) + (uint16(a[1]) >> 4)
	hOffset := bcdOffset / 100
	mOffset := bcdOffset % 100
	fmt.Println(bcdOffset)
	fmt.Println(hOffset)
	fmt.Println(mOffset)
	offset := time.Duration(hOffset)*time.Hour + time.Duration(mOffset)*time.Minute
	fmt.Println(offset)
	fmt.Println(-offset)
}
