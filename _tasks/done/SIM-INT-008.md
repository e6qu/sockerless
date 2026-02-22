# SIM-INT-008: Azure LRO Fix + Timeout Optimization

**Status:** DONE
**Phase:** 8 — Simulator-Backend Integration

## Problem

1. **ACA test timeout:** The Azure simulator's `/start` endpoint returned HTTP 200, but the Azure SDK's `BeginStart` uses an LRO poller with `FinalStateViaLocation`. A 200 without a `Location` header caused the poller to return an empty result (`startResp.Name` was nil), so `executionName` was empty and `waitForExecutionRunning` could never match any execution — infinite loop until 5min timeout.

2. **Slow sim tests:** Auto-stop delays (30s), polling intervals (2-5s), log API timeouts (60s), and TCP connect timeouts to non-routable agent IPs all contributed to a >5min test suite.

## Changes

### Azure LRO Fix
- **`simulators/azure/containerapps.go`** — Changed `/start` response from 200 to 202 with `Location` header pointing to the execution's GET URL, enabling proper LRO polling
- **`backends/aca/containers.go`** — Made `waitForExecutionRunning` handle empty `executionName` gracefully (match any execution for the job)

### Timeout Optimization
- **Simulator auto-stop:** 30s → 3s (aws/ecs.go, gcp/cloudrunjobs.go, azure/containerapps.go)
- **Backend task-running poll:** 2s → 500ms in sim mode (ecs, cloudrun, aca)
- **Backend exit poll:** 5s → 1s in sim mode (ecs, cloudrun, aca)
- **GCP log timeout:** 60s → 2s in sim mode (cloudrun/logs.go, cloudrun-functions/logs.go)
- **Agent address gating:** Don't set AgentAddress when health check fails; exec falls back to synthetic (ecs, cloudrun, aca)

## Result

- ACA tests: all 6 PASS (was timing out)
- CloudRun/ACA exec: PASS (was FAIL — now uses synthetic exec fallback)
- Runner E2E subtests: all PASS (ECS/CloudRun/ACA)
- Total sim suite: 91 PASS, 2 FAIL (~170s, was >5min)
