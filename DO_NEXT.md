# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/azure-sim-acr-tasks` (PR pending) — the faithful **ACR Tasks quick-build** slice in the azure sim (`scheduleRun` + `runs/{runId}`), the keystone the ACA/AZF App-overlay (reverse-agent bootstrap) path builds on. Fetches the build context from sim blob storage, runs `docker build` on the host engine (mirroring the GCP Cloud Build slice), tags the overlay into the local daemon; SDK tests included. Filed **BUG-1782** (`NewACRBuildService` ignores `SOCKERLESS_ENDPOINT_URL`).

### Remaining steps to a green ACA topology cell (continuing Arc 2)

In order; each is the next concrete blocker:

1. **Fix BUG-1782** — thread `endpointURL` into `NewACRBuildService` (backends/azure-common/build.go): build the ARM `RegistriesClient`/`RunsClient` with the `cloud.Configuration` override (mirror `newAzureClientsWithEndpoint`), and build the azblob client against the storage account's advertised `primaryEndpoints.blob` (discover via `armstorage` GetProperties) with HTTP creds allowed. Pass `config.EndpointURL` at the server.go call site.
2. **Wire `provision_aca`** (bleephub/test/run-integration.sh, restored from `/tmp/aca-harness-wip/`): set `SOCKERLESS_ACA_USE_APP=1`, `SOCKERLESS_AZURE_ACR_NAME`, `SOCKERLESS_AZURE_BUILD_STORAGE_ACCOUNT=simstorage`, `SOCKERLESS_AZURE_BUILD_CONTAINER=build-context`, `SOCKERLESS_AZURE_BUILD_PLATFORM=linux/<hostarch>`; create the ACR (ARM PUT) + the build-context blob container; set `SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON` so the sim advertises resolvable `*.blob.localhost:<port>` endpoints (Linux `*.localhost` → loopback).
3. **Reverse-agent exec** — the overlay container's bootstrap dials `ws://host.docker.internal:3375/v1/aca/reverse`. A sibling Podman container *can* reach a host-published port (verified), so this should connect once the overlay (with the bootstrap entrypoint) actually runs. Validate TEST 12 (container job).
4. **TEST 13 (service container) + TEST 14 (dispatcher-spawned runner)** on ACA, then Cloud Run + GCF (same `cloudrun-bootstrap` overlay model).

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
