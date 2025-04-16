// file: cmd/schema_test/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	// We'll import just the schema package for now.
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/schema"
)

func main() {
	// Setup logging
	logger := log.New(os.Stderr, "schema-test: ", log.LstdFlags)
	customLogger := &stdLogger{logger}

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

	// Create and initialize schema validator with our logger.
	validator := schema.NewSchemaValidator(schemaSource, customLogger)
	ctx := context.Background()

	logger.Println("Initializing schema validator...")
	if err := validator.Initialize(ctx); err != nil {
		logger.Fatalf("Failed to initialize schema validator: %v", err)
	}

	logger.Println("Schema validator initialized successfully")

	// Test validation directly without the middleware.
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

// stdLogger adapts a standard log.Logger to our logging.Logger interface.
type stdLogger struct {
	logger *log.Logger
}

func (l *stdLogger) Debug(msg string, args ...any)                  { l.log("DEBUG", msg, args...) }
func (l *stdLogger) Info(msg string, args ...any)                   { l.log("INFO", msg, args...) }
func (l *stdLogger) Warn(msg string, args ...any)                   { l.log("WARN", msg, args...) }
func (l *stdLogger) Error(msg string, args ...any)                  { l.log("ERROR", msg, args...) }
func (l *stdLogger) WithContext(_ context.Context) logging.Logger   { return l }
func (l *stdLogger) WithField(key string, value any) logging.Logger { return l }

func (l *stdLogger) log(level, msg string, args ...any) {
	if len(args) == 0 {
		l.logger.Printf("%s: %s", level, msg)
		return
	}

	formatted := msg
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			key, val := args[i], args[i+1]
			formatted += fmt.Sprintf(" %v=%v", key, val)
		}
	}
	l.logger.Printf("%s: %s", level, formatted)
}
