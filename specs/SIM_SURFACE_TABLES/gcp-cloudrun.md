# Sim surface — gcp-cloudrun

Surface registered in `simulators/gcp/cloudrun.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v1/namespaces/{namespace}/services` | ✓ `simulators/gcp/cloudrun.go:130::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/namespaces/{namespace}/services/{name}` | ✓ `simulators/gcp/cloudrun.go:182::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/namespaces/{namespace}/services` | ✓ `simulators/gcp/cloudrun.go:195::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v1/namespaces/{namespace}/services/{name}` | ✓ `simulators/gcp/cloudrun.go:213::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/namespaces/{namespace}/services/{name}` | ✓ `simulators/gcp/cloudrun.go:262::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/locations/{location}/jobs` | ✓ `simulators/gcp/cloudrunjobs.go:295::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/locations/{location}/jobs/{job}` | ✓ `simulators/gcp/cloudrunjobs.go:360::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/locations/{location}/jobs` | ✓ `simulators/gcp/cloudrunjobs.go:380::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/locations/{location}/jobs/{job}` | ✓ `simulators/gcp/cloudrunjobs.go:395::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/locations/{location}/jobs/{jobAction}` | ✓ `simulators/gcp/cloudrunjobs.go:422::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/locations/{location}/jobs/{job}/executions/{execution}` | ✓ `simulators/gcp/cloudrunjobs.go:571::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/locations/{location}/jobs/{job}/executions` | ✓ `simulators/gcp/cloudrunjobs.go:587::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/locations/{location}/jobs/{job}/executions/{execAction}` | ✓ `simulators/gcp/cloudrunjobs.go:603::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/projects/{project}/locations/{location}/services` | ✓ `simulators/gcp/cloudrunservices.go:454::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/locations/{location}/services/{service}` | ✓ `simulators/gcp/cloudrunservices.go:494::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/projects/{project}/locations/{location}/services` | ✓ `simulators/gcp/cloudrunservices.go:508::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/projects/{project}/locations/{location}/services/{service}` | ✓ `simulators/gcp/cloudrunservices.go:519::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/projects/{project}/locations/{location}/services/{service}` | ✓ `simulators/gcp/cloudrunservices.go:541::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2-services-invoke/{project}/{location}/{service}` | ✓ `simulators/gcp/cloudrunservices.go:600::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
