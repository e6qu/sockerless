# Docker Backend

Thin passthrough: every request to this backend's port is forwarded to a local Docker daemon (Docker Desktop, Colima, Podman, etc.). The backend is the simplest reference shape — no cloud connector, no agent sidecar, no per-container state in the backend's Store. Useful for development, integration tests, and as a baseline when validating the Docker REST API surface itself.

## Reference adaptor

The Docker REST API on its `:3375` port is exercised by three external tools:

| Adaptor | Min version | What it proves |
|---|---|---|
| **Docker Go SDK** (`github.com/docker/docker/client`) | v25+ | Full SDK compatibility — used by `tests/` and `actions/runner`. |
| **`docker` CLI** | 29.x | Wire-level Docker REST v1.44. **Partially blocked by [BUG-991](#known-issues)** for the foreground `docker run` flow. |
| **`podman` CLI** | 5.x | Docker-compat shim (`podman --url tcp://…`). Same wire as `docker`; same BUG-991 surface. |

The contract: anything the Docker SDK / CLI does against `unix:///var/run/docker.sock`, it must do against this backend. Gaps from that contract are real bugs and filed in [BUGS.md](../../BUGS.md).

## Validation

| Test path | What runs | Last green |
|---|---|---|
| `tests/` (Docker SDK, 59 functions: `containers_test.go`, `images_test.go`, `volumes_test.go`, `networks_test.go`, `exec_test.go`, `streaming_test.go`, etc.) | Real Docker Go SDK against the running backend; asserts containers / images / volumes / networks / exec / logs / attach round-trip. | 2026-05-13 (PR #156 on main) |
| `tests/github_runner_e2e_test.go`, `tests/gitlab_runner_e2e_test.go` | The official `actions/runner` and `gitlab-runner` (real binaries) drive Docker REST against this backend; the runner's job executes inside containers it created via this passthrough. | 2026-05-13 |
| `make backends/docker/test` | The leaf-Makefile unit/integration suite per `docs/MAKEFILE_STANDARD.md`. | 2026-05-13 |

The SDK path is the load-bearing validation today. The CLI path is partially blocked — see [Known issues](#known-issues).

## Wiring the adaptor

```bash
# 1. Build + start the backend (default :3375).
cd backends/docker && make build
./sockerless-backend-docker --addr :3375 --log-level info &

# 2. Point any Docker client at it.
export DOCKER_HOST=tcp://localhost:3375
# (podman equivalent: podman --url tcp://localhost:3375 …)
```

| Variable | Default | What it does |
|---|---|---|
| `--addr` | `:3375` | Listen address (host:port). |
| `--docker-host` | auto from env | Override the upstream daemon socket. |
| `--log-level` | `info` | `debug` shows every HTTP request + downstream call. |
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | Upstream daemon when `--docker-host` is unset. |
| `DOCKER_TLS_VERIFY`, `DOCKER_CERT_PATH` | unset | Forwarded to the docker client when talking to a remote upstream. |

The backend has zero local container state: every `GET /containers/{id}` reaches through to the upstream daemon. There is no Store to keep in sync, no agent to deploy.

## Sample

Working `docker` CLI calls against the backend (captured 2026-05-13, real output):

```bash
$ DOCKER_HOST=tcp://localhost:3375 docker version --format '{{.Client.Version}} client / {{.Server.Version}} server'
29.4.2 client / 5.4.2 server

$ curl -sS http://localhost:3375/_ping
OK

$ curl -sS http://localhost:3375/v1.44/version | jq -r '"ApiVersion=\(.ApiVersion) Version=\(.Version) Os=\(.Os)"'
ApiVersion=1.44 Version=5.4.2 Os=darwin
```

Full lifecycle via the **Docker Go SDK** (excerpt from the pattern used by `tests/containers_test.go`):

```go
cli, _ := client.NewClientWithOpts(
    client.WithHost("tcp://localhost:3375"),
    client.WithAPIVersionNegotiation(),
)

resp, _ := cli.ContainerCreate(ctx, &container.Config{
    Image: "alpine:3.20", Cmd: []string{"echo", "hello"},
}, nil, nil, nil, "")
cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
waitCh, errCh := cli.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
select {
case <-waitCh:
    // exited
case err := <-errCh:
    t.Fatalf("wait: %v", err)
}
logs, _ := cli.ContainerLogs(ctx, resp.ID, container.LogsOptions{ShowStdout: true})
// → "hello\n"
```

The SDK lets the caller order `create → start → wait` deterministically. This is the path that `tests/` exercises 59 times per CI run.

## Known issues

- **BUG-991** — `docker run` (CLI) returns `error waiting for container: No such container: <id>`. The CLI's foreground flow sends `POST /containers/{id}/wait?condition=next-exit` *before* `POST /containers/{id}/start`; this backend's wait handler (in `backends/core/handle_containers.go`) checks the local Store directly, finds nothing (containers live in the upstream daemon, not in the Store), and returns 200/StatusCode=0 immediately. The CLI then treats wait-completed-before-start as a crash. Fix shape: delegate to `s.self.ContainerWait` when the local Store has no record and `s.self != nil`. Staged as Phase 158 in [PLAN.md](../../PLAN.md). Until then, drive this backend via the **Docker Go SDK** (which orders the calls deterministically) or via raw `curl`.
- **Image / volume / network list under-fills** when the local Store is empty. `GET /images/json` returns `[]` even when the upstream daemon has images; same handler-shape issue as BUG-991. Will likely fold into Phase 158.

## What's out of scope

- Local container state (intentionally — this is a passthrough, not a stateful backend).
- Image build orchestration beyond what the upstream daemon does (no remote layer mirroring, no per-cloud registry rewrite).
- Multi-host container scheduling (this is a single-daemon passthrough).
- The cloud-side adaptors (aws CLI, gcloud, az) — they belong to the cloud backends (`backends/{ecs,lambda,cloudrun,…}/README.md`).

See also: [docs/POD_MATERIALIZATION.md § Docker](../../docs/POD_MATERIALIZATION.md), [specs/CLOUD_RESOURCE_MAPPING.md § Docker](../../specs/CLOUD_RESOURCE_MAPPING.md), [tests/README.md](../../tests/README.md), [docs/MAKEFILE_STANDARD.md](../../docs/MAKEFILE_STANDARD.md).
