# GitHub Actions runner image for sockerless Lambda dispatch.
#
# Wraps `actions/runner` as a Lambda function via the AWS Lambda
# Runtime API. Each Lambda invocation = one ephemeral runner that
# polls GitHub for one job, runs it, and exits. Sockerless-backend-
# **lambda** is baked alongside the runner; the bootstrap starts it
# on localhost:3375 so the runner's docker create/exec calls dispatch
# **as Lambda invocations** (one per `container:` sub-task) — keeping
# the lambda-runner workflow on Lambda primitives end-to-end. (Project
# rule: backend ↔ host primitive must match. ECS in ECS, Lambda in
# Lambda.)
#
# Lambda 15-minute hard cap means cell 2 is restricted to short
# workflows.

FROM public.ecr.aws/lambda/provided:al2

ARG RUNNER_VERSION=2.334.0
# RUNNER_ARCH controls the actions/runner binary fetched: x64 for
# amd64 lambdas, arm64 for arm64 lambdas. Use a non-TARGETARCH name
# so podman's --platform doesn't override.
ARG RUNNER_ARCH=x64
ARG DOCKER_VERSION=27.3.1
ARG DOCKER_ARCH=x86_64

# AL2 bundles `yum`. Install the runtime deps the runner needs +
# docker CLI for nested container ops + iproute (so sockerless's
# metadata-only network driver code is happy if it ever falls back).
RUN yum install -y \
      curl ca-certificates tar gzip jq sudo shadow-utils \
      iproute libicu git \
    && yum clean all

# Static docker CLI — pure client. Talks to localhost:3375 (sockerless)
# inside the same Lambda execution environment.
RUN curl -fsSL "https://download.docker.com/linux/static/stable/${DOCKER_ARCH}/docker-${DOCKER_VERSION}.tgz" \
    | tar xz --strip-components=1 -C /usr/local/bin docker/docker && \
    /usr/local/bin/docker --version

# Pre-built sockerless-backend-lambda (cross-compiled linux/amd64).
# Sockerless inside the Lambda dispatches each `container:` sub-task
# as a separate Lambda invocation (image-mode container Lambda
# created on demand); workflows stay entirely on Lambda primitives.
COPY sockerless-backend-lambda /usr/local/bin/sockerless-backend-lambda
RUN chmod +x /usr/local/bin/sockerless-backend-lambda

# Sockerless's image-inject path needs the sockerless-agent and
# sockerless-lambda-bootstrap binaries available locally so it can
# COPY them into per-sub-task Lambda images at build time. Defaults
# read from /opt/sockerless/* (see backends/lambda/config.go).
RUN mkdir -p /opt/sockerless
COPY sockerless-agent              /opt/sockerless/sockerless-agent
COPY sockerless-lambda-bootstrap   /opt/sockerless/sockerless-lambda-bootstrap
RUN chmod +x /opt/sockerless/sockerless-agent /opt/sockerless/sockerless-lambda-bootstrap

# actions/runner extracted into /opt/runner. Lambda's filesystem is
# read-only except /tmp + EFS mount points; the runner does its
# config + state writes under /tmp at runtime.
RUN mkdir -p /opt/runner && \
    curl -fsSL \
      "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-${RUNNER_ARCH}-${RUNNER_VERSION}.tar.gz" \
      | tar xz -C /opt/runner

# Stage externals to a non-mount location so the Lambda EFS mount
# at /home/runner/externals (provisioned via FileSystemConfig)
# doesn't hide the runner-shipped JS runtime. Bootstrap rsyncs on
# first invocation. Same shape as the ECS runner image.
RUN cp -a /opt/runner/externals /opt/runner-externals.staged

COPY bootstrap.sh /var/runtime/bootstrap
RUN chmod +x /var/runtime/bootstrap

LABEL com.sockerless.lambda.runner=true

# Lambda's base image expects /var/runtime/bootstrap.
ENTRYPOINT ["/var/runtime/bootstrap"]
