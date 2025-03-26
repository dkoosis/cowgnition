Implementation Strategy
First Step: Simple Working MCP Server
Let's start by implementing just one thing correctly - a minimal MCP server that:

Can initialize and respond to the MCP /initialize endpoint
Has proper separation of concerns
Uses interfaces at package boundaries
Has no circular dependencies

Our focus will be on getting one thing working perfectly rather than trying to fix everything at once.

Start with the core MCP endpoints only:

/mcp/initialize - Basic server info
/mcp/list_resources - Initially just return empty list
/mcp/read_resource - Basic validation only
/mcp/list_tools - Initially just return empty list
/mcp/call_tool - Basic validation only

Add RTM functionality incrementally:

First implement authentication resource
Then add task resources
Finally add tools for manipulating tasks

Test after each step:

Use the MCP Inspector tool as mentioned in the guidance
Write thorough tests

First Component to Build
Starting with the core MCP server with minimal functionality follows both:

The clean architecture principles (domain-oriented design)
The MCP guidance (start with core functionality first)
