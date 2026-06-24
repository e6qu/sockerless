# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/ratchet-up-7` — **ratchet up EC2 + Glue, the two biggest floors (BUG-2200).** ~248 ops, all spec-validated against the vendored `aws-sdk-go-v2` Smithy models (0 divergences) + real SDK/CLI round-trips.

- **EC2 122→270:** Transit Gateway (42 — gateways, route tables, VPC/peering attachments, routes, associations/propagations, prefix-list refs, multicast/connect), IPAM (31 — IPAMs, scopes, pools with real CIDR carving, allocations, resource discoveries), Site-to-Site + Client VPN (35 — customer/VPN gateways, VPN connections with two IPsec tunnels, Client VPN endpoints/routes/auth/target-networks), Verified Access + Traffic Mirroring (41 — instances, trust providers, groups+endpoints with policies, mirror targets/filters/sessions).
- **Glue 102→194:** ML Transforms + Data Quality + column-statistics tasks (34 — rulesets, eval/recommendation runs settle synchronously with honest empty results), Interactive Sessions + Statements + Dev Endpoints + Blueprints (26), Catalog + Table Optimizer + BatchGet families + zero-ETL Integrations (32).
- **Validator fix:** the runtime spec-shape validator applied the Smithy `jsonName` trait for awsJson1.1 services, but awsJson codegen ignores it (verified vs the aws-sdk-go-v2 Glue deserializer reading `case "SparkConnect"` despite `jsonName: "SPARK_CONNECT"`); the validator now accepts a field under the member name OR its jsonName.
- **Process:** two rounds (4+3 agents) using a pre-wired-stub pattern — 7 empty sub-registrar files + their registerEC2/registerGlue calls created up front — so each parallel area agent edited only its own file with zero shared-file collisions. The newest ops need the latest `aws` CLI (CI installs latest v2; verified against a latest-CLI venv).
- Tests: aws sim/sdk/cli build/lint(0)/unit green; contract + cli-shard + all conformance tests pass; spec-shape validator 0 divergences.

**Next candidates:** keep ratcheting **EC2 (270/769) and Glue (194/267)** — both still have large headroom (EC2: capacity reservations, fleets, spot, network insights, local gateways, dedicated hosts, instance-connect, more VPC; Glue: remaining batch/crawler-schedule/partition ops). RDS, Lambda, API Gateway, CloudFront, ElastiCache, AutoScaling, CloudWatch Logs, Route 53, EFS, Cloud Map, Amplify, Scheduler, STS all have headroom. Then live-cloud (1075). Open GitHub issues: #394 (azuread upstream-blocked).

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
