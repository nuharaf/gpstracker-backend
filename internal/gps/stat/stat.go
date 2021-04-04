package stat

import (
	"sync"
	"time"
)

type counter struct {
	base time.Time
	cnt  uint64
}

type time_event struct {
	list [10]time.Time
	idx  int
	mu   sync.Mutex
}

type Stat struct {
	login      time_event
	connect    time_event
	disconnect time_event
	mu         sync.Mutex
	buf        [100]counter
	phead      int
	dur        time.Duration

	created time.Time
}

func (s *Stat) LoginEv(t time.Time) {
	log(&s.login, t)
}
func (s *Stat) ConnectEv(t time.Time) {
	log(&s.connect, t)
}
func (s *Stat) DisconnectEv(t time.Time) {
	log(&s.disconnect, t)
}

func log(l *time_event, t time.Time) {
	l.mu.Lock()
	l.list[l.idx] = t
	l.idx = l.idx + 1
	if l.idx == len(l.list) {
		l.idx = 0
	}
	l.mu.Unlock()
}

func NewStat() *Stat {
	o := &Stat{}
	o.dur = time.Minute
	o.created = time.Now()
	return o
}

func (s *Stat) CounterIncr(amt uint64, t time.Time) {
	s.mu.Lock()
	f := t.Truncate(s.dur)
	last := s.buf[s.phead]
	if f.After(last.base) {
		if last.cnt != 0 {
			s.phead = s.phead + 1
			if s.phead == len(s.buf) {
				s.phead = 0
			}
		}
		s.buf[s.phead].base = f
		s.buf[s.phead].cnt = amt
	} else if f.Equal(last.base) {
		last.cnt = last.cnt + amt
	}
	s.mu.Unlock()
}
