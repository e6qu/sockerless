# Standardized Makefile for sockerless-backend-cloudrun — see docs/MAKEFILE_STANDARD.md.

APP_NAME      := sockerless-backend-cloudrun
GO_PACKAGE    := ./cmd/sockerless-backend-cloudrun
UI_PACKAGE    := backend-cloudrun
DEFAULT_PORT  := 3375
RUN_FLAGS     := --addr :3375
RUN_ENV       := SOCKERLESS_ENDPOINT_URL=http://localhost:4567
FAAS_SMOKE_TESTS := ^TestCloudRunFaaSE2ESmoke$
REPO_ROOT_REL := ../..

include $(REPO_ROOT_REL)/make/go-app.mk
