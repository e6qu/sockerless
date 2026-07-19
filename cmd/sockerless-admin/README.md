# sockerless-admin

Local orchestration server for Sockerless topologies — backends, simulators, and projects. Exposes a REST API + embedded React UI on `:9090` by default. Reads `/v1/health` + `/v1/info` + env vars from each registered component; **never** requires admin-side env vars on the components themselves.

## Reference adaptors

| Adaptor | What it proves |
|---|---|
| **Browser / embedded UI** (`ui/packages/admin`) | The HTTP API at `/api/v1/*` is consumed by the embedded React SPA. The UI is the canonical reference adaptor for the admin REST surface. |
| **`curl` / `httpie` / `gh api`-style HTTP clients** | Every action the UI takes is also driveable as plain REST. The API is the contract; the UI is one consumer. |
| **`sockerless-admin` itself reaching out to components** | The admin polls each registered backend and simulator via `/v1/health` + `/v1/info` per the components-decoupled-from-admin invariant. |
| **`*_test.go` files** | Every API handler has unit tests in the same package — see the file pairing (`api_topology.go` ↔ `api_topology_test.go`, etc.). |

## Validation

| Test path | What runs | Last green |
|---|---|---|
| `cmd/sockerless-admin/*_test.go` (40+ files) | Unit tests for topology CRUD, project lifecycle, process manager, instance lifecycle, OTel wiring, config migration. | 2026-05-26 |
| `ui/packages/admin/` (Vitest) | UI component + route tests via [Vitest](https://vitest.dev). | 2026-05-26 |
| `make cmd/sockerless-admin/test` | Leaf-Makefile suite per [`docs/MAKEFILE_STANDARD.md`](../../docs/MAKEFILE_STANDARD.md). | 2026-05-26 |
| Manual round-trip | `sockerless-admin --addr :9090` -> open `http://localhost:9090/ui/` in a browser, register a backend, start it, drive a container through. | continuous |

## Wiring the adaptor

```sh
# Build (UI first so Go embeds it via go:embed)
cd ui/packages/admin && bun install && bun run build       # → ui/packages/admin/dist/
cd ../../../cmd/sockerless-admin && make build             # → ./sockerless-admin (embeds dist/)

# Run
./sockerless-admin --addr :9090 \
  --backend ecs-dev=http://localhost:3375 \
  --backend lambda-dev=http://localhost:3376 \
  --simulator sim-aws=http://localhost:5111

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
| `--version` | | Print version and exit. |

The admin loads components in this priority order: explicit flags → `--config` file → auto-discover from `~/.sockerless/contexts/` → persisted projects from `~/.sockerless/projects/`.

## API surface

All routes live under `/api/v1/`. Selected paths (full set in `api_*.go`):

| Path | Purpose |
|---|---|
| `GET /api/v1/overview` | Cluster-wide snapshot (component counts, registered components, aggregate container count). |
| `GET /api/v1/components` | List registered backends and simulators with their current health. |
| `GET /api/v1/components/{name}/{health,status,metrics,provider}` | Proxy component inspection calls. |
| `POST /api/v1/components/{name}/reload` | Ask a registered component to reload when it supports reload. |
| `GET /api/v1/containers` | Aggregate `docker ps` across all backends. |
| `GET /api/v1/contexts` | Available CLI contexts. |
| `GET /api/v1/processes` | Process manager list. |
| `POST /api/v1/processes/{name}/{start,stop,restart}` | Start, stop, or restart a managed process. |
| `POST /api/v1/processes/stop-all` | Stop every managed process. |
| `GET /api/v1/processes/{name}/logs` | Tail one managed process log. |
| `GET /api/v1/projects`, `POST /api/v1/projects` | Projects (named groups of resources). |
| `GET /api/v1/resources`, `POST /api/v1/resources/cleanup` | Orphan cloud-resource listing and cleanup. |
| `GET /api/v1/cleanup/scan`, `POST /api/v1/cleanup/{processes,tmp,containers}` | Local cleanup scans and actions. |
| `GET /api/v1/topology`, `PUT /api/v1/topology` | Topology graph (projects x instances x bindings). |
| `GET /api/v1/topology/file` | Active topology file path and existence. |
| `GET /api/v1/topology/instances` | Flat instance list across projects. |
| `POST /api/v1/topology/projects`, `DELETE /api/v1/topology/projects/{project}` | Add or remove topology projects. |
| `POST /api/v1/topology/projects/{project}/instances` | Add an instance. |
| `GET/PUT/DELETE /api/v1/topology/projects/{project}/instances/{instance}` | Inspect, update, or remove an instance. |
| `GET /api/v1/topology/projects/{project}/instances/{instance}/status` | PID + health status for one instance. |
| `POST /api/v1/topology/projects/{project}/instances/{instance}/{start,stop,restart,rebuild,reload}` | Drive real `make` lifecycle for one instance. |
| `POST /api/v1/topology/stop-all` | Schedule the repo's real stack shutdown path. |
| `POST /api/v1/topology/allocate-port?kind=<sim|backend>` | Allocate the next free configured port. |
| `GET /api/v1/topology/projects/{project}/instances/{instance}/logs` | Tail one instance log. |
| `GET /api/v1/topology/projects/{project}/instances/{instance}/diagnostics` | Instance drift diagnostics. |
| `POST /api/v1/topology/projects/{project}/instances/{instance}/proxy` | Reverse-proxy raw HTTP into an instance for ad-hoc debugging. |
| `GET /api/v1/topology/resources` | Topology-aware resource rollup. |
| `GET /api/v1/topology/config-metadata` | Editable config keys grouped by component kind/backend. |
| `GET /api/v1/observability` | Admin observability endpoint configuration. |

The full handler set is enumerated in the source: see the `api_*.go` files in this directory.

## UI

`http://localhost:9090/ui/` serves the embedded SPA. Routes:

- **Overview** — cluster snapshot.
- **Components** — registered backends and simulators, per-component health, env vars, version, last poll, and direct component UI links.
- **Component detail** — one component's health, provider data, metrics, reload action, and recovery commands.
- **Containers** — aggregate `docker ps`.
- **Contexts** — CLI contexts visible to this admin process.
- **Processes / process detail** — local managed-process start, stop, restart, stop-all, and log tail.
- **Topology** — project/instance graph, default local contexts, topology file path, start/stop/restart/rebuild/reload, stop-stack, and make-command recovery panels.
- **Topology resources** — resource rollup across topology instances.
- **Project console** — project-scoped live view.
- **Instance logs** — one topology instance's log stream.
- **Metrics** — component metrics view.
- **Cleanup** — local cleanup scans and actions.

UI implementation lives in `ui/packages/admin/`. The Go binary embeds the built bundle via `go:embed` (build tag `!noui`, on by default).

## Sample

```bash
$ ./sockerless-admin --addr :9090 \
    --backend ecs-dev=http://localhost:3375 \
    --simulator sim-aws=http://localhost:5111 &

$ curl -s http://localhost:9090/api/v1/overview | jq .
{
  "backends": 1,
  "components": [
    {
      "name": "ecs-dev",
      "type": "backend",
      "addr": "http://localhost:3375",
      "health": "up",
      "uptime": 12
    },
    {
      "name": "sim-aws",
      "type": "simulator",
      "addr": "http://localhost:5111",
      "health": "up",
      "uptime": 12
    }
  ],
  "components_down": 0,
  "components_total": 2,
  "components_up": 2,
  "simulators": 1,
  "total_containers": 0
}

$ curl -s http://localhost:9090/api/v1/components | jq '.[]'
{
  "name": "ecs-dev",
  "type": "backend",
  "addr": "http://localhost:3375",
  "health": "up",
  "uptime": 12
}
```

## Shauth browser sign-in

The operator console supported optional Shauth OpenID Connect sign-in without
changing any simulator, backend, SDK, command-line interface, or Terraform API
endpoint. Configure all four values together in a deployed console:

```sh
SOCKERLESS_ADMIN_SHAUTH_ISSUER=https://auth.dev.e6qu.dev
SOCKERLESS_ADMIN_SHAUTH_CLIENT_ID=sockerless-admin-dev
SOCKERLESS_ADMIN_SHAUTH_CLIENT_SECRET=<from AWS Secrets Manager>
SOCKERLESS_ADMIN_PUBLIC_URL=https://admin.dev.e6qu.dev
```

Register these Shauth relying-party coordinates:

- redirect URI: `https://admin.dev.e6qu.dev/auth/shauth/callback`
- post-logout redirect URI: `https://admin.dev.e6qu.dev/auth/signed-out`
- back-channel logout URI: `https://admin.dev.e6qu.dev/auth/shauth/backchannel-logout`

The console discovered Shauth, used authorization code + PKCE and nonce
validation, verified the signed ID token, and accepted only `developer` or
`admin` roles. It displayed the signed-in user, role, initial avatar, and a
logout control. Browser sessions were server-tracked, so a signed OIDC
Back-Channel Logout token revoked matching sessions by `sid` or `sub`; replayed
logout tokens were rejected by `jti`. Signing out initiated logout at Shauth's
discovered `end_session_endpoint` with an ID-token hint, so signing out of the
console ended the shared Shauth session rather than immediately signing the
user back in. Cookies were secure and HTTP-only; development could explicitly
opt into insecure cookies with
`SOCKERLESS_ADMIN_INSECURE_COOKIES=true`.

With no Shauth variables the existing local operator workflow remained
unauthenticated. Partial or non-HTTPS production configuration failed at
startup rather than exposing a partially protected console.

## Known issues

None open. The admin's components-decoupled invariant is load-bearing: components must remain runnable standalone via `make backends/<x>/run`, with the admin reading only `/v1/health` + `/v1/info` + env vars. No admin-side env vars on the components.

## What's out of scope

- **Multi-machine orchestration.** This is a single-machine local-dev orchestrator. For multi-host serverless capacity, the cloud's own orchestrator (ECS Fargate, Cloud Run, ACA, etc.) is the answer; the admin is for routing.
- **Persistent state across restarts.** Project + topology files persist to `~/.sockerless/projects/` and the topology file. Live component health resets on admin restart.
- **Cloud service deployment.** The admin remains a local operator application;
  a public deployment needs its own Amazon Elastic Container Service service,
  Shauth client, and AWS Secrets Manager client secret. The simulator API
  binaries themselves remain cloud-protocol endpoints rather than browser apps.
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
├── api_overview.go               GET /api/v1/overview
├── api_components.go             /api/v1/components — registered components + health
├── api_containers.go             /api/v1/containers — aggregate docker ps
├── api_contexts.go               /api/v1/contexts — CLI contexts visible from here
├── api_cleanup.go                /api/v1/cleanup — local cleanup scans + actions
├── api_processes.go              /api/v1/processes — process start/stop/restart/logs
├── api_projects.go               /api/v1/projects — named resource groups
├── api_resources.go              /api/v1/resources — cloud-resource listing + cleanup
├── api_topology.go               /api/v1/topology — graph CRUD + lifecycle
├── api_topology_config.go        Per-instance config metadata + update + reload
├── api_topology_diagnostics.go   Drift detection
├── api_topology_logs.go          Per-instance log tail
├── api_topology_proxy.go         Reverse-proxy raw HTTP into instances
├── api_topology_resources.go     Topology-aware resource rollup
├── api_observability.go          OTel endpoint configuration
└── *_test.go                     Per-handler unit tests
```

See also: [`docs/ADMIN_ORCHESTRATION.md`](../../docs/ADMIN_ORCHESTRATION.md) for design background and [`cmd/sockerless/README.md`](../sockerless/README.md) for the single-backend CLI counterpart.
