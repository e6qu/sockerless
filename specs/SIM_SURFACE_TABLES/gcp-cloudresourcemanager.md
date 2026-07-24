# Sim surface — gcp-cloudresourcemanager

Surface registered in `simulators/gcp/cloudresourcemanager.go` — the Cloud Resource Manager v1 projects lifecycle (the wire `gcloud projects` and terraform-provider-google's `google_project` speak), the operations.get polls of both API versions, and the Cloud Billing project billing-info read. The v3 projects/folders/tags surface registered in `simulators/gcp/iam.go` (`registerCRMv3`) stays tabled under `gcp-iam`.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v1/projects` | ✓ `simulators/gcp/cloudresourcemanager.go::registerCloudResourceManagerV1` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | settled v1 create Operation (`ProjectCreationStatus` metadata); duplicate projectId → 409 ALREADY_EXISTS |
| `GET /v1/projects` | ✓ `simulators/gcp/cloudresourcemanager.go::registerCloudResourceManagerV1` | ✓ (direct; see coverage matrix) | n/a | ✓ | v1 filter subset incl. gcloud's always-sent `lifecycleState:ACTIVE` |
| `DELETE /v1/projects/{project}` | ✓ `simulators/gcp/cloudresourcemanager.go::registerCloudResourceManagerV1` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | soft-delete → DELETE_REQUESTED; non-ACTIVE → 400 FAILED_PRECONDITION |
| `PUT /v1/projects/{project}` | ✓ `simulators/gcp/cloudresourcemanager.go::registerCloudResourceManagerV1` | ✓ (direct; see coverage matrix) | n/a | n/a | gcloud projects update read-modify-write |
| `POST /v1/projects/{projectAction}` | ✓ `simulators/gcp/cloudresourcemanager.go::registerCloudResourceManagerV1` | ✓ (direct; see coverage matrix) | n/a | n/a | `:undelete` + IAM triple; same per-project policy as the v3 verbs |
| `GET /v1/operations/{operation}` | ✓ `simulators/gcp/cloudresourcemanager.go::crmGetOperation` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | gcloud create waiter + terraform ResourceManagerOperationWaiter |
| `GET /v3/operations/{operation}` | ✓ `simulators/gcp/cloudresourcemanager.go::crmGetOperation` | ✓ (direct; see coverage matrix) | n/a | n/a | resourcemanager GAPIC LRO poller |
| `GET /v1/projects/{project}/billingInfo` | ✓ `simulators/gcp/cloudresourcemanager.go::registerCloudResourceManagerV1` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | Cloud Billing projects.getBillingInfo; read unconditionally by google_project's terraform Read |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
The project store is shared between the v1 surface here and the v3 surface in `iam.go`: one `CRMProject` row keyed by project ID, whose `name` carries the real `projects/{projectNumber}` v3 resource name; every method resolves a `{project}` path segment by ID or number. A project the sim has never seen is a real 403 PERMISSION_DENIED (the API never discloses existence), a duplicate create is a real 409 ALREADY_EXISTS, and delete/undelete follow the real ACTIVE ⇄ DELETE_REQUESTED soft-delete state machine. The organization's pre-provisioned projects ("sockerless" — the deployment default, project number 123456789012 — and "test-project", the SDK/CLI/terraform harness project) are materialized at startup the way the AWS simulator materializes its management account. Tested by `simulators/gcp/sdk-tests/resourcemanager_projects_test.go` (apiv3 GAPIC + v1 legacy client + cloudbilling), `simulators/gcp/cli-tests/projects_test.go` (`gcloud projects create/list/describe/update/delete/undelete/get-iam-policy`), and `simulators/gcp/terraform-tests/main.tf` (`google_project` with `deletion_policy = "DELETE"`, riding `resource_manager_custom_endpoint` + `cloud_billing_custom_endpoint`).
<!-- HAND-WRITTEN END -->
