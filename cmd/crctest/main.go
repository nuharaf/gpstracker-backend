package main

import (
	"fmt"
	"net"
)

func main() {
	// data := []byte{0x05, 0x00, 0x00, 0x04}
	// crc := crc16.Checksum(crc16.X25, data)
	// fmt.Printf("%x", crc)
	// crca := make([]byte, 2)
	// binary.BigEndian.PutUint16(crca, crc)
	// fmt.Printf("%x %x", crca[0], crca[1])
	h, err := net.LookupAddr("114.127.245.7")
	fmt.Println(h)
	fmt.Println(err)

}
