BINARY := sockerless-backend-cloudrun
CMD_DIR := ./cmd/sockerless-backend-cloudrun

.PHONY: build test integration-test clean fmt vet

build:
	CGO_ENABLED=0 go build -o $(BINARY) $(CMD_DIR)

test:
	go test -v -timeout 2m ./...

integration-test:
	SOCKERLESS_INTEGRATION=1 go test -v -timeout 5m ./...

clean:
	rm -f $(BINARY)

fmt:
	go fmt ./...

vet:
	go vet ./...
