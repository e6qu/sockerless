# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/azure-ratchet-1` — **first Azure service ratchet (BUG-2224).** The Azure sim had a coverage gate (`azureMethodFloor`, #689) but no services ratcheted yet; this drives the first wave up against it, spec-validated against vendored ARM Swagger (0 new violations) + real `azure-sdk-for-go`.

- **Container Apps** (Microsoft.App): containerApps 5→11/11, jobs 9→12/12, managedEnvironments 3→19/19 — all 100% (start/stop do real replica work; certificates + managedCertificates CRUD).
- **Container Registry** (Microsoft.ContainerRegistry): 2025-11-01 12→58/58, 2023-07-01 12→52/52, registrytasks 2→25/25 — all 100% (replications/scopeMaps/tokens/credentialSets/connectedRegistries + webhooks with write-only secrets surfaced only via getCallbackConfig).
- **Networking** (Microsoft.Network): NSG/NAT-Gateway/PublicIP-Prefix/RouteTable to 100%; virtualNetwork 6→18/21, loadBalancer 9→22/27, NIC 4→14/15, publicIP 4→6/9 (~50 routes; cross-references read back faithfully).
- **Service Bus + Event Hubs**: all 8 docs to 100% (namespaces CRUD/list-by-sub/PATCH, authorizationRules listKeys/regenerateKeys, disasterRecovery break-pairing/failover, migrationConfigs, networkRuleSets, PEC) — plus a latent PascalCase casing bug (`AuthorizationRules`/`ListKeys`) the new SDK tests exposed, and two stub-404 DR/migration handlers converted to store-backed.
- **Azure total 630→857/2597 (24%→33%); all three sims now actively ratcheting.** Each uses the existing `issueAzureAsyncOperation` LRO helper; integration reconciled floors from one measured pass + a literal-path doc block for the 4 subscription-wide list ops. azure build/lint(0)/dupl(0) + route-validity + doc/spec-consumption + coverage-floor gates pass.
- **Boyscout (BUG-2225, CI-caught on this branch):** EC2 `RunInstances` now honors `ClientToken` idempotency — a retried call (the aws-sdk-go-v2 auto-fills + re-sends the token on every retry) replays the original reservation instead of launching a duplicate batch. Fixed a real flake where `TestEC2_DescribeInstancesPagination` saw doubled instances under CI retry load.

**Next candidates:** keep ratcheting Azure (Storage, Cosmos DB, Redis, Key Vault, remaining networking, EventGrid, APIM) and GCP big surfaces (Logging/Bigtable/Cloud Run/CRM remainders). Or the live-cloud track (BUG-1075). Open GitHub issues: only #394 (azuread, upstream-blocked).

---
### Prior branch (merged #692): ELBv2 NLB stable DNSName (#691, BUG-2223)
Reverted #683's host:port DNSName hijack — DescribeLoadBalancers returns the stable AWS-shaped hostname again; reachability via listener-port bind + ExtraHosts hostname resolution (per-NLB loopback IP on Linux). Plus the appdata CLI shard split (flakiness).

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
