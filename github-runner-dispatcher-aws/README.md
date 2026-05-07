# github-runner-dispatcher

Sockerless-agnostic poller that watches a GitHub repo for queued
`workflow_job`s and spawns one runner container per job via the local
`docker` CLI. Talks to whichever Docker daemon `DOCKER_HOST` points at
— local Podman, Docker Desktop, or sockerless are all equivalent
targets. The dispatcher's only external dependency is
`github.com/BurntSushi/toml`; everything else is in the standard
library.

This is the Phase 110a deliverable from
[`PLAN.md`](../PLAN.md). Phase 113 productionises it (webhook ingress,
GitHub App install model, warm pool); the laptop-foreground polling
shape lands first.

## Usage

```bash
# 1. Pre-pull the runner image to the target daemon.
DOCKER_HOST=tcp://localhost:3375 docker pull \
  729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live:runner-amd64

# 2. Write ~/.sockerless/dispatcher/config.toml:
mkdir -p ~/.sockerless/dispatcher
cat > ~/.sockerless/dispatcher/config.toml <<'EOF'
[[label]]
name        = "sockerless-ecs"
docker_host = "tcp://localhost:3375"
image       = "729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live:runner-amd64"

[[label]]
name        = "sockerless-lambda"
docker_host = "tcp://localhost:3376"
image       = "729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live:runner-amd64"
EOF

# 3. Run.
GITHUB_TOKEN=$(gh auth token) \
  go run ./cmd/github-runner-dispatcher --repo e6qu/sockerless
```

Flags:

| Flag             | Default                                       | Notes |
|------------------|-----------------------------------------------|-------|
| `--repo`         | (required)                                    | `owner/repo`. No default — explicit-only. |
| `--token`        | `$GITHUB_TOKEN`                               | PAT with `repo` + `workflow` scopes. |
| `--config`       | `~/.sockerless/dispatcher/config.toml`        | Optional; missing file = empty config (every job skipped). |
| `--once`         | `false`                                       | Run a single poll cycle + cleanup and exit. Used by smoke tests. |
| `--cleanup-only` | `false`                                       | Run a single GC sweep (containers + GitHub runners) and exit. No polling. |

## Architecture

| Concern              | Behaviour |
|----------------------|-----------|
| Polling              | `GET /repos/{repo}/actions/runs?status=queued` + per-run `GET .../jobs` every 15 s. |
| Dedup                | Per-job seen-set with 5-min TTL. Stateless: each spawned container is stamped with `sockerless.dispatcher.job_id=<jobID>` so a restart can rebuild the set from `docker ps --filter label=…`. No on-disk dispatcher state. |
| Spawner              | `docker run --rm -d --pull never --label sockerless.dispatcher.* <image> -e RUNNER_REG_TOKEN=… …`. |
| Idle cleanup         | The runner image's entrypoint enforces a 60-s "no job arrived" timeout. Cleans up duplicate-spawn races without dispatcher state. |
| State recovery       | On startup: list dispatcher-labelled containers across every configured `docker_host`; rehydrate the seen-set from their `job_id` labels. Daemon-down at startup is non-fatal — the container's still running, the next Liveness check will reconcile. |
| GC sweep             | Every 2 min and at startup: `docker rm` exited / dead dispatcher containers; `DELETE /actions/runners/{id}` for offline `dispatcher-*` runners on GitHub. Keeps the GitHub UI clean even when `--rm` couldn't fire (kernel OOM, daemon restart). |
| Graceful shutdown    | SIGINT / SIGTERM → drain every dispatcher-managed container + delete every dispatcher-prefixed GitHub runner. Bounded to 30 s. |
| Liveness             | `docker info` against each label's `docker_host` before spawning; skip the cycle on failure. Doesn't crash. |
| Auth scopes          | `repo` + `workflow` checked at startup against `X-OAuth-Scopes`. Missing → fail with `gh auth refresh -s …` instructions. |
| Failure handling     | Log + skip. Job stays queued; retried next poll. |
| Logs                 | stdout. No `/metrics` / `/healthz` at this stage — laptop-foreground binary. |

## Module layout

```
github-runner-dispatcher/
├── go.mod                                     # only dep: BurntSushi/toml
├── cmd/github-runner-dispatcher/main.go       # CLI + main loop
└── internal/
    ├── config/                                # TOML config loader
    ├── poller/                                # GitHub Actions REST polling + dedup
    ├── scopes/                                # PAT scope verifier
    └── spawner/                               # `docker run` shell-out
```

## Smoke test

```bash
# Builds, runs the unit tests (no docker required), and hits an
# httptest GitHub fixture end-to-end. Optional: pass --podman to
# also drive a real `docker info` against a local Podman daemon.
go test ./...
```

## Status

Phase 110a — skeleton. 110b will:
- Wire the dispatcher into the live ECR runner image via an updated
  `config.toml` shipped with the live AWS bring-up doc.
- Validate end-to-end against cell 1 (GitHub × ECS) + cell 2
  (GitHub × Lambda) through the existing harness.
