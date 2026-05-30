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
| `POST /restapis` | ✓ `simulators/aws/apigateway.go:117::handleAPIGWCreateRestApi` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /restapis` | ✓ `simulators/aws/apigateway.go:118::handleAPIGWListRestApis` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /restapis/{restApiId}` | ✓ `simulators/aws/apigateway.go:119::handleAPIGWGetRestApi` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /restapis/{restApiId}` | ✓ `simulators/aws/apigateway.go:120::handleAPIGWDeleteRestApi` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /restapis/{restApiId}/resources/{parentId}` | ✓ `simulators/aws/apigateway.go:121::handleAPIGWCreateResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /restapis/{restApiId}/resources` | ✓ `simulators/aws/apigateway.go:122::handleAPIGWListResources` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /restapis/{restApiId}/resources/{resourceId}` | ✓ `simulators/aws/apigateway.go:123::handleAPIGWGetResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /restapis/{restApiId}/resources/{resourceId}` | ✓ `simulators/aws/apigateway.go:124::handleAPIGWDeleteResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}` | ✓ `simulators/aws/apigateway.go:125::handleAPIGWPutMethod` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}` | ✓ `simulators/aws/apigateway.go:126::handleAPIGWGetMethod` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}` | ✓ `simulators/aws/apigateway.go:127::handleAPIGWDeleteMethod` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration` | ✓ `simulators/aws/apigateway.go:128::handleAPIGWPutIntegration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration` | ✓ `simulators/aws/apigateway.go:129::handleAPIGWGetIntegration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration` | ✓ `simulators/aws/apigateway.go:130::handleAPIGWDeleteIntegration` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /restapis/{restApiId}/deployments` | ✓ `simulators/aws/apigateway.go:131::handleAPIGWCreateDeployment` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /restapis/{restApiId}/deployments/{deploymentId}` | ✓ `simulators/aws/apigateway.go:132::handleAPIGWGetDeployment` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /restapis/{restApiId}/deployments/{deploymentId}` | ✓ `simulators/aws/apigateway.go:133::handleAPIGWDeleteDeployment` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /restapis/{restApiId}/stages` | ✓ `simulators/aws/apigateway.go:134::handleAPIGWCreateStage` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /restapis/{restApiId}/stages/{stageName}` | ✓ `simulators/aws/apigateway.go:135::handleAPIGWGetStage` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /restapis/{restApiId}/stages/{stageName}` | ✓ `simulators/aws/apigateway.go:136::handleAPIGWDeleteStage` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:143::handleAPIGWPutMethodResponse` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:144::handleAPIGWGetMethodResponse` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:145::handleAPIGWDeleteMethodResponse` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:146::handleAPIGWPutIntegrationResponse` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:147::handleAPIGWGetIntegrationResponse` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:148::handleAPIGWDeleteIntegrationResponse` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
