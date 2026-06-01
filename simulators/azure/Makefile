# Standardized Makefile for simulator-azure — see docs/MAKEFILE_STANDARD.md.
#
# This sim is its own Go module (outside the workspace), so all go
# invocations use GOWORK=off via GO_ENV. The default `build` produces
# a stripped, no-UI binary (the historical contract); `make build`
# from outside the workspace's bun install is fully self-contained.

APP_NAME       := simulator-azure
GO_PACKAGE     := .
UI_PACKAGE     := simulator-azure
DEFAULT_PORT   := 4568
GO_ENV         := GOWORK=off CGO_ENABLED=0
GO_LDFLAGS      := -s -w
RUN_ENV        := SIM_LISTEN_ADDR=:4568
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

IMAGE := sockerless-simulator-azure
DOCKER_TEST_IMAGE := sockerless-test-azure
DOCKER_TEST_USER := $(shell id -u):$(shell id -g)
DOCKER_SOCK_GID := $(shell stat -f '%g' /var/run/docker.sock 2>/dev/null || stat -c '%g' /var/run/docker.sock 2>/dev/null || echo 0)
DOCKER_TEST_GROUPS := $(DOCKER_SOCK_GID),0
DOCKER_TEST_ENV := -e HOME=/tmp/sockerless-home -e USER=sockerless -e SOCKERLESS_DOCKER_TEST_UIDGID=$(DOCKER_TEST_USER) -e SOCKERLESS_DOCKER_TEST_GROUPS=$(DOCKER_TEST_GROUPS) -e GOWORK=off -e CGO_ENABLED=0 -e TF_IN_AUTOMATION=1 -e CLOUDSDK_CORE_DISABLE_PROMPTS=1 -e AZURE_CORE_NO_COLOR=true -e AWS_PAGER=
DOCKER_REALNET_TEST_ENV := -e HOME=/tmp/sockerless-home -e USER=sockerless -e GOWORK=off -e CGO_ENABLED=0 -e TF_IN_AUTOMATION=1 -e CLOUDSDK_CORE_DISABLE_PROMPTS=1 -e AZURE_CORE_NO_COLOR=true -e AWS_PAGER=
DOCKER_REALNET_FLAGS := --privileged

docker-build: ## build the docker image
	docker build -t $(IMAGE) .

docker-run: docker-build ## run the docker image
	docker run --rm -p $(DEFAULT_PORT):$(DEFAULT_PORT) \
		-e SIM_LISTEN_ADDR=:$(DEFAULT_PORT) \
		-e SIM_LOG_LEVEL=info \
		$(IMAGE)

docker-test-build: ## build the test docker image
	docker build -t $(DOCKER_TEST_IMAGE) -f $(REPO_ROOT)/simulators/Dockerfile.test $(REPO_ROOT)

docker-test: docker-test-build ## run all tests inside docker
	docker run --rm --network host \
		$(DOCKER_REALNET_FLAGS) \
		--security-opt label=disable \
		--group-add $(DOCKER_SOCK_GID) \
		--group-add 0 \
		-v $(REPO_ROOT):/src \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-w /src/simulators/azure \
		$(DOCKER_REALNET_TEST_ENV) \
		$(DOCKER_TEST_IMAGE) make test-all
