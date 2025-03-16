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
	PassCount  int
	FailCount  int
	SkipCount  int
	TotalCount int
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
}

func main() {
	startTime := time.Now()
	scanner := bufio.NewScanner(os.Stdin)

	patterns := compilePatterns()
	printers := setupColorPrinters()
	stats := TestStats{}

	printHeader(printers)

	// Process each line
	for scanner.Scan() {
		line := scanner.Text()
		processLine(line, patterns, printers, &stats)
	}

	// Print summary statistics
	printSummary(startTime, printers, stats)

	if stats.FailCount > 0 {
		os.Exit(1)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}
}

func compilePatterns() Patterns {
	return Patterns{
		Passed:  regexp.MustCompile(`^PASS$|^--- PASS:`),
		Failed:  regexp.MustCompile(`^FAIL$|^--- FAIL:`),
		Skipped: regexp.MustCompile(`^--- SKIP:`),
		Run:     regexp.MustCompile(`^=== RUN\s+(.+)$`),
		Summary: regexp.MustCompile(`^(ok|FAIL)\s+(\S+)\s+(.+)$`),
		Error:   regexp.MustCompile(`^\s*(.+\.go:\d+:|Error:)(.+)$`),
		Package: regexp.MustCompile(`^\?.*\[no test files\]$`),
	}
}

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
	}
}

func printHeader(p ColoredPrinters) {
	fmt.Println(p.Header("┌─────────────────────────────────────────────────────────────────┐"))
	fmt.Println(p.Header("│                  CowGnition Test Runner                         │"))
	fmt.Println(p.Header("└─────────────────────────────────────────────────────────────────┘"))
}

func processLine(line string, patterns Patterns, p ColoredPrinters, stats *TestStats) {
	switch {
	case patterns.Passed.MatchString(line):
		handlePassedLine(line, p, stats)

	case patterns.Failed.MatchString(line):
		handleFailedLine(line, p, stats)

	case patterns.Skipped.MatchString(line):
		handleSkippedLine(line, p, stats)

	case patterns.Run.MatchString(line):
		handleRunLine(line, patterns.Run, p)

	case patterns.Package.MatchString(line):
		handlePackageLine(line, p)

	case patterns.Summary.MatchString(line):
		handleSummaryLine(line, patterns.Summary, p)

	case patterns.Error.MatchString(line):
		handleErrorLine(line, patterns.Error, p)

	default:
		handleDefaultLine(line, p)
	}
}

func handlePassedLine(line string, p ColoredPrinters, stats *TestStats) {
	fmt.Println(p.Pass("✓ " + line))
	if strings.HasPrefix(line, "--- PASS:") {
		stats.PassCount++
		stats.TotalCount++
	}
}

func handleFailedLine(line string, p ColoredPrinters, stats *TestStats) {
	fmt.Println(p.Fail("✗ " + line))
	if strings.HasPrefix(line, "--- FAIL:") {
		stats.FailCount++
		stats.TotalCount++
	}
}

func handleSkippedLine(line string, p ColoredPrinters, stats *TestStats) {
	fmt.Println(p.Skip("⚠ " + line))
	stats.SkipCount++
	stats.TotalCount++
}

func handleRunLine(line string, pattern *regexp.Regexp, p ColoredPrinters) {
	matches := pattern.FindStringSubmatch(line)
	if len(matches) > 1 {
		fmt.Println(p.Run("▶ Running " + matches[1]))
	} else {
		fmt.Println(line)
	}
}

func handlePackageLine(line string, p ColoredPrinters) {
	packageName := strings.Split(line, "[")[0]
	fmt.Println(p.Info("   "+packageName) + p.Skip("[no test files]"))
}

func handleSummaryLine(line string, pattern *regexp.Regexp, p ColoredPrinters) {
	matches := pattern.FindStringSubmatch(line)
	if len(matches) > 3 {
		status := matches[1]
		package_ := matches[2]
		timing := matches[3]

		if status == "ok" {
			fmt.Printf("%s %s %s\n", p.Pass("PASS"), p.Pkg(package_), timing)
		} else {
			fmt.Printf("%s %s %s\n", p.Fail("FAIL"), p.Pkg(package_), timing)
		}
	} else {
		fmt.Println(line)
	}
}

func handleErrorLine(line string, pattern *regexp.Regexp, p ColoredPrinters) {
	matches := pattern.FindStringSubmatch(line)
	if len(matches) > 2 {
		fmt.Printf("   %s %s\n", p.Highlight(matches[1]), p.ErrText(matches[2]))
	} else {
		fmt.Println(line)
	}
}

func handleDefaultLine(line string, p ColoredPrinters) {
	line = strings.Replace(line, "PASS", p.Pass("PASS"), -1)
	line = strings.Replace(line, "FAIL", p.Fail("FAIL"), -1)
	fmt.Println(line)
}

func printSummary(startTime time.Time, p ColoredPrinters, stats TestStats) {
	duration := time.Since(startTime)
	fmt.Println(p.Header("┌─────────────────────────────────────────────────────────────────┐"))
	fmt.Printf("%s │ Summary: %s %d passed, %s %d failed, %s %d skipped %s    │\n",
		p.Header(""),
		p.Pass("✓"),
		stats.PassCount,
		p.Fail("✗"),
		stats.FailCount,
		p.Skip("⚠"),
		stats.SkipCount,
		p.Header(""))
	fmt.Printf("%s │ Total tests: %-47d │\n", p.Header(""), stats.TotalCount)
	fmt.Printf("%s │ Duration: %-50s │\n", p.Header(""), duration.Round(time.Millisecond))
	fmt.Println(p.Header("└─────────────────────────────────────────────────────────────────┘"))
}
