# GitLab Runner image for sockerless gcf dispatch.
#
# Mirror of dockerfile-cloudrun/Dockerfile but bakes
# `sockerless-backend-gcf` + the gcf bootstrap binary. The runner's
# docker executor routes each step container via DOCKER_HOST=
# tcp://localhost:3376 → sockerless-backend-gcf → Cloud Run Function
# (with Phase 118d pod overlay for `services:`).

FROM ubuntu:22.04

ARG GITLAB_RUNNER_VERSION=v17.5.0
ARG DOCKER_VERSION=27.3.1
ARG DOCKER_ARCH=x86_64

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      curl ca-certificates tar gzip jq sudo \
      iproute2 git \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://download.docker.com/linux/static/stable/${DOCKER_ARCH}/docker-${DOCKER_VERSION}.tgz" \
    | tar xz --strip-components=1 -C /usr/local/bin docker/docker

RUN curl -fsSL "https://gitlab-runner-downloads.s3.amazonaws.com/${GITLAB_RUNNER_VERSION}/binaries/gitlab-runner-linux-amd64" \
      -o /usr/local/bin/gitlab-runner && \
    chmod +x /usr/local/bin/gitlab-runner

COPY sockerless-backend-gcf      /usr/local/bin/sockerless-backend-gcf
COPY sockerless-gcf-bootstrap    /opt/sockerless/sockerless-gcf-bootstrap
RUN chmod +x /usr/local/bin/sockerless-backend-gcf \
             /opt/sockerless/sockerless-gcf-bootstrap

COPY bootstrap.sh /usr/local/bin/bootstrap.sh
RUN chmod +x /usr/local/bin/bootstrap.sh

ENV DOCKER_HOST=tcp://localhost:3376
ENV SOCKERLESS_GCF_BOOTSTRAP=/opt/sockerless/sockerless-gcf-bootstrap

ENTRYPOINT ["/usr/local/bin/bootstrap.sh"]
