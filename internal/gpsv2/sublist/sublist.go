package sublist

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"nuha.dev/gpstracker/internal/gpsv2/subscriber"
)

// type MulSublist struct {
// 	slow       *Sublist
// 	normal     *Sublist
// 	slow_dur   time.Duration
// 	prune_dur  time.Duration
// 	last_slow  time.Time
// 	last_prune time.Time
// }

// func NewMulSublist() *MulSublist {
// 	o := &MulSublist{}
// 	o.slow = NewSublist()
// 	o.normal = NewSublist()
// 	o.slow_dur = 5 * time.Second
// 	o.prune_dur = 20 * time.Second
// 	o.last_prune = time.Now()
// 	return o
// }

// func (m *MulSublist) Send(sender string, d []byte) {
// 	m.normal.Send(sender, d)
// 	t0 := time.Now()
// 	if t0.Sub(m.last_slow) > m.slow_dur {
// 		m.slow.Send(sender, d)
// 		m.last_slow = t0
// 	}
// 	if t0.Sub(m.last_prune) > m.prune_dur {
// 		m.normal.Prune()
// 		m.slow.Prune()
// 		m.last_prune = t0
// 	}
// }

// func (m *MulSublist) Subscribe(sub subscriber.Subscriber) {
// 	m.normal.Subscribe(sub)
// }

// func (m *MulSublist) Unsubscribe(sub subscriber.Subscriber) {
// 	m.normal.Subscribe(sub)
// }

// func (m *MulSublist) SubscribeSlow(sub subscriber.Subscriber) {
// 	m.slow.Subscribe(sub)
// }

type SublistMap struct {
	mu   *sync.Mutex
	list map[uint64]Sublist
}

type Sublist struct {
	key       uint64
	list      map[subscriber.Subscriber]bool
	data      []byte
	mu        *sync.Mutex
	prune_dur time.Duration
}

func NewSublistMap() *SublistMap {
	m := SublistMap{}
	m.mu = &sync.Mutex{}
	m.list = map[uint64]Sublist{}
	return &m
}

func (s *SublistMap) GetSublist(key uint64, create bool) (*Sublist, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.list[key]
	if ok {
		return &l, true
	} else {
		if !create {
			return nil, false
		} else {
			m := Sublist{}
			m.list = make(map[subscriber.Subscriber]bool)
			m.key = key
			m.mu = &sync.Mutex{}
			m.prune_dur = 20 * time.Second
			m.data = []byte(fmt.Sprintf(`{"tracker_id" : %d}`, key))
			s.list[key] = m
			return &m, true
		}
	}
}

func (s *Sublist) Subscribe(sub subscriber.Subscriber) {
	s.mu.Lock()
	s.list[sub] = true
	sub.Push(s.key, s.data)
	s.mu.Unlock()
}

func (s *Sublist) Unsubscribe(sub subscriber.Subscriber) {
	s.mu.Lock()
	delete(s.list, sub)
	s.mu.Unlock()
}

func (s *Sublist) MarshalSend(lat, lon float64, speed float32, gps_time, server_time time.Time) {
	s.mu.Lock()
	obj := downstream_type{TrackerId: s.key, GpsTime: gps_time, ServerTime: server_time, Speed: speed, Latitude: lat, Longitude: lon}
	s.data, _ = json.Marshal(obj)
	for sub := range s.list {
		closed := sub.Push(s.key, s.data)
		if closed {
			delete(s.list, sub)
		}
	}
	s.mu.Unlock()
}

type downstream_type struct {
	TrackerId  uint64    `json:"tid"`
	ServerTime time.Time `json:"server_time"`
	GpsTime    time.Time `json:"gps_time"`
	Latitude   float64   `json:"latitude"`
	Longitude  float64   `json:"longitude"`
	Speed      float32   `json:"speed"`
}

func (s *Sublist) Send(sender uint64, d []byte) {
	s.mu.Lock()
	for sub := range s.list {
		closed := sub.Push(sender, d)
		if closed {
			delete(s.list, sub)
		}
	}
	s.mu.Unlock()
}

// func (s *Sublist) Prune() {
// 	s.mu.Lock()
// 	for k := range s.list {
// 		if k.Closed() {
// 			delete(s.list, k)
// 		}
// 	}
// 	s.mu.Unlock()
// }

// func (s *Sublist) Prune() {
// 	s.mu.Lock()
// 	defer s.mu.Unlock()
// 	olen := len(s.list)
// 	tail := olen - 1
// look_bad:
// 	for i := 0; i < olen; i++ {
// 		if s.list[i].err != nil || s.list[i].sub.Closed() { //index i is bad list
// 			//look for replacement
// 			for j := tail; j > i; j-- {
// 				if s.list[j].err == nil && !s.list[j].sub.Closed() {
// 					s.list[i] = s.list[j] //j is good index, replace i with j
// 					if i+1 == j {
// 						//if i and j is adjacent, nothing more to iterate
// 						//i is last known good index, so trim to i+1
// 						s.list = s.list[:i+1]
// 						return
// 					}
// 					tail = j - 1
// 					continue look_bad
// 				}
// 			}
// 			//found no replacement, trim to i because i is last bad index
// 			s.list = s.list[:i]
// 			return
// 		} else if i == tail { //index is is not bad, and happen to be equal with tail
// 			s.list = s.list[:i+1]
// 			return
// 		}
// 	}
// }

// func (s *Sublist) SendPrune(sender string, d []byte) {
// 	olen := len(s.list)
// 	tail := olen - 1
// look_bad:
// 	for i := 0; i < olen; i++ {
// 		if s.list[i].err != nil || s.list[i].sub.Closed() { //index i is bad list
// 			//look for replacement
// 			for j := tail; j > i; j-- {
// 				if s.list[j].err == nil && !s.list[j].sub.Closed() {
// 					s.list[i] = s.list[j] //j is good index, replace i with j
// 					err := s.list[i].sub.Push(sender, d)
// 					s.list[i].err = err
// 					if i+1 == j {
// 						//if i and j is adjacent, nothing more to iterate
// 						//i is last known good index, so trim to i+1
// 						s.list = s.list[:i+1]
// 						return
// 					}
// 					tail = j - 1
// 					continue look_bad
// 				}
// 			}
// 			//found no replacement, trim to i because i is last bad index
// 			s.list = s.list[:i]
// 			return
// 		} else if i == tail { //index i is not bad, and happen to be equal with tail
// 			s.list = s.list[:i+1]
// 			err := s.list[i].sub.Push(sender, d)
// 			s.list[i].err = err
// 			return
// 		}
// 		err := s.list[i].sub.Push(sender, d)
// 		s.list[i].err = err
// 	}
// }
