# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform.

89 phases, 757 tasks, 737 bugs tracked — 734 fixed, 3 open. See [STATUS.md](STATUS.md) for the current roll-up, [BUGS.md](BUGS.md) for the bug log, [PLAN.md](PLAN.md) for the roadmap, [specs/](specs/) for architecture specs (start with [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md), [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md)).

## Phase 90 — No-fakes / no-fallbacks audit (in progress 2026-04-21)

Project-wide audit against the "no fakes, no fallbacks, no placeholders" principle. Every workaround, silent substitution, or placeholder field now gets treated as a bug — not a "known limitation". BUG-729 through BUG-737 were filed from the first sweep.

Fixed in-session:
- **BUG-729** — SSM ack wire format rewritten to match AWS's `SerializeClientMessageWithAcknowledgeContent`: `Flags=3` and UUID packed as LSL at offset 64 + MSL at offset 72 (AWS Java-style `putUuid`). Sim mirrored. `ssmDecoder.seenIDs` dedupe workaround removed. Unit tests pin the wire layout + JSON body + no-dedupe.
- **BUG-730** — Core `ImagePullWithMetadata` stopped synthesising ID / size / layers when registry metadata fetch failed. `FetchImageMetadata` now propagates the real error. `SOCKERLESS_SKIP_IMAGE_CONFIG` opt-in retained pending BUG-737.
- **BUG-731** — `VolumeCreate` / `VolumeRemove` / `VolumeInspect` / `VolumeList` / `VolumePrune` on ECS/Lambda/Cloud Run/ACA return `NotImplemented` with a clear per-cloud message. Dead `VolumeState` structs + placeholder fields deleted.
- **BUG-732** — Dead `cloudrun.NetworkState.FirewallRuleName` placeholder field deleted.
- **BUG-733** — ECS stats no longer fabricates `PIDs: 1` when CloudWatch has no data yet; returns 0.
- **BUG-734** — ECS `getNamespaceName` propagates the underlying error instead of silently substituting the raw namespace ID for the name.

Open (decisions made, fixes in progress):
- **BUG-735** — ECS silently substituting empty scratch volumes for bind mounts when EFS isn't configured. Next up.
- **BUG-736** — Cloud Run + ACA silently dropping `HostConfig.Binds` on the floor.
- **BUG-737** — Remove `SOCKERLESS_SKIP_IMAGE_CONFIG` opt-out entirely; require real registry metadata always.

Follow-up queued as phases 91-94: real per-cloud volume provisioning (EFS access points, GCS bucket mounts, Azure Files shares) with matching simulator slices. Designs in `specs/CLOUD_RESOURCE_MAPPING.md` under "Volume provisioning per backend".

## Phase 89 — Stateless backend audit (2026-04-21)

Per the stateless-backend directive: every cloud backend derives state from cloud actuals; no on-disk state, no canonical in-memory state.

- `specs/CLOUD_RESOURCE_MAPPING.md` formalises how docker concepts (container / pod / image / network) map to each of the 7 backends' cloud resources, plus the restart-recovery contract.
- Every cloud-state-dependent callsite across ECS / Lambda / Cloud Run / ACA was migrated to `resolve*State` helpers that combine an in-process cache with a cloud-derived fallback (BUG-725).
- `core.CloudImageLister` + `core.CloudPodLister` optional interfaces let `BaseServer.ImageList` / `PodList` merge cloud-derived entries. `ListImages` implemented across all 6 cloud backends via ECR SDK + shared `core.OCIListImages` helper for Artifact Registry / ACR (BUG-723). `ListPods` implemented across ECS + cloudrun + aca (BUG-724).
- `resolveNetworkState` reconstructs per-network cloud state (ECS SG + Cloud Map namespace; Cloud Run managed zone; ACA Private DNS zone + NSG) after a backend restart (BUG-726).
- `Store.Images` disk persistence removed; the cache is now in-process only.

## Phase 88 — ACA Apps (2026-04-21)

Closes BUG-716 in code. Two parallel execution paths selected by `SOCKERLESS_ACA_USE_APP`:

- **Apps path**: `ContainerAppsClient.BeginCreateOrUpdate` with `Ingress.External=false` + managed environment + min/max replicas = 1. Peers resolve each other via Private DNS CNAMEs pointing at `ContainerApp.LatestRevisionFqdn` (reachable inside the environment's VNet). Logs query `ContainerAppName_s` in `ContainerAppConsoleLogs_CL`.
- **Jobs path**: unchanged default; keeps `Jobs.BeginStart`.

`Config.Validate()` rejects `UseApp=true` without an existing Environment. Live-Azure validation pending.

## Phase 87 — Cloud Run Services (2026-04-21)

Closes BUG-715 in code. Two parallel execution paths selected by `SOCKERLESS_GCR_USE_SERVICE`:

- **Services path**: `Services.CreateService` with `Ingress=INGRESS_TRAFFIC_INTERNAL_ONLY` + VPC connector + min/max = 1 scaling. Peers resolve each other via Cloud DNS CNAMEs pointing at `Service.Uri`'s host (reachable over the VPC connector). Logs query `cloud_run_revision` + `service_name`.
- **Jobs path**: unchanged default; keeps `Jobs.RunJob`.

`Config.Validate()` rejects `UseService=true` without a VPC connector. Live-GCP validation pending.

## Phase 86 — Simulator parity + Lambda agent-as-handler (closed 2026-04-20, PR #112)

Every cloud-API slice sockerless depends on is a first-class slice in its per-cloud simulator, validated with SDK + CLI + terraform tests (or explicit exemption):

- AWS ECR pull-through cache, Lambda Runtime API (per-invocation HTTP sidecar on `host.docker.internal`), Cloud Map with real Docker-network backing.
- GCP Cloud Build + Secret Manager integration, Cloud DNS private zones with real Docker-network backing.
- Azure Private DNS Zones + NSG + ACR Cache Rules backend SDK wires, managed environment with real Docker-network backing.
- Pre-commit testing contract: every `r.Register` addition needs SDK + CLI + terraform tests (or an explicit `tests-exempt.txt` entry).
- Lambda agent-as-handler pattern: `sockerless-lambda-bootstrap` polling loop, overlay image build in `ContainerCreate`, reverse-agent WebSocket on `/v1/lambda/reverse`.

Phase C live-AWS validated ECS end-to-end in `eu-west-1`: `docker run` / `docker run -d` / FQDN + short-name cross-container DNS / `docker exec`. The live session surfaced 13 real bugs (708–722 minus 715/716); all fixed in-branch. See [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md).

## Stack & structure

- **Simulators** — `simulators/{aws,gcp,azure}/`, each a separate Go module. `simulators/<cloud>/shared/` for container + network helpers, `sdk-tests/` for SDK clients, `cli-tests/` for CLI clients, `terraform-tests/` for provider.
- **Backends** — 7 backends (`backends/docker`, `backends/ecs`, `backends/lambda`, `backends/cloudrun`, `backends/cloudrun-functions`, `backends/aca`, `backends/azure-functions`). Each a separate Go module. Cloud-common shared: `backends/{aws,gcp,azure}-common/` (AuthProvider etc.). Core driver + shared types: `backends/core/`.
- **Agent** — `agent/` with sub-commands for the in-container driver + Lambda bootstrap. Shared simulator library: `github.com/sockerless/simulator` (aliased as `sim`).
- **Frontend** — Docker REST API. `cmd/sockerless/` CLI (zero-deps). UI SPA at `ui/` (Bun / React 19 / Vite / React Router 7 / TanStack / Tailwind 4 / Turborepo), embedded via Go `!noui` build tag. 12 UI packages across core + 6 cloud backends + docker backend + docker frontend + admin + bleephub.
- **Tests** — `tests/` for cross-backend e2e, `tests/upstream/` for external test suite replays (act, gitlab-ci-local), `tests/e2e-live-tests/` for runner orchestration, `tests/terraform-integration/`, `smoke-tests/` for per-cloud Docker-backed smokes.
