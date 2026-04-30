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

## Runner hurdles — full catalog (real, no workarounds)

This section catalogs every concrete pitfall the GitHub Actions runner and GitLab Runner have inflicted on the sockerless integration, with the bug ID where each was fixed (or the open ticket where it's pending). The goal is twofold: (1) speed up debugging when a new failure shape echoes a past one, (2) ground future runner-related design choices in observed behaviour rather than assumed behaviour. Project rule: every hurdle below is a real fix — no fakes, no fallbacks, no silent shims. Hard hurdles get staged across phases; we never give up.

When extending this list, anchor each entry to the right rule in [`specs/CLOUD_RESOURCE_MAPPING.md`](../specs/CLOUD_RESOURCE_MAPPING.md) so the cloud-side primitive map stays the authoritative source for "how does sockerless model this on cloud X?".

### GitHub Actions Runner (cells 1 + 2)

Architectural shape: the runner *is* the workspace. For `container:` jobs it does `docker create -v /home/runner/_work:/__w …` — host bind mounts that assume a shared filesystem with the spawned container. Cells 1 + 2 require both a topology change (runner-as-ECS-task / runner-as-Lambda) and sockerless-side bind-mount → EFS translation.

| # | Hurdle | Resolution | Bug |
|---|---|---|---|
| GH-1 | Bind-mounts `/home/runner/_work`, `/home/runner/externals`, sub-paths (`_temp`, `_actions`, `_tool`, `_temp/_github_home`, `_temp/_github_workflow`) to the job container | `Config.SharedVolumes` translation in ECS / Lambda backends; sub-paths under a configured root are dropped (parent EFS access point already covers them) | BUG-850 (ECS), BUG-861 (Lambda externals) |
| GH-2 | Bind-mount `/var/run/docker.sock` for nested-docker support | Silently dropped on cloud backends; nested `docker run` would need a separate dispatch path back to a sockerless instance, out of scope | BUG-850 |
| GH-3 | `docker exec` issued immediately after task RUNNING, before SSM `ExecuteCommandAgent` is up (5-30 s lag on Fargate) | `cloudExecStart` waits for `ManagedAgents[ExecuteCommandAgent].LastStatus == RUNNING` before invoking ECS `ExecuteCommand`; same wait reused from `RunCommandViaSSM` | BUG-853 |
| GH-4 | Per-job docker network created via `docker network create --label <jobnum> github_network_<hex>` then sub-task attached | ECS overrides `s.Drivers.Network` with `SyntheticNetworkDriver` (metadata-only, name historical) — Linux netns is the wrong abstraction for cloud, where networks map to VPC SG + Cloud Map | BUG-851 |
| GH-5 | Sub-task EFS mount blocked by per-network SG (peer-isolation SG isn't in the EFS mount target's allow-list) | Sub-tasks include both per-network SG *and* operator default SG; per-network SG still gates peer reachability, default SG carries shared-infra access | BUG-852 |
| GH-6 | `workflow_dispatch` only fires on workflows present on the default branch | Harness commits the per-cell YAML to a throwaway branch at the path of an *existing* main-branch workflow file (e.g. `hello-ecs.yml`'s slot) and dispatches with `ref=<branch>` so the throwaway-branch content runs | (n/a — harness pattern) |
| GH-7 | `actions/runner` asset URL uses `osx`, not `darwin`; recent versions only on 2.319+ | Pinned to 2.334.0; URL constructed with `osx` for macOS | BUG-847 |
| GH-8 | `--add-host host-gateway` syntax fails on Podman 5.x (which already provides `host.docker.internal` natively) | Drop the flag entirely; rely on Podman's built-in alias | BUG-849 |
| GH-9 | `docker info` reported hardcoded `amd64` regardless of cloud backend's actual architecture | Architecture surfaced from required `SOCKERLESS_ECS_CPU_ARCHITECTURE` / `SOCKERLESS_LAMBDA_ARCHITECTURE` env vars; Config.Validate refuses empty | BUG-848 |
| GH-10 | Lambda image filesystem is read-only outside `/tmp`; runner can't `config.sh` into `/opt/runner/` | Bootstrap stages `actions/runner` to `/tmp/runner-state/` on first invocation; `_work` symlinks to `/mnt/runner-workspace` (EFS) | BUG-856 partial; corrected in BUG-862 |
| GH-11 | Lambda execution environments reuse across invocations — stale `.runner` / `.credentials` from prior run break re-registration | Bootstrap `rm -f .runner .credentials .credentials_rsaparams` before each `config.sh` | BUG-856 |
| GH-12 | Default Lambda `/tmp` is 512 MB; actions/runner externals + working tree exceed that | Terraform sets `ephemeral_storage = 5GB` on the runner-Lambda | BUG-856 |
| GH-13 | Lambda's single `FileSystemConfig` constraint forces all bind-mount roots into one access point | `SOCKERLESS_LAMBDA_SHARED_VOLUMES` carries multiple `name=path=<apid>` entries pointing at the *same* access-point root; sockerless's per-volume access-point lookup short-circuits on the configured shared volumes | BUG-861 |
| GH-14 | Backend ↔ host primitive mismatch: runner-Lambda baked `sockerless-backend-ecs` and dispatched sub-tasks via `ecs.RunTask` to Fargate ("avoid Lambda-in-Lambda recursion") | **CRITICAL**. Runner-Lambda now bakes `sockerless-backend-lambda`; sub-tasks are fresh image-mode container Lambdas sharing the workspace EFS access point. Codified as universal rule #9 in `specs/CLOUD_RESOURCE_MAPPING.md` | BUG-862 |
| GH-15 | Lambda runtime has no docker daemon → in-Lambda backend can't `docker build` for sub-task image-inject | CodeBuild + S3 build-context provisioned via `terraform/modules/lambda/codebuild.tf`; `awscommon.CodeBuildService` already supports this path; runner-Lambda gets `SOCKERLESS_CODEBUILD_PROJECT` + `SOCKERLESS_BUILD_BUCKET` env vars | BUG-862 |

### GitLab Runner (cells 3 + 4)

Architectural shape: GitLab Runner is a *dispatcher*. The master polls GitLab and uses the docker executor's `docker create + docker exec + docker attach` to spawn the job container. The master is just a docker client — it never bind-mounts its own filesystem; it can run anywhere with `--docker-host` pointing at sockerless. Cells 3 + 4 require zero topology change but exercise different sockerless code paths than the GitHub-runner shape.

| # | Hurdle | Resolution | Bug |
|---|---|---|---|
| GL-1 | Helper image referenced by `sha256:` digest only (no name component) | `resolveImageURI` detects digest-only refs and (a) tries the local image Store for a canonical RepoTag, or (b) surfaces a clear error pointing at `name@sha256:...`; misrouting via Docker Hub is no longer possible | BUG-854 |
| GL-2 | EFS access-point `rootDirectory.path` capped at 100 chars; volume names from `runner-<id>-project-<id>-concurrent-<n>-<hex>-cache-<sha>` exceed that | Sanitised path > 100 chars falls back to `/sockerless/v/<sha256(volname)[:16]>` — deterministic, short, collision-resistant | BUG-855 |
| GL-3 | Helper image pulled from `registry.gitlab.com/gitlab-org/gitlab-runner/gitlab-runner-helper:<tag>`; ECR pull-through cache can't proxy without Secrets Manager auth (project rule: no creds on disk) | Pre-push the helper image to live ECR (`sockerless-live:gitlab-runner-helper-amd64`) at harness setup; configure runner with `--docker-helper-image=<ecr-uri>` | BUG-857 |
| GL-4 | Sockerless's `getRegistryToken` parses ECR's `Basic realm=…` Www-Authenticate header as Bearer-flow; ECR uses Basic auth directly, surfacing as "no realm in Www-Authenticate header" | `isBasicAuthRegistry(host)` short-circuits to `basic:<token>` sentinel for ECR-shaped hosts; `ImageManager.Pull` passes the cloud auth token through `ecrBasicCredential(...)` | BUG-857 |
| GL-5 | Runner does `docker create + start + stop + start` (re-runs the predefined helper container across script stages); second `start` returned 404 because `ContainerStart` only checked PendingCreates, which had been cleared | `ContainerStart` falls back to `ResolveContainerAuto` (PendingCreates → CloudState → Store) and restores the resolved container to PendingCreates so the rest of the flow proceeds; PendingCreates preserved through `waitForTaskRunning` | BUG-858 |
| GL-6 | User-script container delivery: runner pipes the script bytes through the hijacked `docker attach` connection's stdin; sockerless's typed `core.NewCloudLogsAttachDriver` was read-only and discarded the bytes; user `sh` exits 1 in <1 s with no stdout | New typed `ecsStdinAttachDriver` captures stdin into a per-cycle `stdinPipe`; `ContainerStart`, when ECSState records `OpenStdin`, defers `RunTask` via a goroutine that waits for stdin EOF then bakes the buffered bytes into the task definition's `Entrypoint=[sh,-c]` + `Cmd=[<script>]`. Lambda backend mirrors with `lambdaStdinAttachDriver` baking stdin into `lambda.Invoke` Payload (the bootstrap pipes Payload to the user entrypoint as stdin, so `Cmd=[sh]` runs the script). | BUG-859 (ECS) + BUG-860 (Lambda) |
| GL-7 | Per-cycle pipe lifecycle: gitlab-runner reuses the same container ID across script steps; each cycle needs a fresh stdin buffer; CloudState's `containerFromTask` synthesises Config without OpenStdin/Binds/VolumesFrom, so the flag would be lost between cycles | `ECSState.OpenStdin` (and `LambdaState.OpenStdin`) persist the flag across cycles; `launchAfterStdin` does not delete PendingCreates for stdin containers (CloudState alone is insufficient — its synthesised Config lacks OpenStdin/Binds); attach driver's get-or-create pattern (`LoadOrStore`) gives each cycle a fresh pipe after the previous one was consumed | BUG-859 / BUG-860 |
| GL-8 | Race window: gitlab-runner does `attach (hijack 101) → start → stream stdin → close stdin → wait`. Between sockerless's attach handler sending 101 and entering the typed `Attach()` (which calls `pipe.Open()`), there's a tiny window where `start` can arrive first | `ContainerStart`, when `ECSState.OpenStdin` is true, briefly polls (up to 2 s, 20 ms intervals) for an open pipe before declaring no-attach; if `pipe == nil` after the poll, surfaces `InvalidParameterError` rather than running a phantom task | BUG-859 |
| GL-9 | `gitlab-runner-helper` `setVolumePermissions` step expects `chown -R` on a writable volume root that's actually an EFS access-point root | The shortened EFS path from BUG-855 is the access-point root; sockerless's volume create stamps the access point with the right owner/perms via `EFSManager.CreateAccessPoint` so chown is a no-op | (continuous; covered by BUG-855 + EFSManager) |
| GL-10 | Token mint: PAT scope confusion — `personal_access_tokens/self` returns 401 even with valid token if the token is a project-token rather than user-PAT | Validate via `GET /user` instead of `/personal_access_tokens/self`; mint runner via `POST /api/v4/user/runners` (modern API, project_type) which requires `api` + `create_runner` scopes | (n/a — harness; verified 2026-04-29) |
| GL-11 | Project-scoped runner creation needs `project_id` (numeric), not `path_with_namespace` | Harness pre-resolves the project: `GET /api/v4/projects/<urlencoded>` → `id` → use in `POST /user/runners` | (n/a — harness) |

### Predicted next hurdles (open / staged)

When the operator runs the unblock sequence in [DO_NEXT.md](../DO_NEXT.md), expect these. They are NOT bugs yet — they're called out so we recognise them quickly. Each will be filed as a real bug + real fix the moment it manifests.

| # | Predicted hurdle | Why we expect it | Fix shape |
|---|---|---|---|
| P-1 | Sub-task Lambda creation hits ENI cap when `container:` workflows run concurrently | Lambda VPC config consumes ENIs; default account quota is 250 concurrent ENIs per region | Either (a) cap concurrent sub-tasks via a sockerless-side semaphore, or (b) request a quota raise + document the limit. Cloud resource mapping rule: lambda backend is single-AZ-per-function for ENI predictability. |
| P-2 | First image-inject build via CodeBuild fails because the buildspec assumes the source tarball includes a Dockerfile, but image-inject generates the Dockerfile dynamically and uploads it inside the tarball | Existing `awscommon.CodeBuildService.Build` already includes the Dockerfile in the upload tar; need to verify the inline buildspec in `terraform/modules/lambda/codebuild.tf` matches what `Build()` actually uploads | Cross-check: trace `Build()` → context tar → buildspec; align if mismatched |
| P-3 | gitlab-runner helper image's `chmod 600 /scripts/...` requirement clashes with EFS POSIX permissions on the shared access point | EFS access points enforce POSIX UID/GID at mount time | Set the access-point's `posix_user` to match the runner's UID (1001 by default), and `creation_info.permissions = 0700` |
| P-4 | Lambda function name length / character set: gitlab-runner names containers `runner-<id>-project-<id>-concurrent-<n>-<hex>-…`; Lambda function names cap at 64 chars | When gitlab-runner-on-laptop spawns sub-task Lambdas, sockerless will fail validation on long names | Sockerless lambda backend hashes the docker container name → `skls-<sha256[:12]>` and tags the function with the original name; symmetric to ECR's volume-name handling in BUG-855 |
| P-5 | Runner-Lambda exhausts its 5 GB ephemeral disk when a `container:` workflow downloads a large image (multi-GB user image) | image_inject pulls the user image, layers + agent injection, then pushes to ECR — all goes through `/tmp` | Image build delegated to CodeBuild (which has 50 GB disk); runner-Lambda only uploads the build context (Dockerfile + small binaries), so the multi-GB image bytes never touch its `/tmp`. Already in place via the CodeBuild path; verify under load. |
| P-6 | `gitlab-runner --docker-host tcp://localhost:3376` (cell 4 laptop) fails because sockerless's lambda backend rejects bind-mounts that gitlab-runner uses for the helper container's `/cache` and `/builds` | gitlab-runner expects a real docker daemon that allows arbitrary bind mounts; sockerless lambda only accepts named volumes (translated to EFS access points) | (already handled) sockerless's volume code creates ECR access points on demand; gitlab-runner's `docker volume create` flows through; named-volume references are accepted as-is |
| P-7 | Cell 4 sub-task Lambdas hit the 1000-concurrent-execution account default cap when a job spawns many `container:` steps in parallel | Lambda's per-region concurrency limit | Sockerless surfaces `lambda.TooManyRequestsException` clearly; operators raise the quota or use reserved-concurrency on the runner-Lambda function |
| P-8 | gitlab-runner's `docker pull` issues `--platform` flag on multi-arch images; sockerless's image-resolve must propagate platform to the cloud-side dispatch (Lambda function `Architectures: ["x86_64"]` vs the user's image) | Cross-arch dispatch is a known gap in the cloud resource mapping doc | If image platform doesn't match the cluster/function arch, surface `InvalidParameterError` with a clear "image is `<arch>`, cluster expects `<arch>`" message; auto-translate not in scope |

### Phase staging — when a hurdle is too complex for one round

Per the project rule "if very hard we can stage the fixes and refactorings across several phases": each open or predicted hurdle above lists its **fix shape**, not necessarily the immediate fix. When a fix needs more than ~1 day or crosses module boundaries, it gets a dedicated phase entry in [PLAN.md](../PLAN.md) with sub-tasks. Examples already staged:

- **Phase 110b** — runner integration completion (where most of GH-* and GL-* live).
- **Phase 111** — workload identity for runner jobs (cloud-API access from inside `container:` steps).
- **Phase 113** — production-shape `github-runner-dispatcher` (webhook ingress, GitHub App install, multi-repo, deployable).

If a hurdle from this catalog stays open across a phase boundary, it gets a sub-task entry in the new phase, and the catalog row's "Resolution" column links to it. The catalog itself never declares a hurdle "deferred indefinitely" — every entry has either a closed bug ID or an open bug ID with an active phase.
