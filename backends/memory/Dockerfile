FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY api/ ./api/
COPY agent/ ./agent/
COPY sandbox/ ./sandbox/
COPY backends/core/ ./backends/core/
COPY backends/memory/ ./backends/memory/
WORKDIR /src/backends/memory
ENV GOWORK=off
RUN CGO_ENABLED=0 go build -tags noui -ldflags="-s -w" -o /sockerless-backend-memory ./cmd

FROM alpine:3.20
COPY --from=builder /sockerless-backend-memory /usr/local/bin/sockerless-backend-memory
ENTRYPOINT ["sockerless-backend-memory"]
