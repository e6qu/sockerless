# Sim surface — gcp-compute_loadbalancing

Surface registered in `simulators/gcp/compute_loadbalancing.go` (and related files grouped under this table). Rows below are the ops the sim currently registers — extracted by `scripts/seed-surface-tables.sh` from `mux.HandleFunc(...)` calls. ✗ rows for ops not handled by the sim are added when a community-filed issue or audit surfaces them.

## Status legend

- ✓ — implemented + tested
- ✗ — missing (paired with a BUG / deferred-subtask reference; never silent)
- 501 — stubbed NotImplemented (wire-visible gap)
- n/a — no terraform-provider resource for this op

## Implemented ops (extracted from HandleFunc registrations)

| Op (verb + path) | sim handler | sdk-test | tf-test | paged-shape verified | notes |
|---|---|---|---|---|---|
| `POST /compute/v1/projects/{project}/global/healthChecks` | ✓ `simulators/gcp/compute_loadbalancing.go:19::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/healthChecks/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:59::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/healthChecks` | ✓ `simulators/gcp/compute_loadbalancing.go:62::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/healthChecks/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:65::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/global/backendServices` | ✓ `simulators/gcp/compute_loadbalancing.go:69::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/backendServices/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:97::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/backendServices` | ✓ `simulators/gcp/compute_loadbalancing.go:100::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `PATCH /compute/v1/projects/{project}/global/backendServices/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:103::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/backendServices/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:140::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/global/urlMaps` | ✓ `simulators/gcp/compute_loadbalancing.go:144::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/urlMaps/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:163::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/urlMaps` | ✓ `simulators/gcp/compute_loadbalancing.go:166::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/urlMaps/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:169::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/global/targetHttpProxies` | ✓ `simulators/gcp/compute_loadbalancing.go:173::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/targetHttpProxies/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:191::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/targetHttpProxies` | ✓ `simulators/gcp/compute_loadbalancing.go:194::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/targetHttpProxies/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:197::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `POST /compute/v1/projects/{project}/global/forwardingRules` | ✓ `simulators/gcp/compute_loadbalancing.go:201::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/forwardingRules/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:231::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `GET /compute/v1/projects/{project}/global/forwardingRules` | ✓ `simulators/gcp/compute_loadbalancing.go:234::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |
| `DELETE /compute/v1/projects/{project}/global/forwardingRules/{name}` | ✓ `simulators/gcp/compute_loadbalancing.go:237::func` | ✗ (deferred under BUG-1159 sweep) | ✗ (deferred under BUG-1147 sweep) | n/a | |

## Open subtasks staged forward

- sdk-test / tf-test columns are ✗-with-deferral for every row above. Each subsequent surface-touching PR fills in the column for the rows it covers; remaining ✗s are tracked under BUG-1159 (paged-iterator sweep) + BUG-1147 (tf-test parity sweep).
- Missing ops (not in HandleFunc but documented by the cloud provider) get ✗ rows added when a community-filed issue surfaces them or a periodic audit lands a sweep.

<!-- HAND-WRITTEN BEGIN -->
Issue #263 closed the GCP managed load-balancer gap for the global external HTTP load-balancing control-plane chain. The implemented Compute slice covers global health checks, backend services, URL maps, target HTTP proxies, and global forwarding rules with AIP-style global operations. Coverage uses the official Compute Go SDK in `simulators/gcp/sdk-tests/compute_test.go`, `gcloud compute` lifecycle coverage in `simulators/gcp/cli-tests/compute_loadbalancing_test.go`, and Terraform `google_compute_health_check`, `google_compute_backend_service`, `google_compute_url_map`, `google_compute_target_http_proxy`, and `google_compute_global_forwarding_rule` resources in `simulators/gcp/terraform-tests/main.tf`.
<!-- HAND-WRITTEN END -->
