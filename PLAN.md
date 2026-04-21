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
- **90** — No-fakes/no-fallbacks audit. 11 bugs filed, 8 fixed in-sweep (BUG-729/730/731/732/733/734/735/736/737), 3 scoped as dedicated phases (BUG-744/745/746 → Phase 95/96/97). See WHAT_WE_DID.md for the full table.
- **91** — ECS named volumes + named-volume bind mounts backed by EFS access points on a sockerless-owned filesystem. Simulator `EFSAccessPointHostDir` helper, backend `volumes.go` EFS manager, task defs emit real `EFSVolumeConfiguration`. Completes BUG-735 and the ECS half of BUG-736.

## Pending work

### Live-cloud validation runbooks

- **Phase 87 live-GCP** — parallel to `scripts/phase86/*.sh` for AWS. Needs GCP project + VPC connector. Script the runbook, dispatch via a new workflow, validate `docker run` / `docker exec` / cross-container DNS against Services.
- **Phase 88 live-Azure** — same shape for ACA. Needs Azure subscription + managed environment with VNet integration.
- **Phase 86 Lambda live track** — scripted already, deferred at Phase C closure for session-budget reasons. No architectural blockers.

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

### Phase 95 — FaaS invocation-lifecycle tracker (Lambda + GCF + AZF) (queued)

BUG-744 root cause: FaaS backends' CloudState can't distinguish "function is deployed" from "invocation is running". The *function* is `ACTIVE` regardless of invocation state. `docker wait` / `docker inspect` / `docker ps` therefore can't surface an accurate exited state for short-lived runs, and invocation-failure exit codes are lost (the sim returns HTTP 500 with a body but the backend persists no exit code). Same shape on Lambda, GCF, AZF.

Each backend has a cloud-native signal for per-invocation completion — the fix is to wire that signal through, not to keep a local dictionary. Exactly *one* resource per invocation is authoritative:

| Backend | Invocation resource | Completion signal | Exit-code source |
|---|---|---|---|
| Lambda | `lambda:Invoke` response + CloudWatch Logs `END RequestId` | `InvokeResponse` returns, or log stream gets its terminal `REPORT` line | `FunctionError` (`Unhandled`/`Handled`) ⇒ 1; payload OK ⇒ 0; timeout (`REPORT: … Status: timeout`) ⇒ 124 |
| GCF | HTTP response from `ServiceConfig.Uri` | HTTP status from the invoke POST | 2xx ⇒ 0; 4xx/5xx ⇒ 1 (function-code crash); 408 ⇒ 124 (timeout) |
| AZF | HTTP response from Function App default host | HTTP status from the HTTP trigger POST | Same mapping as GCF |

The invocation-driving goroutine in each backend (`ContainerStart` → goroutine that calls `Invoke` / `POST functionURL`) already knows the outcome — it just drops the exit info today. The right design is:

1. **Capture at the source.** The goroutine records `(containerID, exitCode, stoppedAt)` into a small `InvocationResults sync.Map` on the Server struct when the invocation finishes. This is in-memory, crash-scoped (the invocation was one-shot anyway — post-restart the function's done and the user won't call `docker wait` on it). Explicitly not a revival of `Store.Containers`.
2. **CloudState reads from both.** `GetContainer` / `ListContainers` check `InvocationResults` first — if present, container state is `{Status: "exited", Running: false, ExitCode: N, FinishedAt: T}`. If absent, fall through to cloud lookup (function exists ⇒ `running`; function missing ⇒ `false, nil`).
3. **ContainerStop becomes cooperative.** Write `{ExitCode: 137}` into `InvocationResults` + close the wait channel. Subsequent `Wait` unblocks; `Inspect` shows exited.

What this buys in terms of Docker CLI coverage:
- `docker wait` — returns the real invocation exit code (was always 0).
- `docker inspect` — `State.Status` reflects exited after the invocation completes.
- `docker ps` (no `-a`) — the exited container drops off.
- `docker ps -a` — exited containers appear with their exit code.
- `docker stop` + concurrent `docker wait` — stop unblocks wait (BUG-744 third branch).

Post-restart behaviour (`InvocationResults` gone): `CloudState` sees the cloud function still exists and reports it as `running` until the user removes it. That matches docker's contract for a crashed daemon: state after restart derives from whatever the underlying cloud records — same invariant the rest of the backend already observes.

Tests re-enable: `TestLambdaContainerLifecycle`, `TestLambdaContainerLogsFollowLazyStream`, `TestLambdaContainerStopUnblocksWait`, `TestGCFContainerLifecycle`, `TestGCFArithmeticInvalid`, `TestAZFContainerLifecycle`, `TestAZFArithmeticInvalid` (all deleted as stop-gap for BUG-744).

Simulator work alongside (so live-cloud and simulator behaviour match): the GCP Cloud Functions sim already returns the container's exit code via HTTP status. The AZF sim does the same. The AWS Lambda sim's `Invoke` must set `FunctionError` on non-zero exit and include the last-4KB log tail in the `LogResult` response header — required by the Lambda backend's exit-code derivation.

### Phase 96 — Reverse-agent exec for Cloud Run Jobs + ACA Jobs (queued)

BUG-745 root cause: Cloud Run Jobs and ACA Jobs have no control-plane "attach to running container" API — the Jobs products' entire abstraction is "submit work, read logs after". Lambda has the same gap and solved it by running an agent *inside* the container that dials back over WebSocket (see `agent/cmd/sockerless-lambda-bootstrap`). We port that pattern to Cloud Run + ACA Jobs.

Cloud resource mapping:

| Backend | What the agent needs | Where it lives | How the backend reaches it |
|---|---|---|---|
| `cloudrun` (Jobs) | A pre-baked overlay image containing `sockerless-cloudrun-bootstrap` as its ENTRYPOINT; the bootstrap execs the user's original entrypoint+cmd in a subprocess AND dials `SOCKERLESS_CALLBACK_URL` over WS. | Published to the operator's Artifact Registry; backend references via `SOCKERLESS_GCR_PREBUILT_OVERLAY_IMAGE` env var (mirrors `SOCKERLESS_LAMBDA_PREBUILT_OVERLAY_IMAGE`). | Backend listens on `/v1/cloudrun/reverse` WS endpoint; agent container dials via Serverless VPC Access connector or the existing outbound-egress path. |
| `aca` (Jobs) | Same pattern — `sockerless-aca-bootstrap` image. | Operator's ACR, referenced via `SOCKERLESS_ACA_PREBUILT_OVERLAY_IMAGE`. | Backend listens on `/v1/aca/reverse` WS; agent dials via the Managed Environment's outbound NAT. |

Other scope:
- Three bootstraps share the exec-handling code in `agent/reverse` (already factored out for Lambda); only the startup-time "how do I know my container ID" differs per cloud (Lambda takes `SOCKERLESS_CONTAINER_ID` env, Cloud Run reads `CLOUD_RUN_TASK_ATTEMPT` + job-execution tags, ACA reads `CONTAINER_APP_NAME` + job-execution tags).
- Simulator work: the Cloud Run Jobs + ACA Jobs sims need to honour the prebuilt overlay image the same way the Lambda sim already does — just pull + run locally, no push-to-real-registry hoop.
- Re-enable `TestCloudRunContainerExec` / `TestACAContainerExec` once the overlay images ship.

### Phase 97 — Docker labels on FaaS/GCP backends (queued)

BUG-746 root cause: the Docker label map is serialised as JSON and stored in a single GCP *label* (`sockerless_labels`). GCP label values are `[a-z0-9_-]{0,63}` — JSON's `{`, `:`, `"` are rejected by the real API, the sim silently drops them, so `docker ps --filter label=…` can never match a container whose labels never survived the round-trip. ECS already uses AWS tags (no char restrictions) and works.

Per-cloud resource with arbitrary string values (where Docker labels should actually live):

| Backend | Cloud resource that accepts arbitrary strings | Conventions |
|---|---|---|
| `cloudrun` (Jobs + Services) | **Annotations** on `Job` / `Service` (up to 256 KB per value) | Dedicated keys: `sockerless.dev/labels` for the JSON blob, `sockerless.dev/<kv>` for individual labels large enough to filter on without parsing JSON. |
| `cloudrun-functions` (gcf) | **Annotations** on `Function` | Same convention as `cloudrun`. |
| `aca` (Jobs + Apps) | **Tags** on the resource (Azure tag values allow any string) OR **Metadata** on the container template | Prefer tags — `docker ps --filter` can map to Azure's `resource tags` filter directly. |
| `azure-functions` (azf) | **App Settings** (already arbitrary strings) OR **Tags** | Tags for filterability, App Settings for anything the runtime itself reads. |

Other scope:
- Update `core.TagSet.AsGCPLabels` / `AsGCPAnnotations` split so individual label keys sit in GCP labels when they're `[a-z0-9_-]{0,63}` (docker-ps filter path stays fast), and the JSON blob goes to an annotation only.
- Simulator work: the GCP sim's `Function`, `Job`, `Service` resources all already carry annotation maps; add round-trip coverage for arbitrary string values.
- Re-enable the label-filter assertions in `Test{CloudRun,ACA,GCF,AZF}ArithmeticWithLabels` (removed in BUG-746).

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
