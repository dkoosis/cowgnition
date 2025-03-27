# Specify phony targets (targets not associated with files)
.PHONY: all build clean test lint golangci-lint fmt check deps install-tools check-line-length help

# --- Configuration ---

# Colors for output formatting
RESET   := \033[0m
BOLD    := \033[1m
GREEN   := \033[0;32m
YELLOW  := \033[0;33m
RED     := \033[0;31m
BLUE    := \033[0;34m
NC      := $(RESET) # No Color Alias

# Icons (Optional, but makes lines cleaner)
ICON_START := $(BLUE)▶$(NC)
ICON_OK    := $(GREEN)✓$(NC)
ICON_WARN  := $(YELLOW)⚠$(NC)
ICON_FAIL  := $(RED)✗$(NC)
ICON_INFO  := $(BLUE)ℹ$(NC) # Informational icon

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
	@printf "$(GREEN)$(BOLD)✨ All checks passed and build completed successfully! ✨$(NC)\n"

# Build the application
build:
	@printf "$(ICON_START) $(BOLD)Building $(BINARY_NAME)...$(NC)\n"
	@go build $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_PACKAGE) && \
		printf "$(ICON_OK) Build successful$(NC)\n" || \
		(printf "$(ICON_FAIL) Build failed$(NC)\n" && exit 1)
	@printf "\n" # Add spacing

# Clean build artifacts
clean:
	@printf "$(ICON_START) $(BOLD)Cleaning build artifacts...$(NC)\n"
	@rm -f $(BINARY_NAME)
	@go clean -cache -testcache
	@printf "$(ICON_OK) Cleaned$(NC)\n"
	@printf "\n" # Add spacing

# --- Dependency Management ---

# Download dependencies
deps:
	@printf "$(ICON_START) $(BOLD)Downloading dependencies...$(NC)\n"
	@printf "  $(ICON_INFO) Running go mod tidy...\n"; \
	go mod tidy > /dev/null 2>&1; \
	if [ $$? -ne 0 ]; then \
		printf "$(ICON_FAIL) Failed to tidy dependencies$(NC)\n"; \
		exit 1; \
	fi
	@printf "  $(ICON_INFO) Running go mod download...\n"; \
	go mod download > /dev/null 2>&1; \
	if [ $$? -eq 0 ]; then \
		printf "  Dependencies synchronized successfully\n"; \
	else \
		printf "$(ICON_FAIL) Failed to download dependencies$(NC)\n"; \
		exit 1; \
	fi
	@printf "$(ICON_OK) Dependencies downloaded$(NC)\n"
	@printf "\n" # Add spacing


# --- Quality & Testing ---

# Run tests
test:
	@printf "$(ICON_START) $(BOLD)Running tests...$(NC)\n"
	@# The 'go test' command itself will print details, including '? [no test files]'
	@go test ./... && \
		printf "$(ICON_OK) Tests passed$(NC)\n" || \
		(printf "$(ICON_FAIL) Tests failed$(NC)\n" && exit 1)
	@printf "\n" # Add spacing
	@# Note: Formatting the "? [no test files]" lines requires capturing and parsing 'go test' output,
	@# which is complex. This target only reports overall pass/fail summary message.

# Run basic Go linter (go vet)
lint:
	@printf "$(ICON_START) $(BOLD)Running linters (go vet)...$(NC)\n"
	@go vet ./... && \
		printf "$(ICON_OK) go vet passed$(NC)\n" || \
		(printf "$(ICON_FAIL) go vet found issues$(NC)\n" && exit 1)
	@printf "\n" # Add spacing

# Run comprehensive golangci-lint
golangci-lint: install-tools # Ensure tool is installed first
	@printf "$(ICON_START) $(BOLD)Running golangci-lint...$(NC)\n"
	@golangci-lint run && \
		printf "$(ICON_OK) golangci-lint passed$(NC)\n" || \
		(printf "$(ICON_FAIL) golangci-lint failed$(NC)\n" && exit 1)
	@printf "\n" # Add spacing

# Check Go file line lengths using external script
check-line-length:
	@printf "$(ICON_START) $(BOLD)Checking file lengths (warn > $(WARN_LINES), fail > $(FAIL_LINES))...$(NC)\n"
	@if [ ! -x "./scripts/check_file_length.sh" ]; then \
		printf "$(ICON_FAIL) Error: Script './scripts/check_file_length.sh' not found or not executable.$(NC)\n"; \
		exit 1; \
	fi
	@# Execute the script; it should print its own formatted status messages (warnings/success)
	@./scripts/check_file_length.sh $(WARN_LINES) $(FAIL_LINES) $(GO_FILES)
	@# Assuming the script exits non-zero on failure (e.g., file exceeds FAIL_LINES)
	@# and prints its own overall status (like ✓ Warnings issued... or ✗ Errors found...).
	@# We check the exit code to ensure Make fails if the script indicates failure.
	@if [ $$? -ne 0 ]; then exit 1; fi
	@printf "\n" # Add spacing
	@# Note: Ensure './scripts/check_file_length.sh' uses printf with ICON_WARN/ICON_OK/ICON_FAIL for its output.

# Run gofmt to format code
fmt:
	@printf "$(ICON_START) $(BOLD)Formatting code...$(NC)\n"
	@# go fmt doesn't usually produce output on success, so we just print our own status
	@go fmt ./...
	@printf "$(ICON_OK) Code formatted$(NC)\n"
	@printf "\n" # Add spacing

# --- Tooling & Setup ---

# Install required tools (currently just golangci-lint)
install-tools:
	@printf "$(ICON_START) $(BOLD)Installing required tools...$(NC)\n"
	@printf "  golangci-lint: "
	@if command -v golangci-lint >/dev/null 2>&1; then \
		printf "$(ICON_OK) Already installed$(NC)\n"; \
	else \
		printf "$(ICON_INFO) Installing..." ;\
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest && \
		printf " $(ICON_OK) Installed$(NC)\n" || \
		(printf " $(ICON_FAIL) Installation failed$(NC)\n" && exit 1); \
	fi
	@printf "$(ICON_OK) Tools installation check complete$(NC)\n"
	@printf "\n" # Add spacing

# Check for required tools locally
check:
	@printf "$(ICON_START) $(BOLD)Checking for required tools...$(NC)\n"
	@printf "  Go:            "
	@if command -v go >/dev/null 2>&1; then \
		printf "$(ICON_OK) $(shell go version)$(NC)\n"; \
	else \
		printf "$(ICON_FAIL) Not Found - Go is required$(NC)\n"; \
		exit 1; \
	fi
	@printf "  golangci-lint: "
	@if command -v golangci-lint >/dev/null 2>&1; then \
		printf "$(ICON_OK) Found$(NC)\n"; \
	else \
		printf "$(ICON_WARN) Not Found (run 'make install-tools')$(NC)\n"; \
	fi
	@printf "$(ICON_OK) Tool check complete$(NC)\n"
	@printf "\n" # Add spacing


# --- Help ---

# Help target: Display available commands
help:
	@printf "$(BLUE)$(BOLD)CowGnition Make Targets:$(NC)\n"
	@printf "  %-20s %s\n" "all" "Run checks, formatting, tests, and build (default)"
	@printf "  %-20s %s\n" "build" "Build the application"
	@printf "  %-20s %s\n" "clean" "Clean build artifacts"
	@printf "  %-20s %s\n" "test" "Run tests"
	@printf "  %-20s %s\n" "lint" "Run basic 'go vet' linter"
	@printf "  %-20s %s\n" "golangci-lint" "Run comprehensive golangci-lint"
	@printf "  %-20s %s\n" "check-line-length" "Check Go file line count (W:$(WARN_LINES), F:$(FAIL_LINES))"
	@printf "  %-20s %s\n" "fmt" "Format code using 'go fmt'"
	@printf "  %-20s %s\n" "deps" "Tidy and download dependencies"
	@printf "  %-20s %s\n" "install-tools" "Install required development tools (golangci-lint)"
	@printf "  %-20s %s\n" "check" "Check if required tools (Go) are installed"
	@printf "  %-20s %s\n" "help" "Display this help message"
