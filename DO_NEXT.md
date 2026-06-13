# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`docs/streamline-continuity-files` — continuity-file streamline + CLAUDE.md→AGENTS.md symlink + the continuity workflow in AGENTS.md. Docs only. (#553 sim fidelity audit + pod-model fixes is merged.) After this merges: pick the next arc.

## Next

See [PLAN.md](PLAN.md) § Next for the full framing. In order of readiness:

1. **Runner-as-cloud-task live pass** (BUG-1075) — cells 1+2 are sim-proven; the live run against real ECS/Lambda is the remaining piece. User-gated (real cloud spend).
2. **Versioned releases + GHCR images** (issue #363) — tagging, release workflow, image publishing. Self-contained consolidation milestone.
3. **Another sim fidelity audit** — narrow the coverage map to load-bearing ops, probe with the real client (method in [PLAN.md](PLAN.md)).

Re-check `gh issue list --repo e6qu/sockerless` before any consumer-issue work; only #394 (azuread, upstream-blocked) is open.

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
