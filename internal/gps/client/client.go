package client

import (
	"sync"

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

// I think this is thread safe
type ClientState struct {
	Attached *sync.Mutex
	Stat     *stat.Stat
	Sublist  *sublist.MulSublist
}
