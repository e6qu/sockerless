package main

import (
	"flag"
	"os"

	"github.com/rs/zerolog"
	"github.com/sockerless/bleephub"
)

func main() {
	addr := flag.String("addr", ":5555", "listen address")
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	flag.Parse()

	level, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		With().Timestamp().Str("service", "bleephub").Logger().
		Level(level)

	srv := bleephub.NewServer(*addr, logger)
	if err := srv.ListenAndServe(); err != nil {
		logger.Fatal().Err(err).Msg("server exited")
	}
}
