# syntax=docker/dockerfile:1
# Build the real Admin React application before compiling the Go server so the
# resulting binary embeds the same UI served in development and CI.
FROM oven/bun:1.3.10-alpine AS web
WORKDIR /src/ui
COPY ui/package.json ui/bun.lock ./
COPY ui/tsconfig.base.json ./
COPY ui/packages ./packages
RUN --mount=type=cache,target=/root/.bun/install/cache bun install --frozen-lockfile
RUN cd packages/admin && bun run build

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
COPY --from=web /src/ui/packages/admin/dist ./cmd/sockerless-admin/dist
WORKDIR /src/cmd/sockerless-admin
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /out/sockerless-admin .

FROM alpine:3.20
RUN addgroup -S sockerless && adduser -S -G sockerless sockerless
COPY --from=build /out/sockerless-admin /usr/local/bin/sockerless-admin
USER sockerless
EXPOSE 9090
ENTRYPOINT ["sockerless-admin"]
