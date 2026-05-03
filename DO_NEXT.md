# Do Next

Resume pointer. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); architecture in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Resume pointer (2026-05-03 v16 — end of session)

**Goal**: cells 5/6/7/8 GREEN with REAL workload (compile + use eval-arithmetic + probe environment) before merging PR #123.

### What's done since v15
- BUG-937 (3-stage AR-auth-rewrite chain) shipped on `phase-118-faas-pods` (commits `7410f11`, `bb3412e`, `aa03bae`). gitlab-runner-cloudrun rev `00046-qc9` deployed (digest `37bbf207...`).
- Cell 7 v33 confirmed: pull → overlay-build → per-stage Cloud Run Service deploy → bootstrap subprocess all working live.
- New blocker: **BUG-925 postgres-on-Cloud-Run-Service** — gitlab-runner-helper health-check fails because Cloud Run can't expose TCP :5432 + can't inject `WAIT_FOR_SERVICE_TCP_*` env vars correctly.

### Decisions needed before next session
1. Pick a path for BUG-925 (Cloud SQL / Cloud Run sidecar / trim postgres from cell-7/8). User input required — this is the architectural fork.
2. Once cell 7 GREEN, port the same overlay+bootstrap+alias trio to gcf for cell 8.
3. For cells 5+6: refresh GitHub PAT (rate-limited).

**Where we landed (v15)**: Phase 122g + Phase 122h shipped (15+ commits on `phase-118-faas-pods`). Live cell 7 progression confirms the architecture works:
- Cloud Run Service deploys via overlay+bootstrap ✓
- `sockerless-cloudrun-bootstrap: subprocess argv=[/usr/bin/dumb-init /entrypoint gitlab-runner-helper cache-init /gitlab-runner-cache-init] exit=0` (real cmd ran) ✓
- Postgres service container UP and listening on :5432 ✓
- BUILD container (golang:1.22-alpine) deployed ✓
- gitlab-runner trace bytesize=28550 (vs. previous 1144) ✓

**Architectural insights from source (verified 2026-05-03)**:
- `gitlab-runner` v17.5 `executors/docker/internal/exec/exec.go::defaultDocker.Exec` uses HIJACKED `ContainerAttach(Stream+Stdin+Stdout+Stderr)` + `ContainerStart` + raw stdin-pipe per stage. NOT `/exec/...` API.
- ECS + Lambda backends already had `stdin_pipe.go` (~80 lines) — that's why cells 3+4 are GREEN. cloudrun + gcf were missing this.
- Phase 122h ports the proven AWS pattern: stdin_pipe.go (lifted from ecs) + attach_stream.go (hijacked-shaped RWC) + invoke goroutine consumes captured stdin and POSTs `execEnvelope{argv:[/bin/sh], stdin:<captured>}`.

**Current blocker** (new — not yet diagnosed): rev `00040-qj6` of `gitlab-runner-cloudrun` (Phase 122h image, digest `4fc5abd0951729dcc683dd3491bfc10185cdc00749a19d011d86f99f34524f27`) failed Cloud Run startup health probe — bootstrap silently dies before binding PORT 8080. Rolled back to rev `00038-f42` which serves the older Phase 122g code without stdinPipe. Phase 122h code is committed (`commit 9f9f872`) but not running live.

**Next session resume**:
1. Debug rev 00040 startup failure: docker-run the image locally with full Cloud Run env vars (PORT=8080, K_SERVICE, K_REVISION, K_CONFIGURATION) and trace bootstrap.sh step-by-step. Likely candidates: my new `stdin_pipe.go` / `attach_stream.go` panic at init, OR a Go module dep changed binary startup path. Compare to working rev 00038 binary by `strings | grep -c stdinPipe`.
2. Once rev 00040 is healthy, retrigger cell 7 → expect REAL stdin captured + real bash script run on Cloud Run Service.
3. After cell 7 GREEN, port the same stdinPipe pattern to gcf for cell 8.
4. For cells 5+6: confirm GitHub PAT refresh (user did `gh auth refresh` but token is rate-limited — rate limit resets hourly). Then mint scoped PAT, upload to Secret Manager `github-pat`, dispatcher resumes polling.

**Architectural state — clear path forward (Phase 122g)**: today's session diagnosed the actual blocker. Cell 7 falsely reported SUCCESS while running zero workload (BUG-927). Backend logs proved gitlab-runner's docker-executor flow: `attach 200 (216s) → exec 409 'Container not running' → wait 200 → stop 304 × N`. Cloud Run Job (one-shot) cannot host gitlab-runner's "long-lived build container + per-stage exec" model. Stock images (`golang:1.22-alpine`, `postgres:16-alpine`) have no in-container exec endpoint.

**Path forward (per `specs/CLOUD_RESOURCE_MAPPING.md` § Synthesis — Phase 122g)**:

1. **Lift `backends/lambda/image_inject.go` → `backends/gcp-common/image_inject.go`** — shared overlay renderer + Cloud Build trigger for cloudrun + gcf. Renames per cloud (`sockerless-cloudrun-bootstrap`, etc.) but the renderer logic is generic.
2. **New binary `agent/cmd/sockerless-cloudrun-bootstrap`** — mirror `sockerless-lambda-bootstrap`: handles HTTP request body as `execEnvelope{argv,tty,workdir,env,stdin}`, runs cmd, returns `{exitCode,stdout,stderr}` (base64).
3. **cloudrun ContainerCreate** — drop `isRunnerPattern` gating; ALL containers route to Cloud Run Service via overlay. ContainerStart triggers overlay-build (cached by content-hash) + CreateService (`min_instance_count=1` + always-on CPU).
4. **cloudrun ContainerExec — Path B (Lambda Lesson 8)**: HTTP POST to Service URL with `execEnvelope`. NO reverse-agent WS for the common case; reserve WS only for interactive TTY+stdin.
5. **gcf ContainerExec — Path B**: identical pattern POST to `Function.ServiceConfig.Uri`. Bootstrap inside (already present per Phase 118 BUG-884) extends to recognize envelope shape.
6. **Pre-deploy Service per shape (Lesson 1)**: terraform-managed shape catalog seeded for known runner images (`golang:1.22-alpine`, `postgres:16-alpine`, `gitlab-runner-helper:x86_64-v17.5.0`). ContainerStart claims free pool entry by content-hash before paying overlay-build cost.
7. **Pool semantics (Lesson 2)**: ContainerStop → release back to pool (clear `sockerless_allocation` label). ContainerRemove → delete above pool cap.

This dissolves BUG-921/922/923/925/927. After Phase 122g, all 4 cells should GREEN with real workload visible in Cloud Logging + streamed via Cloud Run Service HTTP response (`docker logs --follow` parity).

## Tactical files for resume
- `backends/lambda/image_inject.go` — source of the overlay renderer to lift.
- `backends/lambda/exec_invoke.go` — `execStartViaInvoke` reference for Path B.
- `backends/cloudrun-functions/image_inject.go` — gcf already has overlay; align the lift with this.
- `backends/cloudrun/runner_pattern.go` — DELETE after Phase 122g (gating no longer needed).
- `backends/cloudrun/backend_impl.go::ContainerStart` (line 235-) — refactor to always go Service.
- `agent/cmd/sockerless-lambda-bootstrap/` — copy as the template for `sockerless-cloudrun-bootstrap`.
- `terraform/modules/cloudrun/runner.tf` — extend to pre-create N Services per shape.

## Branch state
- `main` synced with `origin/main` at PR #121 merge.
- `phase-118-faas-pods` (PR #123) — 18+ commits this session; all standard CI green; ready for merge once cells GREEN.
- `cell-workflows-on-main` (PR #124, throwaway) — close after cells 5+6 GREEN; do NOT merge.
- `gitlab-cell-7-test` + `gitlab-cell-8-test` on `origin-gitlab` — pipelines for cells 7+8.

## Live infra (in `sockerless-live-46x3zg4imo`, us-central1)
- Dispatcher Cloud Run Service `github-runner-dispatcher-gcp`
- gitlab-runner-cloudrun (rev 00021-rzl post BUG-922 fix), gitlab-runner-gcf
- VPC `sockerless-vpc` + subnet `sockerless-connector-subnet` + connector `sockerless-connector` (us-central1, 10.8.0.0/28)
- AR repos: `sockerless-live`, `docker-hub` (proxy), `gitlab-registry` (proxy), `sockerless-overlay/gcf`
- Secret Manager: `github-pat`, `gitlab-pat`, `gitlab-runner-token-{cloudrun,gcf}`
- GCS: `sockerless-live-46x3zg4imo-build`, `sockerless-live-46x3zg4imo-runner-workspace`

## Resume runbook (next session, condensed)
1. Read `specs/CLOUD_RESOURCE_MAPPING.md` § Lessons 6 + 8 + Synthesis — Phase 122g scope. Read `backends/lambda/image_inject.go` + `exec_invoke.go` for the references.
2. `cp backends/lambda/image_inject.go backends/gcp-common/image_inject.go`; refactor (drop AWS-isms, parameterize bootstrap binary path).
3. `cp -r agent/cmd/sockerless-lambda-bootstrap agent/cmd/sockerless-cloudrun-bootstrap`; replace AWS Lambda Runtime API plumbing with HTTP server bound to `$PORT`; envelope handler stays identical.
4. cloudrun: rewrite `ContainerCreate` to overlay-build (gcp-common), CreateService, store ServiceURL in CloudState. Drop `isRunnerPattern` + delete `runner_pattern.go`.
5. cloudrun: rewrite `ExecStart` to Path B (POST envelope → parse response). Keep WS path only for `opts.Stdin && opts.Tty`.
6. gcf: extend `Server.handleInvoke`/bootstrap to recognize envelope shape (already does cmd+stdin; add stdout/stderr base64 framing in response).
7. Build + deploy + re-fire cells 5/6/7/8.
8. Verify REAL workload markers in Cloud Logging: `apk add`, `git clone`, `eval-arithmetic`.
9. Capture URLs into STATUS.md. Close PR #124 (do NOT merge). PR #123 ready for user merge.

## Memory of today's wedges (do not repeat)
- DO NOT add `isRunnerPattern` heuristic gating — Phase 122g routes everything through Service.
- DO NOT push images to public registries (Docker Hub, GitLab Registry); only pull via AR remote-proxy.
- DO NOT add hardcoded port maps, hardcoded image lists, or fallback shims; per-image data comes from `Config.ExposedPorts`.
- DO NOT trust gitlab-runner exit-0 as proof of work done — verify Cloud Logging contains workload markers (BUG-927 lesson).
- DO NOT auto-remove containers post-execution — gitlab-runner re-uses container IDs across stages (BUG-922 lesson).
