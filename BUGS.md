# Known Bugs

667 total. 667 fixed. 0 open.

| ID | Sev | Summary | Status |
|----|-----|---------|--------|
| 662 | High | Auto-agent delegates to BaseServer.ContainerStart which reads Store. | fixed |
| 663 | High | docker wait hangs in auto-agent mode. Fix: check local WaitChs first. | fixed |
| 664 | High | CloudState only queried RUNNING tasks. Fix: query RUNNING + STOPPED. | fixed |
| 665 | Med | docker logs 404 after restart — task ID from local state only. Fix: query cloud for task ARN by container ID tag when local state missing. | fixed |
| 666 | Low | docker run -d blocks ~30s on ECS provisioning. Fix: async wait for RUNNING — return immediately after RunTask, poll in background. | fixed |
| 667 | High | Lambda backend missing CloudStateProvider — docker ps, inspect, stop all broken after PendingCreates.Delete. Fix: implement lambdaCloudState querying ListFunctions + tags. | fixed |
