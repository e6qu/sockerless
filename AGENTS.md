# Agent Guidelines

## Simulators are real implementations

The cloud simulators (`simulators/aws/`, `simulators/gcp/`, `simulators/azure/`) are **local reimplementations** of cloud provider services, not mocks, stubs, or fakes. They execute real logic:

- Jobs run. Functions execute. Timeouts fire. Logs are produced.
- Execution behavior is driven by the same cloud-native configuration that the real services honor — `replicaTimeout` for Azure ACA, task template `timeout` for GCP Cloud Run, `StopTask` for AWS ECS.
- There are no synthetic timers, hardcoded delays, or fake completion signals. If a cloud service doesn't have a native timeout mechanism (e.g., ECS tasks), neither does the simulator.
- Log entries are written to the same tables and log groups as the real services, queryable through the same APIs (KQL, Cloud Logging filters, CloudWatch).

When modifying simulators, always ask: "How does the real cloud service behave?" and implement that. Do not add simulator-specific environment variables, synthetic shortcuts, or approximate behaviors. Use the cloud's own configuration knobs.

The simulators run locally on a single machine today. The architecture is designed to eventually distribute execution across multiple machines, with the same API surface.

## Never merge PRs

Create PRs with `gh pr create`. Never run `gh pr merge`. The user handles all merges.
