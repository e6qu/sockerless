# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current State

- Branch: `feat/aws-sim-iam-policy-sim` (PR pending, closes #427).
- Last merged: PR #430 (EC2 standalone ENI ops, #428).
- AWS IAM policy simulation (BUG-1474, issue #427): new `iam_policy_sim.go` — a real evaluator (parse IAM JSON with string-or-list Action/Resource; explicit-deny-wins; `*`/`?` wildcard action (case-insensitive) + resource-ARN matching; NotAction/NotResource; condition operators StringEquals/NotEquals/Like/NotLike/EqualsIgnoreCase, Bool, ArnLike/ArnEquals, `…IfExists`, with MissingContextValues) returning EvalDecision allowed/explicitDeny/implicitDeny. `SimulateCustomPolicy` evaluates PolicyInputList; `SimulatePrincipalPolicy` resolves the role's inline (`iamRolePolicies`) + attached-managed (`iamAttachedPolicies`→`iamPolicies`) policies then reuses the evaluator. Per-(action×resource) EvaluationResults. SDK (deny-wins, resource scoping, wildcards, aws:ResourceTag condition, principal resolution) + CLI (`aws iam simulate-custom-policy`) pass. No Terraform surface (no resource/data-source for policy sim).
- After #427: **no actionable consumer issues remain** (only #394, upstream-blocked). Fall back to auditing the lower-priority items the fidelity audit flagged (AWS ECS fabricated CloudWatch metrics, IMDS accountId, GCS preconditions, ACR checkNameAvailability) or await the consumer's next batch.
- Open GitHub issues: #427 (closing via pending PR), #394 (upstream-blocked).
- Open BUG trackers: BUG-1075, BUG-1104, BUG-1345.
- BUG counters: 1474 filed · 1430 fixed · 5 open · 4 false positives.

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
