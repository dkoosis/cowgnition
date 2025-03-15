# CowGnition 🐄 🧠

<img src="/assets/cowgnition_logo.png" alt="CowGnition Logo" width="100" height="100">

## Have you Herd?

CowGnition connects your Remember The Milk tasks with Claude Desktop. This MCP server lets you ask your AI assistant to manage your to-do lists, look up due dates, and get insights about your tasks through simple conversations. The [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) allows Claude Desktop, and other conversational agents that are MCP clients, to use "tools" like RTM to do things on your behalf. 

## Quick Links

- [Installation](#installation) - Get up and running
- [Configuration](#configuration) - Set up your connection
- [What Can It Do?](#what-can-cowgnition-do) - See it in action
- [Development Guide](GO_PRACTICES.md) - For the technically curious
- [Project Organization](docs/PROJECT_ORGANIZATION.md) - How it all fits together

## Key Features

- **View tasks and lists** - See what's due or browse specific lists
- **Create and update tasks** - Add new items or modify existing ones
- **Complete tasks** - Check things off right from Claude
- **Manage due dates** - Schedule and reschedule with natural language
- **Work with tags** - Organize and filter your tasks

## Installation

```bash
# Install directly (easiest)
go install github.com/cowgnition/cowgnition@latest

# Or build from source
git clone https://github.com/cowgnition/cowgnition.git
cd cowgnition
make build
```

## Configuration

### 1. Get Your RTM Credentials

Head over to [rememberthemilk.com/services/api/keys.rtm](https://www.rememberthemilk.com/services/api/keys.rtm) to get your API key and shared secret.

### 2. Create Your Config

Make a `config.yaml` file with:

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

### 3. Start It Up

```bash
./cowgnition serve --config configs/config.yaml
```

### 4. Connect to Claude

Install it in Claude Desktop:

```bash
mcp install --name "Remember The Milk" --command cowgnition --args "serve --config configs/config.yaml"
```

Or test it first with:

```bash
mcp dev --command ./cowgnition --args "serve --config configs/config.yaml"
```

## What Can CowGnition Do?

Once connected to Claude Desktop, just start chatting naturally:

- "What's due today?"
- "Add milk to my shopping list"
- "Show me all my tasks tagged as 'important'"
- "Set the dentist appointment for next Tuesday"
- "What's on my work list?"

## Authentication

First time connecting? Here's how it works:

1. Ask Claude about your tasks
2. Claude provides an authorization link
3. Visit the link and approve CowGnition
4. You'll get a special code (frob)
5. Tell Claude this code
6. You're connected!

Your RTM credentials stay secure throughout this process.

## Under the Hood

CowGnition uses the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) to connect Claude with your RTM account. For the technically curious, here's what Claude can access:

### Resources

- `auth://rtm` - Your gateway to RTM
- `tasks://today` - See what's due today
- `tasks://list/{list_id}` - View tasks in specific lists
- `lists://all` - Browse your RTM lists
- `tags://all` - See all your organizational tags

### Tools

CowGnition gives Claude these capabilities:

- Creating tasks in any list
- Completing tasks
- Changing due dates
- Setting priorities
- Adding tags
- And more!

## For Developers

Want to contribute? Check out the [development practices guide](GO_PRACTICES.md) and [project organization](docs/PROJECT_ORGANIZATION.md). We follow standard Go project structure with clean architecture principles.

## Thank You

- The Remember The Milk team for their wonderful task system
- Anthropic for creating the Model Context Protocol
- The Go community for excellent development tools

Moo-ve your productivity forward with CowGnition and Claude! 🐄 🧠
