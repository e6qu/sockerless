package main

import (
	"context"
	"flag"
	"os"

	"github.com/rs/zerolog"
	"github.com/sockerless/gitlabhub"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	flag.Parse()

	level, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		With().Timestamp().Str("service", "gitlabhub").Logger().
		Level(level)

	logger.Info().Str("version", version).Str("commit", commit).Msg("starting")

	shutdown, err := gitlabhub.InitTracer("gitlabhub")
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to init tracer")
	}
	defer shutdown(context.Background())

	srv := gitlabhub.NewServer(*addr, logger)
	if err := srv.ListenAndServe(); err != nil {
		logger.Fatal().Err(err).Msg("server exited")
	}
}
