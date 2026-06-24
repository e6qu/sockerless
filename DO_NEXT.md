# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/ratchet-up-9` — **finish Glue + drive ElastiCache/AutoScaling/Route 53 to 100% + ratchet EC2/RDS/API Gateway/CloudFront/CloudWatch Logs (BUG-2202).** ~320 ops in two rounds (5+5 agents), all spec-validated against the vendored `aws-sdk-go-v2` Smithy models (0 divergences) + real SDK/CLI round-trips.

- **Three more services to 100%:** ElastiCache 41→75 (serverless caches/snapshots, global replication groups, cache security groups, update actions, migration), Auto Scaling 25→66 (attach/detach LBs/target-groups/traffic-sources, instance refresh, warm pools, lifecycle actions, real static enumerations), Route 53 33→71 (reusable delegation sets, CIDR collections, DNSSEC/key-signing-keys, traffic-policy instances, VPC-association authorizations).
- **Glue 258→264** — its honest max; the 3 remaining ops (DescribeEntity/GetEntityRecords/ListEntities) query a real external SaaS connector API with no faithful local source (not faked).
- **Big ratchets:** EC2 354→389 (Network Insights / Network Access Analyzer, Route Servers, Local Gateway routes), RDS 64→101 (DB proxies + endpoints + targets, IAM roles, security groups, certificates, automated backups, log files, copy groups), API Gateway v1 62→99 (base-path mappings, documentation, client certs, gateway responses, SDK/export/usage) + v2 44→61 (integration/route responses, model templates, import/export), CloudFront 67→104 (field-level encryption, key-value stores, realtime logs, streaming distributions, VPC origins, anycast IP lists), CloudWatch Logs 36→73 (account/query/resource policies, vended-log deliveries, anomaly detectors, index policies).
- **Fifteen AWS services now at 100%** (Batch, CloudTrail, CodeBuild, WAFv2, ECR, KMS, ELBv2, Lambda, EFS, STS, Scheduler, Cloud Map, ElastiCache, Auto Scaling, Route 53).
- **Process:** two rounds of 5 agents, each owning a distinct service file (no pre-wiring needed). The go-build cache filled the disk mid-run (`go clean -cache` recovered ~58G). The newest ops need the latest `aws` CLI (CI installs latest v2; verified against a latest-CLI venv).
- Tests: aws sim/sdk/cli build/lint(0)/unit green; contract + cli-shard + all conformance tests pass; spec-shape validator 0 divergences.

**Next candidates:** keep ratcheting **EC2 (389/769)** — the biggest remaining floor; drive the mid-size floors toward 100% — RDS (101/164), API Gateway v1 (99/124) + v2 (61/103), CloudFront (104/167), CloudWatch Logs (73/113), Amplify, Cloud Map adjacents. Consider splitting EC2 into its own CLI shard (compute shard is at 27m). Then live-cloud (1075). Open GitHub issues: #394 (azuread upstream-blocked).

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
