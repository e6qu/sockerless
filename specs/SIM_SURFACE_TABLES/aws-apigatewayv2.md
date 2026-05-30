# Sim surface — aws-apigatewayv2

Surface registered in `simulators/aws/apigatewayv2.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v2/apis` | ✓ `simulators/aws/apigatewayv2.go:82::handleAPIGWv2CreateApi` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v2/apis` | ✓ `simulators/aws/apigatewayv2.go:83::handleAPIGWv2ListApis` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}` | ✓ `simulators/aws/apigatewayv2.go:84::handleAPIGWv2GetApi` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}` | ✓ `simulators/aws/apigatewayv2.go:85::handleAPIGWv2DeleteApi` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v2/apis/{apiId}/routes` | ✓ `simulators/aws/apigatewayv2.go:86::handleAPIGWv2CreateRoute` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/routes` | ✓ `simulators/aws/apigatewayv2.go:87::handleAPIGWv2ListRoutes` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v2/apis/{apiId}/integrations` | ✓ `simulators/aws/apigatewayv2.go:88::handleAPIGWv2CreateIntegration` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/integrations` | ✓ `simulators/aws/apigatewayv2.go:89::handleAPIGWv2ListIntegrations` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v2/apis/{apiId}/stages` | ✓ `simulators/aws/apigatewayv2.go:90::handleAPIGWv2CreateStage` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/stages` | ✓ `simulators/aws/apigatewayv2.go:91::handleAPIGWv2ListStages` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /v2/apis/{apiId}/deployments` | ✓ `simulators/aws/apigatewayv2.go:92::handleAPIGWv2CreateDeployment` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/deployments` | ✓ `simulators/aws/apigatewayv2.go:93::handleAPIGWv2ListDeployments` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/deployments/{deploymentId}` | ✓ `simulators/aws/apigatewayv2.go:94::handleAPIGWv2GetDeployment` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}/deployments/{deploymentId}` | ✓ `simulators/aws/apigatewayv2.go:95::handleAPIGWv2DeleteDeployment` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
