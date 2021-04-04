package client

import (
	"nuha.dev/gpstracker/internal/gps/stat"
	"nuha.dev/gpstracker/internal/util/wc"

	"nuha.dev/gpstracker/internal/gps/sublist"
)

type ClientInterface interface {
	Run()
	Conn() *wc.Conn
	Closed() bool
	LoggedIn() bool
	SetState(s *ClientState)
}

type ClientState struct {
	Stat    *stat.Stat
	Sublist *sublist.MulSublist
}
