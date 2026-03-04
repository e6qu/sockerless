# Known Bugs

## BUG-001: `docker run` with simple commands produces no output against cloud backends in simulator mode

**Severity**: High
**Component**: ECS backend (and likely CloudRun, ACA backends too)
**Status**: Fixed — replaced EndpointURL checks with IsTailDevNull, added agent PATH resolution in simulators
**Discovered**: 2026-03-02 during Phase 80 manual E2E verification

### Symptoms

Running `docker run --rm alpine echo "hello"` against the Docker frontend connected to an ECS backend (with AWS simulator) produces no output and exits 0. The echo message is silently lost.

### Root Cause

Two interacting issues in the ECS backend's `handleContainerStart` (`backends/ecs/containers.go:201-207`):

**Issue A — Helper container shortcut**: When `SOCKERLESS_CALLBACK_URL` is set, the start handler classifies containers as either "job containers" (command = `tail -f /dev/null`) or "helper containers" (everything else). Helper containers are auto-stopped after 500ms without calling RunTask at all. This means `docker run alpine echo "hello"` never reaches the simulator — the container is marked "exited 0" immediately.

**Issue B — No agent injection in simulator mode**: When RunTask IS called (for `tail -f /dev/null` containers), the task definition builder skips agent injection in simulator mode. The backend still sets `SOCKERLESS_CALLBACK_URL` and waits for agent callback, but neither the backend nor the simulator injects the agent binary.

### Impact

This doesn't affect CI runner workflows (which use `tail -f /dev/null` containers and `docker exec`), but it means standalone `docker run` with simple commands doesn't produce output when using cloud backends in simulator mode. The memory backend and docker backend work correctly.

---

## BUG-002: `docker exec` uses synthetic fallback in simulator mode (echoes command instead of executing)

**Severity**: Medium
**Component**: ECS backend (and likely CloudRun, ACA backends too)
**Status**: Fixed — same fix as BUG-001 (agent injection based on command type, not simulator detection)
**Related**: BUG-001 Issue B

### Symptoms

After starting a container with `tail -f /dev/null` via `docker start`, running `docker exec <container> echo "hello"` returns the text `echo hello` instead of executing the command and returning `hello`.

### Root Cause

The exec handler falls through to the synthetic driver because no agent is connected. In simulator mode with callback URL, the backend waits for agent callback which times out, and any subsequent `docker exec` uses the synthetic driver which echoes the command text.

### Impact

Same as BUG-001 — doesn't affect CI runner workflows, but breaks interactive use in simulator mode.

---

## BUG-047: `gofmt` violations in cleanup.go and project.go

**Severity**: Low (formatting only)
**Component**: `cmd/sockerless-admin/cleanup.go`, `cmd/sockerless-admin/project.go`
**Status**: Fixed — Sprint 7: Ran `gofmt -w` on both files

### Details

- `cleanup.go:146-158`: Wrong indentation inside `if c.State == "exited" || c.State == "dead"` block — code compiles correctly (Go uses braces) but violates `gofmt` formatting
- `project.go:62-68`: `ProjectConnection` struct has extra padding on field tags (`DockerHost        string` instead of `DockerHost       string`)

---

## BUG-048: GCF backend uses non-cloud-native `X-Sockerless-Command` header

**Severity**: Medium
**Component**: `backends/cloudrun-functions/containers.go`, `simulators/gcp/cloudfunctions.go`
**Status**: Fixed — command now set at create time via `SOCKERLESS_CMD` environment variable

### Details

GCF backend sent `X-Sockerless-Command` header at invoke time to pass the command to the function runtime. Real Cloud Functions don't support custom headers for command passing. Command is now set at function create time via `SOCKERLESS_CMD` env var (base64-encoded JSON), matching the Lambda pattern of setting command at creation rather than invocation.

---

## BUG-049: AZF backend uses non-cloud-native `X-Sockerless-Command` header

**Severity**: Medium
**Component**: `backends/azure-functions/containers.go`, `simulators/azure/functions.go`
**Status**: Fixed — command now set at create time via `SOCKERLESS_CMD` app setting

### Details

Same issue as BUG-048. AZF backend already used `AppCommandLine` for agent callback mode but not for short-lived commands. Command is now set at create time via `SOCKERLESS_CMD` app setting (base64-encoded JSON), and the function is invoked with a plain HTTP POST.

---

## BUG-050: Lambda simulator sets unused `X-Sockerless-Exit-Code` header

**Severity**: Low
**Component**: `simulators/aws/lambda.go`
**Status**: Fixed — removed vestigial header

### Details

Lambda simulator set `X-Sockerless-Exit-Code` response header, but the Lambda backend never reads it (uses `FunctionError` / `X-Amz-Function-Error` from the SDK). Vestigial from a previous naming round.

---

## BUG-051: `SimCommand` field on simulator types is non-cloud-native

**Severity**: Low
**Component**: `simulators/gcp/cloudfunctions.go`, `simulators/azure/functions.go`
**Status**: Fixed — backends now use `SOCKERLESS_CMD` env var/app setting; `SimCommand` retained as backward-compat fallback for SDK tests

### Details

`SimCommand` is explicitly "simulator-only" on types that mirror real cloud APIs. After this fix, backends use `SOCKERLESS_CMD` environment variable (GCF) or app setting (AZF) instead. Simulators read `SOCKERLESS_CMD` first, falling back to `SimCommand` for backward compatibility with SDK tests that set the field directly.

---

## BUG-052: `extractTar` ignores `io.Copy` error — silent file corruption

**Severity**: High
**Component**: `backends/core/handle_containers_archive.go`
**Status**: Fixed — Sprint 8: check error, close file, return error

### Details

`io.Copy(f, tr)` return value was ignored. A truncated tar entry would silently produce a corrupt file on disk. Now checks error, closes the file on failure, and returns the error to the caller.

---

## BUG-053: `handlePutArchive` swallows driver error — returns 200 on failure

**Severity**: High
**Component**: `backends/core/handle_containers_archive.go`
**Status**: Fixed — Sprint 8: return 500 error response when PutArchive fails

### Details

PutArchive error was logged but the handler unconditionally wrote `200 OK`. Clients had no way to know the archive extraction failed. Now returns 500 with error message.

---

## BUG-054: `mergeStagingDir` silently ignores all file copy errors

**Severity**: Medium
**Component**: `backends/core/handle_containers_archive.go`
**Status**: Fixed — Sprint 8: log per-file errors via s.Logger.Warn()

### Details

`os.MkdirAll`, `os.Create`, `io.Copy`, `dst.Chmod` errors were all discarded with `_`. Walk return was ignored. The function is best-effort by design (pre-start staging, shouldn't block container start), but operators had no diagnostics. Now logs warnings per-file.

---

## BUG-055: `createTar` ignores `tw.WriteHeader` and `io.Copy` errors — corrupt tar output

**Severity**: Medium
**Component**: `backends/core/handle_containers_archive.go`
**Status**: Fixed — Sprint 8: changed signature to return error, updated 5 callers

### Details

`tw.WriteHeader(...)` at 3 sites and `io.Copy(tw, f)` at 2 sites all had return values ignored. Changed `createTar` to return `error`. Updated callers in `handle_containers_export.go`, `drivers_agent.go`, `drivers_process.go`, `drivers_synthetic.go`, and `handle_containers_archive.go` (handleGetArchive). HTTP callers that have already written response headers log errors; filesystem drivers propagate errors.

---

## BUG-058: `handleNetworkPrune` doesn't forward `filters` query parameter

**Severity**: High
**Component**: `frontends/docker/networks.go`
**Status**: Fixed — Sprint 8: copy pattern from handleContainerPrune, use postWithQuery

### Details

`s.backend.post(r.Context(), "/networks/prune", nil)` — filters not forwarded to backend. All other prune handlers (container, image, volume) correctly forwarded filters. Now extracts `filters` from query and uses `postWithQuery`.

---

## BUG-059: `handleContainerCommit` ignores JSON decode error on request body

**Severity**: Medium
**Component**: `backends/core/handle_commit.go`
**Status**: Fixed — Sprint 8: check error, return 400 for malformed non-empty body

### Details

`json.NewDecoder(r.Body).Decode(&overrides)` — error was ignored. Malformed JSON body was silently accepted with no overrides applied. Now returns 400 with descriptive message. Empty body (io.EOF) is still valid (no overrides).

---

## BUG-060: `handleImageBuild` ignores `buildargs` JSON unmarshal error

**Severity**: Medium
**Component**: `backends/core/build.go`
**Status**: Fixed — Sprint 8: check error, return 400 with descriptive message

### Details

`_ = json.Unmarshal([]byte(ba), &buildArgs)` — invalid JSON silently dropped all build args, causing builds to proceed without any args. Now returns 400 error.

---

## BUG-061: Agent drivers ignore container-not-found from Store.Get

**Severity**: Low
**Component**: `backends/core/drivers_agent.go`
**Status**: Fixed — Sprint 8: check ok bool, fall through to Fallback driver

### Details

`c, _ := d.Store.Containers.Get(containerID)` — `ok` bool discarded in all 6 callsites (Exec, PutArchive, GetArchive, StatPath, Attach). Zero-value container (empty AgentAddress) silently fell through to the wrong fallback path. Now checks `ok` and delegates to Fallback driver when container is not found.

---

## BUG-062: ECS `startMultiContainerTask` leaks task definition on `runECSTask` failure

**Severity**: Medium
**Component**: `backends/ecs/containers.go`
**Status**: Fixed — Sprint 8: best-effort DeregisterTaskDefinition on error path (2 locations)

### Details

`registerTaskDefinition` succeeds, then `runECSTask` fails — function returned without deregistering the task definition. Orphaned task defs accumulate in ECS. Added best-effort `DeregisterTaskDefinition` on the `runECSTask` error path for both single-container and multi-container flows.

---

## BUG-063: `ExecProcessConfig.Privileged` should be `*bool` with `omitempty`

**Severity**: Low
**Component**: `api/types.go`
**Status**: Fixed — changed to `*bool` with `omitempty`

### Details

Docker API uses `*bool` with `omitempty` for `ExecProcessConfig.Privileged`. Our API used plain `bool`, causing `"privileged":false` to appear in JSON responses instead of being omitted. Fixed by changing the field type to `*bool` with `omitempty` tag.

---

## BUG-064: Cloud backends leave containers stuck "running" when cloud operation fails

**Severity**: High
**Component**: `backends/ecs/containers.go`, `backends/aca/containers.go`, `backends/cloudrun/containers.go`
**Status**: Fixed — added `Store.RevertToCreated()` on all failure paths

### Details

All 3 container backends (ECS, ACA, CloudRun) set container state to "running" (including WaitCh) BEFORE calling the cloud API. If the cloud operation fails, the handler returns an HTTP error but the container remains "running" in the store — it can never exit, stop, or be waited on correctly. Fixed by adding `RevertToCreated()` helper to `Store` and calling it on every cloud operation failure path.

---

## BUG-065: ACA job not cleaned up when `PollUntilDone` fails during start

**Severity**: Medium
**Component**: `backends/aca/containers.go`
**Status**: Fixed — added `s.deleteJob(jobName)` on PollUntilDone failure paths

### Details

`BeginCreateOrUpdate` starts the LRO (Azure may have created the job), then `PollUntilDone` fails. Handler returned error without deleting the orphaned job. Contrast with BeginStart failure which correctly called `s.deleteJob()`. Fixed by adding cleanup on both single and multi-container PollUntilDone failure paths.

---

## BUG-066: CloudRun job not cleaned up when `createOp.Wait` fails during start

**Severity**: Medium
**Component**: `backends/cloudrun/containers.go`
**Status**: Fixed — added `s.deleteJob()` on createOp.Wait failure paths

### Details

`CreateJob` LRO starts, `Wait()` fails. Job may exist in GCP but is never cleaned up. Contrast with RunJob failure which correctly deletes. Fixed by constructing the full job name from parent + jobName and calling `s.deleteJob()` on both single and multi-container Wait failure paths.

---

## BUG-067: GCF function not cleaned up when `op.Wait` fails during create

**Severity**: Medium
**Component**: `backends/cloudrun-functions/containers.go`
**Status**: Fixed — added best-effort function deletion on Wait failure path

### Details

`CreateFunction` LRO starts, `op.Wait()` fails. Function may have been created in GCP but the handler returns error, container isn't stored, and the orphaned function can never be cleaned up. Fixed by calling `DeleteFunction` (best-effort) before returning the error.

---

## BUG-068: AZF Function App not cleaned up when `PollUntilDone` fails during create

**Severity**: Medium
**Component**: `backends/azure-functions/containers.go`
**Status**: Fixed — added best-effort Function App deletion on PollUntilDone failure path

### Details

`BeginCreateOrUpdate` starts the LRO, `PollUntilDone` fails. Function App may exist in Azure but handler returns error, container isn't stored, orphaned app remains. Fixed by calling `WebApps.Delete()` (best-effort) before returning the error.

---

## BUG-069: `handleImageRemove` doesn't delete tag aliases from store

**Severity**: Medium
**Component**: `backends/core/handle_images.go`
**Status**: Fixed — Sprint 10: delete all RepoTags and name-without-tag aliases after deleting by ID

### Details

`Store.Images.Delete(img.ID)` only deletes the image ID key. `StoreImageWithAliases` stores images under up to 6 keys (ID, ref, name-without-tag, docker.io short aliases). Tag entries remained in the store and still resolved to the "deleted" image. Contrast: `handleImagePrune` correctly deleted all aliases. Fixed by copying the alias deletion pattern from prune.

---

## BUG-070: ECS `handleContainerPrune` doesn't clean up cloud resources

**Severity**: High
**Component**: `backends/ecs/extended.go`
**Status**: Fixed — Sprint 10: added task definition deregistration and resource registry cleanup

### Details

Prune only deleted local state (Containers, ContainerNames, ECS store, WaitChs). Did not deregister task definitions or call `MarkCleanedUp`. Contrast: `handleContainerRemove` correctly called `DeregisterTaskDefinition` and `MarkCleanedUp`. Fixed by adding cloud resource cleanup before local state deletion in the prune loop.

---

## BUG-071: FaaS `handleContainerKill` doesn't update container state

**Severity**: High
**Component**: `backends/lambda/containers.go`, `backends/cloudrun-functions/containers.go`, `backends/azure-functions/containers.go`
**Status**: Fixed — Sprint 10: added signal parsing, state transition to "exited", and WaitChs close

### Details

All 3 FaaS kill handlers only disconnected the reverse agent (`AgentRegistry.Remove`) but didn't transition the container to "exited" state or close WaitChs. Container remained "running" indefinitely. Contrast: ECS, ACA, CloudRun kill handlers correctly parsed signal, stopped cloud resource, updated state, and closed WaitChs. Fixed by adding signal parsing (SIGKILL→137), state update, and WaitChs close — minus the cloud stop call since FaaS functions run to completion.

---

## BUG-072: FaaS `handleContainerPrune` doesn't clean up cloud resources

**Severity**: High
**Component**: `backends/lambda/extended.go`, `backends/cloudrun-functions/extended.go`, `backends/azure-functions/extended.go`
**Status**: Fixed — Sprint 10: added cloud function deletion and resource registry cleanup

### Details

All 3 FaaS prune handlers only deleted local state. Did not delete cloud functions or call `MarkCleanedUp`. Contrast: each backend's `handleContainerRemove` correctly deleted the cloud function and called `MarkCleanedUp`. Fixed by adding cloud resource cleanup (Lambda `DeleteFunction`, GCF `DeleteFunction`, AZF `WebApps.Delete`) and `MarkCleanedUp` to each prune handler.

---

## BUG-073: FaaS prune and remove don't clean up LogBuffers

**Severity**: Medium
**Component**: `backends/lambda/containers.go`, `backends/lambda/extended.go`, `backends/cloudrun-functions/containers.go`, `backends/cloudrun-functions/extended.go`, `backends/azure-functions/containers.go`, `backends/azure-functions/extended.go`
**Status**: Fixed — Sprint 10: added `LogBuffers.Delete(c.ID)` to all 6 locations

### Details

FaaS backends store function output in `LogBuffers` during invocation, but neither prune nor remove deleted these entries. Memory leak — log buffers accumulated indefinitely. Contrast: core's `handleContainerPrune` and `handleContainerRemove` both call `LogBuffers.Delete(c.ID)`. Fixed by adding `LogBuffers.Delete` to all 3 FaaS backends' prune and remove handlers.

---

## BUG-074: Docker backend `mapContainerFromDocker` doesn't populate Mounts

**Severity**: High
**Component**: `backends/docker/containers.go`
**Status**: Fixed — Sprint 10: map `info.Mounts` to `api.MountPoint` slice

### Details

`c.Mounts = make([]api.MountPoint, 0)` — initialized empty but never populated from `info.Mounts`. Container inspect always returned empty Mounts array, even for containers with volumes or bind mounts. Fixed by iterating `info.Mounts` and mapping Type, Name, Source, Destination, Driver, Mode, RW, and Propagation fields.

---

## BUG-075: Lambda missing restart handler override

**Severity**: High
**Component**: `backends/lambda/server.go`, `backends/lambda/extended.go`
**Status**: Fixed — Sprint 11: added no-op restart handler matching GCF/AZF pattern

### Details

Lambda didn't override `handleContainerRestart`, inheriting core's restart logic which calls `ProcessLifecycle.Stop()`, `ForceStopContainer()`, `ProcessLifecycle.Cleanup()`, `ProcessLifecycle.Start()` — attempting to re-invoke a Lambda function via process lifecycle. CloudRun Functions and Azure Functions both had explicit no-op restart handlers. Fixed by adding the same pattern to Lambda.

---

## BUG-076: Docker `mapContainerFromDocker` missing HostConfig fields

**Severity**: High
**Component**: `backends/docker/containers.go`
**Status**: Fixed — Sprint 11: map all 14 missing HostConfig fields

### Details

Only mapped 3 of 17 `api.HostConfig` fields: `NetworkMode`, `Binds`, `AutoRemove`. Missing: `PortBindings`, `RestartPolicy`, `Privileged`, `CapAdd`, `CapDrop`, `Init`, `UsernsMode`, `ShmSize`, `Tmpfs`, `SecurityOpt`, `LogConfig`, `ExtraHosts`, `Mounts`, `Isolation`. Fixed by mapping all fields with appropriate type conversions (e.g., `nat.PortMap` → `map[string][]PortBinding`).

---

## BUG-077: Docker `mapContainerFromDocker` missing Config fields

**Severity**: Medium
**Component**: `backends/docker/containers.go`
**Status**: Fixed — Sprint 11: map ExposedPorts, Volumes, Shell, Healthcheck, StopTimeout

### Details

Mapped 14 of 19 `api.ContainerConfig` fields. Missing: `ExposedPorts`, `Volumes`, `Healthcheck`, `Shell`, `StopTimeout`. Fixed by mapping all 5 fields with `nat.PortSet` → `map[string]struct{}` conversion for ExposedPorts and `container.HealthConfig` → `api.HealthcheckConfig` conversion for Healthcheck.

---

## BUG-078: Docker `mapContainerFromDocker` missing State.Health

**Severity**: High
**Component**: `backends/docker/containers.go`
**Status**: Fixed — Sprint 11: map info.State.Health to api.HealthState

### Details

Mapped all `ContainerState` fields except `Health`. Our `api.ContainerState` has `Health *HealthState` but it was never populated from `info.State.Health`. Fixed by mapping Status, FailingStreak, and Log entries with `time.RFC3339Nano` formatting.

---

## BUG-079: Docker `mapContainerFromDocker` missing NetworkSettings.Ports

**Severity**: High
**Component**: `backends/docker/containers.go`
**Status**: Fixed — Sprint 11: map info.NetworkSettings.Ports to api.NetworkSettings.Ports

### Details

Mapped `Networks` but not `Ports`. Our `api.NetworkSettings` has `Ports map[string][]PortBinding` but it was never populated. Fixed by reusing `mapPortBindings` helper to convert `nat.PortMap` to `map[string][]api.PortBinding`.

---

## BUG-080: Docker `handleContainerList` missing Ports, Mounts, NetworkSettings

**Severity**: High
**Component**: `backends/docker/containers.go`
**Status**: Fixed — Sprint 11: map Ports, Mounts, SizeRw, NetworkSettings in list response

### Details

`ContainerSummary` only mapped 9 of 13 fields. Missing: `Ports`, `Mounts`, `SizeRw`, `NetworkSettings`. Fixed by mapping `c.Ports` → `[]api.Port`, `c.Mounts` → `[]api.MountPoint` (via `mapMountsFromSummary` helper), `c.SizeRw`, and `c.NetworkSettings` → `*api.SummaryNetworkSettings`.

---

## BUG-081: Docker network inspect and list missing IPAM and Containers

**Severity**: High
**Component**: `backends/docker/networks.go`
**Status**: Fixed — Sprint 11: map IPAM and Containers in both list and inspect

### Details

Network mapping skipped IPAM and Containers fields. Both exist in `api.Network`. Fixed by adding `mapNetworkIPAMAndContainers` helper that maps `network.IPAM` → `api.IPAM` (Driver, Config, Options) and `map[string]network.EndpointResource` → `map[string]api.EndpointResource`.

---

## BUG-082: Docker `handleImageInspect` missing Config fields

**Severity**: Medium
**Component**: `backends/docker/images.go`
**Status**: Fixed — Sprint 11: map all remaining ContainerConfig fields

### Details

Only mapped 5 of 19 `api.ContainerConfig` fields for image config: `Env`, `Cmd`, `Entrypoint`, `WorkingDir`, `Labels`. Fixed by mapping all remaining fields including `ExposedPorts`, `Volumes`, `Healthcheck`, `StopSignal`, `User`, `Hostname`, `Domainname`, `Tty`, `OpenStdin`, `StdinOnce`, `AttachStdin`, `AttachStdout`, `AttachStderr`, `Shell`, `StopTimeout`.
