# Sockerless — Status

**Date: 2026-05-06 — Cell 8 4/5 stages GREEN; final blocker BUG-956 (multi-image-per-stage materialize race). 12 architectural fixes shipped today.**

## Cell scoreboard (4 GCP cells = the user's "consider it done" gate)

| Cell | Path | State | Notes |
|------|------|-------|-------|
| **5** GH × cloudrun | sockerless-cloudrun | ❌ NOT STARTED | Runner-task image at `tests/runners/github/dockerfile-cloudrun/` already bundles vanilla actions/runner + sockerless. After cell 8 GREEN: `make push-amd64`, update dispatcher TOML, trigger workflow. |
| **6** GH × gcf | sockerless-gcf | ❌ NOT STARTED | Same shape as cell 5; inherits cell 8's gcf stack. |
| **7** GL × cloudrun | sockerless-cloudrun | 🟡 GREEN this morning at digest `f786c300`; today's v52 retest hit transient regional CPU quota (8s-fail) — not a code regression | Re-trigger after quota cooldown to confirm today's gcp-common changes (HTTP middleware Info-level, AR HEAD precheck) didn't regress. |
| **8** GL × gcf | sockerless-gcf | 🟡 **4/5 stages GREEN** at digest `sha256:79621fbe` | prepare_executor + prepare_script + get_sources + step_script start ALL succeeded in v25 trace. Final blocker: BUG-956 (next iteration v26). |

## Today's architectural stack (12 fixes shipped, all verified working)

All in `backends/cloudrun-functions/` unless noted. Order: shipped → impact.

| # | Fix | What it does |
|---|---|---|
| 1 | AR HEAD precheck (`backends/gcp-common/registry_check.go`) | HEAD `/v2/<repo>/manifests/<tag>` short-circuits Cloud Build's ~28 s overhead on cache hit |
| 2 | Multi-container Cloud Run Service direct deploy (`pod_service.go::materializePodService`) | Replaces the slow Cloud Functions wrapper path (was 150 s); now 9-15 s |
| 3 | PendingCreates speculative-running marker through materialize | Container visible to ContainerInspect during the 30 s CreateService window |
| 4 | `resolvePodServiceFromCloud` GetService follow-up on abbreviated annotations | Sidecar pod members findable via annotation match when ListServices truncates |
| 5 | `stdinPipe` + `attachStream` pattern ported from cloudrun | gitlab-runner attach stdin captured for envelope POST replay |
| 6 | Relaxed ContainerAttach overlay-image gate | At attach time c.Config.Image is the user-supplied original, not the overlay URI |
| 7 | 5 s pre-check window for late-arriving stdinPipe | Closes the ContainerStart-vs-Attach race in invokePodServiceMain |
| 8 | OpenStdin=true runner-pattern: skip default-invoke | Don't POST the user's no-op CMD; keep container alive for `docker exec` |
| 9 | VpcAccess + ALL_TRAFFIC on materialize Service revisions | Cross-Cloud-Run calls (gitlab-runner-gcf → sockerless-svc-*) appear as in-VPC source instead of being rejected as external |
| 10 | HTTP middleware ENTRY-level logging (`backends/core/server.go`) | Captures hijacked /attach connections (would otherwise only log on END after stream close) |
| 11 | **Typed.Attach routes through ContainerAttach delegate** | The silent-hang root cause: gcf was using read-only `NewCloudLogsAttachDriver`; cloudrun uses `WrapLegacyContainerAttach` |
| 12 | Multi-stage `invokeRunningRunnerStage` + unique container names | Per-stage stdinPipe drain + envelope POST against existing Service URL; sanitized names get count suffix on collision |

## Final remaining blocker: BUG-956

**Symptom**: cell 8 v25 reached step_script (4/5 stages GREEN) then hit `no service URL` during cleanup_file_variables.

**Root cause**: gitlab-runner v17 docker executor uses **different images per stage** — `gitlab-runner-helper:x86_64-v17.5.0` for prepare/get_sources/upload_artifacts; `golang:1.22-alpine` (user image) for step_script/after_script. With `FF_NETWORK_PER_BUILD=true` all stages join the same per-build network. Each stage's new container with `OpenStdin=true` triggers our network-pod detection — `materializePodService` runs again with 3 members (new build + postgres + OLD build still in PendingCreates) → fails or leaves the original pod-Service unreachable.

**Recommended fix (v26)**: in `backends/cloudrun-functions/network_pod.go::pendingMembersOfNetwork`, exclude containers already materialized in an existing pod-Service. Minimal change, preserves the discovery-from-network-membership model.

## Live infra in `sockerless-live-46x3zg4imo` (us-central1)

| Service | Rev | Digest | Notes |
|---|---|---|---|
| `gitlab-runner-cloudrun` | latest | `sha256:db43b1ec` | Cell 7 baseline (today's HTTP middleware Info-level) |
| `gitlab-runner-gcf` | `00057-6z8` | `sha256:79621fbe` | v25 — 4/5 stages GREEN; v26 will bump |
| `github-runner-dispatcher-gcp` | `00021-fb2` | unchanged | Generic dispatcher (per directive); cells 5+6 ready |
| VPC connector | `sockerless-connector` | e2-micro × 4 min instances | Cloud NAT static IP `34.31.88.230` |

**Cleanup discipline**: every build script ends with `gcloud run services delete sockerless-svc-* / skls-*`. Old per-Service revisions pruned to free regional CPU quota. After today's session: 3 active services, 3 active revisions (down from 81+).

## Project state

- **Branch**: `phase-118-faas-pods` at `f18d7a1`
- **PR**: #123 (open). Title + description need update to cover today's scope.
- **Live project lifetime**: keep `sockerless-live-46x3zg4imo` alive until cells 5/6/7/8 all GREEN.
