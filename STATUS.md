# Sockerless — Status

**Date: 2026-05-06 — Cell 8 GREEN. 14 architectural fixes shipped over the day; BUG-956 + BUG-957 closed v28.**

## Cell scoreboard (4 GCP cells = the user's "consider it done" gate)

| Cell | Path | State | Notes |
|------|------|-------|-------|
| **5** GH × cloudrun | sockerless-cloudrun | ❌ NOT STARTED | Runner-task image at `tests/runners/github/dockerfile-cloudrun/` already bundles vanilla actions/runner + sockerless. Next: `make push-amd64`, update dispatcher TOML if digest pinned, trigger workflow. |
| **6** GH × gcf | sockerless-gcf | ❌ NOT STARTED | Same shape as cell 5; inherits cell 8's gcf stack. Re-build runner image with today's bootstrap (BUG-957 persist) before triggering. |
| **7** GL × cloudrun | sockerless-cloudrun | 🟡 GREEN at digest `f786c300` (this morning) | Today's v52 retest hit transient regional CPU quota (8s-fail) — not a code regression. Re-trigger to re-confirm after today's gcp-common changes. |
| **8** GL × gcf | sockerless-gcf | ✅ **GREEN v28** at gcf digest `sha256:d792e563` | Job 14234857458 succeeded in 147s. `all arithmetic checks pass` + persist save 11MB→GCS→restore 11MB across get_sources's pod-Service A → step_script's pod-Service B. |

## Today's architectural stack (14 fixes, all verified working)

All in `backends/cloudrun-functions/` + `agent/cmd/sockerless-gcf-bootstrap/` unless noted.

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
| 13 | **BUG-956**: `pendingMembersOfNetwork` filters already-materialized OpenStdin=true mains | gitlab-runner v17 spawns NEW build container per stage with different image — new stage's container becomes its OWN pod-Service main + postgres sidecar; old main stays in its own pod-Service for cleanup_file_variables docker exec to resolve |
| 14 | **BUG-957**: gcf bootstrap persist module + content-hash overlay invalidation | Ports BUG-947 tar-pack persist (`agent/cmd/sockerless-gcf-bootstrap/persist.go`) + adds `BootstrapBinaryHash` to `OverlayImageSpec` so updating the bootstrap binary invalidates AR overlay caches; `/builds` carries naturally across pod-Services because gitlab-runner stages run sequentially |

## Live infra in `sockerless-live-46x3zg4imo` (us-central1)

| Service | Rev | Digest | Notes |
|---|---|---|---|
| `gitlab-runner-cloudrun` | latest | `sha256:db43b1ec` | Cell 7 baseline (today's HTTP middleware Info-level) |
| `gitlab-runner-gcf` | `00060-72h` | `sha256:d792e563` | v28 GREEN — final state |
| `github-runner-dispatcher-gcp` | `00021-fb2` | unchanged | Generic dispatcher (per directive); cells 5+6 ready |
| VPC connector | `sockerless-connector` | e2-micro × 4 min instances | Cloud NAT static IP `34.31.88.230` |

**Cleanup discipline**: every build script ends with `gcloud run services delete sockerless-svc-* / skls-*`. Old per-Service revisions pruned to free regional CPU quota.

## Project state

- **Branch**: `phase-118-faas-pods` (commit pending — BUG-956 + BUG-957 architectural fixes + image)
- **PR**: #123 (open). Title + description need update once cells 5/6/7 join cell 8 GREEN.
- **Live project lifetime**: keep `sockerless-live-46x3zg4imo` alive until cells 5/6/7 all GREEN.

## Next sequence (per DO_NEXT.md)

1. Re-trigger cell 7 (v53) to re-confirm cloudrun GREEN after today's gcp-common changes.
2. Rebuild GH runner-task images for cells 5+6 with today's bootstrap (so persist propagates to GH cells too) — `make -C tests/runners/github/dockerfile-{cloudrun,gcf} push-amd64`.
3. Trigger cells 5+6 via dispatcher.
4. State save + close PR #123 description once all 4 GREEN.
