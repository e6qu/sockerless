# Bug Sprint 21 — Resource Leaks, Cloud Parity, Docker Field Mapping, Lifecycle Safety

**Date**: 2026-03-04
**Bugs**: BUG-177 → BUG-201 (25 bugs, 1 false positive)

## Summary

| Bug | OB | Component | Fix |
|-----|-----|-----------|-----|
| BUG-177 | OB-057 | API | False positive — EndpointIPAMConfig already exists |
| BUG-178 | OB-002 | Docker | Map IPAMConfig + IPv6 + DriverOpts in container create NetworkingConfig |
| BUG-179 | OB-003 | Docker | Map IPAMConfig + IPv6 + DriverOpts in network connect |
| BUG-180 | OB-004 | Core | Move pod validation before container store in create |
| BUG-181 | OB-010 | Core | Add pod registry cleanup in container remove and prune |
| BUG-182 | OB-079 | Core | Close WaitChs (not just delete) in force pod remove |
| BUG-183 | OB-080 | Core | RevertToCreated deletes WaitCh without closing |
| BUG-184 | OB-082 | Core | Derive gateway from subnet when empty |
| BUG-185 | OB-078 | Core | Add container-in-use check before image remove (409) |
| BUG-186 | OB-022 | Cloud | Change StopContainer to ForceStopContainer in all 6 cloud remove handlers |
| BUG-187 | OB-020 | Cloud | Add Registry.MarkCleanedUp in ECS/CloudRun/ACA restart handlers |
| BUG-188 | OB-052 | Cloud | Add Registry.MarkCleanedUp after CloudRun deleteJob on re-start |
| BUG-189 | OB-053 | Cloud | Stop leaked ECS task on waitForTaskRunning failure |
| BUG-190 | OB-054 | Cloud | Add nil guards in ECS ENI IP extraction |
| BUG-191 | OB-087 | Cloud | Clear stale ACA state on restart (ACA.Delete) |
| BUG-192 | OB-030 | Cloud | Guard ~22 goroutine StopContainer calls with existence check |
| BUG-193 | OB-058/031 | API+Docker | Add VolumeOptions/TmpfsOptions types + Docker mount mapping |
| BUG-194 | OB-088 | Docker | Attach Content-Type raw-stream for TTY containers |
| BUG-195 | OB-089 | Docker | Exec Content-Type raw-stream for TTY execs |
| BUG-196 | OB-091 | API+Core | Add HostConfigSummary type + set in container list |
| BUG-197 | OB-042 | Core | Add resolveTmpfsMounts in restart handler |
| BUG-198 | OB-041 | Core | Stop/start health checks on pause/unpause |
| BUG-199 | OB-072 | Core | Atomic rename with RenameMu mutex |
| BUG-200 | OB-024 | Core | Capture time.Now() once in emitEvent |
| BUG-201 | OB-096 | Core | Emit die event in synthetic/exec auto-stop paths |

## Files Modified (25)

- `api/types.go` — BUG-193, 196
- `backends/core/handle_containers.go` — BUG-180, 181, 197, 201
- `backends/core/handle_extended.go` — BUG-181, 198, 199
- `backends/core/handle_containers_query.go` — BUG-196
- `backends/core/handle_images.go` — BUG-185
- `backends/core/handle_pods.go` — BUG-182
- `backends/core/handle_exec.go` — BUG-201
- `backends/core/store.go` — BUG-183, 199
- `backends/core/pod.go` — BUG-181
- `backends/core/ipam.go` — BUG-184
- `backends/core/event_bus.go` — BUG-200
- `backends/docker/containers.go` — BUG-178, 193, 194
- `backends/docker/extended.go` — BUG-179
- `backends/docker/exec.go` — BUG-195
- `backends/ecs/containers.go` — BUG-186, 189, 192
- `backends/ecs/extended.go` — BUG-187
- `backends/ecs/eni.go` — BUG-190
- `backends/cloudrun/containers.go` — BUG-186, 188, 192
- `backends/cloudrun/extended.go` — BUG-187
- `backends/aca/containers.go` — BUG-186, 192
- `backends/aca/extended.go` — BUG-187, 191
- `backends/lambda/containers.go` — BUG-186, 192
- `backends/azure-functions/containers.go` — BUG-186, 192
- `backends/cloudrun-functions/containers.go` — BUG-186, 192
- `BUGS.md` — All

## Verification

- `backends/core`: go test -race — PASS
- All 6 cloud backends + docker + frontend: go build — OK
- `make lint` — 0 issues
