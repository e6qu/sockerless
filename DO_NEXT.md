# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/ratchet-up-12` — **drive EC2 + SSM to 100% + ratchet CloudWatch Logs + S3, gate DynamoDB (BUG-2207).** ~291 ops in two rounds (6+5 agents), all spec-validated against the vendored `aws-sdk-go-v2` Smithy models (0 divergences) + real SDK/CLI round-trips.

- **EC2 600→769/769 (100%):** image-management, instance attributes/IAM-profile/console/credit/CPU/placement/SQL-HA, Local Gateway (Outposts) + Capacity Manager + Declarative Policies + Network-Performance, account-settings/ID-format/address-attrs/ClassicLink/enclave-cert/VPN-config/TGW-connect-peer/peering-options/network-ACL, volumes/snapshots/recycle-bin/CoIP/default-VPC/managed-prefix-lists/security-group-references/launch-template-data/DNS-options/Mac-tasks/IPv6.
- **SSM 43→146/146 (100%):** State Manager associations, Automation, Run Command, OpsCenter, Maintenance Window executions, Session Manager, Activations, Inventory, Compliance, Nodes, managed-instance info, document permissions, Patch groups, OpsMetadata, ResourcePolicy, parameter history/labels.
- **CloudWatch Logs 104→111** (the 2 remaining — StartLiveTail, GetLogObject — are HTTP/2 event-stream ops). **S3 91→103** (the 2 remaining — SelectObjectContent, WriteGetObjectResponse — are event-stream / Object-Lambda data-plane). **DynamoDB gated 57/57** + **Amplify 37/37** confirmed complete.
- **Twenty-one AWS services now at 100%.**
- Integration coordination: added EC2 `DescribeMacModificationTasks` (both owning agents deferred it to each other); wired the S3 method×subresource measurement matrix into the `s3ImplementedOps` harness from the S3 agent's report; implemented SSM's final `DeleteOpsItem`. Two CI catches fixed faithfully: a latent ECS `ExecuteCommand` container-handle poll that broke early under load (BUG-2208), and an S3 fidelity over-reach — the S3 agent had hosted the S3 Express dedicated-endpoint ops `ListDirectoryBuckets`/`CreateSession` on the regional `GET /`, which the spec validator correctly flagged (it resolves regional `GET /` to `ListBuckets`); reverted those 2 ops (S3 105→103) since real AWS serves them only from `s3express-control` endpoints the sim doesn't host.
- Tests: aws sim/sdk/cli build/lint(0)/unit green; contract + cli-shard + all conformance tests pass; spec-shape validator 0 divergences; new CLI tests green vs latest `aws` CLI (0 Invalid-choice).

**Next candidates:** the two biggest floors (EC2, SSM) are closed. Remaining headroom is genuinely-streaming/Object-Lambda ops (CloudWatch Logs 111/113, S3 103/107 — both at their faithful max without HTTP/2 event-stream support) and lower-coverage services not yet ratcheted (audit the conformance gate for the next mid-size floor — e.g. unmeasured restJson1/restXml services, or services still tracked only in the catalog). Then live-cloud (1075). Open GitHub issues: #394 (azuread upstream-blocked).

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
