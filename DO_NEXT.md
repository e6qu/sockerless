# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/ratchet-up-11` — **drive RDS + CloudFront to 100% + ratchet EC2 + boyscout fixes (BUG-2206).** ~181 ops in two rounds (5+3 agents), all spec-validated against the vendored `aws-sdk-go-v2` Smithy models (0 divergences) + real SDK/CLI round-trips.

- **Two more services to 100%:** RDS 150→164 (custom DB engine versions, recommendations, snapshot tenant databases, ServerlessV2 platform versions, current-cluster-capacity, ModifyOptionGroup, automated-backups replication, global-cluster + read-replica switchovers), CloudFront 152→167 (connection functions with publish/test, TestFunction, tagging, and the dynamically-dispatched CreateDistribution*/CreateStreamingDistributionWithTags op names registered after a *verified* real-SDK round-trip — plus a real fix: CreateStreamingDistributionWithTags previously returned 400 because the handler only decoded the bare config).
- **Big EC2 ratchet 448→600:** EBS/snapshot mgmt (17), VPC endpoint services + CIDR + secondary nets + security-group-VPC + encryption control (29), instance mgmt — Connect endpoints/serial-console/IMDS-defaults/event-notify/monitor (18), Reserved Instances + Capacity Reservation billing/topology/splitting + Capacity Blocks (29), FPGA images + AllowedImagesSettings + ImageBlockPublicAccess + deregistration-protection + bundle/conversion/store-image tasks (22), networking misc — address transfers/BYOIP/IPv4-pools/NAT-gateway-addresses/trunk-interfaces/carrier-gateways/COIP/NIC-permissions/VGW-propagation/VPN-concentrators/ModifyManagedPrefixList (37).
- **Nineteen AWS services now at 100%.**
- **Boyscout (BUG-2205):** a focused audit of the older EC2/RDS/CloudFront core found 3 real silent-fallback / wrong-error bugs (verified against real AWS error codes, not guessed) — CreateRouteTable fabricated a `10.0.0.0/16` CIDR for a non-existent VPC (now `InvalidVpcID.NotFound`); AssociateAddress silently accepted a non-existent InstanceId (now `InvalidInstanceID.NotFound`); CreateNatGateway returned the wrong error + left VpcId empty for a missing subnet (now `InvalidSubnetID.NotFound`). Verified no regressions across the full EC2 suite. The 4th finding (cfWriteXML response-write error ignore) was classified non-actionable (idiomatic). The CloudFront agent's CreateStreamingDistributionWithTags 400 was a real bug fixed in the same PR.
- Tests: aws sim/sdk/cli build/lint(0)/unit green; contract + cli-shard + all conformance tests pass; spec-shape validator 0 divergences.

**Next candidates:** **EC2 (600/769)** is the biggest remaining floor — keep ratcheting (remaining VPN/customer-gateway extras, EC2-Classic/managed-prefix-list extras, snapshot-presentation, more instance-attribute/launch ops, the few remaining capacity-reservation niche ops). Mid-size floors with headroom: CloudWatch Logs (104/113 — mostly streaming left), SSM (43/146), Amplify, DynamoDB, S3. Then live-cloud (1075). Open GitHub issues: #394 (azuread upstream-blocked).

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
