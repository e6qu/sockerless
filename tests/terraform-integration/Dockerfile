FROM golang:1.26-bookworm

# Install terraform, terragrunt, and utilities
RUN apt-get update && apt-get install -y --no-install-recommends \
        curl jq openssl unzip dnsmasq git iproute2 nftables \
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
# The Azure CLI runs the Azure Container Apps and Azure Functions modules'
# destroy-time sweep of the resources the backends create, as it does for an
# operator's destroy against the real cloud; the harness registers the
# simulator as a cloud and logs in with the service principal.
ARG AZURE_CLI_VERSION=2.90.0
RUN curl -fsSL https://packages.microsoft.com/keys/microsoft.asc \
        | gpg --dearmor -o /usr/share/keyrings/microsoft.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/microsoft.gpg] https://packages.microsoft.com/repos/azure-cli/ bookworm main" \
        > /etc/apt/sources.list.d/azure-cli.list \
    && apt-get update && apt-get install -y --no-install-recommends "azure-cli=${AZURE_CLI_VERSION}-1~bookworm" \
    && rm -rf /var/lib/apt/lists/*
# The Container Apps commands live in a CLI extension; it is installed outside
# the CLI's configuration directory so the harness's per-run configuration
# (cloud registration and login) still finds it.
ENV AZURE_EXTENSION_DIR=/opt/azure-cli-extensions
RUN az extension add --name containerapp --yes

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
# come from the pinned sockerless-cloud modules in tests/go.mod). The module
# proxy and the checksum database occasionally drop a stream mid-transfer;
# a download resumes from the module cache, so a refused attempt is retried
# with a growing wait before the build fails.
RUN cd tests && for attempt in 1 2 3; do \
        go mod download && break; \
        [ "$attempt" = 3 ] && exit 1; \
        sleep $((attempt * 15)); \
    done

# The reverse-agent bootstraps the FaaS and Container Apps backends layer onto
# a workload image (the Cloud Run one also serves Azure Container Apps), built
# for the workload platform.
RUN cd agent && for attempt in 1 2 3; do \
        GOWORK=off go mod download && break; \
        [ "$attempt" = 3 ] && exit 1; \
        sleep $((attempt * 15)); \
    done && \
    for b in cloudrun gcf azf lambda; do \
        CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /opt/sockerless/sockerless-$b-bootstrap ./cmd/sockerless-$b-bootstrap; \
    done

ENTRYPOINT ["/src/tests/terraform-integration/run-test.sh"]
