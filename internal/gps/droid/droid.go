package droid

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/phuslu/log"
	"nuha.dev/gpstracker/internal/gps/client"
	"nuha.dev/gpstracker/internal/gps/server"
	"nuha.dev/gpstracker/internal/store"
	"nuha.dev/gpstracker/internal/util/wc"
)

type runningState int

const (
	not_run runningState = iota
	running
	closing
	closed
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
	c   *wc.Conn
	s   server.ServerInterface
	tid uint64

	err     error
	log     log.Logger
	login   LoginData
	loc     LocationData
	store   store.Store
	session *client.ClientSession
	closer  *sync.Cond
	runningState
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
	o.closer = sync.NewCond(&sync.Mutex{})
	o.runningState = not_run
	return o
}

func (dr *Droid) closeAndSetErr(err error) {
	dr.err = err
	dr.c.Close()
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

func (dr *Droid) TryCloseWait() bool {
	dr.closer.L.Lock()

	if dr.runningState == closed {
		dr.closer.L.Unlock()
		return true
	} else if dr.runningState == closing {
		dr.closer.L.Unlock()
		return false
	} else {
		dr.runningState = closing
		dr.c.Close()
		for dr.runningState != closed {
			dr.closer.Wait()
		}
		dr.closer.L.Unlock()
		return true
	}
}

func (dr *Droid) ConnectionId() uint64 {
	return dr.c.Cid()
}
func (dr *Droid) Run() {
	go func() {
		dr.closer.L.Lock()
		dr.runningState = running
		dr.closer.L.Unlock()
		dr.run()
		dr.closer.L.Lock()
		if dr.runningState == closing {
			dr.runningState = closed
			dr.closer.Signal()
		} else {
			dr.runningState = closed
		}
		dr.closer.L.Unlock()
	}()
}

func (dr *Droid) run() {
	msg, err := dr.readParse()
	if err != nil {
		dr.log.Error().Err(err).Msg("error while reading message")
		dr.closeAndSetErr(err)
		return
	}

	if msg.Type != "login" {
		err := fmt.Errorf("first message not login message")
		dr.log.Error().Err(err).Msg("")
		dr.closeAndSetErr(err)
		return
	} else {
		err := json.Unmarshal(msg.Data, &dr.login)
		if err != nil {
			dr.log.Error().Err(err).Msg("error parsing login message")
			dr.closeAndSetErr(err)
			return
		}
	}

	sn, err := strconv.ParseUint(dr.login.Serial, 16, 64)
	if err != nil {
		dr.log.Error().Err(err).Msg("error parsing serial number")
		dr.closeAndSetErr(err)
		return
	}

	dr.log.Info().Str("event", "login").Str("sn_type", dr.login.SnType).Uint64("serial", sn).Msg("")
	ok, session, _ := dr.s.Login(dr.login.SnType, sn, dr)
	if ok {

		dr.tid = dr.session.TrackerId
		dr.log.Context = log.NewContext(dr.log.Context).Uint64("tid", dr.tid).Value()
		dr.log.Info().Msg("login successful")
		dr.session = session

	} else {
		dr.closeAndSetErr(errRejectedLogin)
		dr.log.Error().Err(errRejectedLogin).Msg("login rejected")
		return
	}

	// dr.state.Attached.Lock()
	fsn := dr.session.FSN
	for {

		msg, err := dr.readParse()
		tread := time.Now().UTC()
		if err != nil {
			dr.log.Error().Err(err).Msg("error while reading message")
			dr.closeAndSetErr(err)
			break
		}

		switch msg.Type {
		case "location":
			err = json.Unmarshal(msg.Data, &dr.loc)
			if err != nil {
				dr.log.Error().Err(err).Msg("error parsing location data")
				dr.closeAndSetErr(err)
				break
			}
			dr.log.Debug().Str("event", "location update").RawJSON("event_data", msg.Data).Msg("")
			dr.session.Sublist.MarshalSend(dr.tid, dr.loc.Latitude, dr.loc.Longitude, dr.loc.Speed, dr.loc.GpsTime, tread)
			dr.store.Put(fsn, dr.loc.Latitude, dr.loc.Longitude, dr.loc.Altitude, dr.loc.Speed, dr.loc.GpsTime, tread)
			dr.session.UpdateLocation(dr.loc.Longitude, dr.loc.Latitude, dr.loc.GpsTime.UTC())
		case "status":
			status := StatusData{}
			err = json.Unmarshal(msg.Data, &status)
			if err != nil {
				dr.log.Error().Err(err).Msg("error parsing status data")
				dr.closeAndSetErr(err)
				break
			}
			dr.log.Debug().Str("event", "status update").Str("status", status.Status).Msg("")

		case "error":
			errdata := ErrorData{}
			err = json.Unmarshal(msg.Data, &errdata)
			if err != nil {
				dr.log.Error().Err(err).Msg("error parsing error data")
				dr.closeAndSetErr(err)
				break
			}
			dr.log.Error().Str("event", "error event").Str("message", errdata.ErrorMessage)

		}

	}
	// dr.state.Attached.Unlock()
}
