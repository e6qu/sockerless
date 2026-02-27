FROM golang:1.25-bookworm

# Install terraform, terragrunt, and utilities
RUN apt-get update && apt-get install -y --no-install-recommends \
        curl jq openssl unzip dnsmasq \
    && rm -rf /var/lib/apt/lists/*

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

# Pre-download Go module dependencies for faster test runs
RUN cd simulators/azure && GOWORK=off go mod download

ENTRYPOINT ["/src/tests/terraform-integration/run-test.sh"]
