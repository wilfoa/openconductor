.PHONY: build run clean test lint fmt vet coverage install check

BINARY  := openconductor
PKG     := ./cmd/openconductor
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

## Build & run

build:
	go build -ldflags '$(LDFLAGS)' -o $(BINARY) $(PKG)

install:
	go install -ldflags '$(LDFLAGS)' $(PKG)

run:
	go run $(PKG)

clean:
	rm -f $(BINARY) coverage.txt

## Quality

test:
	go test -race ./...

coverage:
	go test -race -coverprofile=coverage.txt -covermode=atomic ./...
	go tool cover -func=coverage.txt

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

## Pre-commit (run before pushing)

check: fmt vet lint test
	@echo "All checks passed."

## Help

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Build:"
	@echo "  build       Build binary"
	@echo "  install     Install to GOPATH/bin"
	@echo "  run         Build and run"
	@echo "  clean       Remove build artifacts"
	@echo ""
	@echo "Quality:"
	@echo "  test        Run tests with race detector"
	@echo "  coverage    Run tests with coverage report"
	@echo "  lint        Run golangci-lint"
	@echo "  fmt         Format code with gofmt"
	@echo "  vet         Run go vet"
	@echo "  check       Run fmt + vet + lint + test"
