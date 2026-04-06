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
  'BaseServer\.ContainerInspect'
  'BaseServer\.ContainerList'
  'BaseServer\.ContainerWait'
  'BaseServer\.ContainerRename'
  'BaseServer\.ContainerStats'
)

failed=0
for backend in "${cloud_backends[@]}"; do
  for pattern in "${forbidden_patterns[@]}"; do
    matches=$(grep -rn "$pattern" "$backend"/*.go 2>/dev/null | grep -v '_test\.go' || true)
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

  # Check guarded patterns in delegate files
  for pattern in "${guarded_patterns[@]}"; do
    matches=$(grep -rn "$pattern" "$backend"/backend_delegates.go 2>/dev/null || true)
    if [ -n "$matches" ]; then
      # Verify the delegate resolves the container first
      for line_num in $(echo "$matches" | grep -oP '^\S+:\K\d+'); do
        # Check that ResolveContainerIDAuto or ResolveContainerAuto appears nearby
        context=$(sed -n "$((line_num-5)),$((line_num))p" "$backend"/backend_delegates.go 2>/dev/null || true)
        if ! echo "$context" | grep -q 'ResolveContainer'; then
          echo "WARNING: $backend delegates '$pattern' without container resolution:"
          echo "$matches"
          echo ""
        fi
      done
    fi
  done
done

if [ "$failed" -eq 1 ]; then
  echo "Cloud backends must be stateless. No local container state."
  echo "All operations must go through the cloud API. See AGENTS.md."
  exit 1
fi

echo "check-cloud-backend-isolation: OK"
