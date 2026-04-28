# Sockerless — Status

**104 phases closed (Phase 109 closed in PR #121, merged 2026-04-27). 849 bugs tracked — 849 fixed, 0 open. 1 false positive.** Active branch: **`phase-110-runner-integration`** (Phase 110a — GitLab cells + `github-runner-dispatcher` skeleton; see [PLAN.md § Phase 110](PLAN.md), [docs/RUNNERS.md](docs/RUNNERS.md)). PRs #117 / #118 / #119 / #120 / #121 merged. Mirror `origin-gitlab/main` in sync with `origin/main` as of 2026-04-27.

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

- **Phase 110a (active branch, PR #122)** — Cells 3 + 4 (GitLab × ECS, GitLab × Lambda) against live infra + `github-runner-dispatcher` top-level Go module skeleton. No new sockerless code needed for the GitLab cells (gitlab-runner master is local; docker executor with `--docker-host tcp://localhost:3375` / `:3376` exercises the live backends). Dispatcher is sockerless-agnostic — pure Docker SDK / CLI client.
- **Phase 110b (queued)** — Cells 1 + 2 (GitHub × ECS, GitHub × Lambda). Headline deliverable: **bind-mount → EFS translation** in sockerless ECS + Lambda backends, so GitHub Actions' `container:` directive works end-to-end with the runner running as an ECS task / Lambda invocation. Pre-registered `sockerless-runner` ECS task definition in Terraform (multi-container: runner + sockerless sidecar; EFS-backed `/home/runner/_work`). Runner image pushed to a new `sockerless-runner` ECR repo with `LABEL com.sockerless.ecs.task-definition-family=sockerless-runner`.
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

- **AWS creds: ACTIVE** for the Phase 110a session (2026-04-28).
- **Live AWS infra: UP in eu-west-1** — re-provisioned 2026-04-28 after the previous-session teardown. ECS (35 resources: VPC + NAT + ECS cluster + EFS + ECR + IAM + Cloud Map) and Lambda (8 resources: IAM execution role + ECR repo + log group). NAT Gateway runs ~$0.045/hr — tear down via `terragrunt destroy` from `terraform/environments/{ecs,lambda}/live` when the session ends.
- **Sockerless daemons: RUNNING.** ECS backend on `tcp://localhost:3375` (cluster `sockerless-live`); Lambda backend on `tcp://localhost:3376` (eu-west-1, sharing the ECS VPC subnets per BUG-845). Required env vars: `SOCKERLESS_ECS_CPU_ARCHITECTURE=X86_64` / `SOCKERLESS_LAMBDA_ARCHITECTURE=x86_64` (BUG-848 — no defaults).
- **Smoke verified** — `DOCKER_HOST=tcp://localhost:3375 docker run --rm alpine:latest echo hi` exits 0 from a Fargate task; verifies the BUG-846 AWS-Public-Gallery routing for Docker Hub library refs.
- **Podman machine** — running (applehv VM, user-mode networking, 4 CPU / 10 GiB / 100 GiB). Used for local-Podman dispatcher testing in 110a; not used for cell 3+4 (gitlab-runner master is a darwin-native binary).
- **PAT keychain entries** — `gh` (GitHub) keychain-backed; GitLab PAT in `security(1)` keychain entry `sockerless-gl-pat`.
