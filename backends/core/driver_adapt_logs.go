package core

import (
	"errors"
	"io"

	"github.com/sockerless/api"
)

// LogsDriver narrow→typed adapter, plus a typed `cloudLogsLogs` that
// lifts `core.StreamCloudLogs` into the new framework.
//
// `WrapLegacyLogs` adapts the existing per-backend `ContainerLogs`
// signature (`func(ref, opts) (io.ReadCloser, error)`) into the typed
// `LogsDriver` shape. Backends that override `BaseServer.ContainerLogs`
// today (every cloud backend does, via `self`-dispatch) can plug their
// existing function into `DriverSet104.Logs` without behaviour change.
//
// `NewCloudLogsLogsDriver` is the typed wrapper around `StreamCloudLogs`
// — the helper every cloud backend already uses to fetch logs from
// CloudWatch / Cloud Logging / Log Analytics. Cloud backends that
// haven't yet pre-built their `ContainerLogs` shim can plug this in
// directly.

// LegacyLogsFn matches the existing `BaseServer.ContainerLogs`
// signature so per-backend overrides plug into `WrapLegacyLogs`
// without any further glue.
type LegacyLogsFn func(ref string, opts api.ContainerLogsOptions) (io.ReadCloser, error)

// WrapLegacyLogs returns a LogsDriver that delegates to the supplied
// legacy function. `backend` and `impl` populate `Describe()` for the
// `NotImplDriverError` formatter.
func WrapLegacyLogs(fn LegacyLogsFn, backend, impl string) LogsDriver {
	return &legacyLogsAdapter{fn: fn, backend: backend, impl: impl}
}

type legacyLogsAdapter struct {
	fn      LegacyLogsFn
	backend string
	impl    string
}

func (a *legacyLogsAdapter) Describe() string {
	if a.backend == "" && a.impl == "" {
		return "logs via legacy ContainerLogs function"
	}
	return a.backend + " " + a.impl + " (legacy ContainerLogs adapter)"
}

func (a *legacyLogsAdapter) Logs(dctx DriverContext, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	if a.fn == nil {
		return nil, errors.New("legacy logs adapter: function is nil — register one via WrapLegacyLogs(s.ContainerLogs, ...) or replace this adapter with a typed LogsDriver")
	}
	return a.fn(dctx.Container.ID, opts)
}

// cloudLogsLogsDriver is the typed `LogsDriver` wrapper for
// `core.StreamCloudLogs`. Lifts the cloud-logs read path used by
// every cloud backend (ECS / Lambda / CR / GCF / ACA / AZF) into
// the Phase 104 framework. Backends construct one with their
// per-backend `CloudLogFetchFunc` (the same one Attach uses) plus
// the `StreamCloudLogsOptions` that controls FaaS LogBuffers
// fallback and pre-start tolerance.
type cloudLogsLogsDriver struct {
	server  *BaseServer
	fetch   CloudLogFetchFunc
	sopts   StreamCloudLogsOptions
	backend string
	impl    string
}

// NewCloudLogsLogsDriver constructs the typed read path backed by
// `StreamCloudLogs`. `fetch` is the per-backend log-fetcher (same one
// passed to `NewCloudLogsAttachDriver`). `sopts` mirrors the existing
// `StreamCloudLogs` knobs (CheckLogBuffers for FaaS; AllowCreated for
// the create→attach→start docker flow).
func NewCloudLogsLogsDriver(s *BaseServer, fetch CloudLogFetchFunc, sopts StreamCloudLogsOptions, backend, impl string) LogsDriver {
	return &cloudLogsLogsDriver{server: s, fetch: fetch, sopts: sopts, backend: backend, impl: impl}
}

func (d *cloudLogsLogsDriver) Describe() string {
	return d.backend + " " + d.impl + " (cloud-logs read via StreamCloudLogs)"
}

func (d *cloudLogsLogsDriver) Logs(dctx DriverContext, opts api.ContainerLogsOptions) (io.ReadCloser, error) {
	if d.server == nil || d.fetch == nil {
		return nil, errors.New("cloud-logs driver: server / fetch is nil — backend init must wire both")
	}
	return StreamCloudLogs(d.server, dctx.Container.ID, opts, d.fetch, d.sopts)
}
