# ACA Backend — Terraform Example

This example provisions the Azure infrastructure needed to run Sockerless with the Azure Container Apps (ACA) backend. One-shot containers run as Container Apps Jobs; runner and repeated-exec workloads run as Container Apps with the reverse-agent bootstrap.

## What Gets Created

- **Resource Group** for all Sockerless resources
- **Virtual Network** with a subnet delegated to Container Apps
- **Network Security Group** (allows agent port 9111, outbound HTTPS)
- **Log Analytics Workspace** for monitoring and container logs
- **Container Apps Environment** linked to the subnet and Log Analytics
- **Storage Account** with Azure Files share for volume mounts
- **Azure Container Registry** for container images
- **User-Assigned Managed Identity** with RBAC roles (Contributor, ACR Pull, Storage, Monitoring)
- **Private DNS Zone** for service discovery

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.5
- [Azure CLI](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli) authenticated (`az login`)
- An Azure subscription with sufficient quota
- Go 1.24+ (to build the backend binary)

## Step 1: Apply the Terraform

```bash
cd backends/aca/examples/terraform

terraform init
terraform plan
terraform apply
```

This takes approximately 5-10 minutes (Container Apps Environment and VNet are the slowest).

To customize:

```bash
terraform apply \
  -var="location=westeurope" \
  -var="project_name=myproject" \
  -var="environment=staging"
```

## Step 2: Push Your Container Image to ACR

```bash
ACR_SERVER=$(terraform output -raw acr_login_server)

# Login to ACR
az acr login --name $(echo $ACR_SERVER | cut -d. -f1)

# Tag and push
docker tag alpine:latest ${ACR_SERVER}/alpine:latest
docker push ${ACR_SERVER}/alpine:latest
```

## Step 3: Export the Backend Configuration

```bash
# The backend_env output is sensitive, use -json
eval "$(terraform output -raw backend_env 2>/dev/null)"

# Or manually
export SOCKERLESS_ACA_SUBSCRIPTION_ID=$(az account show --query id -o tsv)
export SOCKERLESS_ACA_RESOURCE_GROUP=$(terraform output -raw resource_group_name)
export SOCKERLESS_ACA_ENVIRONMENT=$(terraform output -raw managed_environment_name)
export SOCKERLESS_ACA_LOCATION=eastus
export SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE=$(terraform output -raw log_analytics_workspace_id)
export SOCKERLESS_ACA_STORAGE_ACCOUNT=$(terraform output -raw storage_account_name)
```

Configure the reverse-agent callback. The backend fails at startup when this is empty, because exec/archive/attach operations require the in-App or in-Job bootstrap to dial back:

```bash
export SOCKERLESS_CALLBACK_URL=http://<YOUR_BACKEND_HOST>:3375
```

## Step 4: Build and Run the Backend

```bash
# Build the backend binary (serves the Docker API directly)
make backends/aca/build-noui

# Run the backend
./sockerless-backend-aca
```

## Step 5: Configure Docker to Use Sockerless

```bash
export DOCKER_HOST=tcp://localhost:3375
```

## Step 6: Use Docker Commands

### Pull and run

```bash
ACR_SERVER=$(cd backends/aca/examples/terraform && terraform output -raw acr_login_server)

docker pull ${ACR_SERVER}/alpine:latest
docker run --rm ${ACR_SERVER}/alpine:latest echo "Hello from Azure Container Apps!"
```

Behind the scenes:
1. `docker create` → Records a pending create until the ACA resource exists
2. `docker start` → ACA Job create/start for one-shot Jobs, or ACA App create/update for runner workloads
3. App-backed workloads wait for the reverse-agent bootstrap to register
4. `docker stop` → `Jobs.BeginStopExecution`
5. `docker rm` → `Jobs.BeginDelete`

### Create, exec, logs

```bash
# Create and start
docker create --name myjob ${ACR_SERVER}/alpine:latest tail -f /dev/null
docker start myjob

# Execute commands
docker exec myjob ls /
docker exec myjob cat /etc/os-release
docker exec -it myjob sh

# View logs (from Log Analytics — may have 30s+ ingestion delay)
docker logs myjob
docker logs -f myjob   # follow mode (polls every 2s)
docker logs --timestamps myjob

# Inspect
docker inspect myjob

# Stop (stops execution)
docker stop myjob

# Restart (deletes old job, creates new)
docker restart myjob

# Remove (deletes the Container Apps Job)
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

### List and prune

```bash
docker ps -a
docker container prune   # removes exited containers + deletes their ACA Jobs
```

## Step 7: Destroy the Infrastructure

```bash
cd backends/aca/examples/terraform
terraform destroy
```

**Important:** Clean up any Sockerless-managed Container Apps Jobs and Apps first:

```bash
RG=$(terraform output -raw resource_group_name)

# List jobs created by Sockerless
az containerapp job list -g $RG --query "[?tags.\"managed-by\"=='sockerless'].name" -o tsv

# Delete them
az containerapp job delete -g $RG -n sockerless-<id> --yes

# List and delete Apps created by Sockerless
az containerapp list -g $RG --query "[?tags.\"managed-by\"=='sockerless'].name" -o tsv
az containerapp delete -g $RG -n sockerless-<id> --yes
```

Then destroy the infrastructure:

```bash
terraform destroy
```

This takes approximately 5-10 minutes.

## Architecture Diagram

```
┌──────────────┐     ┌──────────────────┐     ┌──────────────────────────┐
│  docker CLI  │────▶│ Sockerless       │────▶│ Azure Container Apps     │
│              │     │ Backend           │     │                          │
│ pull, create,│     │ (localhost:3375)  │     │ Jobs.BeginCreateOrUpdate │
│ start, exec, │     │                  │     │ Jobs.BeginStart          │
│ logs, stop   │     │                  │     │ Jobs.BeginStopExecution  │
└──────────────┘     └──────────────────┘     │ Jobs.BeginDelete         │
                                               │ Apps.BeginCreateOrUpdate │
                                               │ Logs.QueryWorkspace      │
                                               └──────────────────────────┘
```

## Reverse Agent

Set `SOCKERLESS_CALLBACK_URL` to a URL reachable from ACA. The bootstrap inside the App or Job dials back to Sockerless and registers the reverse-agent WebSocket used by `docker exec`, `docker attach`, and `docker cp`. There is no management-API exec fallback.

## Key Differences from Vanilla Docker

| Feature | Vanilla Docker | ACA Backend |
|---------|---------------|-------------|
| Create | Immediate | No Azure call (stored locally) |
| Start | Starts process | Creates Job + starts Execution |
| Stop | SIGTERM → SIGKILL | StopExecution |
| Pause/unpause | Freeze cgroups | Not supported |
| Logs follow | Real-time | Polls Log Analytics every 2s |
| Logs delay | Immediate | 30s+ ingestion delay |
| Networks | Real Docker networks | ACA managed environment networking and Private DNS where configured |
| Volumes | Real volumes | Azure Files-backed volumes where configured |
| Resources | Configurable | Configured through ACA Job/App resource settings |

## Estimated Costs

- **Container Apps Environment**: Free tier available (consumption plan)
- **Container Apps Jobs**: Per-execution (vCPU-seconds + memory GiB-seconds)
- **Log Analytics**: First 5 GB/month free, then ~$2.76/GB
- **Storage Account**: ~$0.018/GB/month (LRS)
- **ACR**: Basic SKU ~$5/month
- **VNet**: Free (no gateway)

## Troubleshooting

**Job creation fails:** Ensure the Container Apps Environment is healthy. Check `az containerapp env show -g <rg> -n <env>`.

**Execution stays in provisioning:** The image must be accessible from ACR. Verify the managed identity has `AcrPull` role.

**Logs are empty or delayed:** Log Analytics has an ingestion delay of 30 seconds or more. Wait and retry. The KQL query filters by `ContainerGroupName_s`.

**Agent callback timeout:** `SOCKERLESS_CALLBACK_URL` must be reachable from ACA through the configured egress/VNet path.

**Pause not supported:** Returns `NotImplementedError`. Container Apps has no pause capability.
