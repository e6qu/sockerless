package core

import (
	"errors"
	"io"

	"github.com/sockerless/api"
)

// BuildDriver narrow→typed adapter. The typed BuildDriver.Build takes
// api.ImageBuildOptions directly, so the adapter is a thin pass-through.

// LegacyBuildFn matches BaseServer.ImageBuild.
type LegacyBuildFn func(opts api.ImageBuildOptions, ctxReader io.Reader) (io.ReadCloser, error)

// LegacyBuildAvailableFn reports whether the legacy build path is
// reachable. nil → assume available; legacy ImageBuild surfaces
// NotImplementedError from the impl when no build service is
// configured.
type LegacyBuildAvailableFn func() bool

// WrapLegacyBuild returns a BuildDriver that delegates to the supplied
// legacy function.
func WrapLegacyBuild(fn LegacyBuildFn, available LegacyBuildAvailableFn, backend, impl string) BuildDriver {
	return &legacyBuildAdapter{fn: fn, available: available, backend: backend, impl: impl}
}

type legacyBuildAdapter struct {
	fn        LegacyBuildFn
	available LegacyBuildAvailableFn
	backend   string
	impl      string
}

func (a *legacyBuildAdapter) Describe() string {
	if a.backend == "" && a.impl == "" {
		return "build via legacy ImageBuild function"
	}
	return a.backend + " " + a.impl + " (legacy ImageBuild adapter)"
}

func (a *legacyBuildAdapter) Available() bool {
	if a.available != nil {
		return a.available()
	}
	return a.fn != nil
}

func (a *legacyBuildAdapter) Build(dctx DriverContext, opts api.ImageBuildOptions, ctxReader io.Reader) (io.ReadCloser, error) {
	if a.fn == nil {
		return nil, errors.New("legacy build adapter: function is nil")
	}
	return a.fn(opts, ctxReader)
}
