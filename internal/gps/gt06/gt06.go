package gt06

import (
	"encoding/binary"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/phuslu/log"
	"nuha.dev/gpstracker/internal/gps/client"
	"nuha.dev/gpstracker/internal/gps/server"
	"nuha.dev/gpstracker/internal/gps/subscriber"
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

type GT06 struct {
	c       *wc.Conn
	err     error
	buffer  []byte
	msg     message
	log     log.Logger
	server  server.ServerInterface
	tid     uint64
	session *client.ClientSession
	store   store.Store
	closer  *sync.Cond
	offset  *time.Duration //offset from device
	runningState
	conn_stat
	lastStatusInfo          *statusInfo
	lastGK310Location       *gk310GPSMessage
	lastGT06Location        *gt06GPSMessage
	lastInformationTxPacket string
}

type conn_stat struct {
	mu              sync.Mutex
	connect_time    time.Time
	login_time      time.Time
	disconnect_time time.Time
}

var errRejectedLogin = errors.New("login rejected")

func NewGT06(c *wc.Conn, server server.ServerInterface, store store.Store) *GT06 {
	o := &GT06{c: c, buffer: make([]byte, 1000), store: store}

	o.log = log.DefaultLogger
	o.log.Context = log.NewContext(nil).Str("module", "gt06").Uint64("cid", c.Cid()).Value()
	o.store = store
	o.server = server
	o.runningState = not_run
	o.closer = sync.NewCond(&sync.Mutex{})
	o.conn_stat.connect_time = c.Created()
	return o
}

func (gt06 *GT06) closeAndSetErr(err error) {
	gt06.err = err
	gt06.c.Close()
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

func (gt06 *GT06) Subscribe(sub subscriber.Subscriber) {
	gt06.session.Sublist.Subscribe(sub)
}

// func (gt06 *GT06) SetState(state *client.ClientSession) {
// 	gt06.state = state
// }

func (gt06 *GT06) Closed() bool {
	gt06.closer.L.Lock()
	defer gt06.closer.L.Unlock()
	return gt06.runningState == closed
}

func (gt06 *GT06) TryCloseWait() bool {
	gt06.closer.L.Lock()

	if gt06.runningState == closed {
		gt06.closer.L.Unlock()
		return true
	} else if gt06.runningState == closing {
		gt06.closer.L.Unlock()
		return false
	} else {
		gt06.runningState = closing
		gt06.c.Close()
		for gt06.runningState != closed {
			gt06.closer.Wait()
		}
		gt06.closer.L.Unlock()
		return true
	}
}

func (gt06 *GT06) ConnectionId() uint64 {
	return gt06.c.Cid()
}

func (gt06 *GT06) Run() {
	go func() {
		gt06.closer.L.Lock()
		gt06.runningState = running
		gt06.closer.L.Unlock()
		gt06.run()
		gt06.closer.L.Lock()
		if gt06.runningState == closing {
			gt06.runningState = closed
			gt06.closer.Signal()
		} else {
			gt06.runningState = closed
		}
		gt06.conn_stat.mu.Lock()
		gt06.conn_stat.disconnect_time = time.Now().UTC()
		gt06.conn_stat.mu.Unlock()
		gt06.closer.L.Unlock()
	}()
}

func (gt06 *GT06) run() {
	err := gt06.readMessage()
	if err != nil {
		gt06.closeAndSetErr(err)
		return
	}
	loginTime := time.Now().UTC()
	if gt06.msg.Protocol == loginMessage {
		lm := parseLoginMessage(gt06.msg.Payload)
		sn, err := strconv.ParseUint(lm.SN, 10, 64)
		if err != nil {
			gt06.closeAndSetErr(errRejectedLogin)
			return
		}
		ok, session, _ := gt06.server.Login("imei", sn, gt06)
		if ok {
			gt06.session = session
			if lm.HasTimeOffset {
				gt06.offset = &lm.TimeOffset
			}
			gt06.tid = gt06.session.TrackerId
			gt06.log.Info().Str("event", "login").Uint64("sn", sn).Uint64("tid", gt06.tid).Msg("login accepted")
			// gt06.session.Stat.ConnectEv(gt06.c.Created())
			gt06.log.Context = log.NewContext(gt06.log.Context).Uint64("tid", gt06.tid).Value()
			err := gt06.write(loginOk(gt06.msg.Serial))
			if err != nil {
				gt06.closeAndSetErr(err)
				return
			}
			gt06.conn_stat.mu.Lock()
			gt06.conn_stat.login_time = loginTime
			gt06.conn_stat.mu.Unlock()

		} else {
			gt06.log.Error().Str("event", "login").Uint64("sn", sn).Str("error", "login rejected")
			gt06.closeAndSetErr(errRejectedLogin)
			return
		}
	} else {
		gt06.log.Error().Str("error", "illegal state: first message not login message").Hex("protocol_code", []byte{gt06.msg.Protocol}).Hex("payload", gt06.msg.Payload).Msg("")
	}

	// gt06.state.Attached.Lock()
	fsn := gt06.session.FSN
	for {
		err := gt06.readMessage()
		if err != nil {
			gt06.closeAndSetErr(err)
			return
		}
		tread := time.Now().UTC()
		// gt06.session.Stat.UpdateEv(tread)
		procode := strconv.FormatUint(uint64(gt06.msg.Protocol), 16)
		gt06.log.Trace().Str("procode", procode).Hex("payload", gt06.msg.Payload).Int("serial", gt06.msg.Serial).Msg("receive message from terminal")
		switch gt06.msg.Protocol {
		case byte(timeCheck):
			gt06.log.Info().Str("procode", procode).Msg("terminal time update")
			t := time.Now().UTC()
			err := gt06.write(timeResponse(&t, gt06.msg.Serial))
			if err != nil {
				gt06.closeAndSetErr(err)
				return
			}
		case byte(statusInformation):
			st := parseStatusInformation(gt06.msg.Payload)
			err := gt06.write(statusOk(gt06.msg.Serial))
			if err != nil {
				gt06.closeAndSetErr(err)
				return
			}
			gt06.lastStatusInfo = st
			gt06.log.Debug().Str("procode", procode).Bool("ACC", st.ACC).Msg("terminal status information")
		case byte(gk310GPS):

			loc := parseGK310GPSMessage(gt06.msg.Payload)
			mps_speed := (float32(loc.Speed) * 1000) / 3600
			gt06.store.Put(fsn, loc.Latitude, loc.Longitude, -1, mps_speed, loc.Timestamp, tread)
			gt06.session.Sublist.MarshalSend(gt06.tid, loc.Latitude, loc.Longitude, mps_speed, loc.Timestamp, tread)
			gt06.session.UpdateLocation(loc.Longitude, loc.Latitude, loc.Timestamp.UTC())
			gt06.lastGK310Location = loc
			gt06.log.Debug().Str("procode", procode).Bool("ACC", loc.ACC).Msg("terminal location update code 22")
		case byte(gt06GPS):
			loc := parseGT06GPSMessage(gt06.msg.Payload, gt06.offset)
			mps_speed := (float32(loc.Speed) * 1000) / 3600
			gt06.store.Put(fsn, loc.Latitude, loc.Longitude, -1, mps_speed, loc.Timestamp, tread)
			gt06.session.Sublist.MarshalSend(gt06.tid, loc.Latitude, loc.Longitude, mps_speed, loc.Timestamp, tread)
			gt06.session.UpdateLocation(loc.Longitude, loc.Latitude, loc.Timestamp.UTC())
			gt06.lastGT06Location = loc
			gt06.log.Debug().Str("procode", procode).Msg("terminal location update code 12")
		case byte(informationTxPacket):
			subprocode := strconv.FormatUint(uint64(gt06.msg.Payload[0]), 16)
			switch gt06.msg.Payload[0] {
			case 0x04:
				gt06.lastInformationTxPacket = string(gt06.msg.Payload[1:])
				gt06.log.Debug().Str("procode", procode).Str("subprocode", subprocode).Msg("information packet : terminal status synchronization")
			default:
				gt06.log.Debug().Str("procode", procode).Str("subprocode", subprocode).Msg("information packet : unknown sub protocol code")
			}
		default:
			gt06.log.Error().Hex("data", gt06.msg.Payload).Str("error", "unknown event protocol").Msg("unhandled event protocol")
		}
	}
	// gt06.state.Attached.Unlock()

}

func (gt06 *GT06) readMessage() error {
	var length int       //length field
	var var_buf []byte   //start of variable length data
	var frame_length int //from the beginning of gt06.buffer (including trailer 0x0d 0x0a)
	n, err := gt06.c.ReadFull(gt06.buffer[:4])
	if err != nil {
		gt06.log.Error().Err(err).Msg("Error while reading")

		// gt06.closeErr(err)
		return err
	}
	//check startbit type
	if gt06.buffer[0] == 0x78 {
		length = int(gt06.buffer[2])
		var_buf = gt06.buffer[3:]
		frame_length = length + 5
	} else if gt06.buffer[1] == 0x79 {
		length = int(binary.BigEndian.Uint16(gt06.buffer[2:4]))
		var_buf = gt06.buffer[4:]
		frame_length = length + 6
	} else {
		gt06.log.Error().Str("error", "faulty frame:incorrect header").Hex("faulty_data", gt06.buffer[:n]).Msg("")
		// gt06.closeErr(errBadFrame)
		return errBadFrame
	}

	_, err = gt06.c.ReadFull(gt06.buffer[4:frame_length])
	if err != nil {
		gt06.log.Error().Err(err).Msg("Error while reading")
		// gt06.closeErr(err)
		return err
	}

	if gt06.buffer[frame_length-2] != 0x0D || gt06.buffer[frame_length-1] != 0x0A {
		gt06.log.Error().Str("error", "faulty frame:incorrect trailer").Hex("faulty_data", gt06.buffer[:n]).Msg("")
		// gt06.closeErr(errBadFrame)
		return errBadFrame
	}

	//frame length is `length` + 5
	//payload length is `length` - 5

	gt06.msg.Protocol = var_buf[0]
	gt06.msg.Payload = var_buf[1 : length-4]
	gt06.msg.Serial = int(binary.BigEndian.Uint16(var_buf[length-4 : length-2]))
	return nil
}
