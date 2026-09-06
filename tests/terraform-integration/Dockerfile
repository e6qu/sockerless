FROM golang:1.26-bookworm

# Install terraform, terragrunt, and utilities
RUN apt-get update && apt-get install -y --no-install-recommends \
        curl jq openssl unzip dnsmasq git \
    && rm -rf /var/lib/apt/lists/*

# The Docker CLI the simulators' build slices (Cloud Build, ACR Tasks,
# CodeBuild) drive against the mounted engine.
ARG DOCKER_CLI_VERSION=28.5.1
RUN ARCH=${TARGETARCH:-$(dpkg --print-architecture)} && \
    if [ "$ARCH" = "arm64" ]; then ARCH="aarch64"; else ARCH="x86_64"; fi && \
    curl -fsSL "https://download.docker.com/linux/static/stable/${ARCH}/docker-${DOCKER_CLI_VERSION}.tgz" \
    | tar -xz -C /usr/local/bin --strip-components=1 docker/docker

# The AWS CLI the ECS module's destroy-time sweep runs.
RUN ARCH=${TARGETARCH:-$(dpkg --print-architecture)} && \
    if [ "$ARCH" = "arm64" ]; then ARCH="aarch64"; else ARCH="x86_64"; fi && \
    curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-${ARCH}.zip" -o /tmp/awscli.zip \
    && unzip -q /tmp/awscli.zip -d /tmp && /tmp/aws/install && rm -rf /tmp/aws /tmp/awscli.zip

# act runs the smoke workflow through the backend the way a GitHub Actions
# job would; the same pinned release the upstream act suite builds.
ARG ACT_REF=v0.2.84
RUN git clone --depth 1 --branch ${ACT_REF} https://github.com/nektos/act.git /act \
    && cd /act && go build -o /usr/local/bin/act . \
    && rm -rf /act

# Detect architecture for downloading correct binaries
ARG TARGETARCH

# Install terraform
RUN ARCH=${TARGETARCH:-$(dpkg --print-architecture)} && \
    if [ "$ARCH" = "arm64" ]; then ARCH="arm64"; else ARCH="amd64"; fi && \
    curl -fsSL "https://releases.hashicorp.com/terraform/1.8.5/terraform_1.8.5_linux_${ARCH}.zip" -o /tmp/terraform.zip \
    && unzip /tmp/terraform.zip -d /usr/local/bin/ \
    && rm /tmp/terraform.zip

# Install terragrunt
RUN ARCH=${TARGETARCH:-$(dpkg --print-architecture)} && \
    if [ "$ARCH" = "arm64" ]; then ARCH="arm64"; else ARCH="amd64"; fi && \
    curl -fsSL "https://github.com/gruntwork-io/terragrunt/releases/download/v0.77.22/terragrunt_linux_${ARCH}" -o /usr/local/bin/terragrunt \
    && chmod +x /usr/local/bin/terragrunt

WORKDIR /src
COPY . .

# Pre-download Go module dependencies for faster test runs (the simulators
# come from the pinned sockerless-cloud modules in tests/go.mod).
RUN cd tests && go mod download

# The reverse-agent bootstraps the FaaS and Container Apps backends layer onto
# a workload image (the Cloud Run one also serves Azure Container Apps), built
# for the workload platform.
RUN cd agent && GOWORK=off go mod download && \
    for b in cloudrun gcf azf lambda; do \
        CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /opt/sockerless/sockerless-$b-bootstrap ./cmd/sockerless-$b-bootstrap; \
    done

ENTRYPOINT ["/src/tests/terraform-integration/run-test.sh"]
