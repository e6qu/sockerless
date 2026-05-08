# stack.mk — bring up a working dev stack of (sim + backend + admin)
# with one make target. Each running process gets a PID file in
# .stack-pids/ so `stack-down` can clean up later.
#
# Each `stack-X-Y` target sets STACK_SIM (aws|gcp|azure) and STACK_BE
# (ecs|lambda|cloudrun|gcf|aca|azf), then delegates to `stack-up`.
# The cross-cloud combinations that don't make sense (e.g. aws + gcf)
# aren't exposed.

include $(CURDIR)/make/colors.mk

STACK_PID_DIR := $(CURDIR)/.stack-pids

# Sim defaults — each sim's main package picks its own port. Mirror
# them here so the backend env var points at the right place.
STACK_SIM_PORT_aws   := 4566
STACK_SIM_PORT_gcp   := 4567
STACK_SIM_PORT_azure := 4568

# Backend port is 3375 by convention (every backend defaults to that).
STACK_BE_PORT  := 3375
STACK_ADMIN_PORT := 9090
STACK_BLEEPHUB_PORT := 5555

.PHONY: stack-aws-ecs stack-aws-lambda \
        stack-gcp-cloudrun stack-gcp-gcf \
        stack-azure-aca stack-azure-azf \
        stack-up stack-down stack-status \
        stack-bleephub-up

stack-aws-ecs: ## start sim-aws + backend-ecs + admin
	@$(MAKE) -s stack-up STACK_SIM=aws STACK_BE=ecs STACK_BE_DIR=backends/ecs

stack-aws-lambda: ## start sim-aws + backend-lambda + admin
	@$(MAKE) -s stack-up STACK_SIM=aws STACK_BE=lambda STACK_BE_DIR=backends/lambda

stack-gcp-cloudrun: ## start sim-gcp + backend-cloudrun + admin
	@$(MAKE) -s stack-up STACK_SIM=gcp STACK_BE=cloudrun STACK_BE_DIR=backends/cloudrun

stack-gcp-gcf: ## start sim-gcp + backend-gcf + admin
	@$(MAKE) -s stack-up STACK_SIM=gcp STACK_BE=gcf STACK_BE_DIR=backends/cloudrun-functions

stack-azure-aca: ## start sim-azure + backend-aca + admin
	@$(MAKE) -s stack-up STACK_SIM=azure STACK_BE=aca STACK_BE_DIR=backends/aca

stack-azure-azf: ## start sim-azure + backend-azf + admin
	@$(MAKE) -s stack-up STACK_SIM=azure STACK_BE=azf STACK_BE_DIR=backends/azure-functions

# stack-up — internal target. Build all 3 components, then start them
# as background processes whose PIDs are recorded in .stack-pids/.
stack-up:
	@if [ -z "$(STACK_SIM)" ] || [ -z "$(STACK_BE)" ]; then \
	  echo "stack-up requires STACK_SIM and STACK_BE"; exit 1; \
	fi
	@mkdir -p $(STACK_PID_DIR)
	@printf "$(COLOR_CYAN)▸ Building sim-$(STACK_SIM)$(COLOR_RESET)\n"
	@$(MAKE) -s -C simulators/$(STACK_SIM) build
	@printf "$(COLOR_CYAN)▸ Building backend-$(STACK_BE)$(COLOR_RESET)\n"
	@$(MAKE) -s -C $(STACK_BE_DIR) build
	@printf "$(COLOR_CYAN)▸ Building admin$(COLOR_RESET)\n"
	@$(MAKE) -s -C cmd/sockerless-admin build
	@printf "$(COLOR_CYAN)▸ Starting sim-$(STACK_SIM) on :$(STACK_SIM_PORT_$(STACK_SIM))$(COLOR_RESET)\n"
	@cd simulators/$(STACK_SIM) && \
	  ./simulator-$(STACK_SIM) -addr :$(STACK_SIM_PORT_$(STACK_SIM)) \
	    > $(STACK_PID_DIR)/sim.log 2>&1 & \
	  echo $$! > $(STACK_PID_DIR)/sim.pid
	@sleep 1
	@printf "$(COLOR_CYAN)▸ Starting backend-$(STACK_BE) on :$(STACK_BE_PORT) → http://localhost:$(STACK_SIM_PORT_$(STACK_SIM))$(COLOR_RESET)\n"
	@cd $(STACK_BE_DIR) && \
	  SOCKERLESS_ENDPOINT_URL=http://localhost:$(STACK_SIM_PORT_$(STACK_SIM)) \
	  ./sockerless-backend-$(STACK_BE) --addr :$(STACK_BE_PORT) \
	    > $(STACK_PID_DIR)/backend.log 2>&1 & \
	  echo $$! > $(STACK_PID_DIR)/backend.pid
	@sleep 1
	@printf "$(COLOR_CYAN)▸ Starting admin on :$(STACK_ADMIN_PORT)$(COLOR_RESET)\n"
	@cd cmd/sockerless-admin && \
	  ./sockerless-admin --addr :$(STACK_ADMIN_PORT) \
	    --simulator sim-$(STACK_SIM)=http://localhost:$(STACK_SIM_PORT_$(STACK_SIM)) \
	    --backend backend-$(STACK_BE)=http://localhost:$(STACK_BE_PORT) \
	    > $(STACK_PID_DIR)/admin.log 2>&1 & \
	  echo $$! > $(STACK_PID_DIR)/admin.pid
	@sleep 1
	@printf "\n$(COLOR_GREEN)Stack up:$(COLOR_RESET)\n"
	@printf "  $(COLOR_BOLD)admin UI:$(COLOR_RESET)        http://localhost:$(STACK_ADMIN_PORT)/ui/\n"
	@printf "  $(COLOR_BOLD)backend-$(STACK_BE):$(COLOR_RESET)   http://localhost:$(STACK_BE_PORT)\n"
	@printf "  $(COLOR_BOLD)sim-$(STACK_SIM):$(COLOR_RESET)        http://localhost:$(STACK_SIM_PORT_$(STACK_SIM))\n"
	@printf "  Logs in $(STACK_PID_DIR)/*.log · stop with $(COLOR_BOLD)make stack-down$(COLOR_RESET)\n"

stack-bleephub-up: ## also start bleephub on :5555 (run AFTER a stack-X-Y)
	@if [ ! -d $(STACK_PID_DIR) ]; then \
	  printf "$(COLOR_RED)No stack running — start one first with make stack-aws-ecs (etc).$(COLOR_RESET)\n"; \
	  exit 1; \
	fi
	@printf "$(COLOR_CYAN)▸ Building bleephub$(COLOR_RESET)\n"
	@$(MAKE) -s -C bleephub build
	@printf "$(COLOR_CYAN)▸ Starting bleephub on :$(STACK_BLEEPHUB_PORT)$(COLOR_RESET)\n"
	@cd bleephub && ./bleephub-server -addr :$(STACK_BLEEPHUB_PORT) \
	  > $(STACK_PID_DIR)/bleephub.log 2>&1 & \
	  echo $$! > $(STACK_PID_DIR)/bleephub.pid
	@printf "  $(COLOR_BOLD)bleephub UI:$(COLOR_RESET)    http://localhost:$(STACK_BLEEPHUB_PORT)/ui/\n"

stack-status: ## show running stack components
	@if [ ! -d $(STACK_PID_DIR) ]; then \
	  printf "$(COLOR_DIM)No stack running.$(COLOR_RESET)\n"; \
	  exit 0; \
	fi
	@for pidfile in $(STACK_PID_DIR)/*.pid; do \
	  [ -f "$$pidfile" ] || continue ; \
	  pid=$$(cat $$pidfile); name=$$(basename $$pidfile .pid); \
	  if ps -p $$pid > /dev/null 2>&1; then \
	    printf "  $(COLOR_GREEN)● %-10s$(COLOR_RESET) pid=%s\n" "$$name" "$$pid"; \
	  else \
	    printf "  $(COLOR_RED)○ %-10s$(COLOR_RESET) pid=%s (dead)\n" "$$name" "$$pid"; \
	  fi ; \
	done

stack-down: ## stop all running stack processes
	@if [ ! -d $(STACK_PID_DIR) ]; then \
	  printf "$(COLOR_DIM)No stack running.$(COLOR_RESET)\n"; \
	  exit 0; \
	fi
	@for pidfile in $(STACK_PID_DIR)/*.pid; do \
	  [ -f "$$pidfile" ] || continue ; \
	  pid=$$(cat $$pidfile); name=$$(basename $$pidfile .pid); \
	  kill $$pid 2>/dev/null && \
	    printf "  $(COLOR_DIM)stopped %-10s pid=%s$(COLOR_RESET)\n" "$$name" "$$pid" || \
	    printf "  $(COLOR_DIM)not running %-10s pid=%s$(COLOR_RESET)\n" "$$name" "$$pid" ; \
	  rm -f $$pidfile ; \
	done
	@rm -rf $(STACK_PID_DIR)
	@printf "$(COLOR_GREEN)Stack down.$(COLOR_RESET)\n"
