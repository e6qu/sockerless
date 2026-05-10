# Makefile standardization — proposal

Draft. Reviewable before any code lands. Goal: every independently-buildable app in this repo has its own Makefile with a consistent target surface, and the top-level Makefile delegates to them — no duplicated build commands.

## Why

Today's top-level Makefile (481 lines, 93 targets) hard-codes every `cd <dir> && go build …` recipe inline. Adding a backend or a sim means editing the global Makefile in 3–6 places. Per-app Makefiles + a thin top-level orchestrator fixes that:

- Anyone hacking on `backends/ecs` runs `make build` from inside that dir, no `cd ../..; make build-ecs-with-ui` ceremony.
- `simulators/azure/Makefile` documents how that one sim builds + runs, in the place a developer would look first.
- The top-level Makefile is a list of apps + a delegation rule, not a 481-line script.
- New backends/sims/UI packages: drop in a leaf Makefile (3 lines) — no top-level edit needed when discovery is glob-based.

## Inventory of independently-buildable apps

19 leaf Makefiles total. Three kinds:

### Go binaries with optional embedded UI (12)

| App | Binary | UI package consumed | Default port |
|---|---|---|---|
| `cmd/sockerless-admin` | `sockerless-admin` | `ui/packages/admin` | `:9090` |
| `bleephub` | `bleephub-server` | `ui/packages/bleephub` | `:5555` |
| `backends/docker` | `sockerless-backend-docker` | `ui/packages/backend-docker` | `:3375` |
| `backends/ecs` | `sockerless-backend-ecs` | `ui/packages/backend-ecs` | `:3375` |
| `backends/lambda` | `sockerless-backend-lambda` | `ui/packages/backend-lambda` | `:3375` |
| `backends/cloudrun` | `sockerless-backend-cloudrun` | `ui/packages/backend-cloudrun` | `:3375` |
| `backends/cloudrun-functions` | `sockerless-backend-gcf` | `ui/packages/backend-gcf` | `:3375` |
| `backends/aca` | `sockerless-backend-aca` | `ui/packages/backend-aca` | `:3375` |
| `backends/azure-functions` | `sockerless-backend-azf` | `ui/packages/backend-azf` | `:3375` |
| `simulators/aws` | `simulator-aws` | `ui/packages/simulator-aws` | `:4566` |
| `simulators/gcp` | `simulator-gcp` | `ui/packages/simulator-gcp` | `:4567` |
| `simulators/azure` | `simulator-azure` | `ui/packages/simulator-azure` | `:4568` |

### Go binaries (no UI) (5)

| App | Binary |
|---|---|
| `cmd/sockerless` | `sockerless` (CLI) |
| `agent` | `sockerless-agent`, `sockerless-lambda-bootstrap`, `sockerless-cloudrun-bootstrap`, `sockerless-gcf-bootstrap` |
| `github-runner-dispatcher-aws` | dispatcher binary |
| `github-runner-dispatcher-gcp` | dispatcher binary |
| `github-runner-dispatcher-azure` | dispatcher binary |

### UI packages (13)

| Package | Embeds into |
|---|---|
| `ui/packages/admin` | `cmd/sockerless-admin` |
| `ui/packages/bleephub` | `bleephub` |
| `ui/packages/backend-{docker,ecs,lambda,cloudrun,gcf,aca,azf}` | corresponding backend |
| `ui/packages/simulator-{aws,gcp,azure}` | corresponding simulator |
| `ui/packages/frontend-docker` | (standalone — no Go binary embed) |
| `ui/packages/core` | (shared lib — no embed) |

## Standard target surface

Every leaf Makefile MUST implement these 7 targets. Targets that don't apply to a given kind (e.g. `run` on a UI package) call `@echo "n/a for this app type"` and exit 0 — the contract is "the target name always exists."

| Target | What it does |
|---|---|
| `help` | Print one-line description of every target in this Makefile. Default goal. |
| `install` | Fetch deps (`go mod download` / `bun install`). Idempotent. |
| `build` | Produce the artefact. For Go-with-UI apps, `build` embeds the UI; use `build-noui` to skip. |
| `test` | Run unit tests. Fast — must complete in under a minute on a clean cache. |
| `lint` | Static checks (`go vet` + `gofmt -l` + UI: `tsc --noEmit`). Non-zero exit on findings. |
| `run` | Run the binary in the foreground with sensible default flags. UI packages run `vite dev`. |
| `clean` | Delete build artefacts owned by this app (binary, `dist/`, `.test` caches). |

Optional targets, when meaningful:

| Target | Applies to | What it does |
|---|---|---|
| `build-noui` | Go-with-UI apps | Build the binary with `-tags noui`, no embedded UI. |
| `embed` | Go-with-UI apps | Build the UI + copy `ui/packages/<x>/dist` → local `dist/`. Implicit dep of `build`. |
| `test-integration` | apps with `_integration_test.go` | Run the build-tag-gated integration tests. |
| `dev` | Go-with-UI apps | Run Go server (`-tags noui`) + Vite dev server in parallel. |
| `preview` | UI packages | `vite preview` — serve the built bundle locally. |
| `start` / `stop` | Go binaries | Background daemonization with PID file. (Optional — see "stack" below.) |

## Per-app Makefile shape

Each leaf Makefile is small — 5–10 lines of variables + one `include`. Example:

```make
# backends/ecs/Makefile

APP_NAME      := sockerless-backend-ecs
GO_PACKAGE    := ./cmd/sockerless-backend-ecs
UI_PACKAGE    := backend-ecs
DEFAULT_PORT  := 3375
RUN_FLAGS     := --addr :$(DEFAULT_PORT)

include ../../make/go-app.mk
```

```make
# ui/packages/admin/Makefile

UI_PACKAGE := admin
DEV_PORT   := 5173

include ../../../make/ui-app.mk
```

```make
# simulators/aws/Makefile

APP_NAME      := simulator-aws
GO_PACKAGE    := .
UI_PACKAGE    := simulator-aws
DEFAULT_PORT  := 4566
GO_FLAGS      := GOWORK=off       # this module is outside the workspace
RUN_FLAGS     := -addr :$(DEFAULT_PORT)

include ../../make/go-app.mk
```

Convention: leaf Makefiles only carry **data** (the table above). All recipe code lives in `make/*.mk`.

## Shared `make/` includes

```
make/
├── colors.mk         # Pretty output: $(CYAN), $(GREEN), $(RESET) helpers
├── go-app.mk         # Recipes for Go-binary-with-optional-UI apps
├── go-lib.mk         # Recipes for Go libraries (test/lint/clean only)
├── ui-app.mk         # Recipes for UI packages
└── stack.mk          # Stack-orchestration recipes used by top-level
```

`go-app.mk` outline:

```make
$(APP_NAME): build  ## build the binary

build: embed
	go build $(GO_BUILD_FLAGS) -o $(APP_NAME) $(GO_PACKAGE)

build-noui:
	go build -tags noui $(GO_BUILD_FLAGS) -o $(APP_NAME) $(GO_PACKAGE)

embed:
	$(MAKE) -C $(UI_PKG_DIR) build
	rm -rf dist && cp -r $(UI_PKG_DIR)/dist dist

run: build
	./$(APP_NAME) $(RUN_FLAGS)

dev:
	$(MAKE) -j2 dev-server dev-ui

dev-server: build-noui
	./$(APP_NAME) $(RUN_FLAGS)

dev-ui:
	$(MAKE) -C $(UI_PKG_DIR) run

test:
	go test ./...

test-integration:
	go test -tags integration ./...

lint:
	go vet ./...
	gofmt -l . | tee /dev/stderr | (! read)

install:
	go mod download

clean:
	rm -f $(APP_NAME) ; rm -rf dist
	go clean -testcache

help:
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-18s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
```

`ui-app.mk` outline:

```make
build:
	bun run build

run dev:
	bun run dev

preview: build
	bun run preview

test:
	bun run test

lint:
	bunx tsc --noEmit

install:
	bun install --cwd $(REPO_ROOT)/ui

clean:
	rm -rf dist node_modules .turbo

help:
	@awk … (same)
```

## Top-level Makefile (refactored)

Drops 95% of the existing line count. Becomes a delegation table + stack orchestration.

```make
# Apps — auto-discovered. Drop a Makefile in any of these dirs and it
# joins the rebuild/test/lint set automatically.

GO_APPS := \
  cmd/sockerless cmd/sockerless-admin bleephub agent \
  backends/docker backends/ecs backends/lambda \
  backends/cloudrun backends/cloudrun-functions \
  backends/aca backends/azure-functions \
  simulators/aws simulators/gcp simulators/azure \
  github-runner-dispatcher-aws \
  github-runner-dispatcher-gcp \
  github-runner-dispatcher-azure

UI_APPS := $(wildcard ui/packages/*)

ALL_APPS := $(GO_APPS) $(UI_APPS)

.DEFAULT_GOAL := help

# Per-target fan-out. `make build` builds everything. `make test`
# tests everything. Etc.
build test lint clean install:
	@for app in $(ALL_APPS); do \
	  printf "\n=== $$app: $@ ===\n"; \
	  $(MAKE) -C $$app $@ || exit $$?; \
	done

# Per-app delegation: `make ecs.build` → `make -C backends/ecs build`.
# Naming uses dots so it parses cleanly in shells + ides.
%.build %.test %.lint %.run %.dev %.clean %.embed:
	@app=$(word 1, $(subst ., ,$*)); \
	target=$(word 2, $(subst ., ,$*)); \
	dir=$$(make/find-app.sh $$app); \
	$(MAKE) -C $$dir $$target

# Stack orchestration — see make/stack.mk
include make/stack.mk

help:
	@cat docs/MAKEFILE_STANDARD.md   # or a generated summary
```

## Stack orchestration

The killer feature. `make stack-aws-ecs` brings up a working dev stack for one cloud-backend pair:

```make
# make/stack.mk

STACK_PID_DIR := .stack-pids

# Each stack target = simulator + backend + admin (+ bleephub optional).
# The 6 supported pairs:

stack-aws-ecs:        STACK_SIM=aws      STACK_BE=ecs       stack-up
stack-aws-lambda:     STACK_SIM=aws      STACK_BE=lambda    stack-up
stack-gcp-cloudrun:   STACK_SIM=gcp      STACK_BE=cloudrun  stack-up
stack-gcp-gcf:        STACK_SIM=gcp      STACK_BE=gcf       stack-up
stack-azure-aca:      STACK_SIM=azure    STACK_BE=aca       stack-up
stack-azure-azf:      STACK_SIM=azure    STACK_BE=azf       stack-up

stack-up:
	mkdir -p $(STACK_PID_DIR)
	@echo "→ starting simulator-$(STACK_SIM) on its default port"
	@$(MAKE) -C simulators/$(STACK_SIM) build-noui
	@simulators/$(STACK_SIM)/simulator-$(STACK_SIM) & echo $$! > $(STACK_PID_DIR)/sim.pid
	@echo "→ starting backend-$(STACK_BE) pointed at sim"
	@$(MAKE) -C backends/$(STACK_BE) build-noui
	@SOCKERLESS_ENDPOINT_URL=http://localhost:<sim-port> \
	   backends/$(STACK_BE)/sockerless-backend-$(STACK_BE) & \
	   echo $$! > $(STACK_PID_DIR)/backend.pid
	@echo "→ starting admin server with both registered"
	@$(MAKE) -C cmd/sockerless-admin build
	@cmd/sockerless-admin/sockerless-admin \
	   --simulator sim-$(STACK_SIM)=http://localhost:<sim-port> \
	   --backend $(STACK_BE)=http://localhost:3375 & \
	   echo $$! > $(STACK_PID_DIR)/admin.pid
	@echo
	@echo "Stack up:"
	@echo "  simulator-$(STACK_SIM)  http://localhost:<sim-port>"
	@echo "  backend-$(STACK_BE)     http://localhost:3375"
	@echo "  admin UI                http://localhost:9090/ui/"
	@echo "  Stop with: make stack-down"

stack-down:
	@for pidfile in $(STACK_PID_DIR)/*.pid; do \
	  [ -f $$pidfile ] || continue; \
	  pid=$$(cat $$pidfile); \
	  kill $$pid 2>/dev/null || true; \
	  rm $$pidfile; \
	done
	@rmdir $(STACK_PID_DIR) 2>/dev/null || true
	@echo "Stack down."

stack-status:
	@for pidfile in $(STACK_PID_DIR)/*.pid 2>/dev/null; do \
	  pid=$$(cat $$pidfile); \
	  ps -p $$pid >/dev/null && echo "$$pidfile: $$pid (alive)" || echo "$$pidfile: $$pid (dead)"; \
	done
```

Plus `make stack-bleephub-up` to optionally add bleephub on `:5555`.

> **As-implemented note (Phase 79).** The pre-canned `stack-X-Y` macros above survive and behave the same way for operators, but their bodies have been rewritten to compose per-component targets from `make/components.mk` (`make start-component KIND=… NAME=… PORT=…` etc). See `docs/ADMIN_ORCHESTRATION.md` for the per-component lifecycle surface admin uses to spawn arbitrary topologies (0..N of every kind across multiple projects). PID + log files are now keyed by component NAME (`.stack-pids/<NAME>.{pid,log}`) rather than by role, so admin can manage multiple sims / backends / bleephubs side by side.

## Migration plan

1. Land `make/` directory with `colors.mk`, `go-app.mk`, `go-lib.mk`, `ui-app.mk`, `stack.mk` first.
2. Add the 19 leaf Makefiles in one commit (each is 5–10 lines).
3. Rewrite top-level `Makefile` to delegate. Keep the existing `sim-test-*`, `e2e-*`, `tf-int-*`, `smoke-*`, `upstream-test-*` targets (they're orthogonal cross-cutting concerns) but rename them to fit the dotted-prefix convention if the user wants — or leave them as-is for now.
4. Add CI smoke-test that runs `make help` (validates that every leaf Makefile is wired correctly) + `make build` (validates the whole rebuild path).

## Discussion points before I start

1. **Per-app `run` defaults**. Each app's `run` will start the binary with its default port. Acceptable? Or do we want each `run` to also point at an upstream sim by env-var (i.e. `make run` in `backends/ecs` auto-detects sim availability + sets `SOCKERLESS_ENDPOINT_URL`)?

2. **`build` defaults to with-UI**. The current top-level Makefile has `build-X-with-ui` as the deliberate variant; here I'm proposing `build` = with-UI + `build-noui` for the lean variant. Reverse this? My thinking: a fresh `make build` should produce a runnable binary out of the box, and "runnable" means having the UI baked in for the dashboard pages.

3. **Stack target naming**. `stack-aws-ecs` vs `stack-up CLOUD=aws BE=ecs` vs `aws-ecs.up`. Pick one.

4. **Orthogonal test categories**. The existing `sim-test-*` (sim-vs-backend integration), `tf-int-test-*` (terraform), `smoke-test-*` (Docker-in-Docker smoke), `upstream-test-*` (act + gitlab-ci-local), `e2e-*` (real-runner) — these are cross-cutting. Keep them as top-level targets, OR move each to live next to the test code (`tests/upstream/Makefile`, `tests/runners/Makefile`)?

5. **Auto-discovery vs explicit listing for `GO_APPS`**. Globbing (`backends/*/`, `simulators/*/`) is more robust as new apps land but obscures what's wired up. Explicit listing in `Makefile` is verbose but greppable. Currently I'm proposing explicit. Override?

6. **`help` output format**. Auto-generated from `## comments` in each Makefile (concise, lives with the recipe), or hand-curated table in this doc (richer, drifts more)? I propose auto-generated.

Once you sign off (or course-correct), I'll land the migration in a single commit on `phase-130`.
