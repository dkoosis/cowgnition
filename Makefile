.PHONY: all build clean test lint fmt check update-docs update-errors

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

# Default target - run all checks and build.
all: update-docs check fmt lint test build update-errors
	@printf "${GREEN}✓ All checks passed and build completed successfully!${NC}\n"

# Update docs before build
update-docs:
	@printf "${BLUE}▶ Preparing documentation...${NC}\n"
	@./scripts/update_todo.sh && \
		printf "${GREEN}✓ Documentation prepared${NC}\n"

# Update errors after build
update-errors:
	@printf "${BLUE}▶ Updating error documentation...${NC}\n"
	@./scripts/update_todo.sh && \
		printf "${GREEN}✓ Error documentation updated${NC}\n"

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
	@go test ./... && \
		printf "${GREEN}✓ Tests passed${NC}\n" || \
		(printf "${RED}✗ Tests failed${NC}\n" && exit 1)

# Run linters
lint:
	@printf "${BLUE}▶ Running linters...${NC}\n"
	@go vet ./... && \
		printf "${GREEN}✓ Code looks good${NC}\n" || \
		(printf "${RED}✗ Linting issues found${NC}\n" && exit 1)

# Run gofmt
fmt:
	@printf "${BLUE}▶ Formatting code...${NC}\n"
	@go fmt ./...
	@printf "${GREEN}✓ Code formatted${NC}\n"

# Check for required tools
check:
	@printf "${BLUE}▶ Checking for required tools...${NC}\n"
	@printf "  Go:            "
	@if command -v go >/dev/null 2>&1; then printf "${GREEN}✓${NC}\n"; else printf "${RED}✗${NC}\n"; fi
	@printf "${GREEN}✓ Tool check complete${NC}\n"

# Help target
help:
	@printf "${BLUE}CowGnition Make Targets:${NC}\n"
	@printf "  %-16s %s\n" "all" "Run checks, formatting, tests, and build (default)"
	@printf "  %-16s %s\n" "build" "Build the application"
	@printf "  %-16s %s\n" "clean" "Clean build artifacts"
	@printf "  %-16s %s\n" "test" "Run tests"
	@printf "  %-16s %s\n" "lint" "Run linters"
	@printf "  %-16s %s\n" "fmt" "Format code"
	@printf "  %-16s %s\n" "check" "Check for required tools"
	@printf "  %-16s %s\n" "update-docs" "Update documentation files"
