// Package testing provides utilities for enhancing test output and execution.
package testing

import (
	"fmt"
	"strings"
	"testing"
)

// Colors for terminal output
const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	Gray    = "\033[37m"
)

// SectionDivider prints a section divider to visually separate test sections
func SectionDivider(name string) {
	width := 80
	nameLen := len(name)
	padding := width - nameLen - 4 // 4 for " [ " and " ] "
	
	if padding < 0 {
		padding = 0
	}
	
	leftPad := padding / 2
	rightPad := padding - leftPad
	
	fmt.Printf("\n%s%s%s [ %s%s%s ] %s%s%s\n\n", 
		Bold, Cyan, strings.Repeat("=", leftPad), 
		Yellow, name, Cyan,
		strings.Repeat("=", rightPad), Reset, Bold)
}

// SuccessMessage formats a success message with green color
func SuccessMessage(format string, args ...interface{}) string {
	return fmt.Sprintf("%s%s%s%s", Green, Bold, fmt.Sprintf(format, args...), Reset)
}

// ErrorMessage formats an error message with red color
func ErrorMessage(format string, args ...interface{}) string {
	return fmt.Sprintf("%s%s%s%s", Red, Bold, fmt.Sprintf(format, args...), Reset)
}

// WarningMessage formats a warning message with yellow color
func WarningMessage(format string, args ...interface{}) string {
	return fmt.Sprintf("%s%s%s%s", Yellow, Bold, fmt.Sprintf(format, args...), Reset)
}

// InfoMessage formats an info message with blue color
func InfoMessage(format string, args ...interface{}) string {
	return fmt.Sprintf("%s%s%s", Blue, fmt.Sprintf(format, args...), Reset)
}

// TestResult represents the result of a test
type TestResult struct {
	Name    string
	Success bool
	Message string
}

// TestRunner manages the execution and reporting of tests
type TestRunner struct {
	t        *testing.T
	results  []TestResult
	testName string
}

// NewTestRunner creates a new test runner
func NewTestRunner(t *testing.T, testName string) *TestRunner {
	SectionDivider(testName)
	return &TestRunner{
		t:        t,
		results:  []TestResult{},
		testName: testName,
	}
}

// Run runs a sub-test and records the result
func (r *TestRunner) Run(name string, fn func(t *testing.T)) {
	r.t.Run(name, func(t *testing.T) {
		fmt.Printf("%s▶ %s%s: ", Bold, name, Reset)
		
		success := true
		var message string
		
		// This is a simple way to capture test failures
		// In a complete implementation, you might use t.Cleanup() 
		// with a more robust capture mechanism
		originalFailNow := t.FailNow
		t.FailNow = func() {
			success = false
			message = "Test failed"
			originalFailNow()
		}
		
		fn(t)
		
		if success {
			fmt.Printf("%s\n", SuccessMessage("PASS"))
		} else {
			fmt.Printf("%s\n", ErrorMessage("FAIL"))
		}
		
		r.results = append(r.results, TestResult{
			Name:    name,
			Success: success,
			Message: message,
		})
	})
}

// Summary prints a summary of all test results
func (r *TestRunner) Summary() {
	passed := 0
	for _, result := range r.results {
		if result.Success {
			passed++
		}
	}
	
	fmt.Printf("\n%s%s Test Summary: %d/%d passed %s\n\n", 
		Bold, 
		passed == len(r.results) ? Green : Red,
		passed, len(r.results),
		Reset)
	
	if passed != len(r.results) {
		fmt.Printf("%sFailed tests:%s\n", Bold, Reset)
		for _, result := range r.results {
			if !result.Success {
				fmt.Printf("  - %s: %s\n", result.Name, result.Message)
			}
		}
		fmt.Println()
	}
}

// Helper functions for common assertions

// AssertEquals checks if expected equals actual
func AssertEquals(t *testing.T, expected, actual interface{}, message string) {
	if expected != actual {
		t.Errorf("%s\nExpected: %v\nActual:   %v", message, expected, actual)
	}
}

// AssertTrue checks if condition is true
func AssertTrue(t *testing.T, condition bool, message string) {
	if !condition {
		t.Errorf("%s\nExpected condition to be true", message)
	}
}

// AssertFalse checks if condition is false
func AssertFalse(t *testing.T, condition bool, message string) {
	if condition {
		t.Errorf("%s\nExpected condition to be false", message)
	}
}

// AssertNil checks if value is nil
func AssertNil(t *testing.T, value interface{}, message string) {
	if value != nil {
		t.Errorf("%s\nExpected nil, got: %v", message, value)
	}
}

// AssertNotNil checks if value is not nil
func AssertNotNil(t *testing.T, value interface{}, message string) {
	if value == nil {
		t.Errorf("%s\nExpected non-nil value", message)
	}
}

// AssertContains checks if string contains substring
func AssertContains(t *testing.T, str, substr string, message string) {
	if !strings.Contains(str, substr) {
		t.Errorf("%s\nExpected string to contain: %q\nString: %q", message, substr, str)
	}
}
