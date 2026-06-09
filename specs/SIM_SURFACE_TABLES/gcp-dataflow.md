# Sim surface — gcp-dataflow

Surface registered in `simulators/gcp/dataflow.go`.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v1b3/projects/{project}/locations/{location}/jobs` | ✓ `handleDataflowCreateJob` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider for this REST job slice; see coverage matrix) | n/a | |
| `GET /v1b3/projects/{project}/locations/{location}/jobs` | ✓ `handleDataflowListJobs` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider for this REST job slice; see coverage matrix) | ✓ | |
| `GET /v1b3/projects/{project}/locations/{location}/jobs/{job}` | ✓ `handleDataflowGetJob` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider for this REST job slice; see coverage matrix) | n/a | |
| `PUT /v1b3/projects/{project}/locations/{location}/jobs/{job}` | ✓ `handleDataflowUpdateJob` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider for this REST job slice; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
