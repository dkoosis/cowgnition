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

// TestStats tracks test statistics.
type TestStats struct {
	PassCount      int
	FailCount      int
	SkipCount      int
	TotalCount     int
	PackageResults map[string]string // Track package results.
}

// ColoredPrinters contains formatting functions.
type ColoredPrinters struct {
	Pass      func(a ...interface{}) string
	Fail      func(a ...interface{}) string
	Skip      func(a ...interface{}) string
	Run       func(a ...interface{}) string
	Pkg       func(a ...interface{}) string
	Info      func(a ...interface{}) string
	ErrText   func(a ...interface{}) string
	Header    func(a ...interface{}) string
	Highlight func(a ...interface{}) string
	Separator func(a ...interface{}) string
}

// Patterns contains compiled regex patterns.
type Patterns struct {
	Passed  *regexp.Regexp
	Failed  *regexp.Regexp
	Skipped *regexp.Regexp
	Run     *regexp.Regexp
	Summary *regexp.Regexp
	Error   *regexp.Regexp
	Package *regexp.Regexp
	NoTest  *regexp.Regexp
}

func main() {
	startTime := time.Now()
	scanner := bufio.NewScanner(os.Stdin)

	patterns := compilePatterns()
	printers := setupColorPrinters()
	stats := TestStats{
		PackageResults: make(map[string]string),
	}

	printHeader(printers)

	// Process each line.
	currentPackage := ""
	for scanner.Scan() {
		line := scanner.Text()
		packageName := extractPackageName(line, patterns)
		if packageName != "" {
			currentPackage = packageName
			fmt.Println(printers.Separator("┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄"))
			fmt.Printf("%s %s\n", printers.Pkg("PACKAGE"), printers.Highlight(packageName))
		}

		processLine(line, patterns, printers, &stats, currentPackage)
	}

	// Print summary statistics.
	printSummary(startTime, printers, stats)

	if stats.FailCount > 0 {
		os.Exit(1)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}
}

// extractPackageName gets the package name from test output lines.
func extractPackageName(line string, patterns Patterns) string {
	// Extract package name from PASS/FAIL lines or "ok" summary lines.
	if match := patterns.Summary.FindStringSubmatch(line); len(match) > 2 {
		return match[2]
	}

	// Check for no test files line.
	if match := patterns.NoTest.FindStringSubmatch(line); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}

	return ""
}

// compilePatterns creates all regex patterns used for parsing test output.
func compilePatterns() Patterns {
	return Patterns{
		Passed:  regexp.MustCompile(`^PASS$|^--- PASS:`),
		Failed:  regexp.MustCompile(`^FAIL$|^--- FAIL:`),
		Skipped: regexp.MustCompile(`^--- SKIP:`),
		Run:     regexp.MustCompile(`^=== RUN\s+(.+)$`),
		Summary: regexp.MustCompile(`^(ok|FAIL)\s+(\S+)\s+(.+)$`),
		Error:   regexp.MustCompile(`^\s*(.+\.go:\d+:|Error:)(.+)$`),
		Package: regexp.MustCompile(`^PASS|^FAIL\s+(\S+)`),
		NoTest:  regexp.MustCompile(`^\?\s+([^\[]+)\s+\[no test files\]$`),
	}
}

// setupColorPrinters configures all color formatters for output.
func setupColorPrinters() ColoredPrinters {
	return ColoredPrinters{
		Pass:      color.New(color.FgGreen, color.Bold).SprintFunc(),
		Fail:      color.New(color.FgRed, color.Bold).SprintFunc(),
		Skip:      color.New(color.FgYellow, color.Bold).SprintFunc(),
		Run:       color.New(color.FgCyan).SprintFunc(),
		Pkg:       color.New(color.FgBlue, color.Bold).SprintFunc(),
		Info:      color.New(color.FgWhite).SprintFunc(),
		ErrText:   color.New(color.FgRed).SprintFunc(),
		Header:    color.New(color.FgMagenta, color.Bold).SprintFunc(),
		Highlight: color.New(color.FgHiWhite, color.Bold).SprintFunc(),
		Separator: color.New(color.FgBlue).SprintFunc(),
	}
}

// printHeader displays the application header.
func printHeader(p ColoredPrinters) {
	fmt.Println(p.Header("┌─────────────────────────────────────────────────────────────────┐"))
	fmt.Println(p.Header("│                  CowGnition Test Runner                         │"))
	fmt.Println(p.Header("└─────────────────────────────────────────────────────────────────┘"))
}

// processLine handles a single line of test output.
func processLine(line string, patterns Patterns, p ColoredPrinters, stats *TestStats, currentPackage string) {
	switch {
	case patterns.Passed.MatchString(line):
		handlePassedLine(line, p, stats, currentPackage)

	case patterns.Failed.MatchString(line):
		handleFailedLine(line, p, stats, currentPackage)

	case patterns.Skipped.MatchString(line):
		handleSkippedLine(line, p, stats)

	case patterns.Run.MatchString(line):
		handleRunLine(line, patterns.Run, p)

	case patterns.NoTest.MatchString(line):
		handleNoTestLine(line, p)

	case patterns.Summary.MatchString(line):
		handleSummaryLine(line, patterns.Summary, p, stats)

	case patterns.Error.MatchString(line):
		handleErrorLine(line, patterns.Error, p)

	default:
		handleDefaultLine(line, p)
	}
}

// handlePassedLine processes a successful test line.
func handlePassedLine(line string, p ColoredPrinters, stats *TestStats, currentPackage string) {
	fmt.Println(p.Pass("✓ " + line))
	if strings.HasPrefix(line, "--- PASS:") {
		stats.PassCount++
		stats.TotalCount++
		if currentPackage != "" {
			stats.PackageResults[currentPackage] = "pass"
		}
	}
}

// handleFailedLine processes a failed test line.
func handleFailedLine(line string, p ColoredPrinters, stats *TestStats, currentPackage string) {
	fmt.Println(p.Fail("✗ " + line))
	if strings.HasPrefix(line, "--- FAIL:") {
		stats.FailCount++
		stats.TotalCount++
		if currentPackage != "" {
			stats.PackageResults[currentPackage] = "fail"
		}
	}
}

// handleSkippedLine processes a skipped test line.
func handleSkippedLine(line string, p ColoredPrinters, stats *TestStats) {
	fmt.Println(p.Skip("⚠ " + line))
	stats.SkipCount++
	stats.TotalCount++
}

// handleRunLine processes a test run start line.
func handleRunLine(line string, pattern *regexp.Regexp, p ColoredPrinters) {
	matches := pattern.FindStringSubmatch(line)
	if len(matches) > 1 {
		fmt.Println(p.Run("▶ Running " + matches[1]))
	} else {
		fmt.Println(line)
	}
}

// handleNoTestLine processes a "no test files" line.
func handleNoTestLine(line string, p ColoredPrinters) {
	matches := regexp.MustCompile(`^\?\s+([^\[]+)\s+\[no test files\]$`).FindStringSubmatch(line)
	if len(matches) > 1 {
		packageName := strings.TrimSpace(matches[1])
		fmt.Printf("%s %s %s\n", p.Info("•"), p.Pkg(packageName), p.Skip("[no test files]"))
	} else {
		fmt.Println(line)
	}
}

// handleSummaryLine processes package summary lines.
func handleSummaryLine(line string, pattern *regexp.Regexp, p ColoredPrinters, stats *TestStats) {
	matches := pattern.FindStringSubmatch(line)
	if len(matches) > 3 {
		status := matches[1]
		package_ := matches[2]
		timing := matches[3]

		if status == "ok" {
			fmt.Printf("%s %s %s\n", p.Pass("PASS"), p.Pkg(package_), timing)
			stats.PackageResults[package_] = "pass"
		} else {
			fmt.Printf("%s %s %s\n", p.Fail("FAIL"), p.Pkg(package_), timing)
			stats.PackageResults[package_] = "fail"
		}
	} else {
		fmt.Println(line)
	}
}

// handleErrorLine processes error lines with file references.
func handleErrorLine(line string, pattern *regexp.Regexp, p ColoredPrinters) {
	matches := pattern.FindStringSubmatch(line)
	if len(matches) > 2 {
		fmt.Printf("   %s %s\n", p.Highlight(matches[1]), p.ErrText(matches[2]))
	} else {
		fmt.Println(line)
	}
}

// handleDefaultLine processes any other test output line.
func handleDefaultLine(line string, p ColoredPrinters) {
	line = strings.Replace(line, "PASS", p.Pass("PASS"), -1)
	line = strings.Replace(line, "FAIL", p.Fail("FAIL"), -1)
	fmt.Println(line)
}

// printSummary displays the final test results and statistics.
func printSummary(startTime time.Time, p ColoredPrinters, stats TestStats) {
	duration := time.Since(startTime)

	fmt.Println("\n" + p.Separator("┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄"))

	// Print package results.
	fmt.Println(p.Header("Package Results:"))

	passedPackages := 0
	failedPackages := 0
	for pkg, result := range stats.PackageResults {
		if result == "pass" {
			fmt.Printf("  %s %s\n", p.Pass("✓"), p.Pkg(pkg))
			passedPackages++
		} else {
			fmt.Printf("  %s %s\n", p.Fail("✗"), p.Pkg(pkg))
			failedPackages++
		}
	}

	// Main summary box.
	fmt.Println(p.Header("\n┌─────────────────────────────────────────────────────────────────┐"))
	fmt.Printf("%s │ Summary: %s %d passed, %s %d failed, %s %d skipped %s    │\n",
		p.Header(""),
		p.Pass("✓"),
		stats.PassCount,
		p.Fail("✗"),
		stats.FailCount,
		p.Skip("⚠"),
		stats.SkipCount,
		p.Header(""))
	fmt.Printf("%s │ Packages: %s %d passed, %s %d failed %s                  │\n",
		p.Header(""),
		p.Pass("✓"),
		passedPackages,
		p.Fail("✗"),
		failedPackages,
		p.Header(""))
	fmt.Printf("%s │ Total tests: %-47d │\n", p.Header(""), stats.TotalCount)
	fmt.Printf("%s │ Duration: %-50s │\n", p.Header(""), duration.Round(time.Millisecond))
	fmt.Println(p.Header("└─────────────────────────────────────────────────────────────────┘"))
}
