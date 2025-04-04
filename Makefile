# Specify phony targets (targets not associated with files)
.PHONY: all build clean test lint golangci-lint fmt check deps install-tools check-line-length tree help

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

# Formatting Strings for Alignment
LABEL_FMT := "   %-15s" # Indent 3, Pad label to 15 chars, left-aligned

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
FAIL_LINES := 650  # Fail if lines exceed this

# --- Core Targets ---

# Default target - run all checks and build
all: check deps fmt golangci-lint check-line-length test tree build
	@printf "$(GREEN)$(BOLD)✨ All checks passed and build completed successfully! ✨$(NC)\n"

# Build the application
build:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Building $(BINARY_NAME)...$(NC)\n"
	@go build $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_PACKAGE) && \
		printf "   $(ICON_OK) $(GREEN)Build successful$(NC)\n" || \
		(printf "   $(ICON_FAIL) $(RED)Build failed$(NC)\n" && exit 1)
	@printf "\n" # Add spacing

# Clean build artifacts
clean:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Cleaning build artifacts...$(NC)\n"
	@rm -f $(BINARY_NAME)
	@go clean -cache -testcache
	@printf "   $(ICON_OK) $(GREEN)Cleaned$(NC)\n"
	@printf "\n" # Add spacing

# --- Dependency Management ---

# Download dependencies
deps:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Downloading dependencies...$(NC)\n"
	@printf "   $(ICON_INFO) Running go mod tidy...\n"; \
	go mod tidy > /dev/null 2>&1; \
	if [ $$? -ne 0 ]; then \
		printf "   $(ICON_FAIL) $(RED)Failed to tidy dependencies$(NC)\n"; \
		exit 1; \
	fi
	@printf "   $(ICON_INFO) Running go mod download...\n"; \
	go mod download > /dev/null 2>&1; \
	if [ $$? -eq 0 ]; then \
		printf "   Dependencies synchronized successfully\n"; \
	else \
		printf "   $(ICON_FAIL) $(RED)Failed to download dependencies$(NC)\n"; \
		exit 1; \
	fi
	@printf "   $(ICON_OK) $(GREEN)Dependencies downloaded$(NC)\n"
	@printf "\n" # Add spacing


# --- Quality & Testing ---

# Run tests
test:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Running tests...$(NC)\n"
	@# The 'go test' command itself will print details, including '? [no test files]'
	@go test ./... && \
		printf "   $(ICON_OK) $(GREEN)Tests passed$(NC)\n" || \
		(printf "   $(ICON_FAIL) $(RED)Tests failed$(NC)\n" && exit 1)
	@printf "\n" # Add spacing

# Run basic Go linter (go vet)
lint:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Running linters (go vet)...$(NC)\n"
	@go vet ./... && \
		printf "   $(ICON_OK) $(GREEN)go vet passed$(NC)\n" || \
		(printf "   $(ICON_FAIL) $(RED)go vet found issues$(NC)\n" && exit 1)
	@printf "\n" # Add spacing

# Run comprehensive golangci-lint
golangci-lint: install-tools # Ensure tool is installed first
	@printf "$(ICON_START) $(BOLD)$(BLUE)Running golangci-lint...$(NC)\n"
	@# The golangci-lint tool produces its own output and indentation for errors
	@golangci-lint run && \
		printf "   $(ICON_OK) $(GREEN)golangci-lint passed$(NC)\n" || \
		(printf "   $(ICON_FAIL) $(RED)golangci-lint failed (see errors above)$(NC)\n" && exit 1)
	@printf "\n" # Add spacing

# Check Go file line lengths using external script
check-line-length:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Checking file lengths (warn > $(WARN_LINES), fail > $(FAIL_LINES))...$(NC)\n"
	@if [ ! -x "./scripts/check_file_length.sh" ]; then \
		printf "   $(ICON_FAIL) $(RED)Error: Script './scripts/check_file_length.sh' not found or not executable.$(NC)\n"; \
		exit 1; \
	fi
	@# Execute the script.
	@./scripts/check_file_length.sh $(WARN_LINES) $(FAIL_LINES) $(GO_FILES)
	@# Check the exit status of the previous line explicitly.
	@if [ $$? -ne 0 ]; then \
		printf "   $(ICON_FAIL) $(RED)Check failed (see script output above)$(NC)\n"; \
		exit 1; \
	fi
	@# If we reach here, the script exited 0. Script should have printed its summary.
	@printf "\n" # Add spacing
	@# Note: Ensure './scripts/check_file_length.sh' prints its own summary like "✓ Warnings issued..." or "✓ Check passed".

# Run gofmt to format code
fmt:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Formatting code...$(NC)\n"
	@# go fmt will print its own errors if any
	@go fmt ./... && \
		printf "   $(ICON_OK) $(GREEN)Code formatted$(NC)\n" || \
		(printf "   $(ICON_FAIL) $(RED)Formatting failed (see errors above)$(NC)\n" && exit 1)
	@printf "\n" # Add spacing

# --- Tooling & Setup ---

# Install required tools (currently just golangci-lint)
install-tools:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Installing required tools...$(NC)\n"
	@printf $(LABEL_FMT) "golangci-lint:"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		printf "$(ICON_OK) $(GREEN)Already installed$(NC)\n"; \
	else \
		printf "$(ICON_INFO) $(BLUE)Installing...$(NC)" ;\
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest && \
		printf " $(ICON_OK) $(GREEN)Installed$(NC)\n" || \
		(printf " $(ICON_FAIL) $(RED)Installation failed$(NC)\n" && exit 1); \
	fi
	@printf "   $(ICON_OK) $(GREEN)Tools installation check complete$(NC)\n"
	@printf "\n" # Add spacing

# Check for required tools locally
check:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Checking for required tools...$(NC)\n"
	@printf $(LABEL_FMT) "Go:"
	@if command -v go >/dev/null 2>&1; then \
		printf "$(ICON_OK) $(GREEN)$(shell go version)$(NC)\n"; \
	else \
		printf "$(ICON_FAIL) $(RED)Not Found - Go is required$(NC)\n"; \
		exit 1; \
	fi
	@printf $(LABEL_FMT) "golangci-lint:"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		printf "$(ICON_OK) $(GREEN)Found$(NC)\n"; \
	else \
		printf "$(ICON_WARN) $(YELLOW)Not Found (run 'make install-tools')$(NC)\n"; \
	fi
	@printf "   $(ICON_OK) $(GREEN)Tool check complete$(NC)\n"
	@printf "\n" # Add spacing

# Generate a tree view of the project
tree:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Generating project tree...$(NC)\n"
	@tree -I 'vendor|test' --dirsfirst > ./docs/project_directory_tree.txt
	@printf "   $(ICON_OK) $(GREEN)Project tree generated at ./docs/project_directory_tree.txt$(NC)\n"


# --- Convenience Targets ---

# Run CowGnition server with default settings
run:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Running CowGnition server...$(NC)\n"
	@./$(BINARY_NAME) serve

# Run with HTTP transport instead of stdio
run-http:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Running CowGnition server with HTTP transport...$(NC)\n"
	@./$(BINARY_NAME) serve --transport http

# Run with debug logging
run-debug:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Running CowGnition server with debug logging...$(NC)\n"
	@./$(BINARY_NAME) serve --debug

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
