// Package main contains the script to fix comments in Go files.
package main

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// --- Configuration ---

// packageDescriptions maps package directory names to standard descriptions.
var packageDescriptions = map[string]string{ // Renamed to camelCase
	"config":     "handles loading, parsing, and validating application configuration.",
	"logging":    "provides a common interface and setup for application-wide logging.",
	"mcp":        "implements the Model Context Protocol server logic, including handlers and types.",
	"mcperrors":  "defines domain-specific error types for the MCP layer.",
	"middleware": "provides chainable handlers for processing MCP messages, like validation.",
	"rtm":        "implements the client and service logic for interacting with the Remember The Milk API.",
	"schema":     "handles loading, validation, and error reporting against JSON schemas, specifically MCP.",
	"transport":  "defines interfaces and implementations for sending and receiving MCP messages.",
	"server":     "contains the runner and setup logic for the main CowGnition MCP server process.",
	// Add more specific descriptions here if needed.
}

// defaultDescription provides a fallback comment part.
const defaultDescription = "provides functionality related to its domain." // Renamed to camelCase

// Regex patterns.
var (
	// Matches existing standard package comments.
	existingPkgCommentRegex = regexp.MustCompile(`(?m)^\s*//\s*Package\s+\w+.*\n`)
	// Matches the specific file path comment format used.
	existingFileCommentRegex = regexp.MustCompile(`(?m)^\s*//\s*file:\s*(.*)\s*\n`)
	// Removed unused packageDeclRegex.
)

// generatePackageComment generates a standard package comment line.
func generatePackageComment(pkgName string) string {
	// Correct Go map access
	description, ok := packageDescriptions[pkgName] // Use corrected map name
	if !ok {
		description = fmt.Sprintf("(%s) %s", pkgName, defaultDescription) // Use corrected const name
	}
	return fmt.Sprintf("// Package %s %s\n", pkgName, description)
}

// processGoFile reads a Go file, adds/updates comments, and writes back.
func processGoFile(filePath string, rootDir string) {
	log.Printf("Processing: %s\n", filePath)
	// #nosec G304 -- File path comes from filepath.WalkDir in the current project.
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("!!! Error reading %s: %v\n", filePath, err)
		return
	}
	content := string(contentBytes)

	// Use Go parser to reliably find package name and position
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, contentBytes, parser.PackageClauseOnly)
	if err != nil {
		log.Printf("--- Skipping %s: Failed to parse package clause: %v\n", filePath, err)
		return
	}

	packageName := node.Name.Name
	if packageName == "main" {
		log.Printf("--- Skipping %s: 'package main'.\n", filePath)
		return
	}

	// Find the line number where the package declaration ends
	packageLineEndOffset := fset.Position(node.Name.End()).Offset
	packageLineIndex := -1
	currentOffset := 0
	lines := strings.SplitAfter(content, "\n") // Split keeping newline chars
	for i, line := range lines {
		currentOffset += len(line)
		if currentOffset > packageLineEndOffset {
			packageLineIndex = i
			break
		}
	}

	if packageLineIndex == -1 {
		log.Printf("--- Skipping %s: Could not find package declaration line index.\n", filePath)
		return
	}

	// Generate desired comments
	newPackageComment := generatePackageComment(packageName)
	// Use ToSlash for consistent path separators
	relativePath := filepath.ToSlash(strings.TrimPrefix(filePath, rootDir+string(filepath.Separator)))
	newFileComment := fmt.Sprintf("// file: %s\n", relativePath)

	// Build the new file content buffer
	var newContent bytes.Buffer

	// Add the standard package comment first
	newContent.WriteString(newPackageComment)

	packageLineWritten := false // Keep track if we wrote the package line

	for i, line := range lines {
		// Skip existing comments only if they appear before the determined package line
		if i < packageLineIndex {
			if existingPkgCommentRegex.MatchString(line) || existingFileCommentRegex.MatchString(line) {
				continue // Skip old comments in the header area
			}
		}

		// Write the line if it's not one we're replacing/skipping
		if i == packageLineIndex {
			// Write the actual package declaration line
			newContent.WriteString(line)
			packageLineWritten = true
			// Immediately write the file comment after the package line
			newContent.WriteString(newFileComment)
			// Apply De Morgan's Law for simplification
		} else if !packageLineWritten || !existingFileCommentRegex.MatchString(line) {
			// Write other lines, but skip file comment if it came *after* package line originally AND we already wrote the new one
			newContent.WriteString(line)
		}
	}

	// --- Safety Check ---
	newBytes := newContent.Bytes()
	if bytes.Equal(bytes.TrimSpace(newBytes), bytes.TrimSpace(contentBytes)) {
		log.Printf("--- Skipping %s: No effective changes detected.\n", filePath)
		return
	}

	// --- Write Back ---
	// !! WARNING: Overwrites original file. Backup or use Git!
	err = os.WriteFile(filePath, newBytes, 0600) // Use secure permissions (0600)
	if err != nil {
		log.Printf("!!! Error writing %s: %v\n", filePath, err)
	} else {
		log.Printf("+++ Updated %s\n", filePath)
	}
}

func main() {
	projectRoot := "." // Assumes script is run from the cowgnition root directory
	log.Printf("Scanning for Go files in: %s\n", projectRoot)

	var goFilesToProcess []string
	err := filepath.WalkDir(projectRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip vendor and .git directories
		if d.IsDir() && (d.Name() == "vendor" || d.Name() == ".git") {
			return filepath.SkipDir
		}
		// Determine script's potential directory relative to project root
		selfDir := filepath.Join("cmd", "fix_comments")
		// Skip the script's own directory to avoid self-modification
		if strings.HasPrefix(path, selfDir) {
			// Check if it's the directory itself or a file within
			if (d.IsDir() && path == selfDir) || (!d.IsDir() && strings.HasPrefix(path, selfDir+string(filepath.Separator))) {
				// Decide whether to skip the dir or just files within; skipping dir is cleaner
				if d.IsDir() && path == selfDir {
					return filepath.SkipDir
				}
				// If logic needed refinement: skip files but not necessarily the dir walk if other subdirs existed
				// return nil // effectively skips the file if !d.IsDir() matched
			}
		}

		if !d.IsDir() && strings.HasSuffix(d.Name(), ".go") {
			// No need for AbsPath if we handle relative path correctly later
			goFilesToProcess = append(goFilesToProcess, path) // Store relative path
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Error walking directory: %v", err)
	}

	log.Printf("Found %d Go files to process.\n", len(goFilesToProcess))

	absRoot, _ := filepath.Abs(projectRoot) // Still useful for TrimPrefix

	// Safety: Confirmation commented out for automation. Uncomment for manual use.
	// fmt.Print("This script will modify Go files in place. Type 'yes' to continue: ")
	// var confirm string
	// fmt.Scanln(&confirm)
	// if strings.ToLower(confirm) != "yes" {
	// 	fmt.Println("Aborted.")
	// 	return
	// }
	// fmt.Println("Proceeding with modification...")

	log.Println("Processing files...")
	for _, goFile := range goFilesToProcess {
		// Pass the potentially relative path directly
		processGoFile(goFile, absRoot)
	}

	log.Println("Script finished.")
}
