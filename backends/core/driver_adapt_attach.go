package core

import (
	"errors"
	"io"
	"net"

	"github.com/sockerless/api"
)

// Phase 104 — AttachDriver narrow→typed adapter, plus a typed
// `cloudLogsAttach` that lifts `core.AttachViaCloudLogs` into the
// new framework.
//
// `WrapLegacyAttach` adapts the existing narrow `core.StreamDriver`
// (raw `Attach(ctx, containerID, tty, conn) error`) into the typed
// `AttachDriver104` shape (`Attach(dctx, tty, conn io.ReadWriter) error`).
//
// `NewCloudLogsAttachDriver` is the typed wrapper for the FaaS
// "log-streamed read-only attach" path that today lives at
// `core.AttachViaCloudLogs`. Backends without a real bidirectional
// attach (Lambda, Cloud Run Functions, ACA Jobs without console
// exec) plug this in as their `DriverSet104.Attach` so `docker
// attach -i <fn-cid>` produces a clear read-only stream from
// CloudWatch / Cloud Logging / Log Analytics rather than a generic
// NotImpl.

// WrapLegacyAttach returns an AttachDriver104 that delegates to the
// supplied narrow StreamDriver. `backend` and `impl` populate the
// `Describe()` string used by `NotImplDriverError`.
func WrapLegacyAttach(narrow StreamDriver, backend, impl string) AttachDriver104 {
	return &legacyAttachAdapter{narrow: narrow, backend: backend, impl: impl}
}

type legacyAttachAdapter struct {
	narrow  StreamDriver
	backend string
	impl    string
}

func (a *legacyAttachAdapter) Describe() string {
	if a.backend == "" && a.impl == "" {
		return "attach via legacy narrow driver"
	}
	return a.backend + " " + a.impl + " (legacy-narrow attach adapter)"
}

func (a *legacyAttachAdapter) Attach(dctx DriverContext, tty bool, conn io.ReadWriter) error {
	if a.narrow == nil {
		return errors.New("legacy attach adapter: narrow driver is nil — register one via DriverSet.Stream or replace this adapter with a typed AttachDriver104")
	}
	netConn, ok := conn.(net.Conn)
	if !ok {
		return errors.New("legacy attach adapter: requires net.Conn for hijacked-stream attach; caller passed a non-Conn ReadWriter")
	}
	return a.narrow.Attach(dctx.Ctx, dctx.Container.ID, tty, netConn)
}

// cloudLogsAttachDriver is the typed `AttachDriver104` wrapper for
// `core.AttachViaCloudLogs`. Lifts the FaaS read-only attach into
// the Phase 104 framework so backends like Lambda / cloudrun-
// functions / azure-functions / ACA-Jobs (which have no real
// bidirectional attach) can plug it directly into
// `DriverSet104.Attach`.
type cloudLogsAttachDriver struct {
	server  *BaseServer
	fetch   CloudLogFetchFunc
	backend string
	impl    string
}

// NewCloudLogsAttachDriver constructs the typed read-only attach
// driver backed by `AttachViaCloudLogs`. `fetch` is the per-backend
// log-fetcher (CloudWatch for Lambda; Cloud Logging for Cloud Run
// Functions; Log Analytics for ACA Jobs / Azure Functions).
func NewCloudLogsAttachDriver(s *BaseServer, fetch CloudLogFetchFunc, backend, impl string) AttachDriver104 {
	return &cloudLogsAttachDriver{server: s, fetch: fetch, backend: backend, impl: impl}
}

func (d *cloudLogsAttachDriver) Describe() string {
	return d.backend + " " + d.impl + " (read-only cloud-logs attach via AttachViaCloudLogs)"
}

func (d *cloudLogsAttachDriver) Attach(dctx DriverContext, tty bool, conn io.ReadWriter) error {
	if d.server == nil || d.fetch == nil {
		return errors.New("cloud-logs attach: server / fetch is nil — backend init must wire both")
	}
	rwc, err := AttachViaCloudLogs(d.server, dctx.Container.ID, api.ContainerAttachOptions{
		Stdout: true,
		Stderr: true,
		Stream: true,
		Logs:   true,
	}, d.fetch)
	if err != nil {
		return err
	}
	defer rwc.Close()
	// Pump cloud-logs → caller. Read-only: we ignore conn→rwc.
	if _, err := io.Copy(conn, rwc); err != nil {
		// EOF / closed-connection is the documented terminal state
		// of `docker attach`; treat it as nil so the handler doesn't
		// surface a confusing error after a clean close.
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}
