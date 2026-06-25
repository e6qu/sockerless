# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `fix/ci-flakes-and-open-issues` — **CI flakiness sweep + ELBv2 open issues + CloudTrail fix (BUG-2216/2217/2218/2219).** After #686 a `sim (aws sdk)` job flaked (failed once, passed on no-change re-run), so this is a cross-sim flaky-pattern hardening pass: AWS (ENI-attach sleep→poll, EBS-snapshot poll 2s→60s, DynamoDB-Local readiness 30s→120s), GCP (job-execution sleep→poll, DNS/job `Eventually` 10-15s→30-60s), Azure (AMQP receive + async-op + ACA/ACI waits 2-5s→30s) — all assertions unchanged. Plus the two unblocked open issues: **ELBv2 #685** (omit HealthCheck `Matcher` for TCP target groups — terraform idempotency) and **ELBv2 #683** (real NLB raw-TCP data plane via `net.Listener`+`io.Copy`, per-connection target resolution). Plus incidental: **CloudTrail** was silently dropping all ElastiCache events (missing `2015-02-02` eventSource mapping). aws+gcp+azure build/lint(0) green; conformance + spec-validator(0) + terraform idempotency + cli-shard gates pass. |
| Last merged (#686) | `feat/gcp-ratchet-1` — **GCP operation-coverage gate + ratchet 12 mid-size services (BUG-2214/2215)** — built `gcpMethodFloor`; GCP 1986→2413/5244 (38%→46%), 6 services at 100%; DNS DNSSEC determinism fix + Firestore true incremental streaming. |
| Last merged (#684) | `feat/ratchet-up-14` — **gate-audit: measure + drive IAM to 100% + boyscout fixes (BUG-2213)** — IAM was the sole unmeasured AWS service; gated at 176/176; twenty-seven AWS services at 100%; 5 IAM fail-loud boyscout fixes. |
| Last merged (#682) | `feat/ratchet-up-13` — **event-stream ops + drive CloudWatch/Organizations/SQS/Kinesis to 100% + boyscout fixes (BUG-2211)** — 4 streaming ops + five services to 100%; twenty-six services at 100%. |
| Last merged (#681) | `feat/ratchet-up-12` — **drive EC2 + SSM to 100% + ratchet CloudWatch Logs + S3, gate DynamoDB (BUG-2207)** — ~291 ops; twenty-one services at 100%; 2 CI-caught fixes (ECS-exec poll, S3-Express fidelity). |
| Last merged (#680) | `feat/ratchet-up-11` — **drive RDS + CloudFront to 100% + ratchet EC2 + boyscout fixes (BUG-2206)** — ~181 ops; nineteen services at 100%; 3 boyscout silent-fallback fixes. |
| Last merged (#679) | `feat/ratchet-up-10` — **API Gateway v1+v2 to 100% + ratchet EC2/RDS/CloudFront/CloudWatch Logs + boyscout (BUG-2204)** — ~244 ops; seventeen services at 100%. |
| Last merged (#678) | `feat/ratchet-up-9` — **finish Glue + ElastiCache/AutoScaling/Route 53 to 100% + ratchet EC2/RDS/API Gateway/CloudFront/CloudWatch Logs (BUG-2202)** — ~320 ops; fifteen services at 100%. |
| Last merged (#677) | `feat/ratchet-up-8` — **ratchet EC2/Glue further + Lambda/EFS/STS/Scheduler/Cloud Map to 100% (BUG-2201)** — ~199 ops; twelve services at 100%. |
| Last merged (#676) | `feat/ratchet-up-7` — **ratchet up EC2 + Glue (BUG-2200)** — ~248 ops; EC2 122→270, Glue 102→194; fixed the awsJson `jsonName` validator FP. |
| Last merged (#675) | `feat/ratchet-up-6` — **drove Batch/CloudTrail/CodeBuild/WAFv2/ECR to 100% (BUG-2199)** — ~138 ops; seven AWS services now at 100% (+ KMS, ELBv2). |
| Last merged (#674) | `feat/ratchet-up-5` — **ratchet up RDS/Glue/Lambda/API Gateway/CloudFront/ElastiCache; complete KMS + ELBv2 (BUG-2198)** — ~165 ops; KMS 54/54 + ELBv2 51/51 driven to 100%. |
| Last merged (#673) | `feat/ratchet-up-4` — **ratchet up EC2/ECR/AutoScaling/CloudWatch Logs (BUG-2197)** — EC2 102→122, ECR 26→38, Auto Scaling 13→25, CloudWatch Logs 18→36; ~62 ops. |
| Last merged (#672) | `feat/ratchet-up-3` — **raise CloudTrail/CodeBuild/WAFv2 + ratchet up Glue (BUG-2196)** — CloudTrail 16→23 (+2 bug fixes), CodeBuild 9→22, WAFv2 28→32, Glue 52→78; ~50 ops. |
| Last merged (#671) | `feat/ratchet-up-2` — **ratchet up EC2/RDS/Glue/Lambda/Batch/API Gateway + add CloudFront (BUG-2195)** — EC2 91→102, RDS 25→40, Glue 30→52, Lambda 23→37, Batch 19→24, API Gateway; CloudFront 52/167. |
| Last merged (#670) | `feat/ratchet-up-services` — **ratchet-up the floored services + measure the restJson1 services (BUG-2194)** — EC2/RDS/ElastiCache/Glue/Route53/EFS +77 ops; Lambda/Batch/API Gateway/Amplify/Scheduler measured. |
| Earlier merged | #665–#669 built the AWS service-conformance gate; #574–#664 = the runner/cell + audit + IAM-enforcement + sim-fidelity arc. Full history in `git log` and [WHAT_WE_DID.md](WHAT_WE_DID.md). |
| Open GitHub issues | #394 azuread Terraform Graph override — upstream-blocked (BUG-1345). |
| Bugs | See [BUGS.md](BUGS.md) header (2219 filed · 2175 fixed · 2 open · 16 FP). 2 open: BUG-1075 (live-cloud), BUG-1345 (azuread upstream) — both externally gated. |
| Live infra | None up. |

## What's next

- **AWS service-conformance ratchet (active arc).** The gate measures ~37 AWS services against the vendored `aws-sdk-go-v2` Smithy models; twenty-seven are at 100% (Batch, CloudTrail, CodeBuild, WAFv2, ECR, KMS, ELBv2, Lambda, EFS, STS, EventBridge Scheduler, Cloud Map, ElastiCache, Auto Scaling, Route 53, API Gateway v1, API Gateway v2, RDS, CloudFront, EC2, SSM, CloudWatch, Organizations, SQS, Kinesis, CloudWatch Logs, IAM) — plus DynamoDB + Amplify (gated complete). The gate audit is now complete — all 38 vendored Smithy models are gated (IAM was the last gap), and nearly every measured service is at its faithful max: the only remaining gaps are genuinely-unhostable ops on the regional sim surface (S3 104/107: WriteGetObjectResponse Object-Lambda + the two S3 Express dedicated-endpoint ops; Glue 264/267: SaaS-connector ops). The AWS conformance arc is essentially closed. Resume steps in [DO_NEXT.md](DO_NEXT.md).
- **GCP service-conformance ratchet (now active).** GCP got the same treatment: built `gcpMethodFloor` (the per-Discovery-doc operation-coverage gate it was missing) and ratcheted 12 mid-size services — GCP coverage is now 2413/5244 (46%), with 6 services at 100% (Cloud Build, Redis, Firestore, Storage + near-100 KMS/IAM/Artifact Registry/Eventarc). Next GCP candidates: the larger mid-size services (Spanner 186/198, SQL Admin 136/148, the small-gap batch — API Gateway/ServiceUsage/VPC Access/IAM Credentials), then the big surfaces (Cloud Run, Logging, Bigtable Admin, Cloud Resource Manager) and the Azure simulator (no coverage gate yet). Live-cloud (BUG-1075) remains the biggest externally-gated gap.
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
