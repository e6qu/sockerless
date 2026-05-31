# Sim surface — aws-amplify

Surface registered in `simulators/aws/amplify.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /apps` | ✓ `simulators/aws/amplify.go:228::handleAmplifyCreateApp` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps` | ✓ `simulators/aws/amplify.go:229::handleAmplifyListApps` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}` | ✓ `simulators/aws/amplify.go:230::handleAmplifyGetApp` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}` | ✓ `simulators/aws/amplify.go:231::handleAmplifyUpdateApp` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /apps/{appId}` | ✓ `simulators/aws/amplify.go:232::handleAmplifyDeleteApp` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/branches` | ✓ `simulators/aws/amplify.go:234::handleAmplifyCreateBranch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/branches` | ✓ `simulators/aws/amplify.go:235::handleAmplifyListBranches` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/branches/{name}` | ✓ `simulators/aws/amplify.go:236::handleAmplifyGetBranch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/branches/{name}` | ✓ `simulators/aws/amplify.go:237::handleAmplifyUpdateBranch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /apps/{appId}/branches/{name}` | ✓ `simulators/aws/amplify.go:238::handleAmplifyDeleteBranch` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/webhooks` | ✓ `simulators/aws/amplify.go:240::handleAmplifyCreateWebhook` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/webhooks` | ✓ `simulators/aws/amplify.go:241::handleAmplifyListWebhooks` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /webhooks/{webhookId}` | ✓ `simulators/aws/amplify.go:242::handleAmplifyGetWebhook` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /webhooks/{webhookId}` | ✓ `simulators/aws/amplify.go:243::handleAmplifyUpdateWebhook` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /webhooks/{webhookId}` | ✓ `simulators/aws/amplify.go:244::handleAmplifyDeleteWebhook` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/branches/{name}/jobs` | ✓ `simulators/aws/amplify.go:246::handleAmplifyStartJob` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/branches/{name}/jobs` | ✓ `simulators/aws/amplify.go:247::handleAmplifyListJobs` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/branches/{name}/jobs/{jobId}` | ✓ `simulators/aws/amplify.go:248::handleAmplifyGetJob` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /apps/{appId}/branches/{name}/jobs/{jobId}/stop` | ✓ `simulators/aws/amplify.go:249::handleAmplifyStopJob` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `DELETE /apps/{appId}/branches/{name}/jobs/{jobId}` | ✓ `simulators/aws/amplify.go:250::handleAmplifyDeleteJob` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `GET /apps/{appId}/branches/{name}/jobs/{jobId}/artifacts` | ✓ `simulators/aws/amplify.go:251::handleAmplifyListArtifacts` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | ✓ | |
| `GET /artifacts/{artifactId}` | ✓ `simulators/aws/amplify.go:252::handleAmplifyGetArtifactURL` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /apps/{appId}/accesslogs` | ✓ `simulators/aws/amplify.go:253::handleAmplifyGenerateAccessLogs` | ✓ (direct; see coverage matrix) | n/a (not exposed by provider; see coverage matrix) | n/a | |
| `POST /apps/{appId}/branches/{name}/deployments` | ✓ `simulators/aws/amplify.go:257::handleAmplifyCreateDeployment` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/branches/{name}/deployments/start` | ✓ `simulators/aws/amplify.go:258::handleAmplifyStartDeployment` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /tags/{arn...}` | ✓ `simulators/aws/amplify.go:260::handleAmplifyListTags` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /tags/{arn...}` | ✓ `simulators/aws/amplify.go:261::handleAmplifyTagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /tags/{arn...}` | ✓ `simulators/aws/amplify.go:262::handleAmplifyUntagResource` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/domains` | ✓ `simulators/aws/amplify_domains.go:90::handleAmplifyCreateDomain` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/domains` | ✓ `simulators/aws/amplify_domains.go:91::handleAmplifyListDomains` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/domains/{domainName}` | ✓ `simulators/aws/amplify_domains.go:92::handleAmplifyGetDomain` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/domains/{domainName}` | ✓ `simulators/aws/amplify_domains.go:93::handleAmplifyUpdateDomain` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /apps/{appId}/domains/{domainName}` | ✓ `simulators/aws/amplify_domains.go:94::handleAmplifyDeleteDomain` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/backendenvironments` | ✓ `simulators/aws/amplify_domains.go:96::handleAmplifyCreateBackend` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/backendenvironments` | ✓ `simulators/aws/amplify_domains.go:97::handleAmplifyListBackends` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/backendenvironments/{environmentName}` | ✓ `simulators/aws/amplify_domains.go:98::handleAmplifyGetBackend` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /apps/{appId}/backendenvironments/{environmentName}` | ✓ `simulators/aws/amplify_domains.go:99::handleAmplifyDeleteBackend` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
