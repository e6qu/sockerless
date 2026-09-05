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

1. Pinned the simulators at sockerless-cloud v0.30.3, whose Google Artifact Registry data plane authenticates every `/v2/` request and requires the repository to exist, with every pin moved together (`tests/go.mod` including the `ui-auth` support module, the harness Dockerfiles, the compose build context, the Lambda live-test checkout).
2. Made the Artifact Registry endpoint one coordinate on both Google backends: read once into `Config.ARRegistryEndpoint`, naming the host image references carry and the URL registry HTTP is dialed at for the auth provider, the tag probe, the multi-arch index, and the image resolver; the tag probe reports credential and transport failures instead of rebuilding.
3. Made the shared image pull and push present the Docker client's credential: `X-Registry-Auth` decoded into the registry's `Authorization` value, an identity token refused rather than dropped, a `Basic` challenge answered directly.
4. Made the Cloud Run and Cloud Run Functions harnesses provision Artifact Registry the way Terraform and the operator do — repositories through the API, `docker login -u oauth2accesstoken` with a token minted from the service-account key, in a Docker configuration the simulator's own Cloud Build and Cloud Run host inherit — and gave the Google Terraform modules the `sockerless-overlay` and `gitlab-registry` repositories the backends name.

## Next

- Watch the first CI run of the registry branch; the Cloud Run and Cloud Run Functions harnesses run there against the enforcing Artifact Registry for the first time.
- BUG-2945 once the Azure simulator serves the repository catalog: read `docker images` through `core.OCIListImages` in the Azure Container Registry round trip.
- BUG-2922: migrate the Docker passthrough backend from `github.com/docker/docker` to the `github.com/moby/moby` client and API modules.
- BUG-1075: live-cloud validation beyond AWS Lambda.
