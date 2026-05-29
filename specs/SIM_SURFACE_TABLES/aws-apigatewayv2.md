# Sim surface — aws-apigatewayv2

Surface registered in `simulators/aws/apigatewayv2.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /v2/apis` | ✓ `simulators/aws/apigatewayv2.go:82::handleAPIGWv2CreateApi` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/apis` | ✓ `simulators/aws/apigatewayv2.go:83::handleAPIGWv2ListApis` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/apis/{apiId}` | ✓ `simulators/aws/apigatewayv2.go:84::handleAPIGWv2GetApi` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v2/apis/{apiId}` | ✓ `simulators/aws/apigatewayv2.go:85::handleAPIGWv2DeleteApi` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v2/apis/{apiId}/routes` | ✓ `simulators/aws/apigatewayv2.go:86::handleAPIGWv2CreateRoute` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/apis/{apiId}/routes` | ✓ `simulators/aws/apigatewayv2.go:87::handleAPIGWv2ListRoutes` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v2/apis/{apiId}/integrations` | ✓ `simulators/aws/apigatewayv2.go:88::handleAPIGWv2CreateIntegration` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/apis/{apiId}/integrations` | ✓ `simulators/aws/apigatewayv2.go:89::handleAPIGWv2ListIntegrations` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v2/apis/{apiId}/stages` | ✓ `simulators/aws/apigatewayv2.go:90::handleAPIGWv2CreateStage` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/apis/{apiId}/stages` | ✓ `simulators/aws/apigatewayv2.go:91::handleAPIGWv2ListStages` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /v2/apis/{apiId}/deployments` | ✓ `simulators/aws/apigatewayv2.go:92::handleAPIGWv2CreateDeployment` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/apis/{apiId}/deployments` | ✓ `simulators/aws/apigatewayv2.go:93::handleAPIGWv2ListDeployments` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /v2/apis/{apiId}/deployments/{deploymentId}` | ✓ `simulators/aws/apigatewayv2.go:94::handleAPIGWv2GetDeployment` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /v2/apis/{apiId}/deployments/{deploymentId}` | ✓ `simulators/aws/apigatewayv2.go:95::handleAPIGWv2DeleteDeployment` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
