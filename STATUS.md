# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/azf-multicontainer-pods` (FaaS pod assembly; PR pending). |
| In-flight | **FaaS multi-container pod assembly (BUG-1781).** Investigation found **lambda and gcf already deliver** shared-localhost pods (lambda: chroot subprocesses of one supervisor in one execution env → one shared netns; gcf: one multi-container Cloud Run revision + `/etc/hosts` alias→127.0.0.1). The remaining gap was **azf, which hard-rejected multi-container pods** — now fixed by assembling the pod as ONE App Service site with **sitecontainers** (the native Azure multi-container primitive, one `isMain` + N sidecars sharing a netns). The **azure sim** models the `sitecontainers` sub-resource (CRUD) + starts main+sidecars sharing one netns on invoke (SDK+CLI tests; `TestSDK_AzureFunctions_MultiContainerSharesLocalhost`). The **azf backend** replaces the two fail-fast rejections with a network-pod materializer (mirrors gcf's `shouldDeferOrMaterializeNetworkPod`): the site's `isMain` runs the reverse-agent overlay, sidecars run their RAW service images; cloud-state reconstructs members from a site manifest tag (stateless). The **azf bootstrap** writes `SOCKERLESS_HOST_ALIASES` to `/etc/hosts` for by-name resolution. Proven end to end: a GitHub-`services:`-shaped pod (job + service) on the azf sim — the job reaches the sidecar on `localhost:9099` AND by alias `svc` (`TestAZFMultiContainerPodSharesLocalhost`). |
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

- **A. Arc 3 — GitLab docker-executor parity (in progress).** Phase 1 (the `bleeplab` control-plane sim) is done + real-runner-validated. **Phase 3:** point the runner's `--docker-host` at a sockerless backend; one cloud job end-to-end. **Phase 4:** a `bleeplab-runner-docker-test` harness mirroring the bleephub TEST suite (multi-stage, services, artifacts) across backends.
- **B. FaaS multi-container pod assembly (BUG-1781) — DONE** (lambda + gcf already delivered; azf now assembles via App Service sitecontainers). Remaining FaaS pod polish: a shared-workspace volume across pod members and per-sidecar exec routing are follow-on niceties, not blockers.
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
