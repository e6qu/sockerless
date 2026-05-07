# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform. Designed to power CI runners (GitHub Actions + GitLab Runner) on cloud serverless capacity — see [docs/RUNNERS.md](docs/RUNNERS.md).

See [STATUS.md](STATUS.md) for the current phase roll-up, [BUGS.md](BUGS.md) for the bug log (per-bug fix detail), [PLAN.md](PLAN.md) for the roadmap, [DO_NEXT.md](DO_NEXT.md) for the resume pointer, [specs/](specs/) for architecture (start with [specs/SOCKERLESS_SPEC.md](specs/SOCKERLESS_SPEC.md), [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md), [specs/BACKEND_STATE.md](specs/BACKEND_STATE.md), [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md)).

This file keeps narrative / "why we did it" context that doesn't live in BUGS.md or git log. Per-bug detail belongs in [BUGS.md](BUGS.md) — don't duplicate it here.

## 2026-05-08 — Phase 129 #4 owner-linked orphan-Service sweep

Picked up the first deploy-hygiene thread from DO_NEXT.md: extend the dispatcher's 2-minute Cleanup ticker to reap orphan `sockerless-svc-*` Services left behind when a runner-task dies before issuing ContainerRemove on its child pod-Services. Today's session shipped the **owner-linked variant** that DO_NEXT.md called out as the better long-term shape — couples cleanup to the runner-task lifetime instead of a flat idle-time check.

### How the owner link works

The dispatcher-generic rule (`feedback_dispatcher_generic.md`) forbids the dispatcher from injecting any `SOCKERLESS_*` env into the runner-task. So the owner identifier sockerless writes on each pod-Service has to be discovered sockerless-side, from an ambient cloud signal — not pushed by the dispatcher.

1. **Cloud Run runtime auto-injects `CLOUD_RUN_JOB`.** Every Cloud Run Job execution gets `CLOUD_RUN_JOB=<jobID>` set automatically (the trailing segment of the Job's resource name, e.g. `gh-abc1234-123456`). No opt-in, no dispatcher cooperation.

2. **Sockerless self-discovers + stamps the label.** Both Cloud Run paths (`backends/cloudrun/servicespec.go::buildServiceSpec` and `backends/cloudrun-functions/pod_service.go::materializePodService` + `deployContainerService`) call the new `gcp-common/owner_label.go::OwnerRunnerTaskLabelValue` helper which reads `CLOUD_RUN_JOB` and sanitizes to GCP label-value charset (`[a-z0-9_-]`, 63 chars). When non-empty, the Service gets `sockerless_owner_runner_task=<jobID>` in its GCP labels. Outside a Cloud Run Job (laptop, sim, regular Service), the env var is unset and no label is written — the dispatcher's sweep then leaves those Services alone.

3. **Dispatcher cleanup reaps orphans whose owners are gone or terminal.** `cmd/github-runner-dispatcher-gcp/main.go::Cleanup` now (a) builds a set of live-owner Job IDs from the existing ListManaged result, (b) calls the new `spawner.ListManagedServices` to enumerate `sockerless-svc-*` Services in each (project,region), (c) reads each Service's `sockerless_owner_runner_task` label, (d) calls `spawner.DeleteService` on any Service whose owner is not in the live set. Services with empty owner labels are deliberately left alone (legacy / non-Cloud-Run-Job sockerless); a flat idle-time sweep is the right tool for those (Phase 129 #4 second deliverable, not yet shipped).

### Why owner-linked, not idle-time

DO_NEXT.md evaluated both:
- **Flat 30-min idle check** — easier; risks false-positives on real long-running pod-Services with sporadic traffic patterns.
- **Owner-linked** — strictly correct (a Service whose owner Cloud Run Job is gone is *definitely* orphan); requires plumbing an identifier through dispatcher → runner → sockerless → pod-Service.

The plumbing is small (one env var + one label + one ListServices call) and produces a precise GC signal. The flat-time check is still useful for legacy services that pre-date this label, but doesn't need to be primary.

### Tests + verification

- `backends/gcp-common/owner_label_test.go` — `sanitizeOwnerLabel` charset/length, env-var-unset/set behaviour.
- `github-runner-dispatcher-gcp/internal/spawner/spawner_test.go` — `JobIDFromName` extraction, deterministic re-derivation of jobID from runner name (the property the cleanup join relies on).
- `go test ./...` GREEN in: `backends/gcp-common`, `backends/cloudrun`, `backends/cloudrun-functions`, `github-runner-dispatcher-gcp`.
- `go vet ./...` clean across the same modules.

Live verification deferred until next live-cloud session (per DO_NEXT.md): the live infra was torn down end of 2026-05-07, and Phase 128 (job timeout) + the rest of Phase 129 should ship before the next ephemeral project comes up.

## 2026-05-07 — Phase 123 + 8/8 cells GREEN (milestone closed)

The 17-iteration cells-5+6 saga ended today. Phase 123 (storage backing driver abstraction with `gcs-sync`) shipped, cells 5+6 went GREEN at v17, the 8/8 runner-integration milestone closed. Per-bug fix detail in [BUGS.md](BUGS.md); cell URLs in [STATUS.md](STATUS.md).

### Phase 123 architecture (what landed)

**`gcs-sync` data plane.** Replaces FUSE-on-object-store for shared workspaces. The runner-task tars `/tmp/runner-work` to a per-exec GCS object before forwarding the exec POST; the JOB pod-Service bootstrap untars from the same object before running the subprocess, then tars the modified workspace back; the runner-task untars on response. Pure GCS SDK calls — no FUSE in the data path. Per-step granularity matches GH actions/runner's per-step script pattern.

**The `SOCKERLESS_SYNC_MOUNTS` / `SOCKERLESS_SYNC_VOLUMES` split.** Two distinct env vars carrying two distinct lists, joined by name at the bootstrap:
- `SOCKERLESS_SYNC_MOUNTS=name=mountpath` — injected at materialize time on the JOB main container's spec; lists which named volumes the bootstrap should sync, paired with where they're mounted in the container.
- `SOCKERLESS_SYNC_VOLUMES=name=gs://bucket/object` — injected per-exec via the envelope's `Env` field; lists the per-call GCS objects (one per exec).

The materializer looks up each `SharedVolume` by name (binds are friendly-named at ContainerCreate). Earlier iterations of BUG-967 conflated the mount-list with the object-list, which broke the moment runner-externals + runner-workspace co-existed; the split fixes that cleanly.

**No-fallbacks registry.** `core.StorageBackingRegistry.Resolve(unknown)` and `Resolve(empty)` return errors, never silently pick a default. Every `SharedVolume` MUST have an explicitly-set `Backing`. Cells 7+8 explicitly set `gcs-fuse`; cells 5+6 explicitly set `gcs-sync`. This is the no-fallbacks discipline applied at the registry boundary — silent default selection would mask operator misconfiguration.

### Bug-by-bug closeout

Each fix shipped landed in the same session it surfaced (no-defer rule). Most surfaced AFTER Phase 123 landed because the new sync data plane exposed materialize-time spec details that the old GCSFuse path hid.

- **BUG-964** (commit `48d5b37`): gcf `invokePodServiceMain` gained a `skipIfNoStdin` param. When OpenStdin=false runner-pattern containers materialize as a pod-Service main, do NOT default-invoke — that would run the long-lived JOB CMD as a one-shot subprocess and block forever on `invokeMu`. Mirror of cloudrun BUG-961. The bootstrap stays on its HTTP listener for subsequent `/exec` POSTs.
- **BUG-966** (commit `2591964`): drop `WorkingDir` from the JOB pod-Service container spec. Cloud Run validates `WorkingDir` exists at startup, but under `gcs-sync` the workspace volume is empty until the bootstrap restores from GCS. The bootstrap chdir's per-exec via `envelope.Workdir`; no need to set it at the container level.
- **BUG-967** (3 iterations: `e286ba8` → `05c0ecd` → `3ca6614`): the `SOCKERLESS_SYNC_MOUNTS` / `SOCKERLESS_SYNC_VOLUMES` split landed only after two earlier shapes failed. v1: single env var conflating mount paths and GCS objects (broke multi-volume). v2: name-keyed env carrying both, but per-exec env couldn't reference materialize-time mount paths. v3: split, with the materializer looking up SharedVolume by name (not source path) so binds + named volumes both work.
- **BUG-968** (commit `4dc8cdc`): cloudrun's `OverlayContentTag` keyed only on bootstrap PATH, not content — the AR cache hit forever, deploying yesterday's bootstrap inside fresh containers. gcf had been fixed by BUG-957 but cloudrun was missed in that round. Fix: hash the binary at server startup, stamp `BootstrapBinaryHash` into every `OverlayImageSpec`. Same shape as the gcf fix; applied verbatim to `backends/cloudrun/server.go::NewServer`.
- **BUG-969** (commit `d20cb38`): cloudrun's default `mapCPUMemory` was `512Mi`/container — too small for multi-container revisions where the postgres sidecar's `initdb` OOMs at ~700Mi. Bumped to `1Gi`/container to match gcf's default. Cells 5+6 then progressed to `clone-and-compile`.
- **BUG-970** (today): Cloud Run multi-container pod-Service startup probe failure. The misleading "container failed to bind PORT=8080" took 4 hypotheses + a manual `gcloud run services replace` of an identical multi-container spec to disprove — the actual root cause was regional CPU quota exhaustion. Failed/cancelled prior runs left orphan `sockerless-svc-*` services with `minInstanceCount=1` (always-on), each pinning 1-2 CPUs of regional quota. Same family as BUG-942. Structural fix: materialize sets `minInstanceCount=0` so failed revisions don't pin CPU. Followup work (orphan GC sweep) tracked in [DO_NEXT.md](DO_NEXT.md).
- **BUG-971** (today): after BUG-969 bumped per-container memory to 1Gi, cell 5+6 reached `clone-and-compile` where Go compilation alongside the postgres sidecar still OOMed at ~1.5Gi peak. The lesson: in multi-container revisions, the **sum** matters, not the per-container limit. Fix: bump main container to 2Gi (postgres stays at 1Gi); per-cell memory budget set in `materializePodService`.

### ECS test regression caught + fixed (commit `a521ac4`)

Earlier in the branch I had added a `handleContainerWait` fast-path for backends that store `InvocationResult` directly. The fast-path included a fallback: if `WaitChs` was registered but no `InvocationResult` ever showed up, return exit code 0. ECS registers `WaitChs` (per-cycle for the gitlab-runner pattern) but never stores `InvocationResult` — exit codes come from `CloudState.WaitForExit` querying actual cloud-side state. So when the WaitCh closed (cycle done), the fast-path returned 0 instead of querying ECS.

`TestECSArithmeticInvalid` and `TestArithmeticNonZeroExit/ecs` both failed: the binary correctly exited 1, but the `/wait` endpoint reported 0. Fix: drop the WaitCh-only branch from the fast-path entirely. Backends without `InvocationResult` fall through to the existing `CloudState`-driven path which queries actual cloud-side state.

The original fast-path had a hidden silent fallback that violated the no-fallbacks discipline. We **deleted the fallback rather than expanding it** — the right shape per the project rule.

### Things tried that did NOT work (anti-recipes)

Per the user-stated workflow rule, document REJECTED approaches so future sessions don't relitigate:

| Attempt | Why rejected |
|---|---|
| **GCSFuse for new SharedVolumes** with extra mount options (`--rename-dir-limit=10000 --metadata-cache-ttl-secs=0`) | BUG-965 stale-handle on per-step `event.json` rewrites by GH actions/runner. Mount options can mask but not eliminate stale-handle on rewrites. User directive 2026-05-07: no FUSE-on-object-store for new SharedVolumes. |
| **NFS / Filestore / Memorystore / persistent-mode PDs** | All bill idle. User directive 2026-05-07: zero-scaling, no-cost-when-not-in-use is absolute. Filestore = $160/mo always-on; Memorystore Redis = $50/mo; PDs bill ~$0.04/GiB/mo idle. |
| **`minInstanceCount=1` on pod-Services** | BUG-970 proved this pins regional CPU quota across the entire pipeline lifetime. Failed / cancelled runs leave orphan Services holding 1-2 vCPUs each. After ~6 orphans we'd exhaust regional quota and every subsequent materialize fails with the misleading "PORT=8080" startup error. Structural fix: `minInstanceCount=0`. |
| **Cloud Run startup-probe debug as the failure root cause** | Red herring. Spent 4 hypotheses + a manual `gcloud run services deploy` of the EXACT failing overlay image (which succeeded in <5s locally) before pinning the actual cause to regional CPU quota. The startup-probe error message Cloud Run returns when CPU is unavailable is identical to the one for genuine bind-port failures. Cost: ~3 hours. |
| **Single-container memory bump alone** (BUG-969 fix) | Got cells past `initdb` (postgres sidecar) but BUG-971 surfaced because the SUM matters in multi-container revisions, not the per-container limit alone. Need per-container budgets that account for the sidecar mix. |
| **`handleContainerWait` fast-path with WaitCh-only fallback** | Hidden silent fallback that masked actual exit codes for backends without `InvocationResult` (ECS). Deleted the fallback rather than expanding it — `CloudState.WaitForExit` is the right path for those backends. No-fallbacks discipline at the wait endpoint. |

### Lessons learned

1. **The `gcs-sync` driver pattern is the proven template** for the wider driver-generalization track (Phases 124-127 in PLAN.md). Cloud-agnostic core interface + per-cloud impls + operator-pluggable selection at config time + no-fallbacks discipline at registry resolve. Same shape applies to network, DNS, access drivers.

2. **Misleading error messages cost real time when the actual layer is invisible.** Cloud Run's "PORT=8080" startup error means "container couldn't bind in time" OR "couldn't allocate CPU at all" depending on the underlying cause. Without explicit error differentiation in the cloud's response, only side-channel evidence (manual `gcloud run services replace` of the identical spec; checking quota usage in console) pins the real cause. Lesson: when a "container startup" error doesn't correlate with the container's actual startup behavior locally, suspect a control-plane / quota issue before further bootstrap debugging.

3. **No-fallbacks discipline catches its own violations.** The ECS test regression (BUG: `handleContainerWait` fast-path returning 0 from WaitCh-loaded-but-no-InvocationResult) was a subtle violation that landed silently and surfaced 6 commits later as test failures. Project rule held: delete the fallback, don't expand it. The right path (CloudState-driven wait) was already there; the fast-path just needed to know when not to apply.

4. **Multi-container revision cost is the sum.** BUG-969 + BUG-971 chain showed that per-container memory limits in Cloud Run revisions are individual ceilings, but the sum determines the revision's actual capacity. For our pod-Service shape (golang main + postgres sidecar), the sum got us OOMing during `go build`. Per-cell memory budgets need to factor the sidecar mix.

5. **Cells 5+6 cascade of layered failures** (BUG-959 → 960 → 961 → 962 → 963 → 964 → 965 → 966 → 967 → 968 → 969 → 970 → 971) reinforced the value of the no-fakes / cross-cloud-parity rules: each fix surfaced the next layer immediately because no fallbacks were masking earlier failures. 17 iterations across multiple sessions, 13 architectural fixes, but no rounds were spent re-exploring problems we'd already pinned.

### Multi-day cells-5+6 saga summary

| Iteration | Date | Surfaced | Closed |
|---|---|---|---|
| v1-v2 | 2026-05-03 | — | infrastructure-only |
| v3-v5 | 2026-05-04 | BUG-959/960/961/962/963 | (BUG-959/960/961/962 closed in v5) |
| v6 | 2026-05-04 | BUG-964/965 | BUG-963 closed |
| v7-v9 | 2026-05-05 | (no progress; iterating on BUG-964/965 fix shape) | — |
| v10-v12 | 2026-05-06 | (planning Phase 123 driver abstraction) | — |
| v13 | 2026-05-07 | BUG-966 | BUG-964 closed |
| v14 | 2026-05-07 | BUG-967 (3 iters: v14a/b/c) | BUG-966 closed |
| v15 | 2026-05-07 | BUG-968 | BUG-967 closed |
| v16 | 2026-05-07 | BUG-969 | BUG-968 closed |
| v17 | 2026-05-07 | BUG-970/971 | BUG-969/970/971 closed; **cells 5+6 GREEN** |

13 architectural fixes total across the saga. No fixes deferred; no fallbacks introduced. Every layer of failure produced a same-session structural fix, with cross-cloud parity sweeps catching the equivalent-fix-needed-on-the-other-cloud cases (BUG-958 ↔ BUG-955; BUG-961 ↔ BUG-964; BUG-957 ↔ BUG-968).

## Phase 123 planning session (2026-05-07 — storage backing driver abstraction defined; cells 5+6 staged)

Pivoted from "fix GCSFuse mount options" / "swap to Filestore NFS" to a proper driver abstraction after the user directives.

### What worked (planning + design)

1. **Driver abstraction designed.** `api.StorageBacking` enum + `core.StorageBackingDriver` interface + per-cloud `BackingSpec → cloud Volume` translator. Replaces vestigial `core.StorageDriver` + `api.VolumeDriver` shells (both unused today). Two-layer separation: cloud volume spec (per-cloud translator) + data-plane sync (`PreExec` / `PostExec` hooks for sync-style backings only). See [specs/CLOUD_RESOURCE_MAPPING.md § Storage backing driver abstraction](specs/CLOUD_RESOURCE_MAPPING.md#storage-backing-driver-abstraction-planned--phase-123).

2. **`gcs-sync` chosen as the default backing for shared workspaces.** The runner-task tars `/tmp/runner-work` to a GCS object before forwarding the exec POST; the JOB pod-Service bootstrap untars before subprocess + tars after; runner-task untars on response. Pure GCS SDK calls, no FUSE in the data path, scale-to-zero, $0 idle cost. Implementation reuses existing `persist.go` GCS helpers.

3. **Driver matrix documented with explicit rejection list.** Every option weighed against the directives: `emptyDir` ✓, `gcs-sync` ✓ (new default), `gcs-fuse` ✓ (legacy retain for cells 7+8). Bookmarked: `pd-ephemeral`, `efs-ephemeral` (sockerless-managed lifecycle, scale-to-zero). Rejected: `nfs`, `juicefs+Redis`, `pd-persistent`, `efs-persistent` — all bill idle.

### What we tried that did NOT work (anti-recipes for future)

| Attempt | Why it failed |
|---|---|
| **GCSFuse mount with extra options** (`--rename-dir-limit=10000 --metadata-cache-ttl-secs=0`) — proposed earlier as cheap BUG-965 mitigation | User directive 2026-05-07: **no FUSE-on-object-store for new SharedVolumes.** Mount options can mask but not eliminate stale-handle on rewrites. Rejected as the design path. |
| **Cloud Filestore NFSv3 via `Volume{Nfs}`** — proposed 2026-05-07 morning | User directive 2026-05-07: **zero-scaling, no-cost-when-not-in-use.** Filestore BASIC_HDD ≈ $160/mo always-on. Rejected. |
| **JuiceFS POSIX-on-GCS with Memorystore Redis metadata** — considered as the "have your cake and eat it" option | Memorystore Redis ≈ $50/mo always-on. Violates the directive. |
| **Cloud Run native PD (RWO) attach** — single-writer would have been fine for our pattern | Cloud Run mounts the disk for the lifetime of the revision; even idle, the disk bills. Persistent-mode PDs rejected. (Sockerless-managed ephemeral PDs — created at job start, deleted at end — remain on the bookmarked list.) |
| **GCSFuse extension to use `--implicit-dirs --rename-dir-limit=10000`** | Same root cause as plain GCSFuse; ineffective for `event.json` rewrites. |
| **Inline NFS swap (~30 LOC) without driver abstraction** — proposed first as faster path | User: "we should also implement a storage driver abstraction of sorts so that we can potentially test out and swap out multiple storage options without refactoring each time." Pivoted to full Phase 123 design. The driver abstraction is the deliverable, not a side effect. |

### Lessons learned

1. **"Zero-scaling, no-cost-when-not-in-use" is a hard architectural constraint, not a preference.** Every backing was forced through the lens "does this bill idle? if yes, reject." That filter eliminated all the FUSE + NFS + always-on alternatives in a single pass and left object storage as the only candidate for cross-Service shared state. Documenting the reject list explicitly in CLOUD_RESOURCE_MAPPING.md prevents future "let's try Filestore" relitigation.

2. **Pluggable driver abstraction beats ad-hoc swap.** The user's instinct to build the abstraction *first* (rather than inline-rewrite the call sites) means future options (`pd-ephemeral`, `efs-ephemeral`, `juicefs` if its cost shape ever changes) become TOML config changes — no Go refactor. The cost is ~1300 LOC + tests today, paid back the first time we swap.

3. **Cells 5+6's cascade of layered failures** (BUG-959 → 960 → 961 → 962 → 963 → 964 → 965) showed the value of the no-fakes / cross-cloud-parity rules: each fix surfaced the next layer immediately because no fallbacks were masking earlier failures, and each layer's fix landed with the same cross-cloud mirror (BUG-958 ↔ BUG-955; BUG-961 ↔ BUG-964 pending). Phase 123 puts storage on the same footing — one abstraction, multiple drivers, swap by config.

## Phase 122m fifth session (2026-05-06 PM — cells 5+6 progressed through 5 architectural blockers)

Continued from the fourth session. Cells 7+8 GREEN at start; goal: close cells 5+6 (GH × GCP). Each iteration peeled the next layered blocker.

### What worked (5 architectural fixes shipped on top of BUG-956/957/958)

1. **BUG-959 — GH actions/runner pattern materializes pod-Service on second-arrival.** GH actions/runner creates the JOB container with `OpenStdin=false` (long-lived `tail -f /dev/null`-style entrypoint, runner does `docker exec` per step) — opposite of gitlab-runner's `OpenStdin=true` script-runner. Our `shouldDeferOrMaterializeNetworkPod` always deferred OpenStdin=false containers waiting for an OpenStdin=true sibling that never arrives → both job + postgres deferred forever, no pod-Service materialized. Fix (gcf + cloudrun, identical shape): when current OpenStdin=false has siblings already deferred, materialize with `siblings[0]` (first-arrived = JOB container) as main + current as sidecar. Pass `netMembers[0].ID` to materialize call as authoritative main so Service naming + allocation labels point at the JOB container.

2. **BUG-960 — `Typed.Exec` routes through `s.ExecStart` so envelope-POST is reachable.** Both gcf + cloudrun's `Typed.Exec` was wired to `WrapLegacyExec(s.Drivers.Exec, ...)` (reverse-agent only), bypassing the `s.ExecStart` override that has the `execStartViaInvoke` envelope-POST fallback. The pod-Service runs in Cloud Run; the runner-task is a Cloud Run Job with no public URL — bootstrap can't dial back to register a reverse-agent. Fix: `Typed.Exec = WrapLegacyExecStart(s.ExecStart, ...)`. Plus a sub-fix in `sanitizeServiceContainerName` — re-trim trailing non-alphanumeric AFTER the 50-char cut so names like `<32hex>-postgres16alpine-` (cut lands on hyphen) become valid RFC-1123.

3. **BUG-961 — cloudrun `invokeServiceDefaultCmd` skip-default-invoke when no stdin captured.** Cell 5 v4 hung 10 min: invokeServiceDefaultCmd POSTed an empty body → bootstrap ran the GH JOB container's long-lived `tail -f /dev/null`-style env CMD as a one-shot subprocess → blocked `invokeMu` forever → subsequent /exec POST sat queued until HTTP timeout. Fix: `skipIfNoStdin bool` param. `startSingleContainerService` passes `false` (single-container `docker run` should default-invoke); `startMultiContainerServiceTyped` passes `true` (pod-Service mode skips default-invoke when no stdinPipe captured, leaving the bootstrap listening for /exec POSTs). gitlab-runner attach-stdin pattern (cell 7) registers `stdinPipe` BEFORE the goroutine fires, so `capturedStdin > 0` → still default-invokes with stdin envelope. Plus: `handleInvoke` ENTRY logging in both gcf + cloudrun bootstraps for diagnosis.

4. **BUG-962 — exec response stdcopy stream framing.** gcf+cloudrun's `execStartViaInvoke` returned plain bytes; docker exec non-TTY expects 8-byte stdcopy stream-frame headers `[stream_type, 0, 0, 0, len_be32]`. Cell 6 v4 reported `Unrecognized input header: 115` (= `'s'` from `sockerless-...` stderr). Fix: wrap stdout in `0x01` frame + stderr in `0x02` frame via existing `writeMuxFrame` helper (already in `attach_stream.go` for the attach path).

5. **BUG-963 — dispatcher attaches `Volume{Gcs}` to runner-task `/tmp/runner-work`.** Both cells v5 reached `docker exec` but failed `sh: can't open /__w/_temp/<uuid>.sh: No such file or directory`. Architecture: GH actions/runner writes step scripts to `$RUNNER_WORK/_temp/<uuid>.sh` (= `/tmp/runner-work/_temp/...`), then `docker exec sh /__w/_temp/...`. Sockerless's bind-mount → GCS-volume translation makes the JOB pod-Service mount `/__w` via GCSFuse on the workspace bucket. But the runner-task's `/tmp/runner-work` was plain tmpfs — writes never reached the bucket. Fix: dispatcher TOML's `Label` gains `runner_workspace_bucket`; `spawner.Spawn` adds `Volume{Gcs{Bucket}}` + `VolumeMount{/tmp/runner-work}` to the runner-task Cloud Run Job spec. Cloud Run native GCSFuse mount avoids gcsfuse-CLI which needs CAP_SYS_ADMIN. Operator-side infra config — dispatcher itself stays sockerless-unaware. Cells v6 evidence: cell 5 reached `clone-and-compile`, cell 6 ran 10 min before BUG-964 timeout.

### What we tried that did NOT work

| Attempt | Why it failed |
|---|---|
| **`gh workflow run cell-{5,6}-*.yml --repo e6qu/sockerless`** | HTTP 422 — `workflow_dispatch` only resolves on the default branch (main). Cell 5+6 ymls aren't on main yet. Resume path: PR #124 trigger via `pull_request` event with `paths: [.github/workflows/cell-N-*.yml]`. |
| **Trigger cells 5+6 in parallel without aggressive cleanup** | Cells v3 hit `Quota exceeded for total allowable CPU per project per region` at materialize time. Cleanup `sockerless-svc-*` + `skls-*` services + cancel stale `gh-*` Cloud Run Job executions before each run. |
| **Trigger cell 5 alone by editing only cell-5.yml** | Both workflows fired — `pull_request: paths` filters apply to PR cumulative diff, not individual push diff. PR #124's accumulated diff touches both ymls so both fire on every push. Mitigation: live with parallel + aggressive cleanup. |
| **Pruning docker images via `docker system prune`** | "0B reclaimed" — the `docker` CLI on this Mac is fronted by Podman. `podman system prune -a -f` reclaimed 130 GB after the build hit `no space left on device` mid-iteration. |
| **BUG-960 sanitize 50-char truncation without trailing trim** | After truncation, the cut sometimes landed on a `-` (e.g. `f6026fc66bd94699a1410de4c96b141e_postgres16alpine_0711a7` → 56 chars → cut to 50 → ends in `-`). Cloud Run RFC 1123 rejected. Added re-trim AFTER the cut. |
| **Initial BUG-959 fix: filter ALL containers with FunctionURL** | Sent the new stage's container to the single-container path with `netMembers=0`. Single-container path doesn't drain stdinPipe → step_script script never reached the bootstrap → 60s+ hang. Refined to "filter only OpenStdin=true mains" — keeps sidecars in the member list so each stage's pod-Service revision gets its own postgres copy. |

### Lessons learned

1. **Each cell teaches a different fault line.** Cell 7 (gitlab-runner × cloudrun) needed BUG-958. Cell 8 (gitlab-runner × gcf) needed BUG-956 + BUG-957. Cells 5+6 (GH × GCP) needed BUG-959 + BUG-960 + BUG-961 + BUG-962 + BUG-963 (and now BUG-964 + BUG-965). The "cross-cloud parity sweep" rule (file the cross-cloud equivalent as a same-session sub-task) saves rounds — many of these would have been caught earlier if applied consistently.

2. **GH actions/runner has a fundamentally different docker pattern from gitlab-runner.** gitlab-runner uses `OpenStdin=true` + attach-stdin; the build container is the script-runner. GH actions/runner uses `OpenStdin=false` + long-lived JOB container + `docker exec` per step. Sockerless's network-pod logic was originally tuned for gitlab-runner. Each step of the GH-port revealed a different assumption: who's the "main" (BUG-959), where stdin gets handled (BUG-960/961), how the response is framed (BUG-962), and how the workspace propagates (BUG-963).

3. **Layered architectural fixes look like bug whack-a-mole until you map them.** A bug filed today might unmask 2 more. The discipline: file each layer immediately, ship the fix, observe what surfaces next. Today: 8 bugs filed, 6 closed, 2 staged for next session.

## Phase 122m fourth session (2026-05-06 — cells 7 + 8 GREEN; cells 5 + 6 stack ready)

Continuation of Phase 122k. Cell 8 reached v25 with 4/5 stages GREEN at start of session; final blocker was BUG-956 (multi-image-per-stage materialize race). Today landed three architectural fixes that closed the multi-image-stage gap on both gcf AND cloudrun, plus the bootstrap persist gap that surfaced after the gcf architectural fix landed.

### What worked

1. **BUG-956 — `pendingMembersOfNetwork` filters already-materialized OpenStdin=true mains.** gitlab-runner v17 docker executor spawns a NEW build container per stage with a different image (helper for prep/get_sources, user image for step_script). Before the fix, the new container's network-pod detection saw the OLD main still in PendingCreates and tried to re-materialize a 3-member pod-Service — chaos. After the fix, the OLD main is filtered (resolveGCFFromCloud returns its FunctionURL → it's already in another pod-Service). Sidecars (postgres, OpenStdin=false) stay in the member list so each stage's pod-Service revision gets its own postgres copy — Cloud Run revision loopback semantics are per-revision, so this is required. v27 trace confirmed the architecture worked: step_script's NEW container materialized into its own pod-Service alongside postgres sidecar and ran its envelope. Code: `backends/cloudrun-functions/network_pod.go::pendingMembersOfNetwork`.

2. **BUG-957 — gcf bootstrap got the BUG-947 tar-pack persist module + content-hash overlay invalidation.** Surfaced after BUG-956 landed: step_script's pod-Service couldn't find `/builds/e6qu/sockerless` because get_sources had populated it on a DIFFERENT pod-Service's emptyDir. The cloudrun bootstrap had `persist.go` from BUG-947 (cell 7); the gcf bootstrap didn't. Ported it verbatim, wrapped `handleInvoke` with a buffered response + `saveAll` between subprocess exit and wire response (fail-loudly on save failure — silent data loss between stages would surface as confusing build errors). Plus: added `OverlayImageSpec.BootstrapBinaryHash` field + `gcpcommon.HashBootstrapBinary` helper so updating the bootstrap binary at the same path automatically invalidates AR overlay caches. Without this, the cached overlay image at `gcf-XXXXXXXXXXXXXXXX` would silently reuse the old (no-persist) bootstrap binary even after we built and pushed a new one. v28 trace confirmed: `persist save 1024B → 10959360B` on get_sources's pod-Service A, then `persist restore 10959872B` on step_script's pod-Service B. Cell 8 v28 GREEN at job 14234857458, 147s, `all arithmetic checks pass`.

3. **BUG-958 — cloudrun multi-stage runner-pattern (mirror of gcf BUG-955).** Surfaced after cell 7 v53 timed out at 1h. cloudrun's `ContainerStart` returned `NotModifiedError` immediately when `c.State.Running` without checking for a freshly-registered stdinPipe. gitlab-runner v17 docker executor cycles each stage via stop → re-attach → re-start on the SAME container ID; the second /start short-circuited so no invoke goroutine ever drained the new stdinPipe → runner waited forever on /attach response. cloudrun's `ContainerStop` also deleted the underlying Cloud Run Service synchronously, which would have forced a slow re-create on each /start anyway. Fix: (a) `ContainerStart` mirrors gcf BUG-955 — already-running + OpenStdin=true + fresh stdinPipe → kick `invokeRunningRunnerStage` goroutine; (b) `ContainerStop` keeps the Service alive when `c.Config.OpenStdin` (gitlab-runner's /stop is a soft cycle between stages, not real termination); (c) new `invokeRunningRunnerStage` helper drains stdinPipe + POSTs to existing Service URL + closes/re-registers WaitCh. v54 closed cell 7 GREEN at job 14237010667, 178s.

### What we tried that did NOT work

| Attempt | Why it failed |
|---|---|
| **BUG-956 v26 fix — filter ALL containers with FunctionURL (mains + sidecars)** | Sent step_script's container down the cloudrun-functions single-container path (`netMembers=0` → fall-through to `deployFunctionAsync` + `invokeFunction` with empty argv). That path doesn't drain stdinPipe — step_script's bash script bytes captured by attach never reach the bootstrap. Result: 60s+ hang. Refined v27 fix: only filter OpenStdin=true mains, keep sidecars. Worked. |
| **Cell 7 v53 — re-validate after the BUG-957 changes (no other code changes)** | Surfaced BUG-958. cloudrun didn't have the gcf BUG-955 multi-stage fix; v53 hung 60min then timed out. Filed BUG-958 + shipped fix. |
| **Trigger cells 5 + 6 via `gh workflow run`** | HTTP 422 — workflow_dispatch only resolves on default branch. Workflow files for cells 5+6 are not yet on `main`. Resume path: push to PR #124 (the throwaway PR designed for this trigger via `pull_request` event with path filter on the cell yml). |
| **Pause for GH Actions incident** | User-initiated pause at ~08:48 UTC. Cells 5+6 dispatch deferred. GH Actions still surfacing problems but not dispatch-related — pivoted to cell 7 re-validation which surfaced BUG-958. |

### Lessons learned

1. **Multi-stage gitlab-runner pattern is two distinct architectural problems.** First: re-attach + re-start on the SAME container ID across stages (BUG-955 / BUG-958 fix — kick a new invoke goroutine on already-running + fresh stdinPipe). Second: NEW container per stage with a DIFFERENT image (BUG-956 fix — filter old mains from network-pod members so the new stage materializes as its own pod-Service). Both were latent in cloudrun and gcf. cell 7 hit only the first (single image), cell 8 hit both.

2. **Bootstrap binary updates must invalidate the overlay cache automatically.** Before BUG-957's content-hash addition, swapping the binary at `/opt/sockerless/sockerless-gcf-bootstrap` did nothing — `OverlayContentTag` keyed only on PATH. Today this surfaced as: rebuilt the binary with the persist module, deployed the new gcf-backend image, but step_script still hit the old (no-persist) overlay because the AR tag matched. Fix: hash the binary content at server startup, stamp `BootstrapBinaryHash` into every `OverlayImageSpec`. Tag formula now varies on binary content; AR cache busts on upgrade.

3. **Cell-specific fixes propagate across cells.** Cell 8 (gcf) needed BUG-956 + BUG-957. Cell 7 (cloudrun) didn't need either, but did need BUG-958 — which was the cloudrun equivalent of the gcf BUG-955 fix from the prior session. The pattern: per-cloud backends each evolve their own multi-stage handling, and bug-fix patterns ported across them have been the consistent unblocker. Worth promoting: any time we file a multi-stage runner bug for one backend, file the cross-cloud parity check as a same-session sub-task on the others.

## Phase 122k third session (2026-05-05 → 2026-05-06 — cell 8 v9..v25, 4/5 stages GREEN)

Long autonomous-loop session. Cell 8 progressed from "silent hang at Preparing environment" (v17-v22) through 12 architectural fixes to **4/5 stages GREEN** in v25 (prepare_executor + prepare_script + get_sources + step_script start). Final blocker BUG-956 (multi-image-per-stage materialize race) pinned with concrete fix path.

### What worked (12 architectural fixes shipped, all verified via traces)

The breakthrough came in v23 with two changes that, taken together, make the gcf network-pod path actually viable for gitlab-runner v17:

1. **AR HEAD precheck** (`backends/gcp-common/registry_check.go`). HEAD `/v2/<repo>/manifests/<tag>` short-circuits Cloud Build's ~28 s tag-rebuild overhead even on layer cache hits. Wired into both cloudrun and gcf's `ensureOverlayImage`. This alone took materialize from 60 s to 9-15 s.

2. **Multi-container Cloud Run Service direct deploy** (`pod_service.go::materializePodService`). Replaces the slow Cloud Functions wrapper path (CreateFunction with stub Buildpacks-Go source + UpdateService swap = 150 s) with `Services.CreateService` directly with multi-container `RevisionTemplate`. Cuts pod materialize to 9-15 s, well under gitlab-runner's 120 s SDK timeout.

3. **`PendingCreates` speculative-running marker through materialize**. ContainerStart marks the network-pod main "running" before calling materializePodService synchronously. Concurrent ContainerInspect / cleanup-script docker exec calls during the 30 s CreateService.Wait window resolve to a real container instead of NotFound. `Update` returns false if the entry was already removed by a cancelled async deploy; we `Put` as fallback to handle the race.

4. **`resolvePodServiceFromCloud` GetService follow-up**. ListServices may return abbreviated `Annotations` in some pagination modes; sidecar pod members (matched via `sockerless_pod_members` annotation) need a GetService follow-up to retrieve the full proto.

5. **`stdinPipe` + `attachStream` pattern ported from cloudrun**. New files `backends/cloudrun-functions/{stdin_pipe.go, attach_stream.go}` mirror `backends/cloudrun/{stdin_pipe.go, attach_stream.go}` verbatim. Captures bytes written via the hijacked attach connection's Write into a per-container pipe, replays as the bootstrap's exec envelope `Stdin` payload at deferred-invoke time. Reads block until invokePodServiceMain publishes the bootstrap response (mux-framed stdout + stderr).

6. **Relaxed `ContainerAttach` overlay-image gate**. The original cloudrun code required `c.Config.Image` to be in the sockerless-overlay AR repo to enable the stdin path. But at attach time the image is the user-supplied original (golang:1.22-alpine), not the overlay URI — the rewrite happens inside materializePodService. Drop the gate.

7. **5 s pre-check window for late-arriving stdinPipe**. invokePodServiceMain's goroutine wait for the stdinPipe to be registered before LoadAndDelete'ing it. Cloudrun's invokeServiceDefaultCmd hides the same race behind `waitForServiceURL`'s 30 s polling delay (which gcf doesn't need because `materializePodService` already returned with the URL). Without the explicit pre-check, gcf raced past pipe registration and fell through to default-invoke.

8. **`OpenStdin=true` runner-pattern: skip default-invoke**. When the network-pod main has `Config.OpenStdin=true` AND no stdin was captured via attach, DO NOT POST a default CMD. Don't close WaitChs. Don't PutInvocationResult. The bootstrap stays alive as the HTTP server holding the Service revision. gitlab-runner expects the build container to STAY ALIVE for `docker exec` (the gitlab-runner-build subcommand is a no-op without CI env vars and would exit 0 immediately if invoked).

9. **VpcAccess + ALL_TRAFFIC on materialize Service revisions**. Cloudrun has this since BUG-933 ('Cloud Run rejects cross-project-service-to-service via .a.run.app + Cloud NAT with HTTP 404 because the NAT'd source IP isn't auto-detected as same-project Cloud Run'). gcf was missing it. Without VpcAccess, gitlab-runner-gcf's IAM-gated POSTs to sockerless-svc-* return 401/403; gitlab-runner's docker SDK retries internally without surfacing the error.

10. **HTTP middleware ENTRY-level logging** (`backends/core/server.go::LoggingMiddleware`). Hijacked /attach connections take over the TCP stream; the post-handler END log only fires when the stream closes (could be hours). Adding ENTRY logging makes every request visible the moment it arrives. This was the diagnostic that finally revealed v22 was making /attach calls all along.

11. **Typed.Attach routes through ContainerAttach delegate** (gcf `server.go`). **The silent-hang root cause** for v17-v22. `core/handle_containers_query.go::handleContainerAttach` calls `s.Typed.Attach.Attach(...)`, NOT `s.self.ContainerAttach`. cloudrun wires `Typed.Attach = WrapLegacyContainerAttach(s.ContainerAttach,...)` (routes through the delegate that registers stdinPipe). gcf was wired to `Typed.Attach = NewCloudLogsAttachDriver(...)` — read-only, silently dropped gitlab-runner's stdin. THAT'S why every cell 8 iteration showed `no stdinPipe registered after 5 s wait` even though `/v1.44/containers/.../attach` ENTRY was logged. Mirror cloudrun's wiring.

12. **Multi-stage `invokeRunningRunnerStage`** + **unique container names in pod RevisionTemplate**. Per stage gitlab-runner does `stop` then re-`attach` + re-`start`. Cloud Run revisions are immutable so the stop+start is a no-op at the cloud layer; this function processes the new stdinPipe registered by the new attach and POSTs the captured stage script bytes to the same Service URL. ContainerStart, when the container is already running AND has OpenStdin=true AND a fresh stdinPipe is registered, kicks off `invokeRunningRunnerStage`. Plus: track sanitized container names and append count suffix on collision (`postgres`, `postgres-1`, `postgres-2`) — Cloud Run rejects duplicates with `template.containers: Containers [N, M] have duplicate container names`.

### What we tried that did NOT work (anti-recipes for future sessions)

| Attempt | Why it failed |
|---|---|
| Adding `SockerlessImage` + `BackendPort` fields to dispatcher's `Label` config + multi-container TaskTemplate in `Spawn()` | User clarification: **dispatcher must stay generic and unaware of sockerless**. The runner-task image bundles vanilla runner + sockerless. Dispatcher just submits a single image. Saved as `feedback_dispatcher_generic.md` memory. |
| HTTP 500 for expected exec failures in bootstrap | User directive: 5xx reserved for unexpected panics. Use HTTP 200 + `X-Sockerless-Exit-Code` header / envelope `exitCode`. Saved as `feedback_no_500_default.md`. |
| Quick-fix proposals (delay-window, polling alternatives) alongside structural fixes | Project rule: "always do the right fix, never the quick fix". Structural-only approach. |
| Bumping Cloud Run regional CPU quota via `gcloud beta quotas preferences` | User explicit reject 2026-05-04: solve architecturally. Pool-warming + direct Service deploy + aggressive cleanup are the architectural answers. |
| First hypothesis for v17-v22 silent hang: TCP probe to postgres from gitlab-runner's process | Disproven by v21 — gitlab-runner's docker SDK hijacked /attach correctly; the issue was sockerless's gcf `Typed.Attach` was the read-only `NewCloudLogsAttachDriver`. |
| Second hypothesis: missing VpcAccess (BUG-933 mirror) | Shipped as v20; didn't unblock the silent hang. VpcAccess IS still needed (cell 7 has it; gcf was missing) but wasn't the root cause. |
| Sequential Cloud Builds for pod overlays | Combined ~150 s exceeded gitlab-runner's 120 s ContainerExec timeout. Replaced by parallel goroutines in materializePodService. |
| `gitlab-runner-helper` baked CMD as the build container's default invoke | The CMD `[/usr/bin/dumb-init /entrypoint gitlab-runner-build]` exits 0 with no work without CI env vars; if invoked it closes WaitChs and gitlab-runner reports the container as exited. Skip default-invoke for OpenStdin=true. |

### Lessons learned

1. **Hijacked HTTP connections need ENTRY logging.** The standard middleware-on-end pattern is misleading — a hijacked /attach connection might run for hours before its END log fires. Ship request-entry logging for any backend that hosts hijacked connections. (BUG-955 was diagnosable in 30 minutes once we had ENTRY logs; before that, three iterations chased phantom theories.)

2. **`Typed.Attach` and `s.self.ContainerAttach` are different code paths.** `core/handle_containers_query.go::handleContainerAttach` routes through `s.Typed.Attach.Attach(...)`. The gcf-specific delegate is only reachable if `Typed.Attach = WrapLegacyContainerAttach(s.ContainerAttach,...)`. cloudrun cell 7 GREEN proves this is the right wiring; gcf was using the read-only cloud-logs driver. **Anywhere a cloud backend needs to register state on attach (stdinPipe, attachStream, reverse-agent), `Typed.Attach` must wrap the delegate, not bypass it.**

3. **Cloud Run revision immutability is a hard invariant.** Each docker `start` for the same container is a no-op at the Cloud Run layer (revision already running). Multi-stage gitlab-runner patterns need explicit per-stage re-invoke goroutines. Don't try to mutate a running revision; create a new one for new images, or run the new stage as a one-shot against the existing Service URL.

4. **Cloud Run regional CPU quota is the dominant operational constraint.** Each materialize burns 1 vCPU-min × N members. With min=1 max=1 scaling, every Service revision holds 1 vCPU until deleted. Aggressive cleanup (delete `sockerless-svc-*` after every test, prune old revisions on long-lived Services) is mandatory to avoid cascading quota failures masquerading as silent hangs (Service LRO returns 204 while underlying revision health-check fails in the background).

5. **gitlab-runner v17 docker executor uses different images per stage.** Helper image (gitlab-runner-helper:x86_64-v17.5.0) for prepare/get_sources/restore_cache/upload_artifacts; user image (`golang:1.22-alpine` etc) for step_script/after_script. With `FF_NETWORK_PER_BUILD=true` all join the same per-build network. Network-pod detection cannot blindly bundle every PendingCreate that happens to share the network — must filter out already-materialized members.

6. **Cloudrun cell 7 GREEN is the architectural reference.** Every gcf network-pod fix we shipped today started with "what does cloudrun do". When a gcf path diverged from cloudrun, that divergence was the bug. Rule: **mirror cloudrun's pattern unless gcf has a documented reason to differ.**

## Phase 122k continuation (2026-05-05 second session — cell 8 v9..v15)

Continuing from the morning's Phase 122k. Cell 7 stayed GREEN (no regressions). Cell 8 went from v9 to v15 driving down the "Cannot connect to Docker daemon" / "No such container" failure modes. Cells 5+6 still not started but the user clarified the architecture: **dispatcher stays generic, sockerless+runner pairing lives in the runner image** — saved as `feedback_dispatcher_generic.md` memory.

**Cell 8 iteration log:**

| Iter | Change | Outcome |
|------|--------|---------|
| v9-v10 | execStartViaInvoke entry/exit logs; reduce queryPodServiceContainers from Info to Debug | failed (170s) — runner timeout on docker exec; PostgreSQL+build pod's network-pod path was eating the 120s docker SDK budget across two parallel Cloud Builds (~28s each) + CreateService.Wait (~30-60s) |
| v11 | **AR HEAD precheck on `/v2/<repo>/manifests/<tag>`** — `gcp-common/registry_check.go` does `HEAD /v2/<repo>/manifests/<tag>` with idtoken-derived bearer; on 200 we return imageURI directly, skipping Cloud Build's ~25-30s tag-rebuild overhead even on layer-cache hit | total job 82s (was 170s); now fails with "No such container" on cleanup `docker exec` |
| v12 | `PendingCreates.Update(running)` through materialize, delete only on error | failed (43s) — log "marked running" never appeared |
| v13 | `Put` fallback when `Update` misses; "marked running" log | failed |
| v14 | network-pod decision log + materializePodService entry/exit logs | failed (43s) — every diagnostic log line MISSING from Cloud Logging despite binary having strings |
| v15 | ContainerStart **ENTRY/resolved/NOT FOUND** logs at top of handler + resolvePodServiceFromCloud GetService follow-up | All diagnostic logs fire correctly. Materialize 13 s. Bootstrap exec'd CMD → exit=0. gitlab-runner silent. |
| v16 | ENTRY-level logs on delegates | gitlab-runner makes ZERO docker calls during silent window. Suspected stdinPipe race. |
| v17 | stdinPipe + attachStream pattern ported from cloudrun | gitlab-runner still ZERO calls; never calls `/containers/{id}/attach`. |
| v18 | ContainerAttach overlay-image gate dropped + 5 s pre-check window for late-arriving pipe | Same silent hang. Confirms gitlab-runner is NOT calling Attach at all. |
| v19 | OpenStdin=true runner-pattern: skip default-invoke, keep container alive | Same hang. Confirms it's NOT about WaitChs/exit-code visibility. Ran full 1h job timeout. |
| **v20** | **VpcAccess + ALL_TRAFFIC on gcf network-pod + single-container Service deploys** | **HYPOTHESIS PINNED**: gcf was missing the cloudrun BUG-933 fix. Cloud Run rejects same-project cross-Service POSTs (gitlab-runner-gcf → sockerless-svc-*) as external without VpcAccess + ALL_TRAFFIC. gitlab-runner's docker SDK retries internally, never surfacing the IAM rejection. Fix: `Config.VPCConnector` field + `revTemplate.VpcAccess` block + `SOCKERLESS_GCF_VPC_CONNECTOR` yaml env. Verification pending. |

**Working en route, must not regress:**

- AR HEAD precheck: `backends/gcp-common/registry_check.go` — `CheckTagExists(ctx, imageURI)` uses google.FindDefaultCredentials + 5s context timeout. Cuts ~28s of Cloud Build per overlay when image already in AR. Wired into both cloudrun and gcf `ensureOverlayImage`.
- Multi-container Service direct deploy (`pod_service.go::materializePodService`) replaces the slow Cloud Functions wrapper for pod-mode. Service IS being created correctly post-materialize (verified via `gcloud run services describe sockerless-svc-*`); annotations + labels populated.
- gcf bootstrap envelope path (BUG-951); Service URL fallback for empty `Function.ServiceConfig.uri` (BUG-952).
- PendingCreates speculative-running marker through materialize: `Update` if entry exists, `Put` fallback if missing; delete only on error.

**What we tried that did NOT work** (record so future sessions don't redo):

| Attempt | Why it failed |
|---|---|
| Adding `SockerlessImage` + `BackendPort` fields to dispatcher's `Label` config + multi-container TaskTemplate in `Spawn()` | User clarification: **dispatcher must stay generic and unaware of sockerless**. Reverted. The runner-task image at `tests/runners/github/dockerfile-{cloudrun,gcf}/` already bundles vanilla runner + sockerless backend; the dispatcher just submits a single image. Memory saved as `feedback_dispatcher_generic.md`. |
| Cancelling deployFunctionAsync mid-Cloud-Build then expecting cancelled goroutine to skip PendingCreates manipulation | Goroutine receives ctx.Cancel during `Source.upload to GCS` step; cancellation propagates correctly but cells 5/6/8 still hit the materialize-window race. Not the right layer to fix. |
| HTTP 500 for expected exec failures in bootstrap | User directive: 5xx reserved for unexpected panics. Replaced with HTTP 200 + `X-Sockerless-Exit-Code` header / envelope `exitCode`. Saved as `feedback_no_500_default.md` memory. |
| Quick-fix proposals (delay-window, polling) alongside structural fixes | Project rule: "always do the right fix, never the quick fix". Structural-only approach. |

## Phase 122k — Cell 7 GREEN heavy-workload + cell 8 architectural deep dive (2026-05-05 morning)

User goal recorded: **all 4 GCP cells (5/6/7/8) GREEN with full workflow + evidence + executing where they're supposed to**. Cell 7 GREEN heavy workload achieved; cell 8 close (8 iterations) but final blocker still being verified; cells 5+6 dispatcher refactor not yet started.

**Cell 7 v51 GREEN** (https://gitlab.com/e6qu/sockerless/-/pipelines/2500209956, job 14213994152, 383 s). Heavy workload verified end-to-end: git fetch + git checkout + apk add file + go build eval-arithmetic + run with PostgreSQL sidecar. All 5 arithmetic results correct (11/14/21/13/6.5).

**Closed today** (in order):

| Bug | Title | Fix shape |
|---|---|---|
| 947 | GCSFuse `/builds` ~200× slower than tmpfs for git ops | Bootstrap tar-pack persist module (`agent/cmd/sockerless-cloudrun-bootstrap/persist.go`) downloads single tar object per ad-hoc bind volume from GCS at startup; re-uploads after every exec. Backend emits `Volume_EmptyDir{MEMORY}` (tmpfs) for ad-hoc binds + injects `SOCKERLESS_PERSIST_VOLUMES=name=path=bucket` env on main. Replaces N per-file GCSFuse round-trips with 1 GCS object roundtrip per stage. Cell 7 v51 verified live. |
| 948 | gcf per-step Function deploy hits regional CPU quota | Pool-warming via `SOCKERLESS_GCF_PREWARM_OVERLAYS` env. Pre-deploys N free Functions tagged with overlay content-hash at backend startup. Works for single-container claims; pod-mode addressed by BUG-953 separately. |
| 950 | gcf prewarm contentTag mismatch — pool entries unclaimable | `OverlayContentTag` no longer hashes `UserEntrypoint`/`UserCmd`/`UserWorkdir`; pool entries are reusable across container types with different commands. Runtime entrypoint/cmd/workdir flow through `ServiceConfig.EnvironmentVariables` (or invoke envelope after BUG-951). `RenderOverlayDockerfile` no longer bakes `ENV SOCKERLESS_USER_*`. Also: prewarm refs go through `ResolveGCPImageURI` so prewarm hash matches live workload's AR-resolved hash. |
| 951 | gcf claim-side env-update via `UpdateService` itself debits CPU quota | Drop `updateFunctionUserEnv` claim-path call. `invokeFunction` now POSTs the exec envelope `{"sockerless":{"exec":{"argv":...,"workdir":...,"env":...}}}` carrying user entrypoint+cmd+workdir+env. Bootstrap's `parseExecEnvelope` reads argv from request body (Path B). Pool entries are CR-immutable; any user command runs without touching CR Service config. |
| 952 | `resolveGCFFromCloud` returns empty `Function.ServiceConfig.uri` from `ListFunctions` response | Follow up label-match with `GetFunction` by name to fetch full proto. Last-resort fallback derives URL from underlying Cloud Run Service via `Services.GetService`. Cell 8 v5 logs confirmed the fallback fired live: `derived URL from underlying Cloud Run Service`. |

**In-flight (cell 8 v8, BUG-953)**: gcf pod-mode via direct multi-container Cloud Run Service deploy (skip Cloud Functions wrapper). Cell 7 architecturally identical (cloudrun's `Services.CreateService` for multi-container) deploys in ~30-60 s; gcf's old path through `CreateFunction` + `swapServiceImage` was ~150 s and hit gitlab-runner's 120 s `ContainerExec` timeout. New `pod_service.go::materializePodService` builds per-member overlay images in parallel + calls `Services.CreateService` directly with multi-container `RevisionTemplate`. gcf bootstrap got `SOCKERLESS_SIDECAR=1` mode added (mirrors cloudrun). `cloud_state.queryPodServiceContainers` queries Services with the `sockerless_pod_members` annotation; v8 adds `GetService` follow-up since `ListServices` may abbreviate annotations. Verification pending.

**What we tried that did NOT work** (record so future sessions don't redo):

| Attempt | Why it failed |
|---|---|
| (Cell 8 v1-v6 iteration log — moved targets, each one an honest reach) | See [BUGS.md](BUGS.md) BUG-942/948/950/951/952/953 entries for per-step diagnosis. |
| Sequential pod overlay Cloud Build + CreateFunction in gcf | Combined ~150 s exceeded gitlab-runner's 120 s ContainerExec timeout. v6 parallelization saved 30 s; still over budget for >2 members. Replaced wholesale by direct `Services.CreateService`. |
| HTTP 500 for expected exec failures in bootstrap | User directive: 5xx reserved for unexpected panics. Replaced with HTTP 200 + `X-Sockerless-Exit-Code` header / envelope `exitCode` (commits 29308e1 + envelope failure handling in cb4eb6d). |
| Quick-fix proposals (delay-window, polling) alongside structural fixes | Project rule: "always do the right fix, never the quick fix". Killed every iteration. |
| Bumping Cloud Run regional CPU quota via `gcloud beta quotas preferences` | User explicit reject: solve architecturally. Pool-warming + direct Service deploy are the architectural answers. |

## Phase 122j — vanilla-runner architecture pivot + BUG-947 GCSFuse-vs-git-checkout (2026-05-04)

Long autonomous-loop session. User reset cells 5–8 architecture mid-session: runners stay vanilla (no sockerless-baked custom image); sockerless rides as a sidecar in multi-container Cloud Run Service/Job; gitlab-runner needs no dispatcher (it polls itself); github cells use a thin dispatcher that only calls `Executions.RunJob` with per-execution env override. Cell 7 GREEN under OLD architecture (v49, pipeline 2496721473) was acknowledged as "to be lost in the refactor" — and was lost.

**NEW-architecture progression for cell 7:**
- v5 (today PM, `d82a1cc`+revert): step-Service path (`SOCKERLESS_GCR_USE_SERVICE=1`) routed correctly; git fetch from gitlab.com timed out at 135 s. Root cause: VPC connector min-instances 2 saturated by concurrent step deploys.
- v50 (today PM, pipeline 2498952453): connector raised 2→4; gitlab project_type runner cap purged 50→2. git fetch succeeded (2 MB pack downloaded). Then `git checkout` HUNG — sockerless backend POST exceeded 10-min HTTP exec timeout. **Diagnostic confirmed (22:42 UTC):** `git clone e6qu/sockerless` on bare Cloud Run Service: GCSFuse 211 s vs tmpfs 1 s. ~200× slowdown is per-file metadata round trips, not bandwidth.

**BUG-947 filed.** Architectural exploration:

| Path | Verdict |
|---|---|
| A — emptyDir + single Cloud Run Service revision per gitlab-runner job | **Infeasible.** Cloud Run revisions are immutable; modifying a Service spawns a new instance with fresh emptyDir. gitlab-runner adds containers dynamically across stages; can't deploy them all upfront. |
| B — Cloud Filestore (NFS) | Workable but $160/mo BASIC_HDD floor (1 TiB minimum even empty). GCP has no pay-per-use NFS analog of AWS EFS. Held in reserve for big-repo workloads where tar-pack roundtrip would dominate. |
| C — git config workarounds (`core.useHardlinks=false`) | Forbidden quick fix. |
| `GIT_STRATEGY=none` workaround in cell yml | User explicit reject: must support `clone`/`fetch`/`none`. |
| `fuse-overlayfs` | Cloud Run gen2 may lack syscall caps; per-file sync at exit still slow. |
| LD_PRELOAD shim | Image-specific, fragile, breaks "vanilla runner" contract. |
| Per-job Filestore provisioning on demand | 5-15 min provisioning latency would blow gitlab-runner job timeout. |

**Chosen fix — tar-pack persist module** (`agent/cmd/sockerless-cloudrun-bootstrap/persist.go`, committed `1f06831`): bootstrap downloads single tar object per ad-hoc bind volume from existing per-volume GCS bucket at startup; re-uploads after every exec (under `invokeMu`, so next stage's restore sees fresh data). Replaces N per-file gcsfuse round trips with 1 GCS object roundtrip per stage (~2-5 sec for sockerless-repo-sized data). No new infra; no Filestore; binary grows ~0 MB (raw HTTP + metadata-server token, no GCS SDK dep). Operator-controlled via `SOCKERLESS_PERSIST_VOLUMES=name=path=bucket,...` env injected by the backend on the **main** container only (sidecars marked `SOCKERLESS_SIDECAR=1` skip both restore + save — they share the same kernel tmpfs in a multi-container revision). Backend volume-spec change (emit `Volume_EmptyDir{Memory}` for ad-hoc binds + the env) and image rebuild + cell retest are the next steps — see [DO_NEXT.md](DO_NEXT.md).

**Architectural insight — GCSFuse incompatibility shape:**
GCSFuse omits POSIX hard-link (`fuseops.CreateLinkOp -> "function not implemented"`), has weak cross-dir rename (best-effort, two-phase), no `flock`/`fcntl` advisory locks. git's `index-pack` uses hardlinks for object-pack publish; `git checkout` updates `.git/index` via `O_EXCL` create + atomic rename + `flock`. Errors are non-fatal at the FUSE log layer but trigger lockfile retries that never resolve. **The 200× slowdown comes from per-file metadata ops** — total bytes moved is small. Tar bundles all metadata into one GCS object → one round trip instead of N.

**Connector + NAT config now stable** (do not regress): `sockerless-connector` e2-micro × 4 min instances; `sockerless-nat` static IP `34.31.88.230` MANUAL_ONLY allocation; `sockerless-vpc` no-firewall-rules (default open egress); cross-cloud-run via `Egress: ALL_TRAFFIC` + Cloud NAT.

## Phase 122i — dispatcher rate-limit + gcf pool quota + 3-layer BUG-944 (2026-05-04)

Long session — 13 commits, 7 BUG roots pinned, no cells closed (cell 7 was GREEN at session start; lost when `.gitlab-ci.yml` swap reverted). Dispatcher behaviour around GitHub rate limits + GCP CPU quota is now correct. BUG-944 (cell 6 exit 126) traced through three architectural layers; fixes shipped at all three; verification pending image rebuild.

**Closed**: BUG-938 (Cloud NAT abuse rotation), BUG-939 (runner-task OOM at 4Gi/2cpu), BUG-940 (cleanup uses Execution state not Definition state), BUG-941 (cleanup ticker re-fires GitHub poll during rate-limit), BUG-943 (poller 1+N call burn → 60s + runSeen + proactive back-off).

**In-flight verification**: BUG-942 (pool claim back-off `df75d4d`) and BUG-944 layer 1+2+3 (`d85b652` + `ee63dae` + `a7e3b00`). All three layers shipped; cell 6 retest waiting on image rebuild.

**BUG-944 anatomy — 3 layers peeled**:

| Layer | Symptom | Root cause | Fix |
|-------|---------|-----------|-----|
| 1 | exit 126 in first script step | hypothesis: GCS-Fuse cross-execution metadata cache hides freshly-written script | added `MountOptions=[implicit-dirs, ttl-secs=0, negative-ttl-secs=0]` to all 3 GCSVolumeSource constructions (`d85b652`) |
| 2 | layer-1 fix shipped, still exit 126; deployed function had `volumes: null` | pool-hit branch in `deployFunction` returned early before calling `attachVolumesToFunctionService` — reused functions inherited zero volumes | pool-hit branch now calls attach; idempotent merge by name (`ee63dae`) |
| 3 | layer-2 fix shipped, still exit 126; deployed function had volumes but no `mountOptions` | idempotent merge by name skipped UpdateService when entries already present, even if MountOptions differed — pool-reused funcs from before MountOptions existed had matching names but stale config | full-shape compare; replace stale entries (`a7e3b00`); purged stale pool to force fresh deploys |

**Architectural insight — AWS vs GCP shared storage**:

| Backend | Primitive | Consistency | Default behavior |
|---------|-----------|-------------|-----------------|
| AWS ECS / Lambda | EFS access points (NFSv4) | Strong | Cross-execution writes immediately visible — "just works" |
| GCP Cloud Run / Functions | GCS bucket via gcsfuse | Eventual + 60s positive cache + 5s negative cache | Container sees stale "doesn't exist" for 5s; needs explicit MountOptions |

Cells 1-4 were green on AWS partly because EFS hides this class of bug. GCP's object-store-with-FUSE primitive needs explicit opt-out from caching to behave like a shared FS for ephemeral write/read patterns.

**What we tried that did NOT work** (preserve as anti-recipe — these will tempt future sessions):

1. Lowering gcf default per-function CPU 1 → 0.5 to fit more parallel functions — Cloud Run gen2 execution environment rejects fractional CPU below 1 with `Total cpu < 1 is not supported with gen2 execution environment`. Reverted in `71288bf`. **Gen2 is the constraint we keep** (latest stable, no deprecated APIs per user directive).
2. Requesting Google quota increase via `gcloud beta quotas preferences create CpuAllocPerProjectRegion 20000→200000` — user explicitly rejected ("quota increase is the wrong path"). Withdrew via update to grantedValue.
3. Running 4 cells in parallel — exceeds `cpu_allocation` per-minute window. Solo runs get past CPU; parallel needs pool back-off + multi-container packing.
4. Idempotent volume-attach by NAME only — pool-reused volumes have matching names but stale MountOptions. Layer-3 fix uses full-shape compare.
5. Treating runner image build's `installdependencies.sh` failure as transient retry-loop — actually a real bug (per user directive: "transients and flakiness must be treated as bugs"). Investigation in flight; will name root cause and ship real fix, not loop with retries.

**Strict rules adopted/reinforced this session**:

- **Strict rate-limit policy** (memory `feedback_strict_rate_limit.md`): when honoring upstream rate-limit hints, sleep `max(retryAfter, resetIn) * 1.10 + 1s`. Resuming at reset boundary triggers immediate re-throttle.
- **Verify deployed state field-by-field** before assuming a fix worked. Layered BUG-944 investigation showed how easy it is to ship a fix and "not actually" fix the deployed result. After every gcf/cloudrun fix, dump `gcloud run services describe <skls-*> --format=json` and verify the relevant fields.
- **Transient errors are bugs** (user directive 2026-05-04). No retry-loops disguised as fault-tolerance.

## Phase 122h — gitlab-runner stdin_pipe attempt (2026-05-04, rolled back)

Phase 122h ported the ECS+Lambda stdin_pipe.go pattern to cloudrun (~80 lines). Image built (digest `4fc5abd0951729...`). Cloud Run rev `00040-qj6` health probe failed — bootstrap silently dies before binding PORT 8080. Rolled back to `00038-f42` (Phase 122g code). Code committed (`9f9f872`) but not running live; needs local debug of bootstrap startup with full Cloud Run env vars before re-deploy.

## Phase 122g — overlay+Path-B exec for cloudrun + gcf (2026-05-03, GREEN cell 7)

Lifted `backends/lambda/image_inject.go` → `backends/gcp-common/image_inject.go`; new `agent/cmd/sockerless-cloudrun-bootstrap` HTTP server; cloudrun + gcf ContainerCreate route through Cloud Run Service via overlay. `ExecStart` path B HTTP POST envelope to Service URL with `idtoken.NewClient`. **Result: cell 7 GREEN at pipeline 2496721473 (1020s, all 5 arithmetic markers verified, postgres SELECT version() returned PostgreSQL 16.13).**

Cells 5/6/8 not GREEN at end of 122g — surfaced BUG-925 (postgres TCP via Cloud Run multi-container sidecar, fix shipped 12-step), BUG-923 (gcf ContainerCreate.Wait > 120s, fix shipped async deploy in 122i), BUG-937 (3-stage AR-auth chain, fix shipped).

## Phase 122f — BUG-927 root-cause discovery; Phase 122g architectural plan locked (2026-05-03)

End of session 2026-05-03 v12: cells 5-8 still failing, but the architectural blocker is now precisely diagnosed. Cell 7 reported SUCCESS but Cloud Logging proved ZERO workload markers (no `apk add`, `git clone`, `eval-arithmetic`). Backend trace pattern `attach 200 (216s) → exec 409 'Container not running' → wait 200 → stop 304 × N` confirms gitlab-runner expects a long-lived build container with per-stage `docker exec`; Cloud Run Job (one-shot) cannot host that model and stock images (`golang:1.22-alpine`, `postgres:16-alpine`) have no in-container exec endpoint. BUG-927 captured.

**Phase 122g plan (next session, in DO_NEXT.md)**: lift `backends/lambda/image_inject.go` → `backends/gcp-common/image_inject.go`, new `sockerless-cloudrun-bootstrap` binary, drop `isRunnerPattern` gating, `ContainerExec` = Path B HTTP POST with `execEnvelope` (Lambda's `execStartViaInvoke` analogue) for both cloudrun + gcf. Pre-deploy Service per shape via terraform. This dissolves BUG-921/922/923/925/927.

**Spec doc updates (2026-05-03 v12)**:
- `specs/CLOUD_RESOURCE_MAPPING.md` Lesson 6 REVISED — overlay IS needed on cloudrun for stock images (was wrongly stated as "skip overlay" before BUG-927).
- Lesson 8 ADDED — Lambda's `execStartViaInvoke` Path B as the gcf+cloudrun adaptation pattern.
- Synthesis section rewritten for Phase 122g scope.

## Phase 122e — Cells 5-8 live-GCP bug chain + dispatcher cleanup + spec hardening (closed 2026-05-03)

15+ live-only bugs surfaced + closed (BUG-907..921 + BUG-924). The Phase 122e session captured the architectural state in 4 new spec sections + dispatcher scope cleanup + bootstrap auto-discovery. The "Phase 122f scope" originally framed as "Cloud Run Service path for runner-pattern" turned out to be incomplete — Phase 122g (overlay + Path B exec) is the actual unblock per BUG-927 evidence.

**Spec doc (`specs/CLOUD_RESOURCE_MAPPING.md`) grew from ~840 lines → 1063 lines** with 4 new sections written this session:

1. **Runner job lifecycle (docker executor) — required cloud primitives** — formal state machines for both GitLab Runner (6 phases) and GitHub Actions Runner (5 phases). Critical invariant identified: containers MUST persist across N `docker exec` calls between phases 3-5. Cloud Run Job (one-shot) does NOT fit; Cloud Run Service does.

2. **Per-backend container concerns matrix** — 21 concerns × 7 backends. Every container-level concern (long-lived, bind mount, caps, privileged, user, multi-container, supervisor, exec, network isolation, lifecycle, auto-remove, image pull/push, resource limits, env, workdir, cmd, stdin, logs, exit code, health) mapped to its actual cloud primitive — or explicit "NOT supported — fail loudly" where the cloud genuinely lacks it. No fake fallbacks; backends fail with a clear "use <other backend> for this" error where the host cloud forbids the operation.

3. **Lessons from ECS + Lambda backends → cloudrun + gcf adjustments** — 7 lessons synthesized from the ECS (cells 1+3 GREEN) + Lambda (cells 2+4 GREEN) impls into Phase 122f scope:
   - L1: ECS pre-registered task-def family → cloudrun pre-deploy one Cloud Run Service per runner shape; gcf same.
   - L2: Lambda warm pool by content-hash → gcf already has, verify firing; cloudrun Service min_instance_count=0/1 toggle.
   - L3: ECS SSM ExecuteCommand → cloudrun+gcf reverse-agent (already in ACA).
   - L4: Lambda stdin payload (BUG-875) → cloudrun+gcf reverse-agent stdin.
   - L5: ECS bind-mount → EFS access points → already done (BUG-909 GCS).
   - L6: Lambda overlay-image → cloudrun Service uses Container.command override directly, no overlay; gcf same via UpdateService escape.
   - L7: Tag-based state recovery → already implemented.

4. **Dispatcher scope adjustment** — `github-runner-dispatcher-{aws,gcp,azure}` SHALL only spawn the runner container with RUNNER_REG_TOKEN/REPO/NAME/LABELS. Forbidden: SOCKERLESS_* env, volume mounts, sockerless config injection. The runner image owns its own backend config internally — auto-discovers from GCP instance metadata server (project, region) + convention (build_bucket = `<project>-build`, runner_workspace_bucket = `<project>-runner-workspace`).

**Code changes Phase 122e (12 commits this session)**:
- `backends/gcp-common/image_resolve.go` — bare `sha256:<digest>` refs returned as-is (BUG-918); `gitlab-registry` AR remote-proxy case (BUG-919).
- `backends/core/resolve.go` — `Store.ResolveImage` matches digest IDs with/without `sha256:` prefix + RepoDigests (BUG-918).
- `backends/cloudrun/{config,backend_impl,volumes}.go` — SharedVolumes + bind-mount translator + `Container.command` override (BUG-909, BUG-918 RepoTag substitution, BUG-921 use op.Metadata for execution name).
- `backends/cloudrun-functions/{config,backend_impl,volumes}.go` — same SharedVolumes pattern (BUG-909).
- `github-runner-dispatcher-gcp/internal/spawner/spawner.go` — drop nested Job.Name (BUG-908), drop runOp.Wait (BUG-912), set TaskTemplate.Timeout=3600s (BUG-911), strip SOCKERLESS_* env injection (dispatcher scope cleanup).
- `tests/runners/{github,gitlab}/dockerfile-{cloudrun,gcf}/bootstrap.sh` — fail-loudly env validation (BUG-907), mkdir runner-work (BUG-913), socat $PORT bridge for Cloud Run, sed config.toml for `disable_cache=true` + `helper_image=<full-tag>` (BUG-915 + BUG-918), bash timeout default 3600s (BUG-910), backend logs ride stderr to Cloud Logging, auto-discover sockerless config from GCP metadata server.
- `terraform/modules/cloudrun/runner.tf` (NEW) — `google_storage_bucket.runner_workspace` (BUG-909).
- Live infra deployed: dispatcher Cloud Run Service + Secret Manager (`github-pat`, `gitlab-pat`, `gitlab-runner-token-{cloudrun,gcf}`) + AR remote-proxies (`docker-hub`, `gitlab-registry`) + gitlab-runner Cloud Run Services (cloudrun + gcf).

**Open at end of Phase 122e**: BUG-922 (cloudrun container removed after first exec — Cloud Run Job lifecycle vs runner expectation) and BUG-923 (gcf ContainerCreate blocks 150-200s on Cloud Build + CreateFunction.Wait — exceeds gitlab-runner 120s docker timeout). Both addressed by Phase 122f architectural shift to Cloud Run Service path.

## Phase 122d — Cells 5/6 live-GCP unblock (BUG-907..911) (in flight 2026-05-03)

PR #124 (`cell-workflows-on-main`, throwaway — must NOT merge) lands the cell-5-cloudrun + cell-6-gcf workflow files on `main` so `workflow_dispatch` becomes possible. Carries TEMP `pull_request:` triggers (constrained to `branches: [main]`+`paths: [.github/workflows/cell-{5,6}-*.yml]`) so PR-#124 pushes auto-fire those cells. Closed once cells GREEN — PR #123's cell yamls have only `workflow_dispatch` so the post-merge state is manual-only (matches cells 1+2).

Dispatcher deployed serverless (Cloud Run Service `github-runner-dispatcher-gcp` in `sockerless-live-46x3zg4imo`, min=max=1, no-cpu-throttling, runner SA, GitHub PAT secret-mounted from `github-pat:latest`). Image built via Cloud Build (sockerless-sanctioned per Phase 122c) + pushed to AR `dispatcher:gcp-amd64`. Service URL: https://github-runner-dispatcher-gcp-199307773205.us-central1.run.app — currently serving rev `00005-rv9` (post BUG-911).

**Bug chain surfaced live (closed in same session)**:

- **BUG-907** (bash apostrophe in `${var:?msg}` form). After Phase 122c added fail-loudly env validation, the apostrophe in the human-readable error message ("the dispatcher's gcp_project label config sets this") crossed bash 5.2's lexing boundary inside the embedded message even though the whole expression was double-quoted. Fix: rephrased to remove apostrophes.
- **BUG-907b** (gcf runner image not rebuilt after BUG-907 text fix). Process miss: cloudrun runner rebuilt + pushed but gcf was missed. Fix: `make -C tests/runners/github/dockerfile-gcf push-amd64`. Process improvement: any bootstrap.sh / Dockerfile change must trigger `make push-amd64` for ALL 4 runner images.
- **BUG-908** (`Cloud Run Jobs.CreateJob` rejects nested `Job.Name`). Dispatcher's first poll spawn returned `400 BadRequest: job.name must be empty on CreateJobRequest`. Fix: drop `Name: fullName` from the nested `runpb.Job{}` literal — name comes from top-level `JobId`.
- **BUG-909** (Phase-110b-equivalent for GCP — cloudrun + gcf bind-mount → GCS volume translation). github-runner emits `-v /var/run/docker.sock`, `-v /tmp/runner-work:/__w`, `-v /opt/runner/externals:/__e:ro` (+ subpaths); both backends rejected all of them. Fix: mirror `backends/ecs/config.go::SharedVolume` + `backend_impl.go::ContainerCreate` translator with `SharedVolume{Name,ContainerPath,Bucket}` (GCS bucket replaces EFS access point). `ContainerCreate` translator drops `/var/run/docker.sock`, rewrites matching ContainerPath → named-volume ref, drops sub-paths. Named-volume → Cloud Run `Volume{Gcs{Bucket}}` plumbing already existed; `bucketForVolume` short-circuits via `LookupSharedVolumeByName` for operator-pinned volumes. Dispatcher's `spawner.go` adds Volume + VolumeMount + `SOCKERLESS_GCP_SHARED_VOLUMES=runner-workspace=/tmp/runner-work=<bucket>,runner-externals=/opt/runner/externals=<bucket>` env on every spawned runner Cloud Run Job. Config gains `runner_workspace_bucket` REQUIRED TOML field. New `terraform/modules/cloudrun/runner.tf` provisions the bucket. Live bucket: `sockerless-live-46x3zg4imo-runner-workspace`.
- **BUG-910** (runner timeout kills mid-job). After BUG-909 unlocked Initialize containers, cell 5 ran 57s and `Container called exit(124)`. `exec timeout "${RUNNER_IDLE_SECONDS:-60}" ./run.sh --once` wraps the WHOLE process; variable name implies idle-only but the timeout fires unconditionally. Fix: bumped default to 3600s + dispatcher sets `RUNNER_IDLE_SECONDS=3600` env explicitly.
- **BUG-911** (Cloud Run Job task_timeout default 600s). After BUG-910 lifted bash timeout to 3600s, runner-task ran ~10 min and Cloud Run killed it: `Terminating task because it has reached the maximum timeout of 600 seconds`. Fix: spawner sets `Timeout: durationpb.New(3600 * time.Second)` on `TaskTemplate`.

**State at end of session**: BUG-907..911 closed. Cells 5+6 fourth iteration in flight on PR #124. GitLab cells 7+8 still pending — gitlab-runner serverless deployment plan documented in DO_NEXT.md. Sandbox blocks GitLab API calls until user OKs the curl-PAT-to-gitlab.com pattern (analogous to GH PAT → Secret Manager OK earlier).

## Phase 122c — Sockerless-sanctioned cloud builders + terraform-managed dispatcher resources (in flight 2026-05-02)

Following the Phase 122 dispatchers (GCP + Azure), the multi-arch builder pipeline was made symmetric across all three clouds and the terraform modules were extended to provision the cloud resources the dispatchers + bootstrap reference at runtime.

- **Universal multi-arch manifest assembly** in `backends/core/multiarch.go` — single OCI distribution v2 implementation that fetches each per-arch manifest's digest+size+platform via `GET /v2/<repo>/manifests/<tag>` + `GET /v2/<repo>/blobs/<config-digest>`, builds `application/vnd.docker.distribution.manifest.list.v2+json`, and PUTs it. Each per-cloud builder supplies a `tokenForRepo(repo) (string, error)` callback so the helper signs requests with the cloud-appropriate bearer (ECR basic-base64 / GAR ADC / ACR AAD).
- **Per-cloud `AssembleMultiArchManifest`** added to `core.CloudBuildService` and implemented in `aws-common.CodeBuildService` (ECR auth, strips `Basic ` prefix to use as Bearer), `gcp-common.GCPBuildService` (`google.FindDefaultCredentials` + `cloud-platform` scope), and `azure-common.ACRBuildService` (`azcore.TokenCredential.GetToken` + `https://management.azure.com/.default` scope).
- **Bash bootstrap fail-loudly hardening** (BUG-907) — all 4 runner image bootstrap.sh files now validate every required env (`SOCKERLESS_GCR_PROJECT`, `SOCKERLESS_GCR_REGION`, `SOCKERLESS_GCP_BUILD_BUCKET`, etc.) with `: "${VAR:?<msg>}"`. No fallbacks, no auto-discovery; missing env crashes the runner before it tries to register with GitHub. Apostrophe in original error messages caused bash to mis-lex the embedded message; rephrased to remove apostrophes.
- **Dispatcher config required-fields** — `github-runner-dispatcher-gcp/internal/config/config.go` rejects label entries missing `gcp_project`, `gcp_region`, `image`, `service_account`, or `build_bucket`. Spawner derives `BackendKind` from the matched label name (`sockerless-cloudrun` → `cloudrun`, `sockerless-gcf` → `gcf`) and stamps `SOCKERLESS_<GCR|GCF>_{PROJECT,REGION}` + `SOCKERLESS_GCP_BUILD_BUCKET` on the Cloud Run Job container env.
- **Terraform — GCP modules** (`terraform/modules/{cloudrun,gcf}/main.tf`):
  - `google_storage_bucket.build_context` — `<prefix>-build-context` / `<prefix>-gcf-build-context` (1-day lifecycle expiry, mirrors the AWS lambda module's `aws_s3_bucket.build_context`); name surfaced as `output "build_context_bucket"`.
  - `google_project_service.cloudbuild` + `iam` enabled.
  - Runner SA roles extended: `artifactregistry.writer` (push runtime-built images), `cloudbuild.builds.editor` (submit builds), `run.admin` (cloudrun) / `cloudfunctions.developer` + `run.admin` (gcf) for dispatching sub-tasks, `iam.serviceAccountUser` on self (required when creating Cloud Run / Cloud Functions that run as the same SA), `storage.admin` on build_context, `logging.viewer` (read sub-task logs back).
- **Terraform — AWS modules** (`terraform/modules/{lambda,ecs}/main.tf`):
  - `aws_ecr_pull_through_cache_rule.docker_hub` — `docker-hub` prefix → `registry-1.docker.io`. AWS analogue of the GCP `docker-hub` AR remote-proxy. Sockerless rewrites `docker.io/library/<x>:<t>` to `<account>.dkr.ecr.<region>.amazonaws.com/docker-hub/library/<x>:<t>` and the first pull populates the cache.
  - Pull-through cache rules are singleton per (account, region, prefix); both modules expose `manage_docker_hub_pull_through_cache` (default true) so the operator picks the authoritative module when both are deployed in the same account+region.
- **Terraform — Azure modules** (`terraform/modules/{aca,azf}/main.tf`):
  - `azurerm_container_registry_cache_rule.docker_hub` — `docker-hub/*` ← `docker.io/*`. Azure analogue of the GCP/AWS pull-through paths. Requires Standard/Premium ACR SKU; gated by `create_docker_hub_cache_rule` (default false because the existing modules ship with Basic SKU).
  - Managed identity extended with `AcrPush` (push runtime-built images) + `Contributor` on ACR (required to submit ACR Tasks — the sockerless-sanctioned Azure builder).
- **Specs updated** — `specs/CLOUD_RESOURCE_MAPPING.md` § "Sockerless-sanctioned cloud image builders" gained a "Terraform resources that back the builder pipeline" subsection mapping each backend's terraform module → resources → dispatcher env-var consumers. `terraform validate` clean across all 6 modules.

## Phase 121 — Cloud-faithful GCP simulator hardening (CLOSED 2026-05-02)

PR #123 picked up Phase 118 code-complete but with the gcf integration tests deferred for "Phase 121" — the sim was missing too many real-cloud behaviours for the Phase 118 overlay-and-swap path to round-trip. Closed via a bug chain that surfaced layer by layer in CI:

- **OAuth2 + GCS-on-disk + Cloud Build REST** (BUG-893..899): SA JSON's `token_uri` points at the sim, so `cloudbuild.NewRESTClient` mints authenticated requests against the sim's `/token` endpoint (HS256-signed real-shape JWT, not validated by sim's audience handlers); GCS objects persist as files at `<tmp>/sockerless-sim-gcs/<bucket>/<object>` because `GCSObject.data` is unexported and stripped by JSON encoding in `sim.Store`; Cloud Build step executor runs real `docker build` against local daemon and intercepts `docker push` as a local-image-presence verifier (sim is the registry; build already tagged the image with the AR URL).
- **Cloud Functions Gen2 ↔ Cloud Run service linkage** (BUG-901): real GCP creates a backing Run service when you create a Gen2 function. Sim's `CreateFunction` now stamps `ServiceConfig.Service` and inserts a `ServiceV2` row via a shared `seedServiceV2Defaults` helper. The gcf overlay-and-swap path calls `Run.Services.GetService` / `UpdateService` against this row.
- **proto-JSON numeric enums** (BUG-902/903): `run/apiv2` REST PATCH serializes enum fields as numbers (`"launchStage": 2`, `"state": 2`); sim's `ServiceV2.LaunchStage` and `Condition.State` switched from `string` to the existing `enumString` type that accepts both numeric and quoted-string forms.
- **Cloud-faithful invocation** (BUG-904): the gcf overlay's ENTRYPOINT is `sockerless-gcf-bootstrap` — an HTTP server that never exits. Sim was running `docker run` directly on the overlay, so `handle.Wait()` blocked for the full container lifetime; backend's `invokeFunction` hit its 10-min HTTP timeout; `TestGCFContainerLogs` failed at exactly the test's 5-min `ContainerWait` ceiling. Fix mirrors real Cloud Run: new `invokeOverlayContainerHTTP` starts the overlay container detached on a host-mapped port, polls until the bootstrap HTTP listener is up, POSTs the invocation, reads response body + `X-Sockerless-Exit-Code` header (subprocess exit code propagates correctly), then stops + removes the container. Three new sim helpers in `simulators/gcp/shared/container.go`: `StartHTTPContainer` / `StopAndRemoveContainer` / `StreamContainerLogs`.
- **OCI v1 tar layout in core** (BUG-905): the gcf integration tests preload eval-arithmetic into the backend via `docker save | dockerClient.ImageLoad` (general-purpose code path used by any tarball-shipping deployment). `parseImageTarFull` only indexed config blobs at the tar root (`<digest>.json`) — classic docker save layout. Modern docker (BuildKit-built images) writes OCI v1 layout where the config blob lives under `blobs/sha256/<digest>` and `manifest.json`'s `Config` field carries the full path. Sim parser now indexes every file by full path; manifest's Config lookup resolves both layouts. Refactored to reuse the canonical `ociImageConfig` schema from `registry.go` so the in-tar (`docker save`) and over-the-wire (registry-pull) parsers share one source of truth (also picks up `User` and `ExposedPorts` that the local duplicate struct silently dropped).
- **ListFunctions label filtering** (BUG-906): backend's `resolveGCFFromCloud` queries `ListFunctions(filter="labels.sockerless_allocation:\"<short(id)>\"")` to find the function claimed by a specific container; `claimFreeFunction` queries `labels.sockerless_managed:"true" AND labels.sockerless_overlay_hash:"<tag>" AND -labels.sockerless_allocation:*` to find pool-free entries. Sim's handler ignored `?filter=` entirely and returned every function, so backend's `it.Next()` returned the FIRST (oldest = TestGCFArithmeticSuccess) for every test → all subsequent arithmetic tests invoked the wrong function URL, ran TestSuccess's bootstrap-baked CMD, and got "11" back. Sim now implements `matchesFunctionFilter` parsing AND-joined Cloud Logging clauses: `labels.<k>:"v"` (substring/has, the `:` operator), `labels.<k>="v"` (exact), `-labels.<k>:*` (negation + wildcard for "label is unset/empty"). Reuses `parseClause` + `filterClause` from `logfilter.go`.

Diagnostic infrastructure landed in the same chain: `[testmain] <step>` traces to unbuffered stderr (TestMain stdout is buffered by `go test -v` until the first test fires — silent hangs in setup were invisible), 5-minute TestMain watchdog with goroutine dump (emergency-stop only per operator: "exit when work done, not on timeout"), 5s context timeout on `createGCSBucket`, 3-minute timeouts on inner `go build` invocations, `dockerClient.ImageInspect` verification step that fails TestMain upfront with a clear error if the backend's resolved ENTRYPOINT is empty.

PR #123 ALL CI GREEN as of commit `a646602`. Phase 121b (Azure mirror) and Phase 122 (per-cloud runner dispatchers) queued for the next session.

## Phase 119 — sockerless-as-virtual-kubelet (DISCARDED 2026-05-02)

Briefly explored a Kubernetes API surface (`k8s-shim/`) so the runners' kubernetes executors could spawn workloads as k8s Pods backed by sockerless. Operator pulled the plug: "no k8s, ditch the ARC idea and GKE, it's a bad idea, clean up everything." Reverted the `k8s-shim/` module + the GKE/ARC terraform; no live infra was provisioned (GKE API was never enabled in the live project). The Phase 120 cells use the docker executor + existing dispatcher pattern instead.

## Phase 120 — Live-GCP runner cells (4 cells, docker executor, no k8s) (in flight 2026-05-02)

Four cells, all docker-executor + sockerless backends, mirror of Phase 110's AWS cells 1-4 but on GCP. No k8s, no GKE, no ARC. Code complete on the `phase-118-faas-pods` branch; live-cloud verification pending operator runs.

| Cell | Runner | Backend | Runner image | Dispatcher |
|---|---|---|---|---|
| 5 | github-actions-runner | cloudrun | `sockerless-runner-cloudrun` | github-runner-dispatcher-gcp (label `sockerless-cloudrun`) |
| 6 | github-actions-runner | gcf      | `sockerless-runner-gcf`      | github-runner-dispatcher-gcp (label `sockerless-gcf`) |
| 7 | gitlab-runner         | cloudrun | `sockerless-gitlab-runner-cloudrun` | none (long-lived) |
| 8 | gitlab-runner         | gcf      | `sockerless-gitlab-runner-gcf`      | none (long-lived) |

Per BUG-862 (backend ↔ host primitive must match), each runner image bakes the matching sockerless backend so step containers spawned by the runner go through the in-image backend → Cloud Run Job (cells 5/7) or Cloud Run Function with Phase 118d pod overlay for `services:` (cells 6/8). The existing `github-runner-dispatcher-aws` already supports multi-label / multi-backend routing via its `[[label]]` TOML config — cells 5+6 add two new entries; no dispatcher code changes needed (that compensates for github-runner not having a master daemon, per operator).

Each cell runs an identical pipeline: probe caps/kernel/env/parameters, postgres-sidecar localhost peer reachability (proves Phase 118d pod overlay net-ns sharing), git clone + go build + run `simulators/testdata/eval-arithmetic` with five expressions (`3+4*2`=11, `(10-3)*2`=14, `100/5+1`=21, `2*(3+4)-1`=13, `1.5+2.5*2`=6.5). Cell GREEN gate: all probes return non-error output, postgres reachable, Go compile produces a working binary, all five arithmetic invocations exit 0. Per-cell URLs captured in STATUS.md's 4-cell table. Phase 120 closes when all four GREEN.

Files added:
- `tests/runners/{github,gitlab}/dockerfile-{cloudrun,gcf}/{Dockerfile,bootstrap.sh,Makefile}` — four runner image build trees.
- `.github/workflows/cell-5-cloudrun.yml`, `cell-6-gcf.yml`.
- `tests/runners/gitlab/cell-7-cloudrun.yml`, `cell-8-gcf.yml`.
- `tests/runners/gcp-cells/{harness_test.go,go.mod}` — build-tag-gated `gcp_runner_live` harness.
- `manual-tests/04-gcp-runner-cells.md` — operator runbook.

## Phase 118 — live-GCP cloudrun manual sweep + gcf re-architecture + cross-FaaS pool/cache + pod design (2026-05-02, in progress)

**Sub-118d-gcf code complete (2026-05-02)**: FaaS pod overlay for the gcf backend — supervisor-in-overlay pattern per `specs/CLOUD_RESOURCE_MAPPING.md § "Podman pods on FaaS backends"`. Five files touched, all unit-tested:

- `agent/cmd/sockerless-gcf-bootstrap/main.go` — pod-supervisor mode. When `SOCKERLESS_POD_CONTAINERS` (base64-JSON of `[{name,root,entrypoint,cmd,env,workdir}]`) is set at boot, the bootstrap forks one chroot'd subprocess per non-main pod member as a long-lived background sidecar; the main member runs as the foreground subprocess on each HTTP invoke and its stdout becomes the response body. Sidecar stdout/stderr is teed to the supervisor's own stdout with a `[<name>] ` line prefix so Cloud Logging captures peer output under one log stream. Honest namespace-degradation warning printed at startup (`mount-ns: shared (chroot only)`, `pid-ns: shared`) per spec. `prefixWriter`, `parsePodManifest`, `pickPodMain`, `buildPodMemberCmd` all unit-tested with `t.Setenv`-driven roundtrips.
- `backends/cloudrun-functions/image_inject.go` — `PodOverlaySpec` / `PodMemberSpec` / `PodMemberJSON` types, `RenderPodOverlayDockerfile` (multi-stage `COPY --from=<image> / /containers/<name>/` per member; first member's full rootfs `cp -a /. /containers/<name>/` so the supervisor's chroot sees a complete tree before the layered COPYs land), `EncodePodManifest` / `DecodePodManifest` (base64-JSON wire format with optional `container_id` + `image` for cloud_state round-trip), `PodOverlayContentTag` (sha256 of the pod manifest, format `gcf-pod-<16hex>`), `TarPodOverlayContext` (tar.gz for Cloud Build).
- `backends/cloudrun-functions/pod_materialize.go` — `materializePodFunction` is the new code path that ContainerStart enters when `PodDeferredStart` returns `shouldDefer=false` and the pod has >1 member. It (1) builds the merged pod overlay via Cloud Build, (2) atomically deletes the per-member Functions ContainerCreate already created (best-effort; failures log + proceed), (3) stages the stub Buildpacks-Go source idempotently, (4) creates one merged-pod Function with `sockerless_pod=<podName>` + `sockerless_overlay_hash=<contentTag>` + `sockerless_allocation=<short(mainID)>` labels, (5) swaps the throwaway Buildpacks image for the pod overlay via `Run.Services.UpdateService`, (6) HTTP-invokes the function and fans the result to all pod members (each member's `WaitChs` closes; each member shares the response body via `Store.LogBuffers`).
- `backends/cloudrun-functions/backend_impl.go::ContainerStart` — replaced the previous `multi-container pods are not supported by the cloudrun-functions backend` rejection with the deferred-start materialize path. Single-container path unchanged.
- `backends/cloudrun-functions/cloud_state.go::queryFunctions` — when a Function has the `sockerless_pod` label, the `SOCKERLESS_POD_CONTAINERS` env var is decoded and one `docker ps` row is emitted per member. Each row carries the spec's honest namespace-degradation surface: `HostConfig.PidMode = "shared-degraded"`, plus `Config.Labels["sockerless.namespace.{mount,pid,user,cgroup}"] = "shared-degraded"` and `Config.Labels["sockerless.namespace.{network,ipc,uts}"] = "shared"`. (The ideal `HostConfig.MountNamespaceMode = "shared-degraded"` field is not in `api.HostConfig`'s generated schema; Labels carry the same signal alongside the native `PidMode`.)

Live verification deferred — same pattern as sub-118b (Lambda pool reuse). Sub-118d-lambda (mirror to Lambda backend) is the next sub-task; the bootstrap + image_inject design is cross-cutting so the Lambda mirror is mechanical rather than novel.

**Architectural principle codified (2026-05-02)**: backend code lives in three tiers — `backends/core/` (docker-specific functionality + interfaces/types like `core.Driver*`, `core.CloudLogFetchFunc`, `core.CloudBuildService`, `core.InvocationResult`, `core.TagSet`, `core.ImageRef` for cross-backend consistency); `backends/<cloud>-common/` (per-cloud shared code implementing those interfaces — e.g. `gcp-common.GCPBuildService`); `backends/<cloud-product>/` (per-backend specific code). Cross-cutting design patterns (stateless invariant, content-addressed overlay cache, FaaS reuse pool, supervisor-in-overlay pods, cloud-logs cursor+dedup+settle) are codified in `core` interfaces and documented in `specs/CLOUD_RESOURCE_MAPPING.md`. Lifting code is needs-driven: to `<cloud>-common` only when ≥2 backends on the same cloud need it; to `core` only when ≥2 clouds share semantics. See PLAN.md § Phase 118 for the full statement.

**Session boundary 2026-05-02 ~05:00 UTC — operator requested /clear and resume on sub-118d (FaaS pod overlay) → sub-118e (4 new live-GCP runner cells GH/GL × cloudrun/gcf).** Resume pointer + complete state are in [DO_NEXT.md](DO_NEXT.md). Live-cloud project `sockerless-live-46x3zg4imo` stays up; SA key at `/tmp/sockerless-live-46x3zg4imo-key.json`; cloudrun + gcf backends running on `127.0.0.1:{3375,3376}`.

**Sub-118b code done (2026-05-02 ~05:00 UTC)**: Lambda pool reuse implementation. New `backends/lambda/pool.go` with `claimFreeFunction` (race-tolerant tag-based claim) and `releaseOrDeleteFunction` (count free → delete or untag). `backends/lambda/backend_impl.go::ContainerCreate` computes `OverlayContentTag` early, pool-queries before overlay build, claims-or-creates; on miss, tags the new function with `sockerless-overlay-hash` + `sockerless-allocation`. `ContainerRemove` calls `releaseOrDeleteFunction` instead of unconditional `DeleteFunction`. `Config.PoolMax` (`SOCKERLESS_LAMBDA_POOL_MAX`, default 10) — set 0 to disable pooling. Builds + vets clean + unit tests pass. **Live-AWS test pending operator authorization** — Phase 110 infra in eu-west-1 may still be up; operator to confirm before running cost-bearing Lambda invocations.

**gcf full sweep also green (2026-05-02 04:48 UTC)**: `manual-test-real-workloads.sh gcf` reports ALL 16 ROWS PASS — same script that broke before now passes against the gcf backend. The decisive change was adding `CheckLogBuffers: true` to `core.AttachViaCloudLogs` (alongside the existing Logs driver opt-in), so docker /attach surfaces the FaaS HTTP-invoke response body (stored in `Store.LogBuffers`) BEFORE Cloud Logging has indexed the burst. LogBuffers is the authoritative per-invocation source on FaaS backends; Cloud Logging is for observability with ingestion lag. End-to-end Go-build-and-run inside `golang:1.22-alpine` works through the gcf overlay-and-swap path: Cloud Build builds overlay, CreateFunction with stub Buildpacks-Go, UpdateService swaps to overlay, idtoken-authenticated invoke runs the Go program, output reaches docker client via LogBuffers + cloud-logs combo.

**Sub-118a closed (2026-05-02 04:16 UTC)**: BUG-886 (cloud-logs attach burst loss). cloudrun manual sweep `manual-test-real-workloads.sh cloudrun` reports **ALL 16 ROWS PASS** including the bundle-O fast-burst case (11 markers + content lines all delivered to docker client). The combined fix:

- Cursor refactored from strict `timestamp>lastTS` to `>=lastTS` so tied-timestamp entries (Cloud Logging timestamps batched-write entries within sub-millisecond windows) aren't silently dropped.
- Per-cursor `seen[<unix-nano>:<message>]` dedup so the `>=` change doesn't emit duplicates across follow-loop iterations.
- Final-fetch settle window bumped to 6×3s = 18s after terminal-state detection. Cloud Logging worst-case ingestion lag for batched final-flush is ~15s on Cloud Run; 18s gives margin.
- `io.WriteString` error checking on every write; pipe-close exits the loop instead of silently swallowing further entries.

Instrumentation kept in (DEBUG level) so future cloud-logs intermittents surface in the backend log.



First live-cloud track for the GCP backends, against `sockerless-live-46x3zg4imo` (free-trial billing, ephemeral-project workflow). Cloudrun (Cloud Run Jobs) sweep surfaced and closed BUG-877..885 (image-resolve docker-hub remote-repo, Cloud-Audit-log leakage in container stdout, post-mortem logs empty, `ps -a` duplicate/ghost rows + empty Cmd, `--rm` cleanup leaks, log-ingestion final-fetch race). Live-test verified after fixes: clean stdout, zero leaked Cloud Run Jobs, zero ghost containers post-`--rm`.

GCF surfaced BUG-884 (CRITICAL): Cloud Functions Gen2 `CreateFunction` is gated on Buildpacks-compatible source code (Go/Node/Python/Java/Ruby/PHP/.NET) — there is **no documented path to deploy a generic OCI image directly through the Cloud Run Functions API**. Verified via `gcloud functions deploy --image=` ("unrecognized arguments"), `gcloud functions runtimes list` (only language runtimes), and the official Cloud Functions deploy docs ("no mention of deploying ... from pre-built container images"). Both v2 and v2beta protos confirm no `image_uri` field exists.

**Architectural decision (operator-confirmed)**: gcf MUST keep targeting the Cloud Run Functions API surface (otherwise it'd just be the cloudrun backend). The only documented path is `Functions.CreateFunction(stub-Buildpacks-Go-source)` + post-create `Run.Services.UpdateService(image=overlay)` to swap the throwaway Buildpacks image for sockerless's real overlay (`FROM <user-image>` + `sockerless-gcf-bootstrap`). The stub source is the documented escape hatch — analogous to the existing `attachVolumesToFunctionService` post-create surgery for volume mounts that the Functions API doesn't expose. Stub source is identical project-wide so Buildpacks caches after first deploy; overlay images are content-addressed and cached after first build per `(user-image, bootstrap, user-cmd, user-entrypoint, user-workdir)` hash.

**Stateless image cache + Function reuse pool** designed and documented in [specs/CLOUD_RESOURCE_MAPPING.md § Stateless image cache + Function/Site reuse pool](specs/CLOUD_RESOURCE_MAPPING.md#stateless-image-cache--functionsite-reuse-pool-faas-backends). Image cache lives in AR (queried fresh by content tag); pool state lives in cloud-side labels (`sockerless_overlay_hash`, `sockerless_allocation`); claim/release is etag-conditional CAS for cross-instance safety. Backend restart re-derives pool from `Functions.ListFunctions` — zero local state. `SOCKERLESS_GCF_POOL_MAX` (default 10) caps free pool capacity per content-hash; set 0 to disable pooling. Same shape will apply to azf when that track lights up.

The cloudrun fixes already landed; gcf re-architecture is in flight.

## Phase 110 — runner integration (CLOSED 2026-04-30, all 4 cells GREEN)

All four runner-integration cells GREEN against live AWS in eu-west-1:

| Cell | URL |
|---|---|
| 1 GH × ECS | https://github.com/e6qu/sockerless/actions/runs/25075259911 |
| 2 GH × Lambda | https://github.com/e6qu/sockerless/actions/runs/25113565115 |
| 3 GL × ECS | https://gitlab.com/e6qu/sockerless/-/pipelines/2489246177 |
| 4 GL × Lambda | https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943 |

**Cell 4 closure (commit `5fc3e6b`).** Two final bugs blocked GL × Lambda:

- **BUG-875 (start/attach race).** `Server.ContainerStart` did a one-shot `stdinPipes.Load(id)` before firing the Invoke goroutine. The Docker SDK's standard sequence is /create → /start → /attach, so /start arrives BEFORE /attach registers the pipe. With OpenStdin set but no pipe captured, the goroutine invoked Lambda with `Payload: nil`, AWS encoded that as `{}`, the bootstrap piped `{}` into bash's stdin (the gitlab-runner shell-finder Cmd execs `bash`, which reads commands from stdin), bash tried to execute `{}` as a command, exited 1, and Lambda classified the result as `Unhandled`. Every gitlab-runner predefined-helper container hit this. Fix: move pipe lookup INSIDE the goroutine and poll `stdinPipes` for up to 5 s (50 ms tick).
- **BUG-876 (image-resolve).** `resolveImageURI` rejected `docker.io/library/alpine:latest` as "Docker Hub user/org image" because `parseDockerRef` returns `repo="library/alpine"` (with slash) and the user/org guard `strings.Contains(repo, "/")` fired before the library-image rewrite. Fix: strip `library/` prefix immediately when registry is `""`/`docker.io`/`registry-1.docker.io`.

**Diagnostic infrastructure that pinpointed the bash crash.** Added `LogType=Tail` to every `lambda.Invoke` so the function's last 4 KB of stderr returns inline. Combined with `result.Payload` truncated preview, the log line `/bin/bash: line 1: {}: command not found` surfaced directly in the backend log — without it diagnosis would have required correlating CloudWatch streams across many sub-task functions, each named `skls-<containerID12>` and burned shortly after the failure. Pattern memorialised in `feedback_lambda_invoke_diagnostics.md`.

**Cell 3 closure (commit `aa2419a`).** Earlier in the same session: BUG-868 closed via `launchAfterStdin` per-stage Fargate task plus three lifecycle fixes — `PendingCreates` drop in both `launchAfterStdin` branches, `ContainerInspect` override forcing `Status="running"` while `WaitCh` open, attach-driver stage-boundary barrier (5 s wait for `markRunning` before kicking off the cloud-logs follower).

**Volumes / env / image bookkeeping for Lambda gitlab-runner.** Lambda's single-FSC + 4 KB Environment + 256 KB Cmd budget intersects awkwardly with gitlab-runner's resource use:
- Volumes collapse onto the workspace shared AP via sub-paths (gitlab-runner cache + build → workspace.subpath). Lambda's single-FSC constraint can't host two distinct AP ARNs in one function.
- Env filter drops `CI_*` / `FF_*` / `GITLAB_*` prefixes + 2 KB hard cap (gitlab-runner re-exports them at the top of every stage script via the `eval $'export …'` preamble, so config-level forwarding is pure overhead).
- Sha256-only image refs from gitlab-runner resolve via the local image Store before falling through to ECR pull-through cache routing.

---

Earlier in flight (PR #122 — Phase 110a). Architecture exploration that landed two important separations:

**GitLab vs GitHub runner — dispatcher pattern vs worker pattern.** GitLab Runner is a *dispatcher*: the master polls GitLab and uses the docker executor's `docker create + docker exec` to spawn the job container. The master is just a docker client; it never bind-mounts its own filesystem; it can run anywhere with `--docker-host` pointing at sockerless. **Cells 3 + 4 need zero new sockerless code.** GitHub Actions Runner is a *worker*: it *is* the workspace. For `container:` jobs it does `docker create -v /home/runner/_work:/__w …` — host bind mounts that assume a shared filesystem with the spawned container. On Fargate two tasks don't share filesystems by default. **Cells 1 + 2 require both a topology change (runner-as-ECS-task with EFS-backed workspace) and a sockerless feature (bind-mount → EFS translation).**

**`github-runner-dispatcher-aws` is sockerless-agnostic.** A new top-level Go module (own `go.mod`, separate dep tree) that speaks only the public Docker API / CLI. (Originally named `github-runner-dispatcher`; renamed for naming consistency 2026-05-02 alongside the GCP + Azure variants from Phase 122.) Pointed at local Podman it spawns runners locally; pointed at sockerless via `DOCKER_HOST` it spawns runners in Fargate. The dispatcher doesn't know sockerless exists — same `docker run` call, different daemon. Sockerless's role (sidecar injection, EFS bind-mount translation) is invisible to the dispatcher; it's encoded in (a) image labels set at runner-image build time and (b) a pre-registered ECS task definition that sockerless's ECS backend recognizes via the image label and dispatches to.

**Static task definition for the runner-task.** The runner-task's shape (multi-container: runner + sockerless sidecar; EFS-backed workspace; IAM role; log config) lives in Terraform as a pre-registered ECS task definition with a stable ARN. Sockerless's ECS backend, when it sees an image with `LABEL com.sockerless.ecs.task-definition-family=sockerless-runner`, calls `RunTask --task-definition sockerless-runner:LATEST` with per-job container-override env vars (`REG_TOKEN` / `LABELS` / `RUNNER_NAME`) — no dynamic task-def composition. Operator owns the runner-task spec; sockerless just dispatches. (Job-tasks the runner subsequently spawns inside the workflow keep dynamic composition; that's where the bind-mount-via-EFS feature plugs in.)

Splits into two PRs: 110a (cells 3 + 4 + dispatcher skeleton — closes PR #122) and 110b (sockerless EFS feature + runner image push to ECR + cells 1 + 2). See [PLAN.md § Phase 110](PLAN.md) for the full plan, [docs/RUNNERS.md](docs/RUNNERS.md) for token strategy + wiring.

**Bugs surfaced + fixed in 110a (PR #122):** BUG-845 (Lambda live env was us-east-1; realigned to eu-west-1 + sockerless-tf-state), BUG-846 (Docker Hub PAT path replaced with AWS Public Gallery routing for `alpine`-style library refs — verified live: `docker run alpine echo hi` exits 0 from Fargate), BUG-847 (GH runner asset URL `darwin` → `osx`; pinned 2.319.1 → 2.334.0), BUG-848 (`docker info` reported hardcoded `amd64` — now reflects required `SOCKERLESS_ECS_CPU_ARCHITECTURE` / `SOCKERLESS_LAMBDA_ARCHITECTURE` env vars; ECS RuntimePlatform + Lambda Architectures wired through), BUG-849 (Linux runner container approach: `--add-host host-gateway` syntax fails on Podman 5.x because Podman natively provides `host.docker.internal` and `host.containers.internal` aliases; drop the `--add-host` flag entirely + install docker CLI in the runner image so `container:` directive can do its docker create + exec).

**Bugs surfaced + fixed in 110b (sockerless ECS feature work):** BUG-850 (host-bind-mount → EFS access-point translation in sockerless ECS backend — new `Config.SharedVolumes` config, sub-path drop, docker.sock drop, runner-image entrypoint pre-populates externals on first start), BUG-851 (ECS server overrides `s.Drivers.Network` with metadata-only `SyntheticNetworkDriver`; `LinuxNetworkDriver`'s real `ip netns` calls fail in Fargate and the abstraction is wrong for cloud — VPC SG + Cloud Map are the cloud-side primitives), BUG-852 (sub-tasks include both per-network SG and operator default SG so EFS mount targets stay reachable when peer-isolation SGs are dynamically created per docker network), BUG-853 (`cloudExecStart` waits for `ExecuteCommandAgent.LastStatus == RUNNING` before invoking ECS `ExecuteCommand` — the agent comes up 5–30 s after task RUNNING; without the wait every first `docker exec` after a freshly-started job container fails with `InvalidParameterException: ... execute command agent isn't running`).

**Live infra additions (Terraform `terraform/modules/ecs/runner.tf`):** `sockerless-live-runner` ECS task definition (single-container: actions/runner image with sockerless-backend-ecs baked in, runs sockerless on `tcp://localhost:3375` in background, then registers + runs the runner with `--ephemeral`), two EFS access points (`runner_workspace` rooted at `/sockerless-runner-workspace`, `runner_externals` rooted at `/sockerless-runner-externals`), separate `sockerless-live-runner-task-role` IAM role with full ECS dispatch + EC2 + EFS + ECR + ServiceDiscovery + `ecs:ExecuteCommand` + SSM messages perms (broader than the regular task role because sockerless inside the runner-task is itself a docker daemon dispatching further sub-tasks). Runner image at `729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live:runner-amd64` carries `LABEL com.sockerless.ecs.task-definition-family=sockerless-runner`. Harness's ECS dispatch path (`runEcsRunnerTask` in `tests/runners/github/harness_test.go`) calls `aws ecs run-task` with container overrides for the per-cell registration token + labels.

**Phase 110 — additional learnings (2026-04-28 / 2026-04-29):**

*gitlab-runner script-file delivery is via attach stdin, not bind-mount.* Cell 3 (GitLab × ECS) reached the user-script container start stage, where the alpine container exited 1 in <1 s. Root cause (BUG-859): gitlab-runner's docker executor pipes each script step through the hijacked `docker attach` connection's stdin; sockerless's ECS attach driver was `core.NewCloudLogsAttachDriver` (read-only, `cloudAttachStream.Write` discarded). Fix: new `ecsStdinAttachDriver` that captures stdin into a per-cycle `stdinPipe`; ContainerStart, when ECSState records `OpenStdin`, defers RunTask via a goroutine that waits for stdin EOF then bakes the buffered bytes into the task definition's `Entrypoint=[sh,-c]` + `Cmd=[<script>]`. Per-cycle: each gitlab-runner script step gets a fresh pipe; PendingCreates is preserved across cycles because `CloudState.containerFromTask` synthesises Config without OpenStdin/Binds. Lambda backend mirrors this (BUG-860) — same shape, but the buffered stdin becomes the `lambda.Invoke` Payload, since the existing bootstrap already pipes Payload to the user entrypoint as stdin.

*`backend ↔ host primitive must match` is a class-of-bug rule (BUG-862, CRITICAL).* The runner-Lambda image was originally baking `sockerless-backend-ecs` and dispatching `container:` sub-tasks via `ecs.RunTask` to Fargate "to avoid Lambda-in-Lambda recursion". This is a category error: each cloud's own primitives are the right answer for sub-task workloads on that cloud, even when 15-min caps or concurrency limits make it harder. Documented as universal rule #9 in `specs/CLOUD_RESOURCE_MAPPING.md` with a per-backend runner-on-FaaS dispatch table; codified in `MEMORY.md` workflow rules; flagged at top of `BUGS.md` so future audits catch it. Fix: runner-Lambda now bakes `sockerless-backend-lambda`, dispatches sub-tasks as fresh image-mode container Lambdas sharing the workspace EFS access point via `FileSystemConfig`. Terraform IAM swapped from ECS dispatch perms to Lambda dispatch perms (`lambda:CreateFunction/Invoke/Delete/Get/UpdateConfiguration/Tag/ListFunctions`); env vars all `SOCKERLESS_LAMBDA_*`; agent + bootstrap binaries now staged into `/opt/sockerless` for the in-Lambda backend's image-inject path.

*github-runner-dispatcher-aws Phase 110a deliverable shipped.* Top-level Go module at `github-runner-dispatcher-aws/` (originally `github-runner-dispatcher/`; renamed in PR #123 alongside the new GCP + Azure variants). Sockerless-agnostic — only stdlib + BurntSushi/toml. Polls `GET /repos/{r}/actions/runs?status=queued` every 15 s with a 5-min seen-set; mints ephemeral runner registration tokens on demand (`POST /actions/runners/registration-token`); spawns one `docker run --rm -d --pull never --label sockerless.dispatcher.* …` per queued job via shell-out to the local `docker` CLI (whatever `DOCKER_HOST` points at). State recovery: each spawned container is stamped with `sockerless.dispatcher.{job_id,runner_name,managed_by}` labels — on startup the dispatcher rebuilds the seen-set from `docker ps --filter label=…`; no on-disk state. GC sweep every 2 min reaps exited dispatcher containers + offline `dispatcher-*` GitHub runners. Graceful shutdown bounded to 30 s drains in-flight runners + their GitHub registrations. `--cleanup-only` flag for one-shot drain (cron / pre-redeploy).

*CodeBuild + S3 build-context for no-local-docker rebuilds.* Two-purpose addition. (1) Cell-2 manual rebuild: `cd tests/runners/github/dockerfile-lambda && make codebuild-update` tars the build context, uploads to `s3://sockerless-live-build-context/`, triggers the `sockerless-live-image-builder` CodeBuild project (linux/amd64 standard, privileged_mode for docker), polls until SUCCEEDED, then `aws lambda update-function-code --publish` + `wait function-updated-v2`. Drop-in replacement for `make all` when no Docker Desktop / Podman VM is up locally. (2) Sub-task image builds at runtime: when the runner-Lambda's bundled `sockerless-backend-lambda` spawns a per-`container:` sub-task, it must build a fresh image-mode container Lambda (user image + injected agent + bootstrap). The Lambda runtime has no docker daemon, so `awscommon.CodeBuildService` (existing code path; pre-Phase 110) delegates to this same CodeBuild project. `SOCKERLESS_CODEBUILD_PROJECT` + `SOCKERLESS_BUILD_BUCKET` env vars now wired on the runner-Lambda function. The CodeBuild buildspec is inlined: download context tarball from S3 via `BUILD_CONTEXT_KEY`, ECR login, `docker buildx build --platform linux/amd64 -t $IMAGE_URI --push`. 24-hour S3 lifecycle expiration on the build-context prefix means no debris.

*Where we landed (2026-04-29).* Cell 1 GREEN; cells 2/3/4 fully fixed in source, pending operator-driven runtime steps (one `terragrunt apply`, one `make codebuild-update`, one sockerless restart command). All paths forward documented per-cell in [PLAN.md § Phase 110 — paths forward to GREEN](PLAN.md), per-bug in [BUGS.md](BUGS.md), and as a sequence in [DO_NEXT.md](DO_NEXT.md). Class-of-bug rule (backend ↔ host primitive must match) codified at top of `BUGS.md`, in `MEMORY.md`, and as universal rule #9 in `specs/CLOUD_RESOURCE_MAPPING.md` so future audits re-flag any cross-pollination as P0.

*Cell-2 push through five Lambda primitive walls (2026-04-29).* Cell 2 was driven live against the runner-Lambda this session. Each wall exposed a different Lambda-vs-Docker mismatch and was answered by aligning sockerless's Lambda-side translation with the cloud's actual primitive (and updating `specs/CLOUD_RESOURCE_MAPPING.md` so the implementation rule is documented, not improvised):

  1. **BUG-869 — CodeBuild buildspec produced OCI manifests.** `aws lambda update-function-code` rejects the runner-Lambda image because Lambda image-mode requires Docker schema 2. Fixed by switching the inline buildspec to `docker buildx build --provenance=false --output type=image,name=$IMAGE_URI,oci-mediatypes=false,push=true`. (Lambda's preference for Docker schema 2 is a hard AWS constraint; sockerless's CodeBuild output now matches it. OCI everywhere else stays the default.)
  2. **BUG-870 — operator-provisioned EFS access points have no `sockerless-managed=true` tag.** `accessPointARN` was iterating `ListManagedAccessPoints` (which filters by tag), so it couldn't find APs created by terraform. Fixed by adding `EFSManager.DescribeAccessPoint(apID)` that calls `efs.DescribeAccessPoints(AccessPointId=apID)` directly. Sockerless still owns volume CRUD via the managed-tag filter; ARN lookup of pre-provisioned APs goes through the unfiltered path.
  3. **BUG-871 — Lambda allows AT MOST 1 FileSystemConfig per function and `localMountPath` must match `/mnt/[A-Za-z0-9_.\-]+`.** Verified live with a 3-error `ValidationException`. The runner's `docker create -v /tmp/runner-state/_work:/__w -v /tmp/runner-state/externals:/__e:ro` produced two FSCs at non-`/mnt/` paths. The fix is the Lambda-correct mapping: `fileSystemConfigsForBinds` collapses entries with identical AP ARN into one FSC mounted at `/mnt/sockerless-shared`; `SharedVolume` gains an `EFSSubpath` field; the lambda backend emits `SOCKERLESS_LAMBDA_BIND_LINKS` env on sub-task `CreateFunction`; `agent/cmd/sockerless-lambda-bootstrap` materialises the BIND_LINKS as symlinks (idempotent across execution-environment reuse) before the user entrypoint runs. Documented in `specs/CLOUD_RESOURCE_MAPPING.md` § "Lambda bind-mount translation" — same nature as sockerless's reverse-agent translation of `docker exec` for Lambda (no native primitive, sockerless implements the Docker semantic on top of what the cloud actually offers). Runner-Lambda terraform + bootstrap.sh now stage workspace + externals as EFS subpaths under `/mnt/runner-workspace/{_work,externals}` so the sub-task Lambda — sharing the same access point — sees the same files.
  4. **BUG-872 — pull-through cache prefix mismatch with ECS.** Lambda's `resolveImageURI` hardcoded `ecr-public` while ECS computes `strings.ReplaceAll(registry, ".", "-")` → `public-ecr-aws`. Two SIBLING ECR cache repos with the same upstream but separate caches; only the ECS-style prefix had been populated. Fixed by deriving Lambda's prefix the same way as ECS so both backends share one cache repo.
  5. **BUG-873 — Lambda image-mode rejects OCI manifests AND requires a Lambda Runtime API client at the entrypoint.** Closed by Phase 115 (commit `d5073b4`). The architecturally honest fix: route ALL Lambda CreateFunction calls through `BuildAndPushOverlayImage` — not gated on `CallbackURL`. The overlay (1) bakes `sockerless-lambda-bootstrap` as ENTRYPOINT (resolves Runtime-API gap), (2) is built via plain `docker build` + `docker push` which produces Docker schema 2 (resolves manifest gap), (3) preserves user's original ENTRYPOINT/CMD as `SOCKERLESS_USER_*` env vars. `BuildAndPushOverlayImage` now takes a `core.CloudBuildService` dependency; when supplied + Available(), routes through CodeBuild (no local docker daemon needed — works inside the runner-Lambda). `OverlayContentTag(spec)` produces a stable sha256-based tag so identical inputs reuse a cached ECR image. `awscommon.CodeBuildService.Build` honours fully-qualified Tags[0] image refs and now downloads + extracts the tar.gz context explicitly in pre_build (CodeBuild's S3 source type only auto-extracts ZIPs). Verified live (workflow run 25105165208): CodeBuild SUCCEEDED + Lambda CreateFunction returned the ARN.
  6. **BUG-874 — Lambda has no synchronous "start" primitive (Phase 116).** Resolved by registering a single deployment-time `ExecDriver` based on `SOCKERLESS_CALLBACK_URL`: Path A (reverse-agent, when set) or Path B (`lambdaInvokeExecDriver`, when unset). Path B marshals each `docker exec` as a JSON envelope `{"sockerless":{"exec":{"argv":[...],...}}}`, fires `lambda.Invoke` synchronously, decodes `{"sockerlessExecResult":{...}}` from the response, writes stdout+stderr into the docker-exec hijacked conn with proper multiplexed-stream framing for non-tty execs, returns the exit code. ContainerStart now blocks synchronously on `FunctionActiveV2Waiter` so docker exec doesn't race a Pending function. Workdir + bind-link symlinks are baked into the overlay Dockerfile at build time (Lambda's runtime root fs is read-only outside `/tmp` + EFS, and Lambda's pre-bootstrap chdir would fail on symlink targets that only resolve at runtime). Verified live: cell 2 GH × Lambda GREEN at workflow run https://github.com/e6qu/sockerless/actions/runs/25113565115. Sub-task Lambda CloudWatch (`/aws/lambda/skls-985fd889cec7`) shows `bootstrap exec: argv=[sh -e /__w/_temp/...sh] workdir=/__w/sockerless/sockerless env=44-vars stdin=0-bytes` then real subprocess output (`hello from sockerless lambda`, `Wed Apr 29 14:04:48 UTC 2026`) — two warm invocations, exit 0.

  7. **BUG-868 (open) — gitlab-runner skips step_script (Phase 114 architectural).** Verified live with `--debug` enabled (https://gitlab.com/e6qu/sockerless/-/jobs/14144936826 + 14146329550): trace shows `prepare_script → get_sources → archive_cache_on_failure → upload_artifacts_on_failure → cleanup_file_variables → ERROR exit 1` with NO `step_script` section in between. `archive_cache_on_failure` is the failure-path cleanup chain — gitlab-runner detected the `get_sources` task exit code > 0 (or no actual git fetch happened) and routed away from `step_script`.

      Diagnostic narrowed the root cause precisely. gitlab-runner's docker executor creates BOTH the helper container (`-predefined` suffix) AND the build container with the SAME stdin-reading entrypoint — a `sh -c "if [-x bash]; then exec bash; fi"` shell-detect wrapper that exec's into bash. Per stage, `docker start` re-runs that entrypoint (real Docker re-runs ENTRYPOINT on every `start` of a STOPPED container) and `docker attach -i` pipes the stage's generated shell script as stdin. The shell reads, executes, exits when stdin EOFs. The "Running on $(hostname)..." banner per stage is just the FIRST LINE of the generated shell script.

      Fargate breaks this in two ways: tasks are not restartable (each `RunTask` is a new ARN), and Fargate has no runtime-stdin channel to a running task. Sockerless's BUG-859 `launchAfterStdin` baked stdin into a fresh per-cycle task's `Entrypoint=["sh","-c"], Cmd=[<script>]` — works for the build container but two attempts on the predefined helper went badly:
      - Synchronous `RunTask` with the helper's bash-wrapper entrypoint preserved (BUG-867 filter ON): wrapper exec's into bash, bash reads stdin which closes immediately on Fargate, bash exits 0 in <1s — the stage "succeeds" but did no work, gitlab-runner detects empty workspace at `step_script` time and routes to failure path.
      - `launchAfterStdin` enabled for the helper too (BUG-867 filter OFF, with a 3-second stdin-EOF timeout to handle log-streaming attaches that never write): predefined helper hung 13 minutes (workflow run 14146329550) — the runner's log-streaming `/attach` keeps stdin open without piping, our timeout fired, ran original entrypoint (bash-wrapper expects stdin) → no-stdin-on-Fargate hang.

      Phase 114's architecturally correct answer: launch the predefined helper as a long-lived Fargate task (`Cmd=["while true; do sleep 60; done"]`) ONCE, then dispatch each stage's stdin script via `ecs.ExecuteCommand` (SSM Session Manager) against the live task. The task's `enableExecuteCommand: true` is already set; the existing SSM frame-capture machinery from Round-8 carries the per-session stdin/stdout/exit-code; ContainerStart's per-cycle path reuses the cached task ARN instead of `RunTask`. ~400-600 lines, comparable to Phase 116. Documented in `specs/CLOUD_RESOURCE_MAPPING.md` § "ECS gitlab-runner script delivery" with the gitlab-runner architectural refresher inline.

      Cell 4 (Phase 117): the Lambda translation is simpler since `lambda.Invoke` already provides per-stage dispatch — each gitlab-runner stage's stdin-piped script becomes one `Invoke` with a SCRIPT envelope `{"sockerless":{"script":{"body":"<base64>",...}}}`. The bootstrap parses the envelope, runs `bash -c "<body>"`. EFS carries cross-stage state. Independent of Phase 114; ~250-400 lines.

      Phase 114's first iteration (filter removal + stdin-EOF timeout) regressed cell 3 from "4 stages succeed-but-no-work" to "13-minute hang on prepare_script". Reverted at commit `533312d`; doc + plan updated to document the longer architectural path. Cell 3 closure now gated on Phase 114's full implementation.

Cloud-resource mapping audit follow-up (Task #95): same Lambda-style review on Cloud Run, Cloud Run Functions, Azure Container Apps, and Azure Functions volume + bind primitives. Each cloud will get its own `*-bind-mount translation` subsection in `specs/CLOUD_RESOURCE_MAPPING.md` documenting how Docker `-v` maps to that platform's actual storage attach rules.

## Phase 109 — strict cloud-API fidelity sweep (PR #121, merged 2026-04-27)

19 audit items closed. Triggered by PR #120 CI failures that traced back to synthetic responses. Goal: every sim slice sockerless touches behaves like the real cloud — same wire shape, same validation rules, same state transitions, same SDK / CLI / Terraform-provider compatibility.

**Why these mattered for runner work.** The runner phases (106/107/110) drive workloads at much higher fidelity than the SDK/CLI matrix did. Every fake the runner trips becomes a live-cloud bug. Stamping them out in the sim now keeps the runner integration (Phase 110) from chasing wire-format mismatches under load.

**Closures, grouped by cloud:**

- **AWS** — Lambda VpcConfig from real subnet CIDR; `awsRegion()` / `awsAccountID()` env-var-configurable identity; Secrets Manager + SSM Parameter Store + KMS Encrypt/Decrypt + DynamoDB (with Terraform state-lock semantics — `attribute_not_exists(LockID)` succeeds first time, `ConditionalCheckFailedException` on contention).
- **GCP** — `compute.firewalls`, `compute.routers` + Cloud NAT, `iam.serviceAccounts.generateAccessToken`, operations endpoint persistence (no synthetic `done=true` for unknown ops).
- **Azure** — IMDS metadata token endpoint, Blob Container ARM control plane, NSG rule priority+direction uniqueness, Private DNS AAAA/CNAME/MX/PTR/SRV/TXT records, NAT Gateways + Route Tables, `Azure-AsyncOperation` polling for Container Apps + Jobs, Key Vault (ARM control + data plane subdomain routing), ARM `SystemData.createdAt` preserved across updates (lastModifiedAt stamped fresh).
- **No-fakes audit on test fixtures** — clean. All hardcoded IDs are sim-pre-registered defaults, configuration values, or intentional negative-test inputs.

The pattern across all 19: handlers that returned hardcoded values, accepted invalid input, or skipped validation real cloud APIs enforce. Fixed by making each handler walk the real-cloud shape, including the failure modes (e.g., DynamoDB ConditionalCheckFailedException, ARM `SystemData.createdAt` preservation).

## Phase 108 — sim-parity matrix audit + Phase 106/107 harness scaffolding (PR #120, merged 2026-04-27)

Cumulative 22 bug closures + framework + matrix work.

- **Phase 104 framework migration complete.** 13 typed driver interfaces + `DriverContext` envelope + `Driver.Describe()` composition rule + `SOCKERLESS_<BACKEND>_<DIMENSION>` override resolver. Every dispatch site (Exec, Attach, Logs, Signal, ProcList, FSDiff, FSRead/Write/Export, Commit, Build, Registry) flows through `TypedDriverSet`. Cloud-native typed drivers across every backend — 44/91 cells in the per-backend matrix are cloud-native, bypassing api.Backend; the rest stay on legacy adapters whose api.Backend method already does the cloud-native thing. Per-backend default-driver matrix in [specs/DRIVERS.md](specs/DRIVERS.md).
- **Type tightening underway.** `core.ImageRef` domain object (`{Domain, Path, Tag, Digest}` + `ParseImageRef` + `String()`) lands at the typed `RegistryDriver.Push/Pull` boundary. Handlers parse once at dispatch; the typed driver receives a structured value.
- **Phase 105 waves 1-3.** Golden shape tests for 8 libpod handlers: `pod inspect` + `pod stop`/`kill` Errs serialization + `info`, `containers/json`, rm-report, `images/pull` stream, `networks/json`, `volumes/json`, `system/df`.
- **Phase 108 closed (77/77 ✓).** [`specs/SIM_PARITY_MATRIX.md`](specs/SIM_PARITY_MATRIX.md) audit walked all 77 cloud-API rows (33 AWS / 16 GCP / 28 Azure). Standing rule strengthened (PLAN.md principle #10): any new SDK call added to a backend must update the matrix + add the sim handler in the same commit.
- **Phase 106/107 harnesses shipped.** [`tests/runners/github/harness_test.go`](tests/runners/github/harness_test.go) and [`tests/runners/gitlab/harness_test.go`](tests/runners/gitlab/harness_test.go) — build-tag-gated end-to-end harnesses. Live-cloud runs against real repos pending Phase 110.

**Repo-wide cleanup also landed.** All `Phase NN` / `BUG-NNN` references stripped from Go source comments + docs (the metadata stays in BUGS.md / git log / PR descriptions). Manual-tests directory consolidated; redundant simulator-parity docs deleted; 633 task-archive `.md` files dropped from `_tasks/done/`.

## Round-7 / Round-8 / Round-9 live-AWS sweeps (PRs #117, #118)

Three rounds of live-AWS testing in `eu-west-1` against ECS + Lambda, replaying [manual-tests/02-aws-runbook.md](manual-tests/02-aws-runbook.md) against [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md). 46 bugs closed total (BUG-770..819).

**Headlines:**
- **Round-7 (PR #117).** ImageRemove correctness, ECS task lifecycle (rename, restart, kill-signal mapping), libpod compat, OCI push auth + config-blob, Lambda bootstrap PID + heartbeat, registry persistence robustness.
- **Round-8 + Round-9 (PR #118).** Real registry-to-registry layer mirror (BUG-788, closes 4 retroactive bugs); live SSM frame capture → exit-code marker; sync `docker stop`; per-network SG isolation; Lambda Active-waiter; per-cloud `null_resource sockerless_runtime_sweep` so `terragrunt destroy` is self-sufficient.

These rounds proved the live-AWS path before Phase 110 starts integrating real CI runners against it.

## Older closed phases (compressed)

Per-bug detail in BUGS.md, code-level detail in `git log`.

| Phase(s) | Headline | PR |
|---|---|---|
| 96 / 98 / 99 / 100 / 101 / 102 + 13-bug audit | Reverse-agent + SSM machinery for `docker top / stat / cp / get-archive / put-archive / export / diff / commit / pause`. Shared `core.ReverseAgentRegistry` + `HandleReverseAgentWS`. Sim parity for cloud-native exec/attach. | #115 |
| 91–95 | Real per-cloud volumes — `docker volume create` provisions EFS access points (AWS), GCS buckets (GCP), Azure Files shares (Azure). FaaS invocation-lifecycle tracker + GCP label-value charset compliance. | #114 |
| 87 / 88 / 89 / 90 | Cloud Run Services + ACA Apps (internal-ingress workloads, peers via Cloud DNS / Private DNS CNAMEs). Stateless audit + no-fakes sweep. | #113 |
| 86 | Simulator parity + Lambda agent-as-handler. Pre-commit contract: every new sim handler needs SDK+CLI+terraform coverage. | #112 |

Earlier phases (≤ Phase 85) are summarised in PR descriptions and git log.

## Stack & structure

- **Simulators** — `simulators/{aws,gcp,azure}/`, separate Go modules. `simulators/<cloud>/shared/` for container + network helpers; `sdk-tests/` / `cli-tests/` / `terraform-tests/` for external validation.
- **Backends** — 7 backends (`backends/docker`, `backends/ecs`, `backends/lambda`, `backends/cloudrun`, `backends/cloudrun-functions`, `backends/aca`, `backends/azure-functions`). Each a separate Go module. Cloud-common shared: `backends/{aws,gcp,azure}-common/`. Core driver + shared types: `backends/core/`.
- **Agent** — `agent/` with sub-commands for the in-container driver + Lambda bootstrap. Shared simulator library: `github.com/sockerless/simulator`.
- **Frontend** — Docker REST API. `cmd/sockerless/` zero-dep CLI. UI SPA at `ui/` (Bun / React 19 / Vite / React Router 7 / TanStack / Tailwind 4 / Turborepo), embedded via Go `!noui` build tag.
- **Tests** — `tests/` for cross-backend e2e, `tests/upstream/` for external-suite replays (act, gitlab-ci-local), `tests/runners/{github,gitlab}/` for real-runner harnesses (build-tag-gated), `tests/terraform-integration/`, `smoke-tests/` for per-cloud Docker-backed smokes.
