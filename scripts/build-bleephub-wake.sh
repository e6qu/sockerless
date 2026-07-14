#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
out_dir=${1:-"$repo_root/.build/bleephub-ecs"}
mkdir -p "$out_dir"
out_dir=$(cd "$out_dir" && pwd)
go_cache=${GOCACHE:-/private/tmp/sockerless-go-cache}

pushd "$repo_root/bleephub/terraform/wake" >/dev/null
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 GOCACHE="$go_cache" GOWORK=off go build -trimpath -ldflags='-s -w' -o "$out_dir/bootstrap" .
popd >/dev/null

rm -f "$out_dir/bleephub-wake.zip"
(cd "$out_dir" && zip -q -9 bleephub-wake.zip bootstrap)
rm "$out_dir/bootstrap"
printf '%s\n' "$out_dir/bleephub-wake.zip"
