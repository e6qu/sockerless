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
STACK_SIM_GRPC_PORT_gcp := 4568

# Backend port is 3375 by convention (every backend defaults to that).
STACK_BE_PORT  := 3375
STACK_ADMIN_PORT := 9090

.PHONY: stack-aws-ecs stack-aws-lambda \
        stack-gcp-cloudrun stack-gcp-gcf \
        stack-azure-aca stack-azure-azf \
        stack-up stack-down stack-status \
        stack-https-up stack-https-down stack-https-status stack-https-ca \
        stack-observability-up stack-observability-down \
        stack-observability-status stack-observability-validate

# Phase 87 — observability stack (OTel Collector + VictoriaLogs +
# Jaeger). All Apache 2.0. Independent from stack-X-Y; either can run
# without the other.
STACK_OBS_PID_DIR := $(STACK_PID_DIR)/observability
STACK_OBS_STATE_DIR := $(CURDIR)/.sockerless-state/observability
STACK_OBS_CONFIG_DIR ?= $(CURDIR)/make/observability-config

# Binary paths — operator can override via env to point at locally
# installed copies. Defaults assume the binaries are on $PATH.
OTELCOL ?= otelcol-contrib
VICTORIALOGS ?= victoria-logs
JAEGER ?= jaeger-all-in-one

# Service ports.
STACK_OBS_OTLP_GRPC := 4317
STACK_OBS_OTLP_HTTP := 4318
STACK_OBS_VICTORIALOGS_UI := 9428
STACK_OBS_JAEGER_UI := 16686
STACK_OBS_JAEGER_OTLP := 4319

# Optional local HTTPS gateway (Caddy). This fronts the existing sim
# HTTP ports; it does not change simulator public API routes.
STACK_HTTPS ?= 0
STACK_HTTPS_NAME := https-gateway
STACK_HTTPS_PORT ?= 8443
STACK_HTTPS_ADMIN_PORT ?= 28443
STACK_HTTPS_STATE_DIR := $(CURDIR)/.sockerless-state/https-gateway
STACK_HTTPS_XDG_DATA_HOME := $(STACK_HTTPS_STATE_DIR)/data
STACK_HTTPS_XDG_CONFIG_HOME := $(STACK_HTTPS_STATE_DIR)/config
STACK_HTTPS_CA_CERT := $(STACK_HTTPS_XDG_DATA_HOME)/caddy/pki/authorities/local/root.crt
STACK_HTTPS_CADDYFILE := $(CURDIR)/make/https-gateway/Caddyfile
CADDY ?= caddy

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

# Cloud lookup per backend (used by stack-up to derive the cloud
# argument for rebuild-component / start-component).
STACK_SIM_CLOUD_ecs       := aws
STACK_SIM_CLOUD_lambda    := aws
STACK_SIM_CLOUD_cloudrun  := gcp
STACK_SIM_CLOUD_gcf       := gcp
STACK_SIM_CLOUD_aca       := azure
STACK_SIM_CLOUD_azf       := azure

# stack-up — internal target. Composes per-component start-component
# calls (from make/components.mk) into the pre-canned 1-sim +
# 1-backend + admin topology. Per-instance orchestration (multiple
# sims / backends / projects) goes through admin directly; this
# macro is the operator shortcut for the single-cell workflow.
# Each component lives at .stack-pids/<name>.
stack-up:
	@if [ -z "$(STACK_SIM)" ] || [ -z "$(STACK_BE)" ]; then \
	  echo "stack-up requires STACK_SIM and STACK_BE"; exit 1; \
	fi
	@$(MAKE) -s rebuild-component KIND=sim CLOUD=$(STACK_SIM)
	@$(MAKE) -s rebuild-component KIND=backend CLOUD=$(STACK_SIM_CLOUD_$(STACK_BE)) BACKEND=$(STACK_BE)
	@printf "$(COLOR_CYAN)▸ Building admin$(COLOR_RESET)\n"
	@$(MAKE) -s -C cmd/sockerless-admin build
	@mkdir -p $(STACK_PID_DIR)
	@rm -f $(STACK_PID_DIR)/sim.env
	@rm -f $(STACK_PID_DIR)/backend.env
	@if [ "$(STACK_HTTPS)" = "1" ]; then \
	  $(MAKE) -s stack-https-up STACK_HTTPS_PORT=$(STACK_HTTPS_PORT); \
	  if [ "$(STACK_SIM)" = "azure" ]; then \
	    { \
	      printf "SIM_AZURE_ARM_EXTERNAL_DATA_PLANE_URLS_JSON='%s'\n" '{"storage":{"blob":"https://{account}.blob.azure.sockerless.localhost:$(STACK_HTTPS_PORT)/","file":"https://{account}.file.azure.sockerless.localhost:$(STACK_HTTPS_PORT)/","queue":"https://{account}.queue.azure.sockerless.localhost:$(STACK_HTTPS_PORT)/","table":"https://{account}.table.azure.sockerless.localhost:$(STACK_HTTPS_PORT)/","web":"https://{account}.web.azure.sockerless.localhost:$(STACK_HTTPS_PORT)/","dfs":"https://{account}.dfs.azure.sockerless.localhost:$(STACK_HTTPS_PORT)/"},"keyVault":"https://{vault}.vault.azure.sockerless.localhost:$(STACK_HTTPS_PORT)/","serviceBus":"https://{namespace}.servicebus.azure.sockerless.localhost:$(STACK_HTTPS_PORT)/","eventGrid":"https://{topic}.eventgrid.azure.sockerless.localhost:$(STACK_HTTPS_PORT)/api/events"}'; \
	    } > $(STACK_PID_DIR)/sim.env; \
	  fi; \
	fi
	@if [ "$(STACK_BE)" = "aca" ]; then \
	  { \
	    printf 'SOCKERLESS_ACA_SUBSCRIPTION_ID=00000000-0000-0000-0000-000000000000\n'; \
	    printf 'SOCKERLESS_ACA_RESOURCE_GROUP=sim-rg\n'; \
	    printf 'SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE=default\n'; \
	    printf 'SOCKERLESS_CALLBACK_URL=ws://localhost:$(STACK_BE_PORT)/v1/aca/reverse\n'; \
	  } > $(STACK_PID_DIR)/backend.env; \
	elif [ "$(STACK_BE)" = "azf" ]; then \
	  { \
	    printf 'SOCKERLESS_AZF_SUBSCRIPTION_ID=00000000-0000-0000-0000-000000000000\n'; \
	    printf 'SOCKERLESS_AZF_RESOURCE_GROUP=sim-rg\n'; \
	    printf 'SOCKERLESS_AZF_STORAGE_ACCOUNT=simstorage\n'; \
	    printf 'SOCKERLESS_CALLBACK_URL=ws://localhost:$(STACK_BE_PORT)/v1/azf/reverse\n'; \
	  } > $(STACK_PID_DIR)/backend.env; \
	elif [ "$(STACK_BE)" = "cloudrun" ]; then \
	  { \
	    printf 'SOCKERLESS_GCR_PROJECT=sim-project\n'; \
	    printf 'SOCKERLESS_GCP_LOGADMIN_ENDPOINT=localhost:$(STACK_SIM_GRPC_PORT_gcp)\n'; \
	  } > $(STACK_PID_DIR)/backend.env; \
	elif [ "$(STACK_BE)" = "gcf" ]; then \
	  { \
	    printf 'SOCKERLESS_GCF_PROJECT=sim-project\n'; \
	  } > $(STACK_PID_DIR)/backend.env; \
	elif [ "$(STACK_BE)" = "lambda" ]; then \
	  { \
	    printf 'SOCKERLESS_LAMBDA_ROLE_ARN=arn:aws:iam::123456789012:role/sockerless-sim-lambda\n'; \
	    printf 'SOCKERLESS_CALLBACK_URL=ws://localhost:$(STACK_BE_PORT)/v1/lambda/reverse\n'; \
	  } > $(STACK_PID_DIR)/backend.env; \
	fi
	@$(MAKE) -s start-component KIND=sim CLOUD=$(STACK_SIM) NAME=sim PORT=$(STACK_SIM_PORT_$(STACK_SIM)) ENV_FILE=$(STACK_PID_DIR)/sim.env
	@sleep 1
	@$(MAKE) -s start-component KIND=backend CLOUD=$(STACK_SIM_CLOUD_$(STACK_BE)) BACKEND=$(STACK_BE) \
	  NAME=backend PORT=$(STACK_BE_PORT) SIM_PORT=$(STACK_SIM_PORT_$(STACK_SIM)) ENV_FILE=$(STACK_PID_DIR)/backend.env
	@sleep 1
	@printf "$(COLOR_CYAN)▸ Starting admin on :$(STACK_ADMIN_PORT)$(COLOR_RESET)\n"
	@( cd cmd/sockerless-admin || exit 1; \
	  nohup ./sockerless-admin --addr :$(STACK_ADMIN_PORT) \
	    --simulator sim-$(STACK_SIM)=http://localhost:$(STACK_SIM_PORT_$(STACK_SIM)) \
	    --backend backend-$(STACK_BE)=http://localhost:$(STACK_BE_PORT) \
	    > $(STACK_PID_DIR)/admin.log 2>&1 < /dev/null & \
	  echo $$! > $(STACK_PID_DIR)/admin.pid )
	@sleep 1
	@printf "\n$(COLOR_GREEN)Stack up:$(COLOR_RESET)\n"
	@printf "  $(COLOR_BOLD)admin UI:$(COLOR_RESET)        http://localhost:$(STACK_ADMIN_PORT)/ui/\n"
	@printf "  $(COLOR_BOLD)backend-$(STACK_BE):$(COLOR_RESET)   http://localhost:$(STACK_BE_PORT)\n"
	@printf "  $(COLOR_BOLD)sim-$(STACK_SIM):$(COLOR_RESET)        http://localhost:$(STACK_SIM_PORT_$(STACK_SIM))\n"
	@if [ "$(STACK_HTTPS)" = "1" ]; then \
	  printf "  $(COLOR_BOLD)HTTPS gateway:$(COLOR_RESET) https://$(STACK_SIM).sockerless.localhost:$(STACK_HTTPS_PORT)\n"; \
	fi
	@printf "  Logs in $(STACK_PID_DIR)/*.log · stop with $(COLOR_BOLD)make stack-down$(COLOR_RESET)\n"

stack-status: ## show running stack components
	@$(MAKE) -s status-components

stack-https-up: ## start optional Caddy HTTPS gateway for simulator APIs
	@case "$(CADDY)" in \
	  */*) [ -x "$(CADDY)" ] ;; \
	  *) command -v $(CADDY) > /dev/null 2>&1 ;; \
	esac || { \
	  printf "$(COLOR_RED)Caddy binary not found — install Caddy or set CADDY=/path/to/caddy$(COLOR_RESET)\n"; \
	  exit 1; \
	}
	@if [ ! -f $(STACK_HTTPS_CADDYFILE) ]; then \
	  printf "$(COLOR_RED)missing $(STACK_HTTPS_CADDYFILE)$(COLOR_RESET)\n"; \
	  exit 1; \
	fi
	@mkdir -p $(STACK_PID_DIR) $(STACK_HTTPS_XDG_DATA_HOME) $(STACK_HTTPS_XDG_CONFIG_HOME)
	@if [ -f $(STACK_PID_DIR)/$(STACK_HTTPS_NAME).pid ] && \
	    pid=$$(cat $(STACK_PID_DIR)/$(STACK_HTTPS_NAME).pid 2>/dev/null) && \
	    ps -p $$pid > /dev/null 2>&1; then \
	  printf "$(COLOR_DIM)$(STACK_HTTPS_NAME): already running (pid=$$pid)$(COLOR_RESET)\n"; \
	  $(MAKE) -s stack-https-status; \
	  exit 0; \
	fi
	@{ \
	  printf 'SOCKERLESS_HTTPS_GATEWAY_PORT=%s\n' "$(STACK_HTTPS_PORT)"; \
	  printf 'SOCKERLESS_HTTPS_GATEWAY_ADMIN_PORT=%s\n' "$(STACK_HTTPS_ADMIN_PORT)"; \
	  printf 'SOCKERLESS_HTTPS_GATEWAY_CA_CERT=%s\n' "$(STACK_HTTPS_CA_CERT)"; \
	  printf 'SOCKERLESS_AWS_SIM_PORT=%s\n' "$(STACK_SIM_PORT_aws)"; \
	  printf 'SOCKERLESS_GCP_SIM_PORT=%s\n' "$(STACK_SIM_PORT_gcp)"; \
	  printf 'SOCKERLESS_AZURE_SIM_PORT=%s\n' "$(STACK_SIM_PORT_azure)"; \
	  printf 'SOCKERLESS_HTTPS_GATEWAY_DEFAULT_SIM_PORT=%s\n' "$(STACK_SIM_PORT_aws)"; \
	} > $(STACK_PID_DIR)/$(STACK_HTTPS_NAME).env
	@rm -f $(STACK_PID_DIR)/$(STACK_HTTPS_NAME).exit
	@printf "$(COLOR_CYAN)▸ Starting Caddy HTTPS gateway on :$(STACK_HTTPS_PORT)$(COLOR_RESET)\n"
	@( XDG_DATA_HOME=$(STACK_HTTPS_XDG_DATA_HOME) \
	   XDG_CONFIG_HOME=$(STACK_HTTPS_XDG_CONFIG_HOME) \
	   SOCKERLESS_HTTPS_GATEWAY_PORT=$(STACK_HTTPS_PORT) \
	   SOCKERLESS_HTTPS_GATEWAY_ADMIN_PORT=$(STACK_HTTPS_ADMIN_PORT) \
	   SOCKERLESS_AWS_SIM_PORT=$(STACK_SIM_PORT_aws) \
	   SOCKERLESS_GCP_SIM_PORT=$(STACK_SIM_PORT_gcp) \
	   SOCKERLESS_AZURE_SIM_PORT=$(STACK_SIM_PORT_azure) \
	   SOCKERLESS_HTTPS_GATEWAY_DEFAULT_SIM_PORT=$(STACK_SIM_PORT_aws) \
	   $(CADDY) run --config $(STACK_HTTPS_CADDYFILE) --adapter caddyfile \
	     > $(STACK_PID_DIR)/$(STACK_HTTPS_NAME).log 2>&1 < /dev/null & \
	   echo $$! > $(STACK_PID_DIR)/$(STACK_HTTPS_NAME).pid )
	@sleep 1
	@pid=$$(cat $(STACK_PID_DIR)/$(STACK_HTTPS_NAME).pid); \
	if ! ps -p $$pid > /dev/null 2>&1; then \
	  printf "$(COLOR_RED)$(STACK_HTTPS_NAME) failed to start. Log:$(COLOR_RESET)\n"; \
	  tail -n 80 $(STACK_PID_DIR)/$(STACK_HTTPS_NAME).log 2>/dev/null || true; \
	  rm -f $(STACK_PID_DIR)/$(STACK_HTTPS_NAME).pid; \
	  exit 1; \
	fi
	@$(MAKE) -s stack-https-status

stack-https-status: ## show local HTTPS gateway status and endpoints
	@if [ -f $(STACK_PID_DIR)/$(STACK_HTTPS_NAME).pid ]; then \
	  pid=$$(cat $(STACK_PID_DIR)/$(STACK_HTTPS_NAME).pid); \
	  if ps -p $$pid > /dev/null 2>&1; then \
	    printf "  $(COLOR_GREEN)● %-30s$(COLOR_RESET) pid=%s\n" "$(STACK_HTTPS_NAME)" "$$pid"; \
	  else \
	    printf "  $(COLOR_RED)○ %-30s$(COLOR_RESET) pid=%s (dead)\n" "$(STACK_HTTPS_NAME)" "$$pid"; \
	  fi; \
	else \
	  printf "  $(COLOR_DIM)$(STACK_HTTPS_NAME): not running$(COLOR_RESET)\n"; \
	fi
	@printf "  $(COLOR_BOLD)AWS:$(COLOR_RESET)   https://aws.sockerless.localhost:$(STACK_HTTPS_PORT)\n"
	@printf "  $(COLOR_BOLD)GCP:$(COLOR_RESET)   https://gcp.sockerless.localhost:$(STACK_HTTPS_PORT)\n"
	@printf "  $(COLOR_BOLD)Azure:$(COLOR_RESET) https://azure.sockerless.localhost:$(STACK_HTTPS_PORT)\n"
	@printf "  $(COLOR_BOLD)CA:$(COLOR_RESET)    $(STACK_HTTPS_CA_CERT)\n"
	@if [ ! -f $(STACK_HTTPS_CA_CERT) ]; then \
	  printf "  $(COLOR_DIM)CA file is created by Caddy after local cert issuance.$(COLOR_RESET)\n"; \
	fi

stack-https-ca: ## print the Caddy local CA certificate path
	@if [ ! -f $(STACK_HTTPS_CA_CERT) ]; then \
	  printf "$(COLOR_RED)CA certificate not found at $(STACK_HTTPS_CA_CERT). Run make stack-https-up first.$(COLOR_RESET)\n"; \
	  exit 1; \
	fi
	@printf "%s\n" "$(STACK_HTTPS_CA_CERT)"

stack-https-down: ## stop optional Caddy HTTPS gateway
	@$(MAKE) -s stop-component NAME=$(STACK_HTTPS_NAME)

# stack-observability-up brings up the OTel collector + VictoriaLogs
# + Jaeger as background processes. PIDs land in
# .stack-pids/observability/ to keep them out of stack-down's
# wholesale sweep — operators iterate on observability independently
# from the cell stack.
stack-observability-up: ## start OTel collector + VictoriaLogs + Jaeger
	@for bin in $(OTELCOL) $(VICTORIALOGS) $(JAEGER); do \
	  if ! command -v $$bin > /dev/null 2>&1; then \
	    printf "$(COLOR_RED)$$bin not on PATH — see docs/OBSERVABILITY.md for install$(COLOR_RESET)\n"; \
	    exit 1; \
	  fi; \
	done
	@mkdir -p $(STACK_OBS_PID_DIR) $(STACK_OBS_STATE_DIR)/logs $(STACK_OBS_STATE_DIR)/traces
	@printf "$(COLOR_CYAN)▸ Starting VictoriaLogs on :$(STACK_OBS_VICTORIALOGS_UI)$(COLOR_RESET)\n"
	@( $(VICTORIALOGS) \
	     -storageDataPath=$(STACK_OBS_STATE_DIR)/logs \
	     -httpListenAddr=:$(STACK_OBS_VICTORIALOGS_UI) \
	     -retentionPeriod=7d \
	     > $(STACK_OBS_PID_DIR)/victorialogs.log 2>&1 & \
	   echo $$! > $(STACK_OBS_PID_DIR)/victorialogs.pid )
	@printf "$(COLOR_CYAN)▸ Starting Jaeger on :$(STACK_OBS_JAEGER_UI)$(COLOR_RESET)\n"
	@( SPAN_STORAGE_TYPE=badger \
	   BADGER_EPHEMERAL=false \
	   BADGER_DIRECTORY_VALUE=$(STACK_OBS_STATE_DIR)/traces/values \
	   BADGER_DIRECTORY_KEY=$(STACK_OBS_STATE_DIR)/traces/keys \
	   BADGER_SPAN_STORE_TTL=72h \
	   $(JAEGER) \
	     --collector.otlp.grpc.host-port=:$(STACK_OBS_JAEGER_OTLP) \
	     --query.http-server.host-port=:$(STACK_OBS_JAEGER_UI) \
	     > $(STACK_OBS_PID_DIR)/jaeger.log 2>&1 & \
	   echo $$! > $(STACK_OBS_PID_DIR)/jaeger.pid )
	@sleep 1
	@printf "$(COLOR_CYAN)▸ Starting OTel Collector (OTLP :$(STACK_OBS_OTLP_GRPC))$(COLOR_RESET)\n"
	@( SOCKERLESS_STATE_PIDS_DIR=$(STACK_PID_DIR) \
	   $(OTELCOL) --config=$(STACK_OBS_CONFIG_DIR)/otel-collector.yaml \
	     > $(STACK_OBS_PID_DIR)/otel-collector.log 2>&1 & \
	   echo $$! > $(STACK_OBS_PID_DIR)/otel-collector.pid )
	@sleep 1
	@printf "\n$(COLOR_GREEN)Observability stack up:$(COLOR_RESET)\n"
	@printf "  $(COLOR_BOLD)VictoriaLogs UI:$(COLOR_RESET)  http://localhost:$(STACK_OBS_VICTORIALOGS_UI)/select/vmui\n"
	@printf "  $(COLOR_BOLD)Jaeger UI:$(COLOR_RESET)        http://localhost:$(STACK_OBS_JAEGER_UI)/search\n"
	@printf "  $(COLOR_BOLD)OTLP gRPC:$(COLOR_RESET)        localhost:$(STACK_OBS_OTLP_GRPC)\n"
	@printf "  $(COLOR_BOLD)OTLP HTTP:$(COLOR_RESET)        localhost:$(STACK_OBS_OTLP_HTTP)\n"
	@printf "  $(COLOR_DIM)Components stay decoupled — set OTEL_EXPORTER_OTLP_ENDPOINT to emit traces.$(COLOR_RESET)\n"
	@printf "  $(COLOR_DIM)Logs in $(STACK_PID_DIR)/*.log auto-scraped by the collector's filelog receiver.$(COLOR_RESET)\n"

stack-observability-down: ## stop the observability stack
	@if [ ! -d $(STACK_OBS_PID_DIR) ]; then \
	  printf "$(COLOR_DIM)No observability stack running.$(COLOR_RESET)\n"; exit 0; \
	fi
	@for pidfile in $(STACK_OBS_PID_DIR)/*.pid; do \
	  [ -f "$$pidfile" ] || continue ; \
	  pid=$$(cat $$pidfile); name=$$(basename $$pidfile .pid); \
	  kill $$pid 2>/dev/null && \
	    printf "  $(COLOR_DIM)stopped %-15s pid=%s$(COLOR_RESET)\n" "$$name" "$$pid" || \
	    printf "  $(COLOR_DIM)not running %-15s pid=%s$(COLOR_RESET)\n" "$$name" "$$pid" ; \
	  rm -f $$pidfile ; \
	done
	@rm -rf $(STACK_OBS_PID_DIR)
	@printf "$(COLOR_GREEN)Observability stack down.$(COLOR_RESET)\n"

stack-observability-status: ## show observability stack PIDs
	@if [ ! -d $(STACK_OBS_PID_DIR) ]; then \
	  printf "$(COLOR_DIM)No observability stack running.$(COLOR_RESET)\n"; exit 0; \
	fi
	@for pidfile in $(STACK_OBS_PID_DIR)/*.pid; do \
	  [ -f "$$pidfile" ] || continue ; \
	  pid=$$(cat $$pidfile); name=$$(basename $$pidfile .pid); \
	  if ps -p $$pid > /dev/null 2>&1; then \
	    printf "  $(COLOR_GREEN)● %-15s$(COLOR_RESET) pid=%s\n" "$$name" "$$pid"; \
	  else \
	    printf "  $(COLOR_RED)○ %-15s$(COLOR_RESET) pid=%s (dead)\n" "$$name" "$$pid"; \
	  fi ; \
	done

# stack-observability-validate is the canonical "is the OTel pipeline
# actually working end-to-end?" check. It assumes the stack is up
# (run `make stack-observability-up` first) and a sockerless component
# is running with OTEL_EXPORTER_OTLP_ENDPOINT pointed at the local
# collector. The target then polls VictoriaLogs and Jaeger until at
# least one log line and one trace span land for the requested service
# (default: sockerless-backend-docker).
#
# Usage:
#   make stack-observability-up
#   OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:$(STACK_OBS_OTLP_HTTP) \
#     make stack-docker-up   # or run any backend manually
#   curl http://localhost:3375/v1/version  # generate at least one request
#   make stack-observability-validate
#
# OBS_VALIDATE_SERVICE overrides the service.name being checked.
# OBS_VALIDATE_TIMEOUT_S sets the per-poll timeout in seconds (default 30).
OBS_VALIDATE_SERVICE  ?= sockerless-backend-docker
OBS_VALIDATE_TIMEOUT_S ?= 30
stack-observability-validate: ## assert telemetry lands in VictoriaLogs + Jaeger
	@if [ ! -d $(STACK_OBS_PID_DIR) ]; then \
	  printf "$(COLOR_RED)Observability stack not running. Run 'make stack-observability-up' first.$(COLOR_RESET)\n"; \
	  exit 1; \
	fi
	@printf "$(COLOR_CYAN)▸ Validating telemetry pipeline for service.name=$(OBS_VALIDATE_SERVICE)$(COLOR_RESET)\n"
	@printf "  $(COLOR_DIM)VictoriaLogs query: http://localhost:$(STACK_OBS_VICTORIALOGS_UI)/select/logsql/query?query=service.name:%22$(OBS_VALIDATE_SERVICE)%22$(COLOR_RESET)\n"
	@deadline=$$(( $$(date +%s) + $(OBS_VALIDATE_TIMEOUT_S) )); \
	 logs_seen=0 ; traces_seen=0 ; \
	 while [ $$(date +%s) -lt $$deadline ]; do \
	   if [ $$logs_seen -eq 0 ]; then \
	     count=$$(curl -fsSG "http://localhost:$(STACK_OBS_VICTORIALOGS_UI)/select/logsql/query" \
	       --data-urlencode "query=service.name:\"$(OBS_VALIDATE_SERVICE)\"" \
	       --data-urlencode "limit=1" 2>/dev/null | wc -l | tr -d ' '); \
	     if [ "$$count" -gt 0 ]; then \
	       printf "  $(COLOR_GREEN)✓ VictoriaLogs: %s log line(s) for $(OBS_VALIDATE_SERVICE)$(COLOR_RESET)\n" "$$count"; \
	       logs_seen=1; \
	     fi; \
	   fi; \
	   if [ $$traces_seen -eq 0 ]; then \
	     traces=$$(curl -fsS "http://localhost:$(STACK_OBS_JAEGER_UI)/api/traces?service=$(OBS_VALIDATE_SERVICE)&limit=1" 2>/dev/null \
	       | grep -c '"traceID"' || true); \
	     if [ "$$traces" -gt 0 ]; then \
	       printf "  $(COLOR_GREEN)✓ Jaeger: %s trace(s) for $(OBS_VALIDATE_SERVICE)$(COLOR_RESET)\n" "$$traces"; \
	       traces_seen=1; \
	     fi; \
	   fi; \
	   if [ $$logs_seen -eq 1 ] && [ $$traces_seen -eq 1 ]; then \
	     printf "$(COLOR_GREEN)Observability pipeline healthy.$(COLOR_RESET)\n"; \
	     exit 0; \
	   fi; \
	   sleep 2; \
	 done; \
	 if [ $$logs_seen -eq 0 ]; then \
	   printf "  $(COLOR_RED)✗ VictoriaLogs: no log lines for $(OBS_VALIDATE_SERVICE) within $(OBS_VALIDATE_TIMEOUT_S)s$(COLOR_RESET)\n"; \
	 fi; \
	 if [ $$traces_seen -eq 0 ]; then \
	   printf "  $(COLOR_RED)✗ Jaeger: no traces for $(OBS_VALIDATE_SERVICE) within $(OBS_VALIDATE_TIMEOUT_S)s$(COLOR_RESET)\n"; \
	 fi; \
	 printf "$(COLOR_RED)Observability pipeline UNHEALTHY. Check $(STACK_OBS_PID_DIR)/*.log + the running component's logs.$(COLOR_RESET)\n"; \
	 exit 1

stack-down: ## stop all running stack processes
	@$(MAKE) -s stop-components
	@rm -rf $(STACK_PID_DIR)
	@printf "$(COLOR_GREEN)Stack down.$(COLOR_RESET)\n"
