// cmd/testrunner/main.go
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
)

// TestRunner holds the state and functionality for test output processing.
type TestRunner struct {
	Patterns       Patterns
	Colors         Colors
	PassCount      int
	FailCount      int
	SkipCount      int
	PassedPackages int
	FailedPackages int
	SeenPASS       bool
	SeenFAIL       bool
	StartTime      time.Time
}

// Patterns holds compiled regular expressions.
type Patterns struct {
	NoTest       *regexp.Regexp
	PassPackage  *regexp.Regexp
	FailPackage  *regexp.Regexp
	Run          *regexp.Regexp
	PassTest     *regexp.Regexp
	FailTest     *regexp.Regexp
	SkipTest     *regexp.Regexp
	SinglePass   *regexp.Regexp
	SingleFail   *regexp.Regexp
	ErrorMessage *regexp.Regexp
}

// Colors holds color formatting functions.
type Colors struct {
	Pass      func(...interface{}) string
	Fail      func(...interface{}) string
	Skip      func(...interface{}) string
	Run       func(...interface{}) string
	Package   func(...interface{}) string
	Info      func(...interface{}) string
	Error     func(...interface{}) string
	Header    func(...interface{}) string
	Highlight func(...interface{}) string
}

// NewTestRunner creates and initializes a TestRunner.
func NewTestRunner() *TestRunner {
	return &TestRunner{
		Patterns:  compilePatterns(),
		Colors:    setupColors(),
		StartTime: time.Now(),
	}
}

// compilePatterns creates all needed regex patterns.
func compilePatterns() Patterns {
	return Patterns{
		NoTest:       regexp.MustCompile(`^\?\s+(github\.com/\S+)\s+\[no test files\]$`),
		PassPackage:  regexp.MustCompile(`^ok\s+(github\.com/\S+)\s+(.+)$`),
		FailPackage:  regexp.MustCompile(`^FAIL\s+(github\.com/\S+)\s+(.+)$`),
		Run:          regexp.MustCompile(`^=== RUN\s+(.+)$`),
		PassTest:     regexp.MustCompile(`^--- PASS:`),
		FailTest:     regexp.MustCompile(`^--- FAIL:`),
		SkipTest:     regexp.MustCompile(`^--- SKIP:`),
		SinglePass:   regexp.MustCompile(`^PASS$`),
		SingleFail:   regexp.MustCompile(`^FAIL$`),
		ErrorMessage: regexp.MustCompile(`^\s*(.+\.go:\d+:)(.+)$`),
	}
}

// setupColors configures all color formatters.
func setupColors() Colors {
	return Colors{
		Pass:      color.New(color.FgGreen, color.Bold).SprintFunc(),
		Fail:      color.New(color.FgRed, color.Bold).SprintFunc(),
		Skip:      color.New(color.FgYellow).SprintFunc(),
		Run:       color.New(color.FgCyan).SprintFunc(),
		Package:   color.New(color.FgBlue, color.Bold).SprintFunc(),
		Info:      color.New(color.FgWhite).SprintFunc(),
		Error:     color.New(color.FgRed).SprintFunc(),
		Header:    color.New(color.FgMagenta, color.Bold).SprintFunc(),
		Highlight: color.New(color.FgYellow).SprintFunc(),
	}
}

// Run executes the test runner.
func (tr *TestRunner) Run() int {
	// Print header
	tr.printHeader()

	// Process each line.
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		tr.processLine(scanner.Text())
	}

	// Print summary.
	tr.printSummary()

	// Return exit code.
	if tr.FailCount > 0 || tr.FailedPackages > 0 {
		fmt.Println(tr.Colors.Fail("✗ Tests failed"))
		return 1
	}

	fmt.Println(tr.Colors.Pass("✓ All tests passed"))
	return 0
}

// printHeader displays the application header.
func (tr *TestRunner) printHeader() {
	fmt.Println(tr.Colors.Header("┌─────────────────────────────────────────────────┐"))
	fmt.Println(tr.Colors.Header("│            CowGnition Test Runner               │"))
	fmt.Println(tr.Colors.Header("└─────────────────────────────────────────────────┘"))
}

// processLine handles a single line of test output.
func (tr *TestRunner) processLine(line string) {
	// Skip the initial "Running tests..." line
	if strings.Contains(line, "Running tests...") {
		return
	}

	switch {
	case tr.handleNoTestFiles(line):
		// Handled by function
	case tr.handlePackagePass(line):
		// Handled by function
	case tr.handlePackageFail(line):
		// Handled by function
	case tr.handleSinglePassFail(line):
		// Handled by function
	case tr.handleTestRun(line):
		// Handled by function
	case tr.handleTestPass(line):
		// Handled by function
	case tr.handleTestFail(line):
		// Handled by function
	case tr.handleTestSkip(line):
		// Handled by function
	case tr.handleErrorMessage(line):
		// Handled by function
	default:
		// Just print the line
		fmt.Println(line)
	}
}

// handleNoTestFiles processes a "no test files" line.
func (tr *TestRunner) handleNoTestFiles(line string) bool {
	match := tr.Patterns.NoTest.FindStringSubmatch(line)
	if len(match) > 1 {
		fmt.Printf("%s %s %s\n", tr.Colors.Skip("•"), tr.Colors.Package(match[1]), tr.Colors.Skip("[no test files]"))
		return true
	}
	return false
}

// handlePackagePass processes a package passing line.
func (tr *TestRunner) handlePackagePass(line string) bool {
	match := tr.Patterns.PassPackage.FindStringSubmatch(line)
	if len(match) > 1 {
		// Reset the seen PASS flag for the next package.
		tr.SeenPASS = false
		fmt.Printf("%s %s %s\n", tr.Colors.Pass("PASS"), tr.Colors.Package(match[1]), match[2])
		tr.PassedPackages++
		return true
	}
	return false
}

// handlePackageFail processes a package failing line.
func (tr *TestRunner) handlePackageFail(line string) bool {
	match := tr.Patterns.FailPackage.FindStringSubmatch(line)
	if len(match) > 1 {
		// Reset the seen FAIL flag for the next package.
		tr.SeenFAIL = false
		fmt.Printf("%s %s %s\n", tr.Colors.Fail("FAIL"), tr.Colors.Package(match[1]), match[2])
		tr.FailedPackages++
		return true
	}
	return false
}

// handleSinglePassFail processes standalone PASS/FAIL lines.
func (tr *TestRunner) handleSinglePassFail(line string) bool {
	if tr.Patterns.SinglePass.MatchString(line) {
		if !tr.SeenPASS {
			tr.SeenPASS = true
			// Don't print the standalone PASS
		}
		return true
	}

	if tr.Patterns.SingleFail.MatchString(line) {
		if !tr.SeenFAIL {
			tr.SeenFAIL = true
			// Don't print the standalone FAIL
		}
		return true
	}

	return false
}

// handleTestRun processes a test run line.
func (tr *TestRunner) handleTestRun(line string) bool {
	match := tr.Patterns.Run.FindStringSubmatch(line)
	if len(match) > 1 {
		fmt.Printf("▶ %s\n", match[1])
		return true
	}
	return false
}

// handleTestPass processes a test pass line.
func (tr *TestRunner) handleTestPass(line string) bool {
	if tr.Patterns.PassTest.MatchString(line) {
		fmt.Printf("%s %s\n", tr.Colors.Pass("✓"), line)
		tr.PassCount++
		return true
	}
	return false
}

// handleTestFail processes a test fail line.
func (tr *TestRunner) handleTestFail(line string) bool {
	if tr.Patterns.FailTest.MatchString(line) {
		fmt.Printf("%s %s\n", tr.Colors.Fail("✗"), line)
		tr.FailCount++
		return true
	}
	return false
}

// handleTestSkip processes a test skip line.
func (tr *TestRunner) handleTestSkip(line string) bool {
	if tr.Patterns.SkipTest.MatchString(line) {
		fmt.Printf("%s %s\n", tr.Colors.Skip("⚠"), line)
		tr.SkipCount++
		return true
	}
	return false
}

// handleErrorMessage processes an error message line.
func (tr *TestRunner) handleErrorMessage(line string) bool {
	match := tr.Patterns.ErrorMessage.FindStringSubmatch(line)
	if len(match) > 2 {
		fmt.Printf("    %s%s\n", tr.Colors.Highlight(match[1]), match[2])
		return true
	}
	return false
}

// printSummary displays the final test statistics.
func (tr *TestRunner) printSummary() {
	duration := time.Since(tr.StartTime)

	fmt.Println()
	fmt.Println(tr.Colors.Header("┌─────────────────────────────────────────────────┐"))
	fmt.Printf("%s │ Summary: %s %d passed, %s %d failed, %s %d skipped │\n",
		tr.Colors.Header(""),
		tr.Colors.Pass("✓"),
		tr.PassCount,
		tr.Colors.Fail("✗"),
		tr.FailCount,
		tr.Colors.Skip("⚠"),
		tr.SkipCount)

	// Construct package line.
	pkgLine := fmt.Sprintf(" │ Packages: %s %d passed, %s %d failed",
		tr.Colors.Pass("✓"),
		tr.PassedPackages,
		tr.Colors.Fail("✗"),
		tr.FailedPackages)

	// Pad to match the box width
	padWidth := 45 - len(stripANSI(pkgLine))
	if padWidth < 0 {
		padWidth = 0
	}
	pkgLine += strings.Repeat(" ", padWidth) + "│"

	fmt.Println(tr.Colors.Header(pkgLine))
	fmt.Printf("%s │ Duration: %-35s │\n",
		tr.Colors.Header(""),
		duration.Round(time.Millisecond))
	fmt.Println(tr.Colors.Header("└─────────────────────────────────────────────────┘"))
}

// stripANSI removes ANSI color codes for correct length calculations.
func stripANSI(str string) string {
	ansi := regexp.MustCompile("\x1b\\[[0-9;]*m")
	return ansi.ReplaceAllString(str, "")
}

func main() {
	runner := NewTestRunner()
	os.Exit(runner.Run())
}
