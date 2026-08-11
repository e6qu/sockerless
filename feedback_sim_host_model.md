# Simulator Host Model

The simulator host model has two execution paths:

- Container and FaaS workloads run through Docker/Podman containers via the
  shared simulator container runtime.
- VM-level resources use the real-execution substrate defined in
  [SIMULATOR_REAL_EXECUTION.md](https://github.com/e6qu/sockerless-cloud/blob/main/specs/SIMULATOR_REAL_EXECUTION.md) (sockerless-cloud).

Production simulator handlers must not run user workloads as host processes.
`os/exec` is allowed only for test harnesses, explicitly allowlisted simulator
tooling, and the future real-execution substrate launcher/plumbing where the
host command is the real implementation object such as Firecracker, netns,
netlink, nftables, or a load-balancer proxy.

Missing host capabilities must fail loudly. They must not fall back to
metadata-only success.
