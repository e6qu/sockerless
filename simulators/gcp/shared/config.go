// Package simulator provides a shared framework for building cloud service simulators.
//
// Each simulator is an HTTP server that implements a subset of a cloud provider's
// REST API surface. The framework provides common infrastructure: HTTP server,
// request routing, in-memory state management, authentication passthrough,
// and provider-specific error formatting.
package simulator

import "os"

// Config holds the simulator server configuration.
type Config struct {
	// ListenAddr is the address to listen on (e.g., ":4566").
	ListenAddr string

	// TLSCert is the path to the TLS certificate file. Empty disables TLS.
	TLSCert string

	// TLSKey is the path to the TLS private key file.
	TLSKey string

	// LogLevel is the zerolog log level (trace, debug, info, warn, error).
	LogLevel string

	// Provider identifies the cloud provider (aws, gcp, azure).
	Provider string
}

// ConfigFromEnv loads configuration from environment variables.
//
//	SIM_LISTEN_ADDR — listen address (default ":8443")
//	SIM_TLS_CERT    — TLS certificate file path
//	SIM_TLS_KEY     — TLS private key file path
//	SIM_LOG_LEVEL   — log level (default "info")
func ConfigFromEnv(provider string) Config {
	return Config{
		ListenAddr: envOrDefault("SIM_LISTEN_ADDR", ":8443"),
		TLSCert:    os.Getenv("SIM_TLS_CERT"),
		TLSKey:     os.Getenv("SIM_TLS_KEY"),
		LogLevel:   envOrDefault("SIM_LOG_LEVEL", "info"),
		Provider:   provider,
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
