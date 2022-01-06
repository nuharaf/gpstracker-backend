package device

import (
	"strconv"
	"strings"

	"nuha.dev/gpstracker/internal/gpsv2/conn"
)

const (
	DEVICE_GT06       string = "gt06"
	DEVICE_SIMPLEJSON string = "simplejson"
)

type DeviceIf interface {
	ReplaceConn(c *conn.Conn)
	Stop()
}
type FSN struct {
	SnType string
	Serial uint64
	Base   int
}

type DeviceInfoIf interface {
	CurrentConnInfo() ([]string, uint64)
}

func JoinSn(sn_type string, serial uint64) string {
	switch sn_type {
	case "imei":
		return sn_type + ":" + strconv.FormatUint(serial, 10)
	case "mac":
		fallthrough
	default:
		return sn_type + ":" + strconv.FormatUint(serial, 16)
	}

}

func SplitFSN(fsn string) (string, uint64, error) {
	sntypesn := strings.SplitN(fsn, ":", 2)
	sn, err := strconv.ParseUint(sntypesn[1], 16, 64)
	return sntypesn[0], sn, err
}

type DeviceConfigAttribute struct {
	Config    *DeviceConfig
	Attribute map[string]string
}

type DeviceConfig struct {
	AllowConnect bool   `json:"allow_connect"`
	SublistSend  bool   `json:"sublist_send"`
	Store        bool   `json:"store"`
	Broadcast    bool   `json:"broadcast"`
	LogLevel     string `json:"log_level"`
	ReadDeadline int    `json:"read_deadline"`
}
