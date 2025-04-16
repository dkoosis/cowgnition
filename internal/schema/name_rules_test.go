// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
// File: internal/schema/name_rules_test.go
package schema

// file: internal/schema/name_rules_test.go

import (
	// Import regexp to verify the pattern if needed, though not strictly required for testing ValidateName directly.
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	// For reference, the pattern currently being tested from name_rules.go:
	// EntityTypeTool: Pattern: regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`), MaxLength: 64
	t.Parallel()
	testCases := []struct {
		name          string     // Name for the subtest
		entityType    EntityType // The type of entity
		inputName     string     // The name string to validate
		expectError   bool       // Whether we expect ValidateName to return an error
		errorContains string     // Optional: substring expected in the error message
	}{
		// --- Cases for EntityTypeTool (Based on ^[a-z][a-zA-Z0-9]*$ and MaxLength: 64) ---
		{
			name:        "[Tool] valid - all lowercase",
			entityType:  EntityTypeTool,
			inputName:   "gettasks",
			expectError: false,
		},
		{
			name:        "[Tool] valid - mixed case after first char",
			entityType:  EntityTypeTool,
			inputName:   "getTasksV2",
			expectError: false,
		},
		{
			name:        "[Tool] valid - with numbers",
			entityType:  EntityTypeTool,
			inputName:   "tool123",
			expectError: false,
		},
		{
			name:        "[Tool] valid - single char",
			entityType:  EntityTypeTool,
			inputName:   "a",
			expectError: false,
		},
		{
			name:        "[Tool] valid - exact max length (64)",
			entityType:  EntityTypeTool,
			inputName:   "a" + strings.Repeat("B", 63), // length 64
			expectError: false,
		},
		{
			name:          "[Tool] invalid - starts with uppercase",
			entityType:    EntityTypeTool,
			inputName:     "GetTasks",
			expectError:   true,
			errorContains: "Must start with lowercase letter",
		},
		{
			name:          "[Tool] invalid - contains hyphen",
			entityType:    EntityTypeTool,
			inputName:     "get-tasks",
			expectError:   true,
			errorContains: "alphanumeric characters only",
		},
		{
			name:          "[Tool] invalid - contains underscore",
			entityType:    EntityTypeTool,
			inputName:     "get_tasks",
			expectError:   true,
			errorContains: "alphanumeric characters only",
		},
		{
			name:          "[Tool] invalid - contains slash",
			entityType:    EntityTypeTool,
			inputName:     "xyz/tool",
			expectError:   true,
			errorContains: "alphanumeric characters only",
		},
		{
			name:          "[Tool] invalid - contains space",
			entityType:    EntityTypeTool,
			inputName:     "get tasks",
			expectError:   true,
			errorContains: "alphanumeric characters only",
		},
		{
			name:          "[Tool] invalid - contains period",
			entityType:    EntityTypeTool,
			inputName:     "get.tasks",
			expectError:   true,
			errorContains: "alphanumeric characters only",
		},
		{
			name:          "[Tool] invalid - contains special char",
			entityType:    EntityTypeTool,
			inputName:     "tasks!",
			expectError:   true,
			errorContains: "alphanumeric characters only",
		},
		{
			name:          "[Tool] invalid - starts with number",
			entityType:    EntityTypeTool,
			inputName:     "1tool",
			expectError:   true,
			errorContains: "Must start with lowercase letter",
		},
		{
			name:          "[Tool] invalid - empty string",
			entityType:    EntityTypeTool,
			inputName:     "",
			expectError:   true,
			errorContains: "empty tool name",
		},
		{
			name:          "[Tool] invalid - too long (65)",
			entityType:    EntityTypeTool,
			inputName:     "a" + strings.Repeat("B", 64), // length 65
			expectError:   true,
			errorContains: "exceeds maximum length",
		},

		// --- Cases for Unknown Entity Type ---
		{
			name:          "invalid entity type",
			entityType:    EntityType("unknown"), // Use a made-up type
			inputName:     "someName",
			expectError:   true,
			errorContains: "unknown entity type",
		},

		// --- Placeholder: Add cases for EntityTypeResource and EntityTypePrompt ---
		// (They currently use the same pattern as EntityTypeTool, so tests would be similar)
		{
			name:        "[Resource] valid name",
			entityType:  EntityTypeResource,
			inputName:   "myResource1",
			expectError: false,
		},
		{
			name:          "[Resource] invalid - contains underscore",
			entityType:    EntityTypeResource,
			inputName:     "my_resource",
			expectError:   true,
			errorContains: "alphanumeric characters only",
		},
		{
			name:        "[Prompt] valid name",
			entityType:  EntityTypePrompt,
			inputName:   "promptForTask",
			expectError: false,
		},
		{
			name:          "[Prompt] invalid - starts uppercase",
			entityType:    EntityTypePrompt,
			inputName:     "Prompt1",
			expectError:   true,
			errorContains: "Must start with lowercase letter",
		},
	}

	// Loop through each test case
	for _, tc := range testCases {
		// Use t.Run() to create a subtest for each case. This gives clearer output.
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel() // Indicate that this test case can run in parallel with others

			// Call the function we want to test
			err := ValidateName(tc.entityType, tc.inputName)

			// Check if an error occurred when we didn't expect one
			if !tc.expectError && err != nil {
				t.Errorf("ValidateName(%q, %q) returned unexpected error: %v", tc.entityType, tc.inputName, err)
			}

			// Check if an error did *not* occur when we expected one
			if tc.expectError && err == nil {
				t.Errorf("ValidateName(%q, %q) expected an error, but got none", tc.entityType, tc.inputName)
			}

			// Optionally, check if the error message contains a specific substring
			if tc.expectError && err != nil && tc.errorContains != "" {
				if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("ValidateName(%q, %q) error %q does not contain expected substring %q",
						tc.entityType, tc.inputName, err.Error(), tc.errorContains)
				}
			}
		})
	}
}

// Optional: Add a test specifically for the regex pattern itself if desired.
func TestToolNameRegex(t *testing.T) {
	rule, ok := GetNameRule(EntityTypeTool)
	if !ok {
		t.Fatal("Could not get rule for EntityTypeTool")
	}
	expectedPattern := `^[a-z][a-zA-Z0-9]*$`
	if rule.Pattern.String() != expectedPattern {
		t.Errorf("Expected pattern %q, but got %q", expectedPattern, rule.Pattern.String())
	}
	// You could add more specific regex tests here too
}
