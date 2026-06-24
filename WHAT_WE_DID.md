# Sockerless - What We Built

Roadmap [PLAN.md](PLAN.md) - status [STATUS.md](STATUS.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

Detailed historical narrative lives in PR descriptions and `git log`. This file kept the recent chain and a compact foundation summary.

## 2026-06-24 - Drive Batch / CloudTrail / CodeBuild / WAFv2 / ECR to 100% (BUG-2199)

Drove five AWS services to one hundred percent of their conformance model — roughly one hundred thirty-eight operations — in a single five-agent fan-out, each agent handed the exact missing-operation list and tasked with closing it. Every shape was validated against the vendored `aws-sdk-go-v2` Smithy models (zero divergences) and exercised by real software-development-kit and command-line-interface round-trips. AWS Batch 24→45/45 (consumable resources, service environments, service jobs that settle through a real state walk, quota shares, and GetJobQueueSnapshot). AWS CloudTrail 23→60/60 (the CloudTrail Lake surface — event data stores with a real lifecycle, and Lake queries that run synchronously over the same recorded events LookupEvents already serves, with `eventName`/`eventSource` predicates scoping the rows — plus dashboards, imports, federation, a resource policy mirrored into the IAM gate, and organization delegated-admin registration). AWS CodeBuild 22→59/59 (build batches, compute fleets, sandboxes whose command executions run as real local processes capturing real exit codes, webhooks, report test-cases/code-coverage/trend aggregation, and a resource policy mirrored into the IAM gate). AWS WAFv2 32→55/55 (API keys, a real web-ACL-capacity-unit computation in CheckCapacity recursing through the rule statements, the real AWS Managed Rules catalog, managed rule sets, permission policies, and mobile SDK releases). Amazon ECR 38→58/58 (repository creation templates, registry scanning configuration, signing configuration, pull-time update exclusions, pull-through-cache update/validate, account settings, and image referrers).

All five coverage floors now equal their Smithy model maximum, bringing the number of AWS services the gate measures at one hundred percent to seven (these five plus KMS and ELBv2 from the previous PR). Where the simulator runs no backing infrastructure — ECR image signing and OCI referrers, WAFv2 live-traffic statistics, CloudTrail digest public keys — the operations return honest empty results shaped exactly as the SDK models require, never fabricated data. The newest operations (2024–2025 service launches such as Batch service environments and quota shares, the newest ECR registry ops, CloudTrail event configuration) postdate the pinned local `aws` CLI; their CLI tests were verified against a latest-CLI virtual environment (CI installs the latest v2), and a few are exercised SDK-only where even the latest CLI lacks the subcommand. No agent was cut off and no stub files were created.

## 2026-06-24 - Ratchet up RDS/Glue/Lambda/API Gateway/CloudFront/ElastiCache; complete KMS + ELBv2 (BUG-2198)

Raised eight AWS services by roughly one hundred sixty-five operations in two agent rounds, every shape validated against the vendored `aws-sdk-go-v2` Smithy models (zero divergences) and exercised by real software-development-kit and command-line-interface round-trips. Six services had their coverage floors bumped: Amazon Relational Database Service (RDS) 40→64 (instance and cluster start/stop/failover, global clusters, event subscriptions, DB cluster endpoints, parameter detail, cluster-snapshot copy — and a fixed `GlobalClusterMember` list-element wire-shape bug), AWS Glue 78→102 (table versions, real partition indexes replacing an empty stub, column statistics, a resource policy mirrored into the IAM gate, data-catalog encryption, schema versions), AWS Lambda 37→62 (function event-invoke configuration, provisioned concurrency, code-signing configurations, runtime management, recursion config, layer-version permissions), Amazon API Gateway v1 28→62 and v2 23→44 (API keys, usage plans and keys, models, request validators, authorizers, domain names, API mappings, VPC links — the agent caught that v1 list collections serialize the singular `item` wire key and v2 members are lower-camelCase), Amazon CloudFront 52→67 (origin access identities, continuous deployment policies, monitoring subscriptions), and Amazon ElastiCache 25→41 (snapshots, users and user groups, parameter detail).

Two services were driven to one hundred percent of their conformance model and had their exact-list catalog entries emptied. AWS Key Management Service (KMS) reached 54/54 by implementing the asymmetric-cryptography surface with **real Go standard-library cryptography** — Sign and Verify over RSA-PSS/PKCS1v15 and ECDSA (a real signature verifies and a tampered message raises `KMSInvalidSignatureException`), GenerateMac and VerifyMac over HMAC, GenerateDataKeyPair producing a real RSA/ECC keypair with the private key wrapped under the customer master key's envelope, and DeriveSharedSecret over `crypto/ecdh` — plus custom key stores with a connection-state machine, grant retirement, and multi-region keys. AWS Elastic Load Balancing v2 (ELBv2) reached 51/51 by implementing the mutual-TLS trust-store surface (trust stores, CA-certificate bundles, numbered revocation lists, associations) plus DescribeSSLPolicies returning the real set of AWS predefined SSL policies. Amazon Kinesis remains at 38/39: its only gap is the HTTP/2 event-stream operation `SubscribeToShard`, which was deliberately not faked.

Process: two rounds of four-and-three-agent fan-outs over disjoint files. The KMS agent's transient compile-stub was deleted before commit, and a single cross-file `wastedassign` introduced by the ELBv2 work was fixed at integration; no agent was cut off.

## 2026-06-24 - Ratchet up EC2 / ECR / Auto Scaling / CloudWatch Logs (BUG-2197)

Raised four floored services by roughly sixty-two operations, every shape validated against the vendored `aws-sdk-go-v2` Smithy models (zero divergences) and exercised by real software-development-kit and command-line-interface round-trips. Amazon Elastic Compute Cloud (EC2) 102→122 (network access-control lists and their entries, Virtual Private Cloud peering connections with the pending-acceptance→active transition, managed prefix lists, flow logs, and egress-only internet gateways). Amazon Elastic Container Registry (ECR) 26→38 (lifecycle-policy previews, repository and registry policies, image scanning that returns an honest empty findings result for a stored image rather than fabricated vulnerabilities, replication configuration, image-tag mutability, and tags). AWS Auto Scaling 13→25 (scaling policies whose `ExecutePolicy` actually adjusts the group's desired capacity per the adjustment type, scheduled actions, lifecycle hooks, and instance health/termination). Amazon CloudWatch Logs 18→36 (metric filters with `TestMetricFilter`, subscription filters, export tasks, the data-protection policy, both the legacy and resource-ARN tagging APIs, and DeleteLogStream/DeleteRetentionPolicy). The conformance gate now measures roughly thirty-seven AWS services with steadily-rising floors.

Process: a four-agent fan-out over disjoint files (EC2, ECR, Auto Scaling, CloudWatch Logs), with each agent told explicitly not to create `zz_*.go` stub files — the build-stub problem from the previous PR did not recur, and no agent was cut off.

## 2026-06-24 - Raise CloudTrail / CodeBuild / WAFv2 + ratchet up Glue (BUG-2196)

Raised four floored services by roughly fifty operations, every shape validated against the vendored `aws-sdk-go-v2` Smithy models (zero divergences) and exercised by real software-development-kit and command-line-interface round-trips. CloudTrail 16→23 (ListTrails, insight selectors, and CloudTrail Lake channels); along the way the new tests surfaced two pre-existing fidelity bugs, both fixed: UpdateTrail emitted a `HomeRegion` field its response shape does not define, and GetEventSelectors/GetInsightSelectors read the request's `Name` key when the wire shape uses `TrailName`, so the trail was never found. CodeBuild 9→22 (stop and retry build, report groups, reports — produced for real by parsing the buildspec's `reports:` section so a completed build emits one report per referenced group, never a synthetic one — and source credentials, whose secret token is never echoed back). WAFv2 28→32 (the logging-configuration operations, keyed by the CLOUDFRONT/REGIONAL scope). Glue 52→78 (security configurations, workflows and their runs, classifiers, user-defined functions, and the schema registry). The conformance gate now measures roughly thirty-seven AWS services with higher floors.

Process: a four-agent fan-out over disjoint files — a deliberately smaller batch after the previous PR's six-agent run was halted mid-flight by an upstream weekly usage limit — and this time no agent was cut off. A stale `zz_tmp_glue_stubs.go` (sixteen lines of placeholder handlers labelled "DELETE before commit", left over from earlier scaffolding) redeclared the now-real Glue handlers and broke the package build; it was removed.

## 2026-06-24 - Ratchet up EC2/RDS/Glue/Lambda/Batch/API Gateway + add CloudFront (BUG-2195)

Raised the coverage floor of six services with more high-value operations, every shape validated against the vendored `aws-sdk-go-v2` Smithy models (zero divergences) and exercised by real software-development-kit and command-line-interface round-trips: Amazon Elastic Compute Cloud (EC2) 91→102 (Virtual Private Cloud endpoints plus Amazon Machine Image, placement-group, and DHCP-options round-trips), Relational Database Service (RDS) 25→40 (database-cluster snapshots, cluster parameter and option groups, read replicas, events, and engine metadata), Glue 30→52 (crawlers, jobs and job runs, triggers, and connections), AWS Lambda 23→37 (versions and aliases, event-source mappings, layers, reserved concurrency, and function URLs), AWS Batch 19→24 (job queues, job definitions, and jobs), and Amazon API Gateway version 1 and 2 (deployments, stages, and route updates). Amazon CloudFront (52/167) was added to the conformance gate through the REST registry. The simulator-tests contract hook now maps a `cloudTrailRecordedREST("Op", …)` route to its named operation, so a REST route whose wire path is a lowercase subresource is recognized as tested when its named operation is exercised. The gate now measures roughly thirty-seven AWS services.

Process note: five of the six implementation agents were cut off mid-run by an upstream weekly usage limit, leaving compiling-but-partially-tested work. It was completed by hand — the missing Lambda subresource tests were written, and four cut-off test defects were fixed: Glue `GetTags` now resolves crawler and trigger Amazon Resource Names (the crawlers and triggers already stored their tags; only the lookup switch was missing those cases); the EC2 Amazon Machine Image test confirms deregistration the store-faithful way (a second deregister reports NotFound) rather than through `DescribeImages`, which deliberately synthesizes an image for any requested id to support terraform `data.aws_ami` lookups; the EC2 DHCP-options test's JMESPath was corrected (the value list holds `AttributeValue` objects, so the query needed `.Values[0].Value`); and a Glue command-line test's local response struct was missing a `Description` field. The takeaway: a single weekly-limit event can halt a large fan-out mid-flight, so prefer fewer or smaller agents, or land a partial green state sooner.

## 2026-06-24 - Ratchet-up the floored services + measure the restJson1 services (BUG-2194)

A two-part pass, with every operation grounded in Amazon Web Services' own `aws-sdk-go-v2` Smithy models (the vendored `specs/cloud-api/aws/*.smithy.json.gz`) and verified by the runtime spec-shape validator (zero divergences) plus real software-development-kit (SDK) and `aws` command-line-interface (CLI) round-trips — no guessed shapes. **Part A** raised the coverage floor of six services by 77 operations of faithful control-plane create/read/update/delete on real stores: Amazon Elastic Compute Cloud (EC2) 88→91 (Virtual Private Cloud endpoints — the rest of the targeted networking surface was already implemented), Relational Database Service (RDS) 13→25 (database clusters, subnet and parameter groups, reboot, and tagging across every resource type), ElastiCache 13→25 (replication groups, subnet and parameter groups, reboot, and a tag rework that resolves any ElastiCache ARN), Glue 19→30 (the Data Catalog database/table/partition tree with batch operations), Route 53 10→33 (health checks, traffic policies, Virtual Private Cloud associations, query-logging configurations, and geolocations), and Elastic File System (EFS) 13→29 (file-system and backup policies, replication, account preferences, and both resource-ARN and legacy tagging). **Part B** brought the restJson1 services into the conformance gate by mapping their CloudTrail event sources in `restConformanceSources` and adding coverage floors: AWS Lambda (23/85), AWS Batch (19/45), Amazon API Gateway version 1 (28/124) and version 2 (22/103), Amplify (37/37, complete), and EventBridge Scheduler (9/12 — its `schedulerRecorded` wrapper now also records into the `restRegisteredOps` registry). The service-conformance gate now measures roughly thirty-six AWS services (fifteen tracked operation-by-operation, S3, and twenty by coverage floor).

The simulator-tests contract hook was genuinely improved rather than worked around: `scripts/check-simulator-tests.sh` now reads the `cloudTrailRecordedREST("Op", …)` operation name off a REST route's registration, so a route whose wire path is a lowercase subresource (for example `DELETE /…/file-systems/{id}/policy`, which the SDK invokes as `DeleteFileSystemPolicy`) is recognized as tested when its named operation is exercised — correct for every REST service, not just the one that surfaced it.

## 2026-06-24 - Conformance coverage for the remaining query/REST AWS services (BUG-2193)

Measurement infrastructure only — no new operations — extending the service-conformance gate to **every remaining un-ratcheted AWS service** the simulator registers. Three measurement paths now exist. **awsJson** services are read from the JSON router's targets. **awsQuery / ec2Query** services are read from the query router — and `serviceRegisteredOps` now also reads the router's unversioned legacy bucket (services registered with the plain `Register` rather than `RegisterVersioned` — Amazon Elastic Compute Cloud (EC2), Security Token Service (STS), and others — landed there and were silently measured as zero); intersecting that bucket with each model's own operation set attributes the actions to the right service (EC2 88/769, STS 4/11). **REST** services name their operation as a constant passed to `cloudTrailRecordedREST("Op", "<source>.amazonaws.com", …)` at registration, which now records into a `restRegisteredOps` registry keyed by CloudTrail event source; `restConformanceSources` maps a Smithy shape to its source so the gate measures path-based REST services the same way it reads the routers — Amazon Route 53 (10/71) and Elastic File System (EFS) (13/31). Amazon S3 keeps its bespoke `s3ImplementedOps` harness because it composes its operation name dynamically from method + path + subresource rather than naming it as a constant.

A second ratchet shape accompanies the exact missing-list catalog: a **coverage floor** (`serviceCoverageFloor` + `TestServiceConformance_CoverageFloor`) that locks the implemented-operation *count* for the awsQuery/ec2Query giants (EC2's 769-operation surface, RDS, Glue) and the REST services, where an exact list of several hundred unimplemented operations would bloat the catalog. The count must equal the floor: a drop is a regression, and implementing more operations ratchets the floor up. Fourteen services are on the floor; the conformance gate now measures roughly thirty AWS services (fifteen tracked op-by-op in `serviceConformanceCatalog`, S3, and fourteen by coverage floor). `docs/SERVICE_CONFORMANCE.md` documents the three measurement paths and the two ratchet shapes.

## 2026-06-23 - CloudWatch / Organizations / SSM op completeness + conformance-ratchet expansion to 14 services + S3 (BUG-2192)

A 114-operation completeness pass extending the service-conformance ratchet from 11 Amazon Web Services (AWS) service slices plus Amazon Simple Storage Service (S3) to 14 plus S3. **Amazon CloudWatch** went 10→38/49: alarm management (enable/disable alarm actions, set alarm state, describe alarm history and alarms-for-metric, composite alarms), metric streams, anomaly detectors, insight rules, `GetMetricData`, alarm mute rules, and cross-resource tagging — each served on all three CloudWatch transports because the Go software-development kit (SDK) speaks rpc-v2-cbor, newer `aws` command-line interface (CLI) versions speak awsJson, and older ones speak the legacy query protocol. **AWS Organizations** went 2→53/63: the previous two-operation stub was rewritten into a full stateful resource tree (a default organization, root, and management account are materialized at startup so the `aws:PrincipalOrgID` condition key keeps resolving) — accounts, organizational units, policies with attach/detach and policy-type enablement and effective-policy lookup, handshakes, delegated administrators, service access, a resource policy mirrored into the Identity and Access Management (IAM) enforcement gate, and tags. **AWS Systems Manager (SSM)** went 10→43/146: versioned documents, maintenance windows with their targets and tasks, patch baselines, service settings, and resource data sync, on top of the existing Parameter Store. Every operation is faithful control-plane create/read/update/delete on real backing stores with an SDK test and a CLI test; the runtime spec-shape validator reports zero divergences across all new operations (a `MaintenanceWindowIdentity` field divergence was caught and fixed during validation).

The genuinely-large execution subsystems were locked in the ratchet rather than speculatively implemented (the cloud-slice principle): CloudWatch's dataset/KMS, OpenTelemetry-enrichment, and managed-insight-rule surfaces; Organizations' GovCloud-account and responsibility-transfer surfaces; and SSM's run/automation/session/inventory/compliance/ops-item/association subsystems (103 operations).

Process: built by three concurrent single-purpose agents over disjoint files (no git operations), with `service_conformance_test.go` (the ratchet) reconciled by hand at integration — the catalog entries for the three new services' gap sets were generated programmatically from the coverage measurement to avoid hand-transcription error. The CloudWatch agent also caught a real build-blocker: a new SSM file was named `ssm_maintenance_windows.go`, whose `_windows.go` suffix is a Go operating-system build constraint that silently excluded it from the macOS/Linux build (so the package would not compile); it was renamed to `ssm_maintenance.go`. The CloudWatch rpc-v2-cbor transport is a single dynamic prefix dispatcher (`/service/.../operation/`+op in a loop), which the static contract-hook scanner cannot map to individual operations, so its route is recorded in `tests-exempt.txt` — the operations it serves are individually covered through their awsJson registrations and tests.

## 2026-06-23 - S3 op completeness + conformance-ratchet expansion to 11 services + S3 (BUG-2191)

A ~85-operation completeness pass that extended the service-conformance ratchet from 5 Amazon Web Services (AWS) service slices to 11 plus Amazon Simple Storage Service (S3). **S3** (a REST surface, measured by its own `s3ImplementedOps` harness): closed 13 high-value operations — object access-control lists (get/put), object lock configuration / retention / legal-hold (get/put), `GetObjectAttributes`, `GetObjectTorrent`, the version-1 `ListObjects`, `RestoreObject`, and `UploadPartCopy` — in a new `s3_object_subresources.go` plus the request→operation-name composition in `s3.go`; the runtime restXML spec validator gained required-header disambiguation so it distinguishes `UploadPartCopy` (which carries `x-amz-copy-source`) from `UploadPart`. S3 coverage went 78→**91/107**; the 16 remaining gaps (S3 Express directory buckets, the bucket Metadata-table feature, attribute-based access control, S3 Object Lambda, S3 Select) are ratcheted. The harness now also drives `list-type=2` and an `uploadId`+copy-source request so `ListObjectsV2`/`UploadPartCopy` are detected.

**Conformance-complete (zero gaps):** AWS Step Functions (37/37 — activities + the task-token lifecycle, state-machine versions and aliases, `SendTask*`, map runs, `TestState`/`StartSyncExecution` on the real interpreter), AWS Certificate Manager (17/17 — export/get/revoke a certificate with real X.509 material, account configuration, search), AWS Secrets Manager (23/23 — a resource policy mirrored into the IAM enforcement gate, rotation, version-staging labels, cross-Region replication, batch-get), and Application Auto Scaling (14/14 — scheduled actions, scaling activities). **Near-complete:** Amazon Kinesis Data Streams 38/39 (enhanced fan-out consumers, resource policy, shard merge/split, tags, stream mode — only the HTTP/2 event-stream `SubscribeToShard` is locked), and AWS Key Management Service (KMS) 35/54 (key management — enable/disable, cancel-deletion, alias/description updates, rotation, import key material via a real RSA wrapping key, `GenerateRandom`; the asymmetric-cryptography operations and CloudHSM custom key stores are locked). AWS Elastic Load Balancing v2 (36/51) was added to the ratchet to lock its mutual-TLS trust-store gaps. Every operation is faithful control-plane create/read/update/delete on real backing stores with a software-development-kit (SDK) test and a command-line-interface (CLI) test; the runtime spec-shape validator reports zero divergences across all new operations.

The work was produced by five concurrent single-purpose agents over disjoint files (no git operations — the lesson from the earlier shared-tree corruption), with `service_conformance_test.go` (the ratchet) reconciled by hand at integration. Six CLI tests exercise operations the locally-installed `aws` CLI (2.26.6) predates (ACM revoke/search-certificate; Kinesis tag-resource/account-settings/update-max-record-size/update-stream-warm-throughput) — SDK-verified and confirmed against CLI 2.35.11, the version continuous integration installs.

## 2026-06-23 - ECS op completeness + fully-qualified-naming rule (BUG-2190)

Closed Amazon Elastic Container Service (ECS)'s entire remaining operation surface — the 47 operations the conformance ratchet tracked — as faithful control-plane create/read/update/delete on real backing stores, never fakes: capacity providers, task sets (and `UpdateServicePrimaryTaskSet`), container instances (register/deregister/describe/list, state + agent updates, the `Submit*StateChange` agent-poll operations, and `DiscoverPollEndpoint`), account settings, attributes, task protection, daemons (with their deployments, revisions, and daemon task definitions), and service deployments (describe/list/stop/continue, `DescribeServiceRevisions`, `ListServicesByNamespace`). ECS's entry in the service-conformance ratchet drops to zero gaps. Every operation has a software development kit (SDK) test; every operation that the public `aws` command-line interface (CLI) exposes also has a CLI test. The daemon operations and `StopServiceDeployment`/`ContinueServiceDeployment` are preview-only with no public CLI command, so they are SDK-tested only — the simulator-tests contract hook is satisfied by SDK coverage (a single test surface suffices).

Added a naming rule to `AGENTS.md` (§ "Use proper, fully-qualified service and feature names"): call every cloud service and feature by its real, fully-qualified name in prose, comments, doc titles, and test names — the feature AWS launched on 2025-11-21 is **Amazon ECS Express Mode**, not "ExpressGateway". Renamed sockerless's own feature-label test/helper names accordingly (for example `TestECS_CLI_ExpressGatewayLifecycle` → `TestECS_CLI_ExpressModeLifecycle`), while keeping AWS's real API operation names, SDK type names, and CLI command names verbatim as the wire contract (`CreateExpressGatewayService`, `ExpressGatewayServiceConfiguration`, `create-express-gateway-service` — and the simulator types that deliberately mirror those SDK types). A documentation sweep applying the rule across the prose docs follows on the same branch.

(The ECS work was produced by a single background agent — after a prior multi-agent run on the shared tree corrupted uncommitted work via concurrent git-stash/worktree operations, this run used one agent only and was integrated by hand after a mid-run server error: all 47 operations had registered and built, four CLI tests needed fixing — tolerant cleanups, a de-brittled list assertion, and removing CLI calls for the preview-only operations the public CLI lacks.)

## 2026-06-23 - S3/REST conformance harness + DynamoDB/EventBridge/SNS op completeness + IAM resource-ARN (BUG-2189)

A three-part service-completeness pass. **(1) S3/REST op-coverage harness.** S3 is REST, so its operation is composed from method + path + query subresource at request time (`s3BucketOperationName`/`s3ObjectOperationName`) rather than registered on a router. `s3ImplementedOps` drives those functions over the method × subresource matrix to enumerate the implemented set, normalizes the sim's "Bucket"-infix op names to the API-canonical names, and `TestServiceConformance_S3Ratchet` locks S3's gap set against the vendored Smithy model (78/107; the 29 gaps — S3 Express directory buckets, the bucket Metadata-table feature, ABAC, Object Lambda, S3 Select, object ACL/lock/retention/legal-hold — are documented, mostly newer/niche). **(2) Operation completeness.** Implemented every remaining DynamoDB (31 — backups/PITR, global tables, a resource-based policy mirrored into the IAM gate, Kinesis streaming, exports/imports, contributor insights, DescribeEndpoints), EventBridge (28 — API destinations, connections, global endpoints, partner + consumer event sources, archive/replay updates), and SNS (23 — mobile platform applications/endpoints, the SMS sandbox with a real Pending→Verified state via a deterministic OTP, SMS attributes + opt-out list, data-protection policy) operation as faithful control-plane CRUD on real backing stores, each with SDK + CLI tests; the conformance ratchet for those three services drops to zero gaps. **(3) IAM resource-ARN derivation.** The enforcement gate (`iamResourceARNForRequest`) now derives the request's target resource ARN for EC2 (volume/snapshot/instance/ENI from the request params), DynamoDB (TableName from the body), Lambda (FunctionName / the REST path), KMS, SecretsManager, Step Functions, and Kinesis, so a least-privilege policy scoped to a specific resource ARN enforces (an SDK test proves `dynamodb:PutItem` is allowed on the granted table's ARN and denied on a different table).

ECS's 47-operation surface (capacity providers, task sets, container instances, account settings, attributes, daemons, service deployments, task protection) was attempted by a parallel agent, but that run was cut off by upstream rate-limiting before its tests landed; rather than ship 47 untested operations, the work was reverted and ECS's gaps stay tracked in the conformance ratchet for a focused follow-on. (Lesson: fan out file-mutating agents in isolated git worktrees, not the shared tree — a concurrent agent's git-stash/worktree dance reverted other agents' uncommitted work and had to be recovered from the stashes.)

## 2026-06-23 - Cosmos remaining phases (BUG-2175 closed) + service-API conformance for AWS pub-sub (BUG-2188)

**Cosmos (2175 → closed).** Completed every remaining Azure Cosmos compliance phase beyond the core lifecycle, each verified against Microsoft's vNext emulator differential. A realistic **RU model** (`cosmos_throughput.go`): `x-ms-request-charge` per operation scaled by serialized item size — read ≈1 RU/KB, write/upsert/replace/patch ≈5 RU/KB, query 2.3 + 0.5/result — plus **throughput offers** implementing the azcosmos `ReadThroughput`/`ReplaceThroughput` flow (manual RU/s + autoscale `offerAutopilotSettings`), with a faithful 404 for a shared-throughput resource (no fabricated default offer). **Consistency + sessions** (`cosmos_consistency.go`): `x-ms-consistency-level` validated against the account max (400 on an escalation past the ceiling); monotonic per-(collection,partition) session tokens issued on writes and echoed on reads for read-your-writes. **Server-side programming** (`cosmos_scripts.go`): sproc/UDF/trigger CRUD with the real REST shapes, and a *faithful* sproc-execution subset — a `createDocument` sproc really creates the document, a count sproc returns the real partition count, a bulk-delete sproc really deletes the partition's docs, pre-triggers really mutate the request document; an uninterpretable body fails loud with the real Cosmos error rather than a faked success. **Change feed + conflicts** (`cosmos_changefeed.go`): `A-IM: Incremental feed` on the docs route with `If-None-Match`/`etag` incremental semantics + 304, and an empty (correct, single-region) conflict feed. Two differentials agree with the emulator; sproc *execution* and missing-sproc 404 are recorded as sim-superiority divergences (the emulator doesn't support server-side script execution) — the sim is never regressed to match the oracle.

**Service-API conformance (2188).** Applied the conformance process (docs/SERVICE_CONFORMANCE.md, established for IAM in #663) to the AWS service slices the user named. `service_conformance_test.go` loads each service's vendored Smithy model — the authoritative operation list — computes which operations the sim's routers register, and reports coverage; `TestServiceConformance_Ratchet` locks each service's set of not-yet-implemented operations so API completeness is a measured, enforced number rather than a claim (SQS 14→19/23, SNS 14→19/42, EventBridge 26→29/57, DynamoDB 26/57, ECS 30/77, with the remaining gaps — global tables, SMS sandbox, daemon tasks, partner event sources — tracked as documented out-of-scope entries). Then closed the high-value **pub-sub** gaps the ratchet flagged, each with SDK + CLI tests: SQS `ChangeMessageVisibility(+Batch)`/`DeleteMessageBatch`/`Add`+`RemovePermission`; SNS `Confirm`/`Get`/`SetSubscriptionAttributes`/`Add`+`RemovePermission`; EventBridge `TestEventPattern`/`ListRuleNamesByTarget`/`UpdateEventBus`. S3 (and other REST services) compose their operation from method+path+subresource at request time, so their op-coverage needs a REST-route enumeration harness — documented as a tracked follow-on rather than mis-measured here.

## 2026-06-23 - IAM conformance system + completeness + service-initiated event delivery (BUG-2186/2187)

Prompted by repeated false "100% IAM" claims, we built the machinery to make completeness *measured* and then drove the IAM policy engine to a real zero-gap number.

**Conformance system (2186).** `simulators/aws/iam_conformance_test.go` encodes the authoritative IAM policy grammar as data — a condition-operator catalog (every real operator + a probe vector) and a condition-key registry (the keys the gate populates; `populated:false` rows *are* the tracked non-conformities) — with three gate tests: `TestIAMConformance_Operators` (every supported operator evaluates; every unsupported one safely no-matches), `TestIAMConformance_GoldenCorpus` (the shared `testdata/iam_conformance_vectors.json` run through the engine), and `TestIAMConformance_Ratchet` (locks the gap set; its failure message is the live non-conformity report). `sdk-tests/iam_conformance_differential_test.go` runs the same corpus through the public `SimulateCustomPolicy` API — against the sim by default (wire path ⇄ engine agreement) and, with `SOCKERLESS_IAM_ORACLE=aws`, against real AWS as the external oracle (coordinates-only, the project's "differs only in coordinates" rule). The repeatable process is written up in [docs/SERVICE_CONFORMANCE.md](docs/SERVICE_CONFORMANCE.md). Then every gap the gate surfaced was closed: the `BinaryEquals` operator (→ 26/26); the global condition keys `aws:CurrentTime`/`EpochTime`/`SecureTransport`/`UserAgent`/`PrincipalArn`/`ResourceAccount`/`PrincipalTag/*` plus `aws:MultiFactorAuthPresent`/`MultiFactorAuthAge` (STS now records MFA on a session); a minimal AWS **Organizations** slice (`organizations.go`: `DescribeOrganization`/`ListAccounts`, with the real vendored Smithy model) backing `aws:PrincipalOrgID`; and `aws:ResourceTag/<k>` resolution for 16 services beyond EC2 (lambda/sqs/sns/rds/dynamodb/s3/elbv2/elasticache/ecr/logs/states/kms/secretsmanager/kinesis/glue/batch). Condition-key *name* lookup was also made case-insensitive (real IAM keys are, so Lambda's `AWS:SourceArn` matches the gate's `aws:SourceArn`). Final: **26/26 operators, 24/24 condition keys, 0 gaps.**

**Service-initiated calls (2187).** The four service-to-service condition keys were unpopulated and the sim recorded SNS subscriptions / EventBridge targets / S3 notifications but never delivered them. Added Service-principal matching (`Principal:{Service:…}` matched against the calling service via `aws:CalledVia`) and `iamEvalServiceInitiated`/`iamAuthorizeServiceDelivery`, then built **real event delivery**: SNS Publish → SQS/Lambda, EventBridge PutEvents → SQS/Lambda (the matched rule's ARN as `aws:SourceArn`), and S3 ObjectCreated/Removed → SQS/SNS/Lambda. Each delivery authorizes against the *target's* resource policy with the service condition context populated, and only happens when the policy admits the source service under its `SourceArn` condition — a message really lands in the queue / the function is really invoked, exactly as real AWS gates these (positive + negative SDK/CLI tests). `aws:ViaAWSService` is `false` on direct client calls. A direct client request is never service-initiated, so the source keys are correctly absent there.

## 2026-06-22 - IAM enforcement: resource-scoped condition keys (#661, BUG-2185)

Follow-up to the IAM enforcement work (#657/#659) and the full condition-operator evaluator (#660). The gate could evaluate conditions but only fed **global** condition keys (`aws:username`/`userid`/`SourceIp`/`RequestedRegion`) into the request context, so a least-privilege grant conditioned on a resource's tags or an ECS cluster could never match — a tag-scoped Allow behaved as a blanket Deny.

`iam_condition_context.go` (`iamPopulateResourceConditionKeys`, called from `iamAuthorize` before `iamEvalDecision`) resolves the request's target resource and populates the resource-scoped / service-specific keys: `aws:ResourceTag/<k>` and the service-prefixed `ec2:ResourceTag/<k>` from the targeted EC2 resource's tags (volume / snapshot / instance / network interface, looked up by the request's id parameter); `ecs:cluster` (the targeted cluster's ARN — ECS is awsJson, so the body is read and restored so the downstream handler still sees it); and `aws:RequestTag/<k>` + `aws:TagKeys` from a tag-on-create / CreateTags request. The operator support from #660 already handled the matching — this just feeds the resource context to it. SDK + CLI tests reproduce #661 exactly: `ec2:DeleteVolume` conditioned on `aws:ResourceTag/edd:managed=true` is denied on an untagged volume and allowed once `CreateTags` adds the tag, and `ecs:StopTask` scoped to one cluster denies a call targeting another.

## 2026-06-22 - 3-cloud IAM/identity fidelity sweep (BUG-2177..2184)

A per-cloud audit of every simulator's IAM/identity surface (AWS IAM/STS, GCP IAM, Azure RBAC + Entra + managed identity) against the real cloud APIs, and a broad set of faithful fixes — each shipped with SDK + CLI tests, no fakes/fallbacks.

**AWS.** The policy evaluator (`iam_policy_sim.go`) went from 7 condition operators to the full real-AWS set — `Numeric*`, `Date*`, `IpAddress`/`NotIpAddress` (CIDR), `Null`, `StringNotEqualsIgnoreCase`, `Arn(Not)Equals/Like` — plus the `ForAllValues:`/`ForAnyValue:` set qualifiers, policy-variable substitution (`${aws:username}` in a Resource), and `Principal`/`NotPrincipal` matching (wildcard, account-root, and the role→assumed-role equivalence) for resource-based and trust policies; previously a policy using any unsupported operator silently failed its Allow, behaving as a Deny (2178). STS grew from a single hardcoded `GetCallerIdentity` to `AssumeRole`/`AssumeRoleWithWebIdentity`/`GetSessionToken`, each minting temporary `ASIA…` credentials recorded in `iamTempCreds` so the enforcement gate resolves a temporary key back to the assumed role's policies; `GetCallerIdentity` now reports the caller's real ARN (2179). Added the IAM group surface with real group-policy inheritance, permission boundaries that cap a user to the intersection with the boundary, and `ListUsers` (2180). Resource-based policies (S3 bucket / Lambda / SNS / SQS) are stored centrally with an `iamResourcePolicyDocsForARN` resolver and evaluated by the gate alongside the identity policy (grant if either allows, deny on an explicit Deny in either); the gate now derives the request resource ARN (S3 path, `sns:TopicArn`, `sqs:QueueUrl`) so resource-scoped policies bite, closing #657 phase 2 (2177); and the S3 REST data plane is now enforced at call time — every `GET/PUT/DELETE/POST /{bucket}[/{key}]` resolves `s3:<op>` + the bucket/object ARN through the gate, so a bucket policy that denies a principal blocks the object request (2181). Enforcement stays permissive for unregistered/test credentials, so existing tests are unaffected.

**GCP** (`iam.go`). Bucket `setIamPolicy` now enforces etag optimistic-concurrency and `getIamPolicy` persists the default policy's etag (matching the project handler); `setIamPolicy` validates member syntax; service-account `uniqueId` is numeric and flows into the key JSON `client_id`; predefined roles are served (`GET /v1/roles[/{role}]`); and custom roles get full CRUD (`projects.roles` + `organizations.roles`, with etag concurrency), service-accounts expose getIamPolicy/setIamPolicy/testIamPermissions as resources, and SA `:disable`/`:enable`/PATCH + `queryTestablePermissions` round out the surface (2182).

**Azure.** Role assignments validate the role definition, a GUID name, and the principal id; built-in role definitions carry their real granular permissions (Owner/Contributor/Reader/… with the real GUIDs) instead of a fake `["*"]`; and a role-assignment list endpoint with `$filter` was added (2183). The Entra Graph gained service-principal and application endpoints (with `addPassword`), a created managed identity now registers a synthetic service principal so it resolves by principalId, users support PATCH, group responses carry `@odata.id`, and the IMDS/MSI token endpoint mints a real RS256 JWT verifiable against the sim's JWKS (2184). Etag/If-Match was deliberately not added to role assignments or managed identities — neither carries an etag in the real API, so adding one would be a fake.

## 2026-06-22 - AWS call-time IAM enforcement (#657) + Cosmos partition isolation & query pagination

One PR across two sims.

**AWS IAM enforcement (#657 → BUG-2176).** The simulator authorized every API call regardless of the caller's policy — the policy evaluator (`iamEvalDecision`) was correct but wired only into the diagnostic `SimulatePrincipalPolicy` endpoint. This adds the missing credential→principal→policy binding and a request-time authorization gate. `iam_users.go` adds the IAM user / access-key / inline (`PutUserPolicy`) + managed (`AttachUserPolicy`) surface that didn't exist (only roles did). `iam_enforcement.go` gates the `POST /` dispatch: it extracts the SigV4 access-key id, resolves it to a registered IAM user, collects the user's effective policy documents, derives the IAM action from the awsJson `X-Amz-Target` or the awsQuery `Action` (reusing CloudTrail's service-source mapping), and runs the evaluator. A non-allowed action returns the per-service deny shape — EC2's `UnauthorizedOperation` (XML 403), awsJson services' `AccessDeniedException` (JSON 403), other query services' `AccessDenied` (XML 403). Crucially, enforcement applies *only* to access keys that resolve to a registered IAM user; unknown / static test credentials stay permissive (the sim's existing default), so every existing test is unaffected and only a consumer who mints a real restricted key sees least-privilege block. SDK + CLI tests prove the repro (a key whose policy lacks `ec2:CreateVolume` is denied `UnauthorizedOperation` while the granted `ec2:DescribeVolumes` succeeds) plus the full user lifecycle. Phase 1 is action-level (resource evaluated as `*`, request-derivable condition keys); the resource-ARN + condition-key context the issue emphasizes (aws:ResourceTag/*, ecs:cluster) is staged as BUG-2177.

**Cosmos partition isolation + query pagination (BUG-2175).** `cosmos_partition.go` makes the Cosmos data plane partition-aware, faithful to real Cosmos and verified by the differential against Microsoft's emulator: the partition key is extracted from the `x-ms-documentdb-partitionkey` header and validated against the value at the container's declared partition-key path in the document (a mismatch is a 400, exactly as the emulator); items are identified by `(partition key, id)` so the same id in two partitions are distinct documents; and single-partition queries are scoped to the requested partition. A key subtlety: the azcosmos SDK declares a container's partition-key path through the *data plane* (`POST /dbs/{db}/colls`), not ARM — so the data-plane collection create now persists that path, and the lookup consults it (falling back to the ARM store, then to legacy id-only keying for undeclared raw-HTTP collections). Query pagination honors `x-ms-max-item-count`, emits an opaque base64 `x-ms-continuation` token and `x-ms-item-count`, and resumes from the token. The differential gained scenarios for same-id-different-partition, the pk-mismatch 400, single-partition query scoping, and max-item-count pagination — all agree with the emulator. (It also fixed a pre-existing harness skip: the vNext emulator hardcodes its advertised data-plane endpoint to `127.0.0.1:8081`, so the differential now publishes the container on 8081 and the SDK actually reaches the oracle.) The remaining Cosmos compliance phases (RU model, stored procedures, change feed, consistency/session) stay staged under BUG-2175.

## 2026-06-22 - Cosmos DB compliance: the real azcosmos SDK against the sim + a differential vs Microsoft's emulator

The Cosmos slice's data plane had only ever been reached through a sim-special raw-HTTP `x-ms-cosmos-account` routing shortcut — itself a fidelity violation of the project's "a real client works against the sim differing only in coordinates" rule. This PR makes the **official `azcosmos` Go SDK** (`NewClientWithKey`) perform the full document lifecycle against the sim — create database/container, create/read/replace/upsert/patch/delete items, and parameterized queries — driven exactly as it would be against real Azure Cosmos, differing only in the endpoint.

Three things were required, found by reading the azcosmos SDK and running it against the sim:

1. **Account discovery.** azcosmos's global-endpoint-manager GETs the account root (`GET /`) on its first request and *fails that request* if it errors, so the sim must serve account properties. The new handler returns a single read/write region whose `databaseAccountEndpoint` echoes the client's own endpoint, so the SDK keeps routing every request to the sim.

2. **Cosmos vs storage routing.** The storage data plane has an Azurite-compatible path-style fallback (`blob.go`) that claims any request carrying a "storage signal" — including `x-ms-version`, which azcosmos *also* sends. The raw-HTTP tests didn't send it, so the collision was invisible; the real SDK tripped it (CreateContainer → a 405 from the blob handler). The fix is a precise `cosmosIsDataPlaneRequest` discriminator (master-key `type=master` Authorization, documentdb headers, or the test account header — deliberately *not* bare `x-ms-version`), which both the account-discovery handler and the storage fallback consult so Cosmos traffic is never misrouted.

3. **Response fidelity.** azcosmos reads the item ETag from the HTTP `ETag` header, so the sim now surfaces it (it previously lived only in the body `_etag`), and every document carries the `_attachments: "attachments/"` system field real Cosmos always returns.

The compliance is proven two ways. `TestCosmos_RealSDKDataPlane` drives the lifecycle with the real SDK against the sim and runs in CI without any emulator. `TestCosmos_DifferentialVsEmulator` runs the **same azcosmos client code** against the sim and Microsoft's vNext Cosmos emulator (`--protocol http`, no TLS) — create/read, upsert-replaces, create-conflict-409, read-missing-404, patch-increment, delete-then-404, and a `WHERE … ORDER BY` query — and all seven scenarios agree. As with the DynamoDB-Local and Firestore differentials, the emulator is a reference, not a ceiling: a `cosmosDiffKnownDivergences` registry holds documented sim-superiority cases (empty today). It's Docker-gated and skips when the image is absent; CI pre-pulls the emulator and the azure SDK step's budget is raised to accommodate its startup.

This is compliance phase 1 — the core document lifecycle + query at SDK fidelity, verified against the real emulator. The deeper surface (partition-key isolation, query pagination/continuation, a realistic RU model + autoscale offers, stored procedures, change feed + conflicts, consistency/session semantics) is staged as BUG-2175, with the differential harness as the standing guard that will gate each as it lands.

## 2026-06-22 - GCP NoSQL slices (Firestore transactions + Bigtable data plane), a Firestore differential, and a fresh 3-cloud audit

One large PR that drains the actionable backlog and extends the differential-testing guard to a second cloud.

**Firestore transactions (BUG-2158).** `firestore_transactions.go` adds `beginTransaction` (issues an opaque token pinning a read snapshot), transactional `batchGet`/`runQuery` (report the snapshot readTime), `commit` carrying a token (applies its writes atomically and retires the token — a transaction commits at most once), and `rollback`. An unknown or already-retired token is a loud INVALID_ARGUMENT, never silently accepted. The high-level `client.RunTransaction` read-modify-write pattern, which previously 404'd, now works end-to-end.

**Bigtable data plane (BUG-2159).** The Cloud Bigtable data API is gRPC-only; the GCP sim already runs a gRPC server (admin services + Cloud Logging), so `bigtable_data.go` mounts the `google.bigtable.v2.Bigtable` data service alongside them. It adds an in-memory per-table cell store and all six data RPCs — ReadRows (streamed as cell chunks), MutateRow, MutateRows, CheckAndMutateRow, ReadModifyWriteRow (increment/append), SampleRowKeys — plus a faithful recursive RowFilter evaluator (chain/interleave/condition, family/qualifier/value regex, row-key regex, column/timestamp/value ranges, cells-per-column/-row limits and offset, strip-value, apply-label, deterministic row-sample). A data op against a table admin never created is a loud NotFound; an unsupported RowFilter or mutation is a loud Unimplemented — never a silent wrong result. Exercised through the canonical `cloud.google.com/go/bigtable` client over `BIGTABLE_EMULATOR_HOST`.

**Firestore differential (the cross-cloud arm of #652 lever 5b).** `firestore_differential_test.go` runs the same SDK operations against the sim (REST transport) and Google's own Firestore emulator (gRPC, launched via gcloud), asserting the observable documents match (numbers canonicalized, timestamps reduced to a presence sentinel). As with the DynamoDB-Local differential, the emulator is a reference, not a ceiling: `fsDiffKnownDivergences` records the one legitimate divergence — the REST transport maps a create-on-existing 409 to `Aborted` while the gRPC emulator returns `AlreadyExists` (the same underlying condition) — and asserts it exactly, so the sim is never regressed to match the oracle. CI installs the `cloud-firestore-emulator` component so it runs for real.

**Fresh 3-cloud audit (BUG-2171 AWS / 2172 Azure / 2173 GCP).** Three parallel audits surfaced the recurring banned patterns; the fixes: AWS — SQS/EventBridge reject malformed/invalid input instead of silently defaulting or forwarding null, and the ignored request-body decode error across ~25 handlers (Kinesis, ECS-service, KMS, CodeBuild, Batch, Glue, ACM, CloudWatch) now rejects malformed JSON (empty bodies still tolerated). Azure — **the #652 silent-incompleteness class turned up again**: a malformed OData `$filter` degraded to match-all; it now fails loud with 400 across all five ARM list callers and the storage-table query, plus the batch/dataplane read-swallow fixes. GCP — secretmanager version-ID parse, the list-filter marshal round-trip, the shared spec-validate body read (all 3 copies), and a dead-param silencer in the quota helper.

**Staged:** the Cosmos differential (BUG-2174) — the official Cosmos emulator is large (~1.5GB), slow to start (~2min), serves self-signed HTTPS, and requires full Cosmos master-key HMAC signing on the oracle (the sim uses a simplified routing shortcut), so it needs its own gated CI job rather than the shared 5-minute `sim (azure)` step. The plan is recorded in BUGS.md.

## 2026-06-22 - Closing #652: differential testing vs DynamoDB Local + CloudWatch fail-loud

This PR closes the consumer's #652 "silent incompleteness" meta-issue by landing
the two remaining prevention levers, so all five are now in place: lever 4 +
5a (#653), levers 1 + 2 for DynamoDB (#654), lever 3 (already present), and now
lever 5b + the CloudWatch arm of lever 2.

**Lever 5b — differential testing against a reference oracle.** New
`simulators/aws/sdk-tests/dynamodb_differential_test.go` boots Amazon's own
`amazon/dynamodb-local` in a throwaway container and replays every #652 bug-class
scenario — put/get round-trip, `SET n = n + :v`, the ElectroDB parenthesized
`SET c = (if_not_exists(c, :z) - :v)` decrement, `attribute_not_exists`
put-if-absent, scan/query filter + `begins_with`, TransactWriteItems with an
Update action, delete-then-absent, and the malformed-expression / undefined-ref
fail-loud cases — against BOTH the sim and the oracle, asserting the observable
outcome is identical (numbers canonicalized via `big.Rat`, set members sorted,
errors compared by AWS error code). All ten scenarios agree with DynamoDB Local.

Crucially, per the project's direction, **DynamoDB Local is a reference, not a
ceiling.** The sockerless sim may legitimately become *more* faithful to real
AWS than DynamoDB Local is, and where it does we must not regress the sim to
match the oracle. That case is modeled by a `diffKnownDivergences` registry: a
documented divergence records the expected sim result, the expected oracle
result, and a justification, and the test asserts the observed results match that
documented shape exactly (so a regression on either side still fails). The
registry is empty today; the mechanism is ready for the first real divergence.
The test is Docker-gated — it skips when Docker is absent, and fails loud if
Docker is present but the oracle can't start — and CI pre-pulls the image so it
runs for real in the `sim (aws sdk)` job.

**BUG-2170 — CloudWatch fail-loud (the CloudWatch arm of lever 2).** The Logs
filter-pattern and Insights `filter` evaluators previously degraded a malformed
pattern to a silent non-match. `cwCompileLogPattern` (FilterLogEvents
filterPattern) and `cwParseInsightsFilter` (StartQuery query string) now return
errors on a malformed pattern/query; `handleCWFilterLogEvents` surfaces
`InvalidParameterException` and `handleCWStartQuery` surfaces
`MalformedQueryException`, exactly as real CloudWatch Logs — never an empty
result that reads as "no matching logs". The pattern is compiled once before the
event loop. SDK + CLI tests cover both, and both parser fuzzers were updated to
treat a malformed input as a loud error rather than a crash.

With fail-loud killing the silent-wrong class, the differential sweep catching
divergences against the real oracle, the spec-derived completeness test catching
under-validation, real parsers replacing string-munging, and CloudTrail
classification fixing the cross-cutting concern, the conformance-bug classes
#652 named are now impossible-or-harder across the board. #394 remains the only
open consumer issue (blocked upstream).

## 2026-06-22 - DynamoDB: fail-loud expressions + spec-derived required fields (#652 levers 2 + 1)

Continuing the consumer's #652 "silent incompleteness" meta-issue after #653
landed levers 4 + 5a, this branch implements **lever 2 (fail-loud-by-default)**
and **lever 1 (spec→completeness)** for the service where every reported bug
clustered — DynamoDB. The unifying failure mode the consumer named is "the sim
succeeds with a plausible-but-wrong result instead of computing the right one or
failing loudly." Both changes flip that posture.

**Lever 2 — fail-loud expression evaluator.** `ddbEvalExpr` previously returned a
bare `bool` and degraded any unparseable condition/filter/key-condition
expression to a silent non-match — so a `FilterExpression` the sim couldn't parse
returned `Count: 0` (reads as "no data") and a `ConditionExpression` it couldn't
parse silently failed the condition. `dynamodb_expr.go` now carries an
error-tracking recursive-descent parser: every structural failure point calls
`fail(...)` instead of returning a sentinel, the parse records every `:value` and
`#name` reference, and a new `ddbCompileExpr(kind, expr, names, values)` returns
`(*ddbCompiledExpr, error)` — an error for a syntax error, an invalid comparator
(the greedy tokenizer can yield `==`/`>>`, now rejected), or a reference to an
undefined `#name`/`:value`. The caller surfaces it as a `ValidationException`,
exactly as real DynamoDB. Wired through PutItem / UpdateItem / DeleteItem /
TransactWriteItems conditions, Query (key + filter), Scan filter, and PartiQL
`WHERE` — each compiled once up front so a malformed expression errors even
against an empty table (matching AWS, which validates before scanning).
`ddbMatchesExpression` was removed; the loops call `compiled.match(item, exists)`.

**Lever 1 — spec-derived required-field validation.** New `dynamodb_validate.go`:
a `ddbRequire` registration decorator consults a `ddbRequiredMembers` registry and
rejects an absent/null `@required` input member with the coral-framework
`ValidationException` message *before* any handler logic — so a missing
`TableName` is a validation error, not the phantom `ResourceNotFoundException` the
table-lookup path produced. The registry is not trusted on faith:
`dynamodb_required_fields_test.go` reads the `@required` members straight from the
vendored Smithy model, and `TestDDBRequiredMembersMatchSpec` fails CI if AWS marks
a new member required or the registry drifts, while `TestDDBRequiredMembersEnforced`
drives every registered DynamoDB op with each required member omitted and asserts
the `ValidationException`. This is the "completeness test that fails CI" the
consumer asked for, scoped to the slice the sim implements.

**Lever 3** (lexer→AST evaluators) was already in place for DynamoDB
(condition/filter/update/PartiQL all parse to ASTs), so this branch only had to
make the existing parser fail loud rather than rewrite it. **Remaining under
BUG-2169:** lever 5b (differential testing vs DynamoDB Local). The same
fail-loud gap in CloudWatch's Logs filter-pattern + Insights evaluators is filed
as **BUG-2170** (same class, different service and error semantics — scoped out
of this DynamoDB-focused PR).

Tests: aws sim build / lint(0) / unit green; DynamoDB SDK + CLI green, including
new `TestDynamoDB_ExpressionsFailLoud` (SDK), `TestDynamoDBCLI_ExpressionsFailLoud`
(CLI), and the two spec-completeness tests; `FuzzDDBEvalExpr` clean (it now
exercises the new error paths). #652 is advanced, not closed — it's a multi-PR
program; #394 stays blocked upstream.

## 2026-06-22 - CloudTrail: data plane leaking into the control plane (consumer #650/#651/#652)

The consumer reported CloudTrail returning DynamoDB item-level events from
`LookupEvents` (#651) and phantom `ListBuckets` events (#650), then filed a
meta-issue (#652) on the recurring "silent incompleteness" failure mode. The root
was architectural, not DynamoDB-specific: the central recorder logged *every*
operation across ~28 services as a management event with **no data-vs-management
classification**, so high-volume data-plane events from all data-event services
(DynamoDB items, S3 objects, Lambda Invoke, SQS messages, SNS Publish, Kinesis
records) leaked into the management-events-only `LookupEvents`. And the
unauthenticated container healthcheck (`GET /`, which the S3 slice routes to
ListBuckets) was being recorded, growing the trail with no client involved.

**Fix (BUG-2167/2168).** Phantom events: `cloudTrailShouldRecord` no longer
records unauthenticated requests (real CloudTrail logs authenticated activity);
service-initiated events use a separate `invokedBy` path and are unaffected. The
systemic leak: a **registration-time** data/management classification — each
service's register function declares its data events via
`cloudTrailDeclareDataEvents(source, ops…)`, co-located with the handlers it
describes rather than in a far-away allowlist, and consulted centrally by the one
recorder all events flow through. Data events are now excluded on both the
client-initiated path and the service-initiated (scheduler-fired) path; the
scheduler-fired-SQS test was rewritten to verify firing by reading the message
off the queue (the faithful observation point), and a cross-service SDK
conformance guard was added.

**#652 architecture — started.** Adopted lever 4 (model the cross-cutting concern
at registration, not ad hoc) and lever 5 (a conformance guard asserting no data
event reaches LookupEvents). The larger levers — spec→struct/validation codegen +
a completeness test, fail-loud-by-default for unimplemented constructs, replacing
the remaining string-munging Condition/Filter evaluators with lexer→AST, and
differential testing vs DynamoDB Local — are staged as BUG-2169.

## 2026-06-22 - Cross-cloud NoSQL "silent-wrong evaluator" sweep (consumer #648, BUG-2149..2166)

Started from consumer #648 — a follow-up to #643/#646: the #646 fix made
`SET c = if_not_exists(c,:0) - :v` compute, but a fully-enclosing `(...)` around
the RHS (and even `(:z)`) still resolved to null, because `ddbEvalSetRHS` never
stripped the wrapping parens. ElectroDB always parenthesizes the arithmetic RHS
of `.subtract()`/`.add()`, so every ORM counter decrement corrupted the attribute
to null. Fixed with `ddbStripParens` (removes balanced fully-enclosing parens
repeatedly, leaving `(a) - (b)` intact) in `ddbEvalSetRHS`/`ddbEvalSetOperand`
(BUG-2149). Then audited every cloud sim's DynamoDB-like service for the same
class — an expression/query evaluator that silently returns null/empty/all
instead of computing — and fixed what it found.

**AWS DynamoDB (BUG-2150..2153).** `if_not_exists(path)` and a bare-path SET copy
now resolve NESTED document paths via `ddbResolvePath` (were top-level only, so
`SET #a.#b = if_not_exists(#a.#b,:z)-:v` always took the default and `SET x = a.b`
stored null); number `=`/`<>`/`IN` canonicalize in the condition/filter engine so
`{N:5} == {N:5.0}`; SET arithmetic and relational comparison use exact `big.Rat`
instead of float64 (a counter past 2^53 or a 38-digit number no longer corrupts);
`+`/`-` on a non-numeric operand raises ValidationException instead of silently
using 0.

**GCP Firestore (BUG-2154..2157).** Field transforms (`increment`, `arrayUnion`/
`arrayRemove`, `serverTimestamp`) were decoded into nothing — a high-level-client
`Increment(1)` returned 200 and stored nothing; now decoded and applied with
`transformResults`. `currentDocument` preconditions are enforced (a missing-doc
Update returns NOT_FOUND, an existing-doc Create ALREADY_EXISTS, instead of
silently upserting). Value-equality is typed (integer 1 == double 1.0; arrays/maps
recurse) so `EQUAL`/`IN`/`ARRAY_CONTAINS` match. Query cursors (`startAt`/`endAt`)
and `select` projection are applied. Driving the tests through the high-level
`firestore.NewRESTClient` surfaced two more real fidelity bugs — the gax REST
transport marshals enums as numbers (now decoded), and a transform-only update's
present-but-empty updateMask was collapsing to "replace" and wiping fields (the
mask is now a `*[]string`: nil = replace, non-nil = merge). Firestore transactions
(2158) and the entirely-absent Bigtable data plane (2159) are filed as staged.

**Azure Cosmos DB + Table Storage (BUG-2160..2166).**

**Cosmos DB SQL query engine (BUG-2160, P2 — the headline).** The data plane's
query path used `cosmosParseEqualityQuery`, which split the WHERE clause on a
single `=` and, for everything else (`AND`/`OR`, ordering comparisons, `IN`,
string functions, nested paths, `ORDER BY`, aggregates), silently returned the
*entire* collection. New `cosmos_sql.go` is a real recursive-descent
parser+evaluator (bounds-safe via the shared `sim.Scanner`/`sim.ParseGuard`)
over the SQL subset the azcosmos SDK + runner workloads issue: `SELECT *`,
projection (`SELECT c.a, c.b`), `SELECT VALUE COUNT(1)`; `WHERE` with
`=`/`!=`/`<>`/`<`/`<=`/`>`/`>=`, `AND`/`OR`/`NOT`, parens, `IN (...)`,
`CONTAINS`/`STARTSWITH`/`ENDSWITH`; nested `c.a.b` doc traversal; `@param`
binding compared by VALUE with numeric unification (`5` int == `5.0`,
typed bool/null); `ORDER BY [ASC|DESC]`, `OFFSET n LIMIT m`, `TOP n`. A query
the parser can't handle returns a Cosmos `BadRequest`, never all docs. JOIN /
subqueries / GROUP BY / non-COUNT aggregates are out of scope and fail loudly.

**Cosmos upsert + 409 (BUG-2161).** The `/docs` create handler now honors
`x-ms-documentdb-is-upsert` (upsert → replace existing, returns 200, honors
If-Match) and returns 409 Conflict on a plain (non-upsert) create of an
existing id, matching real Cosmos.

**Cosmos PATCH (BUG-2162).** New `PATCH /dbs/{db}/colls/{coll}/docs/{doc}`
applies the `{operations:[{op,path,value}]}` list (set/add/replace/remove/incr,
JSON-pointer `/a/b` paths) to a deep-copied `Body` (all-or-nothing), honoring
If-Match.

**Table MERGE vs PUT (BUG-2163).** `handleEntityUpsert` previously treated
PUT/MERGE/PATCH identically (full replace), so a MERGE that omitted a property
dropped it. Split by method: PUT replaces wholesale; MERGE/PATCH overlay only
the supplied properties onto the existing entity.

**Table `$filter` typed literals (BUG-2164).** The OData tokenizer recognizes
typed-literal prefixes (`datetime`/`datetimeoffset`/`guid`/`binary`/`x`/`time`/
`duration`) immediately followed by a quote, emitting one string token with the
unwrapped inner value, and strips OData numeric suffixes (`L`/`f`/`d`/`m`) — so
`Created gt datetime'2025-…'` compares by the date, not the literal word.

**Table `$batch` (BUG-2165).** New `storage_table_batch.go` — `POST /$batch`
parses the `multipart/mixed` change-set (each part a full inner HTTP request),
replays each insert/merge/replace/delete against the existing entity handlers
via an in-memory recorder, rolls the whole batch back on any op failure, and
emits the multipart batch response the aztables `SubmitTransaction` parses.

**Table `$select` (BUG-2166).** `handleEntityQuery` projects the returned
property map to the `$select` columns (always retaining PartitionKey/RowKey/
Timestamp) when present.

Tests: 7 new SDK tests (`cosmos_query_sdk_test.go` raw-HTTP at azcosmos's exact
wire shape — Shared-Key auth + endpoint discovery don't fit the single-port
sim, matching the existing `cosmos_test.go` pattern; `table_merge_batch_test.go`
via the real aztables SDK) plus a rewritten Cosmos-SQL fuzz target and unit
suite — all green; module + sdk-tests build/lint/test clean.

## 2026-06-22 - DynamoDB PartiQL + the remaining actionable tail

Closed out every actionable DynamoDB item in one PR (BUG-2141..2148), leaving only
the externally-gated #1075 (live-cloud) and #1345 (azuread upstream) open.

**PartiQL (2142)** — new `dynamodb_partiql.go` implements `ExecuteStatement`,
`BatchExecuteStatement`, and `ExecuteTransaction` as a faithful translation layer
over the existing item engine, not a second store. A bounds-safe lexer/parser
(`sim.Scanner` + `sim.ParseGuard`) builds a statement AST; SELECT/INSERT/UPDATE/
DELETE map onto the engine — WHERE predicates render to a DynamoDB
ConditionExpression evaluated by `ddbEvalExpr`, UPDATE SET/REMOVE render to an
UpdateExpression applied by `ddbApplyUpdateExpression`, SELECT dispatches to a
point read when the WHERE supplies the full key else a filtered scan, with ORDER
BY and an opaque base64 NextToken. BatchExecuteStatement reports per-statement
errors without failing the batch; ExecuteTransaction is all-or-nothing with a
per-statement `CancellationReasons` array. Validated by SDK **and** CLI tests plus
`FuzzDDBPartiQLParse` (zero crashers).

**ConsumedCapacity (2141)** — decoded `ReturnConsumedCapacity` on
Get/Put/Update/Delete/Query/Scan and emit a `ConsumedCapacity` block computed from
a faithful item-size calculation (attribute-name + value bytes → 4KB read / 1KB
write capacity blocks, halved for eventually-consistent reads).

**Completeness-audit tail** — CreateTable now validates that every KeySchema
attribute (table + GSI + LSI) is declared in AttributeDefinitions (2143); the
legacy `AttributeUpdates` ADD action increments numbers / unions sets instead of
overwriting (2144); nested `ProjectionExpression` paths return only the projected
sub-attribute, merging shared prefixes (2145); the legacy `Expected` /
`ConditionalOperator` parameters are translated to the condition engine and
evaluated (2146); `ConsistentRead` against a global secondary index is rejected
(2147); and `DescribeLimits` is implemented (2148). `DescribeEndpoints` was
deliberately not added — it's internal endpoint-discovery, not a public SDK/CLI op.

## 2026-06-21 - DynamoDB: 4 consumer bugs + a completeness/safety sweep

Fixed four consumer-reported DynamoDB defects (#641–#644) and, on the same PR, a
wide completeness + safety sweep of the AWS sim's DynamoDB slice. Two read-only
audit agents (completeness against the vendored smithy spec; safety/robustness)
surfaced the sweep list; every item got a **complete** fix (no workarounds) with
SDK tests.

**Consumer fixes.** TransactWriteItems honours the `Update` action — it was
absent from the request struct, so an `Update` member silently fell through and
its UpdateExpression + ConditionExpression never ran (every transactional
atomic-counter / version-CAS pattern was wrong); the handler now validates
exactly-one-op-per-item, evaluates all conditions, and applies Update via the
single-item engine (BUG-2126). A cancelled transaction returns the per-item
`CancellationReasons` array (one `{Code}` per item, in order) + the
service-prefixed `__type` the SDK/ElectroDB read to map a conditional failure to
a domain conflict (2127). `SET c = if_not_exists(c,:0) - :v` computes instead of
storing `null` — the evaluator splits top-level `+`/`-` before the function-call
branch and resolves each operand recursively (2128). DeleteTable purges the
table's items so they don't reappear on a same-named recreate (2129).

**Completeness/safety sweep.** Query `ScanIndexForward` (descending order) and
`Select=COUNT`; numeric primary-key canonicalization via exact `big.Rat` so `01`,
`1`, `1.0` address the same item (full 38-digit precision, not lossy float);
nested UpdateExpression document paths `a.b[0].c` with real M/L container
creation across SET/REMOVE/ADD/DELETE; a faithful 32-level item-nesting limit on
writes (also bounds the clone/marshal recursion); parallel-Scan
`Segment`/`TotalSegments` disjoint partitioning; `contains()` over S/N/B sets;
`list_append()` in SET; Batch/Transact 25/100 size limits;
`ReturnValuesOnConditionCheckFailure` + the `Item` member on
ConditionalCheckFailedException; ResourceNotFoundException consistency
(BUG-2130..2140). Added `FuzzDDBItemMarshalRoundtrip` + `FuzzDDBProjectItem`
(zero crashers). **Staged, filed not rushed:** ConsumedCapacity (2141), DynamoDB
PartiQL (2142 — its own grammar, a follow-on slice).

## 2026-06-21 - Systematic prevention of the fuzz-found bug classes

Analyzed the last several fuzz/audit rounds: the recurring crashes cluster into 6
classes, almost all in hand-rolled parsing of untrusted input. Instead of
point-fixing each new instance, put author-time prevention in place.

- **Shared safe primitives** in the `sim` library (`simulators/*/shared/safeparse.go`,
  3 copies): `ASCIIFold`/`ASCIIFoldUpper`/`CaseInsensitiveIndex` — byte-length-
  preserving, so an index from the folded copy is valid in the original (kills the
  case-fold-then-slice class behind 2103/2084/2085/2068); and `FrameReader` — a
  bounds-checked byte cursor (`Take`/`Uint16/32/64` return errors, no over-long or
  overflowing read) for binary wire decoders (the slice/overflow/OOM class behind
  2110/2115/2116). Migrated the 3 divergent local fold helpers (CloudWatch
  `cwIndexKeyword`, DynamoDB `ddbASCIIUpper`, Cosmos `caseInsensitiveIndex`) and the
  SSM input-frame decoder onto them.
- **`forcetypeassert` linter enabled** + the entire backlog fixed: every single-value
  `x.(T)` across the sims, core, agent, and bleephub now uses comma-ok with a
  fail-loud fallback (HTTP/cloud error, skip, or safe default — never panic/silence).
  Kills the unchecked-type-assertion class (2069/2087/2070).
- **errcheck tightened** (user directive): removed the `json.Unmarshal` / `io.ReadAll`
  / `json.Decoder.Decode` / `fmt.Sscanf` exclusions from `.golangci.yml` and handled
  every resulting site — a dropped decode/read error is now surfaced, not swallowed.
- **`scripts/check-casefold-slice.sh`** pre-commit guard: bans the inline
  `(strings|bytes).Index(...To{Lower,Upper}(...)` form and points at the helpers.
- **Nightly exploratory-fuzz CI**: `scripts/run-fuzz.sh` (enumerates + runs every
  `FuzzX` target for a fixed time) + `.github/workflows/fuzz-nightly.yml` (cron +
  dispatch, uploads any new crasher). The committed seed corpus already regresses
  known crashers under plain `go test`; this discovers NEW ones automatically rather
  than relying on a human running fuzz each round.

- **Shared parser-safety core** (`simulators/*/shared/parsecore.go`): `sim.Scanner` (a
  bounds-safe string cursor — Peek/PeekAt/Next/Slice/SkipSpace/HasPrefixFold all
  range-checked, never panics on index/slice) + `sim.ParseGuard` (depth + node
  budget). The 5 recursive expression parsers — DynamoDB expressions, CloudWatch
  Insights filter, CloudWatch filter pattern, GCP AIP-160 filter, Azure OData
  `$filter` — were migrated onto it: hand-rolled `s[i]`/`s[i:j]` tokenizer cursors →
  `Scanner`; ad-hoc per-parser depth limits → `ParseGuard`. Grammar/tokens/AST/eval
  kept byte-identical; each migration validated by the parser's full unit suite + a
  40s fuzz run with zero crashers. The expression DSLs are different *languages*, so
  the unification is of the unsafe *scaffolding* (the bug-prone part), not the
  grammars.
- **CI golangci-lint bumped** v2.10.1 → v2.12.2 (latest release, 45 days old — past the
  1-day supply-chain window; local was already there) so the newly-enabled linters run
  in CI too.

All modules lint-clean (0 forcetypeassert / 0 errcheck); all touched tests green. Every
fuzz-class prevention measure is in place and nothing was deferred.

## 2026-06-21 - Round 20: deferrals cleared + fresh fidelity/fuzz (no deferrals)

Per the user directive (fix ALL actionable bugs in the same session/PR — round 19's
three filed-but-deferred items were not acceptable), this round fixed the 3 deferrals
**plus** a fresh fidelity + crash-surface + fuzz round, **14 bugs in one PR**.

- **BUG-2112 (headline — gcp-family docker-label single-source refactor).** Backends
  stay stateless, but docker-label reconstruction now uses ONE reliable carrier instead
  of a per-resource second-source fallback. New `core` helpers
  (`LabelsEnvVar`/`EncodeLabelsEnvValue`/`DecodeLabelsEnvValue`/`LabelsFromEnvSlice`)
  make `SOCKERLESS_LABELS` (an env var on the main container — survives control-plane
  annotation stripping) the single authoritative carrier on every cloudrun + gcf deploy
  path: added the writer to cloudrun `servicespec` + gcf `pod_service` (single +
  multi-container) via `injectMainLabelsEnv`; every reader (cloudrun job/service, gcf
  pod-member/function) reads it authoritatively + fails loud; deleted all GCP-label
  reconstruction (`mergeLabelsAndAnnotations`/`gcpLabelsToTags`/
  `dockerLabelsFromCloudRunService`/`gcpLabelsToHyphenMap`). Round-trip + malformed unit
  tests on both backends. Edits the stateless reconstruction the cloudrun/gcf runner
  cells validate — cells are the final gate.
- **BUG-2113** EventBridge target `InputPath` / `InputTransformer` are now applied on
  delivery (a focused JSONPath extractor + template substitution); unit-tested.
- **BUG-2114** present-but-non-numeric request params now error per protocol instead of
  defaulting to 0 (IAM `MaxItems` → `ValidationError`; CloudWatch PutMetricData `Value`
  + PutMetricAlarm `Threshold` → `InvalidParameterValue`).
- **Fuzz-found crashes (round-20 crash-surface map):** **2115** SSM input-frame decoder
  uint32-overflow → slice panic (reachable from the ECS ExecuteCommand WebSocket);
  **2116** Service Bus AMQP raw transport unbounded `make([]byte, size)` OOM (pre-auth
  TLS); **2117** CloudWatch Insights `cwFlattenJSONInto` recursion hardened. New fuzz
  targets `FuzzDecodeSSMInputFrame`, `FuzzSBAMQPReadFrame`, `FuzzCWFlattenJSONInto`.
- **Fidelity (round-20 fidelity agent, each verified at file:line vs the vendored
  spec):** **2118** Azure Key Vault PATCH UpdateSecret clobbered omitted attributes
  (P2 — disabled a secret on a single-field update; pointer-struct partial-update fix);
  **2119** EFS dropped required `CreationToken` + access-point `ClientToken` +
  mount-target `VpcId` on read-back (P2 — terraform drift); **2120** RDS Port/AZ,
  **2121** ElastiCache Port, **2122** Kinesis DescribeStream Limit/cursor/HasMoreShards,
  **2123** GCP Logging `uniqueWriterIdentity`, **2124** Cloud DNS `changes.list`
  sortOrder + pagination, **2125** SSM GetParametersByPath pagination + ParameterFilters.

New SDK tests (Key Vault partial-update, EFS CreationToken+filter, Kinesis
DescribeStream pagination) + EventBridge engine unit test + core label round-trip tests;
all sim unit suites, touched SDK families, and core/cloudrun/gcf backends green;
extended fuzz over 19 targets found zero new crashers.

## 2026-06-21 - Audit + extended-fuzz + clean-break round 19

Parallel audit agents (sim fidelity / backend error-handling / un-fuzzed parser map)
+ extended fuzzing. **8 real bugs fixed; zero crashers from the existing 16-target
corpus (it's solid) — the two new crash bugs came from un-fuzzed surfaces.** Also
established two standing directives (no backward-compat affordances / clean breaks
since the project isn't launched; one reliable uniform metadata-reconstruction
carrier per resource, no second-source fallback) and filed the larger work they imply.

- **BUG-2104 (ECR filter, wrong results):** ListImages/DescribeImages dropped
  `filter.tagStatus` + `maxResults`/`nextToken` — a TAGGED/UNTAGGED filter returned the
  full image set. Verified the fields in `ecr.smithy.json.gz`; filter by tag presence +
  paginate. SDK test.
- **BUG-2105 (SQS MD5):** ReceiveMessage with a `MessageAttributeNames` subset re-emitted
  the stored full-set `MD5OfMessageAttributes`; aws-sdk-go-v2's `ValidateMessageChecksums`
  fails the call on mismatch → partial reads broke. Recompute over exactly the returned
  subset. SDK test (the SDK auto-validates, so a green subset receive proves it).
- **BUG-2106 / 2107 (GCP pagination):** IAM ListServiceAccounts + Artifact Registry
  ListRepositories returned the full set with no `nextPageToken`; routed through the
  existing `paginateList` helper. SDK tests.
- **BUG-2108 (fail-loud, azf/gcf):** stateless cloud_state reconstruction silently
  swallowed a corrupt decode of sockerless's own data — azf `azfSpecFromProps`
  (SOCKERLESS_CMD/ENTRYPOINT) → fake empty command; gcf `podMembersFromFunction`
  (SOCKERLESS_POD_CONTAINERS) → pod containers vanish from `docker ps`. Now surface the
  error to a loud `Logger.Warn` (matching gcf's existing `functionToContainer`
  convention), distinct from the legit *absent* case. Corrupt-input regression tests.
- **BUG-2109 (clean break):** core `ParseLabelsFromTags` carried `sockerless-labels`
  raw-JSON + `sockerless-labels-<i>` raw-split read paths that **no current writer
  produces** (AsMap only writes `sockerless-labels-b64`). Deleted both + the legacy test
  per the not-launched clean-break directive.
- **BUG-2110 (fuzz-found, P2 DoS):** the Service Bus AMQP wire-frame decoder
  (`parseAMQPFrame` + `amqpValueReader`, reachable from raw WebSocket binary frames)
  panicked on malformed input in several ways — short-frame `binary.BigEndian` index,
  `u32`/`u64` ignoring `take`'s error, `readArray` `r.off--` underflow to index -1, an
  unhashable `[]any` map key, and a huge wire count → billions of iterations / OOM.
  Hardened with frame/size bounds, safe `u32`/`u64`, an underflow guard, a scoped
  recover for unhashable keys, and total-value + recursion-depth budgets. New
  `FuzzAMQPParseFrame` + `FuzzAMQPReadValue`; 3 regression seeds committed.
- **BUG-2111 (fuzz-found DoS):** IAM `iamSLRName` did `parts[0][:1]` on a leading-dot
  `AWSServiceName` (`SplitN` returns `parts[0]==""`) → slice panic. Rune-aware
  title-case guarded on non-empty. New `FuzzIAMSLRName`.

**Filed for follow-up (not silently deferred):** BUG-2112 the gcp-family docker-label
**single-source refactor** — make `SOCKERLESS_LABELS` the one authoritative carrier on
every deploy path, fail loud, delete the GCP-label reconstruction fallback + dead
helpers; it edits the stateless reconstruction the cloudrun/gcf runner cells validate,
so it's its own cell-gated PR. BUG-2113 EventBridge InputPath/InputTransformer (needs a
JSONPath + template engine). BUG-2114 a few marginal default-on-invalid numeric params
(verify the spec error shape before fixing).

## 2026-06-21 - Audit + extended-fuzz round 18

Parallel audit agents (sims/backends/shared/agent/core) + extended fuzzing over the
full target set. **3 real bugs fixed, 1 notable false positive rejected** — and the
fuzzing paid off with a genuine DoS panic.

- **BUG-2101:** GCP Cloud Build `triggers.patch` ignored `updateMask` (wholesale
  replace → terraform drift). `updateMask` is a documented param (verified in the
  cloudbuild-v1 discovery doc); now merges only masked top-level fields. SDK test.
- **BUG-2102:** Azure storage-account PATCH read `properties` but applied only
  sku/kind/tags, dropping accessTier / public-access / TLS / encryption / networkAcls
  updates → azurerm drift. Now merges the present nested properties. SDK test.
- **BUG-2103 (fuzz-found, P2 DoS):** CloudWatch Logs Insights `cwIndexKeyword` returned
  an index from `strings.ToLower(s)` — which expands invalid-UTF-8 bytes to 3-byte
  U+FFFD, changing the byte length — but callers sliced the ORIGINAL string →
  `slice bounds out of range [10:8]` panic on a `stats … as/by …` clause with
  non-ASCII bytes. Fixed with a byte-length-preserving ASCII-only fold; crasher
  committed as a regression corpus seed.
- **FALSE POSITIVE (FP-13):** an agent flagged ~192 AWS sites returning HTTP 400 for
  NotFound/AlreadyExists "instead of 404/409". Rejected after spec verification:
  awsJson1.0/1.1 protocols return 400 for modeled client exceptions regardless of the
  Smithy `@httpError` trait (the existing `TestConformanceCloudMapServiceNotFoundStatus`
  encodes this); only REST / query protocols honor the status, and the sim already does
  for IAM/SNS/EFS/Route53. The model-based "fix" was reverted before shipping. Lesson
  saved to memory.

The rest of the fuzz suite (~50 targets) was clean. Build + lint clean across all sims.

## 2026-06-20 - Error-path fidelity + fresh fidelity audit (combined)

A combined audit — error-path fidelity (the BUG-2094 class: handlers whose error
paths emit the wrong wire shape / wrong code, latent because untested) plus a fresh
general fidelity pass (round-trip fields, list envelopes, filters, pagination) on
load-bearing ops. Five parallel agents + a precise self-grep; every finding verified
at file:line (the agents over-reported — e.g. SSM GetParametersByPath was claimed
un-paginated but already paginates; several Azure 201-vs-200 nits and a Spanner code
were judged not-load-bearing and left). Six real bugs fixed, each with a regression
test driving the real client:

- **BUG-2095:** CloudWatch GetMetricData/PutMetricData (rpc-v2-cbor) emitted
  awsJson-shaped errors via `sim.AWSError` (no `Smithy-Protocol` header) — the
  remaining cbor-error sites after BUG-2094. Routed through `cwWriteCBORError`.
- **BUG-2096:** Cloud Run v2 UpdateService ignored `updateMask`, doing a wholesale
  replace that dropped unmasked fields (terraform drift; the gcf image-swap path).
  Now merges only the masked top-level fields; absent mask = full replace.
- **BUG-2097:** DynamoDB Query silently dropped FilterExpression (the field wasn't
  in the request struct). Added + applied via `ddbMatchesExpression`.
- **BUG-2098:** DynamoDB Scan applied Limit to *matched* items, not *examined* —
  wrong ScannedCount/LastEvaluatedKey with Limit+Filter. Now caps scanned items and
  resumes from the last scanned key. (Query had the same coupling, fixed together.)
- **BUG-2099:** EC2 DescribeInstances had no MaxResults/NextToken pagination on the
  list form; added it mirroring DescribeVolumes (skipped when explicit ids given).
- **BUG-2100:** GCP Pub/Sub's local `gcpError` omitted the `details` array the shared
  helper + real GCP include; added it.

Error codes/shapes verified against the service models + existing conventions, not
guessed (continuing the BUG-2093/2094 discipline). Build + lint clean across both
sims; SDK + CLI tests added.

## 2026-06-20 - Fidelity: validate invalid/missing required numeric request params

The "no-defaulted-behaviour on invalid input" sweep (offered after round 17). The
simulators parsed several required numeric request parameters with a discarded
error, silently defaulting a missing/non-numeric value to 0 instead of returning
the cloud's validation error.

- **BUG-2093:** CloudWatch `PutMetricAlarm` now rejects EvaluationPeriods < 1, and a
  single-metric alarm (MetricName set) missing Period, across all three wire
  protocols (query / awsJson / rpc-v2-cbor) via a shared `cwValidateMetricAlarm`.
  BigQuery `tabledata.list` rejects a present-but-non-numeric/negative `startIndex`
  or `maxResults` with HTTP 400 `INVALID_ARGUMENT`; absent params still take their
  defaults (startIndex 0, all rows). The error codes were verified against the
  cloudwatch Smithy model and the handlers' existing `MissingParameter` convention,
  and BigQuery's established `INVALID_ARGUMENT` reason — not guessed. The other ~18
  silently-defaulted strconv sites are internal (own pagination cursors, stored
  values, content hashes) and left as-is.

- **BUG-2094 (surfaced by the new validation test):** every CloudWatch rpc-v2-cbor
  handler (`handleCWCBOR*`, alarms + dashboards) emitted awsJson-shaped errors via
  `sim.AWSError` — `application/x-amz-json-1.1`, no `Smithy-Protocol` header — so the
  Go SDK rejected them with "unexpected smithy-protocol response header". Latent
  because no test had ever triggered a cbor error path (the existing
  deleted-dashboard test only asserted `err != nil`, which a protocol-parse failure
  also satisfies). Added `cwWriteCBORError` / `cwWriteCBORErrorf` — `Smithy-Protocol:
  rpc-v2-cbor` header + a cbor `{__type, message}` body, the exact shape
  aws-sdk-go-v2's `getProtocolErrorInfo` reads — and routed all 24 cbor error sites
  through them. The dashboard test now asserts the error deserializes to a proper
  `ResourceNotFound` API error.

## 2026-06-20 - Audit + extended-fuzz round 17 — verification-heavy, +11 fuzz targets

A deep audit — five parallel agents across the AWS/GCP/Azure simulators, the cloud
backends, the shared sim library + agent + realexec, and core/docker/bleephub —
plus extended fuzzing (the existing ~21-target suite re-run at longer durations, in
several streams) and targeted hunting (shared-copy divergence, stub/fake/TODO scans).

**Result: the audited surface holds.** After 16 prior hardening rounds, every credible
structural finding the agents surfaced was verified at `file:line` as a *false
positive* guarded by an existing invariant — the reverse-agent readLoop's
`select`-default non-blocking send (no leak/panic), `kinesisMakeShards` clamping
count≥1 (no `Shards[0]` out-of-bounds), `aciNormalizeContainer` guaranteeing the
`properties` map, `awsPage` falling back to the default page size on 0, the lambda
stdin goroutine's explicit 30s timeout, and the streaming aws-chunked reader bounding
each read to the caller's buffer (a huge chunk size only sets a counter). The shared
`state*/process/oci/router/db/otel` copies are in sync; the four divergent files
(`container/middleware/sandbox/server`) are the known legitimate cloud-specializations.
Zero `TODO/FIXME`, no `return nil,nil` stubs. The full fuzz suite (now 32 targets,
tens of millions of executions) produced **zero crashers**. No production bug was
found — and none fabricated.

**Durable deliverable: +11 fuzz targets** for previously-uncovered untrusted-input
parsers, now permanent regression guards. AWS: `parseChunkSize` (aws-chunked size
line), `cloudTrailDecodeToken` (pagination cursor), `cwParsePercentile` /
`cwParseAggs` / `cwParseSortSpec` / `cwParseTimeUnix` (CloudWatch metric & Insights
specs), `amplifyParseBuildSpec`. GCP: `parseGCSContentRange` (resumable-upload
Content-Range — its fuzz invariant respects the `-1` unknown-end/total sentinel).
Azure: `parseBlobCopySource` (copy-source URL), `parseACRImage` (image ref),
`ehAMQPParseEventHubAddress` / `...ConsumerAddress` (AMQP addresses).

## 2026-06-20 - AWS ECS Express Mode (full faithful cloud-slice) + CLI / spec / test upgrades

Added full AWS **ECS Express Mode** support to the AWS simulator — the managed
"Express Gateway service" AWS launched 2025-11-21. Researched from the AWS API
reference, the ECS developer guide, the vendored `aws-sdk-go-v2/service/ecs@v1.85.0`
(exact enums/shapes), botocore, and terraform-provider-aws
(`aws_ecs_express_gateway_service`) — no guessing. The 4 operations
`Create/Describe/Update/DeleteExpressGatewayService` (awsJson1.1,
`AmazonEC2ContainerServiceV20141113.<Op>`) carry the exact request/response shapes,
enums (accessType PUBLIC/PRIVATE, autoScalingMetric
AVERAGE_CPU/AVERAGE_MEMORY/REQUEST_COUNT_PER_TARGET, statusCode
ACTIVE/DRAINING/INACTIVE), defaults (cpu 256 / memory 512 / healthCheckPath /ping /
port 80 / scaling target 60), and error cases.

**Full faithful cloud-slice assembly.** Each Express service composes the REAL
underlying sim resources — an ECS Fargate service + an ELBv2 ALB + target group +
HTTPS:443 listener + ACM cert + EC2 security group + an Application Auto Scaling
scalable-target + target-tracking policy — all describable via their own sim APIs;
the AWS-provided domain is the ALB DNSName, with 25-services-per-ALB consolidation
and a DRAINING→INACTIVE teardown cascade. SDK + CLI + Terraform tests across all
three dimensions; new doc `docs/ECS_EXPRESS_MODE.md` (Express-vs-vanilla-ECS
comparison), cross-linked from README / docs index / CLOUD_RESOURCE_MAPPING /
SIM_SURFACE_TABLES / BACKENDS / POD_MATERIALIZATION. **BUG-2088** captures 3 issues
Terraform testing surfaced (cluster returned as ARN not bare name; auto-scaling
always provisioned; Delete persists INACTIVE so the destroy-waiter converges).

**Real upgrades, no workarounds.** Two CI gaps the feature exposed were fixed at the
root, not papered over:
- The ECS Express Mode command-line interface (CLI) subcommands need a current aws CLI; CI now installs the latest aws
  CLI v2 in the `sim-aws-cli` job so the ECS Express Mode CLI tests run for real. The whole
  aws cli-test suite was drift-verified against aws v2.35.9 — clean. (**BUG-2091**)
- The spec-shape ratchet flagged Express `taskDefinitionArn` because the pinned
  `ecs.smithy` snapshot predated it (the field is genuinely real per the SDK);
  re-vendored the latest aws-models ecs.json via `scripts/fetch-aws-spec.sh ecs`, no
  allowlist. (**BUG-2090**)

**Skip sweep.** A pass over every `t.Skip` site found one test both broken and never
run: bleephub's postgres persistence test built a malformed DSN with no DB creation
and `BLEEPHUB_TEST_POSTGRES_URL` was set in no workflow. Rewrote it to create / use /
drop a unique throwaway database and added a `postgres:16-alpine` service to the
`test-core` CI job so it runs for real. The other ~34 skips are legitimate
platform/capability gates that do run in CI on the right host. (**BUG-2092**)

## 2026-06-20 - Weak-types + deep-fuzz + robustness audit (round 16) — shared sim / realexec / agent / backend core+docker

A focused audit of the four infrastructure areas through three lenses: weak
types / type-assertion panics, deeper/longer fuzzing, and robustness /
concurrency / security. Four parallel audit agents (agent, core, realexec+shared,
docker) surfaced findings; each was verified at file:line before fixing. Ten real
bugs fixed (BUG-2074–2083), nothing deferred.

**Two HIGH data races (core).** `Network.Containers` and
`Container.NetworkSettings.Networks` are maps inside structs stored in
`StateStore`. `Get`/`List`/`Update` hand callers a *copy of the struct* whose map
fields still alias the one stored backing map; connect/disconnect/rename mutate
those maps in place under `Update` while many sites range/len/marshal the aliased
map lock-free — `ContainerList`/`SystemDf` even assign the live map into a
JSON-marshalled response. The result is the uncatchable "concurrent map iteration
and map write" runtime abort — the exact class already fixed for `PathMappings`
but never extended here. Fixed with copy-on-write helpers
(`cloneEndpointResources`/`cloneEndpointSettings`) in every mutating `Update`
closure. `TestNetworkMapsConcurrentReadWrite` (-race) was proven to fail without
the COW and pass with it.

**Two agent resource leaks.** (1) `MainProcess.Unsubscribe` only deleted the
listener from the map without closing its channel, so `AttachSession.stream`'s
`for evt := range ch` never returned while a keep-alive main process kept
running — one leaked goroutine + channel per attach/detach cycle. Now closes the
channel under the write lock. (2) `SessionRegistry.Register` overwrote an in-use
exec id without closing the prior session, orphaning its child process
(unreachable by `CleanupConn`) and double-appending the id to the conn slice; now
tears down the prior session first. Regression tests for both.

**Fuzz-found overflow (core).** A new `FuzzParseMemoryMiB` immediately caught
`ParseMemoryMiB("9223372036854775807G")` returning `-1024` with a nil error:
`n*1024` wrapped negative past the `n<=0` check, so a caller would set a negative
cgroup memory limit. Guarded with `n > math.MaxInt/mult`.

**Sim + realexec + docker hardening.** AWSQueryRouter reflected the raw `Action`
into a hand-built XML error body (output injection → `xml.EscapeText`) and read
the control-plane JSON body unbounded (→ 64 MiB cap); realexec `IPAM.Release`
panicked on a non-IPv4 address (`.To4()` nil-deref) and `ConfigureSNAT` skipped
the source-CIDR validation its siblings do; docker `SystemDf` dereferenced nil
slice entries from the daemon and `httpGet` hardcoded `/v1.44` instead of the
negotiated API version.

**Shared-copy reconcile.** gcp/azure `shared/router.go` carried the stale
pre-versioned `AWSQueryRouter`, and `state_sqlite.go` `List`/`Filter` returned a
nil slice on aws/gcp but `[]` on azure — diverging from the swappable
`MemoryStore` (non-nil empty), so swapping store backends changed a handler's JSON
(`null` vs `[]`). Reconciled all three copies to canonical (router → aws,
SQLite → empty-slice). Confirmed `container.go`/`sandbox.go`/`server.go`/
`middleware.go` are **legitimately cloud-specialized** (per-cloud sandbox
profiles, azure path-normalization middleware, per-cloud container fields) and
left them divergent — the "all of shared/ is byte-identical" premise is true only
for the 14 truly-shared files, all now verified identical.

**Weak-types verdict.** Of ~26 unchecked single-value type assertions across the
four areas, ALL confirmed SAFE: every `sync.Map`/`Store` `any` load
(`InvocationResults`, `HealthChecks`, `StartLocks`, `WaitChs`, `StagingDirs`,
`BuildContexts`, `VolumeDirs`, `PathMappings`, the agent `sessions` map, the sim
`managedContainers` map, etc.) has exactly one writer type, traced Store→Load; the
WS envelope decodes into a typed `Message` struct (no `map[string]any`); docker
converters had zero unchecked assertions.

All 4 modules + 3 sims + 8 backends + agent bootstraps build; core/docker/agent/
realexec/3-shared `-race` clean; fuzzers run 45–90s clean post-fix; gofmt,
simulator-tests, cloud-backend-isolation hooks pass.

## 2026-06-19 - Deep behavioral audit (round 1: AWS error-semantics + tags + paging)

Where the spec-shape ratchet validates response *shapes*, this audit targeted
*semantics*: 6 parallel agents probed async state machines, error codes,
pagination cursors, idempotency, optimistic concurrency, and cross-op invariants
across all three sims and the cloud backends. Each finding was verified at
file:line, and the agents skeptically self-corrected several false positives (the
GCP `base64("sockerless")` fingerprint concern was unfounded — fingerprints are
real per-mutation UUIDs with genuine 412 validation; ARM LROs, Container Apps,
ACR Tasks, and the previously-staged backend BUG-1844/1845/1846 are already
faithful/fixed).

Round 1 fixed the bounded AWS error-semantics + tag + paging findings, each with
SDK (and CLI for the new op) tests:

- **ELBv2** CreateLoadBalancer/CreateTargetGroup now reject a duplicate Name with
  `DuplicateLoadBalancerName`/`DuplicateTargetGroupName` (1902).
- **Route53** CreateHostedZone honors `CallerReference` idempotency →
  `HostedZoneAlreadyExists` (1903).
- **EC2** DeleteSecurityGroup errors `InvalidGroup.NotFound` on a missing group
  (1904) — and via `ec2ErrorXML`, since EC2 uses the query-XML error protocol
  (a `sim.AWSErrorf` awsJson shape parsed as `UnknownError`).
- **ECR** DescribeRepositories errors on a missing named repo (1905); the missing
  `UntagResource` op is implemented and registered (1906).
- **Lambda** DeleteAlias errors `ResourceNotFoundException` on a missing alias
  (1907).
- **CodeBuild** list ops sort before the offset paginator so pages don't
  duplicate/skip (1908).

The larger state-machine / optimistic-concurrency findings are filed as
BUG-1909–1921 with fix-shapes for follow-up PRs. Durable takeaway: the sims are
behaviorally strong; the recurring real gaps are unvalidated optimistic
concurrency (ETag/fingerprint), a few missing create-handler guards, and
synchronous shortcuts that skip intermediate states.

## 2026-06-19 - CloudWatch Logs Insights query engine (completes the query-language program)

The last query surface without a real parser. New `cloudwatch_insights.go` +
`cloudwatch_insights_filter.go` implement the CloudWatch Logs Insights API
(StartQuery / GetQueryResults / StopQuery / DescribeQueries) with a real
executor for the Insights query language. A query is a pipe-delimited sequence of
commands — `fields | filter | stats | sort | limit | dedup` — run synchronously
at StartQuery over the matching log events; each event is flattened into Insights
fields (`@timestamp`, `@message`, `@logStream`, plus parsed-JSON leaf fields like
`level` / `req.path`). `filter` is a full recursive-descent grammar (`= != < <=
> >=`, `like` substring / `/regex/`, `in [...]`, `and`/`or`/`not`, parentheses,
dotted fields); `stats` does count / count_distinct / sum / avg / min / max with
`by` group fields. CloudWatch Logs is awsJson-only, so one handler set covers
both the SDK and CLI. SDK + CLI tests + an engine unit test.

With this, **every query surface the sims expose has a real parser/evaluator**:
GCP list `filter` (AIP-160), DynamoDB Condition/Filter/KeyCondition expressions,
CloudWatch Logs filter-pattern and Insights, Azure OData `$filter`. KQL was
already implemented; S3 Select / Athena SQL are unused by sockerless.

## 2026-06-19 - CloudWatch dashboards + percentile alarms (#608/#609)

Two consumer CloudWatch issues filed after #610 merged, both blocking terraform
applies, fixed in one PR alongside in-flight work:

- **Dashboards (#608, BUG-1900).** The CloudWatch dashboard API was unrouted →
  404, so `aws_cloudwatch_dashboard` couldn't apply. New `cloudwatch_dashboards.go`
  implements PutDashboard / GetDashboard / ListDashboards / DeleteDashboards over
  a name→body store on all three CloudWatch wire protocols (query, awsJson1.0,
  rpc-v2-cbor). Two protocol gotchas: the query `DeleteDashboards` needs a
  `<DeleteDashboardsResult/>` element or botocore errors, and the list
  `LastModified` must encode as a timestamp (cbor tag-1 / RFC3339), not a bare
  string, or the SDK's `*time.Time` field fails to decode.
- **Percentile alarms (#609, BUG-1899).** The #607 alarm store had no
  `ExtendedStatistic` field, so a `p99` alarm lost it and DescribeAlarms returned
  neither Statistic nor ExtendedStatistic — a terraform perpetual diff. Added the
  field across the struct and the put-decode / describe-encode on all three
  protocols; Statistic and ExtendedStatistic are mutually exclusive, so describe
  emits only the one that was set (idempotent). State evaluation computes the
  requested percentile from the metric data.

SDK + CLI + terraform for both. Remaining query-language work: CloudWatch Logs
Insights (StartQuery), a separate SQL-like language — a follow-up PR.

## 2026-06-19 - Query-language program: full filter/expression grammars

Per the directive for full filter-expression / query-language support across the
sims, replaced the partial matchers with real recursive-descent parser+evaluators,
each unit-tested, all in the pass-6 PR:

- **GCP list `filter`** (`filter.go`) — the full AIP-160 grammar: OR / AND
  (explicit + implicit adjacency) / NOT, parentheses, `= != < <= > >= :`, the
  `field:*` has-wildcard, nested dotted paths. Replaces the conjunctive-clause
  matcher behind `gcpApplyListParams`.
- **DynamoDB expressions** (`dynamodb_expr.go`) — Condition / Filter /
  KeyCondition: comparators, BETWEEN, IN, the functions (attribute_exists,
  attribute_not_exists, attribute_type, begins_with, contains, size), AND/OR/NOT
  with parens and precedence, document paths with nested map (`a.b`) + list-index
  (`a[0]`) segments, `#alias` and `:ref`. Replaces the `=`/AND-only subset;
  existing DynamoDB SDK suite unchanged.
- **CloudWatch Logs filter pattern** (`cloudwatch_filter_pattern.go`) — the
  metric-filter language: unstructured (AND terms, `?` optional/OR, `-` exclude,
  quoted phrases) + structured-JSON (`{ $.field op value && || … }`, nested
  `$.a.b`/`$.a[0]` selectors, `*` wildcard). Replaces the naive substring match
  in FilterLogEvents.
- **Azure OData `$filter`** (`odata_filter.go`) — eq/ne/gt/ge/lt/le, and/or/not,
  parens, startswith/endswith/contains/substringof, `/`-nested paths, plus
  `$orderby`. Wired into the Cosmos/APIM/Key-Vault list handlers.

Each grammar is a tokenizer + recursive-descent parser producing an AST that
evaluates against the resource's JSON (or, for DynamoDB, the attribute-value
item). Parse errors degrade safely (match-all for a list filter; non-match for a
DynamoDB condition). Remaining for a future PR: CloudWatch Logs Insights
(StartQuery) — a separate SQL-like language; KQL is already implemented.

## 2026-06-19 - Sim fidelity pass 6 (AWS/GCP/Azure probe + 5 fixes)

A sixth fidelity pass: five parallel read-only Explore agents probed the AWS,
GCP, and Azure sims for the gap classes the recent consumer issues shared —
silently-ignored request params, offset pagination, dropped read-back fields,
unimplemented ops. Every finding was verified at file:line before acting, which
caught a false positive (the azure `acrCatalogPage` "logic error" traced
correct against hand-worked inputs) and several intentional shortcuts /
out-of-scope items (full GCP `filter=` expression engines).

Five verified gaps fixed, each SDK-tested:

- **EC2 `DescribeSecurityGroupRules` (BUG-1887)** honored only the `group-id`
  filter, so a query scoped by `is-egress`/`cidr`/a tag returned every rule.
  Added `ec2SecurityGroupRuleMatchesFilters` over the shared EC2 filter helpers.
- **EventBridge `CreateEventBus` (BUG-1888)** echoed `KmsKeyIdentifier` but never
  stored it → `DescribeEventBus` returned empty and `aws_cloudwatch_event_bus`
  drifted every terraform plan. Added the field to the struct + create handler.
- **Cloud Map `DiscoverInstances` (BUG-1889)** hardcoded `HealthStatus: HEALTHY`
  and ignored the request's HealthStatus filter — a synthetic-data smell. Now it
  reports each instance's real registered health (`AWS_INIT_HEALTH_STATUS`) and
  applies the HEALTHY/UNHEALTHY/ALL filter (with the HEALTHY_OR_ELSE_ALL default).
- **Lambda `GetFunction` (BUG-1890)** omitted the documented `Code.RepositoryType`
  (`S3` for a ZIP package, `ECR` for a container image). Now set from the type.
- **ECS `ListTasks` (BUG-1891)** ignored the `launchType` filter. Added it.

The rest of the pass's findings were then fixed in the same PR (no deferrals):

- **AWS read-param batch (BUG-1892).** CloudWatch `GetLogEvents` honors
  `startFromHead` (default false → latest events first); EC2
  `DescribeNetworkInterfaces`/`DescribeNatGateways` apply their documented filter
  sets; EC2 `DescribeVolumesModifications`/`DescribeTags` paginate via
  MaxResults/NextToken; SQS `ReceiveMessage` returns
  ApproximateFirstReceiveTimestamp and honors MessageAttributeNames — SendMessage
  now stores message attributes and computes the AWS `MD5OfMessageAttributes`
  digest the SDK validates on receive (pinned by an SDK test).
- **DynamoDB ProjectionExpression (BUG-1893).** GetItem/Query/Scan restrict the
  returned item(s) to the named attributes; LastEvaluatedKey is computed from the
  full item before projection so paging is unaffected.
- **GCP/Azure list params (BUG-1894).** A new JSON-evaluated GCP filter/orderBy
  helper (`gcpApplyListParams`, conjunctive `field op value` clauses) wired into
  Compute/AR/BigQuery/Functions lists + Cloud Logging `orderBy`; Azure
  `$top`/`$skiptoken` pagination on Cosmos/APIM/Key-Vault list handlers via the
  existing `armPage`. Faithful for the common documented forms; richer GCP filter
  expressions fall through as match-all (safe for the sim's small lists).

## 2026-06-19 - AWS sim observability fidelity batch (#602–#606)

The consumer filed five new AWS-sim fidelity gaps, all observability. Each was
implemented at cloud-API fidelity with SDK + CLI coverage (+ terraform where the
op maps to a TF resource) in the same PR.

- **EC2 CopySnapshot (#602, BUG-1882).** The EBS lifecycle had create/describe/
  delete but not copy, so the cross-region snapshot DR flow (snapshot → copy →
  restore) couldn't be exercised. `handleCopySnapshot` duplicates the source's
  backing data into a fresh snapshot id, inheriting size/encryption/KMS.

- **CloudWatch metric alarms (#603, BUG-1883).** Metrics existed but the alarm
  API returned InvalidAction. `cloudwatch_alarms.go` implements PutMetricAlarm /
  DescribeAlarms / DeleteAlarms across all three CloudWatch wire protocols
  (query, awsJson1.0, rpc-v2-cbor) over a single `cwAlarms` store, with
  `StateValue` evaluated live from the metric data over the most-recent
  EvaluationPeriods windows (OK / ALARM / INSUFFICIENT_DATA, honouring
  TreatMissingData). Alarm tagging on the cbor surface makes the
  `aws_cloudwatch_metric_alarm` provider's transparent-tagging read idempotent.

- **EMF extraction (#604, BUG-1884).** PutLogEvents stored EMF messages verbatim;
  the metric store was only fed by PutMetricData. `extractEMFMetrics` parses each
  event's `_aws.CloudWatchMetrics` directives and feeds the existing `cwMetrics`
  store, so the standard ECS/Fargate EMF-over-stdout → awslogs → CloudWatch path
  is now queryable through the ordinary metric APIs with no PutMetricData call.

- **FilterLogEvents prefix (#605, BUG-1885).** `FilterLogEvents` decoded only
  `logStreamNames`, so a `logStreamNamePrefix`-scoped query was treated as no
  filter and returned every stream's events — a data-isolation gap. Added prefix
  selection (reusing DescribeLogStreams' `HasPrefix`) + the mutual-exclusion
  error real CloudWatch returns when both are supplied.

- **LookupEvents pagination (#606, BUG-1886).** The CloudTrail `LookupEvents`
  NextToken was an absolute offset over a list re-sorted newest-first each call,
  so an event prepended between page fetches shifted every offset and consecutive
  pages overlapped (duplicate EventIds). The token is now an opaque cursor on the
  last event's stable `(EventTime, EventId)` key, resumed after — immune to
  head-insertion, each event returned exactly once. Also stopped recording the
  read-only `LookupEvents` calls into the trail (real CloudTrail doesn't log
  them), which had self-amplified the drift.

Reused the BUG-1513 lesson: CloudWatch speaks three wire protocols and different
clients pick different ones (local CLI = query, CI CLI = awsJson, Go SDK +
terraform = cbor), so a new CW op must cover all three — the alarm surface
mirrors what metrics already did, caught when the alarm CLI test hit the
query-protocol path locally.

## 2026-06-19 - aca ExposedPorts cloud-state reconstruction (BUG-1879) + DO_NEXT cleanup

With every audit-found bug fixed (five rounds + the staged BUG-1840–1846 backlog),
the remaining open bugs are both externally gated (#1345 azuread-TF-upstream,
#1075 live-cloud spend). Verifying the already-merged aca GitLab cell (#587) — the
work a stale DO_NEXT "NEXT" section still framed as open — surfaced one genuine
latent gap it had flagged: the aca backend's `appToContainer`, which rebuilds an
`api.Container` from the ACA App for cloud-truth queries (post-restart, or a
`services:` inspect), never populated `Config.ExposedPorts`, while the create-time
path carries them from image config. So `docker inspect` returned the ports when
fresh but dropped them once resolved from the cloud — a stateless
cloud-is-source-of-truth divergence.

ACA's container template has no field for ExposedPorts (its ingress carries only
the bootstrap's single `targetPort`, not the image's declared set), so they now
ride a `sockerless-exposed-ports` tag — the same pattern Name/Network/Pod use for
Docker concepts with no cloud-native home. `buildAppSpec` stamps a deterministic
sorted CSV (`encodeExposedPorts`); `appToContainer` reconstructs it
(`parseExposedPorts`). Kept aca-local (Azure tags are charset-permissive) rather
than widening the shared `core.TagSet` for one backend. Tests: `TestAppToContainer_Shape`
(tag→ports) and `TestExposedPortsTag_RoundTrip`. The branch also streamlined the
stale "bleeplab aca cell NEXT" framing in DO_NEXT/STATUS into history.

## 2026-06-19 - Audit round 5: concurrency / leaks / shared-copy divergence

Four audit passes had combed the code with the same "fakes / swallows / dead-code"
lens, so this round switched to lenses none of them used — concurrency-race
correctness, resource leaks, and divergence between the three vendored copies of
the shared sim library — plus an empirical `go test -race`. The race detector was
clean (the races live in untested concurrent paths), but inspection found real
ones.

- **BUG-1875 (concurrency).** Five fixes. `core/reverse_agent.go` assigned
  `OnDroppedMessage` after the constructor had already started `readLoop`, racing
  the read loop's unsynchronized field read — the exact race the code had already
  fixed for `OnSystemMessage`; a new `NewReverseAgentConnWithHandlers` sets both
  handlers before the loop. `core execForOutput` read the result `bytes.Buffer`
  on its 5s-timeout path without waiting for the `io.Copy` goroutine. `core
  DropSession` closed the registry session unconditionally, so after a
  crash-restart re-dial (where `Register` replaces+closes the old conn) the old
  handler's drop killed the NEW live session — now identity-checked. `cloudrun`'s
  invoke-defer belt-and-suspenders `else` branch double-closed a WaitCh that
  `ContainerStop`/`Kill` (a non-atomic LoadAndDelete-then-close) had already
  taken, panicking — removed to restore single-owner close. `agent/server.go`'s
  per-conn `<-mp.Done()` watcher blocked forever when the WS dropped while the
  main process ran, leaking a goroutine per reconnect — now guarded by a
  defer-closed `connDone`.
- **BUG-1876 (resource leak).** The azure sim's EventGrid webhook delivery and
  subscription-validation discarded the `*http.Response` from `http.Post`, never
  closing the body — leaking a connection per subscriber per event. Both drain +
  close now.
- **BUG-1877 (shared-copy divergence).** The three sims vendor a copy of the same
  `shared/` library, and AWS had drifted ahead: its `RegisterUI` guards the root
  redirect against a `ServeMux` panic (when a service already owns `GET /{$}`) and
  its middleware captures the 5xx error body for logging — both missing from the
  GCP/Azure copies. Ported AWS's versions to both (the GCP/Azure panic is latent
  only because no current handler owns their root). Genuinely cloud-specific
  divergences (sandbox profiles, the AWS-query router, Azure path normalization)
  were left alone.

## 2026-06-19 - Audit round 4: deep UI pass + sim structure + harness scripts

A fourth parallel-agent audit, targeting areas the first three didn't deeply
cover: the UI packages (only a shallow pass in #593), the sims' HTTP routing +
emitted URLs (mux-overlap / emitted-URL-roundtrip lenses), and the integration
harness shell scripts. The dominant find was the BUG-1851/1852 class — a failed
query rendered as a confident healthy/empty state — recurring in pages that
pass shallow review.

- **BUG-1872 (bleephub UI).** `RepoDetailPage` destructured five secondary
  queries (commits, webhooks, secrets, environments, releases) as `data = []`
  with no `isError`, so a 500 rendered "No secrets configured" / "This
  repository is empty" — an admin reads "no secrets" when the fetch actually
  failed (the sibling `RepoSecretsPage` already handled it). `OAuthPage` showed
  an infinite spinner on error; Issues/Pulls comment queries rendered "No
  comments yet". All now surface `<InlineError>` on `isError`.
- **BUG-1873 (sim / bleeplab / admin / core UIs).** The simulator-aws/gcp/azure
  Overview pages branched only on `isLoading`, so a failed `/sim/v1/summary`
  collapsed `services` to `{}` and rendered a full board of `?? 0` zeros with
  the header still "running" — the exact docker-frontend defect, unfixed in
  three places. bleeplab Overview/ProjectDetail swallowed pipelines/storage
  errors into empty tables; the shared core `MetricsPage` rendered
  `status?.containers ?? 0` (fabricated 0 on a `useStatus` failure → now `—`);
  admin ComponentDetail/ProcessDetail silently omitted sections on error → now
  render an `ErrorPanel`.
- **BUG-1874 (harness + sim hygiene).** Three host-side `docker build -t`
  invocations lacked `--load`; on a buildx-default host that leaves the image
  in the build cache only, so `smoke-test-{ecs,cloudrun,aca}` were outright
  unusable (`docker run` → "image not found") and `bleephub-gh-docker-test`
  could run a stale image — added `--load` matching the `Makefile:400/453`
  convention. The two smoke ECS-cluster bootstraps used `curl -s` (no `-f`) so
  a non-2xx `CreateCluster` started the backend against a missing cluster → now
  `curl -sf … || fail`. Also marked the aws sim's `apigatewayv2.apiEndpoint`
  field `// external:` — the one emitted URL missing the codebase's
  external-marker convention.
- **FALSE POSITIVE.** The agent flagged the in-harness
  `bleephub-spawn-runner` `docker build` for lacking `--load`. But the harness
  installs the classic `docker.io` CLI (no buildx plugin), which writes the
  build straight to the daemon store regardless of the host's buildx default
  and rejects `--load` as an unknown flag — so it's correct as-is. Verifying
  before acting caught it (the host-side Makefile builds, which DO run the
  buildx-aware CLI, are the real gap — fixed under BUG-1874).

## 2026-06-18 - Audit round 3 deferred items: SQLite integrity, stats fabrication, + a false positive

Closed out the three findings deferred from #597. Verifying each before acting
turned the third into a false positive — a good reminder that an agent-proposed
fix can be flatly wrong.

- **BUG-1869 (shared-sim SQLite integrity).** `SQLiteStore`'s
  `Put`/`Update`/`Get`/`List`/`Len`/`Delete` swallowed every DB error: a failed
  write returned as "stored", a transient read error read back as "absent" (so a
  resource 404s or gets recreated), `List` dropped rows on a scan error and never
  checked `rows.Err()`. The `Store` interface has no error returns, so widening
  it would ripple through all three sims; instead a new `fatalDBErr` panics on an
  UNEXPECTED DB error — net/http recovers a handler panic into a 500, so it's
  loud without nuking the sim, the same fail-loud stance `MakeStore` already
  takes. A legitimate miss stays `sql.ErrNoRows → (zero,false)`. Fixed in all
  three shared copies (azure keeps its `[]T{}` empty-slice List style);
  regression test asserts every op on a closed DB panics; all three sim suites
  stay green.
- **BUG-1868 (fabricated zero stats).** cloudrun/gcf/aca/azf registered no
  `StatsProvider`, so `core buildStatsEntry` returned all-zero CPU/memory as a
  real stats document (a live container shown at 0%), and it also swallowed a
  provider error back to zeros. `buildStatsEntry` now returns `(map, error)` —
  `NotImplementedError` when there's no provider, the provider's error when it
  fails — and the four call sites surface it. ECS keeps its real CloudWatch
  metrics; its legitimate "just-started task, no datapoint yet" zeros are a
  successful empty reading, untouched.
- **BUG-1870 (Spanner `/spanner/` path) — FALSE POSITIVE.** The audit agent
  flagged the prefix as a fake-test masking a wire-path bug. In fact it's a
  documented, load-bearing collapsed-port disambiguation: Cloud SQL registers
  `registerCloudSQLPrefix(srv, "/v1")` and owns the canonical
  `/v1/projects/{project}/instances` (terraform-provider-google's `sqladmin/v1`
  client uses it), so mounting Spanner's identical real path there would panic
  Go's ServeMux at registration. Real GCP separates the two by hostname; the
  single-port sim disambiguates by prefix (the `gcpMountPrefixes` mechanism). The
  test endpoint `baseURL+"/spanner/"` is the per-service coordinate. Reclassified
  as a false positive — "fixing" it would crash the sim.

## 2026-06-18 - Audit round 3: shared sim library, per-cloud common, cloud backends, CLI + harnesses

A third parallel-agent audit, targeting the areas the first two passes didn't
deeply cover: the shared `sim` library (one copy per cloud under
`simulators/<cloud>/shared/`, all three audited via the aws copy), the per-cloud
`backends/*-common` modules, the cloud backend `api.Backend` method
implementations, and the CLI + the simulator test harnesses. Every candidate was
confirmed at its `file:line` before any fix.

**Fixed (each tested in its module):**
- **BUG-1860:** ecs `deriveMACFromIP` fabricated the container MAC from the IP
  octets (and hardcoded `02:42:ac:11:00:02` on parse failure). The real MAC is
  in the same ENI attachment `Details` as the IP — now read via `extractENIMAC`
  (`macAddress`), never synthesized.
- **BUG-1861 / 1866:** orphan-on-swallow — ecs `ContainerKill`/`ContainerRemove`
  and cloudrun/aca `ContainerStop`/`ContainerKill` dropped the cloud
  stop/delete error and reported success, leaving a billable task/Service/App
  running. All now propagate (the `*Strict` delete variants for cloudrun/aca);
  same BUG-1844 class that earlier fixed PodRemove/NetworkRemove but not these.
- **BUG-1862:** ecs `fargateResources` returned the smallest tier (256/512) for
  a request exceeding the largest Fargate tier — now clamps to the max.
- **BUG-1863:** ecs + lambda `ListImages` returned a truncated list as complete
  on a transient `DescribeImages` error (BUG-1853 iterator-swallow class) — now
  propagate.
- **BUG-1864 (BUG-1789 class, 3rd instance):** azure-common's image resolvers
  hardcoded `<acr>.azurecr.io` and the ACR-Task `sourceURL` re-hardcoded
  `<account>.blob.core.windows.net`, ignoring the `SOCKERLESS_AZURE_ACR_ENDPOINT`
  / discovered-blob coordinates that the same module's overlay-push path already
  honors. Latent (the green cells run with no ACR name). Routed both resolvers
  through a new `AzureRegistryHost(acrName)` helper (mirrors gcp-common's
  `OverlayRegistryHost`) and built `sourceURL` from the endpoint
  `ensureBlobClient` discovers; regression test added.
- **BUG-1865:** the shared sim `process.go` swallowed `cmd.StdoutPipe()` /
  `StderrPipe()` errors — a nil reader then panicked the scan goroutine and lost
  all process output. Both checked before `Start` now; `cp`'d to all three
  shared copies.
- **BUG-1867:** CLI fabricated success after a parse failure — `status` printed
  `UP (uptime 0s)`, `context reload` printed `Reloaded`, and admin
  `LoadProjects` silently dropped corrupt configs. All fail loud now.

**Filed open** with fix-shapes (each needs its own careful, separately-tested
change): BUG-1868 (cloudrun/gcf/aca/azf `ContainerStats` fabricate zeros — no
`StatsProvider`), BUG-1869 (shared-sim `state_sqlite` Put/Update/Get/List/Len
swallow DB errors — a `Store` interface change rippling through all 3 sims), and
BUG-1870 (the gcp sim mounts Spanner under a non-canonical `/spanner/` path that
its own tests bake in).

## 2026-06-18 - Audit round 2: fail-loud / no-swallow / no-fake / no-dead-code sweep of the glue/harness/core code

The #593 audit focused on sims → backends → UIs; this round targeted the areas
it didn't — the in-container agent bootstraps, the GitHub-runner dispatchers,
bleephub, bleeplab, and the shared core engine + the Linux realexec host. Five
parallel read-only agents proposed candidates; every one was confirmed at its
`file:line` before any fix (the dispatcher agent even reported a wrong absolute
path — the dirs are repo-root `github-runner-dispatcher-*`, not `dispatchers/`
— which verification caught).

**Fixed (each tested in its module):**
- **BUG-1853 (P1):** the gcp github-runner dispatcher's `it.Next()` loops
  treated a real Cloud Run API error the same as `iterator.Done`. In
  `executionStateForJob` a transient `ListExecutions` blip yielded
  `NO_EXECUTION` — a reap-eligible state — so a network/throttle hiccup during
  a long CI job would delete the live runner Job (the BUG-1752 class). Now
  returns `UNKNOWN` (never reaped) and `ListManaged`/`ListManagedServices`
  propagate the error so the cleanup sweep skips that cycle, matching the
  already-correct Azure twin. AWS + Azure dispatchers were clean.
- **BUG-1854:** agent bootstraps — the cloudrun/gcf PostExec hook silently
  dropped the GCS workspace save on a malformed `SOCKERLESS_SYNC_VOLUMES`
  (data loss); `parseUserArgv` silently returned "no command" on a malformed
  argv; azf `handleInvoke` swallowed a partial body read. All now fail loud.
- **BUG-1855:** core `prepareBuildContext` swallowed every COPY-staging
  filesystem error and `ImageBuild` streamed `Successfully built` anyway —
  an image silently missing its COPY'd files. Now propagates + fails the
  build. Plus a swallowed `docker import` ImageTag and a `_ = layerPath` pin.
- **BUG-1856:** bleeplab runner-verify returned a fabricated `id: 0` instead
  of the real runner identity; job-trace `io.ReadAll` swallow returned `202`
  + an advanced `Range`, acking bytes it never stored.
- **BUG-1857:** bleephub had two dead GraphQL enum silencers
  (`mergeableStateEnum`/`pullRequestReviewDecisionEnum`) — wired them as the
  field types (faithful to GitHub's enum-typed schema; stored values already
  match) and removed a dead token lookup.

- **BUG-1858:** bleephub `Repository.viewerPermission` hardcoded `ADMIN` —
  now computes the real permission from the viewer (`ghUserFromContext`) + repo
  (`store.GetRepo`) via the existing `rbac.go` helpers
  (`canAdminRepo`/`canPushRepo`/`canReadRepo` → ADMIN/WRITE/READ/null).
  Regression test proves an anonymous viewer of a public repo gets a non-ADMIN
  result.
- **BUG-1859 (minor batch):** bleeplab three `_ = readJSON(...)` handlers now
  fail loud (400) + reject a blank project name; receive-pack matches
  `errors.Is(io.EOF/io.ErrUnexpectedEOF)` instead of a `"EOF"` substring; agent
  azf `jobTimeout` treats a malformed value as default (never silently
  unbounded), only an explicit `0` disables; core `image_manager` logs the
  skipped local-record populate instead of swallowing it.

Everything found was fixed in this one PR — nothing deferred.
`simulators/realexec/` audited clean.

## 2026-06-17 - Audit backlog (deferred): cloudrun network-service stateless reconstruction (BUG-1845)

The cloudrun backend's `networkServices` map (user-defined-network ID →
service-style container IDs) was authoritative local state with no cloud
reconstruction: after a backend restart mid-job, `serviceMembersOfNetwork`
returned nil and the next gitlab-runner stage's revision lost its `services:`
sidecars (e.g. redis). The members are bundled sidecars in each per-stage
script-runner Service revision — never their own cloud resource — so the
revision is the only durable record of them.

`buildServiceSpec` now persists the network's service-style members on the
revision as annotations (`sockerless_network_id` + a base64-JSON
`sockerless_network_service_members` of the non-OpenStdin members; the
OpenStdin script-runner siblings are per-stage transients and excluded).
`serviceMembersOfNetwork` rebuilds the in-memory map on a cache miss —
`rebuildNetworkServicesFromCloud` lists the network's Services, takes the
latest revision's member blob, and re-seeds each member into PendingCreates so
the next stage re-bundles it exactly as it would within a single process. A
`networkRebuilt` guard ensures a service-less network doesn't list Services
every stage, and the live `trackNetworkService` path is untouched within one
process (the only green-path change is the additive annotations). gcf is
unaffected — it runs a `services:` job as a single multi-container revision,
not via this map.

Unit test `TestNetworkServiceMembers_PersistAndRebuild` exercises the
persist → simulated-restart → rebuild round-trip; the green `services:` redis
flow (PING/SET/GET across stages) is verified by the bleeplab cloudrun cell.

## 2026-06-17 - Audit backlog (deferred): removed the sim-only `Sim*` fake fields (BUG-1840)

Picked up the first of the three items deferred from #594. The gcp and azure
simulators carried `SimCommand`/`SimImage`/`SimArchitecture` (gcp Cloud Functions
`ServiceConfig`) and `SimCommand` (azure App Service `SiteConfig`) — sim-only
fields on the real cloud resource model, accepted off the wire though no real
client produces them. A faithful sim must not have them.

**gcp.** The image-less "Sim path" of `invokeCloudFunctionProcess` (which ran a
`SimImage` container directly so SDK tests could check invoke semantics without
staging an overlay) was backend-dead: a Cloud Functions Gen2 is backed by a Cloud
Run service, and the gcf backend invokes that service's `svc.Uri`
(→ `/v2-services-invoke`, served by `cloudrunservices.go invokeService`), never the
sim-only `/v2-functions-invoke` endpoint that fronted the Sim path. Removed the
fields, the Sim branch, and the `Sim*`-only `TestCloudFunctions_Invoke*` /
`InvokeArithmetic*` SDK tests. `invokeCloudFunctionProcess` now runs only the
faithful overlay-image path; a function with no deployed image records
"Function invoked" in Cloud Logging and returns `{}`. Real gcp container-execution
coverage stays via the Cloud Run **Jobs** arithmetic tests and the gcf cell.

**azure.** Kept `invokeAzureFunctionProcess` — that IS the real App Service
container-run path the azf backend drives via `LinuxFxVersion` (`DOCKER|<image>`) +
`SOCKERLESS_CMD`/`SOCKERLESS_ENTRYPOINT` app settings. Removed only the sim-only
`SimCommand` fallback and the now-identity `Site.wire()` stripper. The invoke SDK
tests were rewritten to deliver the command exactly as the backend does — a
`SOCKERLESS_CMD` app setting carrying `base64(json(argv))` — and still execute real
`alpine`/`eval-arithmetic` containers.

**Sub-fix (BUG-1824 class).** Verifying the azure rewrite surfaced a pre-existing
local-repro failure: the three sim sdk-tests' `buildGoScratchImage` used
`docker build -t` with no `--load`, so on a `docker-container` buildx-default host
the freshly-built workload image landed in the build cache only and the sim
couldn't run it (eval/`InvokeArithmetic*` 500'd). It now probes
`docker buildx version` and uses `buildx build --load` when present. CI was
unaffected (its docker driver loads to the store); this only blocked local runs.

Cell impact was confirmed by tracing the backend invoke target rather than
re-running the multi-minute gcf/azf cells, which don't exercise the changed code.

## 2026-06-17 - Codebase audit: fallbacks / error-swallowing / fakes / sim-contract / dead code (sims → backends → UIs) + open-issue fixes

A targeted sweep for the anti-patterns the user flagged — fail loudly, never
swallow; no fallbacks or sim special-behaviour that breaks the "sims faithfully
reimplement cloud APIs" contract; avoid defaulted behaviour; no functionally-dead
code. Three parallel read-only audits (one per simulator), a backend audit, and a
UI audit. The audit was productive — it found real P1/P2 issues and a backlog of
genuine fakes/fallbacks.

**Fixed in the sweep:** gcp sim `gcsObjectBytes` silent empty-body on a disk-read
failure → now errors → 500 (BUG-1836); azure sim swallowed real-exec subnet-delete
error → surfaced (1837); azure sim five dead-variable linter-pins removed (1838);
cloudrun backend `resolveExecutionState` fabricated `ExitCode 0` on a cloud-query
failure → now `-1` so a failed job isn't reported as success (1839); gcp Cloud
Build sim's `docker build` step hit the same buildx-`--load` portability bug as the
azure ACR-Tasks sim (BUG-1834) → same buildx-probe fix (1847). Plus the three open
AWS-sim fidelity GitHub issues: CreateVolume now requires AvailabilityZone
(#591/1848), ECS cluster-scoped ops raise `ClusterNotFoundException` for an unknown
cluster (#592/1849), DescribeSnapshots/Volumes honour `MaxResults`+`NextToken`
(#590/1850) — each with an SDK test. Two UI fail-loud fixes: the docker-frontend
dashboard no longer renders a healthy-looking zeroed page on a failed fetch
(1851), and the admin HTTPS-gateway card no longer fabricates plausible endpoint
URLs / CA path when the gateway info is unavailable (1852). Closed the
already-fixed issues #583 (ECS CPU/Mem enforcement) and #569 (process-mode EBS
panic).

**Staged backlog (filed OPEN with fix-shapes, BUG-1840–1846):** the genuine
larger items that need careful, tested, sometimes contract-changing work — the
sim-only `Sim*` fake fields (gcp+azure), aws Cloud Map sockerless-tag DNS, aws ec2
sockerless pre-seeding, gcp fingerprint optimistic-concurrency, the backend
error-swallow batch (PodRemove ×4, core ContainerWait, ecs zero-stats, lambda
pod-row), the cloudrun `networkServices` stateless violation, and the backend
default-param-on-invalid + dead-code batch. These are tracked, not dismissed.

## 2026-06-17 - GitHub `actions/runner` cells on ACA + AZF (both GREEN) — GitHub+GitLab parity on every container backend

The bleephub GitHub `actions/runner` topology cell — a real `actions/runner`
running container-mode jobs (`container:`), service containers (`services:`),
and a dispatcher-spawned runner against a sockerless backend — is now green on
**ACA and Azure Functions**, completing GitHub+GitLab runner parity across all
six container-capable backends (ECS, Lambda-class, Cloud Run, GCF, ACA, AZF).

**ACA** was already wired and stayed green after the #587 backend changes
(faithful ingress, WS keepalive, runner-stage HTTP invoke). **AZF** was newly
wired into the bleephub harness, mirroring aca: the harness image builds
`sockerless-backend-azf` + `sockerless-azf-bootstrap`; a `provision_azf` brings
up the Azure sim + backend with azf's host primitive (an App Service plan), the
ACR-Tasks overlay registry coordinate, cloud-dns service discovery
(`SOCKERLESS_AZF_NETWORK_DISCOVERY=cloud-dns`), the `/v1/azf/reverse` agent
path, and runner-workspace + externals Azure-Files shares; a Makefile target
`bleephub-runner-docker-test-azf` runs it.

Two real bugs surfaced bringing azf up, both fixed:

- **BUG-1834** — the Azure sim's ACR-Tasks overlay build hardcoded the
  buildx-only `--load` flag. The harness container ships the legacy `docker.io`
  builder (no buildx plugin), which rejects `--load` (`unknown flag`), so the
  `container:` job's `ContainerCreate` returned 500. There is no single
  `docker build` invocation that works across the legacy builder, the buildx
  `docker` driver, and the buildx `docker-container` driver, so the sim now
  probes `docker buildx` and uses `docker buildx build --load` when present
  (loads to the daemon store for every driver) else plain `docker build`
  (legacy, store-native), logging the chosen path. (The bleeplab cells had
  passed only on a cached overlay; the bleephub harness wipes its data dir,
  forcing a real build that exposed it.)
- **BUG-1835** — azf cloud-dns `startCloudDNSSite` keyed overlay-vs-raw deploy
  on `OpenStdin`, a gitlab-runner-only signal. A GitHub `container:` job is
  exec-driven but NOT OpenStdin → it was deployed as a raw image with no
  reverse-agent, so `docker exec` of each step failed `exit 126`; and a
  `services:` container (image-default entrypoint) must run its RAW image, not
  the overlay. The fix derives `serviceLike` (no client entrypoint/cmd override
  AND not OpenStdin) from the ORIGINAL client create request, recorded at
  ContainerCreate into `labelServiceLike` BEFORE the image's default
  entrypoint/cmd are merged in (post-merge, the base labels can't distinguish a
  client override from an image default — the same reason aca computes
  `serviceLike` pre-merge). `startCloudDNSSite` reads the marker: a service
  deploys its raw image and runs on the VNet (started by swift integration);
  anything else deploys the overlay and is invoked so the in-site reverse-agent
  registers (`invokeFunctionAsync` blocks for the agent — no fallback).

## 2026-06-17 - azf cloud-dns hardening: connect-after-create alias registration + swift VNet-integration CLI/TF contract

Hardened the merged azf cloud-dns service discovery (`feat/azf-clouddns-hardening`).

**azf `NetworkConnect` on connect-after-create.** The merged cell only ever
created containers *with* their network, so `docker network connect
--network-alias X` *after* create was lost: the core `SyntheticNetworkDriver.Connect`
records the endpoint in `Store.Containers`, which the stateless azf backend
doesn't read. `cloudDNSNetworkConnect` (wired into `NetworkConnect` behind the
cloud-dns config) closes the gap two ways — a connect *before start* stamps the
network + aliases onto the PendingCreate so `startCloudDNSSite` VNet-integrates
and registers them exactly as the create-with-network path does; a connect to an
*already-deployed* site VNet-integrates it into the network's subnet and writes
the `--network-alias` names as Private DNS CNAMEs immediately. Unit test
`TestCloudDNSNetworkConnect_StampsPendingCreate`.

**Swift VNet-integration testing contract (SDK+CLI+Terraform).** The App Service
regional-VNet-integration endpoint (`PUT/GET/DELETE
.../sites/{name}/networkConfig/virtualNetwork`) — the primitive cloud-dns
discovery is built on — gained the CLI (`az rest` round-trip) and Terraform
(`azurerm_app_service_virtual_network_swift_connection` against an EP1 function
app + a `Microsoft.Web/serverFarms`-delegated subnet, in the apply/idempotency/
destroy stack) coverage to join the SDK test from #587. Adding the Terraform
path surfaced and fixed three real azure-sim fidelity bugs. **BUG-1833** — the
swift response returned its resource `id`/`type` from the *operation* path
(`.../sites/{name}/networkConfig/virtualNetwork`, type
`Microsoft.Web/sites/networkConfig`) rather than the canonical *config*
sub-resource id real Azure returns (`.../sites/{name}/config/virtualNetwork`,
type `Microsoft.Web/sites/config`); terraform-provider-azurerm's Create parses
the response `id` (`*read.Model.Id`), and its parser rejects an id without a
`config` segment, so the apply failed `ID was missing the 'config' element`. The
#587 SDK test missed it (it asserted only `subnetResourceId`); the SDK + CLI
tests now also assert the returned `id` carries `/config/virtualNetwork`, so the
regression is caught in the widely-run `sim (azure)` job, not only the gated
`tf (azure)` job. **BUG-1832** — a
delegated subnet dropped the delegation's `actions` array on read-back, so an
`azurerm_subnet` with a `service_delegation` block wasn't idempotent (added an
`Actions []string` round-trip); **BUG-1831** — the swift PUT force-started a
workload container for any non-HTTP site, so VNet-integrating a site with no
container image (a plain Terraform function app) returned `500 … has no
container image` (gated the start on the site actually having a container image —
real Azure VNet integration is a pure networking-config operation; the
redis-with-image services path is unchanged).

## 2026-06-16 - bleeplab GitLab cells on the ACA + AZF backends (both GREEN) + AWS sim faithfulness

The full gitlab-runner docker-executor flow (build → artifact → `services:`)
now runs on **both** Azure backends — aca and **Azure Functions (azf)** — all 4
cell tests green on each. azf's last and hardest hurdle was **faithful cloud-dns
service discovery** so the build site resolves `redis:6379`: it is assembled
end-to-end from real Azure primitives, with the *same backend code against the
sim and real Azure* (no sim-awareness). `NetworkCreate` provisions a
`Microsoft.Network/virtualNetworks` + a subnet delegated to
`Microsoft.Web/serverFarms` + a Private DNS zone linked to the VNet
(`armnetwork`/`armprivatedns`). `ContainerStart` under cloud-dns deploys each
container as its **own** App Service site (a `services:` redis runs its raw
image; the build runs the bootstrap overlay), does App Service **regional VNet
integration** (`WebApps.CreateOrUpdateSwiftVirtualNetworkConnectionWithCheck`)
into the subnet, and registers each `--network-alias` as a Private DNS CNAME →
the site's default hostname. The azure sim realizes these faithfully: a
`Microsoft.Web/serverFarms`-delegated subnet is the App Service container fabric
→ a Docker user-defined network (not the IaaS netns the compute stack uses);
swift integration attaches the site's container to it; a CNAME → a site's
default hostname is realized as a Docker embedded-DNS alias on that site's
container (`realizeCNAMEAsSiteDockerAlias`, the App Service analog of the ACA
`realizeCNAMEAsDockerAlias`). The build site then reaches `redis` over the
shared VNet (DNS) and PING/SET/GET pass. This composes Docker's network + DNS +
services purely from Azure cloud primitives — the bar the whole project holds.

The full gitlab-runner docker-executor flow now also runs on the **Azure
Container Apps (aca) backend** — all 4 cell tests pass, including the redis
`services:` job (PING/SET/GET) over the per-build network. The runner stages
route through the bootstrap's **HTTP buffered-invoke** to the App's ingress
(like cloudrun/gcf), not the reverse-agent WebSocket: the WS exec half-opened
backend→container under the heavy per-stage container churn (gvisor/podman
port-reuse), so `agent.CollectExecWithStdin` blocked forever. The azure sim
implements **faithful ACA ingress** (`registerContainerAppsIngress`): a
`WrapHandler` that reverse-proxies any request whose Host matches an App's
`latestRevisionFqdn` to that App's running replica on its configured ingress
`targetPort` — exactly how real ACA routes an App FQDN to the container, the
same virtual-host shape as the storage data-plane and Functions invoke. The
backend reaches it via the `EndpointURL` coordinate + the FQDN Host header,
differing from real ACA only in that coordinate (no sim-specific endpoint).

Also landed: a backend WebSocket **keepalive** on `agent.ReverseAgentConn`
(ping ticker + `SetPongHandler`/read-deadline, refreshed on pong+data) that
detects a half-open reverse-agent connection and closes it instead of hanging
forever — a real robustness fix for every FaaS backend; the cloudrun cell
re-verified green. The `bleeplab-runner-docker-build` Makefile target now uses
`docker build --load` (the default `docker-container` buildx driver was
otherwise leaving `bleeplab-runner-int:local` STALE — every `docker run` silently
re-ran an old image). A run of aca-cell hurdle fixes (arch, cache-init-via-agent,
azure-files volumes, sim umask/SELinux, cloud-truth NetworkInspect, stdin-attach
precedence, `/bin/sh` stage) precede the green.

**azf cell — WIP (BUG-1828):** the overlay base-ref (BUG-1826) and the azure
sim's AZF-invoke-reach-by-container-bridge-IP (BUG-1827) are fixed, so the
cache-init one-shot runs, but the runner pattern needs a *persistent* workload
container while the azf FaaS invoke is ephemeral (a container per
`/api/function`, removed after) — so gitlab-runner's later `docker exec` hits
"No such container". The fix is to run the App-Service site container
persistently (the provision pins an EP1/Premium = always-on plan) like an ACA
App and route invoke/ingress/exec to it.

**AWS sim faithfulness (#583/#569, BUG-1827):** ECS now applies the advertised
task/container CPU/Memory to the launched container's cgroup (`HostConfig`
`Memory`/`NanoCPUs` from `ecsContainerResourceLimits`), so a Fargate task is
actually bounded the way its `/task` metadata reports; the process-mode
managed-EBS path was hardened so `ebsRemoveDockerVolume` never dereferences a
nil Docker client. SDK probes `TestECS_TaskDefinitionFidelitySDK` +
`TestECS_ManagedEBSRunTaskProcessMode` (and the azure ACA App SDK suite) stay
green.

## 2026-06-16 - bleeplab GitLab cell on the Cloud Run Functions (gcf) backend (GREEN)

The full gitlab-runner docker-executor flow now also runs on the Cloud Run
Functions backend. A real `gitlab-runner` registers against the bleeplab
control-plane simulator and runs the same 3-stage pipeline through a docker
executor whose `--docker-host` is `sockerless-backend-gcf`: **build** (gcc-
compiles `calc.c`, self-test + `6 x 7 = 42`), **test** (consumes the build's
`calc` artifact with no recompile, `sum 1..100 = 5050`), and **integration** (a
`services:` redis container reached by alias over the per-build network-pod —
`redis-cli` PING/SET/GET). All 4 harness assertions pass. gcf reuses the gcp
simulator and the cloudrun backend's `gcp-common`; the redis `services:` job
exercises the BUG-1781 network-pod (one multi-container Cloud Run revision)
assembly — no BUG-964 default-invoke gate was hit.

The harness gains a `gcf` arm: `bleeplab/Dockerfile` builds
`backends/cloudrun-functions` → `sockerless-backend-gcf` + the
`sockerless-gcf-bootstrap`; `run-integration.sh` gains `provision_gcf` (mirrors
`provision_cloudrun` with `SOCKERLESS_GCF_*` coordinates + `/v1/gcf/reverse`,
**without** `SOCKERLESS_GCR_USE_SERVICE` — gcf runs the native multi-container
revision, not a kept-alive Service); the Makefile gets
`bleeplab-runner-docker-test-gcf`.

Validation surfaced and fixed four real backend/bootstrap bugs — each one a
place where the gcf network-pod execution model diverged from a mechanism the
green cloudrun cell already had:

- **BUG-1811** — `ContainerStart` resolved only from `PendingCreates`, so the
  gitlab-runner per-stage start→wait→stop→start cycle failed `NOT FOUND` once a
  container left PendingCreates. Now falls back to `ResolveContainerAuto`
  (CloudState) and re-adds it, mirroring cloudrun.
- **BUG-1812** — `ContainerAttach` routed a gitlab-runner stdin-script to the
  reverse-agent (whose router has no main process in reverse mode — `mp==nil`),
  failing `no main process to attach to`. The network-pod bootstrap registers a
  reverse-agent for every member, so the stdin path now takes precedence over
  the reverse-agent routing.
- **BUG-1813** — the captured attach-stdin script was piped to the image's own
  entrypoint (`gitlab-runner-build`, which ignores a raw script) instead of a
  shell, so `get_sources` ran but never cloned. Now overrides
  `invokeArgv=[/bin/sh]` when stdin is captured, matching cloudrun's
  `postBootstrap`.
- **BUG-1814** — a reused gcf function instance restored its persist (gcs-
  snapshot) `/builds` only at startup, so `upload_artifacts` couldn't see the
  build container's `calc` (and its stale save clobbered the build's snapshot).
  The bootstrap now `restoreAll(persistVols)` before every invoke; cloudrun gets
  this free via `UseService` cold-starting a fresh per-stage instance.

Cross-cutting: the gcf BackendDescriptor architecture is now derived from
`config.BuildPlatform` via a shared `gcpcommon.ArchFromPlatform` (cloudrun's
local helper promoted to `gcp-common`, used by both backends), and the gcp
simulator's gcf function-invoke path reaches the workload by bridge container IP
(the same fix as the Service path in BUG-1810), so it works when the simulator
itself runs inside the harness container.

## 2026-06-16 - bleeplab GitLab cell on the Cloud Run backend (GREEN)

The full gitlab-runner docker-executor flow now runs on the Cloud Run backend.
A real `gitlab-runner` registers against the bleeplab control-plane simulator
and runs a 3-stage pipeline through a docker executor whose `--docker-host` is
`sockerless-backend-cloudrun`: **build** (gcc-compiles `calc.c` from the cloned
repo, runs the self-test + `6 x 7 = 42`), **test** (consumes the build's `calc`
artifact with no recompile, folds `sum 1..100 = 5050`), and **integration** (a
`services:` redis container reached by alias over the per-build pod network —
`redis-cli` PING/SET/GET). Reproducibly green.

The harness is the one-image, `BLEEPLAB_BACKEND`-switched shape that bleephub
proved: `bleeplab/Dockerfile` now also builds `simulator-gcp` +
`sockerless-backend-cloudrun` + `sockerless-cloudrun-bootstrap` (+ `openssl`);
`run-integration.sh` gains `write_fake_sa_json` + `provision_cloudrun` (GCS
buckets, fake service-account JSON whose `token_uri` is the sim's `/token`,
`gcs-sync` workspace, Cloud Build→Artifact Registry overlay through the sim's
`/v2/` published at `127.0.0.1:5000`, reverse-agent `/v1/cloudrun/reverse`); the
Makefile gets `bleeplab-runner-docker-test-cloudrun`. The CI job markers were
made backend-neutral (`BLEEPLAB-{BUILD,TEST,SERVICE}-OK`) so one CI config
covers every backend. gitlab-runner has no `/runner/externals` tree (that's
github-runner only), so the GitHub externals volume was dropped.

**Validation surfaced + fixed three real bugs** (each grounded in evidence from
the harness, not assumption):

- **BUG-1808** — the cloudrun backend hardcoded `Architecture: "amd64"` in its
  docker `/version`, so on an arm64 host gitlab-runner chose the wrong
  (`x86_64`) helper image. Now derived from `config.BuildPlatform` via
  `archFromPlatform`, mirroring how ECS reports the *workload's* arch.
- **BUG-1809** — the gcp sim's Artifact Registry pull-through hydrated only
  `docker-hub` images from the local daemon, but the backend rewrites
  `registry.gitlab.com/<path>` → `<AR>/gitlab-registry/<path>`, so the
  `gitlab-runner-helper` image 404'd. `hydrateOCIImageFromLocalDocker` now also
  maps `/gitlab-registry/` → the local `registry.gitlab.com/<path>` ref.
- **BUG-1810** — the gcp sim's Cloud Run Service one-shot invoke dialed
  `127.0.0.1:<hostPort>`, which is unreachable when the sim runs *inside* the
  harness container (the host-published port binds the host's loopback, not the
  sim container's). The sim now reaches the workload by its **bridge container
  IP:8080** (routable container-to-container), falling back to the host port
  for a sim running directly on the host. A new bootstrap-stdout-on-failure
  diagnostic (`start_service.go`) made the opaque permission-container exit
  diagnosable. bleephub never hit this — its github-runner containers are
  exec-driven (reverse-agent), never one-shot Service invokes; gitlab-runner's
  cache-volume permission container is the first to exercise the path.

## 2026-06-15 - AZF pod polish (shared volume + per-sidecar exec) + bleeplab artifact UI

Three follow-on enhancements after the BUG-1781 AZF pod assembly shipped.

**AZF pod shared-workspace volume.** A GitHub `services:` / GitLab services job's
members need a shared workspace. `materializePodSite` now dedups every member's
translated named-volume binds into one set, attaches each as a site-level Azure
Files share (`UpdateAzureStorageAccounts`), and declares each member's binds as
the sitecontainer's `SiteContainerProperties.VolumeMounts`. The azure sim
realizes a VolumeMount as a per-(site, volume) Docker named volume bound into
every member (`HTTPContainerConfig.Binds` for the main, `ContainerConfig.Binds`
for sidecars), so members mounting the same volume share one workspace — the pod
analog of an ECS task's shared task-level Volumes. The volume persists across
stages and is torn down when the site is deleted (`cleanupSiteContainers`).

**AZF per-sidecar exec.** Sidecars previously ran their RAW service image with no
agent, so `docker exec <sidecar>` failed. They now run the overlay in *sidecar
mode* (`SOCKERLESS_SIDECAR=1`): the bootstrap dials its own reverse-agent (keyed
to the sidecar's container ID) and execs the service as a long-lived foreground
subprocess WITHOUT binding the function HTTP port (the main owns it in the shared
netns). Because `--network container:` shares `/etc/hosts`, the sidecar resolves
`host.docker.internal` from the main and dials back successfully. So
`docker exec <sidecar>` now works, mirroring Cloud Run (where every pod member
registers an agent). A raw image remains the no-overlay fallback (no per-sidecar
exec). `isAZFOverlaid` detects both on-the-fly and pre-built overlay images.

**bleeplab artifact browse UI.** The GitLab-themed bleeplab dashboard gains an
Artifacts panel on the JobDetail page: `jobView` now carries `artifact_filename`,
a new unauthenticated `GET /internal/jobs/{id}/artifact` route streams the
archive with a `Content-Disposition` filename (the runner-facing
`/api/v4/jobs/{id}/artifacts` stays JOB-TOKEN gated), and the page shows the
filename + size with a Download link when the job produced an artifact.

Proven: `TestAZFMultiContainerPodSharesLocalhost` (integration) now also asserts
a marker file written by the sidecar to the shared `/shared` volume is readable
by the main, and `docker exec` into the sidecar returns its output;
`TestArtifactFlow` asserts the internal job view's `artifact_filename` and the
unauthenticated internal download; the bleeplab UI builds + typechecks + its
vitest passes. `sidecarRunSpec` / `isAZFOverlaid` / pod-volume helpers carry unit
tests.

## 2026-06-15 - FaaS multi-container pod assembly (BUG-1781)

Full pod semantics — including localhost / shared-loopback networking between
members — on the FaaS backends, so GitHub `services:` / sidecar `container:`
jobs and GitLab service containers run there.

**Investigation: the premise was partly stale.** Verified against code, **lambda
and gcf already deliver shared-localhost pods**: lambda runs all pod members as
chroot subprocesses of one supervisor inside a single Lambda execution
environment (one shared netns → `localhost` works); gcf co-deploys members into
one multi-container Cloud Run revision and injects `/etc/hosts` alias→127.0.0.1.
The only backend that still hard-rejected multi-container pods was **azf** — the
gap this work closes.

**azf assembles the pod as ONE App Service site with `sitecontainers`** — the
native Azure multi-container primitive (`Microsoft.Web/sites/{name}/sitecontainers`:
one `isMain` container + N sidecars sharing a network namespace), the Azure
analog of an ECS multi-container task / Cloud Run multi-container revision.
Confirmed real across every surface the testing contract requires: the
`armappservice/v5` SDK (`CreateOrUpdateSiteContainer`/`Get`/`List`/`Delete`),
the `az webapp sitecontainers` CLI, and the vendored `web-arm-openapi-2025-03-01`
spec.

- **azure simulator** (`simulators/azure/sitecontainers.go`): models the
  `sitecontainers` ARM sub-resource (CRUD). `invokeAzureFunctionHTTP` starts the
  `isMain` member as the long-lived HTTP container, then each sidecar with
  `NetworkMode: container:<main>` so they share one netns (a sidecar binding a
  port is reachable from the main on `localhost:<port>`) — mirroring the ACA
  multi-container path. A `startUpCommand` carries an argv across the
  string-typed Azure field via shell-quoting (backend) + a quote-aware splitter
  (sim), so an embedded `sh -c '<script>'` survives. SDK + CLI tests; the
  shared-localhost guarantee is proven by
  `TestSDK_AzureFunctions_MultiContainerSharesLocalhost`.

- **azf backend** (`network_pod.go` + `pod_site.go`): the two fail-fast
  rejections (`PodStart`, `ContainerStart`) are replaced with a network-pod
  materializer mirroring gcf's `shouldDeferOrMaterializeNetworkPod` (pure Docker
  signals: user-defined network membership + `Container.Config.OpenStdin`).
  `materializePodSite` creates one site whose `isMain` sitecontainer runs the
  reverse-agent overlay (the runner execs into it) and whose sidecars run their
  **RAW service images** — sidecars must NOT run the overlay, which would bind
  the main's HTTP port in the shared netns; the pre-overlay image + entrypoint
  are stashed in a container label at create time. Cloud-state reconstructs
  every member from a `sockerless-pod-members` site-tag manifest (stateless — no
  local map). `ContainerCreate` defers site creation for networked containers.

- **azf bootstrap** (`agent/cmd/sockerless-azf-bootstrap`): writes
  `SOCKERLESS_HOST_ALIASES` to `/etc/hosts` (mirror of gcf's `writeHostAliases`)
  so a sibling resolves by name to the shared loopback.

Proven end to end: `TestAZFMultiContainerPodSharesLocalhost` runs a
GitHub-`services:`-shaped pod (job container + a service sidecar) on the azf
sim — the job reaches the sidecar both on `localhost:9099` AND by alias `svc`
over the shared netns. Single-container azf paths re-verified green after the
`ContainerCreate`/`ContainerStart` refactor.

## 2026-06-15 - Cloud Map completeness: one instance, many DNS names (BUG-1804)

Real AWS Cloud Map registers an instance *per service* (`ServiceId`+`InstanceId`),
so one task (one IP) may back several services — i.e. resolve under several DNS
names (verified against the vendored `specs/cloud-api/aws/servicediscovery.smithy.json`
model). The sockerless stack only supported ONE name per container, so a GitLab
`services:` alias (gitlab-runner attaches a service container with network alias
`redis`) never resolved. Two layers were completed:

**aws simulator** — the Docker-network DNS realization connected the task
container to the namespace network with a single alias via a plain
`NetworkConnect`, which Docker rejects on the second registration ("network is
already connected" — verified by hand) so the second `RegisterInstance` 500'd
and only one name resolved. `handleCMRegisterInstance` now stores the instance
first, then re-attaches the container with the FULL set of service names it
backs via disconnect-then-reconnect (the same pattern azure's ACA multi-CNAME
path already uses; multiple aliases per endpoint all resolve, verified);
`DeregisterInstance` re-realizes the reduced set. The netns/`/etc/hosts` tier
already aggregated every name, so only the Docker-network tier needed the fix.

**ECS backend** — `ContainerCreate` dropped the request's
`NetworkingConfig.EndpointsConfig[net].Aliases`, and Cloud Map registration used
only the container hostname. It now captures the aliases into
`EndpointSettings.Aliases` and registers the container under its hostname AND
every alias; `deregisterInstance` enumerates the namespace's services (a
container may back several) rather than the old 1:1 container→service mapping
that leaked the extra registrations.

Proven by `TestECS_MultiServiceDNS` (a client task resolves BOTH of a server's
two service names — `web` and `webalias` — via real Cloud Map DNS in Docker) +
`TestDedupeNonEmpty`/`TestCloudMapNamesFor`; the existing `TestECS_CrossTaskDNS`
still passes.

**BUG-1805 — full gitlab-runner `services:` support (resolv.conf wrapper
removed).** With the alias registered (BUG-1804), a `services:` job's build
container still couldn't resolve `redis`. Root cause (verified on Podman, the
local runtime): each network's DNS runs at its gateway and a container gets one
nameserver per attached network, added as networks connect. The ECS backend
wrapped the user's container command in a `/bin/sh` shim that rewrote
`/etc/resolv.conf` to a STATIC snapshot at entrypoint time (to inject the
namespace as a DNS search domain) — capturing only the VPC nameserver and
dropping the namespace network's DNS that the runtime adds when Cloud Map
connects the container *after* it starts. The wrapper also mangled the user's
argv. Fix, respecting module boundaries: **remove the backend's resolv.conf
command-wrapper** (the container argv runs verbatim; DNS is the runtime's), and
have the **sim** realize each service name as BOTH `<service>` and
`<service>.<namespace>` network aliases (both verified to resolve), matching the
netns/`/etc/hosts` tier — so no search domain is needed. The now-dead
`searchDomainsForContainer`/`shellQuoteArgs` helpers + tests were removed.
Validated end to end: the bleeplab GitLab ECS harness runs a 3-stage pipeline
whose integration stage `apk add redis` + `redis-cli -h redis` PING/SET/GET all
succeed over the per-build pod network (TEST 4).

## 2026-06-15 - bleeplab dashboard UI (GitLab-themed) — completes bleephub parity

bleeplab now ships an embedded dashboard UI, the last piece of bleephub parity
(git + artifacts + UI). It's a React 19 / Vite 6 / Tailwind 4 SPA at
`ui/packages/bleeplab/`, built and `//go:embed`ed into the bleeplab binary
exactly as bleephub's is: `bleeplab/ui_embed.go` (`!noui`, `//go:embed all:dist`,
`spaHandler` mounted at `/ui/`) + `bleeplab/ui_noembed.go` (`noui` no-op),
`UI_PACKAGE := bleeplab` in `bleeplab/Makefile` driving the dist-copy in
`make/go-app.mk`, registered in the root Makefile's `GO_UI_APPS` + `UI_APPS`. `/`
redirects to `/ui/`; deep links fall back to `index.html`; the headless runner
harness Dockerfile builds `-tags noui`.

Views (React Router): **Overview** (status metrics + git/artifact storage backend
+ recent pipelines), **Projects** (+ detail with the project's pipelines),
**Pipelines** (+ detail rendered as a GitLab-style stage graph — one column per
stage, status-coloured job cards, artifact sizes), **Job** detail (ANSI trace via
ui-core's LogViewer), **Runners**. It polls every 5s (React Query) and reuses the
shared `@sockerless/ui-core` primitives (AppShell-less custom Shell, DataTable,
StatusBadge, MetricsCard, ThemeToggle, ErrorBoundary).

The UI is fed by a new **read-only `/internal/*` aggregation API** in bleeplab
(`internal_api.go`) — typed view structs (not `map[string]any`) over the
in-memory control-plane state: `/internal/{status,projects,pipelines,
pipelines/{id},jobs/{id},runners,storage}`. Resource detail still comes from the
public `/api/v4` GitLab surface; `/internal` only adds the dashboard projections
(e.g. "every pipeline across every project") with no clean public-API
equivalent. Tested in-process (`TestInternalAPI`) + a UI unit test.

**Theme** (the explicit ask — "approaching the colour schemes of actual
GitLab", distinct from bleephub): same shared design-token contract, GitLab
values — an indigo/purple action accent (`#6E49CB`), the iconic tanuki orange
(`#FC6D26`) as the brand highlight (wordmark + artifact badges), GitLab Pajamas
status greens/reds/oranges, and a purple-tinted dark mode. bleephub stays
neutral-gray + teal, so the two sims are unmistakable at a glance.

## 2026-06-15 - bleeplab object-store-backed CI artifacts (cross-stage passing)

bleeplab now stores and serves CI job artifacts, object-store-backed exactly
like its git storage (and bleephub): an `artifactStore` chosen by env — an
S3-compatible object store, a filesystem dir (`BLEEPLAB_ARTIFACTS_DIR`), or
in-memory — behind the real GitLab runner endpoints `POST /api/v4/jobs/:id/
artifacts` (multipart upload) and `GET /api/v4/jobs/:id/artifacts` (download).
The CI parser now reads per-job `artifacts:` (name/paths/when/expire_in/
untracked) and `dependencies:`; the job response advertises the upload spec and
a typed `dependencies` list — every earlier-stage job that produced an artifact,
with its size and a download token (an explicit `dependencies:` restricts it,
matching GitLab). This completes the user's "storage based on object store, just
like bleephub" ask (git + artifacts).

The GitLab ECS cell now does real **cross-stage** work: build-job compiles
`calc` and publishes it as an artifact; test-job carries no toolchain, downloads
the artifact, and runs the prebuilt binary — proving the artifact round-tripped
through the store and that the test stage depends on the build stage's output.

**Coordinate finding (artifact reachability).** gitlab-runner's in-container
`artifacts-uploader`/`-downloader` use the runner's *config `url`*, not
`CI_API_V4_URL`, so that URL must be reachable from inside the job/helper
container — `http://127.0.0.1:8929` (the harness loopback) gave `connection
refused`. The fix is a coordinate: the runner config `url` is set to
`host.docker.internal:8929`, which resolves to the harness loopback from the
runner process (via `/etc/hosts`) and to the published port from the workload
containers — one URL that works from both vantage points. (bleeplab also sends
`CI_API_V4_URL` in the job variables, as real GitLab does.)

Typed over `any`: the `dependencies` entries are a `jobDependency` struct
(`id`/`name`/`token`/`artifacts_file`), not `map[string]any` in `[]any`. Tested
in-process by `TestArtifactFlow` (upload → dependency advertisement → download,
byte-for-byte) and end-to-end by the harness (TEST 3).

## 2026-06-15 - bleeplab serves git + ECS restart preserves volumes → the GitLab ECS cell builds & runs a real program (BUG-1801, BUG-1802)

The single-job bleeplab GitLab ECS cell is GREEN and does **real work**: a real
`gitlab-runner` (docker executor, `--docker-host` = sockerless ECS backend)
clones the project, then a two-stage pipeline `apk add gcc` + `gcc -O2 -Wall
-Werror -o calc calc.c` and runs a real C arithmetic calculator from the cloned
source — self-test plus verified arithmetic (`6 x 7 = 42`, `7 + 4 = 11`,
`100 / 7 = 14`, `17 % 5 = 2`, and folding `calc` over 1..100 to `5050`). Two
fixes landed it:

**BUG-1801 — bleeplab serves each project as a real git repository.** Without a
git server the harness used `GIT_STRATEGY: none`, so the runner never created
`CI_PROJECT_DIR` and `cd $CI_PROJECT_DIR` failed. bleeplab now serves git over
smart-HTTP with pure-Go **go-git** (`/info/refs` + `git-upload-pack` /
`git-receive-pack`), object-store-backed exactly like bleephub: an `s3fs`
go-billy filesystem → go-git Storer chosen by env (`BLEEPLAB_S3_BUCKET` >
`BLEEPLAB_GIT_DIR` > in-memory). A commit through the GitLab commits API writes
a real go-git commit (additive create/update on the branch) and records the real
SHA; `git_info.repo_url` points at `<BLEEPLAB_EXTERNAL_URL>/<ns>/<project>.git`
(reachable from the job/helper container via `host.docker.internal:8929`) and
`git_info.{sha,refspecs}` drive a faithful clone. The harness switched to
`GIT_STRATEGY: clone`. bleeplab accepts the runner's `gitlab-ci-token` exactly
as GitLab does — coordinate-only, no sockerless-aware special-casing. Validated
in-process by `TestGitCloneSeededProject` (a real go-git client clones; HEAD ==
commit SHA; a second commit is additive).

**BUG-1802 — the ECS backend preserves a container's volume binds across
restart.** gitlab-runner's docker executor runs each build stage by re-starting
the *same* predefined helper container (`create` once → `attach→start→wait→stop`
per stage). On ECS each `/start` spawns a fresh deferred task; the **first**
start carried the `/builds` EFS mount (resolved from `PendingCreates`, which
holds `HostConfig.Binds`), but **later** starts resolved the container from cloud
state via `taskToContainer`, whose reconstructed `HostConfig` dropped `Binds`
entirely — so the re-registered task def had no volume mounts and `get_sources`
cloned into ephemeral storage the build container couldn't see (`cd
/builds/root/demo` → "No such file or directory"). Diagnosed with per-container
DIAG logging: all stage containers resolved the *same* access point, yet the sim
showed two helper tasks with `mountPoints=0 binds=[]`. Fix (backend-side,
stateless): `taskToContainer` reconstructs the named-volume binds from the task
definition's mount points (`SourceVolume:ContainerPath[:ro]`), so every restart
re-registers a task def with the original volumes. Unit-tested
(`TestTaskToContainer_BindsFromMountPoints`). No sim change — the sim already
shares an access point's host dir across tasks deterministically.

(Earlier on this branch the "volume not shared across stages" theory was a
stale-data-dir artifact: the harness `rm -rf` couldn't clear root-owned EFS
files from prior runs, so disk archaeology saw cross-run filesystems. The
authoritative diagnosis came from logging the *resolved* access point and the
*applied* binds, not from inspecting the accreted data dir.)

Still open for full bleephub parity (the user asked for both): a bleeplab
artifact **ArtifactStore** (object-store-backed) and a bleeplab **UI** — the
git/object-store slice is the piece that landed here.

## 2026-06-15 - aws sim EFS access-point writability for GitLab workloads (BUG-1800)

After BUG-1798, the bleeplab GitLab ECS build `step_script` failed at
`mkdir: can't create directory '/builds/project-1.tmp': Permission denied`. Two
sim-side EFS gaps, both fixed:

1. **CreationInfo ignored.** `EFSAccessPointHostDir`/`EFSFileSystemHostDir`
   created the host dir with `os.MkdirAll(…, 0o777)`, whose mode the umask
   reduces to `0755 root` — and the access point's `RootDirectory.CreationInfo`
   (the gitlab `/builds` volume requests `0777`, uid/gid 1000) was never applied.
   New `ensureAccessPointRootDir` applies `CreationInfo.{Permissions(chmod, not
   umask-masked), OwnerUid, OwnerGid(chown, best-effort)}` on creation only (so a
   workload's later perm changes aren't clobbered), defaulting to `0777`.
   Unit-tested (`efs_creationinfo_test.go`).
2. **SELinux.** On an SELinux-enforcing host (a local podman machine) the
   sim-spawned ECS task runs confined as `container_t` and can't write the EFS
   host dir even at `0777`. The sim now mounts task EFS binds with the `z`
   (shared relabel) option → relabels them `container_file_t`; a no-op on hosts
   without SELinux (Docker on CI), so it removes the bleephub harness's manual
   `chcon` note for the bleeplab path.

Validated on a frozen stack: the access-point dir is now `drwxrwxrwx 1000 1000
… container_file_t`, the `/builds` write succeeds, and the cell advances past
the permission error to the next gate — BUG-1801, where the `/builds` volume
doesn't persist across the per-stage Fargate tasks (`cd /builds/project-1` → No
such file or directory; the same docker volume resolves to a different EFS
access point per task). aws sim ECS/EFS SDK tests stay green. No backend
coupling.

## 2026-06-15 - ECS gitlab-runner attach-stdin gate closed (BUG-1798)

Fixed the Phase-3 gate for the bleeplab GitLab ECS cell. gitlab-runner 18's
docker executor does `create(OpenStdin) → /attach(stdin) → /start` (no
`docker exec`) and pipes the stage script to the helper container's stdin (its
default `gitlab-runner-build` reads it). On ECS the `/start` deferral that bakes
the captured stdin into the task command requires the stdin pipe to already
exist + be open — but `ecsStdinAttachDriver.Attach` created the pipe only
**after** a stage-boundary barrier that itself **waits for `/start`** to
register a WaitCh. A dependency inversion: `/start` arrived first, found no pipe
(DIAG: `pipe_exists=false`), fell through, and launched the helper's image-
default command, which hung forever waiting for stdin.

Fix (backend-side, no sim coupling): create + open the stdin pipe **before** the
barrier in the attach driver, and have `ContainerStart` wait briefly
(`waitForOpenStdinPipe`, 5s) for the open pipe before deciding — closing the
create→attach→start race from both ends. Root-caused with temporary DIAG logs
(removed) that captured the exact ordering. Validated: the harness now runs the
helper stages and delivers the script into the build container — the hang is
gone; it advances to the build `step_script`, blocked next only by BUG-1800 (the
aws sim doesn't apply EFS access-point `CreationInfo`, so the `/builds` volume is
`0755 root` and the build job can't write — the next, sim-side gate).

## 2026-06-14 - bleeplab ECS harness + arch-aware image pull (Arc 3 Phase 3, WIP)

Phase 3 points a real `gitlab-runner` 18.11's docker executor at a sockerless
backend. New `bleeplab/Dockerfile` + `bleeplab/test/run-integration.sh` +
`make bleeplab-runner-docker-test-ecs`: the harness provisions a sim-backed ECS
backend (the bleephub `provision_ecs` shape), starts bleeplab, registers the
runner with `[runners.docker] host = tcp://…:3375`, triggers a pipeline, and
asserts success. The runner registers, claims a job, uses sockerless as its
docker host, and image pull + build/helper container create all work.

**BUG-1797 (fixed) — arch-aware image manifest selection.** `core/registry.go`
hardcoded `linux/amd64` when picking from a multi-arch manifest list. The local
sims run workloads on the host engine, so on arm64 hosts the workload is arm64 —
fine for multi-arch images (alpine), but the gitlab-runner-helper `arm64-…` tag
is arm64-only, so the amd64-only selection failed the pull. Fix: select the
manifest matching `SOCKERLESS_WORKLOAD_ARCH` (default amd64 — live unchanged),
falling back to amd64 before erroring; the policy is extracted to
`selectPlatformManifest` and unit-tested. The harness sets the env from `uname`.

**BUG-1798 (open) — the Phase-3 gate.** With the arch fix, the runner reaches
`Preparing environment` and hangs: modern gitlab-runner 18 does
`create(OpenStdin) → attach(stdin) → start` (no `docker exec`) and pipes the
stage script to the helper's stdin, but the ECS deferred-RunTask runs the
helper's image-default `gitlab-runner-build` instead of baking the captured
stdin, so it waits for stdin forever. The next iteration debugs the ECS
attach-stdin deferral for this gitlab-runner-18 helper shape (the path was built
for `docker run -i sh`).

**BUG-1799 (fixed) — proactive: a dangling `sim (aws sdk)` flake.** The PR's CI
surfaced an intermittent `TestECS_TaskArithmetic*` failure (container `ExitCode
-1`) that re-ran green. Root cause: the awsvpc netns-tier `busybox` **pause
container** image was pulled lazily at RunTask time, making a transient
ECR-gallery throttle a per-task lifecycle dependency, recorded only in the task
`StoppedReason` and surfaced as an opaque `-1`. Fixed by pre-pulling busybox in
`TestMain` with retry (the established pattern; busybox backs many ECS tests) +
logging the start failure to stderr so any residual netns flake is diagnosable.
Sim/test-side only — respects the hard sim↔backend code-isolation rule.

## 2026-06-14 - bleeplab: GitLab control-plane simulator (Arc 3 Phase 1)

Started Arc 3 (GitLab docker-executor parity) with the missing piece the
scoping identified: a **GitLab control-plane simulator**, `bleeplab` — the
GitLab analog of `bleephub`. The backend docker-executor attach-stdin path was
already built and proven (GL-1…GL-11 closed); what was absent was a control
plane a real `gitlab-runner` could poll (existing GitLab harnesses used a 4 GB
`gitlab-ce` container, real gitlab.com, or `gitlab-ci-local` which bypasses the
runner API).

`bleeplab` (new module, `cmd/main.go` on `:8929`) implements the real GitLab
API slices a docker-executor runner + orchestrator exercise: the **runner API**
(`POST /api/v4/jobs/request`, `PATCH/PUT /api/v4/jobs/:id`, runner verify/
register/unregister) and the **project/pipeline API** (projects, commits,
pipeline trigger, pipeline/job status, job trace), plus a minimal
`.gitlab-ci.yml` parser (stages, image, script lifecycle, services, variables)
with a stage-gated job queue — the next stage enqueues only after the previous
one succeeds. Fidelity, not fakery: the runner authenticates + polls exactly as
against gitlab.com; bleeplab differs only in coordinates.

Validated end-to-end with a real `gitlab-runner` 18.11.3: it registers, claims a
job via `/jobs/request`, pulls the helper + alpine images, runs the script
(`echo` + `cat /etc/os-release`) on the docker executor, streams the full CI
trace back, and the pipeline rolls up to `success`. Fixed one wire-shape bug
the real runner caught: the job-request `features` object is mixed-type
(`trace_sections` bool vs `failure_reasons` list) so it must be `map[string]any`.
Unit test `TestFullPipelineLifecycle` drives the whole control-plane + runner
flow in-process. Registered in `go.work` + the `core-local` CI shard. Next:
point the runner's `--docker-host` at a sockerless backend (Phase 3).

## 2026-06-14 - GCF (Cloud Run Functions) cell GREEN + exec-via-agent observability (BUG-1795/1796)

The bleephub **gcf** harness now passes **TEST 1–14** against the gcp simulator —
all four container backends (ECS, ACA, Cloud Run, GCF) are GitHub-topology
sim-proven. GCF Gen2 deploys container-jobs as Cloud Run Service revisions, so
the cell reuses the cloudrun overlay + gcs-sync model and the gcp sim needed no
change.

**BUG-1795 — GCF cell bring-up.** Five gaps, each the GCF twin of a cloudrun
fix: (1) the GCF `Typed.Exec` was wired to the bare `ReverseAgentExecDriver`, so
the HTTP exec path (`handleExecStart → Typed.Exec`) bypassed `s.ExecStart` and
its materialize/gcs-sync logic was dead code — rewired through `s.ExecStart` via
`WrapLegacyExecStart`; (2) added `materializeDeferredNetworkPodForExec` (a
no-`services:` job is deferred at ContainerStart); (3) added `warmBootstrap`
(BUG-1794 twin — cold-start the scale-to-zero Service via `/_sockerless/ready`
without running the keepalive); (4) the gcf bootstrap gained the readiness route
+ WS-exec gcs-sync `ExecHooks` (`ServeReverseAgentWithExecHooks`); (5) the gcf
bootstrap's `persist.go` now honours `STORAGE_EMULATOR_HOST` (`gcsBase`/
`gcsAuthToken`/`setGCSAuth` — the #568 prereq was never ported; the workload's
metadata-token fetch 404'd) and the backend injects it (`SOCKERLESS_GCS_WORKLOAD_ENDPOINT`)
+ runs a `gcsSyncPreExec`/`execPostHook` data plane. Plus harness plumbing
(`provision_gcf` + `gcf` case, `bleephub-runner-docker-test-gcf`, both GCF
binaries in `bleephub/Dockerfile`). Coordinate-only, no `if sim` branch.

**BUG-1796 — exec-via-agent observability.** The GCF bring-up exposed that a
reverse-agent `TypeError` (e.g. a failed gcs-sync restore PreExec hook) was
swallowed by `ReverseAgentConn.bridge` — the step failed with an opaque
`exit 255` and the real cause (`metadata token status 404`) lived only in the
workload's own stderr, a separate stream with no exec-ID correlation. Fixed
across the shared core + agent (so every FaaS backend benefits): `bridge` writes
the agent error to the caller's stderr frame and returns `AgentExecErrorExitCode`
(255); `BridgeExec` fails fast if the initial dispatch send fails;
`core/handle_exec.go` logs exec dispatch (with the resolved driver via
`Describe()`) + completion; `ReverseAgentExecDriver.Exec` logs dispatch + exit
code; `HandleReverseAgentWS` logs missing-`session_id`/upgrade/register/replace/
drop; the bootstrap logs serve-loop start/teardown + malformed messages, checks
its final exit-frame send, and a nil-safe `OnDroppedMessage` callback surfaces
full-channel drops. Pure-additive logging except the `TypeError`→stderr+255
behavior fix.

## 2026-06-14 - Cloud Run GitHub-topology cell GREEN (BUG-1794 + BUG-1792 closed)

The bleephub cloudrun harness now passes **TEST 1–14** end-to-end against the
gcp simulator — the Cloud Run container backend is sim-proven for the full
build→push→pull→deploy→materialize→reverse-agent→exec→gcs-sync pipeline, joining
ECS and ACA. Two bugs closed.

**BUG-1794 — the exec-driven Service never cold-started.** A GitHub container
job is materialized as a scale-to-zero Cloud Run Service whose keepalive
(`tail -f /dev/null`) must not run as a request. The materialize path therefore
*skipped the default-invoke entirely* — but a scale-to-zero Service that never
receives a request never creates its first instance, so the overlay bootstrap
never started and never dialed the reverse-agent, and `docker exec` hung. Fix:
the overlay bootstrap serves a `/_sockerless/ready` route (HTTP 204, runs no
user command) and the backend POSTs to it (`warmBootstrap` in
`start_service.go`) to cold-start the revision *without* running the keepalive.
The gcp sim's `/v2-services-invoke/` handler forwards the request path + query
to the bootstrap (a `{path...}` route variant + path/query params on
`postCloudRunServiceInstance`) so the readiness route reaches the bootstrap
instead of collapsing to `/`. Covered by `TestSDK_CloudRunV2Services_ForwardsRequestPath`
(new `echo-request` probe mode) and `TestHandleReady_DoesNotRunDefaultCommand`.

**BUG-1792 — gcs-sync validated.** With the data plane wired (#570), the last
gap was the resumable-upload continuation URL: the sim emitted `Location:`
hardcoded to `https://`, so the official Go storage client — pointed at the
explicit HTTP sim coordinate — followed it and failed `server gave HTTP
response to HTTPS client`. Fix: derive the continuation-URL scheme from the
request (`requestScheme`: `X-Forwarded-Proto` / `r.TLS`), matching real GCS
(HTTPS) and a custom HTTP coordinate alike. Also added the JSON-API `alt=media`
object download, resumable-chunk-via-`POST` (`upload_id`), and `bytes */<total>`
Content-Range parsing the storage client uses. Proven by
`TestGCS_ResumableWriterFollowsCustomEndpoint` + `TestGCS_JSONAPIObjectGetAltMedia`
(official Go storage SDK) and the TEST 12 workspace round-trip (`proof.txt`
written inside the job container is visible in the runner workspace).

## 2026-06-14 - Cloud Run gcs-sync prerequisites + BUGS.md count correction (BUG-1792 partial)

Investigating the last cloudrun-cell TEST 12 gate (every `docker exec` aborts
at exit 255) showed BUG-1792 is bigger than a hardcoded URL: the gcs-sync
per-exec workspace data plane (`GCSSyncDriver.PreExec`/`PostExec`) has **no
callers** — the cloudrun exec path never uploads the workspace to GCS or feeds
the bootstrap a `SOCKERLESS_SYNC_VOLUMES` hint, so the workspace tmpfs stays
empty and the exec's workdir doesn't exist. Cloud Run container-jobs were never
proven end-to-end.

Landed the prerequisites the data-plane wiring will need: the bootstrap's
gcs-sync (`persist.go`/`persist_sync.go`) honours the standard
`STORAGE_EMULATOR_HOST` (a `gcsBase()` helper; unauthenticated emulator mode,
so no metadata-token dependency), and the cloudrun backend injects a
workload-reachable storage coordinate (`SOCKERLESS_GCS_WORKLOAD_ENDPOINT` →
`STORAGE_EMULATOR_HOST` on the task, default empty = real GCS + ADC). The
workload reaches the sim's storage through the same host-gateway/published-port
path the reverse-agent callback uses. Real cloud is unchanged.

Also corrected the BUGS.md ledger: #567 filed BUG-1789/1790/1791 into the Open
table but never struck them when it fixed them in the same PR — the header read
`1745 fixed / 7 open` instead of `1748 / 4`. The remaining BUG-1792 work (wire
`PreExec`/`PostExec` around the exec dispatch) is its own iteration.

## 2026-06-14 - Cloud Run GitHub-topology cell bring-up (partial)

Extends the bleephub GitHub-topology harness (ECS- and ACA-proven) to the
Cloud Run backend, run entirely against the gcp simulator. New
`provision_cloudrun()` cell, `bleephub-runner-docker-test-cloudrun` Make
target, and an image bundling `simulator-gcp` + `sockerless-backend-cloudrun`.
The cell shares the runner workspace via GCS snapshot-sync (gcs-sync), builds
the reverse-agent bootstrap overlay via Cloud Build, and pushes/pulls it
through the sim's `/v2/` by the registry coordinate (BUG-1785).

Six real bugs surfaced and fixed bringing the cell up — the whole pipeline
now runs against the sim: overlay **build → push → pull → deploy →
materialize → reverse-agent dial-back → step exec**.

- **BUG-1789** (gcp-common, two facets): the overlay base-image rewrite and
  the AR auth/registry-endpoint override ignored the `SOCKERLESS_GCP_AR_ENDPOINT`
  coordinate, so the overlay `FROM` and the backend's image-metadata fetch hit
  the real Artifact Registry host. `ResolveGCPImageURI` now builds the host via
  `OverlayRegistryHost`, and `ARAuthProvider` recognises the coordinate host
  (`IsOverlayCoordinateRegistry`) so registry HTTP routes through the backend's
  reachable endpoint. Real cloud unchanged when the coordinate is unset.
- **BUG-1790** (gcp sim): the AR docker-hub pull-through (`hydrateOCIImageFromLocalDocker`)
  served a manifest mixing an OCI manifest type with a Docker v2s2 config type —
  tolerated by `docker pull`, rejected by `docker build`'s `FROM`. Now full OCI.
- **BUG-1791** (cloudrun, two facets): a GH container-job with no `services:`
  was deferred forever (the network-pod defer waited for a sibling that never
  arrived). Added **materialize-on-exec** — the runner always `docker exec`s its
  job container, which lazily deploys the Cloud Run Service (bundling any
  deferred service siblings). And `startSingleContainerService` no longer
  default-invokes an exec-driven container's `tail -f /dev/null` keepalive
  (which ran it as a request and hit the request-lifetime SIGTERM); the
  `skipDefaultInvoke` flag matches the multi-container path's existing skip.

The final TEST 12 gate, **BUG-1792**, is open: the bootstrap's gcs-sync
restore/save hardcodes `https://storage.googleapis.com` and can't reach the
sim's storage from the workload, so each exec aborts at exit 255. Staged as
the next iteration (bootstrap `STORAGE_EMULATOR_HOST` + the workload→sim
published-port reachability the reverse-agent already uses). TEST 13/14 follow.

## 2026-06-14 - Faithful build→push→pull for gcp Cloud Build (BUG-1785, gcp half — closes the bug)

The gcp half mirrors the azure ACR Tasks fix below and completes BUG-1785.
The gcp Cloud Build sim built the overlay into the host's local docker
daemon and the Cloud Run / GCF workload ran that local copy — the sim's
registry never reflected the build, a non-faithful shortcut.

- The Cloud Build `push` step (`simulators/gcp/cloudbuild.go`) now does a
  real `docker push <ref>` + `docker rmi`, exactly as real Cloud Build with
  `IsPushEnabled`. The registry, not the build host, holds the image; the
  Cloud Run / GCF workload pulls it over the standard `/v2/` API.
- The overlay registry host is a **coordinate**: `gcpcommon.OverlayRegistryHost`
  reads `SOCKERLESS_GCP_AR_ENDPOINT` (default = the real
  `<region>-docker.pkg.dev`), parallel to `SOCKERLESS_AZURE_ACR_ENDPOINT`.
  The backend builds the *real* registry ref for cloud and sim alike.
- The cloudrun + cloudrun-functions integration harnesses set that coordinate
  **per-target, exactly like `endpointURL`** — to the sim's published `/v2/`
  at `127.0.0.1:<port>` (Docker auto-trusts loopback as insecure). There is
  **no `if sim` / `if target == "sim"` branch** in backend or test code: a
  sim run differs from a cloud run only in coordinates, so the client path
  is identical and the test proves the real path, not a sim-special one.
- `TestCloudBuild_FaithfulBuildPush` asserts the built image lands in `/v2/`
  and is gone from the local daemon. The `test (gcp backends)` and gcp/gcf
  faas-smoke CI jobs (which always build overlays) exercise the full
  build→push→pull round-trip against the sim.

This is the same lesson as the azure half, generalized into a rule: the
coordinate-only pattern is now documented in
[specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md)
§ "Faithful build → push → pull" and [AGENTS.md](AGENTS.md) § "A sim test
differs from a cloud test ONLY in coordinates", cross-linked both ways.

## 2026-06-13 - Faithful build→push→pull for ACR Tasks (BUG-1785, azure half)

The ACR Tasks sim built the overlay into the host's local docker daemon and
the ACA App ran that local copy — so the sim's registry never reflected the
build, and the run used a non-faithful shortcut. The user flagged it: the
sim must not rely on functionality that isn't strictly faithful to the
cloud. A first attempt to close it went wrong — it coupled the shared
workload runner directly to the registry's in-process store, a dependency
that doesn't exist in the cloud (compute pulls from the registry over the
public `/v2/` API like any client) — and was reverted.

The faithful fix keeps the sim's services agnostic:

- The ACR Tasks build now does a real `docker build` + `docker push` to the
  registry + `docker rmi`, exactly as real ACR Tasks (IsPushEnabled). The
  registry, not the build host, holds the image.
- The ACA App run pulls it over the standard registry API (the existing
  `StartContainerSync` pull path). Registry and compute talk only through
  `/v2/` — no in-process coupling.
- The backend honors a configurable ACR registry endpoint
  (`SOCKERLESS_AZURE_ACR_ENDPOINT`, a legit sovereign/custom-cloud override)
  so the harness can point the overlay ref at a reachable, auto-insecure
  endpoint.
- The harness publishes the sim `/v2/` at `127.0.0.1:5000` and (only on a
  podman machine, which unlike Docker doesn't auto-trust loopback
  registries) drops a scoped, idempotent insecure-registry entry. On Docker
  and Linux CI it's a no-op.

Validated end-to-end: ACA harness TEST 12 (container-mode job) passes with
the real build→push→pull, and the ACR Tasks SDK test asserts the built
image lands in a real registry (a throwaway `registry:2` stand-in) and is
gone from the local daemon. The gcp Cloud Build half of BUG-1785 — same
pattern, but it must thread the cloudrun/gcf overlay flows and their
integration tests — remains as a separate, larger change.

## 2026-06-13 - ACA GitHub container-job topology: TEST 12 green (BUG-1782 + BUG-1783)

Got the GitHub container-mode job (TEST 12) passing on the **ACA** backend
through the bleephub official-runner harness — the first container backend
beyond ECS to run a container job end-to-end, and the validation that the
whole ACA App-overlay + reverse-agent path works.

Two backend/harness fixes made it work, plus the `provision_aca` wiring:

- **BUG-1782:** `NewACRBuildService` (backends/azure-common) ignored
  `SOCKERLESS_ENDPOINT_URL` — its ARM + azblob clients targeted real Azure,
  so the App-overlay bootstrap-build path couldn't reach the sim. Now it
  threads the endpoint: ARM `RegistriesClient`/`RunsClient` use the
  `cloud.Configuration` override (+ `InsecureAllowCredentialWithHTTP`), and
  the blob client is resolved lazily from the storage account's advertised
  `primaryEndpoints.blob` (via `armstorage` GetProperties) — the faithful
  way to reach storage on a custom/sovereign/simulated cloud. Both
  `aca`/`azf` call sites pass `config.EndpointURL`.
- **BUG-1783:** the bleephub `Dockerfile` built `sockerless-cloudrun-bootstrap`
  + `sockerless-agent` glibc-dynamic (CGO on by default under `golang:1.25`).
  Baked into an alpine overlay they failed to exec with `No such file or
  directory` (missing dynamic loader), so the bootstrap never dialed back
  and ACA exec timed out. The canonical agent Makefile already uses
  `CGO_ENABLED=0`; the harness Dockerfile was the anomaly — now static too.

`provision_aca` now drives the App-overlay path: `SOCKERLESS_ACA_USE_APP=1`
+ ACR + a build-context blob container + an arch-matched build platform,
and pins a deterministic `<account>.blob.localhost` storage endpoint
(`SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON` + an `/etc/hosts` alias,
since `*.localhost` isn't special-cased by the container resolver). The
full chain — sim ACR Tasks builds the overlay → ACA App runs it → static
bootstrap dials back → `docker exec` job steps — runs green.

TEST 13 (service container) is the next hurdle: a sibling service App's
alias doesn't resolve from inside the job App (filed **BUG-1784**); TEST 14
(dispatcher) needs a published-port wiring fix. Then Cloud Run + GCF.

## 2026-06-13 - Azure sim ACR Tasks slice (overlay-build keystone for ACA/AZF)

Added a faithful **ACR Tasks quick-build** slice to the azure simulator —
`POST .../registries/{name}/scheduleRun` (DockerBuildRequest LRO) + `GET
.../runs/{runId}`. This is the cloud-API the Azure backends call to build
their reverse-agent bootstrap **overlay image**: `backends/aca` and
`backends/azure-functions` issue `RegistriesClient.BeginScheduleRun` with a
`DockerBuildRequest` and poll the run. The handler fetches the build
context from the sim's blob storage (where the backend's azblob upload
landed), runs `docker build` on the host engine — the sim's build
infrastructure, exactly as the GCP Cloud Build slice
(`simulators/gcp/cloudbuild.go`) does — and tags the image into the local
daemon, where `StartContainerSync` resolves it by tag without a registry
pull. The run completes synchronously and is returned as a 200 with a
terminal-state `Run` body, which the azure-core LRO poller resolves via its
NopPoller path (a 202 would be a hard error for a POST LRO). No
sockerless-aware special-casing: any ACR Tasks client reaching `scheduleRun`
gets the same behavior. SDK tests (`acr_tasks_test.go`) exercise the full
path — upload context, BeginScheduleRun, assert the image is really present
in the local daemon, GetRun round-trip, and a missing-context build that
reports the Run as `Failed`.

Standing up the ACA topology cell on this slice surfaced **BUG-1782**:
`NewACRBuildService` (backends/azure-common) ignores `SOCKERLESS_ENDPOINT_URL`
— it builds the ARM + azblob clients against real Azure — so the App-overlay
path can't reach the sim (or any custom cloud) yet. Filed; the fix (thread
the endpoint override + target the account's advertised blob endpoint) plus
the harness wiring (UseApp, ACR, build-context container) and the
reverse-agent exec validation are the next steps of the Arc-2 ACA build.

## 2026-06-13 - All-backend metadata network driver + experiential-parity principles (Arc 2 groundwork)

Stand-up of the ACA cell of the bleephub GitHub topology harness (Arc 2)
surfaced and fixed a cross-backend defect, and the work crystallized the
principles that govern the rest of the pod + runner sweep. The harness
plumbing itself (multi-backend image, `BLEEPHUB_BACKEND` parameterization)
lands with the ACA cell in the following arc, once container-job exec is
assembled through faithful cloud APIs.

- **BUG-1780:** only the ecs backend overrode `Drivers.Network` to the
  metadata-only `SyntheticNetworkDriver`; lambda/cloudrun/gcf/aca/azf fell
  through to `BaseServer.InitDrivers`' real-Linux-netns driver (`ip netns
  add` + veth). That 400s a `docker network create github_network_<hex>`
  on a host without iproute2 and leaks a meaningless kernel netns where it
  succeeds. Docker networks on these backends map to *cloud* primitives
  (Lambda VPC ENIs, Cloud Run/GCF VPC-connector + Cloud DNS, ACA + AZF NSG
  / Private DNS), never a local netns — so all five now mirror ecs. All
  six cloud backends' test suites pass; a harness run confirmed ACA's
  `docker network create`/`delete` now provisions + tears down the NSG +
  Private DNS zone cleanly.

Codified the **experiential-parity principle** the user articulated:
sockerless backends are providers of the Docker+Podman REST API assembled
out of cloud primitives, and the goal is that a user's experience with
containers, pods, networks, and volumes inside any backend is the same as
local Docker/Podman. Every Docker abstraction is *composed* from cloud
primitives on every backend, FaaS included — including localhost /
shared-loopback networking between pod members — and a FaaS platform that
can't run multiple containers per function is our job to assemble (native
sidecars where offered, else a pod from multiple functions wired by cloud
DNS + a shared volume, with the agent proxying localhost to siblings), not
to reject. And the **sims stay faithful cloud slices** — no special / fake
functionality layered on to support sockerless backends or runners; a
backend's need is met by implementing the real cloud API, never a sim-side
hook. Written into AGENTS.md (new "Assemble Docker abstractions" section +
the cloud-API-fidelity rules-out list), CLOUD_RESOURCE_MAPPING.md
(universal rule 8), and the AZF README; filed **BUG-1781** and staged the
FaaS multi-container assembly as PLAN § Next #1 (replacing the interim
fail-fast rejections).

## 2026-06-13 - Pod-model correctness across backends (Arc 1 of the pod + runner focus)

Opened a sustained focus on the pod model and GitHub/GitLab runner
integration across all backends. Built a verified gap matrix first
(correcting the recon agents' over-claims): only Lambda is live-proven
(BUG-1075); the GitHub container-job topology is sim-proven for ECS only
(the bleephub harness); the other backends have per-backend GitLab
stdin-attach unit tests but no full-topology proof; AZF can't run
multi-container pods. Arc 1 fixed the verified pod-model bugs:

- BUG-1778: Lambda delegated all four Pod lifecycle methods, and GCF
  delegated Stop/Kill/Remove, to BaseServer.Pod* — which read
  Store.Containers and call Store.ForceStopContainer (local in-memory,
  no cloud call), so `docker pod stop/kill/rm` (and lambda `pod start`)
  left the underlying Lambda function / Cloud Run Service running.
  ECS/Cloud Run/ACA already override these to loop their cloud-aware
  Container* methods; lambda+gcf now mirror that. The isolation lint
  forbade BaseServer.Container* but not BaseServer.Pod{Start,Stop,Kill,
  Remove} — that gap is closed so the leak class can't recur.
- BUG-1779: AZF's single-invocation multi-container rejection fired
  late (only at the 2nd ContainerStart, after create succeeded);
  PodStart now rejects up front with one clear error, and the
  constraint is documented in the azf README (rules out `services:` and
  sidecar `container:` jobs on AZF).

Verified non-issue (left as-is): GCF injects pod host-aliases on the
main container only — correct, because sidecars are raw user images
that don't run the sockerless bootstrap, so only main can write
/etc/hosts; main→sidecar (the services-contract direction) works and
sidecar→sidecar-by-name isn't achievable or needed.

## 2026-06-13 - Pod-model / lifecycle review fixes (bundled into the audit PR)

Reviewed the pod + container lifecycle across all 7 backends for needless
delays, fixed timeouts, polling inefficiency, and constraint mismatches.
Verified each high-severity claim against source (the review agents
overstated several, as on the sim audit). Two genuine weaknesses fixed:

- BUG-1776: ACA `attachStream.Read` did a bare `<-respReady` with no
  deadline — the AZF twin bounds the identical buffered-attach wait with a
  deadline (the BUG-1505 fix) but it was never ported. A stalled or
  lifetime-capped ACA Job would strand an attached docker/StdCopy reader
  forever. Ported the AZF pattern (bootstrap window + job-run budget).
- BUG-1777: the recovery-path `WaitForExit` (cloudrun + aca) re-ran a full
  ListJobs+ListServices (with per-pod GetService follow-ups) on every tick
  to check one container's exit. Narrowed to resolve the backing Job once
  then poll that single job's state — the identical resolveExecutionState /
  resolveJobState derivation, one resource — with a list fallback for
  Service/App-backed and vanished jobs. Cloud Run's `waitForServiceURL`
  flat-2s/150-call loop got exponential backoff (1s→15s). GCF left alone
  (its hot path is already event-driven; FaaS has no persistent exit
  state to narrow to).

Deliberately NOT changed (verified non-issues): pod deferred-start has no
timeout but doesn't block a goroutine and mirrors docker-compose semantics;
the 30s stdin-capture waits are warn-and-proceed generous upper bounds;
the O(n) queryTasks/queryFunctions is the documented stateless-invariant
cost; the ACA app-readiness "returns before replica ready" is masked by the
reverse-agent wait in the common path and the multi-container case is
intra-revision (no cross-app DNS) — adding an unconditional revision-ready
poll would regress hot-path latency for an unproven race. The forced delays
(Fargate task-start, Lambda/GCF/AZF limits, ARM LROs) are genuinely
cloud-imposed. The hot-path exit detection (pollTaskExit/pollExecutionExit)
is already single-resource + event-driven; pod materialization already
builds member overlays in parallel and deploys atomically.

## 2026-06-13 - Sim fidelity audit pass (probe the load-bearing gaps)

Ran a registered-op-vs-test-coverage sweep across all three sims, then
applied the established discipline — "untested ≠ working; probe with
assertions" — to the gaps that are both load-bearing (a backend or
terraform actually calls them) AND complex enough to harbor a real
bug. The broad coverage maps were noisy (most "untested" ops are
out-of-slice or already had dedicated fidelity tests the op-name grep
missed — Cloud Map, EFS, ECS are well-covered), so the value was in
narrowing to what sockerless depends on. Three real fidelity bugs,
each confirmed by a real-SDK probe mirroring the exact backend call
pattern, each fixed with a permanent regression test:

- BUG-1773: AWS CreateSecurityGroup never rejected a duplicate name in
  the same VPC — the ECS per-job-network create
  (backends/ecs/network_cloud.go) relies on InvalidGroup.Duplicate to
  reuse an existing SG by name+VPC; against the sim a retry silently
  minted a second SG. Now rejects same name+VPC (different VPCs still
  reuse a name).
- BUG-1774: AWS AuthorizeSecurityGroup{Ingress,Egress} never rejected a
  duplicate rule — the backend re-applies its self-referencing ingress
  rule and swallows exactly InvalidPermission.Duplicate; the sim
  appended a second identical permission, so DescribeSecurityGroups
  read-back accumulated duplicates. Now detects an equivalent existing
  permission (protocol + ports + shared target) and 400s it.
- BUG-1775: GCP rrsets.list ignored its name/type query filter — the
  Cloud Run service-discovery path uses .Name(fqdn).Type("A"); the sim
  returned the whole zone. The backend re-filters client-side so it
  wasn't broken, but the sim diverged from real Cloud DNS. Now honors
  the filter.

## 2026-06-12 - Runner-as-cloud-task topology, sim-proven (cells 1+2 minus the live pass)

The bleephub official-runner harness became the topology proof. Its
image now bundles simulator-aws + sockerless-backend-ecs + the
dispatcher next to bleephub and the runner; the make target mounts the
host docker.sock and a sim-EFS host dir at an identical path inside
and out, so the runner's workspace IS an EFS access point and
`container:` jobs dispatched through the backend land as sim-ECS tasks
on the host engine sharing it. TEST 12 asserts the contract from the
outside: a file written inside the job container shows up on the
runner's EFS workspace. TEST 13 runs a `services:` nginx reachable by
alias from the job container. TEST 14 closes the control plane: the
github-runner dispatcher (new `--api-base`; capability-probe token
verification for header-less tokens) polls bleephub for a queued job
no resident runner can take, spawns an ephemeral runner on the host
engine, and the job completes on it — 14/14 green locally
(BLEEPHUB_TEST_FROM + BLEEPHUB_HOLD knobs make single-test iteration
cheap). Every wall on the way was a real bug, filed + fixed
(BUG-1763..1771): the runner deserializes jobServiceContainers /
object-form `container:` as TemplateTokens (plain JSON maps fail job
start; `env` not `environment`); registration must round-trip
`ephemeral` or config.sh aborts, and completed ephemeral agents now
deregister; runs with no started job reported `in_progress`, hiding
them from `?status=queued` pollers; job messages baked the submitter's
request-Host as the server URL so off-host runners could run but never
complete jobs — `BLEEPHUB_EXTERNAL_URL` is the GHES-shaped fix; the
admin token advertised a scope header without `workflow`; and
dispatcher spawns add `host-gateway` only on engines that need and
accept it. Catalog: docs/RUNNERS.md D-8..D-12.

## 2026-06-12 - GitHub-runner dispatcher hardening (ARC-without-k8s parity)

Source audit of `github-runner-dispatcher-{aws,gcp,azure}` against their
contract (poll → mint token → spawn ephemeral runner → GC from cloud
state) filed and fixed BUG-1752..1762 in one PR. The P1: the Azure
sweep keyed "done" off ACA Job `ProvisioningState`, which reads
`Succeeded` the moment the resource is created — every runner Job
became reap-eligible on the next 2-min tick and `BeginDelete` kills the
in-flight execution. It now classifies the latest JobExecution's
status, the same fix the GCP spawner already carried for Cloud Run's
`TerminalCondition` (whose stale spec rows also got corrected). Both
cloud loops gained the GitHub-side offline-runner reap they claimed in
their flag help but never did, plus a 15-min orphan grace for Jobs
whose start call failed. The runner images' "60-s idle timeout" was
fiction — absent in vanilla/ecs/lambda, and a job-killing
whole-process `timeout` in cloudrun/gcf; all six entrypoints now share
a pre-pickup idle gate (watch /proc for `Runner.Worker`, exit 0 on
idle, never bound a running job). The dispatcher↔image env contract
unified on `RUNNER_REG_TOKEN`/`RUNNER_REPO` (the ECS/Lambda images
required `RUNNER_TOKEN`/`RUNNER_REPO_URL` and could not be spawned by
the dispatcher at all). New per-label knobs on all three:
`runner_job_timeout` (Cloud Run task timeout / ACA ReplicaTimeout /
docker-shape sweep enforcement) and `max_concurrent` (ARC's
`maxRunners` analog). Registration tokens left plain control-plane env
for Secret Manager (TTL-bounded, reaped with the Job) on GCP and Job
secrets + `secretRef` on ACA. Azure also got the GCP deployable
hardening (healthz/$PORT, $REPO, verify retry, rate-limit-aware loop),
2cpu/4Gi runner resources, and its first tests; `ListRunners` now
paginates. Catalog: docs/RUNNERS.md § dispatcher hurdles D-1..D-7.

## 2026-06-12 - Actions follow-ups + the bind-translation gate's mechanism

Same-day follow-up PR to #549 (BUG-1745..1750). **Cancellation is real
now**: cancelling a run sends `JobCancellation` over the runner's open
mid-job poll (the channel the pull-only broker kept exactly for this),
purges undelivered job messages, leaves `always()`/`cancelled()` jobs
runnable (they dispatch with cancelled()==true), and the run concludes
`cancelled` — proven live by the harness cancelling a `sleep 300` on the
official runner and watching the always() cleanup execute. That e2e also
caught the runner's US-spelled `Canceled` result leaking through
normalization unmapped. **Self-hosted actions work**: `uses:` resolves
from bleephub-hosted repos first (GitHub-layout tarballs built from git
storage), proven by a composite-action harness test — 11/11 official-
runner integration tests. Org **runner groups** (CRUD/membership/repo
visibility, undeletable Default) and **startup_failure run shells** for
matched-but-unstartable workflows round out the bleephub side.

On the backends, the **bind-mount→shared-volume translation** — the
documented gate for running GitHub runners as cloud tasks — reached
parity across all six container backends: lambda's config got actually
wired into its bind path, ACA/AZF gained the whole mechanism, and the
sharing contract (writer via named volume, reader via translated host
bind) is integration-proven on ECS and ACA. What remains for the
runner-on-cloud cells is topology (runner images mounting the shared
volume), not translation. Ledger: 1750 filed / 1708 fixed / 2 open.

## 2026-06-12 - Complete GitHub Actions support in bleephub

**The workflow engine now implements GitHub's server-side semantics.**
`on:` triggers parse fully (branch/tag/path filter patterns with ordered
`!` negation, real git diffs for path filters, activity types with
per-event defaults — pushes to an open PR's head branch fire
`pull_request synchronize`); `on: schedule` crons fire from a
minute-aligned dispatcher (POSIX 5-field parser, dom/dow OR rule);
reusable workflows expand server-side (synthetic gate/collector nodes,
typed+defaulted inputs, secrets inherit/mapping, outputs onto
`needs.<caller>`, 4-level nesting bound); and a real expression engine
(GitHub grammar, loose equality, full `github`/`needs`/`vars`/`inputs`/
`matrix` contexts, contains/startsWith/format/join/toJSON/fromJSON)
evaluates job `if:` and `${{ }}` templates — invalid expressions fail the
job like real GitHub.

**Secrets and variables exist at all three scopes** (repo/org/environment)
with the REAL wire contract — `gh secret set` fetches the public key,
seals with libsodium, and the server decrypts; plaintext PUTs are
rejected (the old shape no real client could ever have used). Org
visibility (all/private/selected), name rules, and org→repo→env
precedence merge into runner job messages with masks.

**Workflow runs are now first-class GitHub citizens**: every job mirrors
to a check run under a github-actions check suite; workflow_run /
workflow_job / check_run / check_suite webhook events fire at the real
emission points; PR `mergeable_state` reflects the head commit's checks
and the merge API 405s while required status checks (branch protection)
aren't green. The jobs API serves REAL per-step status/timing — the
runner's timeline records were being silently dropped because the
official runner wraps them in `VssJsonCollectionWrapper` (found against
actions/runner source; same wrapper bug fixed for console-line feeds).
Job logs persist (4MiB cap with explicit markers), run-log zips match
GitHub's layout, runs-on labels route jobs only to matching runners
(hosted aliases run anywhere), org-scoped runner endpoints exist with an
honest `busy`, reruns keep the run id and bump `run_attempt` (archived
attempts retrievable; rerun-failed-jobs carries successful results over),
and workflows enable/disable (disabled = no triggers, dispatch 403).

**The UI got the full GitHub-style Actions experience**: per-repo Actions
tab (runs list, filters, dispatch form built from parsed workflow inputs,
enable/disable), run detail (job sidebar, per-step status, live-tail
logs, rerun/cancel, artifacts, deployment approvals), secrets+variables
management with real in-browser sealed-box encryption, PR merge-box
checks section, runners page with labels/busy. Playwright 21/21, vitest
green, knip/jscpd clean.

Validation: gh Docker harness 115 PASS / 0 FAIL (now covering secrets/
variables/enable-disable/checks); the official-runner integration
harness was found bitrotted (launched binaries retired long ago —
BUG-1739), rewired to host-mode jobs (`jobContainer: null`, real
GitHub's no-container shape) and promoted to CI as
`sim (bleephub actions/runner)` — ALL 9 e2e tests green. Running the
REAL runner exposed two more latent protocol bugs, both fixed: the
broker pushed jobs at busy runners, which the runner silently drops
(BUG-1740 — delivery is now strictly pull-on-poll by free,
label-matching runners), and step `${{ }}` templates went out as
literal tokens the runner never evaluated (BUG-1741 — now
BasicExpression/format() tokens; secrets also ride message.Variables,
where the runner's ToSecretsContext actually reads them). Same PR also
closed consumer issues #548/#547: the azure-sim Entra token endpoint
accepts client_secret_basic and `/authorize` binds login_hint-resolved
users into auth codes (BUG-1742/1743). Ledger at 1743 filed / 1701
fixed / 2 open.

## Earlier milestones (compressed)

Full detail in the PR descriptions and `git log`; one line each:

- **Amplify full slice + bleephub Apps/orgs hardening** (#546) — complete Amplify control+data plane; GitHub Apps install/JWT/OAuth + org provisioning fidelity.
- **Launch hygiene** — spec-validation armed across all test surfaces incl. AWS XML protocols; azurestack retired; docs truth pass.
- **Simulator shape-drift burn-down** — all 28 allowlisted wire-shape bugs fixed (BUG-1658..1685); aws/azure allowlists emptied; knative moved to real `serving.knative.dev` paths; postgres-flexible speaks the real 202+Azure-AsyncOperation LRO.
- **Spec-based simulator validation** — vendored machine-readable cloud-API specs (`specs/cloud-api/`) + two CI gates: static surface conformance (every registered op exists in the spec) and runtime wire-shape validation (`SOCKERLESS_SPEC_VALIDATE`, ratcheted).
- **Simulator conformance + hardening (AWS/GCP/Azure)** (#537/#538/#539) — round-trip/error/pagination fidelity sweep + Go type hardening + sim-UI hardening + CI sim-module unit tests.
- **bleephub parity + durability + GitHub-style UI** (#534..#536) — SQLite/PostgreSQL persistence, filesystem + S3/MinIO git content storage, git HTTP auth, the UI restyle, and a GitHub-API fidelity sweep.
- **Cloud service-slice expansion** — AWS (Step Functions, Batch, CodeBuild, Glue), GCP (Spanner, Dataflow, Bigtable), Azure (Logic Apps, ACI), each with SDK + CLI + Terraform coverage.
- **Terraform idempotency drift sweep** (#491) — `terraform plan -detailed-exitcode` drift assertions on the gcp+azure apply stacks; ~18 read-back fidelity bugs fixed to make `tf (gcp)`/`tf (azure)` green.

## Foundations

Sockerless now includes:

- Docker API-compatible backends for local Docker passthrough and cloud-backed container/FaaS targets.
- High-fidelity AWS, GCP, and Azure local simulators, one binary per cloud, with official SDK/CLI/Terraform coverage tracked in [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).
- A real-execution substrate in `simulators/realexec`: network namespaces, bridges/veth/TAP NICs, IPAM, nftables, Firecracker VM lifecycle, health probes, and load-balancer proxying.
- Cross-cloud OCI `/v2/` registry data-plane implementations for ECR, Artifact Registry, and ACR.
- Bleephub, a GitHub Enterprise-style API simulator covering repos, issues, PRs, Actions, runners, apps, OAuth/OIDC, webhooks, packages, and admin org provisioning.
- Local HTTPS gateway infrastructure through Caddy for providers that require HTTPS endpoint discovery.

Older detailed entries were intentionally compressed out of this file. Use PR descriptions and `git log --oneline --decorate --all` for older implementation history.
