# simulator-gcp

Local reimplementation of the GCP slice that sockerless touches. Not a mock — Cloud Run job executions respect the task template `timeout` for completion, Cloud Functions invoke and produce real log entries, Cloud Logging entries are written and queryable with the standard filter syntax, and Artifact Registry stores real OCI manifests.

## Reference adaptor

The simulator exposes one HTTP endpoint (default `:4567`) that fronts all GCP services. Three external tools exercise that endpoint at GCP-API fidelity:

| Adaptor | Min version | What it proves |
|---|---|---|
| [GCP Go SDK](https://pkg.go.dev/cloud.google.com/go) (`cloud.google.com/go/{run,functions,storage,logging,...}`) | latest | Wire-level SDK compatibility — request/response shapes, error envelopes, pagination, long-running operation polling. |
| [`gcloud` CLI](https://cloud.google.com/sdk/docs/install) | 480+ | Endpoint-override fidelity (`gcloud --api-endpoint-overrides`). The CLI uses the same SDK but exercises a different argument-marshaling path. |
| [Terraform `google` provider](https://registry.terraform.io/providers/hashicorp/google/latest/docs) | v6+ | Full plan → apply → destroy round-trip across `google_cloud_run_v2_job`, `google_cloudfunctions2_function`, `google_storage_bucket`, `google_artifact_registry_repository`, `google_dns_managed_zone`, `google_compute_network`, `google_service_account`, etc. |

Anything any of these three tools does against the real GCP endpoint, it must do against this simulator. Gaps from that contract are real bugs (see [BUGS.md](../../BUGS.md)).

The simulator is the **upstream** for the [Cloud Run](../../backends/cloudrun/README.md) and [Cloud Run Functions](../../backends/cloudrun-functions/README.md) backends during local development and CI.

## Validation

| Test path | What runs | Last green |
|---|---|---|
| `sdk-tests/` | Real `cloud.google.com/go/*` clients against the sim. Per-op assertions on response shape + error codes, including Artifact Registry remote-repository and OCI paths. | 2026-05-18 |
| `cli-tests/` | Real `gcloud` CLI invoked via `os/exec`, parses CLI JSON output, including Artifact Registry endpoint overrides. | 2026-05-18 |
| `terraform-tests/` | Real Terraform `google` provider against the sim. `terraform apply` → assert resource state → `destroy`. | 2026-05-13 |
| `make simulators/gcp/test` | Leaf-Makefile unit + integration suite per [`docs/MAKEFILE_STANDARD.md`](../../docs/MAKEFILE_STANDARD.md). | 2026-05-13 |

CI runs all four on every PR (`.github/workflows/ci.yml`).

## Wiring the adaptor

```bash
# 1. Build + start the sim (default :4567).
cd simulators/gcp
go build -o simulator-gcp .
SIM_LISTEN_ADDR=:4567 ./simulator-gcp
```

```bash
# 2. Point any GCP client at it.
export CLOUDSDK_API_ENDPOINT_OVERRIDES_RUN=http://localhost:4567/
export CLOUDSDK_API_ENDPOINT_OVERRIDES_CLOUDFUNCTIONS=http://localhost:4567/
export CLOUDSDK_API_ENDPOINT_OVERRIDES_ARTIFACTREGISTRY=http://localhost:4567/
export STORAGE_EMULATOR_HOST=localhost:4567
gcloud auth application-default login --no-launch-browser  # or use ADC

gcloud run jobs list --region us-central1
gcloud storage buckets list
```

For Terraform:

```hcl
provider "google" {
  project = "my-project"
  region  = "us-central1"

  endpoints = {
    cloud_run_v2     = "http://localhost:4567/"
    cloudfunctions2  = "http://localhost:4567/"
    artifact_registry = "http://localhost:4567/"
    storage          = "http://localhost:4567/"
    # …any service you exercise.
  }
}
```

## Services

All services use REST/JSON routing with Go 1.22+ path patterns. Long-running operations return an LRO wrapper with `done: true` and the resource in `response`.

| Service | Base Path | Endpoints |
|---|---|---|
| **Cloud Run Jobs** | `/v2/projects/.../jobs` | Create, Get, List, Delete, Run (create execution), Get/List/Cancel Executions |
| **Cloud Functions v2** | `/v2/projects/.../functions` | Create, Get, List, Delete, Invoke |
| **Cloud DNS** | `/dns/v1/projects/...` | Managed Zones (CRUD), Record Sets (CRUD) |
| **GCS** | `/storage/v1/b/...` | Buckets (CRUD, list), Objects (upload, download, list, delete) — JSON + XML APIs |
| **Artifact Registry** | `/v1/projects/.../repositories` | Repositories (CRUD), Docker Images (list), [OCI Distribution](https://github.com/opencontainers/distribution-spec) (`/v2/` manifests + blobs) |
| **Cloud Logging** | `/v2/entries` | Write entries, List entries (with filter) |
| **Compute Engine** | `/compute/v1/projects/...` | Networks (CRUD), Subnetworks (CRUD), Operations |
| **IAM** | `/v1/projects/.../serviceAccounts` | Service Accounts (CRUD), IAM Policies (get/set at any resource scope) |
| **VPC Access** | `/v1/projects/.../connectors` | Connectors (CRUD) |
| **Service Usage** | `/v1/projects/.../services` | Enable, Disable, Get, List, Batch Enable |
| **Operations** | `/v{1,2}/projects/.../operations` | Get (returns immediate DONE) |

## Building

```bash
cd simulators/gcp && go build -o simulator-gcp .
```

## Sample

End-to-end via `gcloud` + `curl`:

```bash
$ SIM_LISTEN_ADDR=:4567 ./simulator-gcp &
$ export CLOUDSDK_API_ENDPOINT_OVERRIDES_RUN=http://localhost:4567/

# Create a job
$ curl -s -X POST 'http://localhost:4567/v2/projects/my-project/locations/us-central1/jobs?jobId=hello-job' \
    -H 'Content-Type: application/json' \
    -d '{"template":{"template":{"timeout":"10s","containers":[{"image":"alpine","command":["echo","hello"]}]}}}'
{"done": true, "response": {"name":"projects/my-project/locations/us-central1/jobs/hello-job",...}}

# Run it
$ curl -s -X POST 'http://localhost:4567/v2/projects/my-project/locations/us-central1/jobs/hello-job:run' -d '{}'

# Check via gcloud
$ gcloud run jobs executions list --job hello-job --region us-central1
NAME             STATUS     COMPLETION_TIME
hello-job-...    Succeeded  2026-...
```

More inline examples (Cloud Run Jobs / Cloud Functions / Cloud Logging / Artifact Registry / GCS) live below; the full per-verb wire shape is captured by the `sdk-tests/` package.

## Project structure

```
gcp/
├── main.go                 Entry point, service registration
├── cloudrunjobs.go         Cloud Run Jobs + Executions
├── cloudfunctions.go       Cloud Functions v2
├── dns.go                  Cloud DNS zones + record sets
├── gcs.go                  GCS buckets + objects, multipart upload
├── artifactregistry.go     Artifact Registry + OCI Distribution
├── logging.go              Cloud Logging entries
├── compute.go              Networks + subnetworks
├── iam.go                  Service accounts + IAM policies
├── vpcaccess.go            VPC Access connectors
├── serviceusage.go         Service enable/disable
├── operations.go           LRO status
├── shared/                 Shared simulator framework
├── sdk-tests/              SDK integration tests
├── cli-tests/              CLI integration tests
└── terraform-tests/        Terraform apply/destroy tests
```

## Testing

```bash
# SDK tests (cloud.google.com/go clients against the running sim)
cd sdk-tests && go test -v ./...

# CLI tests (gcloud CLI shell-outs)
cd cli-tests && go test -v ./...

# Terraform tests (real terraform apply against the sim)
cd terraform-tests && go test -v ./...
```

Each test package's `TestMain` builds the simulator binary, finds a free port, boots the sim, waits for `/health`, runs the suite, then kills the sim. No external services needed.

## Execution model

Cloud Run job executions honor the task template `timeout` field (e.g., `"600s"`). When a timeout is configured, the execution auto-completes after that duration. When a command is provided, the simulator executes it as a real process and streams output to Cloud Logging. When no command and no timeout are set, the execution stays running until explicitly cancelled. Cloud Functions invocations are synchronous and return immediately.

## Known issues

None open. The Cloud Run `BackingPDEphemeral` rejection (Phase 91d bookmark) is enforced at the [`backends/cloudrun`](../../backends/cloudrun/README.md) layer, not the simulator — Cloud Run lacks the protobuf field, so no amount of simulator work changes that.

## What's out of scope

- **gRPC parity**: Cloud Logging's recommended path is gRPC; the sim exposes a gRPC port (default `:4568`) but does not serve every gRPC method. REST + JSON is the canonical surface.
- **DNS resolution at UDP/53**: Cloud DNS stores records but does not serve them via UDP. Pair with dnsmasq for actual lookups.
- **Real authentication**: Bearer tokens are accepted but not cryptographically verified.
- **Multi-region**: sim is single-region.
- **Billing / pricing / quota surfaces**: absent.

## Extended examples

(Quick start with full curl + Go SDK + gcloud + Terraform snippets per service.)

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

The response is a long-running operation with `"done": true` and the job in the `response` field.

**Run the job:**

```bash
curl -s -X POST 'http://localhost:4567/v2/projects/my-project/locations/us-central1/jobs/hello-job:run' \
  -H 'Content-Type: application/json' -d '{}'
```

**Check execution status:**

```bash
curl -s http://localhost:4567/v2/<exec-name>
# Running:  {"runningCount": 1, "succeededCount": 0, "failedCount": 0}
# Done:     {"runningCount": 0, "succeededCount": 1, "completionTime": "2026-..."}
```

**Query execution logs via Cloud Logging:**

```bash
curl -s -X POST 'http://localhost:4567/v2/entries:list' \
  -H 'Content-Type: application/json' \
  -d '{"resourceNames":["projects/my-project"],"filter":"resource.type=\"cloud_run_job\" AND resource.labels.job_name=\"hello-job\""}'
```

**Go SDK (direct HTTP, since the Run v2 SDK defaults to gRPC):**

```go
import (
    "encoding/json"
    "net/http"
    "strings"
)

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
http.DefaultClient.Do(req)

runReq, _ := http.NewRequest("POST",
    "http://localhost:4567/v2/projects/my-project/locations/us-central1/jobs/sdk-job:run",
    strings.NewReader("{}"))
runReq.Header.Set("Content-Type", "application/json")
http.DefaultClient.Do(runReq)
```

### Cloud Functions

Create a function with a command, invoke it, and check logs.

```bash
curl -s -X POST 'http://localhost:4567/v2/projects/my-project/locations/us-central1/functions?functionId=my-fn' \
  -H 'Content-Type: application/json' \
  -d '{"buildConfig":{"runtime":"go121","entryPoint":"Handler"},
       "serviceConfig":{"simCommand":["echo","hello from function"]}}'
# The simCommand field is simulator-specific. The LRO response includes
# serviceConfig.uri pointing to the invoke endpoint.

curl -s -X POST 'http://localhost:4567/v2-functions-invoke/my-fn' -d '{}'
# => hello from function
```

### Cloud Logging

Write and list log entries using the REST API (the Go SDK uses gRPC by default).

```bash
curl -s -X POST 'http://localhost:4567/v2/entries:write' \
  -H 'Content-Type: application/json' \
  -d '{"logName":"projects/my-project/logs/my-app",
       "resource":{"type":"global"},
       "entries":[{"textPayload":"Server started"}]}'

curl -s -X POST 'http://localhost:4567/v2/entries:list' \
  -H 'Content-Type: application/json' \
  -d '{"resourceNames":["projects/my-project"],
       "filter":"logName=\"projects/my-project/logs/my-app\""}'
```

Supported filter predicates: `logName=`, `resource.type=`, `resource.labels.<key>=`, `timestamp>=`.

### GCS (Cloud Storage)

```bash
export STORAGE_EMULATOR_HOST=localhost:4567

curl -s -X POST 'http://localhost:4567/storage/v1/b?project=my-project' -d '{"name":"my-bucket"}'
curl -s -X POST 'http://localhost:4567/upload/storage/v1/b/my-bucket/o?name=hello.txt' \
  -H 'Content-Type: text/plain' -d 'hello world'
curl -s http://localhost:4567/download/storage/v1/b/my-bucket/o/hello.txt
# => hello world
```

Go SDK respects `STORAGE_EMULATOR_HOST`:

```go
os.Setenv("STORAGE_EMULATOR_HOST", "localhost:4567")
client, _ := storage.NewClient(ctx)
client.Bucket("sdk-bucket").Create(ctx, "my-project", nil)
```

### Artifact Registry

```bash
# Create a Docker-format repository
curl -s -X POST 'http://localhost:4567/v1/projects/my-project/locations/us-central1/repositories?repositoryId=my-repo' \
  -H 'Content-Type: application/json' \
  -d '{"format":"DOCKER"}'
```

The simulator also supports OCI Distribution endpoints under `/v2/` for pushing and pulling container images per the [OCI Distribution spec](https://github.com/opencontainers/distribution-spec).

See also: [`backends/cloudrun/README.md`](../../backends/cloudrun/README.md), [`backends/cloudrun-functions/README.md`](../../backends/cloudrun-functions/README.md), [`specs/CLOUD_RESOURCE_MAPPING.md § GCP`](../../specs/CLOUD_RESOURCE_MAPPING.md).
