package testing

import (
	"fmt"
	"strings"
	"testing"
)

// ANSI escape codes for colors and text styles.
const (
	Reset  = "\033[0m"
	Bold   = "\033[1m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Cyan   = "\033[36m"
)

// SectionDivider prints a formatted section divider.
func SectionDivider(t *testing.T, name string) {
	t.Helper()
	width := 80
	nameLen := len(name)
	padding := width - nameLen - 4 // 4 for " [ " and " ] "

	if padding < 0 {
		padding = 0
	}

	leftPad := padding / 2
	rightPad := padding - leftPad

	// Use t.Logf for direct integration with go test output.
	t.Logf("\n%s%s%s [ %s%s%s ] %s%s%s\n",
		Bold, Cyan, strings.Repeat("=", leftPad),
		Yellow, name, Cyan,
		strings.Repeat("=", rightPad), Reset, Bold)
}

// TestResult stores information about a single test's outcome.
type TestResult struct {
	Name    string
	Success bool
	Message string // Used for detailed error messages, including panics.
}

// TestRunner manages test execution and reporting.
type TestRunner struct {
	t       *testing.T
	results []TestResult
}

// NewTestRunner creates a new TestRunner instance.
func NewTestRunner(t *testing.T) *TestRunner {
	t.Helper()
	return &TestRunner{
		t:       t,
		results: []TestResult{},
	}
}

// Run executes a sub-test and records the result.
func (r *TestRunner) Run(name string, fn func(t *testing.T)) {
	r.t.Run(name, func(t *testing.T) {
		success := true
		var message string

		// Use t.Cleanup for robust failure handling (including panics).
		t.Cleanup(func() {
			if r := recover(); r != nil {
				success = false
				message = fmt.Sprintf("Panic: %v", r)
				t.Errorf("Test panicked: %v", r) // Log the panic.
			}

			if t.Failed() {
				success = false
				// If t.Errorf was called, the message is already in the output.
			}

			r.results = append(r.results, TestResult{
				Name:    t.Name(), // Full test name.
				Success: success,
				Message: message,
			})
		})

		fn(t) // Execute the actual test function.
	})
}

// Summary prints a summary of the test run.
func (r *TestRunner) Summary() {
	passed := 0
	failedTests := []TestResult{}

	for _, result := range r.results {
		if result.Success {
			passed++
		} else {
			failedTests = append(failedTests, result)
		}
	}

	r.t.Logf("\n%s%sTest Summary: %d/%d passed%s\n",
		Bold,
		func() string {
			if passed == len(r.results) {
				return Green
			}
			return Red
		}(), // Inline function for color.
		passed, len(r.results),
		Reset)

	if len(failedTests) > 0 {
		r.t.Logf("%sFailed tests:%s\n", Bold, Reset)
		for _, result := range failedTests {
			r.t.Logf("  - %s%s%s\n", Red, result.Name, Reset) // Color failed test names.
			if result.Message != "" {
				r.t.Logf("    %s%s%s\n", Red, result.Message, Reset) // And any panic messages
			}
		}
		r.t.Fail() // Mark the overall test suite as failed.
	}
}

// Helper functions (assertions)

func AssertEquals(t *testing.T, expected, actual interface{}, message string) {
	t.Helper()
	if expected != actual {
		t.Errorf("%s\nExpected: %v\nActual:   %v", message, expected, actual)
	}
}

func AssertTrue(t *testing.T, condition bool, message string) {
	t.Helper()
	if !condition {
		t.Errorf("%s\nExpected condition to be true", message)
	}
}

func AssertFalse(t *testing.T, condition bool, message string) {
	t.Helper()
	if condition {
		t.Errorf("%s\nExpected condition to be false", message)
	}
}

func AssertNil(t *testing.T, value interface{}, message string) {
	t.Helper()
	if value != nil {
		t.Errorf("%s\nExpected nil, got: %v", message, value)
	}
}

func AssertNotNil(t *testing.T, value interface{}, message string) {
	t.Helper()
	if value == nil {
		t.Errorf("%s\nExpected non-nil value", message)
	}
}

func AssertContains(t *testing.T, str, substr string, message string) {
	t.Helper()
	if !strings.Contains(str, substr) {
		t.Errorf("%s\nExpected string to contain: %q\nString: %q", message, substr, str)
	}
}

// SuccessMessage formats a success message.
func SuccessMessage(format string, args ...interface{}) string {
	return fmt.Sprintf("%s%s%s%s", Green, Bold, fmt.Sprintf(format, args...), Reset)
}

// ErrorMessage formats an error message.
func ErrorMessage(format string, args ...interface{}) string {
	return fmt.Sprintf("%s%s%s%s", Red, Bold, fmt.Sprintf(format, args...), Reset)
}

// WarningMessage formats a warning message.
func WarningMessage(format string, args ...interface{}) string {
	return fmt.Sprintf("%s%s%s%s", Yellow, Bold, fmt.Sprintf(format, args...), Reset)
}

// InfoMessage formats an info message.
func InfoMessage(format string, args ...interface{}) string {
	return fmt.Sprintf("%s%s%s", Blue, fmt.Sprintf(format, args...), Reset)
}
