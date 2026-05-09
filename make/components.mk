# components.mk — per-component lifecycle targets used by admin
# orchestration (Phase 79). Each target operates on ONE component
# instance keyed by NAME; the existing stack-X-Y macros in stack.mk
# compose multiple of these to produce the legacy 1-sim + 1-backend
# tuple.
#
# Conventions:
#   - PID files at .stack-pids/<NAME>.pid
#   - Log files at .stack-pids/<NAME>.log
#   - Per-instance env file at .stack-pids/<NAME>.env (sourced before
#     exec; admin writes here when starting via the topology, operator
#     can write by hand for direct CLI use).
#
# Components stay decoupled: these targets pass exactly the env vars
# each binary already documents. No SOCKERLESS_ADMIN_* vars are
# injected. A component started via these targets behaves identically
# to one started by hand.

include $(CURDIR)/make/colors.mk

COMPONENTS_PID_DIR := $(CURDIR)/.stack-pids

.PHONY: start-component stop-component rebuild-component logs-component \
        status-components stop-components

# component-binary returns the on-disk binary path for KIND/CLOUD/BACKEND.
define component-binary
$(strip $(if $(filter sim,$(1)),simulators/$(2)/simulator-$(2), \
$(if $(filter backend,$(1)),$(call backend-binary-path,$(3)), \
$(if $(filter bleephub,$(1)),bleephub/bleephub-server, \
$(if $(filter frontend-docker,$(1)),frontends/docker/sockerless-frontend-docker, \
$(error component-binary: unknown KIND $(1)))))))
endef

define backend-binary-path
$(strip $(if $(filter ecs,$(1)),backends/ecs/sockerless-backend-ecs, \
$(if $(filter lambda,$(1)),backends/lambda/sockerless-backend-lambda, \
$(if $(filter cloudrun,$(1)),backends/cloudrun/sockerless-backend-cloudrun, \
$(if $(filter gcf,$(1)),backends/cloudrun-functions/sockerless-backend-gcf, \
$(if $(filter aca,$(1)),backends/aca/sockerless-backend-aca, \
$(if $(filter azf,$(1)),backends/azure-functions/sockerless-backend-azf, \
$(error backend-binary-path: unknown BACKEND $(1))))))))
endef

# component-build-dir returns the make -C directory for KIND/CLOUD/BACKEND.
define component-build-dir
$(strip $(if $(filter sim,$(1)),simulators/$(2), \
$(if $(filter backend,$(1)),$(call backend-build-dir,$(3)), \
$(if $(filter bleephub,$(1)),bleephub, \
$(if $(filter frontend-docker,$(1)),frontends/docker, \
$(error component-build-dir: unknown KIND $(1)))))))
endef

define backend-build-dir
$(strip $(if $(filter ecs,$(1)),backends/ecs, \
$(if $(filter lambda,$(1)),backends/lambda, \
$(if $(filter cloudrun,$(1)),backends/cloudrun, \
$(if $(filter gcf,$(1)),backends/cloudrun-functions, \
$(if $(filter aca,$(1)),backends/aca, \
$(if $(filter azf,$(1)),backends/azure-functions, \
$(error backend-build-dir: unknown BACKEND $(1))))))))
endef

# component-flag returns the addr/listen flag the binary expects.
# sims use -addr; backends + admin use --addr; bleephub uses -addr.
define component-flag
$(strip $(if $(filter sim,$(1)),-addr, \
$(if $(filter backend,$(1)),--addr, \
$(if $(filter bleephub,$(1)),-addr, \
$(if $(filter frontend-docker,$(1)),--addr, \
$(error component-flag: unknown KIND $(1)))))))
endef

# start-component starts ONE instance.
#
# Required:
#   KIND=sim|backend|bleephub|frontend-docker
#   NAME=<unique instance name>
#   PORT=<int>
# Required for KIND=sim:
#   CLOUD=aws|gcp|azure
# Required for KIND=backend:
#   CLOUD=aws|gcp|azure
#   BACKEND=ecs|lambda|cloudrun|gcf|aca|azf
# Optional for KIND=backend:
#   SIM_PORT=<int>     (sets SOCKERLESS_ENDPOINT_URL=http://localhost:SIM_PORT)
#   ENV_FILE=<path>    (sourced before exec; admin writes per-instance env here)
#
# Idempotent on PID file presence: refuses to start if .stack-pids/<NAME>.pid
# already points at a live process.
start-component:
	@if [ -z "$(KIND)" ] || [ -z "$(NAME)" ] || [ -z "$(PORT)" ]; then \
	  echo "start-component: KIND, NAME, PORT required"; exit 1; \
	fi
	@mkdir -p $(COMPONENTS_PID_DIR)
	@if [ -f $(COMPONENTS_PID_DIR)/$(NAME).pid ] && \
	    pid=$$(cat $(COMPONENTS_PID_DIR)/$(NAME).pid 2>/dev/null) && \
	    ps -p $$pid > /dev/null 2>&1; then \
	  printf "$(COLOR_DIM)$(NAME): already running (pid=$$pid)$(COLOR_RESET)\n"; exit 0; \
	fi
	@printf "$(COLOR_CYAN)▸ start $(NAME) (kind=$(KIND) port=$(PORT))$(COLOR_RESET)\n"
	@bin=$(call component-binary,$(KIND),$(CLOUD),$(BACKEND)); \
	dir=$(call component-build-dir,$(KIND),$(CLOUD),$(BACKEND)); \
	flag=$(call component-flag,$(KIND)); \
	if [ ! -x $$bin ]; then \
	  printf "$(COLOR_RED)$(NAME): binary $$bin missing — run rebuild-component first$(COLOR_RESET)\n"; \
	  exit 1; \
	fi; \
	envline=""; \
	if [ -n "$(SIM_PORT)" ] && [ "$(KIND)" = "backend" ]; then \
	  envline="$$envline SOCKERLESS_ENDPOINT_URL=http://localhost:$(SIM_PORT)"; \
	fi; \
	if [ -n "$(ENV_FILE)" ] && [ -f "$(ENV_FILE)" ]; then \
	  envline="$$envline $$(grep -v '^#' $(ENV_FILE) | xargs)"; \
	fi; \
	(cd $$dir && \
	  env $$envline ./$$(basename $$bin) $$flag :$(PORT) \
	    > $(COMPONENTS_PID_DIR)/$(NAME).log 2>&1 & \
	  echo $$! > $(COMPONENTS_PID_DIR)/$(NAME).pid)
	@printf "  pid=$$(cat $(COMPONENTS_PID_DIR)/$(NAME).pid) log=$(COMPONENTS_PID_DIR)/$(NAME).log\n"

# stop-component sends SIGTERM to NAME's PID + removes the pidfile.
# No-op if the PID is already dead.
stop-component:
	@if [ -z "$(NAME)" ]; then echo "stop-component: NAME required"; exit 1; fi
	@if [ ! -f $(COMPONENTS_PID_DIR)/$(NAME).pid ]; then \
	  printf "$(COLOR_DIM)$(NAME): no pidfile$(COLOR_RESET)\n"; exit 0; \
	fi
	@pid=$$(cat $(COMPONENTS_PID_DIR)/$(NAME).pid); \
	if ps -p $$pid > /dev/null 2>&1; then \
	  kill $$pid && printf "  stopped $(NAME) pid=$$pid\n"; \
	else \
	  printf "$(COLOR_DIM)$(NAME): pid=$$pid already dead$(COLOR_RESET)\n"; \
	fi; \
	rm -f $(COMPONENTS_PID_DIR)/$(NAME).pid

# rebuild-component runs `make build` in the component's dir. KIND
# (+ CLOUD or BACKEND) determines which dir.
rebuild-component:
	@if [ -z "$(KIND)" ]; then echo "rebuild-component: KIND required"; exit 1; fi
	@dir=$(call component-build-dir,$(KIND),$(CLOUD),$(BACKEND)); \
	printf "$(COLOR_CYAN)▸ rebuild $(KIND) ($$dir)$(COLOR_RESET)\n"; \
	$(MAKE) -s -C $$dir build

# logs-component tails NAME's log. LINES (default 200) controls how
# many lines from the tail.
logs-component:
	@if [ -z "$(NAME)" ]; then echo "logs-component: NAME required"; exit 1; fi
	@logfile=$(COMPONENTS_PID_DIR)/$(NAME).log; \
	if [ ! -f $$logfile ]; then \
	  printf "$(COLOR_DIM)no log for $(NAME)$(COLOR_RESET)\n"; exit 0; \
	fi; \
	tail -n $${LINES:-200} $$logfile

status-components: ## list all per-component PIDs + their state
	@if [ ! -d $(COMPONENTS_PID_DIR) ]; then \
	  printf "$(COLOR_DIM)No components running.$(COLOR_RESET)\n"; exit 0; \
	fi
	@for pidfile in $(COMPONENTS_PID_DIR)/*.pid; do \
	  [ -f "$$pidfile" ] || continue ; \
	  pid=$$(cat $$pidfile); name=$$(basename $$pidfile .pid); \
	  if ps -p $$pid > /dev/null 2>&1; then \
	    printf "  $(COLOR_GREEN)● %-30s$(COLOR_RESET) pid=%s\n" "$$name" "$$pid"; \
	  else \
	    printf "  $(COLOR_RED)○ %-30s$(COLOR_RESET) pid=%s (dead)\n" "$$name" "$$pid"; \
	  fi ; \
	done

stop-components: ## stop every running per-component process
	@if [ ! -d $(COMPONENTS_PID_DIR) ]; then exit 0; fi
	@for pidfile in $(COMPONENTS_PID_DIR)/*.pid; do \
	  [ -f "$$pidfile" ] || continue ; \
	  $(MAKE) -s stop-component NAME=$$(basename $$pidfile .pid) ; \
	done
