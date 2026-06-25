# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`fix/ci-flakes-and-open-issues` — **CI flakiness sweep + ELBv2 open issues + CloudTrail fix (BUG-2216/2217/2218/2219).** A `sim (aws sdk)` job flaked after #686 (failed once, passed on a no-change re-run), so this branch hardened flaky patterns across all three sim test suites + closed the two unblocked open GitHub issues + an incidental CloudTrail gap.

- **Flaky-pattern hardening (BUG-2216).** AWS: ENI-attach 800ms-sleep→poll; EBS-snapshot poll 2s→60s (likeliest culprit); DynamoDB-Local readiness 30s→120s. GCP: Cloud Run job-execution 3s-sleep→poll; job/DNS `Eventually` 10-15s→30-60s. Azure: AppTraces 100ms-sleep→`Eventually`; async-op 2s→30s, CLI-JSON 5s→30s, Service Bus + Event Hubs AMQP receive 5s→30s, ACA/ACI log+DNS 10s→30s. No assertion weakened; genuine cloud-delay + timestamp-separator sleeps left alone.
- **ELBv2 #685 (BUG-2217)** — omit HealthCheck `Matcher` for non-HTTP/HTTPS health checks (was breaking terraform idempotency on TCP target groups). SDK+CLI+terraform-idempotency.
- **ELBv2 #683 (BUG-2218)** — real NLB raw-TCP data plane (`elbv2_nlb_proxy.go`): TCP/TCP_UDP listeners bind a `net.Listener`, resolve a healthy target per-connection, `io.Copy` both ways. TLS/UDP honestly excluded. Proven by a raw-bytes round-trip test.
- **CloudTrail (BUG-2219)** — added the missing ElastiCache `2015-02-02` eventSource mapping (all ElastiCache events were being dropped from the trail); tightened the coverage test.
- Verified: aws+gcp+azure build/lint(0); AWS conformance + ELBv2 spec-validator(0) + terraform idempotency + cli-shard guard; touched suites rerun green.

**Next candidates:** keep ratcheting GCP coverage (the prior arc — Spanner/SQL Admin/the small-gap batch, then big surfaces + the Azure coverage gate), or the live-cloud track (BUG-1075). Open GitHub issues: only #394 (azuread, upstream-blocked) remains.

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
