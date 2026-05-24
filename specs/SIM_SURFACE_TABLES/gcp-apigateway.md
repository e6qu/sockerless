# Sim surface — gcp-apigateway

Surface registered in `simulators/gcp/apigateway.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v1/projects/{project}/locations/global/apis` | ✓ `simulators/gcp/apigateway.go:52::handleGCPAPIGWCreateApi` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/locations/global/apis/{api}` | ✓ `simulators/gcp/apigateway.go:53::handleGCPAPIGWGetApi` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/locations/global/apis` | ✓ `simulators/gcp/apigateway.go:54::handleGCPAPIGWListApis` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v1/projects/{project}/locations/global/apis/{api}` | ✓ `simulators/gcp/apigateway.go:55::handleGCPAPIGWDeleteApi` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v1/projects/{project}/locations/global/apis/{api}/configs` | ✓ `simulators/gcp/apigateway.go:58::handleGCPAPIGWCreateConfig` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/locations/global/apis/{api}/configs/{cfg}` | ✓ `simulators/gcp/apigateway.go:59::handleGCPAPIGWGetConfig` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/locations/global/apis/{api}/configs` | ✓ `simulators/gcp/apigateway.go:60::handleGCPAPIGWListConfigs` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v1/projects/{project}/locations/global/apis/{api}/configs/{cfg}` | ✓ `simulators/gcp/apigateway.go:61::handleGCPAPIGWDeleteConfig` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/gateways` | ✓ `simulators/gcp/apigateway.go:64::handleGCPAPIGWCreateGateway` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/gateways/{gw}` | ✓ `simulators/gcp/apigateway.go:65::handleGCPAPIGWGetGateway` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/gateways` | ✓ `simulators/gcp/apigateway.go:66::handleGCPAPIGWListGateways` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/gateways/{gw}` | ✓ `simulators/gcp/apigateway.go:67::handleGCPAPIGWDeleteGateway` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
