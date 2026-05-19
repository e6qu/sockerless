# FaaS Pod Specification

This document records the current contract for FaaS-shaped backends when a Docker/Podman pod requires multiple containers sharing `localhost`.

## Contract

A sockerless pod is valid only when every container in the pod can reach the other pod members through the same network namespace, normally `127.0.0.1:<port>`. If a backend cannot provide that with its real cloud primitive or an implemented bootstrap path, it must fail clearly.

## Current Mapping

| Backend | Current pod materialization | Status |
|---|---|---|
| Lambda | Image-mode Lambda plus the implemented overlay/reverse-agent path for supported workloads. Lambda has no native multi-container function primitive. | Supported only within the implemented Lambda overlay limits. |
| Cloud Functions Gen2 (`gcf`) | Cloud Function resource plus an update to the function's underlying Cloud Run Service template so pod members become real Cloud Run sidecars sharing localhost. | Supported through the Function-owned Cloud Run Service path. |
| Azure Functions (`azf`) | Linux Function App custom container. Azure Functions exposes one custom-container slot to this backend. | Multi-container pods are not supported; use ACA Apps for Azure sidecar workloads. |

## Non-Goals

- Do not map Azure Functions pods to ACA resources from the AZF backend. The AZF backend must stay on Azure Functions primitives.
- Do not describe Lambda Extensions as the current implementation. Extensions are a possible platform concept, but the backend's current pod path is not packaged as arbitrary container sidecars through Lambda layers.
- Do not return synthetic success for pod starts when sidecars are not actually running in the same localhost namespace.

## Test Expectations

Simulator coverage for FaaS pod behavior must use the real client surfaces where applicable: official SDKs, vendor CLIs, and Terraform providers. The simulator may use Docker/Podman network namespaces internally, but the API contract exposed to SDK/CLI/Terraform clients must match the cloud service being simulated.
