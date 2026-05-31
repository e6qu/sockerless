# Sockerless - What We Built

Roadmap [PLAN.md](PLAN.md) - status [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md).

This file is intentionally compact. Detailed phase history lives in PR descriptions, `git log`, and issue threads. Keep this file focused on facts that a fresh session needs after context compaction.

## 2026-05-31 - Local HTTPS Gateway Stage 1

The local HTTPS gateway was implemented as optional transport infrastructure for simulator APIs. It did not change simulator public cloud API shapes.

The stage added `make stack-https-up`, `make stack-https-status`, `make stack-https-ca`, and `make stack-https-down`. The gateway runs Caddy under the repo's existing `.stack-pids` and `.sockerless-state` conventions, fronting the normal simulator HTTP ports with HTTPS names:

- `https://aws.sockerless.localhost:8443`
- `https://gcp.sockerless.localhost:8443`
- `https://azure.sockerless.localhost:8443`
- Azure host-addressed data-plane wildcards such as `{account}.blob.azure.sockerless.localhost`, `{vault}.vault.azure.sockerless.localhost`, and `{namespace}.servicebus.azure.sockerless.localhost`.

`STACK_HTTPS=1 make stack-azure-aca` now starts the gateway with the local stack and configures Azure ARM-advertised data-plane endpoint projection under the gateway hostnames. Direct HTTP and direct simulator TLS through `SIM_TLS_CERT` / `SIM_TLS_KEY` remain supported.

The admin UI topology page now shows gateway status, endpoints, CA path, and the equivalent recovery `make` commands.

## 2026-05-31 - Azure Terraform Through Local HTTPS Gateway

The Azure Terraform harness was moved from generated simulator TLS certificates to the optional local Caddy gateway. The simulator now starts on HTTP loopback for the test, Caddy terminates HTTPS on a random high port, Terraform uses `https://azure.sockerless.localhost:<port>` for AzureRM metadata and ARM endpoints, and the Linux Docker test container trusts Caddy's local CA through `SSL_CERT_FILE`.

The shared simulator Docker test image now includes Caddy from the official package repository, so the macOS delegation path and Linux container path run the same real gateway flow. The harness verifies the direct simulator metadata route before starting Caddy, waits for Caddy's local CA file, verifies `/health`, and validates Azure metadata JSON through the gateway before Terraform starts.

The gateway also preserved high-port Host headers and routed `*.documents.azure.sockerless.localhost` for Cosmos DB document endpoints. BUG-1246 fixed an Azure Storage data-plane middleware bug where the storage wrapper overmatched non-storage `*.localhost` hosts and swallowed `azure.sockerless.localhost` metadata requests.

BUG-1247 fixed the direct GitHub Actions Azure Terraform job by installing Caddy before `make terraform-test`; the Docker test image already had Caddy for containerized runs. BUG-1248 fixed GCP arithmetic SDK coverage to assert the actual Cloud Logging output line, `"Result: 30"`.

The full Azure Terraform apply/destroy test passed through the gateway under the 300-second cap.

## 2026-05-31 - Terraform Provider HTTPS Behavior Audit

We checked whether a generic local HTTPS gateway for simulator APIs made sense, especially for Terraform providers that require HTTPS even when pointed at a local simulator.

The answer was cloud-specific:

- AzureRM requires trusted HTTPS for custom metadata discovery. Its `metadata_host` setting is a hostname, and the provider constructs `https://<host>` internally.
- Azure Stack is also HTTPS-shaped for ARM/metadata endpoint use.
- AzAPI exposes full endpoint URLs and defaults to HTTPS Azure endpoints. It is more configurable than AzureRM, but should work through the same gateway.
- AWS and GCP do not require HTTPS for the current simulator Terraform paths. Their providers accept full custom endpoint URLs, and current sockerless Terraform harnesses use HTTP successfully.

The planned implementation is therefore an optional Caddy/local-HTTPS front door, not a simulator public API change. It should front all three simulators for developer ergonomics, while preserving existing direct HTTP and `SIM_TLS_CERT` / `SIM_TLS_KEY` modes. Azure Terraform is the first target because provider behavior actually requires trusted HTTPS there.

## 2026-05-31 - Latest Merged Simulator Work

Issue #298 and BUG-1242..BUG-1245 were closed.

What changed:

- Azure Cache for Redis has real Azure CLI and azurerm Terraform coverage for cache lifecycle, `listKeys`, firewall rules, SKU round-tripping, and PATCH.
- GCP Memorystore Redis has real `gcloud redis instances` and terraform-provider-google `google_redis_instance` coverage.
- GCP Cloud SQL exposes both `/v1` and `/sql/v1beta4` SQL Admin paths needed by SDK, gcloud, and terraform-provider-google flows.
- GCP Cloud DNS implements public Changes.Create/Get/List and ResourceRecordSets.Get/Patch, including exact delete/add validation and unknown-change NOT_FOUND behavior.

## Current Capabilities Snapshot

Sockerless provides a Docker-compatible REST API backed by cloud backends and local simulators. The project has:

- Backends for Docker passthrough, AWS ECS/Lambda, GCP Cloud Run/GCF, and Azure ACA/AZF.
- One simulator binary per cloud: `simulators/aws`, `simulators/gcp`, and `simulators/azure`.
- Simulator coverage through official SDKs, vendor CLIs, and official Terraform providers.
- Admin UI and stack orchestration for local components.
- Bleephub GitHub API simulator compatible with real `gh` CLI paths.

Recent simulator parity work added or hardened foundational cloud slices across object storage, queues, event systems, streams, DNS, data SaaS, VPC/networking, public IP/NAT, managed load balancers, and VM/instance control planes.

## Deferred Work

- BUG-1075: live-cloud validation remains intentionally deferred. Do not mark live cloud cells green without authenticated real-cloud runs.
- BUG-1104: audit cadence remains open. Every simulator phase should re-check SDK/CLI/Terraform surface claims and file concrete BUG entries before fixing.

## Continuity Rules

- No mocks, fakes, synthetic behavior, silent fallbacks, or degraded modes.
- Public simulator endpoints must match real cloud APIs. Admin/local infrastructure can exist, but it must not leak into public cloud API surfaces.
- Any new simulator public API slice requires SDK, CLI, and Terraform coverage unless the public service has no such client surface.
- User merges PRs. Agents create branches and PRs only.
