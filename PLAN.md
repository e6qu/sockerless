# Sockerless - Roadmap

State [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Goal

Replace Docker Engine with Sockerless for Docker API clients (`docker`, Docker Compose, Testcontainers, CI runners), backed by real cloud infrastructure or high-fidelity local cloud simulators. Bleephub is the GitHub Enterprise Server-compatible service surface: git repository provider, release provider, CI/CD provider, Pages provider, authentication source, packages/container registry, and GitHub-compatible REST/GraphQL/UI/runner implementation.

## Non-Negotiable Principles

1. Match public application programming interfaces exactly: Docker, GitHub, and public cloud APIs.
2. No stubs, fakes, mocks, synthetic behavior, silent fallbacks, degraded modes, or skip-if-tool-absent tests.
3. Cloud backends stay stateless; the cloud is the source of truth.
4. Simulators are real local cloud slices, one binary per cloud, validated through official SDK, command-line interface, and Terraform clients where applicable.
5. Bleephub metadata/index state belongs in SQLite; git objects, artifacts, caches, release assets, packages, job logs, and similar durable bytes belong in object storage or an explicit durable local-development filesystem.
6. Public/user-facing Bleephub behavior must use GitHub-shaped public paths and contracts, not `/internal/*` operator shortcuts.
7. The user merges PRs. Agents create branches, commits, and PRs only.

## Active Focus

**Bleephub real-service hardening + live-cloud validation.**

Merged #791 made GitHub Pages branch publication and committed-reference eventing real. The active branch then closed the release-provider class and continued through the shared parity/UI layer, retained GitHub Classroom product, and fine-grained personal access token workflow: routed release and asset management, real git-tag identity, strict repository isolation, complete lifecycle events, race-safe workflow discovery, saturated light/dark chrome, organization-admin Classroom management, real repository-backed coursework and grading, transition export/import, durable one-time token creation, organization approval, GitHub App-only administration, and resource/permission enforcement.

The merged #783/#787 baselines pushed many shallow GitHub-compatible areas toward real behavior: repository metadata and permissions, pull request status rollups including count-by-state and Actions workflow-run links, durable GitHub Actions workflow-run and attempt history, durable gist/comment/star/fork state, release asset upload, GraphQL release immutable-state exposure, GitHub Pages deployment status routing, notification thread identity/URLs, user/organization/team/audit-log UI public-route usage, OAuth UI endpoint usage with explicit registered clients, registered OAuth client validation, stored-token browser authentication, persisted account suspension, checked credential entropy, Actions artifact and run/job repository scoping, workflow-dispatch ref resolution and organization provisioning for official `gh` command-line interface inputs, runtime enterprise coordinates for the Bleephub UI, Codespaces fail-loud semantics, GitHub App seed identity validation, Docker-backed local-image build loading, full local Docker-backed Bleephub hook coverage, public runner and metrics UI routing, route-level UI code-splitting, repository deletion durable-state cascades, object-backed Actions artifacts/caches/logs/release assets/package files/container-registry blobs/CodeQL database archives/CodeQL variant-analysis query packs/artifact attestation bundles, public security alert ingestion, removal of obsolete operator seed routes, runner-log object-store failure consistency, and incidental AWS simulator CI hardening. The active follow-up branch tightened persisted repository ownership, internal and public GitHub Actions execution-image and runner-label explicitness, container-package publishing through the GitHub Container Registry-compatible data plane, Projects v2 GraphQL owner resolution, GitHub Actions runner public-key wire-format validation, workflow runner-label validation, public repository commit-listing errors for empty or broken git state, empty-repository UI handling for that public commit-listing contract without browser resource errors, and GitHub App installation-token authorization for organization repository creation, then continued from there by finding remaining places where Bleephub still:

- returns shape-only GraphQL/REST data instead of real store/git/object state;
- exposes user workflows through internal-only endpoints;
- fabricates SHAs, refs, timestamps, IDs, permissions, counters, URLs, or repository context;
- falls back silently when storage, Docker, git, auth, or object storage is required;
- differs from GitHub's official REST/GraphQL/runner/client behavior in request/response shape or status codes;
- lacks UI coverage for a public GitHub-compatible workflow.

## Active Branch Priorities

1. Keep hardening Bleephub as a real GitHub-compatible service, not a simulator-specific harness.
2. Favor class fixes over point fixes: e.g. "all release uploads use raw upload contract", "all workflow refs resolve through git storage", "all public payload URLs avoid `/internal/`".
3. Add tests that drive public GitHub-shaped paths or official clients wherever practical.
4. Keep continuity concise and current; old per-bug detail belongs in PR descriptions and `git log`.

## Verified Next Bleephub Gaps

1. Finish BUG-2523 by replacing operator ingestion with GitHub-compatible browser/producer workflows for GitHub Marketplace, CodeQL databases, and hosted-compute network settings; GitHub Classroom and fine-grained personal access tokens were completed on the active branch.
2. Add a complete GitHub GraphQL schema/introspection ratchet; current coverage proves selected consumer surfaces only.
3. Extend page-level light/dark fidelity from the shared shell, repository Code view, Classroom, and account token settings through Settings/Security, issue/pull-request review workflows, Actions logs, organization administration, and remaining App settings.
4. Extend REST proof beyond registered/observed shapes into exhaustive permission, status/header, pagination, redirect, conditional-request, webhook, cascade, and failure-atomicity matrices.

## Standing Work

- **Bleephub full-service parity:** continue closing REST, GraphQL, UI, runner, auth, Pages, release, packages, and repository-provider gaps until Bleephub is usable as a real GitHub-compatible service.
- **BUG-1075 live-cloud validation:** the local-cloud runner cells are sim-proven, but live authenticated cloud validation remains open.
- **BUG-1345 AzureAD Terraform provider upstream blocker:** add AzureAD Terraform tests only after the provider supports a Microsoft Graph endpoint override.
- **BUG-2441 current `knip`/Node deprecation warning:** the unused-export gate passed, but the current `knip` 6.23.0 release still emitted Node's `DEP0205 module.register()` warning.
- **Issue #363 versioned releases + GitHub Container Registry images:** still a release/distribution task.
- **Simulator service ratchets:** AWS/GCP/Azure have operation-coverage gates; continue ratcheting uncovered cloud services when Bleephub focus is not the immediate task.

## Compressed Foundation Summary

- The cloud backend family is Docker-API-shaped and stateless across Docker passthrough, Amazon Elastic Container Service, AWS Lambda-class, Google Cloud Run, Google Cloud Run Functions, Azure Container Apps, and Azure Functions.
- GitHub Actions runner and GitLab docker-executor topologies are sim-proven across the container-capable backends, including container jobs, service containers, artifacts, and dispatcher-spawned runners.
- FaaS multi-container pod semantics were assembled from cloud primitives, including shared-loopback networking and shared workspace behavior.
- AWS, GCP, and Azure simulators have conformance/coverage gates and many service slices ratcheted to 100%; historical per-service detail lives in the merged PRs.
