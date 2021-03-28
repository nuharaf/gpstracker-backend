package store

import (
	"context"
	"fmt"
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
	rid string
	lon float64
	lat float64
	alt float32
	t   time.Time
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

func (st *Store) Put(rid string, lon float64, lat float64, alt float32, t time.Time) {
	rec := record{rid: rid, lon: lon, lat: lat, alt: alt, t: t}
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
				st.logger.Error().Msg("Flush due to full buffer")
				st.flush(buffer[:c])
				c = 0
				st.rearmTimer()
			}
		case <-st.timer.C:
			if c != 0 {
				st.logger.Error().Msg("Flush due to expired timer")
				st.flush(buffer[:c])
				c = 0
			} else {
				st.logger.Error().Msg("Timer expired but no data to flush")
			}

		}
	}
}

func (st *Store) flush(data []record) {
	t0 := time.Now()

	t1 := time.Now()
	_, err = st.dbc.CopyFrom(context.Background(),
		pgx.Identifier{st.table},
		[]string{"rid", "lon", "lat", "alt", "gpst"},
		pgx.CopyFromSlice(len(data), func(i int) ([]interface{}, error) {
			d := data[i]
			return []interface{}{d.rid, d.lon, d.lat, d.alt, d.gpst}, nil
		}))

	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(time.Since(t1).Nanoseconds())

}
