package nullstore

import (
	"time"

	"github.com/rs/zerolog/log"
)

func Put(rid string, lon float64, lat float64, alt float32, gpst time.Time, srvt time.Time) {
	log.With().Str("rid", rid).Float64("lon", lon).Float64("lat", lat).Float32("alt", alt).Time("gpstime", gpst)
}
