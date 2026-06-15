# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/faas-pod-polish-bleeplab-cells` (PR pending). |
| In-flight | **AZF pod polish + bleeplab artifact UI.** (1) **AZF pod shared-workspace volume** — every pod member's named-volume binds attach to the site once (`UpdateAzureStorageAccounts`) and each sitecontainer mounts it via `SiteContainerProperties.VolumeMounts`; the sim realizes a shared Docker volume across members (persists across stages, torn down on site delete). (2) **AZF per-sidecar exec** — sidecars now run the overlay in *sidecar mode* (`SOCKERLESS_SIDECAR=1`: bootstrap dials its own reverse-agent + execs the service, no HTTP-port bind), so `docker exec <sidecar>` works (mirrors Cloud Run); raw-image fallback when no overlay. (3) **bleeplab artifact browse UI** — `jobView.artifact_filename` + an unauthenticated `GET /internal/jobs/{id}/artifact` download route + an Artifacts panel on the GitLab-themed JobDetail page. All proven: `TestAZFMultiContainerPodSharesLocalhost` now also asserts a shared `/shared` file + `docker exec` into the sidecar; `TestArtifactFlow` asserts the internal route + filename. |
| Prev merged (#582) | FaaS multi-container pod assembly (BUG-1781): lambda+gcf already delivered; azf assembled via App Service sitecontainers. |
| Prev merged (#581) | Full gitlab-runner `services:` on the bleeplab GitLab ECS cell (BUG-1804 Cloud Map one-instance-many-DNS-names + BUG-1805 dropped the ECS resolv.conf wrapper). |
| Prev merged (#580) | bleeplab dashboard UI (GitLab-themed) — completed bleephub parity (git + artifacts + UI). |
| Prev merged (#579) | bleeplab object-store-backed CI artifacts (cross-stage passing) + BUG-1803 (azf attach stdin-capture race). |
| Prev merged (#578) | The single-job bleeplab GitLab ECS cell GREEN: **BUG-1801** bleeplab serves each project as a real git repo over smart-HTTP (object-store-backed go-git), `GIT_STRATEGY: clone` materializes `CI_PROJECT_DIR`; **BUG-1802** the ECS backend reconstructs a container's `HostConfig.Binds` from the task def's mount points on restart so gitlab-runner's per-stage helper restarts keep the EFS `/builds` mount. |
| Last merged | #577 BUG-1800 EFS access-point writability. #576 BUG-1798 ECS gitlab attach-stdin. #575 bleeplab ECS harness (BUG-1797, BUG-1799). #574 bleeplab GitLab sim. |
| Open GitHub issues | #394 azuread Terraform Graph override — upstream-blocked (BUG-1345). |
| Bugs | See [BUGS.md](BUGS.md) header. 2 open: BUG-1075 (live-cloud), BUG-1345 (azuread upstream). |
| Live infra | None up. |

## What's next

Ordered continuation plan (full detail in [PLAN.md](PLAN.md) § Next; resume steps in [DO_NEXT.md](DO_NEXT.md)):

- **A. bleeplab GitLab cells on cloudrun / gcf / aca (NEXT, own PR).** The ECS cell is green; extend the harness (one Dockerfile + `BLEEPLAB_BACKEND` switch, mirroring bleephub's 4-backend harness): add the gcp/azure sims + cloudrun/gcf/aca backends + bootstraps, `provision_cloudrun`/`gcf`/`aca` (GCS/Azure-Files, fake-SA, AR/ACR endpoint override, gcs-sync, reverse-agent), the `:5000` sim-registry publish + trust, and a Makefile target per cell. Validate each GREEN (cloudrun first — smallest lift; gcf watch BUG-964). This is validation-heavy (overlay build→push→pull→reverse-agent), so it's its own PR.
- **B. FaaS multi-container pod assembly (BUG-1781) — DONE** (lambda + gcf already delivered; azf assembles via App Service sitecontainers, now with shared-workspace volumes + per-sidecar exec).
- **C. Standing** — live pass (BUG-1075), releases (#363), sim audits.

All four container backends (ECS, ACA, Cloud Run, GCF) are sim-proven for the full GitHub container-job topology; GitLab control plane now exists (bleeplab) and runs jobs with a real gitlab-runner.

## Invariants

- Never auto-merge PRs; the user handles merges.
- **At most one PR open at a time** — put all work in the single in-progress PR; never open a new one while one exists. If two ever exist, **consolidate** their work into one (merge the branches together) — do not evade the rule. Closing a PR *without merging it* abandons and deletes that work for good; it is never a way to park work or dodge the rule. Enforced by `scripts/check-single-open-pr.sh` (pre-commit + the `single-open-pr` CI job).
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
