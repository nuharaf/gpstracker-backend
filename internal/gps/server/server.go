package server

import (
	"nuha.dev/gpstracker/internal/gps/client"
)

type ServerInterface interface {
	Login(sn_type string, serial uint64, c client.ClientInterface) (ok bool)
	// Subscribe(rids []string, sub subscriber.Subscriber)
	// GetGpsClientState(tracker_id uint64) *client.ClientState
}

// type LocationUpdate struct {
// 	RID       string
// 	Latitude  float64
// 	Longitude float64
// 	GPSTime   time.Time
// }
