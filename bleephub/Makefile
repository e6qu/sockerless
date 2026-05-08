# Standardized Makefile for bleephub-server — see docs/MAKEFILE_STANDARD.md.

APP_NAME      := bleephub-server
GO_PACKAGE    := ./cmd
UI_PACKAGE    := bleephub
DEFAULT_PORT  := 5555
RUN_FLAGS     := -addr :5555
GO_ENV        := CGO_ENABLED=0

REPO_ROOT_REL := ..

include $(REPO_ROOT_REL)/make/go-app.mk
