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
	locationData        byte = 0x12
	statusInformation   byte = 0x13
	stringInformation   byte = 0x15
	alarmData           byte = 0x16
	gpsInfo             byte = 0x1A
	gk310GPS            byte = 0x22
	serverCommand       byte = 0x80
	timeCheck           byte = 0x8A
	informationTxPacket byte = 0x94
)

const (
	statusNormal     byte = 0b000
	statusVibAlarm   byte = 0b001
	statusEnterFence byte = 101
	statusExitFence  byte = 110
)

var errBadFrame = errors.New("Bad frame")

type message struct {
	Protocol byte
	Serial   int
	Payload  []byte
}

type gk310GPSMessage struct {
	Latitude  float64
	Longitude float64
	Timestamp time.Time
	SatCount  int
	Speed     int
	MCC       int
	MNC       int
	LAC       int
	CellID    int
	HasACC    bool
	ACC       bool
}

type statusInfo struct {
	Arm       bool
	ACC       bool
	AlarmCode int
	GPS       bool
	Voltage   int
	GSMSignal int
}

func parseLoginMessage(d []byte) (sn string) {
	return hex.EncodeToString(d[:8])
}

func parseStatusInformation(d []byte) *statusInfo {
	m := &statusInfo{}
	m.GPS = d[0]&0b01000000 != 0
	m.AlarmCode = int(d[0]&0b00111000) >> 3
	m.ACC = d[0]&0b00000010 != 0
	m.Arm = d[0]&0b00000001 != 0
	m.Voltage = int(d[1])
	m.GSMSignal = int(d[2])
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

	m.CellID = int(binary.BigEndian.Uint32(append([]byte{0}, d[23:26]...)))
	m.HasACC = true
	m.ACC = d[26] != 0

	return m
}

func timeResponse(t *time.Time, serial int) []byte {
	payload := []byte{byte(t.Year() % 100), byte(t.Month()), byte(t.Day()), byte(t.Hour()), byte(t.Minute()), byte(t.Second())}
	return newResponse(timeCheck, payload, serial)
}

func loginOk(serial int) []byte {
	return newResponse(loginMessage, []byte{}, serial)
}

func statusOk(serial int) []byte {
	return newResponse(statusInformation, []byte{}, serial)
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
