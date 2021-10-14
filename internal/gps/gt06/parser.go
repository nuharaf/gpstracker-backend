package gt06

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"time"

	"nuha.dev/gpstracker/internal/util/crc16"
)

const (
	loginMessage        byte = 0x01
	gt06GPS             byte = 0x12
	statusInformation   byte = 0x13
	stringInformation   byte = 0x15
	alarmData           byte = 0x16
	gpsInfo             byte = 0x1A
	gk310GPS            byte = 0x22
	serverCommand       byte = 0x80
	timeCheck           byte = 0x8A
	informationTxPacket byte = 0x94
)

var errBadFrame = errors.New("Bad frame")

type message struct {
	Protocol byte
	Serial   int
	Payload  []byte
}

type deviceSn struct {
	imei  string
	imsi  string
	iccid string
}

func parseDeviceSn(d []byte) *deviceSn {
	m := &deviceSn{}
	m.imei = hex.EncodeToString(d[:8])
	m.imsi = hex.EncodeToString(d[8:16])
	m.iccid = hex.EncodeToString(d[16:24])
	return m
}

type gk310GPSMessage struct {
	Latitude          float64
	Longitude         float64
	Timestamp         time.Time
	SatCount          int
	Speed             int
	MCC               int
	MNC               int
	LAC               int
	CellID            int
	HasACC            bool
	ACC               bool
	HasDataUploadMode bool
	DataUploadMode    int
	HasGPSReupload    bool
	GPSIsReupload     bool
	GPSDifferential   bool
	GPSPositioned     bool
}

type gt06GPSMessage struct {
	Timestamp       time.Time
	Latitude        float64
	Longitude       float64
	SatCount        int
	Speed           int
	MCC             int
	MNC             int
	LAC             int
	CellID          int
	GPSDifferential bool
	GPSPositioned   bool
}

type statusInfo struct {
	Arm          bool
	ACC          bool
	EngineDisc   bool
	Charging     bool
	AlarmCode    int
	AltAlarmCode int
	Language     int
	GPS          bool
	Voltage      int
	GSMSignal    int
}

type loginMsg struct {
	SN            string
	TimeOffset    time.Duration
	HasTimeOffset bool
	TypeID        [2]byte
}

func parseLoginMessage(d []byte) *loginMsg {
	m := &loginMsg{HasTimeOffset: false}
	m.SN = hex.EncodeToString(d[:8])
	if len(d) > 8 {
		copy(m.TypeID[:], d[8:10])
	}
	if len(d) > 10 {
		m.HasTimeOffset = true
		bcdOffset := (uint16(d[10]) << 4) + (uint16(d[11]) >> 4)
		hOffset := bcdOffset / 100
		mOffset := bcdOffset % 100
		m.TimeOffset = time.Duration(hOffset)*time.Hour + time.Duration(mOffset)*time.Minute
		if d[11]&0b00001000 != 0 {
			m.TimeOffset = -m.TimeOffset
		}
	}
	return m
}

func parseStatusInformation(d []byte) *statusInfo {
	m := &statusInfo{}
	m.EngineDisc = d[0]&0b10000000 != 0
	m.GPS = d[0]&0b01000000 != 0
	m.AlarmCode = int(d[0]&0b00111000) >> 3
	m.Charging = d[0]&0b00000100 != 0
	m.ACC = d[0]&0b00000010 != 0
	m.Arm = d[0]&0b00000001 != 0
	m.Voltage = int(d[1])
	m.GSMSignal = int(d[2])
	m.AltAlarmCode = int(d[3])
	m.Language = int(d[4])
	return m
}

func parseGT06GPSMessage(d []byte) *gt06GPSMessage {
	m := &gt06GPSMessage{}
	m.Timestamp = time.Date(int(d[0])+2000, time.Month(d[1]), int(d[2]), int(d[3]), int(d[4]), int(d[5]), 0, time.Local)
	m.SatCount = int(d[6] & 0x0F)
	lat := float64(binary.BigEndian.Uint32(d[7:11])) / 1800000
	lon := float64(binary.BigEndian.Uint32(d[11:15])) / 1800000
	m.Speed = int(d[15])
	isNorth := d[16]&0b00000100 != 0
	isWest := d[16]&0b00001000 != 0
	if isNorth {
		m.Latitude = lat
	} else {
		m.Latitude = 0 - lat
	}
	if isWest {
		m.Longitude = 0 - lon
	} else {
		m.Longitude = lon
	}
	m.GPSDifferential = d[16]&0b00100000 != 0
	m.GPSPositioned = d[16]&0b00010000 != 0
	m.MCC = int(binary.BigEndian.Uint16(d[18:20]))
	m.MNC = int(d[20])
	m.LAC = int(binary.BigEndian.Uint16(d[21:23]))
	m.CellID = int(binary.BigEndian.Uint32(append([]byte{0}, d[23:26]...)))
	return m
}

func parseGK310GPSMessage(d []byte) *gk310GPSMessage {
	m := &gk310GPSMessage{}
	m.Timestamp = time.Date(int(d[0])+2000, time.Month(d[1]), int(d[2]), int(d[3]), int(d[4]), int(d[5]), 0, time.UTC)
	m.SatCount = int(d[6] & 0x0F)
	lat := float64(binary.BigEndian.Uint32(d[7:11])) / 1800000
	lon := float64(binary.BigEndian.Uint32(d[11:15])) / 1800000
	m.Speed = int(d[15])
	isNorth := d[16]&0b00000100 != 0
	isWest := d[16]&0b00001000 != 0
	if isNorth {
		m.Latitude = lat
	} else {
		m.Latitude = 0 - lat
	}
	if isWest {
		m.Longitude = 0 - lon
	} else {
		m.Longitude = lon
	}
	m.MCC = int(binary.BigEndian.Uint16(d[18:20]))
	m.MNC = int(d[20])
	m.LAC = int(binary.BigEndian.Uint16(d[21:23]))
	m.GPSDifferential = d[16]&0b00100000 != 0
	m.GPSPositioned = d[16]&0b00010000 != 0

	m.CellID = int(binary.BigEndian.Uint32(append([]byte{0}, d[23:26]...)))
	if len(d) > 26 {
		m.HasACC = true
		m.ACC = d[26] != 0
	}
	if len(d) > 27 {
		m.HasDataUploadMode = true
		m.DataUploadMode = int(d[27])

	}
	if len(d) > 28 {
		m.HasGPSReupload = true
		m.GPSIsReupload = d[28] != 0
	}
	return m
}

func newResponse(protocol byte, payload []byte, serial int) []byte {
	lp := len(payload)
	lf := lp + 10
	frame := make([]byte, lf)
	frame[0] = 0x78
	frame[1] = 0x78
	frame[2] = byte(lp + 5)
	frame[3] = protocol
	copy(frame[4:], payload)
	binary.BigEndian.PutUint16(frame[lf-6:lf-4], uint16(serial))
	crc := crc16.Checksum(crc16.X25, frame[2:lf-4])
	binary.BigEndian.PutUint16(frame[lf-4:lf-2], crc)
	frame[lf-2] = 0x0d
	frame[lf-1] = 0x0a
	return frame
}
