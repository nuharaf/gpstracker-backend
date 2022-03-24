package device

import (
	"strconv"
	"time"

	"github.com/phuslu/log"
	"nuha.dev/gpstracker/internal/gpsv2/conn"
)

const (
	DEVICE_GT06       string = "gt06"
	DEVICE_SIMPLEJSON string = "simplejson"
)

type DeviceIf interface {
	ReplaceConn(c *conn.Conn)
	Stop()
	CurrentConnInfo() []string
	GetLocation() Location
}
type FSN struct {
	SnType string
	Serial uint64
	Base   int
}

type Location struct {
	Latitude  float64   `json:"lat"`
	Longitude float64   `json:"lon"`
	Altitude  float32   `json:"alt"`
	Speed     float32   `json:"speed"`
	Timestamp time.Time `json:"gps_time"`
}

type Serial struct {
	sn_type        int
	nsn            uint64
	sn             uint64
	sn_type_string string
	sn_string      string
}

func NewSerial(sn_type int, sn uint64) Serial {
	ser := Serial{}
	ser.sn_type = sn_type
	ser.sn = sn
	ser.nsn = CombineSn(sn_type, sn)
	ser.sn_type_string = SnTypeString(sn_type)
	ser.sn_string = FormatSnPretty(sn_type, sn)
	return ser
}

func NewSerial2(nsn uint64) Serial {
	ser := Serial{}
	sn_type, sn := SplitSn(nsn)
	ser.sn_type = sn_type
	ser.sn = sn
	ser.nsn = nsn
	ser.sn_type_string = SnTypeString(sn_type)
	ser.sn_string = FormatSnPretty(sn_type, sn)
	return ser
}

func (ser Serial) SnTypeString() string {
	return ser.sn_type_string
}

func (ser Serial) Sn() uint64 {
	return ser.sn
}

func (ser Serial) SnString() string {
	return ser.sn_string
}

func (ser Serial) Nsn() uint64 {
	return ser.nsn
}

func (ser Serial) MarshalObject(e *log.Entry) {
	e.Strs("serial", []string{ser.sn_type_string, ser.sn_string})
}

func CombineSn(sn_type int, sn uint64) uint64 {
	return (sn & 0x0fffffffffffffff) | (uint64(sn_type) << 60)
}

func SplitSn(nsn uint64) (int, uint64) {
	return int(nsn >> 60), nsn & 0x0fffffffffffffff
}

func SnTypeString(sn_type int) string {
	switch sn_type {
	case 0:
		return "imei"
	case 1:
		return "mac"
	case 2:
		return "aid"
	case 3:
		return "misc1"
	case 4:
		return "misc2"
	}
	return "other"
}

func FormatSnPretty(sn_type int, sn uint64) string {
	switch sn_type {
	case 0:
		return strconv.FormatUint(sn, 10)
	default:
		return strconv.FormatUint(sn, 16)
	}
}

// func JoinSn(sn_type string, serial uint64) string {
// 	switch sn_type {
// 	case "imei":
// 		return sn_type + ":" + strconv.FormatUint(serial, 10)
// 	case "mac":
// 		fallthrough
// 	default:
// 		return sn_type + ":" + strconv.FormatUint(serial, 16)
// 	}

// }

// func SplitFSN(fsn string) (string, uint64, error) {
// 	sntypesn := strings.SplitN(fsn, ":", 2)
// 	sn, err := strconv.ParseUint(sntypesn[1], 16, 64)
// 	return sntypesn[0], sn, err
// }

type DeviceConfigAttribute struct {
	Config    *DeviceConfig
	Attribute map[string]string
}

type DeviceConfig struct {
	AllowConnect bool   `json:"allow_connect"`
	SublistSend  bool   `json:"send_sublist"`
	Store        bool   `json:"store"`
	Broadcast    bool   `json:"broadcast"`
	LogLevel     string `json:"log_level" `
	ReadDeadline int    `json:"read_deadline"`
}
