# Known Bugs

693 total. 691 fixed. 2 open (P86 AWS manual session 1, 2026-04-19).

| ID | Sev | Summary | Status |
|----|-----|---------|--------|
| 693 | High | ECS task definition registered with unqualified image ref (e.g. `alpine`) — Fargate cannot pull, task stays PENDING forever. `backends/ecs/taskdef.go:buildContainerDef` uses `config.Image` raw; should resolve via ECR pull-through URI like `backends/lambda/image_resolve.go:resolveImageURI`. | open |
| 692 | Critical | `docker run` hangs after POST /containers/create against live ECS backend — no POST /start from docker CLI. Backend attach returns 200 in 0.14ms instead of holding a hijacked connection. Blocks Phase-86 AWS-track runner validation. Likely regression from stateless refactor. | open |
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
