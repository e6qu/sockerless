# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/bleeplab-cloudrun-cell` (PR pending). **bleeplab GitLab cell on cloudrun — GREEN.** The full gitlab-runner docker-executor flow runs on the Cloud Run backend: a 3-stage pipeline (build → artifact → `services:`) passes (gcc-compiled `calc.c`, cross-stage artifact, redis by alias over the per-build pod network). Extended the one-image `BLEEPLAB_BACKEND`-switched harness; CI markers are now backend-neutral. **Reusable findings (validation surfaced + fixed 3 real bugs):**

- **BUG-1808** (cloudrun backend): docker `/version` hardcoded `amd64` → gitlab-runner picks its helper image arch from the DOCKER_HOST's reported arch and chose the `x86_64` helper on arm64. Fixed: derive the reported arch from `config.BuildPlatform` (`archFromPlatform`), like ECS derives from `SOCKERLESS_ECS_CPU_ARCHITECTURE` ("report the workload's arch, not the host's").
- **BUG-1809** (gcp sim): the AR pull-through hydrated only `docker-hub` from the local daemon; the backend rewrites `registry.gitlab.com/<path>` → `<AR>/gitlab-registry/<path>` (gcp-common `image_resolve.go`) so the **gitlab-runner-helper** image 404'd. Fixed: `hydrateOCIImageFromLocalDocker` maps `/gitlab-registry/` → the local `registry.gitlab.com/<path>` ref; the harness stages the arch-matched helper tag.
- **BUG-1810** (gcp sim): the Cloud Run Service one-shot invoke dialed `127.0.0.1:hostPort` — unreachable when the sim runs **inside** the harness container (the host-published port binds the *host's* loopback, not the sim container's; the reverse-agent still works because it dials OUTBOUND to `host.docker.internal`). bleephub never hit it (github-runner containers are exec-driven, not one-shot Service invokes). Fixed: reach the workload by its **bridge container IP:8080** (routable container-to-container), fall back to `127.0.0.1:hostPort` (sim-on-host). Added the bootstrap-stdout-on-failure diagnostic that made this diagnosable.
- **Harness facts:** drop the github `/runner/externals` (gitlab-runner has none); stage `redis:7-alpine` for the services job + the arch-matched `gitlab-runner-helper:<arch>-v<ver>` (registry.gitlab.com, not throttled); `SOCKERLESS_GCR_USE_SERVICE=1` is correct for gitlab-runner (the backend keeps the Service alive across the runner's per-stage container cycle); set `SOCKERLESS_WORKLOAD_ARCH`. On a fresh podman machine the `:5000` insecure drop-in needs `podman machine stop && start`.

Merged previously: #584 (AZF pod polish + artifact UI + BUG-1806/1807), #582 (BUG-1781 FaaS pods), #581 (BUG-1804/1805), #580 (UI), #579 (artifacts), #578 (git + ECS binds).

### NEXT (own PRs): bleeplab GitLab cells on gcf + aca

The ECS **and cloudrun** cells are green. The harness is already the one-image `BLEEPLAB_BACKEND`-switched shape — adding gcf/aca is additive:

- **gcf** (this branch added the gcp sim + cloudrun backend; gcf reuses the gcp sim): add `simulator-gcp` is already there → add `backends/cloudrun-functions` → `sockerless-backend-gcf` + `sockerless-gcf-bootstrap` to `bleeplab/Dockerfile`; add `provision_gcf` (near-copy of `provision_cloudrun` with `SOCKERLESS_GCF_*` names + `/v1/gcf/reverse`); add the Makefile target `(gcf,4567)`. **Watch BUG-964** — the gcf default-invoke gating for the multi-container pod-Service; the redis `services:` job is exactly what trips it. The BUG-1808/1809/1810 fixes from this branch are gcp-sim/cloudrun-side — gcf benefits from 1809/1810 (shared gcp sim) and needs its own arch check (gcf backend may also hardcode arch — verify).
- **aca**: add `simulator-azure` + `backends/aca` → `sockerless-backend-aca` to the Dockerfile (`sockerless-cloudrun-bootstrap` is reused as the ACA bootstrap); add `provision_aca` (storage account, managed env, ACR, build-context container, `SOCKERLESS_AZURE_ACR_ENDPOINT=127.0.0.1:5000`, `/v1/aca/reverse`, `azure-files-ephemeral` volumes, the `*.blob.localhost` hosts alias + `SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON`); Makefile target `(aca,4568)`.

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
