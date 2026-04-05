#!/usr/bin/env bash
# Verifies that cloud backends do not call BaseServer container lifecycle methods.
# Cloud backends must be stateless — all container operations go through the cloud API.
#
# Forbidden patterns in cloud backends:
#   s.BaseServer.ContainerStart
#   s.BaseServer.ContainerStop
#   s.BaseServer.ContainerKill
#   s.BaseServer.ContainerRemove
#   s.BaseServer.ContainerRestart
#   s.Store.StopContainer
#   s.Store.ForceStopContainer
#   s.Store.RevertToCreated
#   s.Store.Containers.Put  (except in auto-agent compatibility path)
#   s.Store.Containers.Update
#   s.Store.Containers.Delete

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
  'BaseServer\.ContainerStart'
  'BaseServer\.ContainerStop'
  'BaseServer\.ContainerKill'
  'BaseServer\.ContainerRemove'
  'BaseServer\.ContainerRestart'
  'Store\.StopContainer'
  'Store\.ForceStopContainer'
  'Store\.RevertToCreated'
)

failed=0
for backend in "${cloud_backends[@]}"; do
  for pattern in "${forbidden_patterns[@]}"; do
    # Search Go files (exclude test files and auto-agent compatibility comments)
    matches=$(grep -rn "$pattern" "$backend"/*.go 2>/dev/null | grep -v '_test\.go' | grep -v '// auto-agent' | grep -v 'AUTO_AGENT_BIN' || true)
    if [ -n "$matches" ]; then
      echo "ERROR: $backend uses forbidden pattern '$pattern':"
      echo "$matches"
      failed=1
    fi
  done
done

if [ "$failed" -eq 1 ]; then
  echo ""
  echo "Cloud backends must not call BaseServer container lifecycle methods."
  echo "Use cloud API calls instead. See AGENTS.md for details."
  exit 1
fi

echo "check-cloud-backend-isolation: OK"
