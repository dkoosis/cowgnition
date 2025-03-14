.PHONY: build clean test

# Variables
BINARY_NAME=cowgnition
MAIN_PACKAGE=./cmd/server
GO_FILES=$(shell find . -name "*.go" -not -path "./vendor/*")
VERSION=$(shell git describe --tags --always --dirty || echo "dev")
LDFLAGS=-ldflags "-X main.version=${VERSION}"

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

# Run with hot reloading (using entr - install with brew install entr)
dev:
	@echo "Starting development server with hot reload..."
	find . -name "*.go" | entr -r go run ${MAIN_PACKAGE} --config configs/config.yaml
