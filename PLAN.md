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

The merged #783 baseline pushed many shallow GitHub-compatible areas toward real behavior: repository metadata and permissions, pull request status rollups including count-by-state and Actions workflow-run links, durable GitHub Actions workflow-run and attempt history, durable gist/comment/star/fork state, persistence loader documentation, release asset upload, GraphQL release immutable-state exposure, GitHub Pages deployment status routing, notification thread identity/URLs, user/organization/team/audit-log UI public-route usage, OAuth UI endpoint usage with explicit registered clients, registered OAuth client validation, stored-token browser authentication, persisted account suspension, checked credential entropy with returned API errors for public GitHub App, OAuth, fine-grained personal access tokens, gist, security advisory, Classroom, OpenID Connect, hosted-compute, runner-token, cache-token, and OAuth App token-management entropy failures, Actions artifact and run/job repository scoping, workflow-dispatch ref resolution and organization provisioning for official `gh` command-line interface inputs, runtime enterprise coordinates for the Bleephub UI, clean localStorage-backed UI tests, Codespaces fail-loud semantics, OAuth device approval, GitHub App seed identity validation, runner harness credential-coordinate naming, Docker-backed local-image build loading across Buildx and legacy Docker frontends, full local Docker-backed Bleephub hook coverage, code-quality setup state boundaries, Actions secrets/variables/context/ref resolution, repository webhooks, runner UI public routing, overview/metrics UI aggregation and Playwright coverage through public GitHub Actions REST routes instead of internal diagnostics, source naming, route-level UI code-splitting, run-control fixtures, and incidental AWS simulator CI hardening for Amazon SQS long polling and AWS Budgets CloudTrail event-source mapping. The active follow-up branch fixed repository deletion cascades for issue/pull children, repository-ID keyed state, selected-repository references, deployment/environment state, environment policies, and Pages deployment records, then continued from there by finding remaining places where Bleephub still:

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
