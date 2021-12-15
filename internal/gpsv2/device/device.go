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

type DeviceConfig struct {
	AllowConnect   bool   `json:"allow_connect"`
	AllowSubscribe bool   `json:"allow_subscribe"`
	Store          bool   `json:"store"`
	Broadcast      bool   `json:"broadcast"`
	LogLevel       string `json:"log_level"`
}
