# GCP runbook — Cloud Run Jobs/Services + Cloud Run Functions

**Status:** placeholder. Live-GCP track is queued in [PLAN.md](../PLAN.md). The terraform live env under `terraform/environments/cloudrun/live/` and `cloudrun-functions/live/` still needs to be added; once it's in place the runbook below gets fleshed out track-by-track.

The shape mirrors [02-aws-runbook.md](02-aws-runbook.md) — same docker / podman CLI surface, same track structure (A core, B podman, C advanced, D function-specific, E peer comms, F pods, G compose, H podman compose, I stateless, J runner integration). The cloud-API parity (every SDK call sockerless makes against Cloud Run / Cloud Run Functions / Cloud Logging / Cloud DNS) is already exercised against the GCP simulator under `simulators/gcp/{sdk-tests,cli-tests,terraform-tests}/`; the live runbook is to verify the same surface against real GCP.

## Prerequisites (when ready)

- GCP project with billing enabled.
- VPC connector for Cloud Run Services peer reachability (required when `SOCKERLESS_GCR_USE_SERVICE=1`).
- Service account with Cloud Run admin + Cloud Logging viewer + Artifact Registry write.
- `gcloud` CLI authenticated.

## Cross-links

- Sim coverage: [specs/SIM_PARITY_MATRIX.md](../specs/SIM_PARITY_MATRIX.md) § GCP — 16/16 cloud-API rows ✓
- Backend code: `backends/cloudrun/`, `backends/cloudrun-functions/`
- Sim handlers: `simulators/gcp/cloudrunjobs.go`, `simulators/gcp/cloudrunservices.go`, `simulators/gcp/cloudfunctions.go`
