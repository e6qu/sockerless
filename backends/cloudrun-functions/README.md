# Cloud Run Functions Backend

Runs Docker containers as Google Cloud Run Functions (2nd gen), with Cloud Logging for log streaming.

## Config (config.yaml)

```yaml
environments:
  my-gcf:
    backend: gcf
    addr: ":9100"
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

## Environment Variables

| Variable | Default | Required | Description |
|---|---|---|---|
| `SOCKERLESS_GCF_PROJECT` | | **yes** | GCP project ID |
| `SOCKERLESS_GCF_REGION` | `us-central1` | no | Functions region |
| `SOCKERLESS_GCF_SERVICE_ACCOUNT` | | no | Service account email for functions |
| `SOCKERLESS_GCF_TIMEOUT` | `3600` | no | Function timeout in seconds (max 3600) |
| `SOCKERLESS_GCF_MEMORY` | `1Gi` | no | Function memory allocation |
| `SOCKERLESS_GCF_CPU` | `1` | no | Function CPU allocation |
| `SOCKERLESS_CALLBACK_URL` | | no | Backend URL for reverse agent callbacks |
| `SOCKERLESS_ENDPOINT_URL` | | no | Custom endpoint (for simulators) |
| `SOCKERLESS_POLL_INTERVAL` | `2s` | no | Cloud API poll interval |
| `SOCKERLESS_LOG_TIMEOUT` | `30s` | no | Cloud Logging query timeout |
| `SOCKERLESS_AGENT_TIMEOUT` | `30s` | no | Agent callback timeout |

## Quick Start

```sh
go build -o sockerless-backend-gcf ./backends/cloudrun-functions/cmd/sockerless-backend-gcf
./sockerless-backend-gcf -addr :9100 -log-level info
```

Flags: `-addr` (default `:9100`), `-tls-cert`, `-tls-key`, `-log-level` (default `info`).

## Cloud Notes

- Requires Cloud Functions API, Cloud Build API, and Cloud Logging API enabled.
- Application Default Credentials or a service account key must be available.
- Uses reverse agent exclusively -- Cloud Functions cannot accept inbound connections.
- The service account needs `cloudfunctions.developer` and `logging.viewer` roles.
- Function timeout max is 3600 seconds (60 minutes) for 2nd gen functions.
- See `specs/CONFIG.md` for the full unified config specification.
