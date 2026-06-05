# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current State

- Branch: `fix/azure-sim-kv-versionless-crypto` (PR pending, closes #423).
- Last merged: PR #424 (AWS ACM DNS validation, #420 + #421).
- Azure KV version-less key crypto (BUG-1466, issue #423): `handleKVKey` only routed crypto for the 4-segment `/keys/{name}/{version}/{verb}` form; the version-less 3-segment `/keys/{name}/{verb}` (`encrypt`/`decrypt`/`sign`/`verify`/`wrapkey`/`unwrapkey`) fell through to 405. Added a `len(segs)==3 && kvIsCryptoVerb(verb)` route that calls `handleKVCryptoKey(..., version="", verb)`; `findVersion("")` already resolves to `latest()` (same as the version-less GET). SDK (`keyvault_versionless_crypto_test.go` — azkeys Encrypt/Decrypt/Sign/Verify/Wrap/Unwrap with version "") and CLI (`az rest` POST `/keys/{name}/encrypt`+`decrypt` in `keyvault_dataplane_test.go`) pass locally. No Terraform data-plane crypto surface. Internal routing only — no new ops, surface table/matrix unchanged.
- Open GitHub issues: #423 (closing via pending PR). #394 (azuread Terraform provider upstream blocker — waiting on hashicorp).
- Open BUG trackers: BUG-1075, BUG-1104, BUG-1345.
- BUG counters: 1466 filed · 1423 fixed · 5 open · 3 false positives.
- After this: no open consumer issues remain (only #394, upstream-blocked); fall back to planned Azure/GCP test-gap PRs or await new issues.

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
