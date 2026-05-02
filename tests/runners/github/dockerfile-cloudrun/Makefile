# Build sockerless-runner-cloudrun image.
#
# Cross-compiles sockerless-backend-cloudrun for linux/amd64, stages it
# next to the Dockerfile, builds the image, pushes to AR.

REPO     ?= us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live
IMAGE    ?= $(REPO):runner-cloudrun-amd64
SOCKERLESS_REPO_ROOT := $(shell cd ../../../../ && pwd)

.PHONY: stage build push all

stage:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
		-C $(SOCKERLESS_REPO_ROOT)/backends/cloudrun/cmd/sockerless-backend-cloudrun \
		-tags noui \
		-o $(CURDIR)/sockerless-backend-cloudrun .
	@ls -la sockerless-backend-cloudrun

build: stage
	docker build --platform linux/amd64 -t $(IMAGE) .

push: build
	docker push $(IMAGE)

all: push
