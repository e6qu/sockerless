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
| `POST /v2/apis` | ✓ `simulators/aws/apigatewayv2.go:182::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis` | ✓ `simulators/aws/apigatewayv2.go:183::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}` | ✓ `simulators/aws/apigatewayv2.go:184::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}` | ✓ `simulators/aws/apigatewayv2.go:185::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/apis/{apiId}/routes` | ✓ `simulators/aws/apigatewayv2.go:186::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/routes` | ✓ `simulators/aws/apigatewayv2.go:187::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/routes/{routeId}` | ✓ `simulators/aws/apigatewayv2.go:188::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/apis/{apiId}/routes/{routeId}` | ✓ `simulators/aws/apigatewayv2.go:189::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}/routes/{routeId}` | ✓ `simulators/aws/apigatewayv2.go:190::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/apis/{apiId}/integrations` | ✓ `simulators/aws/apigatewayv2.go:191::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/integrations` | ✓ `simulators/aws/apigatewayv2.go:192::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/integrations/{integrationId}` | ✓ `simulators/aws/apigatewayv2.go:193::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/apis/{apiId}/integrations/{integrationId}` | ✓ `simulators/aws/apigatewayv2.go:194::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}/integrations/{integrationId}` | ✓ `simulators/aws/apigatewayv2.go:195::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/apis/{apiId}/stages` | ✓ `simulators/aws/apigatewayv2.go:196::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/stages` | ✓ `simulators/aws/apigatewayv2.go:197::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/stages/{stageName}` | ✓ `simulators/aws/apigatewayv2.go:198::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/apis/{apiId}/stages/{stageName}` | ✓ `simulators/aws/apigatewayv2.go:199::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}/stages/{stageName}` | ✓ `simulators/aws/apigatewayv2.go:200::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/apis/{apiId}/deployments` | ✓ `simulators/aws/apigatewayv2.go:201::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/deployments` | ✓ `simulators/aws/apigatewayv2.go:202::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/deployments/{deploymentId}` | ✓ `simulators/aws/apigatewayv2.go:203::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}/deployments/{deploymentId}` | ✓ `simulators/aws/apigatewayv2.go:204::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/apis/{apiId}/authorizers` | ✓ `simulators/aws/apigatewayv2.go:207::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/authorizers` | ✓ `simulators/aws/apigatewayv2.go:208::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/authorizers/{authorizerId}` | ✓ `simulators/aws/apigatewayv2.go:209::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/apis/{apiId}/authorizers/{authorizerId}` | ✓ `simulators/aws/apigatewayv2.go:210::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}/authorizers/{authorizerId}` | ✓ `simulators/aws/apigatewayv2.go:211::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/apis/{apiId}/models` | ✓ `simulators/aws/apigatewayv2.go:214::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/models` | ✓ `simulators/aws/apigatewayv2.go:215::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/models/{modelId}` | ✓ `simulators/aws/apigatewayv2.go:216::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}/models/{modelId}` | ✓ `simulators/aws/apigatewayv2.go:217::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/domainnames` | ✓ `simulators/aws/apigatewayv2.go:221::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/domainnames` | ✓ `simulators/aws/apigatewayv2.go:222::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/domainnames/{domainName}` | ✓ `simulators/aws/apigatewayv2.go:223::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/domainnames/{domainName}` | ✓ `simulators/aws/apigatewayv2.go:224::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/domainnames/{domainName}/apimappings` | ✓ `simulators/aws/apigatewayv2.go:225::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/domainnames/{domainName}/apimappings` | ✓ `simulators/aws/apigatewayv2.go:226::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/domainnames/{domainName}/apimappings/{apiMappingId}` | ✓ `simulators/aws/apigatewayv2.go:227::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/domainnames/{domainName}/apimappings/{apiMappingId}` | ✓ `simulators/aws/apigatewayv2.go:228::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/vpclinks` | ✓ `simulators/aws/apigatewayv2.go:232::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/vpclinks` | ✓ `simulators/aws/apigatewayv2.go:233::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/vpclinks/{vpcLinkId}` | ✓ `simulators/aws/apigatewayv2.go:234::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/vpclinks/{vpcLinkId}` | ✓ `simulators/aws/apigatewayv2.go:235::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/portals` | ✓ `simulators/aws/apigatewayv2_complete.go:123::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/portals` | ✓ `simulators/aws/apigatewayv2_complete.go:124::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/portals/{portalId}` | ✓ `simulators/aws/apigatewayv2_complete.go:125::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/portals/{portalId}` | ✓ `simulators/aws/apigatewayv2_complete.go:126::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/portals/{portalId}` | ✓ `simulators/aws/apigatewayv2_complete.go:127::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/portals/{portalId}/preview` | ✓ `simulators/aws/apigatewayv2_complete.go:128::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/portals/{portalId}/publish` | ✓ `simulators/aws/apigatewayv2_complete.go:129::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/portals/{portalId}/publish` | ✓ `simulators/aws/apigatewayv2_complete.go:130::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/portalproducts` | ✓ `simulators/aws/apigatewayv2_complete.go:133::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/portalproducts` | ✓ `simulators/aws/apigatewayv2_complete.go:134::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/portalproducts/{portalProductId}` | ✓ `simulators/aws/apigatewayv2_complete.go:135::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/portalproducts/{portalProductId}` | ✓ `simulators/aws/apigatewayv2_complete.go:136::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/portalproducts/{portalProductId}` | ✓ `simulators/aws/apigatewayv2_complete.go:137::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/portalproducts/{portalProductId}/sharingpolicy` | ✓ `simulators/aws/apigatewayv2_complete.go:140::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v2/portalproducts/{portalProductId}/sharingpolicy` | ✓ `simulators/aws/apigatewayv2_complete.go:141::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/portalproducts/{portalProductId}/sharingpolicy` | ✓ `simulators/aws/apigatewayv2_complete.go:142::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/portalproducts/{portalProductId}/productpages` | ✓ `simulators/aws/apigatewayv2_complete.go:145::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/portalproducts/{portalProductId}/productpages` | ✓ `simulators/aws/apigatewayv2_complete.go:146::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/portalproducts/{portalProductId}/productpages/{productPageId}` | ✓ `simulators/aws/apigatewayv2_complete.go:147::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/portalproducts/{portalProductId}/productpages/{productPageId}` | ✓ `simulators/aws/apigatewayv2_complete.go:148::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/portalproducts/{portalProductId}/productpages/{productPageId}` | ✓ `simulators/aws/apigatewayv2_complete.go:149::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/portalproducts/{portalProductId}/productrestendpointpages` | ✓ `simulators/aws/apigatewayv2_complete.go:152::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/portalproducts/{portalProductId}/productrestendpointpages` | ✓ `simulators/aws/apigatewayv2_complete.go:153::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/portalproducts/{portalProductId}/productrestendpointpages/{productRestEndpointPageId}` | ✓ `simulators/aws/apigatewayv2_complete.go:154::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/portalproducts/{portalProductId}/productrestendpointpages/{productRestEndpointPageId}` | ✓ `simulators/aws/apigatewayv2_complete.go:155::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/portalproducts/{portalProductId}/productrestendpointpages/{productRestEndpointPageId}` | ✓ `simulators/aws/apigatewayv2_complete.go:156::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/domainnames/{domainName}/routingrules` | ✓ `simulators/aws/apigatewayv2_complete.go:159::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/domainnames/{domainName}/routingrules` | ✓ `simulators/aws/apigatewayv2_complete.go:160::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/domainnames/{domainName}/routingrules/{routingRuleId}` | ✓ `simulators/aws/apigatewayv2_complete.go:161::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v2/domainnames/{domainName}/routingrules/{routingRuleId}` | ✓ `simulators/aws/apigatewayv2_complete.go:162::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/domainnames/{domainName}/routingrules/{routingRuleId}` | ✓ `simulators/aws/apigatewayv2_complete.go:163::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/apis/{apiId}` | ✓ `simulators/aws/apigatewayv2_complete.go:166::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/domainnames/{domainName}/apimappings/{apiMappingId}` | ✓ `simulators/aws/apigatewayv2_complete.go:167::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}/stages/{stageName}/cache/authorizers` | ✓ `simulators/aws/apigatewayv2_complete.go:170::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/apis/{apiId}/integrations/{integrationId}/integrationresponses` | ✓ `simulators/aws/apigatewayv2_extras.go:128::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/integrations/{integrationId}/integrationresponses` | ✓ `simulators/aws/apigatewayv2_extras.go:129::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/integrations/{integrationId}/integrationresponses/{integrationResponseId}` | ✓ `simulators/aws/apigatewayv2_extras.go:130::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/apis/{apiId}/integrations/{integrationId}/integrationresponses/{integrationResponseId}` | ✓ `simulators/aws/apigatewayv2_extras.go:131::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}/integrations/{integrationId}/integrationresponses/{integrationResponseId}` | ✓ `simulators/aws/apigatewayv2_extras.go:132::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /v2/apis/{apiId}/routes/{routeId}/routeresponses` | ✓ `simulators/aws/apigatewayv2_extras.go:135::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/routes/{routeId}/routeresponses` | ✓ `simulators/aws/apigatewayv2_extras.go:136::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/routes/{routeId}/routeresponses/{routeResponseId}` | ✓ `simulators/aws/apigatewayv2_extras.go:137::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /v2/apis/{apiId}/routes/{routeId}/routeresponses/{routeResponseId}` | ✓ `simulators/aws/apigatewayv2_extras.go:138::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}/routes/{routeId}/routeresponses/{routeResponseId}` | ✓ `simulators/aws/apigatewayv2_extras.go:139::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/models/{modelId}/template` | ✓ `simulators/aws/apigatewayv2_extras.go:142::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v2/apis` | ✓ `simulators/aws/apigatewayv2_extras.go:145::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /v2/apis/{apiId}` | ✓ `simulators/aws/apigatewayv2_extras.go:146::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/apis/{apiId}/exports/{specification}` | ✓ `simulators/aws/apigatewayv2_extras.go:147::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /v2/tags/{resourceArn}` | ✓ `simulators/aws/apigatewayv2_extras.go:150::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}/stages/{stageName}/accesslogsettings` | ✓ `simulators/aws/apigatewayv2_extras.go:153::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}/cors` | ✓ `simulators/aws/apigatewayv2_extras.go:154::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}/routes/{routeId}/requestparameters/{requestParameterKey}` | ✓ `simulators/aws/apigatewayv2_extras.go:155::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /v2/apis/{apiId}/stages/{stageName}/routesettings/{routeKey}` | ✓ `simulators/aws/apigatewayv2_extras.go:156::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
