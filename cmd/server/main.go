// Package main implements the CowGnition CLI application.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Version information (populated at build time).
var (
	version    = "dev"
	commitHash = "unknown"
	buildDate  = "unknown"
)

// main is the entry point for the CowGnition CLI application.
// It parses command line arguments and dispatches to the appropriate command.
func main() {
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("[cowgnition] ")

	// Print basic info on startup
	printStartupInfo()

	// Get available commands
	commands := RegisterCommands()

	// If no arguments, show help
	if len(os.Args) < 2 {
		err := commands["help"].Run(string{})
		if err != nil {
			log.Fatalf("main: error running help command: %v", err)
		}
		return
	}

	// Get command name
	cmdName := os.Args[1]

	// Special case for version flag
	if cmdName == "-v" || cmdName == "--version" {
		printVersion()
		return
	}

	// Look up command
	cmd, ok := commands[cmdName]
	if !ok {
		fmt.Printf("Unknown command: %s\n\n", cmdName)
		err := commands["help"].Run(string{})
		if err != nil {
			log.Fatalf("main: error running help command: %v", err)
		}
		os.Exit(1)
	}

	// Run command with arguments
	err := cmd.Run(os.Args[2:])
	if err != nil {
		log.Fatalf("main: error running command: %v", err)
	}
}

// printStartupInfo prints basic information about the application on startup.
func printStartupInfo() {
	execPath, err := os.Executable()
	if err != nil {
		log.Printf("Starting CowGnition from: %s", execPath)
	}
	log.Printf("CowGnition version %s (build: %s)", version, buildDate)
	log.Printf("Running on %s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

// printVersion prints detailed version information.
func printVersion() {
	fmt.Printf("CowGnition - Remember The Milk MCP Server\n")
	fmt.Printf("Version:    %s\n", version)
	fmt.Printf("Commit:     %s\n", commitHash)
	fmt.Printf("Built:      %s\n", buildDate)
	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

// findConfigFile searches for the config file in standard locations if not specified.
func findConfigFile(specifiedPath string) string {
	// If a path is specified and exists, use it
	if specifiedPath != "" {
		_, err := os.Stat(specifiedPath)
		if err == nil {
			return specifiedPath
		}
		if !strings.Contains(specifiedPath, "/") && !strings.Contains(specifiedPath, "\\") {
			// Try in the configs directory if just a filename was provided
			configsPath := filepath.Join("configs", specifiedPath)
			_, err := os.Stat(configsPath)
			if err == nil {
				return configsPath
			}
		}
	}

	// Standard locations to check
	standardPaths := string{
		"config.yaml",
		"configs/config.yaml",
		filepath.Join(os.Getenv("HOME"), ".config", "cowgnition", "config.yaml"),
		"/etc/cowgnition/config.yaml",
	}

	for _, path := range standardPaths {
		_, err := os.Stat(path)
		if err == nil {
			return path
		}
	}

	// If specified path doesn't exist, return it anyway so we can give a proper error
	if specifiedPath != "" {
		return specifiedPath
	}

	// Default to configs/config.yaml even if it doesn't exist
	return "configs/config.yaml"
}

// ErrorMsgEnhanced:2025-03-18
