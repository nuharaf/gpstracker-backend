package gt06

import (
	"encoding/binary"
	"strconv"
	"sync"
	"time"

	"github.com/phuslu/log"
	"nuha.dev/gpstracker/internal/gpsv2/conn"
	"nuha.dev/gpstracker/internal/gpsv2/device"
	"nuha.dev/gpstracker/internal/gpsv2/sublist"

	// "nuha.dev/gpstracker/internal/gpsv2/msgstore"
	"nuha.dev/gpstracker/internal/store"
)

type runningState int

const (
	created runningState = iota
	running
	paused
)

const (
	CONNECTION_CLOSED string = "connection_closed"
)

// type _msg struct {
// 	subid uint64 //Submission Id
// 	msg   string
// }

// type gt06Api struct {
// 	log      log.Logger
// 	fsn      string
// 	tid      uint64
// 	msgstore msgstore.MsgStore
// }

// type pending_msg struct {
// 	server_flag uint32
// 	mu          sync.Mutex
// 	serial      int
// 	msg         string
// 	ready       bool
// 	delivered   bool
// 	cancelled   bool
// }

type GT06 struct {
	c       *conn.Conn
	c_next  *conn.Conn
	c_mu    sync.RWMutex
	err     error
	cmd     *string
	cmd_mu  sync.Mutex
	msg     Message
	log     log.Logger
	fsn     string
	tid     uint64
	conf    *device.DeviceConfig
	sublist *sublist.Sublist
	store   store.Store
	offset  *time.Duration //offset from device
	runningState
	rs_mu sync.Mutex
	gt06_status
	gt06_location
}

type gt06_status struct {
	mu   sync.Mutex
	si   statusInfo
	time time.Time
}

type gt06_location struct {
	mu   sync.Mutex
	loc  gt06GPSMessage
	time time.Time
}

// type lastMsg struct {
// 	si_mu   sync.Mutex
// 	si      statusInfo
// 	si_time time.Time

// 	loc_mu   sync.Mutex
// 	locgt06  gt06GPSMessage
// 	locgk310 gk310GPSMessage
// 	loc_time time.Time
// 	hasgk310 bool

// 	status_sync_mu   sync.Mutex
// 	status_sync      string
// 	status_sync_time time.Time

// 	device_sn_mu   sync.Mutex
// 	device_sn      deviceSn
// 	device_sn_time time.Time
// }

func NewGT06(tid uint64, fsn string, c *conn.Conn, store store.Store, logger log.Logger, login_msg *LoginMessage, sublist *sublist.Sublist, conf *device.DeviceConfig) *GT06 {
	o := &GT06{c: c, store: store}
	o.log = logger
	o.log.Context = log.NewContext(nil).Str("module", "gt06").Str("fsn", fsn).Value()
	o.store = store
	o.runningState = created
	o.msg.Buffer = make([]byte, 1000)
	o.offset = &login_msg.TimeOffset
	o.conf = conf
	o.sublist = sublist
	o.tid = tid
	o.fsn = fsn
	return o
}

func (gt06 *GT06) closeAndSetErr(err error) {
	gt06.err = err
	gt06.log.Error().Err(err).Str("event", CONNECTION_CLOSED).Msg("connection closed caused by error")
	gt06.c.Close()
}

func (gt06 *GT06) writeResponse(protocol byte, payload []byte, serial int) error {
	gt06.log.Trace().Str("proccode", strconv.FormatUint(uint64(protocol), 16)).Hex("payload", payload).Int("serial", serial).Msg("writing response")
	gt06.c_mu.RLock()
	defer gt06.c_mu.RUnlock()
	return gt06.write(newFrame(protocol, payload, serial))
}

func timeResponse(t *time.Time) []byte {
	payload := []byte{byte(t.Year() % 100), byte(t.Month()), byte(t.Day()), byte(t.Hour()), byte(t.Minute()), byte(t.Second())}
	return payload
}

func (gt06 *GT06) write(d []byte) error {
	_, err := gt06.c.Write(d)
	if err != nil {
		gt06.log.Error().Err(err).Msg("Error while writing data")
		return err
	} else {
		return nil
	}
}

func (gt06 *GT06) Run() {

	gt06.rs_mu.Lock()
	gt06.runningState = running
	gt06.rs_mu.Unlock()
	go gt06._run()

}

func (gt06 *GT06) _run() {
	for {
		gt06.c_mu.RLock()
		gt06.run()
		gt06.c_mu.RUnlock()
		gt06.c_mu.Lock()
		if gt06.c_next != nil {
			gt06.c = gt06.c_next
			gt06.c_next = nil
			gt06.c_mu.Unlock()
			continue
		} else {
			gt06.c_mu.Unlock()
			gt06.rs_mu.Lock()
			gt06.runningState = paused
			gt06.rs_mu.Unlock()
			break
		}
	}
}

func (gt06 *GT06) SendMessage(msg string, server_flag uint32, serial int) error {
	gt06.c_mu.RLock()
	defer gt06.c_mu.RUnlock()
	sf := make([]byte, 4)
	binary.BigEndian.PutUint32(sf, server_flag)
	d := newCommand(msg, sf, serial)
	gt06.log.Info().Hex("sending command", d).Msg("")
	n, err := gt06.c.Write(d)
	gt06.log.Info().Msgf("command sent %d", n)
	if err != nil {
		return err
	} else {
		return nil
	}
}

func (gt06 *GT06) ReplaceConn(c *conn.Conn) {
	gt06.rs_mu.Lock()
	if gt06.runningState == running {
		gt06.log.Info().Str("event", CONNECTION_CLOSED).Msg("closing replaced connection")
		gt06.c_next = c
		gt06.c.Close()
		gt06.rs_mu.Unlock()
	} else if gt06.runningState == paused {
		gt06.c = c
		gt06.runningState = running
		go gt06._run()
	}

}

func (gt06 *GT06) update_status_info(si statusInfo, t time.Time) {

	gt06.gt06_status.mu.Lock()
	if gt06.gt06_status.si != si {
		gt06.log.Info().Object("status", &si).Msg("status changed")
	}
	gt06.gt06_status.time = t
	gt06.gt06_status.si = si
	gt06.gt06_status.mu.Unlock()

}

func (gt06 *GT06) update_location(loc gt06GPSMessage, t time.Time) {

	gt06.gt06_location.mu.Lock()
	gt06.gt06_location.loc = loc
	gt06.gt06_location.time = t
	gt06.gt06_location.mu.Unlock()

}

func (gt06 *GT06) run() {
	// var prev_procotol byte

	for {
		err := gt06.readMessage()
		if err != nil {
			gt06.closeAndSetErr(err)
			return
		}
		tread := time.Now().UTC()
		procode := strconv.FormatUint(uint64(gt06.msg.Protocol), 16)
		gt06.log.Trace().Str("procode", procode).Hex("payload", gt06.msg.Payload).Int("serial", gt06.msg.Serial).Msg("receive message from terminal")
		switch gt06.msg.Protocol {
		case byte(timeCheck):
			gt06.log.Info().Str("procode", procode).Msg("terminal time update")
			t := time.Now().UTC()
			gt06.log.Info().Str("procode", procode).Time("update", t).Msg("sending time response")
			err := gt06.writeResponse(timeCheck, timeResponse(&t), gt06.msg.Serial)
			if err != nil {
				gt06.closeAndSetErr(err)
				return
			}
		case byte(statusInformation): //heartbeat
			st := parseStatusInformation(gt06.msg.Payload)
			err := gt06.writeResponse(statusInformation, []byte{}, gt06.msg.Serial)
			if err != nil {
				gt06.closeAndSetErr(err)
				return
			}
			gt06.update_status_info(st, tread)

		case byte(gk310GPS):
			loc := parseGK310GPSMessage(gt06.msg.Payload)
			if gt06.conf.Store {
				gt06.store.Put(gt06.fsn, loc.Latitude, loc.Longitude, -1, loc.Speed, loc.Timestamp, tread)
			}
			gt06.sublist.MarshalSend(loc.Latitude, loc.Longitude, loc.Speed, loc.Timestamp, tread)
			gt06.update_location(loc.gt06GPSMessage, tread)

		case byte(gt06GPS):
			loc := parseGT06GPSMessage(gt06.msg.Payload)

			if gt06.conf.Store {
				gt06.store.Put(gt06.fsn, loc.Latitude, loc.Longitude, -1, loc.Speed, loc.Timestamp, tread)
			}
			gt06.sublist.MarshalSend(loc.Latitude, loc.Longitude, loc.Speed, loc.Timestamp, tread)
			gt06.update_location(loc, tread)
		case byte(gk310GPSAlarm):
			gpsalm := parseGPSAlarm(gt06.msg.Payload, time.UTC)
			loc := gpsalm.gt06GPSMessage
			if gt06.conf.Store {
				gt06.store.Put(gt06.fsn, loc.Latitude, loc.Longitude, -1, loc.Speed, loc.Timestamp, tread)
			}
			gt06.sublist.MarshalSend(loc.Latitude, loc.Longitude, loc.Speed, loc.Timestamp, tread)
			gt06.update_location(loc, tread)
			gt06.update_status_info(gpsalm.statusInfo, tread)
		case byte(gt06GPSAlarm):
			gpsalm := parseGPSAlarm(gt06.msg.Payload, time.Local)
			loc := gpsalm.gt06GPSMessage
			if gt06.conf.Store {
				gt06.store.Put(gt06.fsn, loc.Latitude, loc.Longitude, -1, loc.Speed, loc.Timestamp, tread)
			}
			gt06.sublist.MarshalSend(loc.Latitude, loc.Longitude, loc.Speed, loc.Timestamp, tread)
			gt06.update_location(loc, tread)
			gt06.update_status_info(gpsalm.statusInfo, tread)
		case byte(informationTxPacket):
			subprocode := strconv.FormatUint(uint64(gt06.msg.Payload[0]), 16)
			switch gt06.msg.Payload[0] {
			case 0x04:
				gt06.log.Trace().Str("procode", procode).Str("subprocode", subprocode).Str("message", string(gt06.msg.Payload[1:])).Msg("information packet : terminal status synchronization")
			case 0x0a:
				device_sn := parseDeviceSn(gt06.msg.Payload[1:])
				gt06.log.Trace().Object("device_sn", &device_sn).Msg("information packet : terminal device sn info")
			default:
				gt06.log.Info().Str("procode", procode).Str("subprocode", subprocode).Hex("data", gt06.msg.Payload[1:]).Msg("information packet : unknown sub protocol code")
			}
		case byte(serverCommandResponse):
			cmdRes := parsegk310CommandResponse(gt06.msg.Payload)
			gt06.log.Info().Uint32("server_flag", cmdRes.ServerFlag).Str("message", cmdRes.Message).Msg("command response")
		case byte(stringInformation):
			cmdRes := parsegt06CommandResponse(gt06.msg.Payload)
			gt06.log.Info().Uint32("server_flag", cmdRes.ServerFlag).Str("message", cmdRes.Message).Msg("command response")
		default:
			gt06.log.Error().Hex("data", gt06.msg.Payload).Str("procode", procode).Str("error", "unknown event protocol").Msg("unhandled event protocol")
		}
		// prev_procotol = gt06.msg.Protocol
	}

}

func (gt06 *GT06) readMessage() error {
	gt06.c_mu.RLock()
	defer gt06.c_mu.RUnlock()
	return readMessage(gt06.c, &gt06.msg)
}
