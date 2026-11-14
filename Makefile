BINARY_NAME=claude-dashboard
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build install clean test run fmt lint tidy demo

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/claude-dashboard

install:
	go install $(LDFLAGS) ./cmd/claude-dashboard

clean:
	rm -rf bin/

test:
	go test ./...

run: build
	./bin/$(BINARY_NAME)

fmt:
	go fmt ./...

lint:
	golangci-lint run

tidy:
	go mod tidy

demo: build
	vhs demo/demo.tape
