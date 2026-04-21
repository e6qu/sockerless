# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform.

See [STATUS.md](STATUS.md) for the current phase roll-up, [BUGS.md](BUGS.md) for the bug log, [PLAN.md](PLAN.md) for the roadmap, [specs/](specs/) for architecture specs (start with [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md), [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md)).

## Phase 91 — ECS real volumes via EFS access points (2026-04-21)

`docker volume create` / `-v volname:/mnt` finally provisions real cloud storage on ECS:

- One sockerless-owned EFS filesystem per backend, lazily created on first use (or reused when the operator sets `SOCKERLESS_ECS_AGENT_EFS_ID`), with mount targets in every configured subnet.
- One EFS access point per Docker volume — access-point tags (`sockerless-managed=true` + `sockerless-volume-name=<name>`) carry the mapping so `VolumeInspect` / `VolumeList` / `VolumeRemove` derive from cloud actuals, not an in-memory store.
- Task defs emit `EFSVolumeConfiguration{TransitEncryption=ENABLED, AuthorizationConfig.AccessPointId}` for every named-volume bind; host-path binds (`/h:/c`) stay rejected because Fargate has no host filesystem to bind from.
- Simulator-side `simulators/aws/efs.go` backs every access point with a real host directory under `$SIM_EFS_DATA_DIR` so tasks running locally see persistent files across runs.
- BUG-735 + BUG-736 (ECS half) re-land as this phase.

## Phase 90 — No-fakes / no-fallbacks audit (2026-04-21)

Project-wide audit against the "no fakes, no fallbacks, no placeholders" principle. Every workaround, silent substitution, or placeholder field now gets treated as a bug — not a "known limitation". 11 bugs filed (BUG-729 through BUG-746), 8 fixed in-sweep, 3 scoped as dedicated phases:

| Bug | Area | Resolution |
|---|---|---|
| 729 | SSM ack wire format matches AWS agent (Flags=3 + LSL/MSL layout) | fixed |
| 730 | `ImagePullWithMetadata` no longer synthesises placeholder image records when registry fetch fails | fixed |
| 731 | `VolumeCreate` etc. return `NotImplemented` with a per-cloud message instead of silently storing metadata; Phase 91-94 replace with real provisioning | fixed + superseded on ECS by Phase 91 |
| 732 | Dead `cloudrun.NetworkState.FirewallRuleName` placeholder deleted | fixed |
| 733 | ECS stats no longer fabricates `PIDs: 1` when CloudWatch has no data yet | fixed |
| 734 | ECS `getNamespaceName` propagates the underlying error instead of substituting the raw ID | fixed |
| 735 | ECS rejects host-path bind mounts cleanly; named-volume binds land on EFS (Phase 91) | fixed |
| 736 | Cloud Run + ACA reject bind mounts up-front until Phase 92/93 ship real mount support | fixed (rejection) + queued (provisioning) |
| 737 | `SOCKERLESS_SKIP_IMAGE_CONFIG` opt-out deleted entirely; `ImagePullWithMetadata` requires real metadata | fixed |
| 744 | FaaS CloudState can't signal invocation completion; scope as Phase 95 | scoped |
| 745 | CR/ACA Jobs have no native `docker exec` — scope as Phase 96 (reverse-agent pattern) | scoped |
| 746 | Docker labels don't survive GCP's label-value charset — scope as Phase 97 (GCP annotations / Azure tags) | scoped |

## Phase 89 — Stateless backend audit (2026-04-21)

Per the stateless-backend directive: every cloud backend derives state from cloud actuals; no on-disk state, no canonical in-memory state.

- `specs/CLOUD_RESOURCE_MAPPING.md` formalises how docker concepts (container / pod / image / network / volume) map to each backend's cloud resources + the restart-recovery contract.
- Every cloud-state-dependent callsite across ECS / Lambda / Cloud Run / ACA migrated to `resolve*State` helpers that combine an in-process cache with a cloud-derived fallback.
- `core.CloudImageLister` + `core.CloudPodLister` optional interfaces let `BaseServer.ImageList` / `PodList` merge cloud-derived entries. `ListImages` across all 6 cloud backends (ECR SDK + shared `core.OCIListImages` for AR/ACR). `ListPods` across ECS + cloudrun + aca.
- `resolveNetworkState` reconstructs per-network cloud state (ECS SG + Cloud Map namespace; Cloud Run managed zone; ACA Private DNS + NSG) after a backend restart.
- `Store.Images` disk persistence removed; cache is in-process only.

## Phase 88 — ACA Apps (2026-04-21)

Two execution paths selected by `SOCKERLESS_ACA_USE_APP`:

- **Apps**: `ContainerAppsClient.BeginCreateOrUpdate` with internal ingress + managed environment + min/max = 1. Peers resolve via Private DNS CNAMEs → `ContainerApp.LatestRevisionFqdn`.
- **Jobs** (default): unchanged.

`Config.Validate()` rejects `UseApp=true` without a managed environment.

## Phase 87 — Cloud Run Services (2026-04-21)

Two execution paths selected by `SOCKERLESS_GCR_USE_SERVICE`:

- **Services**: `Services.CreateService` with `INGRESS_TRAFFIC_INTERNAL_ONLY` + VPC connector + scale = 1. Peers resolve via Cloud DNS CNAMEs → `Service.Uri`.
- **Jobs** (default): unchanged.

`Config.Validate()` rejects `UseService=true` without a VPC connector.

## Phase 86 — Simulator parity + Lambda agent-as-handler (2026-04-20, PR #112)

Every cloud-API slice sockerless depends on is a first-class slice in its per-cloud simulator, validated with SDK + CLI + terraform tests (or an explicit `tests-exempt.txt` entry):

- AWS ECR pull-through cache, Lambda Runtime API (per-invocation HTTP sidecar on `host.docker.internal`), Cloud Map backed by real Docker networks.
- GCP Cloud Build + Secret Manager, Cloud DNS private zones backed by real Docker networks.
- Azure Private DNS Zones + NSG + ACR Cache Rules, managed environment backed by real Docker networks.
- Pre-commit testing contract: every `r.Register` addition needs SDK + CLI + terraform coverage.
- Lambda agent-as-handler: `sockerless-lambda-bootstrap` polling loop, overlay-image build in `ContainerCreate`, reverse-agent WebSocket on `/v1/lambda/reverse`.

Phase C live-AWS validated ECS end-to-end in `eu-west-1`: `docker run`, `docker run -d`, FQDN + short-name cross-container DNS, `docker exec`. The live session surfaced 13 real bugs (708–722 minus 715/716); all fixed in-branch. See [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md).

## Stack & structure

- **Simulators** — `simulators/{aws,gcp,azure}/`, each a separate Go module. `simulators/<cloud>/shared/` for container + network helpers, `sdk-tests/` for SDK clients, `cli-tests/` for CLI clients, `terraform-tests/` for provider.
- **Backends** — 7 backends (`backends/docker`, `backends/ecs`, `backends/lambda`, `backends/cloudrun`, `backends/cloudrun-functions`, `backends/aca`, `backends/azure-functions`). Each a separate Go module. Cloud-common shared: `backends/{aws,gcp,azure}-common/` (AuthProvider etc.). Core driver + shared types: `backends/core/`.
- **Agent** — `agent/` with sub-commands for the in-container driver + Lambda bootstrap. Shared simulator library: `github.com/sockerless/simulator` (aliased as `sim`).
- **Frontend** — Docker REST API. `cmd/sockerless/` CLI (zero-deps). UI SPA at `ui/` (Bun / React 19 / Vite / React Router 7 / TanStack / Tailwind 4 / Turborepo), embedded via Go `!noui` build tag. 12 UI packages across core + 6 cloud backends + docker backend + docker frontend + admin + bleephub.
- **Tests** — `tests/` for cross-backend e2e, `tests/upstream/` for external test suite replays (act, gitlab-ci-local), `tests/e2e-live-tests/` for runner orchestration, `tests/terraform-integration/`, `smoke-tests/` for per-cloud Docker-backed smokes.
