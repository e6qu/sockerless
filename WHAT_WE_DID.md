# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform. Designed to power CI runners on cloud serverless capacity — see [docs/RUNNERS.md](docs/RUNNERS.md).

State [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · architecture [specs/](specs/).

This file keeps narrative — *why* each phase, what was surprising, what blocked. Per-bug detail in [BUGS.md](BUGS.md); code-level detail in `git log`.

## 2026-05-09 — Phase 121b Azure sim hardening + driver consolidation + GCP sim invoke routing (PR #135, merged)

Single PR. Multi-layer scope:

- **Azure sim cloud-faithful**: Files data plane on disk (`handleAzureFilesPath`), HS256-signed Azure AD JWT (`mintAzureSimJWT`).
- **All-6-backends test harness restructured** to `SOCKERLESS_TEST_TARGET=sim|cloud`. No skips, no fallbacks, no `//go:build integration` tag, no `SOCKERLESS_INTEGRATION` env. `make test-integration` (sim) / `test-integration-cloud` (cloud) per backend; CI sets sim.
- **In-memory storage backing driver** registered across all 6 backends (`core.MemoryDriver`, `BackingMemory`).
- **Driver consolidation pattern B** (live in `*-common`, shared cross-backend within cloud, callback-based for backend-specific state): `gcp-common.IDTokenAccess`, `aws-common.IAMRoleAccess`, `gcp-common.CloudDNSZoneDNS`, `aws-common.CloudMapDNS`, `azure-common.PrivateDNSZoneDNS`. Per-backend adapters deleted.
- **GCP sim Cloud Run service URI** now routes through sim's own `/v2-services-invoke/{project}/{location}/{service}` handler. Was issuing bogus `https://<svc>-<project>.run.app` URIs — backend invokes dialed real Google IPs and 401'd against the public Cloud Run wildcard cert (103 SANs, none matching). Sim now hosts the URLs it returns; runs the overlay container on demand and forwards the envelope POST body to the bootstrap.
- **GCF envelope parsing**: `invokeFunction` was storing the entire bootstrap response (`{sockerlessExecResult:{exitCode, stdout(b64), stderr(b64)}}`) as logs. Extracted `gcpcommon.ParseExecResult` from `PostExecEnvelope` so both POST-and-parse and parse-only paths share one decoder. Subprocess exit code now propagates through `inv.ExitCode`.
- **GCF Docker labels round-trip**: pod_service was constructing TagSets without `container.Config.Labels`; `serviceToPodMemberContainer` wasn't decoding them on the read path. `dockerLabelsFromCloudRunService` merges svc.Labels + svc.Annotations and reverses the AsMap encoding.
- **Cloudrun TestMain in sim mode** disables overlay path. Bootstrap defaults to long-lived HTTP-server (Path B); overlay-as-PID1 meant arithmetic test containers never exited. `TestCloudRunJobTimeout` removed; timer is fully unit-tested in `agent/cmd/sockerless-cloudrun-bootstrap/main_test.go`.
- **Tooling**: `scripts/check-latest-deps.sh` (pre-push + CI gate, no warn tier, fail-loud); `make upgrade-deps` per module + root fanout. All Go modules + TF providers + Azure SDK majors bumped (`armappcontainers v2→v3`, `armappservice v4→v5`, `armnetwork v6/v7→v8`); v28 Docker SDK breakage fixed; azurerm v4 schema (`enable_https_traffic_only`→`https_traffic_only_enabled`).
- **Publish workflow**: dropped QEMU. Per-arch native runners (`ubuntu-latest` amd64, `ubuntu-24.04-arm` arm64). Tag format `<sha>-<arch>` + manifest-list assembly via `docker buildx imagetools create`.

Driven by user direction (no fallbacks, no skips, all configs explicit, sim/cloud target swappable, cloud-specific drivers extend generic shape, in-cloud duplication consolidates, drop QEMU if unneeded). Scope expanded continuously through CI debugging — TLS failure (sim issued real-cloud URIs) → envelope-decode (logs were JSON not stdout) → label round-trip (TagSet missing Labels). Each surfaced as the prior fix unblocked the next layer.

Deferred to stacked follow-up PRs: 121b-deferred-{I,J,K,L} need per-backend NetworkState models or operator infra not modeled today (host-aliases everywhere; AZF DNS; Lambda VPC; Azure AD access).

## 2026-05-09 — Phase 127 Storage driver expansion (PR #134, merged)

3 new `core.StorageBacking`: `pd-ephemeral` (GCP CE PD), `efs-ephemeral` (AWS EFS access point), `azure-files-ephemeral` (Azure Files share). All honor the no-idle-cost directive. Per-cloud drivers in `gcp-common` / `aws-common` / `azure-common`; per-backend `storageBackings` registries wire them in. 15 unit tests. Existing volume materialization paths unchanged (consolidation deferred).

## 2026-05-09 — Phase 126 Access driver (PR #133, merged)

`AccessMechanism` enum (iam-role / id-token / mTLS / none-internal). `AccessDriver` interface = `Mechanism()` + `WorkloadPrincipal() string` + `AuthenticatedClient(ctx, audience) (*http.Client, error)`. Per-backend adapters: cloudrun + GCF id-token (wraps `idtoken.NewClient`), ECS + Lambda iam-role (SigV4 at SDK), ACA + AZF none-internal. Every `idtoken.NewClient` callsite migrated through `s.Access.AuthenticatedClient`; the `idtoken` import disappears from both backends.

## 2026-05-09 — Phase 125 DNS driver (PR #132, merged)

`DNSMechanism` enum (cloud-map / cloud-dns-zone / service-discovery / private-dns-zone / none). `DNSDriver` = `SearchDomain(ctx, networkID)` + `Mechanism()`. Per-backend adapters: cloudrun cloud-dns-zone, ECS cloud-map, ACA private-dns-zone (FaaS = NoOp). `SOCKERLESS_DNS_SEARCH_DOMAIN` injected at every `ContainerCreate`; cloudrun + gcf bootstraps write `search` line to `/etc/resolv.conf`.

## 2026-05-09 — Phase 124 Network discovery driver (PR #131, merged)

`NetworkDiscoveryKind` enum (host-aliases / cloud-dns / service-mesh / nat-gateway-only). Per-backend adapters: cloudrun cloud-DNS, ECS service-mesh (Cloud Map), ACA cloud-DNS, GCF host-aliases (in-process). `BaseServer.NetworkDiscovery` field; all `cloudServiceRegister/Deregister/Resolve` callsites migrated through the driver. Interface signature widened to include explicit `containerID` so Cloud Map (keys by ID) and DNS-zone (keys by hostname) both fit.

## 2026-05-09 — Phase 128 Runner job timeout (PR #130, merged)

Two-layer timeout: bootstrap timer (`runWithTimeout` in cloudrun + gcf bootstraps; SIGTERM → 30s grace → SIGKILL → exit 124) + cloud-native cap (cloudrun TaskTemplate.Timeout, ACA ReplicaTimeout, Lambda 900s) derived from `core.JobTimeoutDefault()`. `SOCKERLESS_JOB_TIMEOUT_SECONDS` contract; per-job override via `docker run -e` wins.

## 2026-05-09 — Phase 135 Sim host model + native arm64 CI (PR #129, merged)

Workloads dispatch through Docker honouring explicit `Architecture` (sim's `linux/arm64` capacity); per-cloud-product host-metadata services (AWS IMDSv2 + ECS task v4; GCP `metadata.google.internal`; Azure IMDS); static no-`os/exec`-of-workload check; SDK metadata tests; native `ubuntu-24.04-arm` CI runners (no QEMU). 12 bugs closed (BUG-949/972/975-984).

## Older closed phases (compressed)

| PRs | Phases | Headline |
|---|---|---|
| #128 | 134 | Makefile standardization + per-app leaf Makefiles + stack orchestration. |
| #127 | 129#4 + 130–132 | Orphan pod-Service GC; sim parity prep; bleephub workflows + oauth REST + UI. |
| #125 | CI reorg | Workflows reorganized: zero auto-fire on main; live-tests-{cloud}. |
| #122–123 | 110 + 118 + 120–123 | 8/8 runner cells GREEN; FaaS pod overlays; cloud-faithful GCP sim; storage-backing driver pilot. |
| #117–121 | 109 + Round-7/8/9 | Live-AWS bug sweep; strict cloud-API fidelity audit. |
| #112–115 | 86–102 | Sim parity; stateless backends; real volumes; FaaS invocation tracking; reverse-agent exec/cp/diff/commit/pause; Docker pod synthesis. |

Per-bug detail in [BUGS.md](BUGS.md). Per-commit detail in `git log`.
