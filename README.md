# CowGnition 🐄 🧠

<img src="/assets/cowgnition_logo.png" alt="CowGnition Logo" width="100" height="100">

## Have you Herd?

CowGnition connects your [Remember The Milk](https://www.rememberthemilk.com/) (RTM) tasks with Claude Desktop. This MCP server lets you ask your AI assistant to manage your to-do lists, look up due dates, and get insights about your tasks through simple conversations. The [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) allows Claude Desktop, and other conversational agents that are MCP clients, to use "tools" like RTM to do things on your behalf.

## Quick Links

- [Installation](#installation) - Get up and running
- [Configuration](#configuration) - Set up your connection
- [What Can It Do?](#what-can-cowgnition-do) - See it in action
- [Development Overview](docs/development_overview.md) - For the technically curious
- [Project Organization](docs/PROJECT_ORGANIZATION.md) - How it all fits together

## Key Features

- **View tasks and lists** - See what's due or browse specific lists from RTM
- **Create and update tasks** - Add new items or modify existing ones
- **Complete tasks** - Check things off right from Claude
- **Manage due dates** - Schedule and reschedule with natural language
- **Work with tags** - Organize and filter your tasks

## Installation

```bash
# Install directly (easiest)
go install [github.com/dkoosis/cowgnition@latest](https://www.google.com/search?q=https://github.com/dkoosis/cowgnition%40latest)

# Or build from source
git clone [https://github.com/dkoosis/cowgnition.git](https://github.com/dkoosis/cowgnition.git)
cd cowgnition
make build
# Optionally run tests to verify
make test
```

## Configuration

1. Get Your RTM Credentials
   Head over to the Remember The Milk API key page to get your API key and shared secret.

2. Create Your Config
   Create a configuration file at configs/config.yaml with the following content:

```YAML

server:
  name: "CowGnition RTM"
  port: 8080

rtm:
  api_key: "your_api_key"
  shared_secret: "your_shared_secret"

auth:
  # Directory to store authentication tokens.
  # Ensure this directory exists or the application has permissions to create it.
  token_path: "~/.config/cowgnition/tokens"
```

(Note: The application may need the directory specified in auth.token_path to exist.)

3. Start It Up

```Bash

# Ensure you are in the cloned repository directory if built from source
./cowgnition serve --config configs/config.yaml
```

4. Connect to Claude
   Install it in Claude Desktop:

```Bash

mcp install --name "Remember The Milk" --command cowgnition --args "serve --config configs/config.yaml"
```

Or test it first with:

```Bash

mcp dev --command ./cowgnition --args "serve --config configs/config.yaml"
```

## What Can CowGnition Do?

Once connected to Claude Desktop, just start chatting naturally:

"What's due today on RTM?"
"Add milk to my shopping list"
"Show me all my tasks tagged as 'important'"
"Set the dentist appointment for next Tuesday"
"What's on my work list in Remember The Milk?"

## Authentication

First time connecting? Here's how it works:

Ask Claude about your Remember The Milk tasks
Claude provides an authorization link from CowGnition
Visit the link and approve CowGnition access to your RTM account on rememberthemilk.com
You'll get a special code (frob)
Tell Claude this code
You're connected!
Your RTM credentials stay secure throughout this process using OAuth.

## Under the Hood

CowGnition uses the Model Context Protocol (MCP) to connect Claude with your RTM account. For the technically curious, here's what Claude can access:

### Resources

auth://rtm - Your gateway to RTM
tasks://today - See what's due today
tasks://list/{list_id} - View tasks in specific lists
lists://all - Browse your RTM lists
tags://all - See all your organizational tags

### Tools

CowGnition gives Claude these capabilities:

Creating tasks in any list
Completing tasks
Changing due dates
Setting priorities
Adding tags

## And more!

For Developers
Want to contribute? Check out the Development Overview and Project Organization. We follow standard Go project structure with clean architecture principles.

## License

This project is licensed under the MIT License. See the LICENSE file for details.

## Thank You

The Remember The Milk team for their wonderful task system
Anthropic for creating the Model Context Protocol
The Go community for excellent development tools

Moo-ve your productivity forward with CowGnition and Claude! 🐄 🧠
