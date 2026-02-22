# CORE-010: Extract agent injection into core

**Component:** Core (Shared backend library)
**Phase:** 5
**Depends on:** CORE-002
**Estimated effort:** S
**Status:** DONE

---

## Description
Moved buildAgentEntrypoint, isTailDevNull, and buildOriginalCommand functions from ECS/CloudRun/ACA agent_inject.go files to core/agent.go as exported functions (BuildAgentEntrypoint, IsTailDevNull, BuildOriginalCommand). Updated 3 backends to call core.BuildAgentEntrypoint. Deleted 3 duplicate agent_inject.go files.

## Acceptance Criteria
1. core/agent.go has BuildAgentEntrypoint, IsTailDevNull, BuildOriginalCommand exported
2. ECS, Cloud Run, and ACA backends call core.BuildAgentEntrypoint
3. No duplicate agent_inject.go files exist in backend directories
4. All backends build successfully
5. All tests pass unchanged

## Definition of Done
- [x] `go build ./...` passes
- [x] `go vet ./...` passes
- [x] All 102 existing tests pass unchanged
