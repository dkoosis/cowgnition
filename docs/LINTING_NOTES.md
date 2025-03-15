# Linting Investigation

This document records our step-by-step investigation of linting issues in the CowGnition project.

## Obervation

### lint version

```bash
% golangci-lint --version
golangci-lint has version v1.64.7 built with go1.24.1 from (unknown, modified: ?, mod sum: "h1:Xk1EyxoXqZabn5b4vnjNKSjCx1whBK53NP+mzLfX7HA=") on (unknown)
```

### available linters

```
% golangci-lint help linters
Enabled by default linters:
errcheck: Errcheck is a program for checking for unchecked errors in Go code. These unchecked errors can be critical bugs in some cases.
gosimple: Linter for Go source code that specializes in simplifying code. [auto-fix]
govet: Vet examines Go source code and reports suspicious constructs. It is roughly the same as 'go vet' and uses its passes. [auto-fix]
ineffassign: Detects when assignments to existing variables are not used. [fast]
staticcheck: It's a set of rules from staticcheck. It's not the same thing as the staticcheck binary. The author of staticcheck doesn't support or approve the use of staticcheck as a library inside golangci-lint. [auto-fix]
unused: Checks Go code for unused constants, variables, functions and types.

Disabled by default linters:
asasalint: Check for pass []any as any in variadic func(...any).
asciicheck: Checks that all code identifiers does not have non-ASCII symbols in the name. [fast]
bidichk: Checks for dangerous unicode character sequences. [fast]
bodyclose: Checks whether HTTP response body is closed successfully.
canonicalheader: Canonicalheader checks whether net/http.Header uses canonical header. [auto-fix]
containedctx: Containedctx is a linter that detects struct contained context.Context field.
contextcheck: Check whether the function uses a non-inherited context.
copyloopvar: A linter detects places where loop variables are copied. [fast, auto-fix]
cyclop: Checks function and package cyclomatic complexity. [fast]
decorder: Check declaration order and count of types, constants, variables and functions. [fast]
depguard: Go linter that checks if package imports are in a list of acceptable packages. [fast]
dogsled: Checks assignments with too many blank identifiers (e.g. x, _, _, _, := f()). [fast]
dupl: Detects duplicate fragments of code. [fast]
dupword: Checks for duplicate words in the source code. [fast, auto-fix]
durationcheck: Check for two durations multiplied together.
err113: Go linter to check the errors handling expressions. [auto-fix]
errchkjson: Checks types passed to the json encoding functions. Reports unsupported types and reports occurrences where the check for the returned error can be omitted.
errname: Checks that sentinel errors are prefixed with the `Err` and error types are suffixed with the `Error`.
errorlint: Errorlint is a linter for that can be used to find code that will cause problems with the error wrapping scheme introduced in Go 1.13. [auto-fix]
exhaustive: Check exhaustiveness of enum switch statements.
exhaustruct: Checks if all structure fields are initialized.
exptostd: Detects functions from golang.org/x/exp/ that can be replaced by std functions. [auto-fix]
fatcontext: Detects nested contexts in loops and function literals. [auto-fix]
forbidigo: Forbids identifiers.
forcetypeassert: Finds forced type assertions.
funlen: Checks for long functions. [fast]
gci: Checks if code and import statements are formatted, with additional rules. [fast, auto-fix]
ginkgolinter: Enforces standards of using ginkgo and gomega. [auto-fix]
gocheckcompilerdirectives: Checks that go compiler directive comments (//go:) are valid. [fast]
gochecknoglobals: Check that no global variables exist.
gochecknoinits: Checks that no init functions are present in Go code. [fast]
gochecksumtype: Run exhaustiveness checks on Go "sum types".
gocognit: Computes and checks the cognitive complexity of functions. [fast]
goconst: Finds repeated strings that could be replaced by a constant. [fast]
gocritic: Provides diagnostics that check for bugs, performance and style issues. [auto-fix]
gocyclo: Computes and checks the cyclomatic complexity of functions. [fast]
godot: Check if comments end in a period. [fast, auto-fix]
godox: Detects usage of FIXME, TODO and other keywords inside comments. [fast]
gofmt: Checks if the code is formatted according to 'gofmt' command. [fast, auto-fix]
gofumpt: Checks if code and import statements are formatted, with additional rules. [fast, auto-fix]
goheader: Checks if file header matches to pattern. [fast, auto-fix]
goimports: Checks if the code and import statements are formatted according to the 'goimports' command. [fast, auto-fix]
gomoddirectives: Manage the use of 'replace', 'retract', and 'excludes' directives in go.mod. [fast]
gomodguard: Allow and block list linter for direct Go module dependencies. This is different from depguard where there are different block types for example version constraints and module recommendations. [fast]
goprintffuncname: Checks that printf-like functions are named with `f` at the end. [fast]
gosec: Inspects source code for security problems.
gosmopolitan: Report certain i18n/l10n anti-patterns in your Go codebase.
grouper: Analyze expression groups. [fast]
iface: Detect the incorrect use of interfaces, helping developers avoid interface pollution. [auto-fix]
importas: Enforces consistent import aliases. [auto-fix]
inamedparam: Reports interfaces with unnamed method parameters. [fast]
interfacebloat: A linter that checks the number of methods inside an interface. [fast]
intrange: Intrange is a linter to find places where for loops could make use of an integer range. [auto-fix]
ireturn: Accept Interfaces, Return Concrete Types.
lll: Reports long lines. [fast]
loggercheck: Checks key value pairs for common logger libraries (kitlog,klog,logr,zap).
maintidx: Maintidx measures the maintainability index of each function. [fast]
makezero: Finds slice declarations with non-zero initial length.
mirror: Reports wrong mirror patterns of bytes/strings usage. [auto-fix]
misspell: Finds commonly misspelled English words. [fast, auto-fix]
mnd: An analyzer to detect magic numbers. [fast]
musttag: Enforce field tags in (un)marshaled structs.
nakedret: Checks that functions with naked returns are not longer than a maximum size (can be zero). [fast, auto-fix]
nestif: Reports deeply nested if statements. [fast]
nilerr: Finds the code that returns nil even if it checks that the error is not nil.
nilnesserr: Reports constructs that checks for err != nil, but returns a different nil value error.
nilnil: Checks that there is no simultaneous return of `nil` error and an invalid value.
nlreturn: Nlreturn checks for a new line before return and branch statements to increase code clarity. [fast, auto-fix]
noctx: Finds sending http request without context.Context.
nolintlint: Reports ill-formed or insufficient nolint directives. [fast, auto-fix]
nonamedreturns: Reports all named returns.
nosprintfhostport: Checks for misuse of Sprintf to construct a host with port in a URL. [fast]
paralleltest: Detects missing usage of t.Parallel() method in your Go test.
perfsprint: Checks that fmt.Sprintf can be replaced with a faster alternative. [auto-fix]
prealloc: Finds slice declarations that could potentially be pre-allocated. [fast]
predeclared: Find code that shadows one of Go's predeclared identifiers. [fast]
promlinter: Check Prometheus metrics naming via promlint. [fast]
protogetter: Reports direct reads from proto message fields when getters should be used. [auto-fix]
reassign: Checks that package variables are not reassigned.
recvcheck: Checks for receiver type consistency.
revive: Fast, configurable, extensible, flexible, and beautiful linter for Go. Drop-in replacement of golint. [auto-fix]
rowserrcheck: Checks whether Rows.Err of rows is checked successfully.
sloglint: Ensure consistent code style when using log/slog.
spancheck: Checks for mistakes with OpenTelemetry/Census spans.
sqlclosecheck: Checks that sql.Rows, sql.Stmt, sqlx.NamedStmt, pgx.Query are closed.
stylecheck: Stylecheck is a replacement for golint. [auto-fix]
tagalign: Check that struct tags are well aligned. [fast, auto-fix]
tagliatelle: Checks the struct tags.
testableexamples: Linter checks if examples are testable (have an expected output). [fast]
testifylint: Checks usage of github.com/stretchr/testify. [auto-fix]
testpackage: Linter that makes you use a separate _test package. [fast]
thelper: Thelper detects tests helpers which is not start with t.Helper() method.
tparallel: Tparallel detects inappropriate usage of t.Parallel() method in your Go test codes.
unconvert: Remove unnecessary type conversions.
unparam: Reports unused function parameters.
usestdlibvars: A linter that detect the possibility to use variables/constants from the Go standard library. [fast, auto-fix]
usetesting: Reports uses of functions with replacement inside the testing package. [auto-fix]
varnamelen: Checks that the length of a variable's name matches its scope.
wastedassign: Finds wasted assignment statements.
whitespace: Whitespace is a linter that checks for unnecessary newlines at the start and end of functions, if, for, etc. [fast, auto-fix]
wrapcheck: Checks that errors returned from external packages are wrapped.
wsl: Add or remove empty lines. [fast, auto-fix]
zerologlint: Detects the wrong usage of `zerolog` that a user forgets to dispatch with `Send` or `Msg`.
deadcode [deprecated]: Deprecated. [fast]
execinquery [deprecated]: Deprecated. [fast]
exhaustivestruct [deprecated]: Deprecated. [fast]
exportloopref [deprecated]: Deprecated.
golint [deprecated]: Deprecated. [fast]
gomnd [deprecated]: Deprecated. [fast]
ifshort [deprecated]: Deprecated. [fast]
interfacer [deprecated]: Deprecated. [fast]
maligned [deprecated]: Deprecated. [fast]
nosnakecase [deprecated]: Deprecated. [fast]
scopelint [deprecated]: Deprecated. [fast]
structcheck [deprecated]: Deprecated. [fast]
tenv [deprecated]: Tenv is analyzer that detects using os.Setenv instead of t.Setenv since Go1.17.
varcheck [deprecated]: Deprecated. [fast]

Linters presets:
bugs: asasalint, asciicheck, bidichk, bodyclose, contextcheck, durationcheck, errcheck, errchkjson, errorlint, exhaustive, gocheckcompilerdirectives, gochecksumtype, gosec, gosmopolitan, govet, loggercheck, makezero, musttag, nilerr, nilnesserr, noctx, protogetter, reassign, recvcheck, rowserrcheck, spancheck, sqlclosecheck, staticcheck, testifylint, zerologlint
comment: dupword, godot, godox, misspell
complexity: cyclop, funlen, gocognit, gocyclo, maintidx, nestif
error: err113, errcheck, errorlint, wrapcheck
format: gci, gofmt, gofumpt, goimports
import: depguard, gci, goimports, gomodguard
metalinter: gocritic, govet, revive, staticcheck
module: depguard, gomoddirectives, gomodguard
performance: bodyclose, fatcontext, noctx, perfsprint, prealloc
sql: rowserrcheck, sqlclosecheck
style: asciicheck, canonicalheader, containedctx, copyloopvar, decorder, depguard, dogsled, dupl, err113, errname, exhaustruct, exptostd, forbidigo, forcetypeassert, ginkgolinter, gochecknoglobals, gochecknoinits, goconst, gocritic, godot, godox, goheader, gomoddirectives, gomodguard, goprintffuncname, gosimple, grouper, iface, importas, inamedparam, interfacebloat, intrange, ireturn, lll, loggercheck, makezero, mirror, misspell, mnd, musttag, nakedret, nilnil, nlreturn, nolintlint, nonamedreturns, nosprintfhostport, paralleltest, predeclared, promlinter, revive, sloglint, stylecheck, tagalign, tagliatelle, testpackage, tparallel, unconvert, usestdlibvars, varnamelen, wastedassign, whitespace, wrapcheck, wsl
test: exhaustruct, paralleltest, testableexamples, testifylint, testpackage, thelper, tparallel, usetesting
unused: ineffassign, unparam, unused
%
```

## Summary

We identified and addressed two linting issues:

1. Missing method implementations in the RTM service
2. Deprecated linter configuration option

The solutions below fix these issues while maintaining code quality.

## Issue 1: Undefined Methods in RTM Service

### Error

```
internal/rtm/service.go:82:6: s.CleanupExpiredFlows undefined (type *Service has no field or method CleanupExpiredFlows)
internal/rtm/service.go:114:8: s.IsAuthenticated undefined (type *Service has no field or method IsAuthenticated)
```

### Investigation Steps

1. Identified references to two undefined methods in `internal/rtm/service.go`:
   - `IsAuthenticated()`
   - `CleanupExpiredFlows()`
2. Examined the service implementation:
   ```go
   // Service provides methods for interacting with RTM API.
   type Service struct {
       client       *Client
       tokenPath    string
       tokenManager *auth.TokenManager
       mu           sync.RWMutex
       lastSyncTime time.Time
       authStatus   Status
       authFlows    map[string]*Flow
   }
   ```
3. Found that `Initialize()` uses `authStatus` to track auth state, but no method exposes this
4. Found call to `CleanupExpiredFlows()` in a goroutine, but method not implemented
5. These methods are referenced but never defined, causing compilation errors

### Required Implementation

1. `IsAuthenticated()` - Should check if the service has a valid auth token:
   ```go
   func (s *Service) IsAuthenticated() bool {
       s.mu.RLock()
       status := s.authStatus
       s.mu.RUnlock()
       return status == StatusAuthenticated
   }
   ```
2. `CleanupExpiredFlows()` - Should remove expired auth flows:
   ```go
   func (s *Service) CleanupExpiredFlows() {
       s.mu.Lock()
       defer s.mu.Unlock()
       now := time.Now()
       for frob, flow := range s.authFlows {
           if now.Sub(flow.StartTime) > 24*time.Hour {
               delete(s.authFlows, frob)
           }
       }
   }
   ```

### Resolution

Implement both missing methods in `internal/rtm/service.go` to fix compilation errors.

## Issue 2: Deprecated GoVet Check-Shadowing Option

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

1. Added missing methods to `internal/rtm/service.go`:

   - `IsAuthenticated()`
   - `CleanupExpiredFlows()`

2. Updated `.golangci.yml` to remove deprecated configuration

3. Verified fixes with:
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
