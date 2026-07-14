#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
out_dir=${1:-"$repo_root/.build/bleephub-ecs"}
version=${BLEEPHUB_VERSION:-development}
published_at=${BLEEPHUB_PUBLISHED_AT:-not-yet-published}

if [[ ! "$version" =~ ^[0-9a-zA-Z._-]+$ ]]; then
  echo "BLEEPHUB_VERSION must contain only letters, digits, dots, underscores, or hyphens" >&2
  exit 1
fi
if [[ ! "$published_at" =~ ^[0-9TZ:.-]+$|^not-yet-published$ ]]; then
  echo "BLEEPHUB_PUBLISHED_AT must be an RFC 3339 UTC timestamp or not-yet-published" >&2
  exit 1
fi

mkdir -p "$out_dir/startup"
out_dir=$(cd "$out_dir" && pwd)
sed -e "s/__BLEEPHUB_VERSION__/$version/g" -e "s/__BLEEPHUB_PUBLISHED_AT__/$published_at/g" \
  "$repo_root/bleephub/terraform/startup/index.html" > "$out_dir/startup/index.html"
rm -f "$out_dir/bleephub-startup.zip"
(cd "$out_dir/startup" && zip -q -9 ../bleephub-startup.zip index.html)
printf '%s\n%s\n' "$out_dir/startup/index.html" "$out_dir/bleephub-startup.zip"
