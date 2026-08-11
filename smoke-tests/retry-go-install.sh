#!/bin/sh
# Resilient `go install <module>@<version>` for test-harness images, the
# module-at-version counterpart of retry-go-build. The Docker builds fetch
# every module fresh from proxy.golang.org (the setup-go cache the other CI
# jobs use is not available inside a docker build), so a single transient
# proxy failure would otherwise fail the whole image build. Retrying the
# install is safe: once the module cache is populated the recompile is
# deterministic and hits no network.
#
# Usage: retry-go-install <module@version>
# Honours GOBIN for the output location, like `go install` itself.
set -eu

pkg=$1

n=0
until go install -tags noui "$pkg"; do
	n=$((n + 1))
	if [ "$n" -ge 5 ]; then
		echo "retry-go-install: go install $pkg failed after $n attempts" >&2
		exit 1
	fi
	echo "retry-go-install: go install attempt $n failed; retrying in $((n * 5))s" >&2
	sleep $((n * 5))
done
