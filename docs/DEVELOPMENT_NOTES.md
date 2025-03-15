# Development Notes

This document logs key decisions, tooling quirks, and findings to avoid revisiting the same issues in the future.

## Linting Configuration

### GoLangCI-Lint Version Quirks

- **Issue**: The `shadow` linter isn't available in all versions of golangci-lint
- **Decision**: Remove the deprecated `govet.check-shadowing` option entirely rather than replacing it with the `shadow` linter
- **Date**: March 15, 2025
- **Resolution**: Modified `.golangci.yml` to remove the `check-shadowing` option from `govet` settings

### Missing RTM Service Methods

- **Issue**: Linter errors for undefined methods `IsAuthenticated` and `CleanupExpiredFlows` in `internal/rtm/service.go`
- **Decision**: Implement these methods to complete the RTM authentication flow
- **Date**: March 15, 2025
- **Resolution**: Added implementation for:
  - `IsAuthenticated()` - Verifies if the user has valid authentication
  - `CleanupExpiredFlows()` - Removes expired authentication flows
  - `StartAuthFlow()` - Initiates the RTM authentication process
  - `CompleteAuthFlow()` - Finalizes authentication with a frob

## Tooling Decisions

### Code Organization

- Package `rtm` should not import from `server` to avoid circular dependencies
- Authentication types and interfaces are defined in both `auth` and `rtm` packages to prevent cycles

### Build Configuration

- Using Go modules for dependency management
- Build tags used to separate MCP server implementations
- Version information injected at build time via ldflags

### Testing Strategy

- Unit tests focus on individual package functionality
- Integration tests use the real MCP protocol against mock RTM responses
- End-to-end tests verify authentication flows and API calls

## IDE Configuration

- `.editorconfig` ensures consistent formatting across editors
- VSCode settings configured for Go development with gopls
- Using goimports for automatic import organization

## Deployment Considerations

- MCP server needs to be registered with Claude Desktop
- Authentication tokens stored in user's home directory
- OAuth-like flow requires user interaction for initial setup
