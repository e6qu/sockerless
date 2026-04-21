# Sockerless — Roadmap

> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

Current state: [STATUS.md](STATUS.md). Bug log: [BUGS.md](BUGS.md). Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md). Architecture: [specs/](specs/).

## Guiding principles

1. **Docker API fidelity** — match Docker's REST API exactly.
2. **Real execution** — simulators and backends actually run commands; no stubs, fakes, or mocks.
3. **External validation** — proven by unmodified external test suites.
4. **No new frontend abstractions** — Docker REST API is the only interface.
5. **Driver-first handlers** — all handler code through driver interfaces.
6. **LLM-editable files** — source files under 400 lines.
7. **GitHub API fidelity** — bleephub works with unmodified `gh` CLI.
8. **State persistence** — every task ends with state save (PLAN / STATUS / WHAT_WE_DID / BUGS / memory).
9. **No fallbacks, no defers** — every functional gap is a real bug; every bug gets a real fix in the same session it surfaces; cross-cloud sweep on every find.

## Closed phases

- **86** — Simulator parity across AWS + GCP + Azure + Lambda agent-as-handler + Phase C live-AWS ECS validation. See `docs/SIMULATOR_PARITY_{AWS,GCP,AZURE}.md`, [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md), and BUGS.md entries 692–722.
- **87** — Cloud Run Jobs → Services path behind `SOCKERLESS_GCR_USE_SERVICE=1` + `SOCKERLESS_GCR_VPC_CONNECTOR`. Closes BUG-715 in code. Live-GCP runbook pending.
- **88** — ACA Jobs → ContainerApps path behind `SOCKERLESS_ACA_USE_APP=1` + `SOCKERLESS_ACA_ENVIRONMENT`. Closes BUG-716 in code. Live-Azure runbook pending.
- **89** — Stateless-backend audit. `specs/CLOUD_RESOURCE_MAPPING.md` for all 7 backends; every cloud-state-dependent callsite uses `resolve*State` helpers; `ListImages` / `ListPods` cloud-derived; Store.Images disk persistence removed. Closes BUG-723/724/725/726.

## Pending work

### Live-cloud validation runbooks

- **Phase 87 live-GCP** — parallel to `scripts/phase86/*.sh` for AWS. Needs GCP project + VPC connector. Script the runbook, dispatch via a new workflow, validate `docker run` / `docker exec` / cross-container DNS against Services.
- **Phase 88 live-Azure** — same shape for ACA. Needs Azure subscription + managed environment with VNet integration.
- **Phase 86 Lambda live track** — scripted already, deferred at Phase C closure for session-budget reasons. No architectural blockers.

### Phase 91 — ECS real volumes (queued)

Replace the `NotImplemented` returns from BUG-731 with real cloud-side provisioning.

- **Simulator**: new `simulators/aws/efs.go` EFS slice — FileSystem + MountTarget + AccessPoint CRUD. Back each access point with a subdirectory on a host-side Docker volume so the sim's ECS task containers bind-mount the same path.
- **Backend**: `backends/ecs/volume_cloud.go` — `VolumeCreate` ensures a sockerless-tagged EFS exists (reused across volumes), then creates an AccessPoint per volume with `PosixUser` + `RootDirectory` so each volume is isolated. `VolumeRemove` deletes the access point; EFS stays.
- **Spec wiring**: `taskdef.go::buildContainerDef` rejects bind mounts without EFS (BUG-735) but now happily emits `EFSVolumeConfiguration{FileSystemId, AccessPointId, TransitEncryption=ENABLED}` when a volume reference is in scope.
- **Tests**: SDK + CLI + terraform cover `efs:CreateAccessPoint` / `DescribeAccessPoints` / `DeleteAccessPoint`. Integration test spins up two containers sharing the same volume and verifies file visibility.
- **Docs**: spec's "Volume provisioning per backend" row flips from design to "implemented in Phase 91".

### Phase 92 — Cloud Run real volumes (queued)

- **Simulator**: extend `simulators/gcp/storage.go` (GCS slice) to honour `Volume{Gcs{Bucket}}` on the Cloud Run simulator's spec-builder path, bind-mounting a host directory per bucket.
- **Backend**: `backends/cloudrun/volume_cloud.go` — `VolumeCreate` calls `storage.Buckets.Insert` with `sockerless-managed=true` label. Service spec's `RevisionTemplate.Volumes[]` gets `Gcs{Bucket}`; `Container.VolumeMounts` references them. Operator IAM: service account needs `roles/storage.objectAdmin` on sockerless buckets.
- **Out of scope for Phase 92**: Filestore POSIX mounts (different semantics — strong locking, `O_APPEND`). Filed as Phase 92.1 if GCS semantics prove insufficient.
- **Tests**: SDK + CLI.

### Phase 93 — ACA real volumes (queued)

- **Simulator**: `simulators/azure/storage.go` grows Azure Files `fileServices/shares` CRUD (blob slice already present). `simulators/azure/containerappsenv.go` grows `storages` sub-resource.
- **Backend**: `backends/aca/volume_cloud.go` — `VolumeCreate` ensures a sockerless storage account exists, then `FileShares.Create` + `ManagedEnvironmentsStorages.CreateOrUpdate` so the environment knows about the share. ContainerApp spec's `Template.Volumes[]` + `Container.VolumeMounts` reference the env-storage. `VolumeRemove` tears both down.
- **Tests**: SDK + CLI.

### Phase 94 — GCF + AZF volume alignment (queued)

Sockerless targets only the latest generation of each cloud service (no fallbacks between generations). For GCP Cloud Functions that's Cloud Functions v2 (Cloud Run Services under the hood) — inherit Phase 92's implementation via a shared helper. For Azure Functions that's Flex Consumption / Premium plan (BYOS Azure Files) — inherit Phase 93's Azure Files share provisioning.

If operators target an older generation (GCF v1, Azure Functions Consumption plan on older runtimes), the backend fails fast at config validation with a clear "upgrade your function to the supported generation" error rather than degrading silently.

### Phase 68 — Multi-Tenant Backend Pools (queued)

Named pools of backends with scheduling and resource limits. `P68-001` done; remaining tasks:

| Task | Description |
|---|---|
| P68-002 | Pool registry (in-memory, each with own BaseServer + Store) |
| P68-003 | Request router (route by label or default pool) |
| P68-004 | Concurrency limiter (per-pool semaphore, 429 on overflow) |
| P68-005 | Pool lifecycle (create/destroy at runtime via management API) |
| P68-006 | Pool metrics (per-pool stats on `/internal/metrics`) |
| P68-007 | Round-robin scheduling (multi-backend pools) |
| P68-008 | Resource limits (max containers, max memory per pool) |
| P68-009 | Unit + integration tests |
| P68-010 | Save final state |

### Phase 78 — UI Polish (queued)

Dark mode, design tokens, error handling UX, container detail modal, auto-refresh, performance audit, accessibility, E2E smoke, documentation.

### Known workarounds to convert to real fixes

- **BUG-721** — sockerless's SSM `acknowledge` format isn't accepted by the live AWS agent, so the backend dedupes retransmitted `output_stream_data` frames by MessageID. Proper fix is to match the agent's ack-validation rules exactly (likely Flags or PayloadDigest semantics); requires live-AWS testing. Pure sim-path isn't affected.

## Future ideas

- GraphQL subscriptions for real-time event streaming.
- Full GitHub App permission scoping.
- Webhook delivery UI.
- Cost controls (per-pool spending limits, auto-shutdown).
