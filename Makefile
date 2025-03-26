.PHONY: all build clean test lint golangci-lint fmt check deps install-tools help

# Colors for output formatting
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
BLUE := \033[0;34m
NC := \033[0m # No Color

# Variables
BINARY_NAME := cowgnition
MAIN_PACKAGE := ./cmd/server
GO_FILES := $(shell find . -name "*.go" -not -path "./vendor/*" -not -path "./test/*")
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT_HASH := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X main.version=${VERSION} -X main.commitHash=${COMMIT_HASH} -X main.buildDate=${BUILD_DATE}"

# Default target - run all checks and build
all: check deps fmt lint test build
	@printf "${GREEN}✓ All checks passed and build completed successfully!${NC}\n"

# Build the application
build:
	@printf "${BLUE}▶ Building ${BINARY_NAME}...${NC}\n"
	@go build ${LDFLAGS} -o ${BINARY_NAME} ${MAIN_PACKAGE} && \
		printf "${GREEN}✓ Build successful${NC}\n" || \
		(printf "${RED}✗ Build failed${NC}\n" && exit 1)

# Clean build artifacts
clean:
	@printf "${BLUE}▶ Cleaning build artifacts...${NC}\n"
	@rm -f ${BINARY_NAME}
	@go clean -cache -testcache
	@printf "${GREEN}✓ Cleaned${NC}\n"

# Download dependencies
# Download dependencies
deps:
	@printf "${BLUE}▶ Downloading dependencies...${NC}\n"
	@go mod download > /dev/null 2>&1; \
	if [ $$? -eq 0 ]; then \
		printf "  ${BLUE}No new dependencies needed${NC}\n"; \
	else \
		printf "${RED}✗ Failed to download dependencies${NC}\n"; \
		exit 1; \
	fi
	@printf "${GREEN}✓ Dependencies downloaded${NC}\n"

# Run tests
test:
	@printf "${BLUE}▶ Running tests...${NC}\n"
	@go test ./... && \
		printf "${GREEN}✓ Tests passed${NC}\n" || \
		(printf "${RED}✗ Tests failed${NC}\n" && exit 1)

# Run linters
lint:
	@printf "${BLUE}▶ Running linters...${NC}\n"
	@go vet ./... && \
		printf "${GREEN}✓ Code looks good${NC}\n" || \
		(printf "${RED}✗ Linting issues found${NC}\n" && exit 1)

# Run golangci-lint
golangci-lint:
	@printf "${BLUE}▶ Running golangci-lint...${NC}\n"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run && \
		printf "${GREEN}✓ golangci-lint passed${NC}\n" || \
		(printf "${RED}✗ golangci-lint failed${NC}\n" && exit 1); \
	else \
		printf "${YELLOW}⚠ golangci-lint not found, run 'make install-tools' to install${NC}\n"; \
		exit 1; \
	fi

# Run gofmt
fmt:
	@printf "${BLUE}▶ Formatting code...${NC}\n"
	@go fmt ./...
	@printf "${GREEN}✓ Code formatted${NC}\n"

# Install required tools
install-tools:
	@printf "${BLUE}▶ Installing required tools...${NC}\n"
	@printf "  golangci-lint: "
	@if command -v golangci-lint >/dev/null 2>&1; then \
		printf "${GREEN}✓ Already installed${NC}\n"; \
	else \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest && \
		printf "${GREEN}✓ Installed${NC}\n" || \
		printf "${RED}✗ Installation failed${NC}\n"; \
	fi
	@printf "${GREEN}✓ Tools installation complete${NC}\n"

# Check for required tools
check:
	@printf "${BLUE}▶ Checking for required tools...${NC}\n"
	@printf "  Go:            "
	@if command -v go >/dev/null 2>&1; then \
		printf "${GREEN}✓ $(shell go version)${NC}\n"; else printf "${RED}✗${NC}\n"; fi
	@printf "${GREEN}✓ Tool check complete${NC}\n"

# Help target
help:
	@printf "${BLUE}CowGnition Make Targets:${NC}\n"
	@printf "  %-16s %s\n" "all" "Run checks, formatting, tests, and build (default)"
	@printf "  %-16s %s\n" "build" "Build the application"
	@printf "  %-16s %s\n" "clean" "Clean build artifacts"
	@printf "  %-16s %s\n" "test" "Run tests"
	@printf "  %-16s %s\n" "lint" "Run linters"
	@printf "  %-16s %s\n" "golangci-lint" "Run golangci-lint specifically"
	@printf "  %-16s %s\n" "fmt" "Format code"
	@printf "  %-16s %s\n" "deps" "Download dependencies"
	@printf "  %-16s %s\n" "install-tools" "Install required development tools"
