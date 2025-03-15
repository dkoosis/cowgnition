# CowGnition 🐄 🧠

![CowGnition Logo](/assets/cowgnition_logo.png)

## Overview

CowGnition brings Remember The Milk to your Claude Desktop. This MCP (Model Context Protocol) server connects Claude directly to your RTM tasks, letting your AI assistant help manage your to-dos with natural conversation. For RTM enthusiasts who've trusted it for years, CowGnition helps bridge your trusted task system with modern AI assistance.

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

## Quickstart ⚡

### 1. Get Your RTM API Credentials

- Grab an API key from [rememberthemilk.com/services/api/keys.rtm](https://www.rememberthemilk.com/services/api/keys.rtm)
- RTM will provide you with an API key and shared secret (treat these like passwords!)

### 2. Create Your Config

Whip up a `config.yaml` file with your credentials:

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

### 3. Fire It Up

```bash
./cowgnition serve --config configs/config.yaml
```

### 4. Connect to Claude

Add CowGnition to Claude Desktop:

```bash
mcp install --name "Remember The Milk" --command cowgnition --args "serve --config configs/config.yaml"
```

Want to test before committing? Try dev mode:

```bash
mcp dev --command ./cowgnition --args "serve --config configs/config.yaml"
```

Once connected, just start chatting with Claude about your tasks!

## What Can CowGnition Do?

With CowGnition installed in Claude Desktop, you can interact with your Remember The Milk tasks in natural language:

- "Add milk to my shopping list"
- "Show me all tasks due today"
- "Mark the dentist appointment as completed"
- "Remind me to pay rent on the 1st of each month"
- "What tasks are tagged as 'important'?"

## Authentication Flow

Getting connected is straightforward – a simple handshake between CowGnition and your RTM account:

1. Ask Claude to check your tasks, and it'll notice you need to authenticate
2. Claude provides a special RTM authorization link
3. Click the link, tell RTM "yes, I trust CowGnition" and you'll get a special code (frob)
4. Tell Claude this code and you're connected
5. CowGnition securely stores your connection for future chats

Your credentials stay secure – CowGnition just gets the permission it needs to be helpful.

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

## Under the Hood 🔧

CowGnition uses the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) to create the seamless connection between Claude and RTM. If you're curious about the technical details, check out the [MCP documentation](https://github.com/modelcontextprotocol/python-sdk/tree/main).

### Available Resources

These are the data channels Claude can tap into:

- `auth://rtm`: The gateway to your RTM account
- `tasks://all`: Your complete task collection
- `tasks://today`: Just what's due today (the "now" list)
- `tasks://tomorrow`: Tomorrow's responsibilities  
- `tasks://week`: Your week at a glance
- `tasks://list/{list_id}`: Tasks from a specific list (like "Shopping" or "Work")
- `lists://all`: All your carefully curated lists
- `tags://all`: Your organizational tags

Claude uses these behind the scenes to fetch exactly what you're asking about.

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

## For the Technically Curious 🔍

CowGnition connects with the same RTM API that powers all your favorite RTM clients. If you want to peek behind the curtain:

- [Authentication Process](https://docs.google.com/document/d/drive:///1slmdAa8yBZfDwpSP0ZQlsn1cB11BCpULg_50mPzsjEM?fields=name%2Cowners%2CcreatedTime%2CmodifiedTime%2CmimeType%2CwebViewLink%2Ccapabilities%2FcanDownload%2Csize&includeContent=True#authentication)
- [Tasks Structure](https://docs.google.com/document/d/drive:///1slmdAa8yBZfDwpSP0ZQlsn1cB11BCpULg_50mPzsjEM?fields=name%2Cowners%2CcreatedTime%2CmodifiedTime%2CmimeType%2CwebViewLink%2Ccapabilities%2FcanDownload%2Csize&includeContent=True#tasks)
- [Response Formats](https://docs.google.com/document/d/drive:///1slmdAa8yBZfDwpSP0ZQlsn1cB11BCpULg_50mPzsjEM?fields=name%2Cowners%2CcreatedTime%2CmodifiedTime%2CmimeType%2CwebViewLink%2Ccapabilities%2FcanDownload%2Csize&includeContent=True#response-formats)

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Thank You 💌

- Bob and Emily at [Remember The Milk](https://www.rememberthemilk.com/) for creating a thoughtful task management system that many of us have relied on for over a decade
- [Anthropic](https://www.anthropic.com/) for the Model Context Protocol that makes this integration possible
- The open source Go community for all the tools that helped build CowGnition

Moo-ve your tasks forward with CowGnition and Claude 🐄 🧠
