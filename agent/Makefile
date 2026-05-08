# Standardized Makefile for sockerless-agent + bootstrap binaries.
# See docs/MAKEFILE_STANDARD.md.
#
# `agent` ships four binaries: sockerless-agent (the runtime agent),
# sockerless-lambda-bootstrap, sockerless-cloudrun-bootstrap, and
# sockerless-gcf-bootstrap. The default `build` target builds the
# agent; `make build-bootstraps` builds all four.

APP_NAME      := sockerless-agent
GO_PACKAGE    := ./cmd/sockerless-agent
GO_ENV        := CGO_ENABLED=0

REPO_ROOT_REL := ..

include $(REPO_ROOT_REL)/make/go-app.mk

# ── Extra targets for the bootstrap binaries ────────────────────────

.PHONY: build-bootstraps build-lambda-bootstrap build-cloudrun-bootstrap build-gcf-bootstrap

build-bootstraps: build-lambda-bootstrap build-cloudrun-bootstrap build-gcf-bootstrap ## build all bootstrap binaries

build-lambda-bootstrap: ## build sockerless-lambda-bootstrap (linux/amd64)
	$(GO_ENV) GOOS=linux GOARCH=amd64 go build -o sockerless-lambda-bootstrap ./cmd/sockerless-lambda-bootstrap

build-cloudrun-bootstrap: ## build sockerless-cloudrun-bootstrap (linux/amd64)
	$(GO_ENV) GOOS=linux GOARCH=amd64 go build -o sockerless-cloudrun-bootstrap ./cmd/sockerless-cloudrun-bootstrap

build-gcf-bootstrap: ## build sockerless-gcf-bootstrap (linux/amd64)
	$(GO_ENV) GOOS=linux GOARCH=amd64 go build -o sockerless-gcf-bootstrap ./cmd/sockerless-gcf-bootstrap
