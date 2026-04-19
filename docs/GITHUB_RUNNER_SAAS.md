# Running the official GitHub Actions runner against github.com with sockerless

This guide is for the **real** `actions/runner` binary registered with a real `github.com` repo or organization, with `DOCKER_HOST` pointing at a sockerless deployment. For the `act`-based simulator flow used in CI, see [`GITHUB_RUNNER.md`](./GITHUB_RUNNER.md).

## Architecture

```
github.com (cloud)
  │
  │   (long-polled job queue + artifact + log uploads)
  │
  ▼
actions/runner binary (on any machine with Docker client libraries)
  │
  │   DOCKER_HOST=tcp://sockerless:2375
  │
  ▼
sockerless Docker API
  │
  ▼
ECS Fargate (or Lambda) task — your actual job
```

Jobs flow:
1. A workflow triggers on `github.com`. GitHub enqueues the job.
2. The `actions/runner` binary long-polls GitHub, picks up the job, receives the manifest.
3. For each step, the runner creates a container via the Docker API (which reaches sockerless). sockerless launches the task on ECS / Lambda.
4. The runner uses `docker exec` to inject per-step scripts, collects stdout/stderr, and uploads the results back to `github.com`.

## Prerequisites

- A sockerless ECS backend reachable from the machine that will run the `actions/runner` binary. Follow [`ECS_LIVE_SETUP.md`](./ECS_LIVE_SETUP.md) first.
- A github.com repository or organization you can register a self-hosted runner against.
- A Linux host (runner officially supports x64 and arm64 Linux). The host needs the Docker client libraries and network access to both the sockerless endpoint and `api.github.com`.
- A GitHub personal access token (fine-grained) with `actions:write` scope — or be an org admin.

## Step 1: Register the runner

Go to **Repository → Settings → Actions → Runners → New self-hosted runner**. GitHub generates a one-shot registration token and shows copy-pasteable commands. Run them on the host that will poll GitHub. They boil down to:

```bash
mkdir actions-runner && cd actions-runner
curl -o actions-runner-linux-x64.tar.gz -L \
  https://github.com/actions/runner/releases/download/v2.321.0/actions-runner-linux-x64-2.321.0.tar.gz
tar xzf actions-runner-linux-x64.tar.gz
./config.sh --url https://github.com/<owner>/<repo> --token <REG_TOKEN> \
  --name sockerless-runner-1 --labels sockerless,ecs --unattended
```

**Do not** install Docker on this host. The runner uses Docker via the `DOCKER_HOST` env var; you want it to reach *sockerless*, not a local daemon.

## Step 2: Point `DOCKER_HOST` at sockerless

Create `actions-runner/.env` (read by the runner on startup):

```
DOCKER_HOST=tcp://<sockerless-host>:2375
DOCKER_API_VERSION=1.44
```

If your sockerless endpoint uses mTLS:

```
DOCKER_HOST=tcp://<sockerless-host>:2376
DOCKER_TLS_VERIFY=1
DOCKER_CERT_PATH=/etc/sockerless/client-certs
```

## Step 3: Run the runner as a service

Install the provided systemd service:

```bash
sudo ./svc.sh install
sudo ./svc.sh start
sudo systemctl status actions.runner.<owner>-<repo>.sockerless-runner-1.service
```

`svc.sh install` reads `.env` and passes the environment through to the runner's systemd unit.

## Step 4: Test with a workflow

Add `.github/workflows/sockerless-smoke.yml` in your repo:

```yaml
name: sockerless-smoke
on: push
jobs:
  shell:
    runs-on: [self-hosted, sockerless]
    steps:
      - run: uname -a
      - run: echo "$GITHUB_SHA"

  containerized:
    runs-on: [self-hosted, sockerless]
    container: node:20
    steps:
      - run: node --version

  with-services:
    runs-on: [self-hosted, sockerless]
    container: alpine:latest
    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_PASSWORD: test
        ports: [5432:5432]
    steps:
      - run: apk add --no-cache postgresql-client
      - run: PGPASSWORD=test psql -h postgres -U postgres -c 'SELECT 1'
```

Push a commit. The runner should pick up all three jobs. The `with-services` job validates that cross-container DNS (per-hostname Cloud Map services, added in P86-003) works end-to-end.

## What works, what doesn't

| Feature | Status | Notes |
|---|---|---|
| `run:` shell jobs | works | Straight `docker exec`. |
| `container:` directive | works | Fargate launches the directed image directly. |
| `services:` directive | works (after P86-003) | Each service is its own Fargate task; resolved via Cloud Map. |
| Matrix jobs | works | Each matrix leg = one Fargate task. |
| Container actions (`uses: docker://…`) | works | Treated as a one-shot container. |
| `actions/checkout@v4` | works | Same as on any runner. Code lands in the Fargate task's ephemeral disk. |
| JavaScript actions (`uses: org/repo@vN`) | works | Downloaded inside the Fargate task via Node.js. |
| `docker build` in a job | partial | Requires the CodeBuild delegation (`SOCKERLESS_AWS_CODEBUILD_PROJECT`). DinD inside Fargate is not supported. |
| Artifact upload/download | works | Artifacts upload to github.com over HTTPS from the job container. |
| GITHUB_TOKEN | works | GitHub injects as env var; the runner forwards. |
| Cache (`actions/cache@v4`) | works | Cache round-trips over HTTPS to GitHub's storage. |

## Troubleshooting

- **"unable to connect to docker daemon"** — runner can't reach sockerless. Verify `DOCKER_HOST` from the runner host: `curl -sf http://<sockerless-host>:2375/_ping`.
- **Services resolve to wrong IP or fail DNS** — see `docs/ECS_SERVICES_DESIGN.md`. Private DNS on the VPC must be enabled.
- **Jobs take 90+ seconds to start** — that's ECS cold-start for a new task definition. Warm pools aren't implemented; consider a runner-level pool or longer-running containers.
- **`docker login` required** — runner steps that pull from private registries need credentials. Set them as GitHub secrets and pass them through a `docker login` step; sockerless forwards to ECR / wherever.

## Phase 86 AWS track

This doc describes the target flow. The three shapes (shell, `container:`, `services:`) have not yet been validated against a real github.com + live ECS deployment — that's blocked on the AWS-credentials step of the Phase 86 roadmap. When that work lands, this doc will gain "verified on ECS with runner vX.Y.Z" callouts and the capability matrix will fill in the `ecs (live)` column.
