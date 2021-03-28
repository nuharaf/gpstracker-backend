package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"nuha.dev/gpstracker/internal/util"
)

type location struct {
	rid  []byte
	lon  float64
	lat  float64
	alt  float32
	gpst time.Time
}

func main() {
	data := make([]location, 100)

	for i := range data {
		data[i].lat = rand.Float64() * 100
		data[i].lon = rand.Float64() * 100
		data[i].rid = util.GenUUIDb()
		data[i].alt = rand.Float32() * 100
		data[i].gpst = time.Now()
	}

	db_url := "postgresql://postgres:postgres@localhost/gpsv2"
	pool, err := pgxpool.Connect(context.Background(), db_url)
	if err != nil {
		log.Fatal(err)
	}

	// bulk := func() {
	// 	conn, err := pool.Acquire(context.Background())
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	t0 := time.Now()
	// 	for i := 0; i < 10; i++ {
	// 		t1 := time.Now()
	// 		_, err = conn.CopyFrom(context.Background(),
	// 			pgx.Identifier{"locations"},
	// 			[]string{"rid", "lon", "lat", "alt", "gpst"},
	// 			pgx.CopyFromSlice(len(data), func(i int) ([]interface{}, error) {
	// 				d := data[i]
	// 				return []interface{}{d.rid, d.lon, d.lat, d.alt, d.gpst}, nil
	// 			}))

	// 		if err != nil {
	// 			log.Fatal(err)
	// 		}
	// 		fmt.Println(time.Since(t1).Nanoseconds())
	// 	}
	// 	fmt.Println(time.Since(t0).Nanoseconds())
	// }

	txinsert := func() {
		conn, err := pool.Acquire(context.Background())
		if err != nil {
			log.Fatal(err)
		}

		t0 := time.Now()
		for i := 0; i < 10; i++ {
			t1 := time.Now()
			tx, err := conn.Begin(context.Background())
			q := `insert into locations (rid,lon,lat,alt,gpst) values($1,$2,$3,$4,$5)`

			if err != nil {
				log.Fatal(err)
			}
			for _, v := range data {
				_, err := tx.Exec(context.Background(), q, v.rid, v.lon, v.lat, v.alt, v.gpst)
				if err != nil {
					log.Fatal(err)
				}
			}
			err = tx.Commit(context.Background())
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(time.Since(t1).Nanoseconds())
		}
		fmt.Println(time.Since(t0).Nanoseconds())
	}

	txinsert()

}
