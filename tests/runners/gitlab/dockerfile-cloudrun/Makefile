# Build sockerless-gitlab-runner-cloudrun image.

REPO     ?= us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live
IMAGE    ?= $(REPO):gitlab-runner-cloudrun-amd64
SOCKERLESS_REPO_ROOT := $(shell cd ../../../../ && pwd)

.PHONY: stage build push all

stage:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
		-C $(SOCKERLESS_REPO_ROOT)/backends/cloudrun/cmd/sockerless-backend-cloudrun \
		-tags noui \
		-o $(CURDIR)/sockerless-backend-cloudrun .

build: stage
	docker build --platform linux/amd64 -t $(IMAGE) .

push: build
	docker push $(IMAGE)

all: push
