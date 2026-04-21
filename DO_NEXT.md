# Do Next

Snapshot pointer for the next session. Updated after every task.

## Branch state

`post-phase86-continuation` — PR #113. CI validating after BUG-729 + BUG-731 fixes.

## Active: Phase 90 — no-fakes / no-fallbacks audit

Remaining open bugs from the sweep:

1. **BUG-735** — ECS `buildContainerDef` rejects `HostConfig.Binds` with a clear error when `SOCKERLESS_ECS_AGENT_EFS_ID` is unset. No scratch-volume fallback. (Next up.)
2. **BUG-736** — Cloud Run (`jobspec.go` / `servicespec.go`) + ACA (`jobspec.go` / `appspec.go`) reject `HostConfig.Binds` and named-volume mounts until real mount support ships (Phases 91-94).
3. **BUG-737** — Remove `SOCKERLESS_SKIP_IMAGE_CONFIG` env var entirely; audit simulators (ECR / AR / ACR) to confirm they always serve `/v2/` manifest + config so metadata fetches never need a fallback.
4. **Broader sweep** — final pass for swallowed errors (`_ = err`, `if err != nil { return nil }`), silent `continue`-on-error in aggregation loops, `NotImplemented` returns that could be real implementations.

Each goes into BUGS.md with root-cause trace before the fix; docs refreshed after.

## Queued follow-up phases

Designs in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md) under "Volume provisioning per backend". Phases in [PLAN.md](PLAN.md):

- **Phase 91** — ECS EFS access-point provisioning (simulator: `simulators/aws/efs.go`).
- **Phase 92** — Cloud Run GCS bucket-mount provisioning (extend GCS sim slice).
- **Phase 93** — ACA Azure Files share provisioning (simulator: `fileServices/shares` + managed-env `storages` sub-resource).
- **Phase 94** — GCF + AZF inherit latest-generation helpers from Phase 92/93.

## Live-cloud validation runbooks (need creds)

- **Phase 87 live-GCP** — Cloud Run Services validation against real GCP.
- **Phase 88 live-Azure** — ACA Apps validation against real Azure.
- **Phase 86 Lambda live track** — scripted already, deferred for session-budget reasons.

## Other queued

- **Phase 68 — Multi-Tenant Backend Pools** (P68-002 → 010).
- **Phase 78 — UI Polish**.

## Operational state

- AWS: zero residue (state buckets + DDB lock table retained).
- Local sockerless backend: stopped.
- No credentials in environment.
