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
Beyond the registered control-plane ops, Amplify execution is real:

- **Builds** (`amplify_build.go`): StartJob with a build-shaped job type (RELEASE/RETRY/WEB_HOOK) runs a real build when the app has a clonable HTTP(S) git repository AND a buildSpec (branch-level wins). The sim host clones the repo (go-git, branch ref), executes the buildSpec's frontend preBuild/build commands in a node container (`SIM_AMPLIFY_BUILD_IMAGE`, default `public.ecr.aws/docker/library/node:20-alpine`), zips the artifacts `baseDirectory` as the job artifact, and lands SUCCEED/FAILED from the container exit. Per-step logs serve from sim S3 via each step's `logUrl`. Without a clonable repo + buildSpec, jobs keep the synthetic PENDING→RUNNING→SUCCEED flip; MANUAL deployments never build.
- **Hosting data plane** (`amplify_dataplane.go`, host-addressed WrapHandler — not a mux route): serves each branch's active deployment (latest SUCCEED job's artifact zip / fileMap) on `{branch}.{appId}.amplifyapp.com`, the deterministic per-app `{hash}.cloudfront.net` host the subdomain dnsRecords advertise (no CloudFront control-plane object — real Amplify's distribution is internal), and verified custom domains. Custom rules (200/301/302/404/404-200, `<*>` wildcards) and basicAuthCredentials (base64 `user:pass`) are enforced; no deployment ⇒ 404.
- **SSR / WEB_COMPUTE** (`amplify_compute.go`): bundles whose root carries `deploy-manifest.json` (Amplify Hosting deployment spec, version 1) route Static targets from the bundle's `static/` directory and proxy Compute targets to a long-lived node container per branch active-deployment (entrypoint under `compute/{name}/`, PORT=3000, lazily started on first request, replaced on new deploys, stopped on branch/app delete). ImageOptimization targets are 501 (the sim has no image-optimization service).
- **Domain verification** (`amplify_domains.go`): AMPLIFY_MANAGED associations start PENDING_VERIFICATION and flip AVAILABLE (subdomains Verified) only when the advertised certificate-verification CNAME exists in a sim Route 53 hosted zone covering the domain — evaluated at read time, so terraform's `wait_for_verification` polling converges and a domain with no hosted zone stays PENDING_VERIFICATION. CUSTOM-certificate associations settle immediately (no DNS challenge to wait on).

Tests: unit (buildSpec/manifest/rule/host-matcher/verification tables in `simulators/aws/amplify_*_test.go`), SDK e2e (`sdk-tests/amplify_hosting_test.go` — hosting, SSR, real build against a git-http-backend repo, Route 53 verification flow), CLI hosting smoke (`cli-tests/amplify_test.go`), terraform zone+verification-record fixture (`terraform-tests/main.tf`). Build/SSR/hosting e2e need Docker (they ride the always-Docker sdk-tests suite); the unit suite stays Docker-free.
<!-- HAND-WRITTEN END -->
