package main

import (
	"flag"
	"fmt"
	"os"
)

// Command represents a CLI command
type Command struct {
	Name        string
	Description string
	Run         func(args []string) error
}

// RegisterCommands registers CLI commands
func RegisterCommands() map[string]Command {
	return map[string]Command{
		"serve": {
			Name:        "serve",
			Description: "Start the MCP server",
			Run:         serveCommand,
		},
		"version": {
			Name:        "version",
			Description: "Show version information",
			Run:         versionCommand,
		},
		"help": {
			Name:        "help",
			Description: "Show help for commands",
			Run:         helpComman