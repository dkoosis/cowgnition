// file: internal/mcp/schema/name_rules.go

package schema

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
)

// EntityType represents a type of MCP entity that needs name validation.
type EntityType string

const (
	// EntityTypeTool represents a tool entity in MCP.
	EntityTypeTool EntityType = "tool"

	// EntityTypeResource represents a resource entity in MCP.
	EntityTypeResource EntityType = "resource"

	// EntityTypePrompt represents a prompt entity in MCP.
	EntityTypePrompt EntityType = "prompt"
)

// NameRule defines validation rules for an entity name.
type NameRule struct {
	// Pattern is the regex pattern the name must match.
	Pattern *regexp.Regexp

	// Description is a human-readable description of the pattern.
	Description string

	// MaxLength is the maximum allowed length of the name.
	MaxLength int

	// ExampleValid contains examples of valid names.
	ExampleValid []string

	// ExampleInvalid contains examples of invalid names with reasons.
	ExampleInvalid map[string]string
}

// nameRules maps entity types to their validation rules.
var nameRules = map[EntityType]NameRule{
	EntityTypeTool: {
		// Must start with lowercase letter, followed by alphanumeric chars.
		Pattern:     regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`),
		Description: "Must start with lowercase letter, followed by alphanumeric characters only",
		MaxLength:   64, // Observed limit based on testing with Claude Desktop.
		ExampleValid: []string{
			"getTasks",
			"createTask",
			"completeTask",
			"searchByTag",
		},
		ExampleInvalid: map[string]string{
			"GetTasks":  "Starts with uppercase letter",
			"get-tasks": "Contains hyphen",
			"get_tasks": "Contains underscore",
			"get.tasks": "Contains period",
			"get tasks": "Contains space",
			"1getTasks": "Starts with number",
			"getTasks!": "Contains special character",
			"":          "Empty string",
		},
	},
	EntityTypeResource: {
		// Same pattern as tools for now - can be adjusted based on research
		Pattern:     regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`),
		Description: "Must start with lowercase letter, followed by alphanumeric characters only",
		MaxLength:   64,
		ExampleValid: []string{
			"taskList",
			"userProfile",
			"tagCollection",
		},
		ExampleInvalid: map[string]string{
			"Task-List":  "Contains hyphen and starts with uppercase",
			"resource_1": "Contains underscore",
			"*resource":  "Starts with special character",
		},
	},
	EntityTypePrompt: {
		// Same pattern as tools for now - can be adjusted based on research.
		Pattern:     regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`),
		Description: "Must start with lowercase letter, followed by alphanumeric characters only",
		MaxLength:   64,
		ExampleValid: []string{
			"taskCreation",
			"welcomeMessage",
			"helpGuide",
		},
		ExampleInvalid: map[string]string{
			"Prompt-1":     "Starts with uppercase and contains hyphen",
			"prompt_guide": "Contains underscore",
			"prompt guide": "Contains space",
		},
	},
}

// GetNameRule returns the validation rule for a specific entity type.
func GetNameRule(entityType EntityType) (NameRule, bool) {
	rule, ok := nameRules[entityType]
	return rule, ok
}

// ValidateName validates a name against the rules for a specific entity type.
func ValidateName(entityType EntityType, name string) error {
	rule, ok := nameRules[entityType]
	if !ok {
		return errors.Newf("unknown entity type: %s", entityType)
	}

	// Check length.
	if len(name) == 0 {
		return errors.Newf("empty %s name is not allowed", entityType)
	}

	if len(name) > rule.MaxLength {
		return errors.Newf("%s name exceeds maximum length of %d characters", entityType, rule.MaxLength)
	}

	// Check pattern.
	if !rule.Pattern.MatchString(name) {
		return errors.Newf("invalid %s name '%s': %s", entityType, name, rule.Description)
	}

	return nil
}

// GetNamePatternDescription returns a human-readable description of the naming pattern
// for a specific entity type. This is useful for error messages and documentation.
func GetNamePatternDescription(entityType EntityType) string {
	rule, ok := nameRules[entityType]
	if !ok {
		return fmt.Sprintf("No pattern defined for %s", entityType)
	}

	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Rules for %s names:\n", entityType))
	builder.WriteString(fmt.Sprintf("- %s\n", rule.Description))
	builder.WriteString(fmt.Sprintf("- Maximum length: %d characters\n", rule.MaxLength))

	if len(rule.ExampleValid) > 0 {
		builder.WriteString("- Valid examples: ")
		for i, ex := range rule.ExampleValid {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(fmt.Sprintf("\"%s\"", ex))
		}
		builder.WriteString("\n")
	}

	if len(rule.ExampleInvalid) > 0 {
		builder.WriteString("- Invalid examples:\n")
		for ex, reason := range rule.ExampleInvalid {
			builder.WriteString(fmt.Sprintf("  - \"%s\": %s\n", ex, reason))
		}
	}

	return builder.String()
}

// DumpAllRules returns a string containing all validation rules.
// This is useful for documentation and debugging purposes.
func DumpAllRules() string {
	var builder strings.Builder

	builder.WriteString("MCP Entity Name Validation Rules\n")
	builder.WriteString("===============================\n\n")

	for entityType := range nameRules {
		builder.WriteString(GetNamePatternDescription(entityType))
		builder.WriteString("\n")
	}

	builder.WriteString("\nNOTE: These rules are based on observed behavior and testing with Claude Desktop.\n")
	builder.WriteString("They may be stricter than what's explicitly documented in the MCP specification.\n")

	return builder.String()
}
