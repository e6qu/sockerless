# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `debug/cloudrun-service-log-capture-1794` (PR pending). |
| In-flight | **The Cloud Run GitHub-topology cell is fully green — bleephub cloudrun harness TEST 1–14 all pass.** BUG-1794 fixed (the exec-driven scale-to-zero Service was deployed but never received a request, so its bootstrap never cold-started → reverse-agent never registered; fix: the overlay serves a `/_sockerless/ready` route that cold-starts the revision without running the keepalive, and the backend POSTs to it on materialize). BUG-1792 validated end-to-end (gcs-sync workspace round-trip — `proof.txt` written inside the job container is visible in the runner workspace; the last gap was the resumable-upload continuation URL hardcoding HTTPS, now scheme-derived). Cloud Run container backend is now sim-proven for the full build→push→pull→deploy→materialize→reverse-agent→exec→gcs-sync pipeline. |
| Last merged | #571 BUG-1794 filed + surface-the-failure timeout. #570 #569 process-mode managed-EBS fix + cloudrun gcs-sync data plane. #568 BUG-1792 prereqs. #567 Cloud Run cell bring-up. #566 BUG-1785. |
| Open GitHub issues | #394 azuread Terraform Graph override — upstream-blocked (BUG-1345). |
| Bugs | See [BUGS.md](BUGS.md) header. 3 open: BUG-1075 (live-cloud), BUG-1345 (azuread upstream), BUG-1781 (FaaS multi-container pods). |
| Live infra | None up. |

## What's next

Ordered continuation plan (full detail in [PLAN.md](PLAN.md) § Next; resume steps in [DO_NEXT.md](DO_NEXT.md)):

- **A. GCF topology cell** — same `cloudrun-bootstrap` overlay model; the Cloud Run cell (TEST 1–14 green) is the template.
- **B. Arc 3 — GitLab docker-executor parity.**
- **C. FaaS multi-container pod assembly (BUG-1781).**
- **D. Standing** — live pass (BUG-1075), releases (#363), sim audits.

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
