.PHONY: build test clean install help

# Default target
all: build

# Build the binary
build:
	go build -o cowtree

# Run tests
test:
	go test ./... -v

# Run tests with coverage
test-coverage:
	go test ./... -v -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# Clean build artifacts
clean:
	rm -f cowtree coverage.out coverage.html

# Install dependencies
deps:
	go mod download
	go mod tidy

# Install the binary
install: build
	sudo cp cowtree /usr/local/bin/

# Run linter
lint:
	golangci-lint run

# Run benchmarks
bench:
	go test ./pkg/cowgit -bench=. -benchmem

# Quick test (skip integration tests)
test-quick:
	go test ./pkg/cowgit -v -short

# Help
help:
	@echo "Available targets:"
	@echo "  build         Build the cowtree binary"
	@echo "  test          Run all tests"
	@echo "  test-coverage Run tests with coverage report"
	@echo "  test-quick    Run quick tests (skip integration)"
	@echo "  clean         Clean build artifacts"
	@echo "  deps          Install and tidy dependencies"
	@echo "  install       Install binary to /usr/local/bin"
	@echo "  lint          Run linter"
	@echo "  bench         Run benchmarks"
	@echo "  help          Show this help message"