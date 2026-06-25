#!/bin/sh
# Resilient Go build for the smoke-test images. The smoke Docker builds fetch
# every module fresh from proxy.golang.org (the setup-go cache the other CI jobs
# use is not available inside a docker build), so a single transient proxy
# failure — "stream error ... INTERNAL_ERROR; received from peer" mid-zip —
# would otherwise fail the whole image build and the smoke job with it. Retry
# the network step (go mod download) with backoff, then run the deterministic
# build once (it hits no network with the module cache already populated).
#
# Usage: retry-go-build <output-path> [package]
set -eu

out=$1
pkg=${2:--.}
[ "$pkg" = "-." ] && pkg=.

n=0
until GOWORK=off go mod download; do
	n=$((n + 1))
	if [ "$n" -ge 5 ]; then
		echo "retry-go-build: go mod download failed after $n attempts" >&2
		exit 1
	fi
	echo "retry-go-build: go mod download attempt $n failed; retrying in $((n * 5))s" >&2
	sleep $((n * 5))
done

exec env GOWORK=off go build -tags noui -o "$out" "$pkg"
