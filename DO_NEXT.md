# Do Next

Resume pointer for next session. State: [STATUS.md](STATUS.md) · Bugs: [BUGS.md](BUGS.md) · Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md) · Roadmap: [PLAN.md](PLAN.md) · Architecture: [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Milestone closed (2026-05-07)

**8/8 runner-integration cells GREEN.** Phase 123 (storage backing driver abstraction with `gcs-sync`) shipped + 8 supporting fixes (BUG-964 / 966 / 967 / 968 / 969 / 970 / 971 + an ECS test-regression fix from a hidden no-fallbacks violation in the `handleContainerWait` fast-path). Branch `phase-118-faas-pods` is pushed; PR #123 carries the full delta. Live infra (`sockerless-live-46x3zg4imo`, us-central1) is retained for the deploy-hygiene work below.

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

Live project `sockerless-live-46x3zg4imo` (us-central1) is retained for the next session's deploy-hygiene work. Tear it down once the GC sweep ships and is verified — at that point both the structural fix (min_instances=0) and the cleanup safety net are in place, so the project's role as "iteration target with persistent state to debug against" ends.

Free-trial billing acct, ephemeral-project workflow per [project_gcp_live_setup.md memory](.). SA key path + dispatcher service URL + bucket names all in [STATUS.md](STATUS.md).

## Working notes — anything not in the above

- **Don't merge PRs** (project rule). User handles all merges.
- **Don't push `main`** — branch `phase-118-faas-pods` is the only PR-bearing branch right now.
- **Bug discipline**: any new failure surfaced during deploy-hygiene work files in [BUGS.md](BUGS.md) before any fix attempt.
- **Driver-generalization scoping**: each new phase (124/125/126) MUST start with a CLOUD_RESOURCE_MAPPING.md design pass that catalogs the current ad-hoc paths per backend, then the driver interface, then per-cloud impls, then migration. Same shape that Phase 123 used; the spec doc is the design contract before any code lands.
