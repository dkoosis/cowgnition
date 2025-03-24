# Test Directory Structure

This codebase follows a test/function/domain organization pattern:

## Test Functions

- `unit/`: Unit tests focusing on single components in isolation
- `integration/`: Tests that verify components work together
- `conformance/`: Tests that verify protocol compliance
- `fixtures/`: Test data and helpers
- `mocks/`: Mock implementations for testing
- `helpers/`: Utility functions for testing

## Test Domains

- `rtm/`: Tests for RTM API components
- `mcp/`: Tests for MCP protocol components
- `auth/`: Tests for authentication components
- `common/`: Shared components used across domains

## Running Tests

- Unit tests: `go test ./test/unit/...`
- Integration tests: `go test ./test/integration/...`
- Conformance tests: `go test ./test/conformance/...`
- All tests: `go test ./test/...`
