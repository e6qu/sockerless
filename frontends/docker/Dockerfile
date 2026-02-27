FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY api/ ./api/
COPY agent/ ./agent/
COPY backends/core/ ./backends/core/
COPY frontends/docker/ ./frontends/docker/
WORKDIR /src/frontends/docker
ENV GOWORK=off
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /sockerless-frontend ./cmd

FROM alpine:3.20
COPY --from=builder /sockerless-frontend /usr/local/bin/sockerless-frontend
ENTRYPOINT ["sockerless-frontend"]
