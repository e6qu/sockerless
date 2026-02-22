package main

import (
	"flag"
	"os"

	"github.com/rs/zerolog"
	memory "github.com/sockerless/backend-memory"
)

func main() {
	addr := flag.String("addr", ":9100", "listen address")
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	flag.Parse()

	level, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		Level(level).
		With().
		Timestamp().
		Str("component", "backend-memory").
		Logger()

	s := memory.NewServer(logger)
	logger.Info().Str("addr", *addr).Msg("starting memory backend")
	if err := s.ListenAndServe(*addr); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
