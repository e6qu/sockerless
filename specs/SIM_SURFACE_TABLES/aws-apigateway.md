# Sim surface — aws-apigateway

Surface registered in `simulators/aws/apigateway.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /restapis` | ✓ `simulators/aws/apigateway.go:104::handleAPIGWCreateRestApi` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /restapis` | ✓ `simulators/aws/apigateway.go:105::handleAPIGWListRestApis` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /restapis/{restApiId}` | ✓ `simulators/aws/apigateway.go:106::handleAPIGWGetRestApi` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /restapis/{restApiId}` | ✓ `simulators/aws/apigateway.go:107::handleAPIGWDeleteRestApi` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /restapis/{restApiId}/resources/{parentId}` | ✓ `simulators/aws/apigateway.go:108::handleAPIGWCreateResource` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /restapis/{restApiId}/resources` | ✓ `simulators/aws/apigateway.go:109::handleAPIGWListResources` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}` | ✓ `simulators/aws/apigateway.go:110::handleAPIGWPutMethod` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration` | ✓ `simulators/aws/apigateway.go:111::handleAPIGWPutIntegration` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /restapis/{restApiId}/deployments` | ✓ `simulators/aws/apigateway.go:112::handleAPIGWCreateDeployment` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /restapis/{restApiId}/stages` | ✓ `simulators/aws/apigateway.go:113::handleAPIGWCreateStage` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:120::handleAPIGWPutMethodResponse` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:121::handleAPIGWGetMethodResponse` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:122::handleAPIGWDeleteMethodResponse` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:123::handleAPIGWPutIntegrationResponse` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:124::handleAPIGWGetIntegrationResponse` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:125::handleAPIGWDeleteIntegrationResponse` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
