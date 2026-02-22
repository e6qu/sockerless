# Cloud Run Backend — Terraform Example

This example provisions the GCP infrastructure needed to run Sockerless with the Cloud Run Jobs backend. Once applied, you can use standard `docker` CLI commands and they will execute as Cloud Run Job executions.

## What Gets Created

- **GCP API enablement** — Cloud Run, VPC Access, Cloud DNS, Cloud Logging, Artifact Registry, Cloud Storage
- **VPC Network** with Serverless VPC Access Connector
- **Cloud DNS Private Zone** for service discovery
- **Cloud Storage Bucket** for volume mounts
- **Artifact Registry** (Docker repository) for container images
- **IAM Service Account** with roles for invoker, logging, storage, DNS, and Artifact Registry

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.5
- [gcloud CLI](https://cloud.google.com/sdk/docs/install) authenticated (`gcloud auth application-default login`)
- A GCP project with billing enabled
- Go 1.24+ (to build the backend binary)

## Step 1: Apply the Terraform

```bash
cd backends/cloudrun/examples/terraform

terraform init
terraform plan -var="project_id=YOUR_GCP_PROJECT_ID"
terraform apply -var="project_id=YOUR_GCP_PROJECT_ID"
```

This takes approximately 3-5 minutes (VPC connector creation is the slowest).

To customize:

```bash
terraform apply \
  -var="project_id=my-gcp-project" \
  -var="region=europe-west1" \
  -var="environment=staging"
```

## Step 2: Push Your Container Image to Artifact Registry

```bash
AR_URL=$(terraform output -raw artifact_registry_url)
REGION=$(terraform output -raw region)

# Configure Docker for Artifact Registry
gcloud auth configure-docker ${REGION}-docker.pkg.dev

# Tag and push
docker tag alpine:latest ${AR_URL}/alpine:latest
docker push ${AR_URL}/alpine:latest
```

## Step 3: Export the Backend Configuration

```bash
# Quick method
terraform output -raw backend_env

# Or manually
export SOCKERLESS_GCR_PROJECT=$(terraform output -raw project_id)
export SOCKERLESS_GCR_REGION=$(terraform output -raw region)
export SOCKERLESS_GCR_VPC_CONNECTOR=$(terraform output -raw vpc_connector_name)
export SOCKERLESS_GCR_LOG_ID=sockerless
```

For reverse agent mode (optional):

```bash
export SOCKERLESS_CALLBACK_URL=http://<YOUR_BACKEND_HOST>:9100
```

## Step 4: Build and Run the Backend

```bash
cd backends/cloudrun
go build -o sockerless-backend-cloudrun ./cmd/sockerless-backend-cloudrun
./sockerless-backend-cloudrun -addr :9100
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
AR_URL=$(cd backends/cloudrun/examples/terraform && terraform output -raw artifact_registry_url)

# Pull (creates synthetic reference)
docker pull ${AR_URL}/alpine:latest

# Run a command
docker run --rm ${AR_URL}/alpine:latest echo "Hello from Cloud Run!"
```

Behind the scenes:
1. `docker create` → Stores container metadata locally
2. `docker start` → `Jobs.CreateJob` + `Jobs.RunJob`
3. Agent connects (forward polling or reverse callback)
4. `docker rm` → `Jobs.DeleteJob`

### Create, exec, logs

```bash
# Create and start
docker create --name myjob ${AR_URL}/alpine:latest tail -f /dev/null
docker start myjob

# Execute commands
docker exec myjob ls /
docker exec myjob cat /etc/os-release
docker exec -it myjob sh

# View logs (from Cloud Logging)
docker logs myjob
docker logs -f myjob   # follow mode (polls every 5s)
docker logs --timestamps myjob

# Inspect
docker inspect myjob

# Stop (cancels execution)
docker stop myjob

# Remove (deletes the Cloud Run Job)
docker rm myjob
```

### Copy files

```bash
# Copy to container (via agent)
echo "test data" > /tmp/test.txt
docker cp /tmp/test.txt myjob:/tmp/test.txt

# Copy from container
docker cp myjob:/etc/hostname /tmp/hostname.txt
```

### Restart

```bash
# Restart deletes old job, creates new job + execution
docker restart myjob
```

### List and prune

```bash
docker ps -a
docker container prune   # removes exited containers + deletes their Cloud Run Jobs
```

## Step 7: Destroy the Infrastructure

```bash
cd backends/cloudrun/examples/terraform
terraform destroy -var="project_id=YOUR_GCP_PROJECT_ID"
```

**Important:** Clean up any Cloud Run Jobs first:

```bash
# List jobs created by Sockerless
gcloud run jobs list --region=$(terraform output -raw region) --filter="metadata.labels.managed-by=sockerless"

# Delete them
gcloud run jobs delete sockerless-<id> --region=$(terraform output -raw region) --quiet
```

## Architecture Diagram

```
┌──────────────┐     ┌──────────────────┐     ┌─────────────────────────┐
│  docker CLI  │────▶│ Sockerless       │────▶│ Google Cloud Run Jobs   │
│              │     │ Frontend + Backend│     │                         │
│ pull, create,│     │ (localhost:9100)  │     │ Jobs.CreateJob          │
│ start, exec, │     │                  │     │ Jobs.RunJob             │
│ logs, stop   │     │                  │     │ Executions.GetExecution │
└──────────────┘     └──────────────────┘     │ Executions.Cancel       │
                                               │ LogAdmin.Entries        │
                                               └─────────────────────────┘
```

## Agent Modes

### Forward Agent (default)

After `RunJob` creates an execution, the backend polls `GetExecution` until the execution is RUNNING. The agent address is derived from the execution name on port 9111. The backend connects to the agent for exec/attach operations.

### Reverse Agent

Set `SOCKERLESS_CALLBACK_URL` to enable. The agent inside the job execution calls back to the Sockerless backend. Useful when the backend cannot reach the Cloud Run execution directly.

## Key Differences from Vanilla Docker

| Feature | Vanilla Docker | Cloud Run Backend |
|---------|---------------|-------------------|
| Create | Immediate | No GCP call (stored locally) |
| Start | Starts process | Creates Job + runs Execution |
| Stop | SIGTERM → SIGKILL | CancelExecution |
| Logs follow | Real-time | Polls Cloud Logging every 5s |
| Pause/unpause | Freeze cgroups | Not supported |
| Networks | Real Docker networks | In-memory only (VPC connector for egress) |
| Volumes | Real volumes | In-memory only |
| Resources | Configurable | Fixed: 1 CPU / 512 Mi |

## Estimated Costs

- **VPC Connector**: ~$0.01/hr per instance (~$15/month for 2 instances)
- **Cloud Run Jobs**: Per-execution pricing (CPU + memory per second)
- **Cloud Logging**: First 50 GB/month free, then $0.50/GB
- **Artifact Registry**: $0.10/GB/month
- **Cloud Storage**: $0.020/GB/month

The VPC connector has a fixed cost. For development, destroy when not in use.

## Troubleshooting

**Job creation fails:** Ensure the GCP project has billing enabled and the Cloud Run API is active.

**Execution stays in PENDING:** Check that the image exists in Artifact Registry and is accessible by the service account.

**Logs are empty:** Cloud Logging filter uses `resource.type="cloud_run_job"`. Entries may take a few seconds to appear.

**Agent health check fails (forward mode):** The VPC connector must be properly configured for the backend to reach the execution's agent port.
