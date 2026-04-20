package main

import (
	"context"
	"flag"
	"os"

	"github.com/rs/zerolog"
	backend "github.com/sockerless/backend-cloudrun"
	core "github.com/sockerless/backend-core"
)

func main() {
	addr := flag.String("addr", ":3375", "listen address")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file")
	tlsKey := flag.String("tls-key", "", "TLS key file")
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	flag.Parse()

	level, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		Level(level).
		With().Timestamp().Str("component", "backend-cloudrun").Logger()

	var config backend.Config
	if cfg, env, _, err := core.ActiveEnvironmentWithConfig(); err == nil {
		sim, _ := cfg.ResolveSimulator(env)
		config = backend.ConfigFromEnvironment(env, sim)
		logger.Info().Msg("loaded config from config.yaml")
	} else {
		core.LoadContextEnv(logger)
		config = backend.ConfigFromEnv()
	}
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
	if err := s.ListenAndServe(*addr, *tlsCert, *tlsKey); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
