.PHONY: sim-test-ecs sim-test-lambda sim-test-cloudrun sim-test-gcf sim-test-aca sim-test-azf
.PHONY: sim-test-aws sim-test-gcp sim-test-azure sim-test-all
.PHONY: test test-unit test-e2e lint
.PHONY: test-agent test-sandbox test-core test-frontend test-bleephub test-gitlabhub
.PHONY: bleephub-test bleephub-gh-test gitlabhub-test
.PHONY: smoke-test-act smoke-test-act-ecs smoke-test-act-cloudrun smoke-test-act-aca smoke-test-act-all
.PHONY: smoke-test-gitlab smoke-test-gitlab-ecs smoke-test-gitlab-cloudrun smoke-test-gitlab-aca smoke-test-gitlab-all
.PHONY: e2e-github-all e2e-gitlab-all e2e-all
.PHONY: e2e-github-build-memory e2e-github-build-aws e2e-github-build-gcp e2e-github-build-azure
.PHONY: e2e-github-memory e2e-github-ecs e2e-github-lambda
.PHONY: e2e-github-cloudrun e2e-github-gcf e2e-github-aca e2e-github-azf
.PHONY: upstream-test-act-build-memory upstream-test-act-build-aws upstream-test-act-build-gcp upstream-test-act-build-azure
.PHONY: upstream-test-act upstream-test-act-individual upstream-test-act-ecs upstream-test-act-lambda upstream-test-act-all
.PHONY: upstream-test-act-cloudrun upstream-test-act-gcf upstream-test-act-aca upstream-test-act-azf
.PHONY: upstream-test-gitlab-ci-local
.PHONY: upstream-test-gcl-build-memory upstream-test-gcl-build-aws upstream-test-gcl-build-gcp upstream-test-gcl-build-azure
.PHONY: upstream-test-gcl-memory upstream-test-gcl-ecs upstream-test-gcl-lambda
.PHONY: upstream-test-gcl-cloudrun upstream-test-gcl-gcf upstream-test-gcl-aca upstream-test-gcl-azf
.PHONY: upstream-test-gcl-all

# Per-module unit test targets
test-agent:
	@echo "=== test agent ==="
	cd agent && go test -v -race -timeout 2m ./...

test-sandbox:
	@echo "=== test sandbox ==="
	cd sandbox && go test -v -timeout 1m ./...

test-core:
	@echo "=== test backends/core ==="
	cd backends/core && go test -v -timeout 2m ./...

test-frontend:
	@echo "=== test frontends/docker ==="
	cd frontends/docker && go test -v -timeout 1m ./...

test-bleephub:
	@echo "=== test bleephub ==="
	cd bleephub && go test -v -timeout 3m ./...

test-gitlabhub:
	@echo "=== test gitlabhub ==="
	cd gitlabhub && go test -v -timeout 3m ./...

# E2E integration tests (builds + starts backend/frontend/agent binaries)
test-e2e:
	@echo "=== test e2e ==="
	cd tests && go test -v -timeout 5m ./...

# All unit tests (per-module)
test-unit: test-agent test-sandbox test-core test-frontend test-bleephub test-gitlabhub

# All tests (unit + e2e)
test: test-unit test-e2e

# Lint all Go modules (golangci-lint required)
MODULES = api agent sandbox frontends/docker backends/core backends/memory \
  backends/docker backends/ecs backends/lambda backends/cloudrun \
  backends/cloudrun-functions backends/aca backends/azure-functions \
  bleephub gitlabhub

lint:
	@for mod in $(MODULES); do \
	    echo "=== lint $$mod ===" && \
	    cd $(CURDIR)/$$mod && golangci-lint run ./... || exit 1; \
	done

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
	docker build -t sockerless-smoke-act -f smoke-tests/act/Dockerfile .
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
	cd smoke-tests/gitlab && docker compose down -v 2>/dev/null; BACKEND=memory docker compose up --build --abort-on-container-exit --exit-code-from orchestrator

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

e2e-github-build-memory:
	docker build -t $(E2E_GITHUB_IMAGE)-memory -f tests/e2e-live-tests/github-runner/Dockerfile.memory .

e2e-github-build-aws:
	docker build -t $(E2E_GITHUB_IMAGE)-aws -f tests/e2e-live-tests/github-runner/Dockerfile.aws .

e2e-github-build-gcp:
	docker build -t $(E2E_GITHUB_IMAGE)-gcp -f tests/e2e-live-tests/github-runner/Dockerfile.gcp .

e2e-github-build-azure:
	docker build -t $(E2E_GITHUB_IMAGE)-azure -f tests/e2e-live-tests/github-runner/Dockerfile.azure .

e2e-github-memory: e2e-github-build-memory
	docker run --rm -e BACKEND=memory $(E2E_GITHUB_IMAGE)-memory --backend memory

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
	@for b in memory ecs lambda cloudrun gcf aca azf; do \
	    echo "=== E2E GitHub: $$b ===" && \
	    $(MAKE) e2e-github-$$b || exit 1; \
	done

# E2E live tests — GitLab Runner (docker-executor)
e2e-gitlab-%:
	cd tests/e2e-live-tests/gitlab-runner-docker && ./run.sh --backend $*

e2e-gitlab-all:
	@for b in memory ecs lambda cloudrun gcf aca azf; do \
	    echo "=== E2E GitLab: $$b ===" && \
	    $(MAKE) e2e-gitlab-$$b || exit 1; \
	done

# E2E all
e2e-all: e2e-github-all e2e-gitlab-all

# Upstream test suites — act, per-cloud images
UPSTREAM_ACT_IMAGE = sockerless-upstream-act

upstream-test-act-build-memory:
	docker build -t $(UPSTREAM_ACT_IMAGE)-memory -f tests/upstream/act/Dockerfile.memory .

upstream-test-act-build-aws:
	docker build -t $(UPSTREAM_ACT_IMAGE)-aws -f tests/upstream/act/Dockerfile.aws .

upstream-test-act-build-gcp:
	docker build -t $(UPSTREAM_ACT_IMAGE)-gcp -f tests/upstream/act/Dockerfile.gcp .

upstream-test-act-build-azure:
	docker build -t $(UPSTREAM_ACT_IMAGE)-azure -f tests/upstream/act/Dockerfile.azure .

upstream-test-act: upstream-test-act-build-memory
	docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-memory

upstream-test-act-individual: upstream-test-act-build-memory
	docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-memory --individual

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
	@for b in memory ecs lambda cloudrun gcf aca azf; do \
	    echo "=== Upstream Act: $$b ===" && \
	    $(MAKE) upstream-test-act-$$b || true; \
	done

# Upstream test suites — gitlab-ci-local, per-cloud images
UPSTREAM_GCL_IMAGE = sockerless-upstream-gcl

upstream-test-gcl-build-memory:
	docker build -t $(UPSTREAM_GCL_IMAGE)-memory -f tests/upstream/gitlab-ci-local/Dockerfile.memory .

upstream-test-gcl-build-aws:
	docker build -t $(UPSTREAM_GCL_IMAGE)-aws -f tests/upstream/gitlab-ci-local/Dockerfile.aws .

upstream-test-gcl-build-gcp:
	docker build -t $(UPSTREAM_GCL_IMAGE)-gcp -f tests/upstream/gitlab-ci-local/Dockerfile.gcp .

upstream-test-gcl-build-azure:
	docker build -t $(UPSTREAM_GCL_IMAGE)-azure -f tests/upstream/gitlab-ci-local/Dockerfile.azure .

upstream-test-gitlab-ci-local: upstream-test-gcl-build-memory
	docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-memory

upstream-test-gcl-memory: upstream-test-gcl-build-memory
	docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-memory

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
	@for b in memory ecs lambda cloudrun gcf aca azf; do \
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
.PHONY: build-memory-with-ui build-memory-noui
.PHONY: build-ecs-with-ui build-ecs-noui build-lambda-with-ui build-lambda-noui
.PHONY: build-cloudrun-with-ui build-cloudrun-noui build-gcf-with-ui build-gcf-noui
.PHONY: build-aca-with-ui build-aca-noui build-azf-with-ui build-azf-noui
.PHONY: build-docker-backend-with-ui build-docker-backend-noui
.PHONY: build-frontend-with-ui build-frontend-noui

ui-install:
	cd ui && bun install

ui-build: ui-install
	cd ui && bunx turbo run build
	rm -rf backends/memory/dist && cp -r ui/packages/backend-memory/dist backends/memory/dist
	rm -rf backends/ecs/dist && cp -r ui/packages/backend-ecs/dist backends/ecs/dist
	rm -rf backends/lambda/dist && cp -r ui/packages/backend-lambda/dist backends/lambda/dist
	rm -rf backends/cloudrun/dist && cp -r ui/packages/backend-cloudrun/dist backends/cloudrun/dist
	rm -rf backends/cloudrun-functions/dist && cp -r ui/packages/backend-gcf/dist backends/cloudrun-functions/dist
	rm -rf backends/aca/dist && cp -r ui/packages/backend-aca/dist backends/aca/dist
	rm -rf backends/azure-functions/dist && cp -r ui/packages/backend-azf/dist backends/azure-functions/dist
	rm -rf backends/docker/dist && cp -r ui/packages/backend-docker/dist backends/docker/dist
	rm -rf frontends/docker/dist && cp -r ui/packages/frontend-docker/dist frontends/docker/dist

ui-dev:
	cd ui && bunx turbo run dev --filter=@sockerless/ui-backend-memory

ui-test:
	cd ui && bunx turbo run test

ui-clean:
	rm -rf ui/node_modules ui/packages/*/node_modules ui/packages/*/dist ui/.turbo
	rm -rf backends/memory/dist backends/ecs/dist backends/lambda/dist
	rm -rf backends/cloudrun/dist backends/cloudrun-functions/dist
	rm -rf backends/aca/dist backends/azure-functions/dist
	rm -rf backends/docker/dist frontends/docker/dist

build-memory-with-ui: ui-build
	cd backends/memory && go build -o sockerless-backend-memory ./cmd

build-memory-noui:
	cd backends/memory && go build -tags noui -o /dev/null ./cmd

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
	cd backends/docker && go build -o sockerless-backend-docker ./cmd/sockerless-backend-docker

build-docker-backend-noui:
	cd backends/docker && go build -tags noui -o /dev/null ./cmd/sockerless-backend-docker

build-frontend-with-ui: ui-build
	cd frontends/docker && go build -o sockerless-docker-frontend ./cmd

build-frontend-noui:
	cd frontends/docker && go build -tags noui -o /dev/null ./cmd
