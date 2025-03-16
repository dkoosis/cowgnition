# Go Development Best Practices and Tooling

This document outlines our standard tools and practices for Go development. These guidelines help ensure code quality, maintainability, and consistency across our Go projects.

## Core Development Tools

### Required Tools

| Tool                                                             | Purpose                            | Installation                                                            |
| ---------------------------------------------------------------- | ---------------------------------- | ----------------------------------------------------------------------- |
| [Go](https://go.dev/dl/)                                         | The Go compiler and toolchain      | OS-specific package manager or download from go.dev                     |
| [golangci-lint](https://golangci-lint.run/usage/install/)        | Comprehensive linting tool         | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` |
| [goimports](https://pkg.go.dev/golang.org/x/tools/cmd/goimports) | Import organization and formatting | `go install golang.org/x/tools/cmd/goimports@latest`                    |
| [gotestsum](https://github.com/gotestyourself/gotestsum)         | Enhanced test output and reporting | `go install gotest.tools/gotestsum@latest`                              |
| [staticcheck](https://staticcheck.io/docs/getting-started/)      | Advanced static analysis tool      | `go install honnef.co/go/tools/cmd/staticcheck@latest`                  |

### Additional Tooling Decisions

| Tool                                                 | Status        | Purpose              | Notes                                                             |
| ---------------------------------------------------- | ------------- | -------------------- | ----------------------------------------------------------------- |
| [gopls](https://pkg.go.dev/golang.org/x/tools/gopls) | **USING**     | Go language server   | Required for all developers; ensures consistent editor experience |
| [dlv](https://github.com/go-delve/delve)             | **USING**     | Debugger             | Standard debugger for all Go development                          |
| [mockgen](https://github.com/golang/mock)            | **USING**     | Mock generation      | We use this exclusively for mocking in tests                      |
| [entr](https://github.com/eradman/entr)              | **USING**     | File watcher         | Required for our development workflow                             |
| [wire](https://github.com/google/wire)               | **NOT USING** | Dependency injection | We prefer manual DI over code generation for this                 |
| [testify](https://github.com/stretchr/testify)       | **EVALUATE**  | Testing framework    | We use standard library testing only                              |
| [ginkgo/gomega](https://onsi.github.io/ginkgo/)      | **NOT USING** | BDD testing          | We prefer table-driven tests with standard library                |
| [cobra](https://github.com/spf13/cobra)              | **PREFERRED** | CLI framework        | If CLI functionality is needed, we will use Cobra                 |
| [viper](https://github.com/spf13/viper)              | **PREFERRED** | Configuration        | When robust configuration is needed beyond simple flags           |
| [zap](https://github.com/uber-go/zap)                | **PREFERRED** | Logging              | Our standard for structured logging when needed                   |

## Test Tooling with gotestsum

We've standardized on `gotestsum` for test formatting and output. This decision provides several benefits:

1. **Improved readability**: Clear, organized test output with better formatting than standard `go test`
2. **Configurable formats**: Multiple output formats depending on needs:
   - `dots`: Compact output showing each test as a dot (good for large test suites)
   - `pkgname`: Grouped by package with clean pass/fail indicators
   - `testname`: Lists all tests with pass/fail status
   - `standard-verbose`: Similar to `go test -v` but better formatted
   - `standard-quiet`: Minimal output, good for CI
3. **JUnit XML integration**: Provides CI integration with test reporting systems
4. **Failure summary**: Provides concise failure summary at the end of all tests
5. **Watch mode**: Supports watching for changes and re-running tests

Usage examples:

```bash
# Run tests with package-focused output (default in our Makefile)
gotestsum --format pkgname

# Run tests with minimal output (good for CI)
gotestsum --format dots

# Generate JUnit XML for CI integration
gotestsum --format pkgname --junitfile unit-tests.xml

# Watch mode for TDD workflow
gotestsum --watch
```

For our project, `gotestsum` is integrated into the Makefile, and developers should use `make test` rather than running `go test` directly.

## Code Style and Quality Practices

1. **Code Formatting**

   - Always run `goimports` or at minimum `gofmt` to ensure consistent formatting
   - Configure your editor for automatic formatting on save
   - Format should be enforced in CI/CD pipelines

2. **Linting**

   - Use `golangci-lint` with our standard configuration
   - Fix linting issues before committing code
   - Our standard linters include: `govet`, `staticcheck`, `gosec`, `errcheck`, `ineffassign`, and `gocritic`

3. **Documentation**

   - Document all exported functions, types, and packages
   - Follow Go's standard comment style (see [godoc documentation](https://go.dev/blog/godoc))
   - Include examples where appropriate
   - All comments should end with a period, per Go style

4. **Error Handling**
   
   - Use explicit error checking for all operations that can fail
   - Wrap errors with context using `fmt.Errorf("context: %w", err)`
   - Return errors early to avoid deep nesting
   - Use custom error types for specific error conditions that need handling

5. **Testing**
   - Write unit tests for all packages
   - Use table-driven tests for comprehensive case coverage
   - Target at least 70% code coverage for critical functionality
   - Use mocks appropriately to isolate unit tests
   - Use `t.Helper()` for test helper functions to improve error reporting

## Project Structure

We follow the idiomatic Go project layout:

```
project-name/
├── cmd/                    # Command-line applications
│   └── app/                # Main applications
├── internal/               # Private application code
│   ├── service/            # Core business logic
│   ├── handler/            # HTTP handlers
│   └── repository/         # Data access
├── pkg/                    # Public libraries that can be imported
├── api/                    # API definitions and docs
├── configs/                # Configuration files
├── scripts/                # Build and utility scripts
├── test/                   # Additional test tools and data
├── docs/                   # Documentation
├── examples/               # Example code
├── go.mod                  # Module definition
└── go.sum                  # Dependency checksum
```

For more details, see [Standard Go Project Layout](https://github.com/golang-standards/project-layout).

## Development Workflow

1. **Module Management**

   - Use Go modules for dependency management
   - Pin dependencies to specific versions
   - Regularly update dependencies and review changes
   - Run `go mod tidy` before committing changes

2. **Commit Standards**

   - Write clear, descriptive commit messages
   - Reference issue numbers when applicable
   - Keep commits focused on single concerns
   - Use the format: `area: brief description` (e.g., `auth: add token refreshing`)

3. **Code Review Guidelines**

   - Review for readability, correctness, and design
   - Ensure tests are included with new features
   - Verify error handling is comprehensive
   - Check for potential concurrency issues

4. **CI/CD Integration**
   - Run linting, formatting checks, and tests in CI
   - Build binaries for multiple platforms
   - Use automated security scanning

## Go Principles We Follow

Our development is guided by the [Go Proverbs](https://go-proverbs.github.io/):

- Clear is better than clever
- Errors are values
- Don't panic
- Make the zero value useful
- Interface{} says nothing
- Gofmt's style is no one's favorite, yet gofmt is everyone's favorite
- A little copying is better than a little dependency
- Syscall must always be guarded with build tags
- Concurrency is not parallelism
- Channels orchestrate; mutexes serialize
- The bigger the interface, the weaker the abstraction
- Documentation is for users

## Standard Project Files

Every project should include:

1. **README.md** - Project overview, setup instructions, and usage examples
2. **LICENSE** - Open source license (typically MIT or Apache 2.0)
3. **.golangci.yml** - Linter configuration
4. **Makefile** or equivalent build script
5. **.gitignore** appropriate for Go projects
6. **go.mod** and **go.sum** for dependency management

## Function Size and Complexity Guidelines

- Keep functions focused on a single responsibility
- Aim for functions under 30 lines of code where possible
- If a function exceeds 50 lines, consider refactoring
- Maintain cyclomatic complexity under 15 (measured by gocyclo)
- If a file exceeds 300 lines, evaluate splitting it into multiple files
- Use clear, descriptive function and variable names

## References

- [Effective Go](https://go.dev/doc/effective_go) - Official guide to writing idiomatic Go code
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) - Common comments in code reviews
- [Standard Go Project Layout](https://github.com/golang-standards/project-layout) - Common project organization
- [Go Proverbs](https://go-proverbs.github.io/) - Concise guiding principles
- [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md) - Comprehensive style guidelines
- [Dave Cheney's Practical Go](https://dave.cheney.net/practical-go/presentations/qcon-china.html) - Practical Go lessons
