# ADR-005: Unix-Style Pipeline Data Architecture

## Date

2025-04-10

## Status

Proposed

## Context

The CowGnition MCP server initially integrates with Remember The Milk (RTM), but needs to support additional external services in the future. Our previous draft ADR proposed a service interface approach where each service would implement a large interface with methods for all potential capabilities. However, this approach has limitations:

1. Services must implement all interface methods even if they don't support all capabilities.
2. As new capabilities are added, all service implementations would need updating.
3. The architecture doesn't easily support composition of data between different services.
4. There's no clear path for operations that span multiple services (e.g., showing tasks from both RTM and another task system together).

Many MCP use cases involve working with common data types (tasks, dates, people, organizations) that have shared properties and would benefit from generic operations like filtering, sorting, and aggregation. A more data-centric architecture inspired by Unix pipelines would better support these requirements.

## Decision

We will implement a data-centric pipeline architecture with the following key components:

1. **Common Data Format**: A standard `DataCollection` structure that all services produce and consume.
2. **Data Provider Interface**: A minimal interface that services implement to supply data.
3. **Generic Data Operations**: Standalone functions for common operations like sorting, filtering, and transformation.
4. **Pipeline Execution Engine**: A system for defining and executing multi-step data processing pipelines.
5. **Domain-Specific Processors**: Special processors for operations unique to specific domains (e.g., task management).

### Core Interfaces and Types

```go
// DataProvider is the minimal interface that services implement
type DataProvider interface {
    // GetData retrieves data of a specified type with optional parameters
    GetData(ctx context.Context, dataType string, params map[string]interface{}) (DataCollection, error)

    // Describe what types of data this service can provide
    GetDataTypes() []DataTypeInfo
}

// DataCollection represents any collection of structured objects
type DataCollection struct {
    // Metadata about the collection
    Schema     map[string]SchemaProperty `json:"schema"`
    Source     string                    `json:"source"`
    ItemCount  int                       `json:"itemCount"`

    // The actual data
    Items []map[string]interface{} `json:"items"`
}

// Pipeline represents a series of data processing steps
type Pipeline struct {
    Steps []PipelineStep `json:"steps"`
}

// PipelineStep represents a single operation in a data pipeline
type PipelineStep struct {
    Type    string                 `json:"type"`    // "source", "transform", "sink"
    Service string                 `json:"service"` // For "source" and "sink" steps
    Action  string                 `json:"action"`  // What to do
    Params  map[string]interface{} `json:"params"`  // Parameters for the action
}
```

### Example Pipeline

```json
{
  "steps": [
    {
      "type": "source",
      "service": "rtm",
      "action": "getTasks",
      "params": {
        "list": "Inbox",
        "status": "incomplete"
      }
    },
    {
      "type": "transform",
      "action": "filter",
      "params": {
        "property": "dueDate",
        "operation": "lessThan",
        "value": "tomorrow"
      }
    },
    {
      "type": "transform",
      "action": "sort",
      "params": {
        "property": "priority",
        "ascending": false
      }
    },
    {
      "type": "sink",
      "service": "database",
      "action": "store",
      "params": {
        "collection": "urgent_tasks"
      }
    }
  ]
}
```

### Pipeline Execution

The core of the architecture will be a pipeline executor that:

1. Processes steps sequentially
2. Manages data flow between steps
3. Calls the appropriate service methods or generic operations
4. Handles errors and provides diagnostics

Services don't need to implement complex capabilities - they only need to provide data in the standard format. The pipeline executor applies generic operations based on the properties of the data, not on the capabilities of the services.

## Alternatives Considered

1. **Monolithic Service Interface**: Our original approach. Rejected due to poor extensibility and high implementation burden.

2. **Capability-Based Interfaces**: Breaking the service interface into smaller capability interfaces (ToolProvider, ResourceProvider, etc.). Better than monolithic, but still focused on service capabilities rather than data operations.

3. **gRPC-Based Microservices**: Each service as a separate process communicating via gRPC. Too complex for a desktop application.

4. **Plugin Architecture**: Dynamic loading of service implementations. Adds complexity and has platform limitations.

## Consequences

### Positive

1. **True Composability**: Services can be combined in flexible ways, with data flowing naturally between them.
2. **Property-Based Operations**: Operations depend on data properties, not service capabilities.
3. **Minimal Service Implementation**: Services need only implement a simple data provider interface.
4. **Extensibility**: New operations can be added without modifying existing services.
5. **Familiar Mental Model**: The Unix pipeline concept is well understood and powerful.
6. **Cross-Service Operations**: Natural support for operations spanning multiple services.

### Negative

1. **Schema Management**: Requires careful management of data schemas.
2. **Performance Overhead**: Each pipeline step creates a new copy of the data.
3. **Error Handling Complexity**: Errors in pipeline steps need clear propagation.
4. **Serialization Limits**: All data must be serializable.
5. **Learning Curve**: Pipeline-based programming is different from traditional OO approaches.

## Implementation Plan

1. Define the core `DataCollection` and related types
2. Implement basic pipeline execution engine
3. Create standard operations (filter, sort, transform)
4. Adapt RTM service to implement `DataProvider` interface
5. Add MCP tools for creating and executing pipelines
6. Add pipeline persistence and management
7. Implement additional operations and processors

## References

1. Unix Pipeline Philosophy: https://en.wikipedia.org/wiki/Pipeline_(Unix)
2. Dataflow Programming: https://en.wikipedia.org/wiki/Dataflow_programming
3. Go's io.Reader/io.Writer interfaces (conceptual inspiration)
4. Apache NiFi (for complex pipeline management concepts)
