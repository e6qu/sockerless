# Sim surface — aws-amplify

Surface registered in `simulators/aws/amplify.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /apps` | ✓ `simulators/aws/amplify.go:206::handleAmplifyCreateApp` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /apps` | ✓ `simulators/aws/amplify.go:207::handleAmplifyListApps` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /apps/{appId}` | ✓ `simulators/aws/amplify.go:208::handleAmplifyGetApp` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /apps/{appId}` | ✓ `simulators/aws/amplify.go:209::handleAmplifyUpdateApp` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /apps/{appId}` | ✓ `simulators/aws/amplify.go:210::handleAmplifyDeleteApp` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /apps/{appId}/branches` | ✓ `simulators/aws/amplify.go:212::handleAmplifyCreateBranch` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /apps/{appId}/branches` | ✓ `simulators/aws/amplify.go:213::handleAmplifyListBranches` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /apps/{appId}/branches/{name}` | ✓ `simulators/aws/amplify.go:214::handleAmplifyGetBranch` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /apps/{appId}/branches/{name}` | ✓ `simulators/aws/amplify.go:215::handleAmplifyUpdateBranch` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /apps/{appId}/branches/{name}` | ✓ `simulators/aws/amplify.go:216::handleAmplifyDeleteBranch` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /apps/{appId}/webhooks` | ✓ `simulators/aws/amplify.go:218::handleAmplifyCreateWebhook` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /apps/{appId}/webhooks` | ✓ `simulators/aws/amplify.go:219::handleAmplifyListWebhooks` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /webhooks/{webhookId}` | ✓ `simulators/aws/amplify.go:220::handleAmplifyGetWebhook` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /webhooks/{webhookId}` | ✓ `simulators/aws/amplify.go:221::handleAmplifyUpdateWebhook` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /webhooks/{webhookId}` | ✓ `simulators/aws/amplify.go:222::handleAmplifyDeleteWebhook` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /apps/{appId}/branches/{name}/jobs` | ✓ `simulators/aws/amplify.go:224::handleAmplifyStartJob` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /apps/{appId}/branches/{name}/jobs` | ✓ `simulators/aws/amplify.go:225::handleAmplifyListJobs` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /apps/{appId}/branches/{name}/jobs/{jobId}` | ✓ `simulators/aws/amplify.go:226::handleAmplifyGetJob` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /apps/{appId}/branches/{name}/jobs/{jobId}` | ✓ `simulators/aws/amplify.go:227::handleAmplifyStopJob` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /apps/{appId}/branches/{name}/deployments` | ✓ `simulators/aws/amplify.go:231::handleAmplifyCreateDeployment` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /apps/{appId}/branches/{name}/deployments/start` | ✓ `simulators/aws/amplify.go:232::handleAmplifyStartDeployment` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /tags/{arn...}` | ✓ `simulators/aws/amplify.go:234::handleAmplifyListTags` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /tags/{arn...}` | ✓ `simulators/aws/amplify.go:235::handleAmplifyTagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /tags/{arn...}` | ✓ `simulators/aws/amplify.go:236::handleAmplifyUntagResource` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /apps/{appId}/domains` | ✓ `simulators/aws/amplify_domains.go:90::handleAmplifyCreateDomain` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /apps/{appId}/domains` | ✓ `simulators/aws/amplify_domains.go:91::handleAmplifyListDomains` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /apps/{appId}/domains/{domainName}` | ✓ `simulators/aws/amplify_domains.go:92::handleAmplifyGetDomain` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /apps/{appId}/domains/{domainName}` | ✓ `simulators/aws/amplify_domains.go:93::handleAmplifyUpdateDomain` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /apps/{appId}/domains/{domainName}` | ✓ `simulators/aws/amplify_domains.go:94::handleAmplifyDeleteDomain` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /apps/{appId}/backendenvironments` | ✓ `simulators/aws/amplify_domains.go:96::handleAmplifyCreateBackend` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /apps/{appId}/backendenvironments` | ✓ `simulators/aws/amplify_domains.go:97::handleAmplifyListBackends` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /apps/{appId}/backendenvironments/{environmentName}` | ✓ `simulators/aws/amplify_domains.go:98::handleAmplifyGetBackend` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /apps/{appId}/backendenvironments/{environmentName}` | ✓ `simulators/aws/amplify_domains.go:99::handleAmplifyDeleteBackend` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
