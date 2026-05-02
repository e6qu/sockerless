# Sockerless — Status

**104 phases closed. Phase 110 closed — all 4 cells GREEN. Phase 118 + Phase 120 stacked on `phase-118-faas-pods` branch (PR #123): sub-118a (cloudrun manual sweep BUG-877..886), sub-118b (Lambda pool reuse), sub-118d-gcf + sub-118d-lambda (FaaS pod overlay for both gcf and lambda — bootstrap supervisor + pod-aware image_inject + ContainerStart materialize + cloud_state pod row emission, all unit-tested), Phase 120 docker-executor cells code complete (4 runner image Dockerfiles + workflow YAML × 4 + harness + operator runbook). Live-cloud cell URLs pending operator runs. Phase 119 (k8s shim) was explored and **discarded** per operator direction — cells use docker executor + the existing `github-runner-dispatcher` (which compensates for github-runner not having a "master"); no k8s, no GKE, no ARC. Sub-118c (AZF) deferred until cells GREEN.**
- Cell 1 GH×ECS: https://github.com/e6qu/sockerless/actions/runs/25075259911
- Cell 2 GH×Lambda: https://github.com/e6qu/sockerless/actions/runs/25113565115 — 7 architectural walls (BUG-862, 869, 870, 871, 872, 873, 874).
- Cell 3 GL×ECS (2026-04-29): https://gitlab.com/e6qu/sockerless/-/pipelines/2489246177 — job 14148678472 ran `echo "hello from sockerless ecs"` + `env | sort` in 270.8 s on 9 sequential per-stage Fargate tasks. Closed in commit `aa2419a`.
- **Cell 4 GL×Lambda (NEW 2026-04-30): https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943** (status: success) — job 14156021860 ran `echo "hello from sockerless lambda"` + `date -u` in 260.5 s. Closed seven Lambda-primitive bugs across the session culminating in two final fixes:
  1. **stdinPipe/start race**: `/start`'s Invoke goroutine did a one-shot `stdinPipes.Load()`, missing the standard Docker SDK ordering (/create → /start → /attach). Fix: poll `stdinPipes` for up to 5 s inside the goroutine before deciding to skip stdin. Without this, OpenStdin containers Invoked with empty payload `{}`, which the bootstrap piped to bash, producing `/bin/bash: line 1: {}: command not found` and Lambda `Unhandled` errors on every predefined-helper container.
  2. **`docker.io/library/<name>` rejection**: `image_resolve.go` rejected `docker.io/library/alpine:latest` as "Docker Hub user/org image" because `repo` retained the `library/` prefix and matched `strings.Contains(repo, "/")`. Fix: strip `library/` prefix before the user/org check.
  3. Plus diagnostic logging — `LogType=Tail` on every `lambda.Invoke` so the function's last 4 KB of stderr ride back inline; payload preview + log_tail emitted on `FunctionError`. The `{}: command not found` line that pinpointed the race came directly from this tail.

See [PLAN.md](PLAN.md) (roadmap), [BUGS.md](BUGS.md) (bug log), [WHAT_WE_DID.md](WHAT_WE_DID.md) (narrative), [DO_NEXT.md](DO_NEXT.md) (resume pointer), [docs/RUNNERS.md](docs/RUNNERS.md) (runner wiring).

## Branch state

- **`main`** — synced with `origin/main` at PR #121 merge.
- **`origin-gitlab/main`** — mirror, pre-push hooks now mirror-aware so `git push origin-gitlab main` is a clean fast-forward.
- **`phase-110-runner-integration`** — active; baseline commit `f5c1ab7` shipped `docs/RUNNERS.md` + the mirror-aware hooks.

## Recent merges

| PR | Summary |
|---|---|
| #121 | Phase 109 strict cloud-API fidelity sweep — 19 audit items: AWS Lambda VpcConfig + region/account scoping + Secrets Manager + SSM Parameter Store + KMS + DynamoDB; GCP `compute.firewalls` + `compute.routers`/Cloud NAT + `iam.generateAccessToken` + operations endpoint persistence; Azure IMDS token endpoint + Blob Container ARM CRUD + NSG priority+direction validation + Private DNS AAAA/CNAME/MX/PTR/SRV/TXT records + NAT Gateways + Route Tables + Container Apps/Jobs Azure-AsyncOperation polling + Key Vault ARM+data plane + ARM `SystemData.createdAt` preservation. Test-fixtures no-fakes audit clean. |
| #120 | Audit + Phase 104 framework migration + cloud-native typed drivers + Phase 105 waves 1-3 + Phase 108 closed + Phase 106/107 harness scaffolding + ImageRef domain type + Phase 109 first-round (BUG-836..844: real ECS lifecycle, real SSM AgentMessage protocol, real subnet-CIDR IP allocation, real Azure per-site hostnames, real kill signal routing) + repo-wide code/doc cleanup. |
| #119 | Post-PR-#118 state-doc refresh — Phase 104 promoted to active. |
| #118 | Round-8 + Round-9 live-AWS sweep — 30 bugs (BUG-786..819), per-cloud terragrunt sweep parity. |
| #117 | Round-7 live-AWS sweep — 16 bugs (BUG-770..785). |

Older PRs (#112–#115) — sim parity, real volumes, FaaS invocation tracking, reverse-agent ops, Phase 91/86/87/88 closures. Detail in [WHAT_WE_DID.md](WHAT_WE_DID.md) and per-bug entries in [BUGS.md](BUGS.md).

## Open work

- **Phase 118 — live-GCP track + cross-FaaS pool/cache + pod design.**
  - cloudrun (Cloud Run Jobs) bugs BUG-877..885 fixed live.
  - gcf (Cloud Run Functions Gen2) BUG-884 fixed live: stub-Buildpacks source + post-create `Run.Services.UpdateService` image swap; content-addressed AR cache; stateless reuse pool keyed on `sockerless_overlay_hash` + `sockerless_allocation` labels; idtoken-authenticated invocation (no `allUsers` workaround); fail-loud on user-credential ADC. Tested live end-to-end (`docker run --rm alpine echo` returns clean stdout via gcf).
  - **Open: BUG-886** — `core.StreamCloudLogs` loses entries from a fast-burst-then-exit container (bundle-O case). Cursor refactor to `>=lastTS` + seen-set dedup didn't catch it. Likely iterator pagination or write-side blocking issue. Next fix.
  - **Queued in this phase**: Lambda pool reuse (overlay caching exists; needs label-based claim/release on top); AZF live track (greenfield — needs Azure infra setup from operator); FaaS pod implementation (spec done with honest namespace-isolation caveats — net+IPC+UTS shared per podman default, mount+PID isolated per podman default but degraded-shared on FaaS due to no `CAP_SYS_ADMIN`).
- **Phase 110 — closed 2026-04-30.** All 4 cells GREEN.

### 4-cell matrix

| Cell | State | URL |
|---|---|---|
| 1 GH × ECS | ✅ GREEN | https://github.com/e6qu/sockerless/actions/runs/25075259911 |
| 2 GH × Lambda | ✅ GREEN | https://github.com/e6qu/sockerless/actions/runs/25113565115 |
| 3 GL × ECS | ✅ GREEN | https://gitlab.com/e6qu/sockerless/-/pipelines/2489246177 |
| 4 GL × Lambda | ✅ GREEN | https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943 |

Detailed unblock plans per cell live in [PLAN.md § Phase 110 — paths forward to GREEN](PLAN.md). Per-bug closure paths in [BUGS.md](BUGS.md). Resume command + sequence in [DO_NEXT.md](DO_NEXT.md). Full runner hurdle catalog (closed + predicted) in [docs/RUNNERS.md § Runner hurdles](docs/RUNNERS.md). Lambda volume primitive translation in [specs/CLOUD_RESOURCE_MAPPING.md § Lambda bind-mount translation](specs/CLOUD_RESOURCE_MAPPING.md).
- **Phase 110a — github-runner-dispatcher skeleton: shipped** at commit `ba797b6`. Top-level Go module, sockerless-agnostic (only stdlib + BurntSushi/toml). State recovery via container labels (`sockerless.dispatcher.{job_id,runner_name,managed_by}`); GC sweep every 2 min reaps exited containers + offline GitHub runners; graceful shutdown drains in-flight work bounded to 30 s.
- **Phase 113 (queued)** — production-shape `github-runner-dispatcher` (webhook ingress, GitHub App install, multi-repo, deployable). See [PLAN.md § Phase 113].
- **Phase 104 wrapper-removal pass** — gated on docker getting typed cloud-native drivers OR accepting wrappers as permanent. Once decided: drop unused `WrapLegacyXxx` / `LegacyXxxFn` scaffolding and shrink `api.Backend` correspondingly. Coordinated landing.
- **Phase 104 interface tightening** — typed `Signal` enum, `ResolveImageReg(ImageRef)` helper, structured `Stats` struct.
- **Phase 105 wave 4** (lower priority) — events stream, exec start hijack shape, container CRUD beyond list.
- **Phase 68** (Multi-Tenant Backend Pools) — P68-001 done; 9 sub-tasks remain. Phase 110's 4-cell setup uses Phase-68-v1 (one daemon per backend); Phase 68-v2 collapses to label-based dispatch on a single daemon.

## Test counts (head of `main`)

| Category | Count |
|---|---|
| Core unit | 312 |
| Cloud SDK/CLI | AWS 68+, GCP 64+, Azure 57+ |
| Sim-backend integration | 77 (parity matrix at 77/77 ✓) |
| Libpod golden-shape | 8 |
| External-suite replays | 12 (act + gitlab-ci-local) |

## Operational state

- **AWS creds: ACTIVE** as of 2026-04-30 (root `729079515331`, eu-west-1) — refresh via `source aws.sh`.
- **Live AWS infra: UP in eu-west-1** — provisioned 2026-04-28; still running. ECS + Lambda live envs as above. NAT Gateway runs ~$0.045/hr — tear down via `terragrunt destroy` from `terraform/environments/{ecs,lambda}/live` when the session ends.
- **Sockerless daemons:** Lambda backend running on the cell-4-fix binary at `/tmp/sockerless-backend-lambda` (verified live with the GREEN cell 4 pipeline). ECS daemon was last seen on the BUG-859/860 fix binary; cells 3+4 verified GREEN against current code.
- **Smoke verified** — `DOCKER_HOST=tcp://localhost:3375 docker run --rm alpine:latest echo hi` exits 0 from a Fargate task; verifies the BUG-846 AWS-Public-Gallery routing for Docker Hub library refs.
- **Podman machine** — running (applehv VM, user-mode networking, 4 CPU / 10 GiB / 100 GiB). Used for local-Podman dispatcher testing in 110a; not used for cell 3+4 (gitlab-runner master is a darwin-native binary).
- **PAT keychain entries** — `gh` (GitHub) keychain-backed; GitLab PAT in `security(1)` keychain entry `sockerless-gl-pat`.
