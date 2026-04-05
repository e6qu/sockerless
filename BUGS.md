# Known Bugs

663 total (663 fixed, 0 open).

| ID | Sev | Summary | Status |
|----|-----|---------|--------|
| 662 | High | Auto-agent path delegates to BaseServer.ContainerStart which reads Store.Containers, but stateless backends put containers in PendingCreates. Smoke/e2e fail with 404. Fix: move from PendingCreates→Store before delegating. Affects ECS, CloudRun, ACA. | fixed |
| 663 | High | In auto-agent mode, docker wait hangs forever. CloudState.WaitForExit polls simulator but auto-agent runs locally. Fix: check local WaitChs first, fall back to CloudState polling. | fixed |
