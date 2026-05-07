# GCP runbook — Cloud Run Jobs/Services + Cloud Run Functions

**Status:** placeholder. Live-GCP track is queued in [PLAN.md](../PLAN.md). The terraform live env under `terraform/environments/cloudrun/live/` and `cloudrun-functions/live/` still needs to be added; once it's in place the runbook below gets fleshed out track-by-track.

The shape mirrors [02-aws-runbook.md](02-aws-runbook.md) — same docker / podman CLI surface, same track structure (A core, B podman, C advanced, D function-specific, E peer comms, F pods, G compose, H podman compose, I stateless, J runner integration). The cloud-API parity (every SDK call sockerless makes against Cloud Run / Cloud Run Functions / Cloud Logging / Cloud DNS) is already exercised against the GCP simulator under `simulators/gcp/{sdk-tests,cli-tests,terraform-tests}/`; the live runbook is to verify the same surface against real GCP.

## Prerequisites

Per-session ephemeral project workflow (saves cost; keeps state hygiene clean):

```bash
PROJECT=sockerless-live-$(LC_ALL=C tr -dc 'a-z0-9' </dev/urandom | head -c 10)
gcloud projects create "$PROJECT" --name="Sockerless Live"
gcloud billing projects link "$PROJECT" --billing-account=<your-billing-acct-id>
gcloud config set project "$PROJECT"
gcloud auth application-default login   # browser flow, populates ADC
gcloud services enable \
  run.googleapis.com cloudfunctions.googleapis.com cloudbuild.googleapis.com \
  artifactregistry.googleapis.com logging.googleapis.com storage.googleapis.com \
  iam.googleapis.com iamcredentials.googleapis.com cloudresourcemanager.googleapis.com \
  serviceusage.googleapis.com eventarc.googleapis.com --project="$PROJECT"
gcloud artifacts repositories create docker-hub --repository-format=docker \
  --location=us-central1 --mode=remote-repository \
  --remote-docker-repo=DOCKER-HUB --project="$PROJECT"
gcloud artifacts repositories create sockerless-overlay --repository-format=docker \
  --location=us-central1 --project="$PROJECT"
gcloud storage buckets create gs://${PROJECT}-build --location=us-central1 \
  --uniform-bucket-level-access --project="$PROJECT"
```

Service account for the gcf backend (gcf needs to invoke Cloud Run Services with an authenticated ID token; user-credential ADC can't sign these — only service-account creds can):

```bash
SA=sockerless-runner@${PROJECT}.iam.gserviceaccount.com
gcloud iam service-accounts create sockerless-runner --display-name="Sockerless Runner" --project="$PROJECT"
for role in roles/run.admin roles/run.invoker roles/cloudfunctions.developer \
            roles/iam.serviceAccountUser roles/logging.viewer roles/storage.admin \
            roles/artifactregistry.writer roles/cloudbuild.builds.editor; do
  gcloud projects add-iam-policy-binding "$PROJECT" --member="serviceAccount:${SA}" \
    --role="$role" --condition=None --quiet
done
gcloud storage buckets add-iam-policy-binding gs://${PROJECT}-build \
  --member="serviceAccount:${SA}" --role="roles/storage.objectAdmin"
gcloud iam service-accounts keys create /tmp/${PROJECT}-key.json \
  --iam-account="$SA" --project="$PROJECT"
export GOOGLE_APPLICATION_CREDENTIALS=/tmp/${PROJECT}-key.json
```

Why a service-account JSON key over user-credential ADC: `google.golang.org/api/idtoken` (which sockerless uses to authenticate Cloud Run Functions invocations) refuses to sign with user creds — only service-account creds work. Sockerless's gcf backend fails loudly with a clear actionable error if it sees user-credential ADC. `gcloud auth login --impersonate-service-account=<sa>` is the equivalent without a JSON key on disk if you'd rather avoid the file.

Teardown (deletes everything inside in one shot):

```bash
gcloud projects delete "$PROJECT"
```

## Manual test sweep

Once both backends are running (cloudrun on `127.0.0.1:3375`, gcf on `127.0.0.1:3376`):

```bash
DOCKER_HOST=tcp://127.0.0.1:3375 PROBE_TIMEOUT=600 \
  ./scripts/manual-test-real-workloads.sh cloudrun

DOCKER_HOST=tcp://127.0.0.1:3376 PROBE_TIMEOUT=600 \
  ./scripts/manual-test-real-workloads.sh gcf
```

Bundles probed (each is one `docker run`):

- **bundle-O**: 11 OS / kernel / capability probes (uname, /etc/os-release, /proc/cpuinfo, /proc/meminfo, mount, id, hostname, ulimit, ps).
- **bundle-E**: env-var passthrough (`-e FOOBAR=baz -e ANOTHER=qux`).
- **bundle-N**: DNS (`getent hosts google.com`) + outbound HTTPS (`wget --spider`).
- **bundle-W**: real workload — `go run` of an inline arithmetic-evaluator program inside `golang:1.22-alpine` (validates the backend can both pull a multi-hundred-MB image AND exec a build-then-run inside it).

Per-row PASS/FAIL is reported; per-bundle logs land in `/tmp/sockerless-real-workloads/<label>/`. Cold-start latency on the FaaS backends (gcf) is dominated by per-`docker run` Cloud Build + CreateFunction + UpdateService swap (~3-5 min on first contact with a new image content-hash; subsequent invocations of the same image hit the pool/cache).

## Cross-links

- Sim coverage: [specs/SIM_PARITY_MATRIX.md](../specs/SIM_PARITY_MATRIX.md) § GCP — 16/16 cloud-API rows ✓
- Backend code: `backends/cloudrun/`, `backends/cloudrun-functions/`
- Sim handlers: `simulators/gcp/cloudrunjobs.go`, `simulators/gcp/cloudrunservices.go`, `simulators/gcp/cloudfunctions.go`
- Cloud-resource mapping: [specs/CLOUD_RESOURCE_MAPPING.md § GCP Cloud Run](../specs/CLOUD_RESOURCE_MAPPING.md#gcp-cloud-run-backend-cloudrun) and [§ GCP Cloud Run Functions](../specs/CLOUD_RESOURCE_MAPPING.md#gcp-cloud-run-functions-backend-cloudrun-functions--gcf).
