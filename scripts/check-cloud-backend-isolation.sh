#!/usr/bin/env bash
# Verifies that cloud backends are stateless and do not use core engine state.
# Cloud backends must operate exclusively through their cloud provider API.
#
# Forbidden:
#   - BaseServer lifecycle methods (ContainerStart/Stop/Kill/Remove/Restart/Logs/Wait/Attach)
#   - BaseServer query methods (ContainerInspect/List/Top/Update/Stats/Rename/Pause/Unpause)
#   - BaseServer exec methods (ExecCreate)
#   - Store container state methods (StopContainer, ForceStopContainer, RevertToCreated)
#   - Store.Containers writes (Put, Update, Delete)
#   - Store.ContainerNames writes (Put, Delete)
#   - Store.ResolveContainerID / Store.ResolveContainer (use ResolveContainerAuto instead)
#
# Allowed:
#   - Store.WaitChs (ephemeral sync, will be removed)
#   - PendingCreates (transient pre-cloud state)
#   - Backend-specific state stores (ECS, Lambda, etc.)
#   - CloudStateProvider queries
#   - ResolveContainerAuto / ResolveContainerIDAuto
#   - BaseServer methods when guarded by agent address check (ContainerAttach, ContainerTop, etc.)

set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

cloud_backends=(
  backends/ecs
  backends/lambda
  backends/cloudrun
  backends/cloudrun-functions
  backends/aca
  backends/azure-functions
  # Per-cloud shared helper modules run *inside* the cloud backends, so a
  # stateless violation in a shared helper is just as unguarded as one in the
  # backend itself. Scan them too.
  backends/aws-common
  backends/gcp-common
  backends/azure-common
)

forbidden_patterns=(
  # BaseServer lifecycle delegation
  'BaseServer\.ContainerStart'
  'BaseServer\.ContainerStop'
  'BaseServer\.ContainerKill'
  'BaseServer\.ContainerRemove'
  'BaseServer\.ContainerRestart'
  'BaseServer\.ContainerPause'
  'BaseServer\.ContainerUnpause'
  'BaseServer\.ContainerLogs'
  # Cloud backends must query container state through CloudStateProvider paths.
  'BaseServer\.ContainerInspect'
  'BaseServer\.ContainerList'
  # Pod lifecycle: BaseServer.Pod{Start,Stop,Kill,Remove} mutate local Store
  # state and do NO cloud work — delegating them silently leaks the pod's
  # cloud resource (a stateless violation). Cloud backends must override these
  # to loop their own cloud-aware Container* methods.
  'BaseServer\.PodStart'
  'BaseServer\.PodStop'
  'BaseServer\.PodKill'
  'BaseServer\.PodRemove'
  # Store container state methods
  'Store\.StopContainer'
  'Store\.ForceStopContainer'
  'Store\.RevertToCreated'
  # Store.Containers writes (local state mutations)
  'Store\.Containers\.Put'
  'Store\.Containers\.Update'
  'Store\.Containers\.Delete'
  # Store.ContainerNames writes
  'Store\.ContainerNames\.Put'
  'Store\.ContainerNames\.Delete'
  # Direct Store resolution (must use ResolveContainerAuto instead)
  'Store\.ResolveContainerID'
  # Auto-agent (local process spawning — stateless violation)
  'SpawnAutoAgent'
  'StopAutoAgent'
  'Store\.ResolveContainer[^A]'
)

# Patterns allowed when guarded by agent check (file + line must contain AgentAddress)
# These are checked separately — the delegate pattern is OK when properly guarded.
guarded_patterns=(
  'BaseServer\.ContainerTop'
  'BaseServer\.ContainerAttach'
  'BaseServer\.ContainerGetArchive'
  'BaseServer\.ContainerPutArchive'
  'BaseServer\.ContainerStatPath'
  'BaseServer\.ContainerResize'
  'BaseServer\.ExecCreate'
  'BaseServer\.ContainerUpdate'
  'BaseServer\.ContainerWait'
  'BaseServer\.ContainerRename'
  'BaseServer\.ContainerStats'
)

failed=0
for backend in "${cloud_backends[@]}"; do
  for pattern in "${forbidden_patterns[@]}"; do
    # Recurse so each backend's entrypoint (cmd/sockerless-backend-*/main.go)
    # and any nested package are scanned, not just top-level *.go files.
    matches=$(grep -rn "$pattern" "$backend" --include='*.go' 2>/dev/null | grep -v '_test\.go' || true)
    if [ -n "$matches" ]; then
      echo "ERROR: $backend violates stateless rule with '$pattern':"
      echo "$matches"
      echo ""
      failed=1
    fi
  done

  # Reject generated delegate files — all backends must implement methods explicitly
  if [ -f "$backend/backend_delegates_gen.go" ]; then
    echo "ERROR: $backend has generated delegate file backend_delegates_gen.go"
    echo "All backends must implement api.Backend methods explicitly. Delete the gen file."
    echo ""
    failed=1
  fi

  # Check guarded patterns in backend files
  for pattern in "${guarded_patterns[@]}"; do
    matches=$(grep -Hrn "$pattern" "$backend" --include='*.go' 2>/dev/null | grep -v '_test\.go' | grep -vE '^[^:]+:[0-9]+:[[:space:]]*//' || true)
    if [ -n "$matches" ]; then
      # Verify the delegate resolves the container first
      while IFS=: read -r file line_num _; do
        # Check that ResolveContainerIDAuto or ResolveContainerAuto appears nearby.
        start=$((line_num - 20))
        if [ "$start" -lt 1 ]; then
          start=1
        fi
        context=$(sed -n "${start},${line_num}p" "$file" 2>/dev/null || true)
        if ! echo "$context" | grep -q 'ResolveContainer'; then
          echo "ERROR: $backend delegates '$pattern' without container resolution:"
          echo "$file:$line_num"
          echo ""
          failed=1
        fi
      done <<< "$matches"
    fi
  done
done

if [ "$failed" -eq 1 ]; then
  echo "Cloud backends must be stateless. No local container state."
  echo "All operations must go through the cloud API. See AGENTS.md."
  exit 1
fi

echo "check-cloud-backend-isolation: OK"
