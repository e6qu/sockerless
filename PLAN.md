# Sockerless - Roadmap

State [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Goal

Replace Docker Engine with Sockerless for Docker API clients (`docker`, Docker Compose, Testcontainers, CI runners), backed by real cloud infrastructure or the high-fidelity local cloud simulators that live in [sockerless-cloud](https://github.com/e6qu/sockerless-cloud). This repository is the backends, their shared libraries, the agent and bootstrap binaries, the CLI, the admin console, the runner dispatchers, and the harnesses that prove them against the pinned simulators.

## Non-Negotiable Principles

1. Match public application programming interfaces exactly: Docker, Podman, and public cloud APIs.
2. No stubs, fakes, mocks, synthetic behavior, silent fallbacks, degraded modes, or skip-if-tool-absent tests.
3. Cloud backends stay stateless; the cloud is the source of truth.
4. A backend talks to a simulator with the same code and identifiers it uses against the real cloud, differing only in coordinates.
5. Shared code has three homes — `backends/core` for cloud-neutral behaviour, `agent/envelope` for the bootstrap wire contract, and one `*-common` module per cloud family — and the per-cloud modules never import each other (see [AGENTS.md](AGENTS.md)).
6. The user merges PRs. Agents create branches, commits, and PRs only.

## Active Focus

**One implementation per Docker behaviour, and cloud-backend fidelity against the pinned simulators.** The six cloud backends assemble Docker semantics from cloud primitives; where two of them assembled the same thing the assembly is now written once. What remains is the behaviour each cloud primitive genuinely dictates, and proving it against the simulators (locally and in CI) and, where credentials exist, against the real clouds.

## Active Branch Priorities

1. Consolidated the code the six cloud backends and three `*-common` modules had been carrying as copies (about 2,100 identical lines after normalising cloud names, and as much again in near-copies): shared-volume configuration, bind translation, and environment readers; buffered stdin and attach; the bootstrap overlay image and pod manifest; pod lifecycle loops; network-pod materialisation; managed-volume shaping; cloud-error mapping; image reference splitting; the DNS-zone discovery skeleton; the exec-envelope wire contract (once in `agent/envelope`, spoken by the four bootstraps and the four FaaS backends); and, per cloud family, AWS SDK configuration and ECR pull-through routing, the Cloud Run volume mapper and gcs-sync exec staging, and the Azure Log Analytics reader and tag flattening.
2. Fixed the divergences the comparison exposed: the unbounded Cloud Run / Cloud Run Functions attach read, and the Azure Container Apps Log Analytics client that queried over plain HTTP with no credential.
3. Documented the placement rule so the copies do not grow back, and repointed the documentation links the Bleephub extraction had left dangling.

## Next

- Watch the first CI run of the consolidated branch; every backend's unit suite, lint, and the integration harnesses run there against the pinned simulators.
- Registry items in order: BUG-2943 (Google Cloud harnesses push anonymously into repositories they never create) with BUG-2946 (the Artifact Registry endpoint coordinate is split), then BUG-2945 once the Azure simulator serves the repository catalog.
- BUG-2922: migrate the Docker passthrough backend from `github.com/docker/docker` to the `github.com/moby/moby` client and API modules.
- BUG-1075: live-cloud validation beyond AWS Lambda.
