package sublist

import (
	"sync"
	"time"

	"nuha.dev/gpstracker/internal/gps/subscriber"
)

type MulSublist struct {
	slow       *Sublist
	normal     *Sublist
	slow_dur   time.Duration
	prune_dur  time.Duration
	last_slow  time.Time
	last_prune time.Time
}

func NewMulSublist() *MulSublist {
	o := &MulSublist{}
	o.slow = NewSublist()
	o.normal = NewSublist()
	o.slow_dur = 5 * time.Second
	o.prune_dur = 20 * time.Second
	o.last_prune = time.Now()
	return o
}

func (m *MulSublist) Send(sender string, d []byte) {
	m.normal.Send(sender, d)
	t0 := time.Now()
	if t0.Sub(m.last_slow) > m.slow_dur {
		m.slow.Send(sender, d)
		m.last_slow = t0
	}
	if t0.Sub(m.last_prune) > m.prune_dur {
		m.normal.Prune()
		m.slow.Prune()
		m.last_prune = t0
	}
}

func (m *MulSublist) Subscribe(sub subscriber.Subscriber) {
	m.normal.Subscribe(sub)
}

func (m *MulSublist) SubscribeSlow(sub subscriber.Subscriber) {
	m.slow.Subscribe(sub)
}

type subflag struct {
	sub    subscriber.Subscriber
	err    error
	closed bool
}

type Sublist struct {
	list []subflag
	mu   sync.Mutex
}

func NewSublist() *Sublist {
	o := &Sublist{}
	o.list = make([]subflag, 0, 20)
	return o
}

func (s *Sublist) Subscribe(sub subscriber.Subscriber) {
	s.mu.Lock()
	s.list = append(s.list, subflag{sub: sub})
	s.mu.Unlock()
}

func (s *Sublist) Send(sender string, d []byte) {
	for _, sub := range s.list {
		err := sub.sub.Push(sender, d)
		sub.err = err
	}
}

func (s *Sublist) Prune() {
	olen := len(s.list)
	tail := olen - 1
look_bad:
	for i := 0; i < olen; i++ {
		if s.list[i].err != nil || s.list[i].sub.Closed() { //index i is bad list
			//look for replacement
			for j := tail; j > i; j-- {
				if s.list[j].err == nil && !s.list[j].sub.Closed() {
					s.list[i] = s.list[j] //j is good index, replace i with j
					if i+1 == j {
						//if i and j is adjacent, nothing more to iterate
						//i is last known good index, so trim to i+1
						s.list = s.list[:i+1]
						return
					}
					tail = j - 1
					continue look_bad
				}
			}
			//found no replacement, trim to i because i is last bad index
			s.list = s.list[:i]
			return
		} else if i == tail { //index is is not bad, and happen to be equal with tail
			s.list = s.list[:i+1]
			return
		}
	}
}

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
