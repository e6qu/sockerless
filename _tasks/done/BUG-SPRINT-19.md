# Bug Sprint 19 — BUG-139→157

**Date:** 2026-03-04
**Bugs Fixed:** 19 (BUG-139 through BUG-157)
**Focus:** Core lifecycle (stop/restart/start/exec), cloud AgentRegistry leaks, Docker exec detach, frontend attach

## Bugs Fixed

| Bug | Sev | OB | Component | Fix |
|-----|-----|----|-----------|-----|
| BUG-139 | High | OB-069 | Core | handleContainerStop: add StopHealthCheck before Stop |
| BUG-140 | Med | OB-038 | Core | handleContainerStop: emit "die" event before "stop" |
| BUG-141 | Med | OB-039 | Core | handleContainerStop: call ProcessLifecycle.Cleanup |
| BUG-142 | High | OB-032 | Core | handleContainerRestart: revert to created + return error on Start failure |
| BUG-143 | Med | OB-036 | Core | handleContainerStart: guard env appends with hasEnvKey to prevent duplicates |
| BUG-144 | High | OB-070 | Core | handleContainerStart: move exitCh/state-update after Start() succeeds |
| BUG-145 | High | OB-033 | Core | Synthetic auto-stop: check container still running before StopContainer |
| BUG-146 | High | OB-001 | Core | restart_policy: call RevertToCreated on Start failure |
| BUG-147 | High | OB-071 | Core | restart_policy: StopHealthCheck before cleanup, StartHealthCheck after Start |
| BUG-148 | Med | OB-043 | Core | restart_policy: re-fetch container after state update for fresh config |
| BUG-149 | Med | OB-044 | Core | restart_policy: close old WaitCh before creating new one |
| BUG-150 | Med | OB-037 | Core | handleExecStart: revert exec Running state on Hijack failure |
| BUG-151 | High | OB-034 | Cloud | Multi-pod: revert ALL pod containers on error (not just trigger) |
| BUG-152 | High | OB-035 | Cloud | Multi-pod: call AgentRegistry.Remove on error paths |
| BUG-153 | Med | OB-083 | ECS | Single-container: AgentRegistry.Remove in waitForTaskRunning error path |
| BUG-154 | Med | OB-084 | CloudRun | Single-container: AgentRegistry.Remove in waitForExecutionRunning error path |
| BUG-155 | Med | OB-085 | ACA | Single-container: AgentRegistry.Remove in waitForExecutionRunning error path |
| BUG-156 | High | OB-075 | Docker | handleExecStart: early return for Detach=true (ContainerExecStart, no hijack) |
| BUG-157 | High | OB-076 | Frontend | handleContainerAttach: append query.Encode() to dialUpgrade URL |

## Also

- Removed OB-090 (false positive: Docker API uses PascalCase for Event fields)
- Added False Positives section to BUGS.md

## Files Modified

- `backends/core/handle_containers.go` (BUG-139→145)
- `backends/core/restart_policy.go` (BUG-146→149)
- `backends/core/handle_exec.go` (BUG-150)
- `backends/ecs/containers.go` (BUG-151,152,153)
- `backends/cloudrun/containers.go` (BUG-151,152,154)
- `backends/aca/containers.go` (BUG-151,152,155)
- `backends/docker/exec.go` (BUG-156)
- `frontends/docker/containers_stream.go` (BUG-157)
- `BUGS.md`

## Verification

- Core tests: 302 PASS (race detector)
- All 7 modules build clean
- Lint: 0 issues
