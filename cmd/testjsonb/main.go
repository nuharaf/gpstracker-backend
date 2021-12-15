package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v4/pgxpool"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	pool, err := pgxpool.Connect(context.Background(), "postgresql://postgres:postgres@localhost/gpsv2")
	if err != nil {
		log.Fatal(err)
	}

	var d map[string]bool
	err = pool.QueryRow(context.Background(), "select roles from public.user where id=1").Scan(&d)
	if err != nil {
		log.Print(err)
	}
	fmt.Printf("%v", d)
}
