# Sockerless — Current Status

**85 phases complete (756 tasks). 594 bugs fixed. 0 open bugs.**

## Test Results

| Category | Count |
|---|---|
| Core unit tests | 302 PASS |
| Frontend tests | 7 PASS |
| UI tests (Vitest) | 92 PASS |
| Admin tests | 88 PASS |
| Admin Playwright E2E | 17 PASS |
| bleephub | 304 unit + 9 integration + 1 gh CLI |
| Shared ProcessRunner | 15 PASS |
| Cloud SDK | AWS 42, GCP 43, Azure 38 |
| Cloud CLI | AWS 26, GCP 21, Azure 19 |
| Sim-backend integration | 75 PASS |
| GitHub E2E | 186 PASS |
| GitLab E2E | 132 PASS |
| Upstream gitlab-ci-local | 216 PASS |
| Terraform integration | 75 PASS |
| **ECS live (real AWS)** | **11/12 phases PASS** (exec skipped) |
| Lint (18 modules) | 0 issues |

## ECS Live Testing (2026-03-29)

Tested against real AWS ECS Fargate in `eu-west-1` via `terraform/environments/ecs/live/`. 34 AWS resources provisioned (VPC, ECS cluster, EFS, ECR, IAM, Cloud Map, security groups). Results:

- Container create/start/stop/remove → real Fargate tasks
- Logs → real CloudWatch Logs
- Image pull → real registry config fetched from Docker Hub
- Networking → VPC security groups created/deleted
- Volumes, prune, error cases → all pass

6 bugs found and fixed during testing (BUG-584 through BUG-591, BUG-594). See [BUGS.md](BUGS.md) and [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md) for details.

## Architecture

7 backends (docker, ecs, lambda, cloudrun, gcf, aca, azf) sharing a common core with driver interfaces (Exec, Filesystem, Stream, Network, Logging, Service Discovery). Cloud backends have real cloud-native implementations:

- **Logging**: All 6 cloud backends use `core.StreamCloudLogs` with backend-specific `CloudLogFetchFunc` closures (CloudWatch, Cloud Logging, Azure Monitor KQL)
- **Exec/Attach**: ECS uses ExecuteCommand + SSM WebSocket, ACA uses Container Apps exec API WebSocket, Cloud Run requires agent sidecar
- **Networking**: ECS creates VPC Security Groups, CloudRun creates Cloud DNS managed zones, ACA tracks NSG state
- **Service Discovery**: ECS uses AWS Cloud Map (register/deregister/discover), CloudRun uses Cloud DNS A records, ACA uses in-process DNS registry

3 cloud simulators validated against SDKs, CLIs, and Terraform. ECS backend validated against real AWS.

## Known Limitations

1. **FaaS transient failures** — ~1 per sequential E2E run on reverse agent backends
2. **Upstream act individual mode** — azf requires `--individual` flag
3. **Azure terraform tests** — Docker-only (Linux); macOS ignores `SSL_CERT_FILE`
4. **CloudWatch log latency** — `docker run` for short-lived containers may show no inline output; use `docker logs <id>` after a few seconds
5. **ECS exec** — requires agent sidecar or SSM ExecuteCommand (not yet validated on real AWS)
