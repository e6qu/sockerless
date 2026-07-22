# Sockerless - Roadmap

State [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Goal

Replace Docker Engine with Sockerless for Docker API clients (`docker`, Docker Compose, Testcontainers, CI runners), backed by real cloud infrastructure or high-fidelity local cloud simulators. Bleephub is an independent GitHub Enterprise Server-compatible service and consumes Sockerless through its published simulator/backend integration contract.

## Non-Negotiable Principles

1. Match public application programming interfaces exactly: Docker, GitHub, and public cloud APIs.
2. No stubs, fakes, mocks, synthetic behavior, silent fallbacks, degraded modes, or skip-if-tool-absent tests.
3. Cloud backends stay stateless; the cloud is the source of truth.
4. Simulators are real local cloud slices, one binary per cloud, validated through official SDK, command-line interface, and Terraform clients where applicable.
5. Bleephub metadata/index state belongs in SQLite; git objects, artifacts, caches, release assets, packages, job logs, and similar durable bytes belong in object storage or an explicit durable local-development filesystem.
6. Public/user-facing Bleephub behavior must use GitHub-shaped public paths and contracts, not `/internal/*` operator shortcuts.
7. The user merges PRs. Agents create branches, commits, and PRs only.

## Active Focus

**Cloud simulator/backend fidelity, production operation, and live-cloud validation.**

Sockerless Admin and the AWS, Google Cloud, and Microsoft Azure simulator dashboards completed one first-party OpenID Connect boundary for their rendered interfaces and operator data while native cloud protocol routes remained independent. The exact PostgreSQL, Ory Hydra, Shauth, compiled relying-party, and Chromium matrix proved direct and catalog entry, shared sign-on, identity, relying-party and provider logout, correlated logout completion, application-local signed-out return, reload, reauthentication, global revocation, release identity, anonymous fail-closed behavior, active event-stream readiness, and validator-credential isolation.

Production operation had enforceable resource and artifact contracts. Every ordinary GitHub Actions job was bounded to 15 minutes, the historically over-budget AWS edge and Amazon EC2 command-line interface groups were split without losing any of the 630 tests, nightly fuzz targets ran in bounded parallel batches, and clean production builds created every frontend before compiling all 11 UI-bearing Go binaries. Native release tags remained direct architecture manifests while each short-SHA tag remained an OCI index containing exactly Linux ARM64 and AMD64.

The next fidelity work stayed evidence-driven: deterministic Amazon Elastic Container Service Terraform simulator lifecycle, provider-faithful simulator credential authentication, and authenticated real-cloud validation. Standalone products consumed published Sockerless simulator and backend contracts and retained their own source, deployment modules, and product-specific tests.

## Active Branch Priorities

1. Preserved the exact Shauth browser contract in the real PostgreSQL, Ory Hydra, compiled relying-party, and Chromium matrix.
2. Kept every ordinary workflow job within the enforced 15-minute ceiling without narrowing test coverage.
3. Kept production builds and releases incapable of silently omitting embedded web interfaces.
4. Kept nightly fuzzing bounded, complete across discovered targets, and truthful about missing modules, failures, and crashers.
5. Kept continuity concise and current; detailed historical work remained in pull requests and `git log`.

## Verified Next Gaps

1. BUG-2569 still required deterministic local Amazon Elastic Container Service Terraform simulator apply/destroy completion without changing real-provider coordinates or behavior.
2. BUG-2625 still required provider-faithful credential issuance and verification across all three simulators.
3. BUG-1075 still required authenticated validation for the remaining real-cloud backend cells.
4. The merged authentication and workflow contracts still required publication, deployment, and the exact live-origin browser matrix.

## Simulator Console Parity

The three simulator interfaces are one generic application wearing three accent
colours. Layout, navigation, typography, density, and component set are
identical across AWS, Google Cloud, and Azure; only the tint and the navigation
labels differ. Each real console is unmistakably its own product, so an operator
who knows one of them recognises nothing here.

The goal is recognisability, not replication: adopt each cloud's real shell,
information architecture, density, colour, and terminology, using each
vendor's published design guidance, without copying proprietary assets.

Evidence gathered by opening both sides and capturing them, rather than from
memory:

- **AWS** builds its console from the Cloudscape Design System, which is
  published. Its resource pages carry a dark global header, breadcrumbs, a
  collapsible grouped service navigation, a `Resource (count)` heading with an
  information link, a primary create action beside secondary row actions,
  per-column sorting and filtering, selection checkboxes, per-row overflow
  menus, pagination, density settings, and dismissible notifications. Its
  dashboards carry region context, service health, and charts.
- **Azure** publishes an annotated diagram of its portal shell: a dark global
  header, breadcrumb, global search, a horizontal command bar of icon actions,
  a service menu with its own search and expandable groups, and an Essentials
  panel presenting resource properties as a two-column key/value grid.
- **Google Cloud** presents a light global header — hamburger, wordmark, a
  project picker chip, and a wide central search field, with the account and
  tools at the right — over a product navigation whose active item is a filled
  rounded pill. Its resource pages put inline text actions beside the page
  title rather than a button group, describe the resource in a sentence beneath
  it, and carry a filter chip with column settings above a table whose headers
  hold inline help. Empty states pair a hand-drawn illustration with a headline,
  an explanation, the side effect of the action, a filled primary button, and a
  quickstart link. The console itself offers Light, Dark, and Same-as-device
  themes plus an increased-contrast setting.

Three changes, one per cloud, in this order. Each is a separate pull request so
one cloud's interface can be reviewed against its own console rather than
against the other two.

1. **AWS simulator console parity.** Shell, navigation, resource tables, and
   empty states in the Cloudscape idiom. Done.
2. **Azure simulator console parity.** Portal shell, command bar, service menu,
   and Essentials-style resource properties. Done.
3. **Google Cloud simulator console parity.** Light header with its project
   chip and central search, inline text actions beside the page title, the
   filter chip with column settings, and the illustrated empty state.

Every interface carries a light and a dark theme, a theme control in the top
right of the header where each console keeps its own, and text that meets WCAG
AA contrast in both. Contrast is measured against the rendered surfaces rather
than assumed from the palette.

Shared shell components currently live in `ui/packages/core`. Divergence is the
point here, so per-cloud presentation belongs in each simulator package, and
only genuinely cloud-neutral behaviour stays shared.

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
