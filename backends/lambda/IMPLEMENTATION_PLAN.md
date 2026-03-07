# Lambda Backend: Delegate Method Implementation Plan

## Overview

The Lambda backend implements `api.Backend` (65 methods). Currently **12 methods** have cloud-native implementations in `backend_impl.go`:

- `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerKill`, `ContainerRemove`
- `ContainerLogs`, `ContainerRestart`, `ContainerPrune`, `ContainerPause`, `ContainerUnpause`
- `ImagePull`, `ImageLoad`

The remaining **53 methods** delegate to `s.BaseServer.Method()`.

Lambda is a FaaS platform. Many container/image operations have no direct equivalent.

## Priority Summary

| Priority | Count | Description |
|----------|-------|-------------|
| P0 | 3 | BaseServer implementation is actively wrong |
| P1 | 5 | Works but misses cloud-specific features |
| P2 | 45 | Adequate or N/A for FaaS |

---

## P0 — Critical (3 methods)

### ImageBuild
- **BaseServer behavior**: Builds synthetic image from Dockerfile.
- **Why wrong**: Lambda requires images in ECR. The synthetic image cannot be used with `lambda.CreateFunction` (which needs an `ImageUri` pointing to ECR).
- **Short-term implementation**: Return `NotImplementedError` — users should pre-build and push to ECR.
- **Long-term**: Submit to CodeBuild, push to ECR.
- **AWS APIs (long-term)**: `codebuild:StartBuild`, `ecr:CreateRepository`, `ecr:GetAuthorizationToken`

### AuthLogin
- **BaseServer behavior**: Always returns "Login Succeeded" regardless of credentials.
- **Why wrong**: When target is ECR, invalid credentials silently succeed, leading to confusing failures during pull/push.
- **Implementation**: If `ServerAddress` matches ECR (`*.dkr.ecr.*.amazonaws.com`), call `ecr:GetAuthorizationToken` to validate. Otherwise delegate to BaseServer.
- **AWS APIs**: `ecr:GetAuthorizationToken`
- **Dependencies**: Add ECR client to `AWSClients`.

### Info
- **BaseServer behavior**: Returns static descriptor with `KernelVersion: "5.15.0-sockerless"`.
- **Why wrong**: Misleading for Lambda (runs on Amazon Linux 2 kernels).
- **Implementation**: Override to set correct kernel version. Optionally call `lambda:GetAccountSettings` for `NCPU`/`MemTotal` from account limits.
- **AWS APIs**: `lambda:GetAccountSettings` (optional)

---

## P1 — Important (5 methods)

### ContainerAttach
- **BaseServer**: Synthetic stream driver pipe.
- **Implementation**: If agent connected (`AgentAddress == "reverse"`), delegate to BaseServer. Otherwise return `NotImplementedError` — Lambda functions are not interactive.

### ContainerStats
- **BaseServer**: Synthetic stats with zero values.
- **Implementation**: Query CloudWatch metrics (`AWS/Lambda` namespace): `Invocations`, `Duration`, `ConcurrentExecutions`, `Throttles`, `Errors`. Map to Docker stats format.
- **AWS APIs**: `cloudwatch:GetMetricStatistics`
- **Trade-off**: Docker stats format doesn't map cleanly to Lambda metrics. Low priority.

### ImagePush
- **BaseServer**: Synthetic "pushed" progress.
- **Why incomplete**: Silently succeeds without pushing to ECR.
- **Short-term**: Return `NotImplementedError` — users should push to ECR directly.

### ExecCreate
- **BaseServer**: Creates exec instance in-memory.
- **Enhancement**: Adequate. Document that exec is agent-dependent.

### ExecStart
- **BaseServer**: Uses driver chain. Works with agent, falls back to synthetic.
- **Enhancement**: Works correctly when reverse agent is connected. Document as agent-dependent.

---

## P2 — Acceptable / N/A for FaaS (45 methods)

### Fundamentally N/A for Serverless

| Method | Reason |
|--------|--------|
| ContainerPause/Unpause | Already returns `NotImplementedError` |
| ContainerChanges | No persistent filesystem |
| ContainerExport | No filesystem to export |
| ContainerCommit | Cannot create images from functions |
| ContainerResize | No TTY |
| ImageLoad | Already returns `NotImplementedError` |
| ImageSave | No local image storage |
| ImageSearch | No local image index |
| ImageHistory | No layer history |
| All 7 Network methods | Lambda uses VPC config, not Docker networks |
| All 5 Volume methods | Lambda has ephemeral /tmp only |

### Adequate with BaseServer

- **Container**: Inspect, List, Wait, Top, Rename, Update, PutArchive, StatPath, GetArchive
- **Exec**: Inspect, Resize
- **Images**: Inspect, List, Remove, Prune, Tag
- **Pods**: Create, List, Inspect, Exists, Start, Stop, Kill, Remove (single-container pods work via delegation; multi-container rejected at ContainerStart)
- **System**: Df, Events

---

## Implementation Phases

### Phase 1: P0 Fixes (3 methods)
1. **Info** — Fix KernelVersion, optionally enrich with `GetAccountSettings`. ~30 lines.
2. **ImageBuild** — Return `NotImplementedError`. ~10 lines.
3. **AuthLogin** — Validate ECR credentials. Add `ecr.Client` to `AWSClients`. ~50 lines.

### Phase 2: P1 Improvements (2 methods worth implementing)
4. **ContainerAttach** — Return `NotImplementedError` when no agent. ~15 lines.
5. **ImagePush** — Return `NotImplementedError`. ~10 lines.

### Phase 3: Optional Enhancement
6. **ContainerStats** — CloudWatch metrics. Add `cloudwatch.Client`. ~80 lines. Defer unless needed.

### New AWS SDK Clients Needed

| Client | Phase | Package |
|--------|-------|---------|
| `ecr.Client` | 1 | `github.com/aws/aws-sdk-go-v2/service/ecr` |
| `cloudwatch.Client` | 3 | `github.com/aws/aws-sdk-go-v2/service/cloudwatch` |

### Recommended Order
1 → 2 → 3
