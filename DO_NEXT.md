# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current State

- Branch: `main` (clean).
- Last merged: PR #395 (BUG-1104 coverage audit — surface tables and matrix backfill).
- Open GitHub issues: #394 (azuread Terraform provider upstream blocker — waiting on hashicorp).
- Open BUG trackers: BUG-1075, BUG-1104, BUG-1345.
- BUG counters: 1347 filed · 1344 fixed · 5 open · 3 false positives.

## Recently Completed

| PR | Description |
|----|-------------|
| #392 | GCP SA keys, instance templates, Cloud Build/Logging/IAM SDK+CLI tests |
| #393 | bleephub POST /admin/organizations; Azure Entra Graph provisioning + ROPC |
| #394 | (issue) azuread Terraform provider upstream blocker documented |
| #395 | BUG-1104 audit: surface tables and matrix backfill for PRs #388/392/393 |

## Deferred / Blocked

| Item | Blocker |
|------|---------|
| `azuread_group` / `azuread_user` Terraform tests (BUG-1345) | Upstream: no `microsoft_graph_endpoint` override in `hashicorp/terraform-provider-azuread` (issue #1837 upstream, issue #394 here) |
| Live-cloud validation (BUG-1075) | Requires authenticated real-cloud runs; no timeline |

## What to Work On Next

No queued work items. The simulator coverage is current for all implemented slices, all surface tables are up to date, and the only open issue (#394) is blocked upstream.

Potential directions (discuss with user before starting):
- New simulator slices for any cloud area not yet covered (check `specs/SIM_TEST_COVERAGE_MATRIX.md` for gaps).
- Hardening existing surfaces (e.g., pagination shape verification for any `n/a` rows).
- Picking up live-cloud validation for any backend (BUG-1075).

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
