package core

import (
	"errors"
	"io"
	"strings"
)

// RegistryDriver narrow→typed adapter. Wraps the legacy ImagePull /
// ImagePush methods (which take ref+auth and ref+tag+auth respectively
// as separate args) into the typed RegistryDriver shape (which takes
// the joined ref + auth).

// LegacyPullFn matches BaseServer.ImagePull.
type LegacyPullFn func(ref, auth string) (io.ReadCloser, error)

// LegacyPushFn matches BaseServer.ImagePush, which splits the ref
// into name + tag. The adapter joins the typed driver's `ref` arg
// before delegating.
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

func (a *legacyRegistryAdapter) Pull(dctx DriverContext, ref, auth string) (io.ReadCloser, error) {
	if a.pull == nil {
		return nil, errors.New("legacy registry adapter: pull function is nil")
	}
	return a.pull(ref, auth)
}

func (a *legacyRegistryAdapter) Push(dctx DriverContext, ref, auth string) (io.ReadCloser, error) {
	if a.push == nil {
		return nil, errors.New("legacy registry adapter: push function is nil")
	}
	name, tag := splitImageRef(ref)
	return a.push(name, tag, auth)
}

// splitImageRef splits a `<name>[:<tag>]` reference into separate
// name and tag fields. A registry-prefixed ref like
// `host:5000/repo/image:tag` is split on the LAST colon so the port
// in the registry host isn't mistaken for a tag separator.
func splitImageRef(ref string) (name, tag string) {
	if i := strings.LastIndex(ref, ":"); i > 0 {
		// Reject if the colon is inside the registry-host part (no
		// `/` between the colon and end of string means the colon is
		// part of the host:port, not a tag separator).
		if !strings.Contains(ref[i:], "/") {
			return ref[:i], ref[i+1:]
		}
	}
	return ref, ""
}
