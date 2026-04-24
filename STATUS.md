# Sockerless — Status

**96 phases closed (764 tasks). 756 bugs tracked — 748 fixed, 8 open. Phase 100 closed 2026-04-23 (BUG-754). Phase 96 backend-side machinery landed; container-side overlay bootstraps follow. Remaining open bugs: BUG-745 → Phase 96 bootstrap (container-side); BUG-749 → Phase 99; BUG-750/751/752/753 → Phase 98/98b; BUG-756 → sim-Lambda stdout forwarding. 1 false positive. Branch `phase96-onward`.**

See [PLAN.md](PLAN.md) for the roadmap, [BUGS.md](BUGS.md) for the bug log (+ open-bug descriptions), [WHAT_WE_DID.md](WHAT_WE_DID.md) for the narrative, [specs/](specs/) for architecture specs.

## Phase roll-up

| Phase | Scope | Status |
|---|---|---|
| 86 | Simulator parity (AWS + GCP + Azure) + Lambda agent-as-handler | Closed 2026-04-20 (PR #112). Phase C live-AWS validated. |
| 87 | Cloud Run Jobs → Services (internal ingress + VPC connector) | Closed in code 2026-04-21 (PR #113). Live-GCP pending. |
| 88 | ACA Jobs → Apps (internal ingress) | Closed in code 2026-04-21 (PR #113). Live-Azure pending. |
| 89 | Stateless-backend audit — cloud resource mapping, `resolve*State`, cloud-derived `ListImages` / `ListPods`, `resolveNetworkState` | Closed 2026-04-21 (PR #113). |
| 90 | No-fakes/no-fallbacks audit — workarounds, placeholders, silent substitutions all elevated to bugs | Closed. BUG-729/730/731/732/733/734/737 fixed; BUG-735/736 absorbed by Phase 91. |
| 91 | ECS real named-volume + bind-mount provisioning via EFS access points (sim: real host-dir-backed `EFSAccessPointHostDir`; backend: `volumes.go`) | Closed 2026-04-21 on `continue-plan-post-113`. |
| 92 | Cloud Run GCS bucket-mount provisioning (sim: `GCSBucketHostDir` + CR Jobs exec Volume translation; backend: `volumes.go` GCS bucket-per-volume manager; jobspec + servicespec emit `Volume{Gcs{Bucket}}`) | Closed 2026-04-21. |
| 93 | ACA Azure Files share provisioning (sim: `FileShareHostDir` + `managedEnvironmentStorages` CRUD + ACA Jobs exec Volume translation; backend: `volumes.go` share-per-volume + env-storage link; jobspec + appspec emit `Volume{StorageType=AzureFile}`) | Closed 2026-04-21. |
| 94 prereq | Volume managers lifted to `aws-common` / `gcp-common` / `azure-common` so FaaS backends can embed them | Closed 2026-04-21. |
| 94 | GCF + AZF real per-cloud volume provisioning — GCF via Functions v2 + underlying Cloud Run Service escape hatch; AZF via sites/config/azurestorageaccounts | Closed 2026-04-21. |
| 95 | FaaS invocation-lifecycle tracker (Lambda + GCF + AZF) — re-enables 7 deleted tests from BUG-744 | Closed 2026-04-21 — core.InvocationResult + per-backend wiring + 7 tests re-enabled. |
| 96 | Reverse-agent exec for Cloud Run Jobs + ACA Jobs (ports Lambda bootstrap pattern) | Backend-side machinery closed 2026-04-23 — shared `core.ReverseAgentRegistry/HandleReverseAgentWS/ReverseAgent{Exec,Stream}Driver`; CR + ACA wire `/v1/{cloudrun,aca}/reverse` + inject `SOCKERLESS_CALLBACK_URL`. Container-side overlay bootstraps are follow-up work (existing `sockerless-agent --callback --keep-alive <cmd>` is viable). |
| 97 | Docker labels charset-safe on GCP — values failing `[a-z0-9_-]{0,63}` route to annotations / SOCKERLESS_LABELS env var | Closed 2026-04-21. |
| 94b | Lambda EFS volume provisioning via `Function.FileSystemConfigs[]` (reuses `awscommon.EFSManager`) | Closed 2026-04-21. |
| 98 | Agent-driven filesystem + introspection ops (`docker cp` / `export` / `stat` / `top` / `diff`) via reverse-agent or SSM ExecuteCommand | Partial 2026-04-23: `ContainerTop` landed via shared `core.RunContainerTopViaAgent` + new `agent.CollectExec`. Lambda/CR/ACA/GCF/AZF route `docker top` through the reverse-agent; no-session case surfaces a precise NotImplemented. `docker cp` / `stat` / `diff` / `export` still pending — same pattern. |
| 98b | Agent-driven `docker commit` (opt-in via `SOCKERLESS_ENABLE_COMMIT`) | Queued — from BUG-750; depends on Phase 98. |
| 99 | Agent-driven `docker pause` / `unpause` via SIGSTOP/SIGCONT over reverse-agent (Fargate uses SSM `signal`) | Queued — revised from BUG-749's earlier 'platform limit' framing; depends on Phase 96. |
| 100 | Docker backend pod synthesis via shared `sockerless-pod` label convention | Closed 2026-04-23. |

Detail per phase in [WHAT_WE_DID.md](WHAT_WE_DID.md). Open work items queued in [DO_NEXT.md](DO_NEXT.md).

## Test counts

| Category | Count |
|---|---|
| Core unit | 310 |
| Cloud SDK/CLI | AWS 68, GCP 64, Azure 57 |
| Sim-backend integration | 76 (+1 for Phase 91 `TestECSVolumeOperations` full create/inspect/list/remove) |
| GitHub E2E | 186 |
| GitLab E2E | 132 |
| Terraform | 75 |
| UI/Admin/bleephub | 512 |
| Lint (18 modules) | 0 issues |

## ECS live testing

6 rounds against real AWS ECS Fargate (`eu-west-1`). Round 6: Docker CLI all pass, Podman pull+pods pass (container ops blocked by response format), Advanced 3/4. See [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md). Phase 87/88 live-cloud validation runbooks still to be written (GCP/Azure equivalents of `scripts/phase86/*.sh`).
