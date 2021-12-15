package conn

import (
	"bufio"
	"net"

	"github.com/phuslu/log"
)

type Conn struct {
	cid   uint64
	tuple []string
	r     *bufio.Reader
	net.Conn
}

func NewConn(c net.Conn, cid uint64) *Conn {
	sourceip, sourceport, _ := net.SplitHostPort(c.RemoteAddr().String())
	targetip, targetport, _ := net.SplitHostPort(c.LocalAddr().String())

	return &Conn{cid, []string{sourceip, sourceport, targetip, targetport}, bufio.NewReader(c), c}
}

func (b *Conn) Peek(n int) ([]byte, error) {
	return b.r.Peek(n)
}

func (b *Conn) Read(p []byte) (int, error) {
	return b.r.Read(p)
}

func (c *Conn) MarshalObject(e *log.Entry) {
	e.Strs("socket", c.tuple)
}
