# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`fix/cloudrun-gcs-sync-reachability-1792` (PR pending) — **BUG-1792 prerequisites + BUGS.md count correction.** The cloudrun cell (merged #567) reaches step-exec; BUG-1792 turned out bigger than first scoped. **Landed here:** (a) BUGS.md header corrected — #567 left BUG-1789/1790/1791 in Open un-struck (now 1748 fixed / 4 open); (b) the bootstrap's gcs-sync (`persist.go`/`persist_sync.go`) honours `STORAGE_EMULATOR_HOST` (a `gcsBase()` helper; unauthenticated emulator mode skips the metadata token, so no metadata-reachability dependency); (c) the cloudrun backend injects a workload-reachable storage coordinate `SOCKERLESS_GCS_WORKLOAD_ENDPOINT` → `STORAGE_EMULATOR_HOST` on the task (default empty = real GCS); (d) the harness sets it to `host.docker.internal:5000`. Real cloud unchanged. These are the *prerequisites* for the gcs-sync workspace; the data-plane itself is still unwired (below).

### Remaining work

1. **BUG-1792 — wire the gcs-sync exec data plane (last TEST 12 gate).** `GCSSyncDriver.PreExec`/`PostExec` have **no callers** — the cloudrun `ExecStart` calls `BaseServer.ExecStart` directly, so the workspace is never uploaded to GCS, the bootstrap never gets `SOCKERLESS_SYNC_VOLUMES`, the tmpfs stays empty, and the exec's workdir (`/__w/<repo>`) doesn't exist → exit 255. Wire it: in cloudrun `ExecStart`, for each gcs-sync `SharedVolume` bound to the container, call `PreExec(vol, execID, localPath=host workspace dir, remotePath)` → upload + merge the `SOCKERLESS_SYNC_VOLUMES` env hint into the per-exec reverse-agent envelope; call `PostExec` when the exec stream closes (wrap the returned `io.ReadWriteCloser`). The bootstrap restore/save + storage reachability are already in place. **Also seen:** the reverse-agent 90s deploy timeout (`SOCKERLESS_CLOUDRUN_BOOTSTRAP_TIMEOUT_SEC`) is marginal under disk pressure (post-`docker system prune`) — registers reliably warm; bump it in the harness if flaky.
2. **TEST 13 (service container) + TEST 14 (dispatcher)** on Cloud Run once TEST 12 is green.
3. **GCF topology cell** — same overlay model.
4. **FaaS multi-container pod assembly (BUG-1781)**; **Arc 3 GitLab docker-executor parity.**

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
