// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
package schema

import (
	"encoding/json"
	"regexp"
	"strings"
	// Corrected: logging import might be needed if extractSchemaVersion uses v.logger directly.
	// "github.com/dkoosis/cowgnition/internal/logging".
)

// extractSchemaVersion attempts to extract version information from schema data.
// Assumes lock is held by caller if needed (when modifying v.schemaVersion).
// Refactored for lower cyclomatic complexity.
// Corrected: Method receiver changed to *Validator.
func (v *Validator) extractSchemaVersion(data []byte) {
	var schemaDoc map[string]interface{}
	// Use a local logger instance or pass one if needed for warnings.
	// logger := v.logger // Assuming v has logger field accessible.
	if err := json.Unmarshal(data, &schemaDoc); err != nil {
		// logger.Warn("Failed to unmarshal schema to extract version.", "error", err).
		return // Silently fail, this is just for metadata.
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

	// Update the validator state if a new, non-empty version was detected.
	// Assumes lock is held by caller (Initialize).
	if detectedVersion != "" && detectedVersion != v.schemaVersion {
		// logger.Info("Detected schema version.", "version", detectedVersion).
		v.schemaVersion = detectedVersion
	}
}

// --- Helper functions for extractSchemaVersion ---.

// Corrected: Method receiver changed to *Validator.
func (v *Validator) getVersionFromSchemaField(schemaDoc map[string]interface{}) string {
	if schemaField, ok := schemaDoc["$schema"].(string); ok {
		if strings.Contains(schemaField, "draft-2020-12") || strings.Contains(schemaField, "draft/2020-12") {
			return "draft-2020-12"
		}
		if strings.Contains(schemaField, "draft-07") {
			return "draft-07"
		}
		// Add more draft checks if needed.
	}
	return ""
}

// Corrected: Method receiver changed to *Validator.
func (v *Validator) getVersionFromTopLevelFields(schemaDoc map[string]interface{}) string {
	if versionField, ok := schemaDoc["version"].(string); ok {
		return versionField
	}
	return ""
}

// Corrected: Method receiver changed to *Validator.
func (v *Validator) getVersionFromInfoBlock(schemaDoc map[string]interface{}) string {
	if infoBlock, ok := schemaDoc["info"].(map[string]interface{}); ok {
		if versionField, ok := infoBlock["version"].(string); ok {
			return versionField
		}
	}
	return ""
}

// Corrected: Method receiver changed to *Validator.
func (v *Validator) getVersionFromMCPHeuristics(schemaDoc map[string]interface{}) string {
	idRegex := regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`) // Basic YYYY-MM-DD pattern.

	if id, ok := schemaDoc["$id"].(string); ok && strings.Contains(id, "modelcontextprotocol") {
		if matches := idRegex.FindStringSubmatch(id); len(matches) > 1 {
			return matches[1] // Prefer version from $id.
		}
	}

	if title, ok := schemaDoc["title"].(string); ok && strings.Contains(strings.ToLower(title), "mcp") {
		if matches := idRegex.FindStringSubmatch(title); len(matches) > 1 {
			return matches[1] // Fallback to version from title.
		}
	}

	return ""
}

// GetSchemaVersion method is defined in validator.go.
