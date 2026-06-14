# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `debug/cloudrun-reverse-agent-blocker-1794` (PR pending — docs + timeout only). |
| In-flight | **BUG-1794 filed: the cloudrun Service bootstrap never registers its reverse-agent → TEST 12 blocked** (and so the merged BUG-1792 gcs-sync data plane is unvalidated). A fresh podman machine ruled out the "degraded engine" theory — it's a real bug. Established: overlay build→push→pull works (fresh machine needs `podman machine` restart so buildah honors the sim-registry insecure drop-in); the Service reaches "Ready" + skips default-invoke correctly; the callback `host.docker.internal:3375` IS reachable from a container during a live run; but the Service (overlay) container is created and **removed within ~1s**, so the bootstrap stderr is uncapturable. Bootstrap-timeout lowered to 180s so the failure surfaces as "did not register" not a job-timeout. Next: a sim debug knob to keep the stopped Service container (or persist its log sink) to see the crash. |
| Last merged | #570 #569 process-mode managed-EBS fix + cloudrun gcs-sync data plane (WIP). #568 BUG-1792 prereqs. #567 Cloud Run cell bring-up. #566 BUG-1785. |
| Open GitHub issues | #569 (BUG-1793) FIXED in #570. #394 azuread Terraform Graph override — upstream-blocked (BUG-1345). |
| Bugs | See [BUGS.md](BUGS.md) header. 5 open: BUG-1075 (live-cloud), BUG-1345 (azuread upstream), BUG-1781 (FaaS multi-container pods), BUG-1792 (cloudrun gcs-sync data plane — wired, awaiting validation), BUG-1794 (cloudrun reverse-agent registration — the live blocker). |
| Live infra | None up. |

## What's next

Ordered continuation plan (full detail in [PLAN.md](PLAN.md) § Next; resume steps in [DO_NEXT.md](DO_NEXT.md)):

- **A. Finish the Cloud Run cell** — BUG-1792 (gcs-sync workspace reachability) is the last TEST 12 gate; then TEST 13 (service container) + TEST 14 (dispatcher). The build→deploy→materialize→exec pipeline already works against the gcp sim.
- **B. GCF topology cell** — same `cloudrun-bootstrap` overlay model.
- **C. Arc 3 — GitLab docker-executor parity.**
- **D. FaaS multi-container pod assembly (BUG-1781).**
- **E. Standing** — live pass (BUG-1075), releases (#363), sim audits.

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
