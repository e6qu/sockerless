package main

import (
	"flag"
	"os"

	"github.com/rs/zerolog"
	docker "github.com/sockerless/backend-docker"
)

func main() {
	addr := flag.String("addr", ":9100", "listen address")
	dockerHost := flag.String("docker-host", "", "Docker daemon socket (default: auto-detect)")
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
		Str("component", "backend-docker").
		Logger()

	s, err := docker.NewServer(logger, *dockerHost)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create server")
	}

	logger.Info().Str("addr", *addr).Msg("starting docker backend")
	if err := s.ListenAndServe(*addr); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
