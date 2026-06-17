# Sockerless - What We Built

Roadmap [PLAN.md](PLAN.md) - status [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

Detailed historical narrative lives in PR descriptions and `git log`. This file kept the recent chain and a compact foundation summary.

## 2026-06-17 - Audit backlog (deferred): removed the sim-only `Sim*` fake fields (BUG-1840)

Picked up the first of the three items deferred from #594. The gcp and azure
simulators carried `SimCommand`/`SimImage`/`SimArchitecture` (gcp Cloud Functions
`ServiceConfig`) and `SimCommand` (azure App Service `SiteConfig`) — sim-only
fields on the real cloud resource model, accepted off the wire though no real
client produces them. A faithful sim must not have them.

**gcp.** The image-less "Sim path" of `invokeCloudFunctionProcess` (which ran a
`SimImage` container directly so SDK tests could check invoke semantics without
staging an overlay) was backend-dead: a Cloud Functions Gen2 is backed by a Cloud
Run service, and the gcf backend invokes that service's `svc.Uri`
(→ `/v2-services-invoke`, served by `cloudrunservices.go invokeService`), never the
sim-only `/v2-functions-invoke` endpoint that fronted the Sim path. Removed the
fields, the Sim branch, and the `Sim*`-only `TestCloudFunctions_Invoke*` /
`InvokeArithmetic*` SDK tests. `invokeCloudFunctionProcess` now runs only the
faithful overlay-image path; a function with no deployed image records
"Function invoked" in Cloud Logging and returns `{}`. Real gcp container-execution
coverage stays via the Cloud Run **Jobs** arithmetic tests and the gcf cell.

**azure.** Kept `invokeAzureFunctionProcess` — that IS the real App Service
container-run path the azf backend drives via `LinuxFxVersion` (`DOCKER|<image>`) +
`SOCKERLESS_CMD`/`SOCKERLESS_ENTRYPOINT` app settings. Removed only the sim-only
`SimCommand` fallback and the now-identity `Site.wire()` stripper. The invoke SDK
tests were rewritten to deliver the command exactly as the backend does — a
`SOCKERLESS_CMD` app setting carrying `base64(json(argv))` — and still execute real
`alpine`/`eval-arithmetic` containers.

**Sub-fix (BUG-1824 class).** Verifying the azure rewrite surfaced a pre-existing
local-repro failure: the three sim sdk-tests' `buildGoScratchImage` used
`docker build -t` with no `--load`, so on a `docker-container` buildx-default host
the freshly-built workload image landed in the build cache only and the sim
couldn't run it (eval/`InvokeArithmetic*` 500'd). It now probes
`docker buildx version` and uses `buildx build --load` when present. CI was
unaffected (its docker driver loads to the store); this only blocked local runs.

Cell impact was confirmed by tracing the backend invoke target rather than
re-running the multi-minute gcf/azf cells, which don't exercise the changed code.

## 2026-06-17 - Codebase audit: fallbacks / error-swallowing / fakes / sim-contract / dead code (sims → backends → UIs) + open-issue fixes

A targeted sweep for the anti-patterns the user flagged — fail loudly, never
swallow; no fallbacks or sim special-behaviour that breaks the "sims faithfully
reimplement cloud APIs" contract; avoid defaulted behaviour; no functionally-dead
code. Three parallel read-only audits (one per simulator), a backend audit, and a
UI audit. The audit was productive — it found real P1/P2 issues and a backlog of
genuine fakes/fallbacks.

**Fixed in the sweep:** gcp sim `gcsObjectBytes` silent empty-body on a disk-read
failure → now errors → 500 (BUG-1836); azure sim swallowed real-exec subnet-delete
error → surfaced (1837); azure sim five dead-variable linter-pins removed (1838);
cloudrun backend `resolveExecutionState` fabricated `ExitCode 0` on a cloud-query
failure → now `-1` so a failed job isn't reported as success (1839); gcp Cloud
Build sim's `docker build` step hit the same buildx-`--load` portability bug as the
azure ACR-Tasks sim (BUG-1834) → same buildx-probe fix (1847). Plus the three open
AWS-sim fidelity GitHub issues: CreateVolume now requires AvailabilityZone
(#591/1848), ECS cluster-scoped ops raise `ClusterNotFoundException` for an unknown
cluster (#592/1849), DescribeSnapshots/Volumes honour `MaxResults`+`NextToken`
(#590/1850) — each with an SDK test. Two UI fail-loud fixes: the docker-frontend
dashboard no longer renders a healthy-looking zeroed page on a failed fetch
(1851), and the admin HTTPS-gateway card no longer fabricates plausible endpoint
URLs / CA path when the gateway info is unavailable (1852). Closed the
already-fixed issues #583 (ECS CPU/Mem enforcement) and #569 (process-mode EBS
panic).

**Staged backlog (filed OPEN with fix-shapes, BUG-1840–1846):** the genuine
larger items that need careful, tested, sometimes contract-changing work — the
sim-only `Sim*` fake fields (gcp+azure), aws Cloud Map sockerless-tag DNS, aws ec2
sockerless pre-seeding, gcp fingerprint optimistic-concurrency, the backend
error-swallow batch (PodRemove ×4, core ContainerWait, ecs zero-stats, lambda
pod-row), the cloudrun `networkServices` stateless violation, and the backend
default-param-on-invalid + dead-code batch. These are tracked, not dismissed.

## 2026-06-17 - GitHub `actions/runner` cells on ACA + AZF (both GREEN) — GitHub+GitLab parity on every container backend

The bleephub GitHub `actions/runner` topology cell — a real `actions/runner`
running container-mode jobs (`container:`), service containers (`services:`),
and a dispatcher-spawned runner against a sockerless backend — is now green on
**ACA and Azure Functions**, completing GitHub+GitLab runner parity across all
six container-capable backends (ECS, Lambda-class, Cloud Run, GCF, ACA, AZF).

**ACA** was already wired and stayed green after the #587 backend changes
(faithful ingress, WS keepalive, runner-stage HTTP invoke). **AZF** was newly
wired into the bleephub harness, mirroring aca: the harness image builds
`sockerless-backend-azf` + `sockerless-azf-bootstrap`; a `provision_azf` brings
up the Azure sim + backend with azf's host primitive (an App Service plan), the
ACR-Tasks overlay registry coordinate, cloud-dns service discovery
(`SOCKERLESS_AZF_NETWORK_DISCOVERY=cloud-dns`), the `/v1/azf/reverse` agent
path, and runner-workspace + externals Azure-Files shares; a Makefile target
`bleephub-runner-docker-test-azf` runs it.

Two real bugs surfaced bringing azf up, both fixed:

- **BUG-1834** — the Azure sim's ACR-Tasks overlay build hardcoded the
  buildx-only `--load` flag. The harness container ships the legacy `docker.io`
  builder (no buildx plugin), which rejects `--load` (`unknown flag`), so the
  `container:` job's `ContainerCreate` returned 500. There is no single
  `docker build` invocation that works across the legacy builder, the buildx
  `docker` driver, and the buildx `docker-container` driver, so the sim now
  probes `docker buildx` and uses `docker buildx build --load` when present
  (loads to the daemon store for every driver) else plain `docker build`
  (legacy, store-native), logging the chosen path. (The bleeplab cells had
  passed only on a cached overlay; the bleephub harness wipes its data dir,
  forcing a real build that exposed it.)
- **BUG-1835** — azf cloud-dns `startCloudDNSSite` keyed overlay-vs-raw deploy
  on `OpenStdin`, a gitlab-runner-only signal. A GitHub `container:` job is
  exec-driven but NOT OpenStdin → it was deployed as a raw image with no
  reverse-agent, so `docker exec` of each step failed `exit 126`; and a
  `services:` container (image-default entrypoint) must run its RAW image, not
  the overlay. The fix derives `serviceLike` (no client entrypoint/cmd override
  AND not OpenStdin) from the ORIGINAL client create request, recorded at
  ContainerCreate into `labelServiceLike` BEFORE the image's default
  entrypoint/cmd are merged in (post-merge, the base labels can't distinguish a
  client override from an image default — the same reason aca computes
  `serviceLike` pre-merge). `startCloudDNSSite` reads the marker: a service
  deploys its raw image and runs on the VNet (started by swift integration);
  anything else deploys the overlay and is invoked so the in-site reverse-agent
  registers (`invokeFunctionAsync` blocks for the agent — no fallback).

## 2026-06-17 - azf cloud-dns hardening: connect-after-create alias registration + swift VNet-integration CLI/TF contract

Hardened the merged azf cloud-dns service discovery (`feat/azf-clouddns-hardening`).

**azf `NetworkConnect` on connect-after-create.** The merged cell only ever
created containers *with* their network, so `docker network connect
--network-alias X` *after* create was lost: the core `SyntheticNetworkDriver.Connect`
records the endpoint in `Store.Containers`, which the stateless azf backend
doesn't read. `cloudDNSNetworkConnect` (wired into `NetworkConnect` behind the
cloud-dns config) closes the gap two ways — a connect *before start* stamps the
network + aliases onto the PendingCreate so `startCloudDNSSite` VNet-integrates
and registers them exactly as the create-with-network path does; a connect to an
*already-deployed* site VNet-integrates it into the network's subnet and writes
the `--network-alias` names as Private DNS CNAMEs immediately. Unit test
`TestCloudDNSNetworkConnect_StampsPendingCreate`.

**Swift VNet-integration testing contract (SDK+CLI+Terraform).** The App Service
regional-VNet-integration endpoint (`PUT/GET/DELETE
.../sites/{name}/networkConfig/virtualNetwork`) — the primitive cloud-dns
discovery is built on — gained the CLI (`az rest` round-trip) and Terraform
(`azurerm_app_service_virtual_network_swift_connection` against an EP1 function
app + a `Microsoft.Web/serverFarms`-delegated subnet, in the apply/idempotency/
destroy stack) coverage to join the SDK test from #587. Adding the Terraform
path surfaced and fixed three real azure-sim fidelity bugs. **BUG-1833** — the
swift response returned its resource `id`/`type` from the *operation* path
(`.../sites/{name}/networkConfig/virtualNetwork`, type
`Microsoft.Web/sites/networkConfig`) rather than the canonical *config*
sub-resource id real Azure returns (`.../sites/{name}/config/virtualNetwork`,
type `Microsoft.Web/sites/config`); terraform-provider-azurerm's Create parses
the response `id` (`*read.Model.Id`), and its parser rejects an id without a
`config` segment, so the apply failed `ID was missing the 'config' element`. The
#587 SDK test missed it (it asserted only `subnetResourceId`); the SDK + CLI
tests now also assert the returned `id` carries `/config/virtualNetwork`, so the
regression is caught in the widely-run `sim (azure)` job, not only the gated
`tf (azure)` job. **BUG-1832** — a
delegated subnet dropped the delegation's `actions` array on read-back, so an
`azurerm_subnet` with a `service_delegation` block wasn't idempotent (added an
`Actions []string` round-trip); **BUG-1831** — the swift PUT force-started a
workload container for any non-HTTP site, so VNet-integrating a site with no
container image (a plain Terraform function app) returned `500 … has no
container image` (gated the start on the site actually having a container image —
real Azure VNet integration is a pure networking-config operation; the
redis-with-image services path is unchanged).

## 2026-06-16 - bleeplab GitLab cells on the ACA + AZF backends (both GREEN) + AWS sim faithfulness

The full gitlab-runner docker-executor flow (build → artifact → `services:`)
now runs on **both** Azure backends — aca and **Azure Functions (azf)** — all 4
cell tests green on each. azf's last and hardest hurdle was **faithful cloud-dns
service discovery** so the build site resolves `redis:6379`: it is assembled
end-to-end from real Azure primitives, with the *same backend code against the
sim and real Azure* (no sim-awareness). `NetworkCreate` provisions a
`Microsoft.Network/virtualNetworks` + a subnet delegated to
`Microsoft.Web/serverFarms` + a Private DNS zone linked to the VNet
(`armnetwork`/`armprivatedns`). `ContainerStart` under cloud-dns deploys each
container as its **own** App Service site (a `services:` redis runs its raw
image; the build runs the bootstrap overlay), does App Service **regional VNet
integration** (`WebApps.CreateOrUpdateSwiftVirtualNetworkConnectionWithCheck`)
into the subnet, and registers each `--network-alias` as a Private DNS CNAME →
the site's default hostname. The azure sim realizes these faithfully: a
`Microsoft.Web/serverFarms`-delegated subnet is the App Service container fabric
→ a Docker user-defined network (not the IaaS netns the compute stack uses);
swift integration attaches the site's container to it; a CNAME → a site's
default hostname is realized as a Docker embedded-DNS alias on that site's
container (`realizeCNAMEAsSiteDockerAlias`, the App Service analog of the ACA
`realizeCNAMEAsDockerAlias`). The build site then reaches `redis` over the
shared VNet (DNS) and PING/SET/GET pass. This composes Docker's network + DNS +
services purely from Azure cloud primitives — the bar the whole project holds.

The full gitlab-runner docker-executor flow now also runs on the **Azure
Container Apps (aca) backend** — all 4 cell tests pass, including the redis
`services:` job (PING/SET/GET) over the per-build network. The runner stages
route through the bootstrap's **HTTP buffered-invoke** to the App's ingress
(like cloudrun/gcf), not the reverse-agent WebSocket: the WS exec half-opened
backend→container under the heavy per-stage container churn (gvisor/podman
port-reuse), so `agent.CollectExecWithStdin` blocked forever. The azure sim
implements **faithful ACA ingress** (`registerContainerAppsIngress`): a
`WrapHandler` that reverse-proxies any request whose Host matches an App's
`latestRevisionFqdn` to that App's running replica on its configured ingress
`targetPort` — exactly how real ACA routes an App FQDN to the container, the
same virtual-host shape as the storage data-plane and Functions invoke. The
backend reaches it via the `EndpointURL` coordinate + the FQDN Host header,
differing from real ACA only in that coordinate (no sim-specific endpoint).

Also landed: a backend WebSocket **keepalive** on `agent.ReverseAgentConn`
(ping ticker + `SetPongHandler`/read-deadline, refreshed on pong+data) that
detects a half-open reverse-agent connection and closes it instead of hanging
forever — a real robustness fix for every FaaS backend; the cloudrun cell
re-verified green. The `bleeplab-runner-docker-build` Makefile target now uses
`docker build --load` (the default `docker-container` buildx driver was
otherwise leaving `bleeplab-runner-int:local` STALE — every `docker run` silently
re-ran an old image). A run of aca-cell hurdle fixes (arch, cache-init-via-agent,
azure-files volumes, sim umask/SELinux, cloud-truth NetworkInspect, stdin-attach
precedence, `/bin/sh` stage) precede the green.

**azf cell — WIP (BUG-1828):** the overlay base-ref (BUG-1826) and the azure
sim's AZF-invoke-reach-by-container-bridge-IP (BUG-1827) are fixed, so the
cache-init one-shot runs, but the runner pattern needs a *persistent* workload
container while the azf FaaS invoke is ephemeral (a container per
`/api/function`, removed after) — so gitlab-runner's later `docker exec` hits
"No such container". The fix is to run the App-Service site container
persistently (the provision pins an EP1/Premium = always-on plan) like an ACA
App and route invoke/ingress/exec to it.

**AWS sim faithfulness (#583/#569, BUG-1827):** ECS now applies the advertised
task/container CPU/Memory to the launched container's cgroup (`HostConfig`
`Memory`/`NanoCPUs` from `ecsContainerResourceLimits`), so a Fargate task is
actually bounded the way its `/task` metadata reports; the process-mode
managed-EBS path was hardened so `ebsRemoveDockerVolume` never dereferences a
nil Docker client. SDK probes `TestECS_TaskDefinitionFidelitySDK` +
`TestECS_ManagedEBSRunTaskProcessMode` (and the azure ACA App SDK suite) stay
green.

## 2026-06-16 - bleeplab GitLab cell on the Cloud Run Functions (gcf) backend (GREEN)

The full gitlab-runner docker-executor flow now also runs on the Cloud Run
Functions backend. A real `gitlab-runner` registers against the bleeplab
control-plane simulator and runs the same 3-stage pipeline through a docker
executor whose `--docker-host` is `sockerless-backend-gcf`: **build** (gcc-
compiles `calc.c`, self-test + `6 x 7 = 42`), **test** (consumes the build's
`calc` artifact with no recompile, `sum 1..100 = 5050`), and **integration** (a
`services:` redis container reached by alias over the per-build network-pod —
`redis-cli` PING/SET/GET). All 4 harness assertions pass. gcf reuses the gcp
simulator and the cloudrun backend's `gcp-common`; the redis `services:` job
exercises the BUG-1781 network-pod (one multi-container Cloud Run revision)
assembly — no BUG-964 default-invoke gate was hit.

The harness gains a `gcf` arm: `bleeplab/Dockerfile` builds
`backends/cloudrun-functions` → `sockerless-backend-gcf` + the
`sockerless-gcf-bootstrap`; `run-integration.sh` gains `provision_gcf` (mirrors
`provision_cloudrun` with `SOCKERLESS_GCF_*` coordinates + `/v1/gcf/reverse`,
**without** `SOCKERLESS_GCR_USE_SERVICE` — gcf runs the native multi-container
revision, not a kept-alive Service); the Makefile gets
`bleeplab-runner-docker-test-gcf`.

Validation surfaced and fixed four real backend/bootstrap bugs — each one a
place where the gcf network-pod execution model diverged from a mechanism the
green cloudrun cell already had:

- **BUG-1811** — `ContainerStart` resolved only from `PendingCreates`, so the
  gitlab-runner per-stage start→wait→stop→start cycle failed `NOT FOUND` once a
  container left PendingCreates. Now falls back to `ResolveContainerAuto`
  (CloudState) and re-adds it, mirroring cloudrun.
- **BUG-1812** — `ContainerAttach` routed a gitlab-runner stdin-script to the
  reverse-agent (whose router has no main process in reverse mode — `mp==nil`),
  failing `no main process to attach to`. The network-pod bootstrap registers a
  reverse-agent for every member, so the stdin path now takes precedence over
  the reverse-agent routing.
- **BUG-1813** — the captured attach-stdin script was piped to the image's own
  entrypoint (`gitlab-runner-build`, which ignores a raw script) instead of a
  shell, so `get_sources` ran but never cloned. Now overrides
  `invokeArgv=[/bin/sh]` when stdin is captured, matching cloudrun's
  `postBootstrap`.
- **BUG-1814** — a reused gcf function instance restored its persist (gcs-
  snapshot) `/builds` only at startup, so `upload_artifacts` couldn't see the
  build container's `calc` (and its stale save clobbered the build's snapshot).
  The bootstrap now `restoreAll(persistVols)` before every invoke; cloudrun gets
  this free via `UseService` cold-starting a fresh per-stage instance.

Cross-cutting: the gcf BackendDescriptor architecture is now derived from
`config.BuildPlatform` via a shared `gcpcommon.ArchFromPlatform` (cloudrun's
local helper promoted to `gcp-common`, used by both backends), and the gcp
simulator's gcf function-invoke path reaches the workload by bridge container IP
(the same fix as the Service path in BUG-1810), so it works when the simulator
itself runs inside the harness container.

## 2026-06-16 - bleeplab GitLab cell on the Cloud Run backend (GREEN)

The full gitlab-runner docker-executor flow now runs on the Cloud Run backend.
A real `gitlab-runner` registers against the bleeplab control-plane simulator
and runs a 3-stage pipeline through a docker executor whose `--docker-host` is
`sockerless-backend-cloudrun`: **build** (gcc-compiles `calc.c` from the cloned
repo, runs the self-test + `6 x 7 = 42`), **test** (consumes the build's `calc`
artifact with no recompile, folds `sum 1..100 = 5050`), and **integration** (a
`services:` redis container reached by alias over the per-build pod network —
`redis-cli` PING/SET/GET). Reproducibly green.

The harness is the one-image, `BLEEPLAB_BACKEND`-switched shape that bleephub
proved: `bleeplab/Dockerfile` now also builds `simulator-gcp` +
`sockerless-backend-cloudrun` + `sockerless-cloudrun-bootstrap` (+ `openssl`);
`run-integration.sh` gains `write_fake_sa_json` + `provision_cloudrun` (GCS
buckets, fake service-account JSON whose `token_uri` is the sim's `/token`,
`gcs-sync` workspace, Cloud Build→Artifact Registry overlay through the sim's
`/v2/` published at `127.0.0.1:5000`, reverse-agent `/v1/cloudrun/reverse`); the
Makefile gets `bleeplab-runner-docker-test-cloudrun`. The CI job markers were
made backend-neutral (`BLEEPLAB-{BUILD,TEST,SERVICE}-OK`) so one CI config
covers every backend. gitlab-runner has no `/runner/externals` tree (that's
github-runner only), so the GitHub externals volume was dropped.

**Validation surfaced + fixed three real bugs** (each grounded in evidence from
the harness, not assumption):

- **BUG-1808** — the cloudrun backend hardcoded `Architecture: "amd64"` in its
  docker `/version`, so on an arm64 host gitlab-runner chose the wrong
  (`x86_64`) helper image. Now derived from `config.BuildPlatform` via
  `archFromPlatform`, mirroring how ECS reports the *workload's* arch.
- **BUG-1809** — the gcp sim's Artifact Registry pull-through hydrated only
  `docker-hub` images from the local daemon, but the backend rewrites
  `registry.gitlab.com/<path>` → `<AR>/gitlab-registry/<path>`, so the
  `gitlab-runner-helper` image 404'd. `hydrateOCIImageFromLocalDocker` now also
  maps `/gitlab-registry/` → the local `registry.gitlab.com/<path>` ref.
- **BUG-1810** — the gcp sim's Cloud Run Service one-shot invoke dialed
  `127.0.0.1:<hostPort>`, which is unreachable when the sim runs *inside* the
  harness container (the host-published port binds the host's loopback, not the
  sim container's). The sim now reaches the workload by its **bridge container
  IP:8080** (routable container-to-container), falling back to the host port
  for a sim running directly on the host. A new bootstrap-stdout-on-failure
  diagnostic (`start_service.go`) made the opaque permission-container exit
  diagnosable. bleephub never hit this — its github-runner containers are
  exec-driven (reverse-agent), never one-shot Service invokes; gitlab-runner's
  cache-volume permission container is the first to exercise the path.

## 2026-06-15 - AZF pod polish (shared volume + per-sidecar exec) + bleeplab artifact UI

Three follow-on enhancements after the BUG-1781 AZF pod assembly shipped.

**AZF pod shared-workspace volume.** A GitHub `services:` / GitLab services job's
members need a shared workspace. `materializePodSite` now dedups every member's
translated named-volume binds into one set, attaches each as a site-level Azure
Files share (`UpdateAzureStorageAccounts`), and declares each member's binds as
the sitecontainer's `SiteContainerProperties.VolumeMounts`. The azure sim
realizes a VolumeMount as a per-(site, volume) Docker named volume bound into
every member (`HTTPContainerConfig.Binds` for the main, `ContainerConfig.Binds`
for sidecars), so members mounting the same volume share one workspace — the pod
analog of an ECS task's shared task-level Volumes. The volume persists across
stages and is torn down when the site is deleted (`cleanupSiteContainers`).

**AZF per-sidecar exec.** Sidecars previously ran their RAW service image with no
agent, so `docker exec <sidecar>` failed. They now run the overlay in *sidecar
mode* (`SOCKERLESS_SIDECAR=1`): the bootstrap dials its own reverse-agent (keyed
to the sidecar's container ID) and execs the service as a long-lived foreground
subprocess WITHOUT binding the function HTTP port (the main owns it in the shared
netns). Because `--network container:` shares `/etc/hosts`, the sidecar resolves
`host.docker.internal` from the main and dials back successfully. So
`docker exec <sidecar>` now works, mirroring Cloud Run (where every pod member
registers an agent). A raw image remains the no-overlay fallback (no per-sidecar
exec). `isAZFOverlaid` detects both on-the-fly and pre-built overlay images.

**bleeplab artifact browse UI.** The GitLab-themed bleeplab dashboard gains an
Artifacts panel on the JobDetail page: `jobView` now carries `artifact_filename`,
a new unauthenticated `GET /internal/jobs/{id}/artifact` route streams the
archive with a `Content-Disposition` filename (the runner-facing
`/api/v4/jobs/{id}/artifacts` stays JOB-TOKEN gated), and the page shows the
filename + size with a Download link when the job produced an artifact.

Proven: `TestAZFMultiContainerPodSharesLocalhost` (integration) now also asserts
a marker file written by the sidecar to the shared `/shared` volume is readable
by the main, and `docker exec` into the sidecar returns its output;
`TestArtifactFlow` asserts the internal job view's `artifact_filename` and the
unauthenticated internal download; the bleeplab UI builds + typechecks + its
vitest passes. `sidecarRunSpec` / `isAZFOverlaid` / pod-volume helpers carry unit
tests.

## 2026-06-15 - FaaS multi-container pod assembly (BUG-1781)

Full pod semantics — including localhost / shared-loopback networking between
members — on the FaaS backends, so GitHub `services:` / sidecar `container:`
jobs and GitLab service containers run there.

**Investigation: the premise was partly stale.** Verified against code, **lambda
and gcf already deliver shared-localhost pods**: lambda runs all pod members as
chroot subprocesses of one supervisor inside a single Lambda execution
environment (one shared netns → `localhost` works); gcf co-deploys members into
one multi-container Cloud Run revision and injects `/etc/hosts` alias→127.0.0.1.
The only backend that still hard-rejected multi-container pods was **azf** — the
gap this work closes.

**azf assembles the pod as ONE App Service site with `sitecontainers`** — the
native Azure multi-container primitive (`Microsoft.Web/sites/{name}/sitecontainers`:
one `isMain` container + N sidecars sharing a network namespace), the Azure
analog of an ECS multi-container task / Cloud Run multi-container revision.
Confirmed real across every surface the testing contract requires: the
`armappservice/v5` SDK (`CreateOrUpdateSiteContainer`/`Get`/`List`/`Delete`),
the `az webapp sitecontainers` CLI, and the vendored `web-arm-openapi-2025-03-01`
spec.

- **azure simulator** (`simulators/azure/sitecontainers.go`): models the
  `sitecontainers` ARM sub-resource (CRUD). `invokeAzureFunctionHTTP` starts the
  `isMain` member as the long-lived HTTP container, then each sidecar with
  `NetworkMode: container:<main>` so they share one netns (a sidecar binding a
  port is reachable from the main on `localhost:<port>`) — mirroring the ACA
  multi-container path. A `startUpCommand` carries an argv across the
  string-typed Azure field via shell-quoting (backend) + a quote-aware splitter
  (sim), so an embedded `sh -c '<script>'` survives. SDK + CLI tests; the
  shared-localhost guarantee is proven by
  `TestSDK_AzureFunctions_MultiContainerSharesLocalhost`.

- **azf backend** (`network_pod.go` + `pod_site.go`): the two fail-fast
  rejections (`PodStart`, `ContainerStart`) are replaced with a network-pod
  materializer mirroring gcf's `shouldDeferOrMaterializeNetworkPod` (pure Docker
  signals: user-defined network membership + `Container.Config.OpenStdin`).
  `materializePodSite` creates one site whose `isMain` sitecontainer runs the
  reverse-agent overlay (the runner execs into it) and whose sidecars run their
  **RAW service images** — sidecars must NOT run the overlay, which would bind
  the main's HTTP port in the shared netns; the pre-overlay image + entrypoint
  are stashed in a container label at create time. Cloud-state reconstructs
  every member from a `sockerless-pod-members` site-tag manifest (stateless — no
  local map). `ContainerCreate` defers site creation for networked containers.

- **azf bootstrap** (`agent/cmd/sockerless-azf-bootstrap`): writes
  `SOCKERLESS_HOST_ALIASES` to `/etc/hosts` (mirror of gcf's `writeHostAliases`)
  so a sibling resolves by name to the shared loopback.

Proven end to end: `TestAZFMultiContainerPodSharesLocalhost` runs a
GitHub-`services:`-shaped pod (job container + a service sidecar) on the azf
sim — the job reaches the sidecar both on `localhost:9099` AND by alias `svc`
over the shared netns. Single-container azf paths re-verified green after the
`ContainerCreate`/`ContainerStart` refactor.

## 2026-06-15 - Cloud Map completeness: one instance, many DNS names (BUG-1804)

Real AWS Cloud Map registers an instance *per service* (`ServiceId`+`InstanceId`),
so one task (one IP) may back several services — i.e. resolve under several DNS
names (verified against the vendored `specs/cloud-api/aws/servicediscovery.smithy.json`
model). The sockerless stack only supported ONE name per container, so a GitLab
`services:` alias (gitlab-runner attaches a service container with network alias
`redis`) never resolved. Two layers were completed:

**aws simulator** — the Docker-network DNS realization connected the task
container to the namespace network with a single alias via a plain
`NetworkConnect`, which Docker rejects on the second registration ("network is
already connected" — verified by hand) so the second `RegisterInstance` 500'd
and only one name resolved. `handleCMRegisterInstance` now stores the instance
first, then re-attaches the container with the FULL set of service names it
backs via disconnect-then-reconnect (the same pattern azure's ACA multi-CNAME
path already uses; multiple aliases per endpoint all resolve, verified);
`DeregisterInstance` re-realizes the reduced set. The netns/`/etc/hosts` tier
already aggregated every name, so only the Docker-network tier needed the fix.

**ECS backend** — `ContainerCreate` dropped the request's
`NetworkingConfig.EndpointsConfig[net].Aliases`, and Cloud Map registration used
only the container hostname. It now captures the aliases into
`EndpointSettings.Aliases` and registers the container under its hostname AND
every alias; `deregisterInstance` enumerates the namespace's services (a
container may back several) rather than the old 1:1 container→service mapping
that leaked the extra registrations.

Proven by `TestECS_MultiServiceDNS` (a client task resolves BOTH of a server's
two service names — `web` and `webalias` — via real Cloud Map DNS in Docker) +
`TestDedupeNonEmpty`/`TestCloudMapNamesFor`; the existing `TestECS_CrossTaskDNS`
still passes.

**BUG-1805 — full gitlab-runner `services:` support (resolv.conf wrapper
removed).** With the alias registered (BUG-1804), a `services:` job's build
container still couldn't resolve `redis`. Root cause (verified on Podman, the
local runtime): each network's DNS runs at its gateway and a container gets one
nameserver per attached network, added as networks connect. The ECS backend
wrapped the user's container command in a `/bin/sh` shim that rewrote
`/etc/resolv.conf` to a STATIC snapshot at entrypoint time (to inject the
namespace as a DNS search domain) — capturing only the VPC nameserver and
dropping the namespace network's DNS that the runtime adds when Cloud Map
connects the container *after* it starts. The wrapper also mangled the user's
argv. Fix, respecting module boundaries: **remove the backend's resolv.conf
command-wrapper** (the container argv runs verbatim; DNS is the runtime's), and
have the **sim** realize each service name as BOTH `<service>` and
`<service>.<namespace>` network aliases (both verified to resolve), matching the
netns/`/etc/hosts` tier — so no search domain is needed. The now-dead
`searchDomainsForContainer`/`shellQuoteArgs` helpers + tests were removed.
Validated end to end: the bleeplab GitLab ECS harness runs a 3-stage pipeline
whose integration stage `apk add redis` + `redis-cli -h redis` PING/SET/GET all
succeed over the per-build pod network (TEST 4).

## 2026-06-15 - bleeplab dashboard UI (GitLab-themed) — completes bleephub parity

bleeplab now ships an embedded dashboard UI, the last piece of bleephub parity
(git + artifacts + UI). It's a React 19 / Vite 6 / Tailwind 4 SPA at
`ui/packages/bleeplab/`, built and `//go:embed`ed into the bleeplab binary
exactly as bleephub's is: `bleeplab/ui_embed.go` (`!noui`, `//go:embed all:dist`,
`spaHandler` mounted at `/ui/`) + `bleeplab/ui_noembed.go` (`noui` no-op),
`UI_PACKAGE := bleeplab` in `bleeplab/Makefile` driving the dist-copy in
`make/go-app.mk`, registered in the root Makefile's `GO_UI_APPS` + `UI_APPS`. `/`
redirects to `/ui/`; deep links fall back to `index.html`; the headless runner
harness Dockerfile builds `-tags noui`.

Views (React Router): **Overview** (status metrics + git/artifact storage backend
+ recent pipelines), **Projects** (+ detail with the project's pipelines),
**Pipelines** (+ detail rendered as a GitLab-style stage graph — one column per
stage, status-coloured job cards, artifact sizes), **Job** detail (ANSI trace via
ui-core's LogViewer), **Runners**. It polls every 5s (React Query) and reuses the
shared `@sockerless/ui-core` primitives (AppShell-less custom Shell, DataTable,
StatusBadge, MetricsCard, ThemeToggle, ErrorBoundary).

The UI is fed by a new **read-only `/internal/*` aggregation API** in bleeplab
(`internal_api.go`) — typed view structs (not `map[string]any`) over the
in-memory control-plane state: `/internal/{status,projects,pipelines,
pipelines/{id},jobs/{id},runners,storage}`. Resource detail still comes from the
public `/api/v4` GitLab surface; `/internal` only adds the dashboard projections
(e.g. "every pipeline across every project") with no clean public-API
equivalent. Tested in-process (`TestInternalAPI`) + a UI unit test.

**Theme** (the explicit ask — "approaching the colour schemes of actual
GitLab", distinct from bleephub): same shared design-token contract, GitLab
values — an indigo/purple action accent (`#6E49CB`), the iconic tanuki orange
(`#FC6D26`) as the brand highlight (wordmark + artifact badges), GitLab Pajamas
status greens/reds/oranges, and a purple-tinted dark mode. bleephub stays
neutral-gray + teal, so the two sims are unmistakable at a glance.

## 2026-06-15 - bleeplab object-store-backed CI artifacts (cross-stage passing)

bleeplab now stores and serves CI job artifacts, object-store-backed exactly
like its git storage (and bleephub): an `artifactStore` chosen by env — an
S3-compatible object store, a filesystem dir (`BLEEPLAB_ARTIFACTS_DIR`), or
in-memory — behind the real GitLab runner endpoints `POST /api/v4/jobs/:id/
artifacts` (multipart upload) and `GET /api/v4/jobs/:id/artifacts` (download).
The CI parser now reads per-job `artifacts:` (name/paths/when/expire_in/
untracked) and `dependencies:`; the job response advertises the upload spec and
a typed `dependencies` list — every earlier-stage job that produced an artifact,
with its size and a download token (an explicit `dependencies:` restricts it,
matching GitLab). This completes the user's "storage based on object store, just
like bleephub" ask (git + artifacts).

The GitLab ECS cell now does real **cross-stage** work: build-job compiles
`calc` and publishes it as an artifact; test-job carries no toolchain, downloads
the artifact, and runs the prebuilt binary — proving the artifact round-tripped
through the store and that the test stage depends on the build stage's output.

**Coordinate finding (artifact reachability).** gitlab-runner's in-container
`artifacts-uploader`/`-downloader` use the runner's *config `url`*, not
`CI_API_V4_URL`, so that URL must be reachable from inside the job/helper
container — `http://127.0.0.1:8929` (the harness loopback) gave `connection
refused`. The fix is a coordinate: the runner config `url` is set to
`host.docker.internal:8929`, which resolves to the harness loopback from the
runner process (via `/etc/hosts`) and to the published port from the workload
containers — one URL that works from both vantage points. (bleeplab also sends
`CI_API_V4_URL` in the job variables, as real GitLab does.)

Typed over `any`: the `dependencies` entries are a `jobDependency` struct
(`id`/`name`/`token`/`artifacts_file`), not `map[string]any` in `[]any`. Tested
in-process by `TestArtifactFlow` (upload → dependency advertisement → download,
byte-for-byte) and end-to-end by the harness (TEST 3).

## 2026-06-15 - bleeplab serves git + ECS restart preserves volumes → the GitLab ECS cell builds & runs a real program (BUG-1801, BUG-1802)

The single-job bleeplab GitLab ECS cell is GREEN and does **real work**: a real
`gitlab-runner` (docker executor, `--docker-host` = sockerless ECS backend)
clones the project, then a two-stage pipeline `apk add gcc` + `gcc -O2 -Wall
-Werror -o calc calc.c` and runs a real C arithmetic calculator from the cloned
source — self-test plus verified arithmetic (`6 x 7 = 42`, `7 + 4 = 11`,
`100 / 7 = 14`, `17 % 5 = 2`, and folding `calc` over 1..100 to `5050`). Two
fixes landed it:

**BUG-1801 — bleeplab serves each project as a real git repository.** Without a
git server the harness used `GIT_STRATEGY: none`, so the runner never created
`CI_PROJECT_DIR` and `cd $CI_PROJECT_DIR` failed. bleeplab now serves git over
smart-HTTP with pure-Go **go-git** (`/info/refs` + `git-upload-pack` /
`git-receive-pack`), object-store-backed exactly like bleephub: an `s3fs`
go-billy filesystem → go-git Storer chosen by env (`BLEEPLAB_S3_BUCKET` >
`BLEEPLAB_GIT_DIR` > in-memory). A commit through the GitLab commits API writes
a real go-git commit (additive create/update on the branch) and records the real
SHA; `git_info.repo_url` points at `<BLEEPLAB_EXTERNAL_URL>/<ns>/<project>.git`
(reachable from the job/helper container via `host.docker.internal:8929`) and
`git_info.{sha,refspecs}` drive a faithful clone. The harness switched to
`GIT_STRATEGY: clone`. bleeplab accepts the runner's `gitlab-ci-token` exactly
as GitLab does — coordinate-only, no sockerless-aware special-casing. Validated
in-process by `TestGitCloneSeededProject` (a real go-git client clones; HEAD ==
commit SHA; a second commit is additive).

**BUG-1802 — the ECS backend preserves a container's volume binds across
restart.** gitlab-runner's docker executor runs each build stage by re-starting
the *same* predefined helper container (`create` once → `attach→start→wait→stop`
per stage). On ECS each `/start` spawns a fresh deferred task; the **first**
start carried the `/builds` EFS mount (resolved from `PendingCreates`, which
holds `HostConfig.Binds`), but **later** starts resolved the container from cloud
state via `taskToContainer`, whose reconstructed `HostConfig` dropped `Binds`
entirely — so the re-registered task def had no volume mounts and `get_sources`
cloned into ephemeral storage the build container couldn't see (`cd
/builds/root/demo` → "No such file or directory"). Diagnosed with per-container
DIAG logging: all stage containers resolved the *same* access point, yet the sim
showed two helper tasks with `mountPoints=0 binds=[]`. Fix (backend-side,
stateless): `taskToContainer` reconstructs the named-volume binds from the task
definition's mount points (`SourceVolume:ContainerPath[:ro]`), so every restart
re-registers a task def with the original volumes. Unit-tested
(`TestTaskToContainer_BindsFromMountPoints`). No sim change — the sim already
shares an access point's host dir across tasks deterministically.

(Earlier on this branch the "volume not shared across stages" theory was a
stale-data-dir artifact: the harness `rm -rf` couldn't clear root-owned EFS
files from prior runs, so disk archaeology saw cross-run filesystems. The
authoritative diagnosis came from logging the *resolved* access point and the
*applied* binds, not from inspecting the accreted data dir.)

Still open for full bleephub parity (the user asked for both): a bleeplab
artifact **ArtifactStore** (object-store-backed) and a bleeplab **UI** — the
git/object-store slice is the piece that landed here.

## 2026-06-15 - aws sim EFS access-point writability for GitLab workloads (BUG-1800)

After BUG-1798, the bleeplab GitLab ECS build `step_script` failed at
`mkdir: can't create directory '/builds/project-1.tmp': Permission denied`. Two
sim-side EFS gaps, both fixed:

1. **CreationInfo ignored.** `EFSAccessPointHostDir`/`EFSFileSystemHostDir`
   created the host dir with `os.MkdirAll(…, 0o777)`, whose mode the umask
   reduces to `0755 root` — and the access point's `RootDirectory.CreationInfo`
   (the gitlab `/builds` volume requests `0777`, uid/gid 1000) was never applied.
   New `ensureAccessPointRootDir` applies `CreationInfo.{Permissions(chmod, not
   umask-masked), OwnerUid, OwnerGid(chown, best-effort)}` on creation only (so a
   workload's later perm changes aren't clobbered), defaulting to `0777`.
   Unit-tested (`efs_creationinfo_test.go`).
2. **SELinux.** On an SELinux-enforcing host (a local podman machine) the
   sim-spawned ECS task runs confined as `container_t` and can't write the EFS
   host dir even at `0777`. The sim now mounts task EFS binds with the `z`
   (shared relabel) option → relabels them `container_file_t`; a no-op on hosts
   without SELinux (Docker on CI), so it removes the bleephub harness's manual
   `chcon` note for the bleeplab path.

Validated on a frozen stack: the access-point dir is now `drwxrwxrwx 1000 1000
… container_file_t`, the `/builds` write succeeds, and the cell advances past
the permission error to the next gate — BUG-1801, where the `/builds` volume
doesn't persist across the per-stage Fargate tasks (`cd /builds/project-1` → No
such file or directory; the same docker volume resolves to a different EFS
access point per task). aws sim ECS/EFS SDK tests stay green. No backend
coupling.

## 2026-06-15 - ECS gitlab-runner attach-stdin gate closed (BUG-1798)

Fixed the Phase-3 gate for the bleeplab GitLab ECS cell. gitlab-runner 18's
docker executor does `create(OpenStdin) → /attach(stdin) → /start` (no
`docker exec`) and pipes the stage script to the helper container's stdin (its
default `gitlab-runner-build` reads it). On ECS the `/start` deferral that bakes
the captured stdin into the task command requires the stdin pipe to already
exist + be open — but `ecsStdinAttachDriver.Attach` created the pipe only
**after** a stage-boundary barrier that itself **waits for `/start`** to
register a WaitCh. A dependency inversion: `/start` arrived first, found no pipe
(DIAG: `pipe_exists=false`), fell through, and launched the helper's image-
default command, which hung forever waiting for stdin.

Fix (backend-side, no sim coupling): create + open the stdin pipe **before** the
barrier in the attach driver, and have `ContainerStart` wait briefly
(`waitForOpenStdinPipe`, 5s) for the open pipe before deciding — closing the
create→attach→start race from both ends. Root-caused with temporary DIAG logs
(removed) that captured the exact ordering. Validated: the harness now runs the
helper stages and delivers the script into the build container — the hang is
gone; it advances to the build `step_script`, blocked next only by BUG-1800 (the
aws sim doesn't apply EFS access-point `CreationInfo`, so the `/builds` volume is
`0755 root` and the build job can't write — the next, sim-side gate).

## 2026-06-14 - bleeplab ECS harness + arch-aware image pull (Arc 3 Phase 3, WIP)

Phase 3 points a real `gitlab-runner` 18.11's docker executor at a sockerless
backend. New `bleeplab/Dockerfile` + `bleeplab/test/run-integration.sh` +
`make bleeplab-runner-docker-test-ecs`: the harness provisions a sim-backed ECS
backend (the bleephub `provision_ecs` shape), starts bleeplab, registers the
runner with `[runners.docker] host = tcp://…:3375`, triggers a pipeline, and
asserts success. The runner registers, claims a job, uses sockerless as its
docker host, and image pull + build/helper container create all work.

**BUG-1797 (fixed) — arch-aware image manifest selection.** `core/registry.go`
hardcoded `linux/amd64` when picking from a multi-arch manifest list. The local
sims run workloads on the host engine, so on arm64 hosts the workload is arm64 —
fine for multi-arch images (alpine), but the gitlab-runner-helper `arm64-…` tag
is arm64-only, so the amd64-only selection failed the pull. Fix: select the
manifest matching `SOCKERLESS_WORKLOAD_ARCH` (default amd64 — live unchanged),
falling back to amd64 before erroring; the policy is extracted to
`selectPlatformManifest` and unit-tested. The harness sets the env from `uname`.

**BUG-1798 (open) — the Phase-3 gate.** With the arch fix, the runner reaches
`Preparing environment` and hangs: modern gitlab-runner 18 does
`create(OpenStdin) → attach(stdin) → start` (no `docker exec`) and pipes the
stage script to the helper's stdin, but the ECS deferred-RunTask runs the
helper's image-default `gitlab-runner-build` instead of baking the captured
stdin, so it waits for stdin forever. The next iteration debugs the ECS
attach-stdin deferral for this gitlab-runner-18 helper shape (the path was built
for `docker run -i sh`).

**BUG-1799 (fixed) — proactive: a dangling `sim (aws sdk)` flake.** The PR's CI
surfaced an intermittent `TestECS_TaskArithmetic*` failure (container `ExitCode
-1`) that re-ran green. Root cause: the awsvpc netns-tier `busybox` **pause
container** image was pulled lazily at RunTask time, making a transient
ECR-gallery throttle a per-task lifecycle dependency, recorded only in the task
`StoppedReason` and surfaced as an opaque `-1`. Fixed by pre-pulling busybox in
`TestMain` with retry (the established pattern; busybox backs many ECS tests) +
logging the start failure to stderr so any residual netns flake is diagnosable.
Sim/test-side only — respects the hard sim↔backend code-isolation rule.

## 2026-06-14 - bleeplab: GitLab control-plane simulator (Arc 3 Phase 1)

Started Arc 3 (GitLab docker-executor parity) with the missing piece the
scoping identified: a **GitLab control-plane simulator**, `bleeplab` — the
GitLab analog of `bleephub`. The backend docker-executor attach-stdin path was
already built and proven (GL-1…GL-11 closed); what was absent was a control
plane a real `gitlab-runner` could poll (existing GitLab harnesses used a 4 GB
`gitlab-ce` container, real gitlab.com, or `gitlab-ci-local` which bypasses the
runner API).

`bleeplab` (new module, `cmd/main.go` on `:8929`) implements the real GitLab
API slices a docker-executor runner + orchestrator exercise: the **runner API**
(`POST /api/v4/jobs/request`, `PATCH/PUT /api/v4/jobs/:id`, runner verify/
register/unregister) and the **project/pipeline API** (projects, commits,
pipeline trigger, pipeline/job status, job trace), plus a minimal
`.gitlab-ci.yml` parser (stages, image, script lifecycle, services, variables)
with a stage-gated job queue — the next stage enqueues only after the previous
one succeeds. Fidelity, not fakery: the runner authenticates + polls exactly as
against gitlab.com; bleeplab differs only in coordinates.

Validated end-to-end with a real `gitlab-runner` 18.11.3: it registers, claims a
job via `/jobs/request`, pulls the helper + alpine images, runs the script
(`echo` + `cat /etc/os-release`) on the docker executor, streams the full CI
trace back, and the pipeline rolls up to `success`. Fixed one wire-shape bug
the real runner caught: the job-request `features` object is mixed-type
(`trace_sections` bool vs `failure_reasons` list) so it must be `map[string]any`.
Unit test `TestFullPipelineLifecycle` drives the whole control-plane + runner
flow in-process. Registered in `go.work` + the `core-local` CI shard. Next:
point the runner's `--docker-host` at a sockerless backend (Phase 3).

## 2026-06-14 - GCF (Cloud Run Functions) cell GREEN + exec-via-agent observability (BUG-1795/1796)

The bleephub **gcf** harness now passes **TEST 1–14** against the gcp simulator —
all four container backends (ECS, ACA, Cloud Run, GCF) are GitHub-topology
sim-proven. GCF Gen2 deploys container-jobs as Cloud Run Service revisions, so
the cell reuses the cloudrun overlay + gcs-sync model and the gcp sim needed no
change.

**BUG-1795 — GCF cell bring-up.** Five gaps, each the GCF twin of a cloudrun
fix: (1) the GCF `Typed.Exec` was wired to the bare `ReverseAgentExecDriver`, so
the HTTP exec path (`handleExecStart → Typed.Exec`) bypassed `s.ExecStart` and
its materialize/gcs-sync logic was dead code — rewired through `s.ExecStart` via
`WrapLegacyExecStart`; (2) added `materializeDeferredNetworkPodForExec` (a
no-`services:` job is deferred at ContainerStart); (3) added `warmBootstrap`
(BUG-1794 twin — cold-start the scale-to-zero Service via `/_sockerless/ready`
without running the keepalive); (4) the gcf bootstrap gained the readiness route
+ WS-exec gcs-sync `ExecHooks` (`ServeReverseAgentWithExecHooks`); (5) the gcf
bootstrap's `persist.go` now honours `STORAGE_EMULATOR_HOST` (`gcsBase`/
`gcsAuthToken`/`setGCSAuth` — the #568 prereq was never ported; the workload's
metadata-token fetch 404'd) and the backend injects it (`SOCKERLESS_GCS_WORKLOAD_ENDPOINT`)
+ runs a `gcsSyncPreExec`/`execPostHook` data plane. Plus harness plumbing
(`provision_gcf` + `gcf` case, `bleephub-runner-docker-test-gcf`, both GCF
binaries in `bleephub/Dockerfile`). Coordinate-only, no `if sim` branch.

**BUG-1796 — exec-via-agent observability.** The GCF bring-up exposed that a
reverse-agent `TypeError` (e.g. a failed gcs-sync restore PreExec hook) was
swallowed by `ReverseAgentConn.bridge` — the step failed with an opaque
`exit 255` and the real cause (`metadata token status 404`) lived only in the
workload's own stderr, a separate stream with no exec-ID correlation. Fixed
across the shared core + agent (so every FaaS backend benefits): `bridge` writes
the agent error to the caller's stderr frame and returns `AgentExecErrorExitCode`
(255); `BridgeExec` fails fast if the initial dispatch send fails;
`core/handle_exec.go` logs exec dispatch (with the resolved driver via
`Describe()`) + completion; `ReverseAgentExecDriver.Exec` logs dispatch + exit
code; `HandleReverseAgentWS` logs missing-`session_id`/upgrade/register/replace/
drop; the bootstrap logs serve-loop start/teardown + malformed messages, checks
its final exit-frame send, and a nil-safe `OnDroppedMessage` callback surfaces
full-channel drops. Pure-additive logging except the `TypeError`→stderr+255
behavior fix.

## 2026-06-14 - Cloud Run GitHub-topology cell GREEN (BUG-1794 + BUG-1792 closed)

The bleephub cloudrun harness now passes **TEST 1–14** end-to-end against the
gcp simulator — the Cloud Run container backend is sim-proven for the full
build→push→pull→deploy→materialize→reverse-agent→exec→gcs-sync pipeline, joining
ECS and ACA. Two bugs closed.

**BUG-1794 — the exec-driven Service never cold-started.** A GitHub container
job is materialized as a scale-to-zero Cloud Run Service whose keepalive
(`tail -f /dev/null`) must not run as a request. The materialize path therefore
*skipped the default-invoke entirely* — but a scale-to-zero Service that never
receives a request never creates its first instance, so the overlay bootstrap
never started and never dialed the reverse-agent, and `docker exec` hung. Fix:
the overlay bootstrap serves a `/_sockerless/ready` route (HTTP 204, runs no
user command) and the backend POSTs to it (`warmBootstrap` in
`start_service.go`) to cold-start the revision *without* running the keepalive.
The gcp sim's `/v2-services-invoke/` handler forwards the request path + query
to the bootstrap (a `{path...}` route variant + path/query params on
`postCloudRunServiceInstance`) so the readiness route reaches the bootstrap
instead of collapsing to `/`. Covered by `TestSDK_CloudRunV2Services_ForwardsRequestPath`
(new `echo-request` probe mode) and `TestHandleReady_DoesNotRunDefaultCommand`.

**BUG-1792 — gcs-sync validated.** With the data plane wired (#570), the last
gap was the resumable-upload continuation URL: the sim emitted `Location:`
hardcoded to `https://`, so the official Go storage client — pointed at the
explicit HTTP sim coordinate — followed it and failed `server gave HTTP
response to HTTPS client`. Fix: derive the continuation-URL scheme from the
request (`requestScheme`: `X-Forwarded-Proto` / `r.TLS`), matching real GCS
(HTTPS) and a custom HTTP coordinate alike. Also added the JSON-API `alt=media`
object download, resumable-chunk-via-`POST` (`upload_id`), and `bytes */<total>`
Content-Range parsing the storage client uses. Proven by
`TestGCS_ResumableWriterFollowsCustomEndpoint` + `TestGCS_JSONAPIObjectGetAltMedia`
(official Go storage SDK) and the TEST 12 workspace round-trip (`proof.txt`
written inside the job container is visible in the runner workspace).

## 2026-06-14 - Cloud Run gcs-sync prerequisites + BUGS.md count correction (BUG-1792 partial)

Investigating the last cloudrun-cell TEST 12 gate (every `docker exec` aborts
at exit 255) showed BUG-1792 is bigger than a hardcoded URL: the gcs-sync
per-exec workspace data plane (`GCSSyncDriver.PreExec`/`PostExec`) has **no
callers** — the cloudrun exec path never uploads the workspace to GCS or feeds
the bootstrap a `SOCKERLESS_SYNC_VOLUMES` hint, so the workspace tmpfs stays
empty and the exec's workdir doesn't exist. Cloud Run container-jobs were never
proven end-to-end.

Landed the prerequisites the data-plane wiring will need: the bootstrap's
gcs-sync (`persist.go`/`persist_sync.go`) honours the standard
`STORAGE_EMULATOR_HOST` (a `gcsBase()` helper; unauthenticated emulator mode,
so no metadata-token dependency), and the cloudrun backend injects a
workload-reachable storage coordinate (`SOCKERLESS_GCS_WORKLOAD_ENDPOINT` →
`STORAGE_EMULATOR_HOST` on the task, default empty = real GCS + ADC). The
workload reaches the sim's storage through the same host-gateway/published-port
path the reverse-agent callback uses. Real cloud is unchanged.

Also corrected the BUGS.md ledger: #567 filed BUG-1789/1790/1791 into the Open
table but never struck them when it fixed them in the same PR — the header read
`1745 fixed / 7 open` instead of `1748 / 4`. The remaining BUG-1792 work (wire
`PreExec`/`PostExec` around the exec dispatch) is its own iteration.

## 2026-06-14 - Cloud Run GitHub-topology cell bring-up (partial)

Extends the bleephub GitHub-topology harness (ECS- and ACA-proven) to the
Cloud Run backend, run entirely against the gcp simulator. New
`provision_cloudrun()` cell, `bleephub-runner-docker-test-cloudrun` Make
target, and an image bundling `simulator-gcp` + `sockerless-backend-cloudrun`.
The cell shares the runner workspace via GCS snapshot-sync (gcs-sync), builds
the reverse-agent bootstrap overlay via Cloud Build, and pushes/pulls it
through the sim's `/v2/` by the registry coordinate (BUG-1785).

Six real bugs surfaced and fixed bringing the cell up — the whole pipeline
now runs against the sim: overlay **build → push → pull → deploy →
materialize → reverse-agent dial-back → step exec**.

- **BUG-1789** (gcp-common, two facets): the overlay base-image rewrite and
  the AR auth/registry-endpoint override ignored the `SOCKERLESS_GCP_AR_ENDPOINT`
  coordinate, so the overlay `FROM` and the backend's image-metadata fetch hit
  the real Artifact Registry host. `ResolveGCPImageURI` now builds the host via
  `OverlayRegistryHost`, and `ARAuthProvider` recognises the coordinate host
  (`IsOverlayCoordinateRegistry`) so registry HTTP routes through the backend's
  reachable endpoint. Real cloud unchanged when the coordinate is unset.
- **BUG-1790** (gcp sim): the AR docker-hub pull-through (`hydrateOCIImageFromLocalDocker`)
  served a manifest mixing an OCI manifest type with a Docker v2s2 config type —
  tolerated by `docker pull`, rejected by `docker build`'s `FROM`. Now full OCI.
- **BUG-1791** (cloudrun, two facets): a GH container-job with no `services:`
  was deferred forever (the network-pod defer waited for a sibling that never
  arrived). Added **materialize-on-exec** — the runner always `docker exec`s its
  job container, which lazily deploys the Cloud Run Service (bundling any
  deferred service siblings). And `startSingleContainerService` no longer
  default-invokes an exec-driven container's `tail -f /dev/null` keepalive
  (which ran it as a request and hit the request-lifetime SIGTERM); the
  `skipDefaultInvoke` flag matches the multi-container path's existing skip.

The final TEST 12 gate, **BUG-1792**, is open: the bootstrap's gcs-sync
restore/save hardcodes `https://storage.googleapis.com` and can't reach the
sim's storage from the workload, so each exec aborts at exit 255. Staged as
the next iteration (bootstrap `STORAGE_EMULATOR_HOST` + the workload→sim
published-port reachability the reverse-agent already uses). TEST 13/14 follow.

## 2026-06-14 - Faithful build→push→pull for gcp Cloud Build (BUG-1785, gcp half — closes the bug)

The gcp half mirrors the azure ACR Tasks fix below and completes BUG-1785.
The gcp Cloud Build sim built the overlay into the host's local docker
daemon and the Cloud Run / GCF workload ran that local copy — the sim's
registry never reflected the build, a non-faithful shortcut.

- The Cloud Build `push` step (`simulators/gcp/cloudbuild.go`) now does a
  real `docker push <ref>` + `docker rmi`, exactly as real Cloud Build with
  `IsPushEnabled`. The registry, not the build host, holds the image; the
  Cloud Run / GCF workload pulls it over the standard `/v2/` API.
- The overlay registry host is a **coordinate**: `gcpcommon.OverlayRegistryHost`
  reads `SOCKERLESS_GCP_AR_ENDPOINT` (default = the real
  `<region>-docker.pkg.dev`), parallel to `SOCKERLESS_AZURE_ACR_ENDPOINT`.
  The backend builds the *real* registry ref for cloud and sim alike.
- The cloudrun + cloudrun-functions integration harnesses set that coordinate
  **per-target, exactly like `endpointURL`** — to the sim's published `/v2/`
  at `127.0.0.1:<port>` (Docker auto-trusts loopback as insecure). There is
  **no `if sim` / `if target == "sim"` branch** in backend or test code: a
  sim run differs from a cloud run only in coordinates, so the client path
  is identical and the test proves the real path, not a sim-special one.
- `TestCloudBuild_FaithfulBuildPush` asserts the built image lands in `/v2/`
  and is gone from the local daemon. The `test (gcp backends)` and gcp/gcf
  faas-smoke CI jobs (which always build overlays) exercise the full
  build→push→pull round-trip against the sim.

This is the same lesson as the azure half, generalized into a rule: the
coordinate-only pattern is now documented in
[specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md)
§ "Faithful build → push → pull" and [AGENTS.md](AGENTS.md) § "A sim test
differs from a cloud test ONLY in coordinates", cross-linked both ways.

## 2026-06-13 - Faithful build→push→pull for ACR Tasks (BUG-1785, azure half)

The ACR Tasks sim built the overlay into the host's local docker daemon and
the ACA App ran that local copy — so the sim's registry never reflected the
build, and the run used a non-faithful shortcut. The user flagged it: the
sim must not rely on functionality that isn't strictly faithful to the
cloud. A first attempt to close it went wrong — it coupled the shared
workload runner directly to the registry's in-process store, a dependency
that doesn't exist in the cloud (compute pulls from the registry over the
public `/v2/` API like any client) — and was reverted.

The faithful fix keeps the sim's services agnostic:

- The ACR Tasks build now does a real `docker build` + `docker push` to the
  registry + `docker rmi`, exactly as real ACR Tasks (IsPushEnabled). The
  registry, not the build host, holds the image.
- The ACA App run pulls it over the standard registry API (the existing
  `StartContainerSync` pull path). Registry and compute talk only through
  `/v2/` — no in-process coupling.
- The backend honors a configurable ACR registry endpoint
  (`SOCKERLESS_AZURE_ACR_ENDPOINT`, a legit sovereign/custom-cloud override)
  so the harness can point the overlay ref at a reachable, auto-insecure
  endpoint.
- The harness publishes the sim `/v2/` at `127.0.0.1:5000` and (only on a
  podman machine, which unlike Docker doesn't auto-trust loopback
  registries) drops a scoped, idempotent insecure-registry entry. On Docker
  and Linux CI it's a no-op.

Validated end-to-end: ACA harness TEST 12 (container-mode job) passes with
the real build→push→pull, and the ACR Tasks SDK test asserts the built
image lands in a real registry (a throwaway `registry:2` stand-in) and is
gone from the local daemon. The gcp Cloud Build half of BUG-1785 — same
pattern, but it must thread the cloudrun/gcf overlay flows and their
integration tests — remains as a separate, larger change.

## 2026-06-13 - ACA GitHub container-job topology: TEST 12 green (BUG-1782 + BUG-1783)

Got the GitHub container-mode job (TEST 12) passing on the **ACA** backend
through the bleephub official-runner harness — the first container backend
beyond ECS to run a container job end-to-end, and the validation that the
whole ACA App-overlay + reverse-agent path works.

Two backend/harness fixes made it work, plus the `provision_aca` wiring:

- **BUG-1782:** `NewACRBuildService` (backends/azure-common) ignored
  `SOCKERLESS_ENDPOINT_URL` — its ARM + azblob clients targeted real Azure,
  so the App-overlay bootstrap-build path couldn't reach the sim. Now it
  threads the endpoint: ARM `RegistriesClient`/`RunsClient` use the
  `cloud.Configuration` override (+ `InsecureAllowCredentialWithHTTP`), and
  the blob client is resolved lazily from the storage account's advertised
  `primaryEndpoints.blob` (via `armstorage` GetProperties) — the faithful
  way to reach storage on a custom/sovereign/simulated cloud. Both
  `aca`/`azf` call sites pass `config.EndpointURL`.
- **BUG-1783:** the bleephub `Dockerfile` built `sockerless-cloudrun-bootstrap`
  + `sockerless-agent` glibc-dynamic (CGO on by default under `golang:1.25`).
  Baked into an alpine overlay they failed to exec with `No such file or
  directory` (missing dynamic loader), so the bootstrap never dialed back
  and ACA exec timed out. The canonical agent Makefile already uses
  `CGO_ENABLED=0`; the harness Dockerfile was the anomaly — now static too.

`provision_aca` now drives the App-overlay path: `SOCKERLESS_ACA_USE_APP=1`
+ ACR + a build-context blob container + an arch-matched build platform,
and pins a deterministic `<account>.blob.localhost` storage endpoint
(`SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON` + an `/etc/hosts` alias,
since `*.localhost` isn't special-cased by the container resolver). The
full chain — sim ACR Tasks builds the overlay → ACA App runs it → static
bootstrap dials back → `docker exec` job steps — runs green.

TEST 13 (service container) is the next hurdle: a sibling service App's
alias doesn't resolve from inside the job App (filed **BUG-1784**); TEST 14
(dispatcher) needs a published-port wiring fix. Then Cloud Run + GCF.

## 2026-06-13 - Azure sim ACR Tasks slice (overlay-build keystone for ACA/AZF)

Added a faithful **ACR Tasks quick-build** slice to the azure simulator —
`POST .../registries/{name}/scheduleRun` (DockerBuildRequest LRO) + `GET
.../runs/{runId}`. This is the cloud-API the Azure backends call to build
their reverse-agent bootstrap **overlay image**: `backends/aca` and
`backends/azure-functions` issue `RegistriesClient.BeginScheduleRun` with a
`DockerBuildRequest` and poll the run. The handler fetches the build
context from the sim's blob storage (where the backend's azblob upload
landed), runs `docker build` on the host engine — the sim's build
infrastructure, exactly as the GCP Cloud Build slice
(`simulators/gcp/cloudbuild.go`) does — and tags the image into the local
daemon, where `StartContainerSync` resolves it by tag without a registry
pull. The run completes synchronously and is returned as a 200 with a
terminal-state `Run` body, which the azure-core LRO poller resolves via its
NopPoller path (a 202 would be a hard error for a POST LRO). No
sockerless-aware special-casing: any ACR Tasks client reaching `scheduleRun`
gets the same behavior. SDK tests (`acr_tasks_test.go`) exercise the full
path — upload context, BeginScheduleRun, assert the image is really present
in the local daemon, GetRun round-trip, and a missing-context build that
reports the Run as `Failed`.

Standing up the ACA topology cell on this slice surfaced **BUG-1782**:
`NewACRBuildService` (backends/azure-common) ignores `SOCKERLESS_ENDPOINT_URL`
— it builds the ARM + azblob clients against real Azure — so the App-overlay
path can't reach the sim (or any custom cloud) yet. Filed; the fix (thread
the endpoint override + target the account's advertised blob endpoint) plus
the harness wiring (UseApp, ACR, build-context container) and the
reverse-agent exec validation are the next steps of the Arc-2 ACA build.

## 2026-06-13 - All-backend metadata network driver + experiential-parity principles (Arc 2 groundwork)

Stand-up of the ACA cell of the bleephub GitHub topology harness (Arc 2)
surfaced and fixed a cross-backend defect, and the work crystallized the
principles that govern the rest of the pod + runner sweep. The harness
plumbing itself (multi-backend image, `BLEEPHUB_BACKEND` parameterization)
lands with the ACA cell in the following arc, once container-job exec is
assembled through faithful cloud APIs.

- **BUG-1780:** only the ecs backend overrode `Drivers.Network` to the
  metadata-only `SyntheticNetworkDriver`; lambda/cloudrun/gcf/aca/azf fell
  through to `BaseServer.InitDrivers`' real-Linux-netns driver (`ip netns
  add` + veth). That 400s a `docker network create github_network_<hex>`
  on a host without iproute2 and leaks a meaningless kernel netns where it
  succeeds. Docker networks on these backends map to *cloud* primitives
  (Lambda VPC ENIs, Cloud Run/GCF VPC-connector + Cloud DNS, ACA + AZF NSG
  / Private DNS), never a local netns — so all five now mirror ecs. All
  six cloud backends' test suites pass; a harness run confirmed ACA's
  `docker network create`/`delete` now provisions + tears down the NSG +
  Private DNS zone cleanly.

Codified the **experiential-parity principle** the user articulated:
sockerless backends are providers of the Docker+Podman REST API assembled
out of cloud primitives, and the goal is that a user's experience with
containers, pods, networks, and volumes inside any backend is the same as
local Docker/Podman. Every Docker abstraction is *composed* from cloud
primitives on every backend, FaaS included — including localhost /
shared-loopback networking between pod members — and a FaaS platform that
can't run multiple containers per function is our job to assemble (native
sidecars where offered, else a pod from multiple functions wired by cloud
DNS + a shared volume, with the agent proxying localhost to siblings), not
to reject. And the **sims stay faithful cloud slices** — no special / fake
functionality layered on to support sockerless backends or runners; a
backend's need is met by implementing the real cloud API, never a sim-side
hook. Written into AGENTS.md (new "Assemble Docker abstractions" section +
the cloud-API-fidelity rules-out list), CLOUD_RESOURCE_MAPPING.md
(universal rule 8), and the AZF README; filed **BUG-1781** and staged the
FaaS multi-container assembly as PLAN § Next #1 (replacing the interim
fail-fast rejections).

## 2026-06-13 - Pod-model correctness across backends (Arc 1 of the pod + runner focus)

Opened a sustained focus on the pod model and GitHub/GitLab runner
integration across all backends. Built a verified gap matrix first
(correcting the recon agents' over-claims): only Lambda is live-proven
(BUG-1075); the GitHub container-job topology is sim-proven for ECS only
(the bleephub harness); the other backends have per-backend GitLab
stdin-attach unit tests but no full-topology proof; AZF can't run
multi-container pods. Arc 1 fixed the verified pod-model bugs:

- BUG-1778: Lambda delegated all four Pod lifecycle methods, and GCF
  delegated Stop/Kill/Remove, to BaseServer.Pod* — which read
  Store.Containers and call Store.ForceStopContainer (local in-memory,
  no cloud call), so `docker pod stop/kill/rm` (and lambda `pod start`)
  left the underlying Lambda function / Cloud Run Service running.
  ECS/Cloud Run/ACA already override these to loop their cloud-aware
  Container* methods; lambda+gcf now mirror that. The isolation lint
  forbade BaseServer.Container* but not BaseServer.Pod{Start,Stop,Kill,
  Remove} — that gap is closed so the leak class can't recur.
- BUG-1779: AZF's single-invocation multi-container rejection fired
  late (only at the 2nd ContainerStart, after create succeeded);
  PodStart now rejects up front with one clear error, and the
  constraint is documented in the azf README (rules out `services:` and
  sidecar `container:` jobs on AZF).

Verified non-issue (left as-is): GCF injects pod host-aliases on the
main container only — correct, because sidecars are raw user images
that don't run the sockerless bootstrap, so only main can write
/etc/hosts; main→sidecar (the services-contract direction) works and
sidecar→sidecar-by-name isn't achievable or needed.

## 2026-06-13 - Pod-model / lifecycle review fixes (bundled into the audit PR)

Reviewed the pod + container lifecycle across all 7 backends for needless
delays, fixed timeouts, polling inefficiency, and constraint mismatches.
Verified each high-severity claim against source (the review agents
overstated several, as on the sim audit). Two genuine weaknesses fixed:

- BUG-1776: ACA `attachStream.Read` did a bare `<-respReady` with no
  deadline — the AZF twin bounds the identical buffered-attach wait with a
  deadline (the BUG-1505 fix) but it was never ported. A stalled or
  lifetime-capped ACA Job would strand an attached docker/StdCopy reader
  forever. Ported the AZF pattern (bootstrap window + job-run budget).
- BUG-1777: the recovery-path `WaitForExit` (cloudrun + aca) re-ran a full
  ListJobs+ListServices (with per-pod GetService follow-ups) on every tick
  to check one container's exit. Narrowed to resolve the backing Job once
  then poll that single job's state — the identical resolveExecutionState /
  resolveJobState derivation, one resource — with a list fallback for
  Service/App-backed and vanished jobs. Cloud Run's `waitForServiceURL`
  flat-2s/150-call loop got exponential backoff (1s→15s). GCF left alone
  (its hot path is already event-driven; FaaS has no persistent exit
  state to narrow to).

Deliberately NOT changed (verified non-issues): pod deferred-start has no
timeout but doesn't block a goroutine and mirrors docker-compose semantics;
the 30s stdin-capture waits are warn-and-proceed generous upper bounds;
the O(n) queryTasks/queryFunctions is the documented stateless-invariant
cost; the ACA app-readiness "returns before replica ready" is masked by the
reverse-agent wait in the common path and the multi-container case is
intra-revision (no cross-app DNS) — adding an unconditional revision-ready
poll would regress hot-path latency for an unproven race. The forced delays
(Fargate task-start, Lambda/GCF/AZF limits, ARM LROs) are genuinely
cloud-imposed. The hot-path exit detection (pollTaskExit/pollExecutionExit)
is already single-resource + event-driven; pod materialization already
builds member overlays in parallel and deploys atomically.

## 2026-06-13 - Sim fidelity audit pass (probe the load-bearing gaps)

Ran a registered-op-vs-test-coverage sweep across all three sims, then
applied the established discipline — "untested ≠ working; probe with
assertions" — to the gaps that are both load-bearing (a backend or
terraform actually calls them) AND complex enough to harbor a real
bug. The broad coverage maps were noisy (most "untested" ops are
out-of-slice or already had dedicated fidelity tests the op-name grep
missed — Cloud Map, EFS, ECS are well-covered), so the value was in
narrowing to what sockerless depends on. Three real fidelity bugs,
each confirmed by a real-SDK probe mirroring the exact backend call
pattern, each fixed with a permanent regression test:

- BUG-1773: AWS CreateSecurityGroup never rejected a duplicate name in
  the same VPC — the ECS per-job-network create
  (backends/ecs/network_cloud.go) relies on InvalidGroup.Duplicate to
  reuse an existing SG by name+VPC; against the sim a retry silently
  minted a second SG. Now rejects same name+VPC (different VPCs still
  reuse a name).
- BUG-1774: AWS AuthorizeSecurityGroup{Ingress,Egress} never rejected a
  duplicate rule — the backend re-applies its self-referencing ingress
  rule and swallows exactly InvalidPermission.Duplicate; the sim
  appended a second identical permission, so DescribeSecurityGroups
  read-back accumulated duplicates. Now detects an equivalent existing
  permission (protocol + ports + shared target) and 400s it.
- BUG-1775: GCP rrsets.list ignored its name/type query filter — the
  Cloud Run service-discovery path uses .Name(fqdn).Type("A"); the sim
  returned the whole zone. The backend re-filters client-side so it
  wasn't broken, but the sim diverged from real Cloud DNS. Now honors
  the filter.

## 2026-06-12 - Runner-as-cloud-task topology, sim-proven (cells 1+2 minus the live pass)

The bleephub official-runner harness became the topology proof. Its
image now bundles simulator-aws + sockerless-backend-ecs + the
dispatcher next to bleephub and the runner; the make target mounts the
host docker.sock and a sim-EFS host dir at an identical path inside
and out, so the runner's workspace IS an EFS access point and
`container:` jobs dispatched through the backend land as sim-ECS tasks
on the host engine sharing it. TEST 12 asserts the contract from the
outside: a file written inside the job container shows up on the
runner's EFS workspace. TEST 13 runs a `services:` nginx reachable by
alias from the job container. TEST 14 closes the control plane: the
github-runner dispatcher (new `--api-base`; capability-probe token
verification for header-less tokens) polls bleephub for a queued job
no resident runner can take, spawns an ephemeral runner on the host
engine, and the job completes on it — 14/14 green locally
(BLEEPHUB_TEST_FROM + BLEEPHUB_HOLD knobs make single-test iteration
cheap). Every wall on the way was a real bug, filed + fixed
(BUG-1763..1771): the runner deserializes jobServiceContainers /
object-form `container:` as TemplateTokens (plain JSON maps fail job
start; `env` not `environment`); registration must round-trip
`ephemeral` or config.sh aborts, and completed ephemeral agents now
deregister; runs with no started job reported `in_progress`, hiding
them from `?status=queued` pollers; job messages baked the submitter's
request-Host as the server URL so off-host runners could run but never
complete jobs — `BLEEPHUB_EXTERNAL_URL` is the GHES-shaped fix; the
admin token advertised a scope header without `workflow`; and
dispatcher spawns add `host-gateway` only on engines that need and
accept it. Catalog: docs/RUNNERS.md D-8..D-12.

## 2026-06-12 - GitHub-runner dispatcher hardening (ARC-without-k8s parity)

Source audit of `github-runner-dispatcher-{aws,gcp,azure}` against their
contract (poll → mint token → spawn ephemeral runner → GC from cloud
state) filed and fixed BUG-1752..1762 in one PR. The P1: the Azure
sweep keyed "done" off ACA Job `ProvisioningState`, which reads
`Succeeded` the moment the resource is created — every runner Job
became reap-eligible on the next 2-min tick and `BeginDelete` kills the
in-flight execution. It now classifies the latest JobExecution's
status, the same fix the GCP spawner already carried for Cloud Run's
`TerminalCondition` (whose stale spec rows also got corrected). Both
cloud loops gained the GitHub-side offline-runner reap they claimed in
their flag help but never did, plus a 15-min orphan grace for Jobs
whose start call failed. The runner images' "60-s idle timeout" was
fiction — absent in vanilla/ecs/lambda, and a job-killing
whole-process `timeout` in cloudrun/gcf; all six entrypoints now share
a pre-pickup idle gate (watch /proc for `Runner.Worker`, exit 0 on
idle, never bound a running job). The dispatcher↔image env contract
unified on `RUNNER_REG_TOKEN`/`RUNNER_REPO` (the ECS/Lambda images
required `RUNNER_TOKEN`/`RUNNER_REPO_URL` and could not be spawned by
the dispatcher at all). New per-label knobs on all three:
`runner_job_timeout` (Cloud Run task timeout / ACA ReplicaTimeout /
docker-shape sweep enforcement) and `max_concurrent` (ARC's
`maxRunners` analog). Registration tokens left plain control-plane env
for Secret Manager (TTL-bounded, reaped with the Job) on GCP and Job
secrets + `secretRef` on ACA. Azure also got the GCP deployable
hardening (healthz/$PORT, $REPO, verify retry, rate-limit-aware loop),
2cpu/4Gi runner resources, and its first tests; `ListRunners` now
paginates. Catalog: docs/RUNNERS.md § dispatcher hurdles D-1..D-7.

## 2026-06-12 - Actions follow-ups + the bind-translation gate's mechanism

Same-day follow-up PR to #549 (BUG-1745..1750). **Cancellation is real
now**: cancelling a run sends `JobCancellation` over the runner's open
mid-job poll (the channel the pull-only broker kept exactly for this),
purges undelivered job messages, leaves `always()`/`cancelled()` jobs
runnable (they dispatch with cancelled()==true), and the run concludes
`cancelled` — proven live by the harness cancelling a `sleep 300` on the
official runner and watching the always() cleanup execute. That e2e also
caught the runner's US-spelled `Canceled` result leaking through
normalization unmapped. **Self-hosted actions work**: `uses:` resolves
from bleephub-hosted repos first (GitHub-layout tarballs built from git
storage), proven by a composite-action harness test — 11/11 official-
runner integration tests. Org **runner groups** (CRUD/membership/repo
visibility, undeletable Default) and **startup_failure run shells** for
matched-but-unstartable workflows round out the bleephub side.

On the backends, the **bind-mount→shared-volume translation** — the
documented gate for running GitHub runners as cloud tasks — reached
parity across all six container backends: lambda's config got actually
wired into its bind path, ACA/AZF gained the whole mechanism, and the
sharing contract (writer via named volume, reader via translated host
bind) is integration-proven on ECS and ACA. What remains for the
runner-on-cloud cells is topology (runner images mounting the shared
volume), not translation. Ledger: 1750 filed / 1708 fixed / 2 open.

## 2026-06-12 - Complete GitHub Actions support in bleephub

**The workflow engine now implements GitHub's server-side semantics.**
`on:` triggers parse fully (branch/tag/path filter patterns with ordered
`!` negation, real git diffs for path filters, activity types with
per-event defaults — pushes to an open PR's head branch fire
`pull_request synchronize`); `on: schedule` crons fire from a
minute-aligned dispatcher (POSIX 5-field parser, dom/dow OR rule);
reusable workflows expand server-side (synthetic gate/collector nodes,
typed+defaulted inputs, secrets inherit/mapping, outputs onto
`needs.<caller>`, 4-level nesting bound); and a real expression engine
(GitHub grammar, loose equality, full `github`/`needs`/`vars`/`inputs`/
`matrix` contexts, contains/startsWith/format/join/toJSON/fromJSON)
evaluates job `if:` and `${{ }}` templates — invalid expressions fail the
job like real GitHub.

**Secrets and variables exist at all three scopes** (repo/org/environment)
with the REAL wire contract — `gh secret set` fetches the public key,
seals with libsodium, and the server decrypts; plaintext PUTs are
rejected (the old shape no real client could ever have used). Org
visibility (all/private/selected), name rules, and org→repo→env
precedence merge into runner job messages with masks.

**Workflow runs are now first-class GitHub citizens**: every job mirrors
to a check run under a github-actions check suite; workflow_run /
workflow_job / check_run / check_suite webhook events fire at the real
emission points; PR `mergeable_state` reflects the head commit's checks
and the merge API 405s while required status checks (branch protection)
aren't green. The jobs API serves REAL per-step status/timing — the
runner's timeline records were being silently dropped because the
official runner wraps them in `VssJsonCollectionWrapper` (found against
actions/runner source; same wrapper bug fixed for console-line feeds).
Job logs persist (4MiB cap with explicit markers), run-log zips match
GitHub's layout, runs-on labels route jobs only to matching runners
(hosted aliases run anywhere), org-scoped runner endpoints exist with an
honest `busy`, reruns keep the run id and bump `run_attempt` (archived
attempts retrievable; rerun-failed-jobs carries successful results over),
and workflows enable/disable (disabled = no triggers, dispatch 403).

**The UI got the full GitHub-style Actions experience**: per-repo Actions
tab (runs list, filters, dispatch form built from parsed workflow inputs,
enable/disable), run detail (job sidebar, per-step status, live-tail
logs, rerun/cancel, artifacts, deployment approvals), secrets+variables
management with real in-browser sealed-box encryption, PR merge-box
checks section, runners page with labels/busy. Playwright 21/21, vitest
green, knip/jscpd clean.

Validation: gh Docker harness 115 PASS / 0 FAIL (now covering secrets/
variables/enable-disable/checks); the official-runner integration
harness was found bitrotted (launched binaries retired long ago —
BUG-1739), rewired to host-mode jobs (`jobContainer: null`, real
GitHub's no-container shape) and promoted to CI as
`sim (bleephub actions/runner)` — ALL 9 e2e tests green. Running the
REAL runner exposed two more latent protocol bugs, both fixed: the
broker pushed jobs at busy runners, which the runner silently drops
(BUG-1740 — delivery is now strictly pull-on-poll by free,
label-matching runners), and step `${{ }}` templates went out as
literal tokens the runner never evaluated (BUG-1741 — now
BasicExpression/format() tokens; secrets also ride message.Variables,
where the runner's ToSecretsContext actually reads them). Same PR also
closed consumer issues #548/#547: the azure-sim Entra token endpoint
accepts client_secret_basic and `/authorize` binds login_hint-resolved
users into auth codes (BUG-1742/1743). Ledger at 1743 filed / 1701
fixed / 2 open.

## Earlier milestones (compressed)

Full detail in the PR descriptions and `git log`; one line each:

- **Amplify full slice + bleephub Apps/orgs hardening** (#546) — complete Amplify control+data plane; GitHub Apps install/JWT/OAuth + org provisioning fidelity.
- **Launch hygiene** — spec-validation armed across all test surfaces incl. AWS XML protocols; azurestack retired; docs truth pass.
- **Simulator shape-drift burn-down** — all 28 allowlisted wire-shape bugs fixed (BUG-1658..1685); aws/azure allowlists emptied; knative moved to real `serving.knative.dev` paths; postgres-flexible speaks the real 202+Azure-AsyncOperation LRO.
- **Spec-based simulator validation** — vendored machine-readable cloud-API specs (`specs/cloud-api/`) + two CI gates: static surface conformance (every registered op exists in the spec) and runtime wire-shape validation (`SOCKERLESS_SPEC_VALIDATE`, ratcheted).
- **Simulator conformance + hardening (AWS/GCP/Azure)** (#537/#538/#539) — round-trip/error/pagination fidelity sweep + Go type hardening + sim-UI hardening + CI sim-module unit tests.
- **bleephub parity + durability + GitHub-style UI** (#534..#536) — SQLite/PostgreSQL persistence, filesystem + S3/MinIO git content storage, git HTTP auth, the UI restyle, and a GitHub-API fidelity sweep.
- **Cloud service-slice expansion** — AWS (Step Functions, Batch, CodeBuild, Glue), GCP (Spanner, Dataflow, Bigtable), Azure (Logic Apps, ACI), each with SDK + CLI + Terraform coverage.
- **Terraform idempotency drift sweep** (#491) — `terraform plan -detailed-exitcode` drift assertions on the gcp+azure apply stacks; ~18 read-back fidelity bugs fixed to make `tf (gcp)`/`tf (azure)` green.

## Foundations

Sockerless now includes:

- Docker API-compatible backends for local Docker passthrough and cloud-backed container/FaaS targets.
- High-fidelity AWS, GCP, and Azure local simulators, one binary per cloud, with official SDK/CLI/Terraform coverage tracked in [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).
- A real-execution substrate in `simulators/realexec`: network namespaces, bridges/veth/TAP NICs, IPAM, nftables, Firecracker VM lifecycle, health probes, and load-balancer proxying.
- Cross-cloud OCI `/v2/` registry data-plane implementations for ECR, Artifact Registry, and ACR.
- Bleephub, a GitHub Enterprise-style API simulator covering repos, issues, PRs, Actions, runners, apps, OAuth/OIDC, webhooks, packages, and admin org provisioning.
- Local HTTPS gateway infrastructure through Caddy for providers that require HTTPS endpoint discovery.

Older detailed entries were intentionally compressed out of this file. Use PR descriptions and `git log --oneline --decorate --all` for older implementation history.
