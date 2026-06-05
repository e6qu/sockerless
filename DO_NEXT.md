# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current State

- Branch: `feat/aws-sim-ec2-eni-ops` (PR pending, closes #428).
- Last merged: PR #429 (five cross-cloud fidelity-audit fixes, BUG-1468–1472).
- AWS EC2 standalone ENI ops (BUG-1473, issue #428): registered `CreateNetworkInterface`/`AttachNetworkInterface`/`DetachNetworkInterface`/`DeleteNetworkInterface`/`ModifyNetworkInterfaceAttribute`/`AssignPrivateIpAddresses` over the existing `ec2NetworkInterfaces` store (control-plane modeling like `CreateNatGateway`). Added `SourceDestDisabled`/`Description`/`DeviceIndex`/`DeleteOnTermination`/`SecondaryPrivateIps` to the ENI struct; extracted `eniFieldsXML` shared by Describe (`<item>`) and Create (`<networkInterface>`). Unblocks the fck-nat NAT-instance Terraform path (`aws_network_interface` + `source_dest_check=false`). SDK (full lifecycle incl. attach to a RunInstances instance + assign secondary IP), CLI, and Terraform (`network-interface/` fixture — Create + modify source_dest_check + read + delete) all pass. Note: the TF fixture deliberately omits the instance+attachment combo (the bare aws_instance read timed out); attach/detach are covered by the SDK test.
- **Next queued: #427 IAM policy simulation** — `SimulateCustomPolicy`/`SimulatePrincipalPolicy`. This is LARGER: needs a real policy-evaluation engine (parse IAM JSON; evaluate Effect/Action-wildcard/Resource-ARN-match/Condition operators like StringEquals + aws:ResourceTag). The issue frames it as a phased ask.
- Open GitHub issues: #428 (closing via pending PR), #427 (queued), #394 (upstream-blocked).
- Open BUG trackers: BUG-1075, BUG-1104, BUG-1345.
- BUG counters: 1473 filed · 1429 fixed · 5 open · 4 false positives.

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
