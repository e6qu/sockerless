# Sockerless — top-level Makefile.
#
# Thin orchestrator. Per-app recipes live in each app's own Makefile;
# this file just delegates and aggregates. See docs/MAKEFILE_STANDARD.md
# for the standard target surface every app implements.
#
# Common workflows:
#
#   make help              # list targets
#   make build             # build every app
#   make test              # unit-test every app
#   make test-integration  # integration-test every app
#   make lint              # lint every app
#   make clean             # clean every app
#
#   make backends/ecs/build              # build a single app via path
#   make backends/ecs/test-integration   # sim-backed integration tests
#   make bleephub/test-integration       # bleephub gh-runner integration
#   make tests/test                      # cross-backend e2e suite
#   make cmd/sockerless-admin/run        # run a single app via path
#
#   make stack-aws-ecs     # bring up sim+backend+admin for AWS-ECS
#   make stack-status      # show running stack
#   make stack-down        # stop running stack
#
# Cross-cutting Docker-driven suites live in dedicated sections below
# (smoke-test-*, tf-int-test-*, e2e-github-*, e2e-gitlab-*,
# upstream-test-*, bleephub-gh-docker-test). Per-app aliases were
# removed — sockerless is under active development and carries no
# legacy compatibility surface; use the path-delegation form
# (`make <dir>/<target>`) instead.

include make/help.mk
include make/colors.mk

# ── Apps — explicit lists (not glob) ────────────────────────────────
#
# When a new app lands, add it to one of these lists. The fan-out and
# delegation rules below pick it up automatically.

# Go binaries with optional embedded UI (12).
GO_UI_APPS := \
  cmd/sockerless-admin \
  bleephub \
  backends/docker \
  backends/ecs \
  backends/lambda \
  backends/cloudrun \
  backends/cloudrun-functions \
  backends/aca \
  backends/azure-functions \
  simulators/aws \
  simulators/gcp \
  simulators/azure

# Go binaries / libraries without UI (5).
GO_APPS := \
  cmd/sockerless \
  agent \
  github-runner-dispatcher-aws \
  github-runner-dispatcher-gcp \
  github-runner-dispatcher-azure

# UI packages (13). Each consumed by the corresponding GO_UI_APPS entry
# (except `core` which is a shared library, and `frontend-docker` which
# is standalone).
UI_APPS := \
  ui/packages/admin \
  ui/packages/bleephub \
  ui/packages/backend-docker \
  ui/packages/backend-ecs \
  ui/packages/backend-lambda \
  ui/packages/backend-cloudrun \
  ui/packages/backend-gcf \
  ui/packages/backend-aca \
  ui/packages/backend-azf \
  ui/packages/simulator-aws \
  ui/packages/simulator-gcp \
  ui/packages/simulator-azure \
  ui/packages/frontend-docker \
  ui/packages/core

# Test-category Makefiles (sim-vs-backend SDK/CLI/Terraform tests +
# real-runner harnesses + smoke tests + the cross-backend e2e).
TEST_DIRS := \
  tests \
  smoke-tests \
  simulators/aws/sdk-tests simulators/aws/cli-tests simulators/aws/terraform-tests \
  simulators/gcp/sdk-tests simulators/gcp/cli-tests simulators/gcp/terraform-tests \
  simulators/azure/sdk-tests simulators/azure/cli-tests simulators/azure/terraform-tests \
  tests/runners/github tests/runners/gitlab tests/runners/gcp-cells tests/runners/internal

ALL_APPS := $(GO_UI_APPS) $(GO_APPS) $(UI_APPS)

# ── Standard fan-out targets ────────────────────────────────────────
#
# `make build` → run `make build` in every app, in series. Fail fast.

.PHONY: install build build-noui test test-integration lint clean upgrade-deps check-deps

install: ## install deps in every app
	@$(MAKE) -s _fanout TARGET=install APPS="$(ALL_APPS)"

build: ## build every app
	@$(MAKE) -s _fanout TARGET=build APPS="$(GO_UI_APPS) $(GO_APPS) $(UI_APPS)"

build-noui: ## build every Go app with -tags noui (skips UI embed)
	@$(MAKE) -s _fanout TARGET=build-noui APPS="$(GO_UI_APPS) $(GO_APPS)"

test: ## unit-test every app
	@$(MAKE) -s _fanout TARGET=test APPS="$(ALL_APPS)"

test-integration: ## run integration tests across every Go app
	@$(MAKE) -s _fanout TARGET=test-integration APPS="$(GO_UI_APPS) $(GO_APPS) $(TEST_DIRS)"

lint: ## lint every Go app (CI lint runner has no bun — use lint-ui separately)
	@$(MAKE) -s _fanout TARGET=lint APPS="$(GO_UI_APPS) $(GO_APPS)"

lint-ui: ## lint every UI package (requires bun)
	@$(MAKE) -s _fanout TARGET=lint APPS="$(UI_APPS)"

lint-all: lint lint-ui ## lint every app (Go + UI)

clean: ## clean every app's artefacts
	@$(MAKE) -s _fanout TARGET=clean APPS="$(ALL_APPS)"

upgrade-deps: ## bump every Go module's direct deps to latest (per-module independence preserved — each app runs its own upgrade-deps)
	@$(MAKE) -s _fanout TARGET=upgrade-deps APPS="$(GO_UI_APPS) $(GO_APPS)"

check-deps: ## fail if any Go module / Terraform provider is behind its latest published version
	@bash scripts/check-latest-deps.sh

# Internal helper: iterate APPS and run TARGET in each. Stops on
# first failure. Honours --keep-going via $(MAKEFLAGS).
.PHONY: _fanout
_fanout:
	@for app in $(APPS); do \
	  if [ -f "$$app/Makefile" ]; then \
	    printf "$(COLOR_CYAN)▸ %s: %s$(COLOR_RESET)\n" "$$app" "$(TARGET)"; \
	    $(MAKE) -s -C "$$app" $(TARGET) || exit $$?; \
	  else \
	    printf "$(COLOR_DIM)skip %s (no Makefile)$(COLOR_RESET)\n" "$$app"; \
	  fi; \
	done

# ── Per-app delegation via path ─────────────────────────────────────
#
# `make backends/ecs/build` → `$(MAKE) -C backends/ecs build`.
# Works for any standardized target. `$*` is the path; `$@` carries
# the full target with the suffix appended.

# FORCE keeps the recipe from being short-circuited when a directory
# happens to share the target name (e.g. `bleephub/test/` exists on
# disk, so without FORCE `make bleephub/test` would silently report
# "up to date" instead of delegating into bleephub's Makefile).
.PHONY: FORCE
FORCE:

%/install %/build %/build-noui %/embed %/run %/dev %/test %/test-integration %/test-faas-smoke %/lint %/clean %/preview %/help: FORCE
	@$(MAKE) -s -C $* $(notdir $@)

# ── Stack orchestration ─────────────────────────────────────────────

include make/components.mk
include make/stack.mk

# ── api.Backend coverage gate ───────────────────────────────────────
#
# Compiler-style check that every backend implements every method on
# api.Backend. Lives in its own tool module under tools/.

.PHONY: check-backend-coverage check-backend-coverage-enforce
check-backend-coverage:         ; @cd tools/check-backend-coverage && GOWORK=off go run .
check-backend-coverage-enforce: ; @cd tools/check-backend-coverage && GOWORK=off go run . --enforce

# ── Smoke tests (Docker-based) ──────────────────────────────────────
#
# Each target builds a self-contained Docker image that boots the
# sim + backend + a tiny workload inside the container and asserts
# the round-trip. Kept inline at the top level because the recipes
# are short and don't share state with any per-app Makefile.
.PHONY: smoke-test-act smoke-test-act-ecs smoke-test-act-cloudrun smoke-test-act-aca smoke-test-act-all
.PHONY: smoke-test-gitlab smoke-test-gitlab-ecs smoke-test-gitlab-cloudrun smoke-test-gitlab-aca smoke-test-gitlab-all

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

smoke-test-gitlab:
	cd smoke-tests/gitlab && docker compose down -v 2>/dev/null; BACKEND=ecs docker compose up --build --abort-on-container-exit --exit-code-from orchestrator
smoke-test-gitlab-ecs:
	cd smoke-tests/gitlab && docker compose down -v 2>/dev/null; BACKEND=ecs docker compose up --build --abort-on-container-exit --exit-code-from orchestrator
smoke-test-gitlab-cloudrun:
	cd smoke-tests/gitlab && docker compose down -v 2>/dev/null; BACKEND=cloudrun docker compose up --build --abort-on-container-exit --exit-code-from orchestrator
smoke-test-gitlab-aca:
	cd smoke-tests/gitlab && docker compose down -v 2>/dev/null; BACKEND=aca docker compose up --build --abort-on-container-exit --exit-code-from orchestrator
smoke-test-gitlab-all: smoke-test-gitlab smoke-test-gitlab-ecs smoke-test-gitlab-cloudrun smoke-test-gitlab-aca

# ── FaaS simulator smoke tests (Go package integration) ─────────────
#
# These tests run inside the backend Go integration harnesses, against
# local simulators, and exercise the runner-shaped lifecycle:
# create → start → exec×N → wait → remove.
.PHONY: faas-smoke-test-lambda faas-smoke-test-cloudrun faas-smoke-test-gcf faas-smoke-test-aca faas-smoke-test-azf
.PHONY: faas-smoke-test-aws faas-smoke-test-gcp faas-smoke-test-azure faas-smoke-test-all

faas-smoke-test-lambda:   ; @$(MAKE) -s -C backends/lambda test-faas-smoke
faas-smoke-test-cloudrun: ; @$(MAKE) -s -C backends/cloudrun test-faas-smoke
faas-smoke-test-gcf:      ; @$(MAKE) -s -C backends/cloudrun-functions test-faas-smoke
faas-smoke-test-aca:      ; @$(MAKE) -s -C backends/aca test-faas-smoke
faas-smoke-test-azf:      ; @$(MAKE) -s -C backends/azure-functions test-faas-smoke
faas-smoke-test-aws:      faas-smoke-test-lambda
faas-smoke-test-gcp:      faas-smoke-test-cloudrun faas-smoke-test-gcf
faas-smoke-test-azure:    faas-smoke-test-aca faas-smoke-test-azf
faas-smoke-test-all:      faas-smoke-test-aws faas-smoke-test-gcp faas-smoke-test-azure

# ── Terraform integration tests (Docker-based) ──────────────────────
.PHONY: tf-int-test-ecs tf-int-test-lambda tf-int-test-cloudrun tf-int-test-gcf tf-int-test-aca tf-int-test-azf
.PHONY: tf-int-test-aws tf-int-test-gcp tf-int-test-azure tf-int-test-all tf-int-build

TF_INT_IMAGE := sockerless-tf-int

tf-int-build:
	docker build -t $(TF_INT_IMAGE) -f tests/terraform-integration/Dockerfile .

tf-int-test-ecs: tf-int-build       ; docker run --rm $(TF_INT_IMAGE) --backend ecs
tf-int-test-lambda: tf-int-build    ; docker run --rm $(TF_INT_IMAGE) --backend lambda
tf-int-test-cloudrun: tf-int-build  ; docker run --rm $(TF_INT_IMAGE) --backend cloudrun
tf-int-test-gcf: tf-int-build       ; docker run --rm $(TF_INT_IMAGE) --backend gcf
tf-int-test-aca: tf-int-build       ; docker run --rm $(TF_INT_IMAGE) --backend aca
tf-int-test-azf: tf-int-build       ; docker run --rm $(TF_INT_IMAGE) --backend azf
tf-int-test-aws:   tf-int-test-ecs tf-int-test-lambda
tf-int-test-gcp:   tf-int-test-cloudrun tf-int-test-gcf
tf-int-test-azure: tf-int-test-aca tf-int-test-azf
tf-int-test-all:   tf-int-test-aws tf-int-test-gcp tf-int-test-azure

# ── E2E live tests — GitHub Actions runner ──────────────────────────
#
# Per-cloud Docker images that run the official `actions/runner` end
# to end through a real workflow against a sim-mode backend.
.PHONY: e2e-github-build-aws e2e-github-build-gcp e2e-github-build-azure
.PHONY: e2e-github-ecs e2e-github-lambda e2e-github-cloudrun e2e-github-gcf e2e-github-aca e2e-github-azf
.PHONY: e2e-github-all e2e-gitlab-all e2e-all
.PHONY: e2e-github-sim-arithmetic e2e-gitlab-sim-arithmetic e2e-real-runner-sim-arithmetic

E2E_GITHUB_IMAGE := sockerless-e2e-github

e2e-github-build-aws:    ; docker build -t $(E2E_GITHUB_IMAGE)-aws   -f tests/e2e-live-tests/github-runner/Dockerfile.aws   .
e2e-github-build-gcp:    ; docker build -t $(E2E_GITHUB_IMAGE)-gcp   -f tests/e2e-live-tests/github-runner/Dockerfile.gcp   .
e2e-github-build-azure:  ; docker build -t $(E2E_GITHUB_IMAGE)-azure -f tests/e2e-live-tests/github-runner/Dockerfile.azure .

e2e-github-ecs:      e2e-github-build-aws    ; docker run --rm -e BACKEND=ecs      $(E2E_GITHUB_IMAGE)-aws   --backend ecs
e2e-github-lambda:   e2e-github-build-aws    ; docker run --rm -e BACKEND=lambda   $(E2E_GITHUB_IMAGE)-aws   --backend lambda
e2e-github-cloudrun: e2e-github-build-gcp    ; docker run --rm -e BACKEND=cloudrun $(E2E_GITHUB_IMAGE)-gcp   --backend cloudrun
e2e-github-gcf:      e2e-github-build-gcp    ; docker run --rm -e BACKEND=gcf      $(E2E_GITHUB_IMAGE)-gcp   --backend gcf
e2e-github-aca:      e2e-github-build-azure  ; docker run --rm -e BACKEND=aca      $(E2E_GITHUB_IMAGE)-azure --backend aca
e2e-github-azf:      e2e-github-build-azure  ; docker run --rm -e BACKEND=azf      $(E2E_GITHUB_IMAGE)-azure --backend azf

e2e-github-all:
	@for b in ecs lambda cloudrun gcf aca azf; do \
	  printf "$(COLOR_CYAN)=== E2E GitHub: %s ===$(COLOR_RESET)\n" "$$b" && \
	  $(MAKE) -s e2e-github-$$b || exit 1; \
	done

e2e-gitlab-%:
	cd tests/e2e-live-tests/gitlab-runner-docker && ./run.sh --backend $*

e2e-gitlab-all:
	@for b in ecs lambda cloudrun gcf aca azf; do \
	  printf "$(COLOR_CYAN)=== E2E GitLab: %s ===$(COLOR_RESET)\n" "$$b" && \
	  $(MAKE) -s e2e-gitlab-$$b || exit 1; \
	done

e2e-github-sim-arithmetic:
	go test -v -tags github_runner_live -run TestGitHub_Simulator_Arithmetic -timeout 30m ./tests/runners/github

e2e-gitlab-sim-arithmetic:
	go test -v -tags gitlab_runner_live -run TestGitLab_Simulator_Arithmetic -timeout 30m ./tests/runners/gitlab

e2e-real-runner-sim-arithmetic: e2e-github-sim-arithmetic e2e-gitlab-sim-arithmetic

e2e-all: e2e-github-all e2e-gitlab-all

# ── Upstream test suites — act + gitlab-ci-local ────────────────────
#
# Run real upstream CI tooling (nektos/act, gitlab-ci-local) against
# sockerless sim-mode backends. Per-cloud Docker images keep the
# host clean of node/python/go toolchains.
.PHONY: upstream-test-act-build-aws upstream-test-act-build-gcp upstream-test-act-build-azure
.PHONY: upstream-test-act upstream-test-act-individual
.PHONY: upstream-test-act-ecs upstream-test-act-lambda upstream-test-act-cloudrun
.PHONY: upstream-test-act-gcf upstream-test-act-aca upstream-test-act-azf upstream-test-act-all
.PHONY: upstream-test-gcl-build-aws upstream-test-gcl-build-gcp upstream-test-gcl-build-azure
.PHONY: upstream-test-gitlab-ci-local
.PHONY: upstream-test-gcl-ecs upstream-test-gcl-lambda upstream-test-gcl-cloudrun
.PHONY: upstream-test-gcl-gcf upstream-test-gcl-aca upstream-test-gcl-azf upstream-test-gcl-all

UPSTREAM_ACT_IMAGE := sockerless-upstream-act
UPSTREAM_GCL_IMAGE := sockerless-upstream-gcl

upstream-test-act-build-aws:   ; docker build -t $(UPSTREAM_ACT_IMAGE)-aws   -f tests/upstream/act/Dockerfile.aws   .
upstream-test-act-build-gcp:   ; docker build -t $(UPSTREAM_ACT_IMAGE)-gcp   -f tests/upstream/act/Dockerfile.gcp   .
upstream-test-act-build-azure: ; docker build -t $(UPSTREAM_ACT_IMAGE)-azure -f tests/upstream/act/Dockerfile.azure .

upstream-test-act:            upstream-test-act-build-aws ; docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-aws   --backend ecs
upstream-test-act-individual: upstream-test-act-build-aws ; docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-aws   --backend ecs --individual
upstream-test-act-ecs:        upstream-test-act-build-aws ; docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-aws   --backend ecs
upstream-test-act-lambda:     upstream-test-act-build-aws ; docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-aws   --backend lambda
upstream-test-act-cloudrun:   upstream-test-act-build-gcp ; docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-gcp   --backend cloudrun
upstream-test-act-gcf:        upstream-test-act-build-gcp ; docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-gcp   --backend gcf
upstream-test-act-aca:        upstream-test-act-build-azure ; docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-azure --backend aca
upstream-test-act-azf:        upstream-test-act-build-azure ; docker run --rm -v "$(CURDIR)/tests/upstream/act/results:/results" $(UPSTREAM_ACT_IMAGE)-azure --backend azf
upstream-test-act-all:
	@for b in ecs lambda cloudrun gcf aca azf; do \
	  printf "$(COLOR_CYAN)=== Upstream Act: %s ===$(COLOR_RESET)\n" "$$b" && \
	  $(MAKE) -s upstream-test-act-$$b || true; \
	done

upstream-test-gcl-build-aws:   ; docker build -t $(UPSTREAM_GCL_IMAGE)-aws   -f tests/upstream/gitlab-ci-local/Dockerfile.aws   .
upstream-test-gcl-build-gcp:   ; docker build -t $(UPSTREAM_GCL_IMAGE)-gcp   -f tests/upstream/gitlab-ci-local/Dockerfile.gcp   .
upstream-test-gcl-build-azure: ; docker build -t $(UPSTREAM_GCL_IMAGE)-azure -f tests/upstream/gitlab-ci-local/Dockerfile.azure .

upstream-test-gitlab-ci-local: upstream-test-gcl-build-aws   ; docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-aws   --backend ecs
upstream-test-gcl-ecs:         upstream-test-gcl-build-aws   ; docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-aws   --backend ecs
upstream-test-gcl-lambda:      upstream-test-gcl-build-aws   ; docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-aws   --backend lambda
upstream-test-gcl-cloudrun:    upstream-test-gcl-build-gcp   ; docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-gcp   --backend cloudrun
upstream-test-gcl-gcf:         upstream-test-gcl-build-gcp   ; docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-gcp   --backend gcf
upstream-test-gcl-aca:         upstream-test-gcl-build-azure ; docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-azure --backend aca
upstream-test-gcl-azf:         upstream-test-gcl-build-azure ; docker run --rm -v "$(CURDIR)/tests/upstream/gitlab-ci-local/results:/results" $(UPSTREAM_GCL_IMAGE)-azure --backend azf
upstream-test-gcl-all:
	@for b in ecs lambda cloudrun gcf aca azf; do \
	  printf "$(COLOR_CYAN)=== Upstream GCL: %s ===$(COLOR_RESET)\n" "$$b" && \
	  $(MAKE) -s upstream-test-gcl-$$b || true; \
	done

# ── Bleephub gh CLI parity harness (Docker) ─────────────────────────
#
# Builds a Docker image that bundles bleephub + the official `gh`
# binary, then runs bleephub/test/run-gh-test.sh against it. Real gh,
# real TLS, real HTTP. Image build is ~60s cold, ~10s warm.
.PHONY: bleephub-gh-docker-test
bleephub-gh-docker-test:
	@printf "$(COLOR_CYAN)▸ Building bleephub gh-test image…$(COLOR_RESET)\n"
	@docker build -f bleephub/Dockerfile.gh-test -t bleephub-gh-test:local .
	@printf "$(COLOR_CYAN)▸ Running gh CLI parity harness…$(COLOR_RESET)\n"
	@docker run --rm bleephub-gh-test:local
