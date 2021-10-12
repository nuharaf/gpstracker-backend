package logstore

import (
	"time"

	"github.com/phuslu/log"
)

type LogStore struct {
}

func NewStore() *LogStore {
	return &LogStore{}
}

func (l *LogStore) Put(serial_number string, lon float64, lat float64, alt float32, speed float32, gpst time.Time, srvt time.Time) {
	log.Trace().Str("module", "logstore").Str("serial_number", serial_number).Float64("lon", lon).Float64("lat", lat).Float32("alt", alt).Float32("speed", speed).Time("gpstime", gpst).Msg("")
}
