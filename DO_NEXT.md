# Do Next

Resume pointer. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); architecture in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Resume pointer (2026-05-05 v32 — cell 8 v15 in flight; cells 5+6 next)

User goal: **all 4 GCP cells (5, 6, 7, 8) GREEN with full workflow + evidence + executing where they're supposed to**. Cell 7 done; cells 5/6/8 outstanding.

### Cell 8 — v17 status: stdinPipe shipped, gitlab-runner mysteriously silent post-start

**Pipeline 2502018240, rev `00050-p25`, digest `sha256:37e65d40`**:

Successfully ported the cloudrun `stdinPipe + attachStream` pattern to gcf:
- New `backends/cloudrun-functions/{stdin_pipe.go,attach_stream.go}`
- New `Server.{stdinPipes,attachStreams}` sync.Map fields
- `ContainerAttach` delegate registers stdinPipe + returns attachStream when caller wants stdin AND image is sockerless overlay
- `invokePodServiceMain` blocks up to 30s on `pipe.Done()`, POSTs captured stdin via `PostExecEnvelope` with argv=[/bin/sh] + Stdin=base64; falls through to default-invoke if no pipe; publishes stdout+stderr back via `attachStream.publishAttachResponse`
- `LoggingMiddleware` Debug→Info bump in `backends/core/server.go` so every HTTP request hit gets logged

**v17 evidence with full HTTP middleware visibility**:
1. ContainerCreate cache-permission helper (single container)
2. ContainerStart cache-permission → completes 15.6 s, exits 0
3. DELETE cache-permission container
4. POST /images/postgres pull
5. ContainerCreate postgres
6. ContainerStart postgres → netDefer=true, returns 204 quickly
7. POST /images/golang pull
8. ContainerCreate golang build container (a59f4f6e3964)
9. GET /containers/a59f.../json → ContainerInspect (200)
10. ContainerStart build → network-pod decision: netDefer=false, netMembers=2 ✅
11. materialize: 19s (parallel cache-hit overlay builds + Services.CreateService)
12. ContainerStart returns 204 at 17:15:33
13. **invokePodServiceMain enters → finds NO stdinPipe → default-invoke fires** (POSTs user CMD `[/usr/bin/dumb-init /entrypoint gitlab-runner-build]`)
14. **After 17:15:33 — gitlab-runner makes ZERO HTTP calls to sockerless for 15+ min**. Heartbeats to gitlab.com continue.

**Open question for next session**: cell 7 (cloudrun) GREEN has identical stdinPipe code in `backends/cloudrun/start_service.go::invokeServiceDefaultCmd` and the same condition path in `backends/cloudrun/backend_impl.go::ContainerAttach` (with `hasSockerlessOverlayRepo` check). It works there but not in cell 8. Need to either:

(a) Capture cell 7's HTTP request log timeline (re-trigger cell 7 or diff against what's stored) to see if gitlab-runner calls ContainerAttach BEFORE ContainerStart in the cloudrun path.

(b) Inspect what gitlab-runner v17 does internally between ContainerStart return and the next docker call. The trace shows `Preparing environment` (start of `prepare_script` section); after ContainerStart returns, the next stage step should fire ContainerExec or ContainerAttach. Possibly gitlab-runner is waiting on an internal `waitForServicesHealth` TCP probe that we don't proxy.

(c) The `hasSockerlessOverlayRepoGCF` check in our ContainerAttach uses `c.Config.Image` from the BEFORE-materialize state — at attach time the image is still the user-supplied original (golang:1.22-alpine), not the overlay URI. If gitlab-runner attaches BEFORE start, our check returns false and we return NotImplementedError. Fix: drop the overlay check, or use a more permissive condition (e.g. always allow stdin on sockerless-managed containers).

### Cell 8 — historical v9..v16 (resolved by v17 architectural finding)

**v16 evidence** (pipeline 2501822349, rev `00049-lk4`, digest `sha256:d32c33e4`): with all delegate ENTRY logs added, we can see definitively that gitlab-runner makes ZERO docker calls during the silent window. ContainerStart fires correctly (netDefer=false netMembers=2), materialize completes in 13 s, build container's bootstrap exec'd `[/usr/bin/dumb-init /entrypoint gitlab-runner-build]` → exit=0 stdout=0B stderr=0B (gitlab-runner-helper's `build` subcommand is a no-op without CI env vars).

**Root cause**: `cloudrun-functions/pod_service.go::invokePodServiceMain` is invoked immediately after `materializePodService` completes. It POSTs the build container's user CMD to the Service URL, gets exit=0 back, calls `PutInvocationResult` + closes `WaitChs`. From sockerless's perspective the container "exited" — but gitlab-runner expected the container to STAY ALIVE so it could attach stdin and pipe scripts.

**Fix path — port the cloudrun pattern**:

1. `backends/cloudrun/attach_stream.go` — defines `attachStream` + `stdinPipe`. ContainerAttach returns an `attachStream` that reads from the pipe via `publishAttachResponse`.
2. `backends/cloudrun/start_service.go::invokeServiceDefaultCmd` — goroutine that BLOCKS until the per-container `stdinPipe` is populated (via attach), then POSTs the captured script bytes as the request envelope, reads response, publishes back via `attachStream.publishAttachResponse`.
3. `backends/cloudrun/backend_impl.go::ContainerAttach` (around line 1283-1314) — when called for a service-container, registers a `stdinPipe` so invokeServiceDefaultCmd can wait on it.

Mirror this onto `backends/cloudrun-functions/`:
- Add `gcf` equivalent of `attach_stream.go` (or share with cloudrun via gcp-common)
- Modify `pod_service.go::invokePodServiceMain` to wait for stdinPipe
- Modify `cloudrun-functions/backend_impl.go::ContainerAttach` to register stdinPipe like cloudrun does

Also flip `LoggingMiddleware` in `core/server.go` from Debug to Info (already staged in source) to capture every HTTP request hit during the next iteration — confirms whether gitlab-runner is calling Attach or Wait or something else first.

### Cell 8 — v15/v14 historical (resolved by v16 architectural finding)

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
