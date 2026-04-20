# Cloud Run Backend

Runs Docker containers as Google Cloud Run Jobs and Executions, with Cloud Logging for log streaming.

## Config (config.yaml)

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

## Environment Variables

| Variable | Default | Required | Description |
|---|---|---|---|
| `SOCKERLESS_GCR_PROJECT` | | **yes** | GCP project ID |
| `SOCKERLESS_GCR_REGION` | `us-central1` | no | Cloud Run region |
| `SOCKERLESS_GCR_VPC_CONNECTOR` | | no | Serverless VPC Access connector |
| `SOCKERLESS_GCR_LOG_ID` | `sockerless` | no | Cloud Logging log ID |
| `SOCKERLESS_GCR_AGENT_IMAGE` | `sockerless/agent:latest` | no | Sidecar agent container image |
| `SOCKERLESS_GCR_AGENT_TOKEN` | | no | Agent authentication token |
| `SOCKERLESS_CALLBACK_URL` | | no | Backend URL for reverse agent mode |
| `SOCKERLESS_ENDPOINT_URL` | | no | Custom endpoint (for simulators) |
| `SOCKERLESS_POLL_INTERVAL` | `2s` | no | Cloud API poll interval |
| `SOCKERLESS_AGENT_TIMEOUT` | `30s` | no | Agent health-check timeout |
| `SOCKERLESS_LOG_TIMEOUT` | `30s` | no | Cloud Logging query timeout |

## Quick Start

```sh
go build -o sockerless-backend-cloudrun ./backends/cloudrun/cmd/sockerless-backend-cloudrun
./sockerless-backend-cloudrun -addr :3375 -log-level info
```

Flags: `-addr` (default `:3375`), `-tls-cert`, `-tls-key`, `-log-level` (default `info`).

## Cloud Notes

- Requires Cloud Run API and Cloud Logging API enabled in the GCP project.
- Application Default Credentials or a service account key must be available.
- Container images must be in Artifact Registry or GCR within the same project.
- Supports forward agent (polls execution for IP) and reverse agent (`callback_url`).
- VPC connector is only needed if services must reach private VPC resources.
- See `specs/CONFIG.md` for the full unified config specification.
