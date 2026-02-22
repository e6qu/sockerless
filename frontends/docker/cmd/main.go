package main

import (
	"flag"
	"os"

	"github.com/rs/zerolog"
	frontend "github.com/sockerless/frontend"
)

func main() {
	addr := flag.String("addr", ":2375", "listen address (host:port or /path/to/socket)")
	backend := flag.String("backend", "http://localhost:9100", "backend address")
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
		Str("component", "docker-frontend").
		Logger()

	s := frontend.NewServer(logger, *backend)
	logger.Info().Str("addr", *addr).Str("backend", *backend).Msg("starting docker frontend")
	if err := s.ListenAndServe(*addr); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
