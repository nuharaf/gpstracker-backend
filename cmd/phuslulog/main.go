package main

import (
	"errors"

	plog "github.com/phuslu/log"
	zlog "github.com/rs/zerolog/log"
)

func main() {
	var e = errors.New("help")
	plog.DefaultLogger.Error().Err(e).Msg("")
	zlog.Debug().Err(e).Msg("")
	// logger := log.DefaultLogger
	// logger.Context = log.NewContext(nil).Str("first", "first").Value()
	// logger.Info().Caller(1, false).Msg("first level")
	// logger.Context = log.NewContext(logger.Context).Str("second", "second").Value()
	// logger.Info().Caller(1, false).Msg("second level")
	// logger.Context = log.NewContext(logger.Context).Str("third", "third").Value()
	// logger.Info().Caller(1, false).Msg("third  level")

}
