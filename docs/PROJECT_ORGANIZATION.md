# Axe Handle Project Organization

This document describes the architecture, directory structure, organization patterns, and components of the Axe Handle project.

## Overview

Axe Handle is a code generator for creating Model Context Protocol (MCP) servers, written in Go. It takes a service definition as input and produces a Go-based implementation of an MCP-compliant server.

## Table of Contents
- [Core Architecture](#core-architecture)
- [Directory Structure](#directory-structure)
- [Naming Conventions](#naming-conventions)
- [Component Architecture](#component-architecture)
- [Data Flow](#data-flow)
- [Template System](#template-system)
- [Generated Output](#generated-output)

## Core Architecture

Axe Handle follows a pipeline architecture for transforming input schemas into generated code:

```
User Schema (.proto) → Parser → Validator → Mapper → Generator → Go Server
```

### Key Components

1. **Parser**
   - Processes the input schema (Protobuf format)
   - Extracts resources, operations, and types

2. **Validator**
   - Ensures compliance with MCP specifications
   - Validates schema against business rules

3. **Mapper**
   - Transforms parsed schema into an intermediate representation
   - Resolves relationships between resources

4. **Generator**
   - Creates server code from templates using the mapped data
   - Manages output directory structure

### Supporting Systems

1. **Template System**
   - A flexible template engine built around Go's text/template
   - Templates organized by target framework and component type

2. **Validation Utilities**
   - Comprehensive validation for inputs, paths, and schemas
   - Ensures data integrity throughout the pipeline

3. **Error Handling**
   - Structured error types with detailed error wrapping
   - Using Go 1.13+ error wrapping for context enrichment

4. **Logging**
   - Structured logging with levels
   - Performance tracking for generation steps

## Directory Structure

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

### Key Directories

- **`cmd/`**: Contains the main entry points for the application's executables.
- **`internal/`**: Contains private application code that is not meant to be imported by other applications.
- **`pkg/`**: Contains libraries that can be used by other applications.
- **`templates/`**: Contains the actual template files used for code generation.
- **`examples/`**: Contains example input schemas and generated code.
- **`docs/`**: Contains high-level documentation for the project.

## Naming Conventions

### Packages

- Use lowercase, single-word names for packages
- Package names should be nouns
- Avoid underscores in package names

### Files

- Use lowercase with underscores for file names (e.g., `server_generator.go`)
- Test files should be named `*_test.go`
- Main executable files should be in their own package (e.g., `cmd/axe/main.go`)

### Functions and Methods

- Use MixedCaps (camelCase or PascalCase) for function names
- Public functions should start with an uppercase letter
- Private functions should start with a lowercase letter

### Types and Interfaces

- Use PascalCase for type and interface names
- Interfaces with a single method should end with `-er` (e.g., `Parser`)
- Keep interfaces small and focused

## Component Architecture

The Axe Handle project consists of these main components:

### Generator (Axe Handle)

1. **Parser System**
   - Processes Protocol Buffer schemas
   - Validates schema against MCP requirements
   - Extracts resources, operations, and types

2. **Mapper System**
   - Maps parsed schema to MCP concepts
   - Resolves relationships between resources
   - Validates mapping for completeness

3. **Template System**
   - Loads templates from filesystem
   - Renders templates with mapped data
   - Writes generated code to output directory

4. **Code Generator**
   - Coordinates the end-to-end generation process
   - Handles command-line parameters
   - Manages output directory structure

### Generated Server (MCP)

1. **Go-based Server**
   - HTTP endpoints for MCP operations
   - WebSocket support for real-time communication
   - Error handling and validation

2. **Resource Handlers**
   - Operation implementations for each resource
   - Request validation and transformation
   - Response formatting

3. **Documentation**
   - API documentation for the MCP server
   - Resource and operation descriptions
   - Sample requests and responses

## Data Flow

The data flows through the system as follows:

1. **Input**: Protocol Buffer schema defining the service
2. **Parsing**: Schema is parsed into an intermediate representation
3. **Mapping**: Intermediate representation is mapped to MCP concepts
4. **Generation**: Code is generated from templates using mapped data
5. **Output**: Go server implementing the MCP protocol

### Data Flow Diagram

```
┌───────────┐    ┌──────────┐    ┌──────────────┐    ┌───────────┐    ┌───────────┐
│           │    │          │    │              │    │           │    │           │
│  Protobuf │───>│  Parser  │───>│ Intermediate │───>│  Mapper   │───>│  Context  │
│  Schema   │    │          │    │ Representation│    │           │    │ Model     │
│           │    │          │    │              │    │           │    │           │
└───────────┘    └──────────┘    └──────────────┘    └───────────┘    └─────┬─────┘
                                                                            │
                                                                            ▼
┌───────────┐    ┌──────────┐    ┌──────────────┐    ┌───────────┐    ┌───────────┐
│           │    │          │    │              │    │           │    │           │
│  Output   │<───│ Generated│<───│  Rendered    │<───│ Template  │<───│ Template  │
│  Server   │    │  Files   │    │  Templates   │    │ Engine    │    │ Loader    │
│           │    │          │    │              │    │           │    │           │
└───────────┘    └──────────┘    └──────────────┘    └───────────┘    └───────────┘
```

## Template System

Templates are organized by component type:

```
templates/
├── server/
│   ├── main.go.tmpl
│   ├── handlers.go.tmpl
│   ├── types.go.tmpl
│   └── docs.md.tmpl
└── common/
    └── helpers.tmpl
```

### Template Engine

The project uses Go's standard `text/template` and `html/template` packages:

- Templates are parsed once and cached for performance
- Custom functions extend the template functionality
- Each template can include other templates
- Data passed to templates is strongly typed

### Template Functions

A set of helper functions is available within templates:

- Type conversion helpers (camelCase, PascalCase, etc.)
- Code generation helpers (indent, wrapComment, etc.)
- Validation helpers for generated code

## Generated Output

The generated server follows this structure:

```
generated/
├── cmd/
│   └── server/
│       └── main.go           # Entry point
├── internal/
│   ├── handlers/             # Resource handlers
│   ├── models/               # Domain models
│   └── server/               # Server configuration
├── api/
│   └── openapi.yaml          # API documentation
└── README.md                 # Usage instructions
```

The generated code includes:

- Go structs for all resources
- HTTP handlers for all operations
- Request/response validation
- Documentation for the generated API
- Configuration for the server
- Middleware for authentication and logging
