# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current State

- Branch: `main`, synced with `origin/main` after the Azure ARM/DNS fidelity PR merged.
- Active implementation branch: none.
- Open GitHub issues at last check: #304, #309-#312, #315, #321-#329, and #332-#338.
- Open BUG trackers: BUG-1075, BUG-1104, BUG-1254, BUG-1263, BUG-1264, and BUG-1267.
- Last completed work: Azure ARM/DNS issues #313, #314, and #340 were fixed.

## Next Task

Address the GCP issue group next: issue #304 / BUG-1254 plus GCP fidelity issues #309-#311 and #321-#325, unless a higher-priority issue appears.

The Azure ARM/DNS PR closed the narrow control-plane list/validation group. The next highest-value work is the GCP group because it combines the already-planned stale public-client coverage audit with public API shape bugs in Cloud Run, Cloud Logging, GCS, Cloud SQL, and Cloud DNS.

## Provider Facts To Preserve

- AzureRM is the hard requirement. `metadata_host` is host-only and provider source constructs `https://<host>` for custom Azure metadata discovery.
- Azure Stack is also HTTPS-shaped for ARM/metadata use.
- AzAPI exposes full endpoint URLs and defaults to HTTPS Azure endpoints; it is configurable but should work through the same gateway.
- Google Terraform provider custom endpoints are full URLs; current HTTP simulator endpoint overrides are valid and should keep working.
- AWS Terraform provider custom endpoints are full URLs; official docs explicitly support `http://localhost` service endpoints. HTTPS is optional for realism and CA-bundle coverage.
- Existing simulator direct TLS support via `SIM_TLS_CERT` / `SIM_TLS_KEY` stays. The gateway is an operator/developer front door, not a replacement for direct simulator TLS.

## Completed Gateway Stage

- Caddy config plus `make stack-https-{up,status,ca,down}` targets.
- Caddy local-CA trust-store installation was disabled with `skip_install_trust`; provider tests trusted the exported CA file explicitly and kept TLS verification enabled.
- HTTPS routes to current simulator ports:
   - `aws.sockerless.localhost` -> `127.0.0.1:4566`
   - `gcp.sockerless.localhost` -> `127.0.0.1:4567`
   - `azure.sockerless.localhost` -> `127.0.0.1:4568`
   - Azure data-plane wildcards -> Azure simulator, preserving host-addressed routing.
- `STACK_HTTPS=1` local stack integration, including Azure ARM-advertised data-plane URL projection.
- Admin UI visibility for gateway status, endpoints, CA path, and equivalent recovery `make` commands.
- Azure Terraform tests through the gateway, including Caddy state isolation, CA trust, ARM metadata verification, Azure data-plane endpoint projection, and a 300-second test timeout.
- Shared simulator Docker test image with Caddy installed from the official package repository.
- BUG-1246 fixed Azure Storage data-plane middleware overmatching non-storage `*.localhost` hosts.
- SDK/CLI guidance documents real endpoint and CA knobs for AWS CLI/SDKs, gcloud/Google clients, Azure CLI, and Azure SDKs.
- BUG-1250/BUG-1251 fixed stale `gcp-gcs` CLI coverage: `gcloud storage` now has real bucket/object lifecycle coverage, current gcloud multipart uploads work, GCS `buckets.getStorageLayout` returns the public response shape, and GCS timestamps use Cloud Storage-style millisecond precision.
- AWS/GCP now had `make terraform-https-test` targets. They start the simulator on HTTP loopback, put Caddy in front of it, trust Caddy's CA through `SSL_CERT_FILE`, and run the real Terraform provider apply/destroy harness against the gateway's `https://localhost:<ephemeral-port>` single-simulator route. On macOS those targets run inside the shared Linux simulator test image so provider CA trust matches CI.
- Terraform CI installed Caddy for the Terraform matrix and ran AWS/GCP via the HTTPS gateway targets; Azure continued using its mandatory Caddy-backed Terraform harness.
- BUG-1253 fixed stale `gcp-vpcaccess` Terraform coverage by adding `google_vpc_access_connector` to the GCP Terraform stack and marking the matrix row direct.
- AWS simulator fidelity issues #305-#308 and #317-#320 were fixed:
   - S3 `ListObjectsV2` sorted keys, honored `start-after` / `continuation-token`, emitted `NextContinuationToken`, and returned delimiter `CommonPrefixes`.
   - Lambda `FunctionConfiguration` responses no longer leaked request `Code`, uploaded `ZipFile`, or `Tags`; `GetFunction` kept `Code` and `Tags` only as top-level response members.
   - SNS returned `pending confirmation` for confirmation-required protocols unless `ReturnSubscriptionArn=true`, and topic attributes counted confirmed vs pending subscriptions.
   - SQS rejected invalid `MaxNumberOfMessages` values with `InvalidParameterValue` instead of silently clamping them.
   - EC2 `RunInstances` honored `MinCount`/`MaxCount`, returned `pending` instances, transitioned them to `running`, and `DescribeInstances` applied supported filters while rejecting unsupported filter names.
   - ECR `PutImage` generated deterministic content-addressed `sha256:<64-hex>` digests from image manifests.
   - KMS `GenerateDataKey` returned fresh crypto-random plaintext key material and ciphertext that decrypted back to it.
- AWS Amplify issues #330 and #331 were fixed:
   - `StopJob` used the real `DELETE /apps/{appId}/branches/{branchName}/jobs/{jobId}/stop` route and cancelled the job.
   - `DeleteJob` used the real `DELETE /apps/{appId}/branches/{branchName}/jobs/{jobId}` route, removed the job, removed its artifacts, and made later `GetJob` calls return `NotFoundException`.
   - `ListArtifacts`, `GetArtifactUrl`, and `GenerateAccessLogs` were registered with their AWS SDK REST paths and covered through the real AWS SDK and AWS CLI.
- Azure ARM/DNS issues #313, #314, and #340 were fixed:
   - ARM control-plane requests required `api-version` and returned `InvalidApiVersionParameter` when omitted.
   - Empty store-backed ARM lists serialized `{"value":[]}` rather than `{"value":null}`.
   - Private DNS zones implemented list-by-resource-group, and Private DNS virtual network links implemented list-by-zone.
   - The routes were covered through real Azure SDK and Azure CLI tests.

## Remaining Stages

1. Address BUG-1254 / issue #304 plus GCP issues #309-#311 and #321-#325.
2. Address Azure issues #312, #315, and #326-#329.
3. Stage the real-execution compute/networking track from BUG-1267 / issues #332-#336. Start with an architecture/substrate PR before changing public instance, VPC, load balancer, or firewall behavior.

## Deferred Trackers

- BUG-1075: live-cloud validation remains deferred by user direction. Do not mark cloud cells green without authenticated real-cloud runs.
- BUG-1104: audit-cadence meta tracker remains open. Every simulator phase should audit SDK/CLI/Terraform surface claims and file concrete BUGs before fixing.
- BUG-1254: issue #304 tracks larger GCP client-surface coverage gaps discovered by the latest audit pass.
- BUG-1263: GCP API-shape backlog from issues #309-#311 and #321-#325 remains open.
- BUG-1264: Azure API-shape backlog from issues #312, #315, and #326-#329 remains open.
- BUG-1267: issues #332-#336 track the compute/networking real-execution program: Firecracker-backed VM instances, real netns/bridge/tap/IPAM/routing/NAT, nftables security enforcement, and real L4/L7 load balancing with health checks.

## Start Checklist

1. `git fetch origin`
2. `git checkout main`
3. `git pull origin main`
4. `gh issue list --state open --limit 30`
5. Create a fresh branch from `origin/main`.
6. File any concrete gaps in `BUGS.md` before code changes.

## Rules That Matter For This Task

- No simulator-specific public API changes.
- No mocks, fakes, or fallback modes.
- No interactive commands.
- Rebase the PR branch on `origin/main` before pushing.
- User merges PRs; never run `gh pr merge`.
