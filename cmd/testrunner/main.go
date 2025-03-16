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

func main() {
	startTime := time.Now()

	// Setup colors
	passColor := color.New(color.FgGreen, color.Bold).SprintFunc()
	failColor := color.New(color.FgRed, color.Bold).SprintFunc()
	skipColor := color.New(color.FgYellow).SprintFunc()
	pkgColor := color.New(color.FgBlue, color.Bold).SprintFunc()
	headerColor := color.New(color.FgMagenta, color.Bold).SprintFunc()

	// Patterns for packages
	noTestPattern := regexp.MustCompile(`^\?\s+(github\.com/\S+)\s+\[no test files\]$`)
	passPattern := regexp.MustCompile(`^ok\s+(github\.com/\S+)\s+`)
	failPattern := regexp.MustCompile(`^FAIL\s+(github\.com/\S+)\s+`)

	// Test patterns
	runPattern := regexp.MustCompile(`^=== RUN\s+(.+)$`)
	passTestPattern := regexp.MustCompile(`^--- PASS:`)
	failTestPattern := regexp.MustCompile(`^--- FAIL:`)
	skipTestPattern := regexp.MustCompile(`^--- SKIP:`)

	scanner := bufio.NewScanner(os.Stdin)

	passCount := 0
	failCount := 0
	skipCount := 0
	passedPackages := 0
	failedPackages := 0

	// Print header
	fmt.Println(headerColor("┌─────────────────────────────────────────────────┐"))
	fmt.Println(headerColor("│            CowGnition Test Runner               │"))
	fmt.Println(headerColor("└─────────────────────────────────────────────────┘"))

	// Process lines
	for scanner.Scan() {
		line := scanner.Text()

		// Handle package results
		if match := noTestPattern.FindStringSubmatch(line); len(match) > 1 {
			fmt.Printf("%s %s %s\n", skipColor("•"), pkgColor(match[1]), skipColor("[no test files]"))
			continue
		}

		if match := passPattern.FindStringSubmatch(line); len(match) > 1 {
			fmt.Printf("%s %s %s\n", passColor("PASS"), pkgColor(match[1]), strings.TrimPrefix(line, "ok "+match[1]))
			passedPackages++
			continue
		}

		if match := failPattern.FindStringSubmatch(line); len(match) > 1 {
			fmt.Printf("%s %s %s\n", failColor("FAIL"), pkgColor(match[1]), strings.TrimPrefix(line, "FAIL "+match[1]))
			failedPackages++
			continue
		}

		// Handle test results
		if runPattern.MatchString(line) {
			match := runPattern.FindStringSubmatch(line)
			fmt.Printf("▶ %s\n", match[1])
			continue
		}

		if passTestPattern.MatchString(line) {
			fmt.Printf("%s %s\n", passColor("✓"), line)
			passCount++
			continue
		}

		if failTestPattern.MatchString(line) {
			fmt.Printf("%s %s\n", failColor("✗"), line)
			failCount++
			continue
		}

		if skipTestPattern.MatchString(line) {
			fmt.Printf("%s %s\n", skipColor("⚠"), line)
			skipCount++
			continue
		}

		// Default handling
		fmt.Println(line)
	}

	// Print summary
	duration := time.Since(startTime)

	fmt.Println(headerColor("\n┌─────────────────────────────────────────────────┐"))
	fmt.Printf("%s │ Summary: %s %d passed, %s %d failed, %s %d skipped │\n",
		headerColor(""),
		passColor("✓"),
		passCount,
		failColor("✗"),
		failCount,
		skipColor("⚠"),
		skipCount)
	fmt.Printf("%s │ Packages: %s %d passed, %s %d failed          │\n",
		headerColor(""),
		passColor("✓"),
		passedPackages,
		failColor("✗"),
		failedPackages)
	fmt.Printf("%s │ Duration: %-35s │\n",
		headerColor(""),
		duration.Round(time.Millisecond))
	fmt.Println(headerColor("└─────────────────────────────────────────────────┘"))

	if failCount > 0 || failedPackages > 0 {
		fmt.Println(failColor("✗ Tests failed"))
		os.Exit(1)
	} else {
		fmt.Println(passColor("✓ All tests passed"))
	}
}
