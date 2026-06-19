# Landscape: sockerless simulators vs the cloud-emulator ecosystem

Where the sockerless simulators sit relative to LocalStack, moto, the official
cloud emulators, and the post-2026 wave of open-source AWS/GCP/Azure mocks. Also
records the current spec-fidelity audit baseline.

Researched 2026-06-19 (LocalStack docs, getmoto, Google/Microsoft emulator docs,
Hacker News stories). Treat external coverage as a snapshot — these projects move.

## TL;DR — where sockerless is unique

Across LocalStack, moto, the official Google/Microsoft emulators, and every
project surfaced on Hacker News, **no widely-used tool occupies the niche
sockerless does**: a *single, multi-cloud (AWS + GCP + Azure), cloud-API-faithful
simulator that runs real container / serverless workloads*, fully open, with
strict per-response spec validation.

Three properties, none of which the alternatives combine:

1. **Multi-cloud in one surface.** AWS **and** GCP **and** Azure, as cloud
   *slices* (the subset each backend uses) at cloud-API fidelity. Everyone else
   is single-cloud: LocalStack is AWS-core (Azure closed-beta, no native GCP);
   moto is AWS-only; the official Google/Microsoft emulators are per-service and
   data-plane-only.
2. **Runs real workloads.** ECS tasks, Lambda invocations, Cloud Run
   services/jobs, ACA apps, Azure Functions — executed as real containers on the
   host engine, free. LocalStack runs real Lambda/ECS too, **but ECS/ECR are
   paid (Base+, ~$39/mo)** and the free Community edition that used to include
   them was discontinued 2026-03-23. moto and the official GCP/Azure emulators
   don't run real workloads at all (moto's Lambda-in-Docker can't even reach its
   own in-memory state).
3. **Strict live spec validation.** Every response is checked against the
   vendored cloud API spec at test time (`SOCKERLESS_SPEC_VALIDATE`), and a CI
   ratchet (`check-spec-violations.sh`) fails on *any* new divergence. LocalStack
   uses snapshot parity (capture-from-real-AWS), which doesn't catch
   un-snapshotted edges; moto and most others have no formal spec conformance.

The management/control plane is the universal blind spot of the ecosystem: GCP
and Azure officially emulate only stateful **data-plane** services (Pub/Sub,
Firestore, Bigtable, Spanner; Azurite, Cosmos data plane, Service Bus/Event Hubs)
and leave Cloud Run / Compute / Cloud Build / Artifact Registry / Functions /
IAM / Container Apps / App Service / ACR / Key Vault / APIM / Monitor with **no
official emulator**. Sockerless implements those control planes.

## Spec-fidelity audit baseline (2026-06-19)

Ran each cloud's full SDK test suite with `SOCKERLESS_SPEC_VALIDATE` on, checked
divergences against the vendored specs in `specs/cloud-api/<cloud>/`:

| Cloud | Distinct response-shape divergences | Notes |
|-------|-------------------------------------|-------|
| **AWS** | **0** | Full SDK suite, no allowlist file — i.e. zero accepted divergences (strict). |
| **GCP** | **2** (allowlisted) | Firestore `documents:batchGet` + `:runQuery` `type-mismatch` at `$` — documented in `simulators/gcp/spec-violation-allowlist.txt`. |
| **Azure** | **0** | No divergences recorded across the validated responses. |

The ratchet only ever shrinks: every allowlist entry carries a BUG ID, and new
violations fail CI (this is what caught a stray `queryId` field on
`GetQueryResults` in #612). The AWS surface having **no allowlist at all** is the
strongest fidelity signal in the ecosystem — stricter than LocalStack's
snapshot-parity approach.

(Two GCP/Azure local tests — `TestCloudBuild_FaithfulBuildPush`,
`TestACRTasks_ScheduleRunDockerBuild` — fail on a dev machine when the
podman-machine insecure-registry drop-in for `127.0.0.1:50xx` isn't loaded
("HTTP response to HTTPS client" on `docker push`); they pass on CI. Not a code
issue — see the registry-trust note in the runner docs.)

## Competitive landscape

| Project | Cloud(s) | Scope | Real workloads? | Multi-cloud? | Fidelity approach | License / status |
|---------|----------|-------|-----------------|--------------|-------------------|------------------|
| **sockerless sims** | AWS + GCP + Azure | The slice each backend uses (~50 AWS / ~30 GCP / ~35 Azure services) | **Yes** — real containers (ECS, Lambda, Cloud Run, ACA, AZF) | **Yes** | Live per-response spec ratchet vs vendored specs; faithful error shapes/protocols | Open, in-repo |
| **LocalStack** | AWS (core); Azure closed beta; Snowflake GA; **no native GCP** | 120+ AWS services | Yes (Lambda/ECS/ECR/RDS in Docker) — but **ECS/ECR are paid** | No (AWS-centric) | botocore/Smithy-generated stubs; snapshot parity; full protocol set incl. cbor | Commercial; **free Community discontinued 2026-03-23**; token-gated tiers (Hobby free → Base/Ultimate/Enterprise) |
| **moto** | AWS only | 100+ AWS services | No (in-memory mocks; Lambda-in-Docker is leaky — can't reach moto state) | No | Monkeypatches boto3; CRUD mock state | OSS (Apache-2.0), unit-test library |
| **Official GCP emulators** | GCP | **Data-plane only**: Pub/Sub, Firestore, Datastore, Bigtable, Spanner | No | No | Official, per-service binaries | Google; **no** Cloud Run/Compute/Build/AR/Functions/IAM/Logging emulator |
| **Official Azure emulators** | Azure | **Data-plane only**: Azurite (Storage), Cosmos data plane, Service Bus, Event Hubs, Functions Core Tools | No (Functions Core Tools runs a function, not the mgmt API) | No | Official, per-service binaries | Microsoft; **no** Container Apps/App Service/ACR/Key Vault/APIM/Monitor/VNet emulator |
| **Fakecloud** | AWS | ~13 services, depth-over-breadth, **Smithy-model validated** | No | No | Smithy-model-driven (closest in spirit to sockerless's fidelity) | OSS, 2026 (post-paywall wave) |
| **floci** | AWS | LocalStack alt, "real containers, not mocks" | Leaning yes | No | Emulation + real containers | OSS, 2026, immature |
| **MiniStack** | AWS | Core free-tier services (S3/SQS/DynamoDB/KMS), breadth-over-depth | No | No | Mock (maintainer concedes no exception/validation fidelity) | OSS (MIT), 2026 |
| **Hiraeth** | AWS | SQS-first, growing | No | No | Best-effort emulation | OSS, 2026 |
| **LocalEmu** | AWS | Fork of the archived LocalStack repo | Yes (inherited) | No | Inherited from LocalStack | OSS, 2026 |
| **localaz / miniblue / topaz** | Azure | Early Azure emulators (topaz focuses on AMQP/Service Bus) | No | No | Emulation | OSS, 2026, nascent |
| **localgcp / MiniSky** | GCP | "LocalStack for GCP", ~14–16 services incl. Cloud Run-class | No (emulation-grade) | No | Emulation, unproven at scale | OSS, 2026, unaffiliated with LocalStack |
| **gcw-emulator** | GCP | Cloud Workflows only (a gap the official emulators miss) | No | No | Emulation | OSS |
| **fake-gcs-server** | GCP | GCS only | No | No | High GCS fidelity | OSS, popular |
| **Azurite** | Azure | Storage (Blob/Queue/Table) only | No | No | Official, high fidelity | Microsoft |
| **SAM CLI / Cloud Code / Functions Framework** | AWS / GCP | Run one function/container locally | Real container exec | No | **No management API** around the workload | Vendor tooling |
| **gripmock / smocker** | n/a | Generic gRPC / HTTP mocks | No | n/a | Not cloud-faithful | OSS |

A late-2025/2026 wave of open AWS mocks (MiniStack, Hiraeth, Fakecloud,
CloudMock, floci, LocalEmu) and nascent single-cloud Azure/GCP emulators (localaz,
miniblue, topaz, gcw-emulator) appeared largely in reaction to LocalStack
paywalling/archiving its Community tier. **None are multi-cloud**, and almost all
are in-memory mocks; the two closest in *approach* — Fakecloud (Smithy-validated)
and floci (real containers) — are each AWS-only and young.

## Per-cloud coverage: what sockerless implements

### AWS (~50 service slices, ~700 operations)

ACM · Amplify (build/compute/data-plane/domains) · API Gateway v1 + v2 ·
Application Auto Scaling · Auto Scaling · Batch · CloudFront (functions/keys/
policies) · Cloud Map / Service Discovery · CloudTrail · **CloudWatch** (metrics
[query+awsJson+cbor], Logs, EMF, **alarms**, **dashboards**, **filter-pattern**,
**Insights**) · CodeBuild · DynamoDB (incl. full expression grammar) · EC2 (incl.
launch templates, real-exec) · ECR (OCI/layers) · ECS (+ services, exec) · EFS ·
ElastiCache · ELBv2 (+ rules, real-exec) · EventBridge (+ Scheduler) · Glue ·
IAM (+ policies, SLR/OIDC) · Kinesis · KMS (+ grants) · Lambda (+ runtime API,
subresources) · RDS · Route53 · S3 (+ subresources) · Secrets Manager · SNS ·
SQS · Step Functions · SSM · STS · WAFv2.

vs LocalStack tiers: **ECS, ECR, ELBv2, Auto Scaling, App Auto Scaling,
CloudFront, ElastiCache, RDS, CodeBuild, CloudTrail** are LocalStack **Base+
(paid)**; **CloudMap, EFS, Glue, Batch, WAFv2, Amplify** are LocalStack
**Ultimate-only**. Sockerless implements all of these, free.

### GCP (~30 service slices) — *no official emulator exists for most of these*

API Gateway · Artifact Registry · BigQuery · Bigtable (+ gRPC) · Cloud Build ·
Cloud Functions · Cloud KMS · **Cloud Run** (+ Jobs, Services) · Compute (+ load
balancing, real-exec) · Dataflow · Cloud DNS · Eventarc · Firestore · GCS · IAM ·
Cloud Logging (+ filter language) · Memorystore/Redis · Operations · Pub/Sub ·
Secret Manager · Service Usage · Spanner · Cloud SQL Admin · VPC Access.

Google officially emulates only Pub/Sub, Firestore, Datastore, Bigtable, Spanner.
**Cloud Run, Compute, Cloud Build, Artifact Registry, Cloud Functions, IAM,
Cloud Logging — no official emulator.** Sockerless implements all of them.

### Azure (~35 service slices) — *no official emulator exists for the management plane*

ACR (+ Tasks) · APIM · App Service / plans · Authorization · Azure DNS (public +
private) · Blob / Files / Storage data plane · Redis Cache · Compute · **Container
Apps** (+ env, ingress) · Container Instances · Cosmos (management + data) · Entra
(Azure AD) · Event Grid · Event Hubs · **Azure Functions** · Key Vault · KQL (Log
Analytics) · Logic Apps · Managed Identity · Monitor / Insights · Network (+
real-exec) · PostgreSQL Flexible · Service Bus (admin/AMQP/data) · App Service
VNet integration.

Microsoft officially emulates only Storage (Azurite), Cosmos data plane, Service
Bus, Event Hubs. **Container Apps, App Service, ACR, Key Vault, APIM, Cosmos
management, Monitor/Log Analytics, VNet — no official emulator.** Sockerless
implements them.

## Query / expression language coverage

A differentiator most mocks skip — sockerless ships real parsers/evaluators for
every query surface its sims expose (not substring matching):

| Surface | Sockerless | Typical mock |
|---------|-----------|--------------|
| GCP list `filter` (AIP-160) | Full recursive-descent grammar | Ignored or partial |
| DynamoDB Condition/Filter/Key expressions | Full grammar (functions, BETWEEN/IN, AND/OR/NOT, nested paths) | Often `=`/AND-only |
| CloudWatch Logs filter pattern | Unstructured + structured-JSON grammar | Substring |
| CloudWatch Logs Insights (`StartQuery`) | Pipeline engine (fields/filter/stats/sort/limit) | Usually absent |
| Azure OData `$filter` | Full grammar (eq/ne/gt…, and/or/not, functions) | Ignored |
| KQL (Log Analytics) | Implemented | Absent |

## What sockerless does *not* do (honest gaps)

- **Breadth.** LocalStack lists 120+ AWS services and moto 100+; sockerless
  implements ~50 AWS service *slices* — the subset its backends actually use, at
  high fidelity, rather than a wide shallow surface. It is not a general-purpose
  "mock any AWS API" tool.
- **No managed product / UI tooling** comparable to LocalStack's Pro web app,
  resource browser, Cloud Pods, IAM policy enforcement, or chaos engineering.
- **Distributed-system semantics** (eventual consistency, async timing,
  cross-region) are modeled where a backend needs them, not exhaustively.
- **Some long-tail behaviors** are tracked as fidelity bugs rather than fully
  implemented (see `BUGS.md`); the spec ratchet keeps these honest.

## Sources

LocalStack: docs.localstack.cloud (per-service plan badges), pricing page,
2026 packaging-change blog posts, ASF/parity blog. moto: getmoto docs +
`IMPLEMENTATION_COVERAGE.md`. GCP/Azure emulators: cloud.google.com/cli,
learn.microsoft.com (Cosmos vNext GA 2026-06-10, Azurite, Service Bus/Event Hubs
emulators), Testcontainers module docs. HN: Algolia search (MiniStack #47593285,
Fakecloud #47782696, floci #47944829, LocalEmu #48340115, localaz #48509636,
LocalStack-archive #47493657, and the LocalStack-pricing sentiment threads).
