# Bug Sprint 22 — BUG-202 through BUG-226

**Date**: 2026-03-04
**Bugs fixed**: 25
**Focus**: Core lifecycle safety, Docker API parity, API type gaps, frontend conformance

## Bugs Fixed

| Bug | OB | Component | Issue | Fix |
|-----|-----|-----------|-------|-----|
| BUG-202 | OB-040 | Core | Container prune ignores `filters` query param | Parse `label` and `until` filters in PruneIf predicate |
| BUG-203 | OB-097 | Core | Pause check-then-act race | Moved precondition checks inside `Containers.Update` for atomicity |
| BUG-204 | OB-005 | Core | Pod remove without force doesn't check running containers | Added running container check, return 409 if any |
| BUG-205 | OB-006 | Core | Pod start/stop/kill don't cascade to containers | Iterate pod.ContainerIDs and apply start/stop/kill to each |
| BUG-206 | OB-099 | Core | Pod Hostname/SharedNS set outside registry mutex | Added CreatePodWithOpts that sets fields inside mutex |
| BUG-207 | OB-101 | Core | Redundant network connect leaks first IP allocation | Release old IP before allocating new one |
| BUG-208 | OB-102 | Core | Start event emitted with pre-update container snapshot | Re-fetch container after state update before emitting event |
| BUG-209 | OB-023 | Core | Stats streams forever after container exits | Check container running state after each sleep |
| BUG-210 | OB-062 | Core | Container rename doesn't update network Containers map | Update EndpointResource.Name in each connected network |
| BUG-211 | OB-061 | Core | System df image count wrong — aliases not deduplicated | Deduplicate images by ID using `seen` map |
| BUG-212 | OB-025 | Core | Container update treats malformed JSON as empty body | Distinguish io.EOF from parse errors, return 400 for bad JSON |
| BUG-213 | OB-059 | Core | System events only filters by "type" | Added action, container, and label filter support |
| BUG-214 | OB-046 | Docker | Exec inspect drops all ProcessConfig fields | Fetch raw JSON via HTTP API to extract ProcessConfig |
| BUG-215 | OB-049 | Docker | System events raw Docker passthrough | Map events.Message to api.Event with all fields |
| BUG-216 | OB-051 | Docker | Container list missing SizeRootFs field mapping | Added SizeRootFs to container summary |
| BUG-217 | OB-109 | Docker | VirtualSize hardcoded to Size | Use info.VirtualSize/img.VirtualSize instead of .Size |
| BUG-218 | OB-110 | Docker | Network disconnect returns 200 instead of 204 | Changed to http.StatusNoContent |
| BUG-219 | OB-067 | Docker | handleInfo uses context.Background() | Added ctx parameter to Info(), use request context |
| BUG-220 | OB-029 | Docker | Image load hardcodes quiet=false | Read quiet query param, pass to ImageLoad |
| BUG-221 | OB-111 | Docker | mapDockerError loses container ID | Parse resource type and ID from Docker error messages |
| BUG-222 | OB-112 | API | Volume missing UsageData field | Added VolumeUsageData type and UsageData field |
| BUG-223 | OB-114 | API | EndpointSettings missing Links field | Added Links []string field |
| BUG-224 | OB-115 | API | NetworkCreateRequest missing CheckDuplicate field | Added CheckDuplicate bool field |
| BUG-225 | OB-119 | Frontend | handleInfo missing LoggingDriver field | Added "LoggingDriver": "json-file" |
| BUG-226 | OB-120 | Frontend | handlePing missing headers + HEAD support | Added Builder-Version, Content-Length headers; skip body for HEAD |

## Files Modified

- `api/types.go` — BUG-222, 223, 224 + Event.Scope field
- `backends/core/handle_extended.go` — BUG-202, 203, 209, 210, 211, 212, 213
- `backends/core/handle_pods.go` — BUG-204, 205, 206
- `backends/core/pod.go` — BUG-206
- `backends/core/drivers_network.go` — BUG-207
- `backends/core/handle_containers.go` — BUG-208
- `backends/docker/exec.go` — BUG-214
- `backends/docker/extended.go` — BUG-215, 217
- `backends/docker/containers.go` — BUG-216, 221
- `backends/docker/images.go` — BUG-217, 220
- `backends/docker/networks.go` — BUG-218
- `backends/docker/client.go` — BUG-219
- `frontends/docker/system.go` — BUG-225, 226

## Verification

- `cd backends/core && go test -race -count=1 ./...` — 302 PASS
- `cd backends/docker && go build ./...` — OK
- `cd frontends/docker && go build ./...` — OK
- All 7 cloud backends: `go build ./...` — OK
- `make lint` — 0 issues
