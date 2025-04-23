// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
// file: internal/schema/version.go
package schema

import (
	"encoding/json"
	"regexp"
	"strings"
	// Corrected: logging import is needed if extractSchemaVersion uses v.logger directly.
	// "github.com/dkoosis/cowgnition/internal/logging".
)

// extractSchemaVersion attempts to extract version information from schema data.
// Assumes lock is held by caller if needed (when modifying v.schemaVersion).
// Refactored for lower cyclomatic complexity.
// Corrected: Method receiver changed to *Validator.
func (v *Validator) extractSchemaVersion(data []byte) {
	var schemaDoc map[string]interface{}
	// Use the validator's logger instance.
	logger := v.logger
	if err := json.Unmarshal(data, &schemaDoc); err != nil {
		logger.Warn("Failed to unmarshal schema to extract version, version will be unknown.", "error", err) // Added period.
		v.schemaVersion = "[unknown]"                                                                        // Set explicitly if parsing fails.
		return                                                                                               // Silently fail, this is just for metadata.
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
		// Use Debug level for this internal detail.
		logger.Debug("Detected schema version.", "version", detectedVersion) // Simplified log.
		v.schemaVersion = detectedVersion
	} else if detectedVersion == "" && v.schemaVersion == "" {
		// If no version could be detected at all, set to unknown.
		logger.Warn("Could not detect schema version from content.") // Added period.
		v.schemaVersion = "[unknown]"
	}
}

// --- Helper functions for extractSchemaVersion ---.

// getVersionFromSchemaField extracts version from the $schema field.
// Corrected: Method receiver changed to *Validator.
func (v *Validator) getVersionFromSchemaField(schemaDoc map[string]interface{}) string {
	if schemaField, ok := schemaDoc["$schema"].(string); ok {
		// Log the check.
		v.logger.Debug("Checking $schema field for version.", "schemaValue", schemaField) // Added period.
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

// getVersionFromTopLevelFields extracts version from top-level 'version' field.
// Corrected: Method receiver changed to *Validator.
func (v *Validator) getVersionFromTopLevelFields(schemaDoc map[string]interface{}) string {
	if versionField, ok := schemaDoc["version"].(string); ok {
		v.logger.Debug("Found version in top-level 'version' field.", "version", versionField) // Added period.
		return versionField
	}
	return ""
}

// getVersionFromInfoBlock extracts version from 'info.version' field.
// Corrected: Method receiver changed to *Validator.
func (v *Validator) getVersionFromInfoBlock(schemaDoc map[string]interface{}) string {
	if infoBlock, ok := schemaDoc["info"].(map[string]interface{}); ok {
		if versionField, ok := infoBlock["version"].(string); ok {
			v.logger.Debug("Found version in 'info.version' field.", "version", versionField) // Added period.
			return versionField
		}
	}
	return ""
}

// getVersionFromMCPHeuristics extracts version using MCP-specific patterns in $id or title.
// Corrected: Method receiver changed to *Validator.
func (v *Validator) getVersionFromMCPHeuristics(schemaDoc map[string]interface{}) string {
	// Basic YYYY-MM-DD pattern.
	idRegex := regexp.MustCompile(`(\d{4}-\d{2}-\d{2})`)

	if id, ok := schemaDoc["$id"].(string); ok && strings.Contains(id, "modelcontextprotocol") {
		v.logger.Debug("Checking $id field for MCP version.", "idValue", id) // Added period.
		if matches := idRegex.FindStringSubmatch(id); len(matches) > 1 {
			v.logger.Debug("Extracted version from $id field.", "version", matches[1]) // Added period.
			return matches[1]                                                          // Prefer version from $id.
		}
	}

	if title, ok := schemaDoc["title"].(string); ok && strings.Contains(strings.ToLower(title), "mcp") {
		v.logger.Debug("Checking title field for MCP version.", "titleValue", title) // Added period.
		if matches := idRegex.FindStringSubmatch(title); len(matches) > 1 {
			v.logger.Debug("Extracted version from title field.", "version", matches[1]) // Added period.
			return matches[1]                                                            // Fallback to version from title.
		}
	}

	return ""
}

// GetSchemaVersion method is defined in validator.go.
