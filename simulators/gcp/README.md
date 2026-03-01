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

## Quick Start

Start the simulator and try each service with curl or the Go SDK.

```bash
# Build and start the simulator
cd simulators/gcp
go build -o simulator-gcp .
SIM_LISTEN_ADDR=:4567 ./simulator-gcp
```

All examples below assume `http://localhost:4567` as the base URL.

---

### Cloud Run Jobs

Create a job, run it (creating an execution), and check status.

**Create a job:**

```bash
curl -s -X POST 'http://localhost:4567/v2/projects/my-project/locations/us-central1/jobs?jobId=hello-job' \
  -H 'Content-Type: application/json' \
  -d '{
    "template": {
      "template": {
        "timeout": "10s",
        "containers": [{"image": "alpine", "command": ["echo", "hello world"]}]
      }
    }
  }'
```

The response is a long-running operation with `"done": true` and the job in the `response` field:

```json
{
  "name": "projects/my-project/locations/us-central1/operations/...",
  "done": true,
  "response": {
    "name": "projects/my-project/locations/us-central1/jobs/hello-job",
    "uid": "...",
    "template": { "..." : "..." }
  }
}
```

**Run the job (create an execution):**

```bash
curl -s -X POST 'http://localhost:4567/v2/projects/my-project/locations/us-central1/jobs/hello-job:run' \
  -H 'Content-Type: application/json' \
  -d '{}'
```

The response LRO contains the execution. Save the execution name from `response.name` (e.g. `projects/my-project/locations/us-central1/jobs/hello-job/executions/<uuid>`).

**Check execution status:**

```bash
# Replace <exec-name> with the full execution name from the run response
curl -s http://localhost:4567/v2/<exec-name>
```

While running:

```json
{ "runningCount": 1, "succeededCount": 0, "failedCount": 0 }
```

After completion:

```json
{ "runningCount": 0, "succeededCount": 1, "completionTime": "2026-..." }
```

**Query execution logs via Cloud Logging:**

```bash
curl -s -X POST 'http://localhost:4567/v2/entries:list' \
  -H 'Content-Type: application/json' \
  -d '{
    "resourceNames": ["projects/my-project"],
    "filter": "resource.type=\"cloud_run_job\" AND resource.labels.job_name=\"hello-job\""
  }'
```

```json
{
  "entries": [
    { "textPayload": "Container started", "resource": { "type": "cloud_run_job", "labels": { "job_name": "hello-job" } } },
    { "textPayload": "hello world" },
    { "textPayload": "Execution completed successfully" }
  ]
}
```

**Go SDK (direct HTTP, since the Run v2 SDK defaults to gRPC):**

```go
import (
    "encoding/json"
    "net/http"
    "strings"
)

// Create job
job := map[string]any{
    "template": map[string]any{
        "template": map[string]any{
            "timeout": "5s",
            "containers": []map[string]any{
                {"image": "alpine", "command": []string{"echo", "hello"}},
            },
        },
    },
}
body, _ := json.Marshal(job)
req, _ := http.NewRequest("POST",
    "http://localhost:4567/v2/projects/my-project/locations/us-central1/jobs?jobId=sdk-job",
    strings.NewReader(string(body)))
req.Header.Set("Content-Type", "application/json")
resp, _ := http.DefaultClient.Do(req)
defer resp.Body.Close()
// resp.StatusCode == 200, body is LRO with done:true

// Run job
runReq, _ := http.NewRequest("POST",
    "http://localhost:4567/v2/projects/my-project/locations/us-central1/jobs/sdk-job:run",
    strings.NewReader("{}"))
runReq.Header.Set("Content-Type", "application/json")
runResp, _ := http.DefaultClient.Do(runReq)
defer runResp.Body.Close()
// Parse response.name to get the execution name for status polling
```

---

### Cloud Functions

Create a function with a command, invoke it, and check logs.

**Create a function:**

```bash
curl -s -X POST 'http://localhost:4567/v2/projects/my-project/locations/us-central1/functions?functionId=my-fn' \
  -H 'Content-Type: application/json' \
  -d '{
    "buildConfig": { "runtime": "go121", "entryPoint": "Handler" },
    "serviceConfig": { "simCommand": ["echo", "hello from function"] }
  }'
```

The `simCommand` field is simulator-specific: it defines the process to run on each invocation. The response LRO includes `serviceConfig.uri` pointing to the invoke endpoint.

```json
{
  "done": true,
  "response": {
    "name": "projects/my-project/locations/us-central1/functions/my-fn",
    "state": "ACTIVE",
    "serviceConfig": { "uri": "http://localhost:4567/v2-functions-invoke/my-fn" }
  }
}
```

**Invoke the function:**

```bash
curl -s -X POST 'http://localhost:4567/v2-functions-invoke/my-fn' \
  -H 'Content-Type: application/json' \
  -d '{}'
```

```
hello from function
```

The function's stdout is returned as the response body.

**Check logs:**

```bash
curl -s -X POST 'http://localhost:4567/v2/entries:list' \
  -H 'Content-Type: application/json' \
  -d '{
    "resourceNames": ["projects/my-project"],
    "filter": "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"my-fn\""
  }'
```

```json
{
  "entries": [
    { "textPayload": "hello from function", "resource": { "type": "cloud_run_revision", "labels": { "service_name": "my-fn" } } }
  ]
}
```

**Go SDK (direct HTTP):**

```go
// Create function
fn := map[string]any{
    "buildConfig": map[string]any{"runtime": "go121", "entryPoint": "Handler"},
    "serviceConfig": map[string]any{
        "simCommand": []string{"echo", "hi"},
    },
}
body, _ := json.Marshal(fn)
req, _ := http.NewRequest("POST",
    "http://localhost:4567/v2/projects/my-project/locations/us-central1/functions?functionId=sdk-fn",
    strings.NewReader(string(body)))
req.Header.Set("Content-Type", "application/json")
resp, _ := http.DefaultClient.Do(req)
// Parse LRO to get serviceConfig.uri

// Invoke via the returned URI
invokeResp, _ := http.Post(uri, "application/json", strings.NewReader("{}"))
// invokeResp body contains the function's stdout
```

---

### Cloud Logging

Write and list log entries using the REST API. The Go SDK uses gRPC by default (see SDK tests for gRPC examples).

**Write entries:**

```bash
curl -s -X POST 'http://localhost:4567/v2/entries:write' \
  -H 'Content-Type: application/json' \
  -d '{
    "logName": "projects/my-project/logs/my-app",
    "resource": { "type": "global" },
    "entries": [
      { "textPayload": "Server started" },
      { "textPayload": "Request received", "severity": "INFO" }
    ]
  }'
```

```json
{}
```

**List entries:**

```bash
curl -s -X POST 'http://localhost:4567/v2/entries:list' \
  -H 'Content-Type: application/json' \
  -d '{
    "resourceNames": ["projects/my-project"],
    "filter": "logName=\"projects/my-project/logs/my-app\""
  }'
```

```json
{
  "entries": [
    { "logName": "projects/my-project/logs/my-app", "textPayload": "Server started", "resource": { "type": "global" }, "timestamp": "..." },
    { "logName": "projects/my-project/logs/my-app", "textPayload": "Request received", "severity": "INFO" }
  ]
}
```

Supported filter predicates: `logName=`, `resource.type=`, `resource.labels.<key>=`, `timestamp>=`.

**Go SDK (gRPC):**

```go
import (
    "cloud.google.com/go/logging"
    "cloud.google.com/go/logging/logadmin"
    "google.golang.org/api/iterator"
    "google.golang.org/api/option"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

// Connect via gRPC (the simulator serves gRPC on a separate port, default 4568)
conn, _ := grpc.NewClient("localhost:4568", grpc.WithTransportCredentials(insecure.NewCredentials()))

// Write
writeClient, _ := logging.NewClient(ctx, "my-project", option.WithGRPCConn(conn))
writeClient.Logger("my-log").LogSync(ctx, logging.Entry{Payload: "hello"})
writeClient.Close()

// Read
readClient, _ := logadmin.NewClient(ctx, "my-project", option.WithGRPCConn(conn))
it := readClient.Entries(ctx, logadmin.Filter(`logName="projects/my-project/logs/my-log"`))
for {
    entry, err := it.Next()
    if err == iterator.Done { break }
    fmt.Println(entry.Payload) // "hello"
}
readClient.Close()
```

---

### Artifact Registry

Create a Docker-format repository.

**Create a repository:**

```bash
curl -s -X POST 'http://localhost:4567/v1/projects/my-project/locations/us-central1/repositories?repositoryId=my-repo' \
  -H 'Content-Type: application/json' \
  -d '{ "format": "DOCKER" }'
```

```json
{
  "done": true,
  "response": {
    "name": "projects/my-project/locations/us-central1/repositories/my-repo",
    "format": "DOCKER",
    "createTime": "..."
  }
}
```

**Get repository:**

```bash
curl -s http://localhost:4567/v1/projects/my-project/locations/us-central1/repositories/my-repo
```

```json
{
  "name": "projects/my-project/locations/us-central1/repositories/my-repo",
  "format": "DOCKER"
}
```

**List repositories:**

```bash
curl -s http://localhost:4567/v1/projects/my-project/locations/us-central1/repositories
```

```json
{
  "repositories": [
    { "name": "projects/my-project/locations/us-central1/repositories/my-repo", "format": "DOCKER" }
  ]
}
```

The simulator also supports OCI Distribution endpoints under `/v2/` for pushing and pulling container images.

---

### GCS (Cloud Storage)

Create a bucket, upload an object, and download it.

**Create a bucket:**

```bash
curl -s -X POST 'http://localhost:4567/storage/v1/b?project=my-project' \
  -H 'Content-Type: application/json' \
  -d '{ "name": "my-bucket" }'
```

```json
{
  "name": "my-bucket",
  "kind": "storage#bucket",
  "location": "US",
  "storageClass": "STANDARD",
  "timeCreated": "..."
}
```

**Upload an object:**

```bash
curl -s -X POST 'http://localhost:4567/upload/storage/v1/b/my-bucket/o?name=hello.txt' \
  -H 'Content-Type: text/plain' \
  -d 'hello world'
```

```json
{
  "name": "hello.txt",
  "bucket": "my-bucket",
  "size": "11",
  "contentType": "text/plain"
}
```

**Download an object (JSON API):**

```bash
curl -s http://localhost:4567/download/storage/v1/b/my-bucket/o/hello.txt
```

```
hello world
```

**List objects:**

```bash
curl -s http://localhost:4567/storage/v1/b/my-bucket/o
```

```json
{
  "kind": "storage#objects",
  "items": [
    { "name": "hello.txt", "bucket": "my-bucket", "size": "11", "contentType": "text/plain" }
  ]
}
```

**Go SDK:**

```go
import (
    "cloud.google.com/go/storage"
    "io"
    "os"
)

// Point the SDK at the simulator
os.Setenv("STORAGE_EMULATOR_HOST", "localhost:4567")
client, _ := storage.NewClient(ctx)

// Create bucket
client.Bucket("sdk-bucket").Create(ctx, "my-project", nil)

// Upload
w := client.Bucket("sdk-bucket").Object("data.txt").NewWriter(ctx)
w.Write([]byte("hello from SDK"))
w.Close()

// Download
r, _ := client.Bucket("sdk-bucket").Object("data.txt").NewReader(ctx)
data, _ := io.ReadAll(r)
r.Close()
fmt.Println(string(data)) // "hello from SDK"
```
