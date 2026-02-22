package memory

import (
	"context"
	"os"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// NewServer creates a new memory backend server.
// All behavior comes from the core BaseServer with default handlers.
// The WASM sandbox is initialized and injected as a ProcessFactory
// unless SOCKERLESS_SYNTHETIC=1 is set (for CI runners that require
// their own helper binaries, like gitlab-runner).
func NewServer(logger zerolog.Logger) *core.BaseServer {
	s := core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
		ID:              "memory-backend",
		Name:            "sockerless-memory",
		ServerVersion:   "0.1.0",
		Driver:          "memory",
		OperatingSystem: "Sockerless Memory Backend",
		OSType:          "linux",
		Architecture:    "amd64",
		NCPU:            1,
		MemTotal:        1073741824,
	}, core.RouteOverrides{}, logger)

	if os.Getenv("SOCKERLESS_SYNTHETIC") != "1" {
		factory, err := newSandboxFactory(context.Background())
		if err != nil {
			logger.Fatal().Err(err).Msg("failed to initialize WASM sandbox")
		}
		s.ProcessFactory = factory
		s.InitDrivers() // Re-init drivers now that ProcessFactory is set
	} else {
		logger.Info().Msg("synthetic mode: WASM sandbox disabled")
	}

	return s
}
