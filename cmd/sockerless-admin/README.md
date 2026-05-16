# sockerless-admin

Local orchestration server for Sockerless topologies — backends + simulators + bleephub + projects. Exposes a REST API + embedded React UI on `:9090` by default. Reads `/v1/health` + `/v1/info` + env vars from each registered component; **never** requires admin-side env vars on the components themselves.

## Reference adaptors

| Adaptor | What it proves |
|---|---|
| **Browser / embedded UI** (`ui/packages/admin`) | The HTTP API at `/api/*` is consumed by the embedded React SPA. The UI is the canonical reference adaptor for the admin REST surface. |
| **`curl` / `httpie` / `gh api`-style HTTP clients** | Every action the UI takes is also driveable as plain REST. The API is the contract; the UI is one consumer. |
| **`sockerless-admin` itself reaching out to backends** | The admin polls each registered backend / simulator / bleephub via `/v1/health` + `/v1/info` per the [components-decoupled-from-admin invariant](../../memory/feedback_components_decoupled_from_admin.md). |
| **`*_test.go` files** | Every API handler has unit tests in the same package — see the file pairing (`api_topology.go` ↔ `api_topology_test.go`, etc.). |

## Validation

| Test path | What runs | Last green |
|---|---|---|
| `cmd/sockerless-admin/*_test.go` (40+ files) | Unit tests for topology CRUD, project lifecycle, process manager, instance lifecycle, OTel wiring, config migration. | 2026-05-16 |
| `ui/packages/admin/` (Vitest) | UI component + route tests via [Vitest](https://vitest.dev). | 2026-05-16 |
| `make cmd/sockerless-admin/test` | Leaf-Makefile suite per [`docs/MAKEFILE_STANDARD.md`](../../docs/MAKEFILE_STANDARD.md). | 2026-05-16 |
| Manual round-trip | `sockerless-admin --addr :9090` → open `http://localhost:9090/` in a browser, register a backend, start it, drive a container through. | continuous |

## Wiring the adaptor

```sh
# Build (UI first so Go embeds it via go:embed)
cd ui/packages/admin && bun install && bun run build       # → ui/packages/admin/dist/
cd ../../../cmd/sockerless-admin && make build             # → ./sockerless-admin (embeds dist/)

# Run
./sockerless-admin --addr :9090 \
  --backend ecs-dev=http://localhost:3375 \
  --backend lambda-dev=http://localhost:3376 \
  --simulator sim-aws=http://localhost:5111 \
  --bleephub http://localhost:8443

# Or load a config file
./sockerless-admin --addr :9090 --config admin.json
```

### CLI flags

| Flag | Default | Description |
|---|---|---|
| `--addr` | `:9090` | Listen address (host:port). |
| `--config` | unset | Path to `admin.json` topology file. |
| `--backend name=addr` | repeatable | Register a backend by name + URL. |
| `--simulator name=addr` | repeatable | Register a simulator by name + URL. |
| `--bleephub addr` | unset | Register the bleephub coordinator URL. |
| `--version` | | Print version and exit. |

The admin loads components in this priority order: explicit flags → `--config` file → auto-discover from `~/.sockerless/contexts/` → persisted projects from `~/.sockerless/projects/`.

## API surface

All routes live under `/api/`. Selected paths (full set in `api_*.go`):

| Path | Purpose |
|---|---|
| `GET /api/overview` | Cluster-wide snapshot (component count, container count, project count, recent events). |
| `GET /api/components` | List registered backends + simulators + bleephub with their current health. |
| `GET /api/containers` | Aggregate `docker ps` across all backends. |
| `GET /api/contexts` | Available CLI contexts. |
| `GET /api/processes`, `POST /api/processes/{name}/start` | Process manager — start/stop backend / simulator binaries. |
| `GET /api/projects`, `POST /api/projects` | Projects (named groups of resources). |
| `GET /api/resources`, `POST /api/cleanup` | Orphan cloud-resource sweep. |
| `GET /api/topology`, `POST /api/topology` | Topology graph (projects × instances × bindings). |
| `GET /api/topology/diagnostics` | Live drift detection across components. |
| `GET /api/topology/logs/{instance}` | Tail logs from a specific instance. |
| `POST /api/topology/proxy/{instance}/{path}` | Reverse-proxy raw HTTP into an instance for ad-hoc debugging. |
| `GET /api/observability/traces` | OTel trace listing. |

The full handler set is enumerated in the source: see the `api_*.go` files in this directory.

## UI

`http://localhost:9090/` serves the embedded SPA. Routes:

- **Overview** — cluster snapshot.
- **Components** — registered backends / simulators / bleephub + per-component health, env vars, version, last poll.
- **Containers** — aggregate `docker ps`.
- **Projects** — project lifecycle.
- **Topology** — visual graph + drift diagnostics.
- **Observability** — OTel traces + log tail.

UI implementation lives in `ui/packages/admin/`. The Go binary embeds the built bundle via `go:embed` (build tag `!noui`, on by default).

## Sample

```bash
$ ./sockerless-admin --addr :9090 \
    --backend ecs-dev=http://localhost:3375 \
    --simulator sim-aws=http://localhost:5111 &

$ curl -s http://localhost:9090/api/overview | jq .
{
  "components": {"backends": 1, "simulators": 1, "bleephub": 0},
  "containers": 0,
  "projects": 0,
  "lastEvent": "2026-05-16T...Z component registered: ecs-dev"
}

$ curl -s http://localhost:9090/api/components | jq '.[]'
{
  "name": "ecs-dev",
  "kind": "backend",
  "type": "ecs",
  "addr": "http://localhost:3375",
  "health": "healthy",
  "version": "dev",
  "lastPoll": "2026-05-16T..."
}
```

## Known issues

None open. The admin's [components-decoupled invariant](../../memory/feedback_components_decoupled_from_admin.md) is load-bearing: components must remain runnable standalone via `make backends/<x>/run`, with the admin reading only `/v1/health` + `/v1/info` + env vars. No admin-side env vars on the components.

## What's out of scope

- **Multi-machine orchestration.** This is a single-machine local-dev orchestrator. For multi-host serverless capacity, the cloud's own orchestrator (ECS Fargate, Cloud Run, ACA, etc.) is the answer; the admin is for routing.
- **Persistent state across restarts.** Project + topology files persist to `~/.sockerless/projects/` and the topology file. Live component health resets on admin restart.
- **Auth.** The current implementation has none. Run behind a reverse proxy if exposing externally.
- **Cloud-side resource creation.** The admin does not provision AWS / GCP / Azure infra; it observes resources that backends create. Use Terraform for provisioning (see per-backend READMEs).
- **Replacing `sockerless` (CLI).** The CLI is for context + lifecycle on a single backend. The admin is for orchestrating many. They overlap but neither subsumes the other.

## Project structure

```
cmd/sockerless-admin/
├── main.go                       Entry point: flags, registry wiring, HTTP listen
├── bootstrap.go                  Component discovery (config file + ~/.sockerless contexts)
├── instance.go, instance_*.go    Per-instance lifecycle + status
├── config.go, config_metadata.go Admin config schema
├── cleanup.go                    Orphan cloud-resource reaper
├── otel.go                       OTel exporter wiring for the admin itself
├── api_overview.go               GET /api/overview
├── api_components.go             /api/components — registered components + health
├── api_containers.go             /api/containers — aggregate docker ps
├── api_contexts.go               /api/contexts — CLI contexts visible from here
├── api_processes.go              Process manager (start/stop backend binaries)
├── api_projects.go               Projects (named resource groups)
├── api_resources.go              Cloud-resource listing + cleanup
├── api_topology.go               /api/topology — graph CRUD
├── api_topology_diagnostics.go   Drift detection
├── api_topology_logs.go          Per-instance log tail
├── api_topology_proxy.go         Reverse-proxy raw HTTP into instances
├── api_observability.go          OTel traces + log listing
└── *_test.go                     Per-handler unit tests
```

See also: [`docs/ADMIN_ORCHESTRATION.md`](../../docs/ADMIN_ORCHESTRATION.md) for design background, [`cmd/sockerless/README.md`](../sockerless/README.md) for the single-backend CLI counterpart, [`bleephub/README.md`](../../bleephub/README.md) for the GitHub-compat coordinator the admin can wire in.
