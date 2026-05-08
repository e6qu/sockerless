# Standardized Makefile for sockerless-backend-lambda — see docs/MAKEFILE_STANDARD.md.

APP_NAME      := sockerless-backend-lambda
GO_PACKAGE    := ./cmd/sockerless-backend-lambda
UI_PACKAGE    := backend-lambda
DEFAULT_PORT  := 3375
RUN_FLAGS     := --addr :3375
RUN_ENV       := SOCKERLESS_ENDPOINT_URL=http://localhost:4566
REPO_ROOT_REL := ../..

include $(REPO_ROOT_REL)/make/go-app.mk
