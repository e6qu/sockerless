# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/bleeplab-artifacts` (Arc 3 Phase 4; PR pending). **bleeplab now passes CI artifacts between stages, object-store-backed.** The GitLab ECS cell does real cross-stage work: build-job compiles `calc` → publishes it as an artifact; test-job (no toolchain) downloads + runs it without recompiling (`make bleeplab-runner-docker-test-ecs` → TEST 1/2/3 PASS). Merged previously: #578 (BUG-1801 git + BUG-1802 ECS binds), #577 (BUG-1800), #576 (BUG-1798).

### Arc 3 remaining work

1. **(done, #578) BUG-1801 git + BUG-1802 ECS volume binds.** bleeplab serves each project as a real git repo over smart-HTTP (object-store-backed go-git); the ECS backend reconstructs `HostConfig.Binds` from the task def on restart.
2. **(done, this PR) Object-store-backed CI artifacts.** bleeplab artifact store (S3 > `BLEEPLAB_ARTIFACTS_DIR` > memory, mirroring the git storage); runner upload (`POST /api/v4/jobs/:id/artifacts`) + download (`GET …`) endpoints; `artifacts:`/`dependencies:` CI parsing; `dependencies` advertised in the job response (earlier-stage jobs + their artifact size/token). **Coordinate finding:** the in-container artifacts-uploader uses the runner's *config `url`* (not `CI_API_V4_URL`), so that URL must be container-reachable — the harness sets it to `host.docker.internal:8929` (resolves from both the harness host via /etc/hosts and the workload containers via the host gateway).
3. **Remaining bleephub parity (user asked): a bleeplab UI** like bleephub's (React SPA + `ui_embed.go`/`ui_noembed.go` + `UI_PACKAGE` in `bleeplab/Makefile` + `spaHandler` at `/ui/`) showing projects/pipelines/jobs. The git + artifact object-store slices are done; the UI is the last parity piece.
4. **Phase 4 cont. — extend the harness** (services container; more jobs/stages) and bring up the other backends' GitLab cells (cloudrun/gcf/aca), reusing the bleephub overlay model.
5. Then **FaaS multi-container pod assembly (BUG-1781)** and standing items.

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
