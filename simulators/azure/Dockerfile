FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /simulator-azure .

FROM alpine:3.20
RUN apk add --no-cache wget
COPY --from=builder /simulator-azure /usr/local/bin/simulator-azure
ENTRYPOINT ["simulator-azure"]
