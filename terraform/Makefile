.PHONY: fmt lint validate plan-all apply-all clean help

BACKENDS  := ecs cloudrun aca lambda gcf azf
ENVS      := live simulator

# Build the full list of BACKEND-ENV combinations
ENVIRONMENTS := $(foreach b,$(BACKENDS),$(foreach e,$(ENVS),$(b)-$(e)))

# ──────────────────────────────────────────────
# Formatting & Validation
# ──────────────────────────────────────────────

fmt:
	terraform fmt -recursive

lint:
	terraform fmt -check -recursive

validate:
	@for mod in modules/*/; do \
		echo "Validating $$mod..."; \
		(cd $$mod && terraform init -backend=false -input=false > /dev/null 2>&1 && terraform validate) || exit 1; \
	done

# ──────────────────────────────────────────────
# Per-environment targets: init, plan, apply, destroy
# Usage: make init-ecs-live, make plan-lambda-simulator, etc.
# ──────────────────────────────────────────────

# Parse the backend and env from the target name.
# Convention: make <action>-<backend>-<env>
# e.g., make plan-ecs-live  -> BACKEND=ecs, ENV=live

define check_env
	@backend=$$(echo "$*" | rev | cut -d- -f2- | rev); \
	env=$$(echo "$*" | rev | cut -d- -f1 | rev); \
	dir="environments/$${backend}/$${env}"; \
	if [ ! -d "$$dir" ]; then \
		echo "Error: Unknown environment '$*'."; \
		echo "Valid environments: $(ENVIRONMENTS)"; \
		exit 1; \
	fi
endef

init-%:
	$(call check_env)
	@backend=$$(echo "$*" | rev | cut -d- -f2- | rev); \
	env=$$(echo "$*" | rev | cut -d- -f1 | rev); \
	cd "environments/$${backend}/$${env}" && terragrunt init

plan-%:
	$(call check_env)
	@backend=$$(echo "$*" | rev | cut -d- -f2- | rev); \
	env=$$(echo "$*" | rev | cut -d- -f1 | rev); \
	cd "environments/$${backend}/$${env}" && terragrunt plan

apply-%:
	$(call check_env)
	@backend=$$(echo "$*" | rev | cut -d- -f2- | rev); \
	env=$$(echo "$*" | rev | cut -d- -f1 | rev); \
	cd "environments/$${backend}/$${env}" && terragrunt apply

destroy-%:
	$(call check_env)
	@backend=$$(echo "$*" | rev | cut -d- -f2- | rev); \
	env=$$(echo "$*" | rev | cut -d- -f1 | rev); \
	cd "environments/$${backend}/$${env}" && terragrunt destroy

# ──────────────────────────────────────────────
# Bulk operations (all environments)
# ──────────────────────────────────────────────

plan-all:
	cd environments && terragrunt run-all plan

apply-all:
	cd environments && terragrunt run-all apply

# ──────────────────────────────────────────────
# Cleanup
# ──────────────────────────────────────────────

clean:
	find . -type d -name .terragrunt-cache -exec rm -rf {} +
	find . -type d -name .terraform -exec rm -rf {} +
	find . -type f -name .terraform.lock.hcl -delete

# ──────────────────────────────────────────────
# Help
# ──────────────────────────────────────────────

help:
	@echo "Sockerless Terraform/Terragrunt Makefile"
	@echo ""
	@echo "Formatting:"
	@echo "  make fmt                  Auto-format all .tf files"
	@echo "  make lint                 Check formatting (CI-friendly)"
	@echo "  make validate             Validate all modules"
	@echo ""
	@echo "Per-environment (replace BACKEND with ecs|cloudrun|aca|lambda|gcf|azf"
	@echo "                 and ENV with live|simulator):"
	@echo "  make init-BACKEND-ENV     Initialize an environment"
	@echo "  make plan-BACKEND-ENV     Preview changes"
	@echo "  make apply-BACKEND-ENV    Apply changes"
	@echo "  make destroy-BACKEND-ENV  Destroy resources"
	@echo ""
	@echo "Examples:"
	@echo "  make init-ecs-live"
	@echo "  make plan-ecs-simulator"
	@echo "  make apply-lambda-live"
	@echo "  make destroy-cloudrun-simulator"
	@echo ""
	@echo "Bulk operations:"
	@echo "  make plan-all             Plan all environments"
	@echo "  make apply-all            Apply all environments"
	@echo ""
	@echo "Cleanup:"
	@echo "  make clean                Remove .terragrunt-cache, .terraform dirs, and lock files"
