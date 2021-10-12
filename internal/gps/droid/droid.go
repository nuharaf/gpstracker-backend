package droid

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/phuslu/log"
	"nuha.dev/gpstracker/internal/gps/client"
	"nuha.dev/gpstracker/internal/gps/server"
	"nuha.dev/gpstracker/internal/gps/subscriber"
	"nuha.dev/gpstracker/internal/store"
	"nuha.dev/gpstracker/internal/util/wc"
)

type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type LoginData struct {
	SnType string `json:"sn_type"`
	Serial string `json:"serial"`
}

type StatusData struct {
	Status string `json:"status"`
}

type ErrorData struct {
	ErrorMessage string `json:"error"`
}

type LocationData struct {
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
	FixMode     string    `json:"fix_mode"`
	Accuracy    float32   `json:"accuracy"`
}

type Droid struct {
	c         *wc.Conn
	s         server.ServerInterface
	tid       uint64
	logged_in int32
	err       error
	log       log.Logger
	login     LoginData
	loc       LocationData
	store     store.Store
	state     *client.ClientState
}

var errRejectedLogin = errors.New("login rejected")

func NewDroid(c *wc.Conn, server server.ServerInterface, store store.Store) *Droid {
	o := &Droid{c: c, s: server}
	o.log = log.DefaultLogger
	o.log.Context = log.NewContext(nil).Str("module", "droid").Uint64("cid", c.Cid()).Value()
	o.store = store
	o.login = LoginData{}
	o.loc = LocationData{}
	o.s = server
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
	if dr.state != nil {
		dr.state.Stat.DisconnectEv(time.Now().UTC())
	}

}

func (dr *Droid) SetState(state *client.ClientState) {
	dr.state = state
}

func (dr *Droid) readParse() (*Message, error) {
	msg, err := dr.c.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	m := Message{}
	err = json.Unmarshal(msg, &m)
	if err != nil {
		return nil, err
	} else {
		return &m, nil
	}
}

func (dr *Droid) Run() {
	msg, err := dr.readParse()
	if err != nil {
		dr.log.Error().Err(err).Msg("error while reading message")
		dr.closeErr(err)
		return
	}

	if msg.Type != "login" {
		err := fmt.Errorf("first message not login message")
		dr.log.Error().Err(err).Msg("")
		dr.closeErr(err)
		return
	} else {
		err := json.Unmarshal(msg.Data, &dr.login)
		if err != nil {
			dr.log.Error().Err(err).Msg("error parsing login message")
			dr.closeErr(err)
			return
		}
	}

	sn, err := strconv.ParseUint(dr.login.Serial, 16, 64)
	if err != nil {
		dr.log.Error().Err(err).Msg("error parsing serial number")
		dr.closeErr(err)
		return
	}

	dr.log.Info().Str("event", "login").Str("sn_type", dr.login.SnType).Uint64("serial", sn).Msg("")
	ok := dr.s.Login(dr.login.SnType, sn, dr)
	if ok {
		atomic.StoreInt32(&dr.logged_in, 1)
		dr.state.Stat.ConnectEv(dr.c.Created())
		dr.tid = dr.state.TrackerId
		dr.log.Context = log.NewContext(dr.log.Context).Uint64("tid", dr.tid).Value()
		dr.log.Info().Msg("login successful")

	} else {
		dr.closeErr(errRejectedLogin)
		dr.log.Error().Err(errRejectedLogin).Msg("login rejected")
		return
	}

	// dr.state.Attached.Lock()
	fsn := dr.state.FSN
	for {

		msg, err := dr.readParse()
		tread := time.Now().UTC()
		if err != nil {
			dr.log.Error().Err(err).Msg("error while reading message")
			dr.closeErr(err)
			break
		}
		dr.state.Stat.UpdateEv(tread)
		switch msg.Type {
		case "location":
			err = json.Unmarshal(msg.Data, &dr.loc)
			if err != nil {
				dr.log.Error().Err(err).Msg("error parsing location data")
				dr.closeErr(err)
				break
			}
			dr.log.Debug().Str("event", "location update").RawJSON("event_data", msg.Data).Msg("")
			dr.state.Sublist.MarshalSend(dr.tid, dr.loc.Latitude, dr.loc.Longitude, dr.loc.Speed, dr.loc.GpsTime, tread)
			dr.store.Put(fsn, dr.loc.Latitude, dr.loc.Longitude, dr.loc.Altitude, dr.loc.Speed, dr.loc.GpsTime, tread)
			dr.state.UpdateLocation(dr.loc.Longitude, dr.loc.Latitude, dr.loc.GpsTime.UTC())
		case "status":
			status := StatusData{}
			err = json.Unmarshal(msg.Data, &status)
			if err != nil {
				dr.log.Error().Err(err).Msg("error parsing status data")
				dr.closeErr(err)
				break
			}
			dr.log.Debug().Str("event", "status update").Str("status", status.Status).Msg("")
			dr.state.SetKV("status", status.Status)
		case "error":
			errdata := ErrorData{}
			err = json.Unmarshal(msg.Data, &errdata)
			if err != nil {
				dr.log.Error().Err(err).Msg("error parsing error data")
				dr.closeErr(err)
				break
			}
			dr.log.Error().Str("event", "error event").Str("message", errdata.ErrorMessage)
			dr.state.SetKV("error", errdata.ErrorMessage)
		}

	}
	// dr.state.Attached.Unlock()
}
