.PHONY: sim-test-ecs sim-test-lambda sim-test-cloudrun sim-test-gcf sim-test-aca sim-test-azf
.PHONY: sim-test-aws sim-test-gcp sim-test-azure sim-test-all
.PHONY: test test-unit test-e2e lint check-backend-coverage
.PHONY: test-agent test-core test-bleephub test-gitlabhub
.PHONY: bleephub-test bleephub-gh-test gitlabhub-test
.PHONY: smoke-test-act smoke-test-act-ecs smoke-test-act-cloudrun smoke-test-act-aca smoke-test-act-all
.PHONY: smoke-test-gitlab smoke-test-gitlab-ecs smoke-test-gitlab-cloudrun smoke-test-gitlab-aca smoke-test-gitlab-all
.PHONY: e2e-github-all e2e-gitlab-all e2e-all
.PHONY: e2e-github-build-aws e2e-github-build-gcp e2e-github-build-azure
.PHONY: e2e-github-ecs e2e-github-lambda
.PHONY: e2e-github-cloudrun e2e-github-gcf e2e-github-aca e2e-github-azf
.PHONY: upstream-test-act-build-aws upstream-test-act-build-gcp upstream-test-act-build-azure
.PHONY: upstream-test-act upstream-test-act-individual upstream-test-act-ecs upstream-test-act-lambda upstream-test-act-all
.PHONY: upstream-test-act-cloudrun upstream-test-act-gcf upstream-test-act-aca upstream-test-act-azf
.PHONY: upstream-test-gitlab-ci-local
.PHONY: upstream-test-gcl-build-aws upstream-test-gcl-build-gcp upstream-test-gcl-build-azure
.PHONY: upstream-test-gcl-ecs upstream-test-gcl-lambda
.PHONY: upstream-test-gcl-cloudrun upstream-test-gcl-gcf upstream-test-gcl-aca upstream-test-gcl-azf
.PHONY: upstream-test-gcl-all

# Per-module unit test targets
test-agent:
	@echo "=== test agent ==="
	cd agent && go test -v -race -timeout 2m ./...

test-core:
	@echo "=== test backends/core ==="
	cd backends/core && go test -v -timeout 2m ./...

test-bleephub:
	@echo "=== test bleephub ==="
	cd bleephub && go test -tags noui -v -timeout 3m ./...

test-gitlabhub:
	@echo "=== test gitlabhub ==="
	cd gitlabhub && go test -tags noui -v -timeout 3m ./...

# E2E integration tests (builds + starts backend/frontend/agent binaries)
test-e2e:
	@echo "=== test e2e ==="
	cd tests && go test -v -timeout 5m ./...

# All unit tests (per-module)
test-unit: test-agent test-core test-bleephub test-gitlabhub

# All tests (unit + e2e)
test: test-unit test-e2e

# Lint all Go modules (golangci-lint required)
# Modules without UI embed
MODULES = api agent backends/core
# Modules with UI embed (require --build-tags noui when dist/ is absent)
MODULES_UI = backends/docker \
  backends/ecs backends/lambda backends/cloudrun \
  backends/cloudrun-functions backends/aca backends/azure-functions \
  cmd/sockerless-admin bleephub gitlabhub
# Simulator modules with UI embed (separate go.mod, need GOWORK=off)
MODULES_SIM_UI = simulators/aws simulators/gcp simulators/azure

lint:
	@for mod in $(MODULES); do \
	    echo "=== lint $$mod ===" && \
	    cd $(CURDIR)/$$mod && golangci-lint run ./... || exit 1; \
	done
	@for mod in $(MODULES_UI); do \
	    echo "=== lint $$mod ===" && \
	    cd $(CURDIR)/$$mod && golangci-lint run --build-tags noui ./... || exit 1; \
	done
	@for mod in $(MODULES_SIM_UI); do \
	    echo "=== lint $$mod ===" && \
	    cd $(CURDIR)/$$mod && GOWORK=off golangci-lint run --build-tags noui ./... || exit 1; \
	done

# Check that all backends explicitly implement every api.Backend method
# Use --enforce to fail on missing methods (enabled once all backends are complete)
check-backend-coverage:
	@cd tools/check-backend-coverage && GOWORK=off go run .

check-backend-coverage-enforce:
	@cd tools/check-backend-coverage && GOWORK=off go run . --enforce

# Simulator integration tests — individual backends (per-module)
sim-test-ecs:
	cd backends/ecs && $(MAKE) integration-test

sim-test-lambda:
	cd backends/lambda && $(MAKE) integration-test

sim-test-cloudrun:
	cd backends/cloudrun && $(MAKE) integration-test

sim-test-gcf:
	cd backends/cloudrun-functions && $(MAKE) integration-test

sim-test-aca:
	cd backends/aca && $(MAKE) integration-test

sim-test-azf:
	cd backends/azure-functions && $(MAKE) integration-test

# Simulator integration tests — per cloud
sim-test-aws: sim-test-ecs sim-test-lambda

sim-test-gcp: sim-test-cloudrun sim-test-gcf

sim-test-azure: sim-test-aca sim-test-azf

# Simulator integration tests — all backends
sim-test-all: sim-test-aws sim-test-gcp sim-test-azure

# Smoke tests — act (GitHub Actions runner)
smoke-test-act:
	docker build -t sockerless-smoke-act -f smoke-tests/act/Dockerfile.ecs .
	docker run --rm sockerless-smoke-act

smoke-test-act-ecs:
	docker build -t sockerless-smoke-act-ecs -f smoke-tests/act/Dockerfile.ecs .
	docker run --rm sockerless-smoke-act-ecs

smoke-test-act-cloudrun:
	docker build -t sockerless-smoke-act-cloudrun -f smoke-tests/act/Dockerfile.cloudrun .
	docker run --rm sockerless-smoke-act-cloudrun

smoke-test-act-aca:
	docker build -t sockerless-smoke-act-aca -f smoke-tests/act/Dockerfile.aca .
	docker run --rm sockerless-smoke-act-aca

smoke-test-act-all: smoke-test-act smoke-test-act-ecs smoke-test-act-cloudrun smoke-test-act-aca

# Smoke tests — GitLab Runner
smoke-test-gitlab:
	cd smoke-tests/gitlab && docker compose down -v 2>/dev/null; BACKEND=ecs docker compose up --build --abort-on-container-exit --exit-code-from orchestrator

smoke-test-gitlab-ecs:
	cd smoke-tests/gitlab && docker compose -f docker-compose.yml -f docker-compose.ecs.yml down -v 2>/dev/null; BACKEND=ecs docker compose -f docker-compose.yml -f docker-compose.ecs.yml up --build --abort-on-container-exit --exit-code-from orchestrator

smoke-test-gitlab-cloudrun:
	cd smoke-tests/gitlab && docker compose -f docker-compose.yml -f docker-compose.cloudrun.yml down -v 2>/dev/null; BACKEND=cloudrun docker compose -f docker-compose.yml -f docker-compose.cloudrun.yml up --build --abort-on-container-exit --exit-code-from orchestrator

smoke-test-gitlab-aca:
	cd smoke-tests/gitlab && docker compose -f docker-compose.yml -f docker-compose.aca.yml down -v 2>/dev/null; BACKEND=aca docker compose -f docker-compose.yml -f docker-compose.aca.yml up --build --abort-on-container-exit --exit-code-from orchestrator

smoke-test-gitlab-all: smoke-test-gitlab smoke-test-gitlab-ecs smoke-test-gitlab-cloudrun smoke-test-gitlab-aca

# Terraform integration tests — full modules against simulators (Docker-only)
.PHONY: tf-int-test-ecs tf-int-test-lambda tf-int-test-cloudrun tf-int-test-gcf tf-int-test-aca tf-int-test-azf
.PHONY: tf-int-test-aws tf-int-test-gcp tf-int-test-azure tf-int-test-all

TF_INT_IMAGE = sockerless-tf-int

tf-int-build:
	docker build -t $(TF_INT_IMAGE) -f tests/terraform-integration/Dockerfile .

# Individual backends
tf-int-test-ecs: tf-int-build
	docker run --rm -e SKIP_SMOKE_TEST=1 $(TF_INT_IMAGE) ecs

tf-int-test-lambda: tf-int-build
	docker run --rm -e SKIP_SMOKE_TEST=1 $(TF_INT_IMAGE) lambda

tf-int-test-cloudrun: tf-int-build
	docker run --rm -e SKIP_SMOKE_TEST=1 $(TF_INT_IMAGE) cloudrun

tf-int-test-gcf: tf-int-build
	docker run --rm -e SKIP_SMOKE_TEST=1 $(TF_INT_IMAGE) gcf

tf-int-test-aca: tf-int-build
	docker run --rm -e SKIP_SMOKE_TEST=1 $(TF_INT_IMAGE) aca

tf-int-test-azf: tf-int-build
	docker run --rm -e SKIP_SMOKE_TEST=1 $(TF_INT_IMAGE) azf

# Per cloud (both backends, shared Docker build)
tf-int-test-aws: tf-int-build
	docker run --rm -e SKIP_SMOKE_TEST=1 $(TF_INT_IMAGE) ecs
	docker run --rm -e SKIP_SMOKE_TEST=1 $(TF_INT_IMAGE) lambda

tf-int-test-gcp: tf-int-build
	docker run --rm -e SKIP_SMOKE_TEST=1 $(TF_INT_IMAGE) cloudrun
	docker run --rm -e SKIP_SMOKE_TEST=1 $(TF_INT_IMAGE) gcf

tf-int-test-azure: tf-int-build
	docker run --rm -e SKIP_SMOKE_TEST=1 $(TF_INT_IMAGE) aca
	docker run --rm -e SKIP_SMOKE_TEST=1 $(TF_INT_IMAGE) azf

# All backends
tf-int-test-all: tf-int-test-aws tf-int-test-gcp tf-int-test-azure

# E2E live tests — GitHub Actions runner (via act), per-cloud images
E2E_GITHUB_IMAGE = sockerless-e2e-github

e2e-github-build-aws:
	docker build -t $(E2E_GITHUB_IMAGE)-aws -f tests/e2e-live-tests/github-runner/Dockerfile.aws .

e2e-github-build-gcp:
	docker build -t $(E2E_GITHUB_IMAGE)-gcp -f tests/e2e-live-tests/github-runner/Dockerfile.gcp .

e2e-github-build-azure:
	docker build -t $(E2E_GITHUB_IMAGE)-azure -f tests/e2e-live-tests/github-runner/Dockerfile.azure .

e2e-github-ecs: e2e-github-build-aws
	docker run --rm -e BACKEND=ecs $(E2E_GITHUB_IMAGE)-aws --backend ecs

e2e-github-lambda: e2e-github-build-aws
	docker run --rm -e BACKEND=lambda $(E2E_GITHUB_IMAGE)-aws --backend lambda

e2e-github-cloudrun: e2e-github-build-gcp
	docker run --rm -e BACKEND=cloudrun $(E2E_GITHUB_IMAGE)-gcp --backend cloudrun

e2e-github-gcf: e2e-github-build-gcp
	docker run --rm -e BACKEND=gcf $(E2E_GITHUB_IMAGE)-gcp --backend gcf

e2e-github-aca: e2e-github-build-azure
	docker run --rm -e BACKEND=aca $(E2E_GITHUB_IMAGE)-azure --backend aca

e2e-github-azf: e2e-github-build-azure
	docker run --rm -e BACKEND=azf $(E2E_GITHUB_IMAGE)-azure --backend azf

e2e-github-all:
	@for b in ecs lambda cloudrun gcf aca azf; do \
	    echo "=== E2E GitHub: $$b ===" && \
	    $(MAKE) e2e-github-$$b || exit 1; \
	done

# E2E live tests — GitLab Runner (docker-executor)
e2e-gitlab-%:
	cd tests/e2e-live-tests/gitlab-runner-docker && ./run.sh --backend $*

e2e-gitlab-all:
	@for b in ecs lambda cloudrun gcf aca azf; do \
	    echo "=== E2E GitLab: $$b ===" && \
	    $(MAKE) e2e-gitlab-$$b || exit 1; \
	done

# E2E all
e2e-all: e2e-github-all e2e-gitlab-all

# Upstream test suites — act, per-cloud images
UPSTREAM_ACT_IMAGE = sockerless-upstream-act

upstream-test-act-build-aws:
	docker build -t $(UPSTREAM_ACT_IMAGE)-aws -f tests/upstream/act/Dockerfile.aws .

upstream-test-act-build-gcp:
	docker build -t $(UPSTREAM_ACT_IMAGE)-gcp -f tests/upstream/act/Dockerfile.gcp .

upstream-test-act-build-azure:
	docker build -t $(UPSTREAM_ACT_IMAGE)-azure -f tests/upstream/act/Dockerfile.azure .

upstream-test-act: upstream-test-act-build-aws
	docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-aws --backend ecs

upstream-test-act-individual: upstream-test-act-build-aws
	docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-aws --backend ecs --individual

upstream-test-act-ecs: upstream-test-act-build-aws
	docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-aws --backend ecs

upstream-test-act-lambda: upstream-test-act-build-aws
	docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-aws --backend lambda

upstream-test-act-cloudrun: upstream-test-act-build-gcp
	docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-gcp --backend cloudrun

upstream-test-act-gcf: upstream-test-act-build-gcp
	docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-gcp --backend gcf

upstream-test-act-aca: upstream-test-act-build-azure
	docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-azure --backend aca

upstream-test-act-azf: upstream-test-act-build-azure
	docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-azure --backend azf

upstream-test-act-all:
	@for b in ecs lambda cloudrun gcf aca azf; do \
	    echo "=== Upstream Act: $$b ===" && \
	    $(MAKE) upstream-test-act-$$b || true; \
	done

# Upstream test suites — gitlab-ci-local, per-cloud images
UPSTREAM_GCL_IMAGE = sockerless-upstream-gcl

upstream-test-gcl-build-aws:
	docker build -t $(UPSTREAM_GCL_IMAGE)-aws -f tests/upstream/gitlab-ci-local/Dockerfile.aws .

upstream-test-gcl-build-gcp:
	docker build -t $(UPSTREAM_GCL_IMAGE)-gcp -f tests/upstream/gitlab-ci-local/Dockerfile.gcp .

upstream-test-gcl-build-azure:
	docker build -t $(UPSTREAM_GCL_IMAGE)-azure -f tests/upstream/gitlab-ci-local/Dockerfile.azure .

upstream-test-gitlab-ci-local: upstream-test-gcl-build-aws
	docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-aws --backend ecs

upstream-test-gcl-ecs: upstream-test-gcl-build-aws
	docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-aws --backend ecs

upstream-test-gcl-lambda: upstream-test-gcl-build-aws
	docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-aws --backend lambda

upstream-test-gcl-cloudrun: upstream-test-gcl-build-gcp
	docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-gcp --backend cloudrun

upstream-test-gcl-gcf: upstream-test-gcl-build-gcp
	docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-gcp --backend gcf

upstream-test-gcl-aca: upstream-test-gcl-build-azure
	docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-azure --backend aca

upstream-test-gcl-azf: upstream-test-gcl-build-azure
	docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-azure --backend azf

upstream-test-gcl-all:
	@for b in ecs lambda cloudrun gcf aca azf; do \
	    echo "=== Upstream GCL: $$b ===" && \
	    $(MAKE) upstream-test-gcl-$$b || true; \
	done

# bleephub — GitHub Actions runner server integration test (Docker-only)
bleephub-test:
	docker build -f bleephub/Dockerfile -t sockerless-bleephub-test .
	docker run --rm sockerless-bleephub-test

bleephub-gh-test:
	docker build -f bleephub/Dockerfile.gh-test -t sockerless-bleephub-gh-test .
	docker run --rm sockerless-bleephub-gh-test

# gitlabhub — GitLab CI runner server integration test (Docker-only)
gitlabhub-test:
	docker build -f gitlabhub/Dockerfile -t sockerless-gitlabhub-test .
	docker run --rm sockerless-gitlabhub-test

# UI monorepo targets
.PHONY: ui-install ui-build ui-dev ui-test ui-clean
.PHONY: build-ecs-with-ui build-ecs-noui build-lambda-with-ui build-lambda-noui
.PHONY: build-cloudrun-with-ui build-cloudrun-noui build-gcf-with-ui build-gcf-noui
.PHONY: build-aca-with-ui build-aca-noui build-azf-with-ui build-azf-noui
.PHONY: build-docker-backend-with-ui build-docker-backend-noui
.PHONY: build-sim-aws-noui build-sim-gcp-noui build-sim-azure-noui
.PHONY: build-sim-aws-with-ui build-sim-gcp-with-ui build-sim-azure-with-ui
.PHONY: build-admin-with-ui build-admin-noui
.PHONY: build-bleephub-with-ui build-bleephub-noui
.PHONY: build-gitlabhub-with-ui build-gitlabhub-noui
.PHONY: ui-e2e-admin ui-e2e-bleephub ui-e2e-gitlabhub
.PHONY: ui-e2e-backend-ecs ui-e2e-backend-lambda ui-e2e-backend-cloudrun
.PHONY: ui-e2e-backend-gcf ui-e2e-backend-aca ui-e2e-backend-azf ui-e2e-backend-docker
.PHONY: ui-e2e-sim-aws ui-e2e-sim-gcp ui-e2e-sim-azure
.PHONY: ui-e2e-all

ui-install:
	cd ui && bun install

ui-build: ui-install
	cd ui && bunx turbo run build
	rm -rf backends/ecs/dist && cp -r ui/packages/backend-ecs/dist backends/ecs/dist
	rm -rf backends/lambda/dist && cp -r ui/packages/backend-lambda/dist backends/lambda/dist
	rm -rf backends/cloudrun/dist && cp -r ui/packages/backend-cloudrun/dist backends/cloudrun/dist
	rm -rf backends/cloudrun-functions/dist && cp -r ui/packages/backend-gcf/dist backends/cloudrun-functions/dist
	rm -rf backends/aca/dist && cp -r ui/packages/backend-aca/dist backends/aca/dist
	rm -rf backends/azure-functions/dist && cp -r ui/packages/backend-azf/dist backends/azure-functions/dist
	rm -rf backends/docker/dist && cp -r ui/packages/backend-docker/dist backends/docker/dist
	rm -rf simulators/aws/dist && cp -r ui/packages/simulator-aws/dist simulators/aws/dist
	rm -rf simulators/gcp/dist && cp -r ui/packages/simulator-gcp/dist simulators/gcp/dist
	rm -rf simulators/azure/dist && cp -r ui/packages/simulator-azure/dist simulators/azure/dist
	rm -rf cmd/sockerless-admin/dist && cp -r ui/packages/admin/dist cmd/sockerless-admin/dist
	rm -rf bleephub/dist && cp -r ui/packages/bleephub/dist bleephub/dist
	rm -rf gitlabhub/dist && cp -r ui/packages/gitlabhub/dist gitlabhub/dist

ui-dev:
	cd ui && bunx turbo run dev --filter=@sockerless/ui-backend-docker

ui-test:
	cd ui && bunx turbo run test

ui-clean:
	rm -rf ui/node_modules ui/packages/*/node_modules ui/packages/*/dist ui/.turbo
	rm -rf backends/ecs/dist backends/lambda/dist
	rm -rf backends/cloudrun/dist backends/cloudrun-functions/dist
	rm -rf backends/aca/dist backends/azure-functions/dist
	rm -rf backends/docker/dist
	rm -rf simulators/aws/dist simulators/gcp/dist simulators/azure/dist
	rm -rf cmd/sockerless-admin/dist bleephub/dist gitlabhub/dist

build-ecs-with-ui: ui-build
	cd backends/ecs && go build -o sockerless-backend-ecs ./cmd/sockerless-backend-ecs

build-ecs-noui:
	cd backends/ecs && go build -tags noui -o /dev/null ./cmd/sockerless-backend-ecs

build-lambda-with-ui: ui-build
	cd backends/lambda && go build -o sockerless-backend-lambda ./cmd/sockerless-backend-lambda

build-lambda-noui:
	cd backends/lambda && go build -tags noui -o /dev/null ./cmd/sockerless-backend-lambda

build-cloudrun-with-ui: ui-build
	cd backends/cloudrun && go build -o sockerless-backend-cloudrun ./cmd/sockerless-backend-cloudrun

build-cloudrun-noui:
	cd backends/cloudrun && go build -tags noui -o /dev/null ./cmd/sockerless-backend-cloudrun

build-gcf-with-ui: ui-build
	cd backends/cloudrun-functions && go build -o sockerless-backend-gcf ./cmd/sockerless-backend-gcf

build-gcf-noui:
	cd backends/cloudrun-functions && go build -tags noui -o /dev/null ./cmd/sockerless-backend-gcf

build-aca-with-ui: ui-build
	cd backends/aca && go build -o sockerless-backend-aca ./cmd/sockerless-backend-aca

build-aca-noui:
	cd backends/aca && go build -tags noui -o /dev/null ./cmd/sockerless-backend-aca

build-azf-with-ui: ui-build
	cd backends/azure-functions && go build -o sockerless-backend-azf ./cmd/sockerless-backend-azf

build-azf-noui:
	cd backends/azure-functions && go build -tags noui -o /dev/null ./cmd/sockerless-backend-azf

build-docker-backend-with-ui: ui-build
	cd backends/docker && go build -o sockerless-backend-docker ./cmd

build-docker-backend-noui:
	cd backends/docker && go build -tags noui -o /dev/null ./cmd

build-sim-aws-noui:
	cd simulators/aws && GOWORK=off go build -tags noui -o /dev/null .

build-sim-gcp-noui:
	cd simulators/gcp && GOWORK=off go build -tags noui -o /dev/null .

build-sim-azure-noui:
	cd simulators/azure && GOWORK=off go build -tags noui -o /dev/null .

build-admin-with-ui: ui-build
	cd cmd/sockerless-admin && go build -o sockerless-admin .

build-admin-noui:
	cd cmd/sockerless-admin && go build -tags noui -o /dev/null .

build-bleephub-with-ui: ui-build
	cd bleephub && go build -o bleephub-server ./cmd

build-bleephub-noui:
	cd bleephub && go build -tags noui -o /dev/null ./cmd

build-gitlabhub-with-ui: ui-build
	cd gitlabhub && go build -o gitlabhub-server ./cmd

build-gitlabhub-noui:
	cd gitlabhub && go build -tags noui -o /dev/null ./cmd

ui-e2e-bleephub: build-bleephub-with-ui
	cd ui/packages/bleephub && SERVER_BIN="$(CURDIR)/bleephub/bleephub-server" bunx playwright test

ui-e2e-gitlabhub: build-gitlabhub-with-ui
	cd ui/packages/gitlabhub && SERVER_BIN="$(CURDIR)/gitlabhub/gitlabhub-server" bunx playwright test

ui-e2e-admin: build-admin-with-ui
	cd ui/packages/admin && ADMIN_BIN="$(CURDIR)/cmd/sockerless-admin/sockerless-admin" bunx playwright test

build-sim-aws-with-ui: ui-build
	cd simulators/aws && GOWORK=off go build -o simulator-aws .

build-sim-gcp-with-ui: ui-build
	cd simulators/gcp && GOWORK=off go build -o simulator-gcp .

build-sim-azure-with-ui: ui-build
	cd simulators/azure && GOWORK=off go build -o simulator-azure .

ui-e2e-backend-ecs: build-ecs-with-ui
	cd ui/packages/backend-ecs && SOCKERLESS_ENDPOINT_URL=http://localhost:1 BACKEND_BIN="$(CURDIR)/backends/ecs/sockerless-backend-ecs" bunx playwright test

ui-e2e-backend-lambda: build-lambda-with-ui
	cd ui/packages/backend-lambda && SOCKERLESS_ENDPOINT_URL=http://localhost:1 BACKEND_BIN="$(CURDIR)/backends/lambda/sockerless-backend-lambda" bunx playwright test

ui-e2e-backend-cloudrun: build-cloudrun-with-ui
	cd ui/packages/backend-cloudrun && SOCKERLESS_ENDPOINT_URL=http://localhost:1 BACKEND_BIN="$(CURDIR)/backends/cloudrun/sockerless-backend-cloudrun" bunx playwright test

ui-e2e-backend-gcf: build-gcf-with-ui
	cd ui/packages/backend-gcf && SOCKERLESS_ENDPOINT_URL=http://localhost:1 BACKEND_BIN="$(CURDIR)/backends/cloudrun-functions/sockerless-backend-gcf" bunx playwright test

ui-e2e-backend-aca: build-aca-with-ui
	cd ui/packages/backend-aca && SOCKERLESS_ENDPOINT_URL=http://localhost:1 BACKEND_BIN="$(CURDIR)/backends/aca/sockerless-backend-aca" bunx playwright test

ui-e2e-backend-azf: build-azf-with-ui
	cd ui/packages/backend-azf && SOCKERLESS_ENDPOINT_URL=http://localhost:1 BACKEND_BIN="$(CURDIR)/backends/azure-functions/sockerless-backend-azf" bunx playwright test

ui-e2e-backend-docker: build-docker-backend-with-ui
	cd ui/packages/backend-docker && BACKEND_BIN="$(CURDIR)/backends/docker/sockerless-backend-docker" bunx playwright test

ui-e2e-sim-aws: build-sim-aws-with-ui
	cd ui/packages/simulator-aws && SERVER_BIN="$(CURDIR)/simulators/aws/simulator-aws" bunx playwright test

ui-e2e-sim-gcp: build-sim-gcp-with-ui
	cd ui/packages/simulator-gcp && SERVER_BIN="$(CURDIR)/simulators/gcp/simulator-gcp" bunx playwright test

ui-e2e-sim-azure: build-sim-azure-with-ui
	cd ui/packages/simulator-azure && SERVER_BIN="$(CURDIR)/simulators/azure/simulator-azure" bunx playwright test

ui-e2e-all: ui-e2e-admin ui-e2e-bleephub ui-e2e-gitlabhub ui-e2e-backend-ecs ui-e2e-backend-lambda ui-e2e-backend-cloudrun ui-e2e-backend-gcf ui-e2e-backend-aca ui-e2e-backend-azf ui-e2e-backend-docker ui-e2e-sim-aws ui-e2e-sim-gcp ui-e2e-sim-azure
