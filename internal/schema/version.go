// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
package schema

// file: internal/schema/version.go

import (
	"encoding/json"
	"regexp"
	"strings"
)

// extractSchemaVersion attempts to extract version information from schema data.
// Assumes lock is held by caller if needed (when modifying v.schemaVersion).
// Refactored for lower cyclomatic complexity.
func (v *SchemaValidator) extractSchemaVersion(data []byte) {
	var schemaDoc map[string]interface{}
	if err := json.Unmarshal(data, &schemaDoc); err != nil {
		v.logger.Warn("Failed to unmarshal schema to extract version.", "error", err)
		return // Silently fail, this is just for metadata
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

	// Update the validator state if a new, non-empty version was detected
	if detectedVersion != "" && detectedVersion != v.schemaVersion {
		v.logger.Info("Detected schema version.", "version", detectedVersion)
		v.schemaVersion = detectedVersion // Assumes lock is held by caller
	} else if detectedVersion == "" && v.schemaVersion == "" {
		v.logger.Debug("Could not detect specific version information in the schema.")
	}
}

// --- Helper functions for extractSchemaVersion ---

func (v *SchemaValidator) getVersionFromSchemaField(schemaDoc map[string]interface{}) string {
	if schemaField, ok := schemaDoc["$schema"].(string); ok {
		if strings.Contains(schemaField, "draft-2020-12") || strings.Contains(schemaField, "draft/2020-12") {
			return "draft-2020-12"
		}
		if strings.Contains(schemaField, "draft-07") {
			return "draft-07"
		}
		// Add more draft checks if needed
	}
	return ""
}

func (v *SchemaValidator) getVersionFromTopLevelFields(schemaDoc map[string]interface{}) string {
	if versionField, ok := schemaDoc["version"].(string); ok {
		return versionField
	}
	return ""
}

func (v *SchemaValidator) getVersionFromInfoBlock(schemaDoc map[string]interface{}) string {
	if infoBlock, ok := schemaDoc["info"].(map[string]interface{}); ok {
		if versionField, ok := infoBlock["version"].(string); ok {
			return versionField
		}
	}
	return ""
}

func (v *SchemaValidator) getVersionFromMCPHeuristics(schemaDoc map[string]interface{}) string {
	idRegex := regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`) // Basic<x_bin_880>-MM-DD pattern

	if id, ok := schemaDoc["$id"].(string); ok && strings.Contains(id, "modelcontextprotocol") {
		if matches := idRegex.FindStringSubmatch(id); len(matches) > 1 {
			return matches[1] // Prefer version from $id
		}
	}

	if title, ok := schemaDoc["title"].(string); ok && strings.Contains(strings.ToLower(title), "mcp") {
		if matches := idRegex.FindStringSubmatch(title); len(matches) > 1 {
			return matches[1] // Fallback to version from title
		}
	}

	return ""
}

// GetSchemaVersion returns the detected schema version, if available.
func (v *SchemaValidator) GetSchemaVersion() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.schemaVersion
}
