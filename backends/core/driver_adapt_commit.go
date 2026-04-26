package core

import (
	"errors"

	"github.com/sockerless/api"
)

// CommitDriver narrow→typed adapter. Maps the legacy
// ContainerCommit(*api.ContainerCommitRequest) signature into the
// typed CommitDriver shape (which takes a richer DriverContext +
// CommitOptions).

// LegacyCommitFn matches BaseServer.ContainerCommit.
type LegacyCommitFn func(req *api.ContainerCommitRequest) (*api.ContainerCommitResponse, error)

// WrapLegacyCommit returns a CommitDriver that delegates to the
// supplied legacy function. The DriverContext's container ID is
// projected into the legacy req's Container field; CommitOptions
// fields map 1:1 onto the rest of the request.
func WrapLegacyCommit(fn LegacyCommitFn, backend, impl string) CommitDriver {
	return &legacyCommitAdapter{fn: fn, backend: backend, impl: impl}
}

type legacyCommitAdapter struct {
	fn      LegacyCommitFn
	backend string
	impl    string
}

func (a *legacyCommitAdapter) Describe() string {
	if a.backend == "" && a.impl == "" {
		return "commit via legacy ContainerCommit function"
	}
	return a.backend + " " + a.impl + " (legacy ContainerCommit adapter)"
}

func (a *legacyCommitAdapter) Commit(dctx DriverContext, opts CommitOptions) (string, error) {
	if a.fn == nil {
		return "", errors.New("legacy commit adapter: function is nil")
	}
	req := &api.ContainerCommitRequest{
		Container: dctx.Container.ID,
		Author:    opts.Author,
		Comment:   opts.Comment,
		Repo:      opts.Repo,
		Tag:       opts.Tag,
		Pause:     opts.Pause,
		Changes:   opts.Changes,
		Config:    opts.Config,
	}
	resp, err := a.fn(req)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", nil
	}
	return resp.ID, nil
}
