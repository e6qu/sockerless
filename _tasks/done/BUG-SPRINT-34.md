# Bug Sprint 34 — Complete

**Date:** 2026-03-05
**Bugs:** BUG-423 → BUG-436 (14 bugs)
**Branch:** bug-sprint-33

## Summary

Cloud backend logs parity + two core fixes.

### Cloud Logs (BUG-423–430, 433–435)
- Created `backends/core/log_cloud.go` — shared `CloudLogParams` struct with `ParseCloudLogParams`, `FormatLine`, `ApplyTail`, `FilterBufferedOutput`, `WriteMuxLine`, plus cloud-specific filter helpers
- All 6 cloud backends: since/until (423/424), stdout/stderr (426), details (427)
- CloudRun/GCF/ACA/AZF: tail via client-side slicing (425)
- Lambda/GCF/AZF: follow-mode polling (428/429/430)
- ECS/CloudRun/ACA: follow queries skip since/until (433)
- ACA: poll interval 2s→1s (434)
- Lambda/GCF/AZF: LogBuffers filtered through params (435)

### Core (BUG-431, 432, 436)
- `handleImageList` + `handleSystemDf`: ImageSummary.Containers counts containers per image (431/436)
- Health check loop: uses StartInterval during start period (432)

## Files Modified
- `backends/core/log_cloud.go` (new)
- `backends/ecs/logs.go`
- `backends/lambda/logs.go`
- `backends/cloudrun/logs.go`
- `backends/cloudrun-functions/logs.go`
- `backends/aca/logs.go`
- `backends/azure-functions/logs.go`
- `backends/core/handle_images.go`
- `backends/core/handle_extended.go`
- `backends/core/health.go`

## Verification
- `backends/core` tests: 302 PASS
- All 8 backends + frontend: build OK
