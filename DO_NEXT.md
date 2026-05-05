# Do Next

Resume pointer. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); architecture in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Resume pointer (2026-05-05 v32 — cell 8 v15 in flight; cells 5+6 next)

User goal: **all 4 GCP cells (5, 6, 7, 8) GREEN with full workflow + evidence + executing where they're supposed to**. Cell 7 done; cells 5/6/8 outstanding.

### Cell 8 — currently in flight (v15)

**Pipeline 2501668159** (gitlab-cell-8-test branch). Rev `00047-45f`, digest `sha256:ee7e5029...`.

This iteration adds explicit log lines that should reveal why "No such container" fires at runner cleanup time:

```
ContainerStart: ENTRY (every call)
ContainerStart: NOT FOUND in PendingCreates (early return path)
ContainerStart: resolved (after PendingCreates.Get success)
ContainerStart: network-pod decision (after shouldDeferOrMaterializeNetworkPod)
materializePodService: entry/exit (when materialize runs)
```

If `ContainerStart: ENTRY` doesn't appear for the build container at all → routing issue or gitlab-runner is not calling `/containers/{id}/start` for this container.

If `ContainerStart: ENTRY` appears but no `resolved` → `PendingCreates.Get(ref)` is missing the entry; ContainerCreate's `Put` either failed or was deleted by some path between `ContainerCreate` and `ContainerStart`.

If `ContainerStart: resolved` appears with `running=true` already → some other code path marked it Running.

If everything appears but materialize completes in time → the bug is in `cloud_state.queryPodServiceContainers` not finding the Service mid-materialize (resolvePodServiceFromCloud now does GetService follow-up on abbreviated annotations).

### If cell 8 v15 is GREEN

1. Mark BUG-953 closed in BUGS.md.
2. Update STATUS.md / WHAT_WE_DID.md cell 8 row to ✅.
3. Move to cells 5+6.

### If cell 8 v15 still fails

Diagnose from the new logs. Specific failure modes to expect:
- ContainerStart never fires → check whether gitlab-runner calls `/containers/{id}/start` (could be using `docker run` semantics that bypass start, or container was already started by a previous deploy)
- ContainerStart fires for build but PendingCreates lookup misses → check ContainerCreate path; some new deletion was introduced
- ContainerStart resolves but never reaches network-pod branch → state.Running was already true (some other code path marks running prematurely)
- Everything looks right but Service-side query fails post-materialize → `resolvePodServiceFromCloud`'s GetService follow-up isn't matching; check whether label `sockerless_allocation` matches `shortAllocLabel(containerID)` exactly

### Cells 5 + 6 — runner-task images already exist

**Per user directive 2026-05-05: dispatcher stays generic.** No dispatcher code changes needed. The sockerless+vanilla-runner pairing lives inside the runner image at `tests/runners/github/dockerfile-{cloudrun,gcf}/`:

- `tests/runners/github/dockerfile-cloudrun/` — vanilla actions/runner + sockerless-backend-cloudrun + bootstrap.sh that launches sockerless on `:3375` in background, then registers the runner with `RUNNER_REG_TOKEN`. `DOCKER_HOST=tcp://localhost:3375` set as ENV.
- `tests/runners/github/dockerfile-gcf/` — same pattern, port `:3376`, `sockerless-backend-gcf`.

**Implementation plan**:

1. Rebuild `sockerless-backend-cloudrun` + `sockerless-backend-gcf` if needed (cell 8 fix should propagate).
2. `cd tests/runners/github/dockerfile-cloudrun && make push-amd64` — produces a new digest in AR `…/sockerless-live/runner:cloudrun-amd64`.
3. Same for `dockerfile-gcf`.
4. Update dispatcher TOML config in `~/.sockerless/dispatcher-gcp/config.toml` (or wherever the live dispatcher Cloud Run Service reads it) to point each label at the right runner-task image.
5. Trigger via `gh workflow run cell-5-cloudrun.yml` and `gh workflow run cell-6-gcf.yml`.
6. Watch dispatcher logs (`gcloud logging read resource.labels.service_name=github-runner-dispatcher-gcp`) → `Jobs.CreateJob + RunJob` should fire.
7. Watch the runner-task Job execution logs → vanilla actions/runner should pick up the workflow, run the steps via `DOCKER_HOST=…`, exit cleanly.

Cell 6 inherits cell 8's gcf stack — once cell 8 is GREEN, cell 6 should follow.

### Documentation update at branch close

After all 4 cells GREEN:
1. State save: STATUS.md / WHAT_WE_DID.md / DO_NEXT.md / PLAN.md / BUGS.md / MEMORY.md.
2. Update PR #123 title + description to cover all the changes (user requested earlier).
3. PR is ready for user review (never merge — user handles merges).

### Single-line summary

> Cell 7 GREEN. Cell 8 v15 in flight (BUG-953 has 3 architectural fixes shipped: AR tag precheck, multi-container Service direct deploy, PendingCreates speculative-running marker; v15 adds diagnostic logs to pin the remaining "No such container" failure mode). Cells 5/6 just need runner-task image rebuild+push+TOML.
