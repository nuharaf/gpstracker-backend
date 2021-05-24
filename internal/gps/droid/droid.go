package droid

import (
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"nuha.dev/gpstracker/internal/gps/client"
	"nuha.dev/gpstracker/internal/gps/server"
	"nuha.dev/gpstracker/internal/gps/subscriber"
	"nuha.dev/gpstracker/internal/store"
	"nuha.dev/gpstracker/internal/util/wc"
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
	c         *wc.Conn
	s         server.ServerInterface
	rid       string
	logged_in int32
	err       error
	log       zerolog.Logger
	login     loginMsg
	loc       LocationMsg
	store     store.Store
	state     *client.ClientState
}

var errRejectedLogin = errors.New("login rejected")

func NewDroid(c *wc.Conn, server server.ServerInterface) *Droid {
	o := &Droid{c: c, s: server}
	logger := log.With().Str("module", "droid").Uint64("cid", c.Cid()).Logger()
	o.log = logger
	o.login = loginMsg{}
	o.loc = LocationMsg{}
	return o
}

func (dr *Droid) Conn() *wc.Conn {
	return dr.c
}

func (dr *Droid) Closed() bool {
	return dr.c.Closed()
}

func (dr *Droid) LoggedIn() bool {
	return atomic.LoadInt32(&dr.logged_in) == 1
}

func (dr *Droid) Subscribe(sub subscriber.Subscriber) {
	dr.state.Sublist.Subscribe(sub)
}

func (dr *Droid) closeErr(err error) {
	dr.err = err
	dr.c.Close()
	dr.state.Stat.DisconnectEv(time.Now())
}

func (dr *Droid) SetState(state *client.ClientState) {
	dr.state = state
}

func (dr *Droid) Run() {
	for {
		msg, err := dr.c.ReadBytes('\n')
		tread := time.Now()
		if err != nil {
			dr.log.Error().Err(err).Msg("error while reading")
			dr.closeErr(err)
			return
		}
		if dr.logged_in != 1 {
			err := json.Unmarshal(msg, &dr.login)
			if err != nil {
				dr.log.Error().Err(err).Msg("error parsing login message")
				dr.closeErr(err)
				return
			}
			dr.log.Info().Str("event", "login").Str("family", dr.login.Family).Str("serial", dr.login.Serial).Msg("")
			rid, ok := dr.s.Login(dr.login.Family, dr.login.Serial, dr)
			tlogin := time.Now()
			if ok {
				atomic.StoreInt32(&dr.logged_in, 1)
				dr.state.Stat.ConnectEv(dr.c.Created())
				dr.state.Stat.LoginEv(tlogin)
				dr.rid = rid
				dr.log = dr.log.With().Str("rid", rid).Logger()
				dr.log.Info().Msg("login successful")

			} else {
				dr.closeErr(errRejectedLogin)
				dr.log.Err(errRejectedLogin).Msg("login rejected")
				return
			}
		} else {
			err := json.Unmarshal(msg, &dr.loc)
			if err != nil {
				dr.log.Error().Err(err).Msg("error parsing location data")
				dr.closeErr(err)
				return
			}
			dr.log.Debug().Str("event", "location update").RawJSON("event_data", msg).Msg("")
			dr.state.Sublist.Send(dr.rid, []byte{})
			dr.state.Stat.CounterIncr(1, tread)
			dr.store.Put(dr.rid, dr.loc.Latitude, dr.loc.Longitude, dr.loc.Altitude, dr.loc.Speed, dr.loc.GpsTime, tread)
		}

	}
}
