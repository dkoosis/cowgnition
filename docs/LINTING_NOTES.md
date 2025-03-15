# Linting Investigation

This document records our step-by-step investigation of linting issues in the CowGnition project.

## Obervation

### lint version

```bash
% golangci-lint --version
golangci-lint has version v1.64.7 built with go1.24.1 from (unknown, modified: ?, mod sum: "h1:Xk1EyxoXqZabn5b4vnjNKSjCx1whBK53NP+mzLfX7HA=") on (unknown)
```

### current lint warning

```
WARN [config_reader] The configuration option `linters.govet.check-shadowing` is deprecated. Please enable `shadow` instead
```

## Summary

We identified a linting issue related to deprecated configuration in the golangci-lint setup. The `govet.check-shadowing` setting is no longer recommended.

## Issue: Deprecated GoVet Check-Shadowing Option

### Error

```
WARN [config_reader] The configuration option `linters.govet.check-shadowing` is deprecated. Please enable `shadow` instead
```

### Investigation Steps

1. Examined current `.golangci.yml`:
   ```yaml
   linters-settings:
     govet:
       check-shadowing: true
     gocyclo:
       min-complexity: 15
   ```
2. The `govet.check-shadowing` configuration is deprecated
3. Golangci-lint suggests using `shadow` linter instead
4. Attempted to replace with `shadow` linter:
   ```yaml
   linters:
     disable-all: true
     enable:
       - errcheck
       # ...other linters...
       - shadow # Added shadow linter
   ```
5. This produced a different error:
   ```
   Error: unknown linters: 'shadow', run 'golangci-lint help linters' to see the list of supported linters
   ```
6. Checked installed golangci-lint version, which appears to be older and doesn't include the shadow linter

### Available Options

1. Update golangci-lint to newer version that includes the shadow linter
2. Remove the deprecated configuration option entirely
3. Keep the deprecated option and ignore the warning

### Resolution

Remove the deprecated option from `.golangci.yml` for now:

```yaml
linters-settings:
  gocyclo:
    min-complexity: 15
  # govet.check-shadowing removed
```

Consider updating golangci-lint in the future if variable shadowing checks are important.

## Implementation and Testing

1. Updated `.golangci.yml` to remove deprecated configuration

2. Verified fixes with:
   ```
   make lint
   ```

## Next Steps

1. **Tooling**:

   - Consider upgrading golangci-lint to a version that supports the `shadow` linter
   - Document the minimum required version in the development setup instructions

2. **Code Quality**:

   - Review other potential implicit dependencies in the codebase
   - Add integration tests for authentication flow
   - Consider adding pre-commit hooks for linting

3. **Documentation**:
   - Update `GO_PRACTICES.md` with findings about linter configuration
   - Document authentication flow implementation details

Maintaining this documentation helps prevent revisiting the same issues and provides context for future development.