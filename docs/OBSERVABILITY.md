# Observability — Stack A

Sockerless ships with an opt-in observability stack: an OpenTelemetry
Collector fanning out to VictoriaLogs (logs) and Jaeger (traces). All
three binaries are Apache 2.0.

## Two modes

The stack is independent from the cell stack. Either runs without the
other.

**No-OTel mode (default).** Components write to `.stack-pids/<n>.log`
via stdout/stderr redirection. Phase 81 SSE log tail and Phase 86
diagnostic panel pull from those files directly. Operators don't need
the observability stack for day-to-day debugging.

**OTel mode (opt-in).** Operator runs `make stack-observability-up`
which brings up:

| Service | Port | Purpose |
|---|---|---|
| OTel Collector OTLP gRPC | 4317 | Components emit traces here. |
| OTel Collector OTLP HTTP | 4318 | Same, HTTP transport. |
| VictoriaLogs UI | 9428 | Log search + dashboards. |
| Jaeger UI | 16686 | Trace search + flame graphs. |

The collector's `filelog` receiver scrapes `.stack-pids/*.log` and
ships every line to VictoriaLogs tagged with `service.name = <pidfile-
name without .log>`. So even components that don't emit OTLP directly
have their logs searchable in VictoriaLogs, with no binary changes.

Components that emit OTLP traces (Phase 87b will wire admin / sims /
backends / bleephub) flow into Jaeger. They emit only when
`OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317` is set in their
environment — unset = today's stdout-only behaviour.

## Operator workflow

```bash
# 1. Install the binaries (once).
brew install opentelemetry-collector-contrib  # macOS
# OR
go install go.opentelemetry.io/collector/cmd/otelcol-contrib@latest

# Jaeger:
docker pull jaegertracing/all-in-one:latest  # if you prefer Docker
# OR download from https://www.jaegertracing.io/download/

# VictoriaLogs:
brew install victoria-logs  # macOS
# OR
docker pull victoriametrics/victoria-logs:latest

# 2. Bring up the observability stack.
make stack-observability-up

# 3. Tell admin where the dashboards live (so the UI renders deep links).
export OTEL_LOGS_DASHBOARD=http://localhost:9428/select/vmui
export OTEL_TRACES_DASHBOARD=http://localhost:16686/search

# 4. Bring up cells as usual.
make stack-aws-ecs

# 5. (Optional) Tell components to emit OTLP traces.
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317
```

## Validation — is the pipeline actually working?

Use `make stack-observability-validate` to confirm telemetry actually
reaches both backends:

```bash
make stack-observability-up
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 \
  ./backends/docker/sockerless-backend-docker -addr :3375 &
curl http://localhost:3375/v1/version          # generate one request
make stack-observability-validate
```

The target polls VictoriaLogs (`/select/logsql/query?query=service.name:"<svc>"`)
and Jaeger (`/api/traces?service=<svc>`) until both return at least one
record, or the timeout expires (default 30 s). Override the service or
timeout with:

```bash
OBS_VALIDATE_SERVICE=sockerless-backend-cloudrun \
OBS_VALIDATE_TIMEOUT_S=60 \
  make stack-observability-validate
```

This is a manual operator-grade check, not run in CI (CI doesn't
bring up the observability stack). Run it after enabling OTLP on a
new component, after upgrading the OTel collector, or whenever a
diagnostic panel chip looks suspicious.

## Admin UI integration

When `OTEL_LOGS_DASHBOARD` / `OTEL_TRACES_DASHBOARD` are set, the
admin UI's diagnostic panel renders `VictoriaLogs ↗` and `Jaeger ↗`
chips that open the corresponding dashboard pre-filtered by
`service.name = <instance>`.

The chips appear under the "full logs" / "console" links on every
unhealthy / crashed instance row. Healthy rows don't render the panel
at all (Phase 86 mount gate), so the cost of the observability fetch
is bounded.

## Configuration

`make/observability-config/otel-collector.yaml` is the default
collector config. Two top-level sections operators care about:

- `receivers.filelog.include` — defaults to `${SOCKERLESS_STATE_PIDS_DIR}/*.log`,
  the make target sets the env var.
- `exporters` — pointed at VictoriaLogs and Jaeger on their localhost
  ports.

To use a custom config:

```bash
make stack-observability-up STACK_OBS_CONFIG_DIR=/path/to/your/config
```

## Sensible retention

- VictoriaLogs: 7-day retention configured via `-retentionPeriod=7d`.
- Jaeger Badger: 72-hour TTL via `BADGER_SPAN_STORE_TTL=72h`.

State lives under `.sockerless-state/observability/{logs,traces}/`,
so `make purge-state-all` (Phase 84) wipes it alongside other instance
state.

## Components stay decoupled

Same invariant as everywhere else in sockerless: components emit OTLP
**only** when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env.
Unset = today's stdout-only behaviour. Admin doesn't inject the env
var — operators set it themselves on the instance Config (Phase 79
topology) when they want OTel emission for that instance.

## Stack swap

Stack A is the default (Apache 2.0 throughout). The same component
code (OTel SDK on the emit side, `service.name` resource attr on the
collector side) works against:

- **OpenObserve** (AGPL) — replace VictoriaLogs + Jaeger with a single
  OpenObserve binary. Only `make/stack.mk` changes.
- **SigNoz** (MIT) — replace the same. Only `make/stack.mk` changes.

The component-side wiring is OTel SDK, not collector-specific — swap
is `make` work, never code work.

## Next: Phase 87b

Phase 87 ships the stack + UI integration; the next branch wires OTel
SDK into each component's `main.go`:

- `backends/core/otel.go` (already partially there for traces) extended
  to logs export + the zerolog → OTel logs bridge.
- Each backend / sim / admin / bleephub `main.go` gains 3 lines: read
  `OTEL_EXPORTER_OTLP_ENDPOINT`, init the SDK, defer shutdown.
- `otelhttp.NewHandler` wraps each component's mux for per-request
  spans.
