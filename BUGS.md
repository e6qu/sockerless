# Known Bugs

680 total. 680 fixed. 0 open.

| ID | Sev | Summary | Status |
|----|-----|---------|--------|
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
