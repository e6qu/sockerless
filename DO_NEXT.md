# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/cloud-backend-network-driver` (PR pending) — groundwork from the Arc-2 ACA cell stand-up. Fixes **BUG-1780** (lambda/cloudrun/gcf/aca/azf used the real-Linux-netns network driver instead of ecs's metadata-only `SyntheticNetworkDriver` → `docker network create` 400s without iproute2 / leaks a kernel netns; all five now mirror ecs) and codifies two principles across AGENTS.md + CLOUD_RESOURCE_MAPPING.md + the AZF README: **experiential parity** (assemble every Docker abstraction — networks, multi-container pods incl. localhost loopback, volumes — from cloud primitives on every backend so the experience matches local Docker/Podman; filed **BUG-1781**) and **faithful sims** (no special/fake sim functionality for sockerless backends or runners). The ACA topology harness plumbing is preserved for the next arc.

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
