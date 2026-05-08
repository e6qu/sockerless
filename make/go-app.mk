# go-app.mk — standardized recipes for Go-binary apps.
#
# Required variables (set by the leaf Makefile *before* including):
#   APP_NAME     : output binary name (e.g. sockerless-backend-ecs)
#   GO_PACKAGE   : Go package path to build (e.g. ./cmd/sockerless-backend-ecs)
#
# Optional — for apps that embed a UI bundle:
#   UI_PACKAGE   : ui/packages/<name> directory basename (e.g. backend-ecs)
#                  When set, `build` will try to embed the UI's dist/ if
#                  present; falls back to `build-noui` when missing.
#
# Optional — for `run`:
#   DEFAULT_PORT : if set, gets exposed in `make run` echo banner
#   RUN_FLAGS    : flags appended to `./<binary> <RUN_FLAGS>` on `make run`
#   RUN_ENV      : env vars exported before `make run` (e.g.
#                  RUN_ENV := SOCKERLESS_ENDPOINT_URL=http://localhost:4566)
#
# Optional — for non-workspace builds:
#   GO_BUILD_FLAGS : extra `go build` flags
#   GO_ENV         : env prepended to every go invocation (e.g. GOWORK=off)
#
# Convention: this file lives at <repo>/make/go-app.mk. Leaf Makefiles
# discover the repo root by counting up from $(CURDIR) — but to keep
# leaves dead-simple, we accept a REPO_ROOT_REL var the leaf Makefile
# may set. Default: ../.. (works for backends/<x>/, simulators/<x>/,
# cmd/<x>/, bleephub/, agent/).

REPO_ROOT_REL ?= ../..
REPO_ROOT     := $(abspath $(CURDIR)/$(REPO_ROOT_REL))
UI_DIST_SRC   := $(REPO_ROOT)/ui/packages/$(UI_PACKAGE)/dist
LOCAL_DIST    := $(CURDIR)/dist

include $(REPO_ROOT)/make/help.mk
include $(REPO_ROOT)/make/colors.mk

# ── Standardized targets ────────────────────────────────────────────

.PHONY: install build build-noui embed run dev test test-integration lint clean

install: ## install Go module deps
	$(GO_ENV) go mod download

build: ## build the binary (with UI when available, else falls back to build-noui)
ifndef UI_PACKAGE
	$(call STEP,$(APP_NAME): building (no UI configured))
	$(GO_ENV) go build $(GO_BUILD_FLAGS) -o $(APP_NAME) $(GO_PACKAGE)
else
	@if [ -d "$(UI_DIST_SRC)" ]; then \
	  printf "$(COLOR_CYAN)▸ %s: embedding UI from %s$(COLOR_RESET)\n" \
	    "$(APP_NAME)" "$(UI_DIST_SRC)" ; \
	  rm -rf $(LOCAL_DIST) && cp -r $(UI_DIST_SRC) $(LOCAL_DIST) ; \
	  $(GO_ENV) go build $(GO_BUILD_FLAGS) -o $(APP_NAME) $(GO_PACKAGE) ; \
	else \
	  printf "$(COLOR_YEL)▸ %s: no UI dist at %s — falling back to build-noui$(COLOR_RESET)\n" \
	    "$(APP_NAME)" "$(UI_DIST_SRC)" ; \
	  $(MAKE) -s build-noui ; \
	fi
endif

build-noui: ## build the binary with -tags noui (no embedded UI)
	$(call STEP,$(APP_NAME): building -tags noui)
	$(GO_ENV) go build -tags noui $(GO_BUILD_FLAGS) -o $(APP_NAME) $(GO_PACKAGE)

# `embed` is exposed as a stand-alone target so a top-level orchestrator
# can sequence ui-build → embed → go-build deterministically. It does
# NOT invoke another Makefile — it just copies a known dist/ path.
embed: ## copy UI dist from ui/packages/<UI_PACKAGE>/dist into ./dist
ifndef UI_PACKAGE
	@printf "$(COLOR_DIM)$(APP_NAME): no UI configured, skipping embed.$(COLOR_RESET)\n"
else
	@if [ -d "$(UI_DIST_SRC)" ]; then \
	  rm -rf $(LOCAL_DIST) && cp -r $(UI_DIST_SRC) $(LOCAL_DIST) ; \
	  printf "$(COLOR_GREEN)$(APP_NAME): embedded $(UI_DIST_SRC) → $(LOCAL_DIST)$(COLOR_RESET)\n" ; \
	else \
	  printf "$(COLOR_RED)$(APP_NAME): no dist at $(UI_DIST_SRC) — build the UI first$(COLOR_RESET)\n" ; \
	  exit 1 ; \
	fi
endif

run: build ## run the binary in foreground with default flags
	$(call STEP,$(APP_NAME): running on $(or $(DEFAULT_PORT),default port))
	@$(RUN_ENV) ./$(APP_NAME) $(RUN_FLAGS)

# `dev` runs the Go server (no-UI build) AND the UI dev server in
# parallel. Implemented in the simplest portable way: parallel make
# invocation. Press Ctrl-C to stop both.
dev: ## run Go server (no UI) + UI dev server in parallel
ifndef UI_PACKAGE
	@$(MAKE) run
else
	@printf "$(COLOR_CYAN)▸ %s: dev mode — Go @ :$(or $(DEFAULT_PORT),9999) + Vite @ :5173$(COLOR_RESET)\n" "$(APP_NAME)"
	@printf "$(COLOR_DIM)To start Vite, in another terminal:  cd $(REPO_ROOT)/ui/packages/$(UI_PACKAGE) && make run$(COLOR_RESET)\n"
	@$(MAKE) -s build-noui
	@$(RUN_ENV) ./$(APP_NAME) $(RUN_FLAGS)
endif

test: ## run unit tests
	$(GO_ENV) go test ./...

test-integration: ## run integration tests (build-tag + env-var gated)
	$(GO_ENV) SOCKERLESS_INTEGRATION=1 go test -tags integration ./...

lint: ## go vet + gofmt check
	$(GO_ENV) go vet ./...
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
	  printf "$(COLOR_RED)gofmt -l .$(COLOR_RESET)\n%s\n" "$$unformatted"; \
	  exit 1; \
	fi

clean: ## remove built binary + dist
	rm -f $(APP_NAME)
	rm -rf $(LOCAL_DIST)
	$(GO_ENV) go clean -testcache 2>/dev/null || true
