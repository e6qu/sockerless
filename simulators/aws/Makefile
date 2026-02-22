BINARY  := simulator-aws
IMAGE   := sockerless-simulator-aws
PORT    := 4566

.PHONY: build clean run docker-build docker-run test sdk-test cli-test terraform-test docker-test-build docker-test fmt vet

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY) .

clean:
	rm -f $(BINARY)

run: build
	SIM_LISTEN_ADDR=:$(PORT) ./$(BINARY)

docker-build:
	docker build -t $(IMAGE) .

docker-run: docker-build
	docker run --rm -p $(PORT):$(PORT) \
		-e SIM_LISTEN_ADDR=:$(PORT) \
		-e SIM_LOG_LEVEL=info \
		$(IMAGE)

sdk-test: build
	cd sdk-tests && go test -v -count=1 ./...

cli-test: build
	cd cli-tests && go test -v -count=1 -timeout 120s ./...

terraform-test: build
	cd terraform-tests && go test -v -count=1 -timeout 300s ./...

test: sdk-test cli-test terraform-test

docker-test-build:
	docker build -t sockerless-test-aws -f Dockerfile.test .

docker-test: docker-test-build
	docker run --rm -v $(CURDIR):/src -w /src sockerless-test-aws make test

fmt:
	go fmt ./...

vet:
	go vet ./...
