# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/bleeplab-gcf-cell` (PR pending). |
| In-flight | **bleeplab GitLab cell on the Cloud Run Functions (gcf) backend — GREEN.** Extended the one-image, `BLEEPLAB_BACKEND`-switched harness (bleeplab/Dockerfile + run-integration.sh + Makefile) to run the full gitlab-runner docker-executor flow on gcf: a 3-stage pipeline (build → test/artifact → `services:`) passes all 4 tests — gcc-compiled `calc.c` (self-test, 6×7=42, sum 5050), cross-stage `calc` artifact passing, and redis reached by alias over the per-build network-pod (PING/SET/GET). gcf reuses the gcp sim + the cloudrun backend's gcp-common; the redis `services:` job exercises the BUG-1781 network-pod (multi-container revision) assembly. Validation surfaced + fixed 4 real bugs: **BUG-1811** gcf `ContainerStart` only resolved from `PendingCreates`, so a gitlab-runner per-stage container re-start failed NOT FOUND → added the `ResolveContainerAuto` CloudState fallback (mirrors cloudrun); **BUG-1812** gcf `ContainerAttach` routed a gitlab-runner stdin-script to the (mp-less) reverse-agent → the stdinPipe/buffered-invoke path now takes precedence for `opts.Stdin`; **BUG-1813** a gitlab-runner attach-stdin script was piped to the image's own entrypoint (`gitlab-runner-build`, which ignores it) instead of `/bin/sh` → override `invokeArgv=[/bin/sh]` when stdin is captured (mirrors cloudrun's `postBootstrap`); **BUG-1814** a reused gcf function instance restored its persist (gcs-snapshot) `/builds` only once at startup, so `upload_artifacts` couldn't see the build container's `calc` → restore persist volumes before every invoke (cloudrun gets this free via fresh per-stage instances). The gcf BackendDescriptor arch is now derived from `BuildPlatform` via the shared `gcpcommon.ArchFromPlatform` (BUG-1808 class), and the gcp sim's gcf function-invoke reaches the workload by bridge container IP (BUG-1810 class). |
| Prev merged (#585) | bleeplab GitLab cell on the Cloud Run backend — GREEN (BUG-1808 hardcoded arch, BUG-1809 AR `gitlab-registry` pull-through, BUG-1810 sim-in-container reaches workload by bridge IP). |
| Prev merged (#584) | AZF pod polish (shared volume + per-sidecar exec) + bleeplab artifact UI + flaky-test sweep (BUG-1806) + server ReadHeaderTimeout (BUG-1807). |
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

- **A. bleeplab GitLab cell on aca (NEXT, own PR).** The ECS, cloudrun **and gcf** cells are green. **aca** adds the azure sim + ACR + the `*.blob.localhost` hosts alias (its own validation-heavy PR: overlay build→push→pull→reverse-agent). The redis `services:` job exercised the gcf network-pod cleanly (no BUG-964 gate hit). **BUG-1810/1814-class issues likely recur on aca** — the azure sim's Cloud Run-style invoke path and per-stage instance reuse; check the azure sim's `postCloudRunServiceInstance` analog + the aca bootstrap's persist restore-before-invoke.
- **B. FaaS multi-container pod assembly (BUG-1781) — DONE** (lambda + gcf already delivered; azf assembles via App Service sitecontainers, now with shared-workspace volumes + per-sidecar exec).
- **C. Standing** — live pass (BUG-1075), releases (#363), sim audits.

All four container backends (ECS, ACA, Cloud Run, GCF) are sim-proven for the full GitHub container-job topology; GitLab control plane now exists (bleeplab) and runs jobs with a real gitlab-runner — the full GitLab docker-executor flow (build → artifact → `services:`) is GREEN on **ECS, Cloud Run, and GCF** (aca next).

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
