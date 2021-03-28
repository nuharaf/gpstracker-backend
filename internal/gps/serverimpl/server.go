package serverimpl

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"nuha.dev/gpstracker/internal/gps"
	"nuha.dev/gpstracker/internal/gps/droid"
	"nuha.dev/gpstracker/internal/gps/gt06"
)

const (
	CLIENT_GT06 = iota
	CLIENT_DROID
	CLIENT_H02
)

var errUnknownClient = errors.New("Unknown client protocol")

type ClientGps struct {
}

//hold subscriber of one publisher
type subs struct {
	mu   sync.Mutex //no need for RWMutex because there is no multiple reader case
	list map[string]gps.Subscriber
}

//short for multiple subs
type msubs struct {
	mu   sync.RWMutex
	list map[string]*subs
}

type clients struct {
	mu   sync.Mutex
	list map[uint64]gps.ClientInterface
}

type Server struct {
	msubs
	static_pub_list bool //publisher list is immutable after creation
	clients
	logger      zerolog.Logger
	db          *pgxpool.Pool
	loc_chan    chan *location
	config      *ServerConfig
	cid_counter uint64
}

type ServerConfig struct {
	DirectListenerAddr string
	YamuxTunnelAddr    string
	MockLogin          bool
}

type location struct {
	lat float64
	lon float64
	t   time.Time
}

func NewServer(db *pgxpool.Pool, config *ServerConfig) *Server {

	s := &Server{}
	s.clients = clients{mu: sync.Mutex{}, list: make(map[uint64]gps.ClientInterface)}
	s.msubs = msubs{mu: sync.RWMutex{}, list: make(map[string]*subs)}
	s.logger = log.With().Str("module", "server").Logger()
	s.loc_chan = make(chan *location, 10)
	var err error
	if err != nil {
		s.logger.Err(err)
		return nil
	}
	return s
}

func (s *Server) runMuxAcceptLoop() {

}

func (s *Server) runMuxListerner() {
	runLoop := func() {
		s.logger.Info().Msgf("Dialling tunnel %s", s.config.YamuxTunnelAddr)
		yconn, err := net.Dial("tcp", s.config.YamuxTunnelAddr)
		if err != nil {
			s.logger.Err(err)
			return
		}
		session, err := yamux.Client(yconn, nil)
		if err != nil {
			s.logger.Err(err)
			return
		}
		for {
			tconn, err := session.Accept()
			t := time.Now()
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
				info := gps.ConnInfo{RemoteAddr: raddr, CID: cid, TimeCreation: t}
				client, err := s.NewClient(r, tconn, info)
				s.clients.mu.Lock()
				s.clients.list[cid] = client
				s.clients.mu.Unlock()
				if err == nil {
					client.Run()
				}
				s.clients.mu.Lock()
				delete(s.clients.list, cid)
				s.clients.mu.Unlock()
			}()

		}
	}

	for {
		t0 := time.Now()
		runLoop()
		d := time.Now().Sub(t0)
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
		t := time.Now()
		if err != nil {
			s.logger.Err(err)
			ln.Close()
			return
		}
		cid := atomic.AddUint64(&s.cid_counter, 1)
		go func() {
			info := gps.ConnInfo{RemoteAddr: conn.RemoteAddr().String(), CID: cid, TimeCreation: t}
			client, err := s.NewClient(bufio.NewReader(conn), conn, info)
			s.clients.mu.Lock()
			s.clients.list[cid] = client
			s.clients.mu.Unlock()
			if err == nil {
				client.Run()
			}
			s.clients.mu.Lock()
			delete(s.clients.list, cid)
			s.clients.mu.Unlock()

		}()
	}
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

// Subscribe read the msubs, but write to the subs
func (s *Server) Subscribe(rid, subname string, sub gps.Subscriber) error {
	s.msubs.mu.RLock()
	slist, ok := s.msubs.list[rid]
	s.msubs.mu.RUnlock()

	if ok {
		slist.mu.Lock()
		slist.list[subname] = sub
		slist.mu.Unlock()
	} else if !ok && s.static_pub_list {
		s.logger.Warn().Str("publisher_rid", rid).Bool("static_pub_list", s.static_pub_list).Msg("Unable to subscibre to non-existant publisher, possibly misconfiguration")
		return errors.New("publisher doesnt exist")
	} else {
		new_slist := &subs{mu: sync.Mutex{}, list: make(map[string]gps.Subscriber)}
		new_slist.list[subname] = sub
		s.msubs.mu.Lock()
		s.msubs.list[rid] = new_slist
		s.msubs.mu.Unlock()
	}
	return nil
}

func (s *Server) locationStore(loc *location) {

	select {
	case s.loc_chan <- loc:

	default:
	}
}

func (s *Server) seriesWriter() {
	var buf [100]*location
	var i int = 0
	select {
	case l := <-s.loc_chan:
		buf[i] = l
		i = i + 1
		if i == 100 {

		}
	default:

	}
}

//called from gps client goroutine
func (s *Server) Location(rid string, lat, lon float64, t time.Time) {
	s.locationStore(&location{lat: lat, lon: lon, t: t})
	s.broadcast(rid, lat, lon, t)
}

func (s *Server) broadcast(rid string, lat, lon float64, t time.Time) {
	//get subscription list for this rid
	s.msubs.mu.RLock()
	slist, ok := s.msubs.list[rid]
	s.msubs.mu.RUnlock()

	if ok {
		//subscription list for this rid exist
		d, _ := json.Marshal(struct {
			Latitude  float64
			Longitude float64
			Timestamp time.Time
		}{Latitude: lat, Longitude: lon, Timestamp: t})
		//Iterate subscription list . Hold lock because web client can mutate the list
		slist.mu.Lock()
		for subname, sub := range slist.list {
			err := sub.Push(d)
			if err != nil {
				delete(slist.list, subname)
			}
		}
		slist.mu.Unlock()
	} else if !ok && !s.static_pub_list {
		//subscription list for this rid does not exist, initialize with empty list
		new_slist := &subs{mu: sync.Mutex{}, list: make(map[string]gps.Subscriber)}
		//hold lock because we will mutate msubs
		s.msubs.mu.Lock()
		s.msubs.list[rid] = new_slist
		s.msubs.mu.Unlock()
	} else {
		s.logger.Warn().Str("publisher_rid", rid).Bool("static_pub_list", s.static_pub_list).Msg("Can't create list for unregistered publisher,possibly misconfiguration")
	}
}

func (s *Server) Login(family, sn string) (rid string, ok bool) {
	if s.config.MockLogin {
		return family + sn, true
	}
	sqlStmt := `SELECT id FROM public."tracker" where family = $1 AND serial_number =$2`
	err := s.db.QueryRow(context.Background(), sqlStmt, family, sn).Scan(rid)
	if err != nil {
		if err == pgx.ErrNoRows {
			s.logger.Info().Str("action", "login").Str("family", family).Str("sn", sn).Msg("tracker not found")
			return "", false
		} else {
			s.logger.Error().Err(err).Msg("error while querying database")
			return "", false
		}
	} else {
		return rid, true
	}
}

func (s *Server) NewClient(r *bufio.Reader, conn net.Conn, info gps.ConnInfo) (gps.ClientInterface, error) {
	// conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	st, err := r.Peek(1)
	if err != nil {
		s.logger.Log().Err(err)
		return nil, err
	}
	ip, port, _ := net.SplitHostPort(info.RemoteAddr)
	if st[0] == '{' {
		droid := droid.NewDroid(r, conn, info, s)
		s.logger.Info().Str("remote_host", ip).Str("remote_port", port).Uint64("cid", info.CID).Msg("new droid client")
		return droid, nil
	} else if st[0] == 0x78 {
		gt06 := gt06.NewGT06(r, conn, info, s)
		s.logger.Info().Str("remote_host", ip).Str("remote_port", port).Uint64("cid", info.CID).Msg("new gt06 client")
		return gt06, nil
	} else {
		b, _ := ioutil.ReadAll(r)
		s.logger.Error().Hex("data", b).Str("remote_host", ip).Str("remote_port", port).Uint64("cid", info.CID).Msg("unknown data")
		conn.Close()
		return nil, errUnknownClient
	}
}
