# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/ratchet-up-8` — **ratchet EC2/Glue further + drive Lambda/EFS/STS/Scheduler/Cloud Map to 100% (BUG-2201).** ~199 ops, all spec-validated against the vendored `aws-sdk-go-v2` Smithy models (0 divergences) + real SDK/CLI round-trips.

- **EC2 270→354:** Capacity Reservations + EC2 Fleets + Spot (38), Dedicated Hosts + Image/Snapshot attributes + Instance Event Windows + VPC ClassicLink/endpoint-connections/block-public-access (46).
- **Glue 194→258:** ML-task runs + crawler/column-stats schedules + materialized-view refresh + workflow-run properties + connection types (33), custom entity types + usage profiles + Identity Center config + schema-version metadata + source-control sync + code-gen GetMapping/GetPlan/GetDataflowGraph (32). Deferred 3 SaaS-connector entity-introspection ops (DescribeEntity/GetEntityRecords/ListEntities) — they query a real external SaaS API, no faithful local source.
- **Five more services to 100%:** Lambda 62→85 (capacity providers, durable executions, scaling config, **InvokeWithResponseStream with real `vnd.amazon.eventstream` framing**), EFS 29→31, STS 4→11 (GetFederationToken/AssumeRoleWithSAML/AssumeRoot/… real temp creds), EventBridge Scheduler 9→12 (tag ops; resolved a `/tags/{arn}` mux-overlap with Amplify via a faithful in-band SigV4-service discriminator), Cloud Map 16→30.
- **Twelve AWS services now at 100%** (Batch, CloudTrail, CodeBuild, WAFv2, ECR, KMS, ELBv2, Lambda, EFS, STS, Scheduler, Cloud Map).
- **Process:** two rounds (5+2 agents) with the pre-wired-stub pattern; fixed 4 staticcheck QF1012 at integration. The newest ops need the latest `aws` CLI (CI installs latest v2; verified against a latest-CLI venv).
- Tests: aws sim/sdk/cli build/lint(0)/unit green; contract + cli-shard + all conformance tests pass; spec-shape validator 0 divergences.

**Next candidates:** keep ratcheting **EC2 (354/769) and Glue (258/267 — 9 from 100%)**; mid-size floors with headroom: RDS (64/164), API Gateway v1 (62/124) + v2 (44/103), CloudFront (67/167), ElastiCache (41/75), AutoScaling (25/66), CloudWatch Logs (36/113), Route 53 (33/71). Then live-cloud (1075). Open GitHub issues: #394 (azuread upstream-blocked).

## Working agreement

The full before/after-task continuity-file workflow, the no-fakes rules, and branch/PR hygiene live in [AGENTS.md](AGENTS.md). In short: read `STATUS.md`/`DO_NEXT.md` first; run the narrowest meaningful tests for the touched area; file bugs before fixing; update the continuity files in the same commit as the code; rebase on `origin/main` before pushing; never merge the PR.

Narrowest-test recipes for the common surfaces:

```bash
# Simulator SDK probe
cd simulators/<cloud>/sdk-tests && GOWORK=off CGO_ENABLED=0 go test -tags noui -run '<pat>' -timeout 15m .
# Simulator module unit tests + lint
cd simulators/<cloud> && make unit-test
# A backend's unit tests
cd backends/<name> && GOWORK=off go test ./...
# bleephub runner topology harness (self-contained)
make bleephub-runner-docker-test
```
