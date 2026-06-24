# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `feat/ratchet-up-8` — **ratchet EC2/Glue further + drive Lambda/EFS/STS/Scheduler/Cloud Map to 100% (BUG-2201).** ~199 ops in two rounds (5+2 agents over isolated pre-wired files), all spec-validated against the vendored `aws-sdk-go-v2` Smithy models (0 divergences). **EC2 270→354** (Capacity Reservations/Fleets/Spot 38, Dedicated Hosts + Image/Snapshot attrs + Instance Event Windows + VPC misc 46), **Glue 194→258** (ML-task runs/schedules/workflow-runs/connection-types 33, custom entities/usage-profiles/schema-metadata/source-control/code-gen 32). **Five more to 100%:** Lambda 62→85 (capacity providers, durable executions, InvokeWithResponseStream with real event-stream framing), EFS 29→31, STS 4→11, Scheduler 9→12, Cloud Map 16→30. **Twelve AWS services now at 100%.** aws sim/sdk/cli build/lint(0)/unit green; contract + cli-shard + all conformance tests pass. |
| Last merged (#676) | `feat/ratchet-up-7` — **ratchet up EC2 + Glue (BUG-2200)** — ~248 ops; EC2 122→270, Glue 102→194; fixed the awsJson `jsonName` validator FP. |
| Last merged (#675) | `feat/ratchet-up-6` — **drove Batch/CloudTrail/CodeBuild/WAFv2/ECR to 100% (BUG-2199)** — ~138 ops; seven AWS services now at 100% (+ KMS, ELBv2). |
| Last merged (#674) | `feat/ratchet-up-5` — **ratchet up RDS/Glue/Lambda/API Gateway/CloudFront/ElastiCache; complete KMS + ELBv2 (BUG-2198)** — ~165 ops; KMS 54/54 + ELBv2 51/51 driven to 100%. |
| Last merged (#673) | `feat/ratchet-up-4` — **ratchet up EC2/ECR/AutoScaling/CloudWatch Logs (BUG-2197)** — EC2 102→122, ECR 26→38, Auto Scaling 13→25, CloudWatch Logs 18→36; ~62 ops. |
| Last merged (#672) | `feat/ratchet-up-3` — **raise CloudTrail/CodeBuild/WAFv2 + ratchet up Glue (BUG-2196)** — CloudTrail 16→23 (+2 bug fixes), CodeBuild 9→22, WAFv2 28→32, Glue 52→78; ~50 ops. |
| Last merged (#671) | `feat/ratchet-up-2` — **ratchet up EC2/RDS/Glue/Lambda/Batch/API Gateway + add CloudFront (BUG-2195)** — EC2 91→102, RDS 25→40, Glue 30→52, Lambda 23→37, Batch 19→24, API Gateway; CloudFront 52/167. |
| Last merged (#670) | `feat/ratchet-up-services` — **ratchet-up the floored services + measure the restJson1 services (BUG-2194)** — EC2/RDS/ElastiCache/Glue/Route53/EFS +77 ops; Lambda/Batch/API Gateway/Amplify/Scheduler measured. |
| Earlier merged | #665–#669 built the AWS service-conformance gate; #574–#664 = the runner/cell + audit + IAM-enforcement + sim-fidelity arc. Full history in `git log` and [WHAT_WE_DID.md](WHAT_WE_DID.md). |
| Open GitHub issues | #394 azuread Terraform Graph override — upstream-blocked (BUG-1345). |
| Bugs | See [BUGS.md](BUGS.md) header (2201 filed · 2157 fixed · 2 open · 14 FP). 2 open: BUG-1075 (live-cloud), BUG-1345 (azuread upstream) — both externally gated. |
| Live infra | None up. |

## What's next

- **AWS service-conformance ratchet (active arc).** The gate measures ~37 AWS services against the vendored `aws-sdk-go-v2` Smithy models; twelve are at 100% (Batch, CloudTrail, CodeBuild, WAFv2, ECR, KMS, ELBv2, Lambda, EFS, STS, EventBridge Scheduler, Cloud Map). Keep ratcheting the big floors — **EC2 (354/769) and Glue (258/267)** have the most headroom — and drive more mid-size services to 100%. Resume steps in [DO_NEXT.md](DO_NEXT.md).
- **Standing.** Live-cloud validation (BUG-1075, biggest externally-gated gap); versioned releases (#363); fresh sim-fidelity / fallback-and-error-swallowing audits (user guidance: fail loudly, no contract-breaking sim fallbacks, avoid defaulted behaviour).
- **Foundation (done, sim-proven).** GitHub + GitLab runner cells are green on all six container-capable backends (ECS, Lambda-class, Cloud Run, GCF, ACA, AZF); the full GitLab docker-executor flow (build → artifact → `services:`) and FaaS multi-container pod assembly are complete. Detail in `git log` + [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Invariants

- Never auto-merge PRs; the user handles merges.
- **At most one PR open at a time** — put all work in the single in-progress PR; never open a new one while one exists. If two ever exist, **consolidate** their work into one (merge the branches together) — do not evade the rule. Closing a PR *without merging it* abandons and deletes that work for good; it is never a way to park work or dodge the rule. Enforced by `scripts/check-single-open-pr.sh` (pre-commit + the `single-open-pr` CI job).
- Rebase PR branches on `origin/main` before pushing; sync local `main` after.
- File a concrete `BUGS.md` entry before fixing a discovered defect.
- No stubs, fakes, mocks, synthetic responses, silent fallbacks, or degraded modes (see [AGENTS.md](AGENTS.md)).
- Simulators implement real cloud-API slices, one binary per cloud; every public endpoint ships with official SDK + vendor CLI + Terraform coverage where those surfaces exist.
- External identity stays GitHub/GHES-shaped (public paths, fields, `GITHUB_*` vars, runner contract, client-facing UI text); bleephub-specific names only for internal code or operator-only surfaces.
- Coverage authorities: [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md) and [specs/SIM_SURFACE_TABLES](specs/SIM_SURFACE_TABLES).

## Environment notes

- Simulator ports: AWS 4566, GCP 4567, Azure 4568.
- AWS and GCP Terraform providers accept localhost custom endpoints directly; AzureRM needs HTTPS through the local Caddy gateway (`make stack-https-{up,status,ca,down}`). Azure Terraform tests are Docker-only.
- Linux network-fabric tests require `CAP_NET_ADMIN` + iproute2 + nftables; off-Linux they skip through the realexec capability gate.
- Local bleephub runner topology harness: `make bleephub-runner-docker-test` (ECS) / `make bleephub-runner-docker-test-aca` (ACA); self-contained, mounts docker.sock + a sim-storage host dir. `BLEEPHUB_BACKEND` selects the backend; `BLEEPHUB_TEST_FROM` skips to a test; `BLEEPHUB_HOLD=1` freezes the stack on failure. The one harness image bundles the aws + azure sims, backend-ecs + backend-aca, and the cloudrun-bootstrap.
