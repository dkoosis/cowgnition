// Package main implements the CowGnition CLI application.
// file: cmd/server/main.go
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
// These variables store version, commit hash, and build date information,
// which are typically populated during the build process.
var (
	version    = "dev"
	commitHash = "unknown"
	buildDate  = "unknown"
)

// main is the entry point for the CowGnition CLI application.
// It parses command line arguments and dispatches to the appropriate command.
// This function serves as the main entry point for the CowGnition CLI.
// It handles argument parsing, command dispatch, and error handling.
func main() {
	// Set up logging
	// It configures the logging settings for the application,
	// including flags and prefix, to ensure consistent and informative logging.
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("[cowgnition] ")

	// Print basic info on startup
	// It calls the printStartupInfo function to log basic information
	// about the application's execution environment.
	printStartupInfo()

	// Get available commands
	// It retrieves the registered commands by calling the RegisterCommands function.
	commands := RegisterCommands()

	// If no arguments, show help
	// If no command-line arguments are provided (other than the program name),
	// it displays the help message by calling the "help" command's Run function.
	if len(os.Args) < 2 {
		err := commands["help"].Run(nil)
		if err != nil {
			log.Fatalf("main: error running help command: %v", err)
		}
		return
	}

	// Get command name
	// It extracts the command name from the command-line arguments.
	cmdName := os.Args[1]

	// Special case for version flag
	// It handles the special case where the user provides "-v" or "--version"
	// as a command-line argument, in which case it calls the printVersion function
	// to display version information.
	if cmdName == "-v" || cmdName == "--version" {
		printVersion()
		return
	}

	// Look up command
	// It retrieves the Command structure corresponding to the provided
	// command name from the commands map. If the command is not found,
	// it prints an error message and displays the help message.
	cmd, ok := commands[cmdName]
	if !ok {
		fmt.Printf("Unknown command: %s\n\n", cmdName)
		err := commands["help"].Run(nil)
		if err != nil {
			log.Fatalf("main: error running help command: %v", err)
		}
		os.Exit(1)
	}

	// Run command with arguments
	// It executes the specified command by calling its Run function,
	// passing the command-line arguments (excluding the program name
	// and command name). If an error occurs during command execution,
	// it logs the error and exits.
	err := cmd.Run(os.Args[2:])
	if err != nil {
		log.Fatalf("main: error running command: %v", err)
	}
}

// printStartupInfo prints basic information about the application on startup.
// This function prints information about the application's execution environment,
// such as the executable path, version, build date, Go version, OS, and architecture.
// This information is helpful for debugging and identifying the environment in which the application is running.
func printStartupInfo() {
	execPath, err := os.Executable()
	if err != nil {
		log.Printf("Starting CowGnition from: %s", execPath)
	}
	log.Printf("CowGnition version %s (build: %s)", version, buildDate)
	log.Printf("Running on %s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

// printVersion prints detailed version information.
// This function prints detailed version information about the application,
// including the version, commit hash, build date, Go version, OS, and architecture.
// This information is useful for users to identify the specific version of the
// application they are using.
func printVersion() {
	fmt.Printf("CowGnition - Remember The Milk MCP Server\n")
	fmt.Printf("Version:    %s\n", version)
	fmt.Printf("Commit:     %s\n", commitHash)
	fmt.Printf("Built:      %s\n", buildDate)
	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

// findConfigFile searches for the config file in standard locations if not specified.
// This function searches for the configuration file in standard locations
// if the user has not specified a path. It checks for the file in the current
// directory, the "configs" subdirectory, the user's home directory, and
// the /etc directory. This provides flexibility in how the configuration
// file is located.
func findConfigFile(specifiedPath string) string {
	// If a path is specified and exists, use it
	// If the user provides a specific path to the configuration file
	// and the file exists, it uses that path.
	if specifiedPath != "" {
		_, err := os.Stat(specifiedPath)
		if err == nil {
			return specifiedPath
		}
		if !strings.Contains(specifiedPath, "/") && !strings.Contains(specifiedPath, "\\") {
			// Try in the configs directory if just a filename was provided
			// If the user provides only a filename (without any path separators),
			// it attempts to locate the file within the "configs" subdirectory.
			configsPath := filepath.Join("configs", specifiedPath)
			_, err := os.Stat(configsPath)
			if err == nil {
				return configsPath
			}
		}
	}

	// Standard locations to check
	// It defines a list of standard locations where the configuration file
	// might be found.
	standardPaths := string{
		"config.yaml",
		"configs/config.yaml",
		filepath.Join(os.Getenv("HOME"), ".config", "cowgnition", "config.yaml"),
		"/etc/cowgnition/config.yaml",
	}

	// It iterates through the standard paths, checking if a file exists at each location.
	for _, path := range standardPaths {
		_, err := os.Stat(path)
		if err == nil {
			return path
		}
	}

	// If specified path doesn't exist, return it anyway so we can give a proper error
	// If the user provides a specific path but the file doesn't exist,
	// it returns the specified path anyway. This allows the calling function
	// to handle the error appropriately.
	if specifiedPath != "" {
		return specifiedPath
	}

	// Default to configs/config.yaml even if it doesn't exist
	// If no configuration file is found in any of the standard locations,
	// it defaults to "configs/config.yaml", even if that file doesn't exist.
	return "configs/config.yaml"
}

// DocEnhanced: 2025-03-25
