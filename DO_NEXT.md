# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current State

- Branch: `fix/aws-sim-acm-dns-validation` (PR pending, closes #420 + #421).
- Last merged: PR #422 (GCP Cloud KMS service, #419).
- AWS ACM DNS validation (BUG-1464/1465, issues #420/#421): `acm.go` now (a) transitions a DNS-validated AMAZON_ISSUED cert to ISSUED on `DescribeCertificate` once every domain's `_acm-challenge` CNAME exists in the Route53 sim store (`acmReconcileIssuance`/`acmDNSRecordPresent` — honest signal, no synthetic success; a cert with no record stays PENDING), and (b) builds the validation record name from `strings.TrimPrefix(domain, "*.")` so a wildcard SAN no longer yields a literal `*`. SDK (`acm_dns_validation_test.go`), CLI (`acm_dns_validation_test.go` — request + route53 record + describe→ISSUED), and Terraform (`terraform-tests/acm-validation/` — `aws_acm_certificate` wildcard SAN + `aws_route53_record` for_each over DVOs + `aws_acm_certificate_validation`) all pass locally. ACM already had a surface table + matrix row (no new ops).
- Next queued: Azure KV #423 (version-less key crypto routing).
- Open GitHub issues: #420/#421 (closing via pending PR). #423 (Azure KV, queued). #394 (azuread upstream blocker).
- Open BUG trackers: BUG-1075, BUG-1104, BUG-1345.
- BUG counters: 1465 filed · 1422 fixed · 5 open · 3 false positives.

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
