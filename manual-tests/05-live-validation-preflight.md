# Live validation preflight

This is the operator runbook for the remaining live-cloud validation work. It
does not mark any live cell green by itself; it exists so the live run can be
performed repeatably once credentials and ephemeral cloud resources are
available.

## Scope

Validate the cells that simulator CI cannot prove:

| Cell | Backend mode | Live dependency |
|---|---|---|
| Cloud Run Services | `backends/cloudrun` with `SOCKERLESS_GCR_USE_SERVICE=1` | Real Cloud Run Services, Cloud Build, Artifact Registry, Cloud Logging, Cloud DNS |
| Cloud Run Functions | `backends/cloudrun-functions` | Real Cloud Functions Gen2 + underlying Cloud Run service invoke auth |
| ACA Apps | `backends/aca` with `SOCKERLESS_ACA_USE_APP=1` | Real Container Apps, managed environment, ACR, Log Analytics, Private DNS |
| Azure Functions | `backends/azure-functions` | Real Linux Function Apps, ACR, Storage, App Insights / Log Analytics |
| Lambda service mesh | `backends/lambda` with reverse-agent bootstrap | Real Lambda, ECR, CloudWatch Logs, VPC/DNS where configured |

## Non-goals

- Do not use simulators to mark these live cells complete.
- Do not reuse persistent shared cloud projects unless the operator explicitly
  accepts leftover resource risk.
- Do not paper over IAM, quota, DNS, or image-build failures with local
  fallbacks. File the failure in `BUGS.md`, fix it, and rerun the affected cell.

## Preflight checklist

1. Start from a clean branch rebased on `origin/main`.
2. Confirm local simulator CI is green for the same commit:

   ```bash
   make faas-smoke-test-all
   make tests/test
   ```

3. Confirm cloud CLIs are authenticated and point at throwaway live resources:

   ```bash
   aws sts get-caller-identity
   gcloud auth list
   az account show
   ```

4. Create ephemeral cloud resources per the per-cloud runbooks:

   - AWS: [02-aws-runbook.md](02-aws-runbook.md)
   - GCP: [03-gcp-runbook.md](03-gcp-runbook.md)
   - Azure: [04-azure-runbook.md](04-azure-runbook.md)

5. Record the project / account / subscription IDs and the exact git commit in
   `STATUS.md` before dispatching any live jobs.

## Manual run sequence

Run one cloud at a time so failures are attributable and cleanup stays simple.

### AWS

```bash
make e2e-github-lambda
make e2e-gitlab-lambda
DOCKER_HOST=tcp://127.0.0.1:<lambda-port> ./scripts/manual-test-real-workloads.sh lambda
```

Capture CloudWatch log group names, Lambda function names, and ECR repos used
by the run. Teardown must remove all functions, repos, log groups, IAM roles,
and VPC/DNS resources created by the run.

### GCP

```bash
make faas-smoke-test-gcp
make e2e-github-cloudrun
make e2e-github-gcf
make e2e-gitlab-cloudrun
make e2e-gitlab-gcf
```

Then run the GCP manual workload bundles from [03-gcp-runbook.md](03-gcp-runbook.md).
Capture Cloud Run service/job names, Cloud Functions names, Artifact Registry
image names, Cloud Build operation IDs, and Cloud Logging query evidence.

### Azure

```bash
make faas-smoke-test-azure
make e2e-github-aca
make e2e-github-azf
make e2e-gitlab-aca
make e2e-gitlab-azf
```

Then run the Azure manual workload bundles from [04-azure-runbook.md](04-azure-runbook.md).
Capture Container App / Function App names, ACR image names, Log Analytics
queries, managed environment IDs, and Private DNS records.

## Evidence to collect

For every cell, record:

- Command invoked and exit status.
- Git commit SHA.
- Cloud project/account/subscription and region.
- Backend env vars, with secrets redacted.
- Resource names created by the backend.
- Cloud log query and a short excerpt showing real workload stdout.
- Teardown command and confirmation that resources are gone.

## Failure handling

Every failure gets a `BUGS.md` entry before a fix attempt. The entry should
state the cloud, backend, command, expected behavior, actual behavior, and
cloud request ID / operation ID where available.

After fixing, rerun the narrow failing command first. Only mark a cell green
after its full runner/workload sequence passes on the fixed commit.

## Teardown

Prefer project/subscription-scoped deletion for throwaway environments:

```bash
gcloud projects delete "$PROJECT"
az group delete --name "$RESOURCE_GROUP" --yes
```

AWS teardown is resource-specific unless the operator used an account-level
sandbox. Verify no Lambda functions, ECR repos, CloudWatch log groups, IAM
roles, VPC resources, or service-discovery resources remain with the
`sockerless` prefix/tags.
