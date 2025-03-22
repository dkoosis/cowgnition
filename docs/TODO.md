# Reorganized CowGnition - MCP Server Implementation Roadmap

## Implementation Priorities

### 1. AI-Friendly Project Structure Improvements (PRIORITY FOCUS)

```
Restructure the project to be more discoverable and reduce duplication:

1. Centralized Utility Functions:
   - Create a dedicated util package for all general-purpose utilities
   - Move existing helper functions from test files into appropriate util subpackages (util/url, util/string, etc.)
   - Add comprehensive godoc comments explaining function purpose, inputs/outputs, and edge cases

2. Test Helper Organization:
   - Create an internal/testutil package for all test helpers
   - Move common test functions to this package
   - Use consistent naming for test helpers (e.g., prefix with Assert or Test)
   - Add documentation describing when to use each helper

3. Package Boundaries and Dependencies:
   - Review and refactor circular dependencies
   - Ensure each package has a clear single responsibility
   - Replace package-level function variables with direct imports and calls
   - Add package-level documentation explaining purpose and contents

4. File Organization:
   - Create consistent patterns for file organization within packages
   - Add file-level comments describing contents and purpose
   - Consider splitting large files with mixed responsibilities
   - Use standard file naming conventions (e.g., helpers.go, models.go)

5. Documentation Enhancements:
   - Add explicit cross-references in comments
   - Update PROJECT_ORGANIZATION.md to document the new structure
   - Create a UTILITY_FUNCTIONS.md listing all available helpers by category
   - Add examples in godoc comments showing function usage

6. Naming and Conventions:
   - Establish and document consistent naming patterns
   - Rename confusing or abbreviated functions to be more descriptive
   - Use full words rather than abbreviations
   - Apply consistent casing conventions
```

### 2. Utility Package Consolidation

```
Improve code organization to prevent duplication and enhance discoverability:

- Move `internal/server/utils.go` functions to appropriate `pkg/util/*` packages
- Remove duplicate utility functions in:
  - `internal/server/utils.go` vs. `pkg/util/format/format.go` (`formatMarkdownTable`)
  - `internal/server/utils.go` vs. `pkg/util/validation/validation.go` (`validateMimeType`)
  - `internal/server/utils.go` vs. `pkg/util/stringutil/stringutil.go` (`coalesceString`)
  - `rtm/service.go` vs. `pkg/util/format/format.go` (`FormatTaskPriority`)
```

### 3. Test Utilities Rationalization

```
- Consolidate functions between `test/helpers/`, `test/util/testutil/`, and `test/conformance/`
- Move functions used across packages to `pkg/testutil/`
- Standardize functions needed in `mcp_live_resource_test.go` and `rtm_live_test_framework.go`
```

### 4. Package Responsibility Clarification

```
- `internal/testing/` → Move to `pkg/testutil/` or merge with `test/helpers/`
- Separate HTTP helpers from MCP-specific test utilities
- Create clear separation between general utilities and domain-specific ones
```

### 5. Folder Structure Flattening

```
- Replace `test/util/testutil/` with `pkg/testutil/`
- Consider renaming `pkg/util/stringutil` to `pkg/util/strings` for brevity
- Remove or merge nested test directories
```

### 6. Next Phase Steps for Code Reorganization

```
Continue the code reorganization with these specific steps:

1. Update imports in affected files:
   - Identify all files that would benefit from using the new utility packages
   - Update their imports and replace duplicated code with utility calls
   - Remove any redundant utility functions from original files

2. Split the large protocol_validators.go file:
   - Extract resource validation into resource_validator.go
   - Extract tool validation into tool_validator.go
   - Move generic response validation to response_validator.go

3. Reorganize the test directory structure:
   - Create subdirectories in test/conformance by test category
   - Move error tests, resource tests, and tool tests to appropriate subdirectories
   - Update imports and references in all test files

4. Update documentation:
   - Update PROJECT_ORGANIZATION.md with the new structure
   - Create a UTILITY_FUNCTIONS.md to document available utilities
   - Add package documentation for each new utility package
```

### 7. Enhanced AI-Assistant Compatibility

```
Make additional structural improvements to optimize for AI assistance:

1. File Size and Complexity:
   - Limit files to 300-500 lines maximum
   - Break up large files into focused, single-purpose modules
   - Ensure functions stay under 50 lines where possible
   - Refactor complex logic into smaller, named helper functions

2. Directory Structure:
   - Flatten directory hierarchy to maximum 2-3 levels deep
   - Use meaningful package names rather than excessive nesting
   - Group related functionality by domain rather than technical type
   - Eliminate redundant subdirectories

3. Navigation and Discovery Aids:
   - Add "SECTION:" marker comments to denote logical blocks of code
   - Create a CODEBASE_MAP.md with high-level overview of key files
   - Document common cross-file workflows and interactions
   - Add TOC-style comments at the top of larger files

4. Standardized Documentation:
   - Create template comment blocks for common structures
   - Standardize function header format for params, returns, examples
   - Add /examples folder with self-contained usage examples
   - Document architectural decisions in DECISIONS.md

5. Explicit Dependencies:
   - Use explicit imports rather than dot imports
   - Avoid side effects in package initialization
   - Document cross-package dependencies in README files
   - Add interface documentation explaining when to use each interface
```

### 8. Testing Framework Setup (IN PROGRESS)

```
Establish a comprehensive testing framework to ensure server quality:
1. ✅ Set up structured test directories for unit, integration, and conformance tests
2. ✅ Create test helpers and utilities for common testing patterns
3. ✅ Implement test fixtures for RTM API responses
4. Set up GitHub Actions workflow for automated testing
5. Configure test coverage reporting

Enhance test diagnostics and error reporting:
1. Create MCP-specific assertion helper functions to complement existing helpers
   - Add functions like assertStatusCode, assertContentType, assertJSONStructure
   - Encapsulate common validation logic with clear error messages
   - Ensure all helpers use t.Helper() for proper error reporting

2. Improve error message context in existing tests
   - Include test case name in error messages
   - Add request URL and method information to error reports
   - Include relevant request/response details in failures
   - Enhance response validation errors with field-specific context

3. Add request/response logging capabilities to MCPClient
   - Add optional debug mode to MCPClient
   - Implement request logging before sending
   - Add response logging after receiving
   - Make logging conditional based on debug flag
```

### 9. Code Quality Improvements

```
Improve code quality and maintainability:
1. ✅ Fix linting issues in test code
2. ✅ Refactor complex test functions to improve maintainability
3. Fix test failures:
   - Internal/config: Fix path validation to accept temp directories in tests
   - Internal/config: Fix parseInt test for partial matches
   - Internal/rtm: Fix CheckToken test for invalid token detection
   - Test/conformance: Fix resource endpoint test for nonexistent resources
4. Run comprehensive test suite with all fixes
5. Document testing patterns and best practices
6. Implement additional code quality checks
```

### 10. MCP Protocol Conformance Testing (PRIORITY FOCUS)

```
Implement comprehensive tests to verify compliance with the MCP specification:

1. Core endpoint verification:
   - Enhance TestMCPInitializeEndpoint to verify all required capabilities
   - Complete TestMCPResourceEndpoints with more thorough validation
   - Extend TestMCPToolEndpoints to test actual tool execution
   - Implement TestReadResourceAuthenticated (currently skipped)

2. Schema validation:
   - Create Go structs matching MCP protocol schemas
   - Implement JSON schema validators for responses
   - Validate all response fields conform to specification
   - Add validators for each message type (initialization, resources, tools)

3. Error handling validation:
   - Test responses to invalid inputs
   - Verify error formatting conforms to MCP specification
   - Test edge cases like missing parameters
   - Validate behavior with malformed requests

4. Testing helpers enhancement:
   - Complete validateMCPResource helper function
   - Complete validateMCPTool helper function
   - Add helpers for other response types
   - Create test fixture generators for common request patterns

5. Protocol flow conformance:
   - Test complete interaction sequences
   - Verify proper state transitions
   - Test concurrent operations
   - Validate protocol version compatibility
```

### 11. Test Runner Improvements ✅

```
Replace custom test runner with gotestsum for better test output:
1. ✅ Install gotestsum in development environment
2. ✅ Update Makefile to use gotestsum for test targets
3. ✅ Configure appropriate test output format (pkgname or testname)
4. ⏳ Add JUnit XML output for CI integration (will be addressed later)
5. ✅ Update documentation with new test commands
6. ✅ Remove custom test runner implementation
```

### 12. RTM API Integration Testing

```
Test integration with Remember The Milk API:
1. Create mock RTM server for testing without real API credentials
2. Implement tests for authentication flow
3. Test task listing with various filters
4. Test list operations and pagination handling
5. Test all supported RTM API operations
6. Validate error handling for API rate limits and failures
```

### 13. Performance and Load Testing

```
Verify server performance characteristics:
1. Set up benchmarking tools for response times
2. Test memory usage under load
3. Measure and optimize connection handling
4. Test with large datasets to ensure pagination works correctly
5. Measure and optimize startup and shutdown times
```

### 14. Build Environment Optimization

```
Improve dependency management and build process:
1. Update Go module dependencies
2. Set up versioning strategy
3. Configure reproducible builds
4. Document build requirements and development setup
5. Create isolated build environments
```

### 15. Feature Implementation

```
1. Task Resources Implementation
2. List Resources Implementation
3. Tag Resources Implementation
4. Task Management Tools
5. List Management Tools
6. Tag Management Tools
7. Note Management Tools
8. Search and Filter Tools
9. Client Integration Support
10. Security and Performance Optimization
```

### 16. Tasks Suitable for Less Capable AI Assistants

```
These tasks can be delegated to less capable AI assistants:

1. Documentation Enhancements
2. Test Case Expansion
3. Code Formatting and Style
4. Simple Utility Functions
5. Configuration File Templates
6. Linting Issue Resolution
7. Enhancing Logging
8. Data Structure Documentation
9. CLI Command Documentation
10. Error Message Enhancement
```

Enhance development experience:

1. Configure comprehensive linting with golangci-lint
2. Set up pre-commit hooks for quality checks
3. Create developer documentation
4. Implement debugging tools
5. Create tooling for easy local testing of MCP server
