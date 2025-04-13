# CowGnition User Guide 🐄 🧠

Welcome to CowGnition! This guide explains how to install and use CowGnition to connect your Remember The Milk (RTM) tasks with AI assistants like Claude Desktop.

## Features

- ✨ Easy setup with guided process
- 🤝 Seamless integration with Claude Desktop
- 🔒 Secure authentication with Remember The Milk
- 🗣️ Natural language task management
- 💻 Works across platforms (macOS, Windows, Linux*)
  *_\*Claude Desktop is not yet available on Linux, but CowGnition can be used with other MCP clients._

## Installation

You can install CowGnition using Homebrew (macOS/Linux), by downloading a release manually, or by building from source.

### Option 1: Homebrew (macOS/Linux Recommended)

```bash
brew install dkoosis/tap/cowgnition
Option 2: Manual InstallationDownload the latest release for your operating system from the CowGnition GitHub Releases page. Look for the .tar.gz (macOS/Linux) or .zip (Windows) file.Extract the downloaded archive.macOS/Linux: Move the extracted cowgnition binary to a directory in your system's PATH, such as /usr/local/bin. You might need sudo.# Example:
tar -xzf cowgnition-*.tar.gz
sudo mv cowgnition /usr/local/bin/
Windows: Extract the cowgnition.exe file to a directory (e.g., C:\Program Files\CowGnition) and add that directory to your system's PATH environment variable.Option 3: Building from Source (Advanced)If you prefer to build from source (requires Go 1.21+ installed):git clone [https://github.com/dkoosis/cowgnition.git](https://github.com/dkoosis/cowgnition.git)
cd cowgnition
make build
sudo make install # Or manually move the built binary './cowgnition' to your PATH
Setup (cowgnition setup)Before using CowGnition for the first time, run the one-time setup wizard. This command will guide you through connecting CowGnition to your Remember The Milk account and configuring Claude Desktop (if installed).Open your terminal or command prompt and run:cowgnition setup
The setup wizard will:Ask for your Remember The Milk API Key and Shared Secret.How to get RTM API credentials:Go to the RTM API Keys page.Sign in with your RTM account.Request an API key for a "Desktop Application".Copy the displayed API Key and Shared Secret.Paste them into the setup prompt when asked.Guide you through the RTM authentication process. This usually involves visiting a URL in your browser, logging into RTM, granting CowGnition permission, and then returning to the terminal.Attempt to automatically configure Claude Desktop by modifying its claude_desktop_config.json file. It will add an entry to launch the CowGnition server automatically when Claude Desktop starts.Store your RTM authentication token securely using your operating system's keychain/credential manager. CowGnition never stores your RTM password.Follow the on-screen instructions carefully.UsageStart the Server (if not using Claude Desktop auto-launch): If you didn't configure Claude Desktop integration during setup, or if you want to run CowGnition independently (e.g., with another MCP client), start the server manually in your terminal:cowgnition serve
The server will run in the foreground until you stop it (Ctrl+C).Use with Claude Desktop:If setup configured Claude Desktop integration, simply restart Claude Desktop. CowGnition should start automatically in the background.You should see a small hammer icon <img src="https://mintlify.s3.us-west-1.amazonaws.com/mcp/images/claude-desktop-mcp-hammer-icon.svg" style={{display: 'inline', margin: 0, height: '1.3em'}} /> appear near the chat input box, indicating MCP tools are available. Clicking it will show the tools provided by CowGnition (like getTasks, createTask, etc.).Now, you can chat with Claude using natural language to manage your RTM tasks! Try prompts like:"What Remember The Milk tasks are due today?""Add 'Buy groceries ^tomorrow #personal !1' to RTM.""What's on my RTM shopping list?" (Assuming you have a list named 'Shopping')"Complete my RTM task 'Pay electricity bill'." (You might need to provide more context if the name is ambiguous)."Are there any urgent tasks in RTM?"Claude will use the CowGnition tools when appropriate, and may ask for your confirmation before performing actions like creating or completing tasks.Testing Connections (cowgnition test)You can run a diagnostic command to check if CowGnition can connect to the RTM API using your stored credentials:cowgnition test
This command attempts to perform basic checks like verifying your authentication token with RTM. (Note: This refers to the cmd/rtm_connection_test functionality, which might be integrated into the main binary or run separately depending on final implementation).Configuration FileWhile setup handles most things, CowGnition uses a configuration file typically located at ~/.config/cowgnition/cowgnition.yaml (macOS/Linux) or %APPDATA%\cowgnition\cowgnition.yaml (Windows). You generally don't need to edit this manually, but it stores non-sensitive settings. Sensitive credentials like API keys and auth tokens are stored securely in your OS keychain/credential manager.FAQHow do I get Remember The Milk API credentials?Go to the RTM API Keys page, sign in, and request an API key for a "Desktop Application". Use the provided API Key and Shared Secret during cowgnition setup.Does CowGnition store my RTM password?No. CowGnition uses an OAuth-like flow provided by RTM. It stores an authentication token securely, but never sees or stores your actual RTM password.How do I update CowGnition?If installed via Homebrew: brew upgrade cowgnition. If installed manually, download the latest release and replace the old binary. If built from source, git pull the latest changes and make install.How do I remove the Claude Desktop integration?Edit the claude_desktop_config.json file (path shown during setup or in Claude Desktop Developer settings) and remove the "cowgnition" entry (or the specific name you used) under mcpServers. Then restart Claude Desktop.Where can I find logs?CowGnition logs messages to standard error when run manually (cowgnition serve). When run by Claude Desktop, logs might be captured in Claude's log directory (~/Library/Logs/Claude/mcp-server-cowgnition.log on macOS, similar path on Windows - check Claude Desktop documentation or settings). You can
```
