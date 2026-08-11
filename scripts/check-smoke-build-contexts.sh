#!/usr/bin/env bash
set -euo pipefail

# The smoke and gitlab-runner harness images consume the simulators from the
# sockerless-cloud repository as pinned modules. Guard the two halves of that
# contract: every image installs its simulator at the pinned
# SOCKERLESS_CLOUD_VERSION with the intentional noui build tag, and the gitlab
# images still copy the shared local agent module their backend builds need.

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
	if ! grep -Fq 'ARG SOCKERLESS_CLOUD_VERSION=' "$file"; then
		printf '%s: missing ARG SOCKERLESS_CLOUD_VERSION pin\n' "$file" >&2
		failed=1
	fi
	if ! grep -Eq 'go install( -tags noui)? github\.com/e6qu/sockerless-cloud/simulator-(aws|gcp|azure)@\$\{SOCKERLESS_CLOUD_VERSION\}' "$file" &&
		! grep -Eq 'retry-go-install github\.com/e6qu/sockerless-cloud/simulator-(aws|gcp|azure)@\$\{SOCKERLESS_CLOUD_VERSION\}' "$file"; then
		printf '%s: simulator must be installed from the pinned sockerless-cloud module\n' "$file" >&2
		failed=1
	fi
done

for file in "${gitlab_dockerfiles[@]}"; do
	require_line "$file" 'COPY agent/ agent/' 'standalone cloud backend agent module'
	if ! grep -Eq '^RUN GOBIN=\S+ go install -tags noui github\.com/e6qu/sockerless-cloud/simulator-(aws|gcp|azure)@\$\{SOCKERLESS_CLOUD_VERSION\}$' "$file"; then
		printf '%s: simulator install must use the intentional noui build tag\n' "$file" >&2
		failed=1
	fi
done

if [ "$failed" -ne 0 ]; then
	exit 1
fi

echo 'Smoke Docker build contexts install the pinned simulators and include every shared local module.'
