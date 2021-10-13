package stat

import (
	"sync"
	"time"
)

type time_event struct {
	list [10]time.Time
	idx  int
	mu   *sync.Mutex
}

type Stat struct {
	connect time_event
	update  time_event
}

func (s *Stat) getAsList(t *time_event) []time.Time {
	l := make([]time.Time, 0, 10)
	t.mu.Lock()
	l = append(l, t.list[t.idx+1:]...)
	if !t.list[t.idx].IsZero() {
		l = append(l, t.list[:t.idx+1]...)
	}
	t.mu.Unlock()
	return l
}

func (s *Stat) getLastEvent(t *time_event) time.Time {
	var e time.Time
	t.mu.Lock()
	if t.idx == (len(t.list) - 1) {
		e = t.list[0]
	} else {
		e = t.list[t.idx+1]
	}
	t.mu.Unlock()
	return e
}

func (s *Stat) ConnectLast() time.Time {
	return s.getLastEvent(&s.connect)
}

func (s *Stat) UpdateLast() time.Time {
	return s.getLastEvent(&s.update)
}

func (s *Stat) ConnectList() []time.Time {
	return s.getAsList(&s.connect)
}

func (s *Stat) UpdateList() []time.Time {
	return s.getAsList(&s.update)
}

func (s *Stat) UpdateEv(t time.Time) {
	log(&s.update, t)
}

func (s *Stat) ConnectEv(t time.Time) {
	log(&s.connect, t)
}

func log(l *time_event, t time.Time) {
	l.mu.Lock()
	l.list[l.idx] = t
	if l.idx == 0 {
		l.idx = len(l.list) - 1
	} else {
		l.idx = l.idx - 1
	}

	l.mu.Unlock()
}

func NewStat() *Stat {
	o := &Stat{}
	o.connect = new_time_event()
	o.update = new_time_event()
	return o
}

func new_time_event() time_event {
	t := time_event{}
	t.mu = &sync.Mutex{}
	t.idx = len(t.list) - 1
	return t
}

// func (s *Stat) CounterIncr(amt uint64, t time.Time) {
// 	s.mu.Lock()
// 	f := t.Truncate(s.dur)
// 	last := s.buf[s.phead]
// 	if f.After(last.base) {
// 		if last.cnt != 0 {
// 			s.phead = s.phead + 1
// 			if s.phead == len(s.buf) {
// 				s.phead = 0
// 			}
// 		}
// 		s.buf[s.phead].base = f
// 		s.buf[s.phead].cnt = amt
// 	} else if f.Equal(last.base) {
// 		last.cnt = last.cnt + amt
// 	}
// 	s.mu.Unlock()
// }
