// Package validation provides validation utilities used throughout the Cowgnition project.
// These utilities are designed to enforce data integrity and consistency by validating common data formats.
package validation

import (
	"fmt"
	"regexp"
)

// ValidateName checks if a name is valid.
// It ensures that the name adheres to specific character and length constraints.
// This validation is important for maintaining data quality and preventing errors caused by invalid names.
//
// name string: The name to validate.
//
// Returns:
// error: An error if the name is invalid, nil otherwise.
func ValidateName(name string) error {
	// Regular expression for valid names (letters, numbers, spaces, underscores, hyphens).
	nameRegex := regexp.MustCompile(`^[a-zA-Z0-9\s\-_]+$`)
	if !nameRegex.MatchString(name) {
		return fmt.Errorf("ValidateName: invalid name format: %s", name)
	}

	// Check the length of the name.
	if len(name) < 1 || len(name) > 255 {
		return fmt.Errorf("ValidateName: name length must be between 1 and 255 characters: %s", name)
	}

	return nil
}

// ValidateIdentifier checks if an identifier is valid.
// Identifiers are typically used for unique identification of resources or entities.
// This validation ensures that identifiers follow a specific format, which is crucial for data retrieval and manipulation.
//
// identifier string: The identifier to validate.
//
// Returns:
// error: An error if the identifier is invalid, nil otherwise.
func ValidateIdentifier(identifier string) error {
	// Regular expression for valid identifiers (letters, numbers, underscores, hyphens).
	identifierRegex := regexp.MustCompile(`^[a-zA-Z0-9\-_]+$`)
	if !identifierRegex.MatchString(identifier) {
		return fmt.Errorf("ValidateIdentifier: invalid identifier format: %s", identifier)
	}

	// Check the length of the identifier.
	if len(identifier) < 1 || len(identifier) > 100 {
		return fmt.Errorf("ValidateIdentifier: identifier length must be between 1 and 100 characters: %s", identifier)
	}

	return nil
}

// DocEnhanced:2025-03-21
