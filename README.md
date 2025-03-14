# CowGnition

![CowGnition Logo](/assets/cowgnition_logo.png)

## Overview

CowGnition is an MCP (Model Context Protocol) server implementation written in Go that connects Claude Desktop and other MCP clients to the Remember The Milk (RTM) task management service. This server enables AI assistants to interact with your tasks, lists, and reminders through a secure, standardized interface.

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
go install github.com/cowgnition/cowgnition@latest

# Or build from source
git clone https://github.com/cowgnition/cowgnition.git
cd cowgnition
make build
```

## Quickstart

### 1. Get Remember The Milk API credentials

- Register for an API key at [rememberthemilk.com/services/api/keys.rtm](https://www.rememberthemilk.com/services/api/keys.rtm)
- You'll receive an API key and shared secret

### 2. Configure the server

Create a `config.yaml` file:

```yaml
server:
  name: "CowGnition"
  port: 8080

rtm:
  api_key: "your_api_key"
  shared_secret: "your_shared_secret"

auth:
  token_path: "~/.config/cowgnition/tokens"
```

### 3. Run the server

```bash
./cowgnition serve --config configs/config.yaml
```

### 4. Install in Claude Desktop

```bash
mcp install --name "Remember The Milk" --command cowgnition --args "serve --config configs/config.yaml"
```

Or use the development mode to test:

```bash
mcp dev --command ./cowgnition --args "serve --config configs/config.yaml"
```

## What Can CowGnition Do?

With CowGnition installed in Claude Desktop, you can interact with your Remember The Milk tasks in natural language:

- "Add milk to my shopping list"
- "Show me all tasks due today"
- "Mark the dentist appointment as completed"
- "Remind me to pay rent on the 1st of each month"
- "What tasks are tagged as 'important'?"
- "Move the project deadline to next Friday"

## Authentication Flow

The CowGnition server handles the RTM authentication flow:

1. When first accessing RTM resources, Claude will prompt the user to authenticate
2. The server will generate an authentication URL for the user to visit
3. After authorizing access on the Remember The Milk website, the user will receive a verification code
4. The user enters this code in Claude, and the server exchanges it for an auth token
5. The server securely stores the token for future sessions

## Development

### Prerequisites

- Go 1.18 or higher
- Make (optional, for build automation)
- Remember The Milk API key

### Setup Development Environment

```bash
# Clone the repository
git clone https://github.com/cowgnition/cowgnition.git
cd cowgnition

# Run the setup script
./scripts/setup.sh

# Build the project
make build
```

### Development with Hot Reload

For faster development, you can use hot reload:

```bash
make dev
```

This will automatically rebuild and restart the application whenever Go files are changed.

### Implementation Roadmap

For contributors looking to help implement features, check out our [development roadmap](TODO.md) which contains structured guides and prompts for building out the MCP server functionality.

## Architecture

The server is built using a clean architecture approach with several key components:

```
cowgnition/
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

## MCP Tools

CowGnition implements the following MCP tools:

### Task Management

- `add_task`: Create a new task
- `complete_task`: Mark a task as completed
- `uncomplete_task`: Mark a completed task as incomplete
- `delete_task`: Delete a task
- `set_due_date`: Set or update a task's due date
- `set_priority`: Set a task's priority level
- `add_tags`: Add tags to a task
- `remove_tags`: Remove tags from a task
- `add_note`: Add a note to a task

### List Management

- `get_lists`: Retrieve all lists
- `create_list`: Create a new list
- `set_list_name`: Rename a list
- `delete_list`: Delete a list
- `move_task`: Move a task to a different list

### Query Tools

- `get_tasks`: Get all tasks
- `get_tasks_due_today`: Get tasks due today
- `get_tasks_overdue`: Get overdue tasks
- `get_tasks_by_list`: Get tasks in a specific list
- `get_tasks_by_tag`: Get tasks with specific tags
- `get_tasks_by_priority`: Get tasks by priority level
- `search_tasks`: Search tasks using RTM's query syntax

## MCP Resources

CowGnition exposes the following MCP resources:

- `tasks://all`: All tasks across all lists
- `tasks://today`: Tasks due today
- `tasks://tomorrow`: Tasks due tomorrow
- `tasks://week`: Tasks due within the next 7 days
- `tasks://list/{list_id}`: Tasks within a specific list
- `lists://all`: All task lists
- `tags://all`: All tags used in the system

## Contributing

We welcome contributions to the CowGnition project! Please follow these steps:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

Before submitting, please make sure your code passes the existing tests and linting.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgements

- [Model Context Protocol](https://modelcontextprotocol.io/) - For establishing the protocol standard
- [Remember The Milk API](https://www.rememberthemilk.com/services/api/) - For their task management platform
- The open source community for various libraries and tools that made this project possible
