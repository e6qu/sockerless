# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current State

- Branch: `main`, synced with `origin/main` after the SDK/CLI HTTPS gateway and GCS CLI audit PR merged.
- Active implementation branch: none.
- Open GitHub issues at last check: none.
- Open BUG trackers: BUG-1075 and BUG-1104.
- Last completed work: SDK/CLI HTTPS gateway guidance was documented, and BUG-1104 found/fixed stale GCP GCS CLI coverage.

## Next Task

Add optional AWS/GCP Terraform HTTPS gateway examples, preserving the existing direct HTTP Terraform paths.

Azure Terraform was the hard proof point because AzureRM requires trusted HTTPS for custom metadata discovery. It now starts Caddy in the Linux test harness, trusts the Caddy local CA with `SSL_CERT_FILE`, and uses `https://azure.sockerless.localhost:<port>` for `metadata_host` and ARM endpoint flows.

## Provider Facts To Preserve

- AzureRM is the hard requirement. `metadata_host` is host-only and provider source constructs `https://<host>` for custom Azure metadata discovery.
- Azure Stack is also HTTPS-shaped for ARM/metadata use.
- AzAPI exposes full endpoint URLs and defaults to HTTPS Azure endpoints; it is configurable but should work through the same gateway.
- Google Terraform provider custom endpoints are full URLs; current HTTP simulator endpoint overrides are valid and should keep working.
- AWS Terraform provider custom endpoints are full URLs; official docs explicitly support `http://localhost` service endpoints. HTTPS is optional for realism and CA-bundle coverage.
- Existing simulator direct TLS support via `SIM_TLS_CERT` / `SIM_TLS_KEY` stays. The gateway is an operator/developer front door, not a replacement for direct simulator TLS.

## Completed Gateway Stage

- Caddy config plus `make stack-https-{up,status,ca,down}` targets.
- HTTPS routes to current simulator ports:
   - `aws.sockerless.localhost` -> `127.0.0.1:4566`
   - `gcp.sockerless.localhost` -> `127.0.0.1:4567`
   - `azure.sockerless.localhost` -> `127.0.0.1:4568`
   - Azure data-plane wildcards -> Azure simulator, preserving host-addressed routing.
- `STACK_HTTPS=1` local stack integration, including Azure ARM-advertised data-plane URL projection.
- Admin UI visibility for gateway status, endpoints, CA path, and equivalent recovery `make` commands.
- Azure Terraform tests through the gateway, including Caddy state isolation, CA trust, ARM metadata verification, Azure data-plane endpoint projection, and a 300-second test timeout.
- Shared simulator Docker test image with Caddy installed from the official package repository.
- BUG-1246 fixed Azure Storage data-plane middleware overmatching non-storage `*.localhost` hosts.
- SDK/CLI guidance documents real endpoint and CA knobs for AWS CLI/SDKs, gcloud/Google clients, Azure CLI, and Azure SDKs.
- BUG-1250/BUG-1251 fixed stale `gcp-gcs` CLI coverage: `gcloud storage` now has real bucket/object lifecycle coverage, current gcloud multipart uploads work, GCS `buckets.getStorageLayout` returns the public response shape, and GCS timestamps use Cloud Storage-style millisecond precision.

## Remaining Stages

1. Document AWS/GCP HTTPS Terraform examples while preserving direct HTTP configs.
2. Decide whether CI should use Caddy by default for Azure Terraform or keep generated direct simulator certs/direct HTTP where those are the tested public-client paths.

## Deferred Trackers

- BUG-1075: live-cloud validation remains deferred by user direction. Do not mark cloud cells green without authenticated real-cloud runs.
- BUG-1104: audit-cadence meta tracker remains open. Every simulator phase should audit SDK/CLI/Terraform surface claims and file concrete BUGs before fixing.

## Start Checklist

1. `git fetch origin`
2. `git checkout main`
3. `git pull origin main`
4. `gh issue list --state open --limit 30`
5. Create a fresh branch from `origin/main`.
6. File any concrete gaps in `BUGS.md` before code changes.

## Rules That Matter For This Task

- No simulator-specific public API changes.
- No mocks, fakes, or fallback modes.
- No interactive commands.
- Rebase the PR branch on `origin/main` before pushing.
- User merges PRs; never run `gh pr merge`.
