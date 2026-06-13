# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/cloud-backend-network-driver` (PR pending) |
| In-flight | Groundwork from the Arc-2 ACA cell stand-up. **BUG-1780**: only the ecs backend used the metadata-only `SyntheticNetworkDriver`; lambda/cloudrun/gcf/aca/azf fell through to the real-Linux-netns driver (`ip netns add` + veth), which 400s `docker network create` without iproute2 and leaks a meaningless kernel netns where it succeeds. All five now mirror ecs (docker networks map to *cloud* primitives, never a local netns); all six backends' tests pass and a harness run confirmed ACA network create/delete now provisions + tears down the NSG + Private DNS zone. Codified two principles across AGENTS.md + CLOUD_RESOURCE_MAPPING.md + the AZF README: (1) **experiential parity** — every Docker abstraction (networks, multi-container pods incl. localhost loopback, volumes) is *assembled* from cloud primitives on every backend, FaaS included, so the experience matches local Docker/Podman; FaaS multi-container pods are ours to assemble, not reject (filed **BUG-1781**, PLAN § Next #1); (2) **sims stay faithful cloud slices** — no special/fake functionality for sockerless backends or runners. The ACA topology harness plumbing + container-job exec (faithful ACR-Tasks bootstrap overlay) is the next arc. |


| Last merged | #555 pod-model correctness (lambda/gcf cloud-resource leak, isolation-lint Pod* patterns, AZF fail-fast). |
| Open GitHub issues | #394 azuread Terraform Graph override — upstream-blocked (BUG-1345). Re-check GitHub before non-conformance issue work. |
| Bugs | See [BUGS.md](BUGS.md) header for exact counts. 3 open: BUG-1075 (live-cloud cells), BUG-1345 (azuread upstream), BUG-1781 (FaaS multi-container pod assembly). |
| Live infra | None up. |

## What's next

Candidates, no committed timeline (see [PLAN.md](PLAN.md) § Next):

1. **FaaS multi-container pod assembly (BUG-1781)** — assemble pod semantics (with docker networks) from cloud primitives on lambda/gcf/azf so `services:`/sidecar `container:` jobs run there too; replaces the interim fail-fast rejections.
2. **GitHub topology harness sweep** — finish Arc 2: ACA (in flight) → Cloud Run → GCF, proving container jobs + service containers + the dispatcher loop sim-backed on each.
3. **Runner-as-cloud-task live pass** — cells 1+2 are sim-proven; the remaining piece is the live run against real ECS/Lambda (BUG-1075, user-gated spend).
4. **Issue #363 — first versioned release** — tag, release workflow, GHCR images.
5. **Further sim fidelity audits** — same method: narrow the coverage map to ops a stable client calls, probe with assertions.

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
- Local bleephub runner topology harness: `make bleephub-runner-docker-test` (ECS) / `make bleephub-runner-docker-test-aca` (ACA); self-contained, mounts docker.sock + a sim-storage host dir. `BLEEPHUB_BACKEND` selects the backend; `BLEEPHUB_TEST_FROM` skips to a test; `BLEEPHUB_HOLD=1` freezes the stack on failure. The one harness image bundles the aws + azure sims, backend-ecs + backend-aca, and the cloudrun-bootstrap.
