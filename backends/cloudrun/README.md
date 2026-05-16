# Cloud Run Backend

Runs Docker containers as Google Cloud Run Jobs and Executions, with Cloud Logging for log streaming. Frontend speaks Docker REST API v1.44; backend speaks the Cloud Run / Cloud Logging / Artifact Registry / IAM APIs.

## Reference adaptors

| Direction | Adaptor | Min version | What it proves |
|---|---|---|---|
| **Frontend (Docker API)** | [Docker Go SDK](https://pkg.go.dev/github.com/docker/docker/client) | v25+ | `docker run` → Cloud Run Job execution via `tcp://localhost:3375`. |
| | [`docker` CLI](https://docs.docker.com/engine/reference/commandline/cli/) | 29.x | Wire-level [Docker REST API v1.44](https://docs.docker.com/engine/api/v1.44/). |
| **Backend (GCP API)** | [`gcloud` CLI](https://cloud.google.com/sdk/gcloud/reference/run/jobs) | 480+ | `gcloud run jobs describe`, `gcloud logging read` — operators inspect job state. |
| | [GCP Go SDK](https://pkg.go.dev/cloud.google.com/go/run) | v1.6+ | The [Cloud Run Admin v2 REST API](https://cloud.google.com/run/docs/reference/rest) (`jobs.create`, `jobs.run`, `executions.get`) the backend issues. |
| | [Terraform `google` provider](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloud_run_v2_job) | v6+ | `google_cloud_run_v2_job` provisions the job infra; `simulators/gcp/terraform-tests/` covers the path. |

Local development replaces the backend-side upstream with [`simulators/gcp`](../../simulators/gcp/README.md). The container → Job/Execution mapping is documented in [`docs/POD_MATERIALIZATION.md § Cloud Run`](../../docs/POD_MATERIALIZATION.md).

## Validation

| Test path | What runs | Last green |
|---|---|---|
| `tests/` (Docker SDK against running backend, Cloud Run profile) | Container lifecycle round-trip via Cloud Run Job. | 2026-05-13 |
| `simulators/gcp/sdk-tests/` Cloud Run package | The Admin v2 calls this backend issues, validated against the sim. | 2026-05-13 |
| `simulators/gcp/terraform-tests/` | `google_cloud_run_v2_job` apply / destroy round-trip. | 2026-05-13 |
| `make backends/cloudrun/test` | Leaf-Makefile unit + integration suite. | 2026-05-13 |

## Wiring the adaptor

```bash
cd backends/cloudrun && make build
./sockerless-backend-cloudrun --addr :3375 --log-level info &
export DOCKER_HOST=tcp://localhost:3375
```

### Config (config.yaml)

```yaml
environments:
  my-cloudrun:
    backend: cloudrun
    addr: ":3375"
    log_level: info
    gcp:
      project: my-gcp-project-123
      cloudrun:
        region: us-central1
        vpc_connector: projects/my-gcp-project-123/locations/us-central1/connectors/my-vpc
        log_id: sockerless
        log_timeout: 30s
    common:
      agent_image: sockerless/agent:latest
      agent_token: my-secret-token
      callback_url: https://backend.example.com
      poll_interval: 2s
      agent_timeout: 30s
```

Full schema: [`specs/CONFIG.md`](../../specs/CONFIG.md).

### Environment Variables

| Variable | Default | Required | Description |
|---|---|---|---|
| `SOCKERLESS_GCR_PROJECT` | | **yes** | GCP project ID |
| `SOCKERLESS_GCR_REGION` | `us-central1` | no | Cloud Run region |
| `SOCKERLESS_GCR_VPC_CONNECTOR` | | no | Serverless VPC Access connector |
| `SOCKERLESS_GCR_LOG_ID` | `sockerless` | no | Cloud Logging log ID |
| `SOCKERLESS_GCR_AGENT_IMAGE` | `sockerless/agent:latest` | no | Sidecar agent container image |
| `SOCKERLESS_GCR_AGENT_TOKEN` | | no | Agent authentication token |
| `SOCKERLESS_CALLBACK_URL` | | no | Backend URL for reverse agent mode |
| `SOCKERLESS_ENDPOINT_URL` | | no | Custom endpoint (for [`simulators/gcp`](../../simulators/gcp/README.md)) |
| `SOCKERLESS_POLL_INTERVAL` | `2s` | no | Cloud API poll interval |
| `SOCKERLESS_AGENT_TIMEOUT` | `30s` | no | Agent health-check timeout |
| `SOCKERLESS_LOG_TIMEOUT` | `30s` | no | Cloud Logging query timeout |

CLI flags: `-addr` (default `:3375`), `-tls-cert`, `-tls-key`, `-log-level` (default `info`).

## Sample

```bash
$ DOCKER_HOST=tcp://localhost:3375 docker run --rm alpine:3.20 echo "hello from cloudrun"
hello from cloudrun

$ gcloud run jobs executions list --job sockerless-job-abc --region us-central1
NAME                 STATUS     COMPLETION_TIME
sockerless-job-abc-1 Succeeded  2026-05-15T...

$ gcloud logging read 'resource.type="cloud_run_job"' --limit 5 --format json | jq -r '.[].textPayload'
hello from cloudrun
```

## Known issues

None open. Cloud Run lacks a protobuf field for real pd-ephemeral volumes (Phase 91d bookmarked indefinitely) — see [`PLAN.md § Phase 91d`](../../PLAN.md). `BackingPDEphemeral` is rejected; volume requests for pd-ephemeral fail loudly per the no-fallbacks rule.

## What's out of scope

- Cloud Run Services (long-running HTTP endpoints) — this backend uses Jobs only.
- Cloud Build orchestration.
- Native source-based deployments (`gcloud run deploy --source=.`) — operator-side concern.

## Cloud Notes

- Requires Cloud Run API and Cloud Logging API enabled in the GCP project.
- Application Default Credentials or a service account key must be available.
- Container images must be in Artifact Registry or GCR within the same project.
- Supports forward agent (polls execution for IP) and reverse agent (`callback_url`).
- VPC connector is only needed if services must reach private VPC resources.

See also: [`backends/gcp-common`](../gcp-common/), [`simulators/gcp/README.md`](../../simulators/gcp/README.md), [`specs/CLOUD_RESOURCE_MAPPING.md § Cloud Run`](../../specs/CLOUD_RESOURCE_MAPPING.md).
