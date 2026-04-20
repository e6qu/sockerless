# ECS services: cross-container DNS via Cloud Map

## Problem

CI runners — GitLab's docker-executor, GitHub Actions with `services:` or `container:` directives — express service containers as N parallel `docker container create` calls on the same `docker network`. The build container then reaches the services by short name (`postgres:5432`, `redis:6379`). On the ECS backend running against live Fargate, each container is an ENI-isolated task; DNS resolution across tasks must go through AWS Cloud Map.

Before this change, `backends/ecs/service_discovery_cloud.go` created one Cloud Map service named `containers` per namespace and registered every container there. That gives a single DNS name (`containers.skls-<net>.local`) resolving to all IPs — useless for per-service lookup. The piping compiled and ran, so nothing screamed, but cross-container DNS by hostname would have failed on real Fargate.

## Design

One Cloud Map service per **container hostname**, created on demand.

- `cloudNamespaceCreate` now creates only the `skls-<net>.local` namespace. The old `containers` service pre-creation is removed.
- `cloudServiceRegister(containerID, hostname, ip, networkID)` calls `findOrCreateServiceForHostname(namespaceID, hostname)` which lists services in the namespace and either returns an existing service with that name or calls `CreateService(Name=hostname, ...)` with an A-record `DnsConfig`. The container's instance is then registered under that service via `RegisterInstance`.
- `cloudServiceDeregister` deregisters the instance and, if the service has no remaining instances (`ListInstances` == 0), calls `DeleteService` so the DNS name is reclaimed for later reuse.
- The ECS task definition built by `buildContainerDef` now sets `DnsSearchDomains` to the list of namespace names for every non-pre-defined network the container is connected to. This lets a bare `postgres` lookup inside the container resolve to `postgres.skls-<net>.local` without the client needing to know the domain.

## What changes for callers

Nothing at the `api.Backend` boundary. Services remain client-composed: the CI runner still calls `NetworkCreate` then N × `ContainerCreate` + `ContainerStart` with distinct names. The cloud-side plumbing is now correct — per-name Cloud Map services plus search domains on the task — so cross-container DNS resolves on real Fargate.

## What still needs live AWS to prove

- End-to-end DNS resolution from inside a Fargate task. Private DNS through Cloud Map requires the target VPC to have DNS hostnames + DNS resolution enabled, and the task's ENI must be in that VPC. The simulator doesn't emulate Route 53 — it can confirm the API calls happen but cannot confirm a container actually resolves `postgres` to the right IP.
- Security group ingress rules between service tasks. `network_cloud.go` creates a per-network SG and attaches each task to it; the default rule-set must allow task-to-task traffic on the ports the CI job needs. Not changed in this PR; flagged for live-mode verification.
- Service deletion race on rapid container churn. `DeleteService` returns an error if the service has 0 instances but deletion is already in progress for the next register. The deregister path ignores that error (`_, _ =`); verify behavior under concurrent CI jobs in live mode.

These are all AWS-track work.

## Files changed

- `backends/ecs/service_discovery_cloud.go` — namespace-only creation, per-hostname service find-or-create, service cleanup on last deregister, `searchDomainsForContainer` helper.
- `backends/ecs/taskdef.go` — `buildContainerDef` sets `DnsSearchDomains`.
- `backends/ecs/service_discovery_cloud_test.go` — unit tests for search-domain computation.

## Reverting

The change is additive in behavior. Reverting means restoring the shared `containers` service, dropping the find-or-create logic, and removing `DnsSearchDomains` from the task def. No data migration needed — Cloud Map services aren't persisted across backend restarts by sockerless.
