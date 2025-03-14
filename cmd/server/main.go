// Package main implements the CowGnition CLI application.
package main

import (
	"fmt"
	"log"
	"os"
)

// Version information (populated at build time)
var (
	version = "dev"
)

// main is the entry point for the CowGnition CLI application.
// It parses command line arguments and dispatches to the appropriate command.
func main() {
	// Get available commands
	commands := RegisterCommands()

	// If no arguments, show help
	if len(os.Args) < 2 {
		if err := commands["help"].Run([]string{}); err != nil {
			log.Fatalf("Error: %v", err)
		}
		return
	}

	// Get command name
	cmdName := os.Args[1]

	// Look up command
	cmd, ok := commands[cmdName]
	if !ok {
		fmt.Printf("Unknown command: %s\n\n", cmdName)
		if err := commands["help"].Run([]string{}); err != nil {
			log.Fatalf("Error: %v", err)
		}
		os.Exit(1)
	}

	// Run command with arguments
	if err := cmd.Run(os.Args[2:]); err != nil {
		log.Fatalf("Error: %v", err)
	}
}
