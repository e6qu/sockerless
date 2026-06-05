# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/gcp-sim-cloudkms` (PR pending — GCP Cloud KMS, issue #419) |
| In-flight | GCP Cloud KMS service (BUG-1463, issue #419): keyRings/cryptoKeys/cryptoKeyVersions + symmetric encrypt/decrypt with real AES-256-GCM and CRC32C integrity fields. SDK + CLI (`gcloud kms`) + Terraform coverage all green locally. |
| Last merged | PR #418 — DynamoDB GSIs (#416) + ECS Service family (#417) + audit follow-ups (BUG-1457–1460); azf attach-stdin race (BUG-1461); CloudFront Function tagging (BUG-1462) |
| Also merged recently | PR #415 (KMS tagging #413, EC2 API-only modeling #414, Podman image fix); PR #412 (Azure KV version ordering #407) |
| Open GitHub issues | #419 — GCP Cloud KMS (closing via the pending PR). #394 — azuread TF provider upstream blocker (waiting on hashicorp) |
| Bugs | 1463 filed · 1420 fixed · 5 open · 3 false positives |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1345 azuread upstream |
| Planned next | After Cloud KMS PR: planned Azure test-gap PR, then GCP coverage-gap PR |
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
