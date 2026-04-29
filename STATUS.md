# Sockerless — Status

**104 phases closed (Phase 109 closed in PR #121, merged 2026-04-27). 874 bugs tracked — 872 fixed, 2 open (BUG-868 gitlab-runner lifecycle; BUG-874 Lambda start/exec lifecycle, partially closed by Phase 116). 1 false positive.** **PR #122 CI GREEN as of commit `88aca1e`** (10/10 jobs); latest commit `455c019` Phase 116 exec-via-Invoke shipping. Active branch: **`phase-110-runner-integration`**. Cell 1 GH×ECS GREEN. Cell 2 GH×Lambda cleared 6 architectural walls (BUG-862, 869, 870, 871, 872, 873) and Phase 116 partial (synchronous-Active ContainerStart + Path B exec-via-Invoke + build-time symlink baking + workdir off Lambda config) — one remaining diagnostic wall (exec returns 126 without sub-task CloudWatch entries) for next iteration. Cells 3/4 GitLab inherit BUG-868 + remaining BUG-874. Phase 115 closed; Phase 116 substantively shipped; Phase 114 queued.

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

- **Phase 110b (cell 2 advancing)** — Cell 1 (GH×ECS) GREEN at https://github.com/e6qu/sockerless/actions/runs/25075259911. Cell 2 (GH×Lambda) made it through 5 architectural walls this session (BUG-862 wrong backend; BUG-869 OCI manifest from CodeBuild; BUG-870 EFS lookup tag filter; BUG-871 Lambda single-FSC + `/mnt/` constraint with collapse + BIND_LINKS bootstrap symlinks + EFS subpaths; BUG-872 cache prefix mismatch with ECS) and is blocked at **BUG-873**: Lambda image-mode rejects OCI manifests *and* requires a Lambda Runtime API client at the entrypoint. Both walls fall to the same architectural fix — route every Lambda CreateFunction through the existing `BuildAndPushOverlayImage` path, swapping the `os/exec docker build` invocation for `awscommon.CodeBuildService` so it works inside the runner-Lambda. Cells 3/4 GitLab inherit BUG-868 (gitlab-runner `start-attach-script` per-command lifecycle vs Fargate non-restartable task) on top of BUG-873.

### Paths forward to GREEN — full per-cell unblock plan

| Cell | State | Next step |
|---|---|---|
| 1 GH × ECS | ✅ GREEN | none — re-run during sweep |
| 2 GH × Lambda | 🟡 6 walls past, BUG-874 next | Phase 116: ALB fronting the runner-Lambda's sockerless port + reverse-agent dial-back; ContainerStart blocks until Active + dial-back; exec tunnels via reverse-agent |
| 3 GL × ECS | 🟡 progressed past cleanup_file_variables, BUG-868 blocks step_script | Phase 114: long-lived Fargate helper task (`tail -f /dev/null`) + SSM ExecuteCommand per script step |
| 4 GL × Lambda | ⏸ inherits BUG-868 + BUG-874 | unblocks once Phases 114 + 116 land |

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

- **AWS creds: ACTIVE** as of 2026-04-29 (root `729079515331`, eu-west-1).
- **Live AWS infra: UP in eu-west-1** — provisioned 2026-04-28; still running. ECS + Lambda live envs as above. NAT Gateway runs ~$0.045/hr — tear down via `terragrunt destroy` from `terraform/environments/{ecs,lambda}/live` when the session ends.
- **Sockerless daemons: RUNNING but on PRE-FIX BINARIES (mmap'd).** ECS PID 75092 started 2026-04-28 17:17 UTC; Lambda PID 70870 started 2026-04-28 16:53 UTC. Cells 3+4 require restart to pick up BUG-859/860 fixes — the on-disk binaries at `/tmp/sockerless-backend-{ecs,lambda}` are post-fix. See `DO_NEXT.md` § "Sockerless restart command".
- **Smoke verified** — `DOCKER_HOST=tcp://localhost:3375 docker run --rm alpine:latest echo hi` exits 0 from a Fargate task; verifies the BUG-846 AWS-Public-Gallery routing for Docker Hub library refs.
- **Podman machine** — running (applehv VM, user-mode networking, 4 CPU / 10 GiB / 100 GiB). Used for local-Podman dispatcher testing in 110a; not used for cell 3+4 (gitlab-runner master is a darwin-native binary).
- **PAT keychain entries** — `gh` (GitHub) keychain-backed; GitLab PAT in `security(1)` keychain entry `sockerless-gl-pat`.
