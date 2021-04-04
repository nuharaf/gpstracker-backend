package serverimpl

import (
	"bufio"
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"nuha.dev/gpstracker/internal/gps/client"
	"nuha.dev/gpstracker/internal/gps/droid"
	"nuha.dev/gpstracker/internal/gps/gt06"
	"nuha.dev/gpstracker/internal/gps/stat"
	"nuha.dev/gpstracker/internal/gps/store"
	"nuha.dev/gpstracker/internal/gps/sublist"
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
	YamuxTunnelAddr    string
	MockLogin          bool
	YamuxToken         string
}

func NewServer(db *pgxpool.Pool, config *ServerConfig) *Server {

	s := &Server{}
	// s.msubs = msubs{mu: sync.RWMutex{}, list: make(map[string]*subs)}
	s.logger = log.With().Str("module", "server").Logger()
	var err error
	if err != nil {
		s.logger.Err(err)
		return nil
	}
	return s
}

func (s *Server) runMuxListerner() {
	runLoop := func() {
		s.logger.Info().Msgf("Dialling tunnel %s", s.config.YamuxTunnelAddr)
		yconn, err := net.Dial("tcp", s.config.YamuxTunnelAddr)
		if err != nil {
			s.logger.Err(err).Msg("unable to dial yamux server")
			return
		}
		_, err = yconn.Write([]byte(s.config.YamuxToken))
		if err != nil {
			yconn.Close()
			s.logger.Err(err).Msg("unable to authenticate with yamux server")
			return
		}
		status := []byte{0}
		_, err = yconn.Read(status)
		if err != nil {
			yconn.Close()
			s.logger.Err(err).Msg("unable to authenticate with yamux server")
			return
		}
		if status[0] == '+' {
			s.logger.Info().Msg("yamux tunnel accepted")
		} else {
			s.logger.Error().Msg("yamux tunnel rejected")
			return
		}
		session, err := yamux.Client(yconn, nil)
		if err != nil {
			s.logger.Err(err)
			return
		}
		for {
			tconn, err := session.Accept()
			if err != nil {
				s.logger.Err(err)
				return
			}
			cid := atomic.AddUint64(&s.cid_counter, 1)
			go func() {
				r := bufio.NewReader(tconn)
				raddr, err := r.ReadString('\n')
				if err != nil {
					s.logger.Err(err)
					tconn.Close()
					return
				}
				wconn := wc.NewWrappedConn(tconn, raddr, cid, log.Logger)
				client, err := s.NewClient(wconn)
				s.saveClient(cid, client)
				if err == nil {
					client.Run()
				}
				s.delClient(cid)
			}()

		}
	}

	for {
		t0 := time.Now()
		runLoop()
		d := time.Since(t0)
		if d > 10*time.Second {
			time.Sleep(1 * time.Second)
		} else {
			time.Sleep(5 * time.Second)
		}

	}
}

func (s *Server) runDirectListener() {
	ln, err := net.Listen("tcp", s.config.DirectListenerAddr)
	if err != nil {
		panic(err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			s.logger.Err(err)
			ln.Close()
			return
		}
		cid := atomic.AddUint64(&s.cid_counter, 1)
		go func() {
			wconn := wc.NewWrappedConn(conn, conn.RemoteAddr().String(), cid, log.Logger)
			client, err := s.NewClient(wconn)
			s.saveClient(cid, client)
			if err == nil {
				client.Run()
			}
			s.delClient(cid)
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
	if s.config.DirectListenerAddr != "" {
		go func() {
			s.runDirectListener()
		}()
	}
	if s.config.YamuxTunnelAddr != "" {
		go func() {
			s.runMuxListerner()
		}()
	}
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

// func (s *Server) Subscribe(rids []string, sub subscriber.Subscriber) {
// 	s.client_list.mu.Lock()
// 	for _, v := range rids {
// 		saved_client, ok := s.client_list.list[v]
// 		if ok {
// 			saved_client.Subscribe(sub)
// 		}
// 	}
// 	s.client_list.mu.Unlock()
// }

// Subscribe read the msubs, but write to the subs
// func (s *Server) Subscribe(rid, subname string, sub gps.Subscriber) error {
// 	s.msubs.mu.RLock()
// 	slist, ok := s.msubs.list[rid]
// 	s.msubs.mu.RUnlock()

// 	if ok {
// 		slist.mu.Lock()
// 		slist.list[subname] = sub
// 		slist.mu.Unlock()
// 	} else if !ok && s.static_pub_list {
// 		s.logger.Warn().Str("publisher_rid", rid).Bool("static_pub_list", s.static_pub_list).Msg("Unable to subscibre to non-existant publisher, possibly misconfiguration")
// 		return errors.New("publisher doesnt exist")
// 	} else {
// 		new_slist := &subs{mu: sync.Mutex{}, list: make(map[string]gps.Subscriber)}
// 		new_slist.list[subname] = sub
// 		s.msubs.mu.Lock()
// 		s.msubs.list[rid] = new_slist
// 		s.msubs.mu.Unlock()
// 	}
// 	return nil
// }

//called from gps client goroutine
// func (s *Server) Location(rid string, lat, lon float64, t time.Time) {
// 	// s.locationStore(&gps.Location{lat: lat, lon: lon, t: t})
// 	s.broadcast(rid, lat, lon, t)
// }

// func (s *Server) broadcast(rid string, lat, lon float64, t time.Time) {
// 	//get subscription list for this rid
// 	s.msubs.mu.RLock()
// 	slist, ok := s.msubs.list[rid]
// 	s.msubs.mu.RUnlock()

// 	if ok {
// 		//subscription list for this rid exist
// 		d, _ := json.Marshal(struct {
// 			Latitude  float64
// 			Longitude float64
// 			Timestamp time.Time
// 		}{Latitude: lat, Longitude: lon, Timestamp: t})
// 		//Iterate subscription list . Hold lock because web client can mutate the list
// 		slist.mu.Lock()
// 		for subname, sub := range slist.list {
// 			err := sub.Push(d)
// 			if err != nil {
// 				delete(slist.list, subname)
// 			}
// 		}
// 		slist.mu.Unlock()
// 	} else if !ok && !s.static_pub_list {
// 		//subscription list for this rid does not exist, initialize with empty list
// 		new_slist := &subs{mu: sync.Mutex{}, list: make(map[string]gps.Subscriber)}
// 		//hold lock because we will mutate msubs
// 		s.msubs.mu.Lock()
// 		s.msubs.list[rid] = new_slist
// 		s.msubs.mu.Unlock()
// 	} else {
// 		s.logger.Warn().Str("publisher_rid", rid).Bool("static_pub_list", s.static_pub_list).Msg("Can't create list for unregistered publisher,possibly misconfiguration")
// 	}
// }

func (s *Server) Login(family, serial string, c client.ClientInterface) (rid string, ok bool) {
	if s.config.MockLogin {
		rid = family + serial
	} else {
		sqlStmt := `SELECT id FROM public."tracker" where family = $1 AND serial_number =$2`
		err := s.db.QueryRow(context.Background(), sqlStmt, family, serial).Scan(rid)
		if err != nil {
			if err == pgx.ErrNoRows {
				s.logger.Info().Str("action", "login").Str("family", family).Str("sn", serial).Msg("tracker not found")
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

func (s *Server) NewClient(conn *wc.Conn) (client.ClientInterface, error) {
	// conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	st, err := conn.Peek(1)
	if err != nil {
		s.logger.Log().Err(err)
		return nil, err
	}
	ip, port, _ := net.SplitHostPort(conn.RemoteAddr())
	if st[0] == '{' {
		droid := droid.NewDroid(conn, s)
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
