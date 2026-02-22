# Using the GCP simulator with the gcloud CLI

## Prerequisites

- gcloud CLI installed (`gcloud version`)
- Simulator running on `http://localhost:4567`

## Setup

The gcloud CLI supports per-service endpoint overrides via `CLOUDSDK_API_ENDPOINT_OVERRIDES_*` environment variables. Set up an isolated gcloud config and a fake auth token:

```sh
export CLOUDSDK_CONFIG=/tmp/gcloud-sim-config
export CLOUDSDK_AUTH_ACCESS_TOKEN=fake-gcp-token
export CLOUDSDK_CORE_PROJECT=my-project
export CLOUDSDK_CORE_DISABLE_PROMPTS=1
```

Then override the endpoints for the services you need:

```sh
export CLOUDSDK_API_ENDPOINT_OVERRIDES_DNS=http://localhost:4567/
export CLOUDSDK_API_ENDPOINT_OVERRIDES_LOGGING=http://localhost:4567/
export CLOUDSDK_API_ENDPOINT_OVERRIDES_CLOUDFUNCTIONS=http://localhost:4567/
export CLOUDSDK_API_ENDPOINT_OVERRIDES_SERVICEUSAGE=http://localhost:4567/
export CLOUDSDK_API_ENDPOINT_OVERRIDES_VPCACCESS=http://localhost:4567/
```

Note the trailing `/` — gcloud appends API paths directly to the override URL.

## Examples

### Cloud DNS

```sh
# Create a managed zone
gcloud dns managed-zones create my-zone \
  --dns-name=example.com. \
  --description="Test zone" \
  --visibility=private \
  --format=json

# Describe a zone
gcloud dns managed-zones describe my-zone --format=json

# List zones
gcloud dns managed-zones list --format=json

# Delete a zone
gcloud dns managed-zones delete my-zone
```

### Cloud Logging

```sh
# List log entries (via gcloud or direct HTTP)
gcloud logging read "resource.type=global" --project=my-project --format=json
```

### Service Usage

```sh
# Enable a service
gcloud services enable compute.googleapis.com

# List enabled services
gcloud services list --enabled --format=json
```

### VPC Access

```sh
# Create a connector
gcloud compute networks vpc-access connectors create my-connector \
  --region=us-central1 \
  --network=default \
  --range=10.8.0.0/28

# List connectors
gcloud compute networks vpc-access connectors list \
  --region=us-central1 \
  --format=json
```

### Direct HTTP (for services without CLI endpoint overrides)

Some services work better with direct HTTP calls since gcloud doesn't support endpoint overrides for all APIs:

```sh
# Cloud Run Jobs — Create a job
curl -X POST http://localhost:4567/v2/projects/my-project/locations/us-central1/jobs?jobId=my-job \
  -H "Authorization: Bearer fake-gcp-token" \
  -H "Content-Type: application/json" \
  -d '{
    "template": {
      "template": {
        "containers": [{"image": "nginx:latest"}]
      }
    }
  }'

# Cloud Run Jobs — Get a job
curl http://localhost:4567/v2/projects/my-project/locations/us-central1/jobs/my-job \
  -H "Authorization: Bearer fake-gcp-token"

# Cloud Functions — Create a function
curl -X POST "http://localhost:4567/v2/projects/my-project/locations/us-central1/functions?functionId=my-func" \
  -H "Authorization: Bearer fake-gcp-token" \
  -H "Content-Type: application/json" \
  -d '{
    "buildConfig": {"runtime": "docker"},
    "serviceConfig": {"environmentVariables": {"FOO": "bar"}}
  }'

# GCS — Create a bucket
curl -X POST http://localhost:4567/storage/v1/b \
  -H "Authorization: Bearer fake-gcp-token" \
  -H "Content-Type: application/json" \
  -d '{"name": "my-bucket"}'

# GCS — Upload an object
curl -X POST "http://localhost:4567/upload/storage/v1/b/my-bucket/o?name=hello.txt" \
  -H "Authorization: Bearer fake-gcp-token" \
  -H "Content-Type: text/plain" \
  -d 'hello world'

# IAM — Create a service account
curl -X POST http://localhost:4567/v1/projects/my-project/serviceAccounts \
  -H "Authorization: Bearer fake-gcp-token" \
  -H "Content-Type: application/json" \
  -d '{"accountId": "my-sa", "serviceAccount": {"displayName": "My SA"}}'

# Compute — Create a network
curl -X POST http://localhost:4567/compute/v1/projects/my-project/global/networks \
  -H "Authorization: Bearer fake-gcp-token" \
  -H "Content-Type: application/json" \
  -d '{"name": "my-network", "autoCreateSubnetworks": false}'
```

## Supported services

| Service | gcloud Subcommand | Endpoint Override | Notes |
|---------|------------------|-------------------|-------|
| Cloud DNS | `gcloud dns` | `CLOUDSDK_API_ENDPOINT_OVERRIDES_DNS` | Full CLI support |
| Cloud Logging | `gcloud logging` | `CLOUDSDK_API_ENDPOINT_OVERRIDES_LOGGING` | Full CLI support |
| Service Usage | `gcloud services` | `CLOUDSDK_API_ENDPOINT_OVERRIDES_SERVICEUSAGE` | Full CLI support |
| VPC Access | `gcloud compute networks vpc-access` | `CLOUDSDK_API_ENDPOINT_OVERRIDES_VPCACCESS` | Full CLI support |
| Cloud Functions | `gcloud functions` | `CLOUDSDK_API_ENDPOINT_OVERRIDES_CLOUDFUNCTIONS` | Deploy may require direct HTTP |
| Cloud Run Jobs | — | — | Use direct HTTP |
| GCS | — | — | Use direct HTTP or `STORAGE_EMULATOR_HOST` |
| Artifact Registry | — | — | Use direct HTTP or Docker CLI |
| Compute | — | — | Use direct HTTP |
| IAM | — | — | Use direct HTTP |

## Notes

- Authentication is accepted but not validated. Any Bearer token will work.
- All state is in-memory and resets when the simulator restarts.
- `CLOUDSDK_CONFIG` should point to an isolated directory to avoid interfering with your real gcloud configuration.
