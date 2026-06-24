# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/ratchet-up-10` — **drive API Gateway v1+v2 to 100% + ratchet EC2/RDS/CloudFront/CloudWatch Logs + boyscout audit (BUG-2204).** ~244 ops in two rounds (5+2 agents), all spec-validated against the vendored `aws-sdk-go-v2` Smithy models (0 divergences) + real SDK/CLI round-trips.

- **Both API Gateway services to 100%:** v1 99→124/124 (Update* PATCH ops, tags, import/put-rest-api, flush-stage cache, test-invoke, domain-name access associations), v2 61→103/103 (developer portals + products + pages, routing rules, UpdateApi/UpdateApiMapping, ResetAuthorizersCache).
- **Big ratchets:** EC2 389→448 (Transit Gateway multicast/metering/policy-table/route-table-announcements + IPAM policies/BYOASN/prefix-list-resolvers/external-resource-tokens/discovered-resources), RDS 101→150 (restores, reserved instances, blue/green deployments, zero-ETL integrations, tenant databases, shard groups, activity streams, backtrack, export tasks), CloudFront 104→152 (distribution tenants, connection groups, trust stores, resource policy, web-ACL associations, ListDistributionsBy*), CloudWatch Logs 73→104 (OpenSearch integrations, lookup tables, scheduled queries, transformers, import tasks, anomalies — deferred StartLiveTail/GetLogObject HTTP/2 streaming).
- **Seventeen AWS services now at 100%** (Batch, CloudTrail, CodeBuild, WAFv2, ECR, KMS, ELBv2, Lambda, EFS, STS, Scheduler, Cloud Map, ElastiCache, Auto Scaling, Route 53, API Gateway v1, API Gateway v2).
- **Boyscout (BUG-2203):** a read-only audit of the older core surfaced 12 candidates; fixed the 2 genuine ones (`time.Parse` swallows in CloudTrail's S3-key builder + CloudWatch ListDashboards — a corrupt stored timestamp now falls back to now instead of year-0001). The other 10 were unreachable `json.Marshal`-of-plain-data error-drops (Go can't fail on those) + 1 false-positive "race" (`restRegisterOp` is single-threaded at startup, read-only during serving).
- **Process:** each agent owned a distinct service file; fixed 6 `ineffassign` + the inline-vs-list RDS renderer split at integration. Newest ops need the latest `aws` CLI (CI installs latest v2).
- Tests: aws sim/sdk/cli build/lint(0)/unit green; contract + cli-shard + all conformance tests pass; spec-shape validator 0 divergences.

**Next candidates:** **EC2 (448/769)** is the biggest remaining floor — keep ratcheting (carrier gateways, COIP pools, VPC peering options, instance-attribute family, more). Mid-size floors with headroom: RDS (150/164 — 14 from 100%), CloudFront (152/167 — connection functions + streaming), CloudWatch Logs (104/113 — mostly streaming left), Amplify, SSM (43/146), DynamoDB. Then live-cloud (1075). Open GitHub issues: #394 (azuread upstream-blocked).

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
