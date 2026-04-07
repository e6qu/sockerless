# FaaS Pod Specification

How to support pods (multiple containers sharing localhost) on FaaS backends.

## Concept

A Docker/Podman pod groups containers that share a network namespace — they communicate via `localhost`. On container backends (ECS, Cloud Run, ACA), this maps naturally to multi-container tasks/jobs. On FaaS backends, the mapping is less direct but possible.

## Per-Cloud Mapping

### AWS Lambda → Lambda Extensions

Lambda Extensions run as sidecar processes in the same execution environment as the handler. They share:
- **Network**: Same network namespace — `localhost` communication works
- **Lifecycle**: Extensions start before the handler and can run cleanup after invocation

**Implementation:**
```
Pod with svc1 (handler) + svc2 (sidecar):
  → svc1 = Lambda function image (the handler)
  → svc2 = Lambda Extension layer (sidecar binary in /opt/extensions/)
  → Both share localhost and /tmp
```

**API mapping:**
- `podman pod create --name mypod` → register intent
- `podman create --pod mypod --name svc1 nginx:alpine` → handler function
- `podman create --pod mypod --name svc2 alpine sleep 3600` → extension layer
- `podman pod start mypod` → `CreateFunction` with handler image + extension layer

**Limitations:**
- Extensions must be packaged as Lambda layers, not arbitrary container images
- Extensions have a 2-second shutdown window
- No independent lifecycle — extension starts/stops with the handler
- Max 10 extensions per function

### GCP Cloud Functions v2 → Cloud Run Sidecars

Cloud Functions v2 runs on Cloud Run infrastructure. Cloud Run supports multi-container deployments with shared network namespace.

**Implementation:**
```
Pod with svc1 + svc2:
  → Cloud Run Service with 2 containers in the same revision
  → Container 1: svc1 (the "ingress" container, serves the function)
  → Container 2: svc2 (sidecar, shares localhost)
```

**API mapping:**
- `podman pod create --name mypod` → register intent
- `podman create --pod mypod --name svc1 nginx:alpine` → ingress container
- `podman create --pod mypod --name svc2 redis:alpine` → sidecar container
- `podman pod start mypod` → `CreateService` with multi-container revision spec

**Limitations:**
- One container must be the "ingress" (receives traffic)
- Sidecars must declare startup/liveness probes
- Shared volume via `emptyDir` only (no persistent volumes)

### Azure Functions → Azure Container Apps Sidecars

Azure Functions on Container Apps (the "Flex Consumption" plan or Container Apps environment) supports multi-container jobs via ACA's sidecar model.

**Implementation:**
```
Pod with svc1 + svc2:
  → ACA Job with 2 containers
  → Container 1: svc1 (the function app)
  → Container 2: svc2 (sidecar)
  → Both share localhost network
```

**API mapping:**
Same as ACA backend — this IS the ACA multi-container model.

**Limitations:**
- Requires Azure Functions on Container Apps (not Consumption plan)
- Sidecar lifecycle tied to the main container

## Tag Convention for Pods on FaaS

```
Tags on Lambda function:
  sockerless-managed: true
  sockerless-pod: mypod
  sockerless-pod-role: main          # "main" or "sidecar"
  sockerless-pod-sidecar-0: svc2    # sidecar container names
```

Pod listing queries for distinct `sockerless-pod` values.

## Implementation Priority

| Backend | Pod Support | Complexity | Priority |
|---------|------------|------------|----------|
| ECS | Multi-container task (done) | Low | Already implemented |
| Cloud Run | Multi-container revision | Medium | High — native support |
| ACA | Multi-container job (done) | Low | Already implemented |
| Lambda | Extensions as sidecars | High | Medium — layer packaging required |
| GCF v2 | Via Cloud Run sidecars | Medium | Medium — delegates to Cloud Run |
| AZF | Via ACA sidecars | Low | Low — delegates to ACA |

## Not Supported

- **Lambda Consumption**: No sidecar support. Extensions only (binary, not container image).
- **GCF v1**: No sidecar support. Only GCF v2 (Cloud Run-based) supports multi-container.
- **AZF Consumption Plan**: No sidecar support. Only AZF on Container Apps.

For these, `podman pod create` with sidecars should return an error explaining the limitation.
