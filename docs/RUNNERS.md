# CI runners on Sockerless — wiring guide

Sockerless serves the Docker REST API, so any CI runner that talks Docker (GitHub Actions `actions/runner`, GitLab Runner with the `docker` executor) can use Sockerless as its container runtime. Set the runner's `DOCKER_HOST` (or `[runners.docker].host`) to a Sockerless daemon and every job container, service container, and `uses: docker://…` action lands on the configured cloud backend (ECS Fargate, AWS Lambda, etc.) instead of a local Docker daemon.

This is the canonical wiring guide. Per-flow detail and historical harnesses:

- [GITHUB_RUNNER_SAAS.md](./GITHUB_RUNNER_SAAS.md) — real `github.com` + `actions/runner`
- [GITLAB_RUNNER_SAAS.md](./GITLAB_RUNNER_SAAS.md) — real `gitlab.com` + `gitlab-runner`
- [GITHUB_RUNNER.md](./GITHUB_RUNNER.md) — local `act` simulator harness
- [GITLAB_RUNNER_DOCKER.md](./GITLAB_RUNNER_DOCKER.md) — self-hosted GitLab CE compose harness
- [runner-capability-matrix.md](./runner-capability-matrix.md) — per-backend / per-pipeline pass matrix

## Architecture

The runner is **long-lived** — it polls the platform's job queue. The Sockerless daemon is **long-lived** — it dispatches per-job containers to the cloud. The cloud workload is **per-job** — ephemeral Fargate task or Lambda invocation.

```
your laptop / runner host                     │   AWS account
                                              │
┌────────────────┐                            │
│ actions/runner │──┐                         │
│ (long-lived)   │  │                         │
└────────────────┘  │ DOCKER_HOST=tcp://      │
                    ├──→ localhost:3375 ──→ ┌─┴────────┐    ECS RunTask    ┌───────────────┐
┌────────────────┐  │                       │sockerless│───or────────────→ │  ECS Fargate  │
│ gitlab-runner  │──┘                       │ (ECS)    │    Lambda Invoke  │  ──or──       │
│ (long-lived)   │                          └──────────┘                   │  Lambda       │
└────────────────┘                                                         └───────────────┘
                                                                                ▲
┌────────────────┐                          ┌──────────┐                        │
│ actions/runner │──┐ DOCKER_HOST=tcp://    │sockerless│   (same dispatch path) │
│ (Lambda label) │  ├──→ localhost:3376 ──→ │ (Lambda) │ ───────────────────────┘
└────────────────┘  │                       └──────────┘
┌────────────────┐  │
│ gitlab-runner  │──┘
│ (Lambda tag)   │
└────────────────┘
```

**Why two Sockerless daemons.** Today, each daemon binds to one backend. Until the multi-tenant pool router (PLAN.md § Phase 68 v2) lands, label-based routing is "one Sockerless daemon per `runs-on` label / `tags:` value." Two ports = two daemons = two labels.

**Why one Sockerless daemon serves both runners.** A runner's `DOCKER_HOST` is just a Docker REST API endpoint. The same daemon can simultaneously serve a GitHub `actions/runner` *and* a `gitlab-runner` — they don't see each other's containers (GitHub uses a label-namespaced container name; GitLab uses `runner-<id>-project-<id>-...`). One daemon → two clients = fine.

## Coverage matrix (4 cells)

Every cell runs end-to-end against real cloud infrastructure.

| Runner | Backend | Sockerless port | Runner label / tag | Notes |
|---|---|---|---|---|
| GitHub `actions/runner` | ECS Fargate | `:3375` | `sockerless-ecs` | Long-running jobs, services, matrix, artifacts. |
| GitHub `actions/runner` | AWS Lambda | `:3376` | `sockerless-lambda` | One-shot jobs ≤10 min wall. No services, no detached containers. |
| `gitlab-runner` (docker exec) | ECS Fargate | `:3375` | `sockerless-ecs` | Same daemon as GH ECS runner. |
| `gitlab-runner` (docker exec) | AWS Lambda | `:3376` | `sockerless-lambda` | Same daemon as GH Lambda runner. |

**Lambda 15-minute hard limit.** Lambda invocations cannot exceed 15 minutes. The runner pattern (long-poll + dispatch container) is implemented in container mode so the user code runs as a Lambda invocation, but each invocation is the *whole job*. Real CI jobs that take >10 min should use the ECS label. Lambda is for short, fast workloads — lint, format, container actions (`uses: docker://...`), single-command tests.

## Token strategy

Goal: **no long-lived tokens in env vars / project settings / disk plaintext / shell history.**

Two layers:

| Layer | Lifetime | Storage |
|---|---|---|
| Long-lived **personal access token** (PAT) — your identity for the platform API | Until you rotate it | macOS Keychain (encrypted, OS-managed) |
| Short-lived **runner registration token** — minted from the PAT, scoped to one runner | Minutes (mints fresh per harness run; deleted on harness exit) | Process memory only (env var inside the harness, never written to disk) |

### Layer 1 — PAT setup (one-time per machine)

**GitHub** — `gh` CLI is keychain-backed by default on macOS:

```bash
gh auth login          # interactive; pick your method
gh auth status         # confirm: "Token: gho_*** (keychain)"
```

The harness reads via `gh auth token`. The token only enters our process memory; it never lands in shell history, never gets exported to a child process beyond the harness.

**GitLab** — `glab` writes plaintext to `~/.config/glab-cli/config.yml`, which violates the no-disk-plaintext rule. We route around it via macOS Keychain directly. One-time setup:

```bash
# Paste the GitLab PAT (with create_runner + api scopes) when prompted:
security add-generic-password -U -s sockerless-gl-pat -a "$USER" -w
```

The harness reads via:

```bash
GL_TOKEN=$(security find-generic-password -s sockerless-gl-pat -a "$USER" -w)
```

First read of a session triggers a Keychain unlock prompt; "Always allow" makes subsequent reads silent.

### Layer 2 — runner registration tokens (per harness run)

**GitHub flow (mints + uses + deletes):**

```bash
GH_TOKEN=$(gh auth token)                                                    # keychain → memory
REG_TOKEN=$(gh api -X POST \
  /repos/e6qu/sockerless/actions/runners/registration-token --jq .token)
./config.sh --url https://github.com/e6qu/sockerless --token "$REG_TOKEN" \
  --name "sockerless-ecs-$$" --labels sockerless-ecs \
  --unattended --replace --ephemeral
# ... harness dispatches workflow_dispatch, polls for completion ...
RM_TOKEN=$(gh api -X POST \
  /repos/e6qu/sockerless/actions/runners/remove-token --jq .token)
./config.sh remove --token "$RM_TOKEN"
unset GH_TOKEN REG_TOKEN RM_TOKEN
```

`--ephemeral` means GitHub auto-deregisters the runner after one job completes; even if the harness crashes, GitHub auto-cleans within 24 h.

**GitLab flow (creates + uses + deletes):**

```bash
GL_TOKEN=$(security find-generic-password -s sockerless-gl-pat -a "$USER" -w)
PROJECT_ID=$(curl -fsS -H "PRIVATE-TOKEN: $GL_TOKEN" \
  https://gitlab.com/api/v4/projects/e6qu%2Fsockerless | jq -r .id)
RUNNER_JSON=$(curl -fsS -X POST -H "PRIVATE-TOKEN: $GL_TOKEN" \
  https://gitlab.com/api/v4/user/runners \
  -d runner_type=project_type \
  -d "project_id=$PROJECT_ID" \
  -d "tag_list[]=sockerless-ecs")
RUNNER_ID=$(echo "$RUNNER_JSON" | jq -r .id)
RUNNER_AUTH=$(echo "$RUNNER_JSON" | jq -r .token)
gitlab-runner register --non-interactive --url https://gitlab.com \
  --token "$RUNNER_AUTH" --executor docker \
  --docker-host tcp://localhost:3375
# ... harness triggers pipeline, polls for completion ...
gitlab-runner unregister --all-runners
curl -fsS -X DELETE -H "PRIVATE-TOKEN: $GL_TOKEN" \
  "https://gitlab.com/api/v4/runners/$RUNNER_ID"
unset GL_TOKEN RUNNER_AUTH
```

**Self-healing cleanup.** Each harness run starts by listing existing runners with the `sockerless-` name prefix and deleting any leftovers from a previous crash.

### What the repo never holds

- ❌ No PAT in `.env`, `Makefile`, Terraform variables, GitHub Actions secrets, GitLab CI variables
- ❌ No `GITHUB_TOKEN=...` / `GITLAB_TOKEN=...` line in any shell rc we ship
- ❌ No registration token cached on disk past harness exit
- ❌ No `glab auth login` (because that writes plaintext)

The runner's own `.runner` / `config.toml` files contain the *registration* token, not the PAT — and they live under `tests/runners/{github,gitlab}/.runners/<id>/` (gitignored).

### Token rotation

A leaked PAT can be revoked instantly:

- **GitHub** — `https://github.com/settings/tokens` → revoke → `gh auth login` again
- **GitLab** — `https://gitlab.com/-/user_settings/personal_access_tokens` → revoke → `security delete-generic-password -s sockerless-gl-pat -a "$USER" && security add-generic-password -U -s sockerless-gl-pat -a "$USER" -w`

## Local Sockerless wiring

Both daemons run on your laptop, both dispatch to the same AWS account:

```bash
# Terminal 1 — Sockerless ECS daemon, port 2375
sockerless --backend=ecs --listen=tcp://localhost:3375 \
  --aws-region=us-east-1 \
  --ecs-cluster=sockerless-runner-ecs \
  --ecs-subnets=subnet-… --ecs-security-groups=sg-…

# Terminal 2 — Sockerless Lambda daemon, port 2376
sockerless --backend=lambda --listen=tcp://localhost:3376 \
  --aws-region=us-east-1 \
  --lambda-execution-role=arn:aws:iam::…:role/sockerless-lambda
```

Live infra (cluster, subnets, role) is provisioned via `manual-tests/01-infrastructure.md`. Both daemons share the same VPC + IAM scaffolding so runner jobs can reach each other when needed (e.g., a runner needing a transient cache via S3 — the IAM role grants S3:Get/Put on the cache bucket).

## Harness layout

```
tests/runners/
├── github/
│   ├── harness_test.go              # build-tag: github_runner_live
│   ├── workflows/
│   │   ├── hello-ecs.yml            # workflow_dispatch only
│   │   ├── hello-lambda.yml
│   │   ├── gotest-ecs.yml
│   │   └── service-container-ecs.yml
│   └── README.md
├── gitlab/
│   ├── harness_test.go              # build-tag: gitlab_runner_live
│   ├── pipelines/
│   │   ├── hello-ecs.gitlab-ci.yml
│   │   ├── hello-lambda.gitlab-ci.yml
│   │   └── service-job-ecs.gitlab-ci.yml
│   └── README.md
└── internal/
    └── tokens.sh                    # gh_pat() and gl_pat() shell helpers
```

The Go harnesses shell out to `tokens.sh` for the PAT lookup, then make HTTP API calls directly. Workflows are pushed to `e6qu/sockerless` under `.github/workflows/`; pipelines are referenced by path from `tests/runners/gitlab/pipelines/` and triggered via the GitLab API's `play_pipeline` against an ad-hoc branch (no commit to default needed).

### Workflow / pipeline trigger discipline

To keep runner test workflows from firing on every commit:

- **GitHub** workflows under `.github/workflows/sockerless-runner-*.yml` use **only** `workflow_dispatch:` and `pull_request: paths: ['tests/runners/**']`. They never trigger on push to `main`.
- **GitLab** pipelines live under `tests/runners/gitlab/pipelines/` (not at the root `.gitlab-ci.yml`). They're triggered via `POST /projects/:id/pipeline` with `ref` set to a throwaway branch the harness creates.

## Verification (the 4 cells)

Once tokens are wired and live infra is up:

```bash
# GitHub × ECS
go test -tags github_runner_live -run TestGitHub_ECS_Hello -timeout 30m \
  ./tests/runners/github

# GitHub × Lambda
go test -tags github_runner_live -run TestGitHub_Lambda_Hello -timeout 30m \
  ./tests/runners/github

# GitLab × ECS
go test -tags gitlab_runner_live -run TestGitLab_ECS_Hello -timeout 30m \
  ./tests/runners/gitlab

# GitLab × Lambda
go test -tags gitlab_runner_live -run TestGitLab_Lambda_Hello -timeout 30m \
  ./tests/runners/gitlab
```

Each test sub-run handles its own runner lifecycle (mint registration token → register → dispatch → poll → unregister → clean up). Failure modes (network, IAM, image pull) surface as the runner's own log lines + the harness's own assertions, not silent skips.

Cross-references:

- Manual tests against live infra: [`manual-tests/02-aws-runbook.md`](../manual-tests/02-aws-runbook.md)
- Backend capability matrix: [`runner-capability-matrix.md`](./runner-capability-matrix.md)
- Architectural reference: [`specs/CLOUD_RESOURCE_MAPPING.md`](../specs/CLOUD_RESOURCE_MAPPING.md), [`specs/SOCKERLESS_SPEC.md`](../specs/SOCKERLESS_SPEC.md)
