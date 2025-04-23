# file: Makefile

# Specify phony targets (targets not associated with files)
.PHONY: all tree build clean test lint golangci-lint fmt check deps install-tools check-line-length help run run-http run-debug check-gomod

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

# --- Variables ---

# Set the expected Go module path for this project
EXPECTED_MODULE_PATH := github.com/dkoosis/cowgnition

# Other Variables
BINARY_NAME := cowgnition
MAIN_PACKAGE := ./cmd
SCRIPT_DIR := ./scripts

# Find Go files, excluding vendor and test directories (adjust if needed)
GO_FILES := $(shell find . -name "*.go" -not -path "./vendor/*" -not -path "./test/*")
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT_HASH := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -ldflags "-X main.version=${VERSION} -X main.commitHash=${COMMIT_HASH} -X main.buildDate=${BUILD_DATE}"

# Line length check configuration
WARN_LINES := 350  # Warn if lines exceed this
FAIL_LINES := 700  # Fail if lines exceed this

# --- Core Targets ---

# Default target - run all checks and build
all: tree check deps fmt golangci-lint check-line-length test build
	@printf "$(GREEN)$(BOLD)✨ All checks passed and build completed successfully! ✨$(NC)\n"

# Build the application
build:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Building $(BINARY_NAME)...$(NC)\n"
	@go build $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_PACKAGE) && \
		printf "   $(ICON_OK) Setting execute permissions...\n" && \
		chmod +x $(BINARY_NAME) && \
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
	@printf "$(ICON_START) $(BOLD)$(BLUE)Synchronizing dependencies...$(NC)\n"
	@printf "   $(ICON_INFO) Running go mod tidy...\n"
	@go mod tidy; \
	if [ $$? -ne 0 ]; then \
		printf "   $(ICON_FAIL) $(RED)Failed to tidy dependencies$(NC)\n"; \
		exit 1; \
	else \
		printf "   $(ICON_OK) $(GREEN)Dependencies tidied successfully$(NC)\n"; \
	fi
	@printf "   $(ICON_INFO) Running go mod download...\n"
	@go mod download > /dev/null 2>&1; \
	if [ $$? -eq 0 ]; then \
		printf "   $(ICON_OK) $(GREEN)Dependencies downloaded successfully$(NC)\n"; \
	else \
		printf "   $(ICON_FAIL) $(RED)Failed to download dependencies$(NC)\n"; \
		exit 1; \
	fi
	@printf "\n" # Add spacing

# --- Quality & Testing ---

# Run tests
test:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Running tests with gotestsum...$(NC)\n"
	@# gotestsum runs 'go test' underneath and summarizes the output.
	@# Pass RTM credentials if needed (ensure these vars are set in your env)
	@RTM_API_KEY=$(RTM_API_KEY) RTM_SHARED_SECRET=$(RTM_SHARED_SECRET) gotestsum --format testdox -- -coverprofile=coverage.out ./... && \
		printf "   $(ICON_OK) $(GREEN)Tests passed$(NC)\n" || \
		(printf "   $(ICON_FAIL) $(RED)Tests failed$(NC)\n" && exit 1)
	@printf "\n" # Add spacing

# Run basic Go linter (go vet)
lint:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Running basic linter (go vet)...$(NC)\n"
	@go vet ./... && \
		printf "   $(ICON_OK) $(GREEN)go vet passed$(NC)\n" || \
		(printf "   $(ICON_FAIL) $(RED)go vet found issues$(NC)\n" && exit 1)
	@printf "\n" # Add spacing

# Run comprehensive golangci-lint (includes format checks)
golangci-lint: install-tools # Ensure tool is installed first
	@printf "$(ICON_START) $(BOLD)$(BLUE)Running golangci-lint (linters + format check)...$(NC)\n"
	@# The golangci-lint tool produces its own output.
	@# 'run' implicitly uses formatters defined in config for checking.
	@golangci-lint run ./... && \
		printf "   $(ICON_OK) $(GREEN)golangci-lint passed$(NC)\n" || \
		(printf "   $(ICON_FAIL) $(RED)golangci-lint failed (see errors above)$(NC)\n" && exit 1)
	@printf "\n" # Add spacing

# Check Go file line lengths using external script
check-line-length:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Checking file lengths (warn > $(WARN_LINES), fail > $(FAIL_LINES))...$(NC)\n"
	@if [ ! -x "$(SCRIPT_DIR)/check_file_length.sh" ]; then \
		printf "   $(ICON_FAIL) $(RED)Error: Script '$(SCRIPT_DIR)/check_file_length.sh' not found or not executable.$(NC)\n"; \
		exit 1; \
	fi
	@# Execute the script. It should handle its own output and exit status.
	@$(SCRIPT_DIR)/check_file_length.sh $(WARN_LINES) $(FAIL_LINES) $(GO_FILES) || \
		(printf "   $(ICON_FAIL) $(RED)Check failed (see script output above)$(NC)\n" && exit 1)
	@# Script should print its own success message if it passes.
	@printf "\n" # Add spacing

# Check if go.mod exists and has the correct module path
check-gomod:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Checking go.mod module path...$(NC)\n"
	@if [ ! -f "go.mod" ]; then \
		printf "   $(ICON_FAIL) $(RED)go.mod file is missing$(NC)\n"; \
		printf "   $(ICON_INFO) $(YELLOW)Create a go.mod file with: go mod init $(EXPECTED_MODULE_PATH)$(NC)\n"; \
		exit 1; \
	fi
	@if ! grep -q "^module $(EXPECTED_MODULE_PATH)$$" go.mod; then \
		printf "   $(ICON_FAIL) $(RED)go.mod has incorrect module path$(NC)\n"; \
		printf "   $(ICON_INFO) $(YELLOW)Expected: module $(EXPECTED_MODULE_PATH)$(NC)\n"; \
		printf "   $(ICON_INFO) $(YELLOW)Found: $$(grep '^module' go.mod)$(NC)\n"; \
		exit 1; \
	fi
	@printf "   $(ICON_OK) $(GREEN)go.mod has correct module path$(NC)\n"
	@printf "\n" # Add spacing

# Format code using configured formatters (golangci-lint v2+)
fmt: install-tools # Ensure tool is installed first
	@printf "$(ICON_START) $(BOLD)$(BLUE)Formatting code using golangci-lint fmt...$(NC)\n"
	@# Uses formatters defined in .golangci.yml (e.g., gofmt, goimports)
	@golangci-lint fmt ./... && \
		printf "   $(ICON_OK) $(GREEN)Code formatted$(NC)\n" || \
		(printf "   $(ICON_FAIL) $(RED)Formatting failed (see errors above)$(NC)\n" && exit 1)
	@printf "\n" # Add spacing

# --- Tooling & Setup ---

# Install required Go tools (golangci-lint, gotestsum)
install-tools:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Checking/installing required Go tools...$(NC)\n"

	@# Check/Install golangci-lint
	@printf $(LABEL_FMT) "golangci-lint:"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		printf "$(ICON_OK) $(GREEN)Already installed$(NC) ($(shell golangci-lint --version))\n"; \
	else \
		printf "$(ICON_INFO) $(YELLOW)Not Found. Installing...$(NC)" ;\
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest && \
		printf " $(ICON_OK) $(GREEN)Installed$(NC) ($(shell golangci-lint --version))\n" || \
		(printf " $(ICON_FAIL) $(RED)Installation failed$(NC)\n" && exit 1); \
	fi

	@# Check/Install gotestsum
	@printf $(LABEL_FMT) "gotestsum:"
	@if command -v gotestsum >/dev/null 2>&1; then \
		printf "$(ICON_OK) $(GREEN)Already installed$(NC)\n"; \
	else \
		printf "$(ICON_INFO) $(YELLOW)Not Found. Installing...$(NC)" ;\
		go install gotest.tools/gotestsum@latest && \
		printf " $(ICON_OK) $(GREEN)Installed$(NC)\n" || \
		(printf " $(ICON_FAIL) $(RED)Installation failed$(NC)\n" && exit 1); \
	fi

	@printf "   $(ICON_OK) $(GREEN)Go tools check/installation complete$(NC)\n"
	@printf "\n" # Add spacing

# Check for required tools locally
check: install-tools # Optional dependency
	@printf "$(ICON_START) $(BOLD)$(BLUE)Checking for required tools and environment...$(NC)\n"
	
	@# Check for Go command
	@printf $(LABEL_FMT) "Go:"
	@if command -v go >/dev/null 2>&1; then \
		printf "$(ICON_OK) $(GREEN)Installed$(NC) ($(shell go version))\n"; \
	else \
		printf "$(ICON_FAIL) $(RED)Not installed$(NC)\n"; \
		exit 1; \
	fi
	
	@# Check for tree command
	@printf $(LABEL_FMT) "tree:"
	@if command -v tree >/dev/null 2>&1; then \
		printf "$(ICON_OK) $(GREEN)Installed$(NC)\n"; \
	else \
		printf "$(ICON_WARN) $(YELLOW)Not installed$(NC) (needed for 'tree' target)\n"; \
	fi

	@# Check Go Bin Path
	@printf $(LABEL_FMT) "Go Bin Path:"
	@if [ -d "$(shell go env GOPATH)/bin" ] && echo "$$PATH" | grep -q "$(shell go env GOPATH)/bin"; then \
		printf "$(ICON_OK) $(GREEN)Verified in PATH$(NC)\n"; \
	else \
		printf "$(ICON_WARN) $(YELLOW)Not in PATH$(NC) (add $(shell go env GOPATH)/bin to your PATH)\n"; \
	fi
	
	@printf "   $(ICON_OK) $(GREEN)Tool and environment check complete$(NC)\n"
	@printf "\n" # Add spacing

# Generate a tree view of the project
tree:
	@printf "$(ICON_START) $(BOLD)$(BLUE)Generating project tree...$(NC)\n"
	@# Ensure the docs directory exists
	@mkdir -p ./docs
	@# Check if tree command exists before running
	@if ! command -v tree > /dev/null; then \
		printf "   $(ICON_WARN) $(YELLOW)'tree' command not found. Skipping tree generation.$(NC)\n"; \
	else \
		tree -F -I 'vendor|test|docs|.git|.idea|bin|coverage.out|$(BINARY_NAME)' --dirsfirst > ./docs/project_directory_tree.txt && \
		printf "   $(ICON_OK) $(GREEN)Project tree generated at ./docs/project_directory_tree.txt$(NC)\n" || \
		printf "   $(ICON_FAIL) $(RED)Failed to generate project tree.$(NC)\n"; \
	fi
	@printf "\n" # Add spacing

# --- Convenience Targets ---

# Run CowGnition server with default settings
# Depends on build to ensure the binary is up-to-date
run: build
	@printf "$(ICON_START) $(BOLD)$(BLUE)Running CowGnition server...$(NC)\n"
	@./$(BINARY_NAME) serve

# Run with HTTP transport instead of stdio
run-http: build
	@printf "$(ICON_START) $(BOLD)$(BLUE)Running CowGnition server with HTTP transport...$(NC)\n"
	@./$(BINARY_NAME) serve --transport http

# Run with debug logging
run-debug: build
	@printf "$(ICON_START) $(BOLD)$(BLUE)Running CowGnition server with debug logging...$(NC)\n"
	@./$(BINARY_NAME) serve --debug

# --- Help ---

# Help target: Display available commands
help:
	@printf "$(BLUE)$(BOLD)CowGnition Make Targets:$(NC)\n"
	@printf "  %-20s %s\n" "all" "Run checks, format, tests, and build (default)"
	@printf "  %-20s %s\n" "build" "Build the application binary ($(BINARY_NAME))"
	@printf "  %-20s %s\n" "clean" "Clean build artifacts and caches"
	@printf "  %-20s %s\n" "deps" "Tidy and download Go module dependencies"
	@printf "  %-20s %s\n" "install-tools" "Install/update required development tools (golangci-lint)"
	@printf "  %-20s %s\n" "check" "Check if required tools (Go, golangci-lint, etc.) are installed"
	@printf "  %-20s %s\n" "check-gomod" "Check if go.mod module path is correct"
	@printf "\n$(YELLOW)Code Quality & Testing:$(NC)\n"
	@printf "  %-20s %s\n" "test" "Run tests using gotestsum"
	@printf "  %-20s %s\n" "lint" "Run basic 'go vet' checks (subset of golangci-lint)"
	@printf "  %-20s %s\n" "golangci-lint" "Run comprehensive linters and format checks"
	@printf "  %-20s %s\n" "fmt" "Format code using formatters configured in .golangci.yml"
	@printf "  %-20s %s\n" "check-line-length" "Check Go file line count (W:$(WARN_LINES), F:$(FAIL_LINES))"
	@printf "\n$(YELLOW)Running the Application:$(NC)\n"
	@printf "  %-20s %s\n" "run" "Build and run the server with default settings"
	@printf "  %-20s %s\n" "run-http" "Build and run the server with HTTP transport"
	@printf "  %-20s %s\n" "run-debug" "Build and run the server with debug logging enabled"
	@printf "\n$(YELLOW)Other:$(NC)\n"
	@printf "  %-20s %s\n" "tree" "Generate project directory tree view in ./docs/"
	@printf "  %-20s %s\n" "help" "Display this help message"