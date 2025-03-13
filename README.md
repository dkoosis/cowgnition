# CowGnition - Remember The Milk MCP Server

![CowGnition Logo](/media/cowgnition_logo.png)

## Overview

CowGnition is an MCP (Model Context Protocol) server implementation written in Go that connects Claude and other AI assistants to the Remember The Milk (RTM) task management service. This server enables AI assistants to interact with your tasks, lists, and reminders through a secure, standardized interface.

## Key Features

- 🔄 **Bi-directional sync** between Claude and RTM
- 🔐 **Secure authentication** using RTM's OAuth flow
- 📋 **Task management** - create, read, update, and delete tasks
- 📝 **Note handling** - add and edit notes on tasks
- 🏷️ **Tag support** - manage tags on tasks
- ⏰ **Due date management** - set and modify due dates
- 📊 **List organization** - work with multiple lists

## Installation

```bash
# Install the server
go install github.com/cowgnition/rtm-mcp@latest

# Or build from source
git clone https://github.com/cowgnition/rtm-mcp.git
cd rtm-mcp
go build -o rtm-mcp ./cmd/server
```

## Quickstart

### 1. Get Remember The Milk API credentials

- Register for an API key at [rememberthemilk.com/services/api/keys.rtm](https://www.rememberthemilk.com/services/api/keys.rtm)
- You'll receive an API key and shared secret

### 2. Configure the server

Create a `config.yaml` file:

```yaml
server:
  name: "Remember The Milk Tasks"
  port: 8080

rtm:
  api_key: "your_api_key"
  shared_secret: "your_shared_secret"

auth:
  token_path: "~/.config/rtm-mcp/tokens"
```

### 3. Run the server

```bash
rtm-mcp serve --config config.yaml
```

### 4. Install in Claude Desktop

```bash
mcp install --name "Remember The Milk" --command rtm-mcp --args "serve --config config.yaml"
```

Or use the development mode to test:

```bash
mcp dev --command ./rtm-mcp --args "serve --config config.yaml"
```

## Architecture

The server is built using a clean architecture approach with several key components:

```
rtm-mcp/
├── cmd/                      # Command-line entry points
│   └── server/               # Main server application
├── internal/                 # Private application code
│   ├── auth/                 # RTM authentication 
│   ├── config/               # Configuration handling
│   ├── handler/              # MCP protocol handlers
│   ├── rtm/                  # RTM API client
│   └── server/               # MCP server implementation
├── pkg/                      # Shareable libraries
│   ├── mcp/                  # MCP protocol utilities
│   └── rtmapi/               # RTM API Go client
└── docs/                     # Documentation
```

## Authentication Flow

The CowGnition server handles the RTM authentication flow:

1. When first accessing RTM resources, Claude will prompt the user to authenticate
2. The server will generate an authentication URL for the user to visit
3. After authorizing access on the Remember The Milk website, the user will receive a verification code
4. The user enters this code in Claude, and the server exchanges it for an auth token
5. The server securely stores the token for future sessions

## Available MCP Tools

The server exposes several tools through the MCP interface:

```go
// Add a new task
func (s *Server) AddTask(ctx context.Context, name string, listID string, dueDate string) (string, error) {
    // Implementation
}

// Complete a task
func (s *Server) CompleteTask(ctx context.Context, listID string, taskseriesID string, taskID string) (string, error) {
    // Implementation
}

// Add tags to a task
func (s *Server) AddTags(ctx context.Context, listID string, taskseriesID string, taskID string, tags []string) (string, error) {
    // Implementation
}
```

## MCP Resources

The server exposes several resources through the MCP interface:

```go
// Get all tasks
func (s *Server) GetAllTasks(ctx context.Context) (string, error) {
    // Implementation
}

// Get tasks due today
func (s *Server) GetTasksDueToday(ctx context.Context) (string, error) {
    // Implementation
}

// Get tasks in a list
func (s *Server) GetTasksInList(ctx context.Context, listID string) (string, error) {
    // Implementation
}
```

## Advanced Usage

### Custom Filtering

The server supports RTM's powerful filtering syntax:

```go
// Get filtered tasks
func (s *Server) GetFilteredTasks(ctx context.Context, filter string) (string, error) {
    // Implementation using RTM search syntax
}
```

### Timeline Management

For operations that support undo functionality:

```go
// Create a new timeline
func (s *Server) CreateTimeline(ctx context.Context) (string, error) {
    // Implementation
}

// Undo an operation
func (s *Server) UndoOperation(ctx context.Context, transactionID string) (string, error) {
    // Implementation
}
```

## Contributing

We welcome contributions to the CowGnition project! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgements

- [Model Context Protocol](https://modelcontextprotocol.io/) - For establishing the protocol standard
- [Remember The Milk API](https://www.rememberthemilk.com/services/api/) - For their task management platform