package store

import (
	"time"
)

type LocationStore interface {
	Put(fsn string, lon float64, lat float64, alt float32, speed float32, gpst time.Time, srvt time.Time)
}

type MiscStore interface {
	SaveCommandResponse(tid uint64, server_flag uint32, message string, t time.Time)
	SaveEvent(tid uint64, event_type string, message string, message_obj interface{}, t time.Time)
	UpdateAttribute(tid uint64, key string, value string)
}
