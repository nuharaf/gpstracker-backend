package pgstore

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/phuslu/log"
)

type Store struct {
	config *StoreConfig
	cond   *sync.Cond
	wlock  *sync.Mutex
	rbuf   buffer
	wbuf   buffer
	dbc    *pgxpool.Conn
	dbp    *pgxpool.Pool
	log    log.Logger
	table  string
}

type StoreConfig struct {
	BufSize     int
	TickerDur   time.Duration
	MaxAgeFlush time.Duration
}

type buffer struct {
	seq uint64
	t1  time.Time
	t2  time.Time
	buf []record
}

func new_buffer(seq uint64, len int) buffer {
	return buffer{seq: seq, buf: make([]record, 0, len)}
}

type record struct {
	fsn   string
	lon   float64
	lat   float64
	alt   float32
	speed float32
	gpst  time.Time
	srvt  time.Time
}

func NewStore(db *pgxpool.Pool, table string, config *StoreConfig) *Store {
	o := &Store{}
	o.config = config
	o.table = table
	o.dbp = db
	o.log = log.DefaultLogger
	o.log.Context = log.NewContext(nil).Str("module", "pgstore").Value()
	o.wbuf = new_buffer(0, o.config.BufSize)
	o.wlock = &sync.Mutex{}
	o.cond = sync.NewCond(&sync.Mutex{})
	return o
}

func (st *Store) Run() {
	var err error
	st.dbc, err = st.dbp.Acquire(context.Background())
	if err != nil {
		return
	}
	go st.timer_flusher()
	go st.handle()
}

func (st *Store) timer_flusher() {
	ticker := time.NewTicker(st.config.TickerDur)
	for t := range ticker.C {
		st.wlock.Lock()
		if len(st.wbuf.buf) != 0 && t.Sub(st.wbuf.t1) > st.config.MaxAgeFlush {
			st.flush()
		}
		st.wlock.Unlock()
	}
}

func (st *Store) Put(serial_number string, lon float64, lat float64, alt float32, speed float32, gpst time.Time, srvt time.Time) {
	rec := record{fsn: serial_number, lon: lon, lat: lat, alt: alt, speed: speed, gpst: gpst, srvt: srvt}
	st.wlock.Lock()
	if len(st.wbuf.buf) == 0 {
		st.wbuf.t1 = time.Now().UTC()
	}
	st.wbuf.buf = append(st.wbuf.buf, rec)
	if len(st.wbuf.buf) == st.config.BufSize {
		st.flush()
	}
	st.wlock.Unlock()
}

func (st *Store) flush() {
	next := st.wbuf.seq + 1
	st.wbuf.t2 = time.Now().UTC()
	st.cond.L.Lock()
	st.rbuf = st.wbuf
	st.cond.L.Unlock()
	st.cond.Signal()
	st.wbuf = new_buffer(next, st.config.BufSize)

}

// func (st *Store) flush(data []record) {
// 	l := len(data)
// 	t0 := time.Now()
// 	_, err := st.dbc.CopyFrom(context.Background(),
// 		pgx.Identifier{st.table},
// 		[]string{"fsn", "longitude", "latitude", "altitude", "speed", "gps_time", "server_time"},
// 		pgx.CopyFromSlice(len(data), func(i int) ([]interface{}, error) {
// 			d := data[i]
// 			return []interface{}{d.fsn, d.lon, d.lat, d.alt, d.speed, d.gpst, d.srvt}, nil
// 		}))

// 	if err != nil {
// 		st.logger.Err(err).Msg("Flushing error")
// 	} else {
// 		st.logger.Debug().Str("action", "flush").Int("length", l).Dur("time_taken", time.Since(t0)).Msg("Flush successfull")
// 	}
// }

func (st *Store) handle() {
	var err error
	st.log.Info().Msg("starting flusher task")
	for {
		st.cond.L.Lock()
		st.cond.Wait()
		st.log.Debug().Msg("flusher task signalled")
		buf := st.rbuf
		st.cond.L.Unlock()
		t1 := time.Now()
		_, err = st.dbc.CopyFrom(context.Background(),
			pgx.Identifier{st.table},
			[]string{"fsn", "longitude", "latitude", "altitude", "speed", "gps_time", "server_time"},
			pgx.CopyFromSlice(len(buf.buf), func(i int) ([]interface{}, error) {
				d := buf.buf[i]
				return []interface{}{d.fsn, d.lon, d.lat, d.alt, d.speed, d.gpst, d.srvt}, nil
			}))
		if err != nil {
			st.log.Error().Err(err).Msg("flush error")
		} else {
			st.log.Debug().Str("action", "flush").Int("length", len(buf.buf)).Dur("time_taken", time.Since(t1)).Msg("flush successfull")
		}
	}

}
