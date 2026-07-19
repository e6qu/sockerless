#!/usr/bin/env bash
set -euo pipefail

smoke_dockerfiles=(
	smoke-tests/Dockerfile.aca
	smoke-tests/Dockerfile.cloudrun
	smoke-tests/Dockerfile.ecs
	smoke-tests/gitlab/Dockerfile.backend
	smoke-tests/gitlab/Dockerfile.backend-aca
	smoke-tests/gitlab/Dockerfile.backend-cloudrun
	smoke-tests/gitlab/Dockerfile.backend-ecs
)

gitlab_dockerfiles=(
	smoke-tests/gitlab/Dockerfile.backend
	smoke-tests/gitlab/Dockerfile.backend-aca
	smoke-tests/gitlab/Dockerfile.backend-cloudrun
	smoke-tests/gitlab/Dockerfile.backend-ecs
)

failed=0

require_line() {
	file=$1
	line=$2
	reason=$3

	if grep -Fqx "$line" "$file"; then
		return
	fi

	printf '%s: missing %s (%s)\n' "$file" "$line" "$reason" >&2
	failed=1
}

for file in "${smoke_dockerfiles[@]}"; do
	require_line "$file" 'COPY simulators/ui-auth/ /ui-auth/' 'standalone simulator OpenID Connect module'
done

for file in "${gitlab_dockerfiles[@]}"; do
	require_line "$file" 'COPY agent/ agent/' 'standalone cloud backend agent module'
	if ! grep -Eq '^RUN GOWORK=off go build -tags noui -o /simulator-(aws|gcp|azure) \.$' "$file"; then
		printf '%s: simulator build must use the intentional noui build tag\n' "$file" >&2
		failed=1
	fi
done

if [ "$failed" -ne 0 ]; then
	exit 1
fi

echo 'Smoke Docker build contexts include every shared local module.'
