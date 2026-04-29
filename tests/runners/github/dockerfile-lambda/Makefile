# Runner-Lambda image build / push / function-update.
#
# Single-shot rebuild of the runner-Lambda image after a backend
# binary change. Requires:
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
# image. `--publish` cuts a new function version so the next
# invocation picks up the new code without execution-environment
# cache hangover.
update-function:
	aws lambda update-function-code \
		--function-name $(FUNCTION) \
		--image-uri $(ECR_URI) \
		--publish \
		--region $(REGION) \
		--no-cli-pager

# Wait for the function's UpdateStatus to settle. Lambda image-mode
# updates take 30-60 s before invocations route to the new image;
# without this wait, the first invocation may hit the old code.
wait:
	aws lambda wait function-updated-v2 \
		--function-name $(FUNCTION) \
		--region $(REGION)

# One-shot. Stage → build → push → update → wait. The end-to-end
# local-Docker rebuild path after a backend code change.
all: stage build push update-function wait
	@echo "runner-Lambda updated to $(ECR_URI)"

# -----------------------------------------------------------------
# CodeBuild-driven build path (no local docker daemon required).
# Provisions the image via the AWS CodeBuild project provisioned in
# `terraform/modules/lambda/codebuild.tf`. Used when the operator
# doesn't have Docker Desktop / Podman handy locally.
# -----------------------------------------------------------------

CODEBUILD_PROJECT ?= sockerless-live-image-builder
BUILD_BUCKET      ?= sockerless-live-build-context

# Tar the build context (Dockerfile + bootstrap.sh + 3 cross-
# compiled binaries) and upload to the S3 bucket the CodeBuild
# project reads from.
upload-context: stage
	tar --exclude='.gitignore' --exclude='Makefile' --exclude='context.tar.gz' \
		-czf context.tar.gz -C $(CURDIR) .
	@key="build-context/runner-lambda-$$(date +%s).tar.gz" ; \
	echo "Uploading $$key to s3://$(BUILD_BUCKET)/" ; \
	aws s3 cp context.tar.gz "s3://$(BUILD_BUCKET)/$$key" --region $(REGION) --no-cli-pager ; \
	echo "$$key" > .last-context-key
	rm -f context.tar.gz

# Trigger a CodeBuild run that pulls the staged context from S3,
# does `docker build --push`. Blocks until completion (poll every
# 10 s). Use `make codebuild-update` for the full rebuild + Lambda
# update path.
codebuild-build: upload-context
	@key=$$(cat .last-context-key) ; \
	echo "Starting CodeBuild $(CODEBUILD_PROJECT) → $(ECR_URI)" ; \
	build_id=$$(aws codebuild start-build \
		--project-name "$(CODEBUILD_PROJECT)" \
		--region $(REGION) \
		--no-cli-pager \
		--environment-variables-override \
		  "name=BUILD_CONTEXT_KEY,value=$$key,type=PLAINTEXT" \
		  "name=IMAGE_URI,value=$(ECR_URI),type=PLAINTEXT" \
		  "name=TARGET_PLATFORM,value=amd64,type=PLAINTEXT" \
		--query 'build.id' --output text) ; \
	echo "Build id: $$build_id" ; \
	while true ; do \
	  status=$$(aws codebuild batch-get-builds --ids "$$build_id" --region $(REGION) --query 'builds[0].buildStatus' --output text --no-cli-pager) ; \
	  case "$$status" in \
	    SUCCEEDED) echo "Build SUCCEEDED" ; break ;; \
	    FAILED|FAULT|TIMED_OUT|STOPPED) echo "Build $$status" ; exit 1 ;; \
	    *) echo "  status=$$status (polling…)" ; sleep 10 ;; \
	  esac \
	done

# Full CodeBuild path: stage → upload context → CodeBuild → update
# Lambda function to the new image. This is the no-local-docker
# equivalent of `make all`.
codebuild-update: codebuild-build update-function wait
	@echo "runner-Lambda updated to $(ECR_URI) via CodeBuild"

.PHONY: stage build push ecr-login update-function wait all upload-context codebuild-build codebuild-update
