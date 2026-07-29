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
| `POST /apps` | ✓ `simulators/aws/amplify.go:394::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps` | ✓ `simulators/aws/amplify.go:395::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}` | ✓ `simulators/aws/amplify.go:396::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}` | ✓ `simulators/aws/amplify.go:397::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /apps/{appId}` | ✓ `simulators/aws/amplify.go:398::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/branches` | ✓ `simulators/aws/amplify.go:400::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/branches` | ✓ `simulators/aws/amplify.go:401::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/branches/{name}` | ✓ `simulators/aws/amplify.go:402::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/branches/{name}` | ✓ `simulators/aws/amplify.go:403::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /apps/{appId}/branches/{name}` | ✓ `simulators/aws/amplify.go:404::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/webhooks` | ✓ `simulators/aws/amplify.go:406::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/webhooks` | ✓ `simulators/aws/amplify.go:407::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /webhooks/{webhookId}` | ✓ `simulators/aws/amplify.go:408::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /webhooks/{webhookId}` | ✓ `simulators/aws/amplify.go:409::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /webhooks/{webhookId}` | ✓ `simulators/aws/amplify.go:410::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/branches/{name}/jobs` | ✓ `simulators/aws/amplify.go:412::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/branches/{name}/jobs` | ✓ `simulators/aws/amplify.go:413::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/branches/{name}/jobs/{jobId}` | ✓ `simulators/aws/amplify.go:414::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /apps/{appId}/branches/{name}/jobs/{jobId}/stop` | ✓ `simulators/aws/amplify.go:415::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /apps/{appId}/branches/{name}/jobs/{jobId}` | ✓ `simulators/aws/amplify.go:416::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/branches/{name}/jobs/{jobId}/artifacts` | ✓ `simulators/aws/amplify.go:417::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /artifacts/{artifactId}` | ✓ `simulators/aws/amplify.go:418::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/accesslogs` | ✓ `simulators/aws/amplify.go:419::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/branches/{name}/deployments` | ✓ `simulators/aws/amplify.go:423::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/branches/{name}/deployments/start` | ✓ `simulators/aws/amplify.go:424::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /tags/{arn...}` | ✓ `simulators/aws/amplify.go:426::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /tags/{arn...}` | ✓ `simulators/aws/amplify.go:427::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /tags/{arn...}` | ✓ `simulators/aws/amplify.go:428::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/domains` | ✓ `simulators/aws/amplify_domains.go:125::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/domains` | ✓ `simulators/aws/amplify_domains.go:126::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/domains/{domainName}` | ✓ `simulators/aws/amplify_domains.go:127::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/domains/{domainName}` | ✓ `simulators/aws/amplify_domains.go:128::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /apps/{appId}/domains/{domainName}` | ✓ `simulators/aws/amplify_domains.go:129::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /apps/{appId}/backendenvironments` | ✓ `simulators/aws/amplify_domains.go:131::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/backendenvironments` | ✓ `simulators/aws/amplify_domains.go:132::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /apps/{appId}/backendenvironments/{environmentName}` | ✓ `simulators/aws/amplify_domains.go:133::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /apps/{appId}/backendenvironments/{environmentName}` | ✓ `simulators/aws/amplify_domains.go:134::cloudTrailRecordedREST` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
Beyond the registered control-plane ops, Amplify execution is real:

- **Builds** (`amplify_build.go`): StartJob with a build-shaped job type (RELEASE/RETRY/WEB_HOOK) requires a clonable HTTP(S) Git repository, uses the app's encrypted connected-repository credential, resolves a branch/app build specification or the checked-in `amplify.yml`, and executes backend, frontend, and test pre/build/post phases inside the managed multi-language image. Monorepo `applications`/`appRoot`/`buildPath`, build-spec environment values with app/branch precedence, persistent declared cache paths, and artifact collection all drive the real job. Per-step logs serve from Amazon S3 through each step's `logUrl`; SUCCEED/FAILED comes from the container and artifact result. Unsupported repositories and missing or invalid build specifications fail before job creation; manual deployments accept and publish the uploaded ZIP through the real CreateDeployment/StartDeployment flow.
- **Hosting data plane** (`amplify_dataplane.go`, host-addressed WrapHandler — not a mux route): serves each branch's active deployment (latest SUCCEED job's artifact zip / fileMap) on `{branch}.{appId}.amplifyapp.com`, the deterministic per-app `{hash}.cloudfront.net` host the subdomain dnsRecords advertise (no CloudFront control-plane object — real Amplify's distribution is internal), and verified custom domains. Custom rules (200/301/302/404/404-200, `<*>` wildcards) and basicAuthCredentials (base64 `user:pass`) are enforced; no deployment ⇒ 404.
- **SSR / WEB_COMPUTE** (`amplify_compute.go`): bundles whose root carries `deploy-manifest.json` (Amplify Hosting deployment spec, version 1) route Static targets from the bundle's `static/` directory and proxy Compute targets to a long-lived node container per branch active-deployment (entrypoint under `compute/{name}/`, PORT=3000, lazily started on first request, replaced on new deploys, stopped on branch/app delete). ImageOptimization targets are 501 (the sim has no image-optimization service).
- **Domain verification** (`amplify_domains.go`): AMPLIFY_MANAGED associations start PENDING_VERIFICATION and flip AVAILABLE (subdomains Verified) only when the advertised certificate-verification CNAME exists in a sim Route 53 hosted zone covering the domain — evaluated at read time, so terraform's `wait_for_verification` polling converges and a domain with no hosted zone stays PENDING_VERIFICATION. CUSTOM-certificate associations settle immediately (no DNS challenge to wait on).

Tests: unit (buildSpec/manifest/rule/host-matcher/verification tables in `simulators/aws/amplify_*_test.go`), SDK e2e (`sdk-tests/amplify_hosting_test.go` — hosting, SSR, private authenticated Git, Python and Node.js monorepo phases, cache restoration, artifacts, and Route 53 verification), CLI hosting smoke (`cli-tests/amplify_test.go`), Terraform zone+verification-record fixture (`terraform-tests/main.tf`), and authenticated Chromium app/branch lifecycle (`ui/e2e/shauth-rps.mjs`). Build/SSR/hosting e2e need Docker (they ride the always-Docker sdk-tests suite); the unit suite stays Docker-free.
<!-- HAND-WRITTEN END -->
