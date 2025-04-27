# Contributing to CowGnition 🐄 🧠

We appreciate your interest in contributing to CowGnition! By following these guidelines, you'll help us keep the project consistent, maintainable, and a moo-ving force in the MCP ecosystem.

## Getting Started

1. **Familiarize yourself with the project.** Read the [README](README.md) to understand CowGnition's purpose and architecture. Skim the docs to get a sense of the different components.
2. **Check the [TODO](docs/TODO.md)** for current development priorities and open issues. This is a great place to find tasks that need attention.
3. **Set up your development environment.** Follow the instructions in the [README](README.md) to clone the repository, install dependencies, and build the project.
4. **Create a new branch** for your changes. Branch names should be descriptive (e.g., `feature/add-tool-x`, `bugfix/issue-y`).

## Code Style Guidelines

We strive for clean, readable, and idiomatic Go code. Please adhere to the following:

- **Go Formatting:** Use `go fmt` to format your code. Most editors do this automatically on save.
- **Error Handling:** Follow the patterns described in [ADR 001](docs/adr/001_error_handling_strategy.md). Handle errors gracefully and provide informative error messages. Don't let errors stampede through your code!
- **Schema Validation:** Ensure all MCP messages are validated against the official schema as outlined in [ADR 002](docs/adr/002_schema_validation_strategy.md). We're udderly serious about protocol compliance.
- **Logging:** Use the `logging` package for structured logging. Provide context in your log messages.
- **Concurrency:** Write concurrency-safe code. Use mutexes or channels appropriately.
- **Testing:** Write unit tests for your code. Aim for high test coverage.
- **Naming:**
  - Use clear, descriptive names. Avoid abbreviations unless they are widely understood.
  - Follow Go's naming conventions (e.g., `camelCase` for functions, `PascalCase` for types).
  - Use `snake_case` for file names (e.g., `user_service.go`). This helps with readability.
  - Avoid generic names like `utils.go` or `helpers.go`. Be specific!

## Folder Structure

We use a domain-centric folder structure to keep things organized:

```code
├── cmd # Main applications
│ └── server # Server application
├── internal # Internal packages (not for external use)
│ ├── config # Configuration handling
│ ├── mcp # MCP-related logic
│ └── rtm # RTM-related logic
└── docs # Documentation
└── adr # Architectural Decision Records
```

- **`cmd/`:** Contains the main entry points for our applications. Each subdirectory represents a separate executable.
- **`internal/`:** Holds packages that are only intended for use within our project. Avoid importing these packages from external projects.
- **`docs/`:** Contains documentation, including ADRs and other design documents.

## Schema and Validation

### Updating schema.json

When updating `schema.json` or adding new MCP methods:

1. Update the schema definition in `internal/schema/schema.json`
2. Add corresponding method-to-schema mappings in `internal/schema/validator.go` in the `schemaMappings` package-level variable.
3. Run the validator tests to verify mappings are complete: `go test ./internal/schema/...`

The system will log warnings at startup and fail tests for any unmapped schema definitions (matching `*Request` or `*Result`), but it's best to keep mappings updated proactively.

## Contributing Workflow

1. **Fork the repository.**
2. **Create a branch** for your changes.
3. **Make your changes.**
4. **Test your changes.** Ensure all tests pass (`make test`).
5. **Document your changes.** Update any relevant documentation.
6. **Commit your changes.** Write clear, concise commit messages.
7. **Push your branch** to your fork.
8. **Create a pull request (PR).** Provide a detailed description of your changes in the PR.
9. **Be responsive to feedback.** We may ask you to make changes to your PR.

## Code Review Process

All contributions are welcome, but we do have a review process to maintain code quality.

- **Reviewers** will check for code style, correctness, test coverage, and adherence to the guidelines.
- **Be patient** - reviews may take some time.
- **Be respectful** - address feedback politely and explain your reasoning.
- **Iterate** - be prepared to revise your code based on feedback.

## Community

- Join our community forum (TODO: Add link) to ask questions, discuss ideas, and get help.
- Be respectful and inclusive in all interactions.

## License

By contributing to CowGnition, you agree that your contributions will be licensed under the [MIT License](LICENSE).

Thank you for contributing to CowGnition! 🐄 🧠
