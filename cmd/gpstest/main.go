package main

import (
	"nuha.dev/gpstracker/internal/gps/serverimpl"
)

func main() {
	// log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	server := serverimpl.NewServer(":5555")
	server.Run()
}
