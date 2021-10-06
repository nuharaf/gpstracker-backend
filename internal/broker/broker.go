package broker

import (
	"bufio"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Broker struct {
	logger zerolog.Logger
	config BrokerConfig
	rbuf   buffer
	wbuf   buffer
	wlock  *sync.Mutex

	cond  *sync.Cond
	rlock *sync.RWMutex
}

type BrokerConfig struct {
	Addr     string
	BufSize  int
	TimerDur time.Duration
}

type buffer struct {
	seq uint64
	t1  time.Time
	t2  time.Time
	buf net.Buffers
}

func new_buffer(seq uint64, len int) buffer {
	return buffer{seq: seq, buf: make(net.Buffers, 0, len)}
}

func NewBroker(config *BrokerConfig) *Broker {
	br := &Broker{}
	br.config = *config
	br.logger = log.With().Str("module", "broker").Logger()
	br.rlock = &sync.RWMutex{}
	br.cond = sync.NewCond(br.rlock.RLocker())
	br.wbuf = new_buffer(0, config.BufSize)
	br.wlock = &sync.Mutex{}
	return br
}

func (br *Broker) Run() {
	go br.timer_flusher()
	ln, err := net.Listen("tcp", br.config.Addr)
	if err != nil {
		br.logger.Err(err).Msg("unable to listen")
		return
	}
	for {
		br.logger.Info().Msg("accepting new connection ...")
		conn, err := ln.Accept()
		if err != nil {
			br.logger.Err(err).Msg("failed to accept new connection")
			ln.Close()
			return
		}
		bconn := brokerConn{br: br, c: conn, logger: br.logger}
		go func() {
			bconn.handle()
		}()
	}

}

func (br *Broker) StopAccept() {

}

func (br *Broker) timer_flusher() {
	ticker := time.NewTicker(5 * time.Second)
	for t := range ticker.C {
		br.wlock.Lock()
		if len(br.wbuf.buf) != 0 && t.Sub(br.wbuf.t1) > 5*time.Second {
			br.flush()
		}
		br.wlock.Unlock()
	}
}

func (br *Broker) Broadcast(data []byte) {
	br.wlock.Lock()
	if len(br.wbuf.buf) == 0 {
		br.wbuf.t1 = time.Now()
	}
	br.wbuf.buf = append(br.wbuf.buf, data)
	if len(br.wbuf.buf) == br.config.BufSize {
		br.flush()
	}
	br.wlock.Unlock()
}

func (br *Broker) flush() {
	next := br.wbuf.seq + 1
	br.wbuf.t2 = time.Now()
	br.rlock.Lock()
	br.rbuf = br.wbuf
	br.rlock.Unlock()
	br.cond.Broadcast()
	//allocate new buffer
	br.wbuf = new_buffer(next, br.config.BufSize)
}

type brokerConn struct {
	br *Broker
	c  net.Conn
	r  *bufio.Reader
	// token  string
	logger zerolog.Logger
}

func (bc *brokerConn) handle() {
	var err error
	bc.r = bufio.NewReader(bc.c)
	// token, err := bc.r.ReadBytes('\n')
	// if err != nil {
	// 	bc.logger.Err(err).Msg("unable to read token")
	// }
	// //verify token
	// fmt.Println(token)
	// bc.token = string(token)

	for {
		bc.br.cond.L.Lock()
		bc.br.cond.Wait()
		buf := bc.br.rbuf
		bc.br.cond.L.Unlock()
		_ = bc.c.SetWriteDeadline(time.Now().Add(time.Second))
		_, err = buf.buf.WriteTo(bc.c)
		if err != nil {
			bc.logger.Err(err).Msg("error writing buffer")
			return
		}
	}

}
