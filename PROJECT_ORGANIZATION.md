# Project Organization

This document describes the organization and architecture of the CowGnition MCP server.

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

## Data Flow

The typical flow for user interactions is:

1. User makes a request to Claude with Claude Desktop
2. Claude identifies need for RTM data or actions
3. Claude calls CowGnition through MCP
4. CowGnition verifies authentication
5. If not authenticated, CowGnition returns auth instructions
6. If authenticated, CowGnition processes the request
7. CowGnition communicates with RTM API
8. Results flow back to Claude and then to the user

## Authentication Flow

The Remember The Milk authentication implementation follows the OAuth-like flow described in the [RTM API documentation](https://www.rememberthemilk.com/services/api/authentication.rtm):

1. User requests access to RTM resources
2. Server generates auth URL with API key
3. User visits URL and authorizes the application
4. User receives a frob from RTM
5. User provides frob to CowGnition via Claude
6. CowGnition exchanges frob for permanent token
7. Token is stored securely for future sessions

## Design Principles

CowGnition follows these key design principles:

1. **Separation of Concerns** - Each component has a single responsibility
2. **Clean API Boundaries** - Clear interfaces between components
3. **Security First** - Proper handling of authentication and tokens
4. **Graceful Degradation** - Helpful error messages when things go wrong
5. **Configurability** - Customizable through configuration files
6. **Testability** - Components designed for easy testing

## Related Documentation

- [README.md](../README.md) - Project overview and usage instructions
- [GO_PRACTICES.md](../GO_PRACTICES.md) - Go development guidelines
- [TODO.md](../TODO.md) - Development roadmap
