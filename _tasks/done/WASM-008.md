# WASM-008: Test and verify

**Component:** Tests
**Phase:** 15
**Depends on:** WASM-006
**Estimated effort:** M
**Status:** PENDING

---

## Description
Build and test everything. Run existing tests to confirm no regressions. Manual verification of WASM execution.

## Verification Steps

1. **Build**: `go build ./backends/core/...` and `go build ./backends/memory/...`
2. **Existing tests**: `make test` — all pass (cloud backends unaffected)
3. **Simulator tests**: `make sim-test-all` — 105/105 (cloud backends unaffected)
4. **Manual verification**:
   - Start memory backend + frontend
   - `docker run --rm alpine echo "hello world"` → "hello world"
   - `docker run --rm alpine ls /` → Alpine rootfs listing
   - `docker run -d --name test alpine sleep 30` + `docker exec test echo "from exec"` → "from exec"
   - `docker exec -it test sh` → interactive shell
   - `docker cp /tmp/testfile.txt test:/tmp/` + `docker exec test cat /tmp/testfile.txt` → file contents
   - `docker exec test sh -c "echo hello | tr a-z A-Z"` → "HELLO"
   - `docker logs test` → process output
5. **E2E tests**: `make e2e-github-memory` and `make e2e-gitlab-memory` — pass with WASM exec

## Acceptance Criteria
1. `go build ./...` passes for all modules
2. All existing tests pass with zero regressions
3. Manual verification confirms real WASM execution
4. Cloud backends completely unaffected

## Notes
- If E2E tests fail, investigate whether WASM exec behavior differences cause problems with runner expectations
