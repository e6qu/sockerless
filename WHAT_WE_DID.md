# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform. Designed to power CI runners on cloud serverless capacity — see [docs/RUNNERS.md](docs/RUNNERS.md).

State [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · architecture [specs/](specs/).

This file keeps narrative — *why* each phase, what was surprising, what blocked. Per-bug detail in [BUGS.md](BUGS.md); code-level detail in `git log`.

## 2026-05-29 — Azure Service Bus ARM Terraform parity

Issue #276 was valid. The Azure simulator had Service Bus namespace, queue, topic, subscription, and authorization-rule ARM routes, but azurerm reads the namespace child resource `networkRuleSets/default` during `azurerm_servicebus_namespace` refresh. That route was missing, so Terraform aborted before it could prove the namespace+queue lifecycle from the reporter's configuration.

The fix implemented the public Microsoft.ServiceBus ARM child resources the provider and SDK can read in this flow: namespace network rule set get/list/update with persisted properties, empty disaster recovery and migration configuration list reads for namespaces without those features configured, and Azure-shaped 404s for absent disaster recovery aliases or migration configurations such as `$default`. Namespace deletion now also cascades authorization rules and network rule sets.

Coverage uses the official `armservicebus` SDK for namespace, queue, network-rule-set, disaster-recovery, and migration reads; Azure CLI `az rest` for the same ARM paths; and the Azure Terraform harness now provisions `azurerm_servicebus_namespace` plus `azurerm_servicebus_queue` through the simulator. The new `azure-servicebus-arm` surface table tracks this ARM protocol separately from Service Bus admin and message data-plane protocols.

## 2026-05-29 — VM/instance compute API parity

Issue #266 was valid. The foundational audit found that the simulators had network/storage prerequisites for VM-like workloads but not the VM control planes themselves: AWS EC2 lacked instance lifecycle APIs, GCP Compute lacked `instances`, and Azure lacked `Microsoft.Compute/virtualMachines` plus the NIC/public-IP resources VMs wire through.

The fix added public-cloud-compatible VM slices without leaking local execution machinery into public APIs. AWS EC2 now supports `RunInstances`, `DescribeInstances`, `StopInstances`, `StartInstances`, `TerminateInstances`, `DescribeInstanceStatus`, ENI attachment state, subnet-backed private IP allocation, image/key-pair discovery, and tag mutation. GCP Compute now supports instance create/get/list/aggregated-list/start/stop/delete, labels, tags, metadata, attached disks, NICs, machine types, disk types, images, and zonal operations. Azure now supports `Microsoft.Network/networkInterfaces`, `Microsoft.Network/publicIPAddresses`, and `Microsoft.Compute/virtualMachines` with ARM create/get/list/delete, `instanceView`, and power-state operations.

Coverage uses all three required external surfaces: official SDK tests, vendor CLI flows (`aws ec2`, `gcloud compute instances`, and `az rest`), and Terraform resources (`aws_instance`, `google_compute_instance`, `azurerm_network_interface`, `azurerm_linux_virtual_machine`). Direct smoke checks against local simulator processes verified the new AWS/GCP/Azure wire paths; full Docker-backed SDK/CLI/Terraform harness execution remains the CI path.

## 2026-05-29 — Azure Entra per-resource token audiences

Issue #272 was valid. The Azure simulator had already moved its OAuth token endpoint to real-shape RS256 JWTs with a published JWKS, but the payload still hardcoded the ARM audience for every token. That was enough for ARM-only clients and not enough for data-plane SDK paths: Key Vault requests `https://vault.azure.net/.default`, Service Bus requests `https://servicebus.azure.net`, and Storage AAD paths request `https://storage.azure.com/.default`.

The token endpoint now derives JWT `aud` from the same public request fields real Azure clients use. OAuth v2 `scope={resource}/.default` mints a token for that resource, bare resource scopes are accepted, OAuth v1 `resource={resource}` maps directly to `aud`, and omitted audience fields keep the ARM default. ARM and Storage retain Azure's trailing-slash audience shape.

Coverage includes unit tests for the audience derivation table and JWT claim shape, plus simulator SDK-harness HTTP tests that request ARM, Key Vault, Storage, and Service Bus token audiences and decode the returned RS256 JWTs.

## 2026-05-29 — Simulator test-contract matrix backfill

Issue #264 was valid. The project already ran SDK, CLI, and Terraform simulator suites, but the coverage contract was not indexed against the canonical surface tables, so a surface could be added or renamed without a matching client-surface status row. The fix added `specs/SIM_TEST_COVERAGE_MATRIX.md` and wired `scripts/check-simulator-coverage-matrix.sh` into pre-commit and CI. The check fails if the matrix and `specs/SIM_SURFACE_TABLES/*.md` drift, if rows are duplicated, or if tracked rows lack a GitHub issue or BUG reference.

The concrete coverage misses in the report were real and were closed with real external clients. AWS DynamoDB now has a direct `aws dynamodb` CLI create/describe/put/get/query/delete lifecycle test. AWS SQS has a direct queue lifecycle CLI test, and AWS SNS has a direct topic-to-SQS fanout CLI test that verifies the delivered notification envelope.

The GCP Terraform harness now provisions and destroys `google_cloudfunctions2_function`, `google_pubsub_topic`, `google_pubsub_subscription`, `google_cloudbuild_trigger`, `google_logging_project_sink`, and `google_logging_metric`. Making those provider flows real required simulator implementation, not only docs: Cloud Logging now has sink and metric create/list/get/update/delete REST routes, Cloud Build has trigger lifecycle routes, and regional trigger routing distinguishes Cloud Build from Eventarc by the service hostname while preserving the public GCP path shape.

Verification covered fast compile checks, `terraform fmt -check`, the targeted AWS CLI tests for DynamoDB/SQS/SNS, and the Docker-backed GCP Terraform apply/destroy harness.

## 2026-05-29 — Azure azurerm storage endpoint coverage

Issue #269 was valid as a contract requirement: azurerm's storage-container data-plane resource parses the ARM-returned `primary_blob_endpoint` as an Azure-shaped `{account}.blob.{suffix}` URL and compares that suffix with `/metadata/endpoints`. A literal local URL is not compatible with that public provider path.

The implementation from the prior Azure endpoint-composition work already supported the necessary behavior: `SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON` accepts storage templates with `{account}`, and `/metadata/endpoints` derives the storage suffix from the emitted storage endpoint shape. What was missing was the exact regression that proves the realistic azurerm path. The Azure Terraform harness now creates `azurerm_storage_container` with `storage_account_name`, intentionally avoiding `storage_account_id` because that ARM-only form bypasses the Blob data plane. The test asserts that the provider receives a Blob URL-shaped container ID and the canonical ARM `resource_manager_id`.

The Dockerized Azure Terraform apply/destroy harness passed with the new resource, so the simulator's ARM endpoint emission, metadata suffix response, azurerm parser path, and Blob data-plane routing are covered together.

## 2026-05-29 — Managed data SaaS parity

The managed data SaaS phase closed BUG-1201, BUG-1202, and issue #267.

The gap was real: AWS already had DynamoDB, but the GCP simulator lacked BigQuery and Firestore, and the Azure simulator lacked Cosmos DB. The fix added BigQuery's public REST v2 slice for datasets, tables, jobs, synchronous queries, tabledata listing, and streaming inserts. Rows are persisted in the simulator store, table metadata reflects row counts, and the query path evaluates the foundational `SELECT ... FROM ... WHERE field = value` shape used by local clients and tests instead of returning a canned result.

Firestore now has document CRUD plus commit, batchGet, batchWrite, and structured equality runQuery flows over Firestore typed-value JSON documents. Cosmos DB now has both public surfaces: the `Microsoft.DocumentDB/databaseAccounts` ARM control plane for accounts, SQL databases, SQL containers, throughput settings, `listKeys`, and `listConnectionStrings`, plus the SQL data-plane database/container/document CRUD and query paths under `/dbs/...`.

The AWS DynamoDB re-audit found one real parity bug: `Query` and filtered `Scan` returned every table item even when callers supplied equality expressions. DynamoDB now evaluates the common public `KeyConditionExpression` / `FilterExpression` equality form with `ExpressionAttributeNames` and `ExpressionAttributeValues`, and the SDK regression asserts the filtered result.

Coverage was added across the simulator contract surfaces: official Google SDK coverage for BigQuery and Firestore create/read/write flows, Azure simulator SDK-harness coverage for Cosmos ARM and data-plane flows, vendor CLI-harness coverage for GCP and Azure data SaaS routes, Terraform provider resources for `google_bigquery_dataset`, `google_bigquery_table`, `google_firestore_document`, `azurerm_cosmosdb_account`, `azurerm_cosmosdb_sql_database`, and `azurerm_cosmosdb_sql_container`, plus the DynamoDB SDK query/filter regression.

## 2026-05-29 — Azure public DNS parity

The Azure public DNS parity phase closed BUG-1205 / issue #265.

The gap was real: Azure Private DNS covered internal service discovery, but public Azure DNS zones use a separate public ARM surface: `Microsoft.Network/dnsZones` with `api-version=2018-05-01`. The simulator now implements that public slice directly rather than aliasing it to Private DNS. Public zones return Azure-shaped `zoneType: Public`, generated name servers, record counts, and default NS/SOA record sets. Record sets support A, AAAA, CAA, CNAME, MX, NS, PTR, SOA, SRV, and TXT create/get/list/delete routes with the public DNS JSON field casing used by the official SDK.

Coverage uses all three external client surfaces: the official Azure DNS Go SDK (`armdns`) creates zones and A records and exercises the DNS-zone pager; Azure CLI coverage uses `az rest` against the public DNS routes; Terraform coverage adds `azurerm_dns_zone` and `azurerm_dns_a_record` to the Azure simulator apply/destroy harness. Provider metadata and resource-group enumeration now include public DNS resources alongside Private DNS.

## 2026-05-28 — Stream/event ingestion parity

The stream/event ingestion phase closed BUG-1200 by adding the missing AWS and Azure foundational stream services.

AWS now implements Kinesis through the public JSON protocol: stream create/delete/describe/list, shard listing, `PutRecord`, `PutRecords`, shard iterators, `GetRecords`, stream tags, retention-period changes, enhanced monitoring, encryption state, shard-count update, and limits. Records are stored per shard, sequence numbers advance monotonically, partition keys hash to shards, and reads follow Kinesis shard-iterator semantics rather than returning a canned response.

Azure now implements Event Hubs through both public surfaces needed by real clients. The ARM slice covers namespace, event hub, consumer group, and authorization-rule lifecycle, including `listKeys`, `regenerateKeys`, namespace defaults, and `networkRuleSets/default` reads used by azurerm. The data plane extends the existing raw AMQP/TLS listener so the official Event Hubs SDK can send batches and receive events through Event Hubs link addresses and management reads for event hub and partition metadata.

Verification uses the official AWS SDK, AWS CLI, and Terraform Kinesis resource; the official Azure Event Hubs management/data-plane SDKs, Azure CLI/`az rest`, and azurerm Event Hubs resources. While running the full AWS suites, the Lambda Runtime API tests exposed a real local callback bug under Podman: the simulator injected the Podman network gateway as the host callback address, but the per-invocation Runtime API listener runs in the host process. AWS workload callbacks now consistently use `host.docker.internal`, which Podman resolves to the host callback address and Linux Docker maps through `host-gateway`.

## 2026-05-28 — Azure data-plane DNS portability

The Azure data-plane DNS portability phase closed BUG-1211 / issue #247.

The issue was real: Azure Storage, Service Bus, Event Grid, and Key Vault are host-addressed services, and the simulator correctly preserved that shape by returning local hosts such as `{account}.blob.localhost`, `{namespace}.servicebus.localhost`, and `{topic}.eventgrid.localhost`. That still left real macOS-hosted clients unable to resolve wildcard `.localhost` names before their requests reached the simulator.

The fix keeps the Azure public API shape intact and moves the local concern into local infrastructure. The Azure simulator now has an opt-in DNS listener controlled by `SIM_AZURE_DNS_LISTEN_ADDR`. It serves real DNS over UDP and TCP, answers configured local zones such as `localhost` or `sockerless.azure.local` to the simulator target IP, and fails loudly on invalid DNS configuration or bind errors. Clients can point their OS resolver at the simulator DNS listener and continue using the host-addressed endpoint URLs returned by ARM/metadata without URL rewriting or private Host-header injection.

## 2026-05-28 — Advanced event-service parity

The advanced event-service parity phase closed BUG-1213 / issue #249, BUG-1214 / issue #250, and BUG-1215 / issue #251.

AWS EventBridge now implements the remaining public event-service control-plane surfaces needed after the foundational rule/target pass: event-bus lifecycle, bus policy permissions, archive lifecycle, and replay lifecycle/delivery. Replays read the simulator's archived events and deliver them back through the same EventBridge target path, so SQS/SNS targets observe replayed events through the normal cloud-facing APIs. Coverage uses the official AWS SDK, `aws events` CLI commands, and Terraform `aws_cloudwatch_event_bus`, `aws_cloudwatch_event_permission`, and `aws_cloudwatch_event_archive`.

Azure Event Grid now implements domains, domain topics, system topics, partner topics, and event subscriptions scoped to topics, domain topics, system topics, and partner topics. Partner topics were not documented away: the current Azure Go SDK module in this repo no longer exposes partner-topic clients and the current azurerm provider exposes partner namespace/configuration resources but not a partner-topic resource, while Azure CLI does expose partner-topic commands. The simulator therefore implements the public ARM routes and covers them through Azure CLI / `az rest`; SDK and Terraform coverage remain on the current clients/resources the installed providers actually expose.

GCP Eventarc now implements channels, provider discovery/listing, and channel connections through the public Eventarc v1 REST surface. Coverage uses the official `cloud.google.com/go/eventarc/apiv1` client and `gcloud eventarc` for channels, providers, and channel connections. Terraform coverage uses `google_eventarc_channel`; provider-schema inspection against the installed `hashicorp/google` and `hashicorp/google-beta` providers confirmed they do not expose a `google_eventarc_channel_connection` resource.

## 2026-05-28 — Simulator Docker test harness

The simulator Docker test harness phase closed BUG-1216 / issue #253 and BUG-1212 / issue #248.

The broken `docker-test` targets were real harness defects: the AWS, GCP, and Azure simulator Makefiles pointed at nonexistent per-directory `Dockerfile.test` files and mounted only the simulator subdirectory, which excluded the repository-level test fixtures and shared Makefile infrastructure. The fix added one shared `simulators/Dockerfile.test` Linux image with Go, Terraform, AWS CLI, gcloud, Azure CLI, Docker CLI, Make, Git, and supporting tools. The top-level `make docker-test` target and each per-cloud `make docker-test` target now build that image from the repository root, mount the full checkout and host Docker socket, and run the existing SDK/CLI/Terraform categories rather than a parallel test path.

The Azure Terraform macOS problem was also a real local execution bug, not a simulator API limitation. The Terraform providers validate the simulator's self-signed HTTPS CA through Go's OS trust store; Linux honors `SSL_CERT_FILE`, while Darwin's cgo-backed `SystemCertPool` does not. The Azure Terraform test harness now detects direct macOS execution and delegates the same `go test` command into the shared Linux Docker image with `SOCKERLESS_AZURE_TF_IN_DOCKER=1`, so the real `azurestack` and `azurerm` providers run unchanged against the simulator and validate the generated CA through Linux `SSL_CERT_FILE`.

## 2026-05-27 — Foundational event routing

PR #252 was the first implementation pass after the foundational audit and closed BUG-1197..1199 by adding real public event-routing slices for the foundational flows in all three simulators.

AWS now implements the EventBridge `events` JSON protocol for rule lifecycle, tags, targets, and `PutEvents`. The simulator records events, matches basic `source` / `detail-type` event patterns, and delivers matching events to real simulator SQS queues or SNS fanout targets. Coverage uses the official EventBridge SDK, `aws events` CLI commands, and Terraform `aws_cloudwatch_event_rule` / `aws_cloudwatch_event_target` resources.

GCP now implements Eventarc v1 trigger create/get/list/patch/delete routes with regional long-running operations. The wire shape is driven by the public REST/SDK contract, including `eventFilters`, Cloud Run destinations, labels, and provider-compatible trigger reads. Coverage uses the official `cloud.google.com/go/eventarc/apiv1` client, `gcloud eventarc triggers`, and Terraform `google_eventarc_trigger`.

Azure now implements Microsoft.EventGrid topics and event subscriptions through ARM plus a real custom-topic publish endpoint. Webhook event subscriptions receive the subscription-validation event and later published Event Grid events through the endpoint returned by the topic resource. The local simulator allocates a real loopback publish listener for locally addressed topics so Azure CLI/SDK callers can use the returned endpoint directly instead of relying on wildcard `.localhost` DNS. Coverage uses the official Azure Event Grid management SDK, `az rest`, and Terraform `azurerm_eventgrid_topic`.

CI review found two real follow-up defects before merge. The AzureRM Terraform provider calls the Event Grid topic `listKeys` ARM action during `azurerm_eventgrid_topic` creation, so the simulator now implements `POST .../topics/{topic}/listKeys` and returns the real `{key1,key2}` response shape. The Azure ACA/AZF backend modules also now carry the same refreshed OpenTelemetry transitive graph as `backends/core`, so isolated `-tags noui` builds do not fail with missing `go.sum` entries.

Verification found follow-up defects outside the landed foundational flows. Issue #247 / BUG-1211 later closed in the Azure data-plane DNS portability phase. Issues #248 / BUG-1212 and #253 / BUG-1216 were fixed in the simulator Docker test harness phase. Issues #249..#251 / BUG-1213..1215 were fixed in the advanced event-service parity phase.

## 2026-05-27 — Foundational simulator service audit

Audited AWS, GCP, and Azure simulator coverage for foundational service classes: object storage, managed data stores, DNS, queue/message systems, event routing, stream/event ingestion, VM/EC2-like compute, VPC/networking, NAT/egress, gateways, and managed load balancers. The audit result is now recorded in `specs/SIM_FOUNDATIONAL_AUDIT.md`.

The audit found object storage and core queue/message systems already present across the three sims, but filed BUG-1197..1207 for missing or incomplete foundational slices: EventBridge/Eventarc/Event Grid, Kinesis/Event Hubs, BigQuery, Firestore/Datastore, Cosmos DB, EC2/GCE/Azure VM lifecycle APIs, managed load balancers, Azure public DNS, uneven NAT/public-IP parity, and stale surface-table status rows. VM support should expose cloud-compatible public APIs while keeping Firecracker or any other local microVM runtime behind the simulator boundary.

PR #246 also closed the CI/pre-push regressions found while landing the audit. The bleephub invalid-JWT regression now tampers decoded signature bytes before re-encoding, so the verifier test cannot accidentally preserve a valid signature through trailing base64url character changes. The GCF FaaS smoke harness now reuses local image tags across its subprocess TestMain run instead of rebuilding Alpine tags from Public ECR and risking a registry 429. The GCP simulator/backend modules now pin the latest `google.golang.org/api` required by the dependency-freshness hook.

## 2026-05-27 — Issues #243/#244: Azure endpoint hosts + ACA image platforms

PR #245 closed issues #243/#244 and two Azure simulator fidelity bugs. Issue #243 was a real endpoint-host drift: Storage and Key Vault already returned simulator-routable Azure-shaped endpoint fields derived from the ARM request host, but Service Bus, Redis, APIM, PostgreSQL Flexible Server, and Container Apps still emitted production Azure suffixes. That forced callers following ARM-returned data-plane fields away from the simulator.

The fix added a shared Azure request-host helper and applied it to the audited endpoint fields: Service Bus `serviceBusEndpoint`, Redis `hostName`, APIM gateway/portal/management URLs, PostgreSQL Flexible Server `fullyQualifiedDomainName`, Container Apps managed-environment `defaultDomain`, and Container Apps app/revision FQDNs. Service Bus listKeys connection strings now use the same derived namespace endpoint, and the storage path-style dispatcher recognizes the newly advertised non-storage Azure subdomains so collapsed-port routing does not misclassify those hosts as storage requests.

Issue #244 was the Container Apps execution bug behind amd64 CI failures. Jobs and Apps hardcoded `Architecture: "linux/arm64"` for main containers and sidecars. The fix resolves each local image and inspects its manifest platform before starting the real container, matching the Cloud Run Services approach and allowing amd64 images on amd64 hosts.

Coverage includes the Azure simulator package test with a source guard against reintroducing the ACA hardcoded platform, the Azure SDK test suite with endpoint-host assertions for Service Bus/listKeys, APIM, Redis, PostgreSQL, and Container Apps app FQDNs, plus the Azure CLI and shared simulator test suites. The Terraform simulator test starts but remains a Darwin harness limitation locally because Terraform cannot validate the simulator's self-signed certificate through Go's cgo SystemCertPool on macOS; it is expected to run through Docker/CI.

## 2026-05-27 — Issues #239/#240/#241: GCS metadata validation follow-ups

PR #242 closed the three follow-ups from the PR #238 review. Issue #239 was the public-fidelity gap: the simulator accepted every persisted GCS object metadata value verbatim, even where the published Cloud Storage contract is explicit. Issue #240 was the small implementation cleanup around redundant custom-metadata cloning. Issue #241 was the maintainability guard for the disk/store split behind GCS objects.

The fix validates the accepted fields with documented constraints: `customTime` must parse as RFC 3339, and `contentLanguage` must be at most 100 characters. Invalid metadata now returns a GCS-shaped `400 INVALID_ARGUMENT` from the shared persistence path, covering multipart upload, resumable upload finalization, compose, `copyTo`, and `rewriteTo`; resumable upload initiation also preflights the same validation before creating an upload session.

The custom metadata clone now happens exactly once in the normal request-resource flow: `applyTo` clones request metadata and marks it cloned, while `persistGCSObject` remains the store-boundary defensive copy for uncloned maps. A source-level AST guard test fails if future GCS object writes call `objects.Put` or `gcsObjects.Put` outside `persistGCSObject`, preserving the disk-backed byte source of truth.

Coverage includes the official Go storage SDK rejection path for `ObjectHandle.CopierFrom(...).Run(ctx)` with invalid destination metadata, raw JSON API validation coverage for multipart upload, resumable init, compose, `copyTo`, and `rewriteTo`, and the simulator package guard test.

## 2026-05-27 — Issues #236/#237: GCS copy metadata + persistence helper

PR #238 closed the two follow-ups from the storage-copy review. Issue #236 was a real public-surface gap: GCS `rewriteTo` and `copyTo` accept a destination object resource, not just `contentType`, so SDK or raw JSON callers setting destination metadata needed those fields persisted and returned like real Cloud Storage. Issue #237 was the corresponding implementation risk: upload, resumable upload, compose, and copy/rewrite had duplicated object persistence logic.

The fix extends the simulator object model for custom metadata plus the public HTTP metadata fields exercised by copy/rewrite callers: cache control, content disposition, content encoding, content language, storage class, and custom time. Copy/rewrite now starts from the stored source object metadata, applies destination-supplied fields as overrides, and leaves absent fields inherited from the source. JSON metadata responses and download headers return the persisted values.

The write paths now share one `persistGCSObject` helper that writes the real object bytes to the bucket backing directory, computes size/hash/etag, timestamps the object, clones metadata, and updates the simulator store. Upload, resumable upload finalization, compose, and copy/rewrite use that helper.

Coverage uses the official Go storage SDK for `ObjectHandle.CopierFrom(...).Run(ctx)` with destination metadata overrides and source-field inheritance. A raw JSON API `copyTo` regression covers fields not covered by the SDK assertion and verifies metadata survives a follow-up metadata GET and appears in download headers.

## 2026-05-27 — Issues #232/#233/#234: Storage object copy + GCS list ordering

PR #235 closed the storage-copy reports. They were real simulator-fidelity bugs, not caller limitations. Azure Blob had Put/Get/block coverage, but a blob-level `PUT` with `x-ms-copy-source` still fell through to `handlePutBlob`, so SDK callers using `StartCopyFromURL` could receive a successful-looking response while the destination did not contain a real copy. GCS had compose/upload/download/list coverage but not the JSON API object-copy endpoints used by `ObjectHandle.CopierFrom(...).Run(ctx)`, and its object list response inherited Go map iteration order even though real GCS documents lexicographic ordering by object name.

The fix added Azure Copy Blob as the public data-plane operation: source URLs are resolved from host-style and Azurite-style path-style addresses, URL-escaped blob names are decoded, missing sources return `CannotVerifyCopySource`, bytes are copied from the real stored source object, destination metadata wins when supplied and otherwise source metadata is preserved, and the response/destination properties carry Azure copy ID/status/source headers.

GCS gained `rewriteTo` and `copyTo` in the existing object POST surface. Both endpoints share a real byte-copy implementation backed by the simulator's on-disk object store; `rewriteTo` returns a `storage#rewriteResponse` with `done: true` and string byte counts as expected by the official SDK, while `copyTo` returns the destination `storage#object`. `objects.list` now sorts `items[]` by name and `prefixes[]` lexicographically after prefix/delimiter filtering.

Coverage uses the official SDK paths where they exist: Azure `azblob/blob.Client.StartCopyFromURL`, GCS `ObjectHandle.CopierFrom(...).Run(ctx)`, and GCS SDK object iteration. A raw JSON API regression covers `copyTo` plus delimiter-produced prefix ordering.

## 2026-05-26 — Issue #230: Service Bus raw AMQP/TLS transport

PR #231 closed issue #230. The report was real. PR #229 added AMQP-over-WebSocket support, but making `NewWebSocketConn` the only official-SDK path leaked simulator transport plumbing into callers. Real Azure Service Bus exposes raw AMQP/TLS as the SDK's default transport, with WebSocket as an opt-in alternate transport, so the simulator needed the same public boundary.

The fix adds a configurable raw AMQP/TLS listener to the Azure simulator. The listener requires TLS cert/key material when enabled and reuses the same AMQP parser/session implementation as the WebSocket path. Namespace routing prefers the AMQP Open `hostname`, with TLS SNI as early metadata/fallback, because `azservicebus.ClientOptions.CustomEndpoint` redirects the TCP dial target while preserving the original Service Bus namespace/audience. The existing WebSocket endpoint remains available for clients that intentionally select that transport.

The implementation deliberately does not add HTTP-style `/namespace/...` routing for raw AMQP. Real Service Bus AMQP is host-scoped at the TCP/TLS layer and entity-scoped inside AMQP links: queues use `{queue}`, topic sends use `{topic}`, subscription receives use `{topic}/Subscriptions/{subscription}`, claims use `$cbs`, and management links use `{entity}/$management`.

Coverage uses the official `azservicebus` SDK without `NewWebSocketConn`: the tests pass an unchanged Service Bus connection string, configure `CustomEndpoint` to the simulator's raw AMQP listener, provide test TLS config for the self-signed listener certificate, and run both queue Send/Receive and topic/subscription Send/Receive.

## 2026-05-26 — Issues #227/#228: Azure Blob block staging + Service Bus AMQP data plane

PR #229 closed issues #227 and #228. Both reports were real. Blob block operations were being treated as ordinary `PutBlob` or falling through because the blob data-plane dispatcher did not branch on `?comp=block` and `?comp=blocklist`. The fix adds persistent committed/uncommitted block state, StageBlock, CommitBlockList, and GetBlockList handlers, and official `azblob/blockblob` SDK coverage for staging, listing, committing, and downloading the materialized blob.

Service Bus had a deeper protocol gap: the simulator had a real REST message data plane, but the official `azservicebus` client uses AMQP 1.0 over WebSocket. The fix adds the AMQP slice needed by canonical Send/Receive flows: WebSocket upgrade, SASL anonymous, CBS claim RPC, sender and receiver links, link credit, accepted dispositions, management-link open, settled receive-and-delete transfers, topic fan-out to simulator subscriptions, and subscription receiver path normalization. Coverage uses the official Go SDK to send and receive both a queue message and a topic/subscription message through the simulator.

Docs now include `specs/SIM_SURFACE_TABLES/azure-servicebus-data-plane.md`, and the Storage data-plane table lists the block blob rows. BUG-1184 and BUG-1185 are closed; BUG-1075 and BUG-1104 remain the only open BUG entries.

## 2026-05-26 — Phase 226: Azure host-scoped protocol audit

PR #225's Service Bus admin fix exposed the broader pattern: ARM/control-plane coverage does not prove service-native host-scoped SDK protocols work. Phase 226 started with the current Azure host/data-plane surfaces and found the concrete sibling gap in Storage. Blob, File, Queue, and Table data planes already had raw HTTP tests, but only raw wire coverage meant the official SDK call shapes were not locked in.

The phase adds official Azure SDK tests for blob, file, queue, and table lifecycle flows, including List pager calls on every supported List surface. The new SDK coverage found two real protocol gaps: File service-level `GET /?comp=list` ListShares was missing, and the Tables SDK lists entities through `/{table}()` while the raw test used `/{table}`. Both shapes are now implemented.

Docs now include `specs/SIM_SURFACE_TABLES/azure-storage-data-plane.md`, and the Key Vault data-plane table was refreshed for rows already implemented and covered by SDK state-machine tests. BUG-1183 is closed by this phase; BUG-1075 and BUG-1104 remain the only open BUG entries.

## 2026-05-26 — Issue #223: Azure Service Bus ATOM admin protocol

PR #225 closes #223 / BUG-1182. The report was real: the Azure simulator had ARM management routes for `Microsoft.ServiceBus` and REST message data-plane routes under `{namespace}.servicebus.<host>`, but the official `azservicebus/admin` SDK speaks a third protocol: namespace-level ATOM XML admin routes on the Service Bus host. Requests such as `PUT /{queue}?api-version=2021-05` were falling through to the message data-plane dispatcher and returning `ResourceNotFound`.

The fix adds the namespace admin protocol for queues, topics, subscriptions, and rules, including ATOM entry/feed responses, empty-feed not-found behavior expected by the SDK, `$Default` rule creation for subscriptions, and cascade cleanup when topics/subscriptions are deleted. Coverage uses the official Go admin SDK for queue lifecycle plus topic/subscription/rule lifecycle, including paged List calls.

The systematic lesson is that some cloud services have service-native host-scoped protocols beside their ARM/control-plane APIs. A route seeder that only sees top-level mux registrations can miss those sub-surfaces. The next phase plan is to audit host-wrapper/data-plane SDK surfaces across the current Azure services, starting with Service Bus, Key Vault, and Storage, and to keep each rounded-out service represented by a surface table plus paged canonical-client tests.

## 2026-05-26 — Admin stack lifecycle UI + Makefile repair

PR #222 closed the admin-stack cleanup task. The Makefile issue was real: `backend-build-dir` had an unterminated nested `$(strip ...)` call, and the stack lifecycle targets also exposed two runtime bugs once the parser was fixed. Background services were started from short-lived recipe shells and died with the parent, and the Azure/GCP/FaaS stack shortcuts did not write the simulator-safe backend env defaults that the binaries require.

The fix replaces the fragile nested backend lookup expressions with explicit backend path maps, has `start-component` record a durable wrapper PID that forwards stop signals and writes exit status, and has `stack-up` write per-backend env files for the local simulator stacks before starting the backend. `stack-status` / `stack-down` now delegate to the shared component lifecycle targets instead of carrying a second stop/status implementation.

The admin API and UI now cover the operator lifecycle from one place: topology instances and managed processes can start, stop, and restart; the topology page schedules the repo's real `make stack-down`; individual component UIs are linked from admin; empty topology gets default local contexts; and admin/API failure panels show the concrete recovery `make` commands. The skill update added an `avoid-vibe-slop` check requiring real post-start status + request probes for Makefile/script background-service targets.

## 2026-05-26 — Issue #220: Azure Blob List Containers properties

PR #221 closed #220. The issue was real: Azure Blob `GET /?comp=list` returned `<Container><Name>...</Name></Container>` entries without the real Azure `<Properties>` block, so Azure CLI / SDK consumers saw empty `properties.lastModified` and `properties.etag` even though single-container `GET /{container}?restype=container` returned `Last-Modified`.

The fix persisted a real container ETag at create time, returned that same ETag from `Get Container Properties`, and emitted per-container `<Properties><Last-Modified>...</Last-Modified><Etag>...</Etag></Properties>` from `List Containers`. The list response is sorted by container name for deterministic output.

Coverage added both sides that matter for this bug: a raw wire XML regression in `simulators/azure/sdk-tests/blob_keys_certs_test.go`, and a real `az storage container create/show/list` regression in `simulators/azure/cli-tests/blob_test.go`. The PR also carried the generated README badge refresh and a tiny trailing-whitespace cleanup in the `silent-error-swallow-scan` skill.

## 2026-05-25 — Phase 180: GCP Secret Manager lifecycle endpoints

PR #219 closed #218. GCP Secret Manager gained the lifecycle paths real clients were missing: ListSecretVersions, UpdateSecret labels, DeleteSecret, replication metadata preservation, and payload CRC32C behavior. The fix shipped with SDK, gcloud CLI, and Terraform coverage for create/add/list/update/delete flows.

## 2026-05-25 — Phase 179: 2 reopens + 3 community-filed issues

PR #216 closed two reopens (#209 / #210) and three new issues (#213 / #214 / #215), filing and fixing BUG-1174..1180. The reopen themes were the same post-Phase-178 lesson: a green single-client test can still miss another real client path or a content-shape assertion. Fixes covered GCP Memorystore upgrade/failover keying, Pub/Sub IAM verbs, real-shape Azure listKeys, Azure Resources Tags API, Service Bus authorizationRules, AWS IAM managed-policy / instance-profile lifecycle, and API Gateway v1 method/integration response handlers. PR #217 followed with the README badge refresh and hook portability fixes.

## 2026-05-25 — Phase 178: 9 community-filed issues closed (1 reopen + 8 new) + five class-of-bug remediations

PR #202 (Phase 177) merged. The user immediately filed 8 new issues (#203..#210) and reopened #196 (S3 multipart `ListParts` missed in Phase 176/177's S3 sweep). The reopen + the new issues decomposed into **five distinct classes of bug** that the existing skill catalogue caught only partially: (1) partial-table coverage — `surface-table-completeness` was reactive, requiring a table to exist before it could enforce completeness, and the repo had 30+ service surfaces without tables; (2) mux pattern collision — the collapsed-port sim had no scanner to flag pattern shadowing, so the wrong handler responded with a plausible-looking error; (3) wire-shape drift on List / paged endpoints — single-record envelope from a List op silently passed `.Value[0]`-style SDK tests; (4) state-machine fakery — sim stored data flat with no lifecycle states even though SDKs read them; (5) terraform-provider call sequence drift — `simulators/{cloud}/terraform-tests/` existed locally with 77 stock-provider resources across the three clouds but the CI `sim` job invoked only `sdk-test` + `cli-test`, so every recent reopen (BUG-1098 / 1099 / 1142 / 1147) surfaced against a tf-provider sequence the SDK tests had already marked green. Phase 178 closes the 9 community-filed items AND the 5 class-of-bug remediations on a single PR (18 commits, 6 stages + a CI-wiring follow-up).

**Stage A — infrastructure.** `scripts/seed-surface-tables.sh` proactively populates `specs/SIM_SURFACE_TABLES/` from every registered `mux.HandleFunc(...)` / `AWSRouter.Register(...)` call across `simulators/{aws,azure,gcp}/*.go` — 46 tables covering ~700 ops, so the reactive-skill gap (BUG-1145) becomes a proactive one. New `mux-overlap-scan` skill + scanner flags root-greedy wildcard patterns per cloud (286 baseline shadow pairs reported, almost all AWS S3's `{bucket}/{key...}` over literal-prefix services). Pre-commit hook runs the scanner in warn mode. Extended `sim-canonical-config-test` with a paged-iterator rule + `paged-shape verified` column in surface tables. New `sim-state-machine-completeness` skill formalises the data-model invariant for stateful resources with a worked example for Azure KV soft-delete.

**Stage B — AWS routing collisions.** BUG-1154 (#208): `AWSQueryRouter` extended with `RegisterVersioned(version, action, handler)`; dispatch is `(Version, Action) → handler` with the legacy `(Action,)` bucket as fallback. RDS / ElastiCache / SNS migrated to versioned registration; EC2 / IAM / STS keep legacy `Register` since their Action names are globally unique. BUG-1150 (#204): register the missing `POST /v2/apis/{apiId}/deployments` family + add a known-bucket gate to S3's POST/PUT/GET/DELETE `/{bucket}/{key...}` dispatchers so future missing-service paths fall through to canonical 404 instead of being swallowed.

**Stage C — AWS missing ops.** BUG-1148 (#196 reopened): `handleS3ListParts` returns canonical XML in monotonic PartNumber order; `aws-s3-multipart.md` surface table covers every multipart op. Reopen-postmortem in BUGS.md cites `manager.Uploader`'s mid-upload retry path as the SDK code path PR #200's test missed. BUG-1153 (#207): RDS snapshot family with `creating → available → deleted` state machine; SNS `SetTopicAttributes` persists `(AttributeName, AttributeValue)` into per-topic Attributes map; SQS `PurgeQueue` empties Messages while preserving Attributes/Tags.

**Stage D — Azure KV state machine.** BUG-1149 + 1151 (#203 + #205): `kvSecretStored` refactored from flat `name → SecretBundle` to a versioned chain with soft-delete state (`Versions []kvSecretVersion + DeletedAt + ScheduledPurgeAt + RecoveryID`). New handlers cover GetSecretVersion, ListSecretVersions (paged `SecretListResult`), PatchSecret (attribute-only updates per version), GetDeletedSecret, ListDeletedSecrets, RecoverDeletedSecret, PurgeDeletedSecret. State machine: active → soft-deleted (recoverable for 90 days) → recovered / purged. SDK tests `TestKeyVault_State_FullVersionChain` (uses `NewListSecretPropertiesVersionsPager` — the paged-iterator rule) + `TestKeyVault_State_SoftDeleteRoundTrip` (full transition cycle through canonical SDK methods).

**Stage E — Azure App Service config + extras.** BUG-1152 (#206): `/sites/{name}/config/{section}` routes for `appsettings` (PUT + POST `/list`), `connectionstrings` (PUT + POST `/list`), `web` (GET + PUT). POST `/list` follows real Azure's secret-bearing endpoint pattern. BUG-1156 (#210): PG FlexibleServer `configurations` LIST / GET / PUT; APIM `operations` + `backends` + `namedValues` CRUD; Cache Redis `firewallRules` CRUD + `POST .../listKeys`.

**Stage F — GCP missing ops.** BUG-1155 (#209): Cloud SQL `backupRuns.{insert,list,get,delete}` with state machine + `instances/{name}/clone` LRO; selfLink hard-coded `https://`. Memorystore Redis `:upgrade` + `:failover` with state-preserved transitions. Pub/Sub Snapshot CRUD with 7d ExpireTime. API Gateway AIP-130 IAM v1 (getIamPolicy / setIamPolicy / testIamPermissions).

**Stage G (CI follow-up) — gate the tf-tests.** BUG-1161: new `tf (aws|gcp|azure)` matrix job in `.github/workflows/ci.yml` runs `make terraform-test` per cloud (stock `hashicorp/aws` / `hashicorp/google` / `hashicorp/azurerm` providers against the sim binary via `terraform init` + `apply -auto-approve`). The Makefile target + the test sources already existed; this just wires the gate so the tf-provider call shape is exercised every push.

**Stage G (post-gate) — 12 real handler / wire-shape gaps the gate caught and we fixed in the same PR.** BUG-1162..1173:
- AWS `GetBucketAcl` returned 400 on `BucketOwnerEnforced` (real AWS only returns 400 from PutBucketAcl) — BUG-1167.
- azurerm provider rejected `skip_provider_registration=true + resource_provider_registrations="none"` clash (BUG-1163).
- Cloud Run + GCF `grpcAddrFromEndpoint` HTTP+1 fake → replaced with explicit `LogAdminEndpoint` config / `SOCKERLESS_GCP_LOGADMIN_ENDPOINT` env var (BUG-1162). Real GCP has Cloud Logging on its own endpoint; the sim now mirrors that.
- ACA arm64 manifest gap — main.tf switched to `public.ecr.aws/docker/library/alpine:latest` (multi-arch) + ECR-Public rate-limit retry in TestMain (BUG-1167 + helper retry).
- KV subscription-scoped `GET /subscriptions/{sub}/providers/Microsoft.KeyVault/vaults` + `GET .../deletedVaults` + `POST .../deletedVaults/{name}/purge` handlers (terraform-provider-azurerm's KV cache).
- App Service config sub-resources: `slotconfignames` (BUG-1173), `publishingcredentials/list` (BUG-1167-deferred), `authsettings/list` + `authsettingsV2/list`, `logs`, `backup/list`, `basicPublishingCredentialsPolicies/{ftp,scm}`.
- `Microsoft.Web/checkNameAvailability` + `Microsoft.Web/sites/{name}/config/azurestorageaccounts/list` (the latter was registered as GET; real Azure uses POST; the response also dropped `properties` field when empty → provider nil-deref).
- ACA `containerApps/{name}/listSecrets` + `jobs/{name}/listSecrets` (terraform-provider-azurerm reads on every plan).
- Subscription-scoped `GET /subscriptions/{sub}/resources` (KV URL → resource-ID cache lookup).
- Log Analytics `deletedWorkspaces` (workspace-soft-delete cache miss).
- Systematic action-verb canonicalization (BUG-1172): extended `AzurePathNormalizationMiddleware` to lowercase 13 known action / sub-resource segments (`appsettings`, `connectionstrings`, `slotconfignames`, `listsecrets`, `checknameavailability`, `authsettings`, `authsettingsv2`, `publishingcredentials`, `azurestorageaccounts`, `basicpublishingcredentialspolicies`, `deletedvaults`, `deletedworkspaces`). Replaces per-route duplicate handler registrations.

BUGS.md after this branch lands: **1173 filed · 1171 fixed · 2 open.** The proactive surface-table seed + the mux-overlap scanner + the paged-iterator rule + the state-machine skill + the CI-gated tf-tests are the load-bearing scaffolding that should keep similar reopen patterns from recurring — the next time the user finds a missing op, the table audit catches it before "fixed" is claimed, and the tf-provider gate catches divergence between SDK and provider call sequences.

## 2026-05-24 — Phase 177: KV WWW-Authenticate URL reopen + S3 bucket-subresources + four meta-skill improvements

PR #200 merged. User pushed back twice: **issue #193 reopened** (Azure KV `WWW-Authenticate` `authorization` URL had only 3 path-split segments; every Azure SDK indexes `parts[3]` to extract the tenant and panicked), and **issue #201 filed** (S3 bucket-level PUT subresources routed to CreateBucket → 409). Both surfaced the moment a real consumer ran an SDK or terraform against the merged build.

The shape: my test for a fix matches my implementation's path, not the real-client contract. Phase 177 closes both community-filed items plus four meta improvements that turn the fix-and-reopen loop into a single-pass workflow:

- **BUG-1142** (#201) — S3 bucket-subresource dispatcher with 20-entry `bucketSubresourceHandlers` map; bodies persist in `s3BucketConfigs` so PUT → GET round-trips byte-for-byte.
- **BUG-1143** (#193 reopened, postmortem of BUG-1135) — Authorization URL changes to `http://<host>/00000000-0000-0000-0000-000000000000` (4 path segments). `keyvault_sdk_test.go` uses real `azsecrets` / `azkeys` / `azcertificates` SDK clients exercising the full challenge-then-retry handshake.
- **BUG-1144** — `sim-canonical-config-test` extended: "use the SDK, not raw HTTP" + "permitted SDK-config diffs" + "terraform-provider tests".
- **BUG-1145** — `specs/SIM_SURFACE_TABLES/` + `surface-table-completeness` skill. Initial population covered AWS S3 bucket-subresources + Azure KV data plane.
- **BUG-1146** — `reopen-postmortem` skill: every reopen carries (a) what test passed but should have failed, (b) what SDK code path was missed, (c) what new canonical-client test catches the regression. Backfilled BUG-1134 + BUG-1143.
- **BUG-1147** — terraform-tests parity: AWS gains 6 bucket-subresource resources; Azure gains `azurerm_key_vault_secret` exercising the challenge handshake at the terraform layer.

In-PR audit pass closed 5 additional findings (2 pre-existing fakes in s3.go, 2 timeless-comments violations, surface-table gaps). The new `surface-table-completeness` + `reopen-postmortem` skills exercised themselves against the same PR and surfaced their own findings — the new skills work.

## 2026-05-24 — Phase 176: 8 community-filed issues + path-style storage dispatcher

PR #192 merged. User filed 7 new issues + reopened #190 (path-style storage). Phase 176 closed all 8 on a single PR (BUG-1134..1141):

- **#193** Azure KV WWW-Authenticate (BUG-1135) — initial fix; reopened in Phase 177 because the URL format broke the SDK parser. Same commit replaced `"AQAB"` JWK placeholder with real EC / oct key generation.
- **#195** Azure Service Bus REST data plane (BUG-1137) — replaced stub with real `servicebus_dataplane.go`; SendMessage→201, ReceiveAndDelete→200/204, PeekLock→201+Location, CompleteLock→204.
- **#190 reopened** (BUG-1134) — `hasAzureStorageSignal(r)` discriminator (`x-ms-version` / `x-ms-date` / `x-ms-blob-type` / `restype` / `comp` / `SharedKey`) partitions storage path-style requests from IMDS / MSI / Monitor co-tenants on the shared sim port.
- Plus #194 RDS+ElastiCache EngineVersion (BUG-1136), #196 S3 multipart family (BUG-1138; ListParts missed → reopened in Phase 178), #197 GCP `/v1/operations` (BUG-1139), #198 GCS compose + resumable name + https (BUG-1140), #199 Lambda subresources (BUG-1141).

Twelve in-PR audit findings landed in the same branch: real KV crypto (no `AQAB`), SB PeekLock Location with `/messages/` segment, BrokerProperties marshal fail-loud, dead code purges, silent `io.ReadAll(_)` swallows fixed, KV WWW-Authenticate URL annotated `// external:`, streaming-body comments per the positive-confirmation rule, 4 phase-ref test comments rewritten.

## 2026-05-24 — Phase 175: second skill-sweep audit + 3 community-filed issues + signal-driven FaaS smoke tests

The audit cadence introduced in Phase 174 ran for the second time. Six parallel skill agents surfaced 36 candidate sites across 6 BUG umbrellas — every one a real bug, including two recurrences of Phase 174 introductions. Most consequential: two persistence-strip bugs (BUG-1098 shape) — GCP Secret Manager's `payload []byte` unexported, Azure KV Secret/Key/Cert carrying `json:"-"` on Vault+Name. Both fixed via persistence-wrapper records.

Three fresh GitHub issues against the post-Phase-174 sim code (#189 / #190 / #191) — all closed in the same PR.

Side-tracks: de-flaked the FaaS smoke tests (`ContainerStop` instead of file-based polling); split the CI `test` job into `test-core` + `test-backend` matrix×6 + `test-faas-smoke` matrix×5 (11 → 22 jobs); new `timeless-comments` skill codified the rule that code comments must explain current invariants, never narrate evolution.

14 BUGs closed (1120-1133).

## 2026-05-24 — Phase 174: first skill-sweep audit + community-issue triage (PR #180)

Two rounds on one branch. Round 1 ran the 5 specialist skills committed in Phase 173.0 across the repo and found exactly the patterns each was written to detect, including in code just landed in Phase 173 (the meta-shape — write the rule, then violate it elsewhere, then have the rule catch it). 4 quick fixes + 3 follow-up BUGs filed (1109 Azure storage data planes, 1110 streaming-body sweep, 1111 emitted-URL annotations).

Round 2 closed the 3 follow-ups plus all 8 new GitHub issues #181..#188 — case-insensitive ARM routes, Pub/Sub Subscription field drops, relative selfLink, KV duplicated host, placeholder RSA modulus, SQS Attributes drop, SM `latest` alias literal echo. 15 BUGs closed (1105–1119).

## 2026-05-23 — Phase 173: simulator wire-fidelity sweep (PR #179)

Six GitHub issues (#173–#178) surfaced wire-protocol drift across all three sims. The smoking gun was #173 (AWS S3 routes under `/s3/` prefix instead of canonical root) — latent since 165+ phases because three test-side workarounds independently reconfigured around it. The meta blind-spot codified as BUG-1104: simulator tests verifying the sim against itself rather than the real cloud.

Phase 173 sub-phases 0–12: six new project-local Claude skills (canonical-config, emitted-url-roundtrip, streaming-body, silent-error-swallow, dead-code-silencer, backpedal-pattern-audit); sentinel-header logging; ~180 new simulator operations across AWS (S3 routing + aws-chunked + Secrets Manager versions + SQS + SNS + API Gateway v1/v2 + RDS + ElastiCache), GCP (Pub/Sub + Memorystore + API Gateway + Cloud SQL), Azure (Blob data plane + KV keys/certs + Cache Redis + PG FlexibleServer + Service Bus + APIM). All six issues closed before PR merge.

## Compressed older-phase headlines (Phase 78–172)

Per-bug detail lives in [BUGS.md](BUGS.md); per-commit detail in `git log <PR-number>`.

| PR | Phase | Headline |
|---|---|---|
| #172 | pod-model follow-up | Sim pod materialization fidelity — real multi-container execution for ECS / Cloud Run Services + Jobs / ACA Apps; AZF docs corrected to single-container. |
| #170 | 168 follow-up | FaaS runner smokes for Lambda / Cloud Run / GCF / ACA Apps / AZF; AZF bootstrap hardening; GCP AR endpoint-fidelity; live-validation runbook. |
| #166 | 166 | Real fixes for 3 Phase-165 follow-ups: AWS 5 sim handler gaps (KMS / SM / SSM / DDB / S3 path-style), Azure azurerm-against-sim wiring, GCP `iam_beta_custom_endpoint`. |
| #165 | 165 | Third vibe-slop sweep + sim test-pyramid expansion + codex CLI review + continuity-doc compression. 9 BUGs closed. |
| #164 | 164 | Second vibe-slop sweep — 19 BUGs closed (1014–1032): strict-decode sweep, dead-code purges, phase-ref sweep, tf-tests expansion. |
| #163 | 163 | Makefile legacy alias rip-out + docs sweep. |
| #162 | 162 | `docs/VIBE_CODING.md` 23 → 35 patterns; `avoid-vibe-slop` skill expanded 17 → 26. |
| #161 | 161 | First comprehensive vibe-slop sweep — 18 BUGs closed (994–1008); bleephub GraphQL completion. |
| #160 | 160 | Project-local Claude skills (sim-handler-checklist + cross-resource-stack-test); component-README adaptor-led sweep. |
| #159 | 159 | AWS sim CloudFront + ACM + Route 53 + WAFv2 + Amplify + IAM SLR/OIDC (11 sub-tasks). |
| #158 | 158 | Handler→`s.self` delegation; `docs/VIBE_CODING.md` 23-pattern catalogue; first 3 project-local skills. |
| #155–157 | 155–157 | Component ⇄ reference-adaptor docs sweep; `backends/docker/README.md` rewrite. |
| #153–154 | 153–154 | bleephub ↔ GitHub API parity + SQLite persistence + real `gh` CLI compat. |
| #150–152 | 87c/d, 92 | zerolog → OTel logs bridge across 12 components; trace propagation; cloudrun + gcf `Backing: gcs-fuse` deregistered. |
| #147–149 | 91, 91b, 91c | `BackingMemory` translator across 5 backends; Lambda volume-translator framework. |
| #145–146 | 87, 87b | Observability stack (otel-collector + VictoriaLogs + Jaeger). |
| #143–144 | 85–86 | Config edit + hot reload; health + supervision (exit-code capture, `/diagnostics`). |
| #137–142 | 78–84 | UI polish + admin orchestration (`sockerless.yaml` topology, Topology page, per-instance logs + console). |
| #135–136 | 121b | Azure sim hardening; driver consolidation pattern B. |
| #128–134 | 124–134 | Driver framework + Makefile std + sim host model + arm64 CI runners. |
| #125 | CI reorg | Workflows reorganized: zero auto-fire on main; live-tests-{cloud}. |
| #112–123 | 86–123 | Sim parity; stateless backends; FaaS pod overlays; **8/8 runner cells GREEN**. |
