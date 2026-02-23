FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /simulator-aws .

FROM alpine:3.20
RUN apk add --no-cache wget
COPY --from=builder /simulator-aws /usr/local/bin/simulator-aws
ENTRYPOINT ["simulator-aws"]
