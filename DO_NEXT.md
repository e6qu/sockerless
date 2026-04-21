# Do Next

Snapshot pointer for the next session. Updated after every task.

## Branch state

`post-phase86-continuation` — PR #113. CI validating after the Phase 90 no-fakes sweep.

## Active: Phase 90 — no-fakes / no-fallbacks audit

Fixing the open bugs from the sweep, in order:

1. **BUG-731** — `VolumeCreate` / `VolumeRemove` / `VolumeInspect` / `VolumeList` on ECS/Lambda/Cloud Run/ACA return `NotImplemented`. Delete dead `aca.VolumeState.ShareName` + `cloudrun.VolumeState.BucketPath` fields. Real per-cloud volume provisioning filed as separate follow-up phases (EFS / Filestore / Azure Files).
2. **BUG-735** — ECS `buildContainerDef` rejects `HostConfig.Binds` with a clear error when `SOCKERLESS_ECS_AGENT_EFS_ID` is unset; no scratch-volume fallback.
3. **BUG-736** — Cloud Run (`jobspec.go` / `servicespec.go`) + ACA (`jobspec.go` / `appspec.go`) reject `HostConfig.Binds` and named-volume mounts with a clear error until real mount support ships.
4. **BUG-737** — Remove `SOCKERLESS_SKIP_IMAGE_CONFIG` env var; `FetchImageMetadata` always tries the real registry, callers always get a real error on failure. Audit simulators' registry slices (ECR / AR / ACR) to ensure `/v2/` manifest + config endpoints are served for every published image so there's no metadata gap to fall back over.
5. **Broader sweep** — one more pass for swallowed errors (`_ = err`, `if err != nil { return nil }`), silent `continue` on error in aggregation loops, and `NotImplemented` returns that could be real implementations.

Each goes into BUGS.md before the fix, mirrors into STATUS.md / WHAT_WE_DID.md after.

## Workaround that needs live-cloud work

- **BUG-729** — proper SSM ack format so AWS's Session Manager agent stops retransmitting. Requires live-AWS testing to iterate on header layout.

## Pending after Phase 90

See [PLAN.md](PLAN.md). Priority-ordered:

1. **Phase 87 live-GCP runbook** — Cloud Run Services validation against real GCP.
2. **Phase 88 live-Azure runbook** — ACA Apps validation against real Azure.
3. **Phase 86 Lambda live track** — scripted already, deferred for session-budget reasons.
4. **Real volume phases** — follow-up to BUG-731: EFS/EBS for ECS, Filestore/GCS for Cloud Run, Azure Files for ACA. Each its own architectural phase.
5. **Phase 68 — Multi-Tenant Backend Pools** (P68-002 → 010).
6. **Phase 78 — UI Polish**.

## Operational state

- AWS: zero residue (state buckets + DDB lock table retained as cheap reusable infra).
- Local sockerless backend: stopped.
- No credentials in environment.
