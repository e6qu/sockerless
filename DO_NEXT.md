# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Current State

- Branch: `main`, synced with `origin/main` after PR #299 merged.
- Active implementation branch: none.
- Open GitHub issues at last check: none.
- Open BUG trackers: BUG-1075 and BUG-1104.
- Last completed work: issue #298 / BUG-1242..BUG-1245, covering Azure Redis, GCP Memorystore Redis, GCP Cloud SQL, and GCP Cloud DNS Changes client-surface fidelity.

## Next Task

Implement the optional local HTTPS gateway for simulator APIs.

The goal is a realistic local HTTPS front door for clients that require or naturally expect HTTPS, without changing simulator public cloud API shapes. Caddy is the current preferred implementation because it can terminate local TLS, manage a local CA, and reverse-proxy cleanly to existing simulator HTTP ports.

## Provider Facts To Preserve

- AzureRM is the hard requirement. `metadata_host` is host-only and provider source constructs `https://<host>` for custom Azure metadata discovery.
- Azure Stack is also HTTPS-shaped for ARM/metadata use.
- AzAPI exposes full endpoint URLs and defaults to HTTPS Azure endpoints; it is configurable but should work through the same gateway.
- Google Terraform provider custom endpoints are full URLs; current HTTP simulator endpoint overrides are valid and should keep working.
- AWS Terraform provider custom endpoints are full URLs; official docs explicitly support `http://localhost` service endpoints. HTTPS is optional for realism and CA-bundle coverage.
- Existing simulator direct TLS support via `SIM_TLS_CERT` / `SIM_TLS_KEY` stays. The gateway is an operator/developer front door, not a replacement for direct simulator TLS.

## Staged Plan

1. Add Caddy gateway config plus `make` targets to start/stop/status it.
2. Route HTTPS hostnames to current simulator ports:
   - `aws.sockerless.localhost` -> `127.0.0.1:4566`
   - `gcp.sockerless.localhost` -> `127.0.0.1:4567`
   - `azure.sockerless.localhost` -> `127.0.0.1:4568`
   - Azure data-plane wildcards -> Azure simulator, preserving host-addressed routing.
3. Wire Azure Terraform first: `metadata_host`, ARM-advertised data-plane URLs, gateway CA export to Linux test containers through `SSL_CERT_FILE`.
4. Document AWS/GCP HTTPS usage while preserving direct HTTP configs.
5. Add admin UI visibility for gateway status, endpoints, CA path, and equivalent recovery `make` commands.
6. Decide after local proof whether CI should use Caddy by default for Azure Terraform or keep generated direct simulator certs.

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
