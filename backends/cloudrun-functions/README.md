# Cloud Run Functions Backend

Runs Docker containers as Google Cloud Run Functions (2nd gen), with Cloud Logging for log streaming. Frontend speaks Docker REST API v1.44; backend speaks the Cloud Functions v2 / Cloud Logging / Artifact Registry APIs.

## Reference adaptors

| Direction | Adaptor | Min version | What it proves |
|---|---|---|---|
| **Frontend (Docker API)** | [Docker Go SDK](https://pkg.go.dev/github.com/docker/docker/client) | v25+ | `docker run` → Cloud Function invoke via `tcp://localhost:3375`. |
| | [`docker` CLI](https://docs.docker.com/engine/reference/commandline/cli/) | 29.x | Wire-level [Docker REST API v1.44](https://docs.docker.com/engine/api/v1.44/). |
| **Backend (GCP API)** | [`gcloud` CLI](https://cloud.google.com/sdk/gcloud/reference/functions) | 480+ | `gcloud functions describe`, `gcloud functions logs read`. |
| | [GCP Go SDK](https://pkg.go.dev/cloud.google.com/go/functions) | v1.16+ | [Cloud Functions v2 REST API](https://cloud.google.com/functions/docs/reference/rest) calls (`functions.create`, `functions.invoke` via `serviceConfig.uri`). |
| | [Terraform `google` provider](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloudfunctions2_function) | v6+ | `google_cloudfunctions2_function` provisions the function infra. |

Local development replaces the backend-side upstream with [`simulators/gcp`](../../simulators/gcp/README.md). Container mode only (no native runtimes) — see [`memory/feedback_faas_container_mode.md`](../../).

## Validation

| Test path | What runs | Last green |
|---|---|---|
| `tests/` (Docker SDK against running backend, GCF profile) | Container lifecycle round-trip via Cloud Function invoke. | 2026-05-13 |
| `simulators/gcp/sdk-tests/` Cloud Functions package | The v2 calls this backend issues, validated against the sim. | 2026-05-13 |
| `simulators/gcp/terraform-tests/` | `google_cloudfunctions2_function` apply / destroy round-trip. | 2026-05-13 |
| `make backends/cloudrun-functions/test` | Leaf-Makefile unit + integration suite. | 2026-05-13 |

## Wiring the adaptor

```bash
cd backends/cloudrun-functions && make build
./sockerless-backend-cloudrun-functions --addr :3375 --log-level info &
export DOCKER_HOST=tcp://localhost:3375
```

### Config (config.yaml)

```yaml
environments:
  my-gcf:
    backend: gcf
    addr: ":3375"
    log_level: info
    gcp:
      project: my-gcp-project-123
      gcf:
        region: us-central1
        service_account: sockerless@my-gcp-project-123.iam.gserviceaccount.com
        timeout: 3600
        memory: 1Gi
        cpu: "1"
        log_timeout: 30s
    common:
      callback_url: https://backend.example.com
      poll_interval: 2s
      agent_timeout: 30s
```

Full schema: [`specs/CONFIG.md`](../../specs/CONFIG.md).

### Environment Variables

| Variable | Default | Required | Description |
|---|---|---|---|
| `SOCKERLESS_GCF_PROJECT` | | **yes** | GCP project ID |
| `SOCKERLESS_GCF_REGION` | `us-central1` | no | Functions region |
| `SOCKERLESS_GCF_SERVICE_ACCOUNT` | | no | Service account email for functions |
| `SOCKERLESS_GCF_TIMEOUT` | `3600` | no | Function timeout in seconds (max 3600) |
| `SOCKERLESS_GCF_MEMORY` | `1Gi` | no | Function memory allocation |
| `SOCKERLESS_GCF_CPU` | `1` | no | Function CPU allocation |
| `SOCKERLESS_CALLBACK_URL` | | no | Backend URL for reverse agent callbacks |
| `SOCKERLESS_ENDPOINT_URL` | | no | Custom endpoint (for [`simulators/gcp`](../../simulators/gcp/README.md)) |
| `SOCKERLESS_POLL_INTERVAL` | `2s` | no | Cloud API poll interval |
| `SOCKERLESS_LOG_TIMEOUT` | `30s` | no | Cloud Logging query timeout |
| `SOCKERLESS_AGENT_TIMEOUT` | `30s` | no | Agent callback timeout |

CLI flags: `-addr` (default `:3375`), `-tls-cert`, `-tls-key`, `-log-level` (default `info`).

## Sample

```bash
$ DOCKER_HOST=tcp://localhost:3375 docker run --rm alpine:3.20 echo "hello from gcf"
hello from gcf

$ gcloud functions describe sockerless-fn-abc --region us-central1 --gen2
state: ACTIVE
serviceConfig:
  uri: https://sockerless-fn-abc-...run.app

$ gcloud functions logs read sockerless-fn-abc --region us-central1 --gen2 --limit 5
hello from gcf
```

## Known issues

None open. Same volume-rejection rule as the Cloud Run backend — `BackingPDEphemeral` is rejected loudly; no fallback.

## What's out of scope

- 1st-gen Cloud Functions (deprecated; this backend targets 2nd gen).
- Native source builds (`gcloud functions deploy --source=.`).
- Event-triggered functions (Pub/Sub, GCS, Firestore triggers) — sockerless creates HTTP-triggered functions only.

## Cloud Notes

- Requires Cloud Functions API, Cloud Build API, and Cloud Logging API enabled.
- Application Default Credentials or a service account key must be available.
- Uses reverse agent exclusively — Cloud Functions cannot accept inbound connections.
- The service account needs `cloudfunctions.developer` and `logging.viewer` roles.
- Function timeout max is 3600 seconds (60 minutes) for 2nd gen functions.

See also: [`backends/gcp-common`](../gcp-common/), [`simulators/gcp/README.md`](../../simulators/gcp/README.md), [`specs/CLOUD_RESOURCE_MAPPING.md`](../../specs/CLOUD_RESOURCE_MAPPING.md).
