# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/bleeplab-services` (Arc 3; PR pending). |
| In-flight | **Cloud Map completeness + ECS service-alias registration (BUG-1804) — unblocks GitLab `services:`.** A container can now resolve under MULTIPLE Cloud Map DNS names (real Cloud Map's one-instance-many-services model): the aws sim's Docker-network DNS realization re-attaches a task container with the FULL set of service names it backs (disconnect-then-reconnect, since Docker rejects a second `NetworkConnect`); the ECS backend captures the request's `NetworkingConfig` aliases and registers the container under its hostname AND every alias (and deregisters by enumerating the namespace's services). So a GitLab `services:` alias like `redis` resolves. Proven by `TestECS_MultiServiceDNS` (a client resolves both of a server's two service names via real Cloud Map DNS) + the harness log (the redis service container carries `redis` + its hostname as namespace aliases). The full gitlab-runner `services:` e2e on ECS has a separate remaining gate (BUG-1805: `FF_NETWORK_PER_BUILD` build-container produces no trace output) — the harness `services:` job is descoped pending that; the SDK test is the standing validation. |
| Prev merged (#580) | bleeplab dashboard UI (GitLab-themed) — completed bleephub parity (git + artifacts + UI). |
| Prev merged (#579) | bleeplab object-store-backed CI artifacts (cross-stage passing) + BUG-1803 (azf attach stdin-capture race). |
| Prev merged (#578) | The single-job bleeplab GitLab ECS cell GREEN: **BUG-1801** bleeplab serves each project as a real git repo over smart-HTTP (object-store-backed go-git), `GIT_STRATEGY: clone` materializes `CI_PROJECT_DIR`; **BUG-1802** the ECS backend reconstructs a container's `HostConfig.Binds` from the task def's mount points on restart so gitlab-runner's per-stage helper restarts keep the EFS `/builds` mount. |
| Last merged | #577 BUG-1800 EFS access-point writability. #576 BUG-1798 ECS gitlab attach-stdin. #575 bleeplab ECS harness (BUG-1797, BUG-1799). #574 bleeplab GitLab sim. |
| Open GitHub issues | #394 azuread Terraform Graph override — upstream-blocked (BUG-1345). |
| Bugs | See [BUGS.md](BUGS.md) header. 4 open: BUG-1075 (live-cloud), BUG-1345 (azuread upstream), BUG-1781 (FaaS multi-container pods), BUG-1805 (gitlab-runner `services:` e2e gate on ECS). |
| Live infra | None up. |

## What's next

Ordered continuation plan (full detail in [PLAN.md](PLAN.md) § Next; resume steps in [DO_NEXT.md](DO_NEXT.md)):

- **A. Arc 3 — GitLab docker-executor parity (in progress).** Phase 1 (the `bleeplab` control-plane sim) is done + real-runner-validated. **Phase 3:** point the runner's `--docker-host` at a sockerless backend; one cloud job end-to-end. **Phase 4:** a `bleeplab-runner-docker-test` harness mirroring the bleephub TEST suite (multi-stage, services, artifacts) across backends.
- **B. FaaS multi-container pod assembly (BUG-1781).**
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
