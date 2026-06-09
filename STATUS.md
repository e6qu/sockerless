# Sockerless - Status

Roadmap [PLAN.md](PLAN.md) - resume [DO_NEXT.md](DO_NEXT.md) - bugs [BUGS.md](BUGS.md) - narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) - coverage [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md).

## Snapshot

| | |
|---|---|
| Active branch | `bundled-docs-bigtable-execution` |
| In-flight | This branch added GCP Bigtable Terraform provider coverage through the provider's official Bigtable gRPC emulator path, replaced synthetic AWS Batch/CodeBuild/Glue/Step Functions execution success with real workload/process/ASL execution, and refreshed continuity + coverage docs. |
| Last merged | The Azure/GCP service-slice and AWS coverage-audit PR was merged before this branch started. |
| Open GitHub issues | #394 remained upstream-blocked. Re-check GitHub before choosing the next issue batch. |
| Bugs | 1589 filed - 1544 fixed - 6 open - 5 false positives. |
| Open BUGs | BUG-1075 live-cloud validation; BUG-1104 audit cadence; BUG-1345 azuread upstream; BUG-1584 AzureStack provider deprecation warning despite `metadata_host`. |
| Live infra | None up. |

## Invariants

- Never auto-merge PRs; the user handles merges.
- Rebase PR branches on `origin/main` before pushing.
- File concrete BUG entries before fixing discovered defects.
- No stubs, fakes, mocks, synthetic responses, silent fallbacks, or degraded modes.
- Simulators implement real cloud API slices in one binary per cloud.
- Every simulator public endpoint ships with official SDK, vendor CLI, and Terraform coverage where those surfaces exist.
- Coverage authorities are [specs/SIM_TEST_COVERAGE_MATRIX.md](specs/SIM_TEST_COVERAGE_MATRIX.md) and [specs/SIM_SURFACE_TABLES](specs/SIM_SURFACE_TABLES).

## Environment Notes

- AzureRM custom metadata discovery still needs HTTPS through the local Caddy gateway: `make stack-https-{up,status,ca,down}`.
- AWS and GCP Terraform providers accept localhost custom endpoints directly.
- Simulator ports: AWS 4566, GCP 4567, Azure 4568.
- Linux network-fabric tests require `CAP_NET_ADMIN`, `iproute2`, and nftables; off-Linux tests skip through the realexec capability gate.
