package core

import (
	"errors"
	"io"
)

// RegistryDriver legacy adapter. Wraps the BaseServer.ImagePull /
// ImagePush methods (which take name+tag+auth-style strings) into the
// typed RegistryDriver shape (which takes a parsed ImageRef + auth).

// LegacyPullFn matches BaseServer.ImagePull.
type LegacyPullFn func(ref, auth string) (io.ReadCloser, error)

// LegacyPushFn matches BaseServer.ImagePush, which splits the ref into
// name + tag. The adapter pulls those out of the typed ImageRef before
// delegating.
type LegacyPushFn func(name, tag, auth string) (io.ReadCloser, error)

// WrapLegacyRegistry returns a RegistryDriver that delegates Push +
// Pull to the supplied legacy functions. Either may be nil — the
// adapter surfaces a clear "function is nil" error rather than
// crashing.
func WrapLegacyRegistry(pull LegacyPullFn, push LegacyPushFn, backend, impl string) RegistryDriver {
	return &legacyRegistryAdapter{pull: pull, push: push, backend: backend, impl: impl}
}

type legacyRegistryAdapter struct {
	pull    LegacyPullFn
	push    LegacyPushFn
	backend string
	impl    string
}

func (a *legacyRegistryAdapter) Describe() string {
	if a.backend == "" && a.impl == "" {
		return "registry via legacy ImagePull / ImagePush functions"
	}
	return a.backend + " " + a.impl + " (legacy ImagePull/ImagePush adapter)"
}

func (a *legacyRegistryAdapter) Pull(dctx DriverContext, ref ImageRef, auth string) (io.ReadCloser, error) {
	if a.pull == nil {
		return nil, errors.New("legacy registry adapter: pull function is nil")
	}
	return a.pull(ref.String(), auth)
}

func (a *legacyRegistryAdapter) Push(dctx DriverContext, ref ImageRef, auth string) (io.ReadCloser, error) {
	if a.push == nil {
		return nil, errors.New("legacy registry adapter: push function is nil")
	}
	name, tag := ref.NameTag()
	if ref.Domain != "" {
		name = ref.FullName()
	}
	return a.push(name, tag, auth)
}
