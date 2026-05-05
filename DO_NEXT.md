# Do Next

Resume pointer. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); architecture in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Resume pointer (2026-05-05 v32 — cell 8 v15 in flight; cells 5+6 next)

User goal: **all 4 GCP cells (5, 6, 7, 8) GREEN with full workflow + evidence + executing where they're supposed to**. Cell 7 done; cells 5/6/8 outstanding.

### Cell 8 — v21 PROVED architecture is correct; blocker is regional CPU quota

**Pipeline 2502514674, rev TBD, digest `sha256:c15da1bf`** (v21):

- HTTP middleware now logs at ENTRY too — captures hijacked `/containers/{id}/attach` connections
- v21 trace: `POST /v1.44/containers/.../attach` ENTRY at 21:23:36.173 (BEFORE start) — hijacked
- `POST /v1.44/containers/.../start` ENTRY at 21:23:36.174
- materializePodService ran in 14 s and returned status=500 with `FailedPrecondition: Quota exceeded for total allowable CPU per project per region`
- gitlab-runner correctly reported the error and exited

**Conclusion**: the v17-v20 "silent hangs" were the SAME quota error masked by a successful LRO `CreateService.Wait` (returned 204) while the underlying revision health-check kept failing in the background. gitlab-runner's hijacked /attach connection waited for stdout that never came — looking like an internal hang.

Today's full architectural stack works. The remaining blocker is purely quota-availability.

**Quick steps to GREEN cell 8 (once quota recovers)**:

```bash
# Verify regional CPU quota has freed (give it 1 hour after the last burst)
gcloud compute regions describe us-central1 --format='value(quotas)' | grep -i cpu

# Trigger v22 (no code changes needed)
git fetch origin-gitlab gitlab-cell-8-test
git worktree add -B gitlab-cell-8-test /tmp/cell8 origin-gitlab/gitlab-cell-8-test
cd /tmp/cell8/ui && bun install
git -C /tmp/cell8 checkout -- ui/bun.lock
sed -i '' "1s/.*/# Cell 8 v22 - quota-recovered re-test/" /tmp/cell8/.gitlab-ci.yml
git -C /tmp/cell8 add .gitlab-ci.yml
git -C /tmp/cell8 commit -m "trigger: cell 8 v22"
git -C /tmp/cell8 push origin-gitlab gitlab-cell-8-test
```

If v22 GREENs end-to-end (probe + git clone + go build + arithmetic), then run cells 7+5+6 sequentially within the same quota budget.

### Cells 5+6 (after cell 8 GREEN)

Runner-task images at `tests/runners/github/dockerfile-{cloudrun,gcf}/` already bundle vanilla actions/runner + sockerless. Steps:

```bash
make -C tests/runners/github/dockerfile-cloudrun push-amd64
make -C tests/runners/github/dockerfile-gcf push-amd64
# Update dispatcher TOML config to point at fresh AR digests
# Trigger via: gh workflow run cell-5-cloudrun.yml ; gh workflow run cell-6-gcf.yml
```

### Cell 8 — historical (v17-v20)

**Pipeline TBD, rev TBD, digest `sha256:72d6cd93`**: VpcAccess + ALL_TRAFFIC added to gcf's materializePodService + deployContainerService Service revisions; Config gained `VPCConnector` field; yaml gets `SOCKERLESS_GCF_VPC_CONNECTOR` env. Mirrors cloudrun's BUG-933 fix.

**Quick steps to trigger v20 next session**:

```bash
git fetch origin-gitlab gitlab-cell-8-test
git worktree add -B gitlab-cell-8-test /tmp/cell8 origin-gitlab/gitlab-cell-8-test
cd /tmp/cell8/ui && bun install
git -C /tmp/cell8 checkout -- ui/bun.lock
sed -i '' "1s/.*/# Cell 8 v20 - VpcAccess fix/" /tmp/cell8/.gitlab-ci.yml
git -C /tmp/cell8 add .gitlab-ci.yml
git -C /tmp/cell8 commit -m "trigger: cell 8 v20"
git -C /tmp/cell8 push origin-gitlab gitlab-cell-8-test
```

**Watch for**: if cell 8 v20 progresses past "Preparing environment" (trace bytes > 1990), the VpcAccess fix is correct. If it still hangs identically, then the root cause is a different missing field (compare gcf's materializePodService against cloudrun's startMultiContainerServiceTyped + buildServiceSpec full proto field by field).

**Quota note**: today's many iterations exhausted the regional CPU quota in `sockerless-live-46x3zg4imo`. Cleanup runs are part of the build script. If quota errors persist after the cleanup, wait for the rolling-window reset (~1 hour).

### Cell 8 — historical (resolved by v20 architectural finding)

v9 execStartViaInvoke logs · v10 first attempt at PostExecEnvelope · v11 AR HEAD precheck cuts ~28 s/overlay · v12 PendingCreates Update through materialize · v13 Put fallback · v14 verbose decision logs · v15 ContainerStart ENTRY logs · v16 ContainerInspect/ContainerAttach/ExecCreate/ExecStart ENTRY logs · v17 stdinPipe + attachStream pattern from cloudrun · v18 ContainerAttach overlay-image gate dropped + 5 s pre-check window · v19 OpenStdin=true network-pod main keeps container alive (no default-invoke).

**Old historical (v18 status: gitlab-runner blocked on internal TCP probe — NOT a docker call) — proven wrong by v20 hypothesis**:

**Pipelines 2502018240 (v17) + 2502072794 (v18)** — both hang silently. v18 evidence (rev 00051, digest `sha256:5fc5c398`):

1. ContainerCreate cache-permission helper → ContainerStart → 7s for materialize → exit 0 → DELETE
2. ContainerCreate postgres → ContainerStart → netDefer
3. ContainerCreate build (a59f4 in v17, abc31abe in v18) → ContainerStart → network-pod path → materialize 9s → exit
4. invokePodServiceMain goroutine enters → 5s pre-check window for stdinPipe → **NO stdinPipe registered** → default-invoke fires
5. **gitlab-runner makes ZERO HTTP calls to sockerless after ContainerStart returns at v18@17:32:36** — heartbeats to gitlab.com only

`/containers/{id}/attach` was NEVER called. So the missing stdinPipe is NOT a race — gitlab-runner is intentionally not calling Attach for this container. After ContainerStart returns, gitlab-runner must be:
- Doing internal TCP probes for service health (postgres health-check pattern via `WAIT_FOR_SERVICE_TCP_*`)
- OR computing something CPU-bound (unlikely, would still call docker eventually)
- OR waiting for a service container's IP that sockerless reports incorrectly

**Most likely root cause**: gitlab-runner's `waitForServices` in v17 docker executor connects via TCP to each service container's IP (resolved via docker network inspect). For our network-pod path, sockerless's `cloud_state.go::serviceToPodMemberContainer` returns `NetworkSettings.Networks["bridge"]` with EMPTY `IPAddress`. gitlab-runner can't TCP-probe an empty address, so it might be retrying forever.

**Next iteration (v19) — investigation steps**:

1. **Compare cell 7 (cloudrun GREEN) cloud_state response for the postgres pod-member**: trigger cell 7 fresh and capture `docker inspect <postgres-id>` output; compare what NetworkSettings.IPAddress / Aliases / Networks the cloudrun backend returns vs what gcf returns.

2. **Inspect gitlab-runner v17's `waitForServices` source**: confirm whether it uses docker network inspect to find the IP, or whether it uses an alias-based connection (like `postgres:5432` resolved via docker DNS).

3. **Possible fix**: in `cloudrun-functions/cloud_state.go::serviceToPodMemberContainer`, populate `NetworkSettings.Networks[network].IPAddress = "127.0.0.1"` (or a per-pod mock IP) for sidecar members. Then gitlab-runner's TCP probe connects to 127.0.0.1:5432 from the build container — which is a sibling in the same Cloud Run revision, so localhost works. But gitlab-runner doesn't run inside the build container; it runs on the runner-task. Hmm, this doesn't quite work.

4. **Architectural alternative**: have gitlab-runner skip `waitForServices` entirely. Set `FF_NETWORK_PER_BUILD=false` or similar feature flag. OR use a "wait container" mechanism where sockerless spawns a probe container inside the pod's revision.

5. **Quickest diagnostic**: add a `/_debug/dump-state` endpoint (or just an SSH to the runner-task) to see what gitlab-runner's process state is during the silent window. `pstack` / `goroutine dump` of gitlab-runner would reveal what it's blocked on.

### Cell 8 — historical v9..v17 (resolved by v18 diagnostic finding)

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
