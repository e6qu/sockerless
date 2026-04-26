package core

import (
	"errors"

	"github.com/sockerless/api"
)

// FSDiffDriver narrow→typed adapter. WrapLegacyChanges adapts a
// per-backend ContainerChanges signature into the typed FSDiffDriver
// shape. Backends override s.Typed.FSDiff with a typed cloud-native
// impl when they have one (overlay-rootfs upper-dir diff being the
// expected one).

// LegacyChangesFn matches BaseServer.ContainerChanges.
type LegacyChangesFn func(ref string) ([]api.ContainerChangeItem, error)

// WrapLegacyChanges returns an FSDiffDriver that delegates to the
// supplied legacy function.
func WrapLegacyChanges(fn LegacyChangesFn, backend, impl string) FSDiffDriver {
	return &legacyChangesAdapter{fn: fn, backend: backend, impl: impl}
}

type legacyChangesAdapter struct {
	fn      LegacyChangesFn
	backend string
	impl    string
}

func (a *legacyChangesAdapter) Describe() string {
	if a.backend == "" && a.impl == "" {
		return "changes via legacy ContainerChanges function"
	}
	return a.backend + " " + a.impl + " (legacy ContainerChanges adapter)"
}

func (a *legacyChangesAdapter) Changes(dctx DriverContext) ([]api.ContainerChangeItem, error) {
	if a.fn == nil {
		return nil, errors.New("legacy fsdiff adapter: function is nil")
	}
	return a.fn(dctx.Container.ID)
}
