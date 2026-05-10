package main

import (
	"context"
	"flag"
	"os"

	"github.com/rs/zerolog"
	"github.com/sockerless/bleephub"
)

var (
	version = "dev"
	commit  = "none"
)

func main() {
	addr := flag.String("addr", ":5555", "listen address")
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	flag.Parse()

	level, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	obs, err := bleephub.InitObservability("bleephub")
	if err != nil {
		bootLogger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr})
		bootLogger.Fatal().Err(err).Msg("failed to init observability")
	}
	defer func() { _ = obs.Shutdown(context.Background()) }()

	var output zerolog.LevelWriter
	consoleW := zerolog.ConsoleWriter{Out: os.Stderr}
	if obs.LogWriter != nil {
		output = zerolog.MultiLevelWriter(consoleW, obs.LogWriter)
	} else {
		output = zerolog.MultiLevelWriter(consoleW)
	}
	logger := zerolog.New(output).
		With().Timestamp().Str("service", "bleephub").Logger().
		Level(level)

	logger.Info().Str("version", version).Str("commit", commit).Msg("starting")

	srv := bleephub.NewServer(*addr, logger)
	if err := srv.ListenAndServe(); err != nil {
		logger.Fatal().Err(err).Msg("server exited")
	}
}
