# Known Bugs

662 total (662 fixed, 0 open).

| ID | Sev | Summary | Status |
|----|-----|---------|--------|
| 662 | High | Auto-agent path delegates to BaseServer.ContainerStart which reads Store.Containers, but stateless backends put containers in PendingCreates. Smoke/e2e fail with 404. Fix: move from PendingCreates→Store before delegating. Affects ECS, CloudRun, ACA. | fixed |
