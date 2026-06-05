# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current State

- Branch: `feat/gcp-sim-cloudkms` (PR pending, closes #419).
- Last merged: PR #418 (DynamoDB GSIs #416 + ECS Service family #417 + audit follow-ups BUG-1457–1460; azf attach-stdin race BUG-1461; CloudFront Function tagging BUG-1462).
- GCP Cloud KMS service (BUG-1463, issue #419): new `simulators/gcp/cloudkms.go` — keyRings (create/get/list, 404/409), cryptoKeys (create with `cryptoKeyId`+purpose, get/list/patch rotation), cryptoKeyVersions (list/get/destroy → DESTROY_SCHEDULED), and `cryptoKeys:encrypt`/`:decrypt` with real AES-256-GCM per-version material + CRC32C integrity fields. Accepts std AND URL-safe base64 (gcloud emits URL-safe). SDK (`cloudkms_test.go`, 3 tests), CLI (`gcloud kms` encrypt/decrypt round-trip), and Terraform (`fixtures/kms-lifecycle`, `google_kms_key_ring`+`google_kms_crypto_key`) all pass locally. Surface table `gcp-cloudkms.md` + coverage-matrix row added.
- Open GitHub issues: #419 (closing via pending PR). #394 (azuread Terraform provider upstream blocker — waiting on hashicorp).
- Open BUG trackers: BUG-1075, BUG-1104, BUG-1345.
- BUG counters: 1463 filed · 1420 fixed · 5 open · 3 false positives.

## Recently Completed

| PR | Description |
|----|-------------|
| #401 | bleephub auth conformance: session/CSRF OAuth flow + site-admin org endpoint |
| #402 | Phase C (AWS): pagination on 12 list endpoints |
| #403 | Phase C (GCP/Azure): pagination on GCP/Azure list endpoints |
| #404 | Phase D: error envelope fidelity + negative-path SDK error classification tests |
| #405 | Phase E+F: Azure KV data-plane CLI tests; 12 bleephub surface table files; webhook schema fixes (BUG-1396–1398) |

## Deferred / Blocked

| Item | Blocker |
|------|---------|
| `azuread_group` / `azuread_user` Terraform tests (BUG-1345) | Upstream: no `microsoft_graph_endpoint` override in `hashicorp/terraform-provider-azuread` (issue #1837 upstream, issue #394 here) |
| Live-cloud validation (BUG-1075) | Requires authenticated real-cloud runs; no timeline |

## What to Work On Next

**Phase G — New cloud service slices** (see PLAN.md for candidates). Each new slice ships with SDK + CLI + Terraform coverage per standard contract. No scope finalised yet — discuss with user before starting.

## Start Checklist (every session)

1. `git fetch origin && git checkout main && git reset --hard origin/main`
2. `gh issue list --state open --limit 30`
3. Check current open BUGs and the counter in `BUGS.md`.
4. Create a fresh branch from `origin/main`.
5. File BUG entries in `BUGS.md` **before** writing any code.
6. Run `go test ./...` in affected modules after every meaningful edit.

## Rules

- No stubs, fakes, mocks, synthetic responses, or silent fallbacks.
- Every new simulator public API path: SDK + CLI + Terraform coverage where those surfaces exist.
- One PR per cloud area; do not split into sub-phases.
- User merges PRs — never run `gh pr merge`.
- Rebase PR branch on `origin/main` before push.
- File bugs before fixes, not after.
