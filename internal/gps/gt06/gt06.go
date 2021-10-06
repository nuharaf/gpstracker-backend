package gt06

import (
	"encoding/binary"
	"errors"
	"strconv"
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

type GT06 struct {
	c      *wc.Conn
	err    error
	buffer []byte
	msg    message
	log    zerolog.Logger
	LogOpts
	server    server.ServerInterface
	logged_in int32
	tid       uint64
	state     *client.ClientState
	store     store.Store
}

type LogOpts struct {
	log_read  bool
	log_write bool
}

var errRejectedLogin = errors.New("login rejected")

func NewGT06(c *wc.Conn, server server.ServerInterface, store store.Store) *GT06 {
	o := &GT06{c: c, buffer: make([]byte, 1000), store: store}
	logger := log.With().Str("module", "gt06").Uint64("cid", c.Cid()).Logger()
	o.log = logger
	o.LogOpts = LogOpts{log_read: false, log_write: false}
	o.store = store
	o.server = server
	return o
}

func (gt06 *GT06) closeErr(err error) {
	gt06.err = err
	gt06.c.Close()
	if gt06.state != nil {
		gt06.state.Stat.DisconnectEv(time.Now().UTC())
	}

}

func (gt06 *GT06) write(d []byte) {
	if gt06.LogOpts.log_write {
		gt06.log.Debug().Str("operation", "write").Hex("data", d).Msg("")
	}
	_, err := gt06.c.Write(d)
	if err != nil {
		gt06.log.Err(err).Msg("Error while writing data")
		gt06.closeErr(err)
		return
	}
}

func (gt06 *GT06) Conn() *wc.Conn {
	return gt06.c
}

func (gt06 *GT06) Subscribe(sub subscriber.Subscriber) {
	gt06.state.Sublist.Subscribe(sub)
}

func (gt06 *GT06) SetState(state *client.ClientState) {
	gt06.state = state
}

func (gt06 *GT06) Closed() bool {
	return gt06.c.Closed()
}

func (gt06 *GT06) LoggedIn() bool {
	return atomic.LoadInt32(&gt06.logged_in) == 1
}

func (gt06 *GT06) Run() {

	gt06.readMessage()
	if gt06.c.Closed() {
		return
	}

	if gt06.msg.Protocol == loginMessage {
		sn, err := strconv.ParseUint(parseLoginMessage(gt06.msg.Payload), 10, 64)
		if err != nil {
			gt06.closeErr(errRejectedLogin)
			return
		}
		ok := gt06.server.Login("imei", sn, gt06)
		if ok {
			gt06.tid = gt06.state.TrackerId
			gt06.log.Info().Str("event", "login").Uint64("sn", sn).Uint64("tid", gt06.tid).Msg("login accepted")
			gt06.state.Stat.ConnectEv(gt06.c.Created())
			gt06.log = gt06.log.With().Uint64("tid", gt06.tid).Logger()
			gt06.write(loginOk(gt06.msg.Serial))
			atomic.StoreInt32(&gt06.logged_in, 1)

		} else {
			gt06.log.Error().Str("event", "login").Uint64("sn", sn).Str("error", "login rejected")
			gt06.closeErr(errRejectedLogin)
			return
		}
	} else {
		gt06.log.Error().Str("error", "illegal state: first message not login message").Hex("protocol_code", []byte{gt06.msg.Protocol}).Hex("payload", gt06.msg.Payload).Msg("")
	}

	gt06.state.Attached.Lock()
	fsn := gt06.state.FSN
	for {
		gt06.readMessage()
		if gt06.c.Closed() {
			break
		}
		tread := time.Now().UTC()
		gt06.state.Stat.UpdateEv(tread)
		switch gt06.msg.Protocol {
		case byte(timeCheck):
			gt06.log.Info().Str("event", "terminal time update").Msg("")
			t := time.Now().UTC()
			gt06.write(timeResponse(&t, gt06.msg.Serial))
		case byte(statusInformation):
			st := parseStatusInformation(gt06.msg.Payload)
			gt06.write(statusOk(gt06.msg.Serial))
			m := map[string]interface{}{"gsm_signal": st.GSMSignal, "voltage": st.Voltage, "ACC": st.ACC, "GPS": st.GPS}
			gt06.log.Debug().Fields(m)
			gt06.state.AddKV(m)
		case byte(gk310GPS):
			loc := parseGK310GPSMessage(gt06.msg.Payload)
			ci := zerolog.Dict().Int("MCC", loc.MCC).Int("MNC", loc.MNC).Int("cell_id", loc.CellID).Int("LAC", loc.LAC)
			ev := zerolog.Dict().Float64("lat", loc.Latitude).Float64("lon", loc.Longitude).Time("timestamp", loc.Timestamp).Int("sat_count", loc.SatCount).Int("speed", loc.Speed)
			gt06.log.Debug().Str("event", "location update").Dict("event_data", ev.Bool("ACC", loc.ACC).Dict("cell_info", ci)).Msg("")
			mps_speed := (float32(loc.Speed) * 1000) / 3600
			gt06.store.Put(fsn, loc.Latitude, loc.Longitude, -1, mps_speed, loc.Timestamp, tread)
			gt06.state.Sublist.MarshalSend(gt06.tid, loc.Latitude, loc.Longitude, mps_speed, loc.Timestamp, tread)
			gt06.state.UpdateLocation(loc.Longitude, loc.Latitude, loc.Timestamp.UTC())

		case byte(informationTxPacket):
			switch gt06.msg.Payload[0] {
			case 0x04:
				gt06.log.Debug().Str("event", "information packet").Dict("event_data", zerolog.Dict().Str("subevent", "terminal status").Str("status", string(gt06.msg.Payload[1:]))).Msg("")
			default:
				gt06.log.Debug().Str("event", "information packet").Hex("data", gt06.msg.Payload).Msg("ignoring unknown information packet")
			}
		default:
			gt06.log.Error().Hex("data", gt06.msg.Payload).Str("error", "unknown event protocol")
		}

		if gt06.c.Closed() {
			break
		}
	}
	gt06.state.Attached.Unlock()
}

func (gt06 *GT06) readMessage() {
	var length int       //length field
	var var_buf []byte   //start of variable length data
	var frame_length int //from the beginning of gt06.buffer (including trailer 0x0d 0x0a)
	n, err := gt06.c.ReadFull(gt06.buffer[:4])
	if err != nil {
		gt06.log.Err(err).Msg("Error while reading")
		gt06.closeErr(err)
		return
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
		gt06.closeErr(errBadFrame)
		return
	}

	_, err = gt06.c.ReadFull(gt06.buffer[4:frame_length])
	if err != nil {
		gt06.log.Err(err).Msg("Error while reading")
		gt06.closeErr(err)
		return
	}

	if gt06.buffer[frame_length-2] != 0x0D || gt06.buffer[frame_length-1] != 0x0A {
		gt06.log.Error().Str("error", "faulty frame:incorrect trailer").Hex("faulty_data", gt06.buffer[:n]).Msg("")
		gt06.closeErr(errBadFrame)
		return
	}

	//frame length is `length` + 5
	//payload length is `length` - 5

	if gt06.LogOpts.log_read {
		gt06.log.Debug().Str("operation", "read").Hex("data", gt06.buffer[:frame_length]).Msg("")
	}
	gt06.msg.Protocol = var_buf[0]
	gt06.msg.Payload = var_buf[1 : length-4]
	gt06.msg.Serial = int(binary.BigEndian.Uint16(var_buf[length-4 : length-2]))
}
