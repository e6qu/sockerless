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
	addr := flag.String("addr", ":3375", "listen address")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file")
	tlsKey := flag.String("tls-key", "", "TLS key file")
	dockerHost := flag.String("docker-host", "", "Docker daemon socket (default: auto-detect)")
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	flag.Parse()

	level, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	obs, err := core.InitObservability("sockerless-backend-docker")
	if err != nil {
		// Bootstrap-time logging via stderr; the proper logger isn't
		// up yet because we want OTel's LogWriter chained into it.
		bootLogger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr})
		bootLogger.Fatal().Err(err).Msg("failed to init observability")
	}
	defer func() { _ = obs.Shutdown(context.Background()) }()

	// Compose stderr-formatted output with optional OTLP log emission.
	// MultiLevelWriter fans the JSON-encoded zerolog event to both:
	// ConsoleWriter formats it for stderr; OTelLogWriter parses it
	// and emits a Record. When OTel is disabled, LogWriter is nil
	// and we use ConsoleWriter alone.
	var output zerolog.LevelWriter
	consoleW := zerolog.ConsoleWriter{Out: os.Stderr}
	if obs.LogWriter != nil {
		output = zerolog.MultiLevelWriter(consoleW, obs.LogWriter)
	} else {
		output = zerolog.MultiLevelWriter(consoleW)
	}
	logger := zerolog.New(output).
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
	if err := s.ListenAndServe(*addr, *tlsCert, *tlsKey); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
