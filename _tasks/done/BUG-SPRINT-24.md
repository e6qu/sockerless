# Bug Sprint 24 — Final 18 Bugs (BUG-252 → BUG-269)

**Completed**: 2026-03-04

## Summary

Fixed the final 18 open bugs (2 medium, 16 low), closing all known issues.

## Bugs Fixed

| Bug | OB | Component | Fix |
|-----|-----|-----------|-----|
| BUG-252 | OB-018+113 | Docker+API | Added BuildCache type and field to DiskUsageResponse; Docker backend maps du.BuildCache |
| BUG-253 | OB-056 | Lambda/GCF/AZF | FaaS image pull now calls FetchImageConfig to get real CMD/ENV/Entrypoint |
| BUG-254 | OB-060 | Core | Added events for image tag/untag/delete and volume create/destroy |
| BUG-255 | OB-063 | Core | Container update now copies Memory/CPU/BlkioWeight resource fields |
| BUG-256 | OB-064 | Core | Image load parses tar manifest.json for RepoTags and config JSON for Env/Cmd/etc |
| BUG-257 | OB-066 | Lambda/GCF/AZF | FaaS logs checks LogBuffers before cloud log query |
| BUG-258 | OB-068a | API | NetworkSettings: added Gateway, IPAddress, IPPrefixLen, MacAddress |
| BUG-259 | OB-068b | API | HostConfig: added Dns/DnsSearch/DnsOptions, Memory/MemorySwap/MemoryReservation, CpuShares/CpuQuota/CpuPeriod/CpusetCpus/CpusetMems, BlkioWeight, PidMode/IpcMode/UTSMode |
| BUG-260 | OB-068c | API | Image.GraphDriver (new GraphDriverData type), ExecInstance.DetachKeys |
| BUG-261 | OB-103 | ECS | pollTaskExit checks c.State.Running before StopContainer |
| BUG-262 | OB-104 | ECS/CloudRun/ACA | All pollers check c.State.Running before StopContainer |
| BUG-263 | OB-105 | ACA/CloudRun | stopExecution/cancelExecution/deleteJob now wait via PollUntilDone/Wait |
| BUG-264 | OB-108 | Lambda | No-command auto-stop path checks c.State.Running |
| BUG-265 | OB-116 | Frontend | handleImageBuild forwards 16+ additional query params |
| BUG-266 | OB-117 | Frontend | handleContainerCommit forwards pause and changes params |
| BUG-267 | OB-068d | API | ContainerUpdateRequest: added all resource fields |
| BUG-268 | OB-068e | API | EndpointSettings already had IPv6Gateway — confirmed present |
| BUG-269 | OB-068f | API | HealthcheckConfig: added StartInterval |

## Files Modified

- `api/types.go` — BUG-252, 258-260, 267-269
- `backends/core/handle_extended.go` — BUG-252, 255
- `backends/core/handle_images.go` — BUG-254, 256
- `backends/core/handle_volumes.go` — BUG-254
- `backends/docker/extended.go` — BUG-252
- `backends/lambda/images.go` — BUG-253
- `backends/cloudrun-functions/images.go` — BUG-253
- `backends/azure-functions/images.go` — BUG-253
- `backends/lambda/logs.go` — BUG-257
- `backends/cloudrun-functions/logs.go` — BUG-257
- `backends/azure-functions/logs.go` — BUG-257
- `backends/ecs/containers.go` — BUG-261, 262
- `backends/cloudrun/containers.go` — BUG-262, 263
- `backends/aca/containers.go` — BUG-262, 263
- `backends/lambda/containers.go` — BUG-264
- `frontends/docker/images.go` — BUG-265, 266
- `BUGS.md` — all

## Verification

- `cd backends/core && go test -race -count=1 ./...` — PASS
- All 8 backends + frontend: `go build ./...` — OK
- `make lint` — 0 issues across all 19 modules
