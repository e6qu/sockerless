# Standardized Makefile for sockerless-backend-ecs — see docs/MAKEFILE_STANDARD.md.

APP_NAME      := sockerless-backend-ecs
GO_PACKAGE    := ./cmd/sockerless-backend-ecs
UI_PACKAGE    := backend-ecs
DEFAULT_PORT  := 3375
RUN_FLAGS     := --addr :3375
RUN_ENV       := SOCKERLESS_ENDPOINT_URL=http://localhost:4566
REPO_ROOT_REL := ../..

include $(REPO_ROOT_REL)/make/go-app.mk
