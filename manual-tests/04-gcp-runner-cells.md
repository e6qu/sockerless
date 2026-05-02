# Phase 120 — GCP runner cells (5/6/7/8) operator runbook

Four cells, all docker-executor + sockerless backends, no k8s, no GKE,
no ARC. Cells 5+6 use the GCP-native `github-runner-dispatcher-gcp`
(Phase 122) — that compensates for GitHub Actions runner not having a
"master" daemon. The dispatcher creates one Cloud Run Job execution
per queued workflow_job via `cloud.google.com/go/run/apiv2`; the
runner image bakes the matching sockerless backend so step containers
spawned by the runner go through the in-image backend → Cloud Run Job
(cell 5) or Cloud Run Function with Phase 118d pod overlay for
`services:` (cell 6). Cells 7+8 use long-lived `gitlab-runner`
containers deployed once.

| Cell | Runner | Backend | Image | Dispatcher? |
|---|---|---|---|---|
| 5 | github-actions-runner | cloudrun | `sockerless-runner-cloudrun` | github-runner-dispatcher-gcp (label: sockerless-cloudrun) |
| 6 | github-actions-runner | gcf      | `sockerless-runner-gcf`      | github-runner-dispatcher-gcp (label: sockerless-gcf) |
| 7 | gitlab-runner         | cloudrun | `sockerless-gitlab-runner-cloudrun` | none (long-lived runner polls GitLab) |
| 8 | gitlab-runner         | gcf      | `sockerless-gitlab-runner-gcf`      | none |

## Prerequisites

- `gcloud` authenticated with `editor` on `sockerless-live-46x3zg4imo`
  (or whichever live-GCP project is current — see
  `feedback_gcp_live_setup.md`).
- `GOOGLE_APPLICATION_CREDENTIALS` pointing at the SA key for that
  project.
- Sockerless backends running locally on `127.0.0.1:3375` (cloudrun)
  + `127.0.0.1:3376` (gcf) — the Phase 118 setup. Verify with
  `curl -s http://127.0.0.1:3375/_ping; curl -s http://127.0.0.1:3376/_ping`.
- `gh` authenticated against `e6qu/sockerless`.
- `glab` authenticated against the GitLab mirror.

## Step 1 — Build + push runner images

Each runner image bakes the matching sockerless backend per BUG-862.

```bash
# Cell 5 (gh × cloudrun)
cd tests/runners/github/dockerfile-cloudrun
make all   # stage + build + push to AR

# Cell 6 (gh × gcf)
cd ../dockerfile-gcf
make all

# Cell 7 (gl × cloudrun)
cd ../../gitlab/dockerfile-cloudrun
make all

# Cell 8 (gl × gcf)
cd ../dockerfile-gcf
make all
```

After `make all`, four runner images live in
`us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live`:

- `runner-cloudrun-amd64`
- `runner-gcf-amd64`
- `gitlab-runner-cloudrun-amd64`
- `gitlab-runner-gcf-amd64`

## Step 2 — Configure github-runner-dispatcher-gcp (cells 5+6)

```toml
# ~/.sockerless/dispatcher-gcp/config.toml
[[label]]
name             = "sockerless-cloudrun"
gcp_project      = "sockerless-live-46x3zg4imo"
gcp_region       = "us-central1"
image            = "us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live:runner-cloudrun-amd64"
service_account  = "github-runners@sockerless-live-46x3zg4imo.iam.gserviceaccount.com"

[[label]]
name             = "sockerless-gcf"
gcp_project      = "sockerless-live-46x3zg4imo"
gcp_region       = "us-central1"
image            = "us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live:runner-gcf-amd64"
service_account  = "github-runners@sockerless-live-46x3zg4imo.iam.gserviceaccount.com"
```

Run the dispatcher (uses GCP Application Default Credentials — `gcloud auth application-default login` first):

```bash
GITHUB_TOKEN=$(gh auth token) \
  go run ./github-runner-dispatcher-gcp/cmd/github-runner-dispatcher-gcp --repo e6qu/sockerless
```

The dispatcher polls GitHub every 15 s, mints an ephemeral runner
registration token per queued workflow_job, and spawns one runner
container per job onto the matching backend (label-based routing —
already supported by the dispatcher; no code changes needed).

## Step 3 — Deploy gitlab-runner containers (cells 7+8)

```bash
GITLAB_RUNNER_TOKEN_CR=$(security find-generic-password -s sockerless-gl-runner-cloudrun -w)
GITLAB_RUNNER_TOKEN_GCF=$(security find-generic-password -s sockerless-gl-runner-gcf -w)

DOCKER_HOST=tcp://127.0.0.1:3375 docker run -d \
  --name sockerless-gitlab-runner-cloudrun \
  -e GITLAB_URL=https://gitlab.com \
  -e GITLAB_RUNNER_TOKEN="$GITLAB_RUNNER_TOKEN_CR" \
  -e GITLAB_RUNNER_TAGS=sockerless-cloudrun \
  -e GOOGLE_APPLICATION_CREDENTIALS=/tmp/sockerless-live-46x3zg4imo-key.json \
  -e SOCKERLESS_GCR_PROJECT=sockerless-live-46x3zg4imo \
  -e SOCKERLESS_GCR_REGION=us-central1 \
  -e SOCKERLESS_GCP_BUILD_BUCKET=sockerless-live-46x3zg4imo-build \
  -v /tmp/sockerless-live-46x3zg4imo-key.json:/tmp/sockerless-live-46x3zg4imo-key.json:ro \
  us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live:gitlab-runner-cloudrun-amd64

DOCKER_HOST=tcp://127.0.0.1:3376 docker run -d \
  --name sockerless-gitlab-runner-gcf \
  -e GITLAB_URL=https://gitlab.com \
  -e GITLAB_RUNNER_TOKEN="$GITLAB_RUNNER_TOKEN_GCF" \
  -e GITLAB_RUNNER_TAGS=sockerless-gcf \
  -e GOOGLE_APPLICATION_CREDENTIALS=/tmp/sockerless-live-46x3zg4imo-key.json \
  -e SOCKERLESS_GCF_PROJECT=sockerless-live-46x3zg4imo \
  -e SOCKERLESS_GCF_REGION=us-central1 \
  -e SOCKERLESS_GCP_BUILD_BUCKET=sockerless-live-46x3zg4imo-build \
  -e SOCKERLESS_GCF_BOOTSTRAP=/opt/sockerless/sockerless-gcf-bootstrap \
  -v /tmp/sockerless-live-46x3zg4imo-key.json:/tmp/sockerless-live-46x3zg4imo-key.json:ro \
  us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live:gitlab-runner-gcf-amd64
```

Each `docker run` creates a long-lived sockerless-managed Cloud Run
Service (or Job). The gitlab-runner inside polls GitLab; each job's
step containers go through the in-image sockerless backend → Cloud
Run Job (cell 7) or Cloud Run Function (cell 8) per step.

## Step 4 — Run the cells

Two paths — the test harness (Go-controlled, validates each cell
end-to-end) or the dispatch script (one-shot trigger, faster).

### 4a — Test harness (recommended for CI / first-run validation)

```bash
export SOCKERLESS_GH_REPO=e6qu/sockerless
export SOCKERLESS_GL_PROJECT=e6qu/sockerless

go test -v -tags=gcp_runner_live -run TestCell5_GH_Cloudrun -timeout 30m ./tests/runners/gcp-cells
go test -v -tags=gcp_runner_live -run TestCell6_GH_Gcf      -timeout 30m ./tests/runners/gcp-cells
go test -v -tags=gcp_runner_live -run TestCell7_GL_Cloudrun -timeout 30m ./tests/runners/gcp-cells
go test -v -tags=gcp_runner_live -run TestCell8_GL_Gcf      -timeout 30m ./tests/runners/gcp-cells
```

### 4b — Dispatch script (one-shot, captures URLs)

```bash
GITHUB_REPO=e6qu/sockerless GITLAB_REPO=e6qu/sockerless \
  GITLAB_TOKEN=glpat-… \
  ./scripts/dispatch-gcp-cells.sh
```

The script prints the four run URLs as cells get triggered. It does
NOT block waiting for green; watch the URLs externally:

```bash
gh run watch --repo $GITHUB_REPO    # cells 5+6
# cells 7+8: open the printed gitlab pipeline URLs
```

## Step 5 — Capture URLs in STATUS.md

After each cell completes GREEN, append the URL to STATUS.md's
4-cell table (extend the existing AWS table from Phase 110). Each
cell's pipeline body covers (per the cell's `cell-N-{cloudrun,gcf}.yml`):

- `probe-host` — hostname / whoami / id / /etc/os-release / df / mount
- `probe-capabilities` — /proc/self/status caps / cgroup / namespace honesty
- `probe-kernel` — uname -a / /proc/version / /proc/sys/kernel
- `probe-env` — full env dump (filtered)
- `probe-parameters` — getconf -a / ulimit / nproc / memory
- `probe-localhost-peer` — postgres sidecar reachable via localhost (cells 5+6) or `postgres` alias (cells 7+8) — proves Phase 118d pod-overlay net-ns sharing
- `clone-and-compile` — git clone sockerless + go build the
  `simulators/testdata/eval-arithmetic` package
- `run-arithmetic` — exec the binary against five non-trivial
  expressions (`3 + 4 * 2` = 11, `(10 - 3) * 2` = 14, `100 / 5 + 1`
  = 21, `2 * (3 + 4) - 1` = 13, `1.5 + 2.5 * 2` = 6.5)

Cell GREEN gate: every probe section emits non-error output, postgres
is reachable, the Go binary builds, and all five arithmetic
invocations exit 0 with the expected stdout.

## Step 6 — Teardown

```bash
DOCKER_HOST=tcp://127.0.0.1:3375 docker rm -f sockerless-gitlab-runner-cloudrun
DOCKER_HOST=tcp://127.0.0.1:3376 docker rm -f sockerless-gitlab-runner-gcf
# Stop the dispatcher with Ctrl-C; it cleans up runner containers it spawned.
```

Project teardown when the cells are all GREEN:

```bash
gcloud projects delete sockerless-live-46x3zg4imo
```

## Bug-discovery iteration

The first run of each cell will surface bugs (typical: image-resolve
mismatches, sockerless docker-executor `services:` handling, in-image
backend startup races, GCP cold-start latency exceeding the dispatcher's
RUNNER_IDLE_SECONDS, postgres sidecar reachability through the Phase
118d pod overlay). Each bug:

1. Lands in `BUGS.md` with the next free BUG-NNN.
2. Fix on this same `phase-118-faas-pods` branch.
3. Re-run the affected cell until GREEN.
4. Capture the URL in STATUS.md.

The cells close together — Phase 120 closes when all four have
GREEN URLs.
