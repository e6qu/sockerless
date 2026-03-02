BINARY  := simulator-gcp
IMAGE   := sockerless-simulator-gcp
PORT    := 4567

.PHONY: build clean run docker-build docker-run test shared-test sdk-test cli-test terraform-test docker-test-build docker-test fmt vet

build:
	CGO_ENABLED=0 go build -tags noui -ldflags="-s -w" -o $(BINARY) .

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

shared-test:
	cd shared && GOWORK=off go test -v -count=1 ./...

sdk-test: build
	cd sdk-tests && go test -v -count=1 ./...

cli-test: build
	cd cli-tests && go test -v -count=1 -timeout 120s ./...

terraform-test: build
	cd terraform-tests && go test -v -count=1 -timeout 300s ./...

test: shared-test sdk-test cli-test terraform-test

docker-test-build:
	docker build -t sockerless-test-gcp -f Dockerfile.test .

docker-test: docker-test-build
	docker run --rm -v $(CURDIR):/src -w /src sockerless-test-gcp make test

fmt:
	go fmt ./...

vet:
	go vet ./...
