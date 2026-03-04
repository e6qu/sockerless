# Bug Sprint 17 — API & Backends Audit (BUG-123→130)

**Completed**: 2026-03-04

## Bugs Fixed (8)

| Bug | Severity | Component | Fix |
|-----|----------|-----------|-----|
| BUG-123 | High | Core | `handleContainerStart` reverts state on Start() failure — StopHealthCheck + RevertToCreated + 500 error |
| BUG-124 | Med | Core | `handleContainerPrune` adds StopHealthCheck + destroy events (matching handleContainerRemove) |
| BUG-125 | Med | Core | `handleContainerKill` maps all common signals to exit codes (128+N) via signalToExitCode() |
| BUG-126 | Med | Core | `handleExecStart` checks container existence BEFORE marking exec as Running |
| BUG-127 | Med | Core | `handleImageList` deduplicates images by ID using seen map |
| BUG-128 | Med | Docker | `handleExecInspect` adds CanRemove field (SDK struct limitation — only derivable field) |
| BUG-129 | Med | Docker | `handleSystemDf` adds Ports/Mounts for containers, ParentID/VirtualSize for images, Status for volumes |
| BUG-130 | Med | Docker | `handleContainerCreate` auto-pull forwards X-Registry-Auth header |

## Files Modified

- `backends/core/handle_containers.go` — BUG-123 (start revert), BUG-125 (signal map)
- `backends/core/handle_extended.go` — BUG-124 (prune health+events)
- `backends/core/handle_exec.go` — BUG-126 (exec ordering)
- `backends/core/handle_images.go` — BUG-127 (image list dedup)
- `backends/docker/exec.go` — BUG-128 (exec inspect CanRemove)
- `backends/docker/extended.go` — BUG-129 (system df fields)
- `backends/docker/containers.go` — BUG-130 (auto-pull auth)
- `BUGS.md` — Compact summary table for 130 fixed bugs + 30 open bugs

## Verification

- `cd backends/core && go test -race -count=1 ./...` — 286 PASS
- `cd backends/docker && go build ./...` — OK
- `make lint` — 0 issues
