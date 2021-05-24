package serverimpl

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"nuha.dev/gpstracker/internal/gps/client"
	"nuha.dev/gpstracker/internal/gps/droid"
	"nuha.dev/gpstracker/internal/gps/gt06"
	"nuha.dev/gpstracker/internal/gps/stat"
	"nuha.dev/gpstracker/internal/gps/sublist"
	"nuha.dev/gpstracker/internal/store"
	"nuha.dev/gpstracker/internal/store/impl/logstore"
	"nuha.dev/gpstracker/internal/store/impl/pgstore"
	"nuha.dev/gpstracker/internal/util/wc"
)

const (
	CLIENT_GT06 = iota
	CLIENT_DROID
	CLIENT_H02
)

var errUnknownClient = errors.New("Unknown client protocol")

//hold subscriber of one publisher
// type subs struct {
// 	mu   sync.Mutex //no need for RWMutex because there is no multiple reader case
// 	list map[string]gps.Subscriber
// }

// //short for multiple subs
// type msubs struct {
// 	mu   sync.RWMutex
// 	list map[string]*subs
// }

type conn_list struct {
	mu   sync.Mutex
	list map[uint64]client.ClientInterface
}

type clientstate_list struct {
	mu   sync.Mutex
	list map[string]*client.ClientState
}

type Server struct {
	logger      zerolog.Logger
	db          *pgxpool.Pool
	config      *ServerConfig
	cid_counter uint64
	conn_list
	clientstate_list
	store store.Store
}

type ServerConfig struct {
	DirectListenerAddr string
	// EnableTunnel       bool
	// YamuxTunnelAddr    string
	MockLogin bool
	// YamuxToken         string
	MockStore bool
}

func NewServer(db *pgxpool.Pool, config *ServerConfig) *Server {

	s := &Server{}
	// s.msubs = msubs{mu: sync.RWMutex{}, list: make(map[string]*subs)}
	s.logger = log.With().Str("module", "server").Logger()
	s.config = config
	if config.MockStore {
		s.store = logstore.NewStore()
	} else {
		s.store = pgstore.NewStore(db, "location")
	}
	return s
}

func (s *Server) runDirectListener() {
	s.logger.Info().Str("addr", s.config.DirectListenerAddr).Msg("starting direct port listener")
	ln, err := net.Listen("tcp", s.config.DirectListenerAddr)
	if err != nil {
		s.logger.Err(err).Msg("unable to listen")
		return
	}
	for {
		s.logger.Info().Msg("accepting new direct connection ...")
		conn, err := ln.Accept()
		if err != nil {
			s.logger.Err(err).Msg("failed to accept new direct connection")
			ln.Close()
			return
		}

		go func() {
			s.handleConn(conn, conn.RemoteAddr().String())
		}()
	}
}

func (s *Server) saveClient(cid uint64, c client.ClientInterface) {
	s.conn_list.mu.Lock()
	s.conn_list.list[cid] = c
	s.conn_list.mu.Unlock()
}

func (s *Server) delClient(cid uint64) {
	s.conn_list.mu.Lock()
	delete(s.conn_list.list, cid)
	s.conn_list.mu.Unlock()
}

func (s *Server) Run() {
	s.runDirectListener()

}

func (s *Server) GetClientState(rid string) *client.ClientState {
	var state *client.ClientState
	s.clientstate_list.mu.Lock()
	state, ok := s.clientstate_list.list[rid]
	if !ok {
		state := &client.ClientState{Stat: stat.NewStat(), Sublist: sublist.NewMulSublist()}
		s.clientstate_list.list[rid] = state
	}
	s.clientstate_list.mu.Unlock()
	return state
}

func (s *Server) Login(family, serial string, c client.ClientInterface) (rid string, ok bool) {
	if s.config.MockLogin {
		rid = family + serial
	} else {
		sqlStmt := `SELECT id FROM public."tracker" where family = $1 AND serial_number =$2`
		err := s.db.QueryRow(context.Background(), sqlStmt, family, serial).Scan(rid)
		if err != nil {
			if err == pgx.ErrNoRows {
				s.logger.Info().Str("family", family).Str("sn", serial).Msg("tracker not found")
				return "", false
			} else {
				s.logger.Error().Err(err).Msg("error while querying database")
				return "", false
			}
		}
	}
	s.clientstate_list.mu.Lock()
	state, ok := s.clientstate_list.list[rid]
	if !ok {
		state := &client.ClientState{Stat: stat.NewStat(), Sublist: sublist.NewMulSublist()}
		s.clientstate_list.list[rid] = state
	}
	s.clientstate_list.mu.Unlock()
	c.SetState(state)
	return rid, true
}

func (s *Server) handleConn(conn net.Conn, raddr string) {
	cid := atomic.AddUint64(&s.cid_counter, 1)
	wconn := wc.NewWrappedConn(conn, raddr, cid, log.Logger)
	s.logger.Info().Uint64("cid", cid).Msg("creating new client ... ")
	client, err := s.newClient(wconn)
	if err == nil {
		s.saveClient(cid, client)
		s.logger.Info().Uint64("cid", cid).Msg("running client")
		client.Run()
		s.logger.Info().Uint64("cid", cid).Msg("client stopped")
		s.delClient(cid)
	} else {
		s.logger.Error().Uint64("cid", cid).Msg("failed to create client")
	}
}

func (s *Server) newClient(conn *wc.Conn) (client.ClientInterface, error) {
	// conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	st, err := conn.Peek(1)
	if err != nil {
		s.logger.Err(err).Msg("error while peeking from connection")
		return nil, err
	}
	ip, port, _ := net.SplitHostPort(conn.RemoteAddr())
	if st[0] == '{' {
		droid := droid.NewDroid(conn, s, s.store)
		s.logger.Info().Str("remote_host", ip).Str("remote_port", port).Uint64("cid", conn.Cid()).Msg("new droid client")
		return droid, nil
	} else if st[0] == 0x78 {
		gt06 := gt06.NewGT06(conn, s, s.store)
		s.logger.Info().Str("remote_host", ip).Str("remote_port", port).Uint64("cid", conn.Cid()).Msg("new gt06 client")
		return gt06, nil
	} else {
		b, _ := conn.ReadAll()
		s.logger.Error().Hex("data", b).Str("remote_host", ip).Str("remote_port", port).Uint64("cid", conn.Cid()).Msg("unknown data")
		conn.Close()
		return nil, errUnknownClient
	}
}
