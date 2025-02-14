package gt06

import (
	"encoding/binary"
	"encoding/json"
	"net"
	"strconv"
	"strings"
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

type GT06Param struct {
	Store     store.LocationStore
	MiscStore store.MiscStore
	Logger    log.Logger
	Sublist   *sublist.Sublist
}
type GT06 struct {
	c          *conn.Conn
	c_next     *conn.Conn
	c_mu       sync.RWMutex
	c_next_mu  sync.Mutex
	stopped    bool
	stopped_mu sync.Mutex
	attr       map[string]string
	err        error_state
	cmd        command_state
	msg        Message
	log        log.Logger
	ser        device.Serial
	tid        uint64
	conf       *device.DeviceConfig
	sublist    *sublist.Sublist
	store      store.LocationStore
	misc_store store.MiscStore
	offset     *time.Duration //offset from device

	runningState
	rs_mu sync.Mutex
	gt06_status
	gt06_location
}

const (
	command_submitted int = iota
	command_sent
	command_empty
)

type error_state struct {
	mu  sync.Mutex
	err error
	t   time.Time
}

type command_state struct {
	mu                  sync.Mutex
	sent_time           time.Time
	status              int
	server_flag_counter uint32
	serial_counter      int
	current_msg         string
	current_server_flag uint32
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

func NewGT06(tid uint64, ser device.Serial, c *conn.Conn, login_msg *LoginMessage, param *GT06Param, conf_attr *device.DeviceConfigAttribute) *GT06 {
	o := &GT06{c: c}
	o.log = param.Logger
	o.log.Context = log.NewContext(nil).Str("module", "gt06").EmbedObject(ser).Value()
	o.store = param.Store
	o.misc_store = param.MiscStore
	o.runningState = created
	o.msg.Buffer = make([]byte, 1000)
	o.offset = &login_msg.TimeOffset
	o.conf = conf_attr.Config
	o.sublist = param.Sublist
	o.tid = tid
	o.ser = ser
	o.attr = conf_attr.Attribute
	o.cmd.status = command_empty
	return o
}

func (gt06 *GT06) closeAndSetErr(err error, t time.Time) {
	gt06.err.mu.Lock()
	gt06.err.err = err
	gt06.err.t = t
	gt06.err.mu.Unlock()
	gt06.log.Error().Err(err).Str("event", CONNECTION_CLOSED).Msg("connection closed caused by error")
	gt06.c.Close()
}

func (gt06 *GT06) writeResponse(protocol byte, payload []byte, serial int) error {
	gt06.log.Trace().Str("proccode", strconv.FormatUint(uint64(protocol), 16)).Hex("payload", payload).Int("serial", serial).Msg("writing response")
	// gt06.c_mu.RLock()
	// defer gt06.c_mu.RUnlock()
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
	go gt06._run()
}

func (gt06 *GT06) Stop() {
	gt06.stop()
	gt06.c.Close()
}

func (gt06 *GT06) stop() {
	gt06.stopped_mu.Lock()
	gt06.stopped = true
	gt06.stopped_mu.Unlock()
}

func (gt06 *GT06) is_stopped() bool {
	gt06.stopped_mu.Lock()
	f := gt06.stopped
	gt06.stopped_mu.Unlock()
	return f
}

func (gt06 *GT06) _run() {

	defer func() {
		gt06.rs_mu.Lock()
		gt06.runningState = paused
		gt06.rs_mu.Unlock()
		gt06.log.Info().Msg("exit from goroutine runloop")
	}()

	gt06.rs_mu.Lock()
	gt06.runningState = running
	gt06.rs_mu.Unlock()
	for {
		gt06.run() //will block
		if gt06.is_stopped() {
			break
		}
		ok := gt06.use_next_conn()
		if ok {
			time.Sleep(10 * time.Second)
			continue
		} else {
			break
		}
	}
}

// func (gt06 *GT06) SendMessage(msg string, server_flag uint32, serial int) error {
// 	gt06.c_mu.RLock()
// 	defer gt06.c_mu.RUnlock()
// 	sf := make([]byte, 4)
// 	binary.BigEndian.PutUint32(sf, server_flag)
// 	d := newCommand(msg, sf, serial)
// 	gt06.log.Info().Hex("sending command", d).Msg("")
// 	n, err := gt06.c.Write(d)
// 	gt06.log.Info().Msgf("command sent %d", n)
// 	if err != nil {
// 		return err
// 	} else {
// 		return nil
// 	}
// }

func (gt06 *GT06) Error() (error, time.Time) {
	gt06.err.mu.Lock()
	defer gt06.err.mu.Unlock()
	return gt06.err.err, gt06.err.t
}

func (gt06 *GT06) SendCommand(msg string, force bool) (bool, error) {
	return gt06._send_command(msg, force)
}

func (gt06 *GT06) _send_command(msg string, force bool) (bool, error) {
	gt06.cmd.mu.Lock()
	if gt06.cmd.status != command_empty {
		if !force {
			gt06.cmd.mu.Unlock()
			return true, nil
		} else {
			gt06.log.Warn().Msg("there is pending command")
		}
	}
	gt06.cmd.server_flag_counter++
	gt06.cmd.serial_counter++
	server_flag := gt06.cmd.server_flag_counter
	serial := gt06.cmd.serial_counter
	gt06.cmd.current_server_flag = server_flag
	gt06.cmd.current_msg = msg
	sf := make([]byte, 4)
	binary.BigEndian.PutUint32(sf, server_flag)
	d := newCommand(msg, sf, serial)
	gt06.cmd.status = command_submitted
	gt06.cmd.mu.Unlock()

	gt06.c_mu.RLock()
	defer gt06.c_mu.RUnlock()
	gt06.log.Trace().Hex("sending command", d).Msg("")
	n, err := gt06.c.Write(d)
	if err != nil {
		gt06.log.Error().Err(err).Msg("error when sending command")
		return false, err
	} else {
		gt06.log.Trace().Msgf("command sent %d", n)
		t := time.Now().UTC()
		gt06.cmd.mu.Lock()
		gt06.cmd.status = command_sent
		gt06.cmd.sent_time = t
		gt06.cmd.mu.Unlock()
		gt06.misc_store.SaveEvent(gt06.tid, "command.sent", msg, map[string]uint32{"server_flag": server_flag}, t)
		return false, nil
	}
}

func (gt06 *GT06) set_next_conn(c *conn.Conn) {
	gt06.c_next_mu.Lock()
	gt06.c_next = c
	gt06.c_next_mu.Unlock()
}

func (gt06 *GT06) set_conn(c *conn.Conn) {
	gt06.c_mu.Lock()
	gt06.c = c
	gt06.c_mu.Unlock()
}

func (gt06 *GT06) use_next_conn() bool {
	gt06.c_next_mu.Lock()
	defer gt06.c_next_mu.Unlock()

	if gt06.c_next == nil {
		return false
	} else {
		gt06.c_mu.Lock()
		defer gt06.c_mu.Unlock()
		gt06.c = gt06.c_next
		gt06.c_next = nil
		return true
	}
}

func (gt06 *GT06) ReplaceConn(c *conn.Conn) {
	gt06.rs_mu.Lock()
	if gt06.runningState == running {
		gt06.set_next_conn(c)
		gt06.rs_mu.Unlock()
		gt06.log.Info().Str("event", CONNECTION_CLOSED).Msg("closing replaced connection")
		gt06.c.Close()

	} else if gt06.runningState == paused {
		gt06.set_conn(c)
		gt06.rs_mu.Unlock()
		go gt06._run()
	}
}

func (gt06 *GT06) handle_heartbeat(si statusInfo, t time.Time) {
	changed := false
	gt06.gt06_status.mu.Lock()
	if gt06.gt06_status.si != si {
		changed = true
	}
	gt06.gt06_status.time = t
	gt06.gt06_status.si = si
	gt06.gt06_status.mu.Unlock()

	if changed {
		gt06.log.Info().Object("status", &si).Msg("status changed")
		gt06.misc_store.SaveEvent(gt06.tid, "hearbeat.changed", "", si, t)
		buf, _ := json.Marshal(si)
		gt06.sublist.SendEvent("heartbeat.changed", buf, t)
	}
}

func (gt06 *GT06) handle_alarm(si statusInfo, t time.Time) {
	gt06.gt06_status.mu.Lock()
	gt06.gt06_status.time = t
	gt06.gt06_status.si = si
	gt06.gt06_status.mu.Unlock()
	gt06.misc_store.SaveEvent(gt06.tid, "alarm", "", si, t)
	buf, _ := json.Marshal(si)
	gt06.sublist.SendEvent("alarm", buf, t)
}

func (gt06 *GT06) handle_diconnection(t time.Time) {
	gt06.misc_store.SaveEvent(gt06.tid, "disconnected", "", nil, t)
	gt06.sublist.SendEvent("disconnected", []byte{}, t)
}

func (gt06 *GT06) event_run(t time.Time) {
	gt06.misc_store.SaveEvent(gt06.tid, "started", "", nil, t)
	gt06.sublist.SendEvent("started", []byte{}, t)
}

func (gt06 *GT06) handle_location(loc gt06GPSMessage, t time.Time) {
	if gt06.conf.Store {
		gt06.store.Put(gt06.ser.Nsn(), loc.Latitude, loc.Longitude, -1, loc.Speed, loc.Timestamp, t)
	}
	if gt06.conf.SublistSend {
		gt06.sublist.SendLocation(loc.Latitude, loc.Longitude, loc.Speed, loc.Timestamp, t)
	}
	cell_info_changed := false

	gt06.gt06_location.mu.Lock()
	if loc.gt06CellInfo != gt06.gt06_location.loc.gt06CellInfo {
		cell_info_changed = true
	}
	gt06.gt06_location.loc = loc
	gt06.gt06_location.time = t
	gt06.gt06_location.mu.Unlock()

	if cell_info_changed {
		gt06.log.Info().Object("cell_info", &loc.gt06CellInfo).Msg("cell info changed")
		gt06.misc_store.SaveEvent(gt06.tid, "cell_info.changed", "", loc.gt06CellInfo, t)
	}
}

func (gt06 *GT06) GetLocation() device.Location {
	gt06.gt06_location.mu.Lock()
	defer gt06.gt06_location.mu.Unlock()
	loc := device.Location{}
	loc.Speed = gt06.gt06_location.loc.Speed
	loc.Longitude = gt06.gt06_location.loc.Longitude
	loc.Latitude = gt06.gt06_location.loc.Latitude
	loc.Timestamp = gt06.gt06_location.loc.Timestamp
	loc.Altitude = 0
	return loc
}

func should_update_attribute(cmd string) bool {
	if strings.EqualFold(cmd, "VERSION#") {
		return true
	} else if strings.EqualFold(cmd, "PARAM#") {
		return true
	}
	return false
}

func (gt06 *GT06) handle_command_response(cmd_response CommandResponseMessage, t time.Time) {
	flag_matched := false
	do_update_attr := false
	cmd := ""
	gt06.cmd.mu.Lock()
	if cmd_response.ServerFlag == gt06.cmd.current_server_flag {
		flag_matched = true
		gt06.cmd.status = command_empty
		do_update_attr = should_update_attribute(gt06.cmd.current_msg)
		cmd = gt06.cmd.current_msg
	} else {
		gt06.log.Error().Msgf("expecting response with server_flag %d, got %d", gt06.cmd.current_server_flag, cmd_response.ServerFlag)
	}
	gt06.cmd.mu.Unlock()
	gt06.misc_store.SaveEvent(gt06.tid, "command.response", cmd_response.Message, map[string]uint32{
		"server_flag": cmd_response.ServerFlag}, t)
	if flag_matched {
		gt06.misc_store.SaveCommandResponse(gt06.tid, cmd_response.ServerFlag, gt06.cmd.current_msg, gt06.cmd.sent_time, cmd_response.Message, t)
	}
	if do_update_attr {
		gt06.misc_store.UpdateAttribute(gt06.tid, strings.ToUpper(cmd), cmd_response.Message)
	}

}

func (gt06 *GT06) CurrentConnInfo() []string {
	gt06.c_mu.RLock()
	defer gt06.c_mu.RUnlock()
	return gt06.c.ConnAddr()
}

func (gt06 *GT06) run() {
	// var prev_procotol byte
	gt06.c_mu.RLock()
	gt06.event_run(time.Now())
	defer func() {
		gt06.c_mu.RUnlock()
		gt06.log.Info().Msg("exit from readMessage loop")
	}()
	timeout_ping_sent := false
	for {
		err := gt06.readMessage()

		if err != nil {
			if neterr, ok := err.(net.Error); ok && neterr.Timeout() {
				if timeout_ping_sent {
					gt06.handle_diconnection(time.Now())
					gt06.closeAndSetErr(err, time.Now())
					return
				} else {
					timeout_ping_sent = true
					_, err := gt06._send_command("STATUS#", true)
					if err != nil {
						gt06.handle_diconnection(time.Now())
						gt06.closeAndSetErr(err, time.Now())
						return
					}
				}
			} else {
				gt06.handle_diconnection(time.Now())
				gt06.closeAndSetErr(err, time.Now())
				return
			}

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
				gt06.closeAndSetErr(err, time.Now())
				return
			}
		case byte(statusInformation): //heartbeat
			st := parseStatusInformation(gt06.msg.Payload)
			err := gt06.writeResponse(statusInformation, []byte{}, gt06.msg.Serial)
			if err != nil {
				gt06.closeAndSetErr(err, time.Now())
				return
			}
			gt06.handle_heartbeat(st, tread)
			gt06.log.Debug().Str("procode", procode).Object("status_info", &st).Msg("heartbeat")

		case byte(gk310GPS):
			loc := parseGK310GPSMessage(gt06.msg.Payload)
			gt06.handle_location(loc.gt06GPSMessage, tread)
			gt06.log.Debug().Str("procode", procode).Msg("location update")
		case byte(gt06GPS):
			loc := parseGT06GPSMessage(gt06.msg.Payload)
			gt06.handle_location(loc, tread)
			gt06.log.Debug().Str("procode", procode).Msg("location update")
		case byte(gk310GPSAlarm):
			gpsalm := parseGPSAlarm(gt06.msg.Payload, time.UTC)
			gt06.handle_location(gpsalm.gt06GPSMessage, tread)
			gt06.handle_alarm(gpsalm.statusInfo, tread)
			gt06.log.Info().Str("procode", procode).Msg("location update with alarm")
		case byte(gt06GPSAlarm):
			gpsalm := parseGPSAlarm(gt06.msg.Payload, time.Local)
			gt06.handle_location(gpsalm.gt06GPSMessage, tread)
			gt06.handle_alarm(gpsalm.statusInfo, tread)
			gt06.log.Info().Str("procode", procode).Msg("location update with alarm")
		case byte(informationTxPacket):
			subprocode := strconv.FormatUint(uint64(gt06.msg.Payload[0]), 16)
			switch gt06.msg.Payload[0] {
			case 0x04:
				gt06.log.Info().Str("procode", procode).Str("subprocode", subprocode).Str("message", string(gt06.msg.Payload[1:])).Msg("information packet : terminal status synchronization")
				gt06.misc_store.UpdateAttribute(gt06.tid, "terminal_status", string(gt06.msg.Payload[1:]))

			case 0x0a:
				device_sn := parseDeviceSn(gt06.msg.Payload[1:])
				gt06.log.Info().Object("device_sn", &device_sn).Msg("information packet : terminal device sn info")
				gt06.misc_store.UpdateAttribute(gt06.tid, "iccid", device_sn.iccid)
				gt06.misc_store.UpdateAttribute(gt06.tid, "imei", device_sn.imei)
				gt06.misc_store.UpdateAttribute(gt06.tid, "imsi", device_sn.imsi)
			default:
				gt06.log.Info().Str("procode", procode).Str("subprocode", subprocode).Hex("data", gt06.msg.Payload[1:]).Msg("information packet : unknown sub protocol code")
			}
		case byte(serverCommandResponse):
			cmdRes := parsegk310CommandResponse(gt06.msg.Payload)
			gt06.handle_command_response(cmdRes, tread)
			gt06.log.Info().Uint32("server_flag", cmdRes.ServerFlag).Str("message", cmdRes.Message).Msg("command response")

		case byte(stringInformation):
			cmdRes := parsegt06CommandResponse(gt06.msg.Payload)
			gt06.handle_command_response(cmdRes, tread)
			gt06.log.Info().Uint32("server_flag", cmdRes.ServerFlag).Str("message", cmdRes.Message).Msg("command response")

		default:
			gt06.log.Error().Hex("data", gt06.msg.Payload).Str("procode", procode).Str("error", "unknown event protocol").Msg("unhandled event protocol")
		}
		// prev_procotol = gt06.msg.Protocol
	}

}

func (gt06 *GT06) readMessage() error {
	// gt06.c_mu.RLock()
	// defer gt06.c_mu.RUnlock()
	minutes := gt06.conf.ReadDeadline
	_ = gt06.c.SetReadDeadline(time.Now().Add(time.Duration(minutes) * time.Minute))
	return readMessage(gt06.c, &gt06.msg)
}
