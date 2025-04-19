// file: cmd/schema_test/main.go.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	// Import config package for config.SchemaConfig.
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"

	// Import schema package for schema.NewValidator.
	"github.com/dkoosis/cowgnition/internal/schema"
)

func main() {
	// Setup logging.
	logger := log.New(os.Stderr, "schema-test: ", log.LstdFlags)
	customLogger := &stdLogger{logger}

	logger.Println("Starting schema validation test.")

	// --- Corrected Schema Configuration ---.
	// Define the path to the schema file relative to the project root.
	// Adjust this path if necessary based on where you run the test from.
	// Assume running from project root for `go run ./cmd/schema_test`.
	schemaFilePath := filepath.Join("internal", "schema", "schema.json")
	schemaFileAbsPath, err := filepath.Abs(schemaFilePath)
	if err != nil {
		logger.Fatalf("Could not determine absolute path for schema: %v.", err)
	}

	// Check if the resolved schema file exists.
	if _, err := os.Stat(schemaFileAbsPath); os.IsNotExist(err) {
		logger.Fatalf("Schema file not found at resolved path: %s. Ensure the path is correct relative to the execution directory.", schemaFileAbsPath)
	}

	// Create config.SchemaConfig using SchemaOverrideURI.
	schemaCfg := config.SchemaConfig{
		// Use file:// prefix for local file paths.
		SchemaOverrideURI: "file://" + schemaFileAbsPath,
	}
	logger.Printf("Using schema override URI: %s.", schemaCfg.SchemaOverrideURI)
	// --- End Corrected Schema Configuration ---.

	// Corrected: Use NewValidator.
	// Create and initialize schema validator with our logger.
	validator := schema.NewValidator(schemaCfg, customLogger) // Pass config.SchemaConfig.
	ctx := context.Background()

	logger.Println("Initializing schema validator.")
	if err := validator.Initialize(ctx); err != nil {
		logger.Fatalf("Failed to initialize schema validator: %v.", err)
	}

	logger.Println("Schema validator initialized successfully.")

	// Test validation directly.
	testMessage := []byte(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`)
	// Use a schema definition key expected to exist in the loaded schema.
	validationTarget := "JSONRPCRequest"
	logger.Printf("Testing validation with message: %s against schema: %s.", string(testMessage), validationTarget)

	err = validator.Validate(ctx, validationTarget, testMessage)
	if err != nil {
		logger.Printf("Validation failed: %v.", err)
	} else {
		logger.Println("Validation succeeded.")
	}

	logger.Println("Schema validation test completed.")
}

// stdLogger adapts a standard log.Logger to our logging.Logger interface.
type stdLogger struct {
	logger *log.Logger
}

func (l *stdLogger) Debug(msg string, args ...any)                { l.log("DEBUG", msg, args...) }
func (l *stdLogger) Info(msg string, args ...any)                 { l.log("INFO", msg, args...) }
func (l *stdLogger) Warn(msg string, args ...any)                 { l.log("WARN", msg, args...) }
func (l *stdLogger) Error(msg string, args ...any)                { l.log("ERROR", msg, args...) }
func (l *stdLogger) WithContext(_ context.Context) logging.Logger { return l }
func (l *stdLogger) WithField(_ string, _ any) logging.Logger     { return l }

func (l *stdLogger) log(level, msg string, args ...any) {
	if len(args) == 0 {
		l.logger.Printf("%s: %s.", level, msg) // Added period.
		return
	}
	formatted := msg
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			key, val := args[i], args[i+1]
			formatted += fmt.Sprintf(" %v=%v", key, val)
		}
	}
	l.logger.Printf("%s: %s.", level, formatted) // Added period.
}
