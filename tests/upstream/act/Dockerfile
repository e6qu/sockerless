FROM golang:1.25 AS builder

# The v0.2.0 release commit; go install of a subdirectory module needs a
# commit-ish, not the repository-root release tag.
ARG SOCKERLESS_CLOUD_VERSION=723736a8a233e0f3ccb3d6ce0c209a94bfc9afc2

# --- Build all 3 simulators ---
RUN GOBIN=/out go install -tags noui github.com/e6qu/sockerless-cloud/simulator-aws@${SOCKERLESS_CLOUD_VERSION}

RUN GOBIN=/out go install -tags noui github.com/e6qu/sockerless-cloud/simulator-gcp@${SOCKERLESS_CLOUD_VERSION}

RUN GOBIN=/out go install -tags noui github.com/e6qu/sockerless-cloud/simulator-azure@${SOCKERLESS_CLOUD_VERSION}

# --- Copy shared source for backends ---
WORKDIR /src
COPY api/ api/
COPY agent/ agent/
COPY backends/core/ backends/core/
COPY backends/aws-common/ backends/aws-common/
COPY backends/gcp-common/ backends/gcp-common/
COPY backends/azure-common/ backends/azure-common/

# --- Build all 6 backends ---
COPY backends/ecs/ backends/ecs/
RUN cd backends/ecs && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-ecs ./cmd/sockerless-backend-ecs

COPY backends/lambda/ backends/lambda/
RUN cd backends/lambda && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-lambda ./cmd/sockerless-backend-lambda

COPY backends/cloudrun/ backends/cloudrun/
RUN cd backends/cloudrun && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-cloudrun ./cmd/sockerless-backend-cloudrun

COPY backends/cloudrun-functions/ backends/cloudrun-functions/
RUN cd backends/cloudrun-functions && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-gcf ./cmd/sockerless-backend-gcf

COPY backends/aca/ backends/aca/
RUN cd backends/aca && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-aca ./cmd/sockerless-backend-aca

COPY backends/azure-functions/ backends/azure-functions/
RUN cd backends/azure-functions && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-azf ./cmd/sockerless-backend-azf

# --- Build agent ---
RUN cd agent && GOWORK=off go mod download && \
    GOWORK=off go build -o /out/sockerless-agent ./cmd/sockerless-agent

# --- Clone and prep act ---
ARG ACT_REF=v0.2.84
RUN git clone --depth 1 --branch ${ACT_REF} https://github.com/nektos/act.git /act
RUN cd /act && go mod download

# --- Runtime stage ---
FROM golang:1.25

RUN apt-get update && apt-get install -y --no-install-recommends curl git && rm -rf /var/lib/apt/lists/*

# Copy all Sockerless binaries
COPY --from=builder /out/ /usr/local/bin/

# Copy act source with dependencies
COPY --from=builder /act /act
COPY --from=builder /go/pkg/mod /go/pkg/mod

# Copy shared E2E libs and test runner
COPY tests/e2e-live-tests/lib.sh /test/lib.sh
COPY tests/e2e-live-tests/start-backend.sh /test/start-backend.sh
COPY tests/upstream/act/run.sh /test/run.sh
RUN chmod +x /test/*.sh

# Results volume
RUN mkdir /results

ENTRYPOINT ["/test/run.sh", "--act-src", "/act"]
