# simulator-azure

Local reimplementation of the Azure APIs used by the Sockerless ACA and Azure Functions backends. This is not a mock — Container Apps job executions respect `replicaTimeout` for completion, Azure Functions invoke and produce real AppTraces entries, KQL queries parse and filter against real log data, and ACR stores real OCI manifests with chunked upload support.

## Services

### Authentication & Metadata

| Service | Endpoints |
|---------|-----------|
| **OAuth2** | Token endpoint (`/{tenantId}/oauth2/v2.0/token`), OpenID discovery, JWKS |
| **Metadata** | `/metadata/endpoints` — cloud metadata (ARM endpoint, suffixes) |
| **Subscription** | Get subscription, list providers |

### Compute & Containers

| Service | Endpoints |
|---------|-----------|
| **Container App Environments** | CRUD for managed environments |
| **Container App Jobs** | CRUD, Start execution, Stop execution, List/Get executions |
| **Azure Functions (Sites)** | CRUD for function apps, List functions, Invoke (`/api/function`) |
| **App Service Plans** | CRUD (serverFarms) |
| **ACR** | Registry CRUD, Name availability, OCI Distribution (`/v2/` manifests + blobs + chunked upload) |

### Infrastructure

| Service | Endpoints |
|---------|-----------|
| **Resource Groups** | CRUD, List resources, HEAD existence check |
| **Virtual Networks** | CRUD |
| **Subnets** | CRUD (with delegations, NSG references) |
| **Network Security Groups** | CRUD (with security rules) |
| **Managed Identity** | User-assigned identity CRUD |
| **Authorization** | Role definitions (list with OData filter), Role assignments at any scope |

### Storage & Data

| Service | Endpoints |
|---------|-----------|
| **Storage Accounts** | CRUD, List keys |
| **File Shares** | CRUD under storage accounts |
| **Storage Data-Plane** | Host-based routing (`{account}.blob.localhost:{port}`) for blob/file service properties and ACLs |

### Monitoring

| Service | Endpoints |
|---------|-----------|
| **Log Analytics Workspaces** | CRUD, Shared keys |
| **Log Ingestion** | POST entries via data collection rules |
| **Log Query** | KQL query execution (simple `where`/`take` parsing) |
| **Application Insights** | Component CRUD, Billing features, Query |

### DNS

| Service | Endpoints |
|---------|-----------|
| **Private DNS Zones** | CRUD (auto-creates SOA record) |
| **A Records** | CRUD under zones |
| **Virtual Network Links** | CRUD |

## Special handling

- **Double-slash cleanup** — `CleanPathMiddleware` strips leading `//` from paths (azurerm provider appends trailing slash to ARM endpoint)
- **Case-insensitive paths** — `AzurePathNormalizationMiddleware` normalizes known segments (e.g., `/resourcegroups/` -> `/resourceGroups/`)
- **Auth outside mux** — OAuth2 token endpoints are handled as outer middleware to avoid conflicts with ACR's `/v2/` catch-all
- **TLS for Terraform** — Azure Terraform tests use self-signed certs because the azurestack provider hardcodes `https://`; Docker-only (macOS Go 1.20+ ignores `SSL_CERT_FILE`)
- **Storage subdomain routing** — Data-plane requests matched by Host header (`{account}.{service}.localhost`)
- **Sync creates return 200** — go-azure-sdk treats 200 as immediate completion for `BeginCreate` LRO

## Building

```sh
cd simulators/azure
go build -o simulator-azure .
```

## Running

```sh
# Default port 4568
./simulator-azure

# Custom port
SIM_AZURE_PORT=5002 ./simulator-azure

# With TLS (required for Terraform)
SIM_TLS_CERT=cert.pem SIM_TLS_KEY=key.pem ./simulator-azure
```

### SDK configuration

```go
cloud.Configuration{
    ActiveDirectoryAuthorityHost: "http://localhost:4568",
    Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
        cloud.ResourceManager: {Endpoint: "http://localhost:4568", Audience: "http://localhost:4568"},
    },
}
```

## Project structure

```
azure/
├── main.go                 Entry point, middleware setup, service registration
├── auth.go                 OAuth2 tokens, OpenID discovery, path cleanup (102 lines)
├── authorization.go        Role definitions + assignments (263 lines)
├── metadata.go             Cloud metadata endpoint (69 lines)
├── subscription.go         Subscription + providers (65 lines)
├── resourcegroups.go       Resource group CRUD (103 lines)
├── network.go              VNets, subnets, NSGs (336 lines)
├── managedidentity.go      User-assigned identities (102 lines)
├── containerappsenv.go     Container App Environments (139 lines)
├── containerapps.go        Container App Jobs + executions (479 lines)
├── appserviceplan.go       App Service Plans (132 lines)
├── functions.go            Function Apps + invoke (312 lines)
├── acr.go                  Container Registry + OCI Distribution (491 lines)
├── files.go                Storage accounts, file shares, data-plane (481 lines)
├── monitor.go              Log Analytics, log ingestion, KQL query (348 lines)
├── insights.go             Application Insights (169 lines)
├── dns.go                  Private DNS zones, A records, VNet links (406 lines)
├── shared/                 Shared simulator framework
├── sdk-tests/              SDK integration tests (31 tests)
├── cli-tests/              CLI integration tests (17 tests)
└── terraform-tests/        Terraform apply/destroy tests (Docker-only, TLS)
```

## Guides

- [Using with the Azure CLI](docs/cli.md)
- [Using with Terraform](docs/terraform.md)
- [Using with the Azure SDK for Python](docs/python-sdk.md)

## Execution model

Container Apps job executions honor the `replicaTimeout` configuration (in seconds). When a command is provided, the simulator executes it as a real process and streams output to Log Analytics. When a replica timeout is configured and no command is present, the execution auto-completes with `Succeeded` status after that duration. When no timeout and no command are set, the execution stays running until explicitly stopped. Azure Functions invocations are synchronous and inject AppTraces entries queryable via KQL.

## Quick start

All examples below assume the simulator is running on port 4568. Start it with:

```bash
export SIM_LISTEN_ADDR=:4568
./simulator-azure
```

Create a resource group (required by all ARM resources):

```bash
az rest --method PUT \
  --url "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg?api-version=2021-04-01" \
  --body '{"location":"eastus"}'
```

### Container Apps Jobs

Create a job with an echo command, start an execution, then query the logs.

```bash
# Create a Container Apps Job
az rest --method PUT \
  --url "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.App/jobs/my-job?api-version=2023-05-01" \
  --body '{
    "location": "eastus",
    "properties": {
      "configuration": {
        "replicaTimeout": 30,
        "triggerType": "Manual",
        "manualTriggerConfig": { "parallelism": 1, "replicaCompletionCount": 1 }
      },
      "template": {
        "containers": [{
          "name": "app",
          "image": "alpine:latest",
          "command": ["echo", "hello-from-aca"]
        }]
      }
    }
  }'
# => {"id":"/subscriptions/.../jobs/my-job","name":"my-job","properties":{"provisioningState":"Succeeded",...}}

# Start an execution
az rest --method POST \
  --url "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.App/jobs/my-job/start?api-version=2023-05-01" \
  --body '{}'
# => {"name":"my-job-abc1234","id":"..."}   (202 Accepted)

# Wait a moment for the process to finish, then check execution status
az rest --method GET \
  --url "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.App/jobs/my-job/executions?api-version=2023-05-01"
# => {"value":[{"name":"my-job-abc1234","status":"Succeeded","startTime":"...","endTime":"..."}]}

# Query Log Analytics for the execution output
az rest --method POST \
  --url "http://localhost:4568/v1/workspaces/default/query" \
  --body '{"query": "ContainerAppConsoleLogs_CL | where ContainerGroupName_s == \"my-job\""}'
# => {"tables":[{"name":"PrimaryResult","columns":[...],"rows":[["...","my-job","hello-from-aca","stdout"],...]}]}
```

Go SDK:

```go
import (
    "encoding/json"
    "net/http"
    "strings"
)

// Create job
jobBody := `{
    "location": "eastus",
    "properties": {
        "configuration": {"replicaTimeout": 30, "triggerType": "Manual"},
        "template": {"containers": [{"name": "app", "image": "alpine:latest", "command": ["echo", "hello"]}]}
    }
}`
req, _ := http.NewRequest("PUT",
    "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.App/jobs/my-job?api-version=2023-05-01",
    strings.NewReader(jobBody))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer fake-token")
resp, _ := http.DefaultClient.Do(req)
defer resp.Body.Close() // 200 OK or 201 Created

// Start execution
startReq, _ := http.NewRequest("POST",
    "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.App/jobs/my-job/start?api-version=2023-05-01",
    strings.NewReader("{}"))
startReq.Header.Set("Content-Type", "application/json")
startReq.Header.Set("Authorization", "Bearer fake-token")
startResp, _ := http.DefaultClient.Do(startReq) // 202 Accepted
defer startResp.Body.Close()

var result map[string]string
json.NewDecoder(startResp.Body).Decode(&result)
execName := result["name"] // e.g. "my-job-abc1234"

// Query logs via KQL
kqlBody := `{"query": "ContainerAppConsoleLogs_CL | where ContainerGroupName_s == \"my-job\" | take 100"}`
queryReq, _ := http.NewRequest("POST",
    "http://localhost:4568/v1/workspaces/default/query",
    strings.NewReader(kqlBody))
queryReq.Header.Set("Content-Type", "application/json")
queryResp, _ := http.DefaultClient.Do(queryReq) // 200 OK with {tables:[...]}
defer queryResp.Body.Close()
```

### Azure Functions

Create a function app with a simulated command, invoke it, and query AppTraces.

```bash
# Create an App Service Plan (consumption tier)
az rest --method PUT \
  --url "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.Web/serverfarms/my-plan?api-version=2022-09-01" \
  --body '{"location":"eastus","sku":{"name":"Y1","tier":"Dynamic"}}'
# => {"name":"my-plan","properties":{"provisioningState":"Succeeded",...}}

# Create a Function App with simCommand (simulator-only field for real execution)
az rest --method PUT \
  --url "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.Web/sites/my-func-app?api-version=2022-09-01" \
  --body '{
    "location": "eastus",
    "kind": "functionapp",
    "properties": {
      "serverFarmId": "/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.Web/serverfarms/my-plan",
      "siteConfig": {
        "simCommand": ["echo", "hello-from-functions"],
        "appSettings": [
          {"name": "FUNCTIONS_EXTENSION_VERSION", "value": "~4"},
          {"name": "FUNCTIONS_WORKER_RUNTIME", "value": "node"}
        ]
      }
    }
  }'
# => {"name":"my-func-app","properties":{"state":"Running","defaultHostName":"localhost:4568",...}}

# Invoke the function (returns process stdout as response body)
az rest --method POST \
  --url "http://localhost:4568/api/function" \
  --body '{}'
# => hello-from-functions

# Query AppTraces for function execution logs
az rest --method POST \
  --url "http://localhost:4568/v1/workspaces/default/query" \
  --body '{"query": "AppTraces | where AppRoleName == \"my-func-app\""}'
# => {"tables":[{"name":"PrimaryResult","columns":[...],"rows":[["...","hello-from-functions","my-func-app"]]}]}
```

Go SDK:

```go
// Create function app
siteBody := `{
    "location": "eastus",
    "kind": "functionapp",
    "properties": {
        "serverFarmId": "/subscriptions/.../serverfarms/my-plan",
        "siteConfig": {"simCommand": ["echo", "hello-from-functions"]}
    }
}`
req, _ := http.NewRequest("PUT",
    "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.Web/sites/my-func-app?api-version=2022-09-01",
    strings.NewReader(siteBody))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer fake-token")
resp, _ := http.DefaultClient.Do(req) // 200 OK
defer resp.Body.Close()

// Invoke the function
invokeReq, _ := http.NewRequest("POST", "http://localhost:4568/api/function", strings.NewReader("{}"))
invokeReq.Header.Set("Content-Type", "application/json")
invokeResp, _ := http.DefaultClient.Do(invokeReq) // 200 OK, body = process stdout
defer invokeResp.Body.Close()

// Query AppTraces
kqlBody := `{"query": "AppTraces | where AppRoleName == \"my-func-app\" | take 50"}`
queryReq, _ := http.NewRequest("POST",
    "http://localhost:4568/v1/workspaces/default/query",
    strings.NewReader(kqlBody))
queryReq.Header.Set("Content-Type", "application/json")
queryResp, _ := http.DefaultClient.Do(queryReq) // 200 OK
defer queryResp.Body.Close()
```

### Log Analytics

Create a workspace, ingest log entries, and query with KQL.

```bash
# Create a Log Analytics Workspace
az rest --method PUT \
  --url "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.OperationalInsights/workspaces/my-workspace?api-version=2022-10-01" \
  --body '{"location":"eastus","properties":{"retentionInDays":30}}'
# => {"name":"my-workspace","properties":{"provisioningState":"Succeeded","customerId":"<uuid>",...}}

# Get shared keys (used for linking to Container App Environments)
az rest --method POST \
  --url "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.OperationalInsights/workspaces/my-workspace/sharedKeys?api-version=2022-10-01"
# => {"primarySharedKey":"dGVzdHByaW1hcnlrZXkK","secondarySharedKey":"dGVzdHNlY29uZGFyeWtleQo="}

# Ingest log entries via data collection rule
az rest --method POST \
  --url "http://localhost:4568/dataCollectionRules/dcr-1/streams/Custom-Logs" \
  --body '[
    {"TimeGenerated":"2025-01-01T00:00:00Z","ContainerGroupName_s":"my-job","Log_s":"starting","Stream_s":"stdout"},
    {"TimeGenerated":"2025-01-01T00:01:00Z","ContainerGroupName_s":"my-job","Log_s":"running","Stream_s":"stdout"}
  ]'
# => 204 No Content

# Query with KQL (supports where, take/limit, datetime filters)
az rest --method POST \
  --url "http://localhost:4568/v1/workspaces/default/query" \
  --body '{"query": "ContainerAppConsoleLogs_CL | where ContainerGroupName_s == \"my-job\" | take 100"}'
# => {"tables":[{"name":"PrimaryResult","columns":[
#       {"name":"TimeGenerated","type":"datetime"},
#       {"name":"ContainerGroupName_s","type":"string"},
#       {"name":"Log_s","type":"string"},
#       {"name":"Stream_s","type":"string"}
#     ],"rows":[
#       ["2025-01-01T00:00:00Z","my-job","starting","stdout"],
#       ["2025-01-01T00:01:00Z","my-job","running","stdout"]
#     ]}]}
```

Go SDK:

```go
import (
    "encoding/json"
    "net/http"
    "strings"
)

// Create workspace
wsBody := `{"location":"eastus","properties":{"retentionInDays":30}}`
req, _ := http.NewRequest("PUT",
    "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.OperationalInsights/workspaces/my-workspace?api-version=2022-10-01",
    strings.NewReader(wsBody))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer fake-token")
resp, _ := http.DefaultClient.Do(req) // 200 OK
defer resp.Body.Close()

// Ingest logs
entries := []map[string]string{
    {"TimeGenerated": "2025-01-01T00:00:00Z", "ContainerGroupName_s": "my-job", "Log_s": "starting", "Stream_s": "stdout"},
}
body, _ := json.Marshal(entries)
ingestReq, _ := http.NewRequest("POST",
    "http://localhost:4568/dataCollectionRules/dcr-1/streams/Custom-Logs",
    strings.NewReader(string(body)))
ingestReq.Header.Set("Content-Type", "application/json")
ingestResp, _ := http.DefaultClient.Do(ingestReq) // 204 No Content
defer ingestResp.Body.Close()

// KQL query
kqlBody := `{"query": "ContainerAppConsoleLogs_CL | where ContainerGroupName_s == \"my-job\" | take 100"}`
queryReq, _ := http.NewRequest("POST",
    "http://localhost:4568/v1/workspaces/default/query",
    strings.NewReader(kqlBody))
queryReq.Header.Set("Content-Type", "application/json")
queryResp, _ := http.DefaultClient.Do(queryReq) // 200 OK

var result struct {
    Tables []struct {
        Name    string     `json:"name"`
        Columns []struct {
            Name string `json:"name"`
            Type string `json:"type"`
        } `json:"columns"`
        Rows [][]any `json:"rows"`
    } `json:"tables"`
}
json.NewDecoder(queryResp.Body).Decode(&result)
// result.Tables[0].Rows contains the matching log entries
```

### ACR (Container Registry)

Create a container registry.

```bash
# Create an Azure Container Registry
az rest --method PUT \
  --url "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.ContainerRegistry/registries/myregistry?api-version=2023-01-01-preview" \
  --body '{"location":"eastus","sku":{"name":"Basic"},"properties":{"adminUserEnabled":false}}'
# => {"name":"myregistry","properties":{"loginServer":"myregistry.azurecr.io","provisioningState":"Succeeded",...}}

# Verify the registry
az rest --method GET \
  --url "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.ContainerRegistry/registries/myregistry?api-version=2023-01-01-preview"
# => {"name":"myregistry","sku":{"name":"Basic"},...}

# OCI Distribution API — the registry also exposes /v2/ endpoints:
#   GET  /v2/                                    → version check
#   POST /v2/{repo}/blobs/uploads/               → initiate blob upload
#   PATCH /v2/{repo}/blobs/uploads/{uuid}        → chunked upload
#   PUT  /v2/{repo}/blobs/uploads/{uuid}?digest= → finalize blob
#   PUT  /v2/{repo}/manifests/{tag}              → push manifest
#   GET  /v2/{repo}/manifests/{ref}              → pull manifest
```

Go SDK:

```go
// Create registry
regBody := `{"location":"eastus","sku":{"name":"Basic"},"properties":{"adminUserEnabled":false}}`
req, _ := http.NewRequest("PUT",
    "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.ContainerRegistry/registries/myregistry?api-version=2023-01-01-preview",
    strings.NewReader(regBody))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer fake-token")
resp, _ := http.DefaultClient.Do(req) // 200 OK
defer resp.Body.Close()

var registry struct {
    Name       string `json:"name"`
    Properties struct {
        LoginServer       string `json:"loginServer"`
        ProvisioningState string `json:"provisioningState"`
    } `json:"properties"`
}
json.NewDecoder(resp.Body).Decode(&registry)
// registry.Properties.LoginServer == "myregistry.azurecr.io"
```

### Storage

Create a storage account.

```bash
# Create a Storage Account
az rest --method PUT \
  --url "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/mystorageacct?api-version=2023-05-01" \
  --body '{"location":"eastus","kind":"StorageV2","sku":{"name":"Standard_LRS"}}'
# => {"name":"mystorageacct","properties":{"provisioningState":"Succeeded","primaryEndpoints":{"blob":"http://mystorageacct.blob.localhost:4568/","file":"http://mystorageacct.file.localhost:4568/",...}}}

# Verify the account
az rest --method GET \
  --url "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/mystorageacct?api-version=2023-05-01"
# => {"name":"mystorageacct","kind":"StorageV2","sku":{"name":"Standard_LRS"},...}

# List account keys
az rest --method POST \
  --url "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/mystorageacct/listKeys?api-version=2023-05-01"
# => {"keys":[{"keyName":"key1","value":"dGVzdGtleTEK","permissions":"FULL"},{"keyName":"key2",...}]}
```

Go SDK:

```go
// Create storage account
acctBody := `{"location":"eastus","kind":"StorageV2","sku":{"name":"Standard_LRS"}}`
req, _ := http.NewRequest("PUT",
    "http://localhost:4568/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/mystorageacct?api-version=2023-05-01",
    strings.NewReader(acctBody))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer fake-token")
resp, _ := http.DefaultClient.Do(req) // 200 OK
defer resp.Body.Close()

var account struct {
    Name       string `json:"name"`
    Properties struct {
        ProvisioningState string `json:"provisioningState"`
        PrimaryEndpoints  struct {
            Blob string `json:"blob"`
            File string `json:"file"`
        } `json:"primaryEndpoints"`
    } `json:"properties"`
}
json.NewDecoder(resp.Body).Decode(&account)
// account.Properties.PrimaryEndpoints.Blob == "http://mystorageacct.blob.localhost:4568/"
```

## Testing

```sh
# SDK tests (uses Azure SDK for Go)
cd sdk-tests && go test -v ./...

# CLI tests (uses az CLI via `az rest`)
cd cli-tests && go test -v ./...

# Terraform tests (Docker-only, needs TLS)
cd terraform-tests && go test -v ./...
```
