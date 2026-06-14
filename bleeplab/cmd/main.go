// Command bleeplab runs the GitLab control-plane simulator: a real
// gitlab-runner registers against it and runs CI jobs through a
// `--docker-host` pointed at a sockerless backend.
package main

import (
	"flag"
	"os"

	"github.com/rs/zerolog"
	"github.com/sockerless/bleeplab"
)

func main() {
	addr := flag.String("addr", ":8929", "listen address")
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	flag.Parse()

	lvl, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(lvl).With().Timestamp().Str("service", "bleeplab").Logger()

	srv := bleeplab.NewServer(*addr, logger)
	if err := srv.ListenAndServe(); err != nil {
		logger.Fatal().Err(err).Msg("bleeplab server exited")
	}
}
