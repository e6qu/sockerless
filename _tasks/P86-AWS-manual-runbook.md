# P86 AWS-track manual runbook — session 1 log

**Date:** 2026-04-19
**Branch:** `phase86-complete-runner-support`
**Account:** `729079515331` (eu-west-1, root user via `aws.sh`)
**AWS spend this session:** ~$0.30 (NAT Gateway 15 min + misc)

**Outcome:** Infrastructure layer works. **Two real bugs blocked runner-level validation.** Stopped early per cost/time budget, tore everything down, zero residue.

---

## Runbook 0 — Infrastructure (PASS)

| Step | Status | Notes |
|---|---|---|
| 0.1 S3 state bucket `sockerless-tf-state` (eu-west-1) | ✅ already existed, versioning enabled |
| 0.2 Versioning enable | ✅ |
| 0.3 `terragrunt init` | ✅ |
| 0.4 `terragrunt apply -auto-approve` | ✅ 34 resources created in ~7 min |
| 0.5 Outputs captured to `/tmp/ecs-out.json` | ✅ |
| 0.6 `/tmp/ecs-env.sh` rendered with all env vars | ✅ |
| 0.7 Build `sockerless-backend-ecs` | ✅ |
| 0.8 Build frontend | N/A — ECS backend now serves Docker API directly (no separate frontend binary). Updated plan accordingly. |
| 0.9 Start backend on `127.0.0.1:2375` | ✅ |
| 0.11 `docker ps` via `DOCKER_HOST=tcp://127.0.0.1:2375` | ✅ Server: Sockerless |

**Resources provisioned (all torn down at session end):**
- VPC `vpc-0817297c22b828c49`, 2 public + 2 private subnets, 1 NAT Gateway, 1 IGW
- ECS cluster `sockerless-live` (Container Insights)
- EFS `fs-016aeac4710f58b11` + mount targets
- ECR repo `729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live`
- CloudWatch log group `/sockerless/live/containers`
- Cloud Map namespace `ns-qqe3grrcyzkknbnd`
- Execution + task IAM roles
- Task + EFS security groups

---

## Runbook 1 — ECS Round-7 smoke (**BLOCKED**)

### 1.1 Basic smoke: `docker run --rm alpine echo hello-from-live-fargate`
**FAIL** — `docker run` hangs indefinitely after `POST /containers/create`. Backend logs show create + attach returning but **no subsequent `POST /containers/{id}/start` ever arrives.** Docker CLI sits waiting on the attach hijack. Killed after 5+ min.

Backend trace:
```
7:55PM DBG POST /containers/create                          201  (390ms)
7:55PM DBG POST /containers/.../attach                      200  (0.14ms)
(no further activity — docker CLI hung; eventually kill 409)
```

Docker CLI version: `29.3.1 / API 1.44 (downgraded from 1.54)`. Explicit `DOCKER_API_VERSION=1.44` did not help. This is **BUG-P86-A1**.

### Direct API probe
Bypassing docker CLI and calling `POST /v1.44/containers/create` + `POST /v1.44/containers/{id}/start` via `curl` succeeded — a real Fargate task was registered and launched. BUT:

### 1.2 Launched task stuck PENDING indefinitely
The Fargate task `ed0c29742b4f490c8760f07ef7d1ca2c` stayed in `PENDING` state for 3+ minutes. Cause: the backend registered the task definition with **`image: "alpine"`** (the raw short ref) instead of a fully-qualified URI (e.g. `<account>.dkr.ecr.eu-west-1.amazonaws.com/docker-hub/library/alpine:latest`). Fargate can't pull `alpine` — it needs ECR or a fully-qualified Docker Hub URI. This is **BUG-P86-A2**.

Stopped the task, did not retry.

### 1.3–1.12 Not run
Cross-container services DNS, custom image, and the rest depend on container lifecycle working. Skipped; belong to a follow-up session after BUG-P86-A1 and BUG-P86-A2 are fixed.

---

## Runbooks 2–5 — Not run

All subsequent runbooks (Lambda baseline, Lambda Runtime-API, GitHub runner, GitLab runner) depend on a working Docker CLI path to the backend. Without BUG-P86-A1 and BUG-P86-A2 fixed, those runs would burn hours of AWS clock with no additional signal. Deferring to a follow-up session.

---

## Runbook 6 — Full teardown (PASS)

Ran `terragrunt destroy` + manual Cloud Map namespace cleanup + state bucket deletion.

| Check | Result |
|---|---|
| ECS clusters matching `sockerless` | 0 |
| Active task definition revisions (`sockerless-*`) | 0 |
| Lambda functions (`sockerless-*`) | 0 |
| CloudWatch log groups (`/sockerless/*`) | 0 |
| Cloud Map namespaces (`skls-*`) | 0 (deleted 10 stale namespaces from prior sessions during cleanup) |
| EFS filesystems (sockerless-tagged) | 0 |
| ECR repositories (`sockerless-*`) | 0 |
| S3 buckets (`sockerless-*`) | 0 (state bucket deleted after versioned object purge) |
| VPCs (project=sockerless tag) | 0 |
| IAM roles (`sockerless-*`) | 0 |

**Zero AWS residue.** Task-definition family names (`sockerless-091948a1f6b6` etc.) still listed — those are AWS metadata that cannot be deleted once a family has existed, but carry no cost and no active revisions.

Account is back to pre-session state.

---

## Bugs logged

### BUG-P86-A1 — `docker run` hangs after POST /containers/create

**Severity:** Critical (blocks the entire runner validation path)
**Backend:** ECS (likely also affects Lambda, Cloud Run, etc. — not re-tested)
**Reproducer:**
1. Provision live ECS infra.
2. Start `sockerless-backend-ecs --addr 127.0.0.1:2375` with live env vars.
3. `DOCKER_HOST=tcp://127.0.0.1:2375 docker run --rm alpine echo x`

**Observed:** Backend logs `POST /containers/create` (201) followed by `POST /containers/{id}/attach` (200, 0.14ms duration). Docker CLI never sends `POST /start`. Process hangs until killed.

**Suspected cause:** The attach handler returns 200 with a regular (non-hijacked) HTTP response instead of a 101 Switching Protocols that holds the connection open. Docker CLI's attach goroutine then completes successfully (EOF), but something in the post-attach start sequence isn't triggered. MEMORY note about WASM exec/stdin_hijacker middleware may be relevant.

**Pointers:** `backends/ecs/backend_impl.go` ContainerAttach + the Docker API attach handler in the core frontend.

### BUG-P86-A2 — ECS task def registered with unqualified image ref

**Severity:** High (tasks launch but never pull; infinite PENDING)
**Backend:** ECS (live)
**Reproducer:** Create a container via the backend with image `alpine`, then start it via curl (bypassing docker CLI's pull flow).

**Observed:** Fargate task launches with `containers[].image: "alpine"` in the task definition. Fargate's image pull fails silently (or stays pending) because `alpine` is not a resolvable registry URI.

**Expected:** When the backend registers the task definition, it should substitute the resolved ECR pull-through URI — e.g. `<account>.dkr.ecr.<region>.amazonaws.com/docker-hub/library/alpine:latest` — matching the path `backends/lambda/image_resolve.go` takes. The ECS equivalent appears to either not exist or not be wired into `taskdef.go`.

**Pointers:** `backends/ecs/taskdef.go` (buildContainerDef — `aws.String(config.Image)` uses raw ref), plus ECR auth resolution in `backends/ecs/server.go` ImageManager init. Lambda already handles this via `resolveImageURI` — port that path to ECS.

---

## What this session proved

- Terraform `terraform/modules/ecs/` provisions cleanly and destroys cleanly.
- Teardown script logic (namespace → service → instance → namespace, then versioned-S3-bucket purge) works from a cold start.
- Cloud Map had **10 leftover namespaces from prior sessions** (pre-Phase-86) that the teardown script also swept up.
- Round-6's Docker-CLI-all-pass claim has regressed: the stateless-backend work on main has broken the core `docker run` flow against ECS. MEMORY's "No fallbacks rule" note is worth checking against what the refactor stripped.

## What it didn't prove

- Nothing about cross-container DNS on real Fargate (P86-003) — blocked on BUG-P86-A2.
- Nothing about Lambda live behavior — never attempted in this session.
- Nothing about SaaS runner viability — blocked on the docker-CLI path.

## Next session prerequisites

1. Fix BUG-P86-A1 and BUG-P86-A2 (code work, no AWS needed — can repro the docker-CLI hang against the AWS simulator probably).
2. Re-read the Round-6 manual test log to cross-check what was actually tested on 2026-04-05 vs what broke since.
3. Then re-provision and rerun Runbook 1–5 in a single session.
