# GitHub Actions runner image for sockerless gcf dispatch.
#
# Mirror of dockerfile-cloudrun/Dockerfile but bakes
# `sockerless-backend-gcf` (per BUG-862 — backend matches host
# primitive). The gcf backend's image-inject path needs the
# `sockerless-gcf-bootstrap` binary on disk so it can be COPY'd into
# every per-step Cloud Run Function overlay image (Phase 118d).

FROM ubuntu:22.04

ARG RUNNER_VERSION=2.334.0
ARG RUNNER_ARCH=x64
ARG DOCKER_VERSION=27.3.1
ARG DOCKER_ARCH=x86_64

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      curl ca-certificates tar gzip jq sudo \
      iproute2 libicu70 git \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://download.docker.com/linux/static/stable/${DOCKER_ARCH}/docker-${DOCKER_VERSION}.tgz" \
    | tar xz --strip-components=1 -C /usr/local/bin docker/docker && \
    /usr/local/bin/docker --version

# Pre-built sockerless-backend-gcf + the gcf bootstrap binary the
# image_inject path bakes into every per-step overlay image.
COPY sockerless-backend-gcf      /usr/local/bin/sockerless-backend-gcf
COPY sockerless-gcf-bootstrap    /opt/sockerless/sockerless-gcf-bootstrap
RUN chmod +x /usr/local/bin/sockerless-backend-gcf \
             /opt/sockerless/sockerless-gcf-bootstrap

# actions/runner extracted into /opt/runner.
RUN mkdir -p /opt/runner && \
    curl -fsSL \
      "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-${RUNNER_ARCH}-${RUNNER_VERSION}.tar.gz" \
      | tar xz -C /opt/runner && \
    /opt/runner/bin/installdependencies.sh

RUN useradd -m -u 1001 runner && \
    chown -R runner:runner /opt/runner

COPY bootstrap.sh /usr/local/bin/bootstrap.sh
RUN chmod +x /usr/local/bin/bootstrap.sh

ENV DOCKER_HOST=tcp://localhost:3376
ENV RUNNER_LABELS=sockerless-gcf
ENV SOCKERLESS_GCF_BOOTSTRAP=/opt/sockerless/sockerless-gcf-bootstrap

ENTRYPOINT ["/usr/local/bin/bootstrap.sh"]
