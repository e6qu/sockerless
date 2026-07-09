# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/bleephub-actions-runtime-fidelity` |
| Branch purpose | Continued Bleephub GitHub-fidelity hardening after #782. The branch tightened repository metadata, GraphQL, Actions runtime context, Actions artifacts, Codespaces, OAuth, OAuth client validation, browser authentication, releases, GitHub Pages deployments, notifications, user/organization/team/audit-log/OAuth UI routes, credential entropy handling, pull request status rollups, commit statuses, repository webhooks, code-quality state, runner UI routing, source comments, run-control fixtures, and repository-scoped Actions ID resolution so Bleephub behaved more like a real GitHub Enterprise Server-compatible service and less like a compatibility harness. |
| Current fixes | BUG-2391 through BUG-2440 were fixed on this branch. Recent fixes moved Bleephub behavior from shape/fallback/internal shortcuts to real persisted state and public GitHub contracts: repository metadata/permissions came from repository/git/Pages/viewer access; release uploads used GitHub's raw upload host route; Pages deployments used public status URLs and identifiers; notification threads used typed issue/pull request IDs and GitHub REST URLs; user/organization/team/audit-log/OAuth UI management used GitHub Enterprise Server, GitHub REST, and OAuth routes instead of `/internal` management routes; browser sessions require a stored personal access token for the submitted account; OAuth web/device flows require registered OAuth App or GitHub App clients, validate the web-flow client secret, and the UI sends the explicit registered client identifier; suspended user accounts were persisted and enforced across API tokens and browser login; credential generation checked cryptographic entropy; Codespaces failed loudly on backend/image/lifecycle failures; Actions repository/ref/SHA context resolved through real git or stayed absent; Actions workflow dispatch resolves GitHub `ref` inputs through real git storage for full refs, branch names, tag names, and raw commit SHAs; Actions artifacts and public run/job endpoints were scoped to the workflow run's owning repository; GitHub App seed configuration required real explicit owner and installation accounts; simulator Google Cloud service-account credentials were described as endpoint coordinates for the real JWT exchange flow; stale hook-discovered test/type/spec gaps were fixed; Docker-backed Make targets used a shared local-image build helper that loaded images correctly with Buildx or legacy Docker builders; the local Bleephub Go pre-commit hook again ran the full Bleephub suite after the Docker-compatible runtime returned; the overview and metrics UI derive Actions and runner counts from public GitHub REST routes instead of `/internal/metrics` or `/internal/status`; the storage-coordinate UI route was removed; Bleephub UI route code was split into lazy page chunks with explicit vendor chunks; and dependency freshness passed after AWS, Google Cloud, and Bleephub UI tool upgrades. |
| Last merged (#782) | `feat/bleephub-repository-license-graphql` - **Persist Bleephub repository metadata and permissions from real state**. It wired repository license metadata, feature flags, merge settings, Pages capability, pushed/archive timestamps, template provenance, and permissions to real persisted state, and rebalanced AWS Command Line Interface simulator shards without changing required check names. |
| Last merged (#781) | **Bleephub GitHub Apps, Actions, storage, and repository fidelity**. It moved Actions artifacts/caches/logs to object storage, used real AWS simulator S3 tests, advanced GitHub App Manifest/browser installation flows, public runner/workflow paths, SQLite-only metadata, branch-protection check-name cleanup, and repository tag peeling through real git storage. |
| Last merged (#779) | **Bleephub pull request/release fidelity + continuity cleanup**. It made pull requests, reviews, releases, action downloads, CodeQL fixtures, and repository rename/transfer/delete derive from real git/object storage and public GitHub-compatible paths. |
| Older context | #778 closed the actionable open GitHub issues other than upstream-blocked AzureAD, #774 redesigned the Bleephub UI into a functional GitHub clone and added docs, #773 added fuzz/load/concurrency coverage and fixed store races and scale issues, #770/#750/#747 expanded Bleephub REST/UI coverage, and #665-#700 built the simulator conformance-gate and service-ratchet foundation. Older detail lives in PR descriptions and `git log`. |
| Open GitHub issues | #394 azuread Terraform Graph override - upstream-blocked (BUG-1345). |
| Bugs | See [BUGS.md](BUGS.md) header: 2441 filed, 2397 fixed, 3 open, 16 false positives. Open: BUG-2441, BUG-1345, and BUG-1075. |
| Local Docker | Available through Docker CLI compatibility backed by Podman 6.0.1; local Docker-backed Bleephub/Codespaces validation ran through the full Bleephub Go hook. |
| Live infra | None up. |

## What's Next

- Continue the active branch by replacing Bleephub fake, shallow, internal-only, or compatibility-only behavior with real GitHub-compatible REST/GraphQL/UI/runner behavior backed by SQLite metadata plus git/object storage.
- Prefer classes of fixes: public GitHub endpoints over `/internal/*`, real git/object-store state over fabricated IDs/SHAs/timestamps, fail-loud errors over fallbacks, and official client compatibility over hand-built harness behavior.
- Keep the three open gaps visible: BUG-2441 current `knip`/Node deprecation warning, BUG-1075 live-cloud validation, and BUG-1345 AzureAD Terraform provider upstream endpoint support.

## Invariants

- Never auto-merge PRs; the user handles merges.
- At most one PR open at a time. Put all work in the single in-progress PR.
- Rebase PR branches on `origin/main` before pushing; sync local `main` after.
- File a concrete `BUGS.md` entry before fixing a discovered defect.
- No stubs, fakes, mocks, synthetic responses, silent fallbacks, or degraded modes.
- Do not bypass, remove, ignore, stash around, or unstage around pre-commit/pre-push hooks.
- External identity stays GitHub/GitHub Enterprise Server-shaped: public paths, fields, `GITHUB_*` variables, runner contract, and user-facing UI text.
- Simulators remain real local cloud application programming interface slices, one binary per cloud, with official software development kit, command-line interface, and Terraform coverage where those surfaces exist.

## Environment Notes

- Simulator ports: AWS 4566, GCP 4567, Azure 4568.
- Azure Terraform tests are Docker-only because the AzureRM provider needs the local HTTPS gateway.
- Linux network-fabric tests require `CAP_NET_ADMIN`, iproute2, and nftables; off-Linux they skip through the realexec capability gate.
- Bleephub runner topology harnesses: `make bleephub-runner-docker-test` and `make bleephub-runner-docker-test-aca`; `BLEEPHUB_TEST_FROM=N` starts at a numbered test.
