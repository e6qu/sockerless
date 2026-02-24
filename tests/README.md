# tests

End-to-end integration tests that exercise the full Sockerless stack — frontend, backend, and Docker SDK — to verify Docker API compatibility across all backends.

## Overview

The test suite uses the official Docker SDK (`docker v27.5.1`) to send real Docker API requests through a Sockerless frontend to a backend. By default it tests the memory backend; with environment variables, it can also test against cloud simulator backends (ECS, Lambda, CloudRun, Cloud Functions, ACA, Azure Functions).

## Running

```sh
# Memory backend only (default)
cd tests
go test -v ./...

# From the repository root
make test-e2e

# Against an external Docker-compatible endpoint
SOCKERLESS_SOCKET=/var/run/docker.sock go test -v ./...
```

## Test setup

`TestMain` handles the full lifecycle:

1. Builds the memory backend and Docker frontend binaries
2. Starts both on dynamically allocated free ports
3. Waits for health checks (`/internal/v1/info` and `/_ping`)
4. Creates a Docker SDK client pointing at the frontend
5. Optionally starts cloud simulator backends via `SOCKERLESS_SIM`

## Test suites

| File | Tests | What it covers |
|------|-------|----------------|
| `containers_test.go` | 8 | Create, inspect, start, stop, list, kill, remove, wait, name conflicts |
| `images_test.go` | 3 | Pull, inspect, tag |
| `exec_test.go` | 2 | Exec create, inspect, start with output capture |
| `volumes_test.go` | 4 | Volume create, inspect, list, remove |
| `networks_test.go` | 5 | Network create, inspect, list, remove, prune |
| `system_test.go` | 4 | Ping (GET/HEAD), version, info |
| `monitoring_test.go` | 2 | Container stats (single + streaming) |
| `streaming_test.go` | 2 | Container logs, attach with stream demux |
| `script_exec_test.go` | 1 | Archive upload + exec (GitHub Actions pattern) |
| `compose_labels_test.go` | 1 | Docker Compose label filtering |
| `monitoring_resource_tracking_test.go` | 1 | Resource tag preservation |
| `reverse_exec_test.go` | 1 | Reverse agent WebSocket connection |
| `github_runner_e2e_test.go` | 1 | Full GitHub Actions runner workflow |
| `gitlab_runner_e2e_test.go` | 1 | Full GitLab Runner docker-executor workflow |

## Multi-backend testing

The GitHub and GitLab runner E2E tests run against all available backends:

```go
for name, client := range availableRunnerClients(t) {
    t.Run(name, func(t *testing.T) { ... })
}
```

Backend sockets are discovered via environment variables:

| Variable | Backend |
|----------|---------|
| `SOCKERLESS_ECS_SOCKET` | AWS ECS |
| `SOCKERLESS_CLOUDRUN_SOCKET` | GCP Cloud Run |
| `SOCKERLESS_ACA_SOCKET` | Azure Container Apps |
| `SOCKERLESS_DOCKER_SOCKET` | Docker daemon |

## Simulator harness

`sim_harness_test.go` can start cloud simulator backends inline for E2E testing. Set `SOCKERLESS_SIM=ecs,cloudrun,aca` to start specific backends.

## Project structure

```
tests/
├── main_test.go                         TestMain: build, start, health check
├── containers_test.go                   Container lifecycle tests
├── images_test.go                       Image pull/inspect/tag
├── exec_test.go                         Exec create/start
├── volumes_test.go                      Volume CRUD
├── networks_test.go                     Network CRUD + prune
├── system_test.go                       Ping, version, info
├── monitoring_test.go                   Stats (single + stream)
├── streaming_test.go                    Logs + attach
├── script_exec_test.go                  Archive upload + exec
├── compose_labels_test.go               Compose label filtering
├── monitoring_resource_tracking_test.go  Resource tag smoke test
├── reverse_exec_test.go                 Reverse agent WebSocket
├── github_runner_e2e_test.go            GitHub Actions runner E2E
├── gitlab_runner_e2e_test.go            GitLab Runner E2E
├── sim_harness_test.go                  Cloud simulator startup
└── helpers_test.go                      Test utilities and multi-client setup
```
