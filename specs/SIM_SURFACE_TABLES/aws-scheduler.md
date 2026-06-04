# Sim surface — aws-scheduler

Surface registered in `simulators/aws/scheduler.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /schedules/{Name}` | ✓ `simulators/aws/scheduler.go:59::handleSchedulerCreateSchedule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /schedules/{Name}` | ✓ `simulators/aws/scheduler.go:60::handleSchedulerGetSchedule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PUT /schedules/{Name}` | ✓ `simulators/aws/scheduler.go:61::handleSchedulerUpdateSchedule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /schedules/{Name}` | ✓ `simulators/aws/scheduler.go:62::handleSchedulerDeleteSchedule` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /schedules` | ✓ `simulators/aws/scheduler.go:63::handleSchedulerListSchedules` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /schedule-groups/{Name}` | ✓ `simulators/aws/scheduler.go:65::handleSchedulerCreateScheduleGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /schedule-groups/{Name}` | ✓ `simulators/aws/scheduler.go:66::handleSchedulerGetScheduleGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /schedule-groups/{Name}` | ✓ `simulators/aws/scheduler.go:67::handleSchedulerDeleteScheduleGroup` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /schedule-groups` | ✓ `simulators/aws/scheduler.go:68::handleSchedulerListScheduleGroups` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
<!-- HAND-WRITTEN END -->
