# Known Bugs

665 total (663 fixed, 2 open).

| ID | Sev | Summary | Status |
|----|-----|---------|--------|
| 662 | High | Auto-agent delegates to BaseServer.ContainerStart which reads Store. Fix: move PendingCreates→Store before delegating. | fixed |
| 663 | High | docker wait hangs in auto-agent mode — CloudState polls simulator which has no local task. Fix: check local WaitChs first. | fixed |
| 664 | High | CloudState.queryTasks only queried RUNNING tasks. Short-lived containers disappeared after exit. Fix: query both RUNNING and STOPPED. | fixed |
| 665 | Med | docker logs returns 404 for containers found via CloudState. The logs handler needs the ECS task ID for the CloudWatch log stream name, but after backend restart the ECS StateStore is empty. The task ID should be derived from the DescribeTasks result in CloudState, not from local state. | open |
| 666 | Low | docker run -d takes ~30s on real ECS (ECS task provisioning time). Not a bug per se — real cloud latency. But could benefit from async start that returns ID immediately and polls for RUNNING in background. | open |
