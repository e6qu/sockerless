# Standardized Makefile for sockerless-backend-gcf — see docs/MAKEFILE_STANDARD.md.

APP_NAME      := sockerless-backend-gcf
GO_PACKAGE    := ./cmd/sockerless-backend-gcf
UI_PACKAGE    := backend-gcf
DEFAULT_PORT  := 3375
RUN_FLAGS     := --addr :3375
RUN_ENV       := SOCKERLESS_ENDPOINT_URL=http://localhost:4567
REPO_ROOT_REL := ../..

include $(REPO_ROOT_REL)/make/go-app.mk
