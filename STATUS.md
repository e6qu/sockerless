# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `fix/faithful-registry-roundtrip-1785` (PR pending) |
| In-flight (this PR) | **BUG-1785 azure half — faithful build→push→pull for ACR Tasks.** The sim's ACR Tasks built the overlay into the host's local docker daemon and ran it locally (registry never saw the build); an earlier attempt wrongly coupled the shared workload runner to the registry's in-process store (a dependency that doesn't exist in the cloud — user-corrected). Now the build does a real `docker push` to the registry and the run a real `docker pull`, registry and compute agnostic (connected only by `/v2/`). Backend honors `SOCKERLESS_AZURE_ACR_ENDPOINT` (custom-cloud override); the harness publishes the sim `/v2/` at `127.0.0.1:5000` + a podman-machine insecure drop-in (Docker auto-trusts loopback; Podman doesn't). Validated: ACA harness TEST 12 + the ACR Tasks SDK test (real registry:2 stand-in). The gcp Cloud Build half remains (larger — touches cloudrun/gcf overlay flows + their integration tests). |
| Last merged | #558 ACA GitHub container-job topology (TEST 12 green; BUG-1782 + BUG-1783). |
| Open GitHub issues | #394 azuread Terraform Graph override — upstream-blocked (BUG-1345). Re-check GitHub before non-conformance issue work. |
| Bugs | See [BUGS.md](BUGS.md) header for exact counts. 5 open: BUG-1075 (live-cloud), BUG-1345 (azuread upstream), BUG-1781 (FaaS multi-container pods), BUG-1784 (ACA service-container discovery), BUG-1785 (gcp Cloud Build push→pull half remains). |
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
