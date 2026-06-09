# Sim surface — gcp-spanner

Surface registered in `simulators/gcp/spanner.go`.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /spanner/v1/projects/{project}/instances` | ✓ `handleSpannerCreateInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | Service endpoint base maps to official `/v1` Spanner path. |
| `GET /spanner/v1/projects/{project}/instances` | ✓ `handleSpannerListInstances` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | ✓ | |
| `GET /spanner/v1/projects/{project}/instances/{instance}` | ✓ `handleSpannerGetInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /spanner/v1/projects/{project}/instances/{instance}` | ✓ `handleSpannerDeleteInstance` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /spanner/v1/projects/{project}/instances/{instance}/operations/{operation}` | ✓ `handleSpannerGetOperation` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /spanner/v1/projects/{project}/instances/{instance}/databases` | ✓ `handleSpannerCreateDatabase` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /spanner/v1/projects/{project}/instances/{instance}/databases` | ✓ `handleSpannerListDatabases` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | ✓ | |
| `GET /spanner/v1/projects/{project}/instances/{instance}/databases/{database}` | ✓ `handleSpannerGetDatabase` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /spanner/v1/projects/{project}/instances/{instance}/databases/{database}` | ✓ `handleSpannerDeleteDatabase` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /spanner/v1/projects/{project}/instances/{instance}/databases/{database}/operations/{operation}` | ✓ `handleSpannerGetOperation` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /spanner/v1/projects/{project}/instances/{instance}/databases/{database}/sessions` | ✓ `handleSpannerCreateSession` | ✓ (direct; see coverage matrix) | n/a | n/a | |
| `GET /spanner/v1/projects/{project}/instances/{instance}/databases/{database}/sessions` | ✓ `handleSpannerListSessions` | ✓ (direct; see coverage matrix) | n/a | ✓ | |
| `GET /spanner/v1/projects/{project}/instances/{instance}/databases/{database}/sessions/{session}` | ✓ `handleSpannerGetSession` | ✓ (direct; see coverage matrix) | n/a | n/a | |
| `DELETE /spanner/v1/projects/{project}/instances/{instance}/databases/{database}/sessions/{session}` | ✓ `handleSpannerDeleteSession` | ✓ (direct; see coverage matrix) | n/a | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
