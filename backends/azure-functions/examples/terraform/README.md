# Azure Functions Backend — Terraform Example

This example provisions the Azure infrastructure needed to run Sockerless with the Azure Functions backend. Once applied, you can use standard `docker` CLI commands and they will execute as Function App invocations.

## What Gets Created

- **Resource Group** for all Sockerless resources
- **Storage Account** (required by Azure Functions runtime)
- **App Service Plan** (Linux consumption Y1, or configurable SKU)
- **Azure Container Registry** for container images
- **User-Assigned Managed Identity** with RBAC roles (ACR Pull, Storage, Monitoring)
- **Log Analytics Workspace** for monitoring
- **Application Insights** connected to Log Analytics

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.5
- [Azure CLI](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli) authenticated (`az login`)
- An Azure subscription
- Go 1.24+ (to build the backend binary)

## Step 1: Apply the Terraform

```bash
cd backends/azure-functions/examples/terraform

terraform init
terraform plan
terraform apply
```

This takes approximately 3-5 minutes.

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

az acr login --name $(echo $ACR_SERVER | cut -d. -f1)

docker tag alpine:latest ${ACR_SERVER}/alpine:latest
docker push ${ACR_SERVER}/alpine:latest
```

## Step 3: Export the Backend Configuration

```bash
# The backend_env output is sensitive, use eval
eval "$(terraform output -raw backend_env 2>/dev/null)"

# Or manually
export SOCKERLESS_AZF_SUBSCRIPTION_ID=$(az account show --query id -o tsv)
export SOCKERLESS_AZF_RESOURCE_GROUP=$(terraform output -raw resource_group_name)
export SOCKERLESS_AZF_LOCATION=eastus
export SOCKERLESS_AZF_STORAGE_ACCOUNT=$(terraform output -raw storage_account_name)
export SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE=$(terraform output -raw log_analytics_workspace_id)
export SOCKERLESS_CALLBACK_URL=http://<YOUR_BACKEND_HOST>:9100
```

**Important:** `SOCKERLESS_CALLBACK_URL` is required. Azure Functions uses reverse agent mode exclusively — functions cannot accept arbitrary inbound connections. Replace `<YOUR_BACKEND_HOST>` with a publicly reachable address.

## Step 4: Build and Run the Backend

```bash
cd backends/azure-functions
go build -o sockerless-backend-azf ./cmd/sockerless-backend-azf
./sockerless-backend-azf -addr :9100
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
ACR_SERVER=$(cd backends/azure-functions/examples/terraform && terraform output -raw acr_login_server)

docker pull ${ACR_SERVER}/alpine:latest
docker run --rm ${ACR_SERVER}/alpine:latest echo "Hello from Azure Functions!"
```

Behind the scenes:
1. `docker create` → `WebApps.BeginCreateOrUpdate` (Function App with `DOCKER|{image}`)
2. `docker start` → HTTP POST to function URL
3. Agent calls back to `SOCKERLESS_CALLBACK_URL`
4. `docker rm` → `WebApps.Delete`

**Note:** Container creation takes 30-60 seconds because Azure provisions the Function App.

### Create, exec, logs

```bash
# Create (provisions Function App — takes 30-60s)
docker create --name myfunc ${ACR_SERVER}/alpine:latest tail -f /dev/null

# Start (invokes the function, agent calls back)
docker start myfunc

# Execute commands via reverse agent
docker exec myfunc ls /
docker exec myfunc cat /etc/os-release

# View logs (from Application Insights / Log Analytics)
docker logs myfunc

# Inspect
docker inspect myfunc

# Remove (deletes Function App)
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
cd backends/azure-functions/examples/terraform
terraform destroy
```

**Important:** Delete any Function Apps created by Sockerless first:

```bash
RG=$(terraform output -raw resource_group_name)

# List Function Apps created by Sockerless (named skls-*)
az functionapp list -g $RG --query "[?starts_with(name, 'skls-')].name" -o tsv

# Delete them
az functionapp delete -g $RG -n skls-<id>
```

Then destroy:

```bash
terraform destroy
```

## Architecture Diagram

```
┌──────────────┐     ┌──────────────────┐     ┌────────────────────────┐
│  docker CLI  │────▶│ Sockerless       │────▶│ Azure Functions        │
│              │     │ Frontend + Backend│     │                        │
│ pull, create,│     │ (localhost:9100)  │     │ WebApps.CreateOrUpdate │
│ start, exec, │     │                  │◀────│ HTTP POST invoke       │
│ logs, rm     │     │ ◀── agent calls  │     │ WebApps.Delete         │
└──────────────┘     │     back here    │     │ Logs.QueryWorkspace    │
                     └──────────────────┘     └────────────────────────┘
```

## Key Differences from Vanilla Docker

| Feature | Vanilla Docker | AZF Backend |
|---------|---------------|-------------|
| Create | Instant | 30-60s (Function App provisioning) |
| Stop | SIGTERM → SIGKILL | No-op (runs to completion) |
| Kill | Sends signal | Disconnects agent only |
| Logs follow | Real-time | Single snapshot |
| Restart | Stop + start | No-op |
| Networks | Docker networks | Not supported |
| Volumes | Docker volumes | Not supported |
| Exec | nsenter | Reverse agent relay |
| Port bindings | Host port mapping | Not supported (HTTP invoke) |

## Estimated Costs

- **Azure Functions (Consumption)**: First 1M executions/month free, then $0.20/1M
- **App Service Plan (Y1)**: Consumption-based, pay per execution
- **Storage Account**: ~$0.018/GB/month (LRS)
- **ACR**: Basic SKU ~$5/month
- **Log Analytics**: First 5 GB/month free, then ~$2.76/GB
- **Application Insights**: First 5 GB/month free

Very cost-effective for sporadic usage. No idle costs with consumption plan.

## Troubleshooting

**Function App creation fails:** Ensure the subscription has the `Microsoft.Web` resource provider registered. Run `az provider register --namespace Microsoft.Web`.

**Agent callback timeout:** The `SOCKERLESS_CALLBACK_URL` must be reachable from the Function App. If running locally, you may need a tunnel (e.g., ngrok) or a public IP.

**Logs are empty:** Application Insights has an ingestion delay. The KQL query uses `AppTraces | where AppRoleName == "{functionAppName}"`. Wait 30-60 seconds.

**Image not found:** Ensure the image is pushed to ACR and the `DOCKER_REGISTRY_SERVER_URL` app setting points to the correct registry.
