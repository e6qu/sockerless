#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../bleephub"

# Docker-backed Codespaces lifecycle tests are exercised by CI while local
# Docker compatibility is temporarily unavailable. Keep the local hook focused
# on the rest of the Bleephub Go suite instead of requiring a host Docker socket
# for every commit.
docker_backed='Test(LiveCodespaces_UserAndRepo|Codespaces_UserCreateListGetDelete|Codespaces_RepoCreateStartStopDelete|Codespaces_OrgMemberAdministration|Codespaces_OrgList|CodespacesCreateForPullRequest|PersistenceReload_CodespacesAndSecrets)$'

go test -tags noui ./... -count=1 -timeout 300s -skip "$docker_backed"
