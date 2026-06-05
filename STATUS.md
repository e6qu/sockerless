# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/sim-oci-registry-data-plane` (PR pending — shared OCI /v2/ data plane, BUG-1491–1493, #450–#452) |
| In-flight | Cross-cloud OCI Distribution `/v2/` Docker Registry data plane (real `docker push`/`pull` through the shim), one shared library wired into all three sims: **#450** AWS ECR (had no /v2/ at all); **#451** GCP AR (chunked PATCH 405'd); **#452** Azure ACR (blob-upload POST 404'd). New `shared/oci.go` (`sim.OCIRegistry`: base route, chunked blob upload start/PATCH/finalize with sha256 verify, blob GET/HEAD, manifest PUT/GET/HEAD/DELETE by tag+digest, tags/list) mounted per-method on /v2/ (avoids the awsJson `POST /` + apigatewayv2 `/v2/apis` mux conflicts). GCP/Azure's duplicated OCI handlers retired; GCP keeps pull-through hydration via the `HydrateManifest` hook. SDK tests (raw-HTTP chunked push+pull) per cloud. |
| Last merged | PR #449 — six consumer issues (BUG-1485–1490, #441–#447) |
| Also merged recently | PR #448 (three flagged follow-ups); PR #440 (five AWS sim gaps #434–#438); PR #439 (EC2 Launch Template #433) |
| Open GitHub issues | None actionable — only #394 (azuread TF provider upstream blocker) |
| Bugs | 1493 filed · 1449 fixed · 5 open · 4 false positives |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1345 azuread upstream |
| Planned next | Consumer issue queue drained (only #394 upstream-blocked). Options: fresh fidelity audit; Phase G new slices (GCP Spanner/Dataflow/Bigtable, Azure); or await new consumer issues |
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
