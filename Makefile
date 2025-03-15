.PHONY: build clean test lint fmt vet sec ver help doc dev check-tools install-tools test-unit test-integration test-conformance test-coverage
# Colors for better output
GREEN := \033[0;32m
RED := \033[0;31m
YELLOW := \033[0;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

# Variables
BINARY_NAME := cowgnition
MAIN_PACKAGE := ./cmd/server
VERSION := $(shell git describe --tags --always --dirty || echo "dev")
LDFLAGS := -ldflags "-X main.version=${VERSION} -X main.commitHash=$(shell git rev-parse HEAD) -X main.buildDate=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')"
GOPATH ?= $(shell go env GOPATH)

# GO_FILES variable - define it before it's used in a rule
GO_FILES := $(shell find . -name "*.go" -not -path "./vendor/*")

# Get the module path from go.mod
MODULE_PATH := $(shell go list -m)

# Default target - run all checks and build
all: all-checks build

# Run all code quality checks
all-checks: lint vet fmt sec check-size
	@printf "${GREEN}✓ All code quality checks completed${NC}\n"

# Check for large files
check-size:
	@printf "${BLUE}⚙ Checking for large files...${NC}\n"
	@LARGE_FILES=$$(find . -name "*.go" -not -path "./vendor/*" -exec wc -l {} \; | awk '$$1 > 300 {print "   - " $$2 " (" $$1 " lines)"}'); \
	if [ -n "$$LARGE_FILES" ]; then \
		printf "${YELLOW}⚠ Files exceeding 300 line threshold:${NC}\n$$LARGE_FILES\n"; \
	else \
		printf "${GREEN}✓ No files exceed line threshold${NC}\n"; \
	fi

# Build the application
build: fmt
	@printf "${BLUE}⚙ Building...${NC}\n"
	@go build ${LDFLAGS} -o ${BINARY_NAME} ${MAIN_PACKAGE} && \
		printf "${GREEN}✓ Build successful: ${BINARY_NAME}${NC}\n" || \
		(printf "${RED}✗ Build failed${NC}\n"; exit 1)

# Clean build artifacts
clean:
	@printf "${BLUE}⚙ Cleaning...${NC}\n"
	@rm -f ${BINARY_NAME}
	@go clean -cache -testcache
	@printf "${GREEN}✓ Cleaned build artifacts${NC}\n"

# Run tests
test:
	@printf "${BLUE}⚙ Running tests...${NC}\n"
	@go test -v ./...

# Run linters with golangci-lint
lint:
	@printf "${BLUE}⚙ Running linters...${NC}\n"
	@golangci-lint run -v | grep -v "WARN \[config_reader\]" || true

# Format code with goimports (superset of gofmt)
fmt:
	@printf "${BLUE}⚙ Formatting code...${NC}\n"
	@goimports -w ${GO_FILES}
	@printf "${GREEN}✓ Code formatted${NC}\n"

# Run go vet
vet:
	@printf "${BLUE}⚙ Running go vet...${NC}\n"
	@go vet ./... 2>&1 | grep -v "^$$" || true
	@printf "${GREEN}✓ Go vet completed${NC}\n"

# Run gosec security scanner
sec:
	@printf "${BLUE}⚙ Running security scan...${NC}\n"
	@gosec -quiet ./... > /dev/null 2>&1; \
	if [ $$? -eq 0 ]; then \
		printf "${GREEN}✓ No security issues found${NC}\n"; \
	else \
		gosec ./... | grep -E "G[0-9]+.*|Issue.*|Severity.*" | head -n 10; \
		printf "${YELLOW}⚠ Security issues found${NC}\n"; \
	fi

# Print version information
ver:
	@printf "${BLUE}CowGnition Version Information${NC}\n"
	@printf "Version: ${VERSION}\n"
	@printf "Go version: $(shell go version)\n"

# Generate documentation with godoc and open in browser
doc:
	@printf "${BLUE}⚙ Generating and viewing documentation...${NC}\n"
	@godoc -http=:6060 & # Run godoc in the background
	@sleep 2 # Wait for godoc to start
	@open http://localhost:6060/pkg/${MODULE_PATH}/... # Open in browser
	@printf "${GREEN}✓ Documentation server started at http://localhost:6060${NC}\n"
	@printf "${YELLOW}Note: You'll need to manually stop the godoc process when done${NC}\n"

# Run with hot reloading (using entr - install with brew install entr)
dev:
	@printf "${BLUE}⚙ Starting development server with hot reload...${NC}\n"
	@find . -name "*.go" | entr -r go run ${LDFLAGS} ${MAIN_PACKAGE} --config configs/config.yaml

# Check if all required tools are installed
check-tools:
	@printf "${BLUE}⚙ Checking required tools...${NC}\n"
	@tools_missing=0; \
	which golangci-lint > /dev/null || { printf "${RED}✗ golangci-lint not found${NC} - Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest\n"; tools_missing=1; }; \
	which goimports > /dev/null || { printf "${RED}✗ goimports not found${NC} - Install with: go install golang.org/x/tools/cmd/goimports@latest\n"; tools_missing=1; }; \
	which entr > /dev/null || { printf "${RED}✗ entr not found${NC} - Install with: brew install entr (or your system's package manager)\n"; tools_missing=1; }; \
	which godoc > /dev/null || { printf "${RED}✗ godoc not found${NC} - It's part of the Go standard library. Ensure Go is installed correctly.\n"; tools_missing=1; }; \
	if [ $$tools_missing -eq 0 ]; then \
		printf "${GREEN}✓ All required tools are installed!${NC}\n"; \
	else \
		exit 1; \
	fi

# Install development tools
install-tools:
	@printf "${BLUE}⚙ Installing development tools...${NC}\n"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install github.com/securego/gosec/v2/cmd/gosec@latest
	@printf "${GREEN}✓ Development tools installed${NC}\n"

# Run only unit tests
test-unit:
	@echo "Running unit tests..."
	go test -v ./test/unit/...

# Run only integration tests
test-integration:
	@echo "Running integration tests..."
	go test -v ./test/integration/...

# Run only conformance tests
test-conformance:
	@echo "Running conformance tests..."
	go test -v ./test/conformance/...

# Run tests with code coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated at coverage.html"
Remember to add the new target names to your existing .PHONY declaration at the top of the file.RetryClaude can make mistakes. Please double-check responses.
# Help target
help:
	@printf "${BLUE}CowGnition Make Targets${NC}\n"
	@printf "${YELLOW}Usage: make [target]${NC}\n\n"
	@printf "${GREEN}Available targets:${NC}\n"
	@printf "  %-14s %s\n" "all" "Run all checks and build the application (default)"
	@printf "  %-14s %s\n" "all-checks" "Run all code quality checks"
	@printf "  %-14s %s\n" "check-size" "Check for files exceeding size threshold"
	@printf "  %-14s %s\n" "build" "Build the application"
	@printf "  %-14s %s\n" "clean" "Clean build artifacts"
	@printf "  %-14s %s\n" "test" "Run tests"
	@printf "  %-14s %s\n" "lint" "Run all linters with golangci-lint"
	@printf "  %-14s %s\n" "fmt" "Format code with goimports"
	@printf "  %-14s %s\n" "vet" "Run go vet"
	@printf "  %-14s %s\n" "sec" "Run gosec security scanner"
	@printf "  %-14s %s\n" "ver" "Print version information"
	@printf "  %-14s %s\n" "doc" "Generate documentation"
	@printf "  %-14s %s\n" "dev" "Run with hot reloading"
	@printf "  %-14s %s\n" "check-tools" "Check if required tools are installed"
	@printf "  %-14s %s\n" "install-tools" "Install development tools"
