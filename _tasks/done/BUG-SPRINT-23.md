# Bug Sprint 23 — BUG-227 through BUG-251

**Date**: 2026-03-04
**Bugs fixed**: 25
**Focus**: Forward agent fix (CloudRun/ACA), Docker parity, lifecycle safety

## Bugs Fixed

| Bug | OB | Sev | Component | Issue |
|-----|-----|-----|-----------|-------|
| BUG-227 | OB-073 | HIGH | CloudRun | waitForExecutionRunning returns GCP resource path as agent address — forward agent always fails |
| BUG-228 | OB-074 | HIGH | ACA | waitForExecutionRunning returns execution name as agent address — forward agent always fails |
| BUG-229 | OB-055 | Med | Lambda/GCF/AZF | FaaS kill + background goroutine race — goroutine overwrites kill exit code |
| BUG-230 | OB-086 | Med | Lambda/GCF | FaaS create stores container after Registry.Register — orphan cloud resource on interrupt |
| BUG-231 | OB-013 | Med | Core | Image tag doesn't update old alias store entries |
| BUG-232 | OB-014 | Med | Core | tmpfs temp dirs never cleaned up |
| BUG-233 | OB-077 | Med | Core | BuildContexts entry and staging dir leaked on image remove |
| BUG-234 | OB-081 | Med | Core | Health check execs not tracked in Store.Execs — invisible to exec API |
| BUG-235 | OB-045 | Med | Docker | Docker attach/exec ignores stdin — interactive sessions broken |
| BUG-236 | OB-015 | Med | Docker | Docker attach drops logs/detachKeys params |
| BUG-237 | OB-016 | Med | Docker | Docker logs drops details param |
| BUG-238 | OB-047 | Med | Docker | Docker backend missing 6 routes: update, changes, export, commit, resize, exec resize |
| BUG-239 | OB-050 | Med | Docker | Docker system df missing NetworkSettings in container summaries |
| BUG-240 | OB-092 | Med | Frontend | handleImageCreate ignores fromSrc — docker import broken |
| BUG-241 | OB-093 | Med | Frontend | handleContainerResize stub — TTY resize dropped |
| BUG-242 | OB-094 | Med | Frontend | handleExecResize stub — exec TTY resize dropped |
| BUG-243 | OB-095 | Med | Frontend | dialUpgrade ignores request context |
| BUG-244 | OB-098 | Low | Core | Symlinks and hardlinks dropped during tar extraction |
| BUG-245 | OB-100 | Low | Core | Malformed Created timestamp causes incorrect before/since filter |
| BUG-246 | OB-026 | Low | Docker | Docker network inspect drops verbose/scope params |
| BUG-247 | OB-027 | Low | Docker | Docker container start drops checkpoint options |
| BUG-248 | OB-028 | Low | Docker | Docker image inspect missing Metadata.LastTagTime |
| BUG-249 | OB-065 | Low | Cloud | Prune handlers skip AgentRegistry.Remove — stale entries |
| BUG-250 | OB-106 | Low | Lambda | ScanOrphanedResources no pagination — misses beyond first 50 |
| BUG-251 | OB-107 | Low | AZF | CleanupResource always no-op — in-memory lookup always fails after restart |

## Files Modified (31)

backends/core/store.go, handle_containers.go, handle_containers_archive.go, handle_containers_query.go, handle_images.go, health.go
backends/cloudrun/containers.go, backends/aca/containers.go
backends/lambda/containers.go, backends/cloudrun-functions/containers.go, backends/azure-functions/containers.go
backends/docker/containers.go, exec.go, extended.go, server.go, networks.go, images.go
backends/{ecs,cloudrun,aca,lambda,cloudrun-functions,azure-functions}/extended.go
backends/lambda/recovery.go, backends/azure-functions/recovery.go
frontends/docker/images.go, containers_stream.go, exec.go, backend_client.go, networks.go
BUGS.md

## Verification

- All backends compile: `go build ./...` — OK
- Core tests: 302 PASS (`cd backends/core && go test -race -count=1 ./...`)
- Lint: 0 issues (`make lint`)
