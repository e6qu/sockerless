#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

[ "$(uname -s)" = "Linux" ] || fail "real-execution network/NIC test requires Linux"

need_cmd firecracker
need_cmd jailer
need_cmd go
need_cmd ip
need_cmd nft
need_cmd ping
need_cmd sudo

[ -e /dev/kvm ] || fail "real-execution capability test requires /dev/kvm"
if ! { [ -r /dev/kvm ] && [ -w /dev/kvm ]; }; then
  sudo test -r /dev/kvm && sudo test -w /dev/kvm || fail "real-execution capability test requires read/write access to /dev/kvm"
fi

repo_root="$(git rev-parse --show-toplevel)"
cache_dir="$repo_root/.gocache/realexec-root"
mkdir -p "$cache_dir"

cd "$repo_root/simulators/realexec"
sudo env "PATH=$PATH" "GOCACHE=$cache_dir" "HOME=${HOME:-/root}" GOWORK=off go test -tags realexec_host -v -count=1 ./...
