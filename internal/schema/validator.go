// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP. // <<< FIX: Updated package comment
package schema

// file: internal/schema/validator.go

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp" // <<< Added import for regexp used by getVersionFromMCPHeuristics
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

//go:embed schema.json
var embeddedSchemaContent []byte

// ValidatorInterface defines the methods needed for schema validation.
type ValidatorInterface interface {
	Validate(ctx context.Context, messageType string, data []byte) error
	HasSchema(name string) bool
	IsInitialized() bool
	Initialize(ctx context.Context) error
	GetLoadDuration() time.Duration
	GetCompileDuration() time.Duration
	GetSchemaVersion() string
	Shutdown() error
}

// Validator handles loading, compiling, and validating against JSON schemas.
type Validator struct {
	schemaConfig        config.SchemaConfig
	compiler            *jsonschema.Compiler
	schemas             map[string]*jsonschema.Schema
	schemaDoc           map[string]interface{}
	mu                  sync.RWMutex
	httpClient          *http.Client
	initialized         bool
	logger              logging.Logger
	lastLoadDuration    time.Duration
	lastCompileDuration time.Duration
	schemaVersion       string
}

// Ensure Validator implements the interface.
var _ ValidatorInterface = (*Validator)(nil)

// NewValidator creates a new Validator instance with the given schema configuration.
func NewValidator(cfg config.SchemaConfig, logger logging.Logger) *Validator {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	compiler.AssertFormat = true
	compiler.AssertContent = true

	return &Validator{
		schemaConfig: cfg,
		compiler:     compiler,
		schemas:      make(map[string]*jsonschema.Schema),
		schemaDoc:    make(map[string]interface{}),
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		logger:       logger.WithField("component", "schema_validator"),
	}
}

// Initialize loads and compiles the MCP schema definitions.
// It prioritizes the SchemaOverrideURI if set. If loading the override fails
// because the file/URL is not found, it falls back to the embedded schema.
// Other override load errors are considered fatal.
func (v *Validator) Initialize(ctx context.Context) error {
	initStart := time.Now()
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.initialized {
		v.logger.Warn("Schema validator already initialized, skipping.")
		return nil
	}
	v.logger.Info("Initializing schema validator...")

	var schemaData []byte
	var sourceInfo string
	var loadWarning string
	useEmbedded := false // Flag to signal using embedded schema

	loadStart := time.Now()

	if v.schemaConfig.SchemaOverrideURI != "" {
		v.logger.Info("SchemaOverrideURI is set, attempting to load external schema.", "uri", v.schemaConfig.SchemaOverrideURI)
		loadedData, loadErr := loadSchemaFromURI(ctx, v.schemaConfig.SchemaOverrideURI, v.logger, v.httpClient) // Use temp var 'loadedData'

		if loadErr != nil {
			rootCause := errors.Cause(loadErr)
			var validationErr *ValidationError
			isNotFound := os.IsNotExist(rootCause) || (errors.As(loadErr, &validationErr) && validationErr.Code == ErrSchemaNotFound)

			if isNotFound {
				loadWarning = fmt.Sprintf("Schema override '%s' not found, falling back to embedded schema.", v.schemaConfig.SchemaOverrideURI)
				v.logger.Warn(loadWarning)
				useEmbedded = true // Set flag to use embedded
			} else {
				// Fatal error loading override
				v.logger.Error("CRITICAL: Failed to load schema from configured SchemaOverrideURI. Initialization aborted.",
					"uri", v.schemaConfig.SchemaOverrideURI, "error", fmt.Sprintf("%+v", loadErr))
				return errors.Wrapf(loadErr, "failed to load schema from configured override URI '%s'", v.schemaConfig.SchemaOverrideURI)
			}
		} else {
			// Override loaded successfully
			schemaData = loadedData // Assign successfully loaded data
			sourceInfo = fmt.Sprintf("override URI: %s", v.schemaConfig.SchemaOverrideURI)
			v.logger.Info("Successfully loaded schema from override URI.")
		}
	} else {
		// No override URI set, use embedded
		useEmbedded = true
	}

	// Load embedded if flagged
	if useEmbedded {
		v.logger.Info("Using embedded schema.")
		if len(embeddedSchemaContent) == 0 {
			err := errors.New("embedded schema content is empty and no valid override was loaded")
			v.logger.Error("CRITICAL: Embedded schema is empty. Initialization aborted.", "error", err)
			return NewValidationError(ErrSchemaLoadFailed, "Embedded schema content is empty", err)
		}
		schemaData = embeddedSchemaContent
		sourceInfo = "embedded"
	}

	v.lastLoadDuration = time.Since(loadStart)

	if len(schemaData) == 0 {
		err := errors.New("schema data is unexpectedly empty after load/fallback logic")
		v.logger.Error("Schema loading resulted in empty data. Initialization aborted.", "source", sourceInfo, "error", err)
		return NewValidationError(ErrSchemaLoadFailed, "Loaded schema data is empty", err)
	}

	v.logger.Info("Schema content prepared.",
		"duration", v.lastLoadDuration,
		"source", sourceInfo,
		"sizeBytes", len(schemaData))
	if loadWarning != "" {
		v.logger.Warn(loadWarning)
	}

	// --- Compile the loaded schemaData ---
	var parsedDoc map[string]interface{}
	if err := json.Unmarshal(schemaData, &parsedDoc); err != nil {
		v.logger.Error("Failed to parse loaded schema JSON.", "error", err, "source", sourceInfo)
		return NewValidationError(ErrSchemaLoadFailed, "Failed to parse schema JSON", errors.Wrap(err, "json.Unmarshal failed")).
			WithContext("source", sourceInfo)
	}

	v.extractSchemaVersion(schemaData)
	finalSchemaVersion := v.schemaVersion
	if finalSchemaVersion == "" {
		finalSchemaVersion = "[unknown]"
	}

	v.compiler = jsonschema.NewCompiler()
	v.compiler.Draft = jsonschema.Draft2020
	v.compiler.AssertFormat = true
	v.compiler.AssertContent = true

	addStart := time.Now()
	schemaReader := bytes.NewReader(schemaData)
	resourceID := "mcp://schema.json"
	if err := v.compiler.AddResource(resourceID, schemaReader); err != nil {
		v.logger.Error("Failed to add schema resource to compiler.",
			"duration", time.Since(addStart),
			"resourceID", resourceID,
			"source", sourceInfo,
			"error", err)
		return NewValidationError(
			ErrSchemaLoadFailed,
			"Failed to add schema resource",
			errors.Wrap(err, "compiler.AddResource failed"),
		).WithContext("source", sourceInfo).WithContext("schemaSize", len(schemaData))
	}
	v.logger.Info("Schema resource added.", "duration", time.Since(addStart), "resourceID", resourceID)

	compileStart := time.Now()
	compiledSchemas, compileErr := v.compileAllDefinitions(resourceID, parsedDoc)
	v.lastCompileDuration = time.Since(compileStart)

	if compileErr != nil {
		v.logger.Error("Schema compilation finished with errors. Initialization aborted.", "duration", v.lastCompileDuration, "firstError", compileErr)
		return compileErr
	}

	v.schemaDoc = parsedDoc
	v.schemas = compiledSchemas
	v.initialized = true
	initDuration := time.Since(initStart)

	v.logger.Info("✅ Schema validator initialized successfully.",
		"totalDuration", initDuration.String(),
		"loadDuration", v.lastLoadDuration.String(),
		"compileDuration", v.lastCompileDuration.String(),
		"schemaVersion", finalSchemaVersion,
		"schemasCompiled", len(v.schemas),
		"schemaSource", sourceInfo)

	return nil
}

// ... (rest of validator.go remains the same, including the version extraction methods)
// compileAllDefinitions(...), addGenericMappings(...), GetLoadDuration(...), etc.

// compileAllDefinitions compiles the base schema and all found definitions.
func (v *Validator) compileAllDefinitions(baseResourceID string, schemaDoc map[string]interface{}) (map[string]*jsonschema.Schema, error) {
	compiled := make(map[string]*jsonschema.Schema)
	var firstCompileError error

	// Compile base first.
	baseSchema, err := v.compiler.Compile(baseResourceID)
	if err != nil {
		v.logger.Error("CRITICAL: Failed to compile base schema resource.", "resourceID", baseResourceID, "error", err)
		return nil, NewValidationError(
			ErrSchemaCompileFailed,
			"Failed to compile base schema resource",
			errors.Wrap(err, "compiler.Compile failed for base schema"),
		).WithContext("pointer", baseResourceID)
	}
	compiled["base"] = baseSchema
	v.logger.Debug("Compiled base schema definition.", "name", "base")

	// Compile definitions using the passed-in schemaDoc.
	if defs, ok := schemaDoc["definitions"].(map[string]interface{}); ok {
		for name := range defs {
			pointer := baseResourceID + "#/definitions/" + name
			schemaCompiled, errCompile := v.compiler.Compile(pointer)
			if errCompile != nil {
				v.logger.Warn("Failed to compile schema definition.", "name", name, "pointer", pointer, "error", errCompile)
				if firstCompileError == nil {
					firstCompileError = NewValidationError(
						ErrSchemaCompileFailed,
						fmt.Sprintf("Failed to compile schema definition '%s'", name),
						errors.Wrap(errCompile, "compiler.Compile failed"),
					).WithContext("pointer", pointer)
				}
			} else {
				compiled[name] = schemaCompiled
				v.logger.Debug("Compiled schema definition.", "name", name)
			}
		}
	} else {
		v.logger.Warn("No 'definitions' section found in the schema JSON.")
	}

	// Add generic mappings after compiling definitions.
	v.addGenericMappings(compiled)

	return compiled, firstCompileError
}

// addGenericMappings creates mappings from generic type names to specific schema definitions.
func (v *Validator) addGenericMappings(compiledSchemas map[string]*jsonschema.Schema) {
	mappings := map[string][]string{
		"success_response":        {"JSONRPCResponse", "Response"},
		"error_response":          {"JSONRPCError", "Error"},
		"ping_notification":       {"PingRequest", "PingNotification", "JSONRPCNotification"},
		"notification":            {"JSONRPCNotification", "Notification"},
		"request":                 {"JSONRPCRequest", "Request"},
		"CallToolResult":          {"CallToolResult", "ToolResult"},
		"initialize_response":     {"InitializeResult"},
		"tools/list_response":     {"ListToolsResult"},
		"resources/list_response": {"ListResourcesResult"},
		"prompts/list_response":   {"ListPromptsResult"},
	}
	mapped := make([]string, 0)
	for genericName, potentialTargets := range mappings {
		if _, exists := compiledSchemas[genericName]; exists {
			continue
		}
		for _, targetDef := range potentialTargets {
			if targetSchema, ok := compiledSchemas[targetDef]; ok {
				compiledSchemas[genericName] = targetSchema
				mapped = append(mapped, fmt.Sprintf("%s->%s", genericName, targetDef))
				break
			}
		}
	}
	if len(mapped) > 0 {
		v.logger.Debug("Added generic schema mappings.", "mappings", mapped)
	}
}

// GetLoadDuration returns the duration of the last schema load operation.
func (v *Validator) GetLoadDuration() time.Duration {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.lastLoadDuration
}

// GetCompileDuration returns the duration of the last schema compile operation.
func (v *Validator) GetCompileDuration() time.Duration {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.lastCompileDuration
}

// Shutdown performs any cleanup needed for the schema validator.
func (v *Validator) Shutdown() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.initialized {
		return nil
	}
	v.logger.Info("Shutting down schema validator.")
	if transport, ok := v.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	} else if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		dt.CloseIdleConnections()
	}
	v.schemas = nil
	v.schemaDoc = nil
	v.initialized = false
	v.logger.Info("Schema validator shut down.")
	return nil
}

// IsInitialized returns whether the validator has been initialized.
func (v *Validator) IsInitialized() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.initialized
}

// Validate validates the given JSON data against the schema for the specified message type.
func (v *Validator) Validate(_ context.Context, messageType string, data []byte) error {
	if !v.IsInitialized() {
		return NewValidationError(ErrSchemaNotFound, "Schema validator not initialized", nil)
	}
	var instance interface{}
	// <<< FIX: Check unmarshal error before using instance >>>
	if err := json.Unmarshal(data, &instance); err != nil {
		return NewValidationError(
			ErrInvalidJSONFormat,
			"Invalid JSON format",
			errors.Wrap(err, "json.Unmarshal failed"),
		).WithContext("messageType", messageType).
			WithContext("dataPreview", calculatePreview(data)) // Assuming calculatePreview is in helpers.go.
	}

	schemaToUse, schemaUsedKey, ok := v.getSchemaForMessageType(messageType)
	if !ok {
		v.mu.RLock()
		availableKeys := getSchemaKeys(v.schemas)
		v.mu.RUnlock()
		return NewValidationError(
			ErrSchemaNotFound,
			fmt.Sprintf("Schema definition not found for message type '%s' or standard fallbacks.", messageType),
			nil,
		).WithContext("messageType", messageType).
			WithContext("availableSchemas", availableKeys)
	}

	validationStart := time.Now()
	// <<< FIX: Pass the successfully unmarshalled instance >>>
	err := schemaToUse.Validate(instance)
	validationDuration := time.Since(validationStart)

	if err != nil {
		var valErr *jsonschema.ValidationError
		if errors.As(err, &valErr) {
			v.logger.Debug("Schema validation failed.",
				"duration", validationDuration,
				"messageType", messageType,
				"schemaUsed", schemaUsedKey,
				"error", valErr.Message)
			// Pass necessary info to convertValidationError.
			return convertValidationError(valErr, messageType, data) // Assumes convertValidationError is in errors.go.
		}
		v.logger.Error("Unexpected error during schema.Validate.",
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

	v.logger.Debug("Schema validation successful.",
		"duration", validationDuration,
		"messageType", messageType,
		"schemaUsed", schemaUsedKey)
	return nil
}

// getSchemaForMessageType finds the appropriate compiled schema based on the message type.
func (v *Validator) getSchemaForMessageType(messageType string) (*jsonschema.Schema, string, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	// 1. Exact Match.
	if schema, ok := v.schemas[messageType]; ok {
		return schema, messageType, true
	}

	// 2. Fallback Logic.
	var fallbackKey string
	if strings.HasSuffix(messageType, "_notification") || strings.HasPrefix(messageType, "notifications/") {
		fallbackKey = "JSONRPCNotification"
	} else if strings.Contains(messageType, "Response") || strings.Contains(messageType, "Result") || strings.HasSuffix(messageType, "_response") || strings.HasSuffix(messageType, "_result") {
		if strings.Contains(messageType, "Error") || strings.HasSuffix(messageType, "_error") {
			if _, errorExists := v.schemas["JSONRPCError"]; errorExists {
				fallbackKey = "JSONRPCError"
			} else if _, responseExists := v.schemas["JSONRPCResponse"]; responseExists {
				fallbackKey = "JSONRPCResponse"
			} else {
				fallbackKey = "base"
			}
		} else {
			if _, responseExists := v.schemas["JSONRPCResponse"]; responseExists {
				fallbackKey = "JSONRPCResponse"
			} else {
				fallbackKey = "base"
			}
		}
	} else {
		fallbackKey = "JSONRPCRequest"
	}

	if schema, ok := v.schemas[fallbackKey]; ok {
		v.logger.Debug("Using fallback schema.", "originalType", messageType, "fallbackKey", fallbackKey)
		return schema, fallbackKey, true
	}

	// 3. Ultimate Fallback to "base".
	if schema, ok := v.schemas["base"]; ok {
		v.logger.Warn("Using 'base' schema as final fallback.", "originalType", messageType, "initialFallbackAttempt", fallbackKey)
		return schema, "base", true
	}

	// 4. Schema not found even with fallbacks.
	v.logger.Error("Could not find schema definition.", "requestedType", messageType, "fallbackAttempted", fallbackKey)
	return nil, "", false
}

// HasSchema checks if a schema with the given name exists.
func (v *Validator) HasSchema(name string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.schemas == nil {
		return false
	}
	_, ok := v.schemas[name]
	return ok
}

// getSchemaKeys returns the keys of the schemas map for debugging purposes.
func getSchemaKeys(schemas map[string]*jsonschema.Schema) []string {
	if schemas == nil {
		return []string{}
	}
	keys := make([]string, 0, len(schemas))
	for k := range schemas {
		keys = append(keys, k)
	}
	return keys
}

// GetSchemaVersion returns the detected schema version, if available.
func (v *Validator) GetSchemaVersion() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.schemaVersion
}

// extractSchemaVersion attempts to extract version information from schema data.
// Assumes lock is held by caller if needed (when modifying v.schemaVersion).
func (v *Validator) extractSchemaVersion(data []byte) {
	var schemaDoc map[string]interface{}
	logger := v.logger
	if err := json.Unmarshal(data, &schemaDoc); err != nil {
		logger.Warn("Failed to unmarshal schema to extract version, version will be unknown.", "error", err)
		v.schemaVersion = "[unknown]"
		return
	}

	detectedVersion := v.getVersionFromSchemaField(schemaDoc)
	if detectedVersion == "" {
		detectedVersion = v.getVersionFromTopLevelFields(schemaDoc)
	}
	if detectedVersion == "" {
		detectedVersion = v.getVersionFromInfoBlock(schemaDoc)
	}
	if detectedVersion == "" {
		detectedVersion = v.getVersionFromMCPHeuristics(schemaDoc)
	}

	if detectedVersion != "" && detectedVersion != v.schemaVersion {
		logger.Debug("Detected schema version.", "version", detectedVersion)
		v.schemaVersion = detectedVersion
	} else if detectedVersion == "" && v.schemaVersion == "" {
		logger.Warn("Could not detect schema version from content.")
		v.schemaVersion = "[unknown]"
	}
}

// getVersionFromSchemaField extracts version from the $schema field.
func (v *Validator) getVersionFromSchemaField(schemaDoc map[string]interface{}) string {
	if schemaField, ok := schemaDoc["$schema"].(string); ok {
		v.logger.Debug("Checking $schema field for version.", "schemaValue", schemaField)
		if strings.Contains(schemaField, "draft-2020-12") || strings.Contains(schemaField, "draft/2020-12") {
			return "draft-2020-12"
		}
		if strings.Contains(schemaField, "draft-07") {
			return "draft-07"
		}
	}
	return ""
}

// getVersionFromTopLevelFields extracts version from top-level 'version' field.
func (v *Validator) getVersionFromTopLevelFields(schemaDoc map[string]interface{}) string {
	if versionField, ok := schemaDoc["version"].(string); ok {
		v.logger.Debug("Found version in top-level 'version' field.", "version", versionField)
		return versionField
	}
	return ""
}

// getVersionFromInfoBlock extracts version from 'info.version' field.
func (v *Validator) getVersionFromInfoBlock(schemaDoc map[string]interface{}) string {
	if infoBlock, ok := schemaDoc["info"].(map[string]interface{}); ok {
		if versionField, ok := infoBlock["version"].(string); ok {
			v.logger.Debug("Found version in 'info.version' field.", "version", versionField)
			return versionField
		}
	}
	return ""
}

// getVersionFromMCPHeuristics extracts version using MCP-specific patterns in $id or title.
func (v *Validator) getVersionFromMCPHeuristics(schemaDoc map[string]interface{}) string {
	idRegex := regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`) // <<< Ensure regexp is imported

	if id, ok := schemaDoc["$id"].(string); ok && strings.Contains(id, "modelcontextprotocol") {
		v.logger.Debug("Checking $id field for MCP version.", "idValue", id)
		if matches := idRegex.FindStringSubmatch(id); len(matches) > 1 {
			v.logger.Debug("Extracted version from $id field.", "version", matches[1])
			return matches[1]
		}
	}
	if title, ok := schemaDoc["title"].(string); ok && strings.Contains(strings.ToLower(title), "mcp") {
		v.logger.Debug("Checking title field for MCP version.", "titleValue", title)
		if matches := idRegex.FindStringSubmatch(title); len(matches) > 1 {
			v.logger.Debug("Extracted version from title field.", "version", matches[1])
			return matches[1]
		}
	}
	return ""
}
