# Cloud Run Functions Backend — Terraform Example

This example provisions the GCP infrastructure needed to run Sockerless with the Cloud Run Functions (2nd gen) backend. Once applied, you can use standard `docker` CLI commands and they will execute as Cloud Function invocations.

## What Gets Created

- **GCP API enablement** — Cloud Functions, Artifact Registry, Cloud Logging, Cloud Build, Cloud Run
- **Artifact Registry** (Docker repository) for function images
- **IAM Service Account** with roles for Cloud Functions invoker, logging, and Artifact Registry

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.5
- [gcloud CLI](https://cloud.google.com/sdk/docs/install) authenticated (`gcloud auth application-default login`)
- A GCP project with billing enabled
- Go 1.24+ (to build the backend binary)

## Step 1: Apply the Terraform

```bash
cd backends/cloudrun-functions/examples/terraform

terraform init
terraform plan -var="project_id=YOUR_GCP_PROJECT_ID"
terraform apply -var="project_id=YOUR_GCP_PROJECT_ID"
```

This takes approximately 1-2 minutes (mainly API enablement).

## Step 2: Push Your Container Image to Artifact Registry

```bash
AR_URL=$(terraform output -raw artifact_registry_url)
REGION=$(terraform output -raw region)

gcloud auth configure-docker ${REGION}-docker.pkg.dev

docker tag alpine:latest ${AR_URL}/alpine:latest
docker push ${AR_URL}/alpine:latest
```

## Step 3: Export the Backend Configuration

```bash
# Quick method
terraform output -raw backend_env

# Or manually
export SOCKERLESS_GCF_PROJECT=$(terraform output -raw project_id)
export SOCKERLESS_GCF_REGION=$(terraform output -raw region)
export SOCKERLESS_GCF_SERVICE_ACCOUNT=$(terraform output -raw service_account_email)
export SOCKERLESS_CALLBACK_URL=http://<YOUR_BACKEND_HOST>:9100
```

**Important:** `SOCKERLESS_CALLBACK_URL` is required. Cloud Functions uses reverse agent mode exclusively — functions cannot accept inbound connections. Replace `<YOUR_BACKEND_HOST>` with a publicly reachable address.

## Step 4: Build and Run the Backend

```bash
cd backends/cloudrun-functions
go build -o sockerless-backend-gcf ./cmd/sockerless-backend-gcf
./sockerless-backend-gcf -addr :9100
```

## Step 5: Configure Docker to Use Sockerless

```bash
cd frontends/docker
go build -o sockerless-frontend-docker .
./sockerless-frontend-docker -backend http://localhost:9100 -addr unix:///tmp/sockerless.sock

export DOCKER_HOST=unix:///tmp/sockerless.sock
```

## Step 6: Use Docker Commands

### Pull and run

```bash
AR_URL=$(cd backends/cloudrun-functions/examples/terraform && terraform output -raw artifact_registry_url)

docker pull ${AR_URL}/alpine:latest
docker run --rm ${AR_URL}/alpine:latest echo "Hello from Cloud Functions!"
```

Behind the scenes:
1. `docker create` → `Functions.CreateFunction` (Docker runtime, synchronous — waits for build)
2. `docker start` → HTTP POST to function URL
3. Agent calls back to `SOCKERLESS_CALLBACK_URL`
4. `docker rm` → `Functions.DeleteFunction`

**Note:** Container creation is slower than other backends because Cloud Functions builds the image during `CreateFunction`. This can take 1-3 minutes.

### Create, exec, logs

```bash
# Create (this triggers function creation — may take 1-3 min)
docker create --name myfunc ${AR_URL}/alpine:latest tail -f /dev/null

# Start (invokes the function, agent calls back)
docker start myfunc

# Execute commands via reverse agent
docker exec myfunc ls /
docker exec myfunc cat /etc/os-release

# View logs (from Cloud Logging)
docker logs myfunc

# Remove (deletes the Cloud Function)
docker rm -f myfunc
```

### Limitations to be aware of

```bash
# Stop is a no-op (functions run to completion)
docker stop myfunc

# Kill only disconnects the reverse agent
docker kill myfunc

# No follow mode for logs (single snapshot)
docker logs -f myfunc

# Restart is a no-op
docker restart myfunc

# No volumes or bind mounts
docker run -v /data:/data ...   # not supported
```

## Step 7: Destroy the Infrastructure

```bash
cd backends/cloudrun-functions/examples/terraform
terraform destroy -var="project_id=YOUR_GCP_PROJECT_ID"
```

**Important:** Delete any Cloud Functions first:

```bash
# List functions created by Sockerless (named skls-*)
gcloud functions list --region=$(terraform output -raw region) --filter="name~skls-"

# Delete them
gcloud functions delete skls-<id> --region=$(terraform output -raw region) --quiet
```

## Architecture Diagram

```
┌──────────────┐     ┌──────────────────┐     ┌────────────────────────┐
│  docker CLI  │────▶│ Sockerless       │────▶│ Cloud Run Functions    │
│              │     │ Frontend + Backend│     │                        │
│ pull, create,│     │ (localhost:9100)  │     │ Functions.Create       │
│ start, exec, │     │                  │◀────│ HTTP POST invoke       │
│ logs, rm     │     │ ◀── agent calls  │     │ Functions.Delete       │
└──────────────┘     │     back here    │     │ LogAdmin.Entries       │
                     └──────────────────┘     └────────────────────────┘
```

## Key Differences from Vanilla Docker

| Feature | Vanilla Docker | GCF Backend |
|---------|---------------|-------------|
| Create | Instant | 1-3 min (function build) |
| Stop | SIGTERM → SIGKILL | No-op (runs to completion) |
| Kill | Sends signal | Disconnects agent only |
| Logs follow | Real-time | Single snapshot |
| Restart | Stop + start | No-op |
| Networks | Docker networks | Not supported |
| Volumes | Docker volumes | Not supported |
| Exec | nsenter | Reverse agent relay |

## Estimated Costs

- **Cloud Functions**: Per-invocation ($0.40/1M) + compute time
- **Cloud Logging**: First 50 GB/month free
- **Artifact Registry**: $0.10/GB/month
- **Cloud Build**: First 120 min/day free

## Troubleshooting

**Function creation times out:** Cloud Functions builds the Docker image during creation. Ensure the image is valid and the Artifact Registry URL is correct.

**Agent callback timeout:** The `SOCKERLESS_CALLBACK_URL` must be reachable from the Cloud Function. If running locally, you may need a tunnel (e.g., ngrok) or a public IP.

**Logs are empty:** Cloud Logging uses `resource.type="cloud_run_revision"` filter. Entries may take several seconds to appear.
