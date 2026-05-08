# Cross-backend e2e tests. Own Go module, lives outside the workspace.
# Tests are guarded by build tags; `make test` runs the default
# (build-tag-free) subset, `make test-integration` runs the rest.

GO_ENV := GOWORK=off

REPO_ROOT_REL := ..

include $(REPO_ROOT_REL)/make/go-lib.mk
