# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `docs/streamline-continuity-files` (continuity-file streamline + CLAUDE/AGENTS) |
| In-flight | Continuity-file streamline only (this branch): compressed PLAN/STATUS/DO_NEXT/BUGS/WHAT_WE_DID to the live state, made CLAUDE.md a symlink to AGENTS.md, and added the continuity-file workflow to AGENTS.md. No code changes. |

| Last merged | #553 sim fidelity audit + pod-model review fixes (BUG-1773..1777). |
| Open GitHub issues | #394 azuread Terraform Graph override — upstream-blocked (BUG-1345). Re-check GitHub before non-conformance issue work. |
| Bugs | See [BUGS.md](BUGS.md) header for exact counts. 2 open: BUG-1075 (live-cloud cells), BUG-1345 (azuread upstream). |
| Live infra | None up. |

## What's next

Candidates, no committed timeline (see [PLAN.md](PLAN.md) § Next):

1. **Runner-as-cloud-task live pass** — cells 1+2 are sim-proven; the remaining piece is the live run against real ECS/Lambda (BUG-1075, user-gated spend).
2. **Issue #363 — first versioned release** — tag, release workflow, GHCR images. A consolidation milestone now that Actions + all six backends + the runner topology are complete.
3. **Further sim fidelity audits** — same method: narrow the coverage map to ops a stable client calls, probe with assertions.

## Invariants

- Never auto-merge PRs; the user handles merges.
- Rebase PR branches on `origin/main` before pushing; sync local `main` after.
- File a concrete `BUGS.md` entry before fixing a discovered defect.
- No stubs, fakes, mocks, synthetic responses, silent fallbacks, or degraded modes (see [AGENTS.md](AGENTS.md)).
- Simulators implement real cloud-API slices, one binary per cloud; every public endpoint ships with official SDK + vendor CLI + Terraform coverage where those surfaces exist.
- External identity stays GitHub/GHES-shaped (public paths, fields, `GITHUB_*` vars, runner contract, client-facing UI text); bleephub-specific names only for internal code or operator-only surfaces.
- Coverage authorities: [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md) and [specs/SIM_SURFACE_TABLES](specs/SIM_SURFACE_TABLES).

## Environment notes

- Simulator ports: AWS 4566, GCP 4567, Azure 4568.
- AWS and GCP Terraform providers accept localhost custom endpoints directly; AzureRM needs HTTPS through the local Caddy gateway (`make stack-https-{up,status,ca,down}`). Azure Terraform tests are Docker-only.
- Linux network-fabric tests require `CAP_NET_ADMIN` + iproute2 + nftables; off-Linux they skip through the realexec capability gate.
- Local bleephub runner topology harness: `make bleephub-runner-docker-test` (self-contained; mounts docker.sock + a sim-EFS host dir).
