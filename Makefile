.PHONY: build clean test lint fmt vet sec ver help

# Variables
BINARY_NAME=cowgnition
MAIN_PACKAGE=./cmd/server
GO_FILES=$(shell find . -name "*.go" -not -path "./vendor/*")
VERSION=$(shell git describe --tags --always --dirty || echo "dev")
LDFLAGS=-ldflags "-X main.version=${VERSION} -X main.commitHash=$(shell git rev-parse HEAD) -X main.buildDate=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')"
GOPATH?=$(shell go env GOPATH)

# Default target
all: build

# Build the application
build:
	@echo "Building..."
	go build ${LDFLAGS} -o ${BINARY_NAME} ${MAIN_PACKAGE}

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f ${BINARY_NAME}
	go clean -cache -testcache

# Run linters with golangci-lint
lint:
	@echo "Running linters..."
	golangci-lint run

# Format code with goimports (superset of gofmt)
fmt:
	@echo "Formatting code..."
	goimports -w ${GO_FILES}

# Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

# Run gosec security scanner
sec:
	@echo "Running security scan..."
	gosec -quiet ./...

# Print version information
ver:
	@echo "Version: ${VERSION}"
	@echo "Go version: $(shell go version)"

# Run with hot reloading (using entr - install with brew install entr)
dev:
	@echo "Starting development server with hot reload..."
	find . -name "*.go" | entr -r go run ${LDFLAGS} ${MAIN_PACKAGE} --config configs/config.yaml

# Check if all required tools are installed
check-tools:
	@echo "Checking required tools..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	@which goimports > /dev/null || (echo "goimports not found. Install with: go install golang.org/x/tools/cmd/goimports@latest" && exit 1)
	@echo "All tools are installed!"

# Install development tools
install-tools:
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest

# Help target
help:
	@echo "Available targets:"
	@echo "  all           Build the application (default)"
	@echo "  build         Build the application"
	@echo "  clean         Clean build artifacts"
	@echo "  lint          Run all linters with golangci-lint"
	@echo "  fmt           Format code with goimports"
	@echo "  vet           Run go vet"
	@echo "  sec           Run gosec security scanner"
	@echo "  ver           Print version information"
	@echo "  dev           Run with hot reloading"
	@echo "  check-tools   Check if required tools are installed"
	@echo "  install-tools Install development tools"
