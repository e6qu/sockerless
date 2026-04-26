package core

import (
	"errors"
	"io"

	"github.com/sockerless/api"
)

// Typed cloud-native drivers backed by the reverse-agent helpers
// (`RunContainerXxxViaAgent`). Shared across every backend that ships
// a sockerless bootstrap (Lambda, Cloud Run, Cloud Run Functions,
// ACA, Azure Functions). Backends construct one with their existing
// `*ReverseAgentRegistry` and plug it into `TypedDriverSet`.

// NewReverseAgentProcListDriver wraps RunContainerTopViaAgent.
func NewReverseAgentProcListDriver(reg *ReverseAgentRegistry, backend string) ProcListDriver {
	return &reverseAgentProcList{reg: reg, backend: backend}
}

type reverseAgentProcList struct {
	reg     *ReverseAgentRegistry
	backend string
}

func (d *reverseAgentProcList) Describe() string { return d.backend + " ReverseAgentPs" }
func (d *reverseAgentProcList) Top(dctx DriverContext, psArgs string) (*api.ContainerTopResponse, error) {
	return RunContainerTopViaAgent(d.reg, dctx.Container.ID, psArgs)
}

// NewReverseAgentFSDiffDriver wraps RunContainerChangesViaAgent.
func NewReverseAgentFSDiffDriver(reg *ReverseAgentRegistry, backend string) FSDiffDriver {
	return &reverseAgentFSDiff{reg: reg, backend: backend}
}

type reverseAgentFSDiff struct {
	reg     *ReverseAgentRegistry
	backend string
}

func (d *reverseAgentFSDiff) Describe() string { return d.backend + " ReverseAgentFindNewer" }
func (d *reverseAgentFSDiff) Changes(dctx DriverContext) ([]api.ContainerChangeItem, error) {
	return RunContainerChangesViaAgent(d.reg, dctx.Container.ID)
}

// NewReverseAgentFSReadDriver wraps RunContainerStatPathViaAgent +
// RunContainerGetArchiveViaAgent. GetArchive bridges
// *ContainerArchiveResponse → io.Writer via io.Copy.
func NewReverseAgentFSReadDriver(reg *ReverseAgentRegistry, backend string) FSReadDriver {
	return &reverseAgentFSRead{reg: reg, backend: backend}
}

type reverseAgentFSRead struct {
	reg     *ReverseAgentRegistry
	backend string
}

func (d *reverseAgentFSRead) Describe() string { return d.backend + " ReverseAgentTar" }
func (d *reverseAgentFSRead) StatPath(dctx DriverContext, path string) (*api.ContainerPathStat, error) {
	return RunContainerStatPathViaAgent(d.reg, dctx.Container.ID, path)
}
func (d *reverseAgentFSRead) GetArchive(dctx DriverContext, path string, w io.Writer) error {
	resp, err := RunContainerGetArchiveViaAgent(d.reg, dctx.Container.ID, path)
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

// NewReverseAgentFSWriteDriver wraps RunContainerPutArchiveViaAgent.
func NewReverseAgentFSWriteDriver(reg *ReverseAgentRegistry, backend string) FSWriteDriver {
	return &reverseAgentFSWrite{reg: reg, backend: backend}
}

type reverseAgentFSWrite struct {
	reg     *ReverseAgentRegistry
	backend string
}

func (d *reverseAgentFSWrite) Describe() string { return d.backend + " ReverseAgentTarExtract" }
func (d *reverseAgentFSWrite) PutArchive(dctx DriverContext, path string, body io.Reader, _ bool) error {
	return RunContainerPutArchiveViaAgent(d.reg, dctx.Container.ID, path, body)
}

// NewReverseAgentFSExportDriver wraps RunContainerExportViaAgent.
func NewReverseAgentFSExportDriver(reg *ReverseAgentRegistry, backend string) FSExportDriver {
	return &reverseAgentFSExport{reg: reg, backend: backend}
}

type reverseAgentFSExport struct {
	reg     *ReverseAgentRegistry
	backend string
}

func (d *reverseAgentFSExport) Describe() string { return d.backend + " ReverseAgentTarRoot" }
func (d *reverseAgentFSExport) Export(dctx DriverContext, w io.Writer) error {
	rc, err := RunContainerExportViaAgent(d.reg, dctx.Container.ID)
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
