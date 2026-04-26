package core

import (
	"errors"
	"io"

	"github.com/sockerless/api"
)

// FSReadDriver / FSWriteDriver / FSExportDriver narrow→typed adapters.
// Each adapts an existing per-backend method into the typed driver
// shape. FSRead bridges *ContainerArchiveResponse → io.Writer via copy;
// FSWrite is a direct method-arg projection; FSExport bridges
// io.ReadCloser → io.Writer via copy.

// LegacyStatPathFn matches BaseServer.ContainerStatPath.
type LegacyStatPathFn func(ref, path string) (*api.ContainerPathStat, error)

// LegacyGetArchiveFn matches BaseServer.ContainerGetArchive.
type LegacyGetArchiveFn func(ref, path string) (*api.ContainerArchiveResponse, error)

// LegacyPutArchiveFn matches BaseServer.ContainerPutArchive.
type LegacyPutArchiveFn func(ref, path string, noOverwriteDirNonDir bool, body io.Reader) error

// LegacyExportFn matches BaseServer.ContainerExport.
type LegacyExportFn func(ref string) (io.ReadCloser, error)

// WrapLegacyFSRead returns an FSReadDriver that delegates StatPath +
// GetArchive to the supplied legacy functions.
func WrapLegacyFSRead(stat LegacyStatPathFn, get LegacyGetArchiveFn, backend, impl string) FSReadDriver {
	return &legacyFSReadAdapter{stat: stat, get: get, backend: backend, impl: impl}
}

type legacyFSReadAdapter struct {
	stat    LegacyStatPathFn
	get     LegacyGetArchiveFn
	backend string
	impl    string
}

func (a *legacyFSReadAdapter) Describe() string {
	return a.backend + " " + a.impl + " (legacy FS-read adapter)"
}

func (a *legacyFSReadAdapter) StatPath(dctx DriverContext, path string) (*api.ContainerPathStat, error) {
	if a.stat == nil {
		return nil, errors.New("legacy fs-read adapter: stat function is nil")
	}
	return a.stat(dctx.Container.ID, path)
}

func (a *legacyFSReadAdapter) GetArchive(dctx DriverContext, path string, w io.Writer) error {
	if a.get == nil {
		return errors.New("legacy fs-read adapter: get function is nil")
	}
	resp, err := a.get(dctx.Container.ID, path)
	if err != nil {
		return err
	}
	if resp == nil || resp.Reader == nil {
		return nil
	}
	defer resp.Reader.Close()
	_, err = io.Copy(w, resp.Reader)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// WrapLegacyFSWrite returns an FSWriteDriver that delegates PutArchive
// to the supplied legacy function.
func WrapLegacyFSWrite(fn LegacyPutArchiveFn, backend, impl string) FSWriteDriver {
	return &legacyFSWriteAdapter{fn: fn, backend: backend, impl: impl}
}

type legacyFSWriteAdapter struct {
	fn      LegacyPutArchiveFn
	backend string
	impl    string
}

func (a *legacyFSWriteAdapter) Describe() string {
	return a.backend + " " + a.impl + " (legacy FS-write adapter)"
}

func (a *legacyFSWriteAdapter) PutArchive(dctx DriverContext, path string, body io.Reader, noOverwriteDirNonDir bool) error {
	if a.fn == nil {
		return errors.New("legacy fs-write adapter: function is nil")
	}
	return a.fn(dctx.Container.ID, path, noOverwriteDirNonDir, body)
}

// WrapLegacyFSExport returns an FSExportDriver that delegates to the
// supplied legacy function and pipes its reader output into the typed
// driver's Writer.
func WrapLegacyFSExport(fn LegacyExportFn, backend, impl string) FSExportDriver {
	return &legacyFSExportAdapter{fn: fn, backend: backend, impl: impl}
}

type legacyFSExportAdapter struct {
	fn      LegacyExportFn
	backend string
	impl    string
}

func (a *legacyFSExportAdapter) Describe() string {
	return a.backend + " " + a.impl + " (legacy ContainerExport adapter)"
}

func (a *legacyFSExportAdapter) Export(dctx DriverContext, w io.Writer) error {
	if a.fn == nil {
		return errors.New("legacy fs-export adapter: function is nil")
	}
	rc, err := a.fn(dctx.Container.ID)
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
