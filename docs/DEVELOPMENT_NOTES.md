# Development Notes

## Testing Organization Refactoring - March 2025

### Decision: Reorganize Test Directory Structure

We've decided to revise our test directory organization to better support scaling with multiple API implementations (RTM, Asana, etc.) while maintaining clear test function separation.

### Target Structure:

test/
├── fixtures/ # All test fixtures
│ ├── rtm/ # RTM fixtures
│ └── common/ # Shared fixtures
├── helpers/ # Test helper functions
│ ├── common/ # Shared helpers
│ ├── mcp/ # MCP protocol helpers
│ └── rtm/ # RTM-specific helpers
├── mocks/ # Mock implementations
│ └── rtm/ # RTM service mocks
├── testdata/ # Static test data
├── unit/ # Unit tests by package
├── integration/ # Integration tests
│ └── rtm/ # RTM integration
└── conformance/ # Protocol conformance tests
├── mcp/ # MCP protocol validators & framework
└── rtm/ # RTM-specific conformance
Copy

### Implementation Approach:

1. Create the new directory structure
2. Move existing tests to appropriate locations
3. Update imports and references
4. Remove redundant directories and functions
5. Update CI/CD configurations as needed

This organization provides clearer separation of concerns, reduces duplication, and establishes a pattern that will scale as we add more API integrations.

## Linting Configuration

### GoLangCI-Lint Version Quirks

- **Issue**: The `shadow` linter isn't available in all versions of golangci-lint
- **Decision**: Remove the deprecated `govet.check-shadowing` option entirely rather than replacing it with the `shadow` linter
- **Date**: March 15, 2025
- **Resolution**: Modified `.golangci.yml` to remove the `check-shadowing` option from `govet` settings
- **Issue**: The `shadow` linter isn't available in all versions of golangci-lint
- **Decision**: Remove the deprecated `govet.check-shadowing` option entirely rather than replacing it with the `shadow` linter
- **Date**: March 15, 2025
- **Resolution**: Modified `.golangci.yml` to remove the `check-shadowing` option from `govet` settings
