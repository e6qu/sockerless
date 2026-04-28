# Runner-Lambda image build / push / function-update.
#
# Single-shot rebuild of the runner-Lambda image after a backend
# binary change (e.g. BUG-862 architecture correction). Requires:
#   - linux/amd64 sockerless-backend-lambda staged at ./sockerless-backend-lambda
#   - DOCKER_HOST pointing at a daemon that can build linux/amd64
#     (Docker Desktop, Podman VM, sockerless-with-CodeBuild, etc.)
#   - aws CLI authenticated for the target account/region.

REGION       ?= eu-west-1
ACCOUNT_ID   ?= $(shell aws sts get-caller-identity --query Account --output text)
ECR_REPO     ?= sockerless-live
IMAGE_TAG    ?= runner-amd64
FUNCTION     ?= sockerless-live-runner

ECR_URI := $(ACCOUNT_ID).dkr.ecr.$(REGION).amazonaws.com/$(ECR_REPO):$(IMAGE_TAG)

# Cross-compile every linux/amd64 binary the build context needs:
# the lambda backend, the agent, and the lambda bootstrap. The latter
# two get COPYed into /opt/sockerless inside the image so the in-
# Lambda backend's image-inject path can stage them into per-sub-task
# Lambda images. Run before `make build` whenever any backend / agent
# code changes.
stage:
	cd $(CURDIR)/../../../../backends/lambda && \
		GOOS=linux GOARCH=amd64 go build \
		  -o $(CURDIR)/sockerless-backend-lambda \
		  ./cmd/sockerless-backend-lambda
	cd $(CURDIR)/../../../../agent && \
		GOOS=linux GOARCH=amd64 go build \
		  -o $(CURDIR)/sockerless-agent \
		  ./cmd/sockerless-agent
	cd $(CURDIR)/../../../../agent && \
		GOOS=linux GOARCH=amd64 go build \
		  -o $(CURDIR)/sockerless-lambda-bootstrap \
		  ./cmd/sockerless-lambda-bootstrap

# Build linux/amd64 image. The Dockerfile expects
# ./sockerless-backend-lambda to be present (run `make stage` first
# or rely on a pre-staged copy).
build:
	docker buildx build --platform linux/amd64 \
		-t $(ECR_URI) \
		--load \
		.

# Push to ECR. Requires `aws ecr get-login-password` to have run
# against the target region.
push: ecr-login
	docker push $(ECR_URI)

ecr-login:
	aws ecr get-login-password --region $(REGION) | \
		docker login --username AWS --password-stdin $(ACCOUNT_ID).dkr.ecr.$(REGION).amazonaws.com

# Update the runner Lambda function to point at the freshly-pushed
# image. `--publish` cuts a new function version so cell 2 picks up
# the new code without execution-environment cache hangover.
update-function:
	aws lambda update-function-code \
		--function-name $(FUNCTION) \
		--image-uri $(ECR_URI) \
		--publish \
		--region $(REGION) \
		--no-cli-pager

# Wait for the function's UpdateStatus to settle. Lambda image-mode
# updates take 30-60 s before invocations route to the new image;
# without this wait, cell 2's first invocation may hit the old code.
wait:
	aws lambda wait function-updated-v2 \
		--function-name $(FUNCTION) \
		--region $(REGION)

# One-shot. Stage → build → push → update → wait. The end-to-end
# rebuild path; what cell 2 needs after a backend change.
all: stage build push update-function wait
	@echo "runner-Lambda updated to $(ECR_URI)"

.PHONY: stage build push ecr-login update-function wait all
