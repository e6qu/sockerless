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

## Approximate cost report (this session, 2026-05-07)

No exact figure available — billing export not configured (Phase 129's first deliverable). Best-effort estimate based on resource activity:

| Cause | Estimate |
|---|---|
| Cloud Run Services pinned at min_instances=1 (~10 services × ~14 vCPU + 14 GiB across ~6h before BUG-970 fix) | $7-12 |
| Cloud Build for overlay images (~15 builds × 30-60 s) | $0.50-1.00 |
| Cloud Storage (workspace bucket, AR repos) | <$0.50 |
| Egress (mostly same-region) | negligible |
| **Session total estimate** | **$10-15** |

The bulk of cost was the `minInstanceCount=1` debt before BUG-970 surfaced. With Phase 128 + 129 in place, similar future sessions should be ~$1-2 (only active-deploy CPU + Cloud Build).

## Working notes — anything not in the above

- **Don't merge PRs** (project rule). User handles all merges.
- **Don't push `main`** — branch `phase-118-faas-pods` is the only PR-bearing branch right now.
- **Bug discipline**: any new failure surfaced during deploy-hygiene work files in [BUGS.md](BUGS.md) before any fix attempt.
- **Driver-generalization scoping**: each new phase (124/125/126) MUST start with a CLOUD_RESOURCE_MAPPING.md design pass that catalogs the current ad-hoc paths per backend, then the driver interface, then per-cloud impls, then migration. Same shape that Phase 123 used; the spec doc is the design contract before any code lands.
