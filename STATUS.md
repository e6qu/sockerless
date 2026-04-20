# Sockerless — Status

**86 phases (757 tasks). 726 bugs tracked: 724 fixed + 1 partially fixed + 1 open (BUG-716, Phase 88). Phase 86 Phase C CLOSED 2026-04-20. Phase 87 CLOSED in code 2026-04-21 (Cloud Run Services path behind `SOCKERLESS_GCR_USE_SERVICE=1` + `SOCKERLESS_GCR_VPC_CONNECTOR`; BUG-715 fixed via CNAME-to-`Service.Uri` discovery; live-GCP validation pending). Phase 89 near-complete: `specs/CLOUD_RESOURCE_MAPPING.md` for all 7 backends; `Store.Images` disk persistence removed; all 4 cloud backends have `resolve*State` helpers with every cloud-state-dependent callsite migrated (BUG-725 fixed); `docker images` is cloud-derived across all 6 cloud backends (BUG-723 fixed); ECS `ListPods` groups tasks via `sockerless-pod` tag; `resolveNetworkState` lands for ECS+Cloud Run+ACA (BUG-726 fixed). Remaining: `ListPods` for cloudrun+aca (cloudrun now unblocked post-87; aca still blocked on Phase 88), per-backend restart-resilience integration tests. Branch `post-phase86-continuation`.**

## Phase 87 — Cloud Run Services (2026-04-21)

Closed BUG-715 in code. Five slices in `post-phase86-continuation`:

- **87-01** `servicespec.go` — Service proto builder. Internal ingress, VPC connector ALL_TRAFFIC, Min=Max=1.
- **87-02** `cloud_state_services.go` + `store.go` + `gcp.go` — `ServiceName` field, `Services` client, resolveServiceName / resolveServiceCloudRunState / queryServices / serviceToContainer / serviceContainerState (TerminalCondition → running/exited/created). `ListContainers` merges Services when UseService.
- **87-03** `start_service.go` + `backend_impl.go` — ContainerStart branch: startSingleContainerService + startMultiContainerServiceTyped; `deleteService` helper.
- **87-04** `backend_impl.go` — Stop/Kill/Remove delete Service (no cancel equivalent); cache cleared on stop.
- **87-05** `config.go` + `backend_impl.go` + `backend_impl_network.go` + `service_discovery_cloud.go` — Validate gate opened (UseService requires VPCConnector). Logs filter → `cloud_run_revision` + `service_name`. `cloudServiceRegisterCNAME` / `cloudServiceDeregisterCNAME` write CNAMEs from `<hostname>.<network>.internal.` to `Service.Uri` host.

13 unit tests across the slices; `go test` + `golangci-lint` green for the cloudrun module.

## Test Counts

| Category | Count |
|---|---|
| Core unit | 310 |
| Cloud SDK/CLI | AWS 68, GCP 64, Azure 57 |
| Sim-backend integration | 75 |
| GitHub E2E | 186 |
| GitLab E2E | 132 |
| Terraform | 75 |
| UI/Admin/bleephub | 512 |
| Lint (18 modules) | 0 issues |

## ECS Live Testing

6 rounds against real AWS ECS Fargate (`eu-west-1`). Round 6: Docker CLI all pass, Podman pull+pods pass (container ops blocked by response format), Advanced 3/4. See [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md).

## Phase 86 — Complete Runner Support (simulator parity + Phase D)

Done across AWS + GCP + Azure. Full simulator parity audited in `docs/SIMULATOR_PARITY_{AWS,GCP,AZURE}.md` (zero ✖ rows on runner path). All BUGS.md entries closed.

Highlights this phase:
- **A.5 testing contract** — pre-commit hook blocks sim-endpoint additions without matching SDK + CLI + terraform tests.
- **BUG-696 ECR pull-through cache** — AWS ECR cache-rule CRUD + image URI rewriting.
- **BUG-697 Store.Images persistence** — `docker pull` survives backend restart across all six cloud backends.
- **BUG-700 cloud-side network-create Warning** — ECS + Cloud Run + ACA.
- **BUG-701 cross-task DNS** — AWS Cloud Map / GCP Cloud DNS / Azure ACA environments all back themselves with real Docker networks.
- **BUG-702 Azure Private DNS SDK wire** — backend calls real `armprivatedns`.
- **BUG-703 Azure NSG SDK wire + simulator securityRules sub-resource**.
- **BUG-704 GCP Cloud Build slice + BUG-707 Secret Manager integration**.
- **BUG-705 AWS Lambda Runtime API slice** — `simulators/aws/lambda_runtime.go` implements the full cloud contract.
- **BUG-706 Azure ACR Cache Rules** — simulator cache-rule CRUD + backend pull-through resolver.
- **Phase D Lambda agent-as-handler** — bootstrap polling loop (`agent/cmd/sockerless-lambda-bootstrap`), overlay image build in `ContainerCreate`, reverse-agent WebSocket server on `/v1/lambda/reverse`.

Live-AWS session (Phase E) is scripted in `scripts/phase86/*.sh` and dispatched by `.github/workflows/phase86-aws-live.yml`; awaits AWS credentials. Runner-capability matrix live columns stay pending-live until that session runs.
