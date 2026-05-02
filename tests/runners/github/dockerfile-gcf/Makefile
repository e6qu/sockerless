# Build sockerless-runner-gcf multi-arch image.
#
# Cross-compiles sockerless-backend-gcf + sockerless-gcf-bootstrap for
# linux/{amd64,arm64}, builds per-arch images, pushes both, assembles
# manifest list at the suffix-less tag.

REPO            ?= us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live
IMAGE_NAME      ?= runner
TAG_BASE        ?= gcf
IMAGE_AMD64     := $(REPO)/$(IMAGE_NAME):$(TAG_BASE)-amd64
IMAGE_ARM64     := $(REPO)/$(IMAGE_NAME):$(TAG_BASE)-arm64
IMAGE_MULTI     := $(REPO)/$(IMAGE_NAME):$(TAG_BASE)
SOCKERLESS_REPO_ROOT := $(shell cd ../../../../ && pwd)
BACKEND_DIR     := $(SOCKERLESS_REPO_ROOT)/backends/cloudrun-functions/cmd/sockerless-backend-gcf
BOOTSTRAP_DIR   := $(SOCKERLESS_REPO_ROOT)/agent/cmd/sockerless-gcf-bootstrap

.PHONY: stage-amd64 stage-arm64 build-amd64 build-arm64 push-amd64 push-arm64 manifest all

stage-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -C $(BACKEND_DIR)   -tags noui -o $(CURDIR)/sockerless-backend-gcf-amd64 .
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -C $(BOOTSTRAP_DIR)            -o $(CURDIR)/sockerless-gcf-bootstrap-amd64 .

stage-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -C $(BACKEND_DIR)   -tags noui -o $(CURDIR)/sockerless-backend-gcf-arm64 .
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -C $(BOOTSTRAP_DIR)            -o $(CURDIR)/sockerless-gcf-bootstrap-arm64 .

build-amd64: stage-amd64
	cp sockerless-backend-gcf-amd64 sockerless-backend-gcf
	cp sockerless-gcf-bootstrap-amd64 sockerless-gcf-bootstrap
	docker build --platform linux/amd64 -t $(IMAGE_AMD64) .
	rm -f sockerless-backend-gcf sockerless-gcf-bootstrap

build-arm64: stage-arm64
	cp sockerless-backend-gcf-arm64 sockerless-backend-gcf
	cp sockerless-gcf-bootstrap-arm64 sockerless-gcf-bootstrap
	docker build --platform linux/arm64 -t $(IMAGE_ARM64) .
	rm -f sockerless-backend-gcf sockerless-gcf-bootstrap

push-amd64: build-amd64
	docker push $(IMAGE_AMD64)

push-arm64: build-arm64
	docker push $(IMAGE_ARM64)

manifest: push-amd64 push-arm64
	docker manifest rm $(IMAGE_MULTI) 2>/dev/null || true
	docker manifest create $(IMAGE_MULTI) $(IMAGE_AMD64) $(IMAGE_ARM64)
	docker manifest annotate $(IMAGE_MULTI) $(IMAGE_AMD64) --arch amd64 --os linux
	docker manifest annotate $(IMAGE_MULTI) $(IMAGE_ARM64) --arch arm64 --os linux
	docker manifest push $(IMAGE_MULTI)

all: manifest
