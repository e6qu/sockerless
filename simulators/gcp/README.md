# simulator-gcp

Local reimplementation of the GCP APIs used by the Sockerless Cloud Run and Cloud Functions backends. This is not a mock — Cloud Run job executions respect the task template `timeout` for completion, Cloud Functions invoke and produce real log entries, Cloud Logging entries are written and queryable with the standard filter syntax, and Artifact Registry stores real OCI manifests.

## Services

All services use REST/JSON routing with Go 1.22+ path patterns.

| Service | Base Path | Endpoints |
|---------|-----------|-----------|
| **Cloud Run Jobs** | `/v2/projects/.../jobs` | Create, Get, List, Delete, Run (create execution), Get/List/Cancel Executions |
| **Cloud Functions v2** | `/v2/projects/.../functions` | Create, Get, List, Delete, Invoke |
| **Cloud DNS** | `/dns/v1/projects/...` | Managed Zones (CRUD), Record Sets (CRUD) |
| **GCS** | `/storage/v1/b/...` | Buckets (CRUD, list), Objects (upload, download, list, delete) — JSON + XML APIs |
| **Artifact Registry** | `/v1/projects/.../repositories` | Repositories (CRUD), Docker Images (list), OCI Distribution (`/v2/` manifests + blobs) |
| **Cloud Logging** | `/v2/entries` | Write entries, List entries (with filter) |
| **Compute Engine** | `/compute/v1/projects/...` | Networks (CRUD), Subnetworks (CRUD), Operations |
| **IAM** | `/v1/projects/.../serviceAccounts` | Service Accounts (CRUD), IAM Policies (get/set at any resource scope) |
| **VPC Access** | `/v1/projects/.../connectors` | Connectors (CRUD) |
| **Service Usage** | `/v1/projects/.../services` | Enable, Disable, Get, List, Batch Enable |
| **Operations** | `/v{1,2}/projects/.../operations` | Get (returns immediate DONE) |

### Long-running operations

Create/delete operations return an LRO wrapper with the resource in the `response` field and `done: true`. This satisfies both SDK and Terraform clients that poll for completion.

## Building

```sh
cd simulators/gcp
go build -o simulator-gcp .
```

## Running

```sh
# Default port 4567
./simulator-gcp

# Custom port
SIM_GCP_PORT=5001 ./simulator-gcp
```

### SDK configuration

```go
option.WithEndpoint("http://localhost:4567")
option.WithoutAuthentication()
```

Or via environment:
```sh
export STORAGE_EMULATOR_HOST=localhost:4567
```

## Project structure

```
gcp/
├── main.go                 Entry point, service registration
├── cloudrunjobs.go         Cloud Run Jobs + Executions (505 lines)
├── cloudfunctions.go       Cloud Functions v2 (226 lines)
├── dns.go                  Cloud DNS zones + record sets (211 lines)
├── gcs.go                  GCS buckets + objects, multipart upload (380 lines)
├── artifactregistry.go     Artifact Registry + OCI Distribution (445 lines)
├── logging.go              Cloud Logging entries (173 lines)
├── compute.go              Networks + subnetworks (247 lines)
├── iam.go                  Service accounts + IAM policies (257 lines)
├── vpcaccess.go            VPC Access connectors (108 lines)
├── serviceusage.go         Service enable/disable (116 lines)
├── operations.go           LRO status (49 lines)
├── shared/                 Shared simulator framework
├── sdk-tests/              SDK integration tests (20 tests)
├── cli-tests/              CLI integration tests (15 tests)
└── terraform-tests/        Terraform apply/destroy tests
```

## Guides

- [Using with the gcloud CLI](docs/cli.md)
- [Using with Terraform](docs/terraform.md)
- [Using with Google Cloud Python libraries](docs/python-sdk.md)

## Execution model

Cloud Run job executions honor the task template `timeout` field (e.g., `"600s"`). When a timeout is configured, the execution auto-completes after that duration. When a command is provided, the simulator executes it as a real process and streams output to Cloud Logging. When no command and no timeout are set, the execution stays running until explicitly cancelled. Cloud Functions invocations are synchronous and return immediately.

## Testing

```sh
# SDK tests (uses GCP Go SDK + direct HTTP)
cd sdk-tests && go test -v ./...

# CLI tests (uses gcloud CLI)
cd cli-tests && go test -v ./...

# Terraform tests (uses google provider)
cd terraform-tests && go test -v ./...
```
