.PHONY: all build clean test lint fmt dev help check

# Colors for output formatting
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
BLUE := \033[0;34m
NC := \033[0m # No Color

# Variables
BINARY_NAME := cowgnition
MAIN_PACKAGE := ./cmd/server
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT_HASH := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X main.version=${VERSION} -X main.commitHash=${COMMIT_HASH} -X main.buildDate=${BUILD_DATE}"
GO_FILES := $(shell find . -name "*.go" -not -path "./vendor/*")

# Default target - run all checks and build
all: check fmt lint test build
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

# Run tests
test:
	@printf "${BLUE}▶ Running tests...${NC}\n"
	@go test -v ./... && \
		printf "${GREEN}✓ Tests passed${NC}\n" || \
		(printf "${RED}✗ Tests failed${NC}\n" && exit 1)

# Run linters
lint:
	@printf "${BLUE}▶ Running linters...${NC}\n"
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		printf "${RED}✗ golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest${NC}\n"; \
		exit 1; \
	fi
	@golangci-lint run && \
		printf "${GREEN}✓ Code looks good${NC}\n" || \
		(printf "${RED}✗ Linting issues found${NC}\n" && exit 1)

# Format code
fmt:
	@printf "${BLUE}▶ Formatting code...${NC}\n"
	@if ! command -v goimports >/dev/null 2>&1; then \
		printf "${RED}✗ goimports not found. Install with: go install golang.org/x/tools/cmd/goimports@latest${NC}\n"; \
		exit 1; \
	fi
	@goimports -w ${GO_FILES}
	@printf "${GREEN}✓ Code formatted${NC}\n"

# Development mode with hot reload
dev:
	@printf "${BLUE}▶ Starting development server with hot reload...${NC}\n"
	@if ! command -v entr >/dev/null 2>&1; then \
		printf "${RED}✗ entr not found. Install with: brew install entr${NC}\n"; \
		exit 1; \
	fi
	@find . -name "*.go" | entr -r go run ${LDFLAGS} ${MAIN_PACKAGE} serve --config configs/config.yaml

# Check for required tools
check:
	@printf "${BLUE}▶ Checking for required tools...${NC}\n"
	@printf "  Go:            "
	@if command -v go >/dev/null 2>&1; then printf "${GREEN}✓${NC}\n"; else printf "${RED}✗${NC}\n"; fi
	@printf "  golangci-lint: "
	@if command -v golangci-lint >/dev/null 2>&1; then printf "${GREEN}✓${NC}\n"; else printf "${RED}✗${NC}\n"; fi
	@printf "  goimports:     "
	@if command -v goimports >/dev/null 2>&1; then printf "${GREEN}✓${NC}\n"; else printf "${RED}✗${NC}\n"; fi
	@printf "  entr:          "
	@if command -v entr >/dev/null 2>&1; then printf "${GREEN}✓${NC}\n"; else printf "${RED}✗${NC}\n"; fi

# Help target
help:
	@printf "${BLUE}CowGnition Make Targets:${NC}\n"
	@printf "  %-12s %s\n" "all" "Run all checks and build (default)"
	@printf "  %-12s %s\n" "build" "Build the application"
	@printf "  %-12s %s\n" "clean" "Clean build artifacts"
	@printf "  %-12s %s\n" "test" "Run tests"
	@printf "  %-12s %s\n" "lint" "Run linters"
	@printf "  %-12s %s\n" "fmt" "Format code"
	@printf "  %-12s %s\n" "dev" "Run with hot reloading"
	@printf "  %-12s %s\n" "check" "Check for required tools"
	@printf "  %-12s %s\n" "help" "Show this help message"
