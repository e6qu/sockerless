# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current State

- Branch: `test/azure-sim-coverage-gaps` (PR pending — test-only).
- Last merged: PR #425 (Azure KV version-less key crypto, #423).
- Azure test-gap audit: the three headline gaps from the old roadmap (App Insights SDK/CLI, Private DNS A-record SDK, ACR image-ops SDK) were ALREADY covered (insights_test.go, dns_private_test.go, acr_test.go). The genuine remaining gap was **Private DNS non-A record types** — A has a dedicated handler + test; AAAA/CNAME/MX/PTR/SRV/TXT go through a separate generic-loop handler (dns.go:428) that was untested. Added `dns_private_records_test.go` (per-type round-trip for all six). Also added `TestAppInsights_BillingFeatures` (the one untested App Insights SDK op). BUG-1467 was a FALSE POSITIVE: suspected the billing-features response used wrong casing, but App Insights legacy billing-features genuinely uses PascalCase (confirmed vs the SDK serde); a camelCase "fix" broke it and was reverted. The new test guards that.
- Open GitHub issues: none actionable (#394 upstream-blocked).
- Open BUG trackers: BUG-1075, BUG-1104, BUG-1345.
- BUG counters: 1467 filed · 1423 fixed · 5 open · 4 false positives.
- After this: GCP coverage-gap PR (SA-key + instance-template route impls — real missing ops, not just tests), or await new consumer issues.

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
