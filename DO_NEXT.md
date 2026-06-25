# Do Next

Status [STATUS.md](STATUS.md) - roadmap [PLAN.md](PLAN.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Current branch

`feat/ratchet-up-13` — **event-stream ops + drive CloudWatch/Organizations/SQS/Kinesis to 100% + boyscout fixes (BUG-2211).** Implemented the genuinely-streaming ops faithfully and ratcheted the next mid-size floors, all spec-validated against the vendored `aws-sdk-go-v2` Smithy models (0 divergences) + real SDK/CLI.

- **The event-stream ops are faithfully implementable** — the sim already has `awsEventStreamMessage` (used by Lambda InvokeWithResponseStream). All four deferred streaming ops now stream real `application/vnd.amazon.eventstream` framing that the **real aws-sdk-go-v2 client decodes**: CloudWatch Logs StartLiveTail + GetLogObject (→113/113), Kinesis SubscribeToShard (→39/39), S3 SelectObjectContent (a real minimal S3 Select over CSV/JSON-lines, →104). Two load-bearing details: the SDK reader blocks on a mandatory `initial-response` frame, and StartLiveTail/GetLogObject need `DisableEndpointHostPrefix` (they carry `@endpoint(hostPrefix:"stream-")`).
- **Five more services to 100%:** CloudWatch monitoring 38→49 (datasets/CMK, OTel-enrichment, managed-insight-rules, GetInsightRuleReport, DescribeAlarmContributors, GetMetricWidgetImage→real PNG; both awsJson + rpc-v2-cbor), Organizations 53→63 (GovCloud account, responsibility-transfer family, effective-policy validation), SQS 19→23 (DLQ-redrive message-move-task family), CloudWatch Logs 111→113, Kinesis 38→39.
- **Twenty-six AWS services now at 100%.** WriteGetObjectResponse stays deferred (S3 Object Lambda dedicated endpoint, like the S3 Express ops).
- The spec-shape validator gained a `vnd.amazon.eventstream` exemption (the restXml/query shape-validators skip framed event-stream bodies; the SDK decoder validates them — the awsJson path already skipped non-json content-types).
- **Boyscout (BUG-2209/2210):** fixed 3 real request-time concurrency races — added an atomic `Upsert(id, fn)` to the `Store[T]` interface (single-locked create-or-modify) for PutMetricData's append, and made PurgeQueue clear via the atomic `Update` — plus 3 `ReadJSON` fail-loud swallows (Organizations CreateOrganization / ListDelegatedAdministrators, CloudWatch ListDashboards). Classified 2 non-bugs (unreachable `cbor.Marshal` of empty map; idiomatic ignored response-writer error).
- Tests: aws sim/sdk/cli build/lint(0)/unit green; contract + cli-shard + all conformance + streaming spec-validator (0 divergences) pass.

**Next candidates:** essentially every *measured* service is at its faithful max — the only remaining gaps are genuinely-unhostable-on-the-regional-sim ops (S3 WriteGetObjectResponse Object-Lambda dedicated endpoint + the 2 S3 Express dedicated-endpoint ops; Glue's 3 SaaS-connector ops). Highest-value next work: **audit the gate for any AWS service the sim implements but does not yet measure** (add it to `serviceCoverageFloor`/`serviceConformanceCatalog`/`restConformanceSources` so its coverage is gated), or pick up the **live-cloud track (BUG-1075)**. Open GitHub issues: #394 (azuread upstream-blocked).

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
