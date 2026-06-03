# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current State

- Branch: `feat/azure-entra-groups-issue-387` (Entra groups + Graph memberOf).
- Last merged: PR #388 (Azure coverage gaps — App Insights, Private DNS A-records, ACR image ops).
- Open GitHub issues: #387 (in progress this branch).
- Open BUG trackers: BUG-1075 and BUG-1104.
- BUG counters: 1326 filed · 1326 fixed · 2 open · 3 false positives.

## Next Two PRs

### PR A — Azure simulator coverage gaps (test-only; implementations already exist)

All three gaps are missing SDK/CLI tests for live implementations. File BUG numbers for each before writing code.

| Gap | File | What to add |
|-----|------|------------|
| Application Insights | `simulators/azure/sdk-tests/monitor_test.go` (or new `insights_test.go`) | SDK tests: create component → get → assert instrumentation key + workspace link; delete; all via `armappinsights.ComponentsClient` |
| Private DNS A-record CRUD | `simulators/azure/sdk-tests/dns_private_test.go` | SDK tests: create A record → list → get → delete via `armprivatedns.RecordSetsClient` |
| ACR image operations | `simulators/azure/sdk-tests/acr_test.go` | SDK tests: push image manifest → list repositories → list tags → delete via `azcontainerregistry` data-plane client |

All three must also have CLI coverage (`az monitor app-insights component`, `az network private-dns record-set a`, `az acr repository`) and Terraform coverage if the resource is already in `simulators/azure/terraform-tests/main.tf` (App Insights and Private DNS zone already are; extend them).

Matrix rows to update: `azure-monitor` (add App Insights evidence), `azure-acr` (add image ops evidence). Check that `azure-private-dns` or `azure-dns` row exists; add if not.

### PR B — GCP simulator coverage gaps (two missing implementations + missing tests)

File BUG numbers before writing code. Larger scope — two routes are genuinely missing.

**Implementation gaps (new routes needed):**

| Gap | File | What to add |
|-----|------|------------|
| Service account keys | `simulators/gcp/iam.go` | POST/GET(list)/GET(single)/DELETE at `/v1/projects/{p}/serviceAccounts/{email}/keys`; return `ServiceAccountKey` shape with `keyId`, `privateKeyData` (base64 JSON key), `validAfterTime`, `validBeforeTime`, `keyAlgorithm` |
| Compute instance templates | `simulators/gcp/compute.go` | POST/GET/LIST/DELETE at `/compute/v1/projects/{p}/global/instanceTemplates`; store in new `InstanceTemplates` map; return minimal `compute#instanceTemplate` shape with `name`, `selfLink`, `properties` |

**Test gaps (routes exist, clients untested):**

| Gap | SDK test file | What to add |
|-----|--------------|------------|
| Cloud Functions Gen2 CRUD | `simulators/gcp/sdk-tests/functions_test.go` | `functions2.NewCloudFunctionsClient` → Create → Get → List → Delete (Terraform already exercises Gen2; SDK test suite only has invoke tests) |
| Cloud Build trigger CRUD | `simulators/gcp/sdk-tests/build_test.go` | `cloudbuild.NewCloudBuildClient` → CreateBuildTrigger → GetBuildTrigger → ListBuildTriggers → DeleteBuildTrigger |
| Cloud Logging sink+metric CRUD | `simulators/gcp/sdk-tests/logging_test.go` | `logging.NewConfigClient` → CreateSink → GetSink → ListSinks → UpdateSink → DeleteSink; same for Metrics |
| Project IAM policy | `simulators/gcp/sdk-tests/iam_test.go` | `resourcemanager.NewProjectsClient` → GetIamPolicy → SetIamPolicy (add `google_project_iam_member` to Terraform stack too) |

All test gaps also need CLI coverage (`gcloud functions`, `gcloud builds triggers`, `gcloud logging sinks`, `gcloud logging metrics`, `gcloud projects get-iam-policy`).

Matrix rows to update: `gcp-iam` (add SA keys), `gcp-compute` (add instance templates), `gcp-cloudfunctions` (add Gen2 SDK evidence), `gcp-cloudbuild` (add trigger SDK evidence), `gcp-logging` (add sink/metric SDK evidence).

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
