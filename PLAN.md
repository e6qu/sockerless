# Sockerless — Roadmap

> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

State [STATUS.md](STATUS.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/](specs/).

## Guiding principles

1. **Docker API fidelity** — match Docker's REST API exactly.
2. **GitHub API fidelity (bleephub)** — match GitHub's REST + GraphQL paths and shapes exactly, modulo base domain. Including request-body tolerances: if real GitHub accepts string-coerced booleans (what `gh api -f` sends), bleephub accepts them too. The `gh` CLI must work directly against bleephub — not via URL hackery.
3. **Real execution** — sims and backends actually run commands; no stubs, fakes, or mocks.
4. **External validation** — proven by unmodified external test suites (the `gh` binary, the official `actions/runner`, real Docker SDKs, Terraform providers).
5. **Driver-first handlers** — handler code routes through driver interfaces.
6. **LLM-editable files** — source files under 400 lines.
7. **State persistence** — every task ends with a state save (STATUS.md / DO_NEXT.md / WHAT_WE_DID.md / MEMORY.md / `_tasks/done/`).
8. **No fallbacks, no skips, no defers, no fakes** — every functional gap is a real bug; every bug gets a real fix in the same session it surfaces; cross-cloud sweep on every find. **In particular: we are not in legacy maintenance — no shims for old bleephub behavior.** If real GitHub does X, bleephub does X.
9. **Sim parity per commit** — any new SDK call adds a sim handler + matrix row in the same commit.
10. **Single work-branch rule** — all in-flight work lands on one branch. User handles every merge.
11. **Cross-cloud is permanently off the table** — cloud-specific drivers extend the generic shape; cross-cloud duplication is fine, in-cloud duplication consolidates into `*-common`.
12. **Components stay decoupled from admin / UI.** Sims, backends, bleephub remain independently configurable, buildable, runnable. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). No admin-required env vars on components, no startup registration, no "I'm being managed" hooks.
13. **Persistence is opt-in + fail-loud.** Operator-requested persistence (`BLEEPHUB_PERSIST=true`, `SIM_PERSIST=true`) that fails to open must `log.Fatalf`. Never silently fall back to in-memory (BUG-985/986).

## Closed phases (PR index)

Headline-only. Per-bug detail in [BUGS.md](BUGS.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

| PR | Phases | Headline |
|---|---|---|
| #112–123 | 86–123 | Sim parity; stateless backends; FaaS pod overlays; storage-backing driver pilot; **8/8 runner cells GREEN.** |
| #125 | CI reorg | Workflows reorganized: zero auto-fire on main; live-tests-{cloud}. |
| #128 | 134 | Makefile standardization + per-app leaf Makefiles + stack orchestration. |
| #129 | 135 | Sim host model + 3-tier coverage + native arm64 CI runners. |
| #130 | 128 | Runner job timeout (bootstrap timer + cloud-native cap). |
| #131 | 124 | Network discovery driver (host-aliases / cloud-dns / service-mesh / nat-gateway-only). |
| #132 | 125 | DNS driver (cloud-map / cloud-dns-zone / private-dns-zone / service-discovery / none). |
| #133 | 126 | Access driver (iam-role / id-token / mTLS / none-internal). |
| #134 | 127 | Storage driver expansion (pd-ephemeral / efs-ephemeral / azure-files-ephemeral). |
| #135–136 | 121b | Azure sim hardening, driver consolidation pattern B, network-discovery adapter consolidation, AZF/Lambda DNS, Azure AD access. |
| #137–142 | 78–84 | UI polish + admin orchestration (`sockerless.yaml` topology, `TopologyManager`, lifecycle endpoints, UI Topology page, per-instance logs + console, cloud-resources rollup, sim UI parity, per-instance state isolation + BUG-985/986). |
| #143–144 | 85–86 | Config edit + hot reload; health + supervision surface (exit-code capture, `/diagnostics`, `<UnhealthyDiagnosticPanel>`). |
| #145–146 | 87 + 87b | Observability stack (otel-collector + VictoriaLogs + Jaeger) + component-side OTel SDK wiring. |
| #147–149 | 91 + 91b + 91c | `BackingMemory` translator across 5 backends; Lambda volume_translator framework migration; cloudrun + gcf `BackingPDEphemeral` rejection. |
| #150 | 87c | zerolog → OTel logs bridge across all 12 components. |
| #151 | 87d + 92 | Trace propagation + MeterProvider + runtime metrics + `make stack-observability-validate`; `Backing: gcs-fuse` deregistered on cloudrun + gcf (closes BUG-944, ships BUG-987). |
| #152 | docs | `docs/POD_MATERIALIZATION.md` — per-backend pod materialization walked through GH + GitLab runners. |

## Active + planned phases

Each entry: scope, why, acceptance. Pick from [DO_NEXT.md](DO_NEXT.md).

### Phase 153 — bleephub ↔ GitHub API signature parity (in flight on PR #153)

Every bleephub HTTP endpoint matches real GitHub's path + request shape + response shape exactly, modulo base domain. **And** every request body that `gh` CLI sends — including the string-coerced bool/int variants `gh api -f` emits — is accepted exactly as real GitHub accepts it. Bleephub is "maximally compatible": if `gh` works against `api.github.com`, it works against bleephub. Spec: [specs/BLEEPHUB_GITHUB_API_PARITY.md](specs/BLEEPHUB_GITHUB_API_PARITY.md).

**Status**: 12/13 sub-tasks shipped on `docs-cleanup-actionable` branch. P153.13 (real `gh` CLI Docker harness + GitHub-spec body tolerance) is the final piece.

Eight gap buckets covered:

1. **Missing endpoints** ✓ shipped (P153.3, P153.4, P153.5, P153.8) — `GET /apps/{slug}`, `/orgs/{org}/installation`, `/users/{username}/installation`, `PUT|DELETE /app/installations/{id}/suspended`, `GET /installation/repositories`, `PUT|DELETE /user/installations/{id}/repositories/{repo_id}`, hook delivery redelivery, app-level webhook config + deliveries, OAuth `applications/{client_id}/token` family, Checks API.
2. **Permission enforcement** ✓ shipped (P153.6) — `requirePerm(scope, level)` decorator gates write-class endpoints. PAT bypasses (real GH behavior); ghs_/ghu_ check installation perms; gho_ maps classic OAuth scopes.
3. **Repository selection** ✓ shipped (P153.1 + P153.3) — `repository_selection: "selected"` with per-installation `SelectedRepoIDs` allow-list.
4. **Webhook payload + headers** ✓ shipped (P153.7) — `installation:{id}` on every event; all four X-GitHub-Hook-* headers; X-Hub-Signature (SHA1) alongside SHA256.
5. **App-targeted webhook events** ✓ shipped (P153.7) — `installation`, `installation_repositories` fired on store transitions; `installation_target` and `github_app_authorization` reserved for future events.
6. **OAuth token prefixes + refresh tokens** ✓ shipped (P153.1 + P153.2) — `gho_`, `ghu_`, `ghr_`, `ghs_`, `ghp_` recognized.
7. **JSON shape** ✓ shipped (P153.9) — `*_url` HATEOAS fields, `installations_count`, `suspended_at`, `suspended_by`, `single_file_name`.
8. **`gh` CLI compatibility** in flight (P153.13) — bleephub accepts what real GitHub accepts (string-coerced booleans/integers from `gh api -f` calls). Test harness uses real `gh repo create` / `gh issue create` / `gh pr create` end-to-end.

**Bundled: SQLite persistence (P153.12)** — `BLEEPHUB_PERSIST=true` + `BLEEPHUB_DATA_DIR=…` enables write-through SQLite for users / tokens / apps / oauth_apps / installations / installation_tokens / user_to_server_tokens / refresh_tokens / repos. Fail-loud on open failure (BUG-985/986 pattern). Git storage stays in-memory.

Acceptance: `make bleephub-gh-docker-test` exercises Phase 153 surface end-to-end through the real `gh` binary in Docker, including `gh repo create`, `gh issue create`, `gh pr create`. `go test ./...` green in `bleephub/`. UI surfaces installation CRUD + suspend + repo selection + PEM viewer + OAuth Apps + suspend/delete.

### Phase 154 — Broad GitHub API sweep (planned, post-153)

After Phase 153 ships, sweep the rest of the GitHub API surface so the typical `gh` / `octokit` / `probot` workflow runs end-to-end against bleephub without surprise rejections. Audit-first: cross-reference real GitHub's OpenAPI spec against the current `gh_*.go` handlers + GraphQL schema, file per-surface gap tickets, prioritize by gh-CLI hit-rate (commands operators actually run).

Surfaces in scope:

1. **GitHub Apps (deeper)** — `installation_target` + `github_app_authorization` events; new_permissions_accepted flow; per-installation per-permission grant matrix; webhook redelivery with attempt tracking; manifest creation preflight redirect; marketplace stub.
2. **Orgs** — members API (list, add, remove, role change), teams API depth (parent teams, sync to IdP groups), audit log, security manager role, org-level secrets + variables, dependency-graph.
3. **Installing apps in orgs** — `POST /orgs/{org}/installations` if exposed, install URL flow, org-admin approval, repository_selection at org install time.
4. **OIDC** — Actions OIDC token issuance (`ACTIONS_ID_TOKEN_REQUEST_URL` / `_TOKEN`), JWKS endpoint, claims (sub: `repo:owner/name:ref:refs/heads/main`, audience, environment claim), cloud IdP trust contracts.
5. **Webhooks (extras)** — org-level webhooks, enterprise-level, `meta`/`security_advisory`/`secret_scanning_alert` events.
6. **Pipelines + jobs API** — full Actions REST: workflow runs / jobs / steps shape parity, logs download endpoints, artifacts download with redirect, rerun + cancel endpoints, attempt tracking.
7. **Triggering pipelines** — `workflow_dispatch` with inputs validation against `on.workflow_dispatch.inputs`, `repository_dispatch` with client_payload, event-from-API parity.
8. **Users API** — followers/following/blocked, user emails, gpg/ssh keys, status, sponsorship listing.
9. **Groups (teams + IdP)** — team membership, IdP group sync surface (`PATCH /orgs/{org}/teams/{slug}/team-sync/group-mappings`).
10. **SSO integration** — SAML SSO header on PATs, SCIM 2.0 provisioning endpoints (`/scim/v2/...`), enforced-SSO redirects.
11. **GitHub Pages** — `/repos/{o}/{r}/pages` GET/POST/PUT/DELETE + builds endpoint + deployments.
12. **Deployments + Environments** — full deployments API (statuses, deployment_status events), Environments (protection rules, reviewers, secrets, deployment branch policies), branch protection rules.
13. **Issue + PR comments depth** — issue comments full CRUD + reactions; PR review comments (inline / file-line / range), review threads, resolving + reopening threads (`POST /repos/{o}/{r}/pulls/{n}/comments/{id}/replies`, `gh pr review --thread`, `resolveReviewThread` / `unresolveReviewThread` GraphQL mutations), suggested-changes (`suggestion` syntax), comment edit history, multi-line code suggestions, threading mutations from gh CLI's PR view.
14. **Reactions API** — `/repos/{o}/{r}/issues/{n}/reactions` + `/comments/{id}/reactions` + `/pulls/{n}/comments/{id}/reactions` POST/DELETE/GET; full eight reaction types (`+1`, `-1`, `laugh`, `confused`, `heart`, `hooray`, `rocket`, `eyes`); reaction groups on Issue/PR/IssueComment/PRComment GraphQL types with real counts (today returns empty `[]`); reactions on releases + commits + discussions.
15. **Webhook events parity** — full event coverage matching real GH's webhook event index: `branch_protection_rule`, `check_run`/`check_suite` (Apps wire it; need the event delivery), `code_scanning_alert`, `commit_comment`, `create`/`delete` (branch/tag), `dependabot_alert`, `deploy_key`, `deployment`/`deployment_status`, `discussion`/`discussion_comment`, `fork`, `gollum` (wiki), `label`, `member`/`membership`, `meta`, `milestone`, `package`, `page_build`, `project`/`projects_v2*`, `public`, `pull_request_review`/`pull_request_review_comment`/`pull_request_review_thread`, `push` (already), `release`, `repository`/`repository_dispatch`/`repository_import`/`repository_ruleset`, `secret_scanning_alert`, `security_advisory`, `sponsorship`, `star`, `status`, `team`/`team_add`, `watch`, `workflow_dispatch`/`workflow_job`/`workflow_run`. Per-event payload shape verified against real GH samples. Delivery filtering honors `events` field on hook config including wildcard (`*`).

Acceptance per surface: `gh <verb>` (or octokit equivalent) round-trips against bleephub. The Phase 153 Docker harness gets new sub-blocks per surface. Each surface ships as a P154.X sub-commit on a single branch / PR.

### Phase 155 — Documentation refresh: bleephub-specific docs

After Phase 154 ships, sweep every doc that references bleephub and re-align:

- `bleephub/README.md` — fully reflect the Apps + OAuth Apps surface, persistence flag, gh CLI compatibility, and the supported subset of the GitHub webhook event index.
- `specs/BLEEPHUB_GITHUB_API_PARITY.md` — collapse "✓ shipped" rows into a closing changelog; raise the bar for what "parity" means after Phase 154.
- `docs/RUNNERS.md` § GitHub runner contract — point at bleephub for local mode; confirm runner-protocol surface still matches the official `actions/runner` v2.32x.
- `docs/runner-capability-matrix.md` — row for every Phase 153/154 capability (Apps, OAuth, Checks, Pages, Deployments, Environments, SSO).
- `ARCHITECTURE.md` — bleephub block reflects persistence + Apps subsystem.
- `ui/packages/bleephub/README.md` (if missing, create) — what each tab does, what each new dialog covers, how to seed test data.
- Add `docs/BLEEPHUB_GH_CLI.md` walking operators through `gh auth login` against a local bleephub + the supported subset of `gh` commands.

Acceptance: every claim in those docs round-trips through the Docker harness; no doc references missing endpoints or vanished UI elements.

### Phase 156 — Documentation refresh: project-wide

Sweep every other top-level doc and confirm it's still accurate after Phases 153–155:

- `README.md` (root) — module sizes, badges, quick-start, supported-clouds table, contributor section.
- `ARCHITECTURE.md` — backends inventory, driver framework (storage / network / DNS / access), sim host model, observability stack.
- `specs/CLOUD_RESOURCE_MAPPING.md` — re-verify Docker→cloud mapping per backend.
- `docs/OBSERVABILITY.md` — Stack A description, validation harness, metric / trace / log channels.
- `docs/ADMIN_ORCHESTRATION.md` — `sockerless.yaml` schema, REST surface, lifecycle endpoints, config edit + hot reload.
- `docs/MAKEFILE_STANDARD.md` — per-app Makefile contract.
- `docs/POD_MATERIALIZATION.md` — per-backend pod materialization walkthrough.
- `docs/RUNNERS.md` + `docs/GITLAB_RUNNER_DOCKER.md` + `docs/GITLAB_RUNNER_SAAS.md` + `docs/GITHUB_RUNNER.md` + `docs/GITHUB_RUNNER_SAAS.md` — runner setup paths.
- `docs/ECS_LIVE_SETUP.md` / `docs/ECS_SERVICES_DESIGN.md` / `docs/LAMBDA_EXEC_DESIGN.md` — live-cloud setup + design notes.

Acceptance: every doc page links to existing files; CLI examples copy-paste run against current `main`; no "Phase 8X TODO" placeholders remain unaddressed.

### Live-cloud validation track

Per-backend live-cloud sweeps separate from unit/sim CI. Live-AWS ECS validated 2026-04-20. Outstanding:

- Lambda live (deferred from Phase 86).
- Cloud Run Services / ACA Apps live (closed in code 2026-04-21 behind `UseService` / `UseApp`).
- AZF + cloud-dns on Azure live (new in #136).
- Lambda + service-mesh on AWS live (new in #136).
- ACA / AZF + Azure AD access on Azure live (new in #136).

One branch per cell; teardown self-sufficient per `feedback_teardown_aggressive.md`.

### Phase 91d — Real pd-ephemeral on cloudrun + gcf

**Bookmarked indefinitely.** Cloud Run's `runpb.Volume` lacks a PD field; Admin API doesn't expose PD attach as a first-class primitive. Real implementation requires either a sockerless GCE-style backend or a Cloud Run feature change. Reject-with-pointers shape (Phase 91c, PR #149) stays in place.

## Driver phase template

Storage backing (Phase 127) is the pilot. Each driver phase follows:

1. `api/<dim>_driver.go` — enum + struct fields on the relevant config.
2. `backends/core/<dim>_driver.go` — driver interface + registry + no-op default.
3. `backends/<cloud>-common/<dim>_<impl>.go` — per-cloud impl (pattern B: shared by both backends in that cloud).
4. `backends/<cloud-product>/server.go` — wires the per-cloud driver into the backend's registry at startup.
5. Operator config: env var selects the driver per backend.
6. **No-fallbacks at resolve** — unset / unknown driver name returns an error.
7. Migration of existing inline calls to the registry.

Each phase starts with a `specs/CLOUD_RESOURCE_MAPPING.md` design pass.

## Future ideas

- GraphQL subscriptions for real-time event streaming.
- Sockerless GCE-style backend (would unlock Phase 91d real `pd-ephemeral` for real workloads).
- Marketplace / billing on bleephub (currently out of scope — most apps don't use them; revisit if a real consumer asks).
