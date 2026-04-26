package core

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

// DriverContext is the envelope passed to every typed driver call. It
// carries the resolved container, request context, backend identity,
// and a contextual logger so per-dimension drivers don't have to call
// `s.ResolveContainerAuto(...)` themselves and don't have to thread
// the BaseServer through their public surface.
//
// Phase 104: this is the foundation envelope for the 13 typed driver
// dimensions defined in `drivers_phase104.go`. Existing narrow
// interfaces in `drivers.go` (`ExecDriver`, `StreamDriver`,
// `FilesystemDriver`) keep their current shape and are gradually
// absorbed into the typed dimensions one at a time, no behaviour
// change per commit.
type DriverContext struct {
	// Ctx is the request-scoped context. Drivers must honour
	// cancellation; long-running drivers (Build, exec stream) check
	// `ctx.Err()` between IO chunks.
	Ctx context.Context

	// Container is pre-resolved by `BaseServer.ResolveContainerAuto`
	// before the driver call is made. Drivers never re-resolve;
	// they read fields directly (Container.ID, Container.Name,
	// HostConfig, NetworkSettings).
	Container api.Container

	// Backend is the short backend identifier ("docker", "ecs",
	// "lambda", "cloudrun", "gcf", "aca", "azf"). Used by the
	// Describe() composition rule to format NotImpl messages.
	Backend string

	// Region is the cloud region for the call ("eu-west-1",
	// "us-central1", "westeurope") or "" for the local docker
	// backend.
	Region string

	// Logger is a contextual logger preconfigured with backend +
	// container fields. Drivers use this directly without calling
	// `.With().Str(...)` themselves.
	Logger zerolog.Logger
}

// Driver is the common interface implemented by every typed driver
// dimension below (ExecDriver104, AttachDriver104, …). The single
// method Describe() returns a short human-readable description used
// by the NotImpl-composition rule: when an operator hits an action
// whose driver is missing or returns NotImpl, the surfaced error
// names the backend, the dimension, and the missing prerequisite —
// the same shape used in BUG-792's "no phase reference in error"
// fix.
//
// Example Describe() values:
//   - "ecs SSMExec via SSM ExecuteCommand (requires ExecuteCommand-enabled task)"
//   - "lambda ReverseAgentExec (requires SOCKERLESS_CALLBACK_URL on the function)"
//   - "aca ACAConsoleExec via ContainerApp's exec endpoint"
type Driver interface {
	Describe() string
}
