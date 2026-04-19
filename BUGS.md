# Known Bugs

698 total. 693 fixed. 5 open (P86 bug-fix sprint, 2026-04-19).

| ID | Sev | Summary | Status |
|----|-----|---------|--------|
| 698 | Critical | Docker CLI (`docker run` / `docker run -d`) hangs against ECS backend between POST /containers/create and POST /start — `/start` is never sent. Backend responds correctly to all direct curl calls (create → start → attach → wait → delete all work end-to-end). docker CLI prints the container ID from create's response body, then blocks indefinitely. Suspected cause: something in sockerless's create-response body, /version, or /_ping response surface that docker CLI's auto-pull / negotiation logic dislikes. Root cause not yet isolated. Blocks all docker-CLI-driven runner validation. | open |
| 697 | Med | sockerless image store doesn't persist `docker pull` state across backend restarts — after restart the pulled image is gone, so `docker run <img>` can't find it in the store. Test-harness gap: session 1 saw the image pre-pulled from a prior run, masking this. | open |
| 696 | Med | AWS simulator missing ECR pull-through-cache APIs (`CreatePullThroughCacheRule`, `DescribePullThroughCacheRules`) — returns `UnknownOperationException`. ECS backend degrades to raw image ref on simulator, which hides the live-mode behavior. Simulator parity gap per user directive: simulators must fully support every cloud action that runners drive through sockerless. | open |
| 695 | High | `StreamCloudLogs` rejects containers in `created` state unconditionally — breaks `docker run` create→attach→start flow where attach opens before start. Fixed with new `AllowCreated` option in `StreamCloudLogsOptions`; ECS `ContainerAttach` passes it. | fixed |
| 694 | High | `StreamCloudLogs` follow loop exits as soon as `!c.State.Running`, but `created` state is also non-running — attach before start exits the stream on first tick. Fixed by switching exit condition to `isTerminalState(status)` which fires only on `exited`/`dead`/`removing`. | fixed |
| 693 | High | ECS task definition registered with unqualified image ref (e.g. `alpine`) — Fargate cannot pull, task stays PENDING forever. `backends/ecs/taskdef.go:buildContainerDef` used `config.Image` raw; fixed in P86-015 by porting Lambda's `resolveImageURI` to ECS. | fixed |
| 692 | Critical | `docker run` hangs after POST /containers/create against live ECS backend. Root cause: `ContainerAttach` delegated to `BaseServer.ContainerAttach` which returned an EOF pipe because `LocalStreamDriver` had no local driver. Fixed in P86-016 by implementing ECS-specific attach that streams CloudWatch logs. | fixed |
| 662 | High | Auto-agent delegates to BaseServer.ContainerStart which reads Store. | fixed |
| 663 | High | docker wait hangs in auto-agent mode. Fix: check local WaitChs first. | fixed |
| 664 | High | CloudState only queried RUNNING tasks. Fix: query RUNNING + STOPPED. | fixed |
| 665 | Med | docker logs 404 after restart — task ID from local state only. Fix: query cloud for task ARN by container ID tag when local state missing. | fixed |
| 666 | Low | docker run -d blocks ~30s on ECS provisioning. Fix: async wait for RUNNING — return immediately after RunTask, poll in background. | fixed |
| 667 | High | Lambda backend missing CloudStateProvider — docker ps, inspect, stop all broken after PendingCreates.Delete. Fix: implement lambdaCloudState querying ListFunctions + tags. | fixed |
| 668 | High | StreamCloudLogs uses Store.ResolveContainerID + Store.Containers.Get — fails in stateless mode (docker logs 404 on all cloud backends). Fix: use ResolveContainerAuto. | fixed |
| 669 | High | ECS ContainerLogs falls back to BaseServer.ContainerLogs when taskID unknown — stateless violation. Fix: return clear error instead of BaseServer delegation. | fixed |
| 670 | High | BaseServer.ContainerInspect uses Store.ResolveContainer — fails in stateless mode when called via delegates. Fix: use ResolveContainerAuto. | fixed |
| 671 | High | BaseServer.ContainerList uses Store.Containers.List — returns empty on cloud backends. Fix: use CloudState.ListContainers when available. | fixed |
| 672 | High | BaseServer.ContainerTop uses Store.ResolveContainerID + Store.Containers.Get — fails in stateless mode. Fix: use ResolveContainerAuto. | fixed |
| 673 | High | BaseServer.ContainerUpdate uses Store.ResolveContainerID + Store.Containers.Update — fails in stateless mode. Fix: use ResolveContainerIDAuto. | fixed |
| 674 | High | BaseServer.ContainerStart/Stop/Kill/Remove/Restart/Logs/Wait/Attach/Stats/Rename/Pause/Unpause all use Store.ResolveContainerID — fail in stateless mode. Fix: use ResolveContainerAuto. | fixed |
| 675 | High | BaseServer.ExecCreate uses Store.ResolveContainerID + Store.Containers.Get — fails in stateless mode. Fix: use ResolveContainerAuto. | fixed |
| 676 | High | BaseServer.NetworkConnect/Disconnect use Store.ResolveContainerID. Fix: use ResolveContainerIDAuto. | fixed |
| 677 | Med | CloudRun ContainerTop both branches delegate identically to BaseServer.ContainerTop. Fix: return NotImplemented when no agent connected. | fixed |
| 678 | Med | CloudRun ContainerUpdate delegates to BaseServer without resolving container first. Fix: add ResolveContainerAuto check. | fixed |
| 679 | Med | StreamCloudLogs follow-mode checks Store.Containers.Get for running status — fails in stateless mode. Fix: use ResolveContainerAuto. | fixed |
| 680 | Med | Handler files (handle_containers_query, handle_containers, handle_extended, handle_exec, handle_libpod) use Store.ResolveContainerID/ResolveContainer directly. Fix: use ResolveContainerAuto/ResolveContainerIDAuto. | fixed |
| 681 | High | CloudRun/ACA/GCF/AZF CloudState implementations were stubs reading Store.Containers instead of querying cloud APIs. Fix: implement real cloud API queries (ListJobs, ListFunctions, etc.). | fixed |
| 682 | High | GCP label value limit (63 chars) truncates 64-char container IDs — CloudState can't match containers. Fix: store full ID in GCP annotations; read from env var for GCF. | fixed |
| 683 | High | Auto-agent (SpawnAutoAgent/StopAutoAgent) used in ECS, CloudRun, ACA backends — stateless violation (spawns local processes, reads Store.Containers). Fix: remove from all cloud backends. | fixed |
| 684 | High | GCP simulator missing LatestCreatedExecution on Job — CloudState can't determine execution state. Fix: add ExecutionReference, set on RunJob, update CompletionTime on finish. | fixed |
| 685 | High | Azure simulator missing SystemData on ContainerAppJob — CloudState can't read creation time. Fix: add SystemData struct, populate on job creation. | fixed |
| 686 | High | Simulators use os/exec for workloads instead of real containers — exec, archive, and filesystem operations impossible. Fix: migrate to Docker SDK container execution. | fixed |
| 687 | High | `docker logs` returns empty for CloudRun containers — simulator writes to Cloud Logging but backend log query doesn't find entries. Root cause: simulator tried to pull cloud registry URI (AR) locally. Fix: ResolveLocalImage maps AR/ECR/ACR URIs back to Docker Hub. | fixed |
| 688 | High | `docker ps` shows running CloudRun container as not running — GCP simulator missing GET /executions/{id} endpoint, CloudState couldn't fetch execution state. Fix: add GET execution endpoint. | fixed |
| 689 | Med | `docker logs` for short-lived containers may miss output — container exits before log sink flushes. Fix: wait for log drain channel before returning ProcessResult from waitAndCaptureLogs. | fixed |
| 690 | Med | Smoke test `docker stop` returns 304 for running containers — same root cause as BUG-688 (execution state not fetched). Fixed by GET execution endpoint. | fixed |
| 691 | Med | Smoke test long-running container shows empty `docker ps` — same root cause as BUG-688. Fixed by GET execution endpoint. | fixed |
