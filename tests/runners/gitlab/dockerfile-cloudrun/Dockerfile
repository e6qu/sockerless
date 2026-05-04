# GitLab Runner image for sockerless cloudrun dispatch.
#
# Long-lived (one container, polls GitLab forever, dispatches each
# job to its own docker-executor step container). The runner image
# bakes `sockerless-backend-cloudrun` so the docker executor's
# DOCKER_HOST=tcp://localhost:3375 routes step containers through
# sockerless to Cloud Run Jobs.
#
# Deployed once via `docker run -d` against the operator's
# DOCKER_HOST that points at sockerless-cloudrun. Per BUG-862, the
# runner runs as a sockerless-managed Cloud Run Service (long-lived,
# minScale=1).

FROM ubuntu:22.04

ARG GITLAB_RUNNER_VERSION=v17.5.0
ARG DOCKER_VERSION=27.3.1
ARG DOCKER_ARCH=x86_64

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      curl ca-certificates tar gzip jq sudo \
      iproute2 git socat \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://download.docker.com/linux/static/stable/${DOCKER_ARCH}/docker-${DOCKER_VERSION}.tgz" \
    | tar xz --strip-components=1 -C /usr/local/bin docker/docker && \
    /usr/local/bin/docker --version

# gitlab-runner binary (replaces the omnibus-installer for a slimmer
# image; gitlab-runner is a single static binary).
RUN curl -fsSL "https://gitlab-runner-downloads.s3.amazonaws.com/${GITLAB_RUNNER_VERSION}/binaries/gitlab-runner-linux-amd64" \
      -o /usr/local/bin/gitlab-runner && \
    chmod +x /usr/local/bin/gitlab-runner && \
    /usr/local/bin/gitlab-runner --version

COPY sockerless-backend-cloudrun /usr/local/bin/sockerless-backend-cloudrun
# Phase 122g: cloudrun overlay-injects sockerless-cloudrun-bootstrap
# into every per-step Cloud Run Service image so `docker exec` can
# dispatch via Path B HTTP POST (specs/CLOUD_RESOURCE_MAPPING.md
# § Lesson 8). The bootstrap binary lives on the runner image's disk
# so the in-image cloudrun backend can stage it into Cloud Build
# contexts when building per-image overlays.
COPY sockerless-cloudrun-bootstrap /opt/sockerless/sockerless-cloudrun-bootstrap
RUN chmod +x /usr/local/bin/sockerless-backend-cloudrun \
             /opt/sockerless/sockerless-cloudrun-bootstrap

COPY bootstrap.sh /usr/local/bin/bootstrap.sh
RUN chmod +x /usr/local/bin/bootstrap.sh

ENV DOCKER_HOST=tcp://localhost:3375
ENV SOCKERLESS_CLOUDRUN_BOOTSTRAP=/opt/sockerless/sockerless-cloudrun-bootstrap

ENTRYPOINT ["/usr/local/bin/bootstrap.sh"]
