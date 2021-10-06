package main

import (
	"github.com/phuslu/log"
)

func main() {
	logger := log.DefaultLogger
	logger.Context = log.NewContext(nil).Str("first", "first").Value()
	logger.Info().Caller(1, false).Msg("first level")
	logger.Context = log.NewContext(logger.Context).Str("second", "second").Value()
	logger.Info().Caller(1, false).Msg("second level")
	logger.Context = log.NewContext(logger.Context).Str("third", "third").Value()
	logger.Info().Caller(1, false).Msg("third  level")

}
