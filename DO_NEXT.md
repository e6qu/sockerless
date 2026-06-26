# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`fix/elbv2-nlb-stable-dnsname` — **ELBv2 NLB DescribeLoadBalancers returned proxy host:port as DNSName (#691, BUG-2223).**

- Regression from #683 (the NLB raw-TCP data plane): `elbv2ReportedDNSName` overrode the stable AWS-shaped `DNSName` with the proxy's dialable `host:port` for any NLB with a running stream listener. The proxy bound an ephemeral port → reported DNSName changed every run (perpetual `terraform-provider-aws` `dns_name` drift) and `host:port` is not a valid Route53 alias target.
- Fix (reachability made faithful instead of hijacking DNSName): DescribeLoadBalancers always returns the stable `<name>-<id>.elb.<region>.amazonaws.com` hostname (NLB + ALB; `CanonicalHostedZoneId` present so a Route53 alias is valid). The NLB stream proxy binds the **listener's configured port** on a stable host — `127.0.0.1`, or (on Linux, when that port is taken by another NLB) a distinct per-NLB `127.0.0.0/8` address leased via the new `realexec.ReserveNLBLoopbackIPv4` (the real-NLB "each LB has its own IP" shape, collision-free). The AWS-shaped DNSName resolves to that proxy host through injected workload-container hosts entries (`elbv2NLBHostEntries`/`elbv2WorkloadExtraHosts` merged onto the ECS workload `ExtraHosts`) — the faithful production consumer of `elbv2NLBProxyAddress`, so the deadcode gate stays green. CreateListener is best-effort on bind (real AWS never fails CreateListener for a sim-local socket issue).
- Tests: #691 contract guard (DNSName AWS-shaped, colon-free, stable across calls, CanonicalHostedZoneId present); the #683 raw-TCP capability test reworked to resolve the DNSName via the production hosts mapping then connect on the listener port; idempotency-fidelity terraform stack gained a `network` `aws_lb` + TCP listener/target-group + a Route53 alias targeting it (real apply + clean plan, 151s). aws+realexec build/lint(0)/deadcode(0) + conformance + spec-validator(0) green. Closes #691.

**Next candidates:** keep ratcheting GCP big surfaces (Logging/Bigtable/Cloud Run/CRM remainders) and **start ratcheting Azure services** now that its gate exists. Or the live-cloud track (BUG-1075). Open GitHub issues: only #394 (azuread, upstream-blocked).

---
### Prior branch (merged #690): ELBv2 TCP target group HealthCheckPath (#688, BUG-2222)
Same HTTP-only class as #685's Matcher — `HealthCheckPath` was defaulted/emitted for every protocol; now omitted for non-HTTP health checks (`elbv2MatcherApplies` → `elbv2HTTPHealthCheck` + `elbv2DefaultedHealthCheckPath`). SDK/CLI + a TCP `health_check` block in the idempotency TF stack.

---
### Prior branch (merged #689): GCP coverage ratchet round 2 + Azure operation-coverage gate (BUG-2220/2221)
Built `azureMethodFloor` in `simulators/azure/azure_coverage_test.go` (the Swagger-spec analogue of `serviceCoverageFloor`/`gcpMethodFloor`, ratchet over 90 swagger files — all three sims now gated; Azure 630/2597 = 24%); GCP ratcheted 2413→3180/5244 (46%→61%) with ~22 services at 100% (Spanner, Cloud SQL v1, VPC Access, ServiceUsage, IAM Credentials, Dataflow to 100%; CRM v3 11→105, Logging 170→480, Bigtable 65→136, Cloud Run/Functions up); plus a smoke-build proxy-retry resilience fix (BUG-2221).

---
### Prior branch (merged #687): CI flake hardening + ELBv2 #685/#683 + CloudTrail (BUG-2216/2217/2218/2219)
- Flaky-pattern hardening across AWS/GCP/Azure test suites (~20 racy waits → poll-until / widened deadlines; no assertion weakened).
- ELBv2 #685: omit HealthCheck `Matcher` for non-HTTP/HTTPS health checks (terraform idempotency). ELBv2 #683: real NLB raw-TCP data plane, made discoverable via DescribeLoadBalancers (a client `net.Dial`s the reported endpoint). CloudTrail: added the missing ElastiCache `2015-02-02` eventSource mapping (events were being dropped).

---
### Prior branch (merged #686): GCP operation-coverage gate + ratchet (BUG-2214/2215)
Brought the GCP simulator's conformance gate up to AWS parity, all spec-validated against the Discovery schemas (0 new divergences) + real Google Cloud Go SDK.

- **GCP had route-validity + doc-consumption gates but no operation-coverage ratchet.** Built `gcpMethodFloor` in `gcp_coverage_test.go` — per vendored Google Discovery document, it counts how many REST methods the sim implements (a method is covered when a registered route matches its HTTP-method + normalized path under the same `matchSegs` rules the route-validity gate uses) and locks the count with an exact-equality ratchet. `TestServiceConformance_GCPCoverage` logs the per-doc fraction; `TestServiceConformance_GCPCoverageFloor` is the ratchet.
- **12 mid-size services ratcheted** (2 rounds, one service file per agent): **Cloud Build 104→130/130, Memorystore Redis 64→90/90, Firestore admin 89→112/112, Cloud Storage JSON 32→84/84 (all 100%)**; Cloud KMS 122→157/166 (real Go-stdlib crypto for mac/raw/asymmetric/generateRandomBytes; honest metadata-only for EKM/HSM/post-quantum decapsulate), IAM admin 204→264/266 (workload/workforce identity pools, OAuth clients, custom roles, SA keys), Artifact Registry 97→144/147 (packages/versions/tags/files/rules/attachments), Eventarc 97→124/132, BigQuery 38→86/95, Cloud DNS 24→74/80 (real DNSSEC DS digests), Pub/Sub 77→86/92 (schemas), Secret Manager 50→60/64 (regional).
- **GCP coverage 1986→2413/5244 (38%→46%); 6 GCP services now at 100%.**
- The consistently-uncovered remainder per service is the `{+name}`/`{+resource}` reserved-expansion *template alternates* the Discovery docs list alongside each flatPath — the flatPath form every real client uses is covered; matching the template form would need an over-broad catch-all the route-validity gate forbids.
- Integration: the whole module built once all agents finished; floor bumps reconciled from a single measured-coverage pass; 2 staticcheck (dns ECDSA embedded-field) + 1 unused func (iam) cleared.
- Tests: gcp sim build/lint(0) green; route-validity + doc-consumption + coverage-floor gates pass; per-service spec-validator 0 new violations.

**Next candidates:** keep ratcheting GCP — the larger mid-size services (**Spanner 186/198, SQL Admin 136/148**, the small-gap batch **API Gateway 54/60 / ServiceUsage 19/20 / VPC Access 15/16 / IAM Credentials 11/14**), then the big surfaces (**Cloud Run 61/152, Logging 154/508, Bigtable Admin 62/162, Cloud Resource Manager 11/124, Cloud Functions 15/42, Dataflow 8/84**; Compute 174/1994 is enormous — ratchet selectively). Then the **Azure simulator** (no coverage gate yet — build the equivalent), and the **live-cloud track (BUG-1075)**. Open GitHub issues: #394 (azuread upstream-blocked).

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
