# Known Bugs

583 bugs fixed across 45 sprints (BUG-001 through BUG-583). 0 open bugs. See [STATUS.md](STATUS.md) for overall project status.

Per-sprint details in `_tasks/done/BUG-SPRINT-*.md`.

## False Positives

Bugs investigated and confirmed as non-issues.

| ID | Reason |
|----|--------|
| FP-001 | Docker API uses PascalCase for Event fields — current tags are correct |
| FP-002 | EndpointIPAMConfig already exists (added in BUG-175) |
| FP-003 | Changing Container.State/Config/HostConfig to pointers would break too much code |
| FP-004 | Docker emits `kill` then `die` — current order is correct |
| FP-005 | No real filesystem change tracking (known limitation) |
| FP-006 | Stats network stats always zero (known limitation) |
| FP-007 | Missing Ulimits/DeviceRequests/Devices — lower priority |
| FP-008 | FaaS stop doesn't cancel invocations — by design |
| FP-009 | FaaS validation order is correct |
| FP-010 | Docker ContainerUpdate direct pass-through is correct |
| FP-011 | Docker ExecInspect ProcessConfig doesn't include env/workingDir |
| FP-012 | Already fixed in Sprint 43 (BUG-541) |
| FP-013 | Already fixed in Sprint 43 (BUG-542) |
| FP-014 | ConsoleSize matches Docker SDK type |
| FP-015 | Cloud backends don't support pause semantics |
| FP-016 | Cloud backends use registries, not image load |
