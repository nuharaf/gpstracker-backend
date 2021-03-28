package gps

import (
	"time"
)

type ServerInterface interface {
	Login(family, serial string) (rid string, ok bool)
	Location(rid string, lat, lon float64, timestamp time.Time)
}

type ClientInterface interface {
	Run()
	Stat() (in, out uint64)
	Info() ConnInfo
	Closed() bool
	LoggedIn() bool
}

type Subscriber interface {
	Push(loc []byte) error
}

type ConnInfo struct {
	RemoteAddr   string
	CID          uint64
	TimeCreation time.Time
}

type Location struct {
	Latitude  float64
	Longitude float64
	Time      time.Time
}
