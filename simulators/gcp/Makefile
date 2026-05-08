# Standardized Makefile for simulator-gcp — see docs/MAKEFILE_STANDARD.md.
#
# This sim is its own Go module (outside the workspace), so all go
# invocations use GOWORK=off via GO_ENV. The default `build` produces
# a stripped, no-UI binary (the historical contract); `make build`
# from outside the workspace's bun install is fully self-contained.

APP_NAME       := simulator-gcp
GO_PACKAGE     := .
UI_PACKAGE     := simulator-gcp
DEFAULT_PORT   := 4567
GO_ENV         := GOWORK=off CGO_ENABLED=0
GO_LDFLAGS      := -s -w
RUN_ENV        := SIM_LISTEN_ADDR=:4567
RUN_FLAGS      :=

REPO_ROOT_REL  := ../..

include $(REPO_ROOT_REL)/make/go-app.mk

# ── Domain-specific test targets ────────────────────────────────────
# Each sub-tree (sdk-tests, cli-tests, terraform-tests, shared) is a
# separate Go module; the CI calls these directly.

.PHONY: shared-test sdk-test cli-test terraform-test

shared-test: ## go test the shared/ helper module
	cd shared && $(GO_ENV) go test -v -count=1 ./...

sdk-test: build ## go test the SDK-driven sim tests
	cd sdk-tests && $(GO_ENV) go test -v -count=1 ./...

cli-test: build ## go test the CLI-driven sim tests
	cd cli-tests && $(GO_ENV) go test -v -count=1 -timeout 120s ./...

terraform-test: build ## go test the terraform-driven sim tests
	cd terraform-tests && $(GO_ENV) go test -v -count=1 -timeout 300s ./...

# Aggregate that runs all four legacy test categories.
.PHONY: test-all
test-all: shared-test sdk-test cli-test terraform-test ## run every test category

# ── Docker targets ──────────────────────────────────────────────────
.PHONY: docker-build docker-run docker-test docker-test-build

IMAGE := sockerless-simulator-gcp

docker-build: ## build the docker image
	docker build -t $(IMAGE) .

docker-run: docker-build ## run the docker image
	docker run --rm -p $(DEFAULT_PORT):$(DEFAULT_PORT) \
		-e SIM_LISTEN_ADDR=:$(DEFAULT_PORT) \
		-e SIM_LOG_LEVEL=info \
		$(IMAGE)

docker-test-build: ## build the test docker image
	docker build -t sockerless-test-gcp -f Dockerfile.test .

docker-test: docker-test-build ## run all tests inside docker
	docker run --rm -v $(CURDIR):/src -w /src sockerless-test-gcp make test-all
