# Do Next

Resume pointer for next session. State: [STATUS.md](STATUS.md) · Bugs: [BUGS.md](BUGS.md) · Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md) · Roadmap: [PLAN.md](PLAN.md) · Architecture: [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Milestone closed (2026-05-07)

**8/8 runner-integration cells GREEN.** Phase 123 (storage backing driver abstraction with `gcs-sync`) shipped + 8 supporting fixes (BUG-964 / 966 / 967 / 968 / 969 / 970 / 971 + an ECS test-regression fix from a hidden no-fallbacks violation in the `handleContainerWait` fast-path). Branch `phase-118-faas-pods` is pushed; PR #123 carries the full delta.

**Live infra TORN DOWN (2026-05-07 evening)**: `sockerless-live-46x3zg4imo` and `sockerless-live-adi` both in `DELETE_REQUESTED`. GCP soft-delete enters a 30-day recovery window (`gcloud projects undelete <id>`); after that, all resources permanently gone and the project name is freed. Next live-cloud session creates a fresh ephemeral project per the `project_gcp_live_setup.md` workflow.

Cell URLs in [STATUS.md](STATUS.md). Per-bug detail in [BUGS.md](BUGS.md). Day-of narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Next two architectural threads

### 1. Deploy hygiene — orphan `sockerless-svc-*` GC sweep

BUG-970's structural fix (set `minInstanceCount=0` on materialized pod-Services so failed revisions don't pin regional Cloud Run CPU quota) closes the worst of the always-on cost. But cancelled / killed pipelines still leave orphan `sockerless-svc-*` Services behind: `ContainerRemove` cleans up the underlying Service for the happy path; nothing cleans up after the runner-task itself dies. Today's session manually deleted ~8 orphans before cells 5+6 v15 could even allocate CPU.

**Concrete fix shape**: extend `github-runner-dispatcher-gcp`'s existing 2-minute cleanup ticker. It already iterates Cloud Run **Jobs** and reaps terminal executions. Add a parallel sweep that iterates Cloud Run **Services** filtered to `sockerless_managed=true` AND name prefix `sockerless-svc-`, and deletes any whose `LastUpdateTime` is older than N minutes (e.g. 30) AND whose latest revision has zero traffic / zero recent invocations. The dispatcher already has the right credentials and the right per-region scoping; this is a localized addition to `github-runner-dispatcher-gcp/internal/cleanup/`.

Alternative path (also worth considering): `ContainerRemove` already covers happy-path deletion. The orphan source is "runner-task dies before issuing ContainerRemove on its child pod-Services." A sockerless-side fix would be a hint label `sockerless_owner_runner_task=<runner-task-execution-id>`; when the dispatcher's existing cleanup notices the owner runner-task is gone (terminal state in ListExecutions), it deletes the orphan Services in the same sweep. This couples cleanup more tightly to the runner-task lifetime than a flat 30-minute idle check, so it's the better long-term shape.

### 2. Driver-generalization roadmap (Phases 124-127)

Storage backing was the pilot: cloud-agnostic core interface (`StorageBackingDriver`), per-cloud implementations (`emptyDir`, `gcs-sync`, `gcs-fuse`), operator-pluggable selection at config time (TOML `runner_workspace_backing`), and no-fallbacks discipline at registry resolve. That same shape — interface + per-cloud impls + operator-pluggable selection + no-fallbacks — is the template for the next three driver categories.

User principle (verbatim, 2026-05-07): "we want to generalize the approach to using drivers so that we can swap out pieces of backends for each backend since cloud offers a variety of things like networking, DNS, storage and access, but first we just wanted a fully working, minimally, system." 8/8 GREEN cells means we now have the working minimal system — the generalization can begin.

Roadmap entries in [PLAN.md](PLAN.md):

- **Phase 124 — Network driver abstraction.** How containers in the same user-defined network discover and talk to each other. Today: hardcoded per backend (Cloud Map for ECS, `/etc/hosts` injection via `SOCKERLESS_HOST_ALIASES` for cloudrun/gcf, multi-container revision loopback for pod-Services). Driver categories: `host-aliases`, `cloud-dns`, `service-mesh`, `nat-gateway-only`.
- **Phase 125 — DNS driver abstraction.** How `<container-name>.<network>` resolves. Today: per-cloud heuristics. Driver categories: `cloud-map`, `cloud-dns-zone`, `service-discovery`, `private-dns-zone`.
- **Phase 126 — Access driver abstraction.** Container-to-container auth, ingress IAM, service-account binding. Today: scattered. Driver categories: `iam-role`, `id-token`, `mTLS`, `none-internal`.
- **Phase 127 — Storage driver expansion (NICE-TO-HAVE).** Open up the `BackingSpec` union (currently EmptyDir + GCS) to be cloud-agnostic. New drivers (`pd-ephemeral`, `efs-ephemeral`, `azure-files-ephemeral`) plug in without core-package changes.
- **Phase 128 — Runner job timeout (configurable).** Hard cap on Cloud Run Job / Lambda / ECS task duration so a hung subprocess can't pin quota indefinitely. Default 1 h; operator override via dispatcher TOML `runner_job_timeout` + bootstrap env `SOCKERLESS_JOB_TIMEOUT_SECONDS`. SIGTERM → 30 s grace → SIGKILL; bootstrap reports exit code 124 (matches GNU `timeout(1)`). Detail in PLAN.md.
- **Phase 129 — Cost tracking + stale-resource cost-cap.** BigQuery billing export, per-session resource labels (`sockerless_session=<run-id>`), per-session budget alerts ($5 alert / $20 hard cap), and a stale-resource sweeper that extends the dispatcher GC ticker. Detail in PLAN.md. Today's session demonstrated the gap — orphan services from cancelled runs pinned regional CPU quota (BUG-970) without any cost visibility; fixing this is essential before scaling up live-cloud iteration.

Each phase follows the Phase 123 template:

1. `api/<dim>_driver.go` — enum + struct fields on the relevant config (`SharedVolume`, `Network`, etc.).
2. `backends/core/<dim>_driver.go` — driver interface + registry + `EmptyXxx` (no-op default for backends that don't need the dimension).
3. `backends/<cloud>-common/<dim>_<impl>.go` — per-cloud driver impls.
4. `backends/<cloud-product>/<dim>_translator.go` — per-backend translator that maps driver output to that cloud's protobuf.
5. Operator config: TOML / env var that selects the driver per backend.
6. **No-fallbacks at resolve**: unset / unknown driver name returns an error, never silently picks a default.
7. Migration of existing inline calls to use the registry.

Same single-PR-per-phase rule as Phase 123.

## Live-cloud followup

Live projects torn down end of this session. Next live-cloud session creates a fresh `sockerless-live-<rand>` per `project_gcp_live_setup.md` workflow. **Before bringing the next project online, ship Phase 128 (job timeout) + Phase 129 (cost tracking + stale-resource sweeper) FIRST** — without those, the same regional-CPU-quota debt cycle from today's session repeats.

## Cost report — 6-day project lifetime (2026-05-01 → 2026-05-07)

Actual spend reported by user: **~$90**. Initial estimate of $10-15 was wrong because it assumed a single-session footprint; the live projects (`sockerless-live-46x3zg4imo` + `sockerless-live-adi`, both `DELETE_REQUESTED` now) actually existed for 6 days. Best-effort post-hoc breakdown:

| Cost driver | Estimated range |
|---|---|
| `gitlab-runner-cloudrun` Service (multi-container revision, min_instances=1, ~3 days) | $10 |
| `gitlab-runner-gcf` Service (multi-container, min=1, ~5 days) | $13 |
| `github-runner-dispatcher-gcp` Service (single-container, min=1, ~5 days) | $3 |
| Orphan `sockerless-svc-*` from failed cells-5/6 iterations (10+ services × min=1 across varying intervals — the BUG-970 root cause) | $20-40 |
| VPC connector `sockerless-connector` (min-instances=4 per BUG-947 fix, 6 days, e2-micro instances) | $5 |
| Cloud Run Functions / gcf pool warming + per-step deploys | $5-10 |
| Cloud Build (~80 overlay builds + dispatcher + standalone backend rebuilds) | $1-2 |
| Artifact Registry storage (100+ overlay tags) | $1-3 |
| Cloud Logging ingestion (heavy debug logs across 17 cell iterations × multi-container revisions) | $5-15 |
| Cloud NAT egress (image pulls through proxy) | $1-2 |
| **Total range** | **$60-100** (consistent with $90 reported) |

**Where the money goes** (lessons for cost-tracking):
1. **`min_instances=1` is the killer**. Always-on multi-container revisions silently run 24/7 across the project lifetime. Three "core infra" services (`gitlab-runner-cloudrun`, `gitlab-runner-gcf`, `github-runner-dispatcher-gcp`) alone are ~$5-7/day. Orphan `sockerless-svc-*` add another $5-10/day across the worst cell-5/6 iteration windows. Phase 129 deploy-hygiene + min_instances=0 (already shipped via BUG-970 fix on pod-Services) covers this — but the dispatcher-side services still need to either drop to min=0 or auto-tear-down between sessions.
2. **Project lifetime > session length**. We treat live projects as iteration targets retained across sessions. That convenience has a daily-cost floor; per-session teardown (or scheduled overnight teardown) cuts the floor to zero.
3. **Cloud Logging ingestion** is the second-largest line item. Per-line debug logging across 17 cell iterations × multi-container revisions adds up. A log-level-control env (`SOCKERLESS_LOG_LEVEL=info` instead of `debug` in production) on the dispatcher + backend reduces ingest by 5-10×.
4. **VPC connector min-instances** is bookmarked overhead — useful for keeping cross-Cloud-Run latency low, but $0.80/day for min=4 across the project's life.

**Action: Phase 129 must ship before the next live-cloud session brings up a fresh project.** Concretely:
- BigQuery billing export (free at our volumes, gives line-item-by-day visibility).
- Per-session resource labels (`sockerless_session=<id>`) on every Cloud Run Service / Job / AR repo / GCS bucket / VPC connector.
- Per-session budget alert ($5 alert, $20 hard cap).
- Stale-resource sweeper integrated into the dispatcher GC tick (already partially planned for orphan `sockerless-svc-*`; expand to include dispatcher's own siblings + connector idle-min during off-hours).
- Default the live-project workflow to overnight teardown (`gcloud projects delete` end of session, fresh `sockerless-live-<rand>` next session per `project_gcp_live_setup.md`).

## Working notes — anything not in the above

- **Don't merge PRs** (project rule). User handles all merges.
- **Don't push `main`** — branch `phase-118-faas-pods` is the only PR-bearing branch right now.
- **Bug discipline**: any new failure surfaced during deploy-hygiene work files in [BUGS.md](BUGS.md) before any fix attempt.
- **Driver-generalization scoping**: each new phase (124/125/126) MUST start with a CLOUD_RESOURCE_MAPPING.md design pass that catalogs the current ad-hoc paths per backend, then the driver interface, then per-cloud impls, then migration. Same shape that Phase 123 used; the spec doc is the design contract before any code lands.
