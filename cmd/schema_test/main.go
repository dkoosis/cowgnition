// file: cmd/schema_test/main.go
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	// We'll import just the schema package for now
	"github.com/dkoosis/cowgnition/internal/schema"
)

func main() {
	// Setup logging
	logger := log.New(os.Stderr, "schema-test: ", log.LstdFlags)
	logger.Println("Starting schema validation test")

	// Setup schema validator
	schemaPath := filepath.Join("internal", "schema", "min_schema.json")

	// Check if schema file exists
	if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
		logger.Printf("Warning: Schema file not found at %s\n", schemaPath)
		logger.Println("Continuing with URL source instead")
		schemaPath = ""
	}

	// Create schema source configuration
	schemaSource := schema.SchemaSource{
		FilePath: schemaPath,
		// Use a reliable schema URL
		URL: "https://raw.githubusercontent.com/anthropics/ModelContextProtocol/main/schema/mcp-schema.json",
	}

	// Create and initialize schema validator
	validator := schema.NewSchemaValidator(schemaSource)
	ctx := context.Background()

	logger.Println("Initializing schema validator...")
	if err := validator.Initialize(ctx); err != nil {
		logger.Fatalf("Failed to initialize schema validator: %v", err)
	}

	logger.Println("Schema validator initialized successfully")

	// Test validation directly without the middleware
	testMessage := []byte(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`)
	logger.Println("Testing validation with message:", string(testMessage))

	err := validator.Validate(ctx, "JSONRPCRequest", testMessage)
	if err != nil {
		logger.Printf("Validation failed: %v", err)
	} else {
		logger.Println("Validation succeeded")
	}

	logger.Println("Schema validation test completed")
}
