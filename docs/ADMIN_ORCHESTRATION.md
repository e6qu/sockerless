# Admin Orchestration

How `sockerless-admin` controls multiple sims / backends / bleephubs across multiple projects, declaratively from `sockerless.yaml` at the repo root.

> **Invariant:** sims, backends, bleephub, frontend-docker remain independently configurable, buildable, runnable. Admin reads only `/v1/health`, `/v1/info`, env vars they already document. No admin-required env vars on components, no startup registration, no "I'm being managed" hooks. A component started via admin behaves identically to one started by hand.

## sockerless.yaml schema

```yaml
projects:
  - name: my-aws-test
    instances:
      - { name: my-sim,   kind: sim,      cloud: aws,   port: 4566 }
      - { name: my-be,    kind: backend,  cloud: aws,   backend: ecs,
          port: 3375, sim: my-sim,
          config:
            AWS_REGION: us-east-1
            SOCKERLESS_ECS_NETWORK_DISCOVERY: service-mesh }
      - { name: my-bleep, kind: bleephub, port: 5555 }
  - name: my-gcp-test
    instances:
      - { name: gcp-sim,  kind: sim,      cloud: gcp,   port: 4567 }
      - { name: gcp-be,   kind: backend,  cloud: gcp,   backend: cloudrun,
          port: 3376, sim: gcp-sim }

ports:
  ranges:
    sim:             { from: 4500, to: 4999 }
    backend:         { from: 3300, to: 3399 }
    bleephub:        { from: 5500, to: 5599 }
    frontend-docker: { from: 9300, to: 9399 }
```

### Field rules

- **Project model preserved.** Each project is an isolated topology — instance names are unique within a project; `sim` references on backend instances must point to a sim in the **same** project.
- **Port pool is global.** Two instances across any two projects can't claim the same port. Admin's port allocator walks the configured `ports.ranges[<kind>]` and skips both already-claimed ports and currently-bound ports on the host.
- **Per-instance `config` map.** Arbitrary `KEY=VALUE` pairs passed to the component's process at start time. Admin doesn't validate the keys — components own their env-var schema. A backend that doesn't recognise an env var either ignores it or errors at its own `Validate`.
- **Instance kinds:**
  - `sim`: requires `cloud` ∈ {aws, gcp, azure}.
  - `backend`: requires `cloud` + `backend` ∈ valid pair (e.g. cloud=aws + backend=ecs|lambda).
  - `bleephub`: just `port`.
  - `frontend-docker`: just `port`.
- **Validation is fail-loud.** Duplicate names, unknown kinds, port collisions, missing sim refs → admin refuses to load / refuses the PUT. No silent fallback.

## Migration from legacy per-project JSONs

Existing `~/.sockerless/admin/projects/*.json` (the pre-Phase-79 shape, one tuple of (cloud, backend, sim_port, backend_port) per file) auto-migrate to `sockerless.yaml` on first admin start. Each JSON becomes one project with a derived `[sim, backend]` instance list. The legacy JSONs are not deleted; admin just stops reading them once the YAML exists.

## REST surface

All under `/api/v1/topology`:

| Method + path | Purpose |
|---|---|
| `GET    /api/v1/topology` | full topology document |
| `PUT    /api/v1/topology` | replace topology atomically (validate + write + swap) |
| `GET    /api/v1/topology/instances` | flat list of every instance, annotated with project name |
| `POST   /api/v1/topology/projects` | add a project (body = `ProjectConfig`) |
| `DELETE /api/v1/topology/projects/{project}` | remove a project + all its instances |
| `POST   /api/v1/topology/projects/{project}/instances` | add an instance to a project (body = `Instance`) |
| `GET    /api/v1/topology/projects/{project}/instances/{instance}` | look up one instance |
| `PUT    /api/v1/topology/projects/{project}/instances/{instance}` | edit one instance (body name must equal path name) |
| `DELETE /api/v1/topology/projects/{project}/instances/{instance}` | remove one instance |
| `GET    /api/v1/topology/projects/{project}/instances/{instance}/status` | `{running, pid, health, health_detail}` |
| `POST   /api/v1/topology/projects/{project}/instances/{instance}/start` | shells `make start-component` |
| `POST   /api/v1/topology/projects/{project}/instances/{instance}/stop` | shells `make stop-component` |
| `POST   /api/v1/topology/projects/{project}/instances/{instance}/rebuild` | shells `make rebuild-component` |
| `POST   /api/v1/topology/allocate-port?kind=<sim|backend|bleephub|frontend-docker>` | next free port from the configured pool |

Status codes:
- 400 — invalid JSON body or validation failure (renames in `PUT instance`, port collisions, etc).
- 404 — project or instance not found.
- 409 — project / instance already exists; port pool exhausted.
- 503 — lifecycle subsystem not configured (admin started without lifecycle wiring; only fires in tests).
- 500 — file write failure or any other server-side error.

## Make targets

`make/components.mk` exposes per-instance lifecycle. Admin shells these — operators can call them directly too.

```sh
# Start one instance (PID + log under .stack-pids/<NAME>.{pid,log}).
make start-component KIND=sim CLOUD=aws NAME=my-sim PORT=4566

# Backend with sim link.
make start-component KIND=backend CLOUD=aws BACKEND=ecs NAME=my-be PORT=3375 SIM_PORT=4566

# Backend with custom env.
make start-component KIND=backend CLOUD=aws BACKEND=ecs NAME=my-be PORT=3375 \
  SIM_PORT=4566 ENV_FILE=.stack-pids/my-be.env

# Stop / rebuild / tail logs.
make stop-component   NAME=my-be
make rebuild-component KIND=backend CLOUD=aws BACKEND=ecs
make logs-component   NAME=my-be LINES=500

# Sweep all running components.
make status-components
make stop-components
```

The legacy `make stack-aws-ecs` / `stack-gcp-cloudrun` / etc. now compose `rebuild-component` + `start-component` calls under the hood — same effective behaviour.

## Per-instance env files

When an instance has a `config:` map in `sockerless.yaml`, admin's lifecycle shell renders it to `.stack-pids/<name>.env` as `KEY=VALUE` lines (sorted, deterministic). The `make start-component` target sources these into the component's environment via `ENV_FILE=`. This decouples admin from the component's env schema — admin just passes through whatever the operator declared.

## Topology file path

Admin reads + writes `<cwd>/sockerless.yaml` by convention (matches how `make` finds the repo root). The `--topology` flag is reserved for a future override; today admin assumes you run it from the repo root.

## Concurrency

`TopologyManager` serialises every read + write through an `RWMutex`. `Get` returns a defensive deep copy so HTTP handlers can't race each other. `Replace` and the surgical CRUD methods (`AddProject`, `RemoveInstance`, etc.) take the write lock for the validate + persist + swap.

## What admin is *not* responsible for

- **Generating components.** Admin doesn't compile binaries inline — `rebuild-component` shells `make build` for the relevant module. The per-component build remains operator-controllable from outside admin.
- **Owning component runtime state.** PID files + logs are admin's bookkeeping; the component's actual state (containers, sim resources, etc.) lives in the component, retrieved via its own `/v1/...` API.
- **Token / secret storage.** Admin reads + writes plain `sockerless.yaml`. Production secrets belong in operator-side env / secret stores fed to the per-instance `config` map at deploy time.
