package wc

import (
	"bufio"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

type Conn struct {
	reader   *bufio.Reader
	conn     net.Conn
	closed   uint32
	raddr    string
	cid      uint64
	created  time.Time
	byte_in  uint64
	byte_out uint64
	logger   zerolog.Logger
}

func NewWrappedConn(conn net.Conn, raddr string, cid uint64, logger zerolog.Logger) *Conn {
	o := &Conn{reader: bufio.NewReader(conn), conn: conn, raddr: raddr, cid: cid}
	o.created = time.Now()
	o.logger = logger.With().Str("module", "wconn").Logger()
	o.logger.Debug().Str("remote_address", o.raddr).Uint64("cid", o.cid).Msg("connection created")
	return o
}

func (c *Conn) ReadBytes(delim byte) ([]byte, error) {
	d, err := c.reader.ReadBytes(delim)
	atomic.AddUint64(&c.byte_in, uint64(len(d)))
	return d, err
}

func (c *Conn) Close() {
	c.conn.Close()
	atomic.StoreUint32(&c.closed, 1)
	c.logger.Debug().Uint64("byte_in", c.byte_in).Uint64("byte_out", c.byte_out).Uint64("cid", c.cid).Msg("Connection closed")
}

func (c *Conn) Stat() (byte_in uint64, byte_out uint64) {
	return atomic.LoadUint64(&c.byte_in), atomic.LoadUint64(&c.byte_out)
}

func (c *Conn) Cid() uint64 {
	return c.cid
}

func (c *Conn) Closed() bool {
	return atomic.LoadUint32(&c.closed) == 1
}

func (c *Conn) ReadFull(buf []byte) (int, error) {
	n, err := io.ReadFull(c.reader, buf)
	atomic.AddUint64(&c.byte_in, uint64(n))
	return n, err
}

func (c *Conn) ReadAll() ([]byte, error) {
	d, err := io.ReadAll(c.reader)
	atomic.AddUint64(&c.byte_in, uint64(len(d)))
	return d, err
}

func (c *Conn) Write(d []byte) (int, error) {
	n, err := c.conn.Write(d)
	atomic.AddUint64(&c.byte_out, uint64(n))
	return n, err
}

func (c *Conn) Peek(n int) ([]byte, error) {
	return c.reader.Peek(n)
}

func (c *Conn) RemoteAddr() string {
	return c.raddr
}

func (c *Conn) Created() time.Time {
	return c.created
}
