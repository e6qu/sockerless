#!/usr/bin/env bash
set -euo pipefail

root="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
workflow="$root/.github/workflows/publish-container-images.yml"
gha='$'

expect_count() {
	local expected="$1" literal="$2" actual
	actual="$(grep -Fxc -- "$literal" "$workflow" || true)"
	if [[ "$actual" != "$expected" ]]; then
		echo "publication workflow expected $expected exact occurrence(s), found $actual: $literal" >&2
		exit 1
	fi
}

expect_count 1 '    branches: [main]'
expect_count 1 '          - { platform: linux/amd64, runner: ubuntu-latest, suffix: amd64 }'
expect_count 1 '          - { platform: linux/arm64, runner: ubuntu-24.04-arm, suffix: arm64 }'
expect_count 1 "          tags: ${gha}{{ env.REGISTRY }}/e6qu/${gha}{{ matrix.image.name }}:${gha}{{ needs.prepare.outputs.short_sha }}-${gha}{{ matrix.arch.suffix }}"
expect_count 1 '          provenance: false'
if [[ "$(grep -Fc "test \"\$MEDIA_TYPE\" = \"application/vnd.oci.image.manifest.v1+json\"" "$workflow")" != 1 ]]; then
	echo 'publication workflow must verify both architecture tags are direct OCI manifests' >&2
	exit 1
fi
if [[ "$(grep -Fc "test \"\$MEDIA_TYPE\" = \"application/vnd.oci.image.index.v1+json\"" "$workflow")" != 1 ]]; then
	echo 'publication workflow must verify the generic tag is an OCI index' >&2
	exit 1
fi
expect_count 1 "        run: ./scripts/prune-ghcr-images.sh \"${gha}{{ github.repository_owner }}\" \"${gha}{{ matrix.image }}\" 20"

for image in sockerless-admin sockerless-simulator-aws sockerless-simulator-gcp sockerless-simulator-azure; do
	count="$(grep -Fc -- "$image" "$workflow")"
	if [[ "$count" != 3 ]]; then
		echo "publication workflow expected $image in build, manifest, and retention matrices, found $count occurrence(s)" >&2
		exit 1
	fi
done

if grep -Eiq 'tags?:[^#]*(:(latest|main))([[:space:]]|$)' "$workflow"; then
	echo 'publication workflow must not publish latest or main image tags' >&2
	exit 1
fi

fixture="$(mktemp)"
trap 'rm -f "$fixture"' EXIT
jq -n '[
	range(0; 22) as $release
	| (("000000000000" + ($release | tostring))[-12:]) as $tag
	| range(0; 3) as $kind
	| {
		id: ($release * 10 + $kind),
		created_at: ("2026-07-" + (("00" + (($release + 1) | tostring))[-2:]) + "T00:00:00Z"),
		metadata: {container: {tags: [
			if $kind == 0 then $tag
			elif $kind == 1 then ($tag + "-amd64")
			else ($tag + "-arm64") end
		]}}
	}
] + [
	{id: 997, created_at: "2026-08-01T00:00:00Z", metadata: {container: {tags: ["feedfacefeed"]}}},
	{id: 998, created_at: "2026-08-02T00:00:00Z", metadata: {container: {tags: ["000000000021", "latest"]}}},
	{id: 999, created_at: "2026-08-03T00:00:00Z", metadata: {container: {tags: []}}}
]' >"$fixture"

selected="$(jq -r --argjson keep 20 -f "$root/scripts/select-obsolete-container-versions.jq" "$fixture" | sort -n | paste -sd, -)"
if [[ "$selected" != '0,1,2,10,11,12,997,998,999' ]]; then
	echo "retention selector chose unexpected package versions: $selected" >&2
	exit 1
fi

echo 'container publication workflow contract passed'
