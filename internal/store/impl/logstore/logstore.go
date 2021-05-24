package logstore

import (
	"time"

	"github.com/rs/zerolog/log"
)

type LogStore struct {
}

func NewStore() *LogStore {
	return &LogStore{}
}

func (l *LogStore) Put(rid string, lon float64, lat float64, alt float32, speed float32, gpst time.Time, srvt time.Time) {
	log.With().Str("rid", rid).Float64("lon", lon).Float64("lat", lat).Float32("alt", alt).Float32("speed", speed).Time("gpstime", gpst)
}
