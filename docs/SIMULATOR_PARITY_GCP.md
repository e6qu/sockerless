# GCP Simulator Parity

Which GCP service slices `simulators/gcp/` covers, for which surfaces,
and where the runner path relies on each one.

**Status legend:**

- ✔ **implemented** — simulator covers what sockerless uses, with SDK + CLI + terraform tests (or documented exemption).
- ◐ **partial** — some endpoints missing; not on sockerless's runner path.
- ✖ **not implemented** — sockerless uses this but simulator doesn't cover it. Tracked in BUGS.md.
- N/A — sockerless doesn't use this slice.

Current bug count: 707 total / 707 fixed / 0 open on the GCP side of Phase 86.

## Runner-path slices

| Slice | Status | File(s) | Runner usage |
|---|---|---|---|
| **Cloud Run v2 Jobs** — CreateJob, GetJob, ListJobs, DeleteJob, RunJob, ListExecutions, GetExecution, CancelExecution | ✔ | `cloudrunjobs.go` | Cloud Run backend runs every container as a one-shot Job execution. |
| **Cloud Run v2 Services** — CreateService, GetService, ListServices, DeleteService, ReplaceService; revision minting on every replace | ✔ | `cloudrun.go` | Cloud Run backend service-mode path (long-running HTTP). |
| **Cloud Run v1 (Knative-style) services** — apiVersion=serving.knative.dev/v1; generation bumping; `<name>-<5-digit-gen>` revision names | ✔ | `cloudrun.go` | Parity completeness — older clients still use v1. |
| **Cloud Functions v2** — CreateFunction, GetFunction, ListFunctions, DeleteFunction; `/v2-functions-invoke/{fn}` run path via `sim.StartContainerSync` or subprocess (SOCKERLESS_CMD / SimCommand) | ✔ | `cloudfunctions.go` | GCF backend. |
| **Artifact Registry** — CreateRepository, GetRepository, ListRepositories, DeleteRepository; Docker-format registry (OCI distribution v2) | ✔ | `artifactregistry.go` | Cloud Run + GCF backends push overlay images here. |
| **Artifact Registry pull-through / remote-repos** — remote repository mode for `docker-hub`-style proxies | ✔ | `artifactregistry.go` | Cloud Run rewrites Docker Hub refs through AR proxies. |
| **Cloud Build** (BUG-704) — CreateBuild (LRO), GetBuild, CancelBuild (via `{id}:cancel` handler), GetOperation, streaming logs; real `docker build` execution of the source tarball pulled from GCS | ✔ | `cloudbuild.go` | `docker build` via Cloud Run / GCF backends uses Cloud Build for remote builds. |
| **Secret Manager** (BUG-707) — CreateSecret, AddSecretVersion, AccessSecretVersion (incl. `latest` alias) | ✔ | `secretmanager.go` | Cloud Build `availableSecrets.secretManager` + per-step `SecretEnv` resolve to real secret values. |
| **Cloud DNS private zones** (BUG-701 GCP) — ManagedZones CRUD; RecordSets CRUD; private zones auto-back a Docker network (`sim-<zoneId>`); A-record creates connect the target container to the backing network with the record short name as alias | ✔ | `dns.go` | Cross-Job/service DNS via Docker embedded DNS. |
| **Cloud Logging** — WriteLogEntries (HTTP + gRPC transport); ListLogEntries; structured severity + labels; per-function injection helpers | ✔ | `logging.go` | `docker logs` against Cloud Run / GCF reads here. |
| **Cloud Storage** — CreateBucket, PutObject, GetObject, DeleteObject, ListBuckets, HeadBucket; resumable + multipart uploads | ✔ | `gcs.go` | Cloud Build source tarball, terraform state, artifact storage. |
| **Compute (IAM / networking minimal)** — DescribeNetworks, GetProject default SA | ✔ | `compute.go` | Cloud Run service account resolution. |
| **VPC Access connectors** — CreateConnector, GetConnector, ListConnectors, DeleteConnector | ✔ | `vpcaccess.go` | Cloud Run VPC egress (serverless-to-VPC). |
| **IAM** — GetServiceAccount, CreateServiceAccount, ListServiceAccounts, GetIamPolicy, SetIamPolicy | ✔ | `iam.go` | Workload identity / service-account resolution. |
| **Service Usage** — EnableService, ListServices | ✔ | `serviceusage.go` | Terraform's `google_project_service`. |
| **Long-running Operations** — `/v1/operations/{id}` polling for LROs across Cloud Build, Cloud Run, Cloud Functions, Artifact Registry | ✔ | `operations.go` | LRO polling after Begin* methods. |

## Out-of-scope (N/A) slices

Sockerless doesn't touch these, so the simulator doesn't implement them:

- BigQuery, Pub/Sub, Cloud Spanner, Firestore, Cloud SQL, Cloud KMS, Cloud Memorystore, GKE (cloud-native container path uses Cloud Run/GCF), Cloud Endpoints, Cloud CDN, Cloud Monitoring (beyond minimal metrics).

## Exit check

No ✖ rows. All runner-path slices are implemented with SDK + CLI + terraform tests (or documented exemption via `simulators/gcp/tests-exempt.txt`).
