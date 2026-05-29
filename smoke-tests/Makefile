# Smoke-test Makefile — wraps run.sh + the per-cloud Dockerfiles.
# Standardized targets: install / build / test / lint / clean / run / upgrade-deps.

REPO_ROOT_REL := ..

include $(REPO_ROOT_REL)/make/help.mk
include $(REPO_ROOT_REL)/make/colors.mk

.PHONY: install build test lint clean run upgrade-deps

install: ## (no install steps — Docker required)
	@command -v docker >/dev/null || { echo "docker not found"; exit 1; }

build test: ## run all smoke tests via run.sh
	./run.sh

run: test ## alias for test

lint: ## (no lint — shell scripts not yet covered by shellcheck)
	@true

clean: ## (no artefacts to clean)
	@true

upgrade-deps: ## (no module dependencies in smoke-tests)
	@true
