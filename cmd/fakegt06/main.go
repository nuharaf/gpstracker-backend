package main

import (
	"encoding/hex"
	"log"
	"net"
	"time"
)

func main() {
	c, err := net.Dial("tcp", "s1.nuha.dev:6000")
	if err != nil {
		log.Fatal(err)
	}
	loginMsg := []byte{0x78, 0x78, 0x0D, 0x01, 0x01, 0x23, 0x45, 0x67, 0x89, 0x01, 0x23, 0x45, 0x00, 0x01, 0x8C, 0xDD, 0x0D, 0x0A}
	n, err := c.Write(loginMsg[:])
	log.Printf("writing %d\n", n)
	if err != nil {
		log.Fatal(err)
	}
	for {
		_ = c.SetReadDeadline(time.Now().Add(40 * time.Second))
		b := make([]byte, 10)
		n, err := c.Read(b)
		log.Printf("reading %s\n", hex.EncodeToString(b[:n]))
		if err != nil {
			log.Fatal(err)
		}

	}
}
