package gps

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"

	"nuha.dev/gpstracker/internal/gps/h02"
)

type H02 struct {
	c      *net.TCPConn
	r      *bufio.Reader
	s      *Server
	closed bool
	err    error
	buffer []byte
}

func NewH02(r *bufio.Reader, c *net.TCPConn, s *Server) *H02 {
	return &H02{r: r, s: s, c: c, buffer: make([]byte, 1000)}
}

func (h *H02) Close() {
	h.closed = true
	h.c.Close()
}

func (h *H02) Run() {
	for {
		msg, err := h.r.ReadString('\n')
		if err != nil {
			h.err = err
			h.Close()
		}
		fmt.Println(msg)
		if h.closed {
			log.Println(h.err)
			return
		}
		tok := strings.Split(msg, ",")
		switch tok[2] {
		case "V1":
			var loc *h02.H02GPSMessage
			loc, err = h02.ParseGPSMessage(tok)
			fmt.Printf("%+v", loc)

		}
		if err != nil {
			h.err = err
			log.Println(h.err)
			h.Close()
			return
		}
	}
}
