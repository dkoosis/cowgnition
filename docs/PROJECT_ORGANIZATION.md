# Project Organization 📂

This document provides a friendly tour of how CowGnition is organized and architected.

## Directory Structure

CowGnition follows the [standard Go project layout](https://github.com/golang-standards/project-layout) with the following structure:

```
cowgnition/
├── cmd/                      # Command-line entry points
│   └── server/               # Main server application
│       ├── main.go           # Application entry point
│       └── commands.go       # CLI command definitions
├── configs/                  # Configuration files
│   └── config.example.yaml   # Example configuration template
├── internal/                 # Private application code
│   ├── auth/                 # RTM authentication
│   │   ├── auth_manager.go   # Authentication flow management
│   │   └── token_manager.go  # Secure token storage
│   ├── config/               # Configuration handling
│   │   └── config.go         # Config loading and validation
│   ├── server/               # MCP server implementation
│   │   ├── server.go         # MCP server core
│   │   ├── handlers.go       # HTTP handlers for MCP endpoints
│   │   ├── resources.go      # Resource implementations
│   │   ├── tools.go          # Tool implementations
│   │   ├── middleware.go     # HTTP middleware
│   │   └── utils.go          # Helper functions
│   └── rtm/                  # RTM API client
│       ├── client.go         # HTTP client for RTM API
│       ├── service.go        # Business logic for RTM operations
│       └── types.go          # RTM data models
├── pkg/                      # Shareable libraries
│   └── mcp/                  # MCP protocol utilities
│       └── types.go          # MCP type definitions
├── scripts/                  # Build and utility scripts
│   └── setup.sh              # Development environment setup
└── docs/                     # Documentation
    └── PROJECT_ORGANIZATION.md  # This file
```

## Component Architecture

CowGnition follows a layered architecture with clear separation of concerns:

1. **Command Layer** (`cmd/server/`)

   - Handles parsing command-line arguments
   - Initializes and manages application lifecycle
   - Routes to appropriate commands

2. **Server Layer** (`internal/server/`)

   - Implements the Model Context Protocol
   - Exposes resources and tools
   - Manages HTTP endpoints and request handling

3. **Service Layer** (`internal/rtm/service.go`)

   - Implements business logic for RTM operations
   - Manages authentication state
   - Coordinates client calls

4. **Client Layer** (`internal/rtm/client.go`)

   - Handles HTTP communication with RTM API
   - Implements API request signing
   - Parses API responses

5. **Auth Layer** (`internal/auth/`)

   - Manages authentication flows
   - Securely stores tokens
   - Validates authentication state

6. **Config Layer** (`internal/config/`)
   - Loads and validates configuration
   - Provides access to application settings

## MCP Protocol Implementation

The server implements the Model Context Protocol, which provides a standardized way for AI assistants to interact with external services:

1. **Initialization**

   - Server declares its capabilities
   - Client establishes connection parameters

2. **Resources**

   - Read-only data sources
   - Support parametrized paths
   - Return formatted text content

3. **Tools**
   - Action-oriented capabilities
   - Support arguments for customization
   - Return operation results

## Data Flow 🔄

Here's how information flows when you ask Claude about your tasks:

1. You ask Claude something like "What's due today?" in Claude Desktop
2. Claude thinks "I need to check their RTM account"
3. Claude calls CowGnition behind the scenes using MCP
4. CowGnition quickly checks if you're logged in
5. If you're not yet connected, CowGnition sends back auth instructions
6. If you're all set, CowGnition fetches what you need from RTM
7. CowGnition translates RTM's response into something Claude understands
8. Claude presents your task information in a conversational way

All this happens in seconds – the technical complexity stays hidden while you have a natural conversation with Claude.

## Authentication Flow

The Remember The Milk authentication implementation follows the OAuth-like flow described in the [RTM API documentation](https://www.rememberthemilk.com/services/api/authentication.rtm):

1. User requests access to RTM resources
2. Server generates auth URL with API key
3. User visits URL and authorizes the application
4. User receives a frob from RTM
5. User provides frob to CowGnition via Claude
6. CowGnition exchanges frob for permanent token
7. Token is stored securely for future sessions

## Design Principles 🧩

CowGnition is built on these solid principles:

1. **Separation of Concerns** - Everything has one job and does it well, like RTM's focused approach to task management
2. **Clean API Boundaries** - Components talk to each other through clear channels, no confusion
3. **Security First** - Your RTM connection is treated with care and respect
4. **Friendly Failures** - When something goes wrong, you get helpful guidance, not cryptic errors
5. **Flexibility** - Configuration options let you set things up your way
6. **Testability** - Code that can be thoroughly tested is code you can trust

These principles help us create a reliable bridge between Claude and your carefully curated RTM tasks.

## Related Documentation

- [README.md](../README.md) - Project overview and usage instructions
- [GO_PRACTICES.md](../GO_PRACTICES.md) - Go development guidelines
- [TODO.md](../TODO.md) - Development roadmap
