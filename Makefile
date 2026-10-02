BINARY := proxypool
GOFLAGS := -trimpath

.PHONY: build clean run test install

build:
	go build $(GOFLAGS) -o bin/$(BINARY) ./cmd/proxypool

run: build
	./bin/$(BINARY) serve -config configs/example.yaml

test:
	go test ./...

install:
	go install ./cmd/proxypool

clean:
	rm -rf bin/
