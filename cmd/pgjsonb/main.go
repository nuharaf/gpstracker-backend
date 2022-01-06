package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v4/pgxpool"
)

func main() {

	pool, err := pgxpool.Connect(context.Background(), "postgresql://postgres:postgres@localhost/gpsv2")
	if err != nil {
		panic(err.Error())
	}
	attr := make(map[string]string)
	err = pool.QueryRow(context.Background(), "SELECT attribute from tracker where id = 21").Scan(&attr)
	log.Print(err)
	for k, v := range attr {
		log.Print(k, v)
	}
}
