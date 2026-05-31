# Sockerless - What We Built

Roadmap [PLAN.md](PLAN.md) - status [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md).

This file is intentionally compact. Detailed phase history lives in PR descriptions, `git log`, and issue threads. Keep this file focused on facts that a fresh session needs after context compaction.

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
