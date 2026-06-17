# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`fix/audit-backlog-deferred` — working the 3 items deferred from #594. **BUG-1840 DONE + tested** (sim-only `Sim*` field removal):
- **gcp** `cloudfunctions.go`: removed `SimCommand`/`SimImage`/`SimArchitecture` + the backend-dead image-less Sim branch of `invokeCloudFunctionProcess`. Confirmed backend-dead by tracing the invoke target: the gcf backend POSTs to the Cloud Run **service** `svc.Uri` (→ `/v2-services-invoke`, `cloudrunservices.go invokeService`), never the sim-only `/v2-functions-invoke`. The endpoint's no-image path now records "Function invoked" + returns `{}`; the overlay-image path is unchanged. Dropped the `Sim*`-only `TestCloudFunctions_Invoke{ExecutesCommand,NonZeroExit,LogsRealOutput}` + `InvokeArithmetic*` SDK tests; gcp container-exec coverage stays via the Cloud Run **Jobs** arithmetic tests + the gcf cell.
- **azure** `functions.go`: removed the sim-only `SimCommand` fallback + the now-identity `Site.wire()`. Kept the real `invokeAzureFunctionProcess` (the path the azf backend drives via `LinuxFxVersion` + `SOCKERLESS_CMD`/`SOCKERLESS_ENTRYPOINT` app settings). Rewrote the invoke SDK tests to deliver the command via the real `SOCKERLESS_CMD` app-setting contract — they still run real `alpine`/`eval-arithmetic` containers and pass.
- **Sub-fix (BUG-1824 class):** all three sim sdk-tests' `buildGoScratchImage` now probe `docker buildx version` and use `buildx build --load` (was `docker build -t`, which on a `docker-container` buildx-default host leaves the workload image in the build cache only → the sim 500'd on eval/`InvokeArithmetic*`). This was a **pre-existing** local-repro failure (CI's docker driver loads to the store), surfaced while verifying azure.

**BUG-1845 DONE + cell-verified** (cloudrun `networkServices` stateless reconstruction):
- `buildServiceSpec` persists the network's service-style members on each revision (`sockerless_network_id` + `sockerless_network_service_members` annotations, base64-JSON of the non-OpenStdin members — the script-runner siblings are per-stage transients, excluded).
- `serviceMembersOfNetwork` rebuilds the map on a cache miss (`rebuildNetworkServicesFromCloud` → reads the network's latest Service revision, re-seeds each member into PendingCreates), guarded by `networkRebuilt` so a service-less network doesn't re-list Services every stage. The live `trackNetworkService` path is unchanged within one process (no green-path behaviour change beyond the additive annotations).
- Unit test `TestNetworkServiceMembers_PersistAndRebuild` (persist → simulated restart → rebuild round-trip); the green `services:` redis path verified by `make bleeplab-runner-docker-test-cloudrun`. gcf uses a single multi-container revision per `services:` job (not this map) → unaffected.

**Still open (deferred):** **FaaS pod polish** — a shared-workspace volume across azf pod members + per-sidecar exec routing (follow-on, not a blocker per PLAN § FaaS). Rides in the SAME PR (one branch, one PR) if done this arc.

### (history) PR #593 merged (`fcb58281`) — the codebase audit for the anti-patterns flagged 2026-06-17 (fallbacks, error-swallowing, fakes/stubs, sim-contract violations, default-param/defaulted behaviour, dead code) across **sims → backends → UIs**, plus the fixable open GitHub issues. **GitHub issue board now: only #394 (azuread upstream-blocked) open.**

### Next: the staged audit backlog (BUG-1840–1846)

The audit's larger findings, filed OPEN with fix-shapes — the genuine "no fakes / no fallback / fail loud" cleanups that need careful, individually-tested work. Suggested order: **BUG-1840 first** (sim-only `Sim*` fake fields — clearest contract violation, well-defined test rewrite), then the **BUG-1844** backend swallow batch (PodRemove ×4 / core ContainerWait / ecs zero-stats / lambda pod-row), then **BUG-1845** (cloudrun `networkServices` stateless violation), then the rest (1841 aws cloudmap DNS, 1842 aws ec2 seeding, 1843 gcp fingerprint, 1846 default-param + dead-code batch). Each is in [BUGS.md](BUGS.md) § Open with a fix-shape.

### What #593 fixed azure-sim swallows + dead-var pins (1836/1837/1838), cloudrun fabricated exit-0 (1839), gcp Cloud Build buildx-`--load` (1847, the azure→gcp twin of 1834), the three open AWS-sim fidelity issues #591/#592/#590 (1848/1849/1850), and two UI fail-loud fixes (1851 docker-frontend no-error-state, 1852 admin fabricated gateway endpoints). Closed already-fixed issues #583/#569.

**Staged backlog (filed OPEN with fix-shapes — these need careful, tested, sometimes contract-changing work, not a rushed batch):**
- **BUG-1840** gcp+azure sim `SimCommand`/`SimImage`/`SimArchitecture` — sim-only fake fields on the cloud resource model, set only by SDK tests; remove + rewrite the function-invoke tests to the real `SOCKERLESS_CMD` overlay path. (Biggest "no fakes" cleanup.)
- **BUG-1841** aws sim Cloud Map DNS keyed on the `sockerless-container-id` tag → resolve by `AWS_INSTANCE_IPV4`.
- **BUG-1842** aws sim ec2 `ensureSimDefaults` sockerless-harness pre-seed → provision via real EC2 + fail loud.
- **BUG-1843** gcp sim fingerprint constant + no optimistic-concurrency validation.
- **BUG-1844** backend swallow batch (PodRemove `_ = ContainerRemove` ×4, aca NetworkRemove, core ContainerWait `GetContainer` err + `writeWaitBody(0)`, ecs zero-stats-on-error, lambda pod-row drop) — collect+surface; never fabricate success/zero.
- **BUG-1845** cloudrun `networkServices sync.Map` authoritative local state not rebuilt from cloud (stateless violation) → reconstruct from Service revision tags in recovery.
- **BUG-1846** backend default-param-on-invalid (job_timeout/parseDuration) + dead code (cloudrun `maybeAutoRemove` no-op, ecs nil-ARN ListTags, dead pins, lambda replay swallow).

### Just merged (#589): GitHub `actions/runner` cells on aca + azf, both GREEN (all 14 bleephub harness tests: TEST 12 container job, TEST 13 `services:` container, TEST 14 dispatcher-spawned runner), giving GitHub+GitLab runner parity on every container-capable backend. PR **#588 merged** (`dacbc6dc`, azf cloud-dns hardening); PR **#587 merged** (`084b62dd`, GitLab cells).

### This branch — what shipped (reusable findings)

- **azf wired into the bleephub harness** (mirrors aca): `bleephub/Dockerfile` builds `sockerless-backend-azf` + `sockerless-azf-bootstrap`; `provision_azf` in `run-integration.sh` (App Service plan host primitive, `SOCKERLESS_AZF_NETWORK_DISCOVERY=cloud-dns`, `/v1/azf/reverse`, azf bootstrap, runner-ws + runner-externals Azure-Files shares); `azf)` case arm; `bleephub-runner-docker-test-azf` Makefile target. Also added `--load` to the bleephub image build (BUG-1824 class, local-iteration safety).
- **BUG-1834 (azure sim ACR Tasks build):** hardcoded `docker build --load` (buildx-only) → the harness's legacy `docker.io` builder (no buildx plugin) rejects `--load` → `container:` job `ContainerCreate` 500. Fix: probe `docker buildx version`; use `docker buildx build --load` when present (loads to the daemon store for every driver), else plain `docker build` (legacy, store-native). The chosen path is LOGGED (loud, not silent). NOTE on the principle: this is irreducible host-capability detection (no single docker invocation works across legacy / buildx-docker / buildx-docker-container) — both arms run in real environments and errors propagate loudly — NOT a bug-hiding fallback.
- **BUG-1835 (azf cloud-dns discriminator):** `startCloudDNSSite` keyed overlay-vs-raw on `OpenStdin` (gitlab-runner-only). The GitHub `container:` job is exec-driven but NOT OpenStdin → was deployed raw → no reverse-agent → `docker exec` of steps exit 126. And a `services:` container (image-default entrypoint) must run its RAW image, not the overlay. Fix: derive `serviceLike` (no client entrypoint/cmd override AND not OpenStdin) from the ORIGINAL client request at ContainerCreate, recorded into `labelServiceLike` **before** the image-config merge (azf merges the image's default entrypoint/cmd in, so the post-merge base labels can't tell client-override from image-default — same reason aca computes serviceLike pre-merge). `startCloudDNSSite` reads the marker: serviceLike → raw image, started on the VNet by swift integration (BUG-1831 path); else → overlay + `invokeFunctionAsync` (blocks for the in-site reverse-agent, no fallback).
- **The bleephub container-job tests are local/manual** (`make bleephub-runner-docker-test-{aca,azf}`, need docker.sock); CI runs the unit + sim suites, not these harnesses. Validate cells locally before the PR. `BLEEPHUB_HOLD=1` freezes the stack on failure; logs live inside the harness container (podman VM), read via `docker exec <cid> sh -c 'cat /tmp/sockerless-bleephub-data/logs/...'`.

### Next: standing track

- **Live-cloud pass (BUG-1075)** — biggest open gap; the sim-proven cells against real ECS/Lambda/Cloud Run/ACA/AZF (user-gated spend).
- **Versioned releases + GHCR (#363).**
- **Fresh sim fidelity audits** (the repeatable method keeps finding real bugs).

### Just merged (#588 — azf cloud-dns hardening, reusable findings)

- **azf `NetworkConnect` connect-after-create.** Under cloud-dns, `NetworkConnect` was a bare passthrough — a `docker network connect --network-alias X` *after* create registered no Private-DNS record (the cell only worked because gitlab-runner creates containers *with* the network). Root: the core `SyntheticNetworkDriver.Connect` only writes `Store.Containers`, which the stateless azf backend ignores. `cloudDNSNetworkConnect` (backends/azure-functions/network_clouddns.go, wired into `NetworkConnect` behind the cloud-dns config): connect-before-start stamps the network+aliases onto the PendingCreate; live connect VNet-integrates + writes Private DNS CNAMEs now. Unit test `TestCloudDNSNetworkConnect_StampsPendingCreate`.
- **Swift VNet-integration testing contract** (CLI `az rest` round-trip + Terraform `azurerm_app_service_virtual_network_swift_connection` over a `Microsoft.Web/serverFarms`-delegated subnet) joined the #587 SDK test.
- **Three sim fidelity bugs the TF provider path surfaced (each a real fix):**
  - **BUG-1833** — the swift response returned `id`/`type` from the *operation* path (`.../networkConfig/virtualNetwork`, type `…/networkConfig`) instead of the canonical *config* sub-resource (`.../config/virtualNetwork`, type `Microsoft.Web/sites/config`). terraform-provider-azurerm parses the response `id` (`*read.Model.Id`) and its parser requires a `config` segment → apply failed `ID was missing the 'config' element`. The #587 SDK test missed it (asserted only `subnetResourceId`). **Lesson:** for an App Service `config` child reached via an action path (`networkConfig/`, `web/`, …), the resource `id` uses `config/<name>`, NOT the action-path segment — and assert the returned resource `id`, not just properties.
  - **BUG-1832** — a `Microsoft.Web/serverFarms`-delegated subnet dropped `properties.delegations[].properties.actions` → `azurerm_subnet` non-idempotent (drift). Added `Actions []string` round-trip.
  - **BUG-1831** — swift PUT force-started a container for any non-HTTP site → 500 on an imageless function app's VNet integration. Gated the start on `siteContainerImage != ""` (real Azure VNet integration is pure networking config); redis-with-image services path unchanged.
- **Lesson — the `tf (azure)` provider path catches what SDK/CLI don't.** All three bugs passed SDK+CLI but failed the azurerm provider's call/parse sequence; SDK+CLI tests now also assert the swift `id`, so the regression is caught in the widely-run `sim (azure)` job, not only the gated `tf (azure)` job.

#### (historical) aca services-job diagnosis

- **aca cell — build + artifact-cross-stage jobs GREEN; the redis `services:` job is RED (hangs at `prepare_script`).** Root cause filed as **BUG-1825** (P1, open): the runner-stage reverse-agent WebSocket exec (`agent.CollectExecWithStdin`) hangs — the backend's `SendJSON(TypeExec)` returns but the frame never reaches the in-container bootstrap (verified the SAME, correct connection: `rc.ws.RemoteAddr()` == registered session remote; ruled out conn mismatch, re-dial, replica restart, volume mount, redis reachability, gcs PreExec). The identical path works for the build/test jobs, so the trigger is services-job-specific (extra service-discovery aliases + `FF_NETWORK_PER_BUILD`). The backend `ReverseAgentConn` has no keepalive/read-deadline, so the undelivering connection is never detected → infinite hang. cloudrun passes the same job by routing stages via the bootstrap **HTTP buffered-invoke** instead of the WS exec.
  - **DONE this branch:** backend WS keepalive added to `agent.ReverseAgentConn` (ping ticker + `SetPongHandler` + per-message `SetReadDeadline`; refresh on pong+data, NOT on inbound pings → a half-open is still detected). The infinite hang is now a clean ~49 s `exit 126` failure; **aca build/test jobs still pass AND the cloudrun cell still passes all 4 tests (no regression)** — keep it (it fixes a real all-FaaS infinite-hang). The keepalive proved the connection is **fully half-open** (no pongs → server→client dead after handshake), and it's **deterministic** for the services job (gitlab-runner treats prepare `exit 126` as a job failure, not retried → fail-fast ≠ green). Likely root: gvisor/podman port-reuse under the heavy per-stage churn (13/15 reverse-agent sessions dropped) misrouting the persistent backend→container direction.
  - **GREEN-FIX (next, substantial — essentially port cloudrun's UseService invoke to aca):** both cloudrun AND gcf route runner stages through the bootstrap **HTTP buffered-invoke** (never a persistent backend→container gvisor connection); aca uniquely uses the reverse-agent WS exec (`runACACommandViaAgent` → `RunAndCaptureWithStdin`). (1) **azure sim:** add an App-invoke HTTP endpoint that proxies the request (incl. the gitlab stdin envelope) to the container's bridge-IP `:8080` — mirror `simulators/gcp/cloudrunservices.go` (`/v2-services-invoke` + `firstReachableBase`/`<containerIP>:8080`, the BUG-1810 reach). (2) **aca backend:** route runner stages via HTTP POST to that sim endpoint (mirror cloudrun's `invokeServiceDefaultCmd` + `start_service.go`), keeping the App alive across stages (the keep-alive/re-invoke scaffolding is already there). Reuses the bootstrap's existing `handleInvoke` HTTP path (`:8080`) — no bootstrap change needed.

### Fixes landed on this branch (real, standalone)

- **BUG-1824 (P1, Makefile):** `bleeplab-runner-docker-build` now uses `docker build --load` — the default `docker-container` buildx driver builds to cache only, so without `--load` every `docker run` silently used a STALE image (masked a whole session of iteration; debugged dead code). **Unblocks ALL future runner-cell work — keep it.**
- **BUG-1815..1823** (aca cell hurdles): arch via `azurecommon.ArchFromPlatform`; cache-init one-shot runs via agent (`acaCommandRunsViaAgent`); ad-hoc volumes → azure-files-ephemeral; sim share dir `chmod 0777` (umask); SELinux `:z` relabel on sim binds; NetworkInspect cloud-truth membership; stdin-attach precedence over reverse-agent; stdin script under `/bin/sh`; ContainerStop keep-alive + ContainerStart re-invoke (cloudrun parity — note re-invoke does NOT fire under gitlab-runner, which removes+recreates per stage; kept for parity). ExposedPorts carried from image config at create (PARTIAL — redis service inspect resolves via cloud-state which still drops it; complete it in `appToContainer`).

### Previously (gcf cell — merged PR #586)

**bleeplab GitLab cell on gcf — GREEN.** The full gitlab-runner docker-executor flow runs on the Cloud Run Functions backend: a 3-stage pipeline (build → test/artifact → `services:`) passes all 4 tests (gcc-compiled `calc.c`, cross-stage `calc` artifact, redis by alias over the per-build network-pod — PING/SET/GET). gcf reuses the gcp sim + cloudrun backend's gcp-common; the redis `services:` job exercises the BUG-1781 network-pod (multi-container revision) assembly with NO BUG-964 gate hit. **Reusable findings (validation surfaced + fixed 4 real bugs — each mirrors a cloudrun mechanism the gcf network-pod model diverged from):**

- **BUG-1811** (gcf backend): `ContainerStart` resolved ONLY from `PendingCreates`; the gitlab-runner docker executor does start→wait→stop→start cycling per stage on the same container ID, and after the first start the container leaves PendingCreates → re-start failed `NOT FOUND`. Fixed: fall back to `s.ResolveContainerAuto(s.ctx(), ref)` (CloudState) and re-add to PendingCreates (exactly what cloudrun already did).
- **BUG-1812** (gcf backend): `ContainerAttach` checked for a reverse-agent FIRST and routed a stdin attach to it, but the reverse-agent never registers a main process (`mp==nil`; reverse mode carries only exec sessions) → `no main process to attach to`. The gcf network-pod bootstrap registers a reverse-agent for every member (cloudrun's single helper doesn't), so the second per-stage attach broke. Fixed: the `opts.Stdin` stdinPipe/buffered-invoke path now takes precedence over the reverse-agent routing.
- **BUG-1813** (gcf backend): the gitlab-runner attach-stdin script was piped to the image's own entrypoint (`[dumb-init /entrypoint gitlab-runner-build]`), which reads stdin in its own protocol and silently ignores a raw script (banner + exit 0, no clone) → `get_sources` ran but never cloned (0 git requests, `./calc: not found` downstream). Fixed: when stdin is captured, override `invokeArgv=[/bin/sh]` so the bootstrap runs the script under a shell — exactly what cloudrun's `postBootstrap` forces.
- **BUG-1814** (gcf bootstrap): a reused function instance restored its persist (gcs-snapshot) `/builds` only ONCE at startup (it restored *sync* volumes per-invoke but not *persist*), so the reused predefined-helper's `upload_artifacts` kept its stale get_sources workspace → `WARNING: calc: no matching files` and its save even clobbered the build container's fresh snapshot. cloudrun is green on the identical persist classification because `UseService` cold-starts a fresh per-stage instance that restores at startup. Fixed: `restoreAll(persistVols)` before every invoke in the gcf bootstrap's handleInvoke.
- **Cross-cutting (BUG-1808/1810 classes):** the gcf BackendDescriptor arch was hardcoded `amd64` → now derived from `config.BuildPlatform` via the shared `gcpcommon.ArchFromPlatform` (cloudrun's local `archFromPlatform` was promoted to gcp-common; both backends use it). The gcp sim's **gcf function-invoke** path had the same `127.0.0.1:hostPort` unreachability as the Service path (BUG-1810) → now reaches the workload by bridge container IP via the shared `firstReachableBase`.
- **Harness facts:** `provision_gcf` mirrors `provision_cloudrun` with `SOCKERLESS_GCF_*` names + `/v1/gcf/reverse`, but does **NOT** set `SOCKERLESS_GCR_USE_SERVICE` (gcf uses the native multi-container revision / network-pod, not a kept-alive Service). Stage the arch-matched `gitlab-runner-helper:<arch>-v<ver>` + `redis:7-alpine`; set `SOCKERLESS_WORKLOAD_ARCH`. Dockerfile adds `backends/cloudrun-functions` → `sockerless-backend-gcf` + `sockerless-gcf-bootstrap`. Makefile target `bleeplab-runner-docker-test-gcf` = `(gcf,4567)`.

Merged previously: #585 (cloudrun cell BUG-1808/1809/1810), #584 (AZF pod polish + artifact UI + BUG-1806/1807), #582 (BUG-1781 FaaS pods), #581 (BUG-1804/1805), #580 (UI), #579 (artifacts), #578 (git + ECS binds).

### NEXT (own PR): bleeplab GitLab cell on aca

The ECS, cloudrun **and gcf** cells are green. The harness is already the one-image `BLEEPLAB_BACKEND`-switched shape — adding aca is additive:

- **aca**: add `simulator-azure` + `backends/aca` → `sockerless-backend-aca` to the Dockerfile (`sockerless-cloudrun-bootstrap` is reused as the ACA bootstrap); add `provision_aca` (storage account, managed env, ACR, build-context container, `SOCKERLESS_AZURE_ACR_ENDPOINT=127.0.0.1:5000`, `/v1/aca/reverse`, `azure-files-ephemeral` volumes, the `*.blob.localhost` hosts alias + `SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON`); Makefile target `(aca,4568)`. **The gcf cell's 4 bugs predict aca's hurdles** — check the azure sim's `postCloudRunServiceInstance` analog for BUG-1810 (bridge-IP reachability), and whether aca reuses an instance across stages → if so it needs the BUG-1814 persist-restore-before-invoke fix in the aca bootstrap (the cloudrun bootstrap, reused for aca, already gets fresh instances via UseService — verify).

Per-backend deltas (from the cloudrun cell, now proven): ECS has no overlay/registry; cloudrun/gcf use Cloud Build→AR + `SOCKERLESS_GCP_AR_ENDPOINT=127.0.0.1:5000` + `gcs-sync`; aca uses ACR Tasks + `SOCKERLESS_AZURE_ACR_ENDPOINT=127.0.0.1:5000` + Azure-Files. A fresh podman machine needs `podman machine stop && start` for the `:5000` insecure drop-in to load. **BUG-1810 (sim-in-container reaches the workload by bridge IP) likely also applies to the aca/azure-sim Cloud Run-style invoke path — check the azure sim's `postCloudRunServiceInstance` analog if aca's one-shot Service invoke fails the same way.**

### BUG-1781 — what shipped (reusable findings)

- **Premise was partly stale.** Verified against code: **lambda** runs all pod members as chroot subprocesses of one supervisor in a single Lambda execution env (one shared netns → `localhost` works; `agent/cmd/sockerless-lambda-bootstrap`); **gcf** co-deploys members into one multi-container Cloud Run revision + `/etc/hosts` alias→127.0.0.1 (`backends/cloudrun-functions/network_pod.go` + `pod_service.go`). The only backend that rejected pods was **azf**.
- **azf primitive = App Service sitecontainers** (`Microsoft.Web/sites/{name}/sitecontainers` — verified in armappservice/v5 SDK, `az webapp sitecontainers` CLI, and the vendored `web-arm-openapi-2025-03-01` spec). One `isMain` container + N sidecars share a network namespace → intrinsic `localhost`. No agent loopback-proxy needed (unlike the multi-function fallback).
- **Sim:** `simulators/azure/sitecontainers.go` models the sub-resource (CRUD) + `invokeAzureFunctionHTTP` starts the `isMain` HTTP container then each sidecar with `NetworkMode: container:<main>` (shared netns), mirroring the ACA multi-container path. `startUpCommand` round-trips an argv via shell-quoting (backend) + a quote-aware splitter (sim) — naive whitespace splitting mangles `sh -c '<script>'`.
- **Backend:** `network_pod.go` (gcf-mirror `shouldDeferOrMaterializeNetworkPod`) + `pod_site.go::materializePodSite`. The `isMain` runs the reverse-agent overlay; **sidecars run their RAW service image** (stashed in a label pre-overlay), because the overlay bootstrap binds the main's :8080 and would collide in the shared netns. Cloud-state reconstructs members from a `sockerless-pod-members` site-tag manifest (stateless — no local map). The two fail-fast rejections (`PodStart`, `ContainerStart`) are gone.
- **azf bootstrap** writes `SOCKERLESS_HOST_ALIASES` → `/etc/hosts` so a sibling resolves by name (mirror of gcf's `writeHostAliases`).

### Next

1. **Phase 4 cont. — more jobs/stages** and the other backends' GitLab cells (cloudrun/gcf/aca), reusing the bleephub overlay model.
2. **FaaS pod polish (follow-on):** a shared-workspace volume across azf pod members + per-sidecar exec routing; standing items (live pass, releases, sim audits).

### Arc 3 (merged, reusable findings)

1. **(#578/#579/#580)** bleeplab git + artifacts + UI — full bleephub parity.
2. **(#581 — BUG-1804) Cloud Map multi-name + ECS service-alias registration.** The aws sim's Docker-network DNS realization re-attaches a task container with the FULL set of service names it backs (disconnect-then-reconnect, since Docker rejects a 2nd `NetworkConnect`); the ECS backend captures `NetworkingConfig` aliases and registers the container under hostname + every alias, and deregisters by enumerating namespace services. Proven by `TestECS_MultiServiceDNS`.
3. **(#581 — BUG-1805) GitLab `services:` end-to-end on ECS.** Removed the ECS backend's `/etc/resolv.conf` command-wrapper (it froze per-network DNS to a static entrypoint snapshot — dropping the namespace network's DNS the runtime adds on Cloud Map connect — and mangled the user argv); the sim realizes each service as both `<service>` and `<service>.<namespace>` network aliases. **Runtime fact (Podman):** each network's DNS runs at its gateway; a container gets one nameserver per attached network, added as networks connect — so static resolv.conf surgery is wrong.

### bleeplab ECS harness (reusable findings)
- `bleeplab/Dockerfile` bundles `simulator-aws` + `sockerless-backend-ecs` + `sockerless-agent` + `bleeplab` + a real upstream `gitlab-runner` binary; `bleeplab/test/run-integration.sh` provisions ECS (the bleephub `provision_ecs` shape), starts bleeplab, registers the runner with `[runners.docker] host = tcp://127.0.0.1:3375`, triggers a pipeline, asserts success.
- **BUG-1800 (fixed):** the EFS access-point host dir wasn't writable for the workload — `CreationInfo` was ignored (umask reduced `MkdirAll(…,0o777)` to `0755 root`; now `ensureAccessPointRootDir` applies it on creation) AND the bind wasn't SELinux-relabeled (the sim now mounts task EFS binds with `z` so the confined `container_t` workload can write on local podman machines; no-op on CI). **Local SELinux note:** sim-spawned ECS workloads run confined; the `z` relabel handles it automatically (the bleephub ECS harness's manual `chcon` note is no longer needed for bleeplab).
- **BUG-1798 (fixed):** the ECS attach-stdin deferral required the stdin pipe to exist+be open at `/start`, but the attach driver created it only after a barrier waiting for `/start` — a dependency inversion. Fixed by creating the pipe before the barrier (`attach_driver.go`) + `/start` waiting briefly for the open pipe (`waitForOpenStdinPipe`). gitlab-runner 18 does `create → attach(stdin) → start` (no `docker exec`); the script is piped to the helper's stdin, its default `gitlab-runner-build` reads it.
- **BUG-1797 (fixed):** the aws sim runs ECS tasks on the host engine, so workloads are host-arch; the harness exports `SOCKERLESS_WORKLOAD_ARCH` from `uname -m` so core image manifest selection matches (the gitlab-runner-helper tag is arch-specific). Other harnesses keep amd64.
- The runner needs `GIT_STRATEGY: none` (no repo to clone in the sim); the helper image + alpine are pulled by the host engine directly through sockerless (no registry coordinate needed for ECS, unlike the cloudrun/gcf overlay path).

### bleeplab Phase 1 (reusable findings, merged #574)
- Module `bleeplab/` (GitLab analog of `bleephub/`); `cmd/main.go` on `:8929`. Registered in `go.work` + the `core-local` CI shard; needs a standardized `bleeplab/Makefile` (else `make bleeplab/lint` errors "No rule").
- The runner-API job-request `features` field is **mixed-type** (`trace_sections` bool, `failure_reasons` is a `[]JobFailureReason`) — `map[string]any`, advertise only `trace_sections` or the runner fails to decode the payload.

### How the GCF cell was closed (reusable)
GCF Gen2 deploys container-jobs as **Cloud Run Service revisions** (`materializePodService` → `Services.CreateService`), so a container-mode job runs the *same* sim path (`cloudrunservices.go`) the cloudrun cell uses — the sim needed **no** change. Five gaps were the GCF twins of cloudrun fixes (BUG-1795):
- **Exec wiring (the subtle one):** the Docker HTTP exec path is `handleExecStart → s.Typed.Exec.Exec`, NOT the typed `s.ExecStart` method. cloudrun wires `Typed.Exec = WrapLegacyExecStart(s.ExecStart)`; GCF wired the bare `ReverseAgentExecDriver` (`WrapLegacyExec`), so its `ExecStart` materialize/gcs-sync was dead code. Rewired through `s.ExecStart`. If a backend's `ExecStart` override "never runs," check this wiring first.
- materialize-on-exec (`materializeDeferredNetworkPodForExec`), `warmBootstrap` (BUG-1794 twin), bootstrap `/_sockerless/ready` route + `ExecHooks` (`ServeReverseAgentWithExecHooks`), and `STORAGE_EMULATOR_HOST` honored in the bootstrap's `persist.go` (`gcsBase`/`gcsAuthToken` — the #568 prereq was never ported; metadata-token 404 from the workload) + injected by the backend.
- **The reverse-agent restore error was invisible** until BUG-1796: a failed PreExec hook sent a `TypeError` the backend swallowed → opaque exit 255. Now surfaced to the runner's stderr + logged. This is the first tool to reach for when a FaaS exec fails opaquely.

### How the cloudrun cell was closed (reusable, merged #572)
- **BUG-1794:** exec-driven scale-to-zero Service never got a request → bootstrap never cold-started. Fix: `/_sockerless/ready` route + backend `warmBootstrap`; sim forwards request path+query.
- **BUG-1792:** sim resumable-upload `Location` hardcoded `https://` broke the storage client on the HTTP sim coordinate. Fix: derive the scheme from the request.
- **Running the harness locally:** `make bleephub-runner-docker-test-{cloudrun,gcf}`. On a freshly-(re)created or idle podman host, an attached `docker run` can return `unable to upgrade to tcp, received 500` — `podman machine stop && start` clears it (also re-loads the sim-registry insecure drop-in for the build path).

### Reusable findings (cloudrun cell)
- The overlay base image is rewritten to the AR `docker-hub` pull-through repo; the gcp sim hydrates it from the local docker daemon (`hydrateOCIImageFromLocalDocker`, keyed on `/docker-hub/`), so the harness must pre-pull base images locally. Serve a **fully-OCI** manifest — a mixed OCI-manifest/Docker-config image is accepted by `docker pull` but rejected by `docker build`'s `FROM`.
- The registry coordinate is reachable two ways with DIFFERENT addresses: the **host engine** (build/pull) uses the published `127.0.0.1:5000`; the **backend** (metadata fetch) must use its in-container `SOCKERLESS_ENDPOINT_URL` via the `RegistryEndpoint` override (the published port is not reachable from inside the backend's container). Recognising the coordinate host as a cloud registry is what wires that override.
- A GH container-job container is exec-driven (entrypoint overridden to a `tail -f /dev/null` keepalive); on Cloud Run it must NOT be default-invoked (that runs the keepalive as a request → request-lifetime SIGTERM) and, with no `services:`, must be materialized lazily on its first exec.

### Reusable finding (registry round-trip)
A real `docker push`/`pull` to the sim registry needs the host engine to trust it. **Docker auto-trusts loopback registries; Podman does not** — so a harness publishes the sim `/v2/` at `127.0.0.1:<port>` (Docker/Linux CI = no-op) and a podman-machine Make target drops a scoped insecure entry. The backend points the image ref at that reachable endpoint **only by coordinate** — `SOCKERLESS_AZURE_ACR_ENDPOINT` (azure) / `SOCKERLESS_GCP_AR_ENDPOINT` (gcp), default = the real registry. **No `if sim` branch** in backend or test code: the harness sets the coordinate per-target exactly like `endpointURL`, keeping the sim's registry and compute services agnostic and the client path identical to cloud.

### Reusable findings (this branch)
- ACA container-job exec needs the **App overlay** (`SOCKERLESS_ACA_USE_APP=1`) + an ACR-Tasks-built bootstrap image; the sim builds it on the host engine and runs it by local tag.
- The bootstrap/agent **must be statically linked** (`CGO_ENABLED=0`) to exec in musl/alpine/scratch overlays.
- Sim storage-over-HTTP needs a resolvable advertised endpoint: `SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON` pins `<account>.blob.localhost`, plus an `/etc/hosts` alias inside the harness container (`*.localhost` is not special-cased by the container resolver).

## Next (pod model + runner integration focus)

Grounded gap matrix: only **Lambda** is live-proven (BUG-1075); the GitHub container-job topology is **sim-proven for ECS only**; the ACA cell got past networking + lifecycle (BUG-1780) but container-job exec needs the bootstrap overlay; the FaaS backends can't yet assemble multi-container pods (BUG-1781).

- **Arc 2 — GitHub topology harness sweep (ACA → Cloud Run → GCF).** Land the harness plumbing (multi-backend image, `BLEEPHUB_BACKEND` parameterization — preserved in `/tmp/aca-harness-wip/`) and finish the ACA cell: container-job exec needs the reverse-agent bootstrap injected via the ACA App overlay (`SOCKERLESS_ACA_USE_APP=1` + an ACR-Tasks build). **Build this through faithful cloud APIs only** — the azure sim implements real ACR Tasks/Registry semantics and the host engine pulls the overlay as a real client would; no sockerless-aware sim hook. Then Cloud Run + GCF (same `cloudrun-bootstrap` overlay model). Turns "ECS sim-proven" into "all container backends sim-proven."
- **FaaS multi-container pod assembly (BUG-1781).** Assemble pod semantics from cloud primitives on lambda/gcf/azf (sidecars where offered, else a pod from multiple functions + cloud DNS + shared volume, agent proxying localhost to siblings) so `services:`/sidecar `container:` jobs run there; replaces the interim fail-fast rejections.
- **Arc 3 — GitLab docker-executor topology parity** — a sim-backed harness proving the full helper + build + service-container flow across backends.
- **Live pass (BUG-1075)** — once the above are sim-proven, the live run against real ECS/Lambda/etc. (user-gated spend).

Other standing candidate: issue #363 (versioned releases + GHCR). Re-check `gh issue list --repo e6qu/sockerless` before consumer-issue work; only #394 (azuread, upstream-blocked) is open.

## Working agreement

The full before/after-task continuity-file workflow, the no-fakes rules, and branch/PR hygiene live in [AGENTS.md](AGENTS.md). In short: read `STATUS.md`/`DO_NEXT.md` first; run the narrowest meaningful tests for the touched area; file bugs before fixing; update the continuity files in the same commit as the code; rebase on `origin/main` before pushing; never merge the PR.

Narrowest-test recipes for the common surfaces:

```bash
# Simulator SDK probe
cd simulators/<cloud>/sdk-tests && GOWORK=off CGO_ENABLED=0 go test -tags noui -run '<pat>' -timeout 15m .
# Simulator module unit tests + lint
cd simulators/<cloud> && make unit-test
# A backend's unit tests
cd backends/<name> && GOWORK=off go test ./...
# bleephub runner topology harness (self-contained)
make bleephub-runner-docker-test
```
