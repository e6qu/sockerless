# syntax=mirror.gcr.io/docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e
# Build the real Admin React application before compiling the Go server so the
# resulting binary embeds the same UI served in development and CI.
FROM mirror.gcr.io/oven/bun:1.4.3-alpine@sha256:629e17411f1f129dbec3af78d5af9c9f2a937435c80349437206c6b0b7422373 AS web
WORKDIR /src/ui
COPY ui/package.json ui/bun.lock ./
COPY ui/tsconfig.base.json ./
COPY ui/packages ./packages
RUN --mount=type=cache,target=/root/.bun/install/cache bun install --frozen-lockfile
RUN cd packages/admin && bun run build

FROM public.ecr.aws/docker/library/golang:1.26.9-alpine@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS build
WORKDIR /src
COPY . .
COPY --from=web /src/ui/packages/admin/dist ./cmd/sockerless-admin/dist
WORKDIR /src/cmd/sockerless-admin
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /out/sockerless-admin .

FROM public.ecr.aws/docker/library/alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6
RUN addgroup -S sockerless && adduser -S -G sockerless sockerless
COPY --from=build /out/sockerless-admin /usr/local/bin/sockerless-admin
USER sockerless
EXPOSE 9090
ENTRYPOINT ["sockerless-admin"]
