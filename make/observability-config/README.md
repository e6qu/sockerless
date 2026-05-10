# Observability stack config

Default configs that `make stack-observability-up` feeds to the
collector + Jaeger + VictoriaLogs binaries. All paths use environment
variables that the make target sets so the configs work regardless of
where the repo lives on disk:

- `SOCKERLESS_STATE_PIDS_DIR` — `<repo>/.stack-pids` (filelog source).
- `SOCKERLESS_OBSERVABILITY_DIR` — `<repo>/.sockerless-state/observability/`
  (Jaeger badger + VictoriaLogs storage).

Operators can drop their own `*.yaml` next to these and point
`STACK_OBSERVABILITY_CONFIG_DIR` at it; the make target picks up the
override location wholesale.
