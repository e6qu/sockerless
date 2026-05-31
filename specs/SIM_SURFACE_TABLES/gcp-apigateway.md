# Sim surface — gcp-apigateway

Surface registered in `simulators/gcp/apigateway.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v1/projects/{project}/locations/global/apis` | ✓ `simulators/gcp/apigateway.go:53::handleGCPAPIGWCreateApi` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/global/apis/{api}` | ✓ `simulators/gcp/apigateway.go:54::handleGCPAPIGWGetApi` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/global/apis` | ✓ `simulators/gcp/apigateway.go:55::handleGCPAPIGWListApis` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/locations/global/apis/{api}` | ✓ `simulators/gcp/apigateway.go:56::handleGCPAPIGWDeleteApi` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/global/apis/{api}/configs` | ✓ `simulators/gcp/apigateway.go:59::handleGCPAPIGWCreateConfig` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/global/apis/{api}/configs/{cfg}` | ✓ `simulators/gcp/apigateway.go:60::handleGCPAPIGWGetConfig` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/global/apis/{api}/configs` | ✓ `simulators/gcp/apigateway.go:61::handleGCPAPIGWListConfigs` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/locations/global/apis/{api}/configs/{cfg}` | ✓ `simulators/gcp/apigateway.go:62::handleGCPAPIGWDeleteConfig` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/gateways` | ✓ `simulators/gcp/apigateway.go:65::handleGCPAPIGWCreateGateway` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/gateways/{gw}` | ✓ `simulators/gcp/apigateway.go:66::handleGCPAPIGWGetGateway` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v1/projects/{project}/locations/{location}/gateways` | ✓ `simulators/gcp/apigateway.go:67::handleGCPAPIGWListGateways` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v1/projects/{project}/locations/{location}/gateways/{gw}` | ✓ `simulators/gcp/apigateway.go:68::handleGCPAPIGWDeleteGateway` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v1/projects/{project}/locations/{location}/gateways/{gwAction}` | ✓ `simulators/gcp/apigateway.go:76::handleGCPAPIGWIamAction` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
