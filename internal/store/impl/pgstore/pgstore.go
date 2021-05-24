package pgstore

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Store struct {
	ch     chan record
	timer  *time.Timer
	dbc    *pgxpool.Conn
	dbp    *pgxpool.Pool
	logger zerolog.Logger
	dur    time.Duration
	table  string
}

type record struct {
	rid  string
	lon  float64
	lat  float64
	alt  float32
	gpst time.Time
	srvt time.Time
}

func NewStore(db *pgxpool.Pool, table string) *Store {
	var err error
	o := &Store{}
	o.dur = 2 * time.Second
	o.table = table
	o.ch = make(chan record, 10)
	o.timer = time.NewTimer(o.dur)
	o.dbp = db
	o.dbc, err = db.Acquire(context.Background())
	o.logger = log.With().Str("module", "store").Logger()
	if err != nil {
		return nil
	}
	go o.writer()
	return o

}

func (st *Store) Put(rid string, lon float64, lat float64, alt float32, speed float32, gpst time.Time, srvt time.Time) {
	rec := record{rid: rid, lon: lon, lat: lat, alt: alt, gpst: gpst, srvt: srvt}
	select {
	case st.ch <- rec:
	default:
		st.logger.Error().Msg("Store put blocked")
	}
}

func (st *Store) rearmTimer() {
	if !st.timer.Stop() {
		<-st.timer.C
	}
	st.timer.Reset(10 * time.Second)
}

func (st *Store) writer() {
	buffer := make([]record, 20)
	c := 0
	for {
		select {
		case r := <-st.ch:
			buffer[c] = r
			c = c + 1
			if c == len(buffer) {
				st.logger.Debug().Msg("Flush due to full buffer")
				st.flush(buffer[:c])
				c = 0
				st.rearmTimer()
			}
		case <-st.timer.C:
			if c != 0 {
				st.logger.Debug().Msg("Flush due to expired timer")
				st.flush(buffer[:c])
				c = 0
			} else {
				st.logger.Debug().Msg("Timer expired but no data to flush")
			}

		}
	}
}

func (st *Store) flush(data []record) {
	l := len(data)
	t0 := time.Now()
	_, err := st.dbc.CopyFrom(context.Background(),
		pgx.Identifier{st.table},
		[]string{"rid", "longitude", "latitude", "altitude", "gps_time", "server_time"},
		pgx.CopyFromSlice(len(data), func(i int) ([]interface{}, error) {
			d := data[i]
			return []interface{}{d.rid, d.lon, d.lat, d.alt, d.gpst, d.srvt}, nil
		}))

	if err != nil {
		st.logger.Err(err).Msg("Flushing error")
	} else {
		st.logger.Debug().Str("action", "flush").Int("length", l).Dur("time_taken", time.Since(t0)).Msg("Flush successfull")
	}
}
