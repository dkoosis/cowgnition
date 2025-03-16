.PHONY: all build clean test test-coverage lint fmt dev help check static-analysis

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

# Default target - run all checks and build.  Stop on first failure.
all: check fmt static-analysis lint test build
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

# Run tests using gotestsum
test:
	@printf "${BLUE}▶ Running tests with gotestsum...${NC}\n"
	@if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format pkgname -- ./...; \
	else \
		printf "${RED}✗ gotestsum not found. Please install with: go install gotest.tools/gotestsum@latest${NC}\n"; \
		exit 1; \
	fi

# Run tests with coverage using gotestsum
test-coverage:
	@printf "${BLUE}▶ Running tests with coverage...${NC}\n"
	@if command -v gotestsum >/dev/null 2>&1; then \
		gotestsum --format pkgname -- -coverprofile=coverage.out ./...; \
	else \
		printf "${RED}✗ gotestsum not found. Please install with: go install gotest.tools/gotestsum@latest${NC}\n"; \
		exit 1; \
	fi
	@go tool cover -html=coverage.out -o coverage.html
	@printf "${GREEN}✓ Coverage report generated: coverage.html${NC}\n"
	@go tool cover -func=coverage.out

# Run linters
lint:
	@printf "${BLUE}▶ Running linters...${NC}\n"
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		printf "${RED}✗ golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest${NC}\n"; \
		exit 1; \
	fi
	@golangci-lint run --timeout=5m && \
		printf "${GREEN}✓ Code looks good${NC}\n" || \
		(printf "${RED}✗ Linting issues found${NC}\n" && exit 1)

# Run gofmt and goimports
fmt:
	@printf "${BLUE}▶ Formatting code...${NC}\n"
	@if ! command -v goimports >/dev/null 2>&1; then \
		printf "${RED}✗ goimports not found. Install with: go install golang.org/x/tools/cmd/goimports@latest${NC}\n"; \
		exit 1; \
	fi
	@go fmt ./...
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
	@printf "  staticcheck:   "
	@if command -v staticcheck >/dev/null 2>&1; then printf "${GREEN}✓${NC}\n"; else printf "${RED}✗${NC}\n"; fi
	@printf "  gotestsum:     "
	@if command -v gotestsum >/dev/null 2>&1; then printf "${GREEN}✓${NC}\n"; else printf "${RED}✗${NC}\n"; fi

# Static analysis using go vet and staticcheck
static-analysis:
	@printf "${BLUE}▶ Running static analysis...${NC}\n"
	@go vet ./... && \
		printf "${GREEN}✓ go vet passed${NC}\n" || \
		(printf "${RED}✗ go vet found issues${NC}\n" && exit 1)
	@if ! command -v staticcheck >/dev/null 2>&1; then \
		printf "${RED}✗ staticcheck not found. Install with: go install honnef.co/go/tools/cmd/staticcheck@latest${NC}\n"; \
		exit 1; \
	fi
	@staticcheck ./... && \
		printf "${GREEN}✓ staticcheck passed${NC}\n" || \
		(printf "${RED}✗ staticcheck found issues${NC}\n" && exit 1)

# Install development tools
setup-tools:
	@printf "${BLUE}▶ Installing development tools...${NC}\n"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install honnef.co/go/tools/cmd/staticcheck@latest
	@go install gotest.tools/gotestsum@latest
	@printf "${GREEN}✓ Development tools installed${NC}\n"

# Help target
help:
	@printf "${BLUE}CowGnition Make Targets:${NC}\n"
	@printf "  %-16s %s\n" "all" "Run all checks, static analysis, tests, and build (default)"
	@printf "  %-16s %s\n" "build" "Build the application"
	@printf "  %-16s %s\n" "clean" "Clean build artifacts"
	@printf "  %-16s %s\n" "test" "Run tests using gotestsum"
	@printf "  %-16s %s\n" "test-coverage" "Run tests with coverage report"
	@printf "  %-16s %s\n" "lint" "Run linters (with timeout)"
	@printf "  %-16s %s\n" "fmt" "Format code (using gofmt and goimports)"
	@printf "  %-16s %s\n" "dev" "Run with hot reloading (requires entr)"
	@printf "  %-16s %s\n" "check" "Check for required tools"
	@printf "  %-16s %s\n" "static-analysis" "Run static analysis (go vet and staticcheck)"
	@printf "  %-16s %s\n" "setup-tools" "Install development tools"
	@printf "  %-16s %s\n" "help" "Show this help message"
