BINARY := gitlabhub
CMD_DIR := ./cmd/

.PHONY: build test clean fmt vet

build:
	CGO_ENABLED=0 go build -o $(BINARY) $(CMD_DIR)

test:
	go test -v -timeout 3m ./...

clean:
	rm -f $(BINARY)

fmt:
	go fmt ./...

vet:
	go vet ./...
