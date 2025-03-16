// cmd/testrunner/main.go
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/fatih/color"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)

	// Compile regular expressions for different test output patterns
	passedPattern := regexp.MustCompile(`^PASS$|^--- PASS:`)
	failedPattern := regexp.MustCompile(`^FAIL$|^--- FAIL:`)
	skippedPattern := regexp.MustCompile(`^--- SKIP:`)
	runPattern := regexp.MustCompile(`^=== RUN\s+(.+)$`)
	summaryPattern := regexp.MustCompile(`^(ok|FAIL)\s+(\S+)\s+(.+)$`)
	errorPattern := regexp.MustCompile(`^\s*(.+\.go:\d+:|Error:)(.+)$`)

	// Set up colors
	pass := color.New(color.FgGreen, color.Bold).SprintFunc()
	fail := color.New(color.FgRed, color.Bold).SprintFunc()
	skip := color.New(color.FgYellow, color.Bold).SprintFunc()
	run := color.New(color.FgCyan).SprintFunc()
	pkg := color.New(color.FgBlue, color.Bold).SprintFunc()
	errText := color.New(color.FgRed).SprintFunc()

	// Process each line
	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case passedPattern.MatchString(line):
			fmt.Println(pass("✓ " + line))

		case failedPattern.MatchString(line):
			fmt.Println(fail("✗ " + line))

		case skippedPattern.MatchString(line):
			fmt.Println(skip("⚠ " + line))

		case runPattern.MatchString(line):
			matches := runPattern.FindStringSubmatch(line)
			if len(matches) > 1 {
				fmt.Println(run("▶ Running " + matches[1]))
			} else {
				fmt.Println(line)
			}

		case summaryPattern.MatchString(line):
			matches := summaryPattern.FindStringSubmatch(line)
			if len(matches) > 3 {
				status := matches[1]
				package_ := matches[2]
				timing := matches[3]

				if status == "ok" {
					fmt.Printf("%s %s %s\n", pass("PASS"), pkg(package_), timing)
				} else {
					fmt.Printf("%s %s %s\n", fail("FAIL"), pkg(package_), timing)
				}
			} else {
				fmt.Println(line)
			}

		case errorPattern.MatchString(line):
			matches := errorPattern.FindStringSubmatch(line)
			if len(matches) > 2 {
				fmt.Printf("%s %s\n", matches[1], errText(matches[2]))
			} else {
				fmt.Println(line)
			}

		default:
			// Check if line contains "PASS" or "FAIL" words
			if strings.Contains(line, "PASS") {
				line = strings.Replace(line, "PASS", pass("PASS"), -1)
			}
			if strings.Contains(line, "FAIL") {
				line = strings.Replace(line, "FAIL", fail("FAIL"), -1)
			}
			fmt.Println(line)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}
}
