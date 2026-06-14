# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/gcf-topology-cell` (PR pending). |
| In-flight | **The GCF (Cloud Run Functions) GitHub-topology cell is fully green — bleephub `gcf` harness TEST 1–14 all pass**, joining ECS, ACA, and Cloud Run. GCF Gen2 deploys container-jobs as Cloud Run Service revisions, so the cell reuses the cloudrun overlay + gcs-sync model; five gaps were the GCF twins of cloudrun fixes (BUG-1795): `Typed.Exec` rewired through `s.ExecStart`, materialize-on-exec, `warmBootstrap`, bootstrap readiness route + gcs-sync `ExecHooks`, and `STORAGE_EMULATOR_HOST` honored+injected. **Also instrumented the whole exec-via-agent path (BUG-1796):** the GCF bring-up exposed that a reverse-agent `TypeError` was swallowed (opaque exit 255, cause stranded in the workload's stderr) — now surfaced to the caller's stream + mapped to exit 255, with the full exec lifecycle (dispatch/driver/exit/session) logged across all FaaS backends. |
| Last merged | #572 Cloud Run cell GREEN (BUG-1794 + BUG-1792). #571 BUG-1794 filed + timeout. #570 #569 process-mode managed-EBS + cloudrun gcs-sync. #567 Cloud Run cell bring-up. |
| Open GitHub issues | #394 azuread Terraform Graph override — upstream-blocked (BUG-1345). |
| Bugs | See [BUGS.md](BUGS.md) header. 3 open: BUG-1075 (live-cloud), BUG-1345 (azuread upstream), BUG-1781 (FaaS multi-container pods). |
| Live infra | None up. |

## What's next

Ordered continuation plan (full detail in [PLAN.md](PLAN.md) § Next; resume steps in [DO_NEXT.md](DO_NEXT.md)):

- **A. Arc 3 — GitLab docker-executor parity** — a sim-backed harness proving the helper + build + service-container flow across backends.
- **B. FaaS multi-container pod assembly (BUG-1781).**
- **C. Standing** — live pass (BUG-1075), releases (#363), sim audits.

All four container backends (ECS, ACA, Cloud Run, GCF) are now sim-proven for the full GitHub container-job topology.

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
