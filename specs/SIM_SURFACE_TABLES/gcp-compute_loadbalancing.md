# Sim surface — gcp-compute_loadbalancing

Surface registered in `simulators/gcp/compute_loadbalancing.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with an open BUG or issue; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no meaningful client/provider surface for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /compute/v1/projects/{project}/global/healthChecks` | ✓ `simulators/gcp/compute_loadbalancing.go:19::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /compute/v1/projects/{project}/global/healthChecks/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:59::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /compute/v1/projects/{project}/global/healthChecks` | ✓ `simulators/gcp/compute_loadbalancing.go:62::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/healthChecks/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:65::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /compute/v1/projects/{project}/global/backendServices` | ✓ `simulators/gcp/compute_loadbalancing.go:69::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /compute/v1/projects/{project}/global/backendServices/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:97::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /compute/v1/projects/{project}/global/backendServices` | ✓ `simulators/gcp/compute_loadbalancing.go:100::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `PATCH /compute/v1/projects/{project}/global/backendServices/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:103::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/backendServices/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:140::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /compute/v1/projects/{project}/global/urlMaps` | ✓ `simulators/gcp/compute_loadbalancing.go:144::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /compute/v1/projects/{project}/global/urlMaps/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:163::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /compute/v1/projects/{project}/global/urlMaps` | ✓ `simulators/gcp/compute_loadbalancing.go:166::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/urlMaps/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:169::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /compute/v1/projects/{project}/global/targetHttpProxies` | ✓ `simulators/gcp/compute_loadbalancing.go:173::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /compute/v1/projects/{project}/global/targetHttpProxies/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:191::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /compute/v1/projects/{project}/global/targetHttpProxies` | ✓ `simulators/gcp/compute_loadbalancing.go:194::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/targetHttpProxies/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:197::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `POST /compute/v1/projects/{project}/global/forwardingRules` | ✓ `simulators/gcp/compute_loadbalancing.go:201::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /compute/v1/projects/{project}/global/forwardingRules/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:231::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `GET /compute/v1/projects/{project}/global/forwardingRules` | ✓ `simulators/gcp/compute_loadbalancing.go:234::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/forwardingRules/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:237::func` | ✓ (direct; see coverage matrix) | ✓ (direct; see coverage matrix) | n/a | |

## Coverage status

- Row-level SDK/Terraform cells summarize the maintained coverage matrix in `specs/SIM_TEST_COVERAGE_MATRIX.md`; detailed client files and client-family `n/a` decisions live there.
- Missing public-cloud operations that are not registered by the simulator still require a concrete BUG and a row here when discovered by a community issue or periodic audit.

<!-- HAND-WRITTEN BEGIN -->
Issue #263 closed the GCP managed load-balancer gap for the global external HTTP load-balancing control-plane chain. The implemented Compute slice covers global health checks, backend services, URL maps, target HTTP proxies, and global forwarding rules with AIP-style global operations. Coverage uses the official Compute Go SDK in `simulators/gcp/sdk-tests/compute_test.go`, `gcloud compute` lifecycle coverage in `simulators/gcp/cli-tests/compute_loadbalancing_test.go`, and Terraform `google_compute_health_check`, `google_compute_backend_service`, `google_compute_url_map`, `google_compute_target_http_proxy`, and `google_compute_global_forwarding_rule` resources in `simulators/gcp/terraform-tests/main.tf`.
<!-- HAND-WRITTEN END -->
