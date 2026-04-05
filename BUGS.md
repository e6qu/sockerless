# Known Bugs

663 total (662 fixed, 1 open).

| ID | Sev | Summary | Status |
|----|-----|---------|--------|
| 662 | High | Auto-agent path delegates to BaseServer.ContainerStart which reads Store.Containers, but stateless backends put containers in PendingCreates. Smoke/e2e fail with 404. Fix: move from PendingCreates→Store before delegating. Affects ECS, CloudRun, ACA. | fixed |
| 663 | High | In auto-agent mode, docker wait hangs forever. CloudState.WaitForExit polls the simulator's DescribeTasks but auto-agent containers run locally (not via simulator ECS). The simulator has no task for the container so WaitForExit never returns. Fix: handleContainerWait should use Store.WaitChs when container is in Store.Containers (auto-agent), and CloudState.WaitForExit when it's a real cloud container. | open |
