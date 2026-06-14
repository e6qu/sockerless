# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/cloudrun-gcs-sync-dataplane-1792` (PR pending). |
| In-flight | **BUG-1792 gcs-sync exec data plane — wired, integration debug remains.** Implemented the full data plane: cloudrun `ExecStart` runs `GCSSyncDriver.PreExec` per gcs-sync volume + injects `SOCKERLESS_SYNC_VOLUMES` + `PostExec` on exec-stream close; the shared agent gained `ExecHooks` (pre/post) threaded through `ServeReverseAgentWithExecHooks` → `Router` → `ExecSession` so the **WS** exec path syncs (only the HTTP `handleInvoke` path did before); the cloudrun bootstrap passes gcs-sync restore/save hooks. Compiles + agent unit tests green. TEST 12 still exits 255: `gcsSyncPreExec` is a runtime no-op (finds no gcs-sync volumes in `s.config.SharedVolumes` at ExecStart — resolve why) + intermittent reverse-agent registration timeout under local disk pressure. |
| Last merged | #568 BUG-1792 prerequisites + BUGS.md count fix. #567 Cloud Run cell bring-up (BUG-1789/1790/1791). #566 BUG-1785. #565 ACA cell green. |
| Open GitHub issues | #394 azuread Terraform Graph override — upstream-blocked (BUG-1345). Re-check GitHub before non-conformance issue work. |
| Bugs | See [BUGS.md](BUGS.md) header for exact counts. 4 open: BUG-1075 (live-cloud), BUG-1345 (azuread upstream), BUG-1781 (FaaS multi-container pods), BUG-1792 (cloudrun gcs-sync exec data plane unwired — remaining: wire PreExec/PostExec around the exec dispatch). |
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
