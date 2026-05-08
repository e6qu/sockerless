# Standardized Makefile for sockerless-backend-azf — see docs/MAKEFILE_STANDARD.md.

APP_NAME      := sockerless-backend-azf
GO_PACKAGE    := ./cmd/sockerless-backend-azf
UI_PACKAGE    := backend-azf
DEFAULT_PORT  := 3375
RUN_FLAGS     := --addr :3375
RUN_ENV       := SOCKERLESS_ENDPOINT_URL=http://localhost:4568
REPO_ROOT_REL := ../..

include $(REPO_ROOT_REL)/make/go-app.mk
