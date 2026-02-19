.PHONY: build run clean test lint

BINARY := maestro
PKG := ./cmd/maestro

build:
	go build -o $(BINARY) $(PKG)

run:
	go run $(PKG)

clean:
	rm -f $(BINARY)

test:
	go test ./...

lint:
	golangci-lint run ./...
