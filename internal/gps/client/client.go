package client

import (
	"sync"
	"time"

	"nuha.dev/gpstracker/internal/gps/sublist"
)

type ClientInterface interface {
	Run()
	TryCloseWait() bool
	ConnectionId() uint64
	// Conn() *wc.Conn
	// Closed() bool
	// LoggedIn() bool
	// SetState(s *ClientSession)
}

type SimpleLocation struct {
	mu      sync.Mutex
	lat     float64
	lon     float64
	gpstime time.Time
}

func (sl *SimpleLocation) UpdateLocation(lon, lat float64, gpstime time.Time) {
	sl.mu.Lock()
	sl.lat = lat
	sl.lon = lon
	sl.gpstime = gpstime
	sl.mu.Unlock()
}

// I think this is thread safe
type ClientSession struct {
	Client      ClientInterface
	DeadClients []ClientInterface
	mu          sync.Mutex
	// Stat          *stat.Stat
	Sublist       *sublist.Sublist
	TrackerId     uint64
	FSN           string
	last_location SimpleLocation
	// kv_status     map[string]interface{}
	// kv_mu         sync.Mutex
}

func (cs *ClientSession) Lock() {
	cs.mu.Lock()
}

func (cs *ClientSession) Unlock() {
	cs.mu.Unlock()
}

func (cs *ClientSession) UpdateLocation(lon, lat float64, gpstime time.Time) {
	cs.last_location.mu.Lock()
	cs.last_location.lat = lat
	cs.last_location.lon = lon
	cs.last_location.gpstime = gpstime
	cs.last_location.mu.Unlock()
}

// func (cs *ClientSession) SetKV(key string, value string) {
// 	cs.kv_mu.Lock()
// 	cs.kv_status[key] = value
// 	cs.kv_mu.Unlock()
// }

// func (cs *ClientSession) AddKV(kv map[string]interface{}) {
// 	cs.kv_mu.Lock()
// 	for key, value := range kv {
// 		cs.kv_status[key] = value
// 	}
// 	cs.kv_mu.Unlock()
// }

// func (cs *ClientSession) GetKV() map[string]interface{} {
// 	cs.kv_mu.Lock()
// 	res := make(map[string]interface{})
// 	for key, value := range cs.kv_status {
// 		res[key] = value
// 	}
// 	cs.kv_mu.Unlock()
// 	return res
// }

func (cs *ClientSession) GetLastLocation() (lon float64, lat float64, gpstime time.Time) {
	cs.last_location.mu.Lock()
	lon = cs.last_location.lon
	lat = cs.last_location.lat
	gpstime = cs.last_location.gpstime
	cs.last_location.mu.Unlock()
	return
}

func NewClientSession(tid uint64, fsn string) *ClientSession {
	d := &ClientSession{Sublist: sublist.NewSublist()}
	d.FSN = fsn
	d.TrackerId = tid
	// d.kv_status = make(map[string]interface{})
	return d
}

type ClientConfig struct {
}
