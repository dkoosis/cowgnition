# Specify phony targets (targets not associated with files)
.PHONY: all build clean test lint golangci-lint fmt check deps install-tools check-line-length help

# --- Configuration ---

# Colors for output formatting
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
BLUE := \033[0;34m
NC := \033[0m # No Color

# Variables
BINARY_NAME := cowgnition
MAIN_PACKAGE := ./cmd/server
# Find Go files, excluding vendor and test directories (adjust if needed)
GO_FILES := $(shell find . -name "*.go" -not -path "./vendor/*" -not -path "./test/*")
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT_HASH := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X main.version=${VERSION} -X main.commitHash=${COMMIT_HASH} -X main.buildDate=${BUILD_DATE}"

# Line length check configuration
WARN_LINES := 300  # Warn if lines exceed this
FAIL_LINES := 600  # Fail if lines exceed this

# --- Core Targets ---

# Default target - run all checks and build
all: check deps fmt golangci-lint check-line-length test build
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

# --- Dependency Management ---

# Download dependencies
deps:
	@printf "${BLUE}▶ Downloading dependencies...${NC}\n"
	@go mod tidy > /dev/null 2>&1; \
	if [ $$? -eq 0 ]; then \
		printf "  ${BLUE}Running go mod download...${NC}\n"; \
		go mod download > /dev/null 2>&1; \
		if [ $$? -eq 0 ]; then \
			printf "  ${BLUE}Dependencies synchronized successfully${NC}\n"; \
		else \
			printf "${RED}✗ Failed to download dependencies${NC}\n"; \
			exit 1; \
		fi \
	else \
		printf "${RED}✗ Failed to tidy dependencies${NC}\n"; \
		exit 1; \
	fi
	@printf "${GREEN}✓ Dependencies downloaded${NC}\n"


# --- Quality & Testing ---

# Run tests
test:
	@printf "${BLUE}▶ Running tests...${NC}\n"
	@go test ./... && \
		printf "${GREEN}✓ Tests passed${NC}\n" || \
		(printf "${RED}✗ Tests failed${NC}\n" && exit 1)

# Run basic Go linter (go vet)
lint:
	@printf "${BLUE}▶ Running linters (go vet)...${NC}\n"
	@go vet ./... && \
		printf "${GREEN}✓ go vet passed${NC}\n" || \
		(printf "${RED}✗ go vet found issues${NC}\n" && exit 1)

# Run comprehensive golangci-lint
golangci-lint: install-tools # Ensure tool is installed first
	@printf "${BLUE}▶ Running golangci-lint...${NC}\n"
	@golangci-lint run && \
		printf "${GREEN}✓ golangci-lint passed${NC}\n" || \
		(printf "${RED}✗ golangci-lint failed${NC}\n" && exit 1)

# Check Go file line lengths using external script
check-line-length:
	# Ensure the script exists and is executable
	@if [ ! -x "./scripts/check_file_length.sh" ]; then \
		printf "${RED}✗ Error: Script './scripts/check_file_length.sh' not found or not executable.${NC}\n"; \
		exit 1; \
	fi
	# Execute the script; it will print its own status messages.
	@./scripts/check_file_length.sh ${WARN_LINES} ${FAIL_LINES} ${GO_FILES}

# Run gofmt to format code
fmt:
	@printf "${BLUE}▶ Formatting code...${NC}\n"
	@go fmt ./...
	@printf "${GREEN}✓ Code formatted${NC}\n"

# --- Tooling & Setup ---

# Install required tools (currently just golangci-lint)
install-tools:
	@printf "${BLUE}▶ Installing required tools...${NC}\n"
	@printf "  golangci-lint: "
	@if command -v golangci-lint >/dev/null 2>&1; then \
		printf "${GREEN}✓ Already installed${NC}\n"; \
	else \
		printf "Installing..." ;\
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest && \
		printf "${GREEN}✓ Installed${NC}\n" || \
		(printf "${RED}✗ Installation failed${NC}\n" && exit 1); \
	fi
	@printf "${GREEN}✓ Tools installation check complete${NC}\n"

# Check for required tools locally
check:
	@printf "${BLUE}▶ Checking for required tools...${NC}\n"
	@printf "  Go:            "
	@if command -v go >/dev/null 2>&1; then \
		printf "${GREEN}✓ $(shell go version)${NC}\n"; \
	else \
		printf "${RED}✗ Not Found${NC}\n"; \
		exit 1; \
	fi
	@printf "  golangci-lint: "
	@if command -v golangci-lint >/dev/null 2>&1; then \
		printf "${GREEN}✓ Found${NC}\n"; \
	else \
		printf "${YELLOW}⚠ Not Found (run 'make install-tools')${NC}\n"; \
	fi
	@printf "${GREEN}✓ Tool check complete${NC}\n"


# --- Help ---

# Help target: Display available commands
help:
	@printf "${BLUE}CowGnition Make Targets:${NC}\n"
	@printf "  %-20s %s\n" "all" "Run checks, formatting, tests, and build (default)"
	@printf "  %-20s %s\n" "build" "Build the application"
	@printf "  %-20s %s\n" "clean" "Clean build artifacts"
	@printf "  %-20s %s\n" "test" "Run tests"
	@printf "  %-20s %s\n" "lint" "Run basic 'go vet' linter"
	@printf "  %-20s %s\n" "golangci-lint" "Run comprehensive golangci-lint"
	@printf "  %-20s %s\n" "check-line-length" "Check Go file line count (W:${WARN_LINES}, F:${FAIL_LINES})"
	@printf "  %-20s %s\n" "fmt" "Format code using 'go fmt'"
	@printf "  %-20s %s\n" "deps" "Tidy and download dependencies"
	@printf "  %-20s %s\n" "install-tools" "Install required development tools (golangci-lint)"
	@printf "  %-20s %s\n" "check" "Check if required tools (Go) are installed"
	@printf "  %-20s %s\n" "help" "Display this help message"
