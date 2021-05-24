package main

import (
	"flag"

	"github.com/rs/zerolog"
	gps "nuha.dev/gpstracker/internal/gps/serverimpl"
)

func main() {
	debug := flag.Bool("debug", false, "sets log level to debug")
	flag.Parse()
	zerolog.SetGlobalLevel(zerolog.TraceLevel)
	srv := gps.NewServer(nil, &gps.ServerConfig{MockLogin: true})
	srv.Run()
}
