// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
// file: internal/schema/loader.go
//
// Contains logic to load schema content from a specific URI (file or http/s).
// This is used only when SchemaOverrideURI is configured.
package schema

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	// Keep time import.
	"github.com/cockroachdb/errors"
	// Ensure config is NOT imported here, logger is passed in.
	"github.com/dkoosis/cowgnition/internal/logging"
)

// loadSchemaFromURI attempts to load schema data from a given URI,
// which can be a file path (file://...) or an HTTP(S) URL.
// This is now only called when an override URI is explicitly configured.
func loadSchemaFromURI(ctx context.Context, uri string, logger logging.Logger, httpClient *http.Client) ([]byte, error) {
	parsedURI, err := url.Parse(uri)
	if err != nil {
		// Provide context about the specific URI that failed.
		return nil, errors.Wrapf(err, "invalid schema override URI: %s", uri)
	}

	logger.Info("Loading schema from explicitly configured URI.", "uri", uri, "scheme", parsedURI.Scheme) // Added period.

	switch parsedURI.Scheme {
	case "file":
		// Convert file URI path to OS-specific path.
		// Handle potential leading slash on Windows paths.
		filePath := parsedURI.Path
		if os.PathSeparator == '\\' && strings.HasPrefix(filePath, "/") {
			// On Windows, url.Parse keeps the leading /, remove it for local paths.
			// Example: file:///C:/path -> /C:/path, needs to be C:/path.
			// Check if it looks like a windows path after the slash (e.g., /C:/...).
			if len(filePath) > 2 && filePath[2] == ':' {
				filePath = filePath[1:]
			}
		}
		// Ensure it's an absolute path for clarity in logs/errors.
		filePath, absErr := filepath.Abs(filePath)
		if absErr != nil {
			logger.Warn("Could not determine absolute path for schema file URI.", "uriPath", parsedURI.Path, "error", absErr) // Added period.
			// Use the original path for the read attempt anyway.
			filePath = parsedURI.Path
			if os.PathSeparator == '\\' && strings.HasPrefix(filePath, "/") {
				if len(filePath) > 2 && filePath[2] == ':' {
					filePath = filePath[1:]
				}
			}
		}

		logger.Debug("Reading schema file.", "path", filePath) // Added period.
		// Suppress G304 warning because the URI comes from trusted configuration.
		// #nosec G304
		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.Error("Failed to read schema file from override URI.", "path", filePath, "error", err) // Added period.
			code := ErrSchemaLoadFailed
			if os.IsNotExist(err) {
				code = ErrSchemaNotFound
			}
			// Create a structured error using the defined type.
			return nil, NewValidationError(
				code,
				fmt.Sprintf("Failed to read schema override file: %s", filePath),
				err, // Keep original error as cause.
			).WithContext("uri", uri)
		}
		logger.Debug("Schema file read successfully.", "path", filePath, "size_bytes", len(data)) // Added period.
		return data, nil

	case "http", "https":
		logger.Debug("Fetching schema from URL.", "url", uri) // Added period.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
		if err != nil {
			logger.Error("Failed to create HTTP request for schema override URL.", "url", uri, "error", err) // Added period.
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				"Failed to create HTTP request for schema override URL",
				errors.Wrap(err, "http.NewRequestWithContext failed"),
			).WithContext("url", uri)
		}
		// Set standard headers for fetching schema.
		req.Header.Set("Accept", "application/json, application/schema+json, */*")
		req.Header.Set("User-Agent", "CowGnition-Schema-Loader/0.1.0 (schema override fetch)") // Use a descriptive User-Agent.

		// Use the provided httpClient which includes timeout.
		resp, err := httpClient.Do(req)
		if err != nil {
			logger.Error("Network error fetching schema override.", "url", uri, "error", err) // Added period.
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				"Failed to fetch schema from override URL",
				errors.Wrap(err, "httpClient.Do failed"),
			).WithContext("url", uri)
		}
		// Ensure body is closed properly.
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				// Log error during body close.
				logger.Warn("Error closing schema override response body.", "url", uri, "error", closeErr) // Added period.
			}
		}()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)  // Read body for logging context.
			preview := calculatePreview(bodyBytes) // Assumes calculatePreview is in helpers.go.
			code := ErrSchemaLoadFailed
			if resp.StatusCode == http.StatusNotFound {
				code = ErrSchemaNotFound
			}
			logger.Error("Schema override URL returned error status.",
				"url", uri,
				"status", resp.Status,
				"statusCode", resp.StatusCode,
				"responseBodyPreview", preview) // Added period.
			return nil, NewValidationError(
				code,
				fmt.Sprintf("Failed to fetch schema override: HTTP status %d", resp.StatusCode),
				nil, // No Go error cause for HTTP status failure itself.
			).WithContext("url", uri).
				WithContext("statusCode", resp.StatusCode).
				WithContext("responseBodyPreview", preview)
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Error("Failed to read response body from schema override URL.", "url", uri, "error", err) // Added period.
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				"Failed to read schema override from HTTP response",
				errors.Wrap(err, "io.ReadAll failed"),
			).WithContext("url", uri)
		}
		logger.Debug("Successfully downloaded schema override.", "url", uri, "size_bytes", len(data)) // Added period.
		return data, nil

	default:
		logger.Error("Unsupported schema override URI scheme.", "uri", uri, "scheme", parsedURI.Scheme) // Added period.
		return nil, NewValidationError(
			ErrSchemaLoadFailed,
			fmt.Sprintf("Unsupported schema override URI scheme: %s", parsedURI.Scheme),
			nil,
		).WithContext("uri", uri)
	}
}

// Note: Removed loadSchemaFromMultipleSources and getStandardSchemaLocations as they are no longer used.
// Ensure calculatePreview exists in helpers.go.
