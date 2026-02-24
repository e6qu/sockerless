package main

import (
	"context"
	"flag"
	"os"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
	backend "github.com/sockerless/backend-cloudrun"
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
		With().Timestamp().Str("component", "backend-cloudrun").Logger()

	core.LoadContextEnv(logger)
	config := backend.ConfigFromEnv()
	if err := config.Validate(); err != nil {
		logger.Fatal().Err(err).Msg("invalid configuration")
	}

	gcpClients, err := backend.NewGCPClients(context.Background(), config.Project, config.EndpointURL)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to initialize GCP clients")
	}

	s := backend.NewServer(config, gcpClients, logger)
	if err := s.RecoverRegistry(context.Background(), s); err != nil {
		logger.Warn().Err(err).Msg("registry recovery failed (continuing)")
	}
	logger.Info().Str("addr", *addr).Str("project", config.Project).Msg("starting Cloud Run backend")
	if err := s.ListenAndServe(*addr); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
