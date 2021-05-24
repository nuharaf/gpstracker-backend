package main

import (
	"flag"

	"github.com/rs/zerolog"
	gps "nuha.dev/gpstracker/internal/gps/serverimpl"
)

func main() {
	debug := flag.Bool("debug", true, "sets log level to debug")
	listen_addr := flag.String("address", ":5555", "address to listen to")
	flag.Parse()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	srv := gps.NewServer(nil, &gps.ServerConfig{MockLogin: true, DirectListenerAddr: *listen_addr, MockStore: true})
	srv.Run()
}
