# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/azf-clouddns-hardening` (ready for PR). Hardens the merged azure-cells cloud-dns work: (1) azf `NetworkConnect` now registers `--network-alias` on connect-after-create — a PendingCreate gets the network+alias stamped (so `ContainerStart`'s cloud-dns site deploy VNet-integrates + registers it), and a live site gets VNet-integrated + its aliases written as Private DNS CNAMEs now (the synthetic network driver writes only `Store.Containers`, which the stateless backend ignores, so without this a post-create connect was lost); (2) completes the App Service swift-VNet-integration testing contract — CLI (`az rest` round-trip) + Terraform (`azurerm_app_service_virtual_network_swift_connection` + a `Microsoft.Web/serverFarms`-delegated subnet, apply/idempotency/destroy) joining the SDK test from #587. Three sim fidelity bugs surfaced + fixed: **BUG-1833** the swift response returned its `id`/`type` from the operation path (`networkConfig`) instead of the canonical config sub-resource (`config/virtualNetwork`), so the azurerm provider's apply failed `ID was missing the 'config' element` (the #587 SDK test missed it — asserted only `subnetResourceId`; SDK+CLI now assert the `id` too); **BUG-1832** subnet delegation dropped the `actions` array (azurerm idempotency drift); **BUG-1831** swift PUT force-started a container for any non-HTTP site, 500ing an imageless function app's VNet integration (now gated on the site actually having a container image). |
| Last merged (#587) | **bleeplab GitLab cells GREEN on aca + azf** (BUG-1828 closed). The full GitLab docker-executor flow (build → artifact → redis `services:` PING/SET/GET) runs on **both** Azure backends. **aca:** runner stages over HTTP buffered-invoke to a **faithful ACA ingress** (`registerContainerAppsIngress`, Host=App-FQDN → replica `targetPort`) + a backend WS keepalive fixing a real all-FaaS infinite-hang. **azf:** persistent App Service site container, `ContainerStart` CloudState-fallback + re-invoke, OpenStdin-running CloudState, stdin-attach precedence (attach strategy), Azure Files share mounts, ExposedPorts — capped by **faithful cloud-dns service discovery** (VNet + `Microsoft.Web/serverFarms`-delegated subnet + App Service swift VNet integration + linked Private DNS zone; the sim realizes a Web-delegated subnet as a Docker network, attaches the site on swift integration, realizes a CNAME→site-FQDN as an embedded-DNS alias). Same backend code vs sim and real Azure, no sim-awareness. Also: AWS sim ECS CPU/Mem enforcement (#583) + process-mode EBS hardening (#569); CI flake fix (BUG-1830, AWS SDK build split into its own step). Bugs closed: 1815–1830. |
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
| Bugs | See [BUGS.md](BUGS.md) header (1833 filed · 1791 fixed · 2 open · 7 FP). 2 open: BUG-1075 (live-cloud), BUG-1345 (azuread upstream). |
| Live infra | None up. |

## What's next

Ordered continuation plan (full detail in [PLAN.md](PLAN.md) § Next; resume steps in [DO_NEXT.md](DO_NEXT.md)):

- **GitLab docker-executor arc — COMPLETE.** The full build → artifact → `services:` flow is GREEN on every cloud backend: ECS (#578/#581), Cloud Run (#585), GCF (#586), ACA + AZF (#587). FaaS multi-container pod assembly (BUG-1781) done.
- **A. Harden the merged cloud-dns work — DONE (CURRENT branch, ready for PR).** Swift-VNet-integration testing contract completed (CLI + Terraform, joining the SDK test from #587); azf `NetworkConnect` now registers `--network-alias` on connect-after-create (PendingCreate stamp + live VNet-integrate/CNAME). Surfaced + fixed two sim fidelity bugs (BUG-1831 swift-PUT imageless-site 500, BUG-1832 subnet-delegation `actions` drop).
- **B. GitHub `actions/runner` cells on aca + azf** — mirror the GitLab arc for true GitHub+GitLab parity on the Azure backends.
- **C. Standing** — live-cloud pass (BUG-1075, biggest open gap), releases (#363), fresh sim fidelity audits.

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
