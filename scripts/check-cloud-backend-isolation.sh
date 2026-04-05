#!/usr/bin/env bash
# Verifies that cloud backends are stateless and do not use core engine state.
# Cloud backends must operate exclusively through their cloud provider API.
#
# Forbidden:
#   - BaseServer lifecycle methods (ContainerStart/Stop/Kill/Remove/Restart)
#   - Store container state methods (StopContainer, ForceStopContainer, RevertToCreated)
#   - Store.Containers writes (Put, Update, Delete)
#   - Store.ContainerNames writes (Put, Delete)
#
# Allowed:
#   - Store.WaitChs (ephemeral sync, will be removed)
#   - PendingCreates (transient pre-cloud state)
#   - Backend-specific state stores (ECS, Lambda, etc.)
#   - CloudStateProvider queries

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
done

if [ "$failed" -eq 1 ]; then
  echo "Cloud backends must be stateless. No local container state."
  echo "All operations must go through the cloud API. See AGENTS.md."
  exit 1
fi

echo "check-cloud-backend-isolation: OK"
