# Simulator Foundational Service Audit

Date: 2026-05-27

This audit checks whether each per-cloud simulator has real cloud-API slices for foundational services: object storage, basic managed data stores, DNS, queues, event routing, stream/event ingestion, VPC/networking, NAT/egress, and managed load balancers.

This is not a license to add simulator-specific APIs. Any missing row below is a missing public cloud API slice and is tracked in `BUGS.md`.

## Summary

| Category | AWS simulator | GCP simulator | Azure simulator | Finding |
|---|---|---|---|---|
| Object storage | S3 implemented | GCS implemented | Blob/File/Queue/Table storage implemented | Present across all three. Surface tables still need refresh for stale test-gap markers. |
| Queue/message systems | SQS and SNS implemented | Pub/Sub implemented | Service Bus and Storage Queue implemented | Present for core queue/pub-sub flows. |
| Event routing | EventBridge rules/targets plus buses/policies/archives/replays implemented | Eventarc triggers plus channels/providers/channel connections implemented | Event Grid topics/domains/domain topics/system topics/partner topics/subscriptions implemented | Event routing parity is rounded out across the advanced event-service phase. |
| Stream/event ingestion | Kinesis implemented | Pub/Sub present for basic event bus flows | Event Hubs implemented | Present for the foundational stream-ingestion flows. |
| Managed NoSQL/data SaaS | DynamoDB implemented | BigQuery missing; Firestore/Datastore missing | Cosmos DB missing | Real gap. Tracked as BUG-1201 and BUG-1202. |
| DNS | Route 53 and Cloud Map implemented | Cloud DNS implemented | Private DNS implemented; public DNS missing | Azure public DNS gap tracked as BUG-1205. |
| VPC/network primitives | VPC, subnet, IGW, route table, SG, EIP, NAT, ENI describe implemented | Network, subnetwork, firewall, router/NAT, VPC Access implemented | VNet, subnet, NSG, NAT gateway, route table implemented | Present, but NAT/public-IP parity is uneven. Tracked as BUG-1204. |
| VM/EC2-like compute | EC2 instance lifecycle missing | Compute Engine instance lifecycle missing | Azure VM lifecycle missing | Real gap. Public APIs should be cloud-compatible; Firecracker can be the local microVM runtime substrate. Tracked as BUG-1207. |
| Managed load balancers | Missing ELBv2/ELB | Missing Cloud Load Balancing resources | Missing Load Balancer/App Gateway/Front Door/Traffic Manager | Real gap. Tracked as BUG-1203. |
| Gateway/proxy APIs | API Gateway, API Gateway v2, CloudFront implemented | API Gateway implemented | APIM implemented | Present, but not a substitute for managed load-balancer APIs. |

## Current Implemented Slices

### AWS

Foundational slices registered today:

- Object storage: S3, including multipart and many bucket/object subresources.
- Data stores: DynamoDB, RDS, ElastiCache.
- DNS and discovery: Route 53, Cloud Map.
- Queue and pub-sub: SQS, SNS, including SNS to SQS fanout.
- Event routing: EventBridge rules, targets, tags, event buses, bus policies, archives, replays, and `PutEvents`, including SQS/SNS target delivery.
- Networking: EC2 VPCs, subnets, internet gateways, elastic IPs, NAT gateways, route tables, security groups, network-interface describe.
- Gateways and edge: API Gateway v1/v2, CloudFront, WAFv2, ACM.
- Identity and secrets: IAM, STS, Secrets Manager, SSM Parameter Store, KMS.
- Stream ingestion: Kinesis stream lifecycle, shards, records, iterators, tags, retention, monitoring, encryption state, shard-count updates, and limits.

Missing foundational slices:

- EC2 instance lifecycle APIs such as `RunInstances`, `DescribeInstances`, `StartInstances`, `StopInstances`, `TerminateInstances`, and related instance metadata/volume/network attachment flows. BUG-1207.
- ELBv2/classic ELB managed load balancers. BUG-1203.

### GCP

Foundational slices registered today:

- Object storage: GCS.
- Queue/pub-sub: Pub/Sub.
- Event routing: Eventarc trigger lifecycle, channels, provider discovery/listing, and channel connections.
- DNS and discovery: Cloud DNS.
- Networking: Compute networks, subnetworks, firewalls, routers/NAT, VPC Access connectors.
- Gateways: API Gateway.
- Data stores: Cloud SQL, Memorystore Redis, Secret Manager.
- Identity/logging/build: IAM, OAuth2 token endpoint, Cloud Logging, Cloud Build, Service Usage.

Missing foundational slices:

- Compute Engine instance lifecycle APIs, including instance create/get/list/delete/start/stop and network/disk attachment behavior. BUG-1207.
- BigQuery managed analytics. BUG-1201.
- Firestore/Datastore document/key-value store. BUG-1202.
- Cloud Load Balancing resources such as forwarding rules, target proxies, URL maps, backend services, health checks, and addresses. BUG-1203/BUG-1204.

### Azure

Foundational slices registered today:

- Object/storage data planes: Blob, Files, Queues, Tables.
- Queue/message systems: Service Bus ARM/admin/data plane, REST, AMQP-over-WebSocket, and raw AMQP/TLS.
- Event routing: Event Grid topics, domains, domain topics, system topics, partner topics, event subscriptions, subscription validation, and custom-topic publish/delivery.
- Stream ingestion: Event Hubs ARM namespace/event hub/consumer group/auth-rule lifecycle plus AMQP send/receive over raw AMQP/TLS.
- DNS and discovery: Private DNS.
- Networking: Virtual Networks, subnets, Network Security Groups, NAT gateways, route tables.
- Gateways: APIM.
- Data stores: Cache for Redis, PostgreSQL Flexible Server.
- Identity/secrets/logging: Managed Identity, Key Vault, Monitor, Application Insights, authorization/resources/tags.

Missing foundational slices:

- Azure Virtual Machines lifecycle APIs, including VM create/get/list/delete/start/deallocate and NIC/disk/public-IP attachment behavior. BUG-1207.
- Cosmos DB managed NoSQL/table/document data store. BUG-1202.
- Managed load-balancer services: Azure Load Balancer, Application Gateway, Front Door, Traffic Manager. BUG-1203.
- Public IP/Public IP Prefix resources and full NAT association/list behavior. BUG-1204.
- Public Azure DNS zones/record sets. BUG-1205.

## Next Implementation Phase

Recommended order:

1. Managed data stores: add BigQuery plus Firestore/Datastore and Cosmos DB. Keep analytics and NoSQL separate if the PR becomes too large.
2. VM/EC2-like compute substrate design: add public EC2/GCE/Azure VM API slices backed by real local execution. Firecracker is a good candidate for the local microVM runtime, but it stays behind the simulator boundary; callers see only cloud public APIs.
3. Managed load balancing and public network egress: add ELBv2/ELB, GCP load-balancing resources, Azure Load Balancer/Application Gateway/Front Door/Traffic Manager, and the missing public-IP/address pieces.
4. Azure public DNS: add Microsoft.Network/dnsZones and record-set public API parity.
5. Surface-table cleanup: refresh stale surface-table status rows so implemented/tested coverage matches the current repo.

Each added service slice must follow the simulator testing contract: official SDK tests, vendor CLI tests, and Terraform provider tests in the same PR unless the public API is not exposed by one of those client surfaces.
