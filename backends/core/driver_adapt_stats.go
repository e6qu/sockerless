package core

import (
	"errors"
	"io"
)

// StatsDriver narrow→typed adapter. Bridges the shape gap between the
// legacy ContainerStats(ref, stream) (io.ReadCloser, error) and the
// typed StatsDriver.Stats(dctx, stream, w) error by io.Copy'ing the
// returned reader into the supplied writer.

// LegacyStatsFn matches BaseServer.ContainerStats.
type LegacyStatsFn func(ref string, stream bool) (io.ReadCloser, error)

// WrapLegacyStats returns a StatsDriver that delegates to the supplied
// legacy function and pipes its reader output into the typed driver's
// Writer.
func WrapLegacyStats(fn LegacyStatsFn, backend, impl string) StatsDriver {
	return &legacyStatsAdapter{fn: fn, backend: backend, impl: impl}
}

type legacyStatsAdapter struct {
	fn      LegacyStatsFn
	backend string
	impl    string
}

func (a *legacyStatsAdapter) Describe() string {
	if a.backend == "" && a.impl == "" {
		return "stats via legacy ContainerStats function"
	}
	return a.backend + " " + a.impl + " (legacy ContainerStats adapter)"
}

func (a *legacyStatsAdapter) Stats(dctx DriverContext, stream bool, w io.Writer) error {
	if a.fn == nil {
		return errors.New("legacy stats adapter: function is nil")
	}
	rc, err := a.fn(dctx.Container.ID, stream)
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(w, rc)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
