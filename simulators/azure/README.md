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
├── sdk-tests/              SDK integration tests (13 tests)
├── cli-tests/              CLI integration tests (14 tests)
└── terraform-tests/        Terraform apply/destroy tests (Docker-only, TLS)
```

## Guides

- [Using with the Azure CLI](docs/cli.md)
- [Using with Terraform](docs/terraform.md)
- [Using with the Azure SDK for Python](docs/python-sdk.md)

## Execution model

Container Apps job executions honor the `replicaTimeout` configuration (in seconds). When a command is provided, the simulator executes it as a real process and streams output to Log Analytics. When a replica timeout is configured and no command is present, the execution auto-completes with `Succeeded` status after that duration. When no timeout and no command are set, the execution stays running until explicitly stopped. Azure Functions invocations are synchronous and inject AppTraces entries queryable via KQL.

## Testing

```sh
# SDK tests (uses Azure SDK for Go)
cd sdk-tests && go test -v ./...

# CLI tests (uses az CLI via `az rest`)
cd cli-tests && go test -v ./...

# Terraform tests (Docker-only, needs TLS)
cd terraform-tests && go test -v ./...
```
