# Sockerless - Roadmap

State [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Goal

Replace Docker Engine with Sockerless for Docker API clients such as `docker`, Docker Compose, Testcontainers, and CI runners, backed by real cloud infrastructure or high-fidelity local cloud simulators.

## Current Phase

Idle on `main`. No implementation branch is active.

Next planned phase: optional AWS/GCP Terraform HTTPS examples, then a CI policy decision for Azure Terraform's gateway path.

## Guiding Principles

1. Match public APIs exactly: Docker API, GitHub API for bleephub, and public cloud APIs for simulators.
2. No stubs, fakes, mocks, synthetic behavior, silent fallbacks, or degraded modes.
3. Simulators are real local cloud slices, not product-specific emulators.
4. One simulator binary per cloud.
5. Every new simulator public API slice ships with official SDK, vendor CLI, and Terraform-provider coverage where those surfaces exist.
6. Components remain decoupled. Admin UI and local gateway infrastructure must not become required by the simulators' public APIs.
7. User merges PRs. Agents create branches, commits, and PRs only.
8. Continuity docs are updated in every PR and written so they are correct after the PR merges.

## Local HTTPS Gateway

The optional Caddy/local-HTTPS front door for simulator APIs was added.

This is local transport infrastructure, not a simulator public API change. Public simulator routes, headers, request bodies, and response shapes must remain cloud-compatible.

### Provider Facts

- AzureRM is the hard requirement. Its `metadata_host` field is host-only, and provider source builds `https://<host>` for metadata discovery.
- Azure Stack is also HTTPS-shaped for ARM/metadata usage.
- AzAPI exposes full endpoint URLs and defaults to HTTPS Azure endpoints.
- Google provider custom endpoints are full URLs and current HTTP simulator overrides remain valid.
- AWS provider custom endpoints are full URLs and official docs explicitly support `http://localhost` service endpoints.
- Existing direct simulator TLS via `SIM_TLS_CERT` / `SIM_TLS_KEY` remains supported.

Implemented:

- Caddy config and `make stack-https-{up,status,ca,down}` targets.
- HTTPS routing for `aws.sockerless.localhost`, `gcp.sockerless.localhost`, `azure.sockerless.localhost`, and Azure host-addressed data-plane wildcards, including Cosmos DB documents.
- `STACK_HTTPS=1 make stack-azure-aca` style stack integration, including Azure ARM-advertised data-plane URL projection.
- Admin API/UI visibility for gateway status, endpoints, CA path, and recovery make commands.
- Docs for local CA trust and provider-specific HTTPS behavior.
- Azure Terraform harness over the gateway: per-test Caddy state and CA, `metadata_host`/ARM endpoint through `https://azure.sockerless.localhost:<port>`, ARM-advertised Azure data-plane URLs under the gateway, and `SSL_CERT_FILE` CA trust in the Linux Docker test container.
- Shared simulator Docker test image with Caddy installed from the official package repository.

Remaining staged work:

1. **AWS/GCP examples.** Add optional HTTPS Terraform examples while keeping direct HTTP endpoint overrides.
2. **CI decision.** Decide whether Azure Terraform CI should use Caddy by default now that the local proof passed under the 5-minute cap.

## Deferred Work

- BUG-1075: live-cloud validation. Deferred by user direction. Do not mark live cells green without authenticated real-cloud runs.
- BUG-1104: audit cadence. Keep open while simulator work continues; every simulator phase should re-check stale SDK/CLI/Terraform coverage claims.

## Current Capability Summary

- Docker-compatible REST API with cloud backends for AWS, GCP, and Azure plus Docker passthrough.
- AWS/GCP/Azure simulators with SDK, CLI, and Terraform validation for the public API slices sockerless depends on.
- Bleephub GitHub API simulator compatible with real `gh` CLI paths.
- Admin UI and local stack orchestration.
- Foundational simulator parity exists for object storage, queues, events, streams, managed data SaaS, DNS, VPC/networking, NAT/public-IP, managed load balancers, and VM/instance control planes.

## History

Detailed phase history has been intentionally removed from continuity docs to keep fresh sessions actionable. Use PR descriptions, issue threads, and `git log` for older per-phase details.
