# FE-BUG-001: Frontend exec/attach bridge hang

**Component:** Docker REST API Frontend
**Phase:** Bug Fix
**Depends on:** —
**Estimated effort:** S

---

## Description

Fix an intermittent hang in `TestGitHubRunnerContainerJob` caused by a race condition in the frontend's bidirectional bridge for exec and attach operations.

## Root Cause

The frontend's `handleExecStart` and `handleContainerAttach` handlers bridge two TCP connections (client↔backend) bidirectionally:

1. **Goroutine:** `io.Copy(backendConn, clientConn)` — forwards stdin (client→backend)
2. **Main thread:** `io.Copy(clientConn, backendConn)` — forwards stdout/stderr (backend→client)

After the backend→client copy completes (backend closed), the handler calls `CloseWrite(clientConn)` to send a TCP FIN (clean EOF) to the client, then waits on `<-done` for the stdin goroutine to finish.

The goroutine is blocked on `clientConn.Read()` waiting for stdin data that never comes (exec with no stdin attached). The intended sequence is:

1. Frontend: `CloseWrite(clientConn)` → client gets EOF
2. Client: `io.ReadAll` returns → calls `hijacked.Close()` → closes TCP connection
3. Frontend: goroutine's `clientConn.Read()` returns EOF → goroutine exits
4. Frontend: `<-done` unblocks

This works most of the time, but creates a timing-sensitive circular dependency: step 3 depends on step 2, which depends on step 1. If TCP FIN propagation is delayed or the client's close doesn't propagate back promptly, the goroutine blocks indefinitely, causing the handler to hang at `<-done`.

## Fix

After `CloseWrite` (to give the client a clean EOF), also call `CloseRead` on `clientConn` to immediately unblock the stdin goroutine. The backend has already closed, so forwarding more stdin is pointless. `CloseRead` calls `shutdown(fd, SHUT_RD)` which causes any pending `Read()` to return immediately.

## Files Changed

- `frontends/docker/exec.go` — Added `CloseRead` after `CloseWrite` in `handleExecStart`
- `frontends/docker/containers.go` — Added `CloseRead` after `CloseWrite` in `handleContainerAttach`

## Definition of Done

- [x] Fix compiles: `go build ./...` passes
- [x] All 102 existing tests pass (63 PASS, 39 SKIP, 0 FAIL)
- [x] `TestGitHubRunnerContainerJob` passes reliably (5/5 runs)
- [x] Race detector clean: `go test -race` passes
- [x] PLAN.md and STATUS.md updated
