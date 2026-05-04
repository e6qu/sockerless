# Do Next

Resume pointer. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); architecture in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Resume pointer (2026-05-04 v20 — major architectural reset)

**User directives 2026-05-04 (in order received):**
1. "we should use github runner and gitlab runner unmodified"
2. "the only acceptable thing for github is this dispatcher"
3. "the runners should work via docker-like interface of sockerless such that the runners need no changes themselves since they already talk docker"
4. "the dispatcher should just provision runners based on the job demand and should not provide any features other than to start the github runner"
5. "gitlab runner doesn't need a dispatcher because gitlab runner 'docker executor' already behaves like a dispatcher"

These collectively REVERSE the architecture cells 5-8 have been built on (custom runner images that bake `sockerless-backend-XXX` + custom `bootstrap.sh` + actions/runner / gitlab-runner into ONE image). Cell 7 was GREEN under the old architecture; that result will not survive the refactor (the new architecture is incompatible with the current Cloud Run revisions).

## New architecture (sign-off pending — verify before I rip code)

### GitHub side (cells 5 + 6)

```
┌─ github.com (queued workflow_jobs)
│
├── github-runner-dispatcher-gcp (sockerless, Cloud Run Service)
│   • polls GitHub /actions/runs?status=queued
│   • for each queued workflow_job: Executions.RunJob(<predefined-job>) + per-execution env override
│   • per-execution overrides allowed: RUNNER_REG_TOKEN, RUNNER_NAME, RUNNER_LABELS, RUNNER_REPO
│   • NO sockerless-specific env. NO image baking. NO config wiring.
│
└── pre-defined Cloud Run Job (one per cell label, defined by terraform)
    spec.template.template.containers (multi-container TaskTemplate):
      • [0] vanilla actions/runner image (e.g. ghcr.io/actions/actions-runner:2.334.0,
            or AR mirror of the upstream tarball deployed once via terraform).
            ENV: DOCKER_HOST=tcp://localhost:3375 (set in spec, not by dispatcher).
            ENV: RUNNER_* (per-execution).
      • [1] sockerless-backend-cloudrun:latest (or sockerless-backend-gcf:latest).
            ENV: SOCKERLESS_* config from terraform.
            Exposes Docker API on :3375 / :3376.
    Both containers share loopback. Runner exits with --once → execution completes.
```

### GitLab side (cells 7 + 8)

```
┌─ gitlab.com
│
└── pre-deployed Cloud Run Service (one per cell label, defined by terraform), 24/7
    spec.template.containers (multi-container revision):
      • [0] vanilla gitlab/gitlab-runner:v17.5.0 image.
            entrypoint: vanilla `gitlab-runner run`.
            config.toml mounted from a Cloud Run secret (registered with token + URL pre-baked).
            ENV: DOCKER_HOST=tcp://localhost:3375.
            gitlab-runner's docker executor polls GitLab itself (it IS the dispatcher).
      • [1] sockerless-backend-cloudrun:latest (or sockerless-backend-gcf:latest).
            Same as github side — exposes Docker API on :3375/:3376.
```

### What gets deleted (existing code that contradicts the new shape)

- `tests/runners/github/dockerfile-{cloudrun,gcf}/Dockerfile` + `Makefile` + `bootstrap.sh` — custom runner images (forbidden).
- `tests/runners/gitlab/dockerfile-{cloudrun,gcf}/Dockerfile` + `Makefile` + `bootstrap.sh` — custom runner images (forbidden).
- `tests/runners/dockerfile-base/` — BUG-945 pre-baked-base work (moot, no custom image to base off).
- The `runner:{cloudrun,gcf}-amd64` AR images stay deletable (they're orphaned by the new architecture).

### What stays (and gets adjusted)

- `github-runner-dispatcher-{aws,gcp,azure}` + `pkg/poller` + `pkg/scopes` — already correct shape; only changes are removing any sockerless-specific env injection from `internal/spawner/spawner.go::Spawn` (must NOT inject SOCKERLESS_*; only RUNNER_*).
- `backends/cloudrun/cmd/sockerless-backend-cloudrun` + `backends/cloudrun-functions/cmd/sockerless-backend-gcf` — these become the published images that the sidecar references. Need to be pushed to AR independently (one image per backend).
- `terraform/modules/cloudrun/runner.tf` (or new) — define the multi-container Cloud Run Job (github cells) + Service (gitlab cells) per cell label. Configures sockerless backend sidecar with proper env. Configures vanilla runner with proper env. No bootstrap.sh involved.
- All the recently-shipped sockerless backend code (BUG-923 cancellation channel, BUG-944 GCS-Fuse MountOptions, pool back-off, etc.) is unaffected — it's cell-step-container behavior, not runner-task behavior.

## Refactor plan (will execute on sign-off)

Order, smallest first so we can verify incrementally:

1. **Build + push standalone sockerless-backend images** to AR.
   - `us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live/sockerless-backend-cloudrun:latest`
   - `us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live/sockerless-backend-gcf:latest`
   - One Dockerfile each: `FROM gcr.io/distroless/static`, `COPY` the binary, `ENTRYPOINT [...]`. No apt, no .NET, no git → no BUG-945 flake.

2. **Pivot gitlab side first (simpler — no dispatcher involvement).**
   - New `terraform/modules/gitlab-runner/main.tf`: deploys a multi-container Cloud Run Service per cell. Container [0] = `gitlab/gitlab-runner:v17.5.0` vanilla. Container [1] = sockerless-backend sidecar.
   - Cloud Run secret with pre-registered `config.toml` (registration done out-of-band once: `gitlab-runner register --token <gitlab-token> --executor docker --docker-host tcp://localhost:3375 --docker-image alpine --output-config config.toml`).
   - Apply terraform. Trigger cell 7 — verify GREEN with vanilla gitlab-runner + sockerless sidecar.
   - Then cell 8 (gcf variant).

3. **Pivot github side.**
   - New `terraform/modules/github-runner/main.tf`: deploys ONE Cloud Run Job per cell label with multi-container TaskTemplate (vanilla actions/runner + sockerless sidecar). Stays cold (no execution); dispatcher fires executions on demand.
   - Vanilla actions/runner image: pull upstream `actions-runner:2.334.0` tarball, push to AR as `runner:vanilla-2.334.0` (this is republishing, not modifying — still vanilla).
   - Update `github-runner-dispatcher-gcp/internal/spawner/spawner.go::Spawn` to call `Executions.RunJob(<predefined-job>)` with per-execution env overrides. Strip every line that injects SOCKERLESS_* (currently strip).
   - Trigger cell 5 — verify GREEN. Then cell 6.

4. **Delete the custom runner images + Dockerfiles + bootstrap.sh files** from `tests/runners/{github,gitlab}/dockerfile-*/`. Also tear down BUG-945 work (`tests/runners/dockerfile-base/`).

5. **Update specs** (`CLOUD_RESOURCE_MAPPING.md`) to reflect the new architecture: vanilla runner + sockerless sidecar in multi-container revision; dispatcher's only role is execute-on-demand.

## What carries over from prior work (unchanged + still needed)

- BUG-923 cancellation-channel for ContainerCreate→ContainerStart pod materialization (`2b16791`) — this is for the **step containers** sockerless deploys per `docker create` from inside a runner. Unaffected.
- BUG-944 GCS-Fuse MountOptions + idempotent attach (`a7e3b00`) — same, step-container scope.
- BUG-946 integration test build tag (`e733d70`) — unrelated to runner architecture.
- Dispatcher rate-limit/poller fixes (`0f94a53` / `06561dd` / `c6e7dee`) — still correct; dispatcher's poll loop semantics don't change.

## Confirm before I refactor

If this matches your intent, reply "go" and I'll start with step 1 (standalone sockerless-backend images). If anything in the architecture sketch above is wrong, point at it and I'll revise the plan first — no code change until the shape is locked.

## Live infra to leave alone until refactor

- Dispatcher Cloud Run Service `github-runner-dispatcher-gcp` rev `00021-fb2` — keep running, refactor will replace.
- gitlab-runner-{cloudrun,gcf} Cloud Run Services — keep running, refactor will replace.
- VPC + connector + Cloud NAT — stay (architecture-agnostic).
- AR repos + secrets — stay.
