package simplejson

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/phuslu/log"

	"nuha.dev/gpstracker/internal/gpsv2/conn"
	"nuha.dev/gpstracker/internal/gpsv2/device"
	"nuha.dev/gpstracker/internal/gpsv2/sublist"
	"nuha.dev/gpstracker/internal/store"
)

type runningState int

const (
	CONNECTION_CLOSED string = "connection_closed"
)

const (
	created runningState = iota
	running
	paused
)

type SimpleJSON struct {
	conf       *device.DeviceConfig
	c          *conn.Conn
	c_next     *conn.Conn
	c_mu       sync.RWMutex
	c_next_mu  sync.Mutex
	stopped    bool
	stopped_mu sync.Mutex

	fsn     string
	err     error
	log     log.Logger
	store   store.LocationStore
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

func NewSimpleJSON(c *conn.Conn, store store.LocationStore, logger log.Logger, login_msg *LoginMessage, sublist *sublist.Sublist, conf *device.DeviceConfig) *SimpleJSON {
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

func (j *SimpleJSON) set_next_conn(c *conn.Conn) {
	j.c_next_mu.Lock()
	j.c_next = c
	j.c_next_mu.Unlock()
}

func (j *SimpleJSON) set_conn(c *conn.Conn) {
	j.c_mu.Lock()
	j.c = c
	j.c_mu.Unlock()
}

func (j *SimpleJSON) use_next_conn() bool {
	j.c_next_mu.Lock()
	defer j.c_next_mu.Unlock()

	if j.c_next == nil {
		return false
	} else {
		j.c_mu.Lock()
		defer j.c_mu.Unlock()
		j.c = j.c_next
		return true
	}
}

func (j *SimpleJSON) ReplaceConn(c *conn.Conn) {
	j.log.Info().Str("event", CONNECTION_CLOSED).Msg("closing replaced connection")
	j.rs_mu.Lock()
	if j.runningState == running {
		j.set_next_conn(c)
		j.rs_mu.Unlock()
		j.log.Info().Str("event", CONNECTION_CLOSED).Msg("closing replaced connection")
		j.c.Close()

	} else if j.runningState == paused {
		j.set_conn(c)
		j.rs_mu.Unlock()
		go j._run()
	}
}

func (j *SimpleJSON) Run() {

	j.rs_mu.Lock()
	j.runningState = running
	j.rs_mu.Unlock()
	go j._run()

}

func (j *SimpleJSON) Stop() {
	j.stop()
	j.c.Close()
}

func (j *SimpleJSON) stop() {
	j.stopped_mu.Lock()
	j.stopped = true
	j.stopped_mu.Unlock()
}

func (j *SimpleJSON) is_stopped() bool {
	j.stopped_mu.Lock()
	f := j.stopped
	j.stopped_mu.Unlock()
	return f
}

func (j *SimpleJSON) _run() {

	defer func() {
		j.rs_mu.Lock()
		j.runningState = paused
		j.rs_mu.Unlock()
		j.log.Info().Msg("exit from goroutine runloop")
	}()

	j.rs_mu.Lock()
	j.runningState = running
	j.rs_mu.Unlock()
	for {
		j.run() //will block
		if j.is_stopped() {
			break
		}
		ok := j.use_next_conn()
		if ok {
			continue
		} else {
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
