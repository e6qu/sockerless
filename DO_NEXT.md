# Do Next

Resume pointer. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); architecture in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Resume pointer (2026-05-03 v12 — end of session)

**Goal**: cells 5/6/7/8 GREEN with REAL workload (compile + use eval-arithmetic + probe environment) before merging PR #123.

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
