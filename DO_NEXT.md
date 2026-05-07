# Do Next

Resume pointer for next session. State: [STATUS.md](STATUS.md) · Bugs: [BUGS.md](BUGS.md) · Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md) · Roadmap: [PLAN.md](PLAN.md) · Architecture: [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Milestone closed (2026-05-07)

**8/8 runner-integration cells GREEN.** Phase 123 (storage backing driver abstraction with `gcs-sync`) shipped + 8 supporting fixes (BUG-964 / 966 / 967 / 968 / 969 / 970 / 971 + an ECS test-regression fix from a hidden no-fallbacks violation in the `handleContainerWait` fast-path). Branch `phase-118-faas-pods` is pushed; PR #123 carries the full delta.

**Live infra TORN DOWN (2026-05-07 evening)**: `sockerless-live-46x3zg4imo` and `sockerless-live-adi` both in `DELETE_REQUESTED`. GCP soft-delete enters a 30-day recovery window (`gcloud projects undelete <id>`); after that, all resources permanently gone and the project name is freed. Next live-cloud session creates a fresh ephemeral project per the `project_gcp_live_setup.md` workflow.

Cell URLs in [STATUS.md](STATUS.md). Per-bug detail in [BUGS.md](BUGS.md). Day-of narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Next architectural threads

### 1. Deploy hygiene — orphan `sockerless-svc-*` GC sweep ✅ SHIPPED 2026-05-08

Owner-linked variant landed. Sockerless self-discovers `CLOUD_RUN_JOB` (Cloud-Run-injected env var; no dispatcher-side `SOCKERLESS_*` injection, per dispatcher-generic rule) and stamps `sockerless_owner_runner_task=<jobID>` on every pod-Service it creates (cloudrun + gcf both). Dispatcher's existing 2-minute Cleanup now lists `sockerless-svc-*` Services and deletes any whose owner Cloud Run Job is gone/terminal. Code: `gcp-common/owner_label.go`, `cloudrun/servicespec.go`, `cloudrun-functions/pod_service.go`, `github-runner-dispatcher-gcp/internal/spawner/spawner.go`, `cmd/.../main.go::Cleanup`. Spec: `specs/CLOUD_RESOURCE_MAPPING.md § Orphan pod-Service GC (owner-link pattern)`. Tests + go vet GREEN; live verification deferred to next live-cloud session.

Remaining hygiene work (not blocking):
- Time-based sweep of legacy services with empty owner label (only matters once a fleet of pre-rollout services exists in the wild — today's torn-down state means there are none).
- Cloud Run Jobs older than 1h not RUNNING (separate from the Services sweep).
- GCS bucket `workspace/` prune via existing `PruneStaleObjects` driver.

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

Actual spend reported by user: **~$90** across `sockerless-live-46x3zg4imo` + `sockerless-live-adi` (both `DELETE_REQUESTED` now).

**Per-service breakdown is not available programmatically.** Researched (2026-05-07): unlike AWS Cost Explorer (which exposes a real `ce.GetCostAndUsage` API) or Azure Consumption APIs (`ActualCost` / `AmortizedCost`), **Google Cloud has no API endpoint that returns actual cost or usage data**. The Cloud Billing API surface is limited to:
- Account metadata (`/v1/billingAccounts`)
- Pricing catalog (services + SKUs — list pricing, not actual spend)
- Budget management (`/v1/billingAccounts/*/budgets`)

The only paths to itemized spend data are:
1. **BigQuery billing export** — must be enabled in advance (Phase 129's first deliverable). Free at our volume.
2. **Cloud Console Billing Reports** — manual UI access, no programmatic equivalent.

Sources: [Google Cloud Billing API reference](https://docs.cloud.google.com/billing/docs/reference/rest), [Cloud Billing data export to BigQuery docs](https://docs.cloud.google.com/billing/docs/how-to/export-data-bigquery), [Google Developer forum thread on programmatic GCP cost retrieval](https://discuss.google.dev/t/how-to-programmatically-retrieve-gcp-billing-cost-api-vs-bigquery-export/257728).

**Implication**: I will not guess at the per-service breakdown — earlier speculative tables in this section were wrong by direction (initial $10-15) and would be wrong by line-item even if the totals reconciled. The honest record is "user-reported $90 total, no per-service detail available without Console or BigQuery export". The speculation has been deleted from this doc rather than left as a future trap.

**Action: Phase 129 must ship before the next live-cloud session brings up a fresh project.** Concretely:
- **BigQuery billing export** — enable on the live billing account at fresh-project creation time, partitioned by `project_id` + `service` + `sku` + label. Free at our volume; ~MB-scale storage. This is the only programmatic source-of-truth for actual spend.
- **Per-session resource labels** (`sockerless_session=<run-id>`) on every Cloud Run Service + Job + AR repo + GCS bucket + VPC connector sockerless creates. Billing export inherits these → cost queries can filter by session cleanly.
- **Per-session budget alert** via Cloud Billing Budget API ($5 alert, $20 hard cap, scoped to label `sockerless_session=<id>`).
- **Stale-resource sweeper** integrated into the dispatcher GC tick — extend the orphan `sockerless-svc-*` cleanup (already planned) to also cover dispatcher-side `gitlab-runner-cloudrun` / `gitlab-runner-gcf` siblings during off-hours, plus VPC connector min-instance reductions.
- **Default the live-project workflow to overnight teardown** (`gcloud projects delete` end of session, fresh `sockerless-live-<rand>` next session per `project_gcp_live_setup.md`). GCP's 30-day soft-delete window is the safety net.

**For the *current* $90 attribution**: the user can pull line items from the Cloud Console at `https://console.cloud.google.com/billing/019E9E-AF0BD0-6A6F75/reports` filtered by project IDs `sockerless-live-46x3zg4imo` + `sockerless-live-adi`. The 30-day data window keeps these queryable until ~2026-06-07.

## Working notes — anything not in the above

- **Don't merge PRs** (project rule). User handles all merges.
- **Don't push `main`** — branch `phase-118-faas-pods` is the only PR-bearing branch right now.
- **Bug discipline**: any new failure surfaced during deploy-hygiene work files in [BUGS.md](BUGS.md) before any fix attempt.
- **Driver-generalization scoping**: each new phase (124/125/126) MUST start with a CLOUD_RESOURCE_MAPPING.md design pass that catalogs the current ad-hoc paths per backend, then the driver interface, then per-cloud impls, then migration. Same shape that Phase 123 used; the spec doc is the design contract before any code lands.
