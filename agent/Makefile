BINARY := sockerless-agent
CMD_DIR := ./cmd/sockerless-agent

.PHONY: build test clean fmt vet

build:
	CGO_ENABLED=0 go build -o $(BINARY) $(CMD_DIR)

test:
	go test -v -timeout 2m ./...

clean:
	rm -f $(BINARY)

fmt:
	go fmt ./...

vet:
	go vet ./...
