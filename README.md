# CowGnition
<img src="/assets/cowgnition_logo.png" width="100" height="100">

## Overview

CowGnition is an MCP (Model Context Protocol) server implementation written in Go that connects Claude Desktop and other MCP clients to the Remember The Milk (RTM) task management service. This server enables AI assistants to interact with your tasks, lists, and reminders through a secure, standardized interface.

## Quick Links

- [Installation](#installation) - How to install CowGnition
- [Configuration](#configure-the-server) - Setting up your config file
- [Usage Guide](#what-can-cowgnition-do) - Example commands and queries
- [Development Guide](GO_PRACTICES.md) - Development practices and guidelines
- [Project Roadmap](TODO.md) - Current development status and future plans
- [MCP Resources](#mcp-resources) - Available data resources
- [MCP Tools](#mcp-tools) - Available tools and actions

## Key Features

- 🔄 **Bi-directional sync** between Claude and RTM
- 🔐 **Secure authentication** using RTM's OAuth flow
- 📋 **Task management** - create, read, update, and delete tasks
- 📝 **Note handling** - add notes to tasks
- 🏷️ **Tag support** - add and remove tags on tasks
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

For detailed build instructions and development environment setup, see the [Development section](#development) below or the [development practices guide](GO_PRACTICES.md).

## Quickstart

### 1. Get Remember The Milk API credentials

- Register for an API key at [rememberthemilk.com/services/api/keys.rtm](https://www.rememberthemilk.com/services/api/keys.rtm)
- You'll receive an API key and shared secret

### 2. Configure the server

Create a `config.yaml` file:

```yaml
server:
  name: "CowGnition RTM"
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

## Authentication Flow

The CowGnition server handles the RTM authentication flow:

1. When first accessing RTM resources, Claude will prompt the user to authenticate
2. The server will generate an authentication URL for the user to visit
3. After authorizing access on the Remember The Milk website, the user will receive a frob
4. The user enters this frob in Claude, and the server exchanges it for an auth token
5. The server securely stores the token for future sessions

## Development

CowGnition follows standard Go development practices as outlined in our [Go Practices Guide](GO_PRACTICES.md). If you're looking to contribute, please review our [Project Roadmap](TODO.md) to see what features need implementation.

### Prerequisites

- Go 1.18 or higher
- Make (optional, for build automation)
- Remember The Milk API key ([register here](https://www.rememberthemilk.com/services/api/keys.rtm))

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

## Architecture

The server is built using a clean architecture approach with several key components:

```
cowgnition/
├── cmd/                      # Command-line entry points
│   └── server/               # Main server application
├── internal/                 # Private application code
│   ├── auth/                 # RTM authentication 
│   ├── config/               # Configuration handling
│   ├── server/               # MCP server implementation
│   └── rtm/                  # RTM API client
├── pkg/                      # Shareable libraries
│   └── mcp/                  # MCP protocol utilities
└── configs/                  # Configuration files
```

For a more detailed explanation of the project organization, see the [Project Organization](docs/PROJECT_ORGANIZATION.md) document.

## Protocol Implementation

CowGnition implements the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) to enable secure communication between Claude and Remember The Milk. For information about how MCP works, see the [MCP documentation in this repo](https://github.com/modelcontextprotocol/python-sdk/tree/main).

### MCP Resources

CowGnition exposes the following MCP resources:

- `auth://rtm`: Authentication for Remember The Milk
- `tasks://all`: All tasks across all lists
- `tasks://today`: Tasks due today
- `tasks://tomorrow`: Tasks due tomorrow
- `tasks://week`: Tasks due within the next 7 days
- `tasks://list/{list_id}`: Tasks within a specific list
- `lists://all`: All task lists
- `tags://all`: All tags used in the system

## MCP Tools

CowGnition implements the following MCP tools:

### Authentication
- `authenticate`: Complete authentication with Remember The Milk

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

## API Documentation

CowGnition integrates with the Remember The Milk API. For detailed information on the API endpoints and authentication flow, see:

- [Authentication Process](https://docs.google.com/document/d/drive:///1slmdAa8yBZfDwpSP0ZQlsn1cB11BCpULg_50mPzsjEM?fields=name%2Cowners%2CcreatedTime%2CmodifiedTime%2CmimeType%2CwebViewLink%2Ccapabilities%2FcanDownload%2Csize&includeContent=True#authentication)
- [RTM Tasks Structure](https://docs.google.com/document/d/drive:///1slmdAa8yBZfDwpSP0ZQlsn1cB11BCpULg_50mPzsjEM?fields=name%2Cowners%2CcreatedTime%2CmodifiedTime%2CmimeType%2CwebViewLink%2Ccapabilities%2FcanDownload%2Csize&includeContent=True#tasks)
- [Response Formats](https://docs.google.com/document/d/drive:///1slmdAa8yBZfDwpSP0ZQlsn1cB11BCpULg_50mPzsjEM?fields=name%2Cowners%2CcreatedTime%2CmodifiedTime%2CmimeType%2CwebViewLink%2Ccapabilities%2FcanDownload%2Csize&includeContent=True#response-formats)

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgements

- [Model Context Protocol](https://modelcontextprotocol.io/) - For establishing the protocol standard
- [Remember The Milk API](https://www.rememberthemilk.com/services/api/) - For their task management platform
- The open source community for various libraries and tools that made this project possible
