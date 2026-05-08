# Known Bugs

**974 filed · 972 fixed · 2 open · 1 false positive.**

Standing rule: every CI / live-cloud failure lands here with a one-liner before any fix attempt. Workarounds, fakes, placeholders, silent fallbacks, and incomplete implementations are all bugs and get the same treatment. Per-bug fix detail beyond the one-liner: `git log <commit>` or the linked PR.

Live status (cells, branch, milestone) lives in [STATUS.md](STATUS.md).

## Open

| ID | Sev | Area | One-liner |
|----|-----|------|-----------|
| 972 | H | cloudrun + gcf | `ImagePull` rewrites Docker Hub refs to AR proxy unconditionally; sim has no AR proxy → 403. Fix: gate the rewrite on `s.config.EndpointURL == ""` (real GCP only). Same site applies to `ContainerCreate` + every other `gcpcommon.ResolveGCPImageURI` caller. |
| 949 | M | simulators/gcp gcf | `gcf::simCommand` `os/exec`s the workload binary as a host process, ignoring the workload's architecture. Symptom on macOS/arm64 dev hosts: `linux/amd64` workload binary fails to exec. Wrong-axis fix would be "build two binaries"; correct fix is to dispatch workloads through Docker honouring the workload's `Architecture` field (default `linux/arm64`). Sim binary itself stays host-native. See `feedback_sim_workload_arch.md`. |

## False positives

| Area | Finding | Why it's not a bug |
|------|---------|--------------------|
| `backends/aca/azure.go::fakeCredential` | Returns literal `"fake-token"` against simulator endpoints. | Sims don't verify bearer tokens — would require real Azure AD endpoint not emulated. Credential wired only via `newAzureClientsWithEndpoint` (sim path); production uses `azidentity.NewDefaultAzureCredential`. |

## Class-of-bug rules (carried forward)

- **Backend ↔ host primitive must match (P0).** ECS backend in ECS, Lambda backend in Lambda, Cloud Run backend in Cloud Run, Cloud Run Functions in CRF, ACA in ACA, Azure Functions in AZF. Cross-pollution is a critical architectural error. See `MEMORY.md` workflow rules + `specs/CLOUD_RESOURCE_MAPPING.md § runner-on-FaaS dispatch`.
- **No fakes / no fallbacks.** Synthetic exit codes, silent shims, fake-data fallbacks all file as bugs and get real fixes.
- **Cross-cloud sweep on every find.** When a pattern is found in one backend, the same code paths in the other 6 backends / 3 sims get checked in the same commit.

## Resolved (compressed history)

Per-bug detail in `git log` / linked PR.

### 2026-05-08 — Sim test stability (PR #128)

| ID | Sev | Area | One-liner |
|----|-----|------|-----------|
| 974 | M | simulators/azure SDK tests | `TestContainerApps_JobArithmeticInvalid` + `JobArithmeticLogs` used fixed `time.Sleep(2s)` to wait for ACA job execution. Slow CI runners exceed 2s on container pull/start → flake. Fix: replaced both with `require.Eventually(30s, 250ms)` polling for terminal status / log presence. |
| 973 | M | simulators/aws SDK tests | `TestECS_TaskLogsToCloudWatch` used fixed `time.Sleep(2s)` to wait for alpine `echo` stdout to reach CloudWatch. Slow CI runners exceed 2s → "Running" / empty events flake. Fix: `require.Eventually(30s, 250ms)` polling for the expected log line. Surfaced once Makefile standardization unbroke the sim docker build (was masking the test on main). |

### 2026-05-07 — Phase 123 close-out (8/8 GREEN, PR #123)

| ID | Sev | Area | One-liner |
|----|-----|------|-----------|
| 971 | H | cloudrun + gcf | Multi-container revision OOM during go build. Bumped main container to 2Gi; postgres stays 1Gi. |
| 970 | H | Cloud Run | Misleading "container failed to bind PORT=8080" was actually regional CPU quota exhausted by orphan `sockerless-svc-*` with `minInstanceCount=1`. Structural fix: materialize sets `minInstanceCount=0`. Followup orphan GC shipped 2026-05-08 (Phase 129 #4). |
| 969 | H | cloudrun | `mapCPUMemory` defaulted 512Mi/container — too small for postgres `initdb`. Bumped to 1Gi to match gcf. |
| 968 | H | cloudrun | `OverlayContentTag` keyed on bootstrap PATH not content (mirror of gcf BUG-957). Fix: hash binary at startup, stamp into spec. |
| 967 | H | gcs-sync | `SOCKERLESS_SYNC_MOUNTS` (materialize-time, name=mountpath) + `SOCKERLESS_SYNC_VOLUMES` (per-exec, name=gs://bucket/object). 3-iteration shape settle. |
| 966 | H | cloudrun-functions pod-Service | `WorkingDir` in container spec rejected when `gcs-sync` workspace empty until restore. Fix: drop WorkingDir; bootstrap chdirs per-exec via envelope. |
| 965 | M | GCSFuse | Stale-file-handle on `event.json` during clone-and-compile. **Superseded** by Phase 123's `gcs-sync` data plane (no FUSE in data path). |
| 964 | H | gcf | `invokePodServiceMain` default-invoked long-lived JOB CMD (mirror of cloudrun BUG-961). Fix: `skipIfNoStdin=true` from pod-Service materialize call. |

### 2026-05-04 → 2026-05-06 — Cells 5/6/7/8 saga (Phase 122d–122m, PR #123)

| ID | Sev | Area | One-liner |
|----|-----|------|-----------|
| 963 | H | dispatcher | GH runner-task `/tmp/runner-work` was tmpfs — workspace files didn't reach JOB pod-Service GCSFuse mount. Fix: dispatcher TOML's `Label.runner_workspace_bucket` adds `Volume{Gcs}` + `VolumeMount{/tmp/runner-work}` on runner-task spec. |
| 962 | H | gcf + cloudrun | Exec response not docker-stream-framed — runner read byte 0 as header. Fix: `execStartViaInvoke` wraps stdout in 0x01 frame + stderr in 0x02 via existing `writeMuxFrame`. |
| 961 | H | cloudrun | Pod-Service default-invoke hung on long-lived JOB CMD. Fix: `invokeServiceDefaultCmd` adds `skipIfNoStdin bool`; pod-Service mode passes true. |
| 960 | H | gcf + cloudrun | `Typed.Exec` wired to reverse-agent driver only — GH actions/runner can't dial back. Fix: `WrapLegacyExecStart` so non-interactive routes through invoke envelope. Plus gcf `sanitizeServiceContainerName` trailing-trim. |
| 959 | H | gcf + cloudrun | GH actions/runner pattern (OpenStdin=false JOB) deferred forever. Fix: when sibling exists, materialize with first-arrived sibling as main + others as sidecars. |
| 958 | H | cloudrun | Mirror of gcf BUG-955: `ContainerStart` returned NotModifiedError on already-running container without checking for fresh stdinPipe. Fix: kick `invokeRunningRunnerStage` goroutine; ContainerStop preserves Service. |
| 957 | H | gcf | Bootstrap missing tar-pack persist; `OverlayContentTag` keyed on path not content. Fix: port persist module to gcf bootstrap; hash binary into overlay spec. |
| 956 | H | gcf | Multi-image-per-stage materialize race. Fix: `pendingMembersOfNetwork` filters containers already MAIN of an existing pod-Service. |
| 955 | H | gcf | `Typed.Attach` was wired to read-only `NewCloudLogsAttachDriver` (cloudrun was correct). Fix: mirror cloudrun's `WrapLegacyContainerAttach` wiring. |
| 954 | H | gcf | Cell 8 v9-v15 silent hang in prepare_script — root cause was the architectural stack closed by 12 fixes shipped over v9-v25. |
| 953 | H | gcf | Pod materialize too slow (~150s vs 120s gitlab-runner timeout). 4-fix architectural stack: direct multi-container Cloud Run Service (skip Cloud Functions wrapper); GetService follow-up on abbreviated annotations; AR HEAD precheck; PendingCreates speculative-running marker. |
| 952 | M | gcf | `resolveGCFFromCloud` returned empty Function URL (Cloud Functions ListFunctions abbreviates `ServiceConfig.uri`). Fix: GetFunction follow-up; final fallback derives URL from underlying Cloud Run Service. |
| 951 | H | gcf | Pool-claim env update via UpdateService hit regional CPU quota. Fix: drop `updateFunctionUserEnv`; use invoke envelope (Path B). |
| 950 | H | gcf | Prewarm contentTag mismatch — `OverlayContentTag` included Entrypoint/Cmd/Workdir, prewarm couldn't match live workload. Fix: drop those from formula; pass at runtime via env. |
| 948 | H | gcf | Per-step Cloud Function deploys exhaust regional CPU quota; gitlab-runner times out at 120s with misleading "Cannot connect to Docker daemon". Fix: pre-warm pool of N functions at gitlab-runner-gcf startup. |
| 947 | H | GCSFuse | `/builds` workspace incompatible with git checkout — silent hang from missing POSIX hard-link / weak rename / no flock. Fix: bootstrap tar-pack persist module + backend Volume_EmptyDir + `SOCKERLESS_PERSIST_VOLUMES`. Drove eventual Phase 123 storage-driver abstraction. |
| 946 | M | cloudrun + gcf tests | Integration tests panicked with nil dockerClient on default `go test`. Fix: `//go:build integration` tag; explicit error when `-tags integration` without env var. |
| 945 | M | runner image build | podman bridge-network apt failures (no workarounds permitted per security rule). Fix: pre-bake `runner-base-amd64` apt-deps base image; per-iteration build only `COPY`s sockerless binary. |
| 944 | H | gcf | 3-layer pool-volume attach bug (skip on pool hit; idempotent-by-name only; missing MountOptions). Fix at each layer + verification protocol established. |
| 943 | H | dispatcher | Poller 1+N GitHub calls per cycle exhausted 5000/h PAT bucket. Fix: cached `X-RateLimit-Remaining` early-return; `runSeen` set; default cadence 15s → 60s. |
| 942 | H | Cloud Run | Regional `cpu_allocation` per-minute quota exhausted by parallel deploys. Fix: gcf pool `claimFreeFunction` 8-attempt exponential back-off (250ms → 2s, ~5s total). |
| 941 | H | dispatcher | Cleanup ticker re-fired GitHub poll during rate-limit window. Fix: track `rateLimitedUntil` wall-clock; skip Step() while inside window. |
| 940 | H | dispatcher | Cleanup deleted runner-tasks 80s after spawn. Root cause: keyed off Job-DEFINITION reconciliation state, not Execution state. Fix: `executionStateForJob` queries ListExecutions. |
| 939 | H | dispatcher | Runner-task default 512Mi/1cpu OOMed during go build. Fix: 2cpu/4Gi on runner-task spec. |
| 938 | H | dispatcher | GitHub PAT 403 from abuse-flagged Cloud NAT egress IP. Fix: static IP `34.31.88.230`; AUTO_ONLY → MANUAL_ONLY pinned; backoff on 403 not crashloop. |
| 937 | H | cloudrun + gcf ImagePull | Three defects in AR-remote-proxy pull (auth wrap; bearer dance; alias-only-under-AR-ref). Fix: discard caller auth on rewrite; AR/GCR fast-path strips Bearer prefix; alias under original ref. |
| 925 | H | cloudrun postgres `services:` | Cloud Run only exposes HTTPS:443 — gitlab-runner health-check fails on TCP:5432. Fix path A: multi-container Cloud Run sidecar (chosen). |
| 923 | H | gcf ContainerCreate | 150-200s CreateFunction.Wait exceeded gitlab-runner's 120s timeout. Architectural fix: pre-deploy Cloud Run Service per runner-image shape; ContainerCreate updates revision env. |
| 922 | H | cloudrun runner-pattern | Cloud Run Job is one-shot — auto-cleanup deleted Job after first execution, breaking gitlab-runner's exec-into-same-id pattern. Fix: switch runner-pattern containers to Cloud Run Service (long-lived). |

### Earlier 2026-05 / 2026-04 — runner cells (PRs #122 / #123)

BUG-877..921: live-GCP cloudrun manual sweep (cells 5/6/7/8 enablement); Phase 122d→g network-pod overlay path; reverse-agent + Path-B exec + AR/CR routing; gitlab-runner stage delivery on FaaS. Detail in `git log` and per-PR description.

### 2026-04-30 — Phase 110 runner integration (PR #122)

BUG-845..876 (32 fixes):

- **AWS infrastructure** (845–850): terraform live-env alignment to eu-west-1; Docker Hub library refs → AWS Public Gallery / ECR pull-through; ECS bind-mount → SharedVolumes (EFS access points); GitHub Actions `Initialize containers` `/home/runner/_work` path mapping.
- **ECS exec + lifecycle** (851–854, 858): SyntheticNetworkDriver for ECS host; per-network SG + operator default SG on sub-tasks; Fargate ExecuteCommandAgent waiter; digest-only image-ref handling; PendingCreates fallback to ResolveContainerAuto on second start.
- **GitLab × ECS attach-stdin** (855, 859): `/sockerless/v/<sha256>[:16]` short EFS path; `ecsStdinAttachDriver` typed driver routes script-via-attach into stdinPipe → RunTask payload.
- **Lambda overlay-inject** (856, 860–874): runner-image `/tmp/runner-state/_work` env; mirror ECS attach-stdin pattern as `lambdaStdinAttachDriver`; `library/` prefix strip; `LogType=Tail` on every Invoke for inline crash diagnostics.
- **GitLab × Lambda script delivery** (875, 876): empty `{}` Invoke payload was being piped as script; library/ prefix wasn't stripped.
- **Test harness + arch reporting** (847–849, 857, 863): runner OS naming `darwin → osx`; throwaway per-cell branch; ECS+Lambda CpuArchitecture/Architecture mandatory config; SOCKERLESS_ECS_CPU_ARCHITECTURE wired in tests + smoke; gitlab-runner-helper image pre-pushed to ECR.

### 2026-04-27 — Phase 109 strict cloud-API fidelity audit (PR #121)

19 audit items, no new BUG numbers (audit-driven fidelity work). Lambda VpcConfig from real subnet CIDR; AWS Secrets Manager + SSM + KMS + DynamoDB; GCP `compute.firewalls` + `compute.routers`/Cloud NAT + `iam.generateAccessToken` + operations endpoint persistence; Azure IMDS + Blob ARM CRUD + NSG validation + Private DNS records + NAT Gateways + Route Tables + ACA Async-Op polling + Key Vault ARM/data; ARM SystemData preservation.

### 2026-04-27 — Post-PR-#118 audit + Phase 104/105/108 (PR #120)

| Range | Theme |
|---|---|
| 802 / 820–831 | Audit pass: synthetic-exit-code defaults, silent fallbacks in registry/network/build/ipam, Linux netns error-checking, `endpoint.IPAddress` cleanups across all 6 cloud backends. |
| 832–835 | Phase 108 sim-parity: ECS `TagResource`/`UntagResource`; Cloud Run v2 Services routes; Container Apps Apps surface; Azure WebApps `UpdateAzureStorageAccounts`. |
| 836–844 | Phase 104 typed-driver migration CI pass: ECS/Lambda lifecycle correctness; sim/aws SSM AgentMessage frames + `RunTask` real subnet IPs; sim/gcp `enumString` for proto-JSON enum; sim/azure per-site DefaultHostName. |

### 2026-04 — Round-7 / Round-8 / Round-9 live-AWS sweeps (PRs #117, #118)

| Range | Theme |
|---|---|
| 770–785 | Round-7: ImageRemove correctness; ECS task lifecycle (rename, restart, kill-signal mapping); libpod compat; OCI push auth + config-blob; Lambda bootstrap PID + heartbeat; registry persistence robustness. |
| 786–819 | Round-8 + 9: Real registry-to-registry layer mirror (BUG-788 closes 4 retroactive); live SSM frame capture → exit-code marker; sync `docker stop`; per-network SG isolation; Lambda Active-waiter; per-cloud terragrunt sweep. |

### Earlier (PRs #112–#115, ≤ Phase 102)

| Range | Theme |
|---|---|
| 661–684 | Stateless invariants — auto-agent removed; CloudState replaces Store reads in 16 sites; ResolveContainerAuto migration. |
| 685–699 | Phase 86 simulator parity — real Lambda Runtime API; sim workloads via Docker SDK; ServiceV2/JobV2 LatestCreatedExecution; SystemData on ContainerAppJob. |
| 700–711 | Cross-cloud sims + DNS — per-namespace docker network in sims; `pre-register subnet-sim` in EC2 sim; ACA Private DNS Zones; ACR cache-rule CRUD; Cloud Build secret manager. |
| 712–719 | Phase 86 ECS networking — idempotent network/namespace create; `DnsSearchDomains` via `/bin/sh -c`; default port `:3375`. |
| 720–726 | Round-6 ECS exec + state — SSM AgentMessage parser/ack; full SSM IAM; `EnableExecuteCommand: true`; cloud-derived NetworkState; Cloud Map post-RUNNING. |
| 727–769 | Phases 91–102 — per-cloud volumes; reverse-agent for `docker top/stat/cp/get-archive/put-archive/export/diff/commit/pause`; Cloud Run Services / ACA Apps; stateless audit; no-fakes sweep. |

Pre-661 detail in `git log` + per-PR descriptions.

## Cross-cloud sweep notes

- BUG-826 swept 6 cloud backends — the synthetic `exitCode=0` pattern was identical across `core/backend_impl.go`, `core/backend_impl_pods.go`, every cloud backend's `backend_impl.go`.
- BUG-829 swept gcp-common + azure-common — same per-tag silent-continue pattern as BUG-825.
- BUG-714 → BUG-715 (cloudrun) + BUG-716 (aca): all three backends seeded `ep.IPAddress = "0.0.0.0"`; structural fix moved cloudrun + aca to Services/Apps with stable FQDNs (Phase 87/88).
- BUG-712 → BUG-713 (cloudrun): non-idempotent-create pattern. Azure ACA's PUT-style `BeginCreateOrUpdate` is already idempotent; Lambda/GCF/AZF have no cloud-side network creation.
- BUG-710 swept all 7 backend mains + CLI + READMEs + example terraform + `tools/http-trace`.
