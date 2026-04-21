# Sockerless — Status

**91 phases closed (759 tasks). 754 bugs tracked — 743 fixed, 11 open (every open bug is scoped to a phase; no 'platform limit' exits). BUG-744/745/746 → Phase 95/96/97. BUG-748/749/754 rescoped to Phase 94b/99/100. BUG-750/751/752/753 → Phase 98/98b. 1 false positive. Branch `continue-plan-post-113`.**

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
| 94 | GCF + AZF inherit Phase 92/93 helpers | Queued. |
| 95 | FaaS invocation-lifecycle tracker (Lambda + GCF + AZF) — re-enables 7 deleted tests from BUG-744 | Queued. Design: per-backend cloud-native completion signal (Lambda Invoke response + CloudWatch END RequestId; GCF/AZF HTTP response status). |
| 96 | Reverse-agent exec for Cloud Run Jobs + ACA Jobs (ports Lambda bootstrap pattern) | Queued — from BUG-745. |
| 97 | Docker labels as GCP annotations / Azure tags on FaaS + Cloud Run / ACA | Queued — from BUG-746. |
| 94b | Lambda EFS volume provisioning via `Function.FileSystemConfigs[]` (reuse ECS's EFS manager once lifted to `aws-common`) | Queued — revised from BUG-748's earlier 'platform limit' framing. |
| 98 | Agent-driven filesystem + introspection ops (`docker cp` / `export` / `stat` / `top` / `diff`) via reverse-agent or SSM ExecuteCommand | Queued — from BUG-751/752/753; depends on Phase 96. |
| 98b | Agent-driven `docker commit` (opt-in via `SOCKERLESS_ENABLE_COMMIT`) | Queued — from BUG-750; depends on Phase 98. |
| 99 | Agent-driven `docker pause` / `unpause` via SIGSTOP/SIGCONT over reverse-agent (Fargate uses SSM `signal`) | Queued — revised from BUG-749's earlier 'platform limit' framing; depends on Phase 96. |
| 100 | Docker backend pod synthesis via shared `sockerless-pod` label convention | Queued — revised from BUG-754's earlier 'won't fix' framing. |

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
