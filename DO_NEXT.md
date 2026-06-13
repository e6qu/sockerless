# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/pod-model-correctness` (PR pending) — Arc 1 of a sustained **pod model + runner integration** focus across all backends. This PR: fixed the Lambda/GCF `docker pod stop/kill/rm` cloud-resource leak (Pod lifecycle methods were delegated to local-only BaseServer), closed the isolation-lint gap that allowed it, and made AZF's multi-container rejection fail fast + documented (BUG-1778..1779).

## Next (pod model + runner integration focus)

The grounded gap matrix (verified against source, correcting agent over-claims): only **Lambda** is live-proven (BUG-1075); the GitHub container-job topology (container jobs + services + dispatcher) is **sim-proven for ECS only** via the bleephub harness; the other backends have per-backend GitLab stdin-attach unit tests but no full-topology proof; AZF cannot run multi-container pods (single-invocation).

- **Arc 2 — extend the sim-proven GitHub topology harness beyond ECS** to cloudrun / gcf / aca (container jobs + `services:` + the dispatcher loop), documenting AZF's container-job limits. Turns "ECS sim-proven" into "all container backends sim-proven."
- **Arc 3 — GitLab docker-executor topology parity** — a sim-backed harness proving the full helper + build + service-container flow across backends (today only per-backend stdin-attach unit tests exist).
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
