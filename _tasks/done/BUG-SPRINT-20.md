# Bug Sprint 20 — BUG-158→176

**Date**: 2026-03-04
**Bugs fixed**: 19 (BUG-158 through BUG-176)

## Summary

Core lifecycle events, cloud restart parity, AgentRegistry leak, API types.

## Bugs

| Bug | Component | Fix |
|-----|-----------|-----|
| BUG-158 | Core | handleContainerKill: added "die" event after kill |
| BUG-159 | Core | handleContainerKill: added ProcessLifecycle.Cleanup call |
| BUG-160 | Core | handleContainerStop: added "die" event with exitCode before "stop" |
| BUG-161 | Core | handleContainerStart synthetic goroutine: added container-removed guard |
| BUG-162 | Core | WaitForAgent: clean up ready map entry on timeout |
| BUG-163 | ECS | Restart: added StopHealthCheck |
| BUG-164 | ECS | Restart: added "die" event |
| BUG-165 | CloudRun | Restart: added StopHealthCheck |
| BUG-166 | CloudRun | Restart: added "die" event |
| BUG-167 | ACA | Restart: added StopHealthCheck |
| BUG-168 | ACA | Restart: added "die" event |
| BUG-169 | Lambda | Restart: added StopHealthCheck + "die" event |
| BUG-170 | Azure Functions | Restart: added StopHealthCheck + "die" event |
| BUG-171 | CloudRun Functions | Restart: added StopHealthCheck + "die" event |
| BUG-172 | API | NetworkSettings: added Bridge, SandboxID, HairpinMode, LinkLocalIPv6Address fields |
| BUG-173 | API | EndpointSettings: added IPv6Gateway, GlobalIPv6Address, GlobalIPv6PrefixLen, IPAMConfig, DriverOpts |
| BUG-174 | API | Port.IP: removed omitempty (Docker always returns IP field) |
| BUG-175 | API | ContainerConfig: added ArgsEscaped, MacAddress, OnBuild |
| BUG-176 | Core | handleContainerRestart: added missing "stop" event after "die" |

## Files Modified

- `backends/core/handle_containers.go` — BUG-158,159,160,161,176
- `backends/core/agent_registry.go` — BUG-162
- `backends/core/event_bus.go` — Added exported EmitEvent for cloud backends
- `backends/ecs/extended.go` — BUG-163,164
- `backends/cloudrun/extended.go` — BUG-165,166
- `backends/aca/extended.go` — BUG-167,168
- `backends/lambda/extended.go` — BUG-169
- `backends/azure-functions/extended.go` — BUG-170
- `backends/cloudrun-functions/extended.go` — BUG-171
- `api/types.go` — BUG-172,173,174,175

## Verification

- `backends/core` tests: 302 PASS (race)
- All 6 cloud backends: build OK
- `backends/docker`, `frontends/docker`: build OK
- `make lint`: 0 issues
