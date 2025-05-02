// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
package schema

// file: internal/schema/validator.go

import (
	"bytes"
	"context"
	_ "embed" // Used for embedding the default schema.json.
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp" // Used for schema version heuristics.
	"sort"   // Added for sorting unmapped schemas list.
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Import mcptypes package.
	"github.com/santhosh-tekuri/jsonschema/v5"                  // External library for JSON Schema validation.
)

//go:embed schema.json
var embeddedSchemaContent []byte // Holds the content of the default embedded MCP schema.

// file: internal/schema/validator.go

// --- UPDATED: Package-level variable for mappings ---.
// This map connects incoming message type hints (like method names or generic types)
// or outgoing response hints (like request_method + "_response")
// to the corresponding schema definition names found within the compiled schema.
// The validator uses this to select the correct schema for validation.
var schemaMappings = map[string][]string{
	// --- Base JSON-RPC types (used internally or as fallbacks) ---
	"JSONRPCRequest":      {"JSONRPCRequest"},
	"JSONRPCResponse":     {"JSONRPCResponse"},
	"JSONRPCNotification": {"JSONRPCNotification"},
	"JSONRPCError":        {"JSONRPCError"},
	"base":                {"base"},

	// --- Incoming Message Type Hint Mappings ---
	// Generic fallbacks (add base request/server types here)
	"request":      {"JSONRPCRequest", "Request", "ClientRequest", "ServerRequest", "PaginatedRequest"},
	"notification": {"JSONRPCNotification", "Notification"},

	// Specific requests (map method name to request schema definition)
	"initialize":               {"InitializeRequest"},
	"ping":                     {"PingRequest", "JSONRPCRequest"},
	"shutdown":                 {"JSONRPCRequest"},
	"tools/list":               {"ListToolsRequest", "JSONRPCRequest"},
	"tools/call":               {"CallToolRequest"},
	"resources/list":           {"ListResourcesRequest", "JSONRPCRequest"},
	"resources/read":           {"ReadResourceRequest"},
	"resources/subscribe":      {"SubscribeRequest"},
	"resources/unsubscribe":    {"UnsubscribeRequest"},
	"prompts/list":             {"ListPromptsRequest", "JSONRPCRequest"},
	"prompts/get":              {"GetPromptRequest"},
	"logging/setLevel":         {"SetLevelRequest"},
	"$/cancelRequest":          {"CancelledNotification", "$/cancelRequest"}, // Added explicit self-mapping
	"completion/complete":      {"CompleteRequest"},                          // Added
	"sampling/createMessage":   {"CreateMessageRequest"},                     // Added
	"resources/templates/list": {"ListResourceTemplatesRequest"},             // Added
	"roots/list":               {"ListRootsRequest"},                         // Added

	// Specific notifications (map method name to notification schema definition)
	"exit":                                 {"JSONRPCNotification"},
	"notifications/initialized":            {"InitializedNotification"},
	"notifications/progress":               {"ProgressNotification"},
	"notifications/cancelled":              {"CancelledNotification"},
	"notifications/message":                {"LoggingMessageNotification"},
	"notifications/roots/list_changed":     {"RootsListChangedNotification"},
	"notifications/resources/list_changed": {"ResourceListChangedNotification"},
	"notifications/resources/updated":      {"ResourceUpdatedNotification"},
	"notifications/prompts/list_changed":   {"PromptListChangedNotification"},
	"notifications/tools/list_changed":     {"ToolListChangedNotification"},

	// --- Outgoing Response Mappings ---
	// (map request_method + "_response" or generic "response" to Result schema definition)
	// Generic response fallback (add base result types here)
	"response": {"JSONRPCResponse", "Result", "ClientResult", "ServerResult", "PaginatedResult"},

	// Specific responses
	"initialize_response":               {"InitializeResult"},
	"ping_response":                     {"EmptyResult", "JSONRPCResponse", "Result"},
	"shutdown_response":                 {"EmptyResult", "JSONRPCResponse", "Result"},
	"tools/list_response":               {"ListToolsResult"},
	"tools/call_response":               {"CallToolResult"},
	"resources/list_response":           {"ListResourcesResult"},
	"resources/read_response":           {"ReadResourceResult"},
	"resources/subscribe_response":      {"EmptyResult", "JSONRPCResponse", "Result"},
	"resources/unsubscribe_response":    {"EmptyResult", "JSONRPCResponse", "Result"},
	"prompts/list_response":             {"ListPromptsResult"},
	"prompts/get_response":              {"GetPromptResult"},
	"logging/setLevel_response":         {"EmptyResult", "JSONRPCResponse", "Result"},
	"completion/complete_response":      {"CompleteResult"},
	"sampling/createMessage_response":   {"CreateMessageResult"},
	"resources/templates/list_response": {"ListResourceTemplatesResult"},
	"roots/list_response":               {"ListRootsResult"},
}

// Validator handles loading, compiling, and validating data against JSON schemas.
// It manages the schema source (embedded or external), compilation caching, and provides
// the core validation logic based on the jsonschema/v5 library.
type Validator struct {
	schemaConfig        config.SchemaConfig           // Configuration specifying schema source.
	compiler            *jsonschema.Compiler          // The underlying schema compiler instance.
	schemas             map[string]*jsonschema.Schema // Cache of compiled schema definitions by name/pointer AND alias.
	schemaDoc           map[string]interface{}        // Parsed raw schema document (for inspection/heuristics).
	mu                  sync.RWMutex                  // Protects access to schemas, schemaDoc, and initialized status.
	httpClient          *http.Client                  // Used for fetching schemas from HTTP(S) URIs.
	initialized         bool                          // Flag indicating if Initialize() completed successfully.
	logger              logging.Logger                // For internal logging.
	lastLoadDuration    time.Duration                 // Performance metric: time spent loading schema source.
	lastCompileDuration time.Duration                 // Performance metric: time spent compiling schema definitions.
	schemaVersion       string                        // Detected version string of the loaded schema.
}

// Ensure Validator implements the mcptypes.ValidatorInterface.
var _ mcptypes.ValidatorInterface = (*Validator)(nil)

// NewValidator creates a new Validator instance.
// It initializes the schema compiler and HTTP client based on the provided configuration and logger.
// The validator is not ready for use until Initialize() is called successfully.
func NewValidator(cfg config.SchemaConfig, logger logging.Logger) *Validator {
	if logger == nil {
		logger = logging.GetNoopLogger() // Ensure logger is never nil.
	}

	// Configure the jsonschema compiler.
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020 // Use specified draft.
	compiler.AssertFormat = true          // Enable format assertion (e.g., "date-time", "uri").
	compiler.AssertContent = true         // Enable content assertion (e.g., "contentEncoding", "contentMediaType").

	v := &Validator{
		schemaConfig: cfg,
		compiler:     compiler,
		schemas:      make(map[string]*jsonschema.Schema),     // Initialize map.
		schemaDoc:    make(map[string]interface{}),            // Initialize map.
		httpClient:   &http.Client{Timeout: 30 * time.Second}, // Default HTTP client with timeout.
		logger:       logger.WithField("component", "schema_validator"),
		initialized:  false, // Not initialized yet.
	}
	return v
}

// Initialize loads the schema content from the configured source (URI override or embedded),
// compiles the base schema and all its definitions, caches them, performs mapping verification,
// and marks the validator as ready.
// It returns an error if loading or compilation fails critically.
// MODIFIED: Uses explicit lock management as proposed by user.
func (v *Validator) Initialize(_ context.Context) error {
	initStart := time.Now()
	v.mu.Lock() // Lock for modifying validator state.

	// Remove the defer v.mu.Unlock() to manage the lock explicitly

	if v.initialized {
		v.logger.Warn("Schema validator already initialized, skipping.")
		v.mu.Unlock() // Release lock before returning
		return nil
	}
	v.logger.Info("Initializing schema validator...")

	var schemaData []byte
	var sourceInfo string
	var loadWarning string
	useEmbedded := false // Flag to signal using embedded schema.

	loadStart := time.Now()

	// --- Load Schema Data ---
	if v.schemaConfig.SchemaOverrideURI != "" {
		v.logger.Info("SchemaOverrideURI is set, attempting to load external schema.", "uri", v.schemaConfig.SchemaOverrideURI)
		// Pass background context to loader as Initialize context might be short-lived
		// Need to unlock before potentially blocking call
		v.mu.Unlock()
		loadedData, loadErr := loadSchemaFromURI(context.Background(), v.schemaConfig.SchemaOverrideURI, v.logger, v.httpClient)
		v.mu.Lock() // Re-acquire lock after call

		if loadErr != nil {
			// Check if the error is specifically a "not found" type.
			rootCause := errors.Cause(loadErr)
			var validationErr *ValidationError
			isNotFound := os.IsNotExist(rootCause) || (errors.As(loadErr, &validationErr) && validationErr.Code == ErrSchemaNotFound)

			if isNotFound {
				// If override not found, issue a warning and fall back to embedded.
				loadWarning = fmt.Sprintf("Schema override '%s' not found, falling back to embedded schema.", v.schemaConfig.SchemaOverrideURI)
				v.logger.Warn(loadWarning)
				useEmbedded = true // Set flag to use embedded.
			} else {
				// Other loading errors are fatal.
				v.logger.Error("CRITICAL: Failed to load schema from configured SchemaOverrideURI. Initialization aborted.",
					"uri", v.schemaConfig.SchemaOverrideURI, "error", fmt.Sprintf("%+v", loadErr))
				// Wrap the specific load error.
				errToReturn := errors.Wrapf(loadErr, "failed to load schema from configured override URI '%s'", v.schemaConfig.SchemaOverrideURI)
				v.mu.Unlock() // Unlock before returning error
				return errToReturn
			}
		} else {
			// Override loaded successfully.
			schemaData = loadedData
			sourceInfo = fmt.Sprintf("override URI: %s", v.schemaConfig.SchemaOverrideURI)
			v.logger.Info("Successfully loaded schema from override URI.")
		}
	} else {
		// No override URI set, use embedded.
		useEmbedded = true
	}

	// Load embedded schema if flagged (either no override or fallback).
	if useEmbedded {
		v.logger.Info("Using embedded schema.")
		if len(embeddedSchemaContent) == 0 {
			// This is a critical build/embed issue.
			err := errors.New("embedded schema content is empty and no valid override was loaded")
			v.logger.Error("CRITICAL: Embedded schema is empty. Initialization aborted.", "error", err)
			errToReturn := NewValidationError(ErrSchemaLoadFailed, "Embedded schema content is empty", err)
			v.mu.Unlock() // Unlock before returning error
			return errToReturn
		}
		schemaData = embeddedSchemaContent
		sourceInfo = "embedded"
	}

	v.lastLoadDuration = time.Since(loadStart)

	// Final check if schema data is actually present.
	if len(schemaData) == 0 {
		err := errors.New("schema data is unexpectedly empty after load/fallback logic")
		v.logger.Error("Schema loading resulted in empty data. Initialization aborted.", "source", sourceInfo, "error", err)
		errToReturn := NewValidationError(ErrSchemaLoadFailed, "Loaded schema data is empty", err)
		v.mu.Unlock() // Unlock before returning error
		return errToReturn
	}

	v.logger.Info("Schema content prepared.",
		"duration", v.lastLoadDuration,
		"source", sourceInfo,
		"sizeBytes", len(schemaData))
	if loadWarning != "" {
		v.logger.Warn(loadWarning) // Repeat warning if fallback occurred.
	}

	// --- Compile Loaded Schema ---
	var parsedDoc map[string]interface{}
	if err := json.Unmarshal(schemaData, &parsedDoc); err != nil {
		v.logger.Error("Failed to parse loaded schema JSON.", "error", err, "source", sourceInfo)
		errToReturn := NewValidationError(ErrSchemaLoadFailed, "Failed to parse schema JSON", errors.Wrap(err, "json.Unmarshal failed")).
			WithContext("source", sourceInfo)
		v.mu.Unlock() // Unlock before returning error
		return errToReturn
	}

	// Attempt to detect schema version from content.
	v.extractSchemaVersion(schemaData) // Sets v.schemaVersion internally.
	finalSchemaVersion := v.schemaVersion
	if finalSchemaVersion == "" {
		finalSchemaVersion = "[unknown]"
	}

	// Reset compiler and add the new resource.
	v.compiler = jsonschema.NewCompiler() // Create new compiler instance for safety.
	v.compiler.Draft = jsonschema.Draft2020
	v.compiler.AssertFormat = true
	v.compiler.AssertContent = true

	addStart := time.Now()
	schemaReader := bytes.NewReader(schemaData)
	resourceID := "mcp://schema.json" // Base URI for internal references.
	if err := v.compiler.AddResource(resourceID, schemaReader); err != nil {
		v.logger.Error("Failed to add schema resource to compiler.",
			"duration", time.Since(addStart),
			"resourceID", resourceID,
			"source", sourceInfo,
			"error", err)
		errToReturn := NewValidationError(
			ErrSchemaLoadFailed,
			"Failed to add schema resource",
			errors.Wrap(err, "compiler.AddResource failed"),
		).WithContext("source", sourceInfo).WithContext("schemaSize", len(schemaData))
		v.mu.Unlock() // Unlock before returning error
		return errToReturn
	}
	v.logger.Info("Schema resource added.", "duration", time.Since(addStart), "resourceID", resourceID)

	// Compile the base schema and all definitions found within it.
	compileStart := time.Now()
	// Need to unlock before potentially long compile, re-lock after
	v.mu.Unlock()
	compiledSchemas, compileErr := v.compileAllDefinitions(resourceID, parsedDoc)
	v.mu.Lock() // Re-acquire lock
	v.lastCompileDuration = time.Since(compileStart)

	if compileErr != nil {
		// If compilation failed, initialization fails.
		v.logger.Error("Schema compilation finished with errors. Initialization aborted.", "duration", v.lastCompileDuration, "firstError", compileErr)
		// compileErr is already structured
		v.mu.Unlock() // Unlock before returning error
		return compileErr
	}

	// Store compiled schemas and parsed doc, mark as initialized.
	v.schemaDoc = parsedDoc
	v.schemas = compiledSchemas
	v.initialized = true
	initDuration := time.Since(initStart)

	// IMPORTANT: Release the lock before calling VerifyMappingsAgainstSchema
	v.mu.Unlock() // <<< USER'S PROPOSED FIX LOCATION

	// Log success and Verify mappings *after* releasing the write lock
	v.logger.Info("✅ Schema validator initialized successfully.",
		"totalDuration", initDuration.String(),
		"loadDuration", v.lastLoadDuration.String(),
		"compileDuration", v.lastCompileDuration.String(),
		"schemaVersion", finalSchemaVersion,
		"schemasCompiled", len(compiledSchemas),
		"schemaSource", sourceInfo)

	// Now verify mappings after the lock is released
	// Note: VerifyMappingsAgainstSchema acquires its own read lock
	unmappedSchemas := v.VerifyMappingsAgainstSchema()
	if len(unmappedSchemas) > 0 {
		v.logger.Warn("Detected schema definitions without corresponding entries in schemaMappings variable.",
			"unmappedDefinitions", unmappedSchemas,
			"action", "Update schemaMappings variable in internal/schema/validator.go")
	}

	return nil
}

// compileAllDefinitions compiles the base schema and all definitions found under the "definitions" key.
// Returns the map of compiled schemas and the first compilation error encountered, if any.
func (v *Validator) compileAllDefinitions(baseResourceID string, schemaDoc map[string]interface{}) (map[string]*jsonschema.Schema, error) {
	compiled := make(map[string]*jsonschema.Schema)
	var firstCompileError error

	v.logger.Info(">>> DEBUG: Compiling base schema resource...", "resourceID", baseResourceID) // Log before base compile

	// Compile the base schema document itself.
	baseSchema, err := v.compiler.Compile(baseResourceID)
	if err != nil {
		v.logger.Error("CRITICAL: Failed to compile base schema resource.", "resourceID", baseResourceID, "error", err)
		// This is usually fatal for validation.
		return nil, NewValidationError(
			ErrSchemaCompileFailed,
			"Failed to compile base schema resource",
			errors.Wrap(err, "compiler.Compile failed for base schema"),
		).WithContext("pointer", baseResourceID)
	}
	compiled["base"] = baseSchema
	v.logger.Debug("Compiled base schema definition.", "name", "base")

	// Compile individual definitions (e.g., "#/definitions/InitializeRequest").
	if defs, ok := schemaDoc["definitions"].(map[string]interface{}); ok {
		v.logger.Info(">>> DEBUG: Starting compilation of definitions...", "count", len(defs)) // Log count
		i := 0                                                                                 // Counter
		for name := range defs {
			i++
			pointer := baseResourceID + "#/definitions/" + name
			// ---> Log *before* compiling each definition <---
			v.logger.Info(fmt.Sprintf(">>> DEBUG: Compiling definition %d/%d: %s", i, len(defs), name), "pointer", pointer)

			schemaCompiled, errCompile := v.compiler.Compile(pointer)

			if errCompile != nil {
				// Log warnings for individual definition failures but continue.
				// ---> Log the specific compile error <---
				v.logger.Warn("CRITICAL: Failed to compile schema definition.", "name", name, "pointer", pointer, "error", fmt.Sprintf("%+v", errCompile))
				if firstCompileError == nil { // Store only the first error encountered.
					firstCompileError = NewValidationError(
						ErrSchemaCompileFailed,
						fmt.Sprintf("Failed to compile schema definition '%s'", name),
						errors.Wrap(errCompile, "compiler.Compile failed"),
					).WithContext("pointer", pointer)
				}
				// ---> Log *after* failure <---
				v.logger.Warn(">>> DEBUG: Finished compiling definition (FAILED)", "name", name)
			} else {
				// Store successfully compiled definition.
				compiled[name] = schemaCompiled
				// ---> Log *after* success <---
				v.logger.Debug(">>> DEBUG: Finished compiling definition (SUCCESS)", "name", name)
			}
		}
		v.logger.Info(">>> DEBUG: Finished compiling all definitions.") // Log completion
	} else {
		v.logger.Warn("No 'definitions' section found in the schema JSON.")
	}

	// Add convenient aliases based on the package-level schemaMappings variable.
	v.addGenericMappings(compiled)

	// Return the map and the first error encountered (if any).
	// The caller decides if this error is fatal.
	return compiled, firstCompileError
}

// addGenericMappings uses the package-level schemaMappings variable to create aliases
// in the compiledSchemas map.
func (v *Validator) addGenericMappings(compiledSchemas map[string]*jsonschema.Schema) {
	mapped := make([]string, 0)
	// Iterate through desired generic names/aliases defined in schemaMappings.
	for genericName, potentialTargets := range schemaMappings {
		// Skip if the alias name itself was already explicitly defined/compiled (unlikely).
		if _, exists := compiledSchemas[genericName]; exists {
			continue
		}
		// Check potential target definitions in order of preference.
		for _, targetDefName := range potentialTargets {
			if targetSchema, ok := compiledSchemas[targetDefName]; ok {
				// If a target definition was compiled, map the alias name to the compiled target schema.
				compiledSchemas[genericName] = targetSchema
				mapped = append(mapped, fmt.Sprintf("%s->%s", genericName, targetDefName))
				break // Stop checking targets for this alias once a match is found.
			}
		}
	}
	if len(mapped) > 0 {
		v.logger.Debug("Added schema mappings/aliases.", "mappings", mapped)
	}
}

// VerifyMappingsAgainstSchema checks if all compiled schema definitions that appear to be
// requests or results have corresponding entries in the schemaMappings targets.
// This helps ensure the mapping list stays synchronized with the schema definitions.
// Note: This is a heuristic check based on naming conventions (*Request, *Result).
func (v *Validator) VerifyMappingsAgainstSchema() []string {
	// --- Use Read Lock as this doesn't modify state ---.
	v.mu.RLock()
	defer v.mu.RUnlock()
	// --- END ---.

	if v.schemas == nil {
		v.logger.Error("VerifyMappingsAgainstSchema called but schemas map is nil.")
		return nil // Should not happen if called after successful compilation.
	}

	mappedTargets := make(map[string]bool)
	for _, targetList := range schemaMappings {
		for _, targetName := range targetList {
			mappedTargets[targetName] = true
		}
	}

	missingMappings := []string{}

	// Iterate through all compiled schema definition names (keys of v.schemas).
	for defName := range v.schemas {
		// Apply heuristic: Check if definition name looks like a request or result.
		// And ignore the base JSON-RPC types themselves as they are mapped *from*, not *to*.
		isRequestOrResult := (strings.HasSuffix(defName, "Request") || strings.HasSuffix(defName, "Result"))
		isBaseType := (defName == "JSONRPCRequest" || defName == "JSONRPCResponse" ||
			defName == "JSONRPCNotification" || defName == "JSONRPCError" || defName == "base")

		if isRequestOrResult && !isBaseType {
			// Check if this definition name exists as a target in our schemaMappings variable.
			if _, isMapped := mappedTargets[defName]; !isMapped {
				missingMappings = append(missingMappings, defName)
			}
		}
	}

	// Sort for consistent output in logs/tests.
	sort.Strings(missingMappings)
	return missingMappings
}

// GetLoadDuration returns the duration of the last schema loading operation (reading from source).
func (v *Validator) GetLoadDuration() time.Duration {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.lastLoadDuration
}

// GetCompileDuration returns the duration of the last schema compilation operation.
func (v *Validator) GetCompileDuration() time.Duration {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.lastCompileDuration
}

// Shutdown cleans up resources used by the validator, primarily closing idle HTTP connections.
// It also marks the validator as uninitialized and clears internal caches.
func (v *Validator) Shutdown() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.initialized {
		v.logger.Debug("Shutdown called on already uninitialized validator.")
		return nil // Nothing to do if not initialized.
	}
	v.logger.Info("Shutting down schema validator.")

	// Close idle connections in the HTTP client.
	// Check if the transport is the standard http.Transport type.
	if transport, ok := v.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	} else if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		// Fallback: try closing idle connections on the default transport.
		dt.CloseIdleConnections()
	}

	// Clear internal state.
	v.schemas = nil
	v.schemaDoc = nil
	v.initialized = false
	v.schemaVersion = "" // Reset version.
	v.logger.Info("Schema validator shut down.")
	return nil
}

// IsInitialized returns true if the validator has successfully loaded and compiled schemas.
func (v *Validator) IsInitialized() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.initialized
}

// Validate checks if the given JSON data bytes conform to the schema associated with the messageType.
// It first ensures the validator is initialized and the data is valid JSON syntax.
// It then finds the appropriate compiled schema (using fallbacks if necessary) and performs validation.
// Returns a structured ValidationError if validation fails or prerequisites are not met.
func (v *Validator) Validate(_ context.Context, messageType string, data []byte) error { // Rename ctx to _.
	if !v.IsInitialized() {
		return NewValidationError(ErrSchemaNotFound, "Schema validator not initialized", nil)
	}

	var instance interface{}
	// Check if the input data is valid JSON syntax first.
	if err := json.Unmarshal(data, &instance); err != nil {
		// If JSON parsing fails, return a specific error code.
		return NewValidationError(
			ErrInvalidJSONFormat,
			"Invalid JSON format", // More specific message.
			errors.Wrap(err, "json.Unmarshal failed"),
		).WithContext("messageType", messageType).
			WithContext("dataPreview", calculatePreview(data)) // Assumes calculatePreview is in helpers.go.
	}

	// Find the specific compiled schema for the message type, using fallbacks.
	schemaToUse, schemaUsedKey, ok := v.getSchemaForMessageType(messageType)
	if !ok {
		// If no suitable schema definition is found after fallbacks.
		v.mu.RLock()
		availableKeys := getSchemaKeys(v.schemas) // Get keys for error context.
		v.mu.RUnlock()
		return NewValidationError(
			ErrSchemaNotFound,
			fmt.Sprintf("Schema definition not found for message type '%s' or standard fallbacks.", messageType),
			nil,
		).WithContext("messageType", messageType).
			WithContext("availableSchemas", availableKeys) // List available schemas for debugging.
	}

	// Perform the actual validation against the chosen schema.
	validationStart := time.Now()
	// Validate the unmarshalled Go representation (`instance`) against the schema.
	err := schemaToUse.Validate(instance)
	validationDuration := time.Since(validationStart)

	if err != nil {
		// If validation fails, check if it's a jsonschema specific error.
		var valErr *jsonschema.ValidationError
		if errors.As(err, &valErr) {
			// Convert the library-specific error into our custom ValidationError.
			v.logger.Debug("Schema validation failed.",
				"duration", validationDuration,
				"messageType", messageType,
				"schemaUsed", schemaUsedKey,
				"error", valErr.Message) // Log the specific jsonschema message.
			// Pass necessary info to convertValidationError.
			return convertValidationError(valErr, messageType, data) // Assumes convertValidationError is in errors.go.
		}
		// Handle unexpected errors during the validation process itself.
		v.logger.Error("Unexpected error during schema.Validate call.",
			"duration", validationDuration,
			"messageType", messageType,
			"schemaUsed", schemaUsedKey,
			"error", err)
		return NewValidationError(
			ErrValidationFailed,
			"Schema validation failed with unexpected error",
			errors.Wrap(err, "schema.Validate failed unexpectedly"),
		).WithContext("messageType", messageType).
			WithContext("dataPreview", calculatePreview(data))
	}

	// Validation successful.
	v.logger.Debug("Schema validation successful.",
		"duration", validationDuration,
		"messageType", messageType,
		"schemaUsed", schemaUsedKey)
	return nil
}

// getSchemaForMessageType finds the appropriate compiled schema based on the message type hint.
// It uses the package-level schemaMappings variable to look up aliases.
// Returns the schema, the key used to find it (for logging), and true if found.
func (v *Validator) getSchemaForMessageType(messageType string) (*jsonschema.Schema, string, bool) {
	v.mu.RLock() // Read lock for accessing the schemas map.
	defer v.mu.RUnlock()

	// Check the schemaMappings first (this covers exact matches and aliases).
	if potentialTargets, ok := schemaMappings[messageType]; ok {
		for _, targetDefName := range potentialTargets {
			if schema, found := v.schemas[targetDefName]; found {
				// Found a compiled schema for one of the targets.
				// Return the schema, using the original messageType as the key found (for logging clarity).
				return schema, messageType, true
			}
		}
		// If targets were listed but none were compiled, log a warning.
		v.logger.Warn("Mapping found, but target schema definition not compiled.", "mappingKey", messageType, "targets", potentialTargets)
	}

	// If not found via mappings, log and return failure.
	// This replaces the old complex fallback logic.
	v.logger.Error("Could not find schema definition or alias for message type.", "requestedType", messageType)
	return nil, "", false
}

// HasSchema checks if a compiled schema definition exists for the given name (could be definition name or alias).
// Acquires a read lock to safely access the internal schemas map.
func (v *Validator) HasSchema(name string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.schemas == nil {
		return false // Not initialized or shut down.
	}
	_, ok := v.schemas[name]
	return ok
}

// getSchemaKeys returns the names (keys) of all currently compiled schemas.
// Useful for debugging schema loading issues.
func getSchemaKeys(schemas map[string]*jsonschema.Schema) []string {
	if schemas == nil {
		return []string{}
	}
	keys := make([]string, 0, len(schemas))
	for k := range schemas {
		keys = append(keys, k)
	}
	// Consider sorting keys for consistent output if needed: sort.Strings(keys).
	return keys
}

// GetSchemaVersion returns the detected version string of the loaded schema.
// Returns "[unknown]" if the version could not be determined.
func (v *Validator) GetSchemaVersion() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.schemaVersion == "" {
		return "[unknown]"
	}
	return v.schemaVersion
}

// extractSchemaVersion attempts various heuristics to find a version string within the schema content.
// Updates the internal v.schemaVersion field if a version is found.
// Assumes write lock is held by the caller (Initialize).
func (v *Validator) extractSchemaVersion(data []byte) {
	var schemaDoc map[string]interface{}
	logger := v.logger // Use validator's logger.
	if err := json.Unmarshal(data, &schemaDoc); err != nil {
		logger.Warn("Failed to unmarshal schema to extract version; version will be unknown.", "error", err)
		v.schemaVersion = "" // Ensure it's empty if parsing fails.
		return
	}

	// Try different common locations/patterns for version info.
	detectedVersion := v.getVersionFromSchemaField(schemaDoc)
	if detectedVersion == "" {
		detectedVersion = v.getVersionFromTopLevelFields(schemaDoc)
	}
	if detectedVersion == "" {
		detectedVersion = v.getVersionFromInfoBlock(schemaDoc)
	}
	if detectedVersion == "" {
		detectedVersion = v.getVersionFromMCPHeuristics(schemaDoc) // Check MCP specific patterns.
	}

	// Update internal state if a version was found.
	if detectedVersion != "" {
		logger.Debug("Detected schema version.", "version", detectedVersion)
		v.schemaVersion = detectedVersion
	} else {
		logger.Warn("Could not detect schema version from content.")
		v.schemaVersion = "" // Ensure it's empty if none found.
	}
}

// getVersionFromSchemaField checks the "$schema" field for draft versions.
func (v *Validator) getVersionFromSchemaField(schemaDoc map[string]interface{}) string {
	if schemaField, ok := schemaDoc["$schema"].(string); ok {
		v.logger.Debug("Checking $schema field for version hint.", "schemaValue", schemaField)
		// Simple check for known draft identifiers.
		if strings.Contains(schemaField, "draft-2020-12") || strings.Contains(schemaField, "draft/2020-12") {
			return "draft-2020-12"
		}
		if strings.Contains(schemaField, "draft-07") {
			return "draft-07"
		}
	}
	return ""
}

// getVersionFromTopLevelFields checks for a top-level "version" field.
func (v *Validator) getVersionFromTopLevelFields(schemaDoc map[string]interface{}) string {
	if versionField, ok := schemaDoc["version"].(string); ok && versionField != "" {
		v.logger.Debug("Found version in top-level 'version' field.", "version", versionField)
		return versionField
	}
	return ""
}

// getVersionFromInfoBlock checks for an "info.version" field (common in OpenAPI).
func (v *Validator) getVersionFromInfoBlock(schemaDoc map[string]interface{}) string {
	if infoBlock, ok := schemaDoc["info"].(map[string]interface{}); ok {
		if versionField, ok := infoBlock["version"].(string); ok && versionField != "" {
			v.logger.Debug("Found version in 'info.version' field.", "version", versionField)
			return versionField
		}
	}
	return ""
}

// getVersionFromMCPHeuristics checks for MCP-specific date patterns in "$id" or "title".
// Assumes MCP schema versions might be indicated by<y_bin_46>-MM-DD dates.
func (v *Validator) getVersionFromMCPHeuristics(schemaDoc map[string]interface{}) string {
	// Regex to find<y_bin_46>-MM-DD pattern.
	idRegex := regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`)

	// Check $id field.
	if id, ok := schemaDoc["$id"].(string); ok && strings.Contains(id, "modelcontextprotocol") {
		v.logger.Debug("Checking $id field for MCP version.", "idValue", id)
		if matches := idRegex.FindStringSubmatch(id); len(matches) > 1 {
			v.logger.Debug("Extracted potential version date from $id field.", "version", matches[1])
			return matches[1] // Return the matched date string.
		}
	}
	// Check title field.
	if title, ok := schemaDoc["title"].(string); ok && strings.Contains(strings.ToLower(title), "mcp") {
		v.logger.Debug("Checking title field for MCP version.", "titleValue", title)
		if matches := idRegex.FindStringSubmatch(title); len(matches) > 1 {
			v.logger.Debug("Extracted potential version date from title field.", "version", matches[1])
			return matches[1] // Return the matched date string.
		}
	}
	return "" // No heuristic match found.
}

// --- END OF validator.go ---
// Helper functions like calculatePreview, convertValidationError, etc.
// should NOT be included here if they exist in errors.go or helpers.go.
