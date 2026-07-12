# Bleephub ↔ GitHub parity audit

Status: **active parity ratchet on `feat/bleephub-ui-api-completeness-audit`**. Original audit: 2026-05-12. Last verified: 2026-07-12.

## Goal

Every Bleephub client surface should behave like GitHub or GitHub Enterprise Server after changing coordinates only:

- REST: `http(s)://<host>/api/v3/...`
- GraphQL: `http(s)://<host>/api/graphql`
- OAuth and GitHub App browser flows: GitHub-shaped `/login/...`, `/settings/...`, and installation paths
- Git smart HTTP: `http(s)://<host>/<owner>/<repo>.git`
- GitHub Actions runner protocol: `http(s)://<host>/_apis/...`
- Web UI: GitHub-shaped information architecture backed by the same public APIs and durable state

GitHub Enterprise Server coordinates are intentional. Official clients already swap their base URL this way.

## What was verified

### REST route and response contracts

The registered `/api/v3` surface covered the vendored GitHub REST description plus documented GitHub Enterprise Server-only operations. `TestRegisteredAPIv3RoutesExistInGitHubSpec` rejected invented GitHub-namespace routes, and the runtime OpenAPI observer rejected new response-member/type drift. The vendored description lived at `bleephub/testdata/github-openapi.json.gz` and was refreshed by `scripts/update-github-openapi.sh`.

This proved route legitimacy and every response shape exercised by tests. It did **not** prove every status, validation branch, permission combination, pagination edge, webhook, or storage failure for every registered operation. GitHub's REST API is versioned, so the vendored description and compatibility tests remained a ratchet rather than a one-time completeness claim.

### Real state and byte planes

- Repository identity and metadata lived in persisted state; git refs, commits, trees, tags, contents, comparisons, archives, pushes, and Pages branch builds read real git storage.
- Actions artifacts, caches, logs, Pages publications, releases, package files, GitHub Container Registry blobs, CodeQL databases/query packs, and attestation bundles used object storage in persisted mode.
- Releases resolved or created real tag refs and emitted lifecycle webhooks and Actions events.
- GitHub Pages consumed real artifacts or branch trees, ran the pinned Jekyll runtime when required, and served published objects.
- Cloud-backed Codespaces and runner execution failed loudly when their required runtime or storage operation failed.

### Authentication, Apps, and events

The earlier semantic-gap list had become stale. The current implementation and tests covered:

- GitHub App installations with `repository_selection: all|selected` and selected-repository token downscoping;
- installation and installation-repositories lifecycle events;
- webhook `installation` blocks plus `X-GitHub-Hook-ID`, target-type, target-ID, event, delivery, SHA-1, and SHA-256 headers;
- App hook configuration, delivery listing/detail, and redelivery;
- OAuth web/device flows and token-management prefixes;
- GitHub App installation/user tokens, suspension, repository selection, and organization-repository authorization.

### UI organization and themes

The application shell used GitHub's global navigation model. Repository pages used full-width repository context chrome, the familiar primary repository tab order, real Watch/Fork/Star actions, a separate content toolbar, and an administrative overflow/settings group. The browser mutations used GitHub's public repository APIs; a read-only `/ui-data` adapter normalized expected `404` viewer-state checks so ordinary page rendering did not emit console resource errors.

The visual system retained GitHub/Primer light and dark surface/semantic tokens, then added a deliberately more saturated blue/cyan/purple/pink brand layer. Both themes were browser-asserted. Primer's token and color-mode model remained the reference for contrast and theme separation: <https://primer.style/product/primitives/>.

### Retained GitHub Classroom product

Bleephub retained GitHub Classroom as an authenticated browser product while preserving GitHub's six read-only Classroom REST endpoints for the official `go-github` client and GitHub Classroom extension. Organization administrators created, renamed, archived, and deleted classrooms; managed linked or identifier-only rosters; created individual or group assignments; configured deadlines, permissions, feedback pull requests, team limits, and command-based autograding; and exported or imported a lossless transition bundle.

Assignment acceptance generated an organization-owned repository from the real starter git tree, granted the student or group access, created the configured Feedback pull request, installed a real GitHub Actions workflow, and recorded its baseline commit. Submission counts, subsequent commit counts, passing state, and exported points were derived from repository history and completed autograding jobs rather than accepted from management requests. The obsolete `/internal/classrooms...` seed routes no longer existed.

## What is truly left

### 1. External producer/browser workflows still enter through operator routes

These are real stored implementations after ingestion, but their creation/onboarding path is not yet GitHub-user- or producer-shaped:

| Domain | Current ingress | Required completion path |
|---|---|---|
| GitHub Marketplace purchases | `/internal/marketplace/purchases` | Marketplace purchase/change/cancel workflow that drives account billing state and events |
| Hosted-compute network settings | `/internal/orgs/.../network-settings` | GitHub/Azure private-network onboarding workflow that provisions the settings resource before public configuration APIs reference it |

CodeQL databases no longer belonged in this table. The official CodeQL Action's uploads-host raw ZIP request produced the durable database, validated the database bundle and real commit, and replaced the prior language database atomically. Fine-grained personal access tokens likewise entered through authenticated account settings with one-time credential disclosure and organization approval.

The runner execution controller (`/internal/exec/...`) and operator diagnostics (`/internal/{metrics,status,storage}`) were not classified as GitHub API gaps: they are Bleephub control-plane interfaces, and user-facing UI pages already read public GitHub/health routes instead of them.

### 2. GraphQL has no full public-schema ratchet

Bleephub supports the GraphQL queries and mutations required by its consumers, including repository, pull request, issue, discussion, Projects v2, organization, and status/check relationships. It does not yet validate its entire introspectable schema against GitHub's published schema. GitHub GraphQL is strongly typed and introspective; complete parity therefore needs a vendored schema/introspection diff plus official-client queries, not only hand-selected resolver tests.

### 3. REST semantic coverage is observed, not exhaustive

The route and response observer cannot prove unexecuted branches. Remaining audit work should prioritize:

1. installation-token permission matrices on every write family;
2. conditional requests, redirects, pagination/link headers, API-version headers, and rate-limit behavior;
3. webhook/action emission for every mutating resource transition;
4. delete/rename/transfer cascades for every newly persisted repository-owned record;
5. object-store and git-store failure atomicity on every byte-owning operation;
6. official `gh`, `go-github`, Git, package/registry, runner, and Terraform-adjacent consumers rather than hand-built request-only coverage.

GitHub explicitly recommends following redirects, consuming pagination links, using conditional requests, and treating repeated `4xx`/`5xx` responses as real errors; those behaviors remain part of parity even when the JSON body matches.

### 4. UI page-by-page visual and workflow parity remains broader than the shared shell

The shared chrome and repository Code experience are now close to GitHub and theme-complete, but the long-tail pages still need page-level comparison and workflow coverage. Highest-value next slices are:

1. repository Settings organization and the remaining Secret scanning/Dependabot security subpages;
2. issue and pull-request timelines, review controls, and file-diff interactions;
3. Actions workflow/run/job log organization and live updates;
4. organization profile, people, teams, rulesets, audit, and settings hierarchy;
5. account settings, fine-grained token creation, Apps/OAuth management, and installation flows;
6. responsive/mobile navigation, keyboard behavior, focus management, and color-contrast audits;
7. visual regression baselines for both light and dark modes across all routed pages.

### 5. Live service validation remains separate

Bleephub local fidelity does not make the Sockerless cloud backends live-cloud validated. BUG-1075 and the upstream-blocked AzureAD Terraform issue remain tracked in the project roadmap, not as Bleephub API signature defects.

## Acceptance criteria for future parity work

1. A newly served `/api/v3` route matched the official GitHub REST description or a cited GitHub Enterprise Server contract.
2. Positive, permission-denied/not-found, validation, pagination, and persistence-reload paths were tested where applicable.
3. User-facing workflows used GitHub-shaped public/browser paths rather than `/internal/*` setup.
4. Durable metadata stayed in SQLite; git and service bytes stayed in git/object storage.
5. Mutations emitted the webhooks and GitHub Actions events produced by the equivalent GitHub transition.
6. UI mutations used public GitHub APIs, rendered errors visibly, and produced no browser console errors.
7. Light and dark themes were both asserted for shared visual changes.
8. Official clients were preferred wherever an official client surface existed.

## Historical phase summary

Phases 153-155 registered the broad Apps, OAuth, repositories, issues, pull requests, checks, webhooks, teams, releases, deployments, environments, security, Git data, Actions, notifications, and administration surfaces. Later branches completed the vendored REST registration set, GraphQL consumer surfaces, runner protocol, public ingestion, durable state/byte planes, Pages publication, release-provider workflows, and the routed React UI. Per-operation history belongs in `git log`, pull requests, and `WHAT_WE_DID.md`; this document keeps only the current proof boundary and remaining gaps.
