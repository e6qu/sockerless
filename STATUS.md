# Sockerless — Status

**103 phases closed (Phase 108 closed 2026-04-26). 844 bugs tracked — 844 fixed, 0 open. 1 false positive.** PR #118 merged. PR #120 merged. PR #121 open (Phase 109 strict cloud-API fidelity sweep; **10 audit items closed, all CI green**). **Phase 109 — closed in PR #121:** (1) AWS Lambda VpcConfig from real subnet CIDR; (2) AWS region/account scoping via `SOCKERLESS_AWS_*` env vars; (3) GCP `compute.firewalls` resource (was missing); (4) GCP `iam.serviceAccounts.generateAccessToken` (was missing); (5) Azure IMDS metadata token endpoint (was missing — `DefaultAzureCredential` 404'd); (6) Azure Blob Container ARM control plane (was missing — `azurerm_storage_container` 404'd); (7) Azure NSG rule priority+direction uniqueness validation; (8) Azure Private DNS AAAA/CNAME/MX/PTR/SRV/TXT records (was A-only); (9) GCP `compute.routers` + Cloud NAT (was missing); (10) Azure `Microsoft.Network/natGateways` + `routeTables` (was missing). **Phase 109 — pending in PLAN.md:** Azure ACA Apps + Jobs `provisioningState` async transition (Azure-AsyncOperation polling endpoint), GCP Cloud Run Jobs state machine, timestamps respect-on-write, no-fakes test-fixture sweep. Phase 104 framework migration complete + cloud-native typed drivers across every backend (44/91 cells cloud-native); Phase 105 waves 1-3 done (8 libpod-shape golden tests); Phase 108 done (77/77 sim-parity matrix ✓); Phase 106/107 runner harnesses shipped under `tests/runners/{github,gitlab}/`. **Next on this branch:** finish Phase 109 pending sequence, then wrapper-removal + further interface tightening.

See [PLAN.md](PLAN.md) (roadmap), [BUGS.md](BUGS.md) (bug log), [WHAT_WE_DID.md](WHAT_WE_DID.md) (narrative), [DO_NEXT.md](DO_NEXT.md) (resume pointer).

## Branch state

- **`main`** — synced with `origin/main` at PR #120 merge.
- **`phase-109-strict-fidelity-sweep`** — open as PR #121, ~14 commits ahead of main (Phase 109 sweep in flight; 10 closures so far, all CI green).
- **`origin-gitlab/main`** — mirror, lags; pushed when convenient.

## Recent merges

| PR | Summary |
|---|---|
| #121 (open) | Phase 109 strict cloud-API fidelity sweep — AWS Lambda VpcConfig + region/account scoping; GCP `compute.firewalls` + `compute.routers`/Cloud NAT + `iam.generateAccessToken`; Azure IMDS token endpoint + Blob Container ARM CRUD + NSG priority+direction validation + Private DNS AAAA/CNAME/MX/PTR/SRV/TXT records + NAT Gateways + Route Tables. All 10 closed items make the GCP/Azure sims a much closer match to the real-cloud APIs sockerless's GH+GitLab runner phases will exercise. |
| #120 | Audit + Phase 104 framework migration + cloud-native typed drivers + Phase 105 waves 1-3 + Phase 108 closed + Phase 106/107 harness scaffolding + ImageRef domain type + Phase 109 first-round (BUG-836..844: real ECS lifecycle, real SSM AgentMessage protocol, real subnet-CIDR IP allocation, real Azure per-site hostnames, real kill signal routing) + repo-wide code/doc cleanup. |
| #119 | Post-PR-#118 state-doc refresh — Phase 104 promoted to active. |
| #118 | Round-8 + Round-9 live-AWS sweep — 30 bugs (BUG-786..819), per-cloud terragrunt sweep parity. |
| #117 | Round-7 live-AWS sweep — 16 bugs (BUG-770..785). |
| #115 | Phases 96/98/98b/99/100/101/102 + 13-bug audit sweep. |
| #114 | Phase 91 ECS EFS volumes + BUG-735/736/737. |

## Open work (full detail in [PLAN.md](PLAN.md))

- **Phase 104** — cross-backend driver framework. **Framework migration complete + cloud-native coverage near-full.** All 13 adapters; every dispatch site flows through TypedDriverSet. Per-backend default-driver matrix: [specs/DRIVERS.md](specs/DRIVERS.md). 44/91 cells cloud-native (excluding docker, where local SDK passthrough is itself the cloud-native path); the rest stay on legacy adapters whose api.Backend method already does the cloud-native thing. `core.ImageRef` typed domain object landed at the typed `RegistryDriver.Push/Pull` boundary — first instance of the interface-tightening track. Remaining: wrapper-removal pass (gated on docker getting typed drivers OR accepting wrappers as permanent); typed Signal enum / structured Stats; `ResolveImageReg(ImageRef)` helper to migrate the registry-resolution call sites still on `splitImageRefRegistry`.
- **Phase 105** — libpod-shape conformance, rolling. Waves 1-3 done (8 handlers); wave 4 (events stream, exec start hijack, container CRUD) lower-priority.
- **Phase 106 / 107** — real CI runner harnesses shipped under `tests/runners/{github,gitlab}/`, build-tag-gated. End-to-end runs against live cloud + real repo/project pending — needs operator to reactivate AWS root-account key + provision live ECS via [manual-tests/01-infrastructure.md](manual-tests/01-infrastructure.md). Architecture: per-backend daemon (v1) → label-dispatch via Phase 68 (v2). `dind` sub-test included on the GitLab side.
- **Phase 109** — strict cloud-API fidelity audit (in flight). Triggered by PR #120 CI failures that traced back to synthetic responses. Goal: every sim slice sockerless touches behaves like the real cloud — same wire shape, same validation rules, same state transitions, same SDK / CLI / Terraform-provider compatibility. **Closed in this branch:** real ECS task lifecycle (was log-config-gated), real SSM AgentMessage stdin protocol (was dropping binary frames), real subnet-CIDR IP allocation (was hardcoded `10.0.x.x`), real per-site Azure hostnames (was sharing simulator host), AWS-shape default subnet ID (CLI param validator). **Pending sequence in PLAN.md § Phase 109:** Lambda VPC ENI IPs, region/account scoping, Cloud Run jobs + ACA state machines, full VPC/firewall/IAM/service-discovery/storage parity scoped to GH+GitLab runner needs.
- **Phase 68** — Multi-Tenant Backend Pools. P68-001 done; 9 sub-tasks remaining; Phase 106 label-routing motivates this.
- **Live-cloud runbooks** — GCP + Azure terraform live envs to add; per-cloud `sockerless_runtime_sweep` makes destroy self-sufficient.

## Test counts (head of `main`)

| Category | Count |
|---|---|
| Core unit | 312 |
| Cloud SDK/CLI | AWS 68, GCP 64, Azure 57 |
| Sim-backend integration | 77 |
| GitHub E2E | 186 |
| GitLab E2E | 132 |
| Terraform | 75 |
| UI/Admin/bleephub | 512 |
| Lint (18 modules) | 0 |

## AWS access key state

Root-account access key `AKIA2TQEGRDBRV2KFW6L` deactivated by maintainer 2026-04-26 post-round-9. **Reactivate via AWS Console before any future live-AWS test pass** (Phase 106 ECS workloads, Phase 87 live-GCP doesn't need it, Phase 88 live-Azure doesn't need it). Per-cloud `null_resource sockerless_runtime_sweep` (BUG-819) makes `terragrunt apply` + `terragrunt destroy` self-sufficient.
