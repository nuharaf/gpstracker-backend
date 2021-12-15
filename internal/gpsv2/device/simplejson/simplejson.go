package simplejson

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/phuslu/log"

	"nuha.dev/gpstracker/internal/gpsv2/conn"
	"nuha.dev/gpstracker/internal/gpsv2/device"
	"nuha.dev/gpstracker/internal/gpsv2/sublist"
	"nuha.dev/gpstracker/internal/gpsv2/subscriber"
	"nuha.dev/gpstracker/internal/store"
)

type runningState int

const (
	created runningState = iota
	running
	paused
)

type SimpleJSON struct {
	conf    *device.DeviceConfig
	c       *conn.Conn
	c_next  *conn.Conn
	tid     uint64
	fsn     string
	err     error
	log     log.Logger
	store   store.Store
	msg     FrameMessage
	sublist *sublist.Sublist
	runningState
	rs_mu sync.Mutex
	lastMsg
	parsedMsg
}

type parsedMsg struct {
	loc    LocationMessage
	status StatusMessage
	sat    []Sat
}

type lastMsg struct {
	loc_mu   sync.Mutex
	loc      LocationMessage
	loc_time time.Time

	status_mu   sync.Mutex
	status      StatusMessage
	status_time time.Time

	gps_err_mu   sync.Mutex
	gps_err_time time.Time

	gps_init_mu   sync.Mutex
	gps_init_time time.Time

	sat_mu   sync.Mutex
	sat      []Sat
	sat_time time.Time
}

func NewSimpleJSON(c *conn.Conn, store store.Store, logger log.Logger, login_msg *LoginMessage, sublist *sublist.Sublist, conf *device.DeviceConfig) *SimpleJSON {
	o := &SimpleJSON{c: c}
	o.log = logger
	o.log.Context = log.NewContext(nil).Str("module", "simplejson").Value()
	o.store = store
	o.runningState = created
	o.msg.Buffer = make([]byte, 1000)
	o.parsedMsg.sat = make([]Sat, 0, 100)
	o.lastMsg.sat = make([]Sat, 0, 100)
	o.conf = conf
	o.sublist = sublist
	return o
}

func (j *SimpleJSON) closeAndSetErr(err error) {
	j.err = err
	j.c.Close()
}

func (j *SimpleJSON) ReplaceConn(c *conn.Conn) {
	j.rs_mu.Lock()
	if j.runningState == running {
		j.c_next = c
		j.c.Close()
		j.rs_mu.Unlock()
	} else if j.runningState == paused {
		j.c = c
		j.runningState = running
		go j._run()
	}

}

func (j *SimpleJSON) Subscribe(sub subscriber.Subscriber) {
	j.sublist.Subscribe(sub)
}

func (j *SimpleJSON) Unsubscribe(sub subscriber.Subscriber) {
	j.sublist.Unsubscribe(sub)
}

func (j *SimpleJSON) Run() {

	j.rs_mu.Lock()
	j.runningState = running
	j.rs_mu.Unlock()
	go j._run()

}

func (j *SimpleJSON) _run() {
	for {
		j.run()
		j.rs_mu.Lock()
		if j.c_next != nil {
			j.c = j.c_next
			j.c_next = nil
			j.rs_mu.Unlock()
			continue

		} else {
			j.runningState = paused
			j.rs_mu.Unlock()
			break
		}

	}
}

func (j *SimpleJSON) run() {

	for {

		err := readMessage(j.c, &j.msg)
		if err != nil {
			j.log.Error().Err(err).Msg("error while reading message")
			j.closeAndSetErr(err)
			return
		}
		tread := time.Now().UTC()
		switch j.msg.Protocol {
		case LOCATION_UPDATE:
			var loc LocationMessage = j.parsedMsg.loc
			err = json.Unmarshal(j.msg.Payload, &loc)
			if err != nil {
				j.log.Error().Err(err).Msg("error parsing location data")
				j.closeAndSetErr(err)
				break
			}
			j.lastMsg.loc_mu.Lock()
			j.lastMsg.loc_time = tread
			j.lastMsg.loc = loc
			j.lastMsg.loc_mu.Unlock()
			j.sublist.MarshalSend(loc.Latitude, loc.Longitude, loc.Speed, loc.GpsTime, tread)
			if j.conf.Store {
				j.store.Put(j.fsn, loc.Latitude, loc.Longitude, loc.Altitude, loc.Speed, loc.GpsTime, tread)
			}

		case STATUS:
			var status = j.parsedMsg.status
			err = json.Unmarshal(j.msg.Payload, &status)
			if err != nil {
				j.log.Error().Err(err).Msg("error parsing status data")
				j.closeAndSetErr(err)
				break
			}
			j.lastMsg.status_mu.Lock()
			j.lastMsg.status_time = tread
			j.lastMsg.status = status
			j.lastMsg.status_mu.Unlock()

		case SAT_UPDATE:

			err = json.Unmarshal(j.msg.Payload, j.parsedMsg.sat)
			if err != nil {
				j.log.Error().Err(err).Msg("error parsing status data")
				j.closeAndSetErr(err)
				break
			}
			j.lastMsg.sat_mu.Lock()
			j.lastMsg.sat_time = tread
			j.lastMsg.sat = j.lastMsg.sat[:len(j.parsedMsg.sat)]
			copy(j.lastMsg.sat, j.parsedMsg.sat)
			j.lastMsg.sat_mu.Unlock()

		case GPS_ERROR:
			j.lastMsg.gps_err_mu.Lock()
			j.lastMsg.gps_err_time = tread
			j.lastMsg.gps_err_mu.Unlock()

		case GPS_INIT:
			j.lastMsg.gps_init_mu.Lock()
			j.lastMsg.gps_init_time = tread
			j.lastMsg.gps_init_mu.Unlock()

		}

	}
	// dr.state.Attached.Unlock()
}
