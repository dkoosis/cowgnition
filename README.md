# Axe Handle Project

Axe Handle is a code generator for creating Model Context Protocol (MCP) servers, written in Go. It takes a service definition as input and produces a Go-based implementation of an MCP-compliant server. This tool bridges the gap between existing services and AI agents by enabling seamless integration through the MCP standard.

## Getting Started

```bash
# Clone the repository
git clone https://github.com/yourusername/axe-handle.git
cd axe-handle

# Build the project
go build -o axe ./cmd/axe

# Run the generator
./axe generate -i schema.proto -o ./generated
```

## Project Documentation

This project is documented through multiple specialized guides:

### Core Documentation

- [**Project Organization**](./docs/PROJECT_ORGANIZATION.md) - Architecture, directory structure, and component design
- [**Code Standards**](./docs/CODE_STANDARDS.md) - Coding guidelines and conventions
- [**Development Guide**](./docs/DEVELOPMENT_GUIDE.md) - Complete guide to development workflow and tooling

### Project Management

- [**Roadmap**](./docs/ROADMAP.md) - Strategic vision, prioritized tasks, and future enhancements

## Key Features

- **Protocol Definition Input**: Takes Protocol Buffer (protobuf) schema as input
- **Go Output**: Generates a Go server implementation with clean architecture
- **MCP Compliance**: Ensures the generated server follows Model Context Protocol specifications
- **Template-based Generation**: Uses Go templates for customizable output
- **Strong Error Handling**: Implements Go's robust error handling patterns
- **Cross-platform Support**: Single binary deployment with no runtime dependencies

## Technical Stack

- **Go**: For type safety, performance, and simpler deployment
- **Go Modules**: Package management
- **Go Templates**: Templating engine for code generation
- **Cobra/Viper**: CLI framework and configuration management
- **Protobuf**: Schema definition format
- **Go Kit**: (Optional) Microservice toolkit for generated servers

## Project Structure

```
axe-handle/
├── cmd/                      # Command-line entry points
│   └── axe/                  # Main CLI application
├── internal/                 # Private application code
│   ├── generator/            # Code generation logic
│   ├── parser/               # Schema parsing
│   ├── mapper/               # Schema to code mapping
│   └── template/             # Template management
├── pkg/                      # Shareable libraries
│   ├── mcp/                  # MCP protocol definitions
│   └── validations/          # Validation utilities
├── templates/                # Go templates for generated code
│   └── server/               # Server templates
├── examples/                 # Example schemas and generated code
└── docs/                     # Project documentation
```

## Contributing

We welcome contributions to the Axe Handle project! To contribute:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes following our [Code Standards](./docs/CODE_STANDARDS.md)
4. Commit your changes (`git commit -m 'Add amazing feature'`)
5. Push to the branch (`git push origin feature/amazing-feature`)
6. Open a Pull Request

For detailed information on development environment setup, workflow, and quality standards, please refer to our [Development Guide](./docs/DEVELOPMENT_GUIDE.md).

## Project Status

Axe Handle is currently in active development. See the [Roadmap](./docs/ROADMAP.md) for current priorities and future plans.
