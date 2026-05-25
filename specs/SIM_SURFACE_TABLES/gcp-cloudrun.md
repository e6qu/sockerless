# Sim surface — gcp-cloudrun

Surface registered in `simulators/gcp/cloudrun.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v1/namespaces/{namespace}/services` | ✓ `simulators/gcp/cloudrun.go:130::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/namespaces/{namespace}/services/{name}` | ✓ `simulators/gcp/cloudrun.go:182::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/namespaces/{namespace}/services` | ✓ `simulators/gcp/cloudrun.go:195::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /v1/namespaces/{namespace}/services/{name}` | ✓ `simulators/gcp/cloudrun.go:213::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v1/namespaces/{namespace}/services/{name}` | ✓ `simulators/gcp/cloudrun.go:262::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v2/projects/{project}/locations/{location}/jobs` | ✓ `simulators/gcp/cloudrunjobs.go:295::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/projects/{project}/locations/{location}/jobs/{job}` | ✓ `simulators/gcp/cloudrunjobs.go:360::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/projects/{project}/locations/{location}/jobs` | ✓ `simulators/gcp/cloudrunjobs.go:380::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v2/projects/{project}/locations/{location}/jobs/{job}` | ✓ `simulators/gcp/cloudrunjobs.go:395::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v2/projects/{project}/locations/{location}/jobs/{jobAction}` | ✓ `simulators/gcp/cloudrunjobs.go:422::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/projects/{project}/locations/{location}/jobs/{job}/executions/{execution}` | ✓ `simulators/gcp/cloudrunjobs.go:571::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/projects/{project}/locations/{location}/jobs/{job}/executions` | ✓ `simulators/gcp/cloudrunjobs.go:587::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v2/projects/{project}/locations/{location}/jobs/{job}/executions/{execAction}` | ✓ `simulators/gcp/cloudrunjobs.go:603::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v2/projects/{project}/locations/{location}/services` | ✓ `simulators/gcp/cloudrunservices.go:454::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/projects/{project}/locations/{location}/services/{service}` | ✓ `simulators/gcp/cloudrunservices.go:494::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/projects/{project}/locations/{location}/services` | ✓ `simulators/gcp/cloudrunservices.go:508::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v2/projects/{project}/locations/{location}/services/{service}` | ✓ `simulators/gcp/cloudrunservices.go:519::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /v2/projects/{project}/locations/{location}/services/{service}` | ✓ `simulators/gcp/cloudrunservices.go:541::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v2-services-invoke/{project}/{location}/{service}` | ✓ `simulators/gcp/cloudrunservices.go:600::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
