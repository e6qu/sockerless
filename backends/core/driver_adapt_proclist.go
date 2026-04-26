package core

import (
	"errors"

	"github.com/sockerless/api"
)

// ProcListDriver narrow→typed adapter, mirroring the lift pattern of
// the other dimensions. WrapLegacyTop adapts a per-backend
// ContainerTop signature into the typed ProcListDriver shape. Backends
// override s.Typed.ProcList with a typed cloud-native impl when they
// have one.

// LegacyTopFn matches BaseServer.ContainerTop so per-backend overrides
// plug in without further glue.
type LegacyTopFn func(ref string, psArgs string) (*api.ContainerTopResponse, error)

// WrapLegacyTop returns a ProcListDriver that delegates to the
// supplied legacy function.
func WrapLegacyTop(fn LegacyTopFn, backend, impl string) ProcListDriver {
	return &legacyTopAdapter{fn: fn, backend: backend, impl: impl}
}

type legacyTopAdapter struct {
	fn      LegacyTopFn
	backend string
	impl    string
}

func (a *legacyTopAdapter) Describe() string {
	if a.backend == "" && a.impl == "" {
		return "top via legacy ContainerTop function"
	}
	return a.backend + " " + a.impl + " (legacy ContainerTop adapter)"
}

func (a *legacyTopAdapter) Top(dctx DriverContext, psArgs string) (*api.ContainerTopResponse, error) {
	if a.fn == nil {
		return nil, errors.New("legacy proclist adapter: function is nil")
	}
	return a.fn(dctx.Container.ID, psArgs)
}
