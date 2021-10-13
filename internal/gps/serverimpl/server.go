package serverimpl

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/phuslu/log"
	"nuha.dev/gpstracker/internal/gps/client"
	"nuha.dev/gpstracker/internal/gps/droid"
	"nuha.dev/gpstracker/internal/gps/gt06"
	"nuha.dev/gpstracker/internal/store"
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

type clientsession_list struct {
	mu   sync.Mutex
	list map[uint64]*client.ClientSession
}

type ClientStatus struct {
	TrackerId     uint64    `json:"tracker_id"`
	FSN           string    `json:"fsn"`
	LastConnect   time.Time `json:"last_connect,omitempty"`
	LastDisc      time.Time `json:"last_disconnect,omitempty"`
	LastUpdate    time.Time `json:"last_update,omitempty"`
	LastLatitude  float64   `json:"last_latitude,omitempty"`
	LastLongitude float64   `json:"last_longitude,omitempty"`
	LastGpsTime   time.Time `json:"last_timestamp,omitempty"`
}

type ClientStatusDetail struct {
	TrackerId      uint64      `json:"tracker_id"`
	FSN            string      `json:"fsn"`
	ConnectHistory []time.Time `json:"connect_event"`
	DiscHistory    []time.Time `json:"disconnect_event"`
	UpdateHistory  []time.Time `json:"update_event"`
	LastLocation   struct {
		Latitude  float64   `json:"latitude"`
		Longitude float64   `json:"longitude"`
		GpsTime   time.Time `json:"timestamp"`
	} `json:"last_known_location"`
	AdditionalStatus map[string]interface{} `json:"additional_status,omitempty"`
}

type Server struct {
	log         log.Logger
	db          *pgxpool.Pool
	config      *ServerConfig
	cid_counter uint64
	conn_list
	clientsession_list
	store store.Store
}

type ServerConfig struct {
	DirectListenerAddr string
	// EnableTunnel       bool
	// YamuxTunnelAddr    string
	// MockLogin bool
	// YamuxToken         string
	MockStore bool
}

func NewServer(db *pgxpool.Pool, store store.Store, config *ServerConfig) *Server {

	s := &Server{}
	s.conn_list = conn_list{mu: sync.Mutex{}, list: make(map[uint64]client.ClientInterface)}
	s.clientsession_list = clientsession_list{mu: sync.Mutex{}, list: make(map[uint64]*client.ClientSession)}
	// s.msubs = msubs{mu: sync.RWMutex{}, list: make(map[string]*subs)}
	s.log = log.DefaultLogger
	s.log.Context = log.NewContext(nil).Str("module", "server").Value()
	s.config = config
	s.db = db
	s.store = store
	s.initTable()
	return s
}

func (s *Server) initTable() {
	ddl := `CREATE TABLE public.tracker (
	id bigserial NOT NULL,
	sn_type text NOT NULL,
	serial_number int8 NOT NULL,
	fsn text NULL,
	allow_connect bool NULL,
	registered_at timestamptz NULL,
	"attribute" jsonb NULL,
	CONSTRAINT tracker_pk PRIMARY KEY (id),
	CONSTRAINT tracker_un UNIQUE (sn_type, serial_number));`
	_, err := s.db.Exec(context.Background(), ddl)
	if err != nil {
		s.log.Error().Err(err).Msg("failed to create table")
	}

}

func (s *Server) runDirectListener() {
	s.log.Info().Str("addr", s.config.DirectListenerAddr).Msg("starting direct port listener")
	ln, err := net.Listen("tcp", s.config.DirectListenerAddr)
	if err != nil {
		s.log.Error().Err(err).Msg("unable to listen")
		return
	}
	for {
		s.log.Info().Msg("accepting new direct connection ...")
		conn, err := ln.Accept()
		if err != nil {
			s.log.Error().Err(err).Msg("failed to accept new direct connection")
			ln.Close()
			return
		}
		s.handleConn(conn, conn.RemoteAddr().String())
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

func (s *Server) GetGpsClientState(tid uint64) *client.ClientSession {
	var state *client.ClientSession
	s.clientsession_list.mu.Lock()
	defer s.clientsession_list.mu.Unlock()
	state, ok := s.clientsession_list.list[tid]
	if !ok {
		var fsn string
		//verify tracker id is in database
		selectSql := `SELECT sn_type,serial,fsn FROM public."tracker" where id = $1`
		err := s.db.QueryRow(context.Background(), selectSql, tid).Scan(&fsn)
		if err != nil {
			s.log.Error().Err(err).Msg("error while querying tracker by id")
			return nil
		}

		state = client.NewClientSession(tid, fsn)
		s.clientsession_list.list[tid] = state
	}
	return state
}

func (s *Server) GetClientsStatus() []ClientStatus {

	statistic := make([]ClientStatus, 0, 10)
	s.clientsession_list.mu.Lock()
	defer s.clientsession_list.mu.Unlock()

	for tid, state := range s.clientsession_list.list {
		c := ClientStatus{}
		c.TrackerId = tid
		c.FSN = state.FSN
		lon, lat, gpstime := state.GetLastLocation()
		c.LastLatitude = lat
		c.LastLongitude = lon
		c.LastGpsTime = gpstime
		statistic = append(statistic, c)
	}

	return statistic
}

func (s *Server) GetClientStatus(tid uint64) *ClientStatusDetail {
	s.clientsession_list.mu.Lock()
	defer s.clientsession_list.mu.Unlock()
	state, ok := s.clientsession_list.list[tid]
	if !ok {
		return nil
	} else {
		c := &ClientStatusDetail{}
		c.TrackerId = tid
		c.FSN = state.FSN
		lon, lat, gpstime := state.GetLastLocation()
		c.LastLocation.Latitude = lat
		c.LastLocation.Longitude = lon
		c.LastLocation.GpsTime = gpstime
		// c.AdditionalStatus = state.GetKV()
		return c
	}
}

func (s *Server) Login(sn_type string, serial uint64, c client.ClientInterface) (bool, *client.ClientSession, *client.ClientConfig) {

	var tid uint64
	var fsn string
	selectSql := `SELECT id,fsn FROM public."tracker" where sn_type = $1 AND serial_number =$2`
	err := s.db.QueryRow(context.Background(), selectSql, sn_type, serial).Scan(&tid, &fsn)

	if err != nil {
		if err == pgx.ErrNoRows {
			s.log.Info().Str("sn_type", sn_type).Uint64("sn", serial).Msg("tracker not found, registering automatically")
			//auto register tracker
			fsn = sn_type + ":" + strconv.FormatUint(serial, 16)
			insertSql := `INSERT INTO public.tracker (sn_type,serial_number,fsn,allow_connect,registered_at) VALUES ($1,$2,$3,true,now()) RETURNING id`
			err := s.db.QueryRow(context.Background(), insertSql, sn_type, serial, fsn).Scan(&tid)
			if err != nil {
				s.log.Error().Err(err).Msg("error while auto registering tracker")
				return false, nil, nil
			}
		} else {
			s.log.Error().Err(err).Msg("error while querying tracker by serial")
			return false, nil, nil
		}
	} else {
		s.log.Info().Str("sn_type", sn_type).Uint64("sn", serial).Msgf("tracker found with tid : %d", tid)
	}

	s.clientsession_list.mu.Lock()

	session, ok := s.clientsession_list.list[tid]
	if !ok {
		session = client.NewClientSession(tid, fsn)
		s.clientsession_list.list[tid] = session
		s.clientsession_list.mu.Unlock()
	} else {
		s.log.Info().Msgf("Existing session for tid = %d found", tid)
		session.Lock()
		if session.Client != nil {
			s.log.Info().Msgf("Disconnecting previous connection with cid = %d", session.Client.ConnectionId())
			ok := session.Client.TryCloseWait()
			if !ok {
				s.log.Warn().Msgf("Disconnecting attempt for cid = %d failed", session.Client.ConnectionId())
				return false, nil, nil
			}
		}
		session.Client = c
		session.Unlock()

	}

	return true, session, &client.ClientConfig{}
}

func (s *Server) handleConn(conn net.Conn, raddr string) {
	cid := atomic.AddUint64(&s.cid_counter, 1)
	wconn := wc.NewWrappedConn(conn, raddr, cid)
	s.log.Info().Uint64("cid", cid).Msg("creating new client ... ")
	client, err := s.newClient(wconn)
	if err == nil {
		s.saveClient(cid, client)
		s.log.Info().Uint64("cid", cid).Msg("running client")
		client.Run()
	} else {
		s.log.Error().Uint64("cid", cid).Msg("failed to create client")
	}
}

func (s *Server) newClient(conn *wc.Conn) (client.ClientInterface, error) {
	// conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	st, err := conn.Peek(1)
	if err != nil {
		s.log.Error().Err(err).Msg("error while peeking from connection")
		return nil, err
	}
	ip, port, _ := net.SplitHostPort(conn.RemoteAddr())
	if st[0] == '{' {
		droid := droid.NewDroid(conn, s, s.store)
		s.log.Info().Str("remote_host", ip).Str("remote_port", port).Uint64("cid", conn.Cid()).Msg("new droid client")
		return droid, nil
	} else if st[0] == 0x78 {
		gt06 := gt06.NewGT06(conn, s, s.store)
		s.log.Info().Str("remote_host", ip).Str("remote_port", port).Uint64("cid", conn.Cid()).Msg("new gt06 client")
		return gt06, nil
	} else {
		b, _ := conn.ReadAll()
		s.log.Error().Hex("data", b).Str("remote_host", ip).Str("remote_port", port).Uint64("cid", conn.Cid()).Msg("unknown data")
		conn.Close()
		return nil, errUnknownClient
	}
}
