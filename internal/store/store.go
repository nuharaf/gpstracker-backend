package store

import (
	"time"
)

type Store interface {
	Put(serial_number string, lon float64, lat float64, alt float32, speed float32, gpst time.Time, srvt time.Time)
}
