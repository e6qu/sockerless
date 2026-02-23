package main

import (
	"context"
	"flag"
	"os"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
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

	core.LoadContextEnv(logger)

	shutdown, err := core.InitTracer("sockerless-backend")
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to init tracer")
	}
	defer shutdown(context.Background())

	s := memory.NewServer(logger)
	logger.Info().Str("addr", *addr).Msg("starting memory backend")
	if err := s.ListenAndServe(*addr); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
