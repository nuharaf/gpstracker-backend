package simplejson

import (
	"time"
)

type FrameMessage struct {
	Length   int
	Protocol byte
	Payload  []byte
	Buffer   []byte
}

const (
	LOGIN           byte = 0x01
	LOCATION_UPDATE byte = 0x02
	SAT_UPDATE      byte = 0x03
	GPS_ERROR       byte = 0x04
	GPS_INIT        byte = 0x05
	STATUS          byte = 0x06
)

type LoginMessage struct {
	SnType     string `json:"sn_type"`
	Serial     string `json:"serial"`
	DeviceType string `json:"device_type"`
}

type Sat struct {
	SPRN      int64 `json:"svprn"`
	SNR       int64 `json:"snr"`
	UsedInFix bool  `json:"fix"`
}

type LocationMessage struct {
	GpsTime     time.Time      `json:"gps_time"`
	MachineTime time.Time      `json:"machine_time"`
	Latitude    float64        `json:"latitude"`
	Longitude   float64        `json:"longitude"`
	Altitude    float32        `json:"altitude"`
	SatInview   int            `json:"sat_inview"`
	SatTracked  int            `json:"sat_tracked"`
	SatUsed     int            `json:"sat_used"`
	Fix         bool           `json:"fix"`
	FixMode     string         `json:"fix_mode"`
	Speed       float32        `json:"speed"`
	SATList     []*Sat         `json:"-"`
	SATMap      map[int64]*Sat `json:"-"`
	HasRMC      bool           `json:"-"`
	HasGLL      bool           `json:"-"`
	HasVTG      bool           `json:"-"`
	HasGSA      bool           `json:"-"`
	HasGGA      bool           `json:"-"`
	HasGSV      bool           `json:"-"`
}

type StatusMessage struct {
	GpsStatus      bool      `json:"gps_status"`
	LastLongitude  float64   `json:"last_longitude,omitempty"`
	LastLatitude   float64   `json:"last_latitude,omitempty"`
	LastFix        time.Time `json:"last_fix,omitempty"`
	LastSatTracked int       `json:"last_sat_tracked,omitempty"`
	LastSatInview  int       `json:"last_sat_inview,omitempty"`
	LastSatUsed    int       `json:"last_sat_used,omitempty"`
	LastSatUpdate  time.Time `json:"last_sat_update,omitempty"`
}
