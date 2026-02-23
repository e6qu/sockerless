package main

import (
	"flag"
	"os"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
	frontend "github.com/sockerless/frontend"
)

func main() {
	addr := flag.String("addr", ":2375", "listen address (host:port or /path/to/socket)")
	backend := flag.String("backend", "http://localhost:9100", "backend address")
	mgmtAddr := flag.String("mgmt-addr", ":9080", "management API listen address")
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	tlsCert := flag.String("tls-cert", "", "TLS certificate file")
	tlsKey := flag.String("tls-key", "", "TLS private key file")
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

	core.LoadContextEnv(logger)
	s := frontend.NewServer(logger, *backend)

	// Start management API in background, linked to frontend for request counting
	mgmt := frontend.NewMgmtServer(logger, *addr, *backend)
	s.Mgmt = mgmt
	go func() {
		if err := mgmt.ListenAndServe(*mgmtAddr, *tlsCert, *tlsKey); err != nil {
			logger.Error().Err(err).Msg("management API failed")
		}
	}()

	logger.Info().Str("addr", *addr).Str("backend", *backend).Msg("starting docker frontend")
	if err := s.ListenAndServe(*addr, *tlsCert, *tlsKey); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
