# Sim surface — aws-apigateway

Surface registered in `simulators/aws/apigateway.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /restapis` | ✓ `simulators/aws/apigateway.go:104::handleAPIGWCreateRestApi` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /restapis` | ✓ `simulators/aws/apigateway.go:105::handleAPIGWListRestApis` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /restapis/{restApiId}` | ✓ `simulators/aws/apigateway.go:106::handleAPIGWGetRestApi` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /restapis/{restApiId}` | ✓ `simulators/aws/apigateway.go:107::handleAPIGWDeleteRestApi` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /restapis/{restApiId}/resources/{parentId}` | ✓ `simulators/aws/apigateway.go:108::handleAPIGWCreateResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /restapis/{restApiId}/resources` | ✓ `simulators/aws/apigateway.go:109::handleAPIGWListResources` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}` | ✓ `simulators/aws/apigateway.go:110::handleAPIGWPutMethod` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration` | ✓ `simulators/aws/apigateway.go:111::handleAPIGWPutIntegration` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /restapis/{restApiId}/deployments` | ✓ `simulators/aws/apigateway.go:112::handleAPIGWCreateDeployment` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /restapis/{restApiId}/stages` | ✓ `simulators/aws/apigateway.go:113::handleAPIGWCreateStage` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:120::handleAPIGWPutMethodResponse` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:121::handleAPIGWGetMethodResponse` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:122::handleAPIGWDeleteMethodResponse` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PUT /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:123::handleAPIGWPutIntegrationResponse` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:124::handleAPIGWGetIntegrationResponse` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /restapis/{restApiId}/resources/{resourceId}/methods/{httpMethod}/integration/responses/{statusCode}` | ✓ `simulators/aws/apigateway.go:125::handleAPIGWDeleteIntegrationResponse` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
