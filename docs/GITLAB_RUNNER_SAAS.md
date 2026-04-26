# Running `gitlab-runner` against gitlab.com with sockerless

Use the real `gitlab-runner` binary with the **docker executor**, registered against a real `gitlab.com` project or group, with `[runners.docker].host` pointing at a sockerless deployment. For the self-hosted GitLab CE simulator flow used in CI, see [`GITLAB_RUNNER_DOCKER.md`](./GITLAB_RUNNER_DOCKER.md).

## Architecture

```
gitlab.com (cloud)
  │
  │   (long-polled job queue + log stream + artifact upload)
  │
  ▼
gitlab-runner binary (any Linux host)
  │
  │   docker executor:
  │     runners.docker.host = tcp://sockerless:3375
  │
  ▼
sockerless Docker API
  │
  ▼
ECS Fargate (or Lambda) task
```

Each CI job:
1. `gitlab-runner` long-polls `gitlab.com`, receives the job spec.
2. The docker executor creates the **helper container** (`gitlab-runner-helper`) on sockerless, then attaches — the helper stays alive with `tail -f /dev/null` and handles git clone, artifact upload, cache management.
3. The executor creates the **build container** (user-specified `image:` or `ubuntu:latest`), also on sockerless.
4. Each `script:` step is injected via `docker exec` into the build container.
5. Services declared in `services:` are created as additional containers on the same sockerless network.

## Prerequisites

- Sockerless ECS backend reachable from the runner host (see [`ECS_LIVE_SETUP.md`](./ECS_LIVE_SETUP.md)).
- A gitlab.com project / group; you need an admin-level token or a project access token with `ci:write`.
- A Linux host with `gitlab-runner` installed and network access to both the sockerless endpoint and `gitlab.com`.
- **Do not install Docker on the runner host.** The executor uses `runners.docker.host`, which reaches sockerless directly.

## Step 1: Install and register

```bash
# Install from GitLab's apt repo (Debian/Ubuntu):
curl -L "https://packages.gitlab.com/install/repositories/runner/gitlab-runner/script.deb.sh" | sudo bash
sudo apt-get install gitlab-runner

# Register with gitlab.com. Get a registration token from
# https://gitlab.com/<group>/<project>/-/settings/ci_cd → Runners → "New project runner"
sudo gitlab-runner register --non-interactive \
  --url https://gitlab.com/ \
  --registration-token <REG_TOKEN> \
  --name sockerless-runner-1 \
  --tag-list sockerless,ecs \
  --executor docker \
  --docker-image alpine:latest \
  --docker-host tcp://<sockerless-host>:3375 \
  --docker-tls-verify false \
  --docker-pull-policy always \
  --run-untagged
```

(For newer GitLab Runner versions, `--registration-token` is replaced by `--token` obtained from the UI's new Runner API flow.)

## Step 2: Inspect the generated config

`/etc/gitlab-runner/config.toml` should now contain:

```toml
concurrent = 3
check_interval = 3

[[runners]]
  name = "sockerless-runner-1"
  url = "https://gitlab.com/"
  token = "glrt-..."
  executor = "docker"

  [runners.docker]
    host = "tcp://<sockerless-host>:3375"
    image = "alpine:latest"
    tls_verify = false
    disable_cache = true
    privileged = false
    volumes = []
```

For mTLS-protected sockerless, set `tls_verify = true` and mount certs via `[runners.docker].volumes` — see the [gitlab-runner docs](https://docs.gitlab.com/runner/executors/docker.html#use-docker-in-docker-with-tls-enabled) for cert paths.

## Step 3: Smoke-test with a pipeline

Create `.gitlab-ci.yml` in the project:

```yaml
shell-job:
  tags: [sockerless]
  image: alpine:latest
  script:
    - uname -a
    - echo "$CI_COMMIT_SHA"

container-job:
  tags: [sockerless]
  image: node:20
  script:
    - node --version
    - npm --version

service-job:
  tags: [sockerless]
  image: alpine:latest
  services:
    - name: postgres:16
      alias: db
      variables:
        POSTGRES_PASSWORD: test
  before_script:
    - apk add --no-cache postgresql-client
  script:
    - PGPASSWORD=test psql -h db -U postgres -c 'SELECT 1'
```

Push; the pipeline should run all three jobs against sockerless-backed Fargate tasks. The `service-job` exercises per-hostname Cloud Map DNS (P86-003).

## Step 4: Run as a service

The apt package installs `gitlab-runner` as a systemd unit. Verify:

```bash
sudo systemctl status gitlab-runner
sudo gitlab-runner verify   # sanity-checks connectivity
sudo gitlab-runner run --working-directory /home/gitlab-runner --config /etc/gitlab-runner/config.toml
```

## What works, what doesn't

| Feature | Status | Notes |
|---|---|---|
| `image:` per job | works | Fargate task uses the declared image via ECR pull-through. |
| `services:` per job | works (after P86-003) | Each service = its own task on the same sockerless network. |
| `script:` multi-line | works | Injected via `docker exec` in the build container. |
| `before_script:` / `after_script:` | works | Runs sequentially, same exec path. |
| `artifacts:` upload | works | Helper container uploads over HTTPS to gitlab.com. |
| `cache:` | works | Helper uploads cache tarball to gitlab.com's cache store. |
| `docker build` in a job | partial | Use CodeBuild delegation; DinD inside Fargate is unsupported. |
| `parallel:` / `matrix:` | works | Each leg is a separate task. |
| `timeout:` | works | Runner enforces; backend accepts stop. |
| `retry:` | works | Runner retries at job level; sockerless launches a new task. |
| `resource_group:` | works | Job-level serialization handled by gitlab.com; no backend change. |

## Troubleshooting

- **"error during connect: Get http://.../v1.44/containers/json"** — runner can't reach sockerless. `curl -sf http://<sockerless-host>:3375/_ping` from the runner host.
- **Helper container fails to resolve `files.gitlab.com`** — the Fargate subnet needs outbound internet for the helper to upload artifacts. Check the NAT gateway in the Terraform.
- **Services unreachable (`psql -h db` fails)** — verify Cloud Map namespace exists for the sockerless network and the task's security group allows task-to-task traffic. See `docs/ECS_SERVICES_DESIGN.md`.
- **Stuck at `Running with gitlab-runner 18.x.x`** — means the runner can't pull the helper image. sockerless may be refusing the ECR pull; check the sockerless logs for `pull-through cache` errors.

## Live-cloud validation

This doc describes the target flow. The three shapes need validation against real gitlab.com + live ECS — see [`manual-tests/02-aws-runbook.md`](../manual-tests/02-aws-runbook.md) for the canonical sweep.
