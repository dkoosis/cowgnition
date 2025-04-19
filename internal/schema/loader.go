// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
// file: internal/schema/loader.go
//
// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
//
// Schema loading logic:
// 1. The system first checks if a schema override URI is specified in the configuration
// 2. If an override URI is provided:
//   - For file:// URIs: The schema is loaded from the local filesystem
//   - For http(s):// URIs: The schema is fetched from the remote server
//   - Error handling accounts for file not found, HTTP errors, and invalid schema content
//
// 3. If no override is provided or loading fails (depending on configuration):
//   - Falls back to the embedded schema content in validator.go
//
// 4. After loading, the schema is parsed, validated, and compiled for use
//
// This file specifically handles the URI loading logic, supporting both file system
// and HTTP(S) loading with appropriate error context and platform-specific path handling.
package schema

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	// Import time for http client timeout.
	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
)

// loadSchemaFromURI attempts to load schema data from a given URI,
// which can be a file path (file://...) or an HTTP(S) URL.
func loadSchemaFromURI(ctx context.Context, uri string, logger logging.Logger, httpClient *http.Client) ([]byte, error) {
	parsedURI, err := url.Parse(uri)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid schema override URI: %s", uri)
	}

	logger.Info("Loading schema from URI.", "uri", uri, "scheme", parsedURI.Scheme)

	switch parsedURI.Scheme {
	case "file":
		// Convert file URI path to OS-specific path.
		// Handle potential leading slash on Windows paths.
		filePath := parsedURI.Path
		if os.PathSeparator == '\\' && strings.HasPrefix(filePath, "/") {
			filePath = strings.TrimPrefix(filePath, "/")
		}
		logger.Debug("Reading schema file.", "path", filePath)
		data, err := os.ReadFile(filePath) // #nosec G304 -- URI comes from config/flag.
		if err != nil {
			logger.Error("Failed to read schema file.", "path", filePath, "error", err)
			return nil, NewValidationError(
				ErrSchemaNotFound, // Use specific error code.
				"Failed to read schema file from override URI",
				errors.Wrapf(err, "failed to read schema file: %s", filePath),
			).WithContext("uri", uri)
		}
		logger.Debug("Schema file read successfully.", "path", filePath, "size_bytes", len(data))
		return data, nil

	case "http", "https":
		logger.Debug("Fetching schema from URL.", "url", uri)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
		if err != nil {
			logger.Error("Failed to create HTTP request for schema URL.", "url", uri, "error", err)
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				"Failed to create HTTP request for schema override URL",
				errors.Wrap(err, "http.NewRequestWithContext failed"),
			).WithContext("url", uri)
		}
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("User-Agent", "CowGnition-Schema-Loader/0.1.0 (schema override)")

		// Use the provided httpClient which has a timeout.
		resp, err := httpClient.Do(req)
		if err != nil {
			logger.Error("Network error fetching schema override.", "url", uri, "error", err)
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				"Failed to fetch schema from override URL",
				errors.Wrap(err, "httpClient.Do failed"),
			).WithContext("url", uri)
		}
		// Corrected: Check error from resp.Body.Close.
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				logger.Warn("Error closing schema response body.", "url", uri, "error", closeErr)
			}
		}() // Ensure body is closed.

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body) // Read body for logging context.
			logger.Error("Schema override URL returned error status.", "url", uri, "status", resp.Status)
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				fmt.Sprintf("Failed to fetch schema override: HTTP status %d", resp.StatusCode),
				nil,
			).WithContext("url", uri).
				WithContext("statusCode", resp.StatusCode).
				WithContext("responseBody", calculatePreview(bodyBytes))
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Error("Failed to read response body from schema override URL.", "url", uri, "error", err)
			return nil, NewValidationError(
				ErrSchemaLoadFailed,
				"Failed to read schema from override HTTP response",
				errors.Wrap(err, "io.ReadAll failed"),
			).WithContext("url", uri)
		}
		logger.Debug("Successfully downloaded schema override.", "url", uri, "size_bytes", len(data))
		return data, nil

	default:
		logger.Error("Unsupported schema URI scheme.", "uri", uri, "scheme", parsedURI.Scheme)
		return nil, NewValidationError(
			ErrSchemaLoadFailed,
			fmt.Sprintf("Unsupported schema URI scheme: %s", parsedURI.Scheme),
			nil,
		).WithContext("uri", uri)
	}
}

// calculatePreview remains necessary if used by ValidationError logging/context.
// (Implementation is in schema/helpers.go or schema/errors.go).
