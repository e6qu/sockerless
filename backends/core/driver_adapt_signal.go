package core

import "errors"

// SignalDriver narrow→typed adapter. Mirrors the lift-1/2/3 shape.
//
// `WrapLegacyKill` adapts the existing per-backend `ContainerKill`
// signature (`func(ref, signal) error`) into the typed `SignalDriver`.
// Backends that override `BaseServer.ContainerKill` today (every cloud
// backend, via `self`-dispatch) plug their existing function into
// `DriverSet104.Signal` without behaviour change. Pause/unpause are
// expressed as Kill("SIGSTOP") / Kill("SIGCONT") per the SignalDriver
// contract — no separate adapter for those today.
//
// A typed cloud-native SignalDriver impl (SSMKill on ECS,
// ReverseAgentKill on FaaS, ACAConsoleKill on ACA) lands per-backend
// under the migration step, not under this lift — those each touch
// distinct cloud machinery.

// LegacyKillFn matches the existing `BaseServer.ContainerKill`
// signature so per-backend overrides plug into `WrapLegacyKill`
// without further glue.
type LegacyKillFn func(ref string, signal string) error

// WrapLegacyKill returns a SignalDriver that delegates to the supplied
// legacy function. `backend` and `impl` populate `Describe()` for the
// `NotImplDriverError` formatter.
func WrapLegacyKill(fn LegacyKillFn, backend, impl string) SignalDriver {
	return &legacyKillAdapter{fn: fn, backend: backend, impl: impl}
}

type legacyKillAdapter struct {
	fn      LegacyKillFn
	backend string
	impl    string
}

func (a *legacyKillAdapter) Describe() string {
	if a.backend == "" && a.impl == "" {
		return "signal via legacy ContainerKill function"
	}
	return a.backend + " " + a.impl + " (legacy ContainerKill adapter)"
}

func (a *legacyKillAdapter) Kill(dctx DriverContext, signal string) error {
	if a.fn == nil {
		return errors.New("legacy signal adapter: function is nil — register one via WrapLegacyKill(s.ContainerKill, ...) or replace this adapter with a typed SignalDriver")
	}
	return a.fn(dctx.Container.ID, signal)
}
