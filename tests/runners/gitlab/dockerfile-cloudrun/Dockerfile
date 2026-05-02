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
RUN chmod +x /usr/local/bin/sockerless-backend-cloudrun

COPY bootstrap.sh /usr/local/bin/bootstrap.sh
RUN chmod +x /usr/local/bin/bootstrap.sh

ENV DOCKER_HOST=tcp://localhost:3375

ENTRYPOINT ["/usr/local/bin/bootstrap.sh"]
