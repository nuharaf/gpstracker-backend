package server

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/phuslu/log"
	proxyproto "github.com/pires/go-proxyproto"
	"nuha.dev/gpstracker/internal/gpsv2/conn"
	"nuha.dev/gpstracker/internal/gpsv2/device"
	"nuha.dev/gpstracker/internal/gpsv2/device/gt06"
	"nuha.dev/gpstracker/internal/gpsv2/device/simplejson"
	"nuha.dev/gpstracker/internal/gpsv2/sublist"
	"nuha.dev/gpstracker/internal/store"
)

const (
	NEW_CONNECTION      string = "new_connection"
	LOGIN_MESSAGE       string = "login_message"
	LOGIN_MESSAGE_ERROR string = "login_message_error"
	ALLOW_CONNECT_FALSE string = "allow_connect_false"
	NEW_DEVICE_CREATED  string = "new_device_created"
)

type Device struct {
	Dev       device.DeviceIf
	Type      string
	TrackerId uint64
	FSN       string
}

type DeviceList struct {
	mu      sync.Mutex
	list    map[uint64]Device
	fsnlist map[string]uint64
}

func (d *Device) MarshalObject(e *log.Entry) {
	e.Str("fsn", d.FSN).Uint64("tracked_id", d.TrackerId)
}

func (l *DeviceList) deviceFsn(fsn string) (Device, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	tid, ok := l.fsnlist[fsn]
	if !ok {
		return Device{}, false
	}
	dev, ok := l.list[tid]
	if !ok {
		return Device{}, false
	}
	return dev, true
}

func (l *DeviceList) addDevice(fsn string, tid uint64, dev device.DeviceIf, dev_type string) {
	l.mu.Lock()
	l.fsnlist[fsn] = tid
	l.list[tid] = Device{Dev: dev, Type: dev_type, TrackerId: tid, FSN: fsn}
	l.mu.Unlock()
}

type Server struct {
	mu            sync.Mutex
	log           log.Logger
	db            *pgxpool.Pool
	config        *ServerConfig
	cid_counter   uint64
	store         store.Store
	listener      net.Listener
	proxylistener proxyproto.Listener
	device_list   *DeviceList
	sublist       *sublist.SublistMap
}

func NewServer(db *pgxpool.Pool, store store.Store, sublistmap *sublist.SublistMap, config *ServerConfig) *Server {

	s := &Server{}
	s.log = log.DefaultLogger
	s.log.Context = log.NewContext(nil).Str("module", "gps-server").Value()
	s.config = config
	s.db = db
	s.store = store
	s.device_list = &DeviceList{fsnlist: make(map[string]uint64), list: make(map[uint64]Device)}
	s.sublist = sublistmap
	return s
}

// type SerialNumber struct {
// 	SnType string
// 	Serial uint64
// 	FSN    string
// }

// func newSerialNUmber(sn_type string, serial uint64) SerialNumber {
// 	sn := SerialNumber{}
// 	sn.Serial = serial
// 	sn.SnType = sn_type
// 	sn.FSN = sn_type + ":" + strconv.FormatUint(serial, 16)
// 	return sn
// }

type ServerConfig struct {
	ListenerAddr string
}

type LoginHandler struct {
	s           *Server
	c           *conn.Conn
	device_type string
}

func (s *Server) Run() {
	s.runListener()
}

func (s *Server) GetDevice(tid uint64) (Device, bool) {
	s.device_list.mu.Lock()
	defer s.device_list.mu.Unlock()
	d, ok := s.device_list.list[tid]
	return d, ok

}

func (s *Server) NewLoginHandler(c *conn.Conn) *LoginHandler {
	lh := LoginHandler{}
	lh.s = s
	lh.c = c
	return &lh
}

func (s *Server) runListener() {
	s.log.Info().Msgf("starting gps-server on %s", s.config.ListenerAddr)
	ln, err := net.Listen("tcp", s.config.ListenerAddr)
	if err != nil {
		s.log.Error().Err(err).Msg("unable to listen")
		return
	}
	pln := proxyproto.Listener{Listener: ln}
	s.mu.Lock()
	s.listener = ln
	s.proxylistener = pln
	s.mu.Unlock()

	for {
		s.log.Info().Msg("accepting connection ...")
		_c, err := pln.Accept()
		if err != nil {
			s.log.Error().Err(err).Msg("failed to accept new connection")
			pln.Close()
			return
		}
		c := conn.NewConn(_c, s.cid_counter)
		s.cid_counter = s.cid_counter + 1
		s.log.Info().Str("event", string(NEW_CONNECTION)).EmbedObject(c).Uint64("cid", s.cid_counter).Msg("")
		h := s.NewLoginHandler(c)
		go h.handle()
	}
}

// func (s *Server) fetch_config(sn_type string, serial uint64) (uint64, *device.DeviceConfig, error) {

// 	var tid uint64
// 	devconf := device.DeviceConfig{}
// 	selectSql := `SELECT id ,config FROM public."tracker" where sn_type = $1 AND serial_number =$2`
// 	err := s.db.QueryRow(context.Background(), selectSql, sn_type, serial).Scan(&tid, &devconf)

// 	if err != nil {
// 		if err == pgx.ErrNoRows {
// 			insertSql := `INSERT INTO public.tracker (sn_type,serial_number,allow_connect) VALUES ($1,$2,$3,fal) RETURNING id`
// 			err := s.db.QueryRow(context.Background(), insertSql, sn_type, serial).Scan(&tid)
// 			if err != nil {
// 				s.log.Error().Err(err).Msg("error while auto registering tracker")
// 				return 0, nil, err
// 			}
// 			return 0, &devconf, nil
// 		} else {
// 			s.log.Error().Err(err).Msg("error while querying tracker by serial")
// 			return 0, nil, err
// 		}
// 	} else {
// 		return tid, &devconf, err
// 	}
// }

// func (s *Server) fetch_default_config() (json.RawMessage, error) {
// 	var c json.RawMessage
// 	selectSql := `SELECT config FROM public."config_template" where name = 'tracker_default_config'`
// 	err := s.db.QueryRow(context.Background(), selectSql, "tracker").Scan(&c)
// 	if err != nil {
// 		return nil, err
// 	} else {
// 		return c, err
// 	}
// }

func (s *Server) add_tracker_default(sn_type string, serial uint64) (uint64, *device.DeviceConfig, error) {
	var tid uint64
	var config device.DeviceConfig
	query := `INSERT INTO tracker(sn_type,serial_number,config) SELECT $1,$2, config FROM config_template WHERE name='tracker_default_config' RETURNING id,config`
	err := s.db.QueryRow(context.Background(), query, sn_type, serial).Scan(&tid, &config)
	if err != nil {
		return 0, nil, err
	} else {
		return tid, &config, nil
	}

}

func (s *Server) fetch_config(sn_type string, serial uint64) (uint64, *device.DeviceConfig, error) {

	var tid uint64
	var conf device.DeviceConfig
	selectSql := `SELECT id ,config FROM "tracker" where sn_type = $1 AND serial_number =$2`
	err := s.db.QueryRow(context.Background(), selectSql, sn_type, serial).Scan(&tid, &conf)

	if err != nil {
		if err == pgx.ErrNoRows {
			tid, conf, err := s.add_tracker_default(sn_type, serial)
			if err != nil {
				return 0, nil, err
			} else {
				return tid, conf, err
			}
		} else {
			s.log.Error().Err(err).Msg("error while querying tracker by serial")
			return 0, nil, err
		}
	} else {
		return tid, &conf, err
	}
}

// func NewDevice(device_type string, c *conn.Conn, store store.Store) device.DeviceIf {

// 	switch device_type {
// 	case device.DEVICE_GT06:
// 		dev := gt06.NewGT06(c, store)
// 		dev.Run()
// 		return dev
// 	case device.DEVICE_SIMPLEJSON:
// 		dev := simplejson.NewSimpleJSON(c, store)
// 		dev.Run()
// 		return dev
// 	default:
// 		dev := simplejson.NewSimpleJSON(c, store)
// 		dev.Run()
// 		return dev
// 	}
// }

func (h *LoginHandler) handle() {
	var err error
	_ = h.c.SetReadDeadline(time.Now().Add(5 * time.Second))
	b, err := h.c.Peek(1)
	if err != nil {
		h.s.log.Error().Err(err).Str("event", LOGIN_MESSAGE_ERROR).EmbedObject(h.c).Msg("error peeking from connection, will close")
		h.c.Close()
		return
	}

	if b[0] == 0x99 {
		h.device_type = device.DEVICE_SIMPLEJSON
		h.s.log.Trace().EmbedObject(h.c).Msgf("detected as %s", device.DEVICE_SIMPLEJSON)
		h.handleAsSimpleJson()

	} else if b[0] == 0x78 {
		h.device_type = device.DEVICE_GT06
		h.s.log.Trace().EmbedObject(h.c).Msgf("detected as %s", device.DEVICE_GT06)
		h.handleAsGT06()
	}

}

func (h *LoginHandler) MarshalObject(e *log.Entry) {
	e.EmbedObject(h.c).Str("device_type", h.device_type)
}

func (h *LoginHandler) handleAsGT06() {
	var err error
	msg := gt06.Message{}
	msg.Buffer = make([]byte, 100)
	err = gt06.ReadMessage(h.c, &msg)
	if err != nil {
		h.s.log.Error().Err(err).Str("event", LOGIN_MESSAGE_ERROR).EmbedObject(h).Msg("error reading login message")
		h.c.Close()
		return
	}
	_ = h.c.SetReadDeadline(time.Time{})
	if msg.Protocol == gt06.LOGIN {
		loginMessage := gt06.ParseLoginMessage(msg.Payload)
		if err != nil {
			h.s.log.Error().Err(err).Str("event", LOGIN_MESSAGE_ERROR).EmbedObject(h).Msg("error parsing login message")
			h.c.Close()
			return
		} else {
			var sn uint64
			var sn_type = "imei"
			sn, err = strconv.ParseUint(loginMessage.SN, 10, 64)
			if err != nil {
				h.s.log.Error().Err(err).Str("event", LOGIN_MESSAGE_ERROR).EmbedObject(h).Msg("error parsing serial number")
				h.c.Close()
				return
			}
			err := gt06.SendLoginOK(h.c, msg.Serial)
			if err != nil {
				h.s.log.Error().Err(err).EmbedObject(h).Msg("error sending login acknowledge")
				h.c.Close()
				return
			}
			fsn := device.JoinSn(sn_type, sn)
			h.s.log.Info().Str("event", LOGIN_MESSAGE).EmbedObject(h).Str("serial_number_type", sn_type).Uint64("serial_number", sn).Msg("")
			dev, ok := h.s.device_list.deviceFsn(fsn)
			if ok {
				h.s.log.Trace().EmbedObject(h).Msgf("replacing older connection for %d", sn)
				dev.Dev.ReplaceConn(h.c)
			} else {
				tid, conf, err := h.s.fetch_config("imei", sn)
				if err != nil {
					h.c.Close()
					return
				}
				if !conf.AllowConnect {
					h.s.log.Info().Str("event", ALLOW_CONNECT_FALSE).EmbedObject(h).Msgf("device %s not alloweed to connect", fsn)
					h.c.Close()
					return
				}
				h.s.log.Info().Str("event", NEW_DEVICE_CREATED).EmbedObject(h).Msgf("new device %s", fsn)
				var logger = log.DefaultLogger
				logger.Level = log.ParseLevel(conf.LogLevel)
				s, _ := h.s.sublist.GetSublist(tid, true)
				dev := gt06.NewGT06(tid, fsn, h.c, h.s.store, logger, &loginMessage, s, conf)
				dev.Run()
				h.s.device_list.addDevice(fsn, tid, dev, device.DEVICE_GT06)
			}
		}
	} else {
		h.s.log.Error().EmbedObject(h).Str("event", LOGIN_MESSAGE_ERROR).Msgf("message type is not login,type : %x", msg.Protocol)
		h.c.Close()
		return
	}

}

func (h *LoginHandler) handleAsSimpleJson() {
	var err error
	msg := simplejson.FrameMessage{}
	msg.Buffer = make([]byte, 100)
	err = simplejson.ReadMessage(h.c, &msg)
	if err != nil {
		h.s.log.Error().Err(err).Str("event", LOGIN_MESSAGE_ERROR).EmbedObject(h).Msg("error reading login message")
		h.c.Close()
		return
	}
	_ = h.c.SetReadDeadline(time.Time{})
	if msg.Protocol == simplejson.LOGIN {
		loginMessage := simplejson.LoginMessage{}
		err = json.Unmarshal(msg.Payload, &loginMessage)
		if err != nil {
			h.s.log.Error().Err(err).Str("event", LOGIN_MESSAGE_ERROR).EmbedObject(h).Msg("error parsing login message")
			h.c.Close()
			return
		}
		sn, err := strconv.ParseUint(loginMessage.Serial, 16, 64)
		if err != nil {
			h.s.log.Error().Err(err).Str("event", LOGIN_MESSAGE_ERROR).EmbedObject(h).Msg("error parsing serial number")
			h.c.Close()
			return
		}
		var sn_type = loginMessage.SnType
		fsn := device.JoinSn(sn_type, sn)
		h.s.log.Info().Str("event", LOGIN_MESSAGE).EmbedObject(h).Str("serial_number_type", sn_type).Uint64("serial_number", sn).Msg("")
		dev, ok := h.s.device_list.deviceFsn(fsn)
		if ok {
			h.s.log.Trace().EmbedObject(h).Msgf("replacing older connection for %d", sn)
			dev.Dev.ReplaceConn(h.c)
		} else {
			tid, conf, err := h.s.fetch_config(sn_type, sn)
			if err != nil {
				h.c.Close()
				return
			}
			if !conf.AllowConnect {
				h.s.log.Info().Str("event", ALLOW_CONNECT_FALSE).EmbedObject(h).Msgf("device %s not alloweed to connect", fsn)
				h.c.Close()
				return
			}
			h.s.log.Info().Str("event", NEW_DEVICE_CREATED).EmbedObject(h).Msgf("new device %s", fsn)
			var logger = log.DefaultLogger
			logger.Level = log.ParseLevel(conf.LogLevel)
			s, _ := h.s.sublist.GetSublist(tid, true)
			dev := simplejson.NewSimpleJSON(h.c, h.s.store, logger, &loginMessage, s, conf)
			dev.Run()
			h.s.device_list.addDevice(fsn, tid, dev, device.DEVICE_GT06)
		}
	} else {
		h.s.log.Error().EmbedObject(h).Str("event", LOGIN_MESSAGE_ERROR).Msgf("message type is not login,type : %x", msg.Protocol)
		h.c.Close()
		return
	}
}
