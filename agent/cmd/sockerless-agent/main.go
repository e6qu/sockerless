package main

import (
	"flag"
	"os"

	"github.com/rs/zerolog"
	"github.com/sockerless/agent"
)

func main() {
	addr := flag.String("addr", ":9111", "listen address")
	callback := flag.String("callback", "", "reverse connect URL (FaaS mode)")
	keepAlive := flag.Bool("keep-alive", false, "run the remaining args as a main process and serve until exit")
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	flag.Parse()

	level, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		Level(level).
		With().Timestamp().Str("component", "agent").Logger()

	token := os.Getenv("SOCKERLESS_AGENT_TOKEN")

	config := agent.Config{
		Addr:        *addr,
		Token:       token,
		KeepAlive:   *keepAlive,
		CallbackURL: *callback,
		Args:        flag.Args(),
	}

	server := agent.NewServer(config, logger)

	if *callback != "" {
		err = server.ReverseConnect(*callback)
	} else {
		err = server.ListenAndServe()
	}

	if err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
