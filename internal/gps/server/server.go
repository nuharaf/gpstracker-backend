package server

import (
	"nuha.dev/gpstracker/internal/gps/client"
)

type ServerInterface interface {
	Login(family, serial string, c client.ClientInterface) (rid string, ok bool)
	// Subscribe(rids []string, sub subscriber.Subscriber)
	GetClientState(rid string) *client.ClientState
}

// type LocationUpdate struct {
// 	RID       string
// 	Latitude  float64
// 	Longitude float64
// 	GPSTime   time.Time
// }
