# Do Next

Resume pointer. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); architecture in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Resume pointer (2026-05-05 v32 — cell 8 v15 in flight; cells 5+6 next)

User goal: **all 4 GCP cells (5, 6, 7, 8) GREEN with full workflow + evidence + executing where they're supposed to**. Cell 7 done; cells 5/6/8 outstanding.

### Cell 8 — v15 results (HUGE progress, new failure mode)

**Pipeline 2501668159** (gitlab-cell-8-test branch). Rev `00047-45f`, digest `sha256:ee7e5029...`.

**All architectural fixes verified working via diagnostic logs**:
- `ContainerStart: ENTRY` ✅ for cache-permission helper, postgres, and build
- `ContainerStart: resolved` ✅ (running=false, status=created, openStdin=true for build)
- `ContainerStart: network-pod decision` ✅ — for build: `netDefer=false netMembers=2`
- `marked running, entering materialize` ✅ updated=true
- `materializePodService: entry` with both members [build, postgres]
- `materializePodService: exit` at **13 seconds** (well under 120s budget!)
- Both Services deployed: `sockerless-svc-b8229d285672` (cache-permission) + `sockerless-svc-ebbcd6541e74` (build/postgres pod, bootstrap listening on :8080, postgres up on :5432)
- Bootstrap exec'd build's CMD `[/usr/bin/dumb-init /entrypoint gitlab-runner-build]` → exit=0 (expected behavior — bootstrap stays up as HTTP server holding the port)

**New failure mode**: gitlab-runner reaches `Preparing environment` (start of `prepare_script` section) then silently hangs for 30+ minutes. **NO `ExecCreate` / `ExecStart` / `ContainerInspect` calls reach sockerless backend** during this window. trace stuck at 1990 bytes.

### Next iteration (v16) — instrument frontend HTTP layer

Add ENTRY-level logging to:
- `cloudrun-functions/backend_delegates.go::ExecCreate` — log every call
- `cloudrun-functions/backend_delegates.go::ExecStart` — log every call
- `cloudrun-functions/backend_delegates.go::ContainerInspect` — log every call
- Possibly `core/handle_containers_query.go::handleContainerInspect` — log every HTTP request
- Possibly the docker frontend's request middleware to log every URL hit

The goal: prove whether gitlab-runner is making ANY docker calls that reach sockerless during the silent window. If sockerless sees calls but doesn't progress them, the bug is internal. If sockerless sees zero calls, gitlab-runner is hung internally OR talking to a different DOCKER_HOST.

### Hypothesis to verify

`prepareEnvironment` in gitlab-runner v17 docker executor calls `cli.ContainerExecCreate(build, ["sh","-c", prepare_script])` immediately after the "Preparing environment" log. If this hangs, gitlab-runner just waits indefinitely. The hang could be:

1. **HTTP frontend not routing /containers/{id}/exec** — handler missing or returning 404 silently. Check `/Users/zardoz/projects/sockerless/backends/core/handle_docker_api.go` registration: `POST /containers/{id}/exec` is registered (line 86, verified earlier). But maybe gcf's `s.self.ExecCreate` is panicking, eating logs.
2. **gitlab-runner using a different DOCKER_HOST** — but cache-permission and pod containers DID reach sockerless, so this is unlikely.
3. **gitlab-runner in a sleep waiting for a TCP probe** — postgres health check might be hanging because the wait container can't be created.

### If cell 8 v15 still fails

Cancel pipeline 2501668159 (it's wedged). Iterate to v16 with the frontend logging above. Then re-trigger.

### If cell 8 GREENs (e.g. trace eventually advances)

1. Mark BUG-953 closed in BUGS.md.
2. Update STATUS.md / WHAT_WE_DID.md cell 8 row to ✅.
3. Move to cells 5+6.

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
