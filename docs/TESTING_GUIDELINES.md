# Testing Guidelines

This document provides an overview of the testing practices and commands for the CowGnition project.

## Test Commands

CowGnition uses [gotestsum](https://github.com/gotestyourself/gotestsum) for improved test output and reporting. The following test commands are available via the Makefile:

### Running Tests

```bash
# Run all tests with improved formatting
make test

# Run tests with coverage reporting
make test-coverage

# Run tests in a specific package
go test ./internal/config

# Run a specific test
go test ./internal/config -run=TestLoadConfig
```

### Test Output

By default, we use the `pkgname` format for test output, which provides a clean, organized view by package. The output includes:

- Package status (PASS/FAIL) with timing
- Test failures with detailed error messages
- Summary of passed, failed, and skipped tests

### Coverage Reporting

The `make test-coverage` command:
1. Runs all tests with coverage tracking
2. Generates a coverage report in HTML format (coverage.html)
3. Displays a summary of coverage percentages by package

### CI Integration

For CI environments, you can generate JUnit XML output with:

```bash
gotestsum --format pkgname --junitfile test-results.xml -- ./...
```

## Test Structure

Our tests follow these organizational principles:

- **Unit Tests**: Located alongside the code they test with `_test.go` suffix
- **Integration Tests**: Located in `test/integration/` directory
- **Conformance Tests**: Located in `test/conformance/` directory 
- **Fixtures**: Located in `test/fixtures/` directory
- **Helpers**: Reusable test utilities in `test/helpers/` directory
- **Mocks**: Mock implementations in `test/mocks/` directory

## Test Design Principles

1. **Isolation**: Tests should be independent and not rely on each other
2. **Predictability**: Tests should produce the same result on each run
3. **Clarity**: Test names should describe what functionality is being tested
4. **Completeness**: Tests should cover both success and failure cases
5. **Maintainability**: Complex setup should be extracted into helper functions
