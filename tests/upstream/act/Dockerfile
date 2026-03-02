FROM golang:1.25 AS builder

# --- Build all 3 simulators ---
WORKDIR /sim-aws
COPY simulators/aws/ /sim-aws/
RUN GOWORK=off go build -tags noui -o /out/simulator-aws .

WORKDIR /sim-gcp
COPY simulators/gcp/ /sim-gcp/
RUN GOWORK=off go build -tags noui -o /out/simulator-gcp .

WORKDIR /sim-azure
COPY simulators/azure/ /sim-azure/
RUN GOWORK=off go build -tags noui -o /out/simulator-azure .

# --- Copy shared source for backends ---
WORKDIR /src
COPY api/ api/
COPY agent/ agent/
COPY sandbox/ sandbox/
COPY backends/core/ backends/core/

# --- Build all 7 backends ---
COPY backends/memory/ backends/memory/
RUN cd backends/memory && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-backend-memory ./cmd

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

# --- Build frontend ---
COPY frontends/docker/ frontends/docker/
RUN cd frontends/docker && GOWORK=off go mod download && \
    GOWORK=off go build -tags noui -o /out/sockerless-frontend-docker ./cmd

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
