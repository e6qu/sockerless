# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `fix/aca-apps-podman-reverse-agent` (PR pending — ACA Apps Podman reverse-agent callback + continuity refresh) |
| In-flight | BUG-1576 is fixed on this branch: ACA Apps attach-stdin under local Podman now uses a runtime-reachable reverse-agent callback host, and Azure simulator workloads get explicit Podman-machine host alias mappings when needed. |
| Last merged | ECS VPC/netns/metadata/route-table/ExecuteCommand chain is merged through PR #524; prior merged PRs include #520 netns metadata + route-table egress, #519 netns VPC fabric, and #518 VPC isolation. |
| Open GitHub issues | Only upstream/maintenance trackers are known active from the local continuity docs; check GitHub before starting the next consumer issue batch. |
| Bugs | 1576 filed · 1531 fixed · 6 open · 5 false positives |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1345 azuread upstream; BUG-1540 AWS CloudTrail REST-protocol recording sweep. |
| Planned next | After this PR: pick either BUG-1540 CloudTrail REST-protocol coverage or a Phase G service-slice PR, unless new consumer issues arrive first. |
| Test-host gating | GCP/Azure Compute+Network real-exec tests skip off-Linux via `realexec.DetectNetworkCapabilities().Require()` (run for real on the sudo+iproute2/nftables CI runner). EventGrid CLI publish uses loopback + `Host` header (no `*.localhost` DNS dependency). |
| Live infra | None up |

## Invariants

### Process
- Never auto-merge PRs. User handles all merges.
- One branch per phase, one PR per phase.
- Rebase on `origin/main` before pushing.
- File concrete BUG entries before fixing.
- Update continuity docs in every PR.

### Implementation
- No stubs, fakes, mocks, synthetic responses, or silent fallbacks.
- Simulator public APIs must match real cloud public APIs exactly.
- One simulator binary per cloud.
- Every new public API slice ships with official SDK + vendor CLI + Terraform-provider coverage where those surfaces exist.
- `specs/SIM_TEST_COVERAGE_MATRIX.md` and `specs/SIM_SURFACE_TABLES/` are the coverage authorities.

### Infrastructure
- AzureRM provider requires HTTPS for custom metadata discovery (`metadata_host` is host-only); simulator runs behind local Caddy for Azure Terraform tests.
- AWS/GCP Terraform providers accept `http://localhost` custom endpoints.
- Azure simulator port: 4568; AWS: 4566; GCP: 4567.
- Caddy gateway: `make stack-https-{up,status,ca,down}`.

## Deferred Trackers

- **BUG-1075**: live-cloud validation. Do not mark cloud cells green without authenticated real-cloud runs. No timeline set.
- **BUG-1104**: audit cadence. Keep open while simulator work continues. Every simulator phase re-checks stale SDK/CLI/Terraform claims.
- **BUG-1345**: azuread Terraform provider has no `microsoft_graph_endpoint` override. Tracked as issue #394. Unblock when upstream resolves https://github.com/hashicorp/terraform-provider-azuread/issues/1837.
- **Issue #363**: versioned releases and GHCR image publishing. Intentionally deferred while the project is early.
- **Issue #394**: azuread upstream blocker (same as BUG-1345).
