# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/bleeplab-ecs-harness` (Arc 3 Phase 3; PR pending). |
| In-flight | **Arc 3 Phase 3 in progress: the bleeplab ECS harness drives a real `gitlab-runner` 18.11 against sockerless-backend-ecs.** The runner registers with bleeplab, claims a job, uses sockerless as its `--docker-host`, and image pull + build/helper container create all work. Fixed a real core bug found en route (**BUG-1797**: image manifest selection hardcoded amd64 → arch-aware via `SOCKERLESS_WORKLOAD_ARCH`, so the arm64-only gitlab-runner-helper tag pulls on arm64). **Remaining gate — BUG-1798:** the helper container's attach-stdin script isn't delivered (the ECS deferred-RunTask runs the image-default `gitlab-runner-build` instead of baking the piped script), so the job hangs in `Preparing environment`. (Phase 1 — the bleeplab control-plane sim — merged in #574.) |
| Last merged | #574 bleeplab GitLab control-plane sim (Arc 3 Phase 1). #573 GCF cell GREEN (BUG-1795) + exec observability (BUG-1796). #572 Cloud Run cell GREEN (BUG-1794 + BUG-1792). |
| Open GitHub issues | #394 azuread Terraform Graph override — upstream-blocked (BUG-1345). |
| Bugs | See [BUGS.md](BUGS.md) header. 4 open: BUG-1075 (live-cloud), BUG-1345 (azuread upstream), BUG-1781 (FaaS multi-container pods), BUG-1798 (bleeplab ECS gitlab-helper attach-stdin — the Phase-3 gate). |
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
