package droid

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"nuha.dev/gpstracker/internal/gps"
)

type loginMsg struct {
	Family string `json:"family"`
	Serial string `json:"serial"`
}

type LocationMsg struct {
	GpsTime     time.Time `json:"gps_time"`
	MachineTime time.Time `json:"machine_time"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	Altitude    float32   `json:"altitude"`
	Speed       float32   `json:"speed"`
	SatInview   int       `json:"sat_inview"`
	SatTracked  int       `json:"sat_tracked"`
	SatUsed     int       `json:"sat_used"`
	Fix         bool      `json:"fix"`
}

type Droid struct {
	c         net.Conn
	r         *bufio.Reader
	info      gps.ConnInfo
	s         gps.ServerInterface
	rid       string
	logged_in int32
	closed    int32
	err       error
	log       zerolog.Logger
	byte_in   uint64
	byte_out  uint64
	login     loginMsg
	loc       LocationMsg
}

var errRejectedLogin = errors.New("login rejected")

func NewDroid(r *bufio.Reader, c net.Conn, info gps.ConnInfo, server gps.ServerInterface) *Droid {
	o := &Droid{c: c, r: r, info: info, s: server}
	logger := log.With().Str("module", "droid").Uint64("cid", info.CID).Logger()
	o.log = logger
	o.login = loginMsg{}
	o.loc = LocationMsg{}
	return o
}

func (dr *Droid) Info() gps.ConnInfo {
	return dr.info
}

func (dr *Droid) Stat() (in, out uint64) {
	return atomic.LoadUint64(&dr.byte_in), atomic.LoadUint64(&dr.byte_out)
}

func (dr *Droid) Closed() bool {
	return atomic.LoadInt32(&dr.closed) == 1
}

func (dr *Droid) LoggedIn() bool {
	return atomic.LoadInt32(&dr.logged_in) == 1
}

func (dr *Droid) closeErr(err error) {
	dr.err = err
	dr.c.Close()
	atomic.StoreInt32(&dr.closed, 1)
}

func (dr *Droid) Run() {
	for {
		msg, err := dr.r.ReadBytes('\n')
		if err != nil {
			dr.log.Error().Err(err).Msg("error while reading")
			dr.closeErr(err)
			return
		}
		if dr.logged_in != 1 {
			err := json.Unmarshal(msg, &dr.login)
			dr.log.Info().Str("event", "login").Str("family", dr.login.Family).Str("serial", dr.login.Serial).Msg("")
			if err != nil {
				dr.log.Error().Err(err).Msg("error while pasring")
				dr.closeErr(err)
				return
			}
			rid, ok := dr.s.Login(dr.login.Family, dr.login.Serial)
			dr.rid = rid
			if ok {
				atomic.StoreInt32(&dr.logged_in, 1)
			} else {
				dr.closeErr(errRejectedLogin)
			}
		} else {
			err := json.Unmarshal(msg, &dr.loc)
			if err != nil {
				dr.log.Error().Err(err).Msg("error while pasring")
				dr.closeErr(err)
				return
			}
			dr.log.Debug().Str("event", "location update").RawJSON("event_data", msg).Msg("")
			dr.s.Location(dr.rid, dr.loc.Latitude, dr.loc.Longitude, dr.loc.GpsTime)
		}

	}
}
