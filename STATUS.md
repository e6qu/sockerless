# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/bleeplab-azure-cells` → **PR #587 open**. aca cell GREEN; azf cell RED (BUG-1828, tracked follow-up). |
| In-flight | **bleeplab GitLab cells on aca + azf (one PR).** **aca cell — GREEN (all 4 tests, incl. the redis `services:` job — PING/SET/GET).** The services job was RED (runner-stage reverse-agent WS exec half-opened backend→container under per-stage churn). **Fixed (BUG-1825)** by porting cloudrun/gcf's HTTP buffered-invoke to aca: runner stages POST the exec envelope over HTTP to the App ingress at `/`, via the `EndpointURL` coordinate + the App's `LatestRevisionFqdn` Host header; the azure sim implements **faithful ACA ingress** (`registerContainerAppsIngress` — Host=App-FQDN → reverse-proxy to the replica's `targetPort`, like real ACA + the storage data-plane; replaced an earlier sim-specific `/aca-app-invoke` smell). Also landed: backend WS **keepalive** (`agent.ReverseAgentConn`) fixing a real all-FaaS infinite-hang (cloudrun no-regression); **BUG-1824** Makefile `docker build --load`; aca hurdles **BUG-1815..1823**; **BUG-1826** azf overlay base-ref; **BUG-1827** azure-sim AZF invoke reach by container bridge-IP. **AWS sim (#583/#569, BUG-1827):** ECS now enforces the advertised CPU/Memory on the launched container (cgroup); process-mode managed-EBS hardened (no nil-Docker panic) — SDK probes green. **CI fix (BUG-1829):** `TestACAGitLabRunnerAttachStdin` asserted pre-branch stdin→command semantics on a `FROM scratch` overlay (no `/bin/sh`); realigned to the gitlab-runner `/bin/sh` pattern (busybox base + shell-script stdin, mirroring the gcf test) — `test (azure backends)` GREEN again. **azf cell: RED — BUG-1828 (open, multi-hurdle bring-up, partially landed):** the sim now runs the App Service site container **persistently** (lazy-start, kept until site DELETE) and the azf backend `ContainerStart` gained the BUG-1811 CloudState-fallback + re-invoke, so gitlab-runner's per-stage `start→wait→stop→start` no longer 404s — job 4 now runs its stages. Next hurdle: after `prepare_script`, azf CloudState overlays the container `exited`, so `get_sources`' `docker exec` 409s and the clone is skipped (`step_script` then `cd /builds` fails, exit 2). Fix shape (mirror gcf BUG-1811/1812): report an OpenStdin runner container **running** across invokes + route `get_sources` exec to the bootstrap. Existing azf integration + FaaS-smoke suites stay green with the persistent-container change. |
| Prev merged (#586) | bleeplab GitLab cell on the Cloud Run Functions (gcf) backend — GREEN (BUG-1811 ContainerStart CloudState fallback, BUG-1812 stdin-attach precedence, BUG-1813 `/bin/sh` stage, BUG-1814 persist-restore-before-invoke; arch via `gcpcommon.ArchFromPlatform`). |
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
