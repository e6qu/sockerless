# specs/cloud-api/ — vendored cloud API specs

Verbatim snapshots of the official, machine-readable API specifications
for every cloud service the simulators implement. They are the ground
truth the simulators are validated against, so simulator fidelity cannot
silently diverge from what real SDKs/CLIs/providers are generated from.

| Cloud | Format | Upstream | Pin |
|---|---|---|---|
| [`aws/`](aws/SOURCES.md) | Smithy 2.0 JSON (the models the AWS SDKs are generated from) | `aws/aws-sdk-go-v2` `codegen/sdk-codegen/aws-models/` | commit SHA |
| [`gcp/`](gcp/SOURCES.md) | Google API Discovery documents (the source of `google.golang.org/api` clients) | per-service `$discovery/rest` endpoint / central discovery index | document `revision` |
| [`azure/`](azure/SOURCES.md) | Swagger 2.0 (the source of the Azure track-2 SDKs and `go-azure-sdk`) | `Azure/azure-rest-api-specs` | commit SHA |

Every file is gzipped, never edited, and recorded in its directory's
`SOURCES.md` (upstream repo/host, path, license, pin, fetch time). Azure
api-versions follow what the pinned canonical clients actually send (the
SDK modules in `simulators/azure/sdk-tests/go.mod` and
terraform-provider-azurerm's go-azure-sdk imports).

## How the specs are enforced

Two layers, both hermetic (CI never downloads specs):

1. **Static surface conformance** — `simulators/<cloud>/spec_conformance_test.go`
   builds the simulator's full route/operation table in-process
   (`buildSimulator` + the shared lib's pattern recording) and asserts
   every registered operation exists in the vendored spec:
   - AWS: X-Amz-Target values, query-protocol (Version, Action) pairs,
     and REST routes against the Smithy models;
   - GCP: every mux route against the Discovery methods (flatPath/path,
     media upload/download variants);
   - Azure: every mux route against the swagger `paths`/`x-ms-paths`.

   Real-but-unspecified wire surfaces (IMDS, OCI registry data planes,
   LRO polling URLs, resumable-upload sessions, the sim's own `/sim/v1`
   control surface) live in justified allowlists inside the tests —
   never a place to hide an invented path. A reverse check flags
   vendored files no route references (stale vendor).

   These run with the sim module unit tests (`make unit-test`) and gate
   CI hard.

2. **Runtime wire-shape validation (ratcheted)** — when
   `SOCKERLESS_SPEC_VALIDATE=<report.jsonl>` and
   `SOCKERLESS_SPEC_DIR=specs/cloud-api/<cloud>` are set, the simulator
   validates every response body against the spec's output shape
   (members the spec doesn't define, primitive type mismatches) and
   appends violations to the report. The simulator test suites run with
   validation armed; [`scripts/check-spec-violations.sh`](../../scripts/check-spec-violations.sh)
   then fails on any violation not in
   `simulators/<cloud>/spec-violation-allowlist.txt`. Every allowlist
   entry carries a BUG ID — the list only shrinks.

## Refreshing a spec

```sh
scripts/fetch-aws-spec.sh ecs                      # pin to current main
scripts/fetch-gcp-discovery.sh storage.googleapis.com v1
scripts/fetch-azure-spec.sh \
  specification/app/resource-manager/Microsoft.App/ContainerApps/stable/2025-01-01/ContainerApps.json \
  app-arm-containerapps-2025-01-01
```

Each fetch validates the document shape, re-gzips, and rewrites the
file's `SOURCES.md` row. After refreshing, run the simulator unit tests —
the conformance gates tell you exactly which operations moved.

[`scripts/check-spec-freshness.sh`](../../scripts/check-spec-freshness.sh)
reports (never gates) drift between the pins and the upstreams.

## See also

- [`../SIM_SURFACE_TABLES/`](../SIM_SURFACE_TABLES/README.md) — per-service
  operation inventory with handler locations and test coverage.
- [`../SIM_TEST_COVERAGE_MATRIX.md`](../SIM_TEST_COVERAGE_MATRIX.md) — which
  suites cover which services.
- [`simulators/README.md`](../../simulators/README.md) — simulator
  architecture.
