# Build sockerless-gitlab-runner-cloudrun multi-arch image.

REPO            ?= us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live
IMAGE_NAME      ?= gitlab-runner
TAG_BASE        ?= cloudrun
IMAGE_AMD64     := $(REPO)/$(IMAGE_NAME):$(TAG_BASE)-amd64
IMAGE_ARM64     := $(REPO)/$(IMAGE_NAME):$(TAG_BASE)-arm64
IMAGE_MULTI     := $(REPO)/$(IMAGE_NAME):$(TAG_BASE)
SOCKERLESS_REPO_ROOT := $(shell cd ../../../../ && pwd)
BACKEND_DIR     := $(SOCKERLESS_REPO_ROOT)/backends/cloudrun/cmd/sockerless-backend-cloudrun

.PHONY: stage-amd64 stage-arm64 build-amd64 build-arm64 push-amd64 push-arm64 manifest all

stage-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -C $(BACKEND_DIR) -tags noui -o $(CURDIR)/sockerless-backend-cloudrun-amd64 .

stage-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -C $(BACKEND_DIR) -tags noui -o $(CURDIR)/sockerless-backend-cloudrun-arm64 .

build-amd64: stage-amd64
	cp sockerless-backend-cloudrun-amd64 sockerless-backend-cloudrun
	docker build --platform linux/amd64 -t $(IMAGE_AMD64) .
	rm -f sockerless-backend-cloudrun

build-arm64: stage-arm64
	cp sockerless-backend-cloudrun-arm64 sockerless-backend-cloudrun
	docker build --platform linux/arm64 -t $(IMAGE_ARM64) .
	rm -f sockerless-backend-cloudrun

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
