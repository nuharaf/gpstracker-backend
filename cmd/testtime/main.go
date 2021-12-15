package main

import (
	"time"

	plog "github.com/phuslu/log"
	zlog "github.com/rs/zerolog/log"
)

func main() {
	zlog.Debug().Time("now", time.Now().UTC()).Msg("")
	plog.Debug().Time("now", time.Now().UTC()).Msg("")

}
