package main

import (
	"context"
	"flag"
	"os"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
	docker "github.com/sockerless/backend-docker"
)

func main() {
	addr := flag.String("addr", ":9100", "listen address")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file")
	tlsKey := flag.String("tls-key", "", "TLS key file")
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

	shutdown, err := core.InitTracer("sockerless-backend-docker")
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to init tracer")
	}
	defer func() { _ = shutdown(context.Background()) }()

	s, err := docker.NewServer(logger, *dockerHost)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to create server")
	}

	logger.Info().Str("addr", *addr).Msg("starting docker backend")
	if err := s.ListenAndServe(*addr, *tlsCert, *tlsKey); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
