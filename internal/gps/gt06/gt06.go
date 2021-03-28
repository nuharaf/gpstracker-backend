package gt06

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"nuha.dev/gpstracker/internal/gps"
)

type GT06 struct {
	byte_in  uint64
	byte_out uint64
	c        net.Conn
	r        *bufio.Reader
	info     gps.ConnInfo
	closed   int32
	err      error
	buffer   []byte
	msg      message
	log      zerolog.Logger
	LogOpts
	server    gps.ServerInterface
	logged_in int32
	rid       string //registration_id
}

type LogOpts struct {
	log_read  bool
	log_write bool
}

var errRejectedLogin = errors.New("login rejected")

func NewGT06(r *bufio.Reader, c net.Conn, info gps.ConnInfo, server gps.ServerInterface) *GT06 {
	o := &GT06{r: r, c: c, buffer: make([]byte, 1000), info: info}
	logger := log.With().Str("module", "gt06").Uint64("cid", info.CID).Logger()
	o.log = logger
	o.LogOpts = LogOpts{log_read: false, log_write: false}
	o.server = server
	return o
}

func (gt06 *GT06) closeErr(err error) {
	gt06.err = err
	atomic.StoreInt32(&gt06.closed, 1)
	gt06.c.Close()
}

func (gt06 *GT06) write(d []byte) {
	if gt06.LogOpts.log_write {
		gt06.log.Debug().Str("action", "write").Hex("data", d).Msg("")
	}
	_, err := gt06.c.Write(d)
	if err != nil {
		gt06.log.Err(err).Msg("Error while writing data")
		gt06.closeErr(err)
		return
	}
	atomic.AddUint64(&gt06.byte_out, uint64(len(d)))
}

func (gt06 *GT06) Stat() (in, out uint64) {
	return atomic.LoadUint64(&gt06.byte_in), atomic.LoadUint64(&gt06.byte_out)
}

func (gt06 *GT06) Closed() bool {
	return atomic.LoadInt32(&gt06.closed) == 1
}

func (gt06 *GT06) LoggedIn() bool {
	return atomic.LoadInt32(&gt06.logged_in) == 1
}

func (gt06 *GT06) Info() gps.ConnInfo {
	return gt06.info
}

func (gt06 *GT06) Run() {
	for {
		gt06.readMessage()
		if gt06.closed == 1 {
			return
		}
		if gt06.logged_in == 1 {
			switch gt06.msg.Protocol {
			case byte(timeCheck):
				gt06.log.Info().Str("event", "terminal time update").Msg("")
				t := time.Now().UTC()
				gt06.write(timeResponse(&t, gt06.msg.Serial))
			case byte(statusInformation):
				st := parseStatusInformation(gt06.msg.Payload)
				gt06.log.Debug().Str("event", "status information").Dict("event_data", zerolog.Dict().Int("gsm_signal", st.GSMSignal).Int("voltage_level", st.Voltage).Bool("ACC", st.ACC).Bool("GPS", st.GPS)).Msg("")
				gt06.write(statusOk(gt06.msg.Serial))
			case byte(gk310GPS):
				loc := parseGK310GPSMessage(gt06.msg.Payload)
				ci := zerolog.Dict().Int("MCC", loc.MCC).Int("MNC", loc.MNC).Int("cell_id", loc.CellID).Int("LAC", loc.LAC)
				ev := zerolog.Dict().Float64("lat", loc.Latitude).Float64("lon", loc.Longitude).Time("timestamp", loc.Timestamp).Int("sat_count", loc.SatCount).Int("speed", loc.Speed)
				gt06.log.Debug().Str("event", "location update").Dict("event_data", ev.Bool("ACC", loc.ACC).Dict("cell_info", ci)).Msg("")
				gt06.server.Location(gt06.rid, loc.Latitude, loc.Longitude, loc.Timestamp)
			case byte(informationTxPacket):
				switch gt06.msg.Payload[0] {
				case 0x04:
					gt06.log.Info().Str("event", "information packet").Dict("event_data", zerolog.Dict().Str("subevent", "terminal status").Str("status", string(gt06.msg.Payload[1:]))).Msg("")
				default:
					gt06.log.Debug().Str("event", "information packet").Hex("data", gt06.msg.Payload).Msg("ignoring unknown information packet")
				}
			default:
				gt06.log.Error().Hex("data", gt06.msg.Payload).Str("error", "unknown event protocol")
			}
		} else if gt06.msg.Protocol == loginMessage {
			sn := parseLoginMessage(gt06.msg.Payload)
			rid, ok := gt06.server.Login("gt06", sn)
			if ok {
				gt06.rid = rid
				gt06.log.Info().Str("event", "login").Str("sn", sn).Str("rid", rid).Msg("login accepted")
				gt06.write(loginOk(gt06.msg.Serial))
				atomic.StoreInt32(&gt06.logged_in, 1)
			} else {
				gt06.log.Error().Str("event", "login").Str("sn", sn).Str("error", "login rejected")
				gt06.closeErr(errRejectedLogin)
			}
		} else {
			gt06.log.Error().Str("error", "illegal state: first message not login message").Hex("protocol_code", []byte{gt06.msg.Protocol}).Hex("payload", gt06.msg.Payload).Msg("")
		}
		if gt06.closed == 1 {
			return
		}
	}

}

func (gt06 *GT06) readMessage() {

	var length int       //length field
	var var_buf []byte   //start of variable length data
	var frame_length int //from the beginning of gt06.buffer (including trailer 0x0d 0x0a)
	n, err := io.ReadFull(gt06.r, gt06.buffer[:4])
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

	_, err = io.ReadFull(gt06.r, gt06.buffer[4:frame_length])
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
		gt06.log.Debug().Str("action", "read").Hex("data", gt06.buffer[:frame_length]).Msg("")
	}
	atomic.AddUint64(&gt06.byte_in, uint64(frame_length))
	gt06.msg.Protocol = var_buf[0]
	gt06.msg.Payload = var_buf[1 : length-4]
	gt06.msg.Serial = int(binary.BigEndian.Uint16(var_buf[length-4 : length-2]))
}
